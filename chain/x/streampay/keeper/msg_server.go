package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/streampay/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{k: k} }

func (s msgServer) requireAuthority(authority string) error {
	if authority != s.k.authority {
		return fmt.Errorf("invalid authority: expected %s, got %s", s.k.authority, authority)
	}
	return nil
}

// ---- CreateStream --------------------------------------------------------

func (s msgServer) CreateStream(goCtx context.Context, m *types.MsgCreateStream) (*types.MsgCreateStreamResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}

	denom := m.Denom
	if denom == "" {
		denom = p.DefaultDenom
	}
	if denom == "" {
		return nil, fmt.Errorf("denom must be set (no default_denom configured)")
	}
	if err := types.ValidateDenom(denom); err != nil {
		return nil, err
	}
	if err := types.ValidateMemo(m.Memo, p.MemoMaxLen); err != nil {
		return nil, err
	}

	if m.RatePerSecond < p.MinRatePerSecond || m.RatePerSecond > p.MaxRatePerSecond {
		return nil, fmt.Errorf("rate_per_second %d out of [%d, %d]", m.RatePerSecond, p.MinRatePerSecond, p.MaxRatePerSecond)
	}
	if m.Deposit > p.MaxDeposit {
		return nil, fmt.Errorf("deposit %d > max %d", m.Deposit, p.MaxDeposit)
	}

	now := ctx.BlockTime().Unix()
	startTime := m.StartTime
	if startTime == 0 {
		startTime = now
	}
	if startTime+p.MaxHorizonSeconds < startTime {
		return nil, fmt.Errorf("start_time overflow")
	}
	if m.EndTime != 0 {
		if m.EndTime <= startTime {
			return nil, fmt.Errorf("end_time %d <= start_time %d", m.EndTime, startTime)
		}
		if m.EndTime-startTime > p.MaxHorizonSeconds {
			return nil, fmt.Errorf("horizon %d > max %d", m.EndTime-startTime, p.MaxHorizonSeconds)
		}
	}

	// Bound state growth (global + per-sender) before debiting.
	count, err := s.k.CountStreams(ctx)
	if err != nil {
		return nil, err
	}
	if count >= p.MaxStreams {
		return nil, fmt.Errorf("stream count cap reached: %d", p.MaxStreams)
	}
	perSender, err := s.k.CountStreamsForSender(ctx, m.Sender)
	if err != nil {
		return nil, err
	}
	if perSender >= p.MaxStreamsPerSender {
		return nil, fmt.Errorf("sender %s already holds %d streams (cap %d)", m.Sender, perSender, p.MaxStreamsPerSender)
	}

	// Sanctions on both parties: a sanctioned sender / receiver
	// cannot open a fresh stream. The freeze gate inside
	// fundPool protects the denom-level path; this gate protects
	// the chain-level OFAC / national-list path.
	if err := s.k.requireUnsanctioned(ctx, m.Sender, "sender"); err != nil {
		return nil, err
	}
	if err := s.k.requireUnsanctioned(ctx, m.Receiver, "receiver"); err != nil {
		return nil, err
	}

	if err := s.k.fundPool(ctx, denom, m.Sender, m.Deposit); err != nil {
		return nil, err
	}

	id, err := s.k.NextStreamID(ctx)
	if err != nil {
		return nil, err
	}
	stream := types.Stream{
		Id:              id,
		Sender:          m.Sender,
		Receiver:        m.Receiver,
		Denom:           denom,
		RatePerSecond:   m.RatePerSecond,
		Deposit:         m.Deposit,
		StartTime:       startTime,
		EndTime:         m.EndTime,
		Status:          types.Status_STATUS_ACTIVE,
		CreatedAt:       now,
		Memo:            m.Memo,
	}
	if err := s.k.SetStream(ctx, stream); err != nil {
		return nil, err
	}
	if err := s.k.StreamBySender.Set(ctx, collections.Join(m.Sender, id)); err != nil {
		return nil, err
	}
	if err := s.k.StreamByReceiver.Set(ctx, collections.Join(m.Receiver, id)); err != nil {
		return nil, err
	}

	s.k.recordAudit(ctx, id, "create", m.Sender, m.Receiver, fmt.Sprintf("denom=%s rate=%d deposit=%d", denom, m.RatePerSecond, m.Deposit))
	s.k.emit(ctx, "create",
		"id", u64s(id),
		"sender", m.Sender,
		"receiver", m.Receiver,
		"denom", denom,
		"rate", u64s(m.RatePerSecond),
		"deposit", u64s(m.Deposit),
		"start_time", i64s(startTime),
		"end_time", i64s(m.EndTime),
	)
	return &types.MsgCreateStreamResponse{StreamId: id}, nil
}

// ---- DepositToStream -----------------------------------------------------

func (s msgServer) DepositToStream(goCtx context.Context, m *types.MsgDepositToStream) (*types.MsgDepositToStreamResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	st, err := s.k.MustGetStream(ctx, m.StreamId)
	if err != nil {
		return nil, err
	}
	if st.Sender != m.Sender {
		return nil, fmt.Errorf("only sender %s may deposit to stream %d", st.Sender, st.Id)
	}
	if st.Status.IsTerminal() {
		return nil, fmt.Errorf("stream %d is terminal (%s)", st.Id, st.Status)
	}
	if err := s.k.requireUnsanctioned(ctx, m.Sender, "sender"); err != nil {
		return nil, err
	}
	newDeposit, err := types.SafeAdd(st.Deposit, m.Amount)
	if err != nil {
		return nil, err
	}
	if newDeposit > p.MaxDeposit {
		return nil, fmt.Errorf("new deposit %d > max %d", newDeposit, p.MaxDeposit)
	}
	if err := s.k.fundPool(ctx, st.Denom, m.Sender, m.Amount); err != nil {
		return nil, err
	}
	st.Deposit = newDeposit
	if err := s.k.SetStream(ctx, st); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, st.Id, "deposit", m.Sender, "", fmt.Sprintf("amount=%d new_deposit=%d", m.Amount, newDeposit))
	s.k.emit(ctx, "deposit",
		"id", u64s(st.Id),
		"amount", u64s(m.Amount),
		"new_deposit", u64s(newDeposit),
	)
	return &types.MsgDepositToStreamResponse{NewDeposit: newDeposit}, nil
}

// ---- Withdraw ------------------------------------------------------------

func (s msgServer) Withdraw(goCtx context.Context, m *types.MsgWithdraw) (*types.MsgWithdrawResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	st, err := s.k.MustGetStream(ctx, m.StreamId)
	if err != nil {
		return nil, err
	}
	if st.Receiver != m.Receiver {
		return nil, fmt.Errorf("only receiver %s may withdraw from stream %d", st.Receiver, st.Id)
	}
	if st.Status == types.Status_STATUS_CANCELLED {
		return nil, fmt.Errorf("stream %d cancelled; nothing to withdraw post-settlement", st.Id)
	}
	if err := s.k.requireUnsanctioned(ctx, m.Receiver, "receiver"); err != nil {
		return nil, err
	}
	now := ctx.BlockTime().Unix()
	avail := types.Withdrawable(st, now)
	if avail == 0 {
		return nil, fmt.Errorf("nothing accrued yet for stream %d", st.Id)
	}
	amount := m.Amount
	if amount == 0 || amount > avail {
		amount = avail
	}
	if err := s.k.drainPool(ctx, st.Denom, st.Receiver, amount); err != nil {
		return nil, err
	}
	newWithdrawn, err := types.SafeAdd(st.Withdrawn, amount)
	if err != nil {
		return nil, err
	}
	st.Withdrawn = newWithdrawn

	// Lazy completion: once end_time has passed AND withdrawn ==
	// deposit, the stream is fully wound down. Flip to COMPLETED
	// for clarity (and to refuse further mutations cheaply).
	if st.EndTime != 0 && now >= st.EndTime && st.Withdrawn >= st.Deposit {
		st.Status = types.Status_STATUS_COMPLETED
		st.SettledAt = now
	}

	if err := s.k.SetStream(ctx, st); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "withdraw",
		"id", u64s(st.Id),
		"amount", u64s(amount),
		"total_withdrawn", u64s(st.Withdrawn),
	)
	return &types.MsgWithdrawResponse{Withdrawn: amount, TotalWithdrawn: st.Withdrawn}, nil
}

// ---- Pause / Resume ------------------------------------------------------

func (s msgServer) Pause(goCtx context.Context, m *types.MsgPause) (*types.MsgPauseResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	st, err := s.k.MustGetStream(ctx, m.StreamId)
	if err != nil {
		return nil, err
	}
	if st.Sender != m.Sender {
		return nil, fmt.Errorf("only sender %s may pause stream %d", st.Sender, st.Id)
	}
	if st.Status != types.Status_STATUS_ACTIVE {
		return nil, fmt.Errorf("stream %d not ACTIVE (status=%s)", st.Id, st.Status)
	}
	now := ctx.BlockTime().Unix()
	accruedAtPause := types.StreamedAmount(st, now)
	st.Status = types.Status_STATUS_PAUSED
	st.PausedAt = now
	if err := s.k.SetStream(ctx, st); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, st.Id, "pause", m.Sender, "", m.Reason)
	s.k.emit(ctx, "pause",
		"id", u64s(st.Id),
		"paused_at", i64s(now),
		"accrued", u64s(accruedAtPause),
		"reason", m.Reason,
	)
	return &types.MsgPauseResponse{AccruedAtPause: accruedAtPause}, nil
}

func (s msgServer) Resume(goCtx context.Context, m *types.MsgResume) (*types.MsgResumeResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	st, err := s.k.MustGetStream(ctx, m.StreamId)
	if err != nil {
		return nil, err
	}
	if st.Sender != m.Sender {
		return nil, fmt.Errorf("only sender %s may resume stream %d", st.Sender, st.Id)
	}
	if st.Status != types.Status_STATUS_PAUSED {
		return nil, fmt.Errorf("stream %d not PAUSED (status=%s)", st.Id, st.Status)
	}
	now := ctx.BlockTime().Unix()
	// Accumulate the closed pause interval into the totalised
	// pause counter. We clamp pause-resume duration at end_time
	// so a pause that overlaps past end_time only counts the
	// pre-end_time portion (the post-end_time portion is not
	// accrued anyway).
	cur := now
	if st.EndTime != 0 && cur > st.EndTime {
		cur = st.EndTime
	}
	pausedFor := int64(0)
	if cur > st.PausedAt {
		pausedFor = cur - st.PausedAt
	}
	st.PausedAccumulatedSeconds += pausedFor
	st.PausedAt = 0
	st.Status = types.Status_STATUS_ACTIVE
	if err := s.k.SetStream(ctx, st); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, st.Id, "resume", m.Sender, "", fmt.Sprintf("paused_for=%d", pausedFor))
	s.k.emit(ctx, "resume", "id", u64s(st.Id), "paused_for", i64s(pausedFor))
	return &types.MsgResumeResponse{PausedDurationAdded: pausedFor}, nil
}

// ---- Cancel --------------------------------------------------------------

func (s msgServer) Cancel(goCtx context.Context, m *types.MsgCancel) (*types.MsgCancelResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	st, err := s.k.MustGetStream(ctx, m.StreamId)
	if err != nil {
		return nil, err
	}
	if m.Actor != st.Sender && m.Actor != st.Receiver {
		return nil, fmt.Errorf("actor %s not party to stream %d", m.Actor, st.Id)
	}
	if st.Status.IsTerminal() {
		return nil, fmt.Errorf("stream %d already terminal (%s)", st.Id, st.Status)
	}
	now := ctx.BlockTime().Unix()
	streamed := types.StreamedAmount(st, now)
	receiverPayout := uint64(0)
	if streamed > st.Withdrawn {
		receiverPayout = streamed - st.Withdrawn
	}
	senderRefund := uint64(0)
	if st.Deposit > streamed {
		senderRefund = st.Deposit - streamed
	}

	// Sanctions gate on the payout legs.
	//
	// Without this gate, a sanctioned counterparty could still
	// be paid out via the other party's cancel: the receiver's
	// share would drain to the receiver via drainPool, and the
	// sender's refund would drain to a now-sanctioned sender.
	// drainPool checks the per-denom freeze but NOT the chain-
	// level sanctions list — so we re-check it here before
	// touching the pool. If either side would receive into a
	// sanctioned account we refuse the cancel entirely; funds
	// stay pinned in the pool until governance lands a forced-
	// settlement path (M4 dispute / dataslash track).
	if receiverPayout > 0 {
		if err := s.k.requireUnsanctioned(ctx, st.Receiver, "receiver"); err != nil {
			return nil, fmt.Errorf("cancel refused: %w", err)
		}
		if err := s.k.drainPool(ctx, st.Denom, st.Receiver, receiverPayout); err != nil {
			return nil, fmt.Errorf("receiver payout refused: %w", err)
		}
		st.Withdrawn += receiverPayout
	}
	if senderRefund > 0 {
		if err := s.k.requireUnsanctioned(ctx, st.Sender, "sender"); err != nil {
			return nil, fmt.Errorf("cancel refused: %w", err)
		}
		if err := s.k.drainPool(ctx, st.Denom, st.Sender, senderRefund); err != nil {
			return nil, fmt.Errorf("sender refund refused: %w", err)
		}
		// Conceptually the deposit "shrinks" by the refunded
		// amount but we keep the field as a historical
		// statement of intent (audit). Status flip below is the
		// authoritative terminal marker.
	}

	st.Status = types.Status_STATUS_CANCELLED
	st.SettledAt = now
	st.PausedAt = 0
	if err := s.k.SetStream(ctx, st); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, st.Id, "cancel", m.Actor, "", fmt.Sprintf("receiver_payout=%d sender_refund=%d reason=%s", receiverPayout, senderRefund, m.Reason))
	s.k.emit(ctx, "cancel",
		"id", u64s(st.Id),
		"actor", m.Actor,
		"receiver_payout", u64s(receiverPayout),
		"sender_refund", u64s(senderRefund),
		"reason", m.Reason,
	)
	return &types.MsgCancelResponse{ReceiverPayout: receiverPayout, SenderRefund: senderRefund}, nil
}

// ---- Transfer ------------------------------------------------------------

func (s msgServer) Transfer(goCtx context.Context, m *types.MsgTransfer) (*types.MsgTransferResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	st, err := s.k.MustGetStream(ctx, m.StreamId)
	if err != nil {
		return nil, err
	}
	if st.Receiver != m.Receiver {
		return nil, fmt.Errorf("only current receiver may transfer stream %d", st.Id)
	}
	if st.Status.IsTerminal() {
		return nil, fmt.Errorf("stream %d already terminal (%s)", st.Id, st.Status)
	}
	if m.NewReceiver == st.Sender {
		return nil, fmt.Errorf("new_receiver cannot equal sender")
	}
	if err := s.k.requireUnsanctioned(ctx, m.NewReceiver, "new_receiver"); err != nil {
		return nil, err
	}
	// Also re-check the per-denom freeze on the new receiver —
	// otherwise a sanctioned/frozen account could be set up as
	// the future drain target while the current Withdraw
	// pipeline blocks at runtime.
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if s.k.stablecoin.IsAccountBlocked(sdkCtx, st.Denom, m.NewReceiver) {
		return nil, fmt.Errorf("new_receiver %s is frozen / blacklisted on denom %s", m.NewReceiver, st.Denom)
	}
	if err := s.k.StreamByReceiver.Remove(ctx, collections.Join(st.Receiver, st.Id)); err != nil {
		return nil, err
	}
	if err := s.k.StreamByReceiver.Set(ctx, collections.Join(m.NewReceiver, st.Id)); err != nil {
		return nil, err
	}
	old := st.Receiver
	st.Receiver = m.NewReceiver
	if err := s.k.SetStream(ctx, st); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, st.Id, "transfer", m.Receiver, m.NewReceiver, fmt.Sprintf("from=%s to=%s", old, m.NewReceiver))
	s.k.emit(ctx, "transfer",
		"id", u64s(st.Id),
		"from", old,
		"to", m.NewReceiver,
	)
	return &types.MsgTransferResponse{}, nil
}

// ---- ChangeRate ----------------------------------------------------------

func (s msgServer) ChangeRate(goCtx context.Context, m *types.MsgChangeRate) (*types.MsgChangeRateResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	st, err := s.k.MustGetStream(ctx, m.StreamId)
	if err != nil {
		return nil, err
	}
	if st.Sender != m.Sender {
		return nil, fmt.Errorf("only sender %s may change rate of stream %d", st.Sender, st.Id)
	}
	if st.Status != types.Status_STATUS_ACTIVE {
		// We deliberately refuse while PAUSED to keep accounting
		// clean — settling a paused stream's accrual would
		// silently overshoot the previously-frozen amount. The
		// sender's expected flow is Resume first, ChangeRate next.
		return nil, fmt.Errorf("stream %d not ACTIVE (status=%s); resume first", st.Id, st.Status)
	}
	if m.NewRatePerSecond < p.MinRatePerSecond || m.NewRatePerSecond > p.MaxRatePerSecond {
		return nil, fmt.Errorf("new_rate_per_second %d out of [%d, %d]", m.NewRatePerSecond, p.MinRatePerSecond, p.MaxRatePerSecond)
	}
	now := ctx.BlockTime().Unix()
	// ChangeRate semantics: settle then restart.
	//
	// 1. Compute total streamed at the OLD rate.
	// 2. Pay receiver the unclaimed portion immediately —
	//    avoids losing wei to rounding when re-anchoring the
	//    clock under a new rate, and keeps the receiver whole
	//    at the exact moment the rate flips.
	// 3. Reset start_time to now, rate to new_rate, deposit to
	//    the unstreamed runway, withdrawn to 0. From here, the
	//    new rate alone governs future accrual.
	// 4. If the stream is fully streamed (no runway left), the
	//    rate change is meaningless — refuse so the sender
	//    doesn't believe a future flow exists.
	streamed := types.StreamedAmount(st, now)
	unclaimed := streamed - st.Withdrawn
	runway := st.Deposit - streamed
	if runway == 0 {
		return nil, fmt.Errorf("stream %d has no remaining runway; cancel + recreate instead", st.Id)
	}
	if unclaimed > 0 {
		if err := s.k.drainPool(ctx, st.Denom, st.Receiver, unclaimed); err != nil {
			return nil, fmt.Errorf("settling unclaimed before rate change: %w", err)
		}
	}
	st.StartTime = now
	st.PausedAccumulatedSeconds = 0
	st.PausedAt = 0
	st.Deposit = runway
	st.Withdrawn = 0
	st.RatePerSecond = m.NewRatePerSecond
	if err := s.k.SetStream(ctx, st); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, st.Id, "change_rate", m.Sender, "", fmt.Sprintf("new_rate=%d settled=%d remaining=%d", m.NewRatePerSecond, unclaimed, runway))
	s.k.emit(ctx, "change_rate",
		"id", u64s(st.Id),
		"new_rate", u64s(m.NewRatePerSecond),
		"settled", u64s(unclaimed),
		"remaining", u64s(runway),
	)
	return &types.MsgChangeRateResponse{}, nil
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
