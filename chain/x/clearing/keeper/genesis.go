package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"

	"energychain/x/clearing/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if gs == nil {
		gs = types.DefaultGenesis()
	}
	if err := gs.Validate(); err != nil {
		return fmt.Errorf("clearing: invalid genesis: %w", err)
	}
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return err
	}
	if gs.NextMemberId > 0 {
		if err := k.MemberIDSeq.Set(ctx, gs.NextMemberId-1); err != nil {
			return err
		}
	}
	if gs.NextCycleId > 0 {
		if err := k.CycleIDSeq.Set(ctx, gs.NextCycleId-1); err != nil {
			return err
		}
	}
	if gs.NextObligationId > 0 {
		if err := k.ObligationIDSeq.Set(ctx, gs.NextObligationId-1); err != nil {
			return err
		}
	}
	for _, m := range gs.Members {
		if err := k.Members.Set(ctx, m.Id, m); err != nil {
			return err
		}
		if err := k.MemberByAddr.Set(ctx, m.Address, m.Id); err != nil {
			return err
		}
	}
	for _, c := range gs.Cycles {
		if err := k.Cycles.Set(ctx, c.Id, c); err != nil {
			return err
		}
	}
	for _, o := range gs.Obligations {
		if err := k.Obligations.Set(ctx, o.Id, o); err != nil {
			return err
		}
		if err := k.ObligationByCycle.Set(ctx, collections.Join(o.CycleId, o.Id)); err != nil {
			return err
		}
	}
	for _, np := range gs.NetPositions {
		if err := k.NetPositions.Set(ctx, collections.Join3(np.CycleId, np.MemberId, np.Denom), np); err != nil {
			return err
		}
	}
	for _, ev := range gs.DefaultEvents {
		if err := k.DefaultEvents.Set(ctx, collections.Join3(ev.CycleId, ev.MemberId, ev.Denom), ev); err != nil {
			return err
		}
	}
	for _, mg := range gs.Margin {
		if mg.Amount == 0 {
			continue
		}
		if err := k.Margin.Set(ctx, collections.Join(mg.MemberId, mg.Denom), mg.Amount); err != nil {
			return err
		}
	}
	for _, df := range gs.DefaultFund {
		if df.Amount == 0 {
			continue
		}
		if err := k.DefaultFund.Set(ctx, df.Denom, df.Amount); err != nil {
			return err
		}
	}
	for _, rv := range gs.Reservations {
		if rv.Amount == 0 {
			continue
		}
		if err := k.Reservation.Set(ctx, collections.Join(rv.MemberId, rv.Denom), rv.Amount); err != nil {
			return err
		}
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	p, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	mNext, err := k.MemberIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	cNext, err := k.CycleIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	oNext, err := k.ObligationIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	out := types.GenesisState{
		Params:           p,
		NextMemberId:     mNext + 1,
		NextCycleId:      cNext + 1,
		NextObligationId: oNext + 1,
	}
	if err := k.Members.Walk(ctx, nil, func(_ uint64, v types.Member) (bool, error) {
		out.Members = append(out.Members, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Cycles.Walk(ctx, nil, func(_ uint64, v types.Cycle) (bool, error) {
		out.Cycles = append(out.Cycles, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Obligations.Walk(ctx, nil, func(_ uint64, v types.Obligation) (bool, error) {
		out.Obligations = append(out.Obligations, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.NetPositions.Walk(ctx, nil, func(_ collections.Triple[uint64, uint64, string], v types.NetPosition) (bool, error) {
		out.NetPositions = append(out.NetPositions, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.DefaultEvents.Walk(ctx, nil, func(_ collections.Triple[uint64, uint64, string], v types.DefaultEvent) (bool, error) {
		out.DefaultEvents = append(out.DefaultEvents, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Margin.Walk(ctx, nil, func(key collections.Pair[uint64, string], v uint64) (bool, error) {
		out.Margin = append(out.Margin, types.MarginEntry{
			MemberId: key.K1(), Denom: key.K2(), Amount: v,
		})
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.DefaultFund.Walk(ctx, nil, func(d string, v uint64) (bool, error) {
		out.DefaultFund = append(out.DefaultFund, types.DefaultFundEntry{Denom: d, Amount: v})
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Reservation.Walk(ctx, nil, func(key collections.Pair[uint64, string], v uint64) (bool, error) {
		out.Reservations = append(out.Reservations, types.ReservationEntry{
			MemberId: key.K1(), Denom: key.K2(), Amount: v,
		})
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &out, nil
}
