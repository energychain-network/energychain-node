package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxOpenCycles:          DefaultMaxOpenCycles,
		MaxObligationsPerCycle: DefaultMaxObligationsPerCycle,
		MaxMembers:             DefaultMaxMembers,
		MemoMaxLen:             DefaultMemoMaxLen,
		SourceRefMaxLen:        DefaultSourceRefMaxLen,
	}
}

func (p Params) Validate() error {
	if p.MaxOpenCycles == 0 || p.MaxOpenCycles > HardMaxOpenCycles {
		return fmt.Errorf("max_open_cycles out of range")
	}
	if p.MaxObligationsPerCycle == 0 || p.MaxObligationsPerCycle > HardMaxObligationsPerCycle {
		return fmt.Errorf("max_obligations_per_cycle out of range")
	}
	if p.MaxMembers == 0 || p.MaxMembers > HardMaxMembers {
		return fmt.Errorf("max_members out of range")
	}
	if p.MemoMaxLen > HardMemoMaxLen {
		return fmt.Errorf("memo_max_len > %d", HardMemoMaxLen)
	}
	if p.SourceRefMaxLen > HardSourceRefMaxLen {
		return fmt.Errorf("source_ref_max_len > %d", HardSourceRefMaxLen)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:           DefaultParams(),
		NextMemberId:     1,
		NextCycleId:      1,
		NextObligationId: 1,
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}
	if uint32(len(gs.Members)) > gs.Params.MaxMembers {
		return fmt.Errorf("genesis: %d members > max %d", len(gs.Members), gs.Params.MaxMembers)
	}
	memberIDs := map[uint64]bool{}
	memberAddrs := map[string]bool{}
	for _, m := range gs.Members {
		if m.Id == 0 {
			return fmt.Errorf("genesis: member id must be > 0")
		}
		if memberIDs[m.Id] {
			return fmt.Errorf("genesis: duplicate member id %d", m.Id)
		}
		memberIDs[m.Id] = true
		if m.Id >= gs.NextMemberId {
			return fmt.Errorf("genesis: member id %d >= next_member_id %d", m.Id, gs.NextMemberId)
		}
		if !MemberStatusValid(m.Status) {
			return fmt.Errorf("genesis: member %d invalid status", m.Id)
		}
		if err := ValidateAddr("member.address", m.Address); err != nil {
			return err
		}
		if memberAddrs[m.Address] {
			return fmt.Errorf("genesis: duplicate member address %s", m.Address)
		}
		memberAddrs[m.Address] = true
		for d, v := range m.RequiredMargin {
			if err := ValidateDenom(d); err != nil {
				return err
			}
			if v == 0 {
				return fmt.Errorf("genesis: member %d required_margin[%s] is zero (omit instead)", m.Id, d)
			}
		}
		for d := range m.DefaultFundShare {
			if err := ValidateDenom(d); err != nil {
				return err
			}
		}
	}
	cycleIDs := map[uint64]bool{}
	for _, c := range gs.Cycles {
		if c.Id == 0 {
			return fmt.Errorf("genesis: cycle id must be > 0")
		}
		if cycleIDs[c.Id] {
			return fmt.Errorf("genesis: duplicate cycle id %d", c.Id)
		}
		cycleIDs[c.Id] = true
		if c.Id >= gs.NextCycleId {
			return fmt.Errorf("genesis: cycle id %d >= next_cycle_id %d", c.Id, gs.NextCycleId)
		}
		if !CycleStatusValid(c.Status) {
			return fmt.Errorf("genesis: cycle %d invalid status", c.Id)
		}
	}
	oblIDs := map[uint64]bool{}
	for _, o := range gs.Obligations {
		if o.Id == 0 {
			return fmt.Errorf("genesis: obligation id must be > 0")
		}
		if oblIDs[o.Id] {
			return fmt.Errorf("genesis: duplicate obligation id %d", o.Id)
		}
		oblIDs[o.Id] = true
		if o.Id >= gs.NextObligationId {
			return fmt.Errorf("genesis: obligation id %d >= next_obligation_id %d", o.Id, gs.NextObligationId)
		}
		if !cycleIDs[o.CycleId] {
			return fmt.Errorf("genesis: obligation %d references unknown cycle %d", o.Id, o.CycleId)
		}
		if !memberIDs[o.FromMemberId] || !memberIDs[o.ToMemberId] {
			return fmt.Errorf("genesis: obligation %d references unknown member", o.Id)
		}
		if o.FromMemberId == o.ToMemberId {
			return fmt.Errorf("genesis: obligation %d self-leg (member %d)", o.Id, o.FromMemberId)
		}
		if err := ValidateDenom(o.Denom); err != nil {
			return err
		}
		if o.Amount == 0 {
			return fmt.Errorf("genesis: obligation %d amount is zero", o.Id)
		}
	}
	for _, e := range gs.DefaultEvents {
		if !cycleIDs[e.CycleId] {
			return fmt.Errorf("genesis: default_event references unknown cycle %d", e.CycleId)
		}
		if !memberIDs[e.MemberId] {
			return fmt.Errorf("genesis: default_event references unknown member %d", e.MemberId)
		}
	}
	for _, m := range gs.Margin {
		if !memberIDs[m.MemberId] {
			return fmt.Errorf("genesis: margin row references unknown member %d", m.MemberId)
		}
		if err := ValidateDenom(m.Denom); err != nil {
			return err
		}
	}
	for _, df := range gs.DefaultFund {
		if err := ValidateDenom(df.Denom); err != nil {
			return err
		}
	}
	// Reservations must reference known members and known
	// denoms; they may legitimately be zero in the export but
	// should be omitted, so we treat zero rows as an error to
	// keep the export tight and force callers to round-trip
	// cleanly.
	seenResv := map[string]bool{}
	marginByKey := map[string]uint64{}
	for _, mg := range gs.Margin {
		marginByKey[fmt.Sprintf("%d/%s", mg.MemberId, mg.Denom)] = mg.Amount
	}
	for _, rv := range gs.Reservations {
		if !memberIDs[rv.MemberId] {
			return fmt.Errorf("genesis: reservation references unknown member %d", rv.MemberId)
		}
		if err := ValidateDenom(rv.Denom); err != nil {
			return err
		}
		if rv.Amount == 0 {
			return fmt.Errorf("genesis: reservation member=%d denom=%s amount=0 (omit instead)", rv.MemberId, rv.Denom)
		}
		key := fmt.Sprintf("%d/%s", rv.MemberId, rv.Denom)
		if seenResv[key] {
			return fmt.Errorf("genesis: duplicate reservation row member=%d denom=%s", rv.MemberId, rv.Denom)
		}
		seenResv[key] = true
		// Genesis invariant: reservation must be ≤ margin for
		// that (member, denom) pair, otherwise the chain
		// boots in an unrecoverable under-margined state.
		if cur := marginByKey[key]; rv.Amount > cur {
			return fmt.Errorf("genesis: reservation member=%d denom=%s amount=%d > margin=%d", rv.MemberId, rv.Denom, rv.Amount, cur)
		}
	}
	return nil
}
