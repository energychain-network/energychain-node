package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/market/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{k: k} }

func (s msgServer) requireAuthority(authority string) error {
	if authority != s.k.authority {
		return fmt.Errorf("invalid authority: expected %s, got %s", s.k.authority, authority)
	}
	return nil
}

// ---- CreatePair -------------------------------------------------------

func (s msgServer) CreatePair(goCtx context.Context, m *types.MsgCreatePair) (*types.MsgCreatePairResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateMemo(m.Memo, p.MemoMaxLen); err != nil {
		return nil, err
	}
	cnt, err := s.k.CountPairs(ctx)
	if err != nil {
		return nil, err
	}
	if cnt >= p.MaxPairs {
		return nil, fmt.Errorf("pair count cap reached: %d", p.MaxPairs)
	}
	id, err := s.k.NextPairID(ctx)
	if err != nil {
		return nil, err
	}
	now := ctx.BlockTime().Unix()
	pair := types.Pair{
		Id:                   id,
		BaseDenom:            m.BaseDenom,
		QuoteDenom:           m.QuoteDenom,
		Mode:                 m.Mode,
		Status:               types.PairStatus_PAIR_STATUS_ACTIVE,
		BatchIntervalSeconds: m.BatchIntervalSeconds,
		PriceBandLo:          m.PriceBandLo,
		PriceBandHi:          m.PriceBandHi,
		MaxPositionPerUser:   m.MaxPositionPerUser,
		CreatedAt:            now,
		Memo:                 m.Memo,
	}
	if m.Mode == types.MatchMode_MATCH_MODE_FBA {
		pair.NextBatchOpenTime = now
		pair.BatchCloseTime = now + m.BatchIntervalSeconds
	}
	if err := s.k.SetPair(ctx, pair); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, pair.Id, 0, "create_pair", m.Authority, "",
		fmt.Sprintf("base=%s quote=%s mode=%s", pair.BaseDenom, pair.QuoteDenom, pair.Mode))
	s.k.emit(ctx, "create_pair",
		"id", u64s(pair.Id),
		"base", pair.BaseDenom, "quote", pair.QuoteDenom,
		"mode", pair.Mode.String())
	return &types.MsgCreatePairResponse{PairId: pair.Id}, nil
}

// ---- PauseUnpausePair -------------------------------------------------

func (s msgServer) PauseUnpausePair(goCtx context.Context, m *types.MsgPauseUnpausePair) (*types.MsgPauseUnpausePairResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	pair, err := s.k.MustGetPair(ctx, m.PairId)
	if err != nil {
		return nil, err
	}
	if m.Pause {
		pair.Status = types.PairStatus_PAIR_STATUS_PAUSED
	} else {
		pair.Status = types.PairStatus_PAIR_STATUS_ACTIVE
	}
	if err := s.k.SetPair(ctx, pair); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, pair.Id, 0, "pause_unpause", m.Authority, "",
		fmt.Sprintf("pause=%v reason=%s", m.Pause, m.Reason))
	s.k.emit(ctx, "pause_unpause",
		"id", u64s(pair.Id), "status", pair.Status.String(), "reason", m.Reason)
	return &types.MsgPauseUnpausePairResponse{NewStatus: pair.Status}, nil
}

// ---- UpdatePairRisk ---------------------------------------------------

func (s msgServer) UpdatePairRisk(goCtx context.Context, m *types.MsgUpdatePairRisk) (*types.MsgUpdatePairRiskResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	pair, err := s.k.MustGetPair(ctx, m.PairId)
	if err != nil {
		return nil, err
	}
	pair.PriceBandLo = m.PriceBandLo
	pair.PriceBandHi = m.PriceBandHi
	pair.MaxPositionPerUser = m.MaxPositionPerUser
	if err := s.k.SetPair(ctx, pair); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, pair.Id, 0, "update_risk", m.Authority, "",
		fmt.Sprintf("lo=%d hi=%d cap=%d", m.PriceBandLo, m.PriceBandHi, m.MaxPositionPerUser))
	s.k.emit(ctx, "update_risk", "id", u64s(pair.Id))
	return &types.MsgUpdatePairRiskResponse{}, nil
}

// ---- PlaceLimitOrder --------------------------------------------------

func (s msgServer) PlaceLimitOrder(goCtx context.Context, m *types.MsgPlaceLimitOrder) (*types.MsgPlaceLimitOrderResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	pair, err := s.k.MustGetPair(ctx, m.PairId)
	if err != nil {
		return nil, err
	}
	if pair.Status != types.PairStatus_PAIR_STATUS_ACTIVE {
		return nil, fmt.Errorf("pair %d not ACTIVE (status=%s)", pair.Id, pair.Status)
	}
	if err := s.k.requireUnsanctioned(ctx, m.Owner, "owner"); err != nil {
		return nil, err
	}
	if pair.PriceBandLo > 0 && m.Price < pair.PriceBandLo {
		return nil, fmt.Errorf("price %d < band_lo %d", m.Price, pair.PriceBandLo)
	}
	if pair.PriceBandHi > 0 && m.Price > pair.PriceBandHi {
		return nil, fmt.Errorf("price %d > band_hi %d", m.Price, pair.PriceBandHi)
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateMemo(m.Memo, p.MemoMaxLen); err != nil {
		return nil, err
	}
	// State-growth bounds before escrow moves.
	pairOpen, err := s.k.countOpenOrders(ctx, pair.Id)
	if err != nil {
		return nil, err
	}
	if pairOpen >= p.MaxOpenOrdersPerPair {
		return nil, fmt.Errorf("pair %d open-orders cap %d reached", pair.Id, p.MaxOpenOrdersPerPair)
	}
	userOpen, err := s.k.countOpenOrdersForUser(ctx, pair.Id, m.Owner)
	if err != nil {
		return nil, err
	}
	if userOpen >= p.MaxOpenOrdersPerUserPerPair {
		return nil, fmt.Errorf("user %s open-orders cap %d on pair %d reached", m.Owner, p.MaxOpenOrdersPerUserPerPair, pair.Id)
	}

	// Escrow.
	var escrowDenom string
	var escrowAmount uint64
	switch m.Side {
	case types.Side_SIDE_BUY:
		amt, err := types.QuoteForFill(m.Price, m.Quantity)
		if err != nil {
			return nil, err
		}
		// Notional must fit in uint64 — QuoteForFill already
		// checks overflow via SafeMul.
		if amt == 0 {
			return nil, fmt.Errorf("buy notional is zero at price=%d qty=%d (PriceScale=%d)", m.Price, m.Quantity, types.PriceScale)
		}
		escrowDenom, escrowAmount = pair.QuoteDenom, amt
	case types.Side_SIDE_SELL:
		escrowDenom, escrowAmount = pair.BaseDenom, m.Quantity
	}
	if err := s.k.fundPool(ctx, escrowDenom, m.Owner, escrowAmount); err != nil {
		return nil, err
	}
	id, err := s.k.NextOrderID(ctx)
	if err != nil {
		return nil, err
	}
	now := ctx.BlockTime().Unix()
	order := types.Order{
		Id:           id,
		PairId:       pair.Id,
		Owner:        m.Owner,
		Side:         m.Side,
		Price:        m.Price,
		Quantity:     m.Quantity,
		RemainingQty: m.Quantity,
		Status:       types.OrderStatus_ORDER_STATUS_OPEN,
		PlacedAt:     now,
		Memo:         m.Memo,
		// EscrowLocked is the canonical record of what the pool
		// holds on this order's behalf. Decremented exactly per
		// fill, refunded exactly at cancel; eliminates the
		// floor-divide leakage between cancel-time refund and
		// per-fill drain that would otherwise accumulate.
		EscrowLocked: escrowAmount,
	}
	if err := s.k.SetOrder(ctx, order); err != nil {
		return nil, err
	}
	if err := s.k.OrderByOwner.Set(ctx, collections.Join(order.Owner, order.Id)); err != nil {
		return nil, err
	}

	// Try to match (continuous only). FBA orders rest until ClearBatch.
	var filled uint64
	if pair.Mode == types.MatchMode_MATCH_MODE_CONTINUOUS {
		f, err := s.k.matchContinuous(ctx, pair, &order)
		if err != nil {
			return nil, err
		}
		filled = f
	}
	// Rest residual on book if not fully filled.
	if order.RemainingQty > 0 {
		if err := s.k.addToBook(ctx, order); err != nil {
			return nil, err
		}
	}
	// Persist final order state.
	if err := s.k.SetOrder(ctx, order); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, pair.Id, order.Id, "place_order", m.Owner, "",
		fmt.Sprintf("side=%s price=%d qty=%d filled=%d", m.Side, m.Price, m.Quantity, filled))
	s.k.emit(ctx, "place_order",
		"pair_id", u64s(pair.Id),
		"order_id", u64s(order.Id),
		"side", m.Side.String(),
		"price", u64s(m.Price),
		"quantity", u64s(m.Quantity),
		"filled", u64s(filled),
		"remaining", u64s(order.RemainingQty),
	)
	return &types.MsgPlaceLimitOrderResponse{
		OrderId:      order.Id,
		FilledQty:    filled,
		RemainingQty: order.RemainingQty,
	}, nil
}

// ---- CancelOrder ------------------------------------------------------

func (s msgServer) CancelOrder(goCtx context.Context, m *types.MsgCancelOrder) (*types.MsgCancelOrderResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	o, err := s.k.MustGetOrder(ctx, m.OrderId)
	if err != nil {
		return nil, err
	}
	if o.Owner != m.Owner {
		return nil, fmt.Errorf("only owner can cancel")
	}
	if o.Status.IsTerminal() {
		return nil, fmt.Errorf("order %d already terminal (%s)", o.Id, o.Status)
	}
	pair, err := s.k.MustGetPair(ctx, o.PairId)
	if err != nil {
		return nil, err
	}
	// Refund the EXACT escrow still held on this order's
	// behalf. The escrow_locked field is decremented per fill
	// (in applyFill), so reading it here gives the residue
	// without rounding drift.
	var refundDenom string
	switch o.Side {
	case types.Side_SIDE_BUY:
		refundDenom = pair.QuoteDenom
	case types.Side_SIDE_SELL:
		refundDenom = pair.BaseDenom
	}
	refundAmount := o.EscrowLocked
	if err := s.k.requireUnsanctioned(ctx, m.Owner, "owner"); err != nil {
		return nil, err
	}
	if err := s.k.drainPool(ctx, refundDenom, m.Owner, refundAmount); err != nil {
		return nil, err
	}
	if err := s.k.removeFromBook(ctx, o); err != nil {
		return nil, err
	}
	o.EscrowLocked = 0
	o.Status = types.OrderStatus_ORDER_STATUS_CANCELLED
	if err := s.k.SetOrder(ctx, o); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, pair.Id, o.Id, "cancel_order", m.Owner, "", m.Reason)
	s.k.emit(ctx, "cancel_order",
		"order_id", u64s(o.Id),
		"refunded", u64s(refundAmount),
		"reason", m.Reason,
	)
	return &types.MsgCancelOrderResponse{RefundedAmount: refundAmount}, nil
}

// ---- ClearBatch -------------------------------------------------------

func (s msgServer) ClearBatch(goCtx context.Context, m *types.MsgClearBatch) (*types.MsgClearBatchResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	pair, err := s.k.MustGetPair(ctx, m.PairId)
	if err != nil {
		return nil, err
	}
	if pair.Mode != types.MatchMode_MATCH_MODE_FBA {
		return nil, fmt.Errorf("pair %d is not FBA mode", pair.Id)
	}
	if pair.Status != types.PairStatus_PAIR_STATUS_ACTIVE {
		return nil, fmt.Errorf("pair %d not ACTIVE", pair.Id)
	}
	now := ctx.BlockTime().Unix()
	if now < pair.BatchCloseTime {
		return nil, fmt.Errorf("batch not yet closed (until=%d now=%d)", pair.BatchCloseTime, now)
	}
	matched, clearing, done, err := s.k.matchFBA(ctx, pair)
	if err != nil {
		return nil, err
	}
	if done {
		// Open the next batch window.
		pair.NextBatchOpenTime = now
		pair.BatchCloseTime = now + pair.BatchIntervalSeconds
		if err := s.k.SetPair(ctx, pair); err != nil {
			return nil, err
		}
	}
	s.k.recordAudit(ctx, pair.Id, 0, "clear_batch", m.Caller, "",
		fmt.Sprintf("matched=%d clearing=%d done=%v", matched, clearing, done))
	s.k.emit(ctx, "clear_batch",
		"pair_id", u64s(pair.Id),
		"matched", u64s(uint64(matched)),
		"clearing_price", u64s(clearing),
		"done", fmt.Sprintf("%v", done),
	)
	return &types.MsgClearBatchResponse{MatchedPairs: matched, ClearingPrice: clearing, BatchDone: done}, nil
}

// ---- UpdateParams -----------------------------------------------------

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
