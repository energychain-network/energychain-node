package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/scheduler/types"
)

// EndBlock walks the by-next-run index and runs every job whose
// next_run_time <= block_time, up to Params.MaxJobsPerBlock.
//
// Per-tick semantics:
//
//  1. Re-check fee budget against fee_per_run. If the budget cannot
//     cover the run, the job transitions to EXHAUSTED, is removed
//     from the by-next-run index, and is NOT executed (no fee
//     debit). This is the soft-pause path.
//  2. Debit fee_per_run from pool. Update fees_spent / budget on
//     the row.
//  3. Execute the payload inside a CacheContext so a panic / error
//     does not corrupt module state or interrupt the iteration
//     over the remaining due jobs. Success increments successes
//     and clears last_error; failure increments failures, records
//     a (truncated) last_error, and lets the schedule keep
//     ticking. Failure does NOT refund the per-run fee — see Job
//     comment.
//  4. Advance next_run_time using AdvanceNextRun (skips past
//     missed slots in O(1)). If the new next_run_time would be
//     >= end_time, or executions_done >= max_executions, the row
//     transitions to EXHAUSTED.
//  5. Persist the row, re-index by next_run_time when still ACTIVE.
//
// The function returns the number of jobs touched and the number
// successfully executed for tracing / metrics.
func (k Keeper) EndBlock(goCtx context.Context) (touched, succeeded int, err error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	now := ctx.BlockTime().Unix()

	p, err := k.GetParams(ctx)
	if err != nil {
		return 0, 0, err
	}
	cap := p.MaxJobsPerBlock

	// First pass: collect ids of due jobs. We materialise into a
	// slice so the iterator does not see our own index mutations
	// (collections does not guarantee safe in-place mutation
	// during a Walk). Bounded by cap so the slice cost is small.
	type duePair struct {
		nextRun int64
		id      uint64
	}
	due := make([]duePair, 0, cap)
	if err := k.JobByNextRun.Walk(ctx, nil, func(key collections.Pair[int64, uint64]) (bool, error) {
		if key.K1() > now {
			return true, nil
		}
		due = append(due, duePair{key.K1(), key.K2()})
		if uint32(len(due)) >= cap {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return 0, 0, err
	}

	for _, d := range due {
		touched++
		j, ok, err := k.GetJob(ctx, d.id)
		if err != nil || !ok {
			// Stale index entry. Drop it defensively so future
			// blocks do not re-encounter the same phantom row.
			_ = k.JobByNextRun.Remove(ctx, collections.Join(d.nextRun, d.id))
			continue
		}
		if j.Status != types.Status_STATUS_ACTIVE {
			// Defensive: should be impossible because non-ACTIVE
			// jobs are de-indexed eagerly, but if invariants drift
			// (e.g. genesis import bug), clean up here too.
			_ = k.JobByNextRun.Remove(ctx, collections.Join(d.nextRun, d.id))
			continue
		}
		if ok, err := k.tick(ctx, &j, p, now); err != nil {
			return touched, succeeded, err
		} else if ok {
			succeeded++
		}
	}
	return touched, succeeded, nil
}

// tick is the per-job state machine the EndBlocker runs. Returns
// true iff the payload was routed AND its handler returned a nil
// error. State mutations are committed regardless (the per-tick
// fee is consumed even on handler failure, mirroring L1 gas
// semantics).
func (k Keeper) tick(ctx sdk.Context, j *types.Job, p types.Params, now int64) (bool, error) {
	prevNextRun := j.NextRunTime
	prevStatus := j.Status

	// Pre-flight: budget gate. EXHAUSTED transition stays here so
	// we never attempt to Move() with insufficient funds.
	if j.FeeBudget < j.FeePerRun {
		j.Status = types.Status_STATUS_EXHAUSTED
		if err := k.SaveJob(ctx, *j, prevNextRun, prevStatus); err != nil {
			return false, err
		}
		k.recordAudit(ctx, j.Id, "exhausted_budget", j.Owner, fmt.Sprintf("budget=%d fee=%d", j.FeeBudget, j.FeePerRun))
		k.emit(ctx, "exhausted",
			"id", u64s(j.Id),
			"reason", "budget",
		)
		return false, nil
	}

	// Debit fee_per_run from the pool. Any failure here (e.g. the
	// pool was drained by an out-of-band bug) is a hard error
	// that we surface so it gets investigated.
	if err := k.burnOrCollect(ctx, p, j.FeeDenom, j.FeePerRun); err != nil {
		return false, fmt.Errorf("scheduler tick: fee transfer failed for job %d: %w", j.Id, err)
	}
	newBudget, err := types.SafeSub(j.FeeBudget, j.FeePerRun)
	if err != nil {
		return false, err
	}
	j.FeeBudget = newBudget
	newSpent, err := types.SafeAdd(j.FeesSpent, j.FeePerRun)
	if err != nil {
		return false, err
	}
	j.FeesSpent = newSpent

	// Execute inside a CacheContext so a faulty handler does not
	// corrupt module state nor abort the rest of the EndBlocker.
	executed := false
	if k.router == nil {
		j.LastError = "no router wired"
		j.LastErrorTime = now
		j.Failures++
	} else {
		msg, err := k.UnpackPayload(j.Payload)
		if err != nil {
			j.LastError = types.TruncateError(err.Error(), p.ErrorMaxLen)
			j.LastErrorTime = now
			j.Failures++
		} else {
			cacheCtx, write := ctx.CacheContext()
			err = safeRoute(cacheCtx, k.router, msg)
			if err == nil {
				write()
				j.LastError = ""
				j.LastSuccessTime = now
				j.Successes++
				executed = true
			} else {
				j.LastError = types.TruncateError(err.Error(), p.ErrorMaxLen)
				j.LastErrorTime = now
				j.Failures++
			}
		}
	}

	doneInc, err := types.SafeAdd(j.ExecutionsDone, 1)
	if err != nil {
		return false, err
	}
	j.ExecutionsDone = doneInc

	// Compute the next run. Cap-driven transitions to EXHAUSTED
	// short-circuit the by-time index reinsertion.
	if j.MaxExecutions != 0 && j.ExecutionsDone >= j.MaxExecutions {
		j.Status = types.Status_STATUS_EXHAUSTED
	} else {
		j.NextRunTime = types.AdvanceNextRun(j.StartTime, j.NextRunTime, j.IntervalSeconds, now)
		if j.EndTime != 0 && j.NextRunTime >= j.EndTime {
			j.Status = types.Status_STATUS_EXHAUSTED
		}
	}

	if err := k.SaveJob(ctx, *j, prevNextRun, prevStatus); err != nil {
		return false, err
	}

	if executed {
		k.emit(ctx, "tick",
			"id", u64s(j.Id),
			"status", j.Status.String(),
			"next_run", i64s(j.NextRunTime),
			"budget", u64s(j.FeeBudget),
		)
	} else {
		k.emit(ctx, "tick_failed",
			"id", u64s(j.Id),
			"error", j.LastError,
			"next_run", i64s(j.NextRunTime),
		)
	}
	return executed, nil
}

// safeRoute wraps the router call in a recover so a panicking
// handler is converted into a normal error and the EndBlocker
// continues with the next job instead of aborting the whole
// block.
func safeRoute(ctx sdk.Context, router types.MsgRouter, msg sdk.Msg) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("payload panicked: %v", r)
		}
	}()
	return router.RouteMsg(ctx, msg)
}
