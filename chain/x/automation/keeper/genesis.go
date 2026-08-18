package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/automation/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	for _, s := range gs.Schedules {
		if err := k.saveSchedule(ctx, s, 0, types.ScheduleStatus_SCHEDULE_STATUS_UNSPECIFIED); err != nil {
			return err
		}
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	for _, st := range gs.Streams {
		// An ACTIVE stream whose withdrawn exceeds the amount vested at genesis
		// time would underflow every subsequent withdraw/cancel (SafeSub),
		// stranding the deposit. Genesis Validate cannot see block time, so the
		// time-dependent invariant is enforced here.
		if st.Status == types.StreamStatus_STREAM_STATUS_ACTIVE {
			vested := types.VestedAmount(st.Deposit, st.RatePerSec, st.StartTime, st.StopTime, now)
			if st.Withdrawn > vested {
				return types.ErrStreamState.Wrapf("stream %d withdrawn %d exceeds vested %d at genesis", st.Id, st.Withdrawn, vested)
			}
		}
		if err := k.saveStream(ctx, st, 0, types.StreamStatus_STREAM_STATUS_UNSPECIFIED); err != nil {
			return err
		}
	}
	// Reconcile the single escrow account against the outstanding balance of
	// ACTIVE streams (x/stableusd loads first in genesisModuleOrder). A
	// mismatch means withdrawals would over- or under-pay, so fail closed.
	// NOTE: this checks every denom that backs an active stream exactly; it
	// cannot detect surplus escrow parked on a denom with NO active stream
	// (the SettlementKeeper interface intentionally exposes no enumeration).
	// Such stranded balances are a genesis-authoring error and have no release
	// path, so genesis for this module is authored empty in production.
	if k.settlement != nil {
		want, err := gs.EscrowByDenom()
		if err != nil {
			return err
		}
		for denom, amt := range want {
			got := k.settlement.GetBalance(ctx, denom, StreamEscrowAccount())
			if got != amt {
				return types.ErrSettlement.Wrapf("stream escrow %s: balance %d != outstanding %d", denom, got, amt)
			}
		}
	}
	if err := k.ScheduleIDSeq.Set(ctx, gs.ScheduleIdSeq); err != nil {
		return err
	}
	return k.StreamIDSeq.Set(ctx, gs.StreamIdSeq)
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	gs := types.DefaultGenesis()
	p, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	gs.Params = p

	if err := k.Schedules.Walk(ctx, nil, func(_ uint64, s types.Schedule) (bool, error) {
		gs.Schedules = append(gs.Schedules, s)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Streams.Walk(ctx, nil, func(_ uint64, s types.Stream) (bool, error) {
		gs.Streams = append(gs.Streams, s)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if gs.ScheduleIdSeq, err = k.ScheduleIDSeq.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.StreamIdSeq, err = k.StreamIDSeq.Peek(ctx); err != nil {
		return nil, err
	}
	return gs, nil
}
