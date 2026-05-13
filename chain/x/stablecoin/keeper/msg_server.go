package keeper

import (
	"context"
	"fmt"
	"strconv"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"energychain/x/stablecoin/types"
)

type msgServer struct{ Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return &msgServer{Keeper: k} }

var _ types.MsgServer = msgServer{}

func (m msgServer) onlyAuthority(authority string) error {
	if authority != m.GetAuthority() {
		return sdkerrors.ErrUnauthorized.Wrapf("expected authority %s", m.GetAuthority())
	}
	return nil
}

// requireIssuerAdmin gates every compliance / force / mint-pause op.
// Returns (issuer, error) so the caller can use the loaded issuer
// without an extra lookup.
func (m msgServer) requireIssuerAdmin(ctx sdk.Context, issuerID, admin string) (types.Issuer, error) {
	is, ok := m.GetIssuer(ctx, issuerID)
	if !ok {
		return types.Issuer{}, sdkerrors.ErrNotFound.Wrapf("issuer %s", issuerID)
	}
	if is.Admin != admin {
		return types.Issuer{}, sdkerrors.ErrUnauthorized.Wrapf("not issuer admin (expected %s)", is.Admin)
	}
	return is, nil
}

// ---------------------------------------------------------------------------
// Denom lifecycle
// ---------------------------------------------------------------------------

func (m msgServer) RegisterDenom(goCtx context.Context, msg *types.MsgRegisterDenom) (*types.MsgRegisterDenomResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if m.HasDenom(ctx, msg.Id) {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("denom %s already exists", msg.Id)
	}
	params := m.GetParams(ctx)
	if m.CountDenoms(ctx) >= params.MaxDenoms {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("max_denoms cap %d reached", params.MaxDenoms)
	}
	now := ctx.BlockTime().Unix()
	d := types.Denom{
		Id:           msg.Id,
		Symbol:       msg.Symbol,
		Name:         msg.Name,
		Decimals:     msg.Decimals,
		Jurisdiction: msg.Jurisdiction,
		Status:       types.DenomStatus_DENOM_STATUS_ACTIVE,
		PolicyId:     msg.PolicyId,
		CreatedBy:    msg.Authority,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := m.SetDenom(ctx, d); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "stablecoin_denom_registered", "denom_id", d.Id, "symbol", d.Symbol)
	return &types.MsgRegisterDenomResponse{}, nil
}

func (m msgServer) UpdateDenom(goCtx context.Context, msg *types.MsgUpdateDenom) (*types.MsgUpdateDenomResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	d, ok := m.GetDenom(ctx, msg.Id)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("denom %s", msg.Id)
	}
	// SECURITY: RETIRED denoms are immutable. Operators must re-register
	// under a new id rather than resurrect a retired one (which would
	// silently re-enable transfers that holders thought were dead).
	if d.Status == types.DenomStatus_DENOM_STATUS_RETIRED {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("cannot update RETIRED denom")
	}
	if msg.Symbol != "" {
		d.Symbol = msg.Symbol
	}
	if msg.Name != "" {
		d.Name = msg.Name
	}
	if msg.Jurisdiction != "" {
		d.Jurisdiction = msg.Jurisdiction
	}
	// PolicyId is a binding pointer; the empty string is a valid value
	// meaning "unbind". We treat any presence of the field as
	// authoritative — operators set it to "" deliberately.
	d.PolicyId = msg.PolicyId
	d.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.SetDenom(ctx, d); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "stablecoin_denom_updated", "denom_id", d.Id)
	return &types.MsgUpdateDenomResponse{}, nil
}

func (m msgServer) PauseDenom(goCtx context.Context, msg *types.MsgPauseDenom) (*types.MsgPauseDenomResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := m.setDenomStatus(ctx, msg.Id, msg.Reason, types.DenomStatus_DENOM_STATUS_PAUSED, "stablecoin_denom_paused"); err != nil {
		return nil, err
	}
	return &types.MsgPauseDenomResponse{}, nil
}

func (m msgServer) ResumeDenom(goCtx context.Context, msg *types.MsgResumeDenom) (*types.MsgResumeDenomResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := m.setDenomStatus(ctx, msg.Id, msg.Reason, types.DenomStatus_DENOM_STATUS_ACTIVE, "stablecoin_denom_resumed"); err != nil {
		return nil, err
	}
	return &types.MsgResumeDenomResponse{}, nil
}

func (m msgServer) RetireDenom(goCtx context.Context, msg *types.MsgRetireDenom) (*types.MsgRetireDenomResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	// SECURITY: refuse retire while any supply remains. A holder
	// stuck with a "dead" denom would have no way to redeem.
	if m.GetSupply(ctx, msg.Id) > 0 {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("denom %s has non-zero supply; redeem before retire", msg.Id)
	}
	if err := m.setDenomStatus(ctx, msg.Id, msg.Reason, types.DenomStatus_DENOM_STATUS_RETIRED, "stablecoin_denom_retired"); err != nil {
		return nil, err
	}
	return &types.MsgRetireDenomResponse{}, nil
}

// setDenomStatus is the shared body of the three lifecycle ops above.
func (k Keeper) setDenomStatus(
	ctx sdk.Context, id, reason string, status types.DenomStatus, ev string,
) error {
	d, ok := k.GetDenom(ctx, id)
	if !ok {
		return sdkerrors.ErrNotFound.Wrapf("denom %s", id)
	}
	if d.Status == types.DenomStatus_DENOM_STATUS_RETIRED && status != types.DenomStatus_DENOM_STATUS_RETIRED {
		return sdkerrors.ErrInvalidRequest.Wrap("cannot leave RETIRED state")
	}
	d.Status = status
	d.UpdatedAt = ctx.BlockTime().Unix()
	if err := k.SetDenom(ctx, d); err != nil {
		return sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, ev, "denom_id", d.Id, "reason", reason)
	return nil
}

// ---------------------------------------------------------------------------
// Issuer lifecycle
// ---------------------------------------------------------------------------

func (m msgServer) RegisterIssuer(goCtx context.Context, msg *types.MsgRegisterIssuer) (*types.MsgRegisterIssuerResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if _, ok := m.GetIssuer(ctx, msg.Id); ok {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("issuer %s already exists", msg.Id)
	}
	params := m.GetParams(ctx)
	if m.CountIssuers(ctx) >= params.MaxIssuers {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("max_issuers cap %d reached", params.MaxIssuers)
	}
	if uint32(len(msg.MintAuthorities)) > params.MaxMintAuthoritiesPerIssuer {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("too many mint authorities (max %d)",
			params.MaxMintAuthoritiesPerIssuer)
	}
	now := ctx.BlockTime().Unix()
	is := types.Issuer{
		Id:              msg.Id,
		Did:             msg.Did,
		DisplayName:     msg.DisplayName,
		Status:          types.IssuerStatus_ISSUER_STATUS_ACTIVE,
		MintAuthorities: append([]string(nil), msg.MintAuthorities...),
		Admin:           msg.Admin,
		CreatedBy:       msg.Authority,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := m.SetIssuer(ctx, is); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "stablecoin_issuer_registered", "issuer_id", is.Id, "did", is.Did)
	return &types.MsgRegisterIssuerResponse{}, nil
}

func (m msgServer) UpdateIssuer(goCtx context.Context, msg *types.MsgUpdateIssuer) (*types.MsgUpdateIssuerResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	is, ok := m.GetIssuer(ctx, msg.Id)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("issuer %s", msg.Id)
	}
	if is.Status == types.IssuerStatus_ISSUER_STATUS_REVOKED {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("cannot update REVOKED issuer")
	}
	if msg.DisplayName != "" {
		is.DisplayName = msg.DisplayName
	}
	if len(msg.MintAuthorities) > 0 {
		params := m.GetParams(ctx)
		if uint32(len(msg.MintAuthorities)) > params.MaxMintAuthoritiesPerIssuer {
			return nil, sdkerrors.ErrInvalidRequest.Wrapf("too many mint authorities (max %d)",
				params.MaxMintAuthoritiesPerIssuer)
		}
		is.MintAuthorities = append([]string(nil), msg.MintAuthorities...)
	}
	if msg.Admin != "" {
		is.Admin = msg.Admin
	}
	is.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.SetIssuer(ctx, is); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "stablecoin_issuer_updated", "issuer_id", is.Id)
	return &types.MsgUpdateIssuerResponse{}, nil
}

func (m msgServer) SuspendIssuer(goCtx context.Context, msg *types.MsgSuspendIssuer) (*types.MsgSuspendIssuerResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := m.setIssuerStatus(ctx, msg.Id, msg.Reason, types.IssuerStatus_ISSUER_STATUS_SUSPENDED, "stablecoin_issuer_suspended"); err != nil {
		return nil, err
	}
	return &types.MsgSuspendIssuerResponse{}, nil
}

func (m msgServer) RevokeIssuer(goCtx context.Context, msg *types.MsgRevokeIssuer) (*types.MsgRevokeIssuerResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := m.setIssuerStatus(ctx, msg.Id, msg.Reason, types.IssuerStatus_ISSUER_STATUS_REVOKED, "stablecoin_issuer_revoked"); err != nil {
		return nil, err
	}
	return &types.MsgRevokeIssuerResponse{}, nil
}

func (k Keeper) setIssuerStatus(
	ctx sdk.Context, id, reason string, status types.IssuerStatus, ev string,
) error {
	is, ok := k.GetIssuer(ctx, id)
	if !ok {
		return sdkerrors.ErrNotFound.Wrapf("issuer %s", id)
	}
	if is.Status == types.IssuerStatus_ISSUER_STATUS_REVOKED && status != types.IssuerStatus_ISSUER_STATUS_REVOKED {
		return sdkerrors.ErrInvalidRequest.Wrap("cannot leave REVOKED state")
	}
	is.Status = status
	is.UpdatedAt = ctx.BlockTime().Unix()
	if err := k.SetIssuer(ctx, is); err != nil {
		return sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, ev, "issuer_id", is.Id, "reason", reason)
	return nil
}

// ---------------------------------------------------------------------------
// Quotas + reserves
// ---------------------------------------------------------------------------

func (m msgServer) SetMintQuota(goCtx context.Context, msg *types.MsgSetMintQuota) (*types.MsgSetMintQuotaResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if _, ok := m.GetIssuer(ctx, msg.IssuerId); !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("issuer %s", msg.IssuerId)
	}
	if !m.HasDenom(ctx, msg.DenomId) {
		return nil, sdkerrors.ErrNotFound.Wrapf("denom %s", msg.DenomId)
	}
	q, ok := m.GetQuota(ctx, msg.IssuerId, msg.DenomId)
	if !ok {
		q = types.MintQuota{
			IssuerId:    msg.IssuerId,
			DenomId:     msg.DenomId,
			Outstanding: 0,
		}
	}
	// SECURITY: refuse to lower the ceiling below the current outstanding.
	// Otherwise issuers would silently exceed their cap and the
	// invariant outstanding <= ceiling would be broken until burns
	// caught up.
	if msg.Ceiling < q.Outstanding {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"new ceiling %d below current outstanding %d", msg.Ceiling, q.Outstanding)
	}
	q.Ceiling = msg.Ceiling
	q.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.SetQuota(ctx, q); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "stablecoin_quota_set",
		"issuer_id", msg.IssuerId, "denom_id", msg.DenomId, "ceiling", strconv.FormatUint(msg.Ceiling, 10))
	return &types.MsgSetMintQuotaResponse{}, nil
}

func (m msgServer) SetMintPaused(goCtx context.Context, msg *types.MsgSetMintPaused) (*types.MsgSetMintPausedResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if _, err := m.requireIssuerAdmin(ctx, msg.IssuerId, msg.Admin); err != nil {
		return nil, err
	}
	q, ok := m.GetQuota(ctx, msg.IssuerId, msg.DenomId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("quota (%s,%s)", msg.IssuerId, msg.DenomId)
	}
	q.MintPaused = msg.Paused
	q.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.SetQuota(ctx, q); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "stablecoin_quota_paused",
		"issuer_id", msg.IssuerId, "denom_id", msg.DenomId, "paused", boolStr(msg.Paused), "reason", msg.Reason)
	return &types.MsgSetMintPausedResponse{}, nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func (m msgServer) SetReserveRequirement(goCtx context.Context, msg *types.MsgSetReserveRequirement) (*types.MsgSetReserveRequirementResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if !m.HasDenom(ctx, msg.DenomId) {
		return nil, sdkerrors.ErrNotFound.Wrapf("denom %s", msg.DenomId)
	}
	r := types.DenomReserve{
		DenomId:             msg.DenomId,
		OracleTopicId:       msg.OracleTopicId,
		RequiredRatioBps:    msg.RequiredRatioBps,
		MaxStalenessSeconds: msg.MaxStalenessSeconds,
		UpdatedAt:           ctx.BlockTime().Unix(),
	}
	if err := m.SetReserve(ctx, r); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "stablecoin_reserve_set",
		"denom_id", msg.DenomId, "topic", msg.OracleTopicId, "ratio_bps", strconv.FormatUint(uint64(msg.RequiredRatioBps), 10))
	return &types.MsgSetReserveRequirementResponse{}, nil
}

// ---------------------------------------------------------------------------
// Hot path: Mint / Burn / Transfer / Approve / TransferFrom
// ---------------------------------------------------------------------------

func (m msgServer) Mint(goCtx context.Context, msg *types.MsgMint) (*types.MsgMintResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	is, ok := m.GetIssuer(ctx, msg.IssuerId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("issuer %s", msg.IssuerId)
	}
	if is.Status != types.IssuerStatus_ISSUER_STATUS_ACTIVE {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("issuer %s status %s", msg.IssuerId, is.Status)
	}
	if !contains(is.MintAuthorities, msg.Minter) {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("minter %s not in issuer mint authorities", msg.Minter)
	}
	q, ok := m.GetQuota(ctx, msg.IssuerId, msg.DenomId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("quota (%s,%s)", msg.IssuerId, msg.DenomId)
	}
	if q.MintPaused {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("issuer-paused mint for (%s,%s)", msg.IssuerId, msg.DenomId)
	}
	newOutstanding, err := types.SafeAdd(q.Outstanding, msg.Amount)
	if err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("outstanding overflow: %s", err)
	}
	if newOutstanding > q.Ceiling {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"mint exceeds ceiling: new outstanding %d > ceiling %d", newOutstanding, q.Ceiling)
	}

	// Reserve check uses the proposed post-mint outstanding so the
	// invariant holds atomically across the mint.
	if err := m.CheckMintReserveCoverage(ctx, msg.DenomId, newOutstanding); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("reserve: %s", err)
	}

	// Compliance: no sender (mint comes from the void), only receiver.
	if err := m.ComplianceCheck(ctx, msg.DenomId, "", msg.Recipient, msg.Amount, false); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("compliance: %s", err)
	}

	if err := m.creditBalance(ctx, msg.DenomId, msg.Recipient, msg.Amount); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("credit: %s", err)
	}
	q.Outstanding = newOutstanding
	q.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.SetQuota(ctx, q); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set quota: %s", err)
	}
	emit(ctx, "stablecoin_minted",
		"issuer_id", msg.IssuerId, "denom_id", msg.DenomId, "minter", msg.Minter,
		"recipient", msg.Recipient, "amount", strconv.FormatUint(msg.Amount, 10))
	return &types.MsgMintResponse{NewOutstanding: newOutstanding}, nil
}

func (m msgServer) Burn(goCtx context.Context, msg *types.MsgBurn) (*types.MsgBurnResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if !m.HasDenom(ctx, msg.DenomId) {
		return nil, sdkerrors.ErrNotFound.Wrapf("denom %s", msg.DenomId)
	}
	// Burn allows PAUSED so holders can wind down a paused denom; the
	// blacklist gate still fires because a blacklisted holder must
	// not be able to silently destroy tokens (would lose the audit
	// trail). Frozen accounts also cannot burn — operators unfreeze
	// first if intentional drain is needed.
	f := m.GetFlags(ctx, msg.DenomId, msg.Holder)
	if f.Frozen {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("holder %s frozen", msg.Holder)
	}
	if f.Blacklisted {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("holder %s blacklisted", msg.Holder)
	}
	if err := m.debitBalance(ctx, msg.DenomId, msg.Holder, msg.Amount); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("debit: %s", err)
	}
	// Burn shrinks every quota row for this denom proportionally — but
	// because we don't track which issuer minted which token, we apply
	// the burn to the holder's "issuer of record" if any. Fallback:
	// shrink the largest outstanding quota first, deterministically by
	// issuer id sort order.
	if err := m.shrinkQuotaOnBurn(ctx, msg.DenomId, msg.Amount); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("shrink quota: %s", err)
	}
	emit(ctx, "stablecoin_burned",
		"denom_id", msg.DenomId, "holder", msg.Holder,
		"amount", strconv.FormatUint(msg.Amount, 10))
	return &types.MsgBurnResponse{NewTotalSupply: m.GetSupply(ctx, msg.DenomId)}, nil
}

// shrinkQuotaOnBurn distributes a burn amount across the issuers'
// outstanding rows for a denom. Walks issuers in deterministic id
// order and subtracts greedily from the largest outstanding first.
//
// SECURITY: this is intentionally NOT pro-rata — pro-rata math would
// introduce rounding noise and the resulting tiny "dust" could break
// the supply == sum(outstanding) invariant we rely on for reserve
// proofs. Greedy single-issuer drain keeps the math exact.
func (k Keeper) shrinkQuotaOnBurn(ctx sdk.Context, denomID string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	type row struct {
		issuerID string
		out      uint64
	}
	var quotas []row
	rng := collections.NewPrefixedPairRange[string, string](denomID)
	if err := k.QuotaByDenom.Walk(ctx, rng, func(key collections.Pair[string, string]) (bool, error) {
		issuerID := key.K2()
		q, ok := k.GetQuota(ctx, issuerID, denomID)
		if !ok {
			return false, nil
		}
		if q.Outstanding > 0 {
			quotas = append(quotas, row{issuerID: issuerID, out: q.Outstanding})
		}
		return false, nil
	}); err != nil {
		return err
	}
	// Greedy: subtract from highest-outstanding first.
	for amount > 0 {
		if len(quotas) == 0 {
			return fmt.Errorf("burn %d %s exceeds total outstanding", amount, denomID)
		}
		bestIdx := 0
		for i := 1; i < len(quotas); i++ {
			if quotas[i].out > quotas[bestIdx].out {
				bestIdx = i
			}
		}
		take := amount
		if take > quotas[bestIdx].out {
			take = quotas[bestIdx].out
		}
		q, _ := k.GetQuota(ctx, quotas[bestIdx].issuerID, denomID)
		q.Outstanding -= take
		q.UpdatedAt = ctx.BlockTime().Unix()
		if err := k.SetQuota(ctx, q); err != nil {
			return err
		}
		quotas[bestIdx].out -= take
		amount -= take
		if quotas[bestIdx].out == 0 {
			quotas = append(quotas[:bestIdx], quotas[bestIdx+1:]...)
		}
	}
	return nil
}

func (m msgServer) Transfer(goCtx context.Context, msg *types.MsgTransfer) (*types.MsgTransferResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := m.ComplianceCheck(ctx, msg.DenomId, msg.From, msg.To, msg.Amount, false); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("compliance: %s", err)
	}
	if err := m.moveBalance(ctx, msg.DenomId, msg.From, msg.To, msg.Amount); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("move: %s", err)
	}
	emit(ctx, "stablecoin_transferred",
		"denom_id", msg.DenomId, "from", msg.From, "to", msg.To,
		"amount", strconv.FormatUint(msg.Amount, 10))
	return &types.MsgTransferResponse{}, nil
}

func (m msgServer) Approve(goCtx context.Context, msg *types.MsgApprove) (*types.MsgApproveResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if !m.HasDenom(ctx, msg.DenomId) {
		return nil, sdkerrors.ErrNotFound.Wrapf("denom %s", msg.DenomId)
	}
	if err := m.SetAllowance(ctx, msg.DenomId, msg.Owner, msg.Spender, msg.Amount); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set allowance: %s", err)
	}
	emit(ctx, "stablecoin_approved",
		"denom_id", msg.DenomId, "owner", msg.Owner, "spender", msg.Spender,
		"amount", strconv.FormatUint(msg.Amount, 10))
	return &types.MsgApproveResponse{}, nil
}

func (m msgServer) TransferFrom(goCtx context.Context, msg *types.MsgTransferFrom) (*types.MsgTransferFromResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	allowed := m.GetAllowance(ctx, msg.DenomId, msg.From, msg.Spender)
	if allowed < msg.Amount {
		return nil, sdkerrors.ErrUnauthorized.Wrapf(
			"allowance %d < %d for spender %s on %s", allowed, msg.Amount, msg.Spender, msg.From)
	}
	if err := m.ComplianceCheck(ctx, msg.DenomId, msg.From, msg.To, msg.Amount, false); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("compliance: %s", err)
	}
	if err := m.moveBalance(ctx, msg.DenomId, msg.From, msg.To, msg.Amount); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("move: %s", err)
	}
	if err := m.SetAllowance(ctx, msg.DenomId, msg.From, msg.Spender, allowed-msg.Amount); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set allowance: %s", err)
	}
	emit(ctx, "stablecoin_transferred_from",
		"denom_id", msg.DenomId, "spender", msg.Spender, "from", msg.From, "to", msg.To,
		"amount", strconv.FormatUint(msg.Amount, 10))
	return &types.MsgTransferFromResponse{}, nil
}

// ---------------------------------------------------------------------------
// Compliance / force operations
// ---------------------------------------------------------------------------

func (m msgServer) Freeze(goCtx context.Context, msg *types.MsgFreeze) (*types.MsgFreezeResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if _, err := m.requireIssuerAdmin(ctx, msg.IssuerId, msg.Admin); err != nil {
		return nil, err
	}
	f := m.GetFlags(ctx, msg.DenomId, msg.Account)
	f.DenomId = msg.DenomId
	f.Account = msg.Account
	f.Frozen = true
	f.FreezeReason = msg.Reason
	f.FrozenAt = ctx.BlockTime().Unix()
	f.UpdatedBy = msg.Admin
	if err := m.SetFlags(ctx, f); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	m.recordAudit(ctx, msg.DenomId, msg.IssuerId, "freeze", msg.Admin, msg.Account, msg.Reason)
	emit(ctx, "stablecoin_account_frozen",
		"denom_id", msg.DenomId, "account", msg.Account, "issuer_id", msg.IssuerId)
	return &types.MsgFreezeResponse{}, nil
}

func (m msgServer) Unfreeze(goCtx context.Context, msg *types.MsgUnfreeze) (*types.MsgUnfreezeResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if _, err := m.requireIssuerAdmin(ctx, msg.IssuerId, msg.Admin); err != nil {
		return nil, err
	}
	f := m.GetFlags(ctx, msg.DenomId, msg.Account)
	f.DenomId = msg.DenomId
	f.Account = msg.Account
	f.Frozen = false
	f.FreezeReason = ""
	f.FrozenAt = 0
	f.UpdatedBy = msg.Admin
	if err := m.SetFlags(ctx, f); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	m.recordAudit(ctx, msg.DenomId, msg.IssuerId, "unfreeze", msg.Admin, msg.Account, msg.Reason)
	emit(ctx, "stablecoin_account_unfrozen",
		"denom_id", msg.DenomId, "account", msg.Account, "issuer_id", msg.IssuerId)
	return &types.MsgUnfreezeResponse{}, nil
}

func (m msgServer) Blacklist(goCtx context.Context, msg *types.MsgBlacklist) (*types.MsgBlacklistResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if _, err := m.requireIssuerAdmin(ctx, msg.IssuerId, msg.Admin); err != nil {
		return nil, err
	}
	f := m.GetFlags(ctx, msg.DenomId, msg.Account)
	f.DenomId = msg.DenomId
	f.Account = msg.Account
	f.Blacklisted = true
	f.BlacklistReason = msg.Reason
	f.BlacklistedAt = ctx.BlockTime().Unix()
	f.UpdatedBy = msg.Admin
	if err := m.SetFlags(ctx, f); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	m.recordAudit(ctx, msg.DenomId, msg.IssuerId, "blacklist", msg.Admin, msg.Account, msg.Reason)
	emit(ctx, "stablecoin_account_blacklisted",
		"denom_id", msg.DenomId, "account", msg.Account, "issuer_id", msg.IssuerId)
	return &types.MsgBlacklistResponse{}, nil
}

func (m msgServer) Unblacklist(goCtx context.Context, msg *types.MsgUnblacklist) (*types.MsgUnblacklistResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if _, err := m.requireIssuerAdmin(ctx, msg.IssuerId, msg.Admin); err != nil {
		return nil, err
	}
	f := m.GetFlags(ctx, msg.DenomId, msg.Account)
	f.DenomId = msg.DenomId
	f.Account = msg.Account
	f.Blacklisted = false
	f.BlacklistReason = ""
	f.BlacklistedAt = 0
	f.UpdatedBy = msg.Admin
	if err := m.SetFlags(ctx, f); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	m.recordAudit(ctx, msg.DenomId, msg.IssuerId, "unblacklist", msg.Admin, msg.Account, msg.Reason)
	emit(ctx, "stablecoin_account_unblacklisted",
		"denom_id", msg.DenomId, "account", msg.Account, "issuer_id", msg.IssuerId)
	return &types.MsgUnblacklistResponse{}, nil
}

func (m msgServer) ForceTransfer(goCtx context.Context, msg *types.MsgForceTransfer) (*types.MsgForceTransferResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	is, err := m.requireIssuerAdmin(ctx, msg.IssuerId, msg.Admin)
	if err != nil {
		return nil, err
	}
	// SECURITY: A REVOKED issuer cannot force-move tokens. This
	// prevents a once-trusted then-revoked admin from rug-pulling
	// holdings after governance has cut them off.
	if is.Status == types.IssuerStatus_ISSUER_STATUS_REVOKED {
		return nil, sdkerrors.ErrUnauthorized.Wrap("REVOKED issuer cannot force transfer")
	}
	d, ok := m.GetDenom(ctx, msg.DenomId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("denom %s", msg.DenomId)
	}
	if d.Status == types.DenomStatus_DENOM_STATUS_RETIRED {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("force transfer refused on RETIRED denom")
	}
	// Force ops bypass account freeze + sanctions checks because
	// that's their purpose (compliance recovery). We DO check the
	// from balance though — issuers cannot conjure tokens via force.
	if err := m.moveBalance(ctx, msg.DenomId, msg.From, msg.To, msg.Amount); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("move: %s", err)
	}
	m.recordAudit(ctx, msg.DenomId, msg.IssuerId, "force_transfer", msg.Admin, msg.From,
		fmt.Sprintf("to=%s amount=%d reason=%s", msg.To, msg.Amount, msg.Reason))
	emit(ctx, "stablecoin_force_transfer",
		"denom_id", msg.DenomId, "from", msg.From, "to", msg.To,
		"amount", strconv.FormatUint(msg.Amount, 10), "issuer_id", msg.IssuerId)
	return &types.MsgForceTransferResponse{}, nil
}

// ---------------------------------------------------------------------------
// Redemption queue
// ---------------------------------------------------------------------------

func (m msgServer) RequestRedemption(goCtx context.Context, msg *types.MsgRequestRedemption) (*types.MsgRequestRedemptionResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	is, ok := m.GetIssuer(ctx, msg.IssuerId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("issuer %s", msg.IssuerId)
	}
	// Cannot redeem from a revoked issuer — there's no one home to
	// pay out fiat. Suspended is OK because the issuer may still
	// honour pending redemptions before regulators authorise revoke.
	if is.Status == types.IssuerStatus_ISSUER_STATUS_REVOKED {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("issuer revoked; redemption refused")
	}
	d, ok := m.GetDenom(ctx, msg.DenomId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("denom %s", msg.DenomId)
	}
	if d.Status == types.DenomStatus_DENOM_STATUS_RETIRED {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("denom retired; redemption refused")
	}
	// Blacklist refuses redemption (the harder variant of freeze).
	// Frozen holders can still redeem because freeze is reversible
	// and may be applied for unrelated investigation reasons; cutting
	// off legitimate redemption would penalise honest users.
	f := m.GetFlags(ctx, msg.DenomId, msg.Holder)
	if f.Blacklisted {
		return nil, sdkerrors.ErrUnauthorized.Wrap("holder blacklisted")
	}
	params := m.GetParams(ctx)
	if m.CountPendingRedemptionsForHolder(ctx, msg.Holder) >= params.MaxRedemptionsPendingPerHolder {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"holder %s has too many pending redemptions (max %d)", msg.Holder, params.MaxRedemptionsPendingPerHolder)
	}
	// Burn at request time so tokens cannot move while in queue.
	if err := m.debitBalance(ctx, msg.DenomId, msg.Holder, msg.Amount); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("debit: %s", err)
	}
	if err := m.shrinkQuotaOnBurn(ctx, msg.DenomId, msg.Amount); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("shrink quota: %s", err)
	}
	id, err := m.NextRedemptionID(ctx)
	if err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("next id: %s", err)
	}
	r := types.Redemption{
		Id:          id,
		DenomId:     msg.DenomId,
		Holder:      msg.Holder,
		IssuerId:    msg.IssuerId,
		Amount:      msg.Amount,
		Status:      types.RedemptionStatus_REDEMPTION_STATUS_PENDING,
		Memo:        msg.Memo,
		RequestedAt: ctx.BlockTime().Unix(),
	}
	if err := m.SetRedemption(ctx, r); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set redemption: %s", err)
	}
	emit(ctx, "stablecoin_redemption_requested",
		"id", strconv.FormatUint(id, 10), "holder", msg.Holder,
		"denom_id", msg.DenomId, "amount", strconv.FormatUint(msg.Amount, 10))
	return &types.MsgRequestRedemptionResponse{RedemptionId: id}, nil
}

func (m msgServer) FulfillRedemption(goCtx context.Context, msg *types.MsgFulfillRedemption) (*types.MsgFulfillRedemptionResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	r, ok := m.GetRedemption(ctx, msg.RedemptionId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("redemption %d", msg.RedemptionId)
	}
	if r.Status != types.RedemptionStatus_REDEMPTION_STATUS_PENDING {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("redemption %d not pending", msg.RedemptionId)
	}
	if _, err := m.requireIssuerAdmin(ctx, r.IssuerId, msg.Admin); err != nil {
		return nil, err
	}
	r.Status = types.RedemptionStatus_REDEMPTION_STATUS_FULFILLED
	r.PayoutRef = msg.PayoutRef
	r.ResolvedAt = ctx.BlockTime().Unix()
	if err := m.SetRedemption(ctx, r); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set redemption: %s", err)
	}
	m.recordAudit(ctx, r.DenomId, r.IssuerId, "redemption_fulfilled", msg.Admin, r.Holder, msg.PayoutRef)
	emit(ctx, "stablecoin_redemption_fulfilled",
		"id", strconv.FormatUint(r.Id, 10), "payout_ref", msg.PayoutRef)
	return &types.MsgFulfillRedemptionResponse{}, nil
}

func (m msgServer) CancelRedemption(goCtx context.Context, msg *types.MsgCancelRedemption) (*types.MsgCancelRedemptionResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	r, ok := m.GetRedemption(ctx, msg.RedemptionId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("redemption %d", msg.RedemptionId)
	}
	if r.Status != types.RedemptionStatus_REDEMPTION_STATUS_PENDING {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("redemption %d not pending", msg.RedemptionId)
	}
	// Either the holder or the issuer admin may cancel. Holder
	// cancel is the typical "I changed my mind"; admin cancel is
	// "we cannot honour this because of off-chain compliance".
	is, _ := m.GetIssuer(ctx, r.IssuerId)
	if msg.Signer != r.Holder && msg.Signer != is.Admin {
		return nil, sdkerrors.ErrUnauthorized.Wrap("only holder or issuer admin may cancel")
	}
	// Re-mint the burned tokens back to the holder. We restore the
	// quota outstanding too so the issuer's cap reflects reality.
	//
	// SECURITY: cancel intentionally BYPASSES the ceiling cap. Cancel is
	// a recovery path for tokens that were already outstanding before
	// the redemption request (we burned them at request time). If
	// governance lowered the ceiling between request and cancel, a
	// ceiling-aware re-mint would strand the holder's funds in the
	// PENDING state with no path forward. Overflow is still guarded.
	q, ok := m.GetQuota(ctx, r.IssuerId, r.DenomId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("quota (%s,%s)", r.IssuerId, r.DenomId)
	}
	newOut, err := types.SafeAdd(q.Outstanding, r.Amount)
	if err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("outstanding overflow: %s", err)
	}
	if err := m.creditBalance(ctx, r.DenomId, r.Holder, r.Amount); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("credit: %s", err)
	}
	q.Outstanding = newOut
	q.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.SetQuota(ctx, q); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set quota: %s", err)
	}

	r.Status = types.RedemptionStatus_REDEMPTION_STATUS_CANCELLED
	r.CancelReason = msg.Reason
	r.ResolvedAt = ctx.BlockTime().Unix()
	if err := m.SetRedemption(ctx, r); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set redemption: %s", err)
	}
	emit(ctx, "stablecoin_redemption_cancelled",
		"id", strconv.FormatUint(r.Id, 10), "signer", msg.Signer, "reason", msg.Reason)
	return &types.MsgCancelRedemptionResponse{}, nil
}

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (m msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := msg.Params.Validate(); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("params: %s", err)
	}
	if err := m.SetParams(ctx, msg.Params); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "stablecoin_params_updated", "authority", msg.Authority)
	return &types.MsgUpdateParamsResponse{}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
