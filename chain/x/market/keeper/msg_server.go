package keeper

import (
	"context"
	"strconv"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/market/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{k: k} }

var _ types.MsgServer = msgServer{}

func u(v uint64) string { return strconv.FormatUint(v, 10) }

func (s msgServer) requireAuthority(addr string) error {
	if addr != s.k.authority {
		return types.ErrUnauthorized.Wrapf("expected authority %q", s.k.authority)
	}
	return nil
}

func (s msgServer) CreateMarket(ctx context.Context, m *types.MsgCreateMarket) (*types.MsgCreateMarketResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if uint32(m.FeeBps) > p.MaxFeeBps {
		return nil, types.ErrInvalidField.Wrapf("fee_bps exceeds max %d", p.MaxFeeBps)
	}
	if m.BatchInterval < p.MinBatchInterval {
		return nil, types.ErrInvalidField.Wrapf("batch_interval below min %d", p.MinBatchInterval)
	}
	// Listing-bond gate: when params.listing_bond > 0 the market opens in
	// PENDING_BOND and a designated operator must escrow the bond first.
	bondRequired := p.ListingBond.IsPositive()
	if bondRequired && m.Operator == "" {
		return nil, types.ErrInvalidField.Wrap("operator required while listing_bond > 0")
	}
	if m.Operator != "" {
		if err := types.MustBech32(m.Operator); err != nil {
			return nil, err
		}
	}
	if baseTokenID, isRWA := parseRWADenom(m.BaseDenom); isRWA {
		// RWA-base market: base is an x/rwatoken security token, quote is its
		// own settlement stablecoin. The token must exist and be tradable, and
		// the quote leg must be that token's settlement denom (a registered,
		// active settlement denom). This keeps secondary price discovery in the
		// same stablecoin dividends/redemptions settle in.
		if s.k.asset == nil {
			return nil, types.ErrSettlement.Wrap("rwa asset keeper not wired")
		}
		if !s.k.asset.MarketHasToken(ctx, baseTokenID) {
			return nil, types.ErrSettlement.Wrapf("rwa token %d not found", baseTokenID)
		}
		if !s.k.asset.MarketTokenTradable(ctx, baseTokenID) {
			return nil, types.ErrSettlement.Wrapf("rwa token %d not tradable", baseTokenID)
		}
		sd, ok := s.k.asset.MarketSettlementDenom(ctx, baseTokenID)
		if !ok || sd != m.QuoteDenom {
			return nil, types.ErrSettlement.Wrapf("quote must be token %d settlement denom %q", baseTokenID, sd)
		}
		if s.k.settlement == nil || !s.k.settlement.HasDenom(ctx, m.QuoteDenom) {
			return nil, types.ErrSettlement.Wrap("quote must be a registered settlement denom")
		}
		// The listing operator of an RWA market must be the token's issuer
		// (admin): only the issuer may bond-open its own secondary market.
		if m.Operator != "" {
			admin, ok := s.k.asset.MarketTokenAdmin(ctx, baseTokenID)
			if !ok {
				return nil, types.ErrSettlement.Wrapf("rwa token %d admin not found", baseTokenID)
			}
			if m.Operator != admin {
				return nil, types.ErrInvalidField.Wrapf("operator must be rwa token %d admin %s", baseTokenID, admin)
			}
		}
	} else if s.k.settlement == nil || !s.k.settlement.HasDenom(ctx, m.BaseDenom) || !s.k.settlement.HasDenom(ctx, m.QuoteDenom) {
		return nil, types.ErrSettlement.Wrap("base and quote must be registered settlement denoms")
	}
	n, err := s.k.CountMarkets(ctx)
	if err != nil {
		return nil, err
	}
	if n >= p.MaxMarkets {
		return nil, types.ErrLimitExceeded.Wrap("max_markets")
	}
	id, err := nextSeq(ctx, s.k.MarketSeq)
	if err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	now := sdkCtx.BlockTime().Unix()
	status := types.MarketStatus_MARKET_STATUS_ACTIVE
	if bondRequired {
		status = types.MarketStatus_MARKET_STATUS_PENDING_BOND
	}
	mk := types.Market{
		Id: id, BaseDenom: m.BaseDenom, QuoteDenom: m.QuoteDenom,
		Status: status, FeeBps: m.FeeBps,
		MinBaseQty: m.MinBaseQty, BatchInterval: m.BatchInterval, LastBatchTime: now, CreatedAt: now,
		RequireKyc: m.RequireKyc, PolicyId: m.PolicyId,
		Operator: m.Operator, BondAmount: sdkmath.ZeroInt(),
	}
	if err := s.k.Markets.Set(ctx, id, mk); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeMarket, types.AttrAction, "create", types.AttrMarketID, u(id),
		types.AttrStatus, status.String(), types.AttrOperator, m.Operator)
	return &types.MsgCreateMarketResponse{MarketId: id}, nil
}

// PostBond escrows params.listing_bond from the designated operator into the
// module bond pool and opens the market (PENDING_BOND -> ACTIVE). The amount
// escrowed is recorded on the market so later governance changes to the
// param never affect this market's refund.
func (s msgServer) PostBond(ctx context.Context, m *types.MsgPostBond) (*types.MsgPostBondResponse, error) {
	mk, ok, err := s.k.GetMarket(ctx, m.MarketId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("market %d", m.MarketId)
	}
	if mk.Status != types.MarketStatus_MARKET_STATUS_PENDING_BOND {
		return nil, types.ErrMarketState.Wrapf("market %d not awaiting bond (status %s)", mk.Id, mk.Status)
	}
	if m.Operator != mk.Operator {
		return nil, types.ErrUnauthorized.Wrapf("only operator %s may post the bond", mk.Operator)
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	// The bond charged is the CURRENT param (not the one at creation time):
	// pending markets always pay what governance currently requires.
	bond := p.ListingBond
	denom := p.ListingBondDenom
	if bond.IsPositive() {
		if s.k.bank == nil {
			return nil, types.ErrBond.Wrap("bank keeper not wired")
		}
		op, err := sdk.AccAddressFromBech32(m.Operator)
		if err != nil {
			return nil, types.ErrInvalidAddress.Wrapf("operator: %v", err)
		}
		coins := sdk.NewCoins(sdk.NewCoin(denom, bond))
		if err := s.k.bank.SendCoinsFromAccountToModule(ctx, op, types.BondPoolName, coins); err != nil {
			return nil, types.ErrBond.Wrapf("escrow bond: %v", err)
		}
		mk.BondAmount = bond
		mk.BondDenom = denom
	} else {
		// Governance dropped the bond to 0 while this market was pending:
		// activate without escrow.
		mk.BondAmount = sdkmath.ZeroInt()
		mk.BondDenom = ""
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	mk.Status = types.MarketStatus_MARKET_STATUS_ACTIVE
	// Reset the batch clock so the first batch interval starts at activation.
	mk.LastBatchTime = sdkCtx.BlockTime().Unix()
	if err := s.k.Markets.Set(ctx, mk.Id, mk); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeBond, types.AttrAction, "posted",
		types.AttrMarketID, u(mk.Id), types.AttrOperator, mk.Operator,
		types.AttrAmount, mk.BondAmount.String(), types.AttrDenom, mk.BondDenom)
	return &types.MsgPostBondResponse{Amount: mk.BondAmount.String(), Denom: mk.BondDenom}, nil
}

// DelistMarket (governance) winds a market down: every open order is
// cancelled with a full escrow refund, the listing bond recorded at post
// time is returned to the operator, and the market enters the terminal
// DELISTED state.
func (s msgServer) DelistMarket(ctx context.Context, m *types.MsgDelistMarket) (*types.MsgDelistMarketResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	mk, ok, err := s.k.GetMarket(ctx, m.MarketId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("market %d", m.MarketId)
	}
	if mk.Status == types.MarketStatus_MARKET_STATUS_DELISTED {
		return nil, types.ErrMarketState.Wrapf("market %d already delisted", mk.Id)
	}

	// Cancel every open order with a full escrow refund. This is a
	// governance-approved wind-down, so refunds bypass the owner-initiated
	// cancel's sanction block: leaving escrow stranded in a dead market
	// would break the escrow/open-order reconciliation invariant.
	open, err := s.k.openOrders(ctx, mk.Id)
	if err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	var cancelled uint32
	for _, o := range open {
		refund := o.Escrowed
		if o.Side == types.OrderSide_ORDER_SIDE_SELL {
			if err := s.k.baseRefund(ctx, mk, o.Owner, refund); err != nil {
				return nil, err
			}
		} else {
			if err := s.k.moveSettlement(ctx, mk.QuoteDenom, EscrowAccount(), o.Owner, refund); err != nil {
				return nil, err
			}
		}
		o.Escrowed = 0
		o.Status = types.OrderStatus_ORDER_STATUS_CANCELLED
		if err := s.k.closeOrder(ctx, o); err != nil {
			return nil, err
		}
		cancelled++
		emitEvent(sdkCtx, types.EventTypeOrder, types.AttrAction, "cancel", types.AttrOrderID, u(o.Id),
			types.AttrMarketID, u(o.MarketId), types.AttrOwner, o.Owner, types.AttrRefunded, u(refund))
	}

	// Refund the bond recorded at post time (immune to later param changes).
	refunded := sdkmath.ZeroInt()
	if !mk.BondAmount.IsNil() && mk.BondAmount.IsPositive() {
		if s.k.bank == nil {
			return nil, types.ErrBond.Wrap("bank keeper not wired")
		}
		op, err := sdk.AccAddressFromBech32(mk.Operator)
		if err != nil {
			return nil, types.ErrInvalidAddress.Wrapf("operator: %v", err)
		}
		coins := sdk.NewCoins(sdk.NewCoin(mk.BondDenom, mk.BondAmount))
		if err := s.k.bank.SendCoinsFromModuleToAccount(ctx, types.BondPoolName, op, coins); err != nil {
			return nil, types.ErrBond.Wrapf("refund bond: %v", err)
		}
		refunded = mk.BondAmount
		emitEvent(sdkCtx, types.EventTypeBond, types.AttrAction, "refunded",
			types.AttrMarketID, u(mk.Id), types.AttrOperator, mk.Operator,
			types.AttrAmount, refunded.String(), types.AttrDenom, mk.BondDenom)
	}
	mk.BondAmount = sdkmath.ZeroInt()
	mk.BondDenom = ""
	mk.Status = types.MarketStatus_MARKET_STATUS_DELISTED
	if err := s.k.Markets.Set(ctx, mk.Id, mk); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeDelist, types.AttrMarketID, u(mk.Id),
		types.AttrOperator, mk.Operator, types.AttrRefunded, refunded.String())
	return &types.MsgDelistMarketResponse{CancelledOrders: cancelled, Refunded: refunded.String()}, nil
}

func (s msgServer) SetMarketStatus(ctx context.Context, m *types.MsgSetMarketStatus) (*types.MsgSetMarketStatusResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if !types.MarketStatusValid(m.Status) {
		return nil, types.ErrInvalidField.Wrap("invalid status")
	}
	mk, ok, err := s.k.GetMarket(ctx, m.MarketId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("market %d", m.MarketId)
	}
	// PENDING_BOND can only be exited by the operator posting the bond and
	// DELISTED is terminal — governance must not toggle either through here.
	if !types.MarketStatusValid(mk.Status) {
		return nil, types.ErrMarketState.Wrapf("market %d status %s cannot be changed", mk.Id, mk.Status)
	}
	mk.Status = m.Status
	if err := s.k.Markets.Set(ctx, mk.Id, mk); err != nil {
		return nil, err
	}
	emitEvent(sdk.UnwrapSDKContext(ctx), types.EventTypeMarket, types.AttrAction, "status", types.AttrMarketID, u(mk.Id), types.AttrStatus, m.Status.String())
	return &types.MsgSetMarketStatusResponse{}, nil
}

func (s msgServer) PlaceOrder(ctx context.Context, m *types.MsgPlaceOrder) (*types.MsgPlaceOrderResponse, error) {
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if p.Paused {
		return nil, types.ErrPaused
	}
	if !types.OrderSideValid(m.Side) {
		return nil, types.ErrInvalidField.Wrap("invalid side")
	}
	mk, ok, err := s.k.GetMarket(ctx, m.MarketId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("market %d", m.MarketId)
	}
	if mk.Status != types.MarketStatus_MARKET_STATUS_ACTIVE {
		return nil, types.ErrMarketState.Wrapf("market %d not active", mk.Id)
	}
	if m.Quantity < mk.MinBaseQty {
		return nil, types.ErrInvalidField.Wrapf("quantity below market min %d", mk.MinBaseQty)
	}
	if err := s.k.orderCompliance(ctx, mk, m.Owner, m.Quantity); err != nil {
		return nil, err
	}
	cnt, err := s.k.countOpenOrders(ctx, mk.Id)
	if err != nil {
		return nil, err
	}
	if cnt >= p.MaxOpenOrdersPerMarket {
		return nil, types.ErrLimitExceeded.Wrap("max_open_orders_per_market")
	}

	var escrow uint64
	if m.Side == types.OrderSide_ORDER_SIDE_BUY {
		// For an RWA-base market, screen the buyer's eligibility to RECEIVE the
		// units up front (compliance + per-holder cap headroom) so a buy that
		// could never be delivered is rejected at placement rather than voided
		// later or stalling a batch.
		if baseTokenID, isRWA := parseRWADenom(mk.BaseDenom); isRWA && s.k.asset != nil {
			if err := s.k.asset.MarketReceiverOK(ctx, baseTokenID, m.Owner, m.Quantity); err != nil {
				return nil, err
			}
		}
		escrow, err = types.QuoteCeil(m.Price, m.Quantity)
		if err != nil {
			return nil, err
		}
		if err := s.k.moveSettlement(ctx, mk.QuoteDenom, m.Owner, EscrowAccount(), escrow); err != nil {
			return nil, err
		}
	} else {
		escrow = m.Quantity
		if err := s.k.baseEscrow(ctx, mk, m.Owner, escrow); err != nil {
			return nil, err
		}
	}

	id, err := nextSeq(ctx, s.k.OrderSeq)
	if err != nil {
		return nil, err
	}
	seq, err := nextSeq(ctx, s.k.PlaceSeq)
	if err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	o := types.Order{
		Id: id, MarketId: mk.Id, Owner: m.Owner, Side: m.Side, Price: m.Price,
		Quantity: m.Quantity, Filled: 0, Escrowed: escrow,
		Status: types.OrderStatus_ORDER_STATUS_OPEN, CreatedAt: sdkCtx.BlockTime().Unix(), Seq: seq,
	}
	if err := s.k.setOrderOpen(ctx, o); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeOrder, types.AttrAction, "place", types.AttrOrderID, u(id),
		types.AttrMarketID, u(mk.Id), types.AttrOwner, m.Owner, types.AttrSide, m.Side.String(),
		types.AttrPrice, u(m.Price), types.AttrQty, u(m.Quantity))
	return &types.MsgPlaceOrderResponse{OrderId: id}, nil
}

func (s msgServer) CancelOrder(ctx context.Context, m *types.MsgCancelOrder) (*types.MsgCancelOrderResponse, error) {
	o, ok, err := s.k.GetOrder(ctx, m.OrderId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("order %d", m.OrderId)
	}
	if o.Owner != m.Owner {
		return nil, types.ErrUnauthorized.Wrap("not order owner")
	}
	if o.Status != types.OrderStatus_ORDER_STATUS_OPEN {
		return nil, types.ErrOrderState.Wrapf("order %d not open", o.Id)
	}
	// A sanctioned owner cannot reclaim escrow; funds stay escrowed until the
	// sanction clears (consistent with the other settlement rails).
	if s.k.isSanctioned(ctx, o.Owner) {
		return nil, types.ErrCompliance.Wrapf("%s sanctioned", o.Owner)
	}
	mk, ok, err := s.k.GetMarket(ctx, o.MarketId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("market %d", o.MarketId)
	}
	refund := o.Escrowed
	if o.Side == types.OrderSide_ORDER_SIDE_SELL {
		if err := s.k.baseRefund(ctx, mk, o.Owner, refund); err != nil {
			return nil, err
		}
	} else {
		if err := s.k.moveSettlement(ctx, mk.QuoteDenom, EscrowAccount(), o.Owner, refund); err != nil {
			return nil, err
		}
	}
	o.Escrowed = 0
	o.Status = types.OrderStatus_ORDER_STATUS_CANCELLED
	if err := s.k.closeOrder(ctx, o); err != nil {
		return nil, err
	}
	emitEvent(sdk.UnwrapSDKContext(ctx), types.EventTypeOrder, types.AttrAction, "cancel", types.AttrOrderID, u(o.Id),
		types.AttrMarketID, u(o.MarketId), types.AttrOwner, o.Owner, types.AttrRefunded, u(refund))
	return &types.MsgCancelOrderResponse{Refunded: refund}, nil
}

func (s msgServer) UpdateParams(ctx context.Context, m *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if err := s.k.SetParams(ctx, m.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
