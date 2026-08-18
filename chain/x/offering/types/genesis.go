package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	offerings := map[uint64]Offering{}
	var maxID uint64
	for _, o := range gs.Offerings {
		if o.Id == 0 {
			return fmt.Errorf("genesis: offering id must be > 0")
		}
		if _, dup := offerings[o.Id]; dup {
			return fmt.Errorf("genesis: duplicate offering id %d", o.Id)
		}
		if _, err := sdk.AccAddressFromBech32(o.Issuer); err != nil {
			return fmt.Errorf("genesis: offering %d issuer invalid: %w", o.Id, err)
		}
		if o.UnitPrice == 0 {
			return fmt.Errorf("genesis: offering %d unit_price must be > 0", o.Id)
		}
		if o.HardCap == 0 || o.SoftCap > o.HardCap {
			return fmt.Errorf("genesis: offering %d invalid caps", o.Id)
		}
		if !OfferingStatusValid(o.Status) {
			return fmt.Errorf("genesis: offering %d invalid status", o.Id)
		}
		if o.TotalTranches == 0 || o.TotalTranches > gs.Params.MaxTranches {
			return fmt.Errorf("genesis: offering %d total_tranches out of range", o.Id)
		}
		if o.ReleasedTranches > o.TotalTranches {
			return fmt.Errorf("genesis: offering %d released_tranches > total", o.Id)
		}
		if o.InjectionsDone > o.TotalTranches {
			return fmt.Errorf("genesis: offering %d injections_done > total", o.Id)
		}
		if o.ReleasedTranches > o.InjectionsDone {
			return fmt.Errorf("genesis: offering %d released %d > injected %d", o.Id, o.ReleasedTranches, o.InjectionsDone)
		}
		if o.Raised > o.HardCap {
			return fmt.Errorf("genesis: offering %d raised %d > hard_cap %d", o.Id, o.Raised, o.HardCap)
		}
		if o.Raised%o.UnitPrice != 0 {
			return fmt.Errorf("genesis: offering %d raised not a multiple of unit_price", o.Id)
		}
		if o.ReleasedAmount > o.Raised {
			return fmt.Errorf("genesis: offering %d released_amount > raised", o.Id)
		}
		offerings[o.Id] = o
		if o.Id > maxID {
			maxID = o.Id
		}
	}
	if uint32(len(gs.Offerings)) > gs.Params.MaxOfferings {
		return fmt.Errorf("genesis: offerings exceed max")
	}

	contribSum := map[uint64]uint64{}
	unitSum := map[uint64]uint64{}
	seen := map[string]bool{}
	for _, s := range gs.Subscriptions {
		o, ok := offerings[s.OfferingId]
		if !ok {
			return fmt.Errorf("genesis: subscription references unknown offering %d", s.OfferingId)
		}
		if _, err := sdk.AccAddressFromBech32(s.Investor); err != nil {
			return fmt.Errorf("genesis: subscription investor invalid: %w", err)
		}
		key := fmt.Sprintf("%d|%s", s.OfferingId, s.Investor)
		if seen[key] {
			return fmt.Errorf("genesis: duplicate subscription %s", key)
		}
		seen[key] = true
		if s.Contributed == 0 {
			return fmt.Errorf("genesis: subscription %s zero contribution", key)
		}
		if s.Contributed%o.UnitPrice != 0 {
			return fmt.Errorf("genesis: subscription %s contribution not a multiple of unit_price", key)
		}
		if s.Units != s.Contributed/o.UnitPrice {
			return fmt.Errorf("genesis: subscription %s units mismatch", key)
		}
		cs, err := SafeAdd(contribSum[s.OfferingId], s.Contributed)
		if err != nil {
			return fmt.Errorf("genesis: offering %d contribution overflow", s.OfferingId)
		}
		contribSum[s.OfferingId] = cs
		unitSum[s.OfferingId] += s.Units
	}
	for id, o := range offerings {
		if contribSum[id] != o.Raised {
			return fmt.Errorf("genesis: offering %d raised %d != sum of contributions %d", id, o.Raised, contribSum[id])
		}
		switch o.Status {
		case OfferingStatus_OFFERING_STATUS_SUCCEEDED, OfferingStatus_OFFERING_STATUS_DEFAULTED:
			if o.AllocatedUnits != o.Raised/o.UnitPrice {
				return fmt.Errorf("genesis: offering %d allocated_units mismatch", id)
			}
		}
	}

	if gs.OfferingIdSeq < maxID {
		return fmt.Errorf("genesis: offering_id_seq %d < max id %d", gs.OfferingIdSeq, maxID)
	}
	return nil
}
