package keeper

import (
	"context"
	"fmt"
	"strconv"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/eac/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return &msgServer{k: k} }

var _ types.MsgServer = (*msgServer)(nil)

// ---- helpers --------------------------------------------------------------

func (s msgServer) onlyAuthority(addr string) error {
	if addr != s.k.authority {
		return fmt.Errorf("expected authority %s, got %s", s.k.authority, addr)
	}
	return nil
}

func (s msgServer) requireIssuerAdmin(ctx context.Context, issuerID, admin string) (types.Issuer, error) {
	is, err := s.k.Issuers.Get(ctx, issuerID)
	if err != nil {
		return types.Issuer{}, fmt.Errorf("issuer %q not found", issuerID)
	}
	if is.Admin != admin {
		return types.Issuer{}, fmt.Errorf("only issuer admin (%s) may perform this action", is.Admin)
	}
	return is, nil
}

// ---- Issuer ---------------------------------------------------------------

func (s msgServer) RegisterIssuer(ctx context.Context, m *types.MsgRegisterIssuer) (*types.MsgRegisterIssuerResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.onlyAuthority(m.Authority); err != nil {
		return nil, err
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if uint32(len(m.Kinds)) > params.MaxKindsPerIssuer {
		return nil, fmt.Errorf("kinds exceeds max_kinds_per_issuer %d", params.MaxKindsPerIssuer)
	}
	if has, err := s.k.Issuers.Has(ctx, m.Id); err != nil {
		return nil, err
	} else if has {
		return nil, fmt.Errorf("issuer %q already registered", m.Id)
	}
	count, err := s.k.CountIssuers(ctx)
	if err != nil {
		return nil, err
	}
	if count >= params.MaxIssuers {
		return nil, fmt.Errorf("max_issuers reached (%d)", params.MaxIssuers)
	}

	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	is := types.Issuer{
		Id:          m.Id,
		Did:         m.Did,
		DisplayName: m.DisplayName,
		Status:      types.IssuerStatus_ISSUER_STATUS_ACTIVE,
		Kinds:       append([]int32(nil), m.Kinds...),
		Authority:   m.IssuerAuthority,
		Admin:       m.Admin,
		CreatedBy:   m.Authority,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.k.Issuers.Set(ctx, m.Id, is); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, 0, m.Id, "register_issuer", m.Authority, m.Admin,
		fmt.Sprintf("did=%s authority=%s", m.Did, m.IssuerAuthority))
	emit(sdk.UnwrapSDKContext(ctx), "eac_issuer_registered",
		[2]string{"issuer_id", m.Id},
		[2]string{"did", m.Did},
		[2]string{"authority", m.IssuerAuthority},
	)
	s.maybeRegisterERC20(sdk.UnwrapSDKContext(ctx), m.Id)
	return &types.MsgRegisterIssuerResponse{}, nil
}

// maybeRegisterERC20 is the nil-safe hook into x/erc20. Issuer-id
// is namespaced via the "eac" prefix so EAC issuers cannot collide
// with x/bank coin denoms or the stablecoin module's "scn"-
// prefixed denoms. Failure is logged + surfaced as an event but
// MUST NOT roll back the underlying RegisterIssuer message —
// otherwise an EVM-side outage would block all new issuers.
func (s msgServer) maybeRegisterERC20(ctx sdk.Context, issuerID string) {
	if s.k.erc20 == nil {
		return
	}
	hooked := eacIssuerToERC20(issuerID)
	if s.k.erc20.IsDenomRegistered(ctx, hooked) {
		return
	}
	if err := s.k.erc20.CreateNewTokenPair(ctx, hooked); err != nil {
		ctx.Logger().With("module", types.ModuleName).
			Error("auto-register ERC20 TokenPair failed",
				"issuer_id", issuerID, "hooked_denom", hooked, "err", err)
		emit(ctx, "eac_erc20_register_failed",
			[2]string{"issuer_id", issuerID},
			[2]string{"hooked_denom", hooked},
			[2]string{"err", err.Error()},
		)
		return
	}
	emit(ctx, "eac_erc20_registered",
		[2]string{"issuer_id", issuerID},
		[2]string{"hooked_denom", hooked},
	)
}

// eacIssuerToERC20 builds the canonical x/erc20 denom string for an
// EAC issuer. Centralised so off-chain tooling can reproduce the
// mapping without grepping the keeper.
func eacIssuerToERC20(issuerID string) string { return "eac" + issuerID }

func (s msgServer) UpdateIssuer(ctx context.Context, m *types.MsgUpdateIssuer) (*types.MsgUpdateIssuerResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.onlyAuthority(m.Authority); err != nil {
		return nil, err
	}
	is, err := s.k.Issuers.Get(ctx, m.Id)
	if err != nil {
		return nil, fmt.Errorf("issuer %q not found", m.Id)
	}
	if is.Status == types.IssuerStatus_ISSUER_STATUS_REVOKED {
		return nil, fmt.Errorf("issuer %q is revoked and cannot be updated", m.Id)
	}
	if m.DisplayName != "" {
		is.DisplayName = m.DisplayName
	}
	if m.Kinds != nil {
		params, err := s.k.GetParams(ctx)
		if err != nil {
			return nil, err
		}
		if uint32(len(m.Kinds)) > params.MaxKindsPerIssuer {
			return nil, fmt.Errorf("kinds exceeds max_kinds_per_issuer %d", params.MaxKindsPerIssuer)
		}
		is.Kinds = append([]int32(nil), m.Kinds...)
	}
	if m.IssuerAuthority != "" {
		is.Authority = m.IssuerAuthority
	}
	if m.Admin != "" {
		is.Admin = m.Admin
	}
	is.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Issuers.Set(ctx, m.Id, is); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, 0, m.Id, "update_issuer", m.Authority, "", "")
	emit(sdk.UnwrapSDKContext(ctx), "eac_issuer_updated", [2]string{"issuer_id", m.Id})
	return &types.MsgUpdateIssuerResponse{}, nil
}

func (s msgServer) setIssuerStatus(ctx context.Context, authority, id, reason string, st types.IssuerStatus) error {
	if err := s.onlyAuthority(authority); err != nil {
		return err
	}
	is, err := s.k.Issuers.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("issuer %q not found", id)
	}
	if is.Status == types.IssuerStatus_ISSUER_STATUS_REVOKED && st != types.IssuerStatus_ISSUER_STATUS_REVOKED {
		return fmt.Errorf("issuer %q is revoked", id)
	}
	is.Status = st
	is.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Issuers.Set(ctx, id, is); err != nil {
		return err
	}
	s.k.recordAudit(ctx, 0, id, "issuer_status", authority, "",
		fmt.Sprintf("status=%s reason=%s", st, reason))
	emit(sdk.UnwrapSDKContext(ctx), "eac_issuer_status",
		[2]string{"issuer_id", id},
		[2]string{"status", st.String()},
		[2]string{"reason", reason},
	)
	return nil
}

func (s msgServer) SuspendIssuer(ctx context.Context, m *types.MsgSuspendIssuer) (*types.MsgSuspendIssuerResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.setIssuerStatus(ctx, m.Authority, m.Id, m.Reason, types.IssuerStatus_ISSUER_STATUS_SUSPENDED); err != nil {
		return nil, err
	}
	return &types.MsgSuspendIssuerResponse{}, nil
}

func (s msgServer) RevokeIssuer(ctx context.Context, m *types.MsgRevokeIssuer) (*types.MsgRevokeIssuerResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.setIssuerStatus(ctx, m.Authority, m.Id, m.Reason, types.IssuerStatus_ISSUER_STATUS_REVOKED); err != nil {
		return nil, err
	}
	return &types.MsgRevokeIssuerResponse{}, nil
}

// ---- Certificate ----------------------------------------------------------

func (s msgServer) IssueBatch(ctx context.Context, m *types.MsgIssueBatch) (*types.MsgIssueBatchResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	is, err := s.k.Issuers.Get(ctx, m.IssuerId)
	if err != nil {
		return nil, fmt.Errorf("issuer %q not found", m.IssuerId)
	}
	if is.Authority != m.IssuerAuthority {
		return nil, fmt.Errorf("only issuer authority (%s) may issue", is.Authority)
	}
	if is.Status != types.IssuerStatus_ISSUER_STATUS_ACTIVE {
		return nil, fmt.Errorf("issuer %q is not ACTIVE (status=%s)", is.Id, is.Status)
	}
	if !types.IssuerKindAllowed(is.Kinds, m.Kind) {
		return nil, fmt.Errorf("issuer %q not authorised for kind %s", is.Id, m.Kind)
	}

	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if uint64(m.Units) > uint64(params.MaxUnitsPerBatch) {
		return nil, fmt.Errorf("units %d exceeds max_units_per_batch %d", m.Units, params.MaxUnitsPerBatch)
	}
	if params.HourWindowSeconds > 0 {
		if uint64(m.HourEnd-m.HourStart) != uint64(params.HourWindowSeconds) {
			return nil, fmt.Errorf("hour window must be exactly %d seconds", params.HourWindowSeconds)
		}
	}

	// Defense-in-depth: refuse to mint to a sanctioned recipient at
	// issuance time. This stops a compromised issuer authority from
	// laundering tokens through a sanctioned wallet.
	if params.RequireSanctionsClear && s.k.SanctionsHook() != nil {
		if s.k.SanctionsHook().IsSanctioned(sdk.UnwrapSDKContext(ctx), m.Recipient) {
			return nil, fmt.Errorf("recipient %s is on the sanctions list", m.Recipient)
		}
	}

	// Reject double-issuance of the same off-chain serial. NATIVE kind
	// is allowed to omit the serial; the index is keyed only when
	// source_serial is non-empty.
	if m.SourceSerial != "" {
		key := collections.Join3(int32(m.Kind), m.SourceRegistry, m.SourceSerial)
		if has, err := s.k.SourceSerialIndex.Has(ctx, key); err != nil {
			return nil, err
		} else if has {
			return nil, fmt.Errorf("source serial already issued (kind=%s registry=%s serial=%s)",
				m.Kind, m.SourceRegistry, m.SourceSerial)
		}
	}

	id, err := s.k.NextCertID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	cert := types.Certificate{
		Id:             id,
		IssuerId:       m.IssuerId,
		Kind:           m.Kind,
		Technology:     m.Technology,
		ProjectId:      m.ProjectId,
		DeviceId:       m.DeviceId,
		GridZone:       m.GridZone,
		HourStart:      m.HourStart,
		HourEnd:        m.HourEnd,
		VintageYear:    m.VintageYear,
		SourceRegistry: m.SourceRegistry,
		SourceSerial:   m.SourceSerial,
		IssuedUnits:    m.Units,
		RetiredUnits:   0,
		Status:         types.CertificateStatus_CERTIFICATE_STATUS_ACTIVE,
		PolicyId:       m.PolicyId,
		CreatedBy:      m.IssuerAuthority,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.k.Certificates.Set(ctx, id, cert); err != nil {
		return nil, err
	}
	if err := s.k.CertByIssuer.Set(ctx, collections.Join(m.IssuerId, id)); err != nil {
		return nil, err
	}
	if m.SourceSerial != "" {
		key := collections.Join3(int32(m.Kind), m.SourceRegistry, m.SourceSerial)
		if err := s.k.SourceSerialIndex.Set(ctx, key, id); err != nil {
			return nil, err
		}
	}
	if err := s.k.creditBalance(ctx, id, m.Recipient, m.Units); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, id, m.IssuerId, "issue_batch", m.IssuerAuthority, m.Recipient,
		fmt.Sprintf("kind=%s units=%d project=%s", m.Kind, m.Units, m.ProjectId))
	emit(sdk.UnwrapSDKContext(ctx), "eac_certificate_issued",
		[2]string{"certificate_id", strconv.FormatUint(id, 10)},
		[2]string{"issuer_id", m.IssuerId},
		[2]string{"kind", m.Kind.String()},
		[2]string{"units", strconv.FormatUint(m.Units, 10)},
		[2]string{"recipient", m.Recipient},
	)
	return &types.MsgIssueBatchResponse{CertificateId: id}, nil
}

func (s msgServer) SealCertificate(ctx context.Context, m *types.MsgSealCertificate) (*types.MsgSealCertificateResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	cert, err := s.k.Certificates.Get(ctx, m.CertificateId)
	if err != nil {
		return nil, fmt.Errorf("certificate %d not found", m.CertificateId)
	}
	if _, err := s.requireIssuerAdmin(ctx, cert.IssuerId, m.Admin); err != nil {
		return nil, err
	}
	if cert.Status == types.CertificateStatus_CERTIFICATE_STATUS_FULLY_RETIRED {
		return nil, fmt.Errorf("certificate %d is fully retired", m.CertificateId)
	}
	cert.Status = types.CertificateStatus_CERTIFICATE_STATUS_SEALED
	cert.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Certificates.Set(ctx, m.CertificateId, cert); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, m.CertificateId, cert.IssuerId, "seal", m.Admin, "", m.Reason)
	emit(sdk.UnwrapSDKContext(ctx), "eac_certificate_sealed",
		[2]string{"certificate_id", strconv.FormatUint(m.CertificateId, 10)},
		[2]string{"reason", m.Reason},
	)
	return &types.MsgSealCertificateResponse{}, nil
}

func (s msgServer) UnsealCertificate(ctx context.Context, m *types.MsgUnsealCertificate) (*types.MsgUnsealCertificateResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	cert, err := s.k.Certificates.Get(ctx, m.CertificateId)
	if err != nil {
		return nil, fmt.Errorf("certificate %d not found", m.CertificateId)
	}
	if _, err := s.requireIssuerAdmin(ctx, cert.IssuerId, m.Admin); err != nil {
		return nil, err
	}
	if cert.Status != types.CertificateStatus_CERTIFICATE_STATUS_SEALED {
		return nil, fmt.Errorf("certificate %d is not sealed", m.CertificateId)
	}
	cert.Status = types.CertificateStatus_CERTIFICATE_STATUS_ACTIVE
	cert.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Certificates.Set(ctx, m.CertificateId, cert); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, m.CertificateId, cert.IssuerId, "unseal", m.Admin, "", m.Reason)
	emit(sdk.UnwrapSDKContext(ctx), "eac_certificate_unsealed",
		[2]string{"certificate_id", strconv.FormatUint(m.CertificateId, 10)},
	)
	return &types.MsgUnsealCertificateResponse{}, nil
}

func (s msgServer) Transfer(ctx context.Context, m *types.MsgTransfer) (*types.MsgTransferResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	cert, err := s.k.Certificates.Get(ctx, m.CertificateId)
	if err != nil {
		return nil, fmt.Errorf("certificate %d not found", m.CertificateId)
	}
	if cert.Status != types.CertificateStatus_CERTIFICATE_STATUS_ACTIVE {
		return nil, fmt.Errorf("certificate %d is not transferable (status=%s)", m.CertificateId, cert.Status)
	}
	if err := s.k.transferCompliance(ctx, m.CertificateId, m.From, m.To, m.Units); err != nil {
		return nil, err
	}
	if err := s.k.moveBalance(ctx, m.CertificateId, m.From, m.To, m.Units); err != nil {
		return nil, err
	}
	emit(sdk.UnwrapSDKContext(ctx), "eac_transfer",
		[2]string{"certificate_id", strconv.FormatUint(m.CertificateId, 10)},
		[2]string{"from", m.From},
		[2]string{"to", m.To},
		[2]string{"units", strconv.FormatUint(m.Units, 10)},
	)
	return &types.MsgTransferResponse{}, nil
}

func (s msgServer) Retire(ctx context.Context, m *types.MsgRetire) (*types.MsgRetireResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	cert, err := s.k.Certificates.Get(ctx, m.CertificateId)
	if err != nil {
		return nil, fmt.Errorf("certificate %d not found", m.CertificateId)
	}
	// Sealed certs may still be retired by the holder so compliance
	// liquidations are not blocked. Only FULLY_RETIRED is terminal.
	if cert.Status == types.CertificateStatus_CERTIFICATE_STATUS_FULLY_RETIRED {
		return nil, fmt.Errorf("certificate %d is fully retired", m.CertificateId)
	}
	beneficiary := m.Beneficiary
	if beneficiary == "" {
		beneficiary = m.Retirer
	}
	// Compliance: sender = holder, receiver = beneficiary. For
	// proxy retire the beneficiary must also clear sanctions so that
	// the immutable record can't credit a sanctioned party.
	if err := s.k.transferCompliance(ctx, m.CertificateId, m.Retirer, beneficiary, m.Units); err != nil {
		return nil, err
	}

	if err := s.k.debitBalance(ctx, m.CertificateId, m.Retirer, m.Units); err != nil {
		return nil, err
	}
	newRetired, err := types.SafeAdd(cert.RetiredUnits, m.Units)
	if err != nil {
		return nil, err
	}
	if newRetired > cert.IssuedUnits {
		return nil, fmt.Errorf("retire would exceed issued (issued=%d, retired+new=%d)", cert.IssuedUnits, newRetired)
	}
	cert.RetiredUnits = newRetired
	if cert.RetiredUnits == cert.IssuedUnits {
		cert.Status = types.CertificateStatus_CERTIFICATE_STATUS_FULLY_RETIRED
	}
	cert.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Certificates.Set(ctx, m.CertificateId, cert); err != nil {
		return nil, err
	}

	id, err := s.k.NextRetirementID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	r := types.Retirement{
		Id:            id,
		CertificateId: m.CertificateId,
		Retirer:       m.Retirer,
		Beneficiary:   beneficiary,
		Amount:        m.Units,
		Purpose:       m.Purpose,
		Memo:          m.Memo,
		RetiredAt:     now,
		Height:        sdk.UnwrapSDKContext(ctx).BlockHeight(),
	}
	if err := s.k.Retirements.Set(ctx, id, r); err != nil {
		return nil, err
	}
	if err := s.k.RetirementByBeneficiary.Set(ctx, collections.Join(beneficiary, id)); err != nil {
		return nil, err
	}
	if err := s.k.RetirementByCertificate.Set(ctx, collections.Join(m.CertificateId, id)); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, m.CertificateId, cert.IssuerId, "retire", m.Retirer, beneficiary,
		fmt.Sprintf("units=%d purpose=%s", m.Units, m.Purpose))
	emit(sdk.UnwrapSDKContext(ctx), "eac_retired",
		[2]string{"retirement_id", strconv.FormatUint(id, 10)},
		[2]string{"certificate_id", strconv.FormatUint(m.CertificateId, 10)},
		[2]string{"retirer", m.Retirer},
		[2]string{"beneficiary", beneficiary},
		[2]string{"units", strconv.FormatUint(m.Units, 10)},
		[2]string{"purpose", m.Purpose},
	)
	return &types.MsgRetireResponse{RetirementId: id}, nil
}

// ---- Bridge --------------------------------------------------------------

func (s msgServer) SetBridgeAttestation(ctx context.Context, m *types.MsgSetBridgeAttestation) (*types.MsgSetBridgeAttestationResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.onlyAuthority(m.Authority); err != nil {
		return nil, err
	}
	cert, err := s.k.Certificates.Get(ctx, m.CertificateId)
	if err != nil {
		return nil, fmt.Errorf("certificate %d not found", m.CertificateId)
	}
	br := types.BridgeAttestation{
		CertificateId:       m.CertificateId,
		SourceRegistry:      cert.SourceRegistry,
		SourceSerial:        cert.SourceSerial,
		OracleTopicId:       m.OracleTopicId,
		MaxStalenessSeconds: m.MaxStalenessSeconds,
		UpdatedAt:           sdk.UnwrapSDKContext(ctx).BlockTime().Unix(),
	}
	if err := s.k.Bridges.Set(ctx, m.CertificateId, br); err != nil {
		return nil, err
	}
	emit(sdk.UnwrapSDKContext(ctx), "eac_bridge_attestation_set",
		[2]string{"certificate_id", strconv.FormatUint(m.CertificateId, 10)},
		[2]string{"oracle_topic_id", m.OracleTopicId},
	)
	return &types.MsgSetBridgeAttestationResponse{}, nil
}

func (s msgServer) BridgeMint(ctx context.Context, m *types.MsgBridgeMint) (*types.MsgBridgeMintResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	cert, err := s.k.Certificates.Get(ctx, m.CertificateId)
	if err != nil {
		return nil, fmt.Errorf("certificate %d not found", m.CertificateId)
	}
	if cert.Status != types.CertificateStatus_CERTIFICATE_STATUS_ACTIVE {
		return nil, fmt.Errorf("certificate %d is not ACTIVE (status=%s)", m.CertificateId, cert.Status)
	}
	is, err := s.k.Issuers.Get(ctx, cert.IssuerId)
	if err != nil {
		return nil, fmt.Errorf("issuer %q not found", cert.IssuerId)
	}
	if is.Authority != m.IssuerAuthority {
		return nil, fmt.Errorf("only issuer authority (%s) may bridge-mint", is.Authority)
	}
	if is.Status != types.IssuerStatus_ISSUER_STATUS_ACTIVE {
		return nil, fmt.Errorf("issuer %q is not ACTIVE", is.Id)
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if params.RequireSanctionsClear && s.k.SanctionsHook() != nil {
		if s.k.SanctionsHook().IsSanctioned(sdk.UnwrapSDKContext(ctx), m.Recipient) {
			return nil, fmt.Errorf("recipient %s is on the sanctions list", m.Recipient)
		}
	}
	if err := s.k.CheckBridgeMintCoverage(ctx, cert, m.Units); err != nil {
		return nil, err
	}
	newIssued, err := types.SafeAdd(cert.IssuedUnits, m.Units)
	if err != nil {
		return nil, err
	}
	cert.IssuedUnits = newIssued
	cert.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Certificates.Set(ctx, m.CertificateId, cert); err != nil {
		return nil, err
	}
	if err := s.k.creditBalance(ctx, m.CertificateId, m.Recipient, m.Units); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, m.CertificateId, cert.IssuerId, "bridge_mint", m.IssuerAuthority, m.Recipient,
		fmt.Sprintf("units=%d new_issued=%d", m.Units, newIssued))
	emit(sdk.UnwrapSDKContext(ctx), "eac_bridge_minted",
		[2]string{"certificate_id", strconv.FormatUint(m.CertificateId, 10)},
		[2]string{"units", strconv.FormatUint(m.Units, 10)},
		[2]string{"recipient", m.Recipient},
	)
	return &types.MsgBridgeMintResponse{NewIssuedUnits: newIssued}, nil
}

func (s msgServer) BridgeBurn(ctx context.Context, m *types.MsgBridgeBurn) (*types.MsgBridgeBurnResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	cert, err := s.k.Certificates.Get(ctx, m.CertificateId)
	if err != nil {
		return nil, fmt.Errorf("certificate %d not found", m.CertificateId)
	}
	if cert.Status == types.CertificateStatus_CERTIFICATE_STATUS_FULLY_RETIRED {
		return nil, fmt.Errorf("certificate %d is fully retired", m.CertificateId)
	}
	// BridgeBurn requires a registered bridge attestation so the
	// off-chain oracle has a topic to listen on. If absent, the
	// burn would be unrecoverable on the upstream registry.
	if has, err := s.k.Bridges.Has(ctx, m.CertificateId); err != nil {
		return nil, err
	} else if !has {
		return nil, fmt.Errorf("certificate %d has no bridge attestation; cannot bridge-burn", m.CertificateId)
	}
	if err := s.k.debitBalance(ctx, m.CertificateId, m.Holder, m.Units); err != nil {
		return nil, err
	}
	newIssued, err := types.SafeSub(cert.IssuedUnits, m.Units)
	if err != nil {
		return nil, err
	}
	if newIssued < cert.RetiredUnits {
		return nil, fmt.Errorf("bridge burn would push issued (%d) below retired (%d)", newIssued, cert.RetiredUnits)
	}
	cert.IssuedUnits = newIssued
	cert.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Certificates.Set(ctx, m.CertificateId, cert); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, m.CertificateId, cert.IssuerId, "bridge_burn", m.Holder, m.ExternalRecipient,
		fmt.Sprintf("units=%d external=%s", m.Units, m.ExternalRecipient))
	emit(sdk.UnwrapSDKContext(ctx), "eac_bridge_burned",
		[2]string{"certificate_id", strconv.FormatUint(m.CertificateId, 10)},
		[2]string{"holder", m.Holder},
		[2]string{"units", strconv.FormatUint(m.Units, 10)},
		[2]string{"external_recipient", m.ExternalRecipient},
	)
	return &types.MsgBridgeBurnResponse{}, nil
}

// ---- Params --------------------------------------------------------------

func (s msgServer) UpdateParams(ctx context.Context, m *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.onlyAuthority(m.Authority); err != nil {
		return nil, err
	}
	if err := s.k.SetParams(ctx, m.Params); err != nil {
		return nil, err
	}
	emit(sdk.UnwrapSDKContext(ctx), "eac_params_updated")
	return &types.MsgUpdateParamsResponse{}, nil
}

