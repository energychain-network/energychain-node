package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/clearing/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{k: k} }

func (s msgServer) requireAuthority(authority string) error {
	if authority != s.k.authority {
		return fmt.Errorf("invalid authority: expected %s, got %s", s.k.authority, authority)
	}
	return nil
}

// ---- RegisterMember --------------------------------------------------

func (s msgServer) RegisterMember(goCtx context.Context, m *types.MsgRegisterMember) (*types.MsgRegisterMemberResponse, error) {
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
	// Block double-registration of the same Bech32 address —
	// the on-chain identity has to be 1:1 with the clearing
	// member id or the netting algorithm can attribute moves
	// to the wrong principal.
	if _, ok, err := s.k.MemberByAddress(ctx, m.MemberAddress); err != nil {
		return nil, err
	} else if ok {
		return nil, fmt.Errorf("address %s already registered as clearing member", m.MemberAddress)
	}
	if err := s.k.requireUnsanctioned(ctx, m.MemberAddress, "member_address"); err != nil {
		return nil, err
	}
	cnt, err := s.k.CountMembers(ctx)
	if err != nil {
		return nil, err
	}
	if cnt >= p.MaxMembers {
		return nil, fmt.Errorf("max_members cap %d reached", p.MaxMembers)
	}
	id, err := s.k.NextMemberID(ctx)
	if err != nil {
		return nil, err
	}
	now := ctx.BlockTime().Unix()
	mem := types.Member{
		Id:               id,
		Address:          m.MemberAddress,
		Status:           types.MemberStatus_MEMBER_STATUS_ACTIVE,
		RequiredMargin:   m.RequiredMargin,
		DefaultFundShare: map[string]uint64{},
		JoinedAt:         now,
		Memo:             m.Memo,
	}
	if err := s.k.SetMember(ctx, mem); err != nil {
		return nil, err
	}
	if err := s.k.MemberByAddr.Set(ctx, mem.Address, mem.Id); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, 0, mem.Id, "register_member", m.Authority, m.MemberAddress, m.Memo)
	s.k.emit(ctx, "register_member", "id", u64s(mem.Id), "address", mem.Address)
	return &types.MsgRegisterMemberResponse{MemberId: mem.Id}, nil
}

// ---- UpdateMemberStatus ----------------------------------------------

func (s msgServer) UpdateMemberStatus(goCtx context.Context, m *types.MsgUpdateMemberStatus) (*types.MsgUpdateMemberStatusResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	mem, err := s.k.MustGetMember(ctx, m.MemberId)
	if err != nil {
		return nil, err
	}
	// EXPELLED is terminal — once set it cannot be reverted.
	if mem.Status == types.MemberStatus_MEMBER_STATUS_EXPELLED {
		return nil, fmt.Errorf("member %d already expelled", m.MemberId)
	}
	mem.Status = m.NewStatus
	if err := s.k.SetMember(ctx, mem); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, 0, mem.Id, "update_member_status", m.Authority, mem.Address, m.Reason)
	s.k.emit(ctx, "update_member_status", "id", u64s(mem.Id), "status", mem.Status.String(), "reason", m.Reason)
	return &types.MsgUpdateMemberStatusResponse{}, nil
}

// ---- PostMargin -------------------------------------------------------

func (s msgServer) PostMargin(goCtx context.Context, m *types.MsgPostMargin) (*types.MsgPostMarginResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	mem, err := s.k.MustGetMember(ctx, m.MemberId)
	if err != nil {
		return nil, err
	}
	if mem.Address != m.MemberAddress {
		return nil, fmt.Errorf("member_address mismatch (registered %s, signer %s)", mem.Address, m.MemberAddress)
	}
	if mem.Status == types.MemberStatus_MEMBER_STATUS_EXPELLED {
		return nil, fmt.Errorf("member %d expelled", mem.Id)
	}
	if err := s.k.requireUnsanctioned(ctx, m.MemberAddress, "member"); err != nil {
		return nil, err
	}
	if err := s.k.fundPool(ctx, m.Denom, m.MemberAddress, m.Amount); err != nil {
		return nil, err
	}
	cur, err := s.k.GetMargin(ctx, mem.Id, m.Denom)
	if err != nil {
		return nil, err
	}
	nb, err := types.SafeAdd(cur, m.Amount)
	if err != nil {
		return nil, err
	}
	if err := s.k.setMargin(ctx, mem.Id, m.Denom, nb); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, 0, mem.Id, "post_margin", m.MemberAddress, m.Denom, u64s(m.Amount))
	s.k.emit(ctx, "post_margin",
		"member_id", u64s(mem.Id), "denom", m.Denom,
		"amount", u64s(m.Amount), "balance", u64s(nb))
	return &types.MsgPostMarginResponse{NewBalance: nb}, nil
}

// ---- WithdrawMargin ---------------------------------------------------

func (s msgServer) WithdrawMargin(goCtx context.Context, m *types.MsgWithdrawMargin) (*types.MsgWithdrawMarginResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	mem, err := s.k.MustGetMember(ctx, m.MemberId)
	if err != nil {
		return nil, err
	}
	if mem.Address != m.MemberAddress {
		return nil, fmt.Errorf("member_address mismatch")
	}
	if mem.Status == types.MemberStatus_MEMBER_STATUS_SUSPENDED {
		return nil, fmt.Errorf("member %d suspended", mem.Id)
	}
	cur, err := s.k.GetMargin(ctx, mem.Id, m.Denom)
	if err != nil {
		return nil, err
	}
	if cur < m.Amount {
		return nil, fmt.Errorf("insufficient margin: have %d, requested %d", cur, m.Amount)
	}
	nb := cur - m.Amount
	// Enforce floors. Two distinct caps:
	//   1) per-denom required_margin (configurable per-member
	//      onboarding deposit). Expelled members are allowed
	//      to drain BELOW this floor since they cannot incur
	//      new obligations; the floor exists for risk
	//      onboarding, not for backing existing obligations.
	//   2) outstanding reservation (gross outbound exposure
	//      across all non-terminal cycles). This is NEVER
	//      bypassable, even for EXPELLED members, because
	//      these amounts are owed to counterparties and
	//      represent commitments made before expulsion. An
	//      expelled member's margin remains locked until the
	//      cycles their obligations belong to are settled or
	//      cancelled.
	if mem.Status != types.MemberStatus_MEMBER_STATUS_EXPELLED {
		if reqd, ok := mem.RequiredMargin[m.Denom]; ok && nb < reqd {
			return nil, fmt.Errorf("withdraw would breach required_margin[%s] = %d (would leave %d)", m.Denom, reqd, nb)
		}
	}
	resv, err := s.k.GetReservation(ctx, mem.Id, m.Denom)
	if err != nil {
		return nil, err
	}
	if nb < resv {
		return nil, fmt.Errorf("withdraw would breach outstanding reservation[%s] = %d (would leave %d)", m.Denom, resv, nb)
	}
	// Chain-level sanctions re-check on the payee leg — the
	// stablecoin freeze (drainPool) catches denom-level
	// blocks, but a chain-sanctioned address not yet frozen
	// per-denom must still be refused.
	if err := s.k.requireUnsanctioned(ctx, m.MemberAddress, "member"); err != nil {
		return nil, err
	}
	if err := s.k.setMargin(ctx, mem.Id, m.Denom, nb); err != nil {
		return nil, err
	}
	if err := s.k.drainPool(ctx, m.Denom, m.MemberAddress, m.Amount); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, 0, mem.Id, "withdraw_margin", m.MemberAddress, m.Denom, u64s(m.Amount))
	s.k.emit(ctx, "withdraw_margin",
		"member_id", u64s(mem.Id), "denom", m.Denom,
		"amount", u64s(m.Amount), "balance", u64s(nb))
	return &types.MsgWithdrawMarginResponse{NewBalance: nb}, nil
}

// ---- FundDefaultFund --------------------------------------------------

func (s msgServer) FundDefaultFund(goCtx context.Context, m *types.MsgFundDefaultFund) (*types.MsgFundDefaultFundResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	mem, err := s.k.MustGetMember(ctx, m.MemberId)
	if err != nil {
		return nil, err
	}
	if mem.Address != m.MemberAddress {
		return nil, fmt.Errorf("member_address mismatch")
	}
	if mem.Status == types.MemberStatus_MEMBER_STATUS_EXPELLED {
		return nil, fmt.Errorf("expelled members cannot contribute")
	}
	if err := s.k.requireUnsanctioned(ctx, m.MemberAddress, "member"); err != nil {
		return nil, err
	}
	if err := s.k.fundPool(ctx, m.Denom, m.MemberAddress, m.Amount); err != nil {
		return nil, err
	}
	if mem.DefaultFundShare == nil {
		mem.DefaultFundShare = map[string]uint64{}
	}
	curShare := mem.DefaultFundShare[m.Denom]
	newShare, err := types.SafeAdd(curShare, m.Amount)
	if err != nil {
		return nil, err
	}
	mem.DefaultFundShare[m.Denom] = newShare
	if err := s.k.SetMember(ctx, mem); err != nil {
		return nil, err
	}
	curFund, err := s.k.GetDefaultFund(ctx, m.Denom)
	if err != nil {
		return nil, err
	}
	newFund, err := types.SafeAdd(curFund, m.Amount)
	if err != nil {
		return nil, err
	}
	if err := s.k.setDefaultFund(ctx, m.Denom, newFund); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, 0, mem.Id, "fund_default_fund", m.MemberAddress, m.Denom, u64s(m.Amount))
	s.k.emit(ctx, "fund_default_fund",
		"member_id", u64s(mem.Id), "denom", m.Denom,
		"amount", u64s(m.Amount), "fund_total", u64s(newFund))
	return &types.MsgFundDefaultFundResponse{NewShare: newShare}, nil
}

// ---- OpenCycle --------------------------------------------------------

func (s msgServer) OpenCycle(goCtx context.Context, m *types.MsgOpenCycle) (*types.MsgOpenCycleResponse, error) {
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
	open, err := s.k.CountOpenCycles(ctx)
	if err != nil {
		return nil, err
	}
	if open >= p.MaxOpenCycles {
		return nil, fmt.Errorf("max_open_cycles cap %d reached", p.MaxOpenCycles)
	}
	id, err := s.k.NextCycleID(ctx)
	if err != nil {
		return nil, err
	}
	now := ctx.BlockTime().Unix()
	c := types.Cycle{
		Id:        id,
		Status:    types.CycleStatus_CYCLE_STATUS_OPEN,
		OpenTime:  now,
		Memo:      m.Memo,
	}
	if err := s.k.SetCycle(ctx, c); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, c.Id, 0, "open_cycle", m.Authority, "", m.Memo)
	s.k.emit(ctx, "open_cycle", "cycle_id", u64s(c.Id), "memo", m.Memo)
	return &types.MsgOpenCycleResponse{CycleId: c.Id}, nil
}

// ---- SubmitObligation -------------------------------------------------

func (s msgServer) SubmitObligation(goCtx context.Context, m *types.MsgSubmitObligation) (*types.MsgSubmitObligationResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateSourceRef(m.SourceRef, p.SourceRefMaxLen); err != nil {
		return nil, err
	}
	cycle, err := s.k.MustGetCycle(ctx, m.CycleId)
	if err != nil {
		return nil, err
	}
	if cycle.Status != types.CycleStatus_CYCLE_STATUS_OPEN {
		return nil, fmt.Errorf("cycle %d not OPEN (status=%s)", cycle.Id, cycle.Status)
	}
	if cycle.ObligationsCount >= p.MaxObligationsPerCycle {
		return nil, fmt.Errorf("cycle %d obligation cap %d reached", cycle.Id, p.MaxObligationsPerCycle)
	}
	from, err := s.k.MustGetMember(ctx, m.FromMemberId)
	if err != nil {
		return nil, err
	}
	to, err := s.k.MustGetMember(ctx, m.ToMemberId)
	if err != nil {
		return nil, err
	}
	if from.Status == types.MemberStatus_MEMBER_STATUS_EXPELLED || to.Status == types.MemberStatus_MEMBER_STATUS_EXPELLED {
		return nil, fmt.Errorf("expelled member cannot be a leg")
	}
	if from.Status == types.MemberStatus_MEMBER_STATUS_SUSPENDED && m.Submitter != s.k.authority {
		// Suspended members may still receive (so the cycle can
		// flush their inbound legs) but cannot create new
		// outbound obligations unless the authority is acting
		// on their behalf (e.g. close-out batches).
		return nil, fmt.Errorf("from_member %d suspended", from.Id)
	}
	// Authorization gate: authority OR the from-side address.
	// A third-party submitter must not be able to saddle
	// either side with a debt; the authority bypass exists
	// so upstream modules (market / auction / contract) can
	// post composite settlement bundles via system flows.
	if m.Submitter != s.k.authority && m.Submitter != from.Address {
		return nil, fmt.Errorf("submitter %s is neither authority nor from_member.address %s", m.Submitter, from.Address)
	}
	if err := types.ValidateDenom(m.Denom); err != nil {
		return nil, err
	}
	// Reserve the GROSS outbound amount against the from-side
	// margin BEFORE persisting the obligation. This closes the
	// "submit-then-withdraw-then-default" loop: every accepted
	// obligation is backed by margin at submission time, and
	// the reservation is released only at settle / cancel time
	// (in symmetric proportion). The check is gross, not net,
	// because the netting is a within-cycle optimisation and
	// we cannot allow cash leakage on the way to the netting
	// pass: a member with 100 incoming and 100 outgoing nets
	// to zero, but if their margin is 0 and the incoming leg
	// arrives only AFTER they withdraw, the outgoing leg is
	// uncovered. Margin must be available at submit time.
	curResv, err := s.k.GetReservation(ctx, from.Id, m.Denom)
	if err != nil {
		return nil, err
	}
	margin, err := s.k.GetMargin(ctx, from.Id, m.Denom)
	if err != nil {
		return nil, err
	}
	newResv, err := types.SafeAdd(curResv, m.Amount)
	if err != nil {
		return nil, err
	}
	if margin < newResv {
		return nil, fmt.Errorf("insufficient margin: have %d, required reservation %d (cur %d + new %d)",
			margin, newResv, curResv, m.Amount)
	}
	if _, err := s.k.AddReservation(ctx, from.Id, m.Denom, m.Amount); err != nil {
		return nil, err
	}
	id, err := s.k.NextObligationID(ctx)
	if err != nil {
		return nil, err
	}
	now := ctx.BlockTime().Unix()
	o := types.Obligation{
		Id:           id,
		CycleId:      cycle.Id,
		FromMemberId: from.Id,
		ToMemberId:   to.Id,
		Denom:        m.Denom,
		Amount:       m.Amount,
		SourceRef:    m.SourceRef,
		SubmittedAt:  now,
	}
	if err := s.k.SetObligation(ctx, o); err != nil {
		return nil, err
	}
	cycle.ObligationsCount++
	if err := s.k.SetCycle(ctx, cycle); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, cycle.Id, from.Id, "submit_obligation", m.Submitter, to.Address,
		fmt.Sprintf("denom=%s amount=%d ref=%s", m.Denom, m.Amount, m.SourceRef))
	s.k.emit(ctx, "submit_obligation",
		"cycle_id", u64s(cycle.Id),
		"obligation_id", u64s(o.Id),
		"from", u64s(from.Id),
		"to", u64s(to.Id),
		"denom", m.Denom,
		"amount", u64s(m.Amount),
	)
	return &types.MsgSubmitObligationResponse{ObligationId: o.Id}, nil
}

// ---- CloseCycle / SettleCycle / CancelCycle ---------------------------

func (s msgServer) CloseCycle(goCtx context.Context, m *types.MsgCloseCycle) (*types.MsgCloseCycleResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	c, err := s.k.MustGetCycle(ctx, m.CycleId)
	if err != nil {
		return nil, err
	}
	if c.Status != types.CycleStatus_CYCLE_STATUS_OPEN {
		return nil, fmt.Errorf("cycle %d not OPEN (status=%s)", c.Id, c.Status)
	}
	c.Status = types.CycleStatus_CYCLE_STATUS_CLOSED
	c.CloseTime = ctx.BlockTime().Unix()
	if err := s.k.SetCycle(ctx, c); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, c.Id, 0, "close_cycle", m.Authority, "", "")
	s.k.emit(ctx, "close_cycle", "cycle_id", u64s(c.Id))
	return &types.MsgCloseCycleResponse{}, nil
}

func (s msgServer) SettleCycle(goCtx context.Context, m *types.MsgSettleCycle) (*types.MsgSettleCycleResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	c, err := s.k.MustGetCycle(ctx, m.CycleId)
	if err != nil {
		return nil, err
	}
	if c.Status != types.CycleStatus_CYCLE_STATUS_CLOSED {
		return nil, fmt.Errorf("cycle %d not CLOSED (status=%s)", c.Id, c.Status)
	}
	c, evs, err := s.k.settle(ctx, c)
	if err != nil {
		return nil, err
	}
	netCount := uint32(0)
	if err := s.k.NetPositions.Walk(ctx, nil, func(key collections.Triple[uint64, uint64, string], _ types.NetPosition) (bool, error) {
		if key.K1() == c.Id {
			netCount++
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, c.Id, 0, "settle_cycle", m.Authority, "",
		fmt.Sprintf("status=%s nets=%d defaults=%d", c.Status, netCount, len(evs)))
	return &types.MsgSettleCycleResponse{
		NetPositionsCount:  netCount,
		DefaultEventsCount: uint32(len(evs)),
		HasDefaults:        c.Status == types.CycleStatus_CYCLE_STATUS_DEFAULTED,
	}, nil
}

func (s msgServer) CancelCycle(goCtx context.Context, m *types.MsgCancelCycle) (*types.MsgCancelCycleResponse, error) {
	if err := m.ValidateBasic(); err != nil {
		return nil, err
	}
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	c, err := s.k.MustGetCycle(ctx, m.CycleId)
	if err != nil {
		return nil, err
	}
	if c.Status.IsTerminal() {
		return nil, fmt.Errorf("cycle %d already terminal (%s)", c.Id, c.Status)
	}
	if _, err := s.k.cancel(ctx, c); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, c.Id, 0, "cancel_cycle", m.Authority, "", m.Reason)
	s.k.emit(ctx, "cancel_cycle", "cycle_id", u64s(c.Id), "reason", m.Reason)
	return &types.MsgCancelCycleResponse{}, nil
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
