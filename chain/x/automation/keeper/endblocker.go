package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/automation/types"
)

// EndBlock runs the cron then the stream auto-finalize sweep. Both are bounded
// per block by Params so a backlog cannot blow the block gas budget; leftover
// work is naturally picked up on the next block.
func (k Keeper) EndBlock(goCtx context.Context) error {
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	if err := k.runDueSchedules(ctx, p); err != nil {
		return err
	}
	return k.finalizeDueStreams(ctx, p)
}

func (k Keeper) runDueSchedules(ctx sdk.Context, p types.Params) error {
	now := ctx.BlockTime().Unix()

	// Materialise due ids first so we do not observe our own index mutations
	// during iteration.
	type duePair struct {
		nextRun int64
		id      uint64
	}
	due := make([]duePair, 0, p.MaxRunsPerBlock)
	if err := k.ScheduleByNextRun.Walk(ctx, nil, func(key collections.Pair[int64, uint64]) (bool, error) {
		if key.K1() > now {
			return true, nil
		}
		due = append(due, duePair{key.K1(), key.K2()})
		return uint32(len(due)) >= p.MaxRunsPerBlock, nil
	}); err != nil {
		return err
	}

	for _, d := range due {
		sch, ok, err := k.GetSchedule(ctx, d.id)
		if err != nil {
			return err
		}
		if !ok || sch.Status != types.ScheduleStatus_SCHEDULE_STATUS_ACTIVE {
			// Stale index entry; drop defensively.
			_ = k.ScheduleByNextRun.Remove(ctx, collections.Join(d.nextRun, d.id))
			continue
		}
		if err := k.tickSchedule(ctx, &sch, p, now); err != nil {
			return err
		}
	}
	return nil
}

// tickSchedule executes one schedule run inside a CacheContext so a failing
// cross-module call neither corrupts state nor aborts the EndBlocker; the run
// is still counted and the schedule advanced.
func (k Keeper) tickSchedule(ctx sdk.Context, sch *types.Schedule, p types.Params, now int64) error {
	prevNext, prevStatus := sch.NextRun, sch.Status

	// Defensive: a schedule maintained by this module is never ACTIVE at/over
	// its cap (tickSchedule completes it on the run that reaches the cap), but
	// a malformed genesis could be. Complete it WITHOUT an extra funded run
	// rather than dispatching one more action.
	if (sch.MaxRuns != 0 && sch.RunsDone >= sch.MaxRuns) ||
		(sch.EndTime != 0 && sch.NextRun > sch.EndTime) {
		sch.Status = types.ScheduleStatus_SCHEDULE_STATUS_COMPLETED
		return k.saveSchedule(ctx, *sch, prevNext, prevStatus)
	}

	cacheCtx, write := ctx.CacheContext()
	runErr := k.dispatch(cacheCtx, *sch, p)
	if runErr == nil {
		write()
		sch.Successes++
		sch.LastError = ""
	} else {
		sch.Failures++
		sch.LastError = truncate(runErr.Error(), 256)
	}
	sch.LastRun = now
	sch.RunsDone++

	// Advance and apply terminal conditions.
	if sch.MaxRuns != 0 && sch.RunsDone >= sch.MaxRuns {
		sch.Status = types.ScheduleStatus_SCHEDULE_STATUS_COMPLETED
	} else {
		sch.NextRun = types.AdvanceNextRun(sch.NextRun, sch.IntervalSeconds, now)
		if sch.EndTime != 0 && sch.NextRun > sch.EndTime {
			sch.Status = types.ScheduleStatus_SCHEDULE_STATUS_COMPLETED
		}
	}
	if err := k.saveSchedule(ctx, *sch, prevNext, prevStatus); err != nil {
		return err
	}
	if runErr == nil {
		emitEvent(ctx, types.EventTypeTick, types.AttrAction, "ok", types.AttrScheduleID, u(sch.Id),
			types.AttrNextRun, i(sch.NextRun), types.AttrStatus, sch.Status.String())
	} else {
		emitEvent(ctx, types.EventTypeTick, types.AttrAction, "fail", types.AttrScheduleID, u(sch.Id),
			types.AttrError, sch.LastError)
	}
	return nil
}

// dispatch routes a schedule to its typed cross-module action. The creator is
// always the acting authority; the target module re-checks authorization
// (token admin, sanctions, market state) at execution time.
func (k Keeper) dispatch(ctx sdk.Context, sch types.Schedule, p types.Params) error {
	switch sch.Action {
	case types.ActionKind_ACTION_KIND_MINCAST_INJECT:
		if k.mincast == nil {
			return fmt.Errorf("mincast keeper not wired")
		}
		return k.mincast.InjectTreasuryFrom(ctx, sch.Creator, sch.TargetId, sch.AmountPerRun)
	case types.ActionKind_ACTION_KIND_MINCAST_CLOSE_MATURED:
		if k.mincast == nil {
			return fmt.Errorf("mincast keeper not wired")
		}
		_, err := k.mincast.CloseMaturedInvests(ctx, sch.Creator, sch.TargetId, p.MaxClosePerRun)
		return err
	case types.ActionKind_ACTION_KIND_RWA_SNAPSHOT:
		if k.rwa == nil {
			return fmt.Errorf("rwa keeper not wired")
		}
		_, err := k.rwa.SnapshotHolders(ctx, sch.Creator, sch.TargetId)
		return err
	case types.ActionKind_ACTION_KIND_RWA_DIVIDEND:
		if k.rwa == nil {
			return fmt.Errorf("rwa keeper not wired")
		}
		snapID, err := k.rwa.SnapshotHolders(ctx, sch.Creator, sch.TargetId)
		if err != nil {
			return err
		}
		_, err = k.rwa.DistributeDividend(ctx, sch.Creator, sch.TargetId, snapID, sch.AmountPerRun)
		return err
	default:
		return fmt.Errorf("unknown action %v", sch.Action)
	}
}

// finalizeDueStreams pays out streams whose vesting has completed (now >=
// stop_time) so receivers are settled even if they never call WithdrawStream.
// Bounded per block; sanctioned-receiver or zero-remainder streams are skipped
// (left ACTIVE) rather than erroring the block.
func (k Keeper) finalizeDueStreams(ctx sdk.Context, p types.Params) error {
	now := ctx.BlockTime().Unix()

	type duePair struct {
		stop int64
		id   uint64
	}
	due := make([]duePair, 0, p.MaxFinalizePerBlock)
	if err := k.StreamByStop.Walk(ctx, nil, func(key collections.Pair[int64, uint64]) (bool, error) {
		if key.K1() > now {
			return true, nil
		}
		due = append(due, duePair{key.K1(), key.K2()})
		return uint32(len(due)) >= p.MaxFinalizePerBlock, nil
	}); err != nil {
		return err
	}

	for _, d := range due {
		st, ok, err := k.GetStream(ctx, d.id)
		if err != nil {
			return err
		}
		if !ok || st.Status != types.StreamStatus_STREAM_STATUS_ACTIVE {
			_ = k.StreamByStop.Remove(ctx, collections.Join(d.stop, d.id))
			continue
		}
		// Finalize inside a cache so a single bad stream (e.g. sanctioned
		// receiver) cannot abort the sweep.
		cacheCtx, write := ctx.CacheContext()
		amount, werr := k.withdrawStreamTo(cacheCtx, &st, now)
		if werr != nil {
			continue
		}
		write()
		emitEvent(ctx, types.EventTypeStream, types.AttrAction, "finalize", types.AttrStreamID, u(st.Id),
			types.AttrReceiver, st.Receiver, types.AttrAmount, u(amount))
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
