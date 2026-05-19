package keeper

import (
	"context"
	"fmt"
	"strconv"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/carbon/types"
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

// recipientSanctionsCheck runs the receiver-side sanctions gate for
// issuance / bridge-mint flows that don't go through transferCompliance.
func (s msgServer) recipientSanctionsCheck(ctx context.Context, params types.Params, addr string) error {
	if !params.RequireSanctionsClear || s.k.SanctionsHook() == nil {
		return nil
	}
	if s.k.SanctionsHook().IsSanctioned(sdk.UnwrapSDKContext(ctx), addr) {
		return fmt.Errorf("recipient %s is on the sanctions list", addr)
	}
	return nil
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
	if uint32(len(m.Categories)) > params.MaxCategoriesPerIssuer {
		return nil, fmt.Errorf("categories exceeds max_categories_per_issuer %d", params.MaxCategoriesPerIssuer)
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
		Categories:  append([]int32(nil), m.Categories...),
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
	emit(sdk.UnwrapSDKContext(ctx), "carbon_issuer_registered",
		[2]string{"issuer_id", m.Id},
		[2]string{"did", m.Did},
		[2]string{"authority", m.IssuerAuthority},
	)
	s.maybeRegisterERC20(sdk.UnwrapSDKContext(ctx), m.Id)
	return &types.MsgRegisterIssuerResponse{}, nil
}

// maybeRegisterERC20 is the nil-safe hook into x/erc20. The issuer
// ID is namespaced via the "carbon" prefix so it cannot collide
// with x/bank denoms, stablecoin "scn"-prefixed denoms, or EAC
// "eac"-prefixed denoms. Failure is logged and surfaced as an
// event but MUST NOT roll back the underlying RegisterIssuer
// message.
func (s msgServer) maybeRegisterERC20(ctx sdk.Context, issuerID string) {
	if s.k.erc20 == nil {
		return
	}
	hooked := carbonIssuerToERC20(issuerID)
	if s.k.erc20.IsDenomRegistered(ctx, hooked) {
		return
	}
	if err := s.k.erc20.CreateNewTokenPair(ctx, hooked); err != nil {
		ctx.Logger().With("module", types.ModuleName).
			Error("auto-register ERC20 TokenPair failed",
				"issuer_id", issuerID, "hooked_denom", hooked, "err", err)
		emit(ctx, "carbon_erc20_register_failed",
			[2]string{"issuer_id", issuerID},
			[2]string{"hooked_denom", hooked},
			[2]string{"err", err.Error()},
		)
		return
	}
	emit(ctx, "carbon_erc20_registered",
		[2]string{"issuer_id", issuerID},
		[2]string{"hooked_denom", hooked},
	)
}

// carbonIssuerToERC20 builds the canonical x/erc20 denom string for
// a carbon issuer. Centralised so off-chain tooling can reproduce
// the mapping without grepping the keeper.
func carbonIssuerToERC20(issuerID string) string { return "carbon" + issuerID }

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
	if m.Categories != nil {
		params, err := s.k.GetParams(ctx)
		if err != nil {
			return nil, err
		}
		if uint32(len(m.Categories)) > params.MaxCategoriesPerIssuer {
			return nil, fmt.Errorf("categories exceeds max_categories_per_issuer %d", params.MaxCategoriesPerIssuer)
		}
		is.Categories = append([]int32(nil), m.Categories...)
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
	emit(sdk.UnwrapSDKContext(ctx), "carbon_issuer_updated", [2]string{"issuer_id", m.Id})
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
	emit(sdk.UnwrapSDKContext(ctx), "carbon_issuer_status",
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

// ---- Issuance helpers -----------------------------------------------------

// commonIssue applies all the gates / writes shared by IssueAllowance
// and IssueOffset. Returns the new asset id.
func (s msgServer) commonIssue(ctx context.Context, asset types.Asset, recipient string) (uint64, error) {
	is, err := s.k.Issuers.Get(ctx, asset.IssuerId)
	if err != nil {
		return 0, fmt.Errorf("issuer %q not found", asset.IssuerId)
	}
	if is.Authority != asset.CreatedBy {
		return 0, fmt.Errorf("only issuer authority (%s) may issue", is.Authority)
	}
	if is.Status != types.IssuerStatus_ISSUER_STATUS_ACTIVE {
		return 0, fmt.Errorf("issuer %q is not ACTIVE (status=%s)", is.Id, is.Status)
	}
	if !types.IssuerCategoryAllowed(is.Categories, asset.Category) {
		return 0, fmt.Errorf("issuer %q not authorised for category %s", is.Id, asset.Category)
	}

	params, err := s.k.GetParams(ctx)
	if err != nil {
		return 0, err
	}
	if uint64(asset.IssuedUnits) > uint64(params.MaxUnitsPerBatch) {
		return 0, fmt.Errorf("units %d exceeds max_units_per_batch %d", asset.IssuedUnits, params.MaxUnitsPerBatch)
	}
	if err := types.ValidateCCPLabels(asset.CcpLabels, params.MaxCcpLabels); err != nil {
		return 0, err
	}
	if err := s.recipientSanctionsCheck(ctx, params, recipient); err != nil {
		return 0, err
	}

	// EAC mutual exclusion: only OFFSET assets can carry the link.
	// Reserve units up front so two parallel issuances sharing the
	// same EAC certificate can't both squeak past the cap.
	if asset.LinkedEacCertificateId != 0 {
		if asset.Category != types.AssetCategory_ASSET_CATEGORY_OFFSET {
			return 0, fmt.Errorf("linked_eac_certificate_id only valid for OFFSET")
		}
		if err := s.k.reserveEACClaim(ctx, asset.LinkedEacCertificateId, asset.IssuedUnits); err != nil {
			return 0, err
		}
	}

	if asset.SourceSerial != "" {
		key := collections.Join3(int32(asset.Category), asset.SourceRegistry, asset.SourceSerial)
		if has, err := s.k.SourceSerialIndex.Has(ctx, key); err != nil {
			return 0, err
		} else if has {
			return 0, fmt.Errorf("source serial already issued (category=%s registry=%s serial=%s)",
				asset.Category, asset.SourceRegistry, asset.SourceSerial)
		}
	}

	id, err := s.k.NextAssetID(ctx)
	if err != nil {
		return 0, err
	}
	asset.Id = id
	asset.RetiredUnits = 0
	asset.Status = types.AssetStatus_ASSET_STATUS_ACTIVE
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	asset.CreatedAt = now
	asset.UpdatedAt = now
	if err := s.k.Assets.Set(ctx, id, asset); err != nil {
		return 0, err
	}
	if err := s.k.AssetByIssuer.Set(ctx, collections.Join(asset.IssuerId, id)); err != nil {
		return 0, err
	}
	if asset.SourceSerial != "" {
		key := collections.Join3(int32(asset.Category), asset.SourceRegistry, asset.SourceSerial)
		if err := s.k.SourceSerialIndex.Set(ctx, key, id); err != nil {
			return 0, err
		}
	}
	if err := s.k.creditBalance(ctx, id, recipient, asset.IssuedUnits); err != nil {
		return 0, err
	}
	return id, nil
}

func (s msgServer) IssueAllowance(ctx context.Context, m *types.MsgIssueAllowance) (*types.MsgIssueAllowanceResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	asset := types.Asset{
		IssuerId:       m.IssuerId,
		Category:       types.AssetCategory_ASSET_CATEGORY_ALLOWANCE,
		Registry:       m.Registry,
		Program:        m.Program,
		Jurisdiction:   m.Jurisdiction,
		VintageYear:    m.VintageYear,
		SourceRegistry: m.SourceRegistry,
		SourceSerial:   m.SourceSerial,
		Article6Status: types.Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE,
		IssuedUnits:    m.Units,
		PolicyId:       m.PolicyId,
		CreatedBy:      m.IssuerAuthority,
	}
	id, err := s.commonIssue(ctx, asset, m.Recipient)
	if err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, id, m.IssuerId, "issue_allowance", m.IssuerAuthority, m.Recipient,
		fmt.Sprintf("program=%s units=%d jurisdiction=%s", m.Program, m.Units, m.Jurisdiction))
	emit(sdk.UnwrapSDKContext(ctx), "carbon_allowance_issued",
		[2]string{"asset_id", strconv.FormatUint(id, 10)},
		[2]string{"issuer_id", m.IssuerId},
		[2]string{"program", m.Program},
		[2]string{"units", strconv.FormatUint(m.Units, 10)},
		[2]string{"recipient", m.Recipient},
	)
	return &types.MsgIssueAllowanceResponse{AssetId: id}, nil
}

func (s msgServer) IssueOffset(ctx context.Context, m *types.MsgIssueOffset) (*types.MsgIssueOffsetResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	asset := types.Asset{
		IssuerId:               m.IssuerId,
		Category:               types.AssetCategory_ASSET_CATEGORY_OFFSET,
		Registry:               m.Registry,
		Program:                m.Program,
		ProjectId:              m.ProjectId,
		Methodology:            m.Methodology,
		Jurisdiction:           m.Jurisdiction,
		VintageYear:            m.VintageYear,
		SourceRegistry:         m.SourceRegistry,
		SourceSerial:           m.SourceSerial,
		CcpLabels:              append([]string(nil), m.CcpLabels...),
		Article6Status:         m.Article6Status,
		HostCountry:            m.HostCountry,
		RecipientCountry:       m.RecipientCountry,
		LinkedEacCertificateId: m.LinkedEacCertificateId,
		IssuedUnits:            m.Units,
		PolicyId:               m.PolicyId,
		CreatedBy:              m.IssuerAuthority,
	}
	id, err := s.commonIssue(ctx, asset, m.Recipient)
	if err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, id, m.IssuerId, "issue_offset", m.IssuerAuthority, m.Recipient,
		fmt.Sprintf("project=%s methodology=%s units=%d a6=%s linked_eac=%d",
			m.ProjectId, m.Methodology, m.Units, m.Article6Status, m.LinkedEacCertificateId))
	emit(sdk.UnwrapSDKContext(ctx), "carbon_offset_issued",
		[2]string{"asset_id", strconv.FormatUint(id, 10)},
		[2]string{"issuer_id", m.IssuerId},
		[2]string{"project_id", m.ProjectId},
		[2]string{"units", strconv.FormatUint(m.Units, 10)},
		[2]string{"article6_status", m.Article6Status.String()},
	)
	return &types.MsgIssueOffsetResponse{AssetId: id}, nil
}

// ---- Seal -----------------------------------------------------------------

func (s msgServer) SealAsset(ctx context.Context, m *types.MsgSealAsset) (*types.MsgSealAssetResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	asset, err := s.k.Assets.Get(ctx, m.AssetId)
	if err != nil {
		return nil, fmt.Errorf("asset %d not found", m.AssetId)
	}
	if _, err := s.requireIssuerAdmin(ctx, asset.IssuerId, m.Admin); err != nil {
		return nil, err
	}
	if asset.Status == types.AssetStatus_ASSET_STATUS_FULLY_RETIRED {
		return nil, fmt.Errorf("asset %d is fully retired", m.AssetId)
	}
	asset.Status = types.AssetStatus_ASSET_STATUS_SEALED
	asset.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Assets.Set(ctx, m.AssetId, asset); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, m.AssetId, asset.IssuerId, "seal", m.Admin, "", m.Reason)
	emit(sdk.UnwrapSDKContext(ctx), "carbon_asset_sealed",
		[2]string{"asset_id", strconv.FormatUint(m.AssetId, 10)},
		[2]string{"reason", m.Reason},
	)
	return &types.MsgSealAssetResponse{}, nil
}

func (s msgServer) UnsealAsset(ctx context.Context, m *types.MsgUnsealAsset) (*types.MsgUnsealAssetResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	asset, err := s.k.Assets.Get(ctx, m.AssetId)
	if err != nil {
		return nil, fmt.Errorf("asset %d not found", m.AssetId)
	}
	if _, err := s.requireIssuerAdmin(ctx, asset.IssuerId, m.Admin); err != nil {
		return nil, err
	}
	if asset.Status != types.AssetStatus_ASSET_STATUS_SEALED {
		return nil, fmt.Errorf("asset %d is not sealed", m.AssetId)
	}
	asset.Status = types.AssetStatus_ASSET_STATUS_ACTIVE
	asset.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Assets.Set(ctx, m.AssetId, asset); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, m.AssetId, asset.IssuerId, "unseal", m.Admin, "", m.Reason)
	emit(sdk.UnwrapSDKContext(ctx), "carbon_asset_unsealed",
		[2]string{"asset_id", strconv.FormatUint(m.AssetId, 10)},
	)
	return &types.MsgUnsealAssetResponse{}, nil
}

// ---- Transfer / Retire ---------------------------------------------------

func (s msgServer) Transfer(ctx context.Context, m *types.MsgTransfer) (*types.MsgTransferResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	asset, err := s.k.Assets.Get(ctx, m.AssetId)
	if err != nil {
		return nil, fmt.Errorf("asset %d not found", m.AssetId)
	}
	if asset.Status != types.AssetStatus_ASSET_STATUS_ACTIVE {
		return nil, fmt.Errorf("asset %d is not transferable (status=%s)", m.AssetId, asset.Status)
	}
	if err := s.k.transferCompliance(ctx, m.AssetId, m.From, m.To, m.Units); err != nil {
		return nil, err
	}
	if err := s.k.moveBalance(ctx, m.AssetId, m.From, m.To, m.Units); err != nil {
		return nil, err
	}
	emit(sdk.UnwrapSDKContext(ctx), "carbon_transfer",
		[2]string{"asset_id", strconv.FormatUint(m.AssetId, 10)},
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
	asset, err := s.k.Assets.Get(ctx, m.AssetId)
	if err != nil {
		return nil, fmt.Errorf("asset %d not found", m.AssetId)
	}
	if asset.Status == types.AssetStatus_ASSET_STATUS_FULLY_RETIRED {
		return nil, fmt.Errorf("asset %d is fully retired", m.AssetId)
	}
	beneficiary := m.Beneficiary
	if beneficiary == "" {
		beneficiary = m.Retirer
	}
	if err := s.k.transferCompliance(ctx, m.AssetId, m.Retirer, beneficiary, m.Units); err != nil {
		return nil, err
	}

	// Article 6: when crossing borders, require CA_APPLIED.
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if params.RequireArticle6ForCrossBorder &&
		asset.Category == types.AssetCategory_ASSET_CATEGORY_OFFSET &&
		asset.HostCountry != "" {
		// The asset is internationally scoped (host country recorded);
		// the retiree MUST commit to a jurisdiction. Otherwise a
		// caller could simply omit the field and silently bypass the
		// CA gate when host != beneficiary's country.
		if m.BeneficiaryJurisdiction == "" {
			return nil, fmt.Errorf(
				"asset %d has host_country=%s; retire requires beneficiary_jurisdiction",
				m.AssetId, asset.HostCountry)
		}
		if asset.HostCountry != m.BeneficiaryJurisdiction &&
			asset.Article6Status != types.Article6Status_ARTICLE6_STATUS_CA_APPLIED {
			return nil, fmt.Errorf(
				"cross-border retire requires article6_status=CA_APPLIED (host=%s, beneficiary_jurisdiction=%s, current=%s)",
				asset.HostCountry, m.BeneficiaryJurisdiction, asset.Article6Status)
		}
	}

	if err := s.k.debitBalance(ctx, m.AssetId, m.Retirer, m.Units); err != nil {
		return nil, err
	}
	newRetired, err := types.SafeAdd(asset.RetiredUnits, m.Units)
	if err != nil {
		return nil, err
	}
	if newRetired > asset.IssuedUnits {
		return nil, fmt.Errorf("retire would exceed issued (issued=%d, retired+new=%d)", asset.IssuedUnits, newRetired)
	}
	asset.RetiredUnits = newRetired
	if asset.RetiredUnits == asset.IssuedUnits {
		asset.Status = types.AssetStatus_ASSET_STATUS_FULLY_RETIRED
	}
	asset.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Assets.Set(ctx, m.AssetId, asset); err != nil {
		return nil, err
	}

	id, err := s.k.NextRetirementID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	r := types.Retirement{
		Id:                     id,
		AssetId:                m.AssetId,
		Retirer:                m.Retirer,
		Beneficiary:            beneficiary,
		Amount:                 m.Units,
		Purpose:                m.Purpose,
		Claim:                  m.Claim,
		Memo:                   m.Memo,
		Article6StatusAtRetire: asset.Article6Status,
		RetiredAt:              now,
		Height:                 sdk.UnwrapSDKContext(ctx).BlockHeight(),
	}
	if err := s.k.Retirements.Set(ctx, id, r); err != nil {
		return nil, err
	}
	if err := s.k.RetirementByBeneficiary.Set(ctx, collections.Join(beneficiary, id)); err != nil {
		return nil, err
	}
	if err := s.k.RetirementByAsset.Set(ctx, collections.Join(m.AssetId, id)); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, m.AssetId, asset.IssuerId, "retire", m.Retirer, beneficiary,
		fmt.Sprintf("units=%d purpose=%s claim=%s a6=%s", m.Units, m.Purpose, m.Claim, asset.Article6Status))
	emit(sdk.UnwrapSDKContext(ctx), "carbon_retired",
		[2]string{"retirement_id", strconv.FormatUint(id, 10)},
		[2]string{"asset_id", strconv.FormatUint(m.AssetId, 10)},
		[2]string{"retirer", m.Retirer},
		[2]string{"beneficiary", beneficiary},
		[2]string{"units", strconv.FormatUint(m.Units, 10)},
		[2]string{"purpose", m.Purpose},
		[2]string{"article6_status", asset.Article6Status.String()},
	)
	return &types.MsgRetireResponse{RetirementId: id}, nil
}

// ---- Article 6 -----------------------------------------------------------

func (s msgServer) SetArticle6Status(ctx context.Context, m *types.MsgSetArticle6Status) (*types.MsgSetArticle6StatusResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	asset, err := s.k.Assets.Get(ctx, m.AssetId)
	if err != nil {
		return nil, fmt.Errorf("asset %d not found", m.AssetId)
	}
	if asset.Category != types.AssetCategory_ASSET_CATEGORY_OFFSET {
		return nil, fmt.Errorf("article6 status only applies to OFFSET assets")
	}
	if _, err := s.requireIssuerAdmin(ctx, asset.IssuerId, m.Admin); err != nil {
		return nil, err
	}
	asset.Article6Status = m.Status
	asset.HostCountry = m.HostCountry
	asset.RecipientCountry = m.RecipientCountry
	asset.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Assets.Set(ctx, m.AssetId, asset); err != nil {
		return nil, err
	}

	if m.Status == types.Article6Status_ARTICLE6_STATUS_AUTHORIZED ||
		m.Status == types.Article6Status_ARTICLE6_STATUS_CA_APPLIED {
		auth := types.Article6Authorization{
			AssetId:          m.AssetId,
			HostCountry:      m.HostCountry,
			RecipientCountry: m.RecipientCountry,
			DocumentUri:      m.DocumentUri,
			DocumentHash:     m.DocumentHash,
			AuthorizedAt:     sdk.UnwrapSDKContext(ctx).BlockTime().Unix(),
		}
		if err := s.k.Article6Authorizations.Set(ctx, m.AssetId, auth); err != nil {
			return nil, err
		}
	} else {
		// Downgrading status invalidates any prior authorisation row;
		// removing it prevents stale documents from outliving the
		// status they backed and confusing MRV consumers.
		if has, err := s.k.Article6Authorizations.Has(ctx, m.AssetId); err != nil {
			return nil, err
		} else if has {
			if err := s.k.Article6Authorizations.Remove(ctx, m.AssetId); err != nil {
				return nil, err
			}
		}
	}

	s.k.recordAudit(ctx, m.AssetId, asset.IssuerId, "set_article6", m.Admin, "",
		fmt.Sprintf("status=%s host=%s recipient=%s", m.Status, m.HostCountry, m.RecipientCountry))
	emit(sdk.UnwrapSDKContext(ctx), "carbon_article6_set",
		[2]string{"asset_id", strconv.FormatUint(m.AssetId, 10)},
		[2]string{"status", m.Status.String()},
		[2]string{"host_country", m.HostCountry},
		[2]string{"recipient_country", m.RecipientCountry},
	)
	return &types.MsgSetArticle6StatusResponse{}, nil
}

// ---- Bridge --------------------------------------------------------------

func (s msgServer) SetBridgeAttestation(ctx context.Context, m *types.MsgSetBridgeAttestation) (*types.MsgSetBridgeAttestationResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.onlyAuthority(m.Authority); err != nil {
		return nil, err
	}
	asset, err := s.k.Assets.Get(ctx, m.AssetId)
	if err != nil {
		return nil, fmt.Errorf("asset %d not found", m.AssetId)
	}
	br := types.BridgeAttestation{
		AssetId:             m.AssetId,
		SourceRegistry:      asset.SourceRegistry,
		SourceSerial:        asset.SourceSerial,
		OracleTopicId:       m.OracleTopicId,
		MaxStalenessSeconds: m.MaxStalenessSeconds,
		UpdatedAt:           sdk.UnwrapSDKContext(ctx).BlockTime().Unix(),
	}
	if err := s.k.Bridges.Set(ctx, m.AssetId, br); err != nil {
		return nil, err
	}
	emit(sdk.UnwrapSDKContext(ctx), "carbon_bridge_attestation_set",
		[2]string{"asset_id", strconv.FormatUint(m.AssetId, 10)},
		[2]string{"oracle_topic_id", m.OracleTopicId},
	)
	return &types.MsgSetBridgeAttestationResponse{}, nil
}

func (s msgServer) BridgeMint(ctx context.Context, m *types.MsgBridgeMint) (*types.MsgBridgeMintResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	asset, err := s.k.Assets.Get(ctx, m.AssetId)
	if err != nil {
		return nil, fmt.Errorf("asset %d not found", m.AssetId)
	}
	if asset.Status != types.AssetStatus_ASSET_STATUS_ACTIVE {
		return nil, fmt.Errorf("asset %d is not ACTIVE (status=%s)", m.AssetId, asset.Status)
	}
	is, err := s.k.Issuers.Get(ctx, asset.IssuerId)
	if err != nil {
		return nil, fmt.Errorf("issuer %q not found", asset.IssuerId)
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
	if err := s.recipientSanctionsCheck(ctx, params, m.Recipient); err != nil {
		return nil, err
	}
	if err := s.k.CheckBridgeMintCoverage(ctx, asset, m.Units); err != nil {
		return nil, err
	}
	// Bridge mint expands the EAC claim against the same EAC link.
	if asset.LinkedEacCertificateId != 0 {
		if err := s.k.reserveEACClaim(ctx, asset.LinkedEacCertificateId, m.Units); err != nil {
			return nil, err
		}
	}
	newIssued, err := types.SafeAdd(asset.IssuedUnits, m.Units)
	if err != nil {
		return nil, err
	}
	asset.IssuedUnits = newIssued
	asset.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Assets.Set(ctx, m.AssetId, asset); err != nil {
		return nil, err
	}
	if err := s.k.creditBalance(ctx, m.AssetId, m.Recipient, m.Units); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, m.AssetId, asset.IssuerId, "bridge_mint", m.IssuerAuthority, m.Recipient,
		fmt.Sprintf("units=%d new_issued=%d", m.Units, newIssued))
	emit(sdk.UnwrapSDKContext(ctx), "carbon_bridge_minted",
		[2]string{"asset_id", strconv.FormatUint(m.AssetId, 10)},
		[2]string{"units", strconv.FormatUint(m.Units, 10)},
		[2]string{"recipient", m.Recipient},
	)
	return &types.MsgBridgeMintResponse{NewIssuedUnits: newIssued}, nil
}

func (s msgServer) BridgeBurn(ctx context.Context, m *types.MsgBridgeBurn) (*types.MsgBridgeBurnResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	asset, err := s.k.Assets.Get(ctx, m.AssetId)
	if err != nil {
		return nil, fmt.Errorf("asset %d not found", m.AssetId)
	}
	if asset.Status == types.AssetStatus_ASSET_STATUS_FULLY_RETIRED {
		return nil, fmt.Errorf("asset %d is fully retired", m.AssetId)
	}
	if has, err := s.k.Bridges.Has(ctx, m.AssetId); err != nil {
		return nil, err
	} else if !has {
		return nil, fmt.Errorf("asset %d has no bridge attestation; cannot bridge-burn", m.AssetId)
	}
	// Sanctions gate on the holder: bridge burn skips
	// transferCompliance (no on-chain receiver), so we apply the
	// sender check explicitly to keep parity with Retire / Transfer.
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if params.RequireSanctionsClear && s.k.SanctionsHook() != nil {
		if s.k.SanctionsHook().IsSanctioned(sdk.UnwrapSDKContext(ctx), m.Holder) {
			return nil, fmt.Errorf("holder %s is on the sanctions list", m.Holder)
		}
	}
	if err := s.k.debitBalance(ctx, m.AssetId, m.Holder, m.Units); err != nil {
		return nil, err
	}
	newIssued, err := types.SafeSub(asset.IssuedUnits, m.Units)
	if err != nil {
		return nil, err
	}
	if newIssued < asset.RetiredUnits {
		return nil, fmt.Errorf("bridge burn would push issued (%d) below retired (%d)", newIssued, asset.RetiredUnits)
	}
	asset.IssuedUnits = newIssued
	asset.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.Assets.Set(ctx, m.AssetId, asset); err != nil {
		return nil, err
	}
	// Release the EAC claim equal to the burn so the same EAC capacity
	// can be re-claimed once the burn settles upstream.
	if asset.LinkedEacCertificateId != 0 {
		cur, err := s.k.GetEACClaimed(ctx, asset.LinkedEacCertificateId)
		if err != nil {
			return nil, err
		}
		next, err := types.SafeSub(cur, m.Units)
		if err != nil {
			return nil, fmt.Errorf("EAC claim underflow on burn: %w", err)
		}
		if err := s.k.EACOffsetClaimed.Set(ctx, asset.LinkedEacCertificateId, next); err != nil {
			return nil, err
		}
	}
	s.k.recordAudit(ctx, m.AssetId, asset.IssuerId, "bridge_burn", m.Holder, m.ExternalRecipient,
		fmt.Sprintf("units=%d external=%s", m.Units, m.ExternalRecipient))
	emit(sdk.UnwrapSDKContext(ctx), "carbon_bridge_burned",
		[2]string{"asset_id", strconv.FormatUint(m.AssetId, 10)},
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
	emit(sdk.UnwrapSDKContext(ctx), "carbon_params_updated")
	return &types.MsgUpdateParamsResponse{}, nil
}
