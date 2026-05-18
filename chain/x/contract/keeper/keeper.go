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

	"energychain/x/contract/types"
)

// contractMarginPool is the deterministic 20-byte module address
// that holds all in-flight contract margins. Funds debited from
// a party on DepositMargin are credited here; settlements move
// virtually between the in-keeper margin_buyer / margin_seller
// counters without touching the pool. WithdrawMargin and the
// Default slash leg are the only paths back out of the pool.
var contractMarginPool = authtypes.NewModuleAddress("contract_margin_pool").String()

// MarginPoolAddress returns the canonical pool address.
func MarginPoolAddress() string { return contractMarginPool }

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	sanctions  types.SanctionsKeeper
	stablecoin types.StablecoinKeeper
	oracle     types.OracleKeeper
	audit      types.AuditKeeper

	Schema collections.Schema

	Params          collections.Item[types.Params]
	Contracts       collections.Map[uint64, types.Contract]
	IDSeq           collections.Sequence
	ContractByParty collections.KeySet[collections.Pair[string, uint64]]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	sanctions types.SanctionsKeeper,
	stablecoin types.StablecoinKeeper,
	oracle types.OracleKeeper,
	audit types.AuditKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("contract: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc: cdc, storeService: storeService, authority: authority,
		sanctions: sanctions, stablecoin: stablecoin, oracle: oracle, audit: audit,

		Params:    collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Contracts: collections.NewMap(sb, types.ContractCollectionPrefix, "contracts", collections.Uint64Key, codec.CollValue[types.Contract](cdc)),
		IDSeq:     collections.NewSequence(sb, types.ContractIDSeqPrefix, "contract_id_seq"),
		ContractByParty: collections.NewKeySet(sb, types.ContractByPartyPrefix, "contract_by_party",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
	}
	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// ---- params --------------------------------------------------------------

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

// ---- contract CRUD -------------------------------------------------------

func (k Keeper) GetContract(ctx context.Context, id uint64) (types.Contract, bool, error) {
	c, err := k.Contracts.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Contract{}, false, nil
		}
		return types.Contract{}, false, err
	}
	return c, true, nil
}

func (k Keeper) MustGetContract(ctx context.Context, id uint64) (types.Contract, error) {
	c, ok, err := k.GetContract(ctx, id)
	if err != nil {
		return types.Contract{}, err
	}
	if !ok {
		return types.Contract{}, fmt.Errorf("contract %d not found", id)
	}
	return c, nil
}

func (k Keeper) SetContract(ctx context.Context, c types.Contract) error {
	return k.Contracts.Set(ctx, c.Id, c)
}

func (k Keeper) NextContractID(ctx context.Context) (uint64, error) {
	n, err := k.IDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}

func (k Keeper) CountContracts(ctx context.Context) (uint32, error) {
	v, err := k.IDSeq.Peek(ctx)
	if err != nil {
		return 0, err
	}
	if v > uint64(^uint32(0)) {
		return ^uint32(0), nil
	}
	return uint32(v), nil
}

func (k Keeper) CountContractsForParty(ctx context.Context, party string) (uint32, error) {
	rng := collections.NewPrefixedPairRange[string, uint64](party)
	var n uint32
	if err := k.ContractByParty.Walk(ctx, rng, func(_ collections.Pair[string, uint64]) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// ---- pool plumbing -------------------------------------------------------

func (k Keeper) fundPool(ctx context.Context, denom, from string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if k.stablecoin == nil {
		return fmt.Errorf("stablecoin keeper not wired")
	}
	if !k.stablecoin.HasDenom(sdkCtx, denom) {
		return fmt.Errorf("denom %q not registered", denom)
	}
	if k.stablecoin.IsDenomPaused(sdkCtx, denom) {
		return fmt.Errorf("denom %q paused; pay-in refused", denom)
	}
	if k.stablecoin.IsAccountBlocked(sdkCtx, denom, from) {
		return fmt.Errorf("payer %s is frozen / blacklisted on denom %s", from, denom)
	}
	return k.stablecoin.Move(sdkCtx, denom, from, MarginPoolAddress(), amount)
}

func (k Keeper) drainPool(ctx context.Context, denom, to string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if k.stablecoin == nil {
		return fmt.Errorf("stablecoin keeper not wired")
	}
	if k.stablecoin.IsAccountBlocked(sdkCtx, denom, to) {
		return fmt.Errorf("payee %s is frozen / blacklisted on denom %s", to, denom)
	}
	return k.stablecoin.Move(sdkCtx, denom, MarginPoolAddress(), to, amount)
}

func (k Keeper) requireUnsanctioned(ctx context.Context, who, role string) error {
	if k.sanctions == nil {
		return nil
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if k.sanctions.IsSanctioned(sdkCtx, who) {
		return fmt.Errorf("%s %s is sanctioned", role, who)
	}
	return nil
}

func (k Keeper) recordAudit(ctx context.Context, id uint64, action, actor, subject, detail string) {
	if k.audit == nil {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.audit.RecordContractAction(sdkCtx, id, action, actor, subject, detail)
}
