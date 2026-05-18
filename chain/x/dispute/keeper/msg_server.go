package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/dispute/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return &msgServer{k} }

// ---- Arbitrators (authority only) -------------------------------------

func (s msgServer) RegisterArbitrator(ctx context.Context, m *types.MsgRegisterArbitrator) (*types.MsgRegisterArbitratorResponse, error) {
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
	if uint32(len(m.Jurisdictions)) > p.MaxJurisdictionsPerArbitrator {
		return nil, fmt.Errorf("too many jurisdictions: %d > %d", len(m.Jurisdictions), p.MaxJurisdictionsPerArbitrator)
	}
	if uint32(len(m.AccreditedStandards)) > p.MaxStandardsPerArbitrator {
		return nil, fmt.Errorf("too many standards: %d > %d", len(m.AccreditedStandards), p.MaxStandardsPerArbitrator)
	}
	cur, err := s.k.countArbitrators(ctx)
	if err != nil {
		return nil, err
	}
	if cur >= uint64(p.MaxArbitrators) {
		return nil, fmt.Errorf("max_arbitrators reached: %d", p.MaxArbitrators)
	}
	// DID uniqueness — one accreditation per DID. Re-issuing
	// a previously-revoked DID requires a governance migration.
	if _, ok, err := s.k.GetArbitratorByDID(ctx, m.Did); err != nil {
		return nil, err
	} else if ok {
		return nil, fmt.Errorf("DID %s already registered as arbitrator", m.Did)
	}
	// Signer-address uniqueness — strict 1:1 so the signer of
	// a CastVote unambiguously identifies one arbitrator.
	if _, ok, err := s.k.GetArbitratorBySigner(ctx, m.SignerAddress); err != nil {
		return nil, err
	} else if ok {
		return nil, fmt.Errorf("signer_address %s already bound to another arbitrator", m.SignerAddress)
	}
	id, err := s.k.NextArbitratorID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	a := types.Arbitrator{
		Id: id, Did: m.Did, SignerAddress: m.SignerAddress, Name: m.Name,
		AccreditedStandards: m.AccreditedStandards, Jurisdictions: m.Jurisdictions,
		Status:       types.ArbitratorStatus_ARBITRATOR_STATUS_ACCREDITED,
		AccreditedAt: now, UpdatedAt: now,
	}
	if err := s.k.SetArbitrator(ctx, a); err != nil {
		return nil, err
	}
	if err := s.k.ArbitratorByDID.Set(ctx, m.Did, id); err != nil {
		return nil, err
	}
	if err := s.k.ArbitratorBySigner.Set(ctx, m.SignerAddress, id); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dispute.arbitrator.registered",
		sdk.NewAttribute("id", u64s(id)),
		sdk.NewAttribute("did", m.Did),
		sdk.NewAttribute("signer", m.SignerAddress),
	)
	s.k.recordAudit(ctx, 0, "arbitrator.register", m.Authority, m.Did, m.Name)
	return &types.MsgRegisterArbitratorResponse{ArbitratorId: id}, nil
}

func (s msgServer) UpdateArbitratorStatus(ctx context.Context, m *types.MsgUpdateArbitratorStatus) (*types.MsgUpdateArbitratorStatusResponse, error) {
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
	a, err := s.k.MustGetArbitrator(ctx, m.ArbitratorId)
	if err != nil {
		return nil, err
	}
	a.Status = m.NewStatus
	a.UpdatedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.SetArbitrator(ctx, a); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dispute.arbitrator.status_updated",
		sdk.NewAttribute("id", u64s(m.ArbitratorId)),
		sdk.NewAttribute("new_status", m.NewStatus.String()),
	)
	s.k.recordAudit(ctx, 0, "arbitrator.status", m.Authority, a.Did, m.Reason)
	return &types.MsgUpdateArbitratorStatusResponse{}, nil
}

// ---- Disputes ---------------------------------------------------------

func (s msgServer) OpenDispute(ctx context.Context, m *types.MsgOpenDispute) (*types.MsgOpenDisputeResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateURI("claim_uri", m.ClaimUri, p.UriMaxLen); err != nil {
		return nil, err
	}
	if err := types.ValidateMemo(m.Memo, p.MemoMaxLen); err != nil {
		return nil, err
	}
	// Bond bounds — both parties must believe the bond is
	// commensurate with the dispute. The chain enforces an
	// upper cap on `respondent_bond_required` so a griefing
	// plaintiff cannot impose unreachable barriers.
	if m.PlaintiffBond < p.MinPlaintiffBond {
		return nil, fmt.Errorf("plaintiff_bond %d < min %d", m.PlaintiffBond, p.MinPlaintiffBond)
	}
	if m.PlaintiffBond > p.MaxPlaintiffBond {
		return nil, fmt.Errorf("plaintiff_bond %d > max %d", m.PlaintiffBond, p.MaxPlaintiffBond)
	}
	if m.RespondentBondRequired > p.MaxPlaintiffBond {
		return nil, fmt.Errorf("respondent_bond_required %d > max %d", m.RespondentBondRequired, p.MaxPlaintiffBond)
	}
	if err := s.k.requireStablecoin(ctx, m.BondDenom); err != nil {
		return nil, err
	}
	// Sanctions on either party at open-time block the entire
	// dispute. After open, per-leg checks are applied at the
	// specific transfer time too.
	if err := s.k.requireUnsanctioned(ctx, m.Plaintiff); err != nil {
		return nil, err
	}
	if err := s.k.requireUnsanctioned(ctx, m.Respondent); err != nil {
		return nil, err
	}
	// Bound active disputes globally so a flood of openings
	// cannot DoS the end-block sweep.
	active, err := s.k.activeDisputeCount(ctx)
	if err != nil {
		return nil, err
	}
	if active >= p.MaxOpenDisputes {
		return nil, fmt.Errorf("max_open_disputes reached: %d", p.MaxOpenDisputes)
	}
	id, err := s.k.NextDisputeID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	d := types.Dispute{
		Id: id, Plaintiff: m.Plaintiff, Respondent: m.Respondent,
		SubjectKind: m.SubjectKind, SubjectRef: m.SubjectRef,
		ClaimUri: m.ClaimUri, ClaimHash: m.ClaimHash, Memo: m.Memo,
		BondDenom: m.BondDenom, PlaintiffBond: m.PlaintiffBond,
		RespondentBondRequired: m.RespondentBondRequired,
		Status:           types.DisputeStatus_DISPUTE_STATUS_OPEN,
		OpenedAt:         now,
		RespondDeadline:  now + p.RespondPeriodSeconds,
	}
	// Move the plaintiff's bond into the pool BEFORE persisting
	// state so a failed transfer aborts the open atomically.
	if err := s.k.moveBond(ctx, m.BondDenom, m.Plaintiff, DisputePoolAccount, m.PlaintiffBond); err != nil {
		return nil, fmt.Errorf("plaintiff bond transfer: %w", err)
	}
	if err := s.k.SetDispute(ctx, d); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dispute.opened",
		sdk.NewAttribute("id", u64s(id)),
		sdk.NewAttribute("plaintiff", m.Plaintiff),
		sdk.NewAttribute("respondent", m.Respondent),
		sdk.NewAttribute("subject_kind", m.SubjectKind.String()),
		sdk.NewAttribute("bond_denom", m.BondDenom),
		sdk.NewAttribute("plaintiff_bond", u64s(m.PlaintiffBond)),
		sdk.NewAttribute("respond_deadline", i64s(d.RespondDeadline)),
	)
	s.k.recordAudit(ctx, id, "dispute.open", m.Plaintiff, m.Respondent, m.ClaimHash)
	return &types.MsgOpenDisputeResponse{DisputeId: id}, nil
}

func (s msgServer) Respond(ctx context.Context, m *types.MsgRespond) (*types.MsgRespondResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	d, err := s.k.MustGetDispute(ctx, m.DisputeId)
	if err != nil {
		return nil, err
	}
	if d.Status != types.DisputeStatus_DISPUTE_STATUS_OPEN {
		return nil, fmt.Errorf("dispute %d not OPEN: %s", d.Id, d.Status)
	}
	if m.Respondent != d.Respondent {
		return nil, fmt.Errorf("only the named respondent may respond")
	}
	if m.Bond < d.RespondentBondRequired {
		return nil, fmt.Errorf("bond %d < required %d", m.Bond, d.RespondentBondRequired)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if now >= d.RespondDeadline {
		return nil, fmt.Errorf("respond_deadline passed")
	}
	if err := s.k.requireStablecoin(ctx, d.BondDenom); err != nil {
		return nil, err
	}
	if err := s.k.requireUnsanctioned(ctx, m.Respondent); err != nil {
		return nil, err
	}
	if err := s.k.moveBond(ctx, d.BondDenom, m.Respondent, DisputePoolAccount, m.Bond); err != nil {
		return nil, fmt.Errorf("respondent bond transfer: %w", err)
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	d.RespondentBond = m.Bond
	d.Status = types.DisputeStatus_DISPUTE_STATUS_RESPONDED
	d.DeliberationDeadline = now + p.DeliberationPeriodSeconds
	if err := s.k.SetDispute(ctx, d); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dispute.responded",
		sdk.NewAttribute("id", u64s(d.Id)),
		sdk.NewAttribute("respondent", m.Respondent),
		sdk.NewAttribute("bond", u64s(m.Bond)),
		sdk.NewAttribute("deliberation_deadline", i64s(d.DeliberationDeadline)),
	)
	s.k.recordAudit(ctx, d.Id, "dispute.respond", m.Respondent, d.Plaintiff, u64s(m.Bond))
	return &types.MsgRespondResponse{}, nil
}

func (s msgServer) SubmitEvidence(ctx context.Context, m *types.MsgSubmitEvidence) (*types.MsgSubmitEvidenceResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateURI("uri", m.Uri, p.UriMaxLen); err != nil {
		return nil, err
	}
	d, err := s.k.MustGetDispute(ctx, m.DisputeId)
	if err != nil {
		return nil, err
	}
	if types.DisputeIsTerminal(d.Status) {
		return nil, fmt.Errorf("dispute %d already terminal: %s", d.Id, d.Status)
	}
	// Submitter must be a party OR a tribunal arbitrator. The
	// authority is NOT allowed to inject evidence — the
	// dispute record is the parties' joint factual record.
	allowed := m.Submitter == d.Plaintiff || m.Submitter == d.Respondent
	if !allowed {
		arbID, ok, err := s.k.GetArbitratorBySigner(ctx, m.Submitter)
		if err != nil {
			return nil, err
		}
		if ok {
			if has, err := s.k.Tribunal.Has(ctx, collections.Join(d.Id, arbID)); err != nil {
				return nil, err
			} else if has {
				allowed = true
			}
		}
	}
	if !allowed {
		return nil, fmt.Errorf("unauthorized: %s is not a party nor an assigned arbitrator", m.Submitter)
	}
	// Bound the per-dispute evidence so a party cannot DoS by
	// flooding. EvidenceCount returns count = next - 1.
	cnt, err := s.k.EvidenceCount(ctx, d.Id)
	if err != nil {
		return nil, err
	}
	if uint32(cnt) >= p.MaxEvidencePerDispute {
		return nil, fmt.Errorf("max_evidence_per_dispute reached: %d", p.MaxEvidencePerDispute)
	}
	eid, err := s.k.nextEvidenceID(ctx, d.Id)
	if err != nil {
		return nil, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	ev := types.Evidence{
		DisputeId: d.Id, Id: eid, Submitter: m.Submitter,
		Uri: m.Uri, Hash: m.Hash, SubmittedAt: now,
	}
	if err := s.k.Evidence.Set(ctx, collections.Join(d.Id, eid), ev); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dispute.evidence.submitted",
		sdk.NewAttribute("dispute_id", u64s(d.Id)),
		sdk.NewAttribute("evidence_id", u64s(eid)),
		sdk.NewAttribute("submitter", m.Submitter),
	)
	s.k.recordAudit(ctx, d.Id, "dispute.evidence", m.Submitter, "", m.Hash)
	return &types.MsgSubmitEvidenceResponse{EvidenceId: eid}, nil
}

func (s msgServer) AssignTribunal(ctx context.Context, m *types.MsgAssignTribunal) (*types.MsgAssignTribunalResponse, error) {
	if m.Authority != s.k.GetAuthority() {
		return nil, fmt.Errorf("unauthorized: not authority")
	}
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	d, err := s.k.MustGetDispute(ctx, m.DisputeId)
	if err != nil {
		return nil, err
	}
	if d.Status != types.DisputeStatus_DISPUTE_STATUS_RESPONDED &&
		d.Status != types.DisputeStatus_DISPUTE_STATUS_DELIBERATING {
		return nil, fmt.Errorf("dispute %d not in RESPONDED or DELIBERATING: %s", d.Id, d.Status)
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	// Existing-tribunal counting + duplicate guard. We refuse
	// re-assignment that would overflow panel_size; partial
	// top-ups (e.g. after a recusal) are supported by sending
	// only the new ids.
	existing := map[uint64]bool{}
	rng := collections.NewPrefixedPairRange[uint64, uint64](d.Id)
	it, err := s.k.Tribunal.Iterate(ctx, rng)
	if err != nil {
		return nil, err
	}
	for ; it.Valid(); it.Next() {
		k, err := it.Key()
		if err != nil {
			_ = it.Close()
			return nil, err
		}
		existing[k.K2()] = true
	}
	if err := it.Close(); err != nil {
		return nil, err
	}
	if uint32(len(existing))+uint32(len(m.ArbitratorIds)) > p.PanelSize {
		return nil, fmt.Errorf("tribunal full: existing %d + new %d > panel_size %d",
			len(existing), len(m.ArbitratorIds), p.PanelSize)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	for _, aid := range m.ArbitratorIds {
		if existing[aid] {
			return nil, fmt.Errorf("arbitrator %d already on tribunal", aid)
		}
		a, err := s.k.MustGetArbitrator(ctx, aid)
		if err != nil {
			return nil, err
		}
		if a.Status != types.ArbitratorStatus_ARBITRATOR_STATUS_ACCREDITED {
			return nil, fmt.Errorf("arbitrator %d not ACCREDITED: %s", aid, a.Status)
		}
		// Conflict of interest: the arbitrator's signer must
		// not coincide with either party.
		if a.SignerAddress == d.Plaintiff || a.SignerAddress == d.Respondent {
			return nil, fmt.Errorf("arbitrator %d has conflict (party)", aid)
		}
		tm := types.TribunalMember{
			DisputeId: d.Id, ArbitratorId: aid, AssignedAt: now,
		}
		if err := s.k.Tribunal.Set(ctx, collections.Join(d.Id, aid), tm); err != nil {
			return nil, err
		}
	}
	// Flip status to DELIBERATING the first time a tribunal
	// is assigned. Subsequent top-ups leave the status alone.
	if d.Status == types.DisputeStatus_DISPUTE_STATUS_RESPONDED {
		d.Status = types.DisputeStatus_DISPUTE_STATUS_DELIBERATING
		if err := s.k.SetDispute(ctx, d); err != nil {
			return nil, err
		}
	}
	s.k.emit(ctx, "dispute.tribunal.assigned",
		sdk.NewAttribute("id", u64s(d.Id)),
		sdk.NewAttribute("added", u64s(uint64(len(m.ArbitratorIds)))),
	)
	s.k.recordAudit(ctx, d.Id, "tribunal.assign", m.Authority, "", "")
	return &types.MsgAssignTribunalResponse{}, nil
}

func (s msgServer) CastVote(ctx context.Context, m *types.MsgCastVote) (*types.MsgCastVoteResponse, error) {
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
	d, err := s.k.MustGetDispute(ctx, m.DisputeId)
	if err != nil {
		return nil, err
	}
	if d.Status != types.DisputeStatus_DISPUTE_STATUS_DELIBERATING {
		return nil, fmt.Errorf("dispute %d not DELIBERATING: %s", d.Id, d.Status)
	}
	a, err := s.k.MustGetArbitrator(ctx, m.ArbitratorId)
	if err != nil {
		return nil, err
	}
	// Arbitrator authentication: signer must match the
	// on-file signer_address, OR (if a DIDKeeper is wired)
	// be a current controller of the arbitrator's DID. This
	// is the same pattern x/mrv uses for verifier signing.
	if !s.arbitratorOwns(ctx, a, m.VoterAddress) {
		return nil, fmt.Errorf("address %s does not represent arbitrator %d", m.VoterAddress, a.Id)
	}
	if a.Status != types.ArbitratorStatus_ARBITRATOR_STATUS_ACCREDITED {
		return nil, fmt.Errorf("arbitrator %d not ACCREDITED: %s", a.Id, a.Status)
	}
	// Late-binding conflict-of-interest gate: the COI check
	// at AssignTribunal looks at the arbitrator's registered
	// signer_address. If the DIDKeeper later rotates a
	// party's address into the arbitrator's controllers, that
	// party would otherwise be able to vote on their own
	// dispute. Re-check the voter address against both
	// parties at vote time to close the gap.
	if m.VoterAddress == d.Plaintiff || m.VoterAddress == d.Respondent {
		return nil, fmt.Errorf("conflict: voter is a party of dispute %d", d.Id)
	}
	// Tribunal membership required.
	if has, err := s.k.Tribunal.Has(ctx, collections.Join(d.Id, a.Id)); err != nil {
		return nil, err
	} else if !has {
		return nil, fmt.Errorf("arbitrator %d not on tribunal of dispute %d", a.Id, d.Id)
	}
	// One vote per arbitrator per dispute. Refuse silent
	// re-votes — change-of-mind is governance territory.
	if has, err := s.k.Votes.Has(ctx, collections.Join(d.Id, a.Id)); err != nil {
		return nil, err
	} else if has {
		return nil, fmt.Errorf("arbitrator %d already voted", a.Id)
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	v := types.Vote{
		DisputeId: d.Id, ArbitratorId: a.Id, Choice: m.Choice,
		Reason: m.Reason, CastAt: now,
	}
	if err := s.k.Votes.Set(ctx, collections.Join(d.Id, a.Id), v); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dispute.vote.cast",
		sdk.NewAttribute("dispute_id", u64s(d.Id)),
		sdk.NewAttribute("arbitrator_id", u64s(a.Id)),
		sdk.NewAttribute("choice", m.Choice.String()),
	)
	s.k.recordAudit(ctx, d.Id, "dispute.vote", m.VoterAddress, "", m.Choice.String())
	return &types.MsgCastVoteResponse{}, nil
}

func (s msgServer) FinalizeRuling(ctx context.Context, m *types.MsgFinalizeRuling) (*types.MsgFinalizeRulingResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateURI("ruling_uri", m.RulingUri, p.UriMaxLen); err != nil {
		return nil, err
	}
	d, err := s.k.MustGetDispute(ctx, m.DisputeId)
	if err != nil {
		return nil, err
	}
	if d.Status != types.DisputeStatus_DISPUTE_STATUS_DELIBERATING {
		return nil, fmt.Errorf("dispute %d not DELIBERATING: %s", d.Id, d.Status)
	}
	isAuthority := m.Actor == s.k.GetAuthority()
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	// Non-authority callers may only finalize once quorum is
	// reached OR deliberation_deadline has passed. Authority
	// can finalize anytime to ship governance overrides.
	tally, err := s.tallyVotes(ctx, d.Id)
	if err != nil {
		return nil, err
	}
	panelSize, err := s.panelSize(ctx, d.Id)
	if err != nil {
		return nil, err
	}
	if !isAuthority {
		quorumReached := quorumMet(tally.total, panelSize, p.QuorumBps)
		past := now >= d.DeliberationDeadline
		if !quorumReached && !past {
			return nil, fmt.Errorf("not finalizable: quorum not met (%d/%d) and deadline not passed", tally.total, panelSize)
		}
	}
	slashBps := p.DefaultSlashBps
	if isAuthority && m.OverrideSlashBps > 0 {
		slashBps = m.OverrideSlashBps
	}
	outcome := decideOutcome(tally)
	return s.applyFinalize(ctx, d, m.Actor, outcome, slashBps, m.RulingUri, m.RulingHash)
}

func (s msgServer) CancelDispute(ctx context.Context, m *types.MsgCancelDispute) (*types.MsgCancelDisputeResponse, error) {
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
	d, err := s.k.MustGetDispute(ctx, m.DisputeId)
	if err != nil {
		return nil, err
	}
	if types.DisputeIsTerminal(d.Status) {
		return nil, fmt.Errorf("dispute %d already terminal: %s", d.Id, d.Status)
	}
	isAuthority := m.Actor == s.k.GetAuthority()
	isPlaintiff := m.Actor == d.Plaintiff
	// Plaintiff can withdraw ONLY while OPEN (before the
	// respondent has posted). Once the respondent has joined,
	// only authority can cancel — withdrawing after the
	// respondent has committed risks bond griefing.
	if !isAuthority {
		if !isPlaintiff {
			return nil, fmt.Errorf("unauthorized: %s is not plaintiff nor authority", m.Actor)
		}
		if d.Status != types.DisputeStatus_DISPUTE_STATUS_OPEN {
			return nil, fmt.Errorf("plaintiff can only cancel while OPEN; current %s", d.Status)
		}
	}
	// Refund all posted bonds in full. The slashed-portion
	// stays in the pool only on RESOLVED, never on CANCELLED.
	if d.PlaintiffBond > 0 {
		if err := s.k.moveBond(ctx, d.BondDenom, DisputePoolAccount, d.Plaintiff, d.PlaintiffBond); err != nil {
			return nil, fmt.Errorf("refund plaintiff: %w", err)
		}
	}
	if d.RespondentBond > 0 {
		if err := s.k.moveBond(ctx, d.BondDenom, DisputePoolAccount, d.Respondent, d.RespondentBond); err != nil {
			return nil, fmt.Errorf("refund respondent: %w", err)
		}
	}
	d.Status = types.DisputeStatus_DISPUTE_STATUS_CANCELLED
	d.ResolvedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.SetDispute(ctx, d); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dispute.cancelled",
		sdk.NewAttribute("id", u64s(d.Id)),
		sdk.NewAttribute("actor", m.Actor),
		sdk.NewAttribute("reason", m.Reason),
	)
	s.k.recordAudit(ctx, d.Id, "dispute.cancel", m.Actor, "", m.Reason)
	return &types.MsgCancelDisputeResponse{}, nil
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
	s.k.emit(ctx, "dispute.params.updated", sdk.NewAttribute("authority", m.Authority))
	s.k.recordAudit(ctx, 0, "params.update", m.Authority, "", "")
	return &types.MsgUpdateParamsResponse{}, nil
}

// ---- helpers ----------------------------------------------------------

// arbitratorOwns returns true when `signer` can speak for `a`.
// Either an exact match on the on-file signer_address, or a
// DIDKeeper-confirmed controller of the arbitrator's DID.
func (s msgServer) arbitratorOwns(ctx context.Context, a types.Arbitrator, signer string) bool {
	if a.SignerAddress == signer {
		return true
	}
	if s.k.did != nil && s.k.did.IsControllerOf(sdk.UnwrapSDKContext(ctx), a.Did, signer) {
		return true
	}
	return false
}

type voteTally struct {
	plaintiff  uint32
	respondent uint32
	split      uint32
	noFault    uint32
	abstain    uint32
	total      uint32
}

func (s msgServer) tallyVotes(ctx context.Context, disputeID uint64) (voteTally, error) {
	var t voteTally
	rng := collections.NewPrefixedPairRange[uint64, uint64](disputeID)
	it, err := s.k.Votes.Iterate(ctx, rng)
	if err != nil {
		return t, err
	}
	defer it.Close()
	for ; it.Valid(); it.Next() {
		v, err := it.Value()
		if err != nil {
			return t, err
		}
		t.total++
		switch v.Choice {
		case types.VoteChoice_VOTE_CHOICE_PLAINTIFF:
			t.plaintiff++
		case types.VoteChoice_VOTE_CHOICE_RESPONDENT:
			t.respondent++
		case types.VoteChoice_VOTE_CHOICE_SPLIT:
			t.split++
		case types.VoteChoice_VOTE_CHOICE_NO_FAULT:
			t.noFault++
		case types.VoteChoice_VOTE_CHOICE_ABSTAIN:
			t.abstain++
		}
	}
	return t, nil
}

func (s msgServer) panelSize(ctx context.Context, disputeID uint64) (uint32, error) {
	var n uint32
	rng := collections.NewPrefixedPairRange[uint64, uint64](disputeID)
	it, err := s.k.Tribunal.Iterate(ctx, rng)
	if err != nil {
		return 0, err
	}
	defer it.Close()
	for ; it.Valid(); it.Next() {
		n++
	}
	return n, nil
}

// quorumMet treats abstentions as participating but not
// counted toward a side. Quorum is computed against panel
// size, not the participating subset, so abstainers do not
// help reach quorum.
func quorumMet(totalVotes, panelSize uint32, quorumBps uint32) bool {
	if panelSize == 0 {
		return false
	}
	// votes * 10000 >= panel * quorumBps
	return uint64(totalVotes)*10_000 >= uint64(panelSize)*uint64(quorumBps)
}

// decideOutcome is deterministic. The order of precedence —
// PLAINTIFF wins over RESPONDENT wins over SPLIT wins over
// NO_FAULT — only kicks in when there is a strict majority of
// non-abstain votes; otherwise the chain returns NO_FAULT
// to preserve the bond status quo.
func decideOutcome(t voteTally) types.RulingOutcome {
	maxCount := uint32(0)
	var winner types.RulingOutcome = types.RulingOutcome_RULING_OUTCOME_NO_FAULT
	choices := []struct {
		c types.RulingOutcome
		n uint32
	}{
		{types.RulingOutcome_RULING_OUTCOME_PLAINTIFF, t.plaintiff},
		{types.RulingOutcome_RULING_OUTCOME_RESPONDENT, t.respondent},
		{types.RulingOutcome_RULING_OUTCOME_SPLIT, t.split},
		{types.RulingOutcome_RULING_OUTCOME_NO_FAULT, t.noFault},
	}
	var ties int
	for _, c := range choices {
		if c.n > maxCount {
			maxCount = c.n
			winner = c.c
			ties = 1
		} else if c.n == maxCount && c.n > 0 {
			ties++
		}
	}
	if maxCount == 0 || ties > 1 {
		return types.RulingOutcome_RULING_OUTCOME_NO_FAULT
	}
	return winner
}

// applyFinalize is the bond-disbursement chokepoint. The
// caller has already validated state/timing and picked the
// outcome + slash_bps. Refund/slash math runs through
// SlashAmount so the two halves always sum to the posted
// bond, and slashed funds remain in the module pool (a
// future governance-controlled "treasury sweep" pathway can
// distribute them).
func (s msgServer) applyFinalize(
	ctx context.Context,
	d types.Dispute,
	actor string,
	outcome types.RulingOutcome,
	slashBps uint32,
	rulingURI, rulingHash string,
) (*types.MsgFinalizeRulingResponse, error) {
	var refundPlaintiff, refundRespondent uint64
	switch outcome {
	case types.RulingOutcome_RULING_OUTCOME_PLAINTIFF:
		// Respondent loses: slash respondent_bond, refund plaintiff in full.
		_, refundRespondent = types.SlashAmount(d.RespondentBond, slashBps)
		refundPlaintiff = d.PlaintiffBond
	case types.RulingOutcome_RULING_OUTCOME_RESPONDENT:
		_, refundPlaintiff = types.SlashAmount(d.PlaintiffBond, slashBps)
		refundRespondent = d.RespondentBond
	case types.RulingOutcome_RULING_OUTCOME_SPLIT:
		_, refundPlaintiff = types.SlashAmount(d.PlaintiffBond, slashBps)
		_, refundRespondent = types.SlashAmount(d.RespondentBond, slashBps)
	case types.RulingOutcome_RULING_OUTCOME_NO_FAULT:
		refundPlaintiff = d.PlaintiffBond
		refundRespondent = d.RespondentBond
	default:
		return nil, fmt.Errorf("unsupported outcome")
	}
	if refundPlaintiff > 0 {
		if err := s.k.moveBond(ctx, d.BondDenom, DisputePoolAccount, d.Plaintiff, refundPlaintiff); err != nil {
			return nil, fmt.Errorf("refund plaintiff: %w", err)
		}
	}
	if refundRespondent > 0 {
		if err := s.k.moveBond(ctx, d.BondDenom, DisputePoolAccount, d.Respondent, refundRespondent); err != nil {
			return nil, fmt.Errorf("refund respondent: %w", err)
		}
	}
	d.Status = types.DisputeStatus_DISPUTE_STATUS_RESOLVED
	d.Outcome = outcome
	// Normalize: NO_FAULT means no slash, period — preserve
	// the metadata invariant so off-chain consumers can rely
	// on (outcome == NO_FAULT) ⇒ (slash_bps == 0).
	if outcome == types.RulingOutcome_RULING_OUTCOME_NO_FAULT {
		d.SlashBps = 0
	} else {
		d.SlashBps = slashBps
	}
	d.RulingUri = rulingURI
	d.RulingHash = rulingHash
	d.ResolvedAt = sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	if err := s.k.SetDispute(ctx, d); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "dispute.resolved",
		sdk.NewAttribute("id", u64s(d.Id)),
		sdk.NewAttribute("outcome", outcome.String()),
		sdk.NewAttribute("slash_bps", u32s(slashBps)),
		sdk.NewAttribute("actor", actor),
	)
	s.k.recordAudit(ctx, d.Id, "dispute.finalize", actor, outcome.String(), rulingHash)
	return &types.MsgFinalizeRulingResponse{Outcome: outcome}, nil
}
