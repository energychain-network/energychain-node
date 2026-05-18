package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/mrv/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return &msgServer{k} }

// ---- Schemas (authority only) -----------------------------------------

func (s msgServer) RegisterSchema(ctx context.Context, m *types.MsgRegisterSchema) (*types.MsgRegisterSchemaResponse, error) {
	if m.Authority != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: not authority")
	}
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateURI("schema_uri", m.SchemaUri, p.UriMaxLen); err != nil {
		return nil, err
	}
	// Bound the number of schemas: governance writes are
	// cheap, so a misconfigured proposal could otherwise
	// flood state.
	cur, err := s.countSchemas(ctx)
	if err != nil {
		return nil, err
	}
	if cur >= uint64(p.MaxSchemas) {
		return nil, fmt.Errorf("max_schemas reached: %d", p.MaxSchemas)
	}
	id, err := s.k.NextSchemaID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	sc := types.Schema{
		Id: id, Name: m.Name, Version: m.Version, Jurisdiction: m.Jurisdiction,
		AssetClass: m.AssetClass, TimeWindow: m.TimeWindow, OutputFormat: m.OutputFormat,
		SchemaUri: m.SchemaUri, SchemaHash: m.SchemaHash,
		Status: types.SchemaStatus_SCHEMA_STATUS_ACTIVE, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.k.SetSchema(ctx, sc); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "mrv.schema.registered",
		sdk.NewAttribute("id", u64s(id)),
		sdk.NewAttribute("name", m.Name),
		sdk.NewAttribute("version", m.Version),
		sdk.NewAttribute("jurisdiction", m.Jurisdiction),
		sdk.NewAttribute("asset_class", m.AssetClass.String()),
	)
	s.k.recordAudit(ctx, 0, "schema.register", m.Authority, "", m.Name+"/"+m.Version)
	return &types.MsgRegisterSchemaResponse{SchemaId: id}, nil
}

func (s msgServer) UpdateSchemaStatus(ctx context.Context, m *types.MsgUpdateSchemaStatus) (*types.MsgUpdateSchemaStatusResponse, error) {
	if m.Authority != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: not authority")
	}
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateReason(m.Reason, p.ReasonMaxLen); err != nil {
		return nil, err
	}
	sc, err := s.k.MustGetSchema(ctx, m.SchemaId)
	if err != nil {
		return nil, err
	}
	sc.Status = m.NewStatus
	sc.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.SetSchema(ctx, sc); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "mrv.schema.status_updated",
		sdk.NewAttribute("id", u64s(m.SchemaId)),
		sdk.NewAttribute("new_status", m.NewStatus.String()),
	)
	s.k.recordAudit(ctx, 0, "schema.status", m.Authority, "", m.Reason)
	return &types.MsgUpdateSchemaStatusResponse{}, nil
}

// ---- Verifiers (authority only) ---------------------------------------

func (s msgServer) RegisterVerifier(ctx context.Context, m *types.MsgRegisterVerifier) (*types.MsgRegisterVerifierResponse, error) {
	if m.Authority != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: not authority")
	}
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if uint32(len(m.Jurisdictions)) > p.MaxJurisdictionsPerVerifier {
		return nil, fmt.Errorf("too many jurisdictions: %d > %d", len(m.Jurisdictions), p.MaxJurisdictionsPerVerifier)
	}
	if uint32(len(m.AccreditedStandards)) > p.MaxStandardsPerVerifier {
		return nil, fmt.Errorf("too many standards: %d > %d", len(m.AccreditedStandards), p.MaxStandardsPerVerifier)
	}
	cur, err := s.countVerifiers(ctx)
	if err != nil {
		return nil, err
	}
	if cur >= uint64(p.MaxVerifiers) {
		return nil, fmt.Errorf("max_verifiers reached: %d", p.MaxVerifiers)
	}
	// DID uniqueness — a single DID can be registered as a
	// verifier exactly once. Re-registration after revocation
	// requires a governance-level data migration.
	if _, ok, err := s.k.GetVerifierByDID(ctx, m.Did); err != nil {
		return nil, err
	} else if ok {
		return nil, fmt.Errorf("DID %s already registered as verifier", m.Did)
	}
	// Signer-address uniqueness — the chain enforces a strict
	// 1:1 between signer_address and verifier_id so that the
	// signer of an AttestReport unambiguously identifies one
	// verifier. Sharing addresses across verifiers would let
	// one operator silently switch identity between two
	// accreditation profiles.
	if _, ok, err := s.k.GetVerifierBySigner(ctx, m.SignerAddress); err != nil {
		return nil, err
	} else if ok {
		return nil, fmt.Errorf("signer_address %s already bound to another verifier", m.SignerAddress)
	}
	id, err := s.k.NextVerifierID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	v := types.Verifier{
		Id: id, Did: m.Did, Name: m.Name, SignerAddress: m.SignerAddress,
		AccreditedStandards: m.AccreditedStandards, Jurisdictions: m.Jurisdictions,
		Status: types.VerifierStatus_VERIFIER_STATUS_ACCREDITED,
		AccreditedAt: now, UpdatedAt: now,
	}
	if err := s.k.SetVerifier(ctx, v); err != nil {
		return nil, err
	}
	if err := s.k.VerifierByDID.Set(ctx, m.Did, id); err != nil {
		return nil, err
	}
	if err := s.k.VerifierBySigner.Set(ctx, m.SignerAddress, id); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "mrv.verifier.registered",
		sdk.NewAttribute("id", u64s(id)),
		sdk.NewAttribute("did", m.Did),
		sdk.NewAttribute("name", m.Name),
	)
	s.k.recordAudit(ctx, 0, "verifier.register", m.Authority, m.Did, m.Name)
	return &types.MsgRegisterVerifierResponse{VerifierId: id}, nil
}

func (s msgServer) UpdateVerifierStatus(ctx context.Context, m *types.MsgUpdateVerifierStatus) (*types.MsgUpdateVerifierStatusResponse, error) {
	if m.Authority != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: not authority")
	}
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateReason(m.Reason, p.ReasonMaxLen); err != nil {
		return nil, err
	}
	v, ok, err := s.k.GetVerifier(ctx, m.VerifierId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("verifier %d not found", m.VerifierId)
	}
	v.Status = m.NewStatus
	v.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.SetVerifier(ctx, v); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "mrv.verifier.status_updated",
		sdk.NewAttribute("id", u64s(m.VerifierId)),
		sdk.NewAttribute("new_status", m.NewStatus.String()),
	)
	s.k.recordAudit(ctx, 0, "verifier.status", m.Authority, v.Did, m.Reason)
	return &types.MsgUpdateVerifierStatusResponse{}, nil
}

// ---- Reports -----------------------------------------------------------

func (s msgServer) SubmitReport(ctx context.Context, m *types.MsgSubmitReport) (*types.MsgSubmitReportResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	// Authorization: submitter must be the subject OR the
	// authority (close-out flow for delinquent reporters).
	if m.Submitter != m.Subject && m.Submitter != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: submitter %s is not subject nor authority", m.Submitter)
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateURI("payload_uri", m.PayloadUri, p.UriMaxLen); err != nil {
		return nil, err
	}
	if err := types.ValidateMemo(m.Memo, p.MemoMaxLen); err != nil {
		return nil, err
	}
	// Compliance: both submitter and subject must be unsanctioned.
	if err := s.k.requireUnsanctioned(ctx, m.Submitter); err != nil {
		return nil, err
	}
	if m.Subject != m.Submitter {
		if err := s.k.requireUnsanctioned(ctx, m.Subject); err != nil {
			return nil, err
		}
	}
	// Schema must exist and be ACTIVE; DEPRECATED schemas
	// can be queried but no new reports may be filed against
	// them.
	sc, err := s.k.MustGetSchema(ctx, m.SchemaId)
	if err != nil {
		return nil, err
	}
	if sc.Status != types.SchemaStatus_SCHEMA_STATUS_ACTIVE {
		return nil, fmt.Errorf("schema %d not ACTIVE: %s", m.SchemaId, sc.Status)
	}
	// Per-subject cap to bound state growth from a single
	// reporter pumping out reports.
	cur, err := s.k.ReportCount(ctx, m.Subject)
	if err != nil {
		return nil, err
	}
	if cur >= uint64(p.MaxReportsPerSubject) {
		return nil, fmt.Errorf("max_reports_per_subject reached for %s: %d", m.Subject, p.MaxReportsPerSubject)
	}
	id, err := s.k.NextReportID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	r := types.Report{
		Id: id, SchemaId: m.SchemaId, Subject: m.Subject,
		PeriodStart: m.PeriodStart, PeriodEnd: m.PeriodEnd,
		PayloadUri: m.PayloadUri, PayloadHash: m.PayloadHash,
		Aggregate: m.Aggregate,
		Status:    types.ReportStatus_REPORT_STATUS_DRAFT,
		SubmittedAt: now, Memo: m.Memo,
	}
	if err := s.k.SetReport(ctx, r); err != nil {
		return nil, err
	}
	if err := s.k.ReportBySubject.Set(ctx, collections.Join(m.Subject, id)); err != nil {
		return nil, err
	}
	if err := s.k.ReportBySchema.Set(ctx, collections.Join(m.SchemaId, id)); err != nil {
		return nil, err
	}
	if _, err := s.k.incReportCount(ctx, m.Subject); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "mrv.report.submitted",
		sdk.NewAttribute("id", u64s(id)),
		sdk.NewAttribute("schema_id", u64s(m.SchemaId)),
		sdk.NewAttribute("subject", m.Subject),
		sdk.NewAttribute("period_start", i64s(m.PeriodStart)),
		sdk.NewAttribute("period_end", i64s(m.PeriodEnd)),
	)
	s.k.recordAudit(ctx, id, "report.submit", m.Submitter, m.Subject, m.PayloadHash)
	return &types.MsgSubmitReportResponse{ReportId: id}, nil
}

// AttestReport: an ACCREDITED verifier signs a DRAFT report,
// flipping it to ATTESTED. Verifier identity is verified
// two ways:
//   1. verifier_address must be a registered controller of
//      the verifier DID, when a DIDKeeper is wired
//   2. failing 1, the verifier_address must match what the
//      keeper has on file for the verifier (fail-closed: if
//      no DIDKeeper is wired, the verifier may only attest
//      from the chain address authority used at registration)
// In the current minimal wiring, the chain-side address ↔
// verifier_id binding is established the first time that
// verifier successfully attests, then enforced on subsequent
// attests via VerifierByAddr.
func (s msgServer) AttestReport(ctx context.Context, m *types.MsgAttestReport) (*types.MsgAttestReportResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateURI("verifier_payload_uri", m.VerifierPayloadUri, p.UriMaxLen); err != nil {
		return nil, err
	}
	if err := s.k.requireUnsanctioned(ctx, m.VerifierAddress); err != nil {
		return nil, err
	}
	v, err := s.requireAttestingVerifier(ctx, m.VerifierId, m.VerifierAddress)
	if err != nil {
		return nil, err
	}
	r, err := s.k.MustGetReport(ctx, m.ReportId)
	if err != nil {
		return nil, err
	}
	if r.Status != types.ReportStatus_REPORT_STATUS_DRAFT {
		return nil, fmt.Errorf("report %d not DRAFT: %s", m.ReportId, r.Status)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	r.Status = types.ReportStatus_REPORT_STATUS_ATTESTED
	r.VerifierId = v.Id
	r.VerifierPayloadUri = m.VerifierPayloadUri
	r.VerifierPayloadHash = m.VerifierPayloadHash
	r.AttestedAt = now
	if err := s.k.SetReport(ctx, r); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "mrv.report.attested",
		sdk.NewAttribute("id", u64s(m.ReportId)),
		sdk.NewAttribute("verifier_id", u64s(v.Id)),
		sdk.NewAttribute("verifier_address", m.VerifierAddress),
	)
	s.k.recordAudit(ctx, m.ReportId, "report.attest", m.VerifierAddress, r.Subject, v.Did)
	return &types.MsgAttestReportResponse{}, nil
}

func (s msgServer) RejectReport(ctx context.Context, m *types.MsgRejectReport) (*types.MsgRejectReportResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateReason(m.Reason, p.ReasonMaxLen); err != nil {
		return nil, err
	}
	v, err := s.requireAttestingVerifier(ctx, m.VerifierId, m.VerifierAddress)
	if err != nil {
		return nil, err
	}
	r, err := s.k.MustGetReport(ctx, m.ReportId)
	if err != nil {
		return nil, err
	}
	if r.Status != types.ReportStatus_REPORT_STATUS_DRAFT {
		return nil, fmt.Errorf("report %d not DRAFT: %s", m.ReportId, r.Status)
	}
	r.Status = types.ReportStatus_REPORT_STATUS_REJECTED
	r.VerifierId = v.Id
	if err := s.k.SetReport(ctx, r); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "mrv.report.rejected",
		sdk.NewAttribute("id", u64s(m.ReportId)),
		sdk.NewAttribute("verifier_id", u64s(v.Id)),
	)
	s.k.recordAudit(ctx, m.ReportId, "report.reject", m.VerifierAddress, r.Subject, m.Reason)
	return &types.MsgRejectReportResponse{}, nil
}

func (s msgServer) RetractReport(ctx context.Context, m *types.MsgRetractReport) (*types.MsgRetractReportResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateReason(m.Reason, p.ReasonMaxLen); err != nil {
		return nil, err
	}
	r, err := s.k.MustGetReport(ctx, m.ReportId)
	if err != nil {
		return nil, err
	}
	// Only the subject of the report OR the authority may retract.
	if m.Actor != r.Subject && m.Actor != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: actor %s is not subject nor authority", m.Actor)
	}
	if types.ReportIsTerminal(r.Status) {
		return nil, fmt.Errorf("report %d already terminal: %s", m.ReportId, r.Status)
	}
	r.Status = types.ReportStatus_REPORT_STATUS_RETRACTED
	if err := s.k.SetReport(ctx, r); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "mrv.report.retracted",
		sdk.NewAttribute("id", u64s(m.ReportId)),
		sdk.NewAttribute("actor", m.Actor),
	)
	s.k.recordAudit(ctx, m.ReportId, "report.retract", m.Actor, r.Subject, m.Reason)
	return &types.MsgRetractReportResponse{}, nil
}

// ---- View-key grants ---------------------------------------------------

func (s msgServer) GrantViewKey(ctx context.Context, m *types.MsgGrantViewKey) (*types.MsgGrantViewKeyResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	// Total live-grant cap to bound state growth and the
	// per-block sweep work. Live-grant count is approximated
	// by all rows in the Grants collection — revoked rows
	// stay until pruning, which deliberately keeps an audit
	// trail.
	cur, err := s.countGrants(ctx)
	if err != nil {
		return nil, err
	}
	if cur >= uint64(p.MaxViewKeyGrants) {
		return nil, fmt.Errorf("max_view_key_grants reached: %d", p.MaxViewKeyGrants)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if m.ExpiresAt <= now {
		return nil, fmt.Errorf("expires_at must be in the future")
	}
	if m.ExpiresAt-now > p.MaxGrantTtlSeconds {
		return nil, fmt.Errorf("ttl > max_grant_ttl_seconds (%d)", p.MaxGrantTtlSeconds)
	}
	if err := s.k.requireUnsanctioned(ctx, m.Granter); err != nil {
		return nil, err
	}
	// Referential integrity for scoped grants.
	if m.ReportId != 0 {
		r, err := s.k.MustGetReport(ctx, m.ReportId)
		if err != nil {
			return nil, err
		}
		// A subject can only grant access to reports they own.
		if r.Subject != m.Granter {
			return nil, fmt.Errorf("report %d not owned by granter", m.ReportId)
		}
	}
	if m.SchemaId != 0 {
		if _, err := s.k.MustGetSchema(ctx, m.SchemaId); err != nil {
			return nil, err
		}
	}
	id, err := s.k.NextGrantID(ctx)
	if err != nil {
		return nil, err
	}
	g := types.ViewKeyGrant{
		Id: id, Granter: m.Granter, GranteeDid: m.GranteeDid,
		ReportId: m.ReportId, SchemaId: m.SchemaId,
		ExpiresAt: m.ExpiresAt, CreatedAt: now,
	}
	if err := s.k.SetGrant(ctx, g); err != nil {
		return nil, err
	}
	if err := s.k.GrantByGranter.Set(ctx, collections.Join(m.Granter, id)); err != nil {
		return nil, err
	}
	if err := s.k.GrantByGrantee.Set(ctx, collections.Join(m.GranteeDid, id)); err != nil {
		return nil, err
	}
	if err := s.k.GrantByExpiry.Set(ctx, collections.Join(m.ExpiresAt, id)); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "mrv.view_key.granted",
		sdk.NewAttribute("id", u64s(id)),
		sdk.NewAttribute("granter", m.Granter),
		sdk.NewAttribute("grantee_did", m.GranteeDid),
		sdk.NewAttribute("expires_at", i64s(m.ExpiresAt)),
	)
	s.k.recordAudit(ctx, m.ReportId, "view_key.grant", m.Granter, m.GranteeDid, "")
	return &types.MsgGrantViewKeyResponse{GrantId: id}, nil
}

func (s msgServer) RevokeViewKey(ctx context.Context, m *types.MsgRevokeViewKey) (*types.MsgRevokeViewKeyResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateReason(m.Reason, p.ReasonMaxLen); err != nil {
		return nil, err
	}
	g, err := s.k.MustGetGrant(ctx, m.GrantId)
	if err != nil {
		return nil, err
	}
	if m.Actor != g.Granter && m.Actor != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: actor %s is not granter nor authority", m.Actor)
	}
	// Idempotent — revoking an already-revoked grant is a no-op.
	if g.Revoked {
		return &types.MsgRevokeViewKeyResponse{}, nil
	}
	g.Revoked = true
	if err := s.k.SetGrant(ctx, g); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "mrv.view_key.revoked",
		sdk.NewAttribute("id", u64s(m.GrantId)),
		sdk.NewAttribute("actor", m.Actor),
	)
	s.k.recordAudit(ctx, g.ReportId, "view_key.revoke", m.Actor, g.GranteeDid, m.Reason)
	return &types.MsgRevokeViewKeyResponse{}, nil
}

func (s msgServer) UpdateParams(ctx context.Context, m *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if m.Authority != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: not authority")
	}
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.k.SetParams(ctx, m.Params); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "mrv.params.updated", sdk.NewAttribute("authority", m.Authority))
	s.k.recordAudit(ctx, 0, "params.update", m.Authority, "", "")
	return &types.MsgUpdateParamsResponse{}, nil
}

// requireAttestingVerifier is the single chokepoint enforcing
// who may attest/reject as a given verifier_id. Three rules,
// applied in order:
//
//  1. The verifier must exist.
//  2. The verifier must be ACCREDITED (SUSPENDED / REVOKED
//     loses all attestation rights immediately).
//  3. The signing address must either:
//     a. match the verifier's on-file signer_address — the
//        always-on, chain-native default; OR
//     b. be a current controller of the verifier's DID, when
//        a DIDKeeper is wired. The DID path exists so a
//        verifier can rotate keys through their DID document
//        without re-registering on every chain.
//
// Both paths converge on "the signer represents this
// verifier". A nil DIDKeeper falls back to (a) only — closing
// the original gap where any address could attest as any
// verifier in deployments without DID resolution.
func (s msgServer) requireAttestingVerifier(ctx context.Context, verifierID uint64, signer string) (types.Verifier, error) {
	v, ok, err := s.k.GetVerifier(ctx, verifierID)
	if err != nil {
		return types.Verifier{}, err
	}
	if !ok {
		return types.Verifier{}, fmt.Errorf("verifier %d not found", verifierID)
	}
	if v.Status != types.VerifierStatus_VERIFIER_STATUS_ACCREDITED {
		return types.Verifier{}, fmt.Errorf("verifier %d not ACCREDITED: %s", verifierID, v.Status)
	}
	if v.SignerAddress == signer {
		return v, nil
	}
	if s.k.did != nil && s.k.did.IsControllerOf(sdk.UnwrapSDKContext(ctx), v.Did, signer) {
		return v, nil
	}
	return types.Verifier{}, fmt.Errorf("address %s is neither the registered signer nor a controller of verifier DID %s", signer, v.Did)
}

// ---- internal counters -------------------------------------------------
//
// schemas / verifiers / grants are append-only (never
// physically deleted — only flipped to terminal status), so
// the per-resource sequence's Peek() gives an exact "rows
// ever created" count. Using Peek keeps cap-enforcement O(1)
// instead of the O(N) walk this used to perform, which
// was a real DoS vector once the grant table grew into the
// millions allowed by the hard cap.
func (s msgServer) countSchemas(ctx context.Context) (uint64, error) {
	return s.k.SchemaIDSeq.Peek(ctx)
}

func (s msgServer) countVerifiers(ctx context.Context) (uint64, error) {
	return s.k.VerifierIDSeq.Peek(ctx)
}

func (s msgServer) countGrants(ctx context.Context) (uint64, error) {
	return s.k.GrantIDSeq.Peek(ctx)
}
