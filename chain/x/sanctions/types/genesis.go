package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxLists:                 DefaultMaxLists,
		MaxEntriesPerList:        DefaultMaxEntriesPerList,
		MaxProposalActions:       DefaultMaxProposalActions,
		HitsLogMax:               DefaultHitsLogMax,
		MaxPendingProposals:      DefaultMaxPendingProposals,
		RequireProposalForAdds:   false,
	}
}

func (p Params) Validate() error {
	if p.MaxLists == 0 || p.MaxLists > MaxListsUpper {
		return fmt.Errorf("max_lists %d outside (0, %d]", p.MaxLists, MaxListsUpper)
	}
	if p.MaxEntriesPerList == 0 || p.MaxEntriesPerList > MaxEntriesPerListUpper {
		return fmt.Errorf("max_entries_per_list %d outside (0, %d]", p.MaxEntriesPerList, MaxEntriesPerListUpper)
	}
	if p.MaxProposalActions == 0 || p.MaxProposalActions > MaxProposalActionsUpper {
		return fmt.Errorf("max_proposal_actions %d outside (0, %d]", p.MaxProposalActions, MaxProposalActionsUpper)
	}
	if p.HitsLogMax > HitsLogMaxUpper {
		return fmt.Errorf("hits_log_max %d exceeds %d", p.HitsLogMax, HitsLogMaxUpper)
	}
	if p.MaxPendingProposals == 0 || p.MaxPendingProposals > MaxPendingProposalsUpper {
		return fmt.Errorf("max_pending_proposals %d outside (0, %d]", p.MaxPendingProposals, MaxPendingProposalsUpper)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:    DefaultParams(),
		Lists:     []SanctionList{},
		Entries:   []SanctionEntry{},
		Proposals: []Proposal{},
		Hits:      []SanctionHit{},
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("params: %w", err)
	}
	if uint32(len(gs.Lists)) > gs.Params.MaxLists {
		return fmt.Errorf("lists count %d exceeds max %d", len(gs.Lists), gs.Params.MaxLists)
	}

	listSeen := make(map[string]bool, len(gs.Lists))
	for i, l := range gs.Lists {
		if err := ValidateListID(l.Id); err != nil {
			return fmt.Errorf("list %d: %w", i, err)
		}
		if err := ValidateListName(l.Name); err != nil {
			return fmt.Errorf("list %d: %w", i, err)
		}
		if err := ValidateJurisdiction(l.Jurisdiction); err != nil {
			return fmt.Errorf("list %d: %w", i, err)
		}
		if err := ValidateSourceURI(l.SourceUri); err != nil {
			return fmt.Errorf("list %d: %w", i, err)
		}
		if !ListStatusValid(l.Status) {
			return fmt.Errorf("list %d: invalid status", i)
		}
		if listSeen[l.Id] {
			return fmt.Errorf("list %d: duplicate id %s", i, l.Id)
		}
		listSeen[l.Id] = true
	}

	type entryKey struct{ list, subject string }
	entrySeen := make(map[entryKey]bool, len(gs.Entries))
	for i, e := range gs.Entries {
		if !listSeen[e.ListId] {
			return fmt.Errorf("entry %d: references unknown list %s", i, e.ListId)
		}
		if err := ValidateSubject(e.Subject); err != nil {
			return fmt.Errorf("entry %d: %w", i, err)
		}
		if !SubjectKindValid(e.SubjectKind) {
			return fmt.Errorf("entry %d: invalid subject_kind", i)
		}
		if !EntryStatusValid(e.Status) {
			return fmt.Errorf("entry %d: invalid status", i)
		}
		if err := ValidateProgram(e.Program); err != nil {
			return fmt.Errorf("entry %d: %w", i, err)
		}
		if err := ValidateReason(e.Reason); err != nil {
			return fmt.Errorf("entry %d: %w", i, err)
		}
		if err := ValidateSourceRef(e.SourceRef); err != nil {
			return fmt.Errorf("entry %d: %w", i, err)
		}
		key := entryKey{list: e.ListId, subject: e.Subject}
		if entrySeen[key] {
			return fmt.Errorf("entry %d: duplicate (list, subject) %s/%s", i, e.ListId, e.Subject)
		}
		entrySeen[key] = true
	}

	propSeen := make(map[uint64]bool, len(gs.Proposals))
	for i, p := range gs.Proposals {
		if p.Id == 0 {
			return fmt.Errorf("proposal %d: id must be > 0", i)
		}
		if propSeen[p.Id] {
			return fmt.Errorf("proposal %d: duplicate id %d", i, p.Id)
		}
		propSeen[p.Id] = true
		if !listSeen[p.ListId] {
			return fmt.Errorf("proposal %d: references unknown list %s", i, p.ListId)
		}
		if !ProposalStatusValid(p.Status) {
			return fmt.Errorf("proposal %d: invalid status", i)
		}
		if uint32(len(p.Actions)) > gs.Params.MaxProposalActions {
			return fmt.Errorf("proposal %d: too many actions", i)
		}
		for j, a := range p.Actions {
			if !ProposalActionKindValid(a.Kind) {
				return fmt.Errorf("proposal %d action %d: invalid kind", i, j)
			}
		}
		if p.Id > gs.ProposalCounter {
			return fmt.Errorf("proposal %d: id %d exceeds counter %d", i, p.Id, gs.ProposalCounter)
		}
	}

	hitSeen := make(map[uint64]bool, len(gs.Hits))
	for i, h := range gs.Hits {
		if h.Id == 0 {
			return fmt.Errorf("hit %d: id must be > 0", i)
		}
		if hitSeen[h.Id] {
			return fmt.Errorf("hit %d: duplicate id %d", i, h.Id)
		}
		hitSeen[h.Id] = true
		if h.Id > gs.HitCounter {
			return fmt.Errorf("hit %d: id %d exceeds counter %d", i, h.Id, gs.HitCounter)
		}
	}
	return nil
}
