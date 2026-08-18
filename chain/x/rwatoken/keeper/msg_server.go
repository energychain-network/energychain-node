package keeper

import (
	"context"
	"strconv"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/rwatoken/types"
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

// requireAdmin loads a token and asserts the caller is its admin or the
// governance authority (break-glass).
func (s msgServer) requireAdmin(ctx context.Context, tokenID uint64, caller string) (types.Token, error) {
	t, ok, err := s.k.GetToken(ctx, tokenID)
	if err != nil {
		return types.Token{}, err
	}
	if !ok {
		return types.Token{}, types.ErrNotFound.Wrapf("token %d", tokenID)
	}
	if caller != t.Admin && caller != s.k.authority {
		return types.Token{}, types.ErrUnauthorized.Wrapf("not admin of token %d", tokenID)
	}
	return t, nil
}

// ---- token lifecycle ------------------------------------------------------

// CreateToken is permissioned: only accounts on the governance-managed
// issuer whitelist (or the authority itself) may issue an RWA share token
// and become its admin. Every transfer still runs the full compliance
// chokepoint downstream.
func (s msgServer) CreateToken(ctx context.Context, m *types.MsgCreateToken) (*types.MsgCreateTokenResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if ok, err := s.k.IsIssuer(ctx, m.Admin); err != nil {
		return nil, err
	} else if !ok {
		return nil, types.ErrUnauthorized.Wrapf("%s is not a whitelisted issuer (governance manages the list via MsgAddIssuer)", m.Admin)
	}
	if has, err := s.k.TokenBySymbol.Has(ctx, m.Symbol); err != nil {
		return nil, err
	} else if has {
		return nil, types.ErrAlreadyExists.Wrapf("symbol %q", m.Symbol)
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	n, err := s.k.CountTokens(ctx)
	if err != nil {
		return nil, err
	}
	if n >= params.MaxTokens {
		return nil, types.ErrLimitExceeded.Wrap("max_tokens")
	}
	// The issuer (admin) must not be sanctioned at creation time.
	if params.RequireSanctionsClear && s.k.compliance != nil && s.k.compliance.IsSanctioned(sdkCtx, m.Admin) {
		return nil, types.ErrCompliance.Wrap("admin sanctioned")
	}
	// Fail fast if the settlement denom does not exist on the stableusd
	// ledger: otherwise the token is created but every dividend, pool
	// funding and redemption later fails at the settlement leg.
	if s.k.settlement != nil && !s.k.settlement.HasDenom(ctx, m.SettlementDenom) {
		return nil, types.ErrSettlement.Wrapf("settlement denom %q not found", m.SettlementDenom)
	}
	// Device bindings must reference real, ACTIVE hardware operated by the
	// issuer, anchoring the on-chain asset to its physical backing.
	if err := s.k.validateDeviceBindings(ctx, m.Admin, m.DeviceIds); err != nil {
		return nil, err
	}
	id, err := s.k.NextTokenID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdkCtx.BlockTime().Unix()
	t := types.Token{
		Id:                     id,
		Symbol:                 m.Symbol,
		Name:                   m.Name,
		Admin:                  m.Admin,
		AssetClass:             m.AssetClass,
		Decimals:               m.Decimals,
		TotalSupply:            0,
		Status:                 types.TokenStatus_TOKEN_STATUS_ACTIVE,
		SettlementDenom:        m.SettlementDenom,
		PolicyId:               m.PolicyId,
		RequireKyc:             m.RequireKyc,
		RedemptionPrice:        m.RedemptionPrice,
		RedemptionDelaySeconds: m.RedemptionDelaySeconds,
		PerHolderCap:           m.PerHolderCap,
		MetadataUri:            m.MetadataUri,
		DeviceIds:              m.DeviceIds,
		MaturityTime:           m.MaturityTime,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	if err := s.k.setToken(ctx, t); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "create_token", m.Admin, u(id), m.Symbol)
	emitEvent(sdkCtx, types.EventTypeToken, types.AttrAction, "create", types.AttrTokenID, u(id), types.AttrSymbol, m.Symbol)
	return &types.MsgCreateTokenResponse{TokenId: id}, nil
}

func (s msgServer) UpdateToken(ctx context.Context, m *types.MsgUpdateToken) (*types.MsgUpdateTokenResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	t, err := s.requireAdmin(ctx, m.TokenId, m.Admin)
	if err != nil {
		return nil, err
	}
	if err := s.k.validateDeviceBindings(ctx, t.Admin, m.DeviceIds); err != nil {
		return nil, err
	}
	t.MetadataUri = m.MetadataUri
	t.PolicyId = m.PolicyId
	t.RedemptionPrice = m.RedemptionPrice
	t.RedemptionDelaySeconds = m.RedemptionDelaySeconds
	t.PerHolderCap = m.PerHolderCap
	t.DeviceIds = m.DeviceIds
	t.MaturityTime = m.MaturityTime
	t.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Tokens.Set(ctx, t.Id, t); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "update_token", m.Admin, u(t.Id), "")
	emitEvent(sdkCtx, types.EventTypeToken, types.AttrAction, "update", types.AttrTokenID, u(t.Id))
	return &types.MsgUpdateTokenResponse{}, nil
}

func (s msgServer) SetTokenStatus(ctx context.Context, m *types.MsgSetTokenStatus) (*types.MsgSetTokenStatusResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	t, err := s.requireAdmin(ctx, m.TokenId, m.Admin)
	if err != nil {
		return nil, err
	}
	t.Status = m.Status
	t.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Tokens.Set(ctx, t.Id, t); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "set_status", m.Admin, u(t.Id), m.Status.String())
	emitEvent(sdkCtx, types.EventTypeToken, types.AttrAction, "set_status", types.AttrTokenID, u(t.Id))
	return &types.MsgSetTokenStatusResponse{}, nil
}

// ---- supply / transfer ----------------------------------------------------

func (s msgServer) Mint(ctx context.Context, m *types.MsgMint) (*types.MsgMintResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	t, err := s.k.IssueUnits(ctx, m.TokenId, m.Admin, m.Recipient, m.Amount)
	if err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "mint", m.Admin, m.Recipient, u(m.Amount))
	emitEvent(sdkCtx, types.EventTypeSupply, types.AttrAction, "mint", types.AttrTokenID, u(t.Id),
		types.AttrTo, m.Recipient, types.AttrAmount, u(m.Amount))
	return &types.MsgMintResponse{}, nil
}

// Burn destroys units held by `holder`. It is an admin recovery / buy-back
// destroy path: it bypasses transfer-policy and freeze gates (the admin is
// explicitly authorized) but always reduces supply by the burned amount.
func (s msgServer) Burn(ctx context.Context, m *types.MsgBurn) (*types.MsgBurnResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	t, err := s.requireAdmin(ctx, m.TokenId, m.Admin)
	if err != nil {
		return nil, err
	}
	if err := s.k.debitBalance(ctx, t.Id, m.Holder, m.Amount); err != nil {
		return nil, err
	}
	newSupply, err := types.SafeSub(t.TotalSupply, m.Amount)
	if err != nil {
		return nil, err
	}
	t.TotalSupply = newSupply
	t.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Tokens.Set(ctx, t.Id, t); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "burn", m.Admin, m.Holder, u(m.Amount))
	emitEvent(sdkCtx, types.EventTypeSupply, types.AttrAction, "burn", types.AttrTokenID, u(t.Id),
		types.AttrFrom, m.Holder, types.AttrAmount, u(m.Amount))
	return &types.MsgBurnResponse{}, nil
}

func (s msgServer) Transfer(ctx context.Context, m *types.MsgTransfer) (*types.MsgTransferResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	t, ok, err := s.k.GetToken(ctx, m.TokenId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("token %d", m.TokenId)
	}
	if err := s.k.complianceCheck(ctx, t, m.From, m.To, m.Amount, false); err != nil {
		return nil, err
	}
	if err := s.k.requirePerHolderCap(ctx, t, m.To, m.Amount); err != nil {
		return nil, err
	}
	if err := s.k.moveUnits(ctx, t.Id, m.From, m.To, m.Amount); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeTransfer, types.AttrAction, "transfer", types.AttrTokenID, u(t.Id),
		types.AttrFrom, m.From, types.AttrTo, m.To, types.AttrAmount, u(m.Amount))
	return &types.MsgTransferResponse{}, nil
}

// ForceTransfer is the regulatory seizure path: it moves units out of a
// (typically frozen) holder regardless of that holder's flags. The
// destination is still guarded against freeze and sanctions.
func (s msgServer) ForceTransfer(ctx context.Context, m *types.MsgForceTransfer) (*types.MsgForceTransferResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	t, err := s.requireAdmin(ctx, m.TokenId, m.Admin)
	if err != nil {
		return nil, err
	}
	if s.k.IsFrozen(ctx, t.Id, m.To) {
		return nil, types.ErrFrozen.Wrap("destination frozen")
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if params.RequireSanctionsClear && s.k.compliance != nil && s.k.compliance.IsSanctioned(sdkCtx, m.To) {
		return nil, types.ErrCompliance.Wrap("destination sanctioned")
	}
	if err := s.k.moveUnits(ctx, t.Id, m.From, m.To, m.Amount); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "force_transfer", m.Admin, m.From, m.Reason)
	emitEvent(sdkCtx, types.EventTypeCompliance, types.AttrAction, "force_transfer", types.AttrTokenID, u(t.Id),
		types.AttrFrom, m.From, types.AttrTo, m.To, types.AttrReason, m.Reason)
	return &types.MsgForceTransferResponse{}, nil
}

func (s msgServer) setFreeze(ctx context.Context, tokenID uint64, admin, holder string, frozen bool, action string) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	t, err := s.requireAdmin(ctx, tokenID, admin)
	if err != nil {
		return err
	}
	f := s.k.GetFlags(ctx, t.Id, holder)
	f.TokenId = t.Id
	f.Holder = holder
	f.Frozen = frozen
	if err := s.k.setFlags(ctx, f); err != nil {
		return err
	}
	s.k.recordAudit(ctx, action, admin, holder, u(t.Id))
	emitEvent(sdkCtx, types.EventTypeCompliance, types.AttrAction, action, types.AttrTokenID, u(t.Id), types.AttrAccount, holder)
	return nil
}

func (s msgServer) Freeze(ctx context.Context, m *types.MsgFreeze) (*types.MsgFreezeResponse, error) {
	if err := s.setFreeze(ctx, m.TokenId, m.Admin, m.Holder, true, "freeze"); err != nil {
		return nil, err
	}
	return &types.MsgFreezeResponse{}, nil
}

func (s msgServer) Unfreeze(ctx context.Context, m *types.MsgUnfreeze) (*types.MsgUnfreezeResponse, error) {
	if err := s.setFreeze(ctx, m.TokenId, m.Admin, m.Holder, false, "unfreeze"); err != nil {
		return nil, err
	}
	return &types.MsgUnfreezeResponse{}, nil
}

// ---- snapshot / dividend --------------------------------------------------

// TakeSnapshot freezes every real holder's balance for the token at the
// current height. The module escrow slot is excluded: units already
// pending redemption are exiting and are not entitled to future dividends.
func (s msgServer) TakeSnapshot(ctx context.Context, m *types.MsgTakeSnapshot) (*types.MsgTakeSnapshotResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	t, err := s.requireAdmin(ctx, m.TokenId, m.Admin)
	if err != nil {
		return nil, err
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	id, err := s.k.NextSnapshotID(ctx)
	if err != nil {
		return nil, err
	}

	rng := collections.NewPrefixedPairRange[uint64, string](t.Id)
	var total uint64
	var holders uint32
	type row struct {
		holder string
		amount uint64
	}
	var rows []row
	if err := s.k.Balances.Walk(ctx, rng, func(key collections.Pair[uint64, string], amt uint64) (bool, error) {
		holder := key.K2()
		if holder == types.RedemptionEscrow {
			return false, nil
		}
		// Bound the snapshot's gas/memory footprint: refuse (rather than
		// truncate — a partial snapshot would silently misprice dividends)
		// once the holder set exceeds the governance-set cap.
		if holders >= params.MaxHoldersPerSnapshot {
			return true, types.ErrLimitExceeded.Wrapf(
				"token %d has more than max_holders_per_snapshot (%d) holders", t.Id, params.MaxHoldersPerSnapshot)
		}
		sum, e := types.SafeAdd(total, amt)
		if e != nil {
			return true, e
		}
		total = sum
		holders++
		rows = append(rows, row{holder, amt})
		return false, nil
	}); err != nil {
		return nil, err
	}
	for _, r := range rows {
		if err := s.k.SnapshotBalances.Set(ctx, collections.Join(id, r.holder), r.amount); err != nil {
			return nil, err
		}
	}
	snap := types.Snapshot{
		Id:          id,
		TokenId:     t.Id,
		TotalSupply: total,
		TakenAt:     sdkCtx.BlockTime().Unix(),
		HolderCount: holders,
	}
	if err := s.k.Snapshots.Set(ctx, id, snap); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "take_snapshot", m.Admin, u(t.Id), u(id))
	emitEvent(sdkCtx, types.EventTypeSnapshot, types.AttrAction, "take", types.AttrTokenID, u(t.Id),
		types.AttrSnapshot, u(id), types.AttrAmount, u(total))
	return &types.MsgTakeSnapshotResponse{SnapshotId: id}, nil
}

// CreateDistribution escrows total_amount of the token's settlement denom
// into the dividend pool against a snapshot. Holders then claim their
// pro-rata share.
func (s msgServer) CreateDistribution(ctx context.Context, m *types.MsgCreateDistribution) (*types.MsgCreateDistributionResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	t, err := s.requireAdmin(ctx, m.TokenId, m.Admin)
	if err != nil {
		return nil, err
	}
	snap, ok, err := s.k.GetSnapshot(ctx, m.SnapshotId)
	if err != nil {
		return nil, err
	}
	if !ok || snap.TokenId != t.Id {
		return nil, types.ErrSnapshotState.Wrapf("snapshot %d not found for token %d", m.SnapshotId, t.Id)
	}
	if snap.TotalSupply == 0 {
		return nil, types.ErrDistribution.Wrap("snapshot has zero total supply")
	}
	id, err := s.k.NextDistributionID(ctx)
	if err != nil {
		return nil, err
	}
	// Escrow the full dividend up-front so claims are always covered.
	if err := s.k.fundPool(ctx, t, m.Admin, DividendPoolAccount(t.Id), m.TotalAmount); err != nil {
		return nil, err
	}
	d := types.Distribution{
		Id:            id,
		TokenId:       t.Id,
		SnapshotId:    m.SnapshotId,
		Denom:         t.SettlementDenom,
		TotalAmount:   m.TotalAmount,
		ClaimedAmount: 0,
		CreatedAt:     sdkCtx.BlockTime().Unix(),
	}
	if err := s.k.Distributions.Set(ctx, id, d); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "create_distribution", m.Admin, u(t.Id), u(id))
	emitEvent(sdkCtx, types.EventTypeDistribution, types.AttrAction, "create", types.AttrTokenID, u(t.Id),
		types.AttrDistrib, u(id), types.AttrAmount, u(m.TotalAmount))
	return &types.MsgCreateDistributionResponse{DistributionId: id}, nil
}

func (s msgServer) ClaimDistribution(ctx context.Context, m *types.MsgClaimDistribution) (*types.MsgClaimDistributionResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	d, ok, err := s.k.GetDistribution(ctx, m.DistributionId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("distribution %d", m.DistributionId)
	}
	t, ok, err := s.k.GetToken(ctx, d.TokenId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("token %d", d.TokenId)
	}
	if has, e := s.k.DistributionClaim.Has(ctx, collections.Join(m.DistributionId, m.Holder)); e != nil {
		return nil, e
	} else if has {
		return nil, types.ErrDistribution.Wrap("already claimed")
	}
	// Recipient compliance gate: do not pay dividends to a frozen,
	// sanctioned, non-KYC, or policy-blocked holder (the holder receives
	// settlement, so it is gated as a receipt; token status is not gated
	// because dividends legitimately run on PAUSED/MATURED tokens).
	if err := s.k.partyCompliance(ctx, t, "", m.Holder, 0); err != nil {
		return nil, err
	}
	snap, ok, err := s.k.GetSnapshot(ctx, d.SnapshotId)
	if err != nil {
		return nil, err
	}
	if !ok || snap.TotalSupply == 0 {
		return nil, types.ErrSnapshotState.Wrap("snapshot missing or empty")
	}
	bal, err := s.k.GetSnapshotBalance(ctx, d.SnapshotId, m.Holder)
	if err != nil {
		return nil, err
	}
	if bal == 0 {
		return nil, types.ErrDistribution.Wrap("holder had no balance at snapshot")
	}
	share, err := types.MulDivFloor(d.TotalAmount, bal, snap.TotalSupply)
	if err != nil {
		return nil, err
	}
	if share == 0 {
		return nil, types.ErrDistribution.Wrap("pro-rata share rounds to zero")
	}
	if err := s.k.payFromPool(ctx, t, DividendPoolAccount(t.Id), m.Holder, share); err != nil {
		return nil, err
	}
	claimed, err := types.SafeAdd(d.ClaimedAmount, share)
	if err != nil {
		return nil, err
	}
	d.ClaimedAmount = claimed
	if err := s.k.Distributions.Set(ctx, d.Id, d); err != nil {
		return nil, err
	}
	row := types.DistributionClaimRow{
		DistributionId: d.Id,
		Holder:         m.Holder,
		Amount:         share,
		ClaimedAt:      sdkCtx.BlockTime().Unix(),
	}
	if err := s.k.DistributionClaim.Set(ctx, collections.Join(d.Id, m.Holder), row); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeDistribution, types.AttrAction, "claim", types.AttrDistrib, u(d.Id),
		types.AttrAccount, m.Holder, types.AttrAmount, u(share))
	return &types.MsgClaimDistributionResponse{Amount: share}, nil
}

// ---- redemption -----------------------------------------------------------

func (s msgServer) FundPool(ctx context.Context, m *types.MsgFundPool) (*types.MsgFundPoolResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	t, err := s.requireAdmin(ctx, m.TokenId, m.Admin)
	if err != nil {
		return nil, err
	}
	if err := s.k.fundPool(ctx, t, m.Admin, RedemptionPoolAccount(t.Id), m.Amount); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "fund_pool", m.Admin, u(t.Id), u(m.Amount))
	emitEvent(sdkCtx, types.EventTypePool, types.AttrAction, "fund", types.AttrTokenID, u(t.Id), types.AttrAmount, u(m.Amount))
	return &types.MsgFundPoolResponse{}, nil
}

// RequestRedemption escrows the holder's units (supply unchanged) and
// schedules a T+N settlement payout at redemption_price per unit.
func (s msgServer) RequestRedemption(ctx context.Context, m *types.MsgRequestRedemption) (*types.MsgRequestRedemptionResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	t, ok, err := s.k.GetToken(ctx, m.TokenId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("token %d", m.TokenId)
	}
	// Redemption is allowed while ACTIVE or MATURED, never while PAUSED.
	if t.Status == types.TokenStatus_TOKEN_STATUS_PAUSED {
		return nil, types.ErrTokenState.Wrap("token paused")
	}
	if t.RedemptionPrice == 0 {
		return nil, types.ErrRedemptionState.Wrap("redemption disabled (price 0)")
	}
	// Divestiture compliance gate (holder is the source of the units):
	// freeze, sanctions, and the bound identity policy must clear before
	// the units are escrowed for redemption.
	if err := s.k.partyCompliance(ctx, t, m.Holder, "", m.Units); err != nil {
		return nil, err
	}
	// partyCompliance only enforces KYC on the receiving leg; a redemption
	// pays settlement *out* to the holder, so the holder (the source leg)
	// must also be KYC-cleared on a KYC-gated token before they can cash out.
	if t.RequireKyc && s.k.compliance != nil {
		if err := s.k.compliance.RequireKYC(sdkCtx, m.Holder); err != nil {
			return nil, types.ErrCompliance.Wrapf("redeemer KYC: %v", err)
		}
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	pending, err := s.k.CountPendingRedemptionsForHolder(ctx, m.Holder)
	if err != nil {
		return nil, err
	}
	if pending >= params.MaxPendingRedemptionsPerHolder {
		return nil, types.ErrLimitExceeded.Wrap("max_pending_redemptions_per_holder")
	}
	payout, err := types.SafeMul(m.Units, t.RedemptionPrice)
	if err != nil {
		return nil, err
	}
	// Escrow the units (supply unchanged) so they cannot be re-spent while
	// the settlement window runs; a later cancel returns the exact amount.
	if err := s.k.moveUnits(ctx, t.Id, m.Holder, types.RedemptionEscrow, m.Units); err != nil {
		return nil, err
	}
	id, err := s.k.NextRedemptionID(ctx)
	if err != nil {
		return nil, err
	}
	now := sdkCtx.BlockTime().Unix()
	r := types.Redemption{
		Id:           id,
		TokenId:      t.Id,
		Holder:       m.Holder,
		Units:        m.Units,
		Denom:        t.SettlementDenom,
		Payout:       payout,
		Status:       types.RedemptionStatus_REDEMPTION_STATUS_PENDING,
		RequestedAt:  now,
		ExecuteAfter: now + t.RedemptionDelaySeconds,
	}
	if err := s.k.setRedemption(ctx, r); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "request_redemption", m.Holder, u(t.Id), u(m.Units))
	emitEvent(sdkCtx, types.EventTypeRedemption, types.AttrAction, "request", types.AttrTokenID, u(t.Id),
		types.AttrAccount, m.Holder, types.AttrRedeemID, u(id), types.AttrPayout, u(payout))
	return &types.MsgRequestRedemptionResponse{RedemptionId: id, Payout: payout, ExecuteAfter: r.ExecuteAfter}, nil
}

// ExecuteRedemption settles a matured redemption: it pays the holder from
// the redemption pool and burns the escrowed units (supply decreases).
// Anyone may execute once the T+N window elapses; the admin may settle
// early.
func (s msgServer) ExecuteRedemption(ctx context.Context, m *types.MsgExecuteRedemption) (*types.MsgExecuteRedemptionResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	r, ok, err := s.k.GetRedemption(ctx, m.RedemptionId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("redemption %d", m.RedemptionId)
	}
	if r.Status != types.RedemptionStatus_REDEMPTION_STATUS_PENDING {
		return nil, types.ErrRedemptionState.Wrap("not pending")
	}
	t, ok, err := s.k.GetToken(ctx, r.TokenId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("token %d", r.TokenId)
	}
	now := sdkCtx.BlockTime().Unix()
	isAdmin := m.Executor == t.Admin || m.Executor == s.k.authority
	if !isAdmin && now < r.ExecuteAfter {
		return nil, types.ErrRedemptionState.Wrapf("settlement window not reached (execute_after=%d)", r.ExecuteAfter)
	}
	// Settlement compliance gate (holder receives settlement): freeze,
	// sanctions, KYC, and the bound policy must clear. A blocked holder
	// cannot be paid; the admin can CancelRedemption to release the
	// escrowed units back to them.
	if err := s.k.partyCompliance(ctx, t, "", r.Holder, 0); err != nil {
		return nil, err
	}
	// Pay first (fails closed if the pool is underfunded), then burn the
	// escrowed units so supply only drops once the holder is paid.
	if err := s.k.payFromPool(ctx, t, RedemptionPoolAccount(t.Id), r.Holder, r.Payout); err != nil {
		return nil, err
	}
	if err := s.k.debitBalance(ctx, t.Id, types.RedemptionEscrow, r.Units); err != nil {
		return nil, err
	}
	newSupply, err := types.SafeSub(t.TotalSupply, r.Units)
	if err != nil {
		return nil, err
	}
	t.TotalSupply = newSupply
	t.UpdatedAt = now
	if err := s.k.Tokens.Set(ctx, t.Id, t); err != nil {
		return nil, err
	}
	r.Status = types.RedemptionStatus_REDEMPTION_STATUS_SETTLED
	r.ResolvedAt = now
	if err := s.k.setRedemption(ctx, r); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "execute_redemption", m.Executor, r.Holder, u(r.Id))
	emitEvent(sdkCtx, types.EventTypeRedemption, types.AttrAction, "execute", types.AttrTokenID, u(t.Id),
		types.AttrRedeemID, u(r.Id), types.AttrPayout, u(r.Payout))
	return &types.MsgExecuteRedemptionResponse{}, nil
}

// CancelRedemption returns the escrowed units to the holder (supply-neutral
// release). Admin only. Refused if the holder is now frozen.
func (s msgServer) CancelRedemption(ctx context.Context, m *types.MsgCancelRedemption) (*types.MsgCancelRedemptionResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	r, ok, err := s.k.GetRedemption(ctx, m.RedemptionId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("redemption %d", m.RedemptionId)
	}
	if _, err := s.requireAdmin(ctx, r.TokenId, m.Admin); err != nil {
		return nil, err
	}
	if r.Status != types.RedemptionStatus_REDEMPTION_STATUS_PENDING {
		return nil, types.ErrRedemptionState.Wrap("not pending")
	}
	if s.k.IsFrozen(ctx, r.TokenId, r.Holder) {
		return nil, types.ErrFrozen.Wrap("holder frozen; cannot return units")
	}
	if err := s.k.moveUnits(ctx, r.TokenId, types.RedemptionEscrow, r.Holder, r.Units); err != nil {
		return nil, err
	}
	r.Status = types.RedemptionStatus_REDEMPTION_STATUS_CANCELLED
	r.ResolvedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.setRedemption(ctx, r); err != nil {
		return nil, err
	}
	s.k.recordAudit(ctx, "cancel_redemption", m.Admin, r.Holder, m.Reason)
	emitEvent(sdkCtx, types.EventTypeRedemption, types.AttrAction, "cancel", types.AttrRedeemID, u(r.Id), types.AttrReason, m.Reason)
	return &types.MsgCancelRedemptionResponse{}, nil
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

// ---- issuer whitelist ------------------------------------------------------

func (s msgServer) AddIssuer(ctx context.Context, m *types.MsgAddIssuer) (*types.MsgAddIssuerResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if err := types.MustBech32(m.Issuer); err != nil {
		return nil, err
	}
	if has, err := s.k.Issuers.Has(ctx, m.Issuer); err != nil {
		return nil, err
	} else if has {
		return nil, types.ErrAlreadyExists.Wrapf("issuer %s", m.Issuer)
	}
	if err := s.k.Issuers.Set(ctx, m.Issuer); err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	emitEvent(sdkCtx, types.EventTypeIssuer, types.AttrAction, "add",
		types.AttrAccount, m.Issuer, "display_name", m.DisplayName)
	return &types.MsgAddIssuerResponse{}, nil
}

func (s msgServer) RemoveIssuer(ctx context.Context, m *types.MsgRemoveIssuer) (*types.MsgRemoveIssuerResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if has, err := s.k.Issuers.Has(ctx, m.Issuer); err != nil {
		return nil, err
	} else if !has {
		return nil, types.ErrNotFound.Wrapf("issuer %s", m.Issuer)
	}
	if err := s.k.Issuers.Remove(ctx, m.Issuer); err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	emitEvent(sdkCtx, types.EventTypeIssuer, types.AttrAction, "remove", types.AttrAccount, m.Issuer)
	return &types.MsgRemoveIssuerResponse{}, nil
}
