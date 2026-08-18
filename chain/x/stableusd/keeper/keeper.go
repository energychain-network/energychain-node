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

	"energychain/x/stableusd/types"
)

// Keeper owns an isolated balance ledger per registered stable denom (no
// x/bank dependency): each denom has its own supply, allowance graph, and
// per-account flag map. Cross-module compliance flows through x/identity
// (sanctions / KYC / transfer policies) and reserve attestation through
// the x/assethub oracle. Both cross-module pointers are nil-safe so the
// keeper stays buildable across milestones.
type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	compliance types.ComplianceKeeper
	reserve    types.ReserveKeeper

	Schema collections.Schema

	Params collections.Item[types.Params]
	Denoms collections.Map[string, types.StableDenom]

	Balances   collections.Map[collections.Pair[string, string], uint64]
	Supply     collections.Map[string, uint64]
	Allowances collections.Map[collections.Triple[string, string, string], uint64]
	Flags      collections.Map[collections.Pair[string, string], types.AccountFlags]

	Redemptions        collections.Map[uint64, types.Redemption]
	RedemptionByHolder collections.KeySet[collections.Pair[string, uint64]]
	RedemptionIDSeq    collections.Sequence
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	compliance types.ComplianceKeeper,
	reserve types.ReserveKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("stableusd: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		compliance:   compliance,
		reserve:      reserve,

		Params: collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Denoms: collections.NewMap(sb, types.DenomPrefix, "denoms", collections.StringKey, codec.CollValue[types.StableDenom](cdc)),

		Balances: collections.NewMap(sb, types.BalancePrefix, "balances",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey), collections.Uint64Value),
		Supply: collections.NewMap(sb, types.SupplyPrefix, "supply", collections.StringKey, collections.Uint64Value),
		Allowances: collections.NewMap(sb, types.AllowancePrefix, "allowances",
			collections.TripleKeyCodec(collections.StringKey, collections.StringKey, collections.StringKey), collections.Uint64Value),
		Flags: collections.NewMap(sb, types.FlagsPrefix, "flags",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey), codec.CollValue[types.AccountFlags](cdc)),

		Redemptions: collections.NewMap(sb, types.RedemptionPrefix, "redemptions", collections.Uint64Key, codec.CollValue[types.Redemption](cdc)),
		RedemptionByHolder: collections.NewKeySet(sb, types.RedemptionByHolderPfx, "redemption_by_holder",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		RedemptionIDSeq: collections.NewSequence(sb, types.RedemptionIDSeqPrefix, "redemption_seq"),
	}
	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// ---- params ---------------------------------------------------------------

func (k Keeper) SetParams(ctx context.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return k.Params.Set(ctx, p)
}

func (k Keeper) GetParams(ctx context.Context) types.Params {
	p, err := k.Params.Get(ctx)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			sdk.UnwrapSDKContext(ctx).Logger().Error("stableusd params decode", "err", err)
		}
		return types.DefaultParams()
	}
	return p
}

// ---- denom -----------------------------------------------------------------

func (k Keeper) GetDenom(ctx context.Context, id string) (types.StableDenom, bool) {
	d, err := k.Denoms.Get(ctx, id)
	if err != nil {
		return types.StableDenom{}, false
	}
	return d, true
}

func (k Keeper) HasDenom(ctx context.Context, id string) bool {
	has, _ := k.Denoms.Has(ctx, id)
	return has
}

// DenomActive reports whether a settlement denom exists and is ACTIVE (not
// PAUSED or RETIRED). Cross-module callers (x/bridge) use it to refuse moving
// settlement that the ledger has administratively halted.
func (k Keeper) DenomActive(ctx context.Context, id string) bool {
	d, ok := k.GetDenom(ctx, id)
	if !ok {
		return false
	}
	return d.Status == types.DenomStatus_DENOM_STATUS_ACTIVE
}

func (k Keeper) CountDenoms(ctx context.Context) (uint64, error) {
	keys, err := k.Denoms.Iterate(ctx, nil)
	if err != nil {
		return 0, err
	}
	ks, err := keys.Keys()
	if err != nil {
		return 0, err
	}
	return uint64(len(ks)), nil
}

func (k Keeper) isMinter(d types.StableDenom, addr string) bool {
	for _, m := range d.Minters {
		if m == addr {
			return true
		}
	}
	return false
}

// ---- ledger ----------------------------------------------------------------

func (k Keeper) GetBalance(ctx context.Context, denomID, account string) uint64 {
	v, err := k.Balances.Get(ctx, collections.Join(denomID, account))
	if err != nil {
		return 0
	}
	return v
}

func (k Keeper) setBalance(ctx context.Context, denomID, account string, amount uint64) error {
	key := collections.Join(denomID, account)
	if amount == 0 {
		return k.Balances.Remove(ctx, key)
	}
	return k.Balances.Set(ctx, key, amount)
}

func (k Keeper) GetSupply(ctx context.Context, denomID string) uint64 {
	v, err := k.Supply.Get(ctx, denomID)
	if err != nil {
		return 0
	}
	return v
}

func (k Keeper) setSupply(ctx context.Context, denomID string, total uint64) error {
	if total == 0 {
		return k.Supply.Remove(ctx, denomID)
	}
	return k.Supply.Set(ctx, denomID, total)
}

// credit increases balance and supply atomically (mint path).
func (k Keeper) credit(ctx context.Context, denomID, account string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	next, err := types.SafeAdd(k.GetBalance(ctx, denomID, account), amount)
	if err != nil {
		return err
	}
	nextSupply, err := types.SafeAdd(k.GetSupply(ctx, denomID), amount)
	if err != nil {
		return err
	}
	if err := k.setBalance(ctx, denomID, account, next); err != nil {
		return err
	}
	return k.setSupply(ctx, denomID, nextSupply)
}

// debit decreases balance and supply atomically (burn path).
func (k Keeper) debit(ctx context.Context, denomID, account string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	next, err := types.SafeSub(k.GetBalance(ctx, denomID, account), amount)
	if err != nil {
		return types.ErrInsufficientFunds.Wrapf("%s/%s", denomID, account)
	}
	nextSupply, err := types.SafeSub(k.GetSupply(ctx, denomID), amount)
	if err != nil {
		return err
	}
	if err := k.setBalance(ctx, denomID, account, next); err != nil {
		return err
	}
	return k.setSupply(ctx, denomID, nextSupply)
}

// move transfers between accounts without changing supply.
func (k Keeper) move(ctx context.Context, denomID, from, to string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	src := k.GetBalance(ctx, denomID, from)
	if src < amount {
		return types.ErrInsufficientFunds.Wrapf("%s/%s have %d need %d", denomID, from, src, amount)
	}
	dst, err := types.SafeAdd(k.GetBalance(ctx, denomID, to), amount)
	if err != nil {
		return err
	}
	if err := k.setBalance(ctx, denomID, from, src-amount); err != nil {
		return err
	}
	return k.setBalance(ctx, denomID, to, dst)
}

// MoveBalance is the cross-module payout entrypoint (x/rwa dividends,
// x/offering settlements, x/mincast treasury). It enforces denom
// existence, neither party blocked, and source liquidity, but skips the
// full transfer-policy gate — callers run their own holder-side
// compliance. The denom must be usable (ACTIVE) on the receive leg.
func (k Keeper) MoveBalance(ctx context.Context, denomID, from, to string, amount uint64) error {
	if !k.HasDenom(ctx, denomID) {
		return types.ErrNotFound.Wrapf("denom %q", denomID)
	}
	if k.IsAccountBlocked(ctx, denomID, from) || k.IsAccountBlocked(ctx, denomID, to) {
		return types.ErrFrozen
	}
	return k.move(ctx, denomID, from, to, amount)
}

// ---- bridge mint / burn ----------------------------------------------------

// BridgeMint issues `amount` of a settlement denom to `recipient` as the 1:1
// wrapped representation of an inbound cross-chain deposit (USDC/USDT custodied
// off-chain -> native stablecoin). It is NOT a public message: it is reachable
// only through the in-process SettlementKeeper wiring, i.e. x/bridge after an
// attestor quorum has been reached. Unlike the admin MsgMint path it ALWAYS
// enforces reserve coverage (ignoring the global MintRequiresReserve toggle):
// the attested off-chain collateral is the independent ceiling that bounds
// total issuance even if the bridge's attestor quorum is fully subverted, so
// a compromised quorum can never mint beyond the proven reserves.
func (k Keeper) BridgeMint(ctx context.Context, denomID, recipient string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	d, ok := k.GetDenom(ctx, denomID)
	if !ok {
		return types.ErrNotFound.Wrapf("denom %q", denomID)
	}
	// Recipient/denom compliance (status, freeze, sanctions, KYC, policy).
	if err := k.ComplianceCheck(sdkCtx, denomID, "", recipient, amount, false); err != nil {
		return err
	}
	newOutstanding, err := types.SafeAdd(k.GetSupply(ctx, denomID), amount)
	if err != nil {
		return err
	}
	if err := k.ensureBridgeReserve(sdkCtx, d, newOutstanding); err != nil {
		return err
	}
	if err := k.credit(ctx, denomID, recipient, amount); err != nil {
		return err
	}
	k.recordAudit(sdkCtx, "bridge_mint", types.ModuleName, recipient, strconv.FormatUint(amount, 10))
	emitEvent(sdkCtx, types.EventTypeSupply, types.AttrAction, "bridge_mint", types.AttrDenom, denomID,
		types.AttrTo, recipient, types.AttrAmount, strconv.FormatUint(amount, 10))
	return nil
}

// BridgeBurn destroys `amount` of `holder`'s settlement balance as the 1:1
// counterpart of an outbound cross-chain withdrawal (native stablecoin ->
// USDC/USDT released off-chain). Reachable only through SettlementKeeper wiring
// (x/bridge MsgLock). allowPaused=true so a wind-down (PAUSED) denom can still
// be exited; RETIRED and frozen/blacklisted holders are refused.
func (k Keeper) BridgeBurn(ctx context.Context, denomID, holder string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := k.ComplianceCheck(sdkCtx, denomID, holder, "", amount, true); err != nil {
		return err
	}
	if err := k.debit(ctx, denomID, holder, amount); err != nil {
		return err
	}
	k.recordAudit(sdkCtx, "bridge_burn", types.ModuleName, holder, strconv.FormatUint(amount, 10))
	emitEvent(sdkCtx, types.EventTypeSupply, types.AttrAction, "bridge_burn", types.AttrDenom, denomID,
		types.AttrFrom, holder, types.AttrAmount, strconv.FormatUint(amount, 10))
	return nil
}

// ensureBridgeReserve mirrors CheckReserveCoverage but is unconditional: the
// bridge mint path must always be backed by fresh attested reserves at the
// denom's required ratio. Fails closed on an unbound topic, an unwired oracle,
// a missing/stale attestation, or insufficient coverage.
func (k Keeper) ensureBridgeReserve(ctx sdk.Context, d types.StableDenom, newOutstanding uint64) error {
	if d.ReserveTopic == "" {
		return types.ErrReserve.Wrapf("denom %q must bind a reserve topic for bridge mint", d.Id)
	}
	if k.reserve == nil {
		return types.ErrReserve.Wrapf("denom %q reserve oracle not wired", d.Id)
	}
	value, ts, ok := k.reserve.GetTopicValue(ctx, d.ReserveTopic)
	if !ok {
		return types.ErrReserve.Wrapf("denom %q reserve topic %q has no attested value", d.Id, d.ReserveTopic)
	}
	if d.MaxStalenessSeconds > 0 {
		if age := ctx.BlockTime().Unix() - ts; age > d.MaxStalenessSeconds {
			return types.ErrReserve.Wrapf("denom %q reserve stale (age %ds > %ds)", d.Id, age, d.MaxStalenessSeconds)
		}
	}
	ratio := d.RequiredRatioBps
	if ratio == 0 {
		ratio = types.BpsDenominator
	}
	if !types.CoverageOK(newOutstanding, value, ratio) {
		return types.ErrReserve.Wrapf("denom %q coverage insufficient (outstanding %d, reserve %d, ratio %dbps)",
			d.Id, newOutstanding, value, ratio)
	}
	return nil
}

// ---- allowance -------------------------------------------------------------

func (k Keeper) GetAllowance(ctx context.Context, denomID, owner, spender string) uint64 {
	v, err := k.Allowances.Get(ctx, collections.Join3(denomID, owner, spender))
	if err != nil {
		return 0
	}
	return v
}

func (k Keeper) setAllowance(ctx context.Context, denomID, owner, spender string, amount uint64) error {
	key := collections.Join3(denomID, owner, spender)
	if amount == 0 {
		return k.Allowances.Remove(ctx, key)
	}
	return k.Allowances.Set(ctx, key, amount)
}

// ---- flags -----------------------------------------------------------------

func (k Keeper) GetFlags(ctx context.Context, denomID, account string) types.AccountFlags {
	f, err := k.Flags.Get(ctx, collections.Join(denomID, account))
	if err != nil {
		return types.AccountFlags{DenomId: denomID, Account: account}
	}
	return f
}

func (k Keeper) setFlags(ctx context.Context, f types.AccountFlags) error {
	key := collections.Join(f.DenomId, f.Account)
	if !f.Frozen && !f.Blacklisted {
		return k.Flags.Remove(ctx, key)
	}
	return k.Flags.Set(ctx, key, f)
}

func (k Keeper) IsAccountBlocked(ctx context.Context, denomID, account string) bool {
	f := k.GetFlags(ctx, denomID, account)
	return f.Frozen || f.Blacklisted
}

// ---- compliance ------------------------------------------------------------

// ComplianceCheck is the single chokepoint for every transfer-like op.
// Fail-closed order: denom status, per-denom flags (both parties),
// sanctions (both), KYC (both, when params.RequireKyc), and the bound
// x/identity transfer policy. allowPaused lets burn / force / cancel
// paths drain a PAUSED denom; RETIRED always refuses.
func (k Keeper) ComplianceCheck(ctx sdk.Context, denomID, from, to string, amount uint64, allowPaused bool) error {
	d, ok := k.GetDenom(ctx, denomID)
	if !ok {
		return types.ErrNotFound.Wrapf("denom %q", denomID)
	}
	switch d.Status {
	case types.DenomStatus_DENOM_STATUS_RETIRED:
		return types.ErrDenomState.Wrapf("denom %q retired", denomID)
	case types.DenomStatus_DENOM_STATUS_PAUSED:
		if !allowPaused {
			return types.ErrDenomState.Wrapf("denom %q paused", denomID)
		}
	}

	for _, party := range []string{from, to} {
		if party == "" {
			continue
		}
		f := k.GetFlags(ctx, denomID, party)
		if f.Blacklisted {
			return types.ErrFrozen.Wrapf("%s blacklisted on %s", party, denomID)
		}
		if f.Frozen {
			return types.ErrFrozen.Wrapf("%s frozen on %s", party, denomID)
		}
	}

	if k.compliance != nil {
		for _, party := range []string{from, to} {
			if party != "" && k.compliance.IsSanctioned(ctx, party) {
				return types.ErrCompliance.Wrapf("%s sanctioned", party)
			}
		}
		if k.GetParams(ctx).RequireKyc {
			for _, party := range []string{from, to} {
				if party == "" {
					continue
				}
				if err := k.compliance.RequireKYC(ctx, party); err != nil {
					return types.ErrCompliance.Wrapf("%s: %v", party, err)
				}
			}
		}
		if d.PolicyId != "" {
			if err := k.compliance.EvaluateTransfer(ctx, types.ModuleName, denomID, d.PolicyId, from, to, amount); err != nil {
				return types.ErrCompliance.Wrap(err.Error())
			}
		}
	}
	return nil
}

// ---- reserve gate ----------------------------------------------------------

// CheckReserveCoverage refuses a mint when params.MintRequiresReserve and
// the denom binds a reserve topic, unless the attested reserve covers the
// post-mint outstanding at required_ratio_bps and is not stale. Fails
// closed on missing topic / value / oracle wiring.
func (k Keeper) CheckReserveCoverage(ctx sdk.Context, d types.StableDenom, newOutstanding uint64) error {
	if !k.GetParams(ctx).MintRequiresReserve {
		return nil
	}
	if d.ReserveTopic == "" {
		return types.ErrReserve.Wrapf("denom %q requires reserve but none bound", d.Id)
	}
	if k.reserve == nil {
		return types.ErrReserve.Wrapf("denom %q requires reserve but oracle not wired", d.Id)
	}
	value, ts, ok := k.reserve.GetTopicValue(ctx, d.ReserveTopic)
	if !ok {
		return types.ErrReserve.Wrapf("denom %q reserve topic %q has no attested value", d.Id, d.ReserveTopic)
	}
	if d.MaxStalenessSeconds > 0 {
		if age := ctx.BlockTime().Unix() - ts; age > d.MaxStalenessSeconds {
			return types.ErrReserve.Wrapf("denom %q reserve stale (age %ds > %ds)", d.Id, age, d.MaxStalenessSeconds)
		}
	}
	ratio := d.RequiredRatioBps
	if ratio == 0 {
		ratio = types.BpsDenominator
	}
	if !types.CoverageOK(newOutstanding, value, ratio) {
		return types.ErrReserve.Wrapf("denom %q coverage insufficient (outstanding %d, reserve %d, ratio %dbps)",
			d.Id, newOutstanding, value, ratio)
	}
	return nil
}

// ---- redemption ------------------------------------------------------------

func (k Keeper) GetRedemption(ctx context.Context, id uint64) (types.Redemption, bool) {
	r, err := k.Redemptions.Get(ctx, id)
	if err != nil {
		return types.Redemption{}, false
	}
	return r, true
}

func (k Keeper) setRedemption(ctx context.Context, r types.Redemption) error {
	if err := k.Redemptions.Set(ctx, r.Id, r); err != nil {
		return err
	}
	if r.Status == types.RedemptionStatus_REDEMPTION_STATUS_PENDING {
		return k.RedemptionByHolder.Set(ctx, collections.Join(r.Holder, r.Id))
	}
	return k.RedemptionByHolder.Remove(ctx, collections.Join(r.Holder, r.Id))
}

func (k Keeper) countPendingRedemptions(ctx context.Context, holder string) (uint32, error) {
	rng := collections.NewPrefixedPairRange[string, uint64](holder)
	it, err := k.RedemptionByHolder.Iterate(ctx, rng)
	if err != nil {
		return 0, err
	}
	defer it.Close()
	var n uint32
	for ; it.Valid(); it.Next() {
		n++
	}
	return n, nil
}

// ---- audit -----------------------------------------------------------------

func (k Keeper) recordAudit(ctx sdk.Context, action, actor, subject, detail string) {
	if k.compliance != nil {
		k.compliance.RecordAction(ctx, types.ModuleName, action, actor, subject, detail)
	}
}
