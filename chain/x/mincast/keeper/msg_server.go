package keeper

import (
	"context"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/mincast/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{k: k} }

var _ types.MsgServer = msgServer{}

func u(v uint64) string { return strconv.FormatUint(v, 10) }

func (s msgServer) getMarket(ctx context.Context, id uint64) (types.Market, error) {
	m, ok, err := s.k.GetMarket(ctx, id)
	if err != nil {
		return types.Market{}, err
	}
	if !ok {
		return types.Market{}, types.ErrNotFound.Wrapf("market %d", id)
	}
	return m, nil
}

func (s msgServer) requireAdmin(ctx context.Context, id uint64, caller string) (types.Market, error) {
	m, err := s.getMarket(ctx, id)
	if err != nil {
		return types.Market{}, err
	}
	if caller != m.Admin && caller != s.k.authority {
		return types.Market{}, types.ErrUnauthorized.Wrapf("not admin of market %d", id)
	}
	return m, nil
}

// requireFunderClear blocks a sanctioned funder from pushing settlement into
// a market pool (mirrors the rwatoken/stableusd pool-funding guard).
func (s msgServer) requireFunderClear(ctx context.Context, funder string) error {
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return err
	}
	if params.RequireSanctionsClear && s.k.compliance != nil && s.k.compliance.IsSanctioned(sdk.UnwrapSDKContext(ctx), funder) {
		return types.ErrCompliance.Wrapf("%s sanctioned", funder)
	}
	return nil
}

// ---- market lifecycle ------------------------------------------------------

func (s msgServer) CreateMarket(ctx context.Context, m *types.MsgCreateMarket) (*types.MsgCreateMarketResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := s.k.requireSettlement(); err != nil {
		return nil, err
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.requireFunderClear(ctx, m.Admin); err != nil {
		return nil, err
	}
	if m.InitialPrice < params.MinInitialPrice {
		return nil, types.ErrInvalidField.Wrapf("initial_price < min %d", params.MinInitialPrice)
	}
	if m.MintFeeBps > params.MaxFeeBps || m.MeltFeeBps > params.MaxFeeBps {
		return nil, types.ErrInvalidField.Wrapf("fee exceeds max %d bps", params.MaxFeeBps)
	}
	if has, e := s.k.MarketByDenom.Has(ctx, m.Denom); e != nil {
		return nil, e
	} else if has {
		return nil, types.ErrAlreadyExists.Wrapf("denom %q", m.Denom)
	}
	if !s.k.settlement.HasDenom(ctx, m.SettlementDenom) {
		return nil, types.ErrSettlement.Wrapf("settlement denom %q not found", m.SettlementDenom)
	}
	n, err := s.k.CountMarkets(ctx)
	if err != nil {
		return nil, err
	}
	if n >= params.MaxMarkets {
		return nil, types.ErrLimitExceeded.Wrap("max_markets")
	}
	id, err := s.k.NextMarketID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdkCtx.BlockTime().Unix()
	market := types.Market{
		Id:              id,
		Denom:           m.Denom,
		Name:            m.Name,
		Admin:           m.Admin,
		SettlementDenom: m.SettlementDenom,
		InitialPrice:    m.InitialPrice,
		MintFeeBps:      m.MintFeeBps,
		MeltFeeBps:      m.MeltFeeBps,
		Status:          types.MarketStatus_MARKET_STATUS_ACTIVE,
		PolicyId:        m.PolicyId,
		RequireKyc:      m.RequireKyc,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.k.setMarket(ctx, market); err != nil {
		return nil, err
	}
	if err := s.k.MarketByDenom.Set(ctx, m.Denom, id); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "create_market", m.Admin, m.Denom, u(id))
	emitEvent(sdkCtx, types.EventTypeMarket, types.AttrAction, "create", types.AttrMarketID, u(id), types.AttrDenom, m.Denom)
	return &types.MsgCreateMarketResponse{MarketId: id}, nil
}

func (s msgServer) UpdateMarket(ctx context.Context, m *types.MsgUpdateMarket) (*types.MsgUpdateMarketResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	market, err := s.requireAdmin(ctx, m.MarketId, m.Admin)
	if err != nil {
		return nil, err
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if m.MintFeeBps > params.MaxFeeBps || m.MeltFeeBps > params.MaxFeeBps {
		return nil, types.ErrInvalidField.Wrapf("fee exceeds max %d bps", params.MaxFeeBps)
	}
	market.Name = m.Name
	market.MintFeeBps = m.MintFeeBps
	market.MeltFeeBps = m.MeltFeeBps
	market.PolicyId = m.PolicyId
	market.RequireKyc = m.RequireKyc
	market.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.setMarket(ctx, market); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeMarket, types.AttrAction, "update", types.AttrMarketID, u(market.Id))
	return &types.MsgUpdateMarketResponse{}, nil
}

func (s msgServer) SetMarketStatus(ctx context.Context, m *types.MsgSetMarketStatus) (*types.MsgSetMarketStatusResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	market, err := s.requireAdmin(ctx, m.MarketId, m.Admin)
	if err != nil {
		return nil, err
	}
	if !types.MarketStatusValid(m.Status) {
		return nil, types.ErrInvalidField.Wrap("invalid status")
	}
	market.Status = m.Status
	market.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.setMarket(ctx, market); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeMarket, types.AttrAction, "status", types.AttrMarketID, u(market.Id), types.AttrStatus, m.Status.String())
	return &types.MsgSetMarketStatusResponse{}, nil
}

// ---- bonding curve ---------------------------------------------------------

func (s msgServer) Mint(ctx context.Context, m *types.MsgMint) (*types.MsgMintResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	market, err := s.getMarket(ctx, m.MarketId)
	if err != nil {
		return nil, err
	}
	if market.Status != types.MarketStatus_MARKET_STATUS_ACTIVE {
		return nil, types.ErrMarketState.Wrap("mint requires an ACTIVE market")
	}
	// Buyer receives mincast units -> gate as a receipt (KYC on buyer).
	if err := s.k.partyCompliance(ctx, market, "", m.Buyer, m.PayAmount); err != nil {
		return nil, err
	}
	fee, err := types.MulBps(m.PayAmount, market.MintFeeBps)
	if err != nil {
		return nil, err
	}
	principal, err := types.SafeSub(m.PayAmount, fee)
	if err != nil {
		return nil, err
	}
	units, err := types.MintUnits(market.Treasury, market.Supply, market.InitialPrice, principal)
	if err != nil {
		return nil, err
	}
	if units == 0 {
		return nil, types.ErrZeroOut.Wrap("pay_amount too small to mint a unit")
	}
	if units < m.MinUnitsOut {
		return nil, types.ErrSlippage.Wrapf("units %d < min %d", units, m.MinUnitsOut)
	}
	// Pull the full payment into the treasury (principal + fee both back the
	// supply and lift the floor).
	if err := s.k.moveSettlement(ctx, market.SettlementDenom, m.Buyer, TreasuryAccount(market.Id), m.PayAmount); err != nil {
		return nil, err
	}
	if market.Treasury, err = types.SafeAdd(market.Treasury, m.PayAmount); err != nil {
		return nil, err
	}
	if market.Supply, err = types.SafeAdd(market.Supply, units); err != nil {
		return nil, err
	}
	if err := s.k.creditBalance(ctx, market.Id, m.Buyer, units); err != nil {
		return nil, err
	}
	if err := s.k.commitFloor(ctx, &market); err != nil {
		return nil, err
	}
	market.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.setMarket(ctx, market); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "mint", m.Buyer, market.Denom, u(units))
	emitEvent(sdkCtx, types.EventTypeTrade, types.AttrAction, "mint", types.AttrMarketID, u(market.Id),
		types.AttrAccount, m.Buyer, types.AttrAmount, u(m.PayAmount), types.AttrUnits, u(units),
		types.AttrFee, u(fee), types.AttrFloor, u(market.FloorPrice))
	return &types.MsgMintResponse{Units: units, FloorPrice: market.FloorPrice}, nil
}

func (s msgServer) Melt(ctx context.Context, m *types.MsgMelt) (*types.MsgMeltResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	market, err := s.getMarket(ctx, m.MarketId)
	if err != nil {
		return nil, err
	}
	if !types.MarketStatusValid(market.Status) {
		return nil, types.ErrMarketState.Wrap("market not tradable")
	}
	// Seller divests units and receives settlement -> gate the seller.
	if err := s.k.partyCompliance(ctx, market, m.Seller, "", m.Units); err != nil {
		return nil, err
	}
	gross, err := types.MeltGross(market.Treasury, market.Supply, m.Units)
	if err != nil {
		return nil, err
	}
	if gross == 0 {
		return nil, types.ErrZeroOut.Wrap("units too small to redeem any settlement")
	}
	// The final exit (burning the entire supply) drains the whole treasury to
	// the last holder, who owns 100% of the backing. No melt fee is charged
	// because there are no remaining holders for it to benefit, and this keeps
	// the invariant treasury == 0 whenever supply == 0 — without it, leftover
	// fees would orphan in the treasury and a later bootstrap mint could skim
	// them risk-free.
	fullExit := m.Units == market.Supply
	var fee uint64
	payout := gross
	if fullExit {
		payout = market.Treasury
	} else {
		fee, err = types.MulBps(gross, market.MeltFeeBps)
		if err != nil {
			return nil, err
		}
		payout, err = types.SafeSub(gross, fee)
		if err != nil {
			return nil, err
		}
	}
	if payout == 0 {
		return nil, types.ErrZeroOut.Wrap("melt fee consumes the entire payout")
	}
	if payout < m.MinSettlementOut {
		return nil, types.ErrSlippage.Wrapf("settlement %d < min %d", payout, m.MinSettlementOut)
	}
	// Burn the seller's units first (fails closed if they lack balance).
	if err := s.k.debitBalance(ctx, market.Id, m.Seller, m.Units); err != nil {
		return nil, err
	}
	if market.Supply, err = types.SafeSub(market.Supply, m.Units); err != nil {
		return nil, err
	}
	// Only the payout leaves the treasury; the fee stays and lifts the floor.
	if market.Treasury, err = types.SafeSub(market.Treasury, payout); err != nil {
		return nil, err
	}
	if err := s.k.moveSettlement(ctx, market.SettlementDenom, TreasuryAccount(market.Id), m.Seller, payout); err != nil {
		return nil, err
	}
	if err := s.k.commitFloor(ctx, &market); err != nil {
		return nil, err
	}
	market.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.setMarket(ctx, market); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "melt", m.Seller, market.Denom, u(m.Units))
	emitEvent(sdkCtx, types.EventTypeTrade, types.AttrAction, "melt", types.AttrMarketID, u(market.Id),
		types.AttrAccount, m.Seller, types.AttrUnits, u(m.Units), types.AttrSettlement, u(payout),
		types.AttrFee, u(fee), types.AttrFloor, u(market.FloorPrice))
	return &types.MsgMeltResponse{Settlement: payout, FloorPrice: market.FloorPrice}, nil
}

func (s msgServer) Transfer(ctx context.Context, m *types.MsgTransfer) (*types.MsgTransferResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	market, err := s.getMarket(ctx, m.MarketId)
	if err != nil {
		return nil, err
	}
	if market.Status == types.MarketStatus_MARKET_STATUS_CLOSED {
		return nil, types.ErrMarketState.Wrap("transfers disabled on a CLOSED market")
	}
	if !types.MarketStatusValid(market.Status) {
		return nil, types.ErrMarketState.Wrap("market not tradable")
	}
	if err := s.k.partyCompliance(ctx, market, m.From, m.To, m.Amount); err != nil {
		return nil, err
	}
	if err := s.k.moveUnits(ctx, market.Id, m.From, m.To, m.Amount); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeTransfer, types.AttrAction, "transfer", types.AttrMarketID, u(market.Id),
		types.AttrFrom, m.From, types.AttrTo, m.To, types.AttrAmount, u(m.Amount))
	return &types.MsgTransferResponse{}, nil
}

// ---- treasury / reward injection -------------------------------------------

func (s msgServer) InjectTreasury(ctx context.Context, m *types.MsgInjectTreasury) (*types.MsgInjectTreasuryResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	market, err := s.getMarket(ctx, m.MarketId)
	if err != nil {
		return nil, err
	}
	// Injecting into an empty market would orphan funds (no holders back it)
	// and seed the bootstrap-mint skim; the next bootstrap minter would own
	// 100% of the injected treasury for free. Require live supply.
	if market.Supply == 0 {
		return nil, types.ErrMarketState.Wrap("cannot inject treasury into a market with no supply")
	}
	if err := s.requireFunderClear(ctx, m.Funder); err != nil {
		return nil, err
	}
	if err := s.k.moveSettlement(ctx, market.SettlementDenom, m.Funder, TreasuryAccount(market.Id), m.Amount); err != nil {
		return nil, err
	}
	if market.Treasury, err = types.SafeAdd(market.Treasury, m.Amount); err != nil {
		return nil, err
	}
	if err := s.k.commitFloor(ctx, &market); err != nil {
		return nil, err
	}
	market.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.setMarket(ctx, market); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeTreasury, types.AttrAction, "inject", types.AttrMarketID, u(market.Id),
		types.AttrAmount, u(m.Amount), types.AttrFloor, u(market.FloorPrice))
	return &types.MsgInjectTreasuryResponse{FloorPrice: market.FloorPrice}, nil
}

func (s msgServer) FundReward(ctx context.Context, m *types.MsgFundReward) (*types.MsgFundRewardResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	market, err := s.getMarket(ctx, m.MarketId)
	if err != nil {
		return nil, err
	}
	if err := s.requireFunderClear(ctx, m.Funder); err != nil {
		return nil, err
	}
	if err := s.k.moveSettlement(ctx, market.SettlementDenom, m.Funder, RewardAccount(market.Id), m.Amount); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeTreasury, types.AttrAction, "fund_reward", types.AttrMarketID, u(market.Id), types.AttrAmount, u(m.Amount))
	return &types.MsgFundRewardResponse{}, nil
}

// ---- invest ----------------------------------------------------------------

func (s msgServer) OpenInvest(ctx context.Context, m *types.MsgOpenInvest) (*types.MsgOpenInvestResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	market, err := s.getMarket(ctx, m.MarketId)
	if err != nil {
		return nil, err
	}
	if market.Status != types.MarketStatus_MARKET_STATUS_ACTIVE {
		return nil, types.ErrMarketState.Wrap("invest requires an ACTIVE market")
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if m.ApyBps > params.MaxApyBps {
		return nil, types.ErrInvalidField.Wrapf("apy_bps > max %d", params.MaxApyBps)
	}
	if m.TermSeconds > params.MaxInvestTermSeconds {
		return nil, types.ErrInvalidField.Wrapf("term_seconds > max %d", params.MaxInvestTermSeconds)
	}
	if err := s.k.partyCompliance(ctx, market, m.Investor, m.Investor, m.Units); err != nil {
		return nil, err
	}
	// Floor-valued principal, snapshotted now so a later floor move cannot
	// inflate the owed yield.
	principalValue, err := types.MeltGross(market.Treasury, market.Supply, m.Units)
	if err != nil {
		return nil, err
	}
	yield, err := types.ProrateYield(principalValue, m.ApyBps, m.TermSeconds, params.YearSeconds)
	if err != nil {
		return nil, err
	}
	// Escrow the units (supply unchanged) so they cannot be melted or
	// transferred while locked.
	if err := s.k.moveUnits(ctx, market.Id, m.Investor, types.InvestEscrow, m.Units); err != nil {
		return nil, err
	}
	id, err := s.k.NextInvestID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdkCtx.BlockTime().Unix()
	iv := types.Invest{
		Id:             id,
		MarketId:       market.Id,
		Investor:       m.Investor,
		PrincipalUnits: m.Units,
		Yield:          yield,
		ApyBps:         m.ApyBps,
		OpenedAt:       now,
		Maturity:       now + m.TermSeconds,
		Status:         types.InvestStatus_INVEST_STATUS_ACTIVE,
	}
	if err := s.k.setInvest(ctx, iv); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "open_invest", m.Investor, market.Denom, u(id))
	emitEvent(sdkCtx, types.EventTypeInvest, types.AttrAction, "open", types.AttrMarketID, u(market.Id),
		types.AttrAccount, m.Investor, types.AttrInvestID, u(id), types.AttrUnits, u(m.Units), types.AttrYield, u(yield))
	return &types.MsgOpenInvestResponse{InvestId: id, Yield: yield, Maturity: iv.Maturity}, nil
}

func (s msgServer) CloseInvest(ctx context.Context, m *types.MsgCloseInvest) (*types.MsgCloseInvestResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	iv, ok, err := s.k.GetInvest(ctx, m.InvestId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("invest %d", m.InvestId)
	}
	if iv.Status != types.InvestStatus_INVEST_STATUS_ACTIVE {
		return nil, types.ErrInvestState.Wrap("not active")
	}
	now := sdkCtx.BlockTime().Unix()
	if now < iv.Maturity {
		return nil, types.ErrNotMatured.Wrapf("matures at %d", iv.Maturity)
	}
	market, err := s.getMarket(ctx, iv.MarketId)
	if err != nil {
		return nil, err
	}
	// Investor receives yield + unlocked units -> gate as a receipt.
	if err := s.k.partyCompliance(ctx, market, "", iv.Investor, iv.PrincipalUnits); err != nil {
		return nil, err
	}
	// Pay yield from the reward pool (fails closed if underfunded).
	if iv.Yield > 0 {
		if s.k.RewardBalance(ctx, market) < iv.Yield {
			return nil, types.ErrRewardShort.Wrapf("reward pool needs %d", iv.Yield)
		}
		if err := s.k.moveSettlement(ctx, market.SettlementDenom, RewardAccount(market.Id), iv.Investor, iv.Yield); err != nil {
			return nil, err
		}
	}
	// Return the escrowed principal units.
	if err := s.k.moveUnits(ctx, market.Id, types.InvestEscrow, iv.Investor, iv.PrincipalUnits); err != nil {
		return nil, err
	}
	iv.Status = types.InvestStatus_INVEST_STATUS_REDEEMED
	iv.ResolvedAt = now
	if err := s.k.setInvest(ctx, iv); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "close_invest", m.Caller, iv.Investor, u(iv.Id))
	emitEvent(sdkCtx, types.EventTypeInvest, types.AttrAction, "close", types.AttrMarketID, u(market.Id),
		types.AttrInvestID, u(iv.Id), types.AttrYield, u(iv.Yield))
	return &types.MsgCloseInvestResponse{Yield: iv.Yield}, nil
}

func (s msgServer) CancelInvest(ctx context.Context, m *types.MsgCancelInvest) (*types.MsgCancelInvestResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	iv, ok, err := s.k.GetInvest(ctx, m.InvestId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("invest %d", m.InvestId)
	}
	if iv.Status != types.InvestStatus_INVEST_STATUS_ACTIVE {
		return nil, types.ErrInvestState.Wrap("not active")
	}
	if m.Investor != iv.Investor && m.Investor != s.k.authority {
		return nil, types.ErrUnauthorized.Wrap("only the investor may cancel")
	}
	// Early exit forfeits the yield and returns the locked principal units.
	if err := s.k.moveUnits(ctx, iv.MarketId, types.InvestEscrow, iv.Investor, iv.PrincipalUnits); err != nil {
		return nil, err
	}
	iv.Status = types.InvestStatus_INVEST_STATUS_CANCELLED
	iv.ResolvedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.setInvest(ctx, iv); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "cancel_invest", m.Investor, iv.Investor, u(iv.Id))
	emitEvent(sdkCtx, types.EventTypeInvest, types.AttrAction, "cancel", types.AttrMarketID, u(iv.MarketId), types.AttrInvestID, u(iv.Id))
	return &types.MsgCancelInvestResponse{}, nil
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
