package keeper

import (
	"context"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/stableusd/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{k: k} }

var _ types.MsgServer = msgServer{}

func (s msgServer) requireAuthority(addr string) error {
	if addr != s.k.authority {
		return types.ErrUnauthorized.Wrapf("expected authority %q", s.k.authority)
	}
	return nil
}

// requireAdmin loads the denom and asserts the caller is its admin or the
// governance authority. Governance is always allowed as a break-glass.
func (s msgServer) requireAdmin(ctx context.Context, denomID, caller string) (types.StableDenom, error) {
	d, ok := s.k.GetDenom(ctx, denomID)
	if !ok {
		return types.StableDenom{}, types.ErrNotFound.Wrapf("denom %q", denomID)
	}
	if caller != d.Admin && caller != s.k.authority {
		return types.StableDenom{}, types.ErrUnauthorized.Wrap("not denom admin")
	}
	return d, nil
}

// ---- denom lifecycle ------------------------------------------------------

func (s msgServer) CreateDenom(ctx context.Context, m *types.MsgCreateDenom) (*types.MsgCreateDenomResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if s.k.HasDenom(ctx, m.Id) {
		return nil, types.ErrAlreadyExists.Wrapf("denom %q", m.Id)
	}
	params := s.k.GetParams(ctx)
	n, err := s.k.CountDenoms(ctx)
	if err != nil {
		return nil, err
	}
	if n >= uint64(params.MaxDenoms) {
		return nil, types.ErrLimitExceeded.Wrap("max_denoms")
	}
	now := sdkCtx.BlockTime().Unix()
	d := types.StableDenom{
		Id:          m.Id,
		Symbol:      m.Symbol,
		Decimals:    m.Decimals,
		PegCurrency: m.PegCurrency,
		Admin:       m.Admin,
		Status:      types.DenomStatus_DENOM_STATUS_ACTIVE,
		PolicyId:    m.PolicyId,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.k.Denoms.Set(ctx, m.Id, d); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "create_denom", m.Authority, m.Id, m.Symbol)
	emitEvent(sdkCtx, types.EventTypeDenom, types.AttrAction, "create", types.AttrDenom, m.Id)
	return &types.MsgCreateDenomResponse{}, nil
}

func (s msgServer) AddMinter(ctx context.Context, m *types.MsgAddMinter) (*types.MsgAddMinterResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	d, err := s.requireAdmin(ctx, m.DenomId, m.Admin)
	if err != nil {
		return nil, err
	}
	if s.k.isMinter(d, m.Minter) {
		return nil, types.ErrAlreadyExists.Wrap("minter already authorized")
	}
	if uint32(len(d.Minters)) >= s.k.GetParams(ctx).MaxMintersPerDenom {
		return nil, types.ErrLimitExceeded.Wrap("max_minters_per_denom")
	}
	d.Minters = append(d.Minters, m.Minter)
	d.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Denoms.Set(ctx, d.Id, d); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "add_minter", m.Admin, m.DenomId, m.Minter)
	emitEvent(sdkCtx, types.EventTypeDenom, types.AttrAction, "add_minter", types.AttrDenom, m.DenomId, types.AttrAccount, m.Minter)
	return &types.MsgAddMinterResponse{}, nil
}

func (s msgServer) RemoveMinter(ctx context.Context, m *types.MsgRemoveMinter) (*types.MsgRemoveMinterResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	d, err := s.requireAdmin(ctx, m.DenomId, m.Admin)
	if err != nil {
		return nil, err
	}
	idx := -1
	for i, mnt := range d.Minters {
		if mnt == m.Minter {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, types.ErrNotFound.Wrap("minter not authorized")
	}
	d.Minters = append(d.Minters[:idx], d.Minters[idx+1:]...)
	d.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Denoms.Set(ctx, d.Id, d); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "remove_minter", m.Admin, m.DenomId, m.Minter)
	emitEvent(sdkCtx, types.EventTypeDenom, types.AttrAction, "remove_minter", types.AttrDenom, m.DenomId, types.AttrAccount, m.Minter)
	return &types.MsgRemoveMinterResponse{}, nil
}

func (s msgServer) SetDenomStatus(ctx context.Context, m *types.MsgSetDenomStatus) (*types.MsgSetDenomStatusResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	d, err := s.requireAdmin(ctx, m.DenomId, m.Admin)
	if err != nil {
		return nil, err
	}
	if d.Status == types.DenomStatus_DENOM_STATUS_RETIRED {
		return nil, types.ErrDenomState.Wrap("retired is terminal")
	}
	d.Status = m.Status
	d.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Denoms.Set(ctx, d.Id, d); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "set_status", m.Admin, m.DenomId, m.Status.String())
	emitEvent(sdkCtx, types.EventTypeDenom, types.AttrAction, "set_status", types.AttrDenom, m.DenomId)
	return &types.MsgSetDenomStatusResponse{}, nil
}

func (s msgServer) BindReserve(ctx context.Context, m *types.MsgBindReserve) (*types.MsgBindReserveResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	d, err := s.requireAdmin(ctx, m.DenomId, m.Admin)
	if err != nil {
		return nil, err
	}
	d.ReserveTopic = m.ReserveTopic
	d.RequiredRatioBps = m.RequiredRatioBps
	d.MaxStalenessSeconds = m.MaxStalenessSeconds
	d.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Denoms.Set(ctx, d.Id, d); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "bind_reserve", m.Admin, m.DenomId, m.ReserveTopic)
	emitEvent(sdkCtx, types.EventTypeDenom, types.AttrAction, "bind_reserve", types.AttrDenom, m.DenomId)
	return &types.MsgBindReserveResponse{}, nil
}

// ---- supply ----------------------------------------------------------------

func (s msgServer) Mint(ctx context.Context, m *types.MsgMint) (*types.MsgMintResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	d, ok := s.k.GetDenom(ctx, m.DenomId)
	if !ok {
		return nil, types.ErrNotFound.Wrapf("denom %q", m.DenomId)
	}
	if !s.k.isMinter(d, m.Minter) {
		return nil, types.ErrUnauthorized.Wrap("not an authorized minter")
	}
	// The minter itself must not be sanctioned: ComplianceCheck only sees
	// the (empty) sender and the recipient on a mint, so the signer is
	// gated here explicitly to stop a sanctioned-but-still-listed minter
	// from issuing new supply.
	if s.k.compliance != nil && s.k.compliance.IsSanctioned(sdkCtx, m.Minter) {
		return nil, types.ErrCompliance.Wrap("minter sanctioned")
	}
	// Recipient must clear compliance (no sender on a mint).
	if err := s.k.ComplianceCheck(sdkCtx, m.DenomId, "", m.Recipient, m.Amount, false); err != nil {
		return nil, err
	}
	newOutstanding, err := types.SafeAdd(s.k.GetSupply(ctx, m.DenomId), m.Amount)
	if err != nil {
		return nil, err
	}
	if err := s.k.CheckReserveCoverage(sdkCtx, d, newOutstanding); err != nil {
		return nil, err
	}
	if err := s.k.credit(ctx, m.DenomId, m.Recipient, m.Amount); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "mint", m.Minter, m.Recipient, strconv.FormatUint(m.Amount, 10))
	emitEvent(sdkCtx, types.EventTypeSupply, types.AttrAction, "mint", types.AttrDenom, m.DenomId,
		types.AttrTo, m.Recipient, types.AttrAmount, strconv.FormatUint(m.Amount, 10))
	return &types.MsgMintResponse{}, nil
}

func (s msgServer) Burn(ctx context.Context, m *types.MsgBurn) (*types.MsgBurnResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	// Burning one's own balance is allowed even on a PAUSED denom (wind
	// down), but blacklisted/frozen holders cannot move value out.
	if err := s.k.ComplianceCheck(sdkCtx, m.DenomId, m.Holder, "", m.Amount, true); err != nil {
		return nil, err
	}
	if err := s.k.debit(ctx, m.DenomId, m.Holder, m.Amount); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "burn", m.Holder, m.Holder, strconv.FormatUint(m.Amount, 10))
	emitEvent(sdkCtx, types.EventTypeSupply, types.AttrAction, "burn", types.AttrDenom, m.DenomId,
		types.AttrFrom, m.Holder, types.AttrAmount, strconv.FormatUint(m.Amount, 10))
	return &types.MsgBurnResponse{}, nil
}

// ---- transfers -------------------------------------------------------------

func (s msgServer) Transfer(ctx context.Context, m *types.MsgTransfer) (*types.MsgTransferResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := s.k.ComplianceCheck(sdkCtx, m.DenomId, m.From, m.To, m.Amount, false); err != nil {
		return nil, err
	}
	if err := s.k.move(ctx, m.DenomId, m.From, m.To, m.Amount); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeTransfer, types.AttrAction, "transfer", types.AttrDenom, m.DenomId,
		types.AttrFrom, m.From, types.AttrTo, m.To, types.AttrAmount, strconv.FormatUint(m.Amount, 10))
	return &types.MsgTransferResponse{}, nil
}

func (s msgServer) Approve(ctx context.Context, m *types.MsgApprove) (*types.MsgApproveResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if !s.k.HasDenom(ctx, m.DenomId) {
		return nil, types.ErrNotFound.Wrapf("denom %q", m.DenomId)
	}
	if err := s.k.setAllowance(ctx, m.DenomId, m.Owner, m.Spender, m.Amount); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeTransfer, types.AttrAction, "approve", types.AttrDenom, m.DenomId,
		types.AttrFrom, m.Owner, types.AttrTo, m.Spender, types.AttrAmount, strconv.FormatUint(m.Amount, 10))
	return &types.MsgApproveResponse{}, nil
}

func (s msgServer) TransferFrom(ctx context.Context, m *types.MsgTransferFrom) (*types.MsgTransferFromResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	// Compliance is enforced against the value owner and recipient; the
	// spender is only an operator and is not a value counterparty.
	if err := s.k.ComplianceCheck(sdkCtx, m.DenomId, m.From, m.To, m.Amount, false); err != nil {
		return nil, err
	}
	allow := s.k.GetAllowance(ctx, m.DenomId, m.From, m.Spender)
	if allow < m.Amount {
		return nil, types.ErrInsufficientAllow.Wrapf("have %d need %d", allow, m.Amount)
	}
	if err := s.k.move(ctx, m.DenomId, m.From, m.To, m.Amount); err != nil {
		return nil, err
	}
	if err := s.k.setAllowance(ctx, m.DenomId, m.From, m.Spender, allow-m.Amount); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeTransfer, types.AttrAction, "transfer_from", types.AttrDenom, m.DenomId,
		types.AttrFrom, m.From, types.AttrTo, m.To, types.AttrAmount, strconv.FormatUint(m.Amount, 10))
	return &types.MsgTransferFromResponse{}, nil
}

// ---- compliance controls ---------------------------------------------------

func (s msgServer) setFlag(ctx context.Context, denomID, admin, account string, frozen, blacklisted *bool, action string) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if _, err := s.requireAdmin(ctx, denomID, admin); err != nil {
		return err
	}
	f := s.k.GetFlags(ctx, denomID, account)
	f.DenomId = denomID
	f.Account = account
	if frozen != nil {
		f.Frozen = *frozen
	}
	if blacklisted != nil {
		f.Blacklisted = *blacklisted
	}
	if err := s.k.setFlags(ctx, f); err != nil {
		return err
	}
	s.k.recordAudit(sdkCtx, action, admin, account, denomID)
	emitEvent(sdkCtx, types.EventTypeCompliance, types.AttrAction, action, types.AttrDenom, denomID, types.AttrAccount, account)
	return nil
}

func (s msgServer) Freeze(ctx context.Context, m *types.MsgFreeze) (*types.MsgFreezeResponse, error) {
	tru := true
	if err := s.setFlag(ctx, m.DenomId, m.Admin, m.Account, &tru, nil, "freeze"); err != nil {
		return nil, err
	}
	return &types.MsgFreezeResponse{}, nil
}

func (s msgServer) Unfreeze(ctx context.Context, m *types.MsgUnfreeze) (*types.MsgUnfreezeResponse, error) {
	fal := false
	if err := s.setFlag(ctx, m.DenomId, m.Admin, m.Account, &fal, nil, "unfreeze"); err != nil {
		return nil, err
	}
	return &types.MsgUnfreezeResponse{}, nil
}

func (s msgServer) Blacklist(ctx context.Context, m *types.MsgBlacklist) (*types.MsgBlacklistResponse, error) {
	tru := true
	if err := s.setFlag(ctx, m.DenomId, m.Admin, m.Account, nil, &tru, "blacklist"); err != nil {
		return nil, err
	}
	return &types.MsgBlacklistResponse{}, nil
}

func (s msgServer) Unblacklist(ctx context.Context, m *types.MsgUnblacklist) (*types.MsgUnblacklistResponse, error) {
	fal := false
	if err := s.setFlag(ctx, m.DenomId, m.Admin, m.Account, nil, &fal, "unblacklist"); err != nil {
		return nil, err
	}
	return &types.MsgUnblacklistResponse{}, nil
}

// ForceTransfer is the regulatory seizure path: it moves funds out of a
// (typically frozen/blacklisted) holder regardless of that holder's
// flags. The destination must still not be sanctioned or blacklisted.
func (s msgServer) ForceTransfer(ctx context.Context, m *types.MsgForceTransfer) (*types.MsgForceTransferResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if _, err := s.requireAdmin(ctx, m.DenomId, m.Admin); err != nil {
		return nil, err
	}
	// Destination-side guard only (source is being seized).
	if s.k.IsAccountBlocked(ctx, m.DenomId, m.To) {
		return nil, types.ErrFrozen.Wrap("destination blocked")
	}
	if s.k.compliance != nil && s.k.compliance.IsSanctioned(sdkCtx, m.To) {
		return nil, types.ErrCompliance.Wrap("destination sanctioned")
	}
	if err := s.k.move(ctx, m.DenomId, m.From, m.To, m.Amount); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "force_transfer", m.Admin, m.From, m.Reason)
	emitEvent(sdkCtx, types.EventTypeCompliance, types.AttrAction, "force_transfer", types.AttrDenom, m.DenomId,
		types.AttrFrom, m.From, types.AttrTo, m.To, types.AttrReason, m.Reason)
	return &types.MsgForceTransferResponse{}, nil
}

// ---- redemption ------------------------------------------------------------

func (s msgServer) RequestRedemption(ctx context.Context, m *types.MsgRequestRedemption) (*types.MsgRequestRedemptionResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	// Holder must clear compliance; tokens are burned on request so the
	// redeemed amount cannot be re-spent while the fiat leg settles.
	if err := s.k.ComplianceCheck(sdkCtx, m.DenomId, m.Holder, "", m.Amount, false); err != nil {
		return nil, err
	}
	pending, err := s.k.countPendingRedemptions(ctx, m.Holder)
	if err != nil {
		return nil, err
	}
	if pending >= s.k.GetParams(ctx).MaxPendingRedemptionsPerHolder {
		return nil, types.ErrLimitExceeded.Wrap("max_pending_redemptions_per_holder")
	}
	// Move the tokens into the module escrow rather than burning them: the
	// supply is unchanged while the fiat leg settles, so a later cancel
	// returns the exact escrowed amount and can never inflate supply.
	if err := s.k.move(ctx, m.DenomId, m.Holder, types.RedemptionEscrow, m.Amount); err != nil {
		return nil, err
	}
	seq, err := s.k.RedemptionIDSeq.Next(ctx)
	if err != nil {
		return nil, err
	}
	id := seq + 1
	r := types.Redemption{
		Id:        id,
		DenomId:   m.DenomId,
		Holder:    m.Holder,
		Amount:    m.Amount,
		Status:    types.RedemptionStatus_REDEMPTION_STATUS_PENDING,
		Memo:      m.Memo,
		CreatedAt: sdkCtx.BlockTime().Unix(),
	}
	if err := s.k.setRedemption(ctx, r); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "request_redemption", m.Holder, m.DenomId, strconv.FormatUint(m.Amount, 10))
	emitEvent(sdkCtx, types.EventTypeRedemption, types.AttrAction, "request", types.AttrDenom, m.DenomId,
		types.AttrAccount, m.Holder, types.AttrRedeemID, strconv.FormatUint(id, 10))
	return &types.MsgRequestRedemptionResponse{RedemptionId: id}, nil
}

func (s msgServer) SettleRedemption(ctx context.Context, m *types.MsgSettleRedemption) (*types.MsgSettleRedemptionResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	r, ok := s.k.GetRedemption(ctx, m.RedemptionId)
	if !ok {
		return nil, types.ErrNotFound.Wrapf("redemption %d", m.RedemptionId)
	}
	if _, err := s.requireAdmin(ctx, r.DenomId, m.Admin); err != nil {
		return nil, err
	}
	if r.Status != types.RedemptionStatus_REDEMPTION_STATUS_PENDING {
		return nil, types.ErrRedemptionState.Wrap("not pending")
	}
	// Burn the escrowed tokens now that the off-chain fiat payout is done.
	if err := s.k.debit(ctx, r.DenomId, types.RedemptionEscrow, r.Amount); err != nil {
		return nil, err
	}
	r.Status = types.RedemptionStatus_REDEMPTION_STATUS_SETTLED
	r.ResolvedAt = sdkCtx.BlockTime().Unix()
	if m.Memo != "" {
		r.Memo = m.Memo
	}
	if err := s.k.setRedemption(ctx, r); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "settle_redemption", m.Admin, r.Holder, strconv.FormatUint(r.Id, 10))
	emitEvent(sdkCtx, types.EventTypeRedemption, types.AttrAction, "settle", types.AttrRedeemID, strconv.FormatUint(r.Id, 10))
	return &types.MsgSettleRedemptionResponse{}, nil
}

// CancelRedemption returns the escrowed tokens to the holder. Because the
// tokens were only moved (not burned), this is a supply-neutral escrow
// release and can never inflate supply. Refused if the holder became
// blacklisted/frozen meanwhile.
func (s msgServer) CancelRedemption(ctx context.Context, m *types.MsgCancelRedemption) (*types.MsgCancelRedemptionResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	r, ok := s.k.GetRedemption(ctx, m.RedemptionId)
	if !ok {
		return nil, types.ErrNotFound.Wrapf("redemption %d", m.RedemptionId)
	}
	if _, err := s.requireAdmin(ctx, r.DenomId, m.Admin); err != nil {
		return nil, err
	}
	if r.Status != types.RedemptionStatus_REDEMPTION_STATUS_PENDING {
		return nil, types.ErrRedemptionState.Wrap("not pending")
	}
	if s.k.IsAccountBlocked(ctx, r.DenomId, r.Holder) {
		return nil, types.ErrFrozen.Wrap("holder blocked; cannot refund")
	}
	if err := s.k.move(ctx, r.DenomId, types.RedemptionEscrow, r.Holder, r.Amount); err != nil {
		return nil, err
	}
	r.Status = types.RedemptionStatus_REDEMPTION_STATUS_CANCELLED
	r.ResolvedAt = sdkCtx.BlockTime().Unix()
	if m.Reason != "" {
		r.Memo = m.Reason
	}
	if err := s.k.setRedemption(ctx, r); err != nil {
		return nil, err
	}
	s.k.recordAudit(sdkCtx, "cancel_redemption", m.Admin, r.Holder, m.Reason)
	emitEvent(sdkCtx, types.EventTypeRedemption, types.AttrAction, "cancel", types.AttrRedeemID, strconv.FormatUint(r.Id, 10))
	return &types.MsgCancelRedemptionResponse{}, nil
}

// ---- params ----------------------------------------------------------------

func (s msgServer) UpdateParams(ctx context.Context, m *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if err := s.k.SetParams(ctx, m.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
