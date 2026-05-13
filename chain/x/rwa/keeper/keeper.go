package keeper

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"energychain/x/rwa/types"
)

// DistributionPoolAccount is the deterministic 20-byte module-derived
// account that holds funded settlement_denom for in-flight
// distributions. Using authtypes.NewModuleAddress guarantees the
// address is the standard length (so it round-trips through
// sdk.AccAddressFromBech32) and is derived deterministically from the
// well-known string below — independent of the chain's bech32 prefix
// or any future module-account permission registration.
var DistributionPoolAccount = authtypes.NewModuleAddress("rwa_distribution_pool").String()

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	policy     types.PolicyKeeper
	sanctions  types.SanctionsKeeper
	stablecoin types.StablecoinKeeper
	audit      types.AuditKeeper

	Schema collections.Schema

	Params  collections.Item[types.Params]
	Issuers collections.Map[string, types.Issuer]

	Tokens         collections.Map[uint64, types.Token]
	TokenBySymbol  collections.Map[string, uint64]
	TokenByIssuer  collections.KeySet[collections.Pair[string, uint64]]
	TokenIDSeq     collections.Sequence

	Balances       collections.Map[collections.Pair[uint64, string], uint64]
	BalanceByOwner collections.KeySet[collections.Pair[string, uint64]]

	AccountFlags collections.Map[collections.Pair[uint64, string], types.AccountFlags]

	Frozen collections.Map[collections.Pair[uint64, string], types.FrozenBalance]

	Lockups    collections.Map[collections.Triple[uint64, string, uint64], types.Lockup]
	LockupSeq  collections.Sequence

	Snapshots         collections.Map[uint64, types.SnapshotMeta]
	SnapshotByToken   collections.KeySet[collections.Pair[uint64, uint64]]
	SnapshotBalances  collections.Map[collections.Pair[uint64, string], uint64]
	SnapshotIDSeq     collections.Sequence

	Distributions       collections.Map[uint64, types.Distribution]
	DistributionByToken collections.KeySet[collections.Pair[uint64, uint64]]
	DistributionClaims  collections.Map[collections.Pair[uint64, string], types.DistributionClaim]
	DistributionIDSeq   collections.Sequence

	Redemptions               collections.Map[uint64, types.RedemptionRequest]
	RedemptionByHolder        collections.KeySet[collections.Pair[string, uint64]]
	RedemptionPendingByTok    collections.KeySet[collections.Pair[uint64, uint64]]
	RedemptionPendingByHolder collections.KeySet[collections.Pair[string, uint64]]
	RedemptionIDSeq           collections.Sequence

	PendingRedemptionUnits collections.Map[uint64, uint64]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	policy types.PolicyKeeper,
	sanctions types.SanctionsKeeper,
	stablecoin types.StablecoinKeeper,
	audit types.AuditKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("rwa: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		policy:       policy,
		sanctions:    sanctions,
		stablecoin:   stablecoin,
		audit:        audit,

		Params:  collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Issuers: collections.NewMap(sb, types.IssuerCollectionPrefix, "issuers", collections.StringKey, codec.CollValue[types.Issuer](cdc)),

		Tokens:        collections.NewMap(sb, types.TokenCollectionPrefix, "tokens", collections.Uint64Key, codec.CollValue[types.Token](cdc)),
		TokenBySymbol: collections.NewMap(sb, types.TokenBySymbolPrefix, "token_by_symbol", collections.StringKey, collections.Uint64Value),
		TokenByIssuer: collections.NewKeySet(sb, types.TokenByIssuerPrefix, "token_by_issuer",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		TokenIDSeq: collections.NewSequence(sb, types.TokenIDSeqPrefix, "token_id_seq"),

		Balances: collections.NewMap(sb, types.BalanceCollectionPrefix, "balances",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			collections.Uint64Value),
		BalanceByOwner: collections.NewKeySet(sb, types.BalanceByOwnerPrefix, "balance_by_owner",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		AccountFlags: collections.NewMap(sb, types.AccountFlagsPrefix, "account_flags",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.AccountFlags](cdc)),

		Frozen: collections.NewMap(sb, types.FrozenBalancePrefix, "frozen",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.FrozenBalance](cdc)),

		Lockups: collections.NewMap(sb, types.LockupCollectionPrefix, "lockups",
			collections.TripleKeyCodec(collections.Uint64Key, collections.StringKey, collections.Uint64Key),
			codec.CollValue[types.Lockup](cdc)),
		LockupSeq: collections.NewSequence(sb, types.LockupIDSeqPrefix, "lockup_id_seq"),

		Snapshots: collections.NewMap(sb, types.SnapshotCollectionPrefix, "snapshots",
			collections.Uint64Key, codec.CollValue[types.SnapshotMeta](cdc)),
		SnapshotByToken: collections.NewKeySet(sb, types.SnapshotByTokenPrefix, "snapshot_by_token",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),
		SnapshotBalances: collections.NewMap(sb, types.SnapshotBalancePrefix, "snapshot_balances",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			collections.Uint64Value),
		SnapshotIDSeq: collections.NewSequence(sb, types.SnapshotIDSeqPrefix, "snapshot_id_seq"),

		Distributions: collections.NewMap(sb, types.DistributionCollectionPrefix, "distributions",
			collections.Uint64Key, codec.CollValue[types.Distribution](cdc)),
		DistributionByToken: collections.NewKeySet(sb, types.DistributionByTokenPrefix, "dist_by_token",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),
		DistributionClaims: collections.NewMap(sb, types.DistributionClaimPrefix, "dist_claims",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.DistributionClaim](cdc)),
		DistributionIDSeq: collections.NewSequence(sb, types.DistributionIDSeqPrefix, "dist_id_seq"),

		Redemptions: collections.NewMap(sb, types.RedemptionCollectionPrefix, "redemptions",
			collections.Uint64Key, codec.CollValue[types.RedemptionRequest](cdc)),
		RedemptionByHolder: collections.NewKeySet(sb, types.RedemptionByHolderPrefix, "red_by_holder",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		RedemptionPendingByTok: collections.NewKeySet(sb, types.RedemptionPendingByTokenPrefix, "red_pending_by_token",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),
		RedemptionPendingByHolder: collections.NewKeySet(sb, types.RedemptionPendingByHolderPrefix, "red_pending_by_holder",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		RedemptionIDSeq: collections.NewSequence(sb, types.RedemptionIDSeqPrefix, "red_id_seq"),

		PendingRedemptionUnits: collections.NewMap(sb, types.PendingRedemptionUnitsPrefix, "pending_red_units",
			collections.Uint64Key, collections.Uint64Value),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string                    { return k.authority }
func (k Keeper) SanctionsHook() types.SanctionsKeeper    { return k.sanctions }
func (k Keeper) StablecoinHook() types.StablecoinKeeper  { return k.stablecoin }

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

// ---- issuer ---------------------------------------------------------------

func (k Keeper) GetIssuer(ctx context.Context, id string) (types.Issuer, bool, error) {
	is, err := k.Issuers.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Issuer{}, false, nil
		}
		return types.Issuer{}, false, err
	}
	return is, true, nil
}

func (k Keeper) CountIssuers(ctx context.Context) (uint32, error) {
	var n uint32
	if err := k.Issuers.Walk(ctx, nil, func(string, types.Issuer) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
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

// HasToken is the cheap read used by other modules (e.g. x/escrow)
// to fail-fast on bad token IDs before launching a multi-step flow.
func (k Keeper) HasToken(ctx sdk.Context, tokenID uint64) bool {
	has, _ := k.Tokens.Has(ctx, tokenID)
	return has
}

func (k Keeper) GetTokenBySymbol(ctx context.Context, symbol string) (types.Token, bool, error) {
	id, err := k.TokenBySymbol.Get(ctx, symbol)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Token{}, false, nil
		}
		return types.Token{}, false, err
	}
	return k.GetToken(ctx, id)
}

func (k Keeper) NextTokenID(ctx context.Context) (uint64, error) {
	id, err := k.TokenIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return id + 1, nil
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

func (k Keeper) setToken(ctx context.Context, t types.Token) error {
	if err := k.Tokens.Set(ctx, t.Id, t); err != nil {
		return err
	}
	if err := k.TokenBySymbol.Set(ctx, t.Symbol, t.Id); err != nil {
		return err
	}
	return k.TokenByIssuer.Set(ctx, collections.Join(t.IssuerId, t.Id))
}

// ---- balance plumbing -----------------------------------------------------

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
	pk := collections.Join(tokenID, holder)
	ok := collections.Join(holder, tokenID)
	if amt == 0 {
		if err := k.Balances.Remove(ctx, pk); err != nil {
			return err
		}
		// drop the per-account index entry but only when the row is
		// truly gone — defensive: a future addition might keep zero
		// rows around for some reason.
		if err := k.BalanceByOwner.Remove(ctx, ok); err != nil {
			return err
		}
		return nil
	}
	if err := k.Balances.Set(ctx, pk, amt); err != nil {
		return err
	}
	return k.BalanceByOwner.Set(ctx, ok)
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
		return fmt.Errorf("balance underflow for token %d holder %s: %w", tokenID, holder, err)
	}
	return k.setBalance(ctx, tokenID, holder, next)
}

// ---- frozen / lockup helpers ---------------------------------------------

func (k Keeper) GetFrozen(ctx context.Context, tokenID uint64, account string) (uint64, error) {
	f, err := k.Frozen.Get(ctx, collections.Join(tokenID, account))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return f.Amount, nil
}

// SumLocked walks every lockup row for (token, account) and returns
// (unmaturedTotal, maturedTotal). Mature rows are not removed here;
// they self-expire on each transferable read.
func (k Keeper) SumLocked(ctx context.Context, tokenID uint64, account string, now int64) (uint64, uint64, uint32, error) {
	rng := collections.NewSuperPrefixedTripleRange[uint64, string, uint64](tokenID, account)
	var unmatured, matured uint64
	var rowCount uint32
	if err := k.Lockups.Walk(ctx, rng, func(_ collections.Triple[uint64, string, uint64], v types.Lockup) (bool, error) {
		rowCount++
		if v.UnlockTime > now {
			next, err := types.SafeAdd(unmatured, v.Amount)
			if err != nil {
				return true, err
			}
			unmatured = next
			return false, nil
		}
		next, err := types.SafeAdd(matured, v.Amount)
		if err != nil {
			return true, err
		}
		matured = next
		return false, nil
	}); err != nil {
		return 0, 0, 0, err
	}
	return unmatured, matured, rowCount, nil
}

// transferable returns the number of units the holder may spend
// right now: balance − frozen − unmatured_lockups.
func (k Keeper) transferable(ctx context.Context, tokenID uint64, holder string, now int64) (
	balance, frozen, locked, transferable uint64, err error,
) {
	balance, err = k.GetBalance(ctx, tokenID, holder)
	if err != nil {
		return
	}
	frozen, err = k.GetFrozen(ctx, tokenID, holder)
	if err != nil {
		return
	}
	locked, _, _, err = k.SumLocked(ctx, tokenID, holder, now)
	if err != nil {
		return
	}
	reserved, e := types.SafeAdd(frozen, locked)
	if e != nil {
		err = e
		return
	}
	if reserved >= balance {
		transferable = 0
		return
	}
	transferable = balance - reserved
	return
}

func (k Keeper) Transferable(ctx context.Context, tokenID uint64, holder string) (
	balance, frozen, locked, transferable uint64, err error,
) {
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	return k.transferable(ctx, tokenID, holder, now)
}

// ---- compliance ----------------------------------------------------------

// transferCompliance is the unified gate used by Transfer.
// Order: address-only sanctions → token paused → KYC flag → policy DSL.
// ForceTransfer / Mint / Burn explicitly bypass parts of this so they
// can do issuer-driven recovery; each of those callers must still
// invoke the parts they want.
func (k Keeper) transferCompliance(ctx context.Context, t types.Token, sender, receiver string, amount uint64) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	if t.Status != types.TokenStatus_TOKEN_STATUS_ACTIVE {
		return fmt.Errorf("token %d not active (status=%s)", t.Id, t.Status)
	}

	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	if params.RequireSanctionsClear && k.sanctions != nil {
		if sender != "" && k.sanctions.IsSanctioned(sdkCtx, sender) {
			return fmt.Errorf("sender %s is sanctioned", sender)
		}
		if receiver != "" && k.sanctions.IsSanctioned(sdkCtx, receiver) {
			return fmt.Errorf("receiver %s is sanctioned", receiver)
		}
	}
	if t.RequireKycHolders {
		if err := k.requireKYC(ctx, t.Id, receiver); err != nil {
			return err
		}
	}
	if k.policy != nil && t.PolicyId != "" {
		idStr := strconv.FormatUint(t.Id, 10)
		if err := k.policy.EvaluateTransfer(sdkCtx, "rwa", idStr, sender, receiver, amount); err != nil {
			return fmt.Errorf("policy denied: %w", err)
		}
	}
	return nil
}

func (k Keeper) requireKYC(ctx context.Context, tokenID uint64, account string) error {
	if account == "" {
		return nil
	}
	f, err := k.AccountFlags.Get(ctx, collections.Join(tokenID, account))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return fmt.Errorf("account %s has no KYC flag for token %d", account, tokenID)
		}
		return err
	}
	if !f.KycCleared {
		return fmt.Errorf("account %s KYC not cleared for token %d", account, tokenID)
	}
	return nil
}

// requirePerHolderCap refuses to push a holder above the token's
// configured per-holder cap. cap=0 disables.
func (k Keeper) requirePerHolderCap(ctx context.Context, t types.Token, account string, delta uint64) error {
	if t.PerHolderCap == 0 {
		return nil
	}
	cur, err := k.GetBalance(ctx, t.Id, account)
	if err != nil {
		return err
	}
	next, err := types.SafeAdd(cur, delta)
	if err != nil {
		return err
	}
	if next > t.PerHolderCap {
		return fmt.Errorf("per-holder cap exceeded: %d > %d", next, t.PerHolderCap)
	}
	return nil
}

// ---- audit helper -------------------------------------------------------

func (k Keeper) recordAudit(ctx context.Context, tokenID uint64, issuerID, action, actor, subject, detail string) {
	if k.audit == nil {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.audit.RecordRWAAction(sdkCtx, tokenID, issuerID, action, actor, subject, detail)
}

// ---- pending redemption units --------------------------------------------

func (k Keeper) GetPendingRedemptionUnits(ctx context.Context, tokenID uint64) (uint64, error) {
	v, err := k.PendingRedemptionUnits.Get(ctx, tokenID)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}

func (k Keeper) addPendingUnits(ctx context.Context, tokenID, delta uint64) error {
	cur, err := k.GetPendingRedemptionUnits(ctx, tokenID)
	if err != nil {
		return err
	}
	next, err := types.SafeAdd(cur, delta)
	if err != nil {
		return err
	}
	if next == 0 {
		return k.PendingRedemptionUnits.Remove(ctx, tokenID)
	}
	return k.PendingRedemptionUnits.Set(ctx, tokenID, next)
}

func (k Keeper) subPendingUnits(ctx context.Context, tokenID, delta uint64) error {
	cur, err := k.GetPendingRedemptionUnits(ctx, tokenID)
	if err != nil {
		return err
	}
	next, err := types.SafeSub(cur, delta)
	if err != nil {
		return err
	}
	if next == 0 {
		return k.PendingRedemptionUnits.Remove(ctx, tokenID)
	}
	return k.PendingRedemptionUnits.Set(ctx, tokenID, next)
}

// ---- redemption helpers --------------------------------------------------

func (k Keeper) NextRedemptionID(ctx context.Context) (uint64, error) {
	id, err := k.RedemptionIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return id + 1, nil
}

// CountPendingRedemptionsForHolder is the bound used by the per-holder
// rate-limit. Walks the (holder, *) pending-only index so cost scales
// with currently-pending requests, not lifetime requests.
func (k Keeper) CountPendingRedemptionsForHolder(ctx context.Context, holder string) (uint32, error) {
	rng := collections.NewPrefixedPairRange[string, uint64](holder)
	var n uint32
	if err := k.RedemptionPendingByHolder.Walk(ctx, rng, func(_ collections.Pair[string, uint64]) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// PruneMaturedLockups removes lockup rows whose unlock_time has passed.
// Called from AddLockup so the MaxLockupsPerHolder cap applies to
// active rows only and per-holder lockup walks stay bounded over the
// life of the account.
func (k Keeper) PruneMaturedLockups(ctx context.Context, tokenID uint64, account string, now int64) (uint32, error) {
	rng := collections.NewSuperPrefixedTripleRange[uint64, string, uint64](tokenID, account)
	var matured []collections.Triple[uint64, string, uint64]
	if err := k.Lockups.Walk(ctx, rng, func(key collections.Triple[uint64, string, uint64], v types.Lockup) (bool, error) {
		if v.UnlockTime <= now {
			matured = append(matured, key)
		}
		return false, nil
	}); err != nil {
		return 0, err
	}
	for _, k2 := range matured {
		if err := k.Lockups.Remove(ctx, k2); err != nil {
			return 0, err
		}
	}
	return uint32(len(matured)), nil
}

// ---- snapshot helpers ----------------------------------------------------

func (k Keeper) NextSnapshotID(ctx context.Context) (uint64, error) {
	id, err := k.SnapshotIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return id + 1, nil
}

// ---- distribution helpers ------------------------------------------------

func (k Keeper) NextDistributionID(ctx context.Context) (uint64, error) {
	id, err := k.DistributionIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return id + 1, nil
}

// ---- lockup helpers ------------------------------------------------------

func (k Keeper) NextLockupID(ctx context.Context) (uint64, error) {
	id, err := k.LockupSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return id + 1, nil
}
