package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"energychain/x/rwatoken/types"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	compliance types.ComplianceKeeper
	settlement types.SettlementKeeper
	assethub   types.AssetHubKeeper

	Schema collections.Schema

	Params        collections.Item[types.Params]
	Tokens        collections.Map[uint64, types.Token]
	TokenBySymbol collections.Map[string, uint64]
	TokenIDSeq    collections.Sequence

	Balances collections.Map[collections.Pair[uint64, string], uint64]
	Flags    collections.Map[collections.Pair[uint64, string], types.AccountFlags]

	Snapshots        collections.Map[uint64, types.Snapshot]
	SnapshotIDSeq    collections.Sequence
	SnapshotBalances collections.Map[collections.Pair[uint64, string], uint64]

	Distributions     collections.Map[uint64, types.Distribution]
	DistributionIDSeq collections.Sequence
	DistributionClaim collections.Map[collections.Pair[uint64, string], types.DistributionClaimRow]

	Redemptions               collections.Map[uint64, types.Redemption]
	RedemptionIDSeq           collections.Sequence
	RedemptionByHolder        collections.KeySet[collections.Pair[string, uint64]]
	RedemptionPendingByHolder collections.KeySet[collections.Pair[string, uint64]]

	// Issuers is the governance-managed whitelist of accounts allowed to
	// create RWA share tokens (CreateToken gate).
	Issuers collections.KeySet[string]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	compliance types.ComplianceKeeper,
	settlement types.SettlementKeeper,
	assethub types.AssetHubKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("rwatoken: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		compliance:   compliance,
		settlement:   settlement,
		assethub:     assethub,

		Params: collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Tokens: collections.NewMap(sb, types.TokenPrefix, "tokens", collections.Uint64Key, codec.CollValue[types.Token](cdc)),
		TokenBySymbol: collections.NewMap(sb, types.TokenBySymbolPrefix, "token_by_symbol",
			collections.StringKey, collections.Uint64Value),
		TokenIDSeq: collections.NewSequence(sb, types.TokenIDSeqPrefix, "token_id_seq"),

		Balances: collections.NewMap(sb, types.BalancePrefix, "balances",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			collections.Uint64Value),
		Flags: collections.NewMap(sb, types.FlagsPrefix, "flags",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.AccountFlags](cdc)),

		Snapshots: collections.NewMap(sb, types.SnapshotPrefix, "snapshots",
			collections.Uint64Key, codec.CollValue[types.Snapshot](cdc)),
		SnapshotIDSeq: collections.NewSequence(sb, types.SnapshotIDSeqPrefix, "snapshot_id_seq"),
		SnapshotBalances: collections.NewMap(sb, types.SnapshotBalancePrefix, "snapshot_balances",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			collections.Uint64Value),

		Distributions: collections.NewMap(sb, types.DistributionPrefix, "distributions",
			collections.Uint64Key, codec.CollValue[types.Distribution](cdc)),
		DistributionIDSeq: collections.NewSequence(sb, types.DistributionIDSeqPrefix, "distribution_id_seq"),
		DistributionClaim: collections.NewMap(sb, types.DistributionClaimPrefix, "distribution_claims",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.DistributionClaimRow](cdc)),

		Redemptions: collections.NewMap(sb, types.RedemptionPrefix, "redemptions",
			collections.Uint64Key, codec.CollValue[types.Redemption](cdc)),
		RedemptionIDSeq: collections.NewSequence(sb, types.RedemptionIDSeqPrefix, "redemption_id_seq"),
		RedemptionByHolder: collections.NewKeySet(sb, types.RedemptionByHolderPfx, "redemption_by_holder",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		RedemptionPendingByHolder: collections.NewKeySet(sb, types.RedemptionPendingByHolderPfx, "redemption_pending_by_holder",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		Issuers: collections.NewKeySet(sb, types.IssuerPrefix, "issuers", collections.StringKey),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// IsIssuer reports whether addr is on the governance-managed issuer
// whitelist. The authority (gov) is implicitly always an issuer.
func (k Keeper) IsIssuer(ctx context.Context, addr string) (bool, error) {
	if addr == k.authority {
		return true, nil
	}
	return k.Issuers.Has(ctx, addr)
}

// DividendPoolAccount and RedemptionPoolAccount are the deterministic
// module-derived bech32 accounts that custody a token's settlement denom
// inside the x/stableusd ledger. They are split so that funds escrowed
// for a pro-rata dividend can never be drained to satisfy a buy-back
// (or vice versa), and per-token derivation keeps one token's float
// isolated from another's.
func DividendPoolAccount(tokenID uint64) string {
	return authtypes.NewModuleAddress(fmt.Sprintf("rwatoken/dividend/%d", tokenID)).String()
}

func RedemptionPoolAccount(tokenID uint64) string {
	return authtypes.NewModuleAddress(fmt.Sprintf("rwatoken/redemption/%d", tokenID)).String()
}

// ---- params ---------------------------------------------------------------

func (k Keeper) SetParams(ctx context.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return k.Params.Set(ctx, p)
}

func (k Keeper) GetParams(ctx context.Context) (types.Params, error) {
	p, err := k.Params.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.DefaultParams(), nil
		}
		return types.Params{}, err
	}
	return p, nil
}

// ---- token ----------------------------------------------------------------

func (k Keeper) GetToken(ctx context.Context, id uint64) (types.Token, bool, error) {
	t, err := k.Tokens.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Token{}, false, nil
		}
		return types.Token{}, false, err
	}
	return t, true, nil
}

func (k Keeper) HasToken(ctx context.Context, id uint64) bool {
	has, _ := k.Tokens.Has(ctx, id)
	return has
}

// TokenAdmin returns a token's admin address, used by cross-module
// callers (x/offering) to authorize issuer-driven flows.
func (k Keeper) TokenAdmin(ctx context.Context, id uint64) (string, bool) {
	t, ok, err := k.GetToken(ctx, id)
	if err != nil || !ok {
		return "", false
	}
	return t.Admin, true
}

// TokenSettlementDenom returns a token's settlement denom so cross-module
// payouts (x/offering) use the same stablecoin as dividends/redemptions.
func (k Keeper) TokenSettlementDenom(ctx context.Context, id uint64) (string, bool) {
	t, ok, err := k.GetToken(ctx, id)
	if err != nil || !ok {
		return "", false
	}
	return t.SettlementDenom, true
}

// TokenRequiresKYC reports whether a token gates holders on KYC, used by
// cross-module callers (x/offering) to require investor KYC before they
// commit subscription capital toward a KYC-gated security.
func (k Keeper) TokenRequiresKYC(ctx context.Context, id uint64) (bool, bool) {
	t, ok, err := k.GetToken(ctx, id)
	if err != nil || !ok {
		return false, false
	}
	return t.RequireKyc, true
}

// AllocateUnits is the cross-module primary-issuance entrypoint (x/offering
// allocations). It is a thin error-only wrapper over IssueUnits.
func (k Keeper) AllocateUnits(ctx context.Context, tokenID uint64, caller, recipient string, amount uint64) error {
	_, err := k.IssueUnits(ctx, tokenID, caller, recipient, amount)
	return err
}

func (k Keeper) setToken(ctx context.Context, t types.Token) error {
	if err := k.Tokens.Set(ctx, t.Id, t); err != nil {
		return err
	}
	return k.TokenBySymbol.Set(ctx, t.Symbol, t.Id)
}

func (k Keeper) NextTokenID(ctx context.Context) (uint64, error) {
	n, err := k.TokenIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}

func (k Keeper) CountTokens(ctx context.Context) (uint32, error) {
	var n uint32
	if err := k.Tokens.Walk(ctx, nil, func(uint64, types.Token) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// ---- balance plumbing ------------------------------------------------------

func (k Keeper) GetBalance(ctx context.Context, tokenID uint64, holder string) (uint64, error) {
	v, err := k.Balances.Get(ctx, collections.Join(tokenID, holder))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}

func (k Keeper) setBalance(ctx context.Context, tokenID uint64, holder string, amt uint64) error {
	key := collections.Join(tokenID, holder)
	if amt == 0 {
		return k.Balances.Remove(ctx, key)
	}
	return k.Balances.Set(ctx, key, amt)
}

func (k Keeper) creditBalance(ctx context.Context, tokenID uint64, holder string, amt uint64) error {
	if amt == 0 {
		return nil
	}
	cur, err := k.GetBalance(ctx, tokenID, holder)
	if err != nil {
		return err
	}
	next, err := types.SafeAdd(cur, amt)
	if err != nil {
		return err
	}
	return k.setBalance(ctx, tokenID, holder, next)
}

func (k Keeper) debitBalance(ctx context.Context, tokenID uint64, holder string, amt uint64) error {
	if amt == 0 {
		return nil
	}
	cur, err := k.GetBalance(ctx, tokenID, holder)
	if err != nil {
		return err
	}
	next, err := types.SafeSub(cur, amt)
	if err != nil {
		return types.ErrInsufficientFunds.Wrapf("token %d holder %s: have %d need %d", tokenID, holder, cur, amt)
	}
	return k.setBalance(ctx, tokenID, holder, next)
}

// moveUnits transfers RWA units between two slots without touching supply.
func (k Keeper) moveUnits(ctx context.Context, tokenID uint64, from, to string, amt uint64) error {
	if err := k.debitBalance(ctx, tokenID, from, amt); err != nil {
		return err
	}
	return k.creditBalance(ctx, tokenID, to, amt)
}

// ---- flags -----------------------------------------------------------------

func (k Keeper) GetFlags(ctx context.Context, tokenID uint64, holder string) types.AccountFlags {
	f, err := k.Flags.Get(ctx, collections.Join(tokenID, holder))
	if err != nil {
		return types.AccountFlags{TokenId: tokenID, Holder: holder}
	}
	return f
}

func (k Keeper) setFlags(ctx context.Context, f types.AccountFlags) error {
	key := collections.Join(f.TokenId, f.Holder)
	if !f.Frozen {
		return k.Flags.Remove(ctx, key)
	}
	return k.Flags.Set(ctx, key, f)
}

func (k Keeper) IsFrozen(ctx context.Context, tokenID uint64, holder string) bool {
	return k.GetFlags(ctx, tokenID, holder).Frozen
}

// ---- compliance ------------------------------------------------------------

// complianceCheck is the single chokepoint for holder-to-holder transfers.
// Fail-closed order: token status, per-token freeze (both parties),
// sanctions (both, when params.RequireSanctionsClear), KYC on the receiver
// (when token.RequireKyc), then the bound x/identity transfer policy.
// allowPaused lets force / burn / redemption paths act on a PAUSED token;
// MATURED always refuses ordinary transfers.
func (k Keeper) complianceCheck(ctx context.Context, t types.Token, from, to string, amount uint64, allowPaused bool) error {
	switch t.Status {
	case types.TokenStatus_TOKEN_STATUS_MATURED:
		return types.ErrTokenState.Wrapf("token %d matured", t.Id)
	case types.TokenStatus_TOKEN_STATUS_PAUSED:
		if !allowPaused {
			return types.ErrTokenState.Wrapf("token %d paused", t.Id)
		}
	case types.TokenStatus_TOKEN_STATUS_ACTIVE:
	default:
		return types.ErrTokenState.Wrapf("token %d status %s", t.Id, t.Status)
	}
	return k.partyCompliance(ctx, t, from, to, amount)
}

// partyCompliance applies the party-level compliance gates — per-token
// freeze (both parties), sanctions (both, when params.RequireSanctionsClear),
// KYC on the receiver (when token.RequireKyc), and the bound x/identity
// transfer policy — independent of the token lifecycle status. It is the
// shared chokepoint reused by complianceCheck (transfers/mint) and by the
// settlement legs (dividend claim, redemption request/execute) so a frozen,
// sanctioned, non-KYC, or policy-blocked holder cannot extract settlement
// even though dividends and redemptions legitimately run on PAUSED/MATURED
// tokens.
func (k Keeper) partyCompliance(ctx context.Context, t types.Token, from, to string, amount uint64) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	for _, party := range []string{from, to} {
		if party == "" {
			continue
		}
		if k.IsFrozen(ctx, t.Id, party) {
			return types.ErrFrozen.Wrapf("%s frozen on token %d", party, t.Id)
		}
	}

	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	if params.RequireSanctionsClear && k.compliance != nil {
		if from != "" && k.compliance.IsSanctioned(sdkCtx, from) {
			return types.ErrCompliance.Wrapf("%s sanctioned", from)
		}
		if to != "" && k.compliance.IsSanctioned(sdkCtx, to) {
			return types.ErrCompliance.Wrapf("%s sanctioned", to)
		}
	}
	if t.RequireKyc && to != "" && k.compliance != nil {
		if err := k.compliance.RequireKYC(sdkCtx, to); err != nil {
			return types.ErrCompliance.Wrapf("receiver KYC: %v", err)
		}
	}
	if t.PolicyId != "" && k.compliance != nil {
		assetRef := fmt.Sprintf("%d", t.Id)
		if err := k.compliance.EvaluateTransfer(sdkCtx, types.ModuleName, assetRef, t.PolicyId, from, to, amount); err != nil {
			return types.ErrCompliance.Wrapf("policy: %v", err)
		}
	}
	return nil
}

// IssueUnits is the shared primary-issuance primitive used by MsgMint and
// by cross-module allocators (x/offering). caller must be the token admin
// or the governance authority. It runs the recipient compliance gate and
// per-holder cap, then credits the recipient and increases total supply.
// The signer (caller) is also checked for sanctions so a sanctioned issuer
// cannot create new supply.
func (k Keeper) IssueUnits(ctx context.Context, tokenID uint64, caller, recipient string, amount uint64) (types.Token, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	t, ok, err := k.GetToken(ctx, tokenID)
	if err != nil {
		return types.Token{}, err
	}
	if !ok {
		return types.Token{}, types.ErrNotFound.Wrapf("token %d", tokenID)
	}
	if caller != t.Admin && caller != k.authority {
		return types.Token{}, types.ErrUnauthorized.Wrapf("not admin of token %d", tokenID)
	}
	if amount == 0 {
		return types.Token{}, types.ErrInvalidField.Wrap("amount must be > 0")
	}
	params, err := k.GetParams(ctx)
	if err != nil {
		return types.Token{}, err
	}
	if params.RequireSanctionsClear && k.compliance != nil && k.compliance.IsSanctioned(sdkCtx, caller) {
		return types.Token{}, types.ErrCompliance.Wrap("issuer sanctioned")
	}
	if err := k.complianceCheck(ctx, t, "", recipient, amount, false); err != nil {
		return types.Token{}, err
	}
	if err := k.requirePerHolderCap(ctx, t, recipient, amount); err != nil {
		return types.Token{}, err
	}
	newSupply, err := types.SafeAdd(t.TotalSupply, amount)
	if err != nil {
		return types.Token{}, err
	}
	if err := k.creditBalance(ctx, t.Id, recipient, amount); err != nil {
		return types.Token{}, err
	}
	t.TotalSupply = newSupply
	t.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := k.Tokens.Set(ctx, t.Id, t); err != nil {
		return types.Token{}, err
	}
	return t, nil
}

// requirePerHolderCap refuses to push a holder above the token's per-holder
// cap. cap == 0 disables. The escrow slot is exempt (it is module-owned).
func (k Keeper) requirePerHolderCap(ctx context.Context, t types.Token, holder string, delta uint64) error {
	if t.PerHolderCap == 0 || holder == types.RedemptionEscrow {
		return nil
	}
	cur, err := k.GetBalance(ctx, t.Id, holder)
	if err != nil {
		return err
	}
	next, err := types.SafeAdd(cur, delta)
	if err != nil {
		return err
	}
	if next > t.PerHolderCap {
		return types.ErrLimitExceeded.Wrapf("per-holder cap exceeded: %d > %d", next, t.PerHolderCap)
	}
	return nil
}

// validateDeviceBindings checks that every device a token binds exists,
// is ACTIVE, and is operated by the token admin — so the on-chain asset
// is provably anchored to real hardware the issuer controls. The break-
// glass authority is exempt (governance can bind on an operator's behalf).
// It is nil-safe: when the assethub keeper is not wired (unit tests) the
// ids are accepted as-is. ValidateBasic has already bounded count, length
// and rejected duplicates.
func (k Keeper) validateDeviceBindings(ctx context.Context, admin string, deviceIDs []string) error {
	if len(deviceIDs) == 0 || k.assethub == nil {
		return nil
	}
	for _, id := range deviceIDs {
		operator, active, found := k.assethub.DeviceOperator(ctx, id)
		if !found {
			return types.ErrInvalidField.Wrapf("device %q not found", id)
		}
		if !active {
			return types.ErrInvalidField.Wrapf("device %q not active", id)
		}
		if admin != k.authority && operator != admin {
			return types.ErrUnauthorized.Wrapf("device %q operated by %s, not admin %s", id, operator, admin)
		}
	}
	return nil
}

func (k Keeper) recordAudit(ctx context.Context, action, actor, subject, detail string) {
	if k.compliance == nil {
		return
	}
	k.compliance.RecordAction(sdk.UnwrapSDKContext(ctx), types.ModuleName, action, actor, subject, detail)
}

// ---- settlement (x/stableusd) ----------------------------------------------

func (k Keeper) requireSettlement() error {
	if k.settlement == nil {
		return types.ErrSettlement.Wrap("settlement keeper not wired")
	}
	return nil
}

// payFromPool moves settlement-denom funds out of one of a token's pools
// to a recipient, failing closed when the pool is underfunded.
func (k Keeper) payFromPool(ctx context.Context, t types.Token, pool, to string, amount uint64) error {
	if err := k.requireSettlement(); err != nil {
		return err
	}
	if amount == 0 {
		return nil
	}
	if k.settlement.GetBalance(ctx, t.SettlementDenom, pool) < amount {
		return types.ErrPoolUnderfunded.Wrapf("token %d pool needs %d %s", t.Id, amount, t.SettlementDenom)
	}
	return k.settlement.MoveBalance(ctx, t.SettlementDenom, pool, to, amount)
}

// fundPool moves settlement-denom funds from a payer into one of a
// token's pools. The payer is gated on sanctions so a sanctioned admin
// cannot escrow dividends or top up redemption liquidity (mirrors the
// issuer guard on mint/issuance).
func (k Keeper) fundPool(ctx context.Context, t types.Token, from, pool string, amount uint64) error {
	if err := k.requireSettlement(); err != nil {
		return err
	}
	if !k.settlement.HasDenom(ctx, t.SettlementDenom) {
		return types.ErrSettlement.Wrapf("unknown settlement denom %q", t.SettlementDenom)
	}
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	if params.RequireSanctionsClear && k.compliance != nil && k.compliance.IsSanctioned(sdk.UnwrapSDKContext(ctx), from) {
		return types.ErrCompliance.Wrapf("%s sanctioned", from)
	}
	return k.settlement.MoveBalance(ctx, t.SettlementDenom, from, pool, amount)
}

// RedemptionPoolBalance reports the settlement float available for
// buy-backs (the persistent pool funded via MsgFundPool).
func (k Keeper) RedemptionPoolBalance(ctx context.Context, t types.Token) uint64 {
	if k.settlement == nil {
		return 0
	}
	return k.settlement.GetBalance(ctx, t.SettlementDenom, RedemptionPoolAccount(t.Id))
}

// ---- sequences -------------------------------------------------------------

func (k Keeper) NextSnapshotID(ctx context.Context) (uint64, error) {
	n, err := k.SnapshotIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}

func (k Keeper) NextDistributionID(ctx context.Context) (uint64, error) {
	n, err := k.DistributionIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}

func (k Keeper) NextRedemptionID(ctx context.Context) (uint64, error) {
	n, err := k.RedemptionIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}

func (k Keeper) CountPendingRedemptionsForHolder(ctx context.Context, holder string) (uint32, error) {
	rng := collections.NewPrefixedPairRange[string, uint64](holder)
	var n uint32
	if err := k.RedemptionPendingByHolder.Walk(ctx, rng, func(collections.Pair[string, uint64]) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// ---- snapshot / distribution / redemption reads ----------------------------

func (k Keeper) GetSnapshot(ctx context.Context, id uint64) (types.Snapshot, bool, error) {
	s, err := k.Snapshots.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Snapshot{}, false, nil
		}
		return types.Snapshot{}, false, err
	}
	return s, true, nil
}

func (k Keeper) GetSnapshotBalance(ctx context.Context, snapshotID uint64, holder string) (uint64, error) {
	v, err := k.SnapshotBalances.Get(ctx, collections.Join(snapshotID, holder))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}

func (k Keeper) GetDistribution(ctx context.Context, id uint64) (types.Distribution, bool, error) {
	d, err := k.Distributions.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Distribution{}, false, nil
		}
		return types.Distribution{}, false, err
	}
	return d, true, nil
}

// ClaimableAmount returns a holder's pro-rata share of a distribution and
// whether it has already been claimed. share is the floor of
// total_amount * snapshot_balance / snapshot_total_supply.
func (k Keeper) ClaimableAmount(ctx context.Context, distID uint64, holder string) (amount uint64, claimed bool, err error) {
	d, ok, err := k.GetDistribution(ctx, distID)
	if err != nil || !ok {
		return 0, false, err
	}
	if has, e := k.DistributionClaim.Has(ctx, collections.Join(distID, holder)); e != nil {
		return 0, false, e
	} else if has {
		return 0, true, nil
	}
	s, ok, err := k.GetSnapshot(ctx, d.SnapshotId)
	if err != nil || !ok || s.TotalSupply == 0 {
		return 0, false, err
	}
	bal, err := k.GetSnapshotBalance(ctx, d.SnapshotId, holder)
	if err != nil {
		return 0, false, err
	}
	share, err := types.MulDivFloor(d.TotalAmount, bal, s.TotalSupply)
	if err != nil {
		return 0, false, err
	}
	return share, false, nil
}

func (k Keeper) GetRedemption(ctx context.Context, id uint64) (types.Redemption, bool, error) {
	r, err := k.Redemptions.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Redemption{}, false, nil
		}
		return types.Redemption{}, false, err
	}
	return r, true, nil
}

// setRedemption persists a redemption and keeps the (holder, id) indexes
// in sync: the all-index always carries the row, the pending-index only
// while status == PENDING.
func (k Keeper) setRedemption(ctx context.Context, r types.Redemption) error {
	if err := k.Redemptions.Set(ctx, r.Id, r); err != nil {
		return err
	}
	if err := k.RedemptionByHolder.Set(ctx, collections.Join(r.Holder, r.Id)); err != nil {
		return err
	}
	pendKey := collections.Join(r.Holder, r.Id)
	if r.Status == types.RedemptionStatus_REDEMPTION_STATUS_PENDING {
		return k.RedemptionPendingByHolder.Set(ctx, pendKey)
	}
	return k.RedemptionPendingByHolder.Remove(ctx, pendKey)
}
