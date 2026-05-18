package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxArbitrators:                DefaultMaxArbitrators,
		MaxOpenDisputes:               DefaultMaxOpenDisputes,
		MaxEvidencePerDispute:         DefaultMaxEvidencePerDispute,
		PanelSize:                     DefaultPanelSize,
		QuorumBps:                     DefaultQuorumBps,
		RespondPeriodSeconds:          DefaultRespondPeriodSeconds,
		DeliberationPeriodSeconds:     DefaultDeliberationPeriodSeconds,
		MinPlaintiffBond:              DefaultMinPlaintiffBond,
		MaxPlaintiffBond:              DefaultMaxPlaintiffBond,
		DefaultSlashBps:               DefaultSlashBps,
		MemoMaxLen:                    DefaultMemoMaxLen,
		ReasonMaxLen:                  DefaultReasonMaxLen,
		UriMaxLen:                     DefaultURIMaxLen,
		MaxJurisdictionsPerArbitrator: DefaultMaxJurisdictionsPerArbitrator,
		MaxStandardsPerArbitrator:     DefaultMaxStandardsPerArbitrator,
		MaxFinalizationsPerBlock:      DefaultMaxFinalizationsPerBlock,
	}
}

func (p Params) Validate() error {
	if p.MaxArbitrators == 0 || p.MaxArbitrators > HardMaxArbitrators {
		return fmt.Errorf("max_arbitrators out of range")
	}
	if p.MaxOpenDisputes == 0 || p.MaxOpenDisputes > HardMaxOpenDisputes {
		return fmt.Errorf("max_open_disputes out of range")
	}
	if p.MaxEvidencePerDispute == 0 || p.MaxEvidencePerDispute > HardMaxEvidencePerDispute {
		return fmt.Errorf("max_evidence_per_dispute out of range")
	}
	if p.PanelSize == 0 || p.PanelSize > HardMaxPanelSize {
		return fmt.Errorf("panel_size out of range")
	}
	if p.PanelSize%2 == 0 {
		// even panels can deadlock at exact 50%/50% split; reject
		return fmt.Errorf("panel_size must be odd")
	}
	if p.QuorumBps == 0 || p.QuorumBps > HardMaxQuorumBps {
		return fmt.Errorf("quorum_bps out of range")
	}
	if p.RespondPeriodSeconds <= 0 || p.RespondPeriodSeconds > HardMaxRespondPeriodSeconds {
		return fmt.Errorf("respond_period_seconds out of range")
	}
	if p.DeliberationPeriodSeconds <= 0 || p.DeliberationPeriodSeconds > HardMaxDeliberationPeriodSeconds {
		return fmt.Errorf("deliberation_period_seconds out of range")
	}
	if p.MinPlaintiffBond == 0 {
		return fmt.Errorf("min_plaintiff_bond must be > 0")
	}
	if p.MaxPlaintiffBond == 0 || p.MaxPlaintiffBond > HardMaxPlaintiffBond {
		return fmt.Errorf("max_plaintiff_bond out of range")
	}
	if p.MinPlaintiffBond > p.MaxPlaintiffBond {
		return fmt.Errorf("min_plaintiff_bond > max_plaintiff_bond")
	}
	if p.DefaultSlashBps > HardMaxSlashBps {
		return fmt.Errorf("default_slash_bps > %d", HardMaxSlashBps)
	}
	if p.MemoMaxLen > HardMemoMaxLen {
		return fmt.Errorf("memo_max_len > %d", HardMemoMaxLen)
	}
	if p.ReasonMaxLen > HardReasonMaxLen {
		return fmt.Errorf("reason_max_len > %d", HardReasonMaxLen)
	}
	if p.UriMaxLen == 0 || p.UriMaxLen > HardURIMaxLen {
		return fmt.Errorf("uri_max_len out of range")
	}
	if p.MaxJurisdictionsPerArbitrator == 0 || p.MaxJurisdictionsPerArbitrator > HardMaxJurisdictionsPerArbitrator {
		return fmt.Errorf("max_jurisdictions_per_arbitrator out of range")
	}
	if p.MaxStandardsPerArbitrator == 0 || p.MaxStandardsPerArbitrator > HardMaxStandardsPerArbitrator {
		return fmt.Errorf("max_standards_per_arbitrator out of range")
	}
	if p.MaxFinalizationsPerBlock == 0 || p.MaxFinalizationsPerBlock > HardMaxFinalizationsPerBlock {
		return fmt.Errorf("max_finalizations_per_block out of range")
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:           DefaultParams(),
		NextArbitratorId: 1,
		NextDisputeId:    1,
	}
}

// Validate enforces shape, uniqueness, and cross-references
// at genesis. Runtime keeper code assumes loaded state already
// satisfies these invariants.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	if uint32(len(gs.Arbitrators)) > gs.Params.MaxArbitrators {
		return fmt.Errorf("genesis: %d arbitrators > max %d", len(gs.Arbitrators), gs.Params.MaxArbitrators)
	}
	arbIDs := map[uint64]bool{}
	arbDIDs := map[string]bool{}
	arbSigners := map[string]bool{}
	for _, a := range gs.Arbitrators {
		if a.Id == 0 {
			return fmt.Errorf("genesis: arbitrator id must be > 0")
		}
		if arbIDs[a.Id] {
			return fmt.Errorf("genesis: duplicate arbitrator id %d", a.Id)
		}
		arbIDs[a.Id] = true
		if a.Id >= gs.NextArbitratorId {
			return fmt.Errorf("genesis: arbitrator id %d >= next_arbitrator_id %d", a.Id, gs.NextArbitratorId)
		}
		if !ArbitratorStatusValid(a.Status) {
			return fmt.Errorf("genesis: arbitrator %d invalid status", a.Id)
		}
		if err := ValidateDID("arbitrator.did", a.Did); err != nil {
			return err
		}
		if arbDIDs[a.Did] {
			return fmt.Errorf("genesis: duplicate arbitrator DID %s", a.Did)
		}
		arbDIDs[a.Did] = true
		if err := ValidateAddr("arbitrator.signer_address", a.SignerAddress); err != nil {
			return err
		}
		if arbSigners[a.SignerAddress] {
			return fmt.Errorf("genesis: duplicate arbitrator signer %s", a.SignerAddress)
		}
		arbSigners[a.SignerAddress] = true
		if err := ValidateNonEmpty("arbitrator.name", a.Name, NameMaxLen); err != nil {
			return err
		}
		if uint32(len(a.Jurisdictions)) > gs.Params.MaxJurisdictionsPerArbitrator {
			return fmt.Errorf("genesis: arbitrator %d too many jurisdictions", a.Id)
		}
		for _, j := range a.Jurisdictions {
			if err := ValidateJurisdiction("arbitrator.jurisdictions", j); err != nil {
				return err
			}
		}
		if uint32(len(a.AccreditedStandards)) > gs.Params.MaxStandardsPerArbitrator {
			return fmt.Errorf("genesis: arbitrator %d too many standards", a.Id)
		}
	}

	disputeIDs := map[uint64]bool{}
	for _, d := range gs.Disputes {
		if d.Id == 0 {
			return fmt.Errorf("genesis: dispute id must be > 0")
		}
		if disputeIDs[d.Id] {
			return fmt.Errorf("genesis: duplicate dispute id %d", d.Id)
		}
		disputeIDs[d.Id] = true
		if d.Id >= gs.NextDisputeId {
			return fmt.Errorf("genesis: dispute id %d >= next_dispute_id %d", d.Id, gs.NextDisputeId)
		}
		if !DisputeStatusValid(d.Status) {
			return fmt.Errorf("genesis: dispute %d invalid status", d.Id)
		}
		if err := ValidateAddr("dispute.plaintiff", d.Plaintiff); err != nil {
			return err
		}
		if err := ValidateAddr("dispute.respondent", d.Respondent); err != nil {
			return err
		}
		if d.Plaintiff == d.Respondent {
			return fmt.Errorf("genesis: dispute %d plaintiff == respondent", d.Id)
		}
		if !SubjectKindValid(d.SubjectKind) {
			return fmt.Errorf("genesis: dispute %d invalid subject_kind", d.Id)
		}
		if err := ValidateSubjectRef(d.SubjectRef); err != nil {
			return err
		}
		if err := ValidateDenom(d.BondDenom); err != nil {
			return err
		}
		if d.PlaintiffBond == 0 {
			return fmt.Errorf("genesis: dispute %d plaintiff_bond=0", d.Id)
		}
		if d.RespondentBondRequired == 0 {
			return fmt.Errorf("genesis: dispute %d respondent_bond_required=0", d.Id)
		}
		if err := ValidateHash("dispute.claim_hash", d.ClaimHash); err != nil {
			return err
		}
		if d.Status == DisputeStatus_DISPUTE_STATUS_RESOLVED {
			if !RulingOutcomeValid(d.Outcome) {
				return fmt.Errorf("genesis: resolved dispute %d invalid outcome", d.Id)
			}
			if d.SlashBps > 10_000 {
				return fmt.Errorf("genesis: resolved dispute %d slash_bps > 10000", d.Id)
			}
			if err := ValidateHash("dispute.ruling_hash", d.RulingHash); err != nil {
				return err
			}
		}
	}
	{
		var openCount uint32
		for _, d := range gs.Disputes {
			if DisputeIsActive(d.Status) {
				openCount++
			}
		}
		if openCount > gs.Params.MaxOpenDisputes {
			return fmt.Errorf("genesis: %d active disputes > max %d", openCount, gs.Params.MaxOpenDisputes)
		}
	}

	tribunalSeen := map[[2]uint64]bool{}
	for _, tm := range gs.TribunalMembers {
		if tm.DisputeId == 0 || tm.ArbitratorId == 0 {
			return fmt.Errorf("genesis: tribunal entry has zero id")
		}
		if !disputeIDs[tm.DisputeId] {
			return fmt.Errorf("genesis: tribunal references unknown dispute %d", tm.DisputeId)
		}
		if !arbIDs[tm.ArbitratorId] {
			return fmt.Errorf("genesis: tribunal references unknown arbitrator %d", tm.ArbitratorId)
		}
		key := [2]uint64{tm.DisputeId, tm.ArbitratorId}
		if tribunalSeen[key] {
			return fmt.Errorf("genesis: duplicate tribunal entry (%d,%d)", tm.DisputeId, tm.ArbitratorId)
		}
		tribunalSeen[key] = true
	}

	voteSeen := map[[2]uint64]bool{}
	for _, v := range gs.Votes {
		if v.DisputeId == 0 || v.ArbitratorId == 0 {
			return fmt.Errorf("genesis: vote has zero id")
		}
		if !disputeIDs[v.DisputeId] {
			return fmt.Errorf("genesis: vote references unknown dispute %d", v.DisputeId)
		}
		if !arbIDs[v.ArbitratorId] {
			return fmt.Errorf("genesis: vote references unknown arbitrator %d", v.ArbitratorId)
		}
		if !VoteChoiceValid(v.Choice) {
			return fmt.Errorf("genesis: vote (%d,%d) invalid choice", v.DisputeId, v.ArbitratorId)
		}
		key := [2]uint64{v.DisputeId, v.ArbitratorId}
		if voteSeen[key] {
			return fmt.Errorf("genesis: duplicate vote (%d,%d)", v.DisputeId, v.ArbitratorId)
		}
		voteSeen[key] = true
	}

	evidenceSeqByDispute := map[uint64]uint64{}
	for _, es := range gs.EvidenceSeqs {
		if es.DisputeId == 0 {
			return fmt.Errorf("genesis: evidence_seq has zero dispute_id")
		}
		if _, dup := evidenceSeqByDispute[es.DisputeId]; dup {
			return fmt.Errorf("genesis: duplicate evidence_seq for dispute %d", es.DisputeId)
		}
		if !disputeIDs[es.DisputeId] {
			return fmt.Errorf("genesis: evidence_seq references unknown dispute %d", es.DisputeId)
		}
		evidenceSeqByDispute[es.DisputeId] = es.NextEvidenceId
	}
	evidencePerDispute := map[uint64]uint32{}
	evidenceSeen := map[[2]uint64]bool{}
	for _, e := range gs.Evidence {
		if e.DisputeId == 0 || e.Id == 0 {
			return fmt.Errorf("genesis: evidence has zero id")
		}
		if !disputeIDs[e.DisputeId] {
			return fmt.Errorf("genesis: evidence references unknown dispute %d", e.DisputeId)
		}
		key := [2]uint64{e.DisputeId, e.Id}
		if evidenceSeen[key] {
			return fmt.Errorf("genesis: duplicate evidence (%d,%d)", e.DisputeId, e.Id)
		}
		evidenceSeen[key] = true
		if err := ValidateAddr("evidence.submitter", e.Submitter); err != nil {
			return err
		}
		if e.Uri == "" {
			return fmt.Errorf("genesis: evidence (%d,%d) empty uri", e.DisputeId, e.Id)
		}
		if err := ValidateHash("evidence.hash", e.Hash); err != nil {
			return err
		}
		if next, ok := evidenceSeqByDispute[e.DisputeId]; ok && e.Id >= next {
			return fmt.Errorf("genesis: evidence (%d,%d) id >= next %d", e.DisputeId, e.Id, next)
		}
		evidencePerDispute[e.DisputeId]++
		if evidencePerDispute[e.DisputeId] > gs.Params.MaxEvidencePerDispute {
			return fmt.Errorf("genesis: dispute %d evidence count > max %d", e.DisputeId, gs.Params.MaxEvidencePerDispute)
		}
	}

	return nil
}
