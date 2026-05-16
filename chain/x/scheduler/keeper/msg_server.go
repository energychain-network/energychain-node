package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/scheduler/types"
)

type msgServer struct{ k Keeper }

// NewMsgServerImpl wires the auto-generated MsgServer interface
// over the Keeper.
func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{k: k} }

// ---- helpers -------------------------------------------------------------

func (s msgServer) requireAuthority(authority string) error {
	if authority != s.k.authority {
		return fmt.Errorf("invalid authority: expected %s, got %s", s.k.authority, authority)
	}
	return nil
}

// loadAndCheckOwned loads the job and verifies ownership +
// allowed-status precondition. Returns the loaded row plus the
// current params (callers commonly need both).
func (s msgServer) loadAndCheckOwned(ctx context.Context, id uint64, caller string, allowed ...types.Status) (types.Job, types.Params, error) {
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return types.Job{}, types.Params{}, err
	}
	j, err := s.k.MustGetJob(ctx, id)
	if err != nil {
		return types.Job{}, types.Params{}, err
	}
	if j.Owner != caller {
		return types.Job{}, types.Params{}, fmt.Errorf("only owner %s may operate on job %d", j.Owner, id)
	}
	if len(allowed) == 0 {
		return j, p, nil
	}
	for _, a := range allowed {
		if j.Status == a {
			return j, p, nil
		}
	}
	return types.Job{}, types.Params{}, fmt.Errorf("job %d in status %s, expected one of %v", id, j.Status, allowed)
}

// ---- CreateJob -----------------------------------------------------------

func (s msgServer) CreateJob(goCtx context.Context, m *types.MsgCreateJob) (*types.MsgCreateJobResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}

	if m.IntervalSeconds < p.MinIntervalSeconds || m.IntervalSeconds > p.MaxIntervalSeconds {
		return nil, fmt.Errorf("interval_seconds %d out of range [%d, %d]", m.IntervalSeconds, p.MinIntervalSeconds, p.MaxIntervalSeconds)
	}
	if uint32(m.Payload.Size()) > p.MaxPayloadBytes {
		return nil, fmt.Errorf("payload %d bytes exceeds max %d", m.Payload.Size(), p.MaxPayloadBytes)
	}

	feeDenom := m.FeeDenom
	if feeDenom == "" {
		feeDenom = p.DefaultFeeDenom
	}
	if feeDenom == "" {
		return nil, fmt.Errorf("fee_denom must be set (no default_fee_denom configured)")
	}
	if err := types.ValidateDenom(feeDenom); err != nil {
		return nil, err
	}

	// Bound state growth (global + per-owner) before debiting any
	// funds — failure here must not strand the owner's stablecoin.
	count, err := s.k.CountJobs(ctx)
	if err != nil {
		return nil, err
	}
	if count >= p.MaxJobs {
		return nil, fmt.Errorf("job count cap reached: %d", p.MaxJobs)
	}
	ownerCount, err := s.k.CountJobsForOwner(ctx, m.Owner)
	if err != nil {
		return nil, err
	}
	if ownerCount >= p.MaxJobsPerOwner {
		return nil, fmt.Errorf("owner %s already holds %d jobs (cap %d)", m.Owner, ownerCount, p.MaxJobsPerOwner)
	}

	if err := types.RejectSelfScheduler(m.Payload); err != nil {
		return nil, err
	}
	if err := types.ValidatePayload(m.Payload, m.Owner, s.k.registry); err != nil {
		return nil, err
	}

	now := ctx.BlockTime().Unix()
	startTime := m.StartTime
	if startTime == 0 {
		startTime = now + m.IntervalSeconds
	}
	if m.EndTime != 0 && m.EndTime <= startTime {
		return nil, fmt.Errorf("end_time %d must be > start_time %d", m.EndTime, startTime)
	}
	if startTime-now > p.MaxIntervalSeconds {
		return nil, fmt.Errorf("start_time too far in the future (> max_interval_seconds)")
	}

	if err := s.k.fundPool(ctx, feeDenom, m.Owner, m.InitialBudget); err != nil {
		return nil, err
	}

	id, err := s.k.NextJobID(ctx)
	if err != nil {
		return nil, err
	}
	j := types.Job{
		Id:              id,
		Owner:           m.Owner,
		Payload:         m.Payload,
		StartTime:       startTime,
		EndTime:         m.EndTime,
		IntervalSeconds: m.IntervalSeconds,
		MaxExecutions:   m.MaxExecutions,
		NextRunTime:     startTime,
		FeeDenom:        feeDenom,
		FeePerRun:       m.FeePerRun,
		FeeBudget:       m.InitialBudget,
		Status:          types.Status_STATUS_ACTIVE,
		Memo:            m.Memo,
		CreatedAt:       now,
	}
	if err := s.k.SaveJob(ctx, j, 0, types.Status_STATUS_UNSPECIFIED); err != nil {
		return nil, err
	}
	if err := s.k.JobByOwner.Set(ctx, collections.Join(j.Owner, j.Id)); err != nil {
		return nil, err
	}

	s.k.recordAudit(ctx, j.Id, "create", j.Owner, j.Payload.TypeUrl)
	s.k.emit(ctx, "create",
		"id", u64s(j.Id),
		"owner", j.Owner,
		"type_url", j.Payload.TypeUrl,
		"interval", i64s(j.IntervalSeconds),
		"start_time", i64s(j.StartTime),
		"fee_denom", j.FeeDenom,
		"fee_per_run", u64s(j.FeePerRun),
		"initial_budget", u64s(j.FeeBudget),
	)
	return &types.MsgCreateJobResponse{JobId: j.Id}, nil
}

// ---- TopUpJob ------------------------------------------------------------

func (s msgServer) TopUpJob(goCtx context.Context, m *types.MsgTopUpJob) (*types.MsgTopUpJobResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	j, _, err := s.loadAndCheckOwned(ctx, m.JobId, m.Owner,
		types.Status_STATUS_ACTIVE, types.Status_STATUS_PAUSED, types.Status_STATUS_EXHAUSTED)
	if err != nil {
		return nil, err
	}

	if err := s.k.fundPool(ctx, j.FeeDenom, m.Owner, m.Amount); err != nil {
		return nil, err
	}

	prevNextRun := j.NextRunTime
	prevStatus := j.Status

	newBudget, err := types.SafeAdd(j.FeeBudget, m.Amount)
	if err != nil {
		return nil, err
	}
	j.FeeBudget = newBudget

	// Optional atomic resume from EXHAUSTED. Must satisfy the
	// SAME gates as MsgResumeJob — otherwise a top-up could
	// resurrect a row past its MaxExecutions cap or end_time
	// horizon, defeating the controlled-lifetime guarantee that
	// callers rely on at create time.
	now := ctx.BlockTime().Unix()
	if m.ResumeIfExhausted && j.Status == types.Status_STATUS_EXHAUSTED {
		switch {
		case j.FeeBudget < j.FeePerRun:
			// Silently skip — top-up insufficient. Caller sees
			// EXHAUSTED in the response and can retry with more.
		case j.EndTime != 0 && now >= j.EndTime:
			// end_time already passed — refuse loudly so the
			// caller doesn't believe their top-up resurrected
			// a job that physically cannot run.
			return nil, fmt.Errorf("job %d end_time %d already passed; cannot resume", j.Id, j.EndTime)
		case j.MaxExecutions != 0 && j.ExecutionsDone >= j.MaxExecutions:
			return nil, fmt.Errorf("job %d max_executions %d reached; cannot resume", j.Id, j.MaxExecutions)
		default:
			j.NextRunTime = types.AdvanceNextRun(j.StartTime, j.NextRunTime, j.IntervalSeconds, now)
			j.Status = types.Status_STATUS_ACTIVE
		}
	}

	if err := s.k.SaveJob(ctx, j, prevNextRun, prevStatus); err != nil {
		return nil, err
	}

	s.k.recordAudit(ctx, j.Id, "topup", j.Owner, fmt.Sprintf("amount=%d new_budget=%d", m.Amount, j.FeeBudget))
	s.k.emit(ctx, "topup",
		"id", u64s(j.Id),
		"amount", u64s(m.Amount),
		"new_budget", u64s(j.FeeBudget),
		"status", j.Status.String(),
	)
	return &types.MsgTopUpJobResponse{NewBudget: j.FeeBudget, Status: j.Status}, nil
}

// ---- WithdrawJob ---------------------------------------------------------

func (s msgServer) WithdrawJob(goCtx context.Context, m *types.MsgWithdrawJob) (*types.MsgWithdrawJobResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	j, _, err := s.loadAndCheckOwned(ctx, m.JobId, m.Owner,
		types.Status_STATUS_PAUSED, types.Status_STATUS_EXHAUSTED, types.Status_STATUS_CANCELLED)
	if err != nil {
		return nil, err
	}
	amount := m.Amount
	if amount == 0 || amount > j.FeeBudget {
		amount = j.FeeBudget
	}
	if amount == 0 {
		return nil, fmt.Errorf("nothing to withdraw")
	}
	prevNextRun := j.NextRunTime
	prevStatus := j.Status

	if err := s.k.drainPool(ctx, j.FeeDenom, j.Owner, amount); err != nil {
		return nil, err
	}
	newBudget, err := types.SafeSub(j.FeeBudget, amount)
	if err != nil {
		return nil, err
	}
	j.FeeBudget = newBudget
	if err := s.k.SaveJob(ctx, j, prevNextRun, prevStatus); err != nil {
		return nil, err
	}

	s.k.recordAudit(ctx, j.Id, "withdraw", j.Owner, fmt.Sprintf("amount=%d remaining=%d", amount, j.FeeBudget))
	s.k.emit(ctx, "withdraw",
		"id", u64s(j.Id),
		"amount", u64s(amount),
		"remaining", u64s(j.FeeBudget),
	)
	return &types.MsgWithdrawJobResponse{Withdrawn: amount, NewBudget: j.FeeBudget}, nil
}

// ---- PauseJob / ResumeJob -----------------------------------------------

func (s msgServer) PauseJob(goCtx context.Context, m *types.MsgPauseJob) (*types.MsgPauseJobResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	j, _, err := s.loadAndCheckOwned(ctx, m.JobId, m.Owner, types.Status_STATUS_ACTIVE)
	if err != nil {
		return nil, err
	}
	prevNextRun := j.NextRunTime
	prevStatus := j.Status
	j.Status = types.Status_STATUS_PAUSED
	if err := s.k.SaveJob(ctx, j, prevNextRun, prevStatus); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, j.Id, "pause", j.Owner, m.Reason)
	s.k.emit(ctx, "pause", "id", u64s(j.Id), "reason", m.Reason)
	return &types.MsgPauseJobResponse{}, nil
}

func (s msgServer) ResumeJob(goCtx context.Context, m *types.MsgResumeJob) (*types.MsgResumeJobResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	j, _, err := s.loadAndCheckOwned(ctx, m.JobId, m.Owner, types.Status_STATUS_PAUSED, types.Status_STATUS_EXHAUSTED)
	if err != nil {
		return nil, err
	}
	if j.FeeBudget < j.FeePerRun {
		return nil, fmt.Errorf("budget %d < fee_per_run %d; top up first", j.FeeBudget, j.FeePerRun)
	}
	now := ctx.BlockTime().Unix()
	if j.EndTime != 0 && now >= j.EndTime {
		return nil, fmt.Errorf("job %d end_time %d already passed", j.Id, j.EndTime)
	}
	if j.MaxExecutions != 0 && j.ExecutionsDone >= j.MaxExecutions {
		return nil, fmt.Errorf("job %d already exhausted max_executions %d", j.Id, j.MaxExecutions)
	}
	prevNextRun := j.NextRunTime
	prevStatus := j.Status
	j.NextRunTime = types.AdvanceNextRun(j.StartTime, j.NextRunTime, j.IntervalSeconds, now)
	j.Status = types.Status_STATUS_ACTIVE
	if err := s.k.SaveJob(ctx, j, prevNextRun, prevStatus); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, j.Id, "resume", j.Owner, "")
	s.k.emit(ctx, "resume", "id", u64s(j.Id), "next_run", i64s(j.NextRunTime))
	return &types.MsgResumeJobResponse{}, nil
}

// ---- CancelJob -----------------------------------------------------------

func (s msgServer) CancelJob(goCtx context.Context, m *types.MsgCancelJob) (*types.MsgCancelJobResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	j, _, err := s.loadAndCheckOwned(ctx, m.JobId, m.Owner,
		types.Status_STATUS_ACTIVE, types.Status_STATUS_PAUSED, types.Status_STATUS_EXHAUSTED)
	if err != nil {
		return nil, err
	}
	prevNextRun := j.NextRunTime
	prevStatus := j.Status
	j.Status = types.Status_STATUS_CANCELLED

	refunded := uint64(0)
	if m.Refund && j.FeeBudget > 0 {
		if err := s.k.drainPool(ctx, j.FeeDenom, j.Owner, j.FeeBudget); err != nil {
			return nil, err
		}
		refunded = j.FeeBudget
		j.FeeBudget = 0
	}
	if err := s.k.SaveJob(ctx, j, prevNextRun, prevStatus); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, j.Id, "cancel", j.Owner, m.Reason)
	s.k.emit(ctx, "cancel",
		"id", u64s(j.Id),
		"refunded", u64s(refunded),
		"reason", m.Reason,
	)
	return &types.MsgCancelJobResponse{Refunded: refunded}, nil
}

// ---- UpdateJob -----------------------------------------------------------

func (s msgServer) UpdateJob(goCtx context.Context, m *types.MsgUpdateJob) (*types.MsgUpdateJobResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	j, p, err := s.loadAndCheckOwned(ctx, m.JobId, m.Owner,
		types.Status_STATUS_ACTIVE, types.Status_STATUS_PAUSED, types.Status_STATUS_EXHAUSTED)
	if err != nil {
		return nil, err
	}
	prevNextRun := j.NextRunTime
	prevStatus := j.Status

	if m.Payload != nil && m.Payload.TypeUrl != "" {
		if uint32(m.Payload.Size()) > p.MaxPayloadBytes {
			return nil, fmt.Errorf("payload %d > max %d", m.Payload.Size(), p.MaxPayloadBytes)
		}
		if err := types.RejectSelfScheduler(m.Payload); err != nil {
			return nil, err
		}
		if err := types.ValidatePayload(m.Payload, j.Owner, s.k.registry); err != nil {
			return nil, err
		}
		j.Payload = m.Payload
	}
	if m.IntervalSeconds > 0 {
		if m.IntervalSeconds < p.MinIntervalSeconds || m.IntervalSeconds > p.MaxIntervalSeconds {
			return nil, fmt.Errorf("interval_seconds %d out of range [%d, %d]", m.IntervalSeconds, p.MinIntervalSeconds, p.MaxIntervalSeconds)
		}
		j.IntervalSeconds = m.IntervalSeconds
		// Recompute next_run_time so the schedule honors the new cadence.
		now := ctx.BlockTime().Unix()
		j.NextRunTime = types.AdvanceNextRun(j.StartTime, j.NextRunTime, j.IntervalSeconds, now)
	}
	if m.EndTime != 0 {
		j.EndTime = m.EndTime
	}
	if m.MaxExecutions != 0 {
		j.MaxExecutions = m.MaxExecutions
	}

	if err := s.k.SaveJob(ctx, j, prevNextRun, prevStatus); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, j.Id, "update", j.Owner, "")
	s.k.emit(ctx, "update", "id", u64s(j.Id))
	return &types.MsgUpdateJobResponse{}, nil
}

// ---- UpdateParams --------------------------------------------------------

func (s msgServer) UpdateParams(goCtx context.Context, m *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := s.k.SetParams(ctx, m.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
