package keeper

import (
	"context"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/offering/types"
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

func (s msgServer) getOffering(ctx context.Context, id uint64) (types.Offering, error) {
	o, ok, err := s.k.GetOffering(ctx, id)
	if err != nil {
		return types.Offering{}, err
	}
	if !ok {
		return types.Offering{}, types.ErrNotFound.Wrapf("offering %d", id)
	}
	return o, nil
}

// requireIssuer asserts the caller controls the offering (its issuer or the
// governance authority).
func (s msgServer) requireIssuer(o types.Offering, caller string) error {
	if caller != o.Issuer && caller != s.k.authority {
		return types.ErrUnauthorized.Wrapf("not issuer of offering %d", o.Id)
	}
	return nil
}

// ---- lifecycle ------------------------------------------------------------

func (s msgServer) CreateOffering(ctx context.Context, m *types.MsgCreateOffering) (*types.MsgCreateOfferingResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if s.k.rwa == nil {
		return nil, types.ErrRWA.Wrap("rwa keeper not wired")
	}
	if err := s.k.requireSettlement(); err != nil {
		return nil, err
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if m.TotalTranches > params.MaxTranches {
		return nil, types.ErrInvalidField.Wrapf("total_tranches > max %d", params.MaxTranches)
	}
	n, err := s.k.CountOfferings(ctx)
	if err != nil {
		return nil, err
	}
	if n >= params.MaxOfferings {
		return nil, types.ErrLimitExceeded.Wrap("max_offerings")
	}
	admin, ok := s.k.rwa.TokenAdmin(ctx, m.TokenId)
	if !ok {
		return nil, types.ErrRWA.Wrapf("token %d not found", m.TokenId)
	}
	if admin != m.Issuer {
		return nil, types.ErrUnauthorized.Wrap("issuer is not the token admin")
	}
	denom, ok := s.k.rwa.TokenSettlementDenom(ctx, m.TokenId)
	if !ok {
		return nil, types.ErrRWA.Wrapf("token %d settlement denom unknown", m.TokenId)
	}
	if !s.k.settlement.HasDenom(ctx, denom) {
		return nil, types.ErrSettlement.Wrapf("settlement denom %q not found", denom)
	}
	id, err := s.k.NextOfferingID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdkCtx.BlockTime().Unix()
	o := types.Offering{
		Id:                id,
		TokenId:           m.TokenId,
		Issuer:            m.Issuer,
		Denom:             denom,
		UnitPrice:         m.UnitPrice,
		SoftCap:           m.SoftCap,
		HardCap:           m.HardCap,
		StartTime:         m.StartTime,
		EndTime:           m.EndTime,
		Status:            types.OfferingStatus_OFFERING_STATUS_OPEN,
		TotalTranches:     m.TotalTranches,
		RequiredInjection: m.RequiredInjection,
		InjectionInterval: m.InjectionInterval,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.k.setOffering(ctx, o); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeOffering, types.AttrAction, "create", types.AttrOfferingID, u(id), types.AttrIssuer, m.Issuer)
	return &types.MsgCreateOfferingResponse{OfferingId: id}, nil
}

func (s msgServer) Subscribe(ctx context.Context, m *types.MsgSubscribe) (*types.MsgSubscribeResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	o, err := s.getOffering(ctx, m.OfferingId)
	if err != nil {
		return nil, err
	}
	if o.Status != types.OfferingStatus_OFFERING_STATUS_OPEN {
		return nil, types.ErrOfferingState.Wrap("not open")
	}
	now := sdkCtx.BlockTime().Unix()
	if now < o.StartTime || now >= o.EndTime {
		return nil, types.ErrWindow.Wrapf("now=%d window=[%d,%d)", now, o.StartTime, o.EndTime)
	}
	if m.Amount%o.UnitPrice != 0 {
		return nil, types.ErrInvalidField.Wrap("amount must be a multiple of unit_price")
	}
	// Compliance gate: capital enters the treasury here and later mints a
	// (possibly KYC-gated) security, so screen the investor up front rather
	// than only at allocation time.
	if err := s.k.gateInvestor(ctx, o, m.Investor); err != nil {
		return nil, err
	}
	newRaised, err := types.SafeAdd(o.Raised, m.Amount)
	if err != nil {
		return nil, err
	}
	if newRaised > o.HardCap {
		return nil, types.ErrCap.Wrapf("hard_cap %d exceeded", o.HardCap)
	}
	// Escrow the investor's funds into the per-offering treasury.
	if err := s.k.move(ctx, o.Denom, m.Investor, TreasuryAccount(o.Id), m.Amount); err != nil {
		return nil, err
	}
	units := m.Amount / o.UnitPrice
	sub, _, err := s.k.GetSubscription(ctx, o.Id, m.Investor)
	if err != nil {
		return nil, err
	}
	sub.OfferingId = o.Id
	sub.Investor = m.Investor
	if sub.Contributed, err = types.SafeAdd(sub.Contributed, m.Amount); err != nil {
		return nil, err
	}
	if sub.Units, err = types.SafeAdd(sub.Units, units); err != nil {
		return nil, err
	}
	if err := s.k.setSubscription(ctx, sub); err != nil {
		return nil, err
	}
	o.Raised = newRaised
	o.UpdatedAt = now
	if err := s.k.setOffering(ctx, o); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "subscribe", m.Investor, u(o.Id), u(m.Amount))
	emitEvent(sdkCtx, types.EventTypeSubscription, types.AttrAction, "subscribe", types.AttrOfferingID, u(o.Id),
		types.AttrInvestor, m.Investor, types.AttrAmount, u(m.Amount), types.AttrUnits, u(units))
	return &types.MsgSubscribeResponse{Units: units}, nil
}

func (s msgServer) CancelOffering(ctx context.Context, m *types.MsgCancelOffering) (*types.MsgCancelOfferingResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	o, err := s.getOffering(ctx, m.OfferingId)
	if err != nil {
		return nil, err
	}
	if err := s.requireIssuer(o, m.Issuer); err != nil {
		return nil, err
	}
	if o.Status != types.OfferingStatus_OFFERING_STATUS_OPEN {
		return nil, types.ErrOfferingState.Wrap("only OPEN offerings can be cancelled")
	}
	o.Status = types.OfferingStatus_OFFERING_STATUS_CANCELLED
	o.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.setOffering(ctx, o); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeOffering, types.AttrAction, "cancel", types.AttrOfferingID, u(o.Id), types.AttrReason, m.Reason)
	return &types.MsgCancelOfferingResponse{}, nil
}

func (s msgServer) CloseOffering(ctx context.Context, m *types.MsgCloseOffering) (*types.MsgCloseOfferingResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	o, err := s.getOffering(ctx, m.OfferingId)
	if err != nil {
		return nil, err
	}
	if o.Status != types.OfferingStatus_OFFERING_STATUS_OPEN {
		return nil, types.ErrOfferingState.Wrap("not open")
	}
	now := sdkCtx.BlockTime().Unix()
	if now < o.EndTime && o.Raised < o.HardCap {
		return nil, types.ErrWindow.Wrap("offering still open (end_time not reached and hard_cap not met)")
	}
	if o.Raised >= o.SoftCap {
		o.Status = types.OfferingStatus_OFFERING_STATUS_SUCCEEDED
		o.SucceededAt = now
		o.AllocatedUnits = o.Raised / o.UnitPrice
	} else {
		o.Status = types.OfferingStatus_OFFERING_STATUS_FAILED
	}
	o.UpdatedAt = now
	if err := s.k.setOffering(ctx, o); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeOffering, types.AttrAction, "close", types.AttrOfferingID, u(o.Id), types.AttrStatus, o.Status.String())
	return &types.MsgCloseOfferingResponse{Status: o.Status}, nil
}

// ---- allocation -----------------------------------------------------------

func (s msgServer) ClaimAllocation(ctx context.Context, m *types.MsgClaimAllocation) (*types.MsgClaimAllocationResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	o, err := s.getOffering(ctx, m.OfferingId)
	if err != nil {
		return nil, err
	}
	if o.Status != types.OfferingStatus_OFFERING_STATUS_SUCCEEDED && o.Status != types.OfferingStatus_OFFERING_STATUS_DEFAULTED {
		return nil, types.ErrOfferingState.Wrap("allocations only after a successful raise")
	}
	if s.k.rwa == nil {
		return nil, types.ErrRWA.Wrap("rwa keeper not wired")
	}
	sub, ok, err := s.k.GetSubscription(ctx, o.Id, m.Investor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrap("no subscription")
	}
	if sub.Allocated {
		return nil, types.ErrAlreadyExists.Wrap("allocation already claimed")
	}
	// Mint the RWA units to the investor (compliance enforced by rwatoken).
	if err := s.k.rwa.AllocateUnits(ctx, o.TokenId, o.Issuer, m.Investor, sub.Units); err != nil {
		return nil, types.ErrRWA.Wrap(err.Error())
	}
	sub.Allocated = true
	if err := s.k.setSubscription(ctx, sub); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeSubscription, types.AttrAction, "allocate", types.AttrOfferingID, u(o.Id),
		types.AttrInvestor, m.Investor, types.AttrUnits, u(sub.Units))
	return &types.MsgClaimAllocationResponse{Units: sub.Units}, nil
}

// ---- servicing: inject / release / claim returns --------------------------

func (s msgServer) InjectReturn(ctx context.Context, m *types.MsgInjectReturn) (*types.MsgInjectReturnResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	o, err := s.getOffering(ctx, m.OfferingId)
	if err != nil {
		return nil, err
	}
	if err := s.requireIssuer(o, m.Issuer); err != nil {
		return nil, err
	}
	if o.Status != types.OfferingStatus_OFFERING_STATUS_SUCCEEDED {
		return nil, types.ErrOfferingState.Wrap("can only inject into a live successful offering")
	}
	if o.InjectionsDone >= o.TotalTranches {
		return nil, types.ErrOfferingState.Wrap("all injections already completed")
	}
	if m.Amount < o.RequiredInjection {
		return nil, types.ErrInvalidField.Wrapf("amount %d < required_injection %d", m.Amount, o.RequiredInjection)
	}
	// Yield flows from the issuer into the returns pool for investors.
	if err := s.k.move(ctx, o.Denom, m.Issuer, ReturnsAccount(o.Id), m.Amount); err != nil {
		return nil, err
	}
	o.InjectionsDone++
	if o.InjectedTotal, err = types.SafeAdd(o.InjectedTotal, m.Amount); err != nil {
		return nil, err
	}
	o.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.setOffering(ctx, o); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeReturn, types.AttrAction, "inject", types.AttrOfferingID, u(o.Id), types.AttrAmount, u(m.Amount))
	return &types.MsgInjectReturnResponse{}, nil
}

func (s msgServer) ReleaseTranche(ctx context.Context, m *types.MsgReleaseTranche) (*types.MsgReleaseTrancheResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	o, err := s.getOffering(ctx, m.OfferingId)
	if err != nil {
		return nil, err
	}
	if err := s.requireIssuer(o, m.Issuer); err != nil {
		return nil, err
	}
	if o.Status != types.OfferingStatus_OFFERING_STATUS_SUCCEEDED {
		return nil, types.ErrOfferingState.Wrap("tranches release only on a live successful offering")
	}
	if o.ReleasedTranches >= o.TotalTranches {
		return nil, types.ErrOfferingState.Wrap("all tranches released")
	}
	next := o.ReleasedTranches + 1
	// Gate: the issuer must have injected the yield for this tranche first.
	if o.InjectionsDone < next {
		return nil, types.ErrTrancheGate.Wrapf("inject tranche %d yield before release (injected=%d)", next, o.InjectionsDone)
	}
	amount := types.TrancheAmount(o.Raised, o.TotalTranches, next)
	if err := s.k.move(ctx, o.Denom, TreasuryAccount(o.Id), o.Issuer, amount); err != nil {
		return nil, err
	}
	o.ReleasedTranches = next
	if o.ReleasedAmount, err = types.SafeAdd(o.ReleasedAmount, amount); err != nil {
		return nil, err
	}
	o.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.setOffering(ctx, o); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeTranche, types.AttrAction, "release", types.AttrOfferingID, u(o.Id),
		types.AttrAmount, u(amount), "tranche", u(uint64(next)))
	return &types.MsgReleaseTrancheResponse{Amount: amount}, nil
}

func (s msgServer) ClaimReturns(ctx context.Context, m *types.MsgClaimReturns) (*types.MsgClaimReturnsResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	o, err := s.getOffering(ctx, m.OfferingId)
	if err != nil {
		return nil, err
	}
	if o.Status != types.OfferingStatus_OFFERING_STATUS_SUCCEEDED && o.Status != types.OfferingStatus_OFFERING_STATUS_DEFAULTED {
		return nil, types.ErrOfferingState.Wrap("no returns to claim")
	}
	sub, ok, err := s.k.GetSubscription(ctx, o.Id, m.Investor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrap("no subscription")
	}
	if err := s.k.gateRecipient(ctx, m.Investor); err != nil {
		return nil, err
	}
	claimable, err := s.k.ClaimableReturns(ctx, o, sub)
	if err != nil {
		return nil, err
	}
	if claimable == 0 {
		return nil, types.ErrNothingClaim.Wrap("no returns available")
	}
	if err := s.k.move(ctx, o.Denom, ReturnsAccount(o.Id), m.Investor, claimable); err != nil {
		return nil, err
	}
	if sub.ReturnsClaimed, err = types.SafeAdd(sub.ReturnsClaimed, claimable); err != nil {
		return nil, err
	}
	if err := s.k.setSubscription(ctx, sub); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeReturn, types.AttrAction, "claim", types.AttrOfferingID, u(o.Id),
		types.AttrInvestor, m.Investor, types.AttrAmount, u(claimable))
	return &types.MsgClaimReturnsResponse{Amount: claimable}, nil
}

// ---- default / refund -----------------------------------------------------

func (s msgServer) FlagDefault(ctx context.Context, m *types.MsgFlagDefault) (*types.MsgFlagDefaultResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	o, err := s.getOffering(ctx, m.OfferingId)
	if err != nil {
		return nil, err
	}
	if o.Status != types.OfferingStatus_OFFERING_STATUS_SUCCEEDED {
		return nil, types.ErrOfferingState.Wrap("only a live successful offering can default")
	}
	if o.InjectionsDone >= o.TotalTranches {
		return nil, types.ErrNotDelinquent.Wrap("all injections already completed")
	}
	now := sdkCtx.BlockTime().Unix()
	due := types.DueInjections(now, o.SucceededAt, o.InjectionInterval, o.TotalTranches)
	if due <= o.InjectionsDone {
		return nil, types.ErrNotDelinquent.Wrapf("due=%d injected=%d", due, o.InjectionsDone)
	}
	o.Status = types.OfferingStatus_OFFERING_STATUS_DEFAULTED
	o.DefaultTreasury = s.k.TreasuryBalance(ctx, o)
	o.UpdatedAt = now
	if err := s.k.setOffering(ctx, o); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeOffering, types.AttrAction, "default", types.AttrOfferingID, u(o.Id),
		types.AttrAmount, u(o.DefaultTreasury))
	return &types.MsgFlagDefaultResponse{}, nil
}

// ClaimRefund returns capital to investors: the full contribution for a
// FAILED/CANCELLED raise (treasury holds the entire raise), or a pro-rata
// share of the remaining treasury snapshotted at default for a DEFAULTED
// raise.
func (s msgServer) ClaimRefund(ctx context.Context, m *types.MsgClaimRefund) (*types.MsgClaimRefundResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	o, err := s.getOffering(ctx, m.OfferingId)
	if err != nil {
		return nil, err
	}
	sub, ok, err := s.k.GetSubscription(ctx, o.Id, m.Investor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrap("no subscription")
	}
	if sub.Refunded {
		return nil, types.ErrAlreadyExists.Wrap("already refunded")
	}
	if err := s.k.gateRecipient(ctx, m.Investor); err != nil {
		return nil, err
	}
	var refund uint64
	switch o.Status {
	case types.OfferingStatus_OFFERING_STATUS_FAILED, types.OfferingStatus_OFFERING_STATUS_CANCELLED:
		refund = sub.Contributed
	case types.OfferingStatus_OFFERING_STATUS_DEFAULTED:
		refund, err = types.MulDivFloor(o.DefaultTreasury, sub.Contributed, o.Raised)
		if err != nil {
			return nil, err
		}
	default:
		return nil, types.ErrOfferingState.Wrap("refunds only after failure/cancel/default")
	}
	if refund == 0 {
		// Mark settled so the row can't be retried in a loop, but there is
		// nothing to move (e.g. default with an empty remaining treasury).
		sub.Refunded = true
		if err := s.k.setSubscription(ctx, sub); err != nil {
			return nil, err
		}
		return &types.MsgClaimRefundResponse{Amount: 0}, nil
	}
	if err := s.k.move(ctx, o.Denom, TreasuryAccount(o.Id), m.Investor, refund); err != nil {
		return nil, err
	}
	sub.Refunded = true
	if err := s.k.setSubscription(ctx, sub); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeRefund, types.AttrAction, "refund", types.AttrOfferingID, u(o.Id),
		types.AttrInvestor, m.Investor, types.AttrAmount, u(refund))
	return &types.MsgClaimRefundResponse{Amount: refund}, nil
}

// ---- params ---------------------------------------------------------------

func (s msgServer) UpdateParams(ctx context.Context, m *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if err := s.k.SetParams(ctx, m.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
