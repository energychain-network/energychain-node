package keeper

import (
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/identity/types"
)

// Keeper holds the declarative collections backing the identity module's
// persistent state.
type Keeper struct {
	cdc          codec.Codec
	storeService corestore.KVStoreService
	authority    string

	Schema collections.Schema

	Params      collections.Item[types.Params]
	Identities  collections.Map[string, types.Identity]
	ByRole      collections.KeySet[collections.Pair[string, string]]
}

func NewKeeper(cdc codec.Codec, storeService corestore.KVStoreService, authority string) Keeper {
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,

		Params: collections.NewItem(
			sb,
			types.ParamsCollectionPrefix,
			"params",
			codec.CollValue[types.Params](cdc),
		),
		Identities: collections.NewMap(
			sb,
			types.IdentityCollectionPrefix,
			"identities",
			collections.StringKey,
			codec.CollValue[types.Identity](cdc),
		),
		ByRole: collections.NewKeySet(
			sb,
			types.IdentityByRoleCollectionPrefix,
			"by_role",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
		),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(fmt.Errorf("identity keeper: build collections schema: %w", err))
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string {
	return k.authority
}

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (k Keeper) SetParams(ctx sdk.Context, params types.Params) error {
	if err := k.Params.Set(ctx, params); err != nil {
		return fmt.Errorf("store identity params: %w", err)
	}
	return nil
}

func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	params, err := k.Params.Get(ctx)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("identity params decode failed", "err", err)
		}
		return types.DefaultParams()
	}
	return params
}

// ---------------------------------------------------------------------------
// Identity CRUD
// ---------------------------------------------------------------------------

func (k Keeper) SetIdentity(ctx sdk.Context, identity types.Identity) error {
	if err := k.Identities.Set(ctx, identity.Address, identity); err != nil {
		return fmt.Errorf("store identity %s: %w", identity.Address, err)
	}
	if err := k.ByRole.Set(ctx, collections.Join(identity.Role, identity.Address)); err != nil {
		return fmt.Errorf("index identity by role: %w", err)
	}
	return nil
}

// GetIdentity returns (identity, true) on success or zero value with false
// when the address is unknown or the stored value is corrupted.
func (k Keeper) GetIdentity(ctx sdk.Context, address string) (types.Identity, bool) {
	identity, err := k.Identities.Get(ctx, address)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("identity decode failed", "address", address, "err", err)
		}
		return types.Identity{}, false
	}
	return identity, true
}

func (k Keeper) DeleteIdentity(ctx sdk.Context, address string) {
	identity, found := k.GetIdentity(ctx, address)
	if !found {
		return
	}
	if err := k.Identities.Remove(ctx, address); err != nil {
		ctx.Logger().Error("identity remove failed", "address", address, "err", err)
		return
	}
	if err := k.ByRole.Remove(ctx, collections.Join(identity.Role, address)); err != nil {
		ctx.Logger().Error("identity role-index remove failed", "address", address, "role", identity.Role, "err", err)
	}
}

// ---------------------------------------------------------------------------
// Iteration helpers
// ---------------------------------------------------------------------------

// MaxQueryResults caps the size of any list response served to RPC.
const MaxQueryResults = 1000

// getAllIdentitiesUnbounded is for genesis export only.
func (k Keeper) getAllIdentitiesUnbounded(ctx sdk.Context) []types.Identity {
	var out []types.Identity
	if err := k.Identities.Walk(ctx, nil, func(_ string, v types.Identity) (bool, error) {
		out = append(out, v)
		return false, nil
	}); err != nil {
		ctx.Logger().Error("identity walk failed", "err", err)
	}
	return out
}

func (k Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	return &types.GenesisState{
		Identities: k.getAllIdentitiesUnbounded(ctx),
		Params:     k.GetParams(ctx),
	}
}

func (k Keeper) IsRegistered(ctx sdk.Context, address string) bool {
	has, err := k.Identities.Has(ctx, address)
	if err != nil {
		ctx.Logger().Error("identity has failed", "address", address, "err", err)
		return false
	}
	return has
}

// HasRole returns true if the address has an active identity with the
// specified role.
func (k Keeper) HasRole(ctx sdk.Context, address, role string) bool {
	identity, found := k.GetIdentity(ctx, address)
	if !found {
		return false
	}
	return identity.Role == role && identity.Status == types.StatusActive
}
