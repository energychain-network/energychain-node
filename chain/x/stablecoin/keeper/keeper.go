package keeper

import (
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/stablecoin/types"
)

// Keeper is the stablecoin module backbone. It owns its own balance
// ledger (no x/bank dependency) so each registered currency has an
// isolated supply, allowance graph, and account-flags map — what the
// spec calls "各自独立合约空间" (separate contract spaces per
// currency).
//
// Cross-module dependencies are narrow expected-keeper interfaces:
//   - PolicyKeeper:    DSL gate on every transfer / mint
//   - SanctionsKeeper: defense-in-depth sanctions check
//   - DIDKeeper:       optional KYC credential gate
//   - OracleKeeper:    reserve attestation gate on mint
//   - AuditKeeper:     compliance audit hook (forced ops + freezes)
//
// All cross-module pointers are nil-safe; the keeper falls through
// to "permissive" when the corresponding module has not been wired
// yet. This keeps the dependency graph buildable across milestones.
type Keeper struct {
	cdc          codec.Codec
	storeService corestore.KVStoreService
	authority    string

	policy    types.PolicyKeeper
	sanctions types.SanctionsKeeper
	did       types.DIDKeeper
	oracle    types.OracleKeeper
	audit     types.AuditKeeper

	Schema collections.Schema

	Params collections.Item[types.Params]

	Denoms   collections.Map[string, types.Denom]
	Issuers  collections.Map[string, types.Issuer]
	Quotas   collections.Map[collections.Pair[string, string], types.MintQuota]    // (issuer_id, denom_id)
	QuotaByDenom collections.KeySet[collections.Pair[string, string]]              // (denom_id, issuer_id)
	Reserves collections.Map[string, types.DenomReserve]

	Balances   collections.Map[collections.Pair[string, string], uint64]                        // (denom_id, account)
	Allowances collections.Map[collections.Triple[string, string, string], uint64]              // (denom_id, owner, spender)
	Supply     collections.Map[string, uint64]                                                  // denom_id -> total
	Flags      collections.Map[collections.Pair[string, string], types.AccountFlags]            // (denom_id, account)

	Redemptions        collections.Map[uint64, types.Redemption]
	RedemptionByHolder collections.KeySet[collections.Pair[string, uint64]] // (holder, id) — pending only
	RedemptionByDenom  collections.KeySet[collections.Pair[string, uint64]] // (denom_id, id) — pending only
	RedemptionIDSeq    collections.Sequence
}

func NewKeeper(
	cdc codec.Codec,
	storeService corestore.KVStoreService,
	authority string,
	policy types.PolicyKeeper,
	sanctions types.SanctionsKeeper,
	did types.DIDKeeper,
	oracle types.OracleKeeper,
	audit types.AuditKeeper,
) Keeper {
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		policy:       policy,
		sanctions:    sanctions,
		did:          did,
		oracle:       oracle,
		audit:        audit,

		Params: collections.NewItem(sb, types.ParamsCollectionPrefix, "params",
			codec.CollValue[types.Params](cdc)),

		Denoms: collections.NewMap(sb, types.DenomCollectionPrefix, "denoms",
			collections.StringKey, codec.CollValue[types.Denom](cdc)),

		Issuers: collections.NewMap(sb, types.IssuerCollectionPrefix, "issuers",
			collections.StringKey, codec.CollValue[types.Issuer](cdc)),

		Quotas: collections.NewMap(sb, types.QuotaCollectionPrefix, "quotas",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
			codec.CollValue[types.MintQuota](cdc)),

		QuotaByDenom: collections.NewKeySet(sb, types.QuotaByDenomPrefix, "quota_by_denom",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey)),

		Reserves: collections.NewMap(sb, types.ReserveCollectionPrefix, "reserves",
			collections.StringKey, codec.CollValue[types.DenomReserve](cdc)),

		Balances: collections.NewMap(sb, types.BalanceCollectionPrefix, "balances",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
			collections.Uint64Value),

		Allowances: collections.NewMap(sb, types.AllowanceCollectionPrefix, "allowances",
			collections.TripleKeyCodec(collections.StringKey, collections.StringKey, collections.StringKey),
			collections.Uint64Value),

		Supply: collections.NewMap(sb, types.SupplyCollectionPrefix, "supply",
			collections.StringKey, collections.Uint64Value),

		Flags: collections.NewMap(sb, types.FlagsCollectionPrefix, "flags",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
			codec.CollValue[types.AccountFlags](cdc)),

		Redemptions: collections.NewMap(sb, types.RedemptionCollectionPrefix, "redemptions",
			collections.Uint64Key, codec.CollValue[types.Redemption](cdc)),

		RedemptionByHolder: collections.NewKeySet(sb, types.RedemptionByHolderPrefix, "redemption_by_holder",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		RedemptionByDenom: collections.NewKeySet(sb, types.RedemptionByDenomPrefix, "redemption_by_denom",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		RedemptionIDSeq: collections.NewSequence(sb, types.RedemptionIDSeqPrefix, "redemption_seq"),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(fmt.Errorf("stablecoin keeper: build schema: %w", err))
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (k Keeper) SetParams(ctx sdk.Context, p types.Params) error { return k.Params.Set(ctx, p) }

func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	p, err := k.Params.Get(ctx)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("stablecoin params decode", "err", err)
		}
		return types.DefaultParams()
	}
	return p
}

// ---------------------------------------------------------------------------
// Denom
// ---------------------------------------------------------------------------

func (k Keeper) SetDenom(ctx sdk.Context, d types.Denom) error {
	return k.Denoms.Set(ctx, d.Id, d)
}

func (k Keeper) GetDenom(ctx sdk.Context, id string) (types.Denom, bool) {
	d, err := k.Denoms.Get(ctx, id)
	if err != nil {
		return types.Denom{}, false
	}
	return d, true
}

func (k Keeper) HasDenom(ctx sdk.Context, id string) bool {
	has, _ := k.Denoms.Has(ctx, id)
	return has
}

func (k Keeper) CountDenoms(ctx sdk.Context) uint32 {
	var n uint32
	_ = k.Denoms.Walk(ctx, nil, func(_ string, _ types.Denom) (bool, error) {
		n++
		return false, nil
	})
	return n
}

// ---------------------------------------------------------------------------
// Issuer
// ---------------------------------------------------------------------------

func (k Keeper) SetIssuer(ctx sdk.Context, is types.Issuer) error {
	return k.Issuers.Set(ctx, is.Id, is)
}

func (k Keeper) GetIssuer(ctx sdk.Context, id string) (types.Issuer, bool) {
	is, err := k.Issuers.Get(ctx, id)
	if err != nil {
		return types.Issuer{}, false
	}
	return is, true
}

func (k Keeper) CountIssuers(ctx sdk.Context) uint32 {
	var n uint32
	_ = k.Issuers.Walk(ctx, nil, func(_ string, _ types.Issuer) (bool, error) {
		n++
		return false, nil
	})
	return n
}

// IsMintAuthority returns true iff `addr` is currently listed as a
// mint authority for issuer `issuerID`. O(N) over the mint authority
// list — bounded by Params.MaxMintAuthoritiesPerIssuer.
func (k Keeper) IsMintAuthority(ctx sdk.Context, issuerID, addr string) bool {
	is, ok := k.GetIssuer(ctx, issuerID)
	if !ok {
		return false
	}
	for _, a := range is.MintAuthorities {
		if a == addr {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Quotas
// ---------------------------------------------------------------------------

func (k Keeper) SetQuota(ctx sdk.Context, q types.MintQuota) error {
	if err := k.Quotas.Set(ctx, collections.Join(q.IssuerId, q.DenomId), q); err != nil {
		return err
	}
	return k.QuotaByDenom.Set(ctx, collections.Join(q.DenomId, q.IssuerId))
}

func (k Keeper) GetQuota(ctx sdk.Context, issuerID, denomID string) (types.MintQuota, bool) {
	q, err := k.Quotas.Get(ctx, collections.Join(issuerID, denomID))
	if err != nil {
		return types.MintQuota{}, false
	}
	return q, true
}

// ---------------------------------------------------------------------------
// Reserve
// ---------------------------------------------------------------------------

func (k Keeper) SetReserve(ctx sdk.Context, r types.DenomReserve) error {
	return k.Reserves.Set(ctx, r.DenomId, r)
}

func (k Keeper) GetReserve(ctx sdk.Context, denomID string) (types.DenomReserve, bool) {
	r, err := k.Reserves.Get(ctx, denomID)
	if err != nil {
		return types.DenomReserve{}, false
	}
	return r, true
}

// ---------------------------------------------------------------------------
// Ledger primitives: Balance + Supply + Allowance
// ---------------------------------------------------------------------------

// GetBalance returns the holder's balance of `denomID`, or 0 when the
// row is absent. Zero rows are pruned by SetBalance to keep state
// growth proportional to the active holder set.
func (k Keeper) GetBalance(ctx sdk.Context, denomID, account string) uint64 {
	v, err := k.Balances.Get(ctx, collections.Join(denomID, account))
	if err != nil {
		return 0
	}
	return v
}

// SetBalance writes (or deletes when amount==0) the (denom, account)
// row. The keeper is the only writer of the supply ledger; callers
// MUST use creditBalance / debitBalance instead of poking SetBalance
// directly so supply stays in sync.
func (k Keeper) SetBalance(ctx sdk.Context, denomID, account string, amount uint64) error {
	key := collections.Join(denomID, account)
	if amount == 0 {
		return k.Balances.Remove(ctx, key)
	}
	return k.Balances.Set(ctx, key, amount)
}

func (k Keeper) GetSupply(ctx sdk.Context, denomID string) uint64 {
	v, err := k.Supply.Get(ctx, denomID)
	if err != nil {
		return 0
	}
	return v
}

func (k Keeper) setSupply(ctx sdk.Context, denomID string, total uint64) error {
	if total == 0 {
		return k.Supply.Remove(ctx, denomID)
	}
	return k.Supply.Set(ctx, denomID, total)
}

// creditBalance is the keeper-internal "+= amount" with overflow guard.
// Updates supply atomically with the per-account row.
func (k Keeper) creditBalance(ctx sdk.Context, denomID, account string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	cur := k.GetBalance(ctx, denomID, account)
	next, err := types.SafeAdd(cur, amount)
	if err != nil {
		return fmt.Errorf("credit %s/%s: %w", denomID, account, err)
	}
	if err := k.SetBalance(ctx, denomID, account, next); err != nil {
		return err
	}
	supply := k.GetSupply(ctx, denomID)
	nextSupply, err := types.SafeAdd(supply, amount)
	if err != nil {
		return fmt.Errorf("supply %s: %w", denomID, err)
	}
	return k.setSupply(ctx, denomID, nextSupply)
}

// debitBalance is the keeper-internal "-= amount" with underflow guard.
// Mirror of creditBalance for symmetry.
func (k Keeper) debitBalance(ctx sdk.Context, denomID, account string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	cur := k.GetBalance(ctx, denomID, account)
	next, err := types.SafeSub(cur, amount)
	if err != nil {
		return fmt.Errorf("debit %s/%s: %w", denomID, account, err)
	}
	if err := k.SetBalance(ctx, denomID, account, next); err != nil {
		return err
	}
	supply := k.GetSupply(ctx, denomID)
	nextSupply, err := types.SafeSub(supply, amount)
	if err != nil {
		return fmt.Errorf("supply %s: %w", denomID, err)
	}
	return k.setSupply(ctx, denomID, nextSupply)
}

// MoveBalance is the public peer-to-peer transfer entrypoint used by
// other modules (e.g. x/rwa for dividend payouts and redemption
// settlements). It debits `from` and credits `to` without changing
// the denom supply, mirroring moveBalance but skipping x/stablecoin's
// own ComplianceCheck pipeline. Callers MUST run their own gates
// (sanctions / freeze / KYC at the rwa-token layer) before invoking
// MoveBalance — the function only enforces that the denom exists,
// the source has enough balance, and the credit does not overflow.
//
// Use plain user transfers via MsgTransfer / ComplianceCheck; this is
// reserved for module-controlled payouts.
func (k Keeper) MoveBalance(ctx sdk.Context, denomID, from, to string, amount uint64) error {
	if !k.HasDenom(ctx, denomID) {
		return fmt.Errorf("denom %q not registered", denomID)
	}
	return k.moveBalance(ctx, denomID, from, to, amount)
}

// moveBalance is debit+credit without touching supply (transfer path).
func (k Keeper) moveBalance(ctx sdk.Context, denomID, from, to string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	src := k.GetBalance(ctx, denomID, from)
	if src < amount {
		return fmt.Errorf("insufficient balance for %s/%s: have %d need %d", denomID, from, src, amount)
	}
	dst, err := types.SafeAdd(k.GetBalance(ctx, denomID, to), amount)
	if err != nil {
		return fmt.Errorf("credit overflow %s/%s: %w", denomID, to, err)
	}
	if err := k.SetBalance(ctx, denomID, from, src-amount); err != nil {
		return err
	}
	return k.SetBalance(ctx, denomID, to, dst)
}

// GetAllowance returns the spender's currently-approved budget over
// owner's `denomID` balance. Zero rows are pruned (same as balances).
func (k Keeper) GetAllowance(ctx sdk.Context, denomID, owner, spender string) uint64 {
	v, err := k.Allowances.Get(ctx, collections.Join3(denomID, owner, spender))
	if err != nil {
		return 0
	}
	return v
}

func (k Keeper) SetAllowance(ctx sdk.Context, denomID, owner, spender string, amount uint64) error {
	key := collections.Join3(denomID, owner, spender)
	if amount == 0 {
		return k.Allowances.Remove(ctx, key)
	}
	return k.Allowances.Set(ctx, key, amount)
}

// ---------------------------------------------------------------------------
// Account flags (freeze + blacklist)
// ---------------------------------------------------------------------------

func (k Keeper) GetFlags(ctx sdk.Context, denomID, account string) types.AccountFlags {
	f, err := k.Flags.Get(ctx, collections.Join(denomID, account))
	if err != nil {
		return types.AccountFlags{DenomId: denomID, Account: account}
	}
	return f
}

func (k Keeper) SetFlags(ctx sdk.Context, f types.AccountFlags) error {
	// Prune cleared rows so state grows only with the actively-flagged set.
	if !f.Frozen && !f.Blacklisted {
		return k.Flags.Remove(ctx, collections.Join(f.DenomId, f.Account))
	}
	return k.Flags.Set(ctx, collections.Join(f.DenomId, f.Account), f)
}

// ---------------------------------------------------------------------------
// Compliance pipeline
// ---------------------------------------------------------------------------

// ComplianceCheck is the single chokepoint every transfer-like
// operation flows through. It runs (in order, fail-closed):
//
//  1. Denom must be ACTIVE (mints + transfers refused for PAUSED /
//     RETIRED; force ops allowed against PAUSED so issuers can wind
//     down).
//  2. Counterparty freeze flags (per-denom). Blacklist is strictly
//     stronger than freeze — both refuse send/receive but blacklist
//     also refuses redemption.
//  3. Sanctions check on both counterparties via SanctionsKeeper.
//  4. Optional KYC credential gate from Params.RequireKycCredential.
//  5. PolicyKeeper.EvaluateTransfer when the denom binds a policy.
//
// `allowPaused` is set true on the burn / cancelRedemption / force
// operation paths so an issuer can still drain a paused denom.
func (k Keeper) ComplianceCheck(
	ctx sdk.Context,
	denomID, sender, receiver string,
	amount uint64,
	allowPaused bool,
) error {
	d, ok := k.GetDenom(ctx, denomID)
	if !ok {
		return fmt.Errorf("denom %q not found", denomID)
	}
	switch d.Status {
	case types.DenomStatus_DENOM_STATUS_RETIRED:
		return fmt.Errorf("denom %q retired", denomID)
	case types.DenomStatus_DENOM_STATUS_PAUSED:
		if !allowPaused {
			return fmt.Errorf("denom %q paused", denomID)
		}
	}

	// 2. account flags
	if sender != "" {
		f := k.GetFlags(ctx, denomID, sender)
		if f.Frozen {
			return fmt.Errorf("sender %s is frozen on denom %s", sender, denomID)
		}
		if f.Blacklisted {
			return fmt.Errorf("sender %s is blacklisted on denom %s", sender, denomID)
		}
	}
	if receiver != "" {
		f := k.GetFlags(ctx, denomID, receiver)
		if f.Frozen {
			return fmt.Errorf("receiver %s is frozen on denom %s", receiver, denomID)
		}
		if f.Blacklisted {
			return fmt.Errorf("receiver %s is blacklisted on denom %s", receiver, denomID)
		}
	}

	// 3. sanctions
	if k.sanctions != nil {
		if sender != "" && k.sanctions.IsSanctioned(ctx, sender) {
			return fmt.Errorf("sender %s is sanctioned", sender)
		}
		if receiver != "" && k.sanctions.IsSanctioned(ctx, receiver) {
			return fmt.Errorf("receiver %s is sanctioned", receiver)
		}
	}

	// 4. KYC credential (defense-in-depth)
	if cred := k.GetParams(ctx).RequireKycCredential; cred != "" && k.did != nil {
		if sender != "" && !k.did.HasCredential(ctx, sender, cred) {
			return fmt.Errorf("sender %s missing required credential %s", sender, cred)
		}
		if receiver != "" && !k.did.HasCredential(ctx, receiver, cred) {
			return fmt.Errorf("receiver %s missing required credential %s", receiver, cred)
		}
	}

	// 5. policy DSL — x/policy resolves the bound policy from
	//    (asset_class, asset_id). Denom.PolicyId is informational
	//    metadata for clients; the authoritative binding lives in
	//    x/policy state.
	if k.policy != nil {
		if err := k.policy.EvaluateTransfer(ctx, "stablecoin", denomID, sender, receiver, amount); err != nil {
			return fmt.Errorf("policy denied: %w", err)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Reserve attestation gate
// ---------------------------------------------------------------------------

// CheckMintReserveCoverage refuses a mint if the oracle-attested
// reserve does not cover the post-mint outstanding at the configured
// ratio, or if the attestation is too stale.
//
// Fails closed when:
//   - Params.MintRequiresReserve is true AND no reserve is bound for
//     the denom (caller must wire one before mints succeed);
//   - oracle topic returns no value;
//   - last attestation is older than max_staleness_seconds;
//   - reported value is negative (treated as missing);
//   - newOutstanding * 10_000 > reserveValue * required_ratio_bps,
//     guarded against overflow with uint128-style multiplication.
func (k Keeper) CheckMintReserveCoverage(
	ctx sdk.Context,
	denomID string,
	newOutstanding uint64,
) error {
	if !k.GetParams(ctx).MintRequiresReserve {
		return nil
	}
	r, ok := k.GetReserve(ctx, denomID)
	if !ok {
		return fmt.Errorf("denom %s requires reserve attestation but none configured", denomID)
	}
	if k.oracle == nil {
		return fmt.Errorf("denom %s requires reserve attestation but oracle is not wired", denomID)
	}
	value, ts, ok := k.oracle.GetAggregatedReserve(ctx, r.OracleTopicId)
	if !ok {
		return fmt.Errorf("denom %s reserve topic %s has no attestation", denomID, r.OracleTopicId)
	}
	if value < 0 {
		return fmt.Errorf("denom %s reserve value negative", denomID)
	}
	if r.MaxStalenessSeconds > 0 {
		now := ctx.BlockTime().Unix()
		if now-ts > int64(r.MaxStalenessSeconds) {
			return fmt.Errorf("denom %s reserve attestation stale (age %ds, max %ds)",
				denomID, now-ts, r.MaxStalenessSeconds)
		}
	}
	// outstanding * 10_000 <= reserve * required_ratio_bps. We compare
	// against a non-negative reserve, lifted to uint64 first, then
	// guard the multiplications.
	reserve := uint64(value)
	lhs, err := types.SafeMulU128(newOutstanding, 10_000)
	if err != nil {
		return fmt.Errorf("denom %s outstanding overflow: %w", denomID, err)
	}
	rhs, err := types.SafeMulU128(reserve, uint64(r.RequiredRatioBps))
	if err != nil {
		return fmt.Errorf("denom %s reserve overflow: %w", denomID, err)
	}
	if cmpU128(lhs, rhs) > 0 {
		return fmt.Errorf("denom %s reserve coverage insufficient (outstanding %d * 10000 > reserve %d * %d bps)",
			denomID, newOutstanding, reserve, r.RequiredRatioBps)
	}
	return nil
}

// cmpU128 returns -1/0/1 for a <=> b on the 128-bit pair returned by
// types.SafeMulU128. Inlined into the keeper because it's a one-shot
// helper that doesn't deserve a public types/* surface.
func cmpU128(a, b types.U128) int {
	if a.Hi != b.Hi {
		if a.Hi > b.Hi {
			return 1
		}
		return -1
	}
	if a.Lo == b.Lo {
		return 0
	}
	if a.Lo > b.Lo {
		return 1
	}
	return -1
}

// ---------------------------------------------------------------------------
// Redemption queue
// ---------------------------------------------------------------------------

func (k Keeper) NextRedemptionID(ctx sdk.Context) (uint64, error) {
	cur, err := k.RedemptionIDSeq.Peek(ctx)
	if err != nil {
		return 0, err
	}
	next := cur + 1
	return next, k.RedemptionIDSeq.Set(ctx, next)
}

func (k Keeper) SetRedemption(ctx sdk.Context, r types.Redemption) error {
	if err := k.Redemptions.Set(ctx, r.Id, r); err != nil {
		return err
	}
	// Maintain secondary indexes only for PENDING entries — once
	// resolved, the indexes are removed so "list pending redemptions
	// for X" stays cheap as historical records pile up.
	if r.Status == types.RedemptionStatus_REDEMPTION_STATUS_PENDING {
		_ = k.RedemptionByHolder.Set(ctx, collections.Join(r.Holder, r.Id))
		_ = k.RedemptionByDenom.Set(ctx, collections.Join(r.DenomId, r.Id))
	} else {
		_ = k.RedemptionByHolder.Remove(ctx, collections.Join(r.Holder, r.Id))
		_ = k.RedemptionByDenom.Remove(ctx, collections.Join(r.DenomId, r.Id))
	}
	return nil
}

func (k Keeper) GetRedemption(ctx sdk.Context, id uint64) (types.Redemption, bool) {
	r, err := k.Redemptions.Get(ctx, id)
	if err != nil {
		return types.Redemption{}, false
	}
	return r, true
}

func (k Keeper) CountPendingRedemptionsForHolder(ctx sdk.Context, holder string) uint32 {
	var n uint32
	rng := collections.NewPrefixedPairRange[string, uint64](holder)
	_ = k.RedemptionByHolder.Walk(ctx, rng, func(_ collections.Pair[string, uint64]) (bool, error) {
		n++
		return false, nil
	})
	return n
}

// ---------------------------------------------------------------------------
// Audit hook (nil-safe)
// ---------------------------------------------------------------------------

func (k Keeper) recordAudit(ctx sdk.Context, denomID, issuerID, action, actor, subject, detail string) {
	if k.audit == nil {
		return
	}
	k.audit.RecordStablecoinAction(ctx, denomID, issuerID, action, actor, subject, detail)
}

// ---------------------------------------------------------------------------
// Genesis
// ---------------------------------------------------------------------------

func (k Keeper) InitGenesis(ctx sdk.Context, gs types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return fmt.Errorf("set params: %w", err)
	}
	for i, d := range gs.Denoms {
		if err := k.SetDenom(ctx, d); err != nil {
			return fmt.Errorf("denom %d: %w", i, err)
		}
	}
	for i, is := range gs.Issuers {
		if err := k.SetIssuer(ctx, is); err != nil {
			return fmt.Errorf("issuer %d: %w", i, err)
		}
	}
	for i, q := range gs.Quotas {
		if err := k.SetQuota(ctx, q); err != nil {
			return fmt.Errorf("quota %d: %w", i, err)
		}
	}
	for i, r := range gs.Reserves {
		if err := k.SetReserve(ctx, r); err != nil {
			return fmt.Errorf("reserve %d: %w", i, err)
		}
	}
	// Seed balances + supplies AFTER the denom registrations so
	// validators can reject malformed genesis early.
	for i, b := range gs.Balances {
		if err := k.Balances.Set(ctx, collections.Join(b.DenomId, b.Account), b.Amount); err != nil {
			return fmt.Errorf("balance %d: %w", i, err)
		}
	}
	for i, a := range gs.Allowances {
		if err := k.Allowances.Set(ctx, collections.Join3(a.DenomId, a.Owner, a.Spender), a.Amount); err != nil {
			return fmt.Errorf("allowance %d: %w", i, err)
		}
	}
	for i, s := range gs.Supplies {
		if err := k.Supply.Set(ctx, s.DenomId, s.TotalSupply); err != nil {
			return fmt.Errorf("supply %d: %w", i, err)
		}
	}
	for i, f := range gs.Flags {
		if err := k.SetFlags(ctx, f); err != nil {
			return fmt.Errorf("flags %d: %w", i, err)
		}
	}
	for i, r := range gs.Redemptions {
		if err := k.SetRedemption(ctx, r); err != nil {
			return fmt.Errorf("redemption %d: %w", i, err)
		}
	}
	if gs.RedemptionIdSeq > 0 {
		if err := k.RedemptionIDSeq.Set(ctx, gs.RedemptionIdSeq); err != nil {
			return fmt.Errorf("seed redemption seq: %w", err)
		}
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	gs := types.DefaultGenesis()
	gs.Params = k.GetParams(ctx)

	_ = k.Denoms.Walk(ctx, nil, func(_ string, d types.Denom) (bool, error) {
		gs.Denoms = append(gs.Denoms, d)
		return false, nil
	})
	_ = k.Issuers.Walk(ctx, nil, func(_ string, is types.Issuer) (bool, error) {
		gs.Issuers = append(gs.Issuers, is)
		return false, nil
	})
	_ = k.Quotas.Walk(ctx, nil, func(_ collections.Pair[string, string], q types.MintQuota) (bool, error) {
		gs.Quotas = append(gs.Quotas, q)
		return false, nil
	})
	_ = k.Reserves.Walk(ctx, nil, func(_ string, r types.DenomReserve) (bool, error) {
		gs.Reserves = append(gs.Reserves, r)
		return false, nil
	})
	_ = k.Balances.Walk(ctx, nil, func(key collections.Pair[string, string], v uint64) (bool, error) {
		if v == 0 {
			return false, nil
		}
		gs.Balances = append(gs.Balances, types.Balance{
			DenomId: key.K1(), Account: key.K2(), Amount: v,
		})
		return false, nil
	})
	_ = k.Allowances.Walk(ctx, nil, func(key collections.Triple[string, string, string], v uint64) (bool, error) {
		if v == 0 {
			return false, nil
		}
		gs.Allowances = append(gs.Allowances, types.Allowance{
			DenomId: key.K1(), Owner: key.K2(), Spender: key.K3(), Amount: v,
		})
		return false, nil
	})
	_ = k.Supply.Walk(ctx, nil, func(denomID string, total uint64) (bool, error) {
		gs.Supplies = append(gs.Supplies, types.Supply{DenomId: denomID, TotalSupply: total})
		return false, nil
	})
	_ = k.Flags.Walk(ctx, nil, func(_ collections.Pair[string, string], f types.AccountFlags) (bool, error) {
		gs.Flags = append(gs.Flags, f)
		return false, nil
	})
	_ = k.Redemptions.Walk(ctx, nil, func(_ uint64, r types.Redemption) (bool, error) {
		gs.Redemptions = append(gs.Redemptions, r)
		return false, nil
	})
	if seq, err := k.RedemptionIDSeq.Peek(ctx); err == nil {
		gs.RedemptionIdSeq = seq
	}
	return gs
}
