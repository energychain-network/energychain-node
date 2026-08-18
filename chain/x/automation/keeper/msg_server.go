package keeper

import (
	"context"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/automation/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{k: k} }

var _ types.MsgServer = msgServer{}

func u(v uint64) string { return strconv.FormatUint(v, 10) }
func i(v int64) string  { return strconv.FormatInt(v, 10) }

// ---- schedules -------------------------------------------------------------

func (s msgServer) CreateSchedule(ctx context.Context, m *types.MsgCreateSchedule) (*types.MsgCreateScheduleResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if m.IntervalSeconds < params.MinIntervalSeconds {
		return nil, types.ErrInvalidField.Wrapf("interval < min %d", params.MinIntervalSeconds)
	}
	// Validate the target exists for the action's module.
	switch m.Action {
	case types.ActionKind_ACTION_KIND_MINCAST_INJECT, types.ActionKind_ACTION_KIND_MINCAST_CLOSE_MATURED:
		if s.k.mincast == nil {
			return nil, types.ErrActionFailed.Wrap("mincast keeper not wired")
		}
		if !s.k.mincast.HasMarket(ctx, m.TargetId) {
			return nil, types.ErrNotFound.Wrapf("mincast market %d", m.TargetId)
		}
	case types.ActionKind_ACTION_KIND_RWA_SNAPSHOT, types.ActionKind_ACTION_KIND_RWA_DIVIDEND:
		if s.k.rwa == nil {
			return nil, types.ErrActionFailed.Wrap("rwa keeper not wired")
		}
		if !s.k.rwa.HasToken(ctx, m.TargetId) {
			return nil, types.ErrNotFound.Wrapf("rwa token %d", m.TargetId)
		}
	default:
		return nil, types.ErrInvalidField.Wrap("invalid action")
	}

	n, err := s.k.CountSchedules(ctx)
	if err != nil {
		return nil, err
	}
	if n >= params.MaxSchedules {
		return nil, types.ErrLimitExceeded.Wrap("max_schedules")
	}

	now := sdkCtx.BlockTime().Unix()
	nextRun := m.StartTime
	if nextRun == 0 {
		nextRun = now + m.IntervalSeconds
	}
	if m.EndTime != 0 && nextRun > m.EndTime {
		return nil, types.ErrInvalidField.Wrap("next_run is past end_time")
	}

	id, err := s.k.NextScheduleID(ctx)
	if err != nil {
		return nil, err
	}
	sch := types.Schedule{
		Id:              id,
		Creator:         m.Creator,
		Action:          m.Action,
		TargetId:        m.TargetId,
		AmountPerRun:    m.AmountPerRun,
		IntervalSeconds: m.IntervalSeconds,
		NextRun:         nextRun,
		EndTime:         m.EndTime,
		MaxRuns:         m.MaxRuns,
		Status:          types.ScheduleStatus_SCHEDULE_STATUS_ACTIVE,
		CreatedAt:       now,
	}
	if err := s.k.saveSchedule(ctx, sch, 0, types.ScheduleStatus_SCHEDULE_STATUS_UNSPECIFIED); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeSchedule, types.AttrAction, "create", types.AttrScheduleID, u(id),
		types.AttrCreator, m.Creator, types.AttrNextRun, i(nextRun))
	return &types.MsgCreateScheduleResponse{ScheduleId: id}, nil
}

func (s msgServer) requireScheduleOwner(ctx context.Context, id uint64, caller string) (types.Schedule, error) {
	sch, ok, err := s.k.GetSchedule(ctx, id)
	if err != nil {
		return types.Schedule{}, err
	}
	if !ok {
		return types.Schedule{}, types.ErrNotFound.Wrapf("schedule %d", id)
	}
	if caller != sch.Creator && caller != s.k.authority {
		return types.Schedule{}, types.ErrUnauthorized.Wrap("not schedule creator")
	}
	return sch, nil
}

func (s msgServer) PauseSchedule(ctx context.Context, m *types.MsgPauseSchedule) (*types.MsgPauseScheduleResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sch, err := s.requireScheduleOwner(ctx, m.ScheduleId, m.Creator)
	if err != nil {
		return nil, err
	}
	if sch.Status != types.ScheduleStatus_SCHEDULE_STATUS_ACTIVE {
		return nil, types.ErrScheduleState.Wrap("only active schedules can be paused")
	}
	prevNext, prevStatus := sch.NextRun, sch.Status
	sch.Status = types.ScheduleStatus_SCHEDULE_STATUS_PAUSED
	if err := s.k.saveSchedule(ctx, sch, prevNext, prevStatus); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeSchedule, types.AttrAction, "pause", types.AttrScheduleID, u(sch.Id))
	return &types.MsgPauseScheduleResponse{}, nil
}

func (s msgServer) ResumeSchedule(ctx context.Context, m *types.MsgResumeSchedule) (*types.MsgResumeScheduleResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sch, err := s.requireScheduleOwner(ctx, m.ScheduleId, m.Creator)
	if err != nil {
		return nil, err
	}
	if sch.Status != types.ScheduleStatus_SCHEDULE_STATUS_PAUSED {
		return nil, types.ErrScheduleState.Wrap("only paused schedules can be resumed")
	}
	prevNext, prevStatus := sch.NextRun, sch.Status
	// Roll next_run forward past any slots missed while paused.
	now := sdkCtx.BlockTime().Unix()
	if sch.NextRun <= now {
		sch.NextRun = types.AdvanceNextRun(sch.NextRun, sch.IntervalSeconds, now)
	}
	if sch.EndTime != 0 && sch.NextRun > sch.EndTime {
		return nil, types.ErrScheduleState.Wrap("no runs remain before end_time")
	}
	sch.Status = types.ScheduleStatus_SCHEDULE_STATUS_ACTIVE
	if err := s.k.saveSchedule(ctx, sch, prevNext, prevStatus); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeSchedule, types.AttrAction, "resume", types.AttrScheduleID, u(sch.Id), types.AttrNextRun, i(sch.NextRun))
	return &types.MsgResumeScheduleResponse{}, nil
}

func (s msgServer) CancelSchedule(ctx context.Context, m *types.MsgCancelSchedule) (*types.MsgCancelScheduleResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sch, err := s.requireScheduleOwner(ctx, m.ScheduleId, m.Creator)
	if err != nil {
		return nil, err
	}
	if sch.Status == types.ScheduleStatus_SCHEDULE_STATUS_COMPLETED {
		return nil, types.ErrScheduleState.Wrap("already terminal")
	}
	prevNext, prevStatus := sch.NextRun, sch.Status
	sch.Status = types.ScheduleStatus_SCHEDULE_STATUS_COMPLETED
	if err := s.k.saveSchedule(ctx, sch, prevNext, prevStatus); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeSchedule, types.AttrAction, "cancel", types.AttrScheduleID, u(sch.Id))
	return &types.MsgCancelScheduleResponse{}, nil
}

// ---- streams ---------------------------------------------------------------

func (s msgServer) CreateStream(ctx context.Context, m *types.MsgCreateStream) (*types.MsgCreateStreamResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := s.k.requireSettlement(); err != nil {
		return nil, err
	}
	if !s.k.settlement.HasDenom(ctx, m.Denom) {
		return nil, types.ErrSettlement.Wrapf("unknown denom %q", m.Denom)
	}
	// Gate both legs: a sanctioned sender cannot escrow, a sanctioned receiver
	// cannot be a beneficiary.
	if s.k.isSanctioned(ctx, m.Sender) {
		return nil, types.ErrCompliance.Wrapf("%s sanctioned", m.Sender)
	}
	if s.k.isSanctioned(ctx, m.Receiver) {
		return nil, types.ErrCompliance.Wrapf("%s sanctioned", m.Receiver)
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	n, err := s.k.CountStreams(ctx)
	if err != nil {
		return nil, err
	}
	if n >= params.MaxStreams {
		return nil, types.ErrLimitExceeded.Wrap("max_streams")
	}

	start := m.StartTime
	if start == 0 {
		start = sdkCtx.BlockTime().Unix()
	}
	stop, err := types.StopTime(start, m.Deposit, m.RatePerSec)
	if err != nil {
		return nil, err
	}
	// Escrow the full deposit up-front so the receiver's vested balance is
	// always covered.
	if err := s.k.moveSettlement(ctx, m.Denom, m.Sender, StreamEscrowAccount(), m.Deposit); err != nil {
		return nil, err
	}
	id, err := s.k.NextStreamID(ctx)
	if err != nil {
		return nil, err
	}
	st := types.Stream{
		Id:         id,
		Sender:     m.Sender,
		Receiver:   m.Receiver,
		Denom:      m.Denom,
		Deposit:    m.Deposit,
		RatePerSec: m.RatePerSec,
		StartTime:  start,
		StopTime:   stop,
		Status:     types.StreamStatus_STREAM_STATUS_ACTIVE,
		CreatedAt:  sdkCtx.BlockTime().Unix(),
	}
	if err := s.k.saveStream(ctx, st, 0, types.StreamStatus_STREAM_STATUS_UNSPECIFIED); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeStream, types.AttrAction, "create", types.AttrStreamID, u(id),
		types.AttrSender, m.Sender, types.AttrReceiver, m.Receiver, types.AttrAmount, u(m.Deposit))
	return &types.MsgCreateStreamResponse{StreamId: id, StopTime: stop}, nil
}

func (s msgServer) WithdrawStream(ctx context.Context, m *types.MsgWithdrawStream) (*types.MsgWithdrawStreamResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	st, ok, err := s.k.GetStream(ctx, m.StreamId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("stream %d", m.StreamId)
	}
	if st.Status != types.StreamStatus_STREAM_STATUS_ACTIVE {
		return nil, types.ErrStreamState.Wrap("stream not active")
	}
	amount, err := s.k.withdrawStreamTo(ctx, &st, sdkCtx.BlockTime().Unix())
	if err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeStream, types.AttrAction, "withdraw", types.AttrStreamID, u(st.Id),
		types.AttrReceiver, st.Receiver, types.AttrAmount, u(amount))
	return &types.MsgWithdrawStreamResponse{Withdrawn: amount}, nil
}

func (s msgServer) CancelStream(ctx context.Context, m *types.MsgCancelStream) (*types.MsgCancelStreamResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	st, ok, err := s.k.GetStream(ctx, m.StreamId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("stream %d", m.StreamId)
	}
	if m.Sender != st.Sender && m.Sender != s.k.authority {
		return nil, types.ErrUnauthorized.Wrap("only the stream sender may cancel")
	}
	if st.Status != types.StreamStatus_STREAM_STATUS_ACTIVE {
		return nil, types.ErrStreamState.Wrap("stream not active")
	}
	// A sanctioned sender cannot pull the unvested refund (mirrors the
	// receiver gate below); their funds stay escrowed and the receiver can
	// still withdraw their own vested balance via WithdrawStream.
	if s.k.isSanctioned(ctx, st.Sender) {
		return nil, types.ErrCompliance.Wrapf("sender %s sanctioned", st.Sender)
	}
	now := sdkCtx.BlockTime().Unix()
	vested := types.VestedAmount(st.Deposit, st.RatePerSec, st.StartTime, st.StopTime, now)
	recvPay, err := types.SafeSub(vested, st.Withdrawn)
	if err != nil {
		return nil, err
	}
	ownerRefund, err := types.SafeSub(st.Deposit, vested)
	if err != nil {
		return nil, err
	}
	// The receiver keeps everything vested; pay it out unless they are
	// sanctioned (in which case the funds stay escrowed and cancel is refused
	// rather than paying a frozen party).
	if recvPay > 0 {
		if s.k.isSanctioned(ctx, st.Receiver) {
			return nil, types.ErrCompliance.Wrapf("receiver %s sanctioned", st.Receiver)
		}
		if err := s.k.moveSettlement(ctx, st.Denom, StreamEscrowAccount(), st.Receiver, recvPay); err != nil {
			return nil, err
		}
	}
	if ownerRefund > 0 {
		if err := s.k.moveSettlement(ctx, st.Denom, StreamEscrowAccount(), st.Sender, ownerRefund); err != nil {
			return nil, err
		}
	}
	prevStop, prevStatus := st.StopTime, st.Status
	st.Withdrawn = vested
	st.Status = types.StreamStatus_STREAM_STATUS_CANCELLED
	if err := s.k.saveStream(ctx, st, prevStop, prevStatus); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeStream, types.AttrAction, "cancel", types.AttrStreamID, u(st.Id),
		types.AttrReceiver, u(recvPay), types.AttrSender, u(ownerRefund))
	return &types.MsgCancelStreamResponse{ReceiverPaid: recvPay, SenderRefunded: ownerRefund}, nil
}

func (s msgServer) UpdateParams(ctx context.Context, m *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if m.Authority != s.k.authority {
		return nil, types.ErrUnauthorized.Wrapf("expected authority %q", s.k.authority)
	}
	if err := s.k.SetParams(ctx, m.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
