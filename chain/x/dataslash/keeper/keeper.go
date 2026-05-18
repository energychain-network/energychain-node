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

	"energychain/x/dataslash/types"
)

// DataslashPoolAccount holds every provider's bond and any
// slashed remainder (which stays in the pool until a
// governance-controlled treasury sweep distributes it). Using
// authtypes.NewModuleAddress makes the address chain-
// independent.
var DataslashPoolAccount = authtypes.NewModuleAddress("dataslash_pool").String()

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	sanctions  types.SanctionsKeeper
	stablecoin types.StablecoinKeeper
	audit      types.AuditKeeper

	Schema collections.Schema

	Params collections.Item[types.Params]

	Providers          collections.Map[uint64, types.Provider]
	ProviderIDSeq      collections.Sequence
	ProviderByDID      collections.Map[string, uint64]
	ProviderBySigner   collections.Map[string, uint64]
	ProviderByStatus   collections.KeySet[collections.Pair[uint32, uint64]]
	ProviderByJailUntil  collections.KeySet[collections.Pair[int64, uint64]]
	ProviderByWithdrawAt collections.KeySet[collections.Pair[int64, uint64]]

	Infractions          collections.Map[uint64, types.Infraction]
	InfractionIDSeq      collections.Sequence
	InfractionByProvider collections.KeySet[collections.Pair[uint64, uint64]]
	InfractionByKind     collections.KeySet[collections.Pair[uint32, uint64]]

	JailRecords          collections.Map[uint64, types.JailRecord]
	JailRecordIDSeq      collections.Sequence
	JailRecordByProvider collections.KeySet[collections.Pair[uint64, uint64]]

	BanRecords          collections.Map[uint64, types.BanRecord]
	BanRecordIDSeq      collections.Sequence
	BanRecordByProvider collections.KeySet[collections.Pair[uint64, uint64]]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	sanctions types.SanctionsKeeper,
	stablecoin types.StablecoinKeeper,
	audit types.AuditKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("dataslash: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc: cdc, storeService: storeService, authority: authority,
		sanctions: sanctions, stablecoin: stablecoin, audit: audit,

		Params: collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),

		Providers:     collections.NewMap(sb, types.ProviderCollectionPrefix, "providers", collections.Uint64Key, codec.CollValue[types.Provider](cdc)),
		ProviderIDSeq: collections.NewSequence(sb, types.ProviderIDSeqPrefix, "provider_id_seq"),
		ProviderByDID: collections.NewMap(sb, types.ProviderByDIDPrefix, "provider_by_did",
			collections.StringKey, collections.Uint64Value),
		ProviderBySigner: collections.NewMap(sb, types.ProviderBySignerPrefix, "provider_by_signer",
			collections.StringKey, collections.Uint64Value),
		ProviderByStatus: collections.NewKeySet(sb, types.ProviderByStatusPrefix, "provider_by_status",
			collections.PairKeyCodec(collections.Uint32Key, collections.Uint64Key)),
		ProviderByJailUntil: collections.NewKeySet(sb, types.ProviderByJailUntilPrefix, "provider_by_jail_until",
			collections.PairKeyCodec(collections.Int64Key, collections.Uint64Key)),
		ProviderByWithdrawAt: collections.NewKeySet(sb, types.ProviderByWithdrawAtPrefix, "provider_by_withdraw_at",
			collections.PairKeyCodec(collections.Int64Key, collections.Uint64Key)),

		Infractions:     collections.NewMap(sb, types.InfractionCollectionPrefix, "infractions", collections.Uint64Key, codec.CollValue[types.Infraction](cdc)),
		InfractionIDSeq: collections.NewSequence(sb, types.InfractionIDSeqPrefix, "infraction_id_seq"),
		InfractionByProvider: collections.NewKeySet(sb, types.InfractionByProviderPrefix, "infraction_by_provider",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),
		InfractionByKind: collections.NewKeySet(sb, types.InfractionByKindPrefix, "infraction_by_kind",
			collections.PairKeyCodec(collections.Uint32Key, collections.Uint64Key)),

		JailRecords:     collections.NewMap(sb, types.JailRecordCollectionPrefix, "jail_records", collections.Uint64Key, codec.CollValue[types.JailRecord](cdc)),
		JailRecordIDSeq: collections.NewSequence(sb, types.JailRecordIDSeqPrefix, "jail_record_id_seq"),
		JailRecordByProvider: collections.NewKeySet(sb, types.JailRecordByProviderPrefix, "jail_record_by_provider",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),

		BanRecords:     collections.NewMap(sb, types.BanRecordCollectionPrefix, "ban_records", collections.Uint64Key, codec.CollValue[types.BanRecord](cdc)),
		BanRecordIDSeq: collections.NewSequence(sb, types.BanRecordIDSeqPrefix, "ban_record_id_seq"),
		BanRecordByProvider: collections.NewKeySet(sb, types.BanRecordByProviderPrefix, "ban_record_by_provider",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),
	}
	sch, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = sch
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }
func (k Keeper) PoolAddress() string  { return DataslashPoolAccount }

// ---- Params -----------------------------------------------------------

func (k Keeper) SetParams(ctx context.Context, p types.Params) error { return k.Params.Set(ctx, p) }
func (k Keeper) GetParams(ctx context.Context) (types.Params, error) {
	v, err := k.Params.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.DefaultParams(), nil
		}
		return types.Params{}, err
	}
	return v, nil
}

// ---- Sequences --------------------------------------------------------

func (k Keeper) NextProviderID(ctx context.Context) (uint64, error)   { return k.next(ctx, k.ProviderIDSeq) }
func (k Keeper) NextInfractionID(ctx context.Context) (uint64, error) { return k.next(ctx, k.InfractionIDSeq) }
func (k Keeper) NextJailRecordID(ctx context.Context) (uint64, error) { return k.next(ctx, k.JailRecordIDSeq) }
func (k Keeper) NextBanRecordID(ctx context.Context) (uint64, error)  { return k.next(ctx, k.BanRecordIDSeq) }
func (k Keeper) next(ctx context.Context, seq collections.Sequence) (uint64, error) {
	v, err := seq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return v + 1, nil
}

// ---- Providers --------------------------------------------------------

// SetProvider persists a provider and keeps its covering
// indexes in sync. Indexes touched:
//   - by_status:        moves between status buckets
//   - by_jail_until:    removed when leaving JAILED, added on JAIL
//   - by_withdraw_at:   removed when leaving UNBONDING, added on REQUEST_UNBOND
//
// Caller is responsible for setting the provider's UpdatedAt
// before calling.
func (k Keeper) SetProvider(ctx context.Context, p types.Provider) error {
	prev, ok, err := k.GetProvider(ctx, p.Id)
	if err != nil {
		return err
	}
	if ok {
		if prev.Status != p.Status {
			if err := k.ProviderByStatus.Remove(ctx, collections.Join(uint32(prev.Status), p.Id)); err != nil {
				return err
			}
		}
		if prev.Status == types.ProviderStatus_PROVIDER_STATUS_JAILED && p.Status != types.ProviderStatus_PROVIDER_STATUS_JAILED {
			if err := k.ProviderByJailUntil.Remove(ctx, collections.Join(prev.JailUntil, p.Id)); err != nil {
				return err
			}
		}
		if prev.Status == types.ProviderStatus_PROVIDER_STATUS_UNBONDING && p.Status != types.ProviderStatus_PROVIDER_STATUS_UNBONDING {
			if err := k.ProviderByWithdrawAt.Remove(ctx, collections.Join(prev.WithdrawAt, p.Id)); err != nil {
				return err
			}
		}
	}
	if err := k.Providers.Set(ctx, p.Id, p); err != nil {
		return err
	}
	if err := k.ProviderByStatus.Set(ctx, collections.Join(uint32(p.Status), p.Id)); err != nil {
		return err
	}
	if p.Status == types.ProviderStatus_PROVIDER_STATUS_JAILED && p.JailUntil > 0 {
		if err := k.ProviderByJailUntil.Set(ctx, collections.Join(p.JailUntil, p.Id)); err != nil {
			return err
		}
	}
	if p.Status == types.ProviderStatus_PROVIDER_STATUS_UNBONDING && p.WithdrawAt > 0 {
		if err := k.ProviderByWithdrawAt.Set(ctx, collections.Join(p.WithdrawAt, p.Id)); err != nil {
			return err
		}
	}
	return nil
}

func (k Keeper) GetProvider(ctx context.Context, id uint64) (types.Provider, bool, error) {
	v, err := k.Providers.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Provider{}, false, nil
		}
		return types.Provider{}, false, err
	}
	return v, true, nil
}
func (k Keeper) MustGetProvider(ctx context.Context, id uint64) (types.Provider, error) {
	v, ok, err := k.GetProvider(ctx, id)
	if err != nil {
		return v, err
	}
	if !ok {
		return v, fmt.Errorf("provider %d not found", id)
	}
	return v, nil
}
func (k Keeper) GetProviderByDID(ctx context.Context, did string) (uint64, bool, error) {
	v, err := k.ProviderByDID.Get(ctx, did)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return v, true, nil
}
func (k Keeper) GetProviderBySigner(ctx context.Context, addr string) (uint64, bool, error) {
	v, err := k.ProviderBySigner.Get(ctx, addr)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return v, true, nil
}

// ---- Compliance hooks -------------------------------------------------

func (k Keeper) requireUnsanctioned(ctx context.Context, who string) error {
	if k.sanctions == nil {
		return nil
	}
	if k.sanctions.IsSanctioned(sdk.UnwrapSDKContext(ctx), who) {
		return fmt.Errorf("address %s is sanctioned", who)
	}
	return nil
}

func (k Keeper) recordAudit(ctx context.Context, providerID uint64, action, actor, subject, detail string) {
	if k.audit == nil {
		return
	}
	k.audit.RecordDataslashAction(sdk.UnwrapSDKContext(ctx), providerID, action, actor, subject, detail)
}

func (k Keeper) requireStablecoin(ctx context.Context, denom string) error {
	if k.stablecoin == nil {
		return fmt.Errorf("stablecoin keeper not wired; dataslash bonds disabled")
	}
	if !k.stablecoin.HasDenom(sdk.UnwrapSDKContext(ctx), denom) {
		return fmt.Errorf("bond_denom %q not registered", denom)
	}
	if k.stablecoin.IsDenomPaused(sdk.UnwrapSDKContext(ctx), denom) {
		return fmt.Errorf("bond_denom %q is paused", denom)
	}
	return nil
}

// moveBond re-checks per-account compliance before invoking
// the stablecoin Move primitive. Pool-account legs skip the
// blocked-check since the pool is module-derived.
func (k Keeper) moveBond(ctx context.Context, denom, from, to string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	if k.stablecoin == nil {
		return fmt.Errorf("stablecoin keeper not wired")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if from != DataslashPoolAccount {
		if k.stablecoin.IsAccountBlocked(sdkCtx, denom, from) {
			return fmt.Errorf("from %s blocked for denom %q", from, denom)
		}
	}
	if to != DataslashPoolAccount {
		if k.stablecoin.IsAccountBlocked(sdkCtx, denom, to) {
			return fmt.Errorf("to %s blocked for denom %q", to, denom)
		}
	}
	return k.stablecoin.Move(sdkCtx, denom, from, to, amount)
}

// tryRefundOrForfeit mirrors the x/dispute helper: best-
// effort transfer used by EndBlock paths only. When the
// recipient is blocked / the denom is paused, the residual
// stays in the pool and the dispute / withdraw still
// completes — prevents a blocked counterparty from wedging
// the sweep budget.
func (k Keeper) tryRefundOrForfeit(ctx context.Context, providerID uint64, denom, to string, amount uint64, leg string) {
	if amount == 0 {
		return
	}
	if err := k.moveBond(ctx, denom, DataslashPoolAccount, to, amount); err != nil {
		sdkCtx := sdk.UnwrapSDKContext(ctx)
		sdkCtx.Logger().Info("dataslash: forfeit unrefundable bond",
			"provider_id", providerID, "to", to, "amount", amount, "leg", leg, "err", err)
		k.emit(ctx, "dataslash.bond.forfeited",
			sdk.NewAttribute("provider_id", u64s(providerID)),
			sdk.NewAttribute("to", to),
			sdk.NewAttribute("amount", u64s(amount)),
			sdk.NewAttribute("denom", denom),
			sdk.NewAttribute("leg", leg),
			sdk.NewAttribute("reason", err.Error()),
		)
		k.recordAudit(ctx, providerID, "dataslash.bond_forfeit", k.authority, to, err.Error())
	}
}

// ---- Counts -----------------------------------------------------------

// countProviders uses the sequence as the running total — a
// provider row is never physically deleted (BANNED/WITHDRAWN
// are terminal flips), so the sequence is the exact count of
// rows ever created.
func (k Keeper) countProviders(ctx context.Context) (uint64, error) {
	return k.ProviderIDSeq.Peek(ctx)
}

// IsActiveSigner is the convenience hook upstream modules
// (oracle / meter / bridge) call before accepting data.
// Returns (provider_id, status, ok). ok is true only when a
// provider exists for the address; the caller decides what
// to do with the status (typically: refuse unless ACTIVE and
// bond_amount >= required_bond).
func (k Keeper) IsActiveSigner(ctx context.Context, signer string, expectedRole types.ProviderRole) (uint64, types.Provider, bool, error) {
	id, ok, err := k.GetProviderBySigner(ctx, signer)
	if err != nil {
		return 0, types.Provider{}, false, err
	}
	if !ok {
		return 0, types.Provider{}, false, nil
	}
	p, err := k.MustGetProvider(ctx, id)
	if err != nil {
		return 0, types.Provider{}, false, err
	}
	if expectedRole != types.ProviderRole_PROVIDER_ROLE_UNSPECIFIED && p.Role != expectedRole {
		return id, p, false, nil
	}
	if !types.ProviderIsActiveForData(p.Status) {
		return id, p, false, nil
	}
	if p.BondAmount < p.RequiredBond {
		return id, p, false, nil
	}
	return id, p, true, nil
}
