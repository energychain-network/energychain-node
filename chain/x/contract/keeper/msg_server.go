package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/contract/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{k: k} }

func (s msgServer) requireAuthority(authority string) error {
	if authority != s.k.authority {
		return fmt.Errorf("invalid authority: expected %s, got %s", s.k.authority, authority)
	}
	return nil
}

// loadAndCheckParty loads the contract and verifies that `caller`
// is buyer or seller. Returns the loaded contract plus a flag
// indicating which role caller plays (isBuyer).
func (s msgServer) loadAndCheckParty(ctx context.Context, id uint64, caller string) (types.Contract, bool, error) {
	c, err := s.k.MustGetContract(ctx, id)
	if err != nil {
		return types.Contract{}, false, err
	}
	switch caller {
	case c.Buyer:
		return c, true, nil
	case c.Seller:
		return c, false, nil
	}
	return types.Contract{}, false, fmt.Errorf("caller %s is not party to contract %d", caller, id)
}

// ---- CreateContract -----------------------------------------------------

func (s msgServer) CreateContract(goCtx context.Context, m *types.MsgCreateContract) (*types.MsgCreateContractResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}

	if m.SettlementPeriodSeconds < p.MinSettlementPeriodSeconds || m.SettlementPeriodSeconds > p.MaxSettlementPeriodSeconds {
		return nil, fmt.Errorf("settlement_period_seconds %d out of [%d, %d]",
			m.SettlementPeriodSeconds, p.MinSettlementPeriodSeconds, p.MaxSettlementPeriodSeconds)
	}
	if m.MaxOracleStalenessSeconds > p.MaxOracleStalenessSecondsCap {
		return nil, fmt.Errorf("max_oracle_staleness_seconds %d > cap %d",
			m.MaxOracleStalenessSeconds, p.MaxOracleStalenessSecondsCap)
	}
	if err := types.ValidateMemo(m.Memo, p.MemoMaxLen); err != nil {
		return nil, err
	}

	// State-growth bounds before doing any debits.
	count, err := s.k.CountContracts(ctx)
	if err != nil {
		return nil, err
	}
	if count >= p.MaxContracts {
		return nil, fmt.Errorf("contract count cap reached: %d", p.MaxContracts)
	}
	for _, party := range []string{m.Buyer, m.Seller} {
		n, err := s.k.CountContractsForParty(ctx, party)
		if err != nil {
			return nil, err
		}
		if n >= p.MaxContractsPerParty {
			return nil, fmt.Errorf("party %s already holds %d contracts (cap %d)", party, n, p.MaxContractsPerParty)
		}
	}

	// Sanctions on both parties before opening the row.
	for _, who := range []string{m.Buyer, m.Seller} {
		if err := s.k.requireUnsanctioned(ctx, who, "party"); err != nil {
			return nil, err
		}
	}

	now := ctx.BlockTime().Unix()
	startTime := m.StartTime
	if startTime == 0 {
		startTime = now
	}
	if m.EndTime != 0 && m.EndTime <= startTime {
		return nil, fmt.Errorf("end_time %d <= start_time %d", m.EndTime, startTime)
	}
	grace := m.GracePeriodSeconds
	if grace == 0 {
		grace = p.DefaultGracePeriodSeconds
	}

	id, err := s.k.NextContractID(ctx)
	if err != nil {
		return nil, err
	}
	c := types.Contract{
		Id:                        id,
		Kind:                      m.Kind,
		Status:                    types.Status_STATUS_DRAFT,
		Buyer:                     m.Buyer,
		Seller:                    m.Seller,
		SchemaUri:                 m.SchemaUri,
		SchemaHash:                m.SchemaHash,
		AssetDenom:                m.AssetDenom,
		StrikePrice:               m.StrikePrice,
		NotionalQuantity:          m.NotionalQuantity,
		PriceOracleTopic:          m.PriceOracleTopic,
		QuantityOracleTopic:       m.QuantityOracleTopic,
		MaxOracleStalenessSeconds: m.MaxOracleStalenessSeconds,
		MarginRequirement:         m.MarginRequirement,
		SettlementPeriodSeconds:   m.SettlementPeriodSeconds,
		NextSettlementTime:        startTime + m.SettlementPeriodSeconds,
		StartTime:                 startTime,
		EndTime:                   m.EndTime,
		GracePeriodSeconds:        grace,
		CreatedAt:                 now,
		Memo:                      m.Memo,
	}
	if err := s.k.SetContract(ctx, c); err != nil {
		return nil, err
	}
	if err := s.k.ContractByParty.Set(ctx, collections.Join(c.Buyer, c.Id)); err != nil {
		return nil, err
	}
	if err := s.k.ContractByParty.Set(ctx, collections.Join(c.Seller, c.Id)); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, c.Id, "create", m.Drafter, "", fmt.Sprintf("kind=%s buyer=%s seller=%s", c.Kind, c.Buyer, c.Seller))
	s.k.emit(ctx, "create",
		"id", u64s(c.Id),
		"kind", c.Kind.String(),
		"buyer", c.Buyer,
		"seller", c.Seller,
		"denom", c.AssetDenom,
		"start_time", i64s(c.StartTime),
	)
	return &types.MsgCreateContractResponse{ContractId: c.Id}, nil
}

// ---- DepositMargin ------------------------------------------------------

func (s msgServer) DepositMargin(goCtx context.Context, m *types.MsgDepositMargin) (*types.MsgDepositMarginResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	c, isBuyer, err := s.loadAndCheckParty(ctx, m.ContractId, m.Party)
	if err != nil {
		return nil, err
	}
	if c.Status.IsTerminal() {
		return nil, fmt.Errorf("contract %d is terminal (%s)", c.Id, c.Status)
	}
	if err := s.k.requireUnsanctioned(ctx, m.Party, "party"); err != nil {
		return nil, err
	}
	// Overflow guard BEFORE moving funds. SafeAdd here is purely
	// defensive: with uint64 caps on stablecoin supply this is a
	// theoretical case, but reordering keeps the pool-balance
	// invariant ("pool = sum(in-keeper margins)") atomic — we
	// never leave a stranded credit in the pool.
	var newBal uint64
	if isBuyer {
		nb, err := types.SafeAdd(c.MarginBuyer, m.Amount)
		if err != nil {
			return nil, err
		}
		newBal = nb
	} else {
		nb, err := types.SafeAdd(c.MarginSeller, m.Amount)
		if err != nil {
			return nil, err
		}
		newBal = nb
	}
	if err := s.k.fundPool(ctx, c.AssetDenom, m.Party, m.Amount); err != nil {
		return nil, err
	}
	if isBuyer {
		c.MarginBuyer = newBal
	} else {
		c.MarginSeller = newBal
	}
	if err := s.k.SetContract(ctx, c); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "deposit_margin",
		"id", u64s(c.Id), "party", m.Party, "amount", u64s(m.Amount))
	bal := c.MarginBuyer
	if !isBuyer {
		bal = c.MarginSeller
	}
	return &types.MsgDepositMarginResponse{NewBalance: bal}, nil
}

// ---- WithdrawMargin -----------------------------------------------------

func (s msgServer) WithdrawMargin(goCtx context.Context, m *types.MsgWithdrawMargin) (*types.MsgWithdrawMarginResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	c, isBuyer, err := s.loadAndCheckParty(ctx, m.ContractId, m.Party)
	if err != nil {
		return nil, err
	}
	bal := c.MarginBuyer
	if !isBuyer {
		bal = c.MarginSeller
	}
	if bal == 0 {
		return nil, fmt.Errorf("no margin to withdraw")
	}
	// On ACTIVE / PAUSED / DRAFT: withdraw only the excess above
	// margin_requirement. On terminal status: the full balance.
	// DISPUTED freezes all withdrawals.
	switch c.Status {
	case types.Status_STATUS_DISPUTED:
		return nil, fmt.Errorf("contract %d disputed; withdrawals frozen", c.Id)
	}
	maxWithdraw := bal
	if !c.Status.IsTerminal() {
		if bal <= c.MarginRequirement {
			return nil, fmt.Errorf("party %s margin %d <= requirement %d; nothing withdrawable", m.Party, bal, c.MarginRequirement)
		}
		maxWithdraw = bal - c.MarginRequirement
	}
	amt := m.Amount
	if amt == 0 || amt > maxWithdraw {
		amt = maxWithdraw
	}
	// Chain-level sanctions re-check on the receiving party.
	// The stablecoin freeze (IsAccountBlocked) is a separate
	// dimension; both must clear before margin returns to a
	// real account. Mirrors the streampay.Cancel fix.
	if err := s.k.requireUnsanctioned(ctx, m.Party, "party"); err != nil {
		return nil, err
	}
	if err := s.k.drainPool(ctx, c.AssetDenom, m.Party, amt); err != nil {
		return nil, err
	}
	if isBuyer {
		c.MarginBuyer -= amt
	} else {
		c.MarginSeller -= amt
	}
	if err := s.k.SetContract(ctx, c); err != nil {
		return nil, err
	}
	s.k.emit(ctx, "withdraw_margin",
		"id", u64s(c.Id), "party", m.Party, "amount", u64s(amt))
	newBal := c.MarginBuyer
	if !isBuyer {
		newBal = c.MarginSeller
	}
	return &types.MsgWithdrawMarginResponse{Withdrawn: amt, NewBalance: newBal}, nil
}

// ---- Sign / Revoke ------------------------------------------------------

func (s msgServer) Sign(goCtx context.Context, m *types.MsgSign) (*types.MsgSignResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	c, isBuyer, err := s.loadAndCheckParty(ctx, m.ContractId, m.Party)
	if err != nil {
		return nil, err
	}
	if c.Status != types.Status_STATUS_DRAFT {
		return nil, fmt.Errorf("contract %d not in DRAFT (status=%s)", c.Id, c.Status)
	}
	if err := s.k.requireUnsanctioned(ctx, m.Party, "party"); err != nil {
		return nil, err
	}
	if isBuyer {
		if c.SignedByBuyer {
			return nil, fmt.Errorf("buyer already signed")
		}
		c.SignedByBuyer = true
	} else {
		if c.SignedBySeller {
			return nil, fmt.Errorf("seller already signed")
		}
		c.SignedBySeller = true
	}
	// Activation rule: both signed AND both margins >= requirement.
	if c.SignedByBuyer && c.SignedBySeller &&
		c.MarginBuyer >= c.MarginRequirement && c.MarginSeller >= c.MarginRequirement {
		c.Status = types.Status_STATUS_ACTIVE
		c.ActivatedAt = ctx.BlockTime().Unix()
	}
	if err := s.k.SetContract(ctx, c); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, c.Id, "sign", m.Party, "", "")
	s.k.emit(ctx, "sign",
		"id", u64s(c.Id), "party", m.Party, "status", c.Status.String())
	return &types.MsgSignResponse{NewStatus: c.Status}, nil
}

func (s msgServer) Revoke(goCtx context.Context, m *types.MsgRevoke) (*types.MsgRevokeResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	c, isBuyer, err := s.loadAndCheckParty(ctx, m.ContractId, m.Party)
	if err != nil {
		return nil, err
	}
	if c.Status != types.Status_STATUS_DRAFT {
		return nil, fmt.Errorf("revoke only valid in DRAFT (status=%s)", c.Status)
	}
	if isBuyer {
		if !c.SignedByBuyer {
			return nil, fmt.Errorf("buyer has not signed")
		}
		c.SignedByBuyer = false
	} else {
		if !c.SignedBySeller {
			return nil, fmt.Errorf("seller has not signed")
		}
		c.SignedBySeller = false
	}
	if err := s.k.SetContract(ctx, c); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, c.Id, "revoke", m.Party, "", m.Reason)
	s.k.emit(ctx, "revoke", "id", u64s(c.Id), "party", m.Party, "reason", m.Reason)
	return &types.MsgRevokeResponse{}, nil
}

// ---- Settle -------------------------------------------------------------

func (s msgServer) Settle(goCtx context.Context, m *types.MsgSettle) (*types.MsgSettleResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	c, err := s.k.MustGetContract(ctx, m.ContractId)
	if err != nil {
		return nil, err
	}
	if c.Status != types.Status_STATUS_ACTIVE {
		return nil, fmt.Errorf("contract %d not ACTIVE (status=%s)", c.Id, c.Status)
	}
	if c.DisputeLocked {
		return nil, fmt.Errorf("contract %d disputed; settlements frozen", c.Id)
	}
	now := ctx.BlockTime().Unix()
	if now < c.NextSettlementTime {
		return nil, fmt.Errorf("contract %d not yet due: next=%d now=%d", c.Id, c.NextSettlementTime, now)
	}
	// Sanctions on both parties at every settlement leg (defends
	// against post-create sanctions actions).
	for _, who := range []string{c.Buyer, c.Seller} {
		if err := s.k.requireUnsanctioned(ctx, who, "party"); err != nil {
			return nil, err
		}
	}

	// Catch up overdue periods, capped at MaxCatchupPeriodsPerSettle.
	// We use the CURRENT oracle reading for every period (the
	// reading is the latest-aggregated value at this block); old
	// readings are intentionally not replayed, and the caller is
	// warned of this in the per-period semantic. This biases the
	// chain toward prompt settlement.
	periods := uint32(0)
	netDelta := int64(0)
	for periods < p.MaxCatchupPeriodsPerSettle && now >= c.NextSettlementTime {
		delta, err := s.k.settlementDelta(ctx, c, now)
		if err != nil {
			return nil, err
		}
		if err := applyDelta(&c, delta); err != nil {
			// Insufficient margin to honour this period's
			// settlement — surface as a clean error so the
			// caller routes via MsgDefault instead.
			return nil, fmt.Errorf("settlement period %d: %w; route via Default", c.SettlementsCount+1, err)
		}
		netDelta += delta
		c.NextSettlementTime += c.SettlementPeriodSeconds
		c.LastSettledTime = now
		c.SettlementsCount++
		periods++
		// Auto-EXPIRE check: if end_time passed and no more
		// settlements remain due, flip to EXPIRED.
		if c.EndTime != 0 && c.NextSettlementTime > c.EndTime {
			c.Status = types.Status_STATUS_EXPIRED
			c.TerminatedAt = now
			break
		}
	}
	if periods == 0 {
		return nil, fmt.Errorf("no settlements due")
	}
	if err := s.k.SetContract(ctx, c); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, c.Id, "settle", m.Caller, "", fmt.Sprintf("periods=%d net=%d", periods, netDelta))
	s.k.emit(ctx, "settle",
		"id", u64s(c.Id),
		"periods", fmt.Sprintf("%d", periods),
		"net_amount", fmt.Sprintf("%d", netDelta),
		"next_settlement", i64s(c.NextSettlementTime),
		"status", c.Status.String(),
	)
	return &types.MsgSettleResponse{PeriodsSettled: periods, NetAmount: netDelta}, nil
}

// ---- Default ------------------------------------------------------------

func (s msgServer) Default(goCtx context.Context, m *types.MsgDefault) (*types.MsgDefaultResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	c, err := s.k.MustGetContract(ctx, m.ContractId)
	if err != nil {
		return nil, err
	}
	if c.Status != types.Status_STATUS_ACTIVE {
		return nil, fmt.Errorf("contract %d not ACTIVE (status=%s)", c.Id, c.Status)
	}
	if c.DisputeLocked {
		return nil, fmt.Errorf("contract %d disputed; default frozen", c.Id)
	}
	now := ctx.BlockTime().Unix()
	overdueBy := now - c.NextSettlementTime
	if overdueBy < c.GracePeriodSeconds {
		return nil, fmt.Errorf("contract %d not overdue: overdue=%ds grace=%ds", c.Id, overdueBy, c.GracePeriodSeconds)
	}
	// Determine the defaulting side via the CURRENT delta:
	// whoever would owe at the latest oracle reading is taken
	// to have failed to honour the next settlement.
	delta, err := s.k.settlementDelta(ctx, c, now)
	if err != nil {
		// Oracle unavailable or stale — we cannot determine
		// the defaulting side, so refuse the default. The
		// counterparty must either wait for oracle freshness
		// or route via Terminate.
		return nil, fmt.Errorf("cannot determine default direction: %w", err)
	}
	var defaulter, winner string
	var slashed uint64
	switch {
	case delta > 0:
		defaulter, winner = c.Buyer, c.Seller
		slashed = c.MarginBuyer
		c.MarginSeller += c.MarginBuyer
		c.MarginBuyer = 0
	case delta < 0:
		defaulter, winner = c.Seller, c.Buyer
		slashed = c.MarginSeller
		c.MarginBuyer += c.MarginSeller
		c.MarginSeller = 0
	default:
		// delta == 0 means nothing is owed — there is nothing
		// to default on. Refuse and recommend Settle instead
		// (which will no-op the period).
		return nil, fmt.Errorf("no settlement owed at current reading; call Settle to advance")
	}
	c.Status = types.Status_STATUS_DEFAULTED
	c.TerminatedAt = now
	if err := s.k.SetContract(ctx, c); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, c.Id, "default", m.Caller, defaulter, fmt.Sprintf("slashed=%d to=%s", slashed, winner))
	s.k.emit(ctx, "default",
		"id", u64s(c.Id),
		"defaulter", defaulter,
		"winner", winner,
		"slashed", u64s(slashed),
		"reason", m.Reason,
	)
	return &types.MsgDefaultResponse{DefaultingParty: defaulter, SlashedAmount: slashed}, nil
}

// ---- Terminate ----------------------------------------------------------

func (s msgServer) Terminate(goCtx context.Context, m *types.MsgTerminate) (*types.MsgTerminateResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	c, isBuyer, err := s.loadAndCheckParty(ctx, m.ContractId, m.Party)
	if err != nil {
		return nil, err
	}
	if c.Status.IsTerminal() {
		return nil, fmt.Errorf("contract %d already terminal (%s)", c.Id, c.Status)
	}
	// Terminate is allowed in DRAFT (cancel before activation)
	// and ACTIVE / PAUSED / DISPUTED — but DISPUTED's resolution
	// owns the row first, so we refuse here.
	if c.Status == types.Status_STATUS_DISPUTED {
		return nil, fmt.Errorf("contract %d disputed; resolve dispute before terminating", c.Id)
	}
	bit := types.TerminateSigBuyer
	if !isBuyer {
		bit = types.TerminateSigSeller
	}
	if c.TerminateSignalMask&bit != 0 {
		return nil, fmt.Errorf("%s already signalled terminate", m.Party)
	}
	c.TerminateSignalMask |= bit

	// DRAFT contracts terminate immediately on first signal
	// (no counterparty has committed funds beyond margin we
	// will refund via Withdraw). Active / Paused contracts
	// require BOTH parties to signal.
	flip := false
	switch c.Status {
	case types.Status_STATUS_DRAFT:
		flip = true
	default:
		if c.TerminateSignalMask == types.TerminateSigBoth {
			flip = true
		}
	}
	if flip {
		c.Status = types.Status_STATUS_TERMINATED
		c.TerminatedAt = ctx.BlockTime().Unix()
	}
	if err := s.k.SetContract(ctx, c); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, c.Id, "terminate", m.Party, "", m.Reason)
	s.k.emit(ctx, "terminate",
		"id", u64s(c.Id), "party", m.Party,
		"status", c.Status.String(), "reason", m.Reason)
	return &types.MsgTerminateResponse{NewStatus: c.Status}, nil
}

// ---- Dispute hooks (authority-only until x/dispute lands) --------------

func (s msgServer) MarkDisputed(goCtx context.Context, m *types.MsgMarkDisputed) (*types.MsgMarkDisputedResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	c, err := s.k.MustGetContract(ctx, m.ContractId)
	if err != nil {
		return nil, err
	}
	if c.Status != types.Status_STATUS_ACTIVE && c.Status != types.Status_STATUS_PAUSED {
		return nil, fmt.Errorf("contract %d not ACTIVE/PAUSED (status=%s)", c.Id, c.Status)
	}
	if c.DisputeLocked {
		return nil, fmt.Errorf("contract %d already disputed", c.Id)
	}
	c.DisputeLocked = true
	c.Status = types.Status_STATUS_DISPUTED
	if err := s.k.SetContract(ctx, c); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, c.Id, "dispute_mark", m.Authority, "", m.Reason)
	s.k.emit(ctx, "dispute_mark", "id", u64s(c.Id), "reason", m.Reason)
	return &types.MsgMarkDisputedResponse{}, nil
}

func (s msgServer) ResolveDispute(goCtx context.Context, m *types.MsgResolveDispute) (*types.MsgResolveDisputeResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	c, err := s.k.MustGetContract(ctx, m.ContractId)
	if err != nil {
		return nil, err
	}
	if !c.DisputeLocked {
		return nil, fmt.Errorf("contract %d not disputed", c.Id)
	}
	c.DisputeLocked = false
	c.Status = types.Status_STATUS_ACTIVE
	if err := s.k.SetContract(ctx, c); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, c.Id, "dispute_resolve", m.Authority, "", m.Reason)
	s.k.emit(ctx, "dispute_resolve", "id", u64s(c.Id), "reason", m.Reason)
	return &types.MsgResolveDisputeResponse{}, nil
}

// ---- UpdateParams -------------------------------------------------------

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
