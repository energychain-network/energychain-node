package keeper

import (
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"energychain/x/oracle/types"
)

// ModuleAccountName is the bech32-derivable name of the module account
// that holds escrowed provider bonds. Wired in app.go via maccPerms.
const ModuleAccountName = types.ModuleName

// Keeper is the new (M1) oracle keeper. It models Topics, bonded
// Providers, per-(topic, provider) Submissions, deterministic
// AggregatedValues written by EndBlock, and a separate ReserveAttestation
// stream for the PoR sub-interface consumed by x/stablecoin.
type Keeper struct {
	cdc          codec.Codec
	storeService corestore.KVStoreService
	authority    string

	bank types.BankKeeper

	Schema collections.Schema

	Params               collections.Item[types.Params]
	Topics               collections.Map[string, types.Topic]
	Providers            collections.Map[string, types.Provider]
	Submissions          collections.Map[collections.Pair[string, string], types.Submission]
	Aggregated           collections.Map[string, types.AggregatedValue]
	ReserveAttestations  collections.Map[string, types.ReserveAttestation]
	ReserveAttByAsset    collections.KeySet[collections.Pair[string, string]]
	BondReleaseQueue     collections.KeySet[collections.Pair[int64, string]]
}

func NewKeeper(
	cdc codec.Codec,
	storeService corestore.KVStoreService,
	authority string,
	bank types.BankKeeper,
) Keeper {
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		bank:         bank,

		Params: collections.NewItem(sb, types.ParamsCollectionPrefix, "params",
			codec.CollValue[types.Params](cdc)),
		Topics: collections.NewMap(sb, types.TopicCollectionPrefix, "topics",
			collections.StringKey, codec.CollValue[types.Topic](cdc)),
		Providers: collections.NewMap(sb, types.ProviderCollectionPrefix, "providers",
			collections.StringKey, codec.CollValue[types.Provider](cdc)),
		Submissions: collections.NewMap(sb, types.SubmissionCollectionPrefix, "submissions",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
			codec.CollValue[types.Submission](cdc)),
		Aggregated: collections.NewMap(sb, types.AggregatedCollectionPrefix, "aggregated",
			collections.StringKey, codec.CollValue[types.AggregatedValue](cdc)),
		ReserveAttestations: collections.NewMap(sb, types.ReserveAttestationCollectionPrefix, "reserve_attestations",
			collections.StringKey, codec.CollValue[types.ReserveAttestation](cdc)),
		ReserveAttByAsset: collections.NewKeySet(sb, types.ReserveAttByAssetPrefix, "reserve_att_by_asset",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey)),
		BondReleaseQueue: collections.NewKeySet(sb, types.BondReleaseQueuePrefix, "bond_release_queue",
			collections.PairKeyCodec(collections.Int64Key, collections.StringKey)),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(fmt.Errorf("oracle keeper: build collections schema: %w", err))
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// ModuleAddress returns the bech32 address of the oracle module account
// that escrows provider bonds.
func (k Keeper) ModuleAddress() sdk.AccAddress {
	return authtypes.NewModuleAddress(ModuleAccountName)
}

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (k Keeper) SetParams(ctx sdk.Context, p types.Params) error {
	return k.Params.Set(ctx, p)
}

func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	p, err := k.Params.Get(ctx)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("oracle params decode", "err", err)
		}
		return types.DefaultParams()
	}
	return p
}

// ---------------------------------------------------------------------------
// Topics
// ---------------------------------------------------------------------------

func (k Keeper) SetTopic(ctx sdk.Context, t types.Topic) error {
	return k.Topics.Set(ctx, t.Id, t)
}

func (k Keeper) GetTopic(ctx sdk.Context, id string) (types.Topic, bool) {
	t, err := k.Topics.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("oracle topic decode", "id", id, "err", err)
		}
		return types.Topic{}, false
	}
	return t, true
}

func (k Keeper) HasTopic(ctx sdk.Context, id string) bool {
	has, _ := k.Topics.Has(ctx, id)
	return has
}

func (k Keeper) walkTopics(ctx sdk.Context, fn func(types.Topic) (stop bool)) error {
	return k.Topics.Walk(ctx, nil, func(_ string, t types.Topic) (bool, error) {
		return fn(t), nil
	})
}

// ---------------------------------------------------------------------------
// Providers
// ---------------------------------------------------------------------------

func (k Keeper) SetProvider(ctx sdk.Context, p types.Provider) error {
	return k.Providers.Set(ctx, p.Address, p)
}

func (k Keeper) GetProvider(ctx sdk.Context, addr string) (types.Provider, bool) {
	p, err := k.Providers.Get(ctx, addr)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("oracle provider decode", "addr", addr, "err", err)
		}
		return types.Provider{}, false
	}
	return p, true
}

// IsActiveProvider returns true when the address is registered, ACTIVE,
// and (if topic_id non-empty) allowed to submit to that topic per the
// topic's allow_list.
func (k Keeper) IsActiveProvider(ctx sdk.Context, addr, topicID string) bool {
	p, ok := k.GetProvider(ctx, addr)
	if !ok || p.Status != types.ProviderStatus_PROVIDER_STATUS_ACTIVE {
		return false
	}
	if topicID == "" {
		return true
	}
	t, ok := k.GetTopic(ctx, topicID)
	if !ok {
		return false
	}
	if len(t.AllowList) == 0 {
		return true
	}
	for _, a := range t.AllowList {
		if a == addr {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Submissions
// ---------------------------------------------------------------------------

func (k Keeper) SetSubmission(ctx sdk.Context, s types.Submission) error {
	return k.Submissions.Set(ctx, collections.Join(s.TopicId, s.Provider), s)
}

func (k Keeper) GetSubmission(ctx sdk.Context, topicID, provider string) (types.Submission, bool) {
	s, err := k.Submissions.Get(ctx, collections.Join(topicID, provider))
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("oracle submission decode", "err", err)
		}
		return types.Submission{}, false
	}
	return s, true
}

// ---------------------------------------------------------------------------
// Aggregated values
// ---------------------------------------------------------------------------

func (k Keeper) SetAggregated(ctx sdk.Context, v types.AggregatedValue) error {
	return k.Aggregated.Set(ctx, v.TopicId, v)
}

func (k Keeper) GetAggregated(ctx sdk.Context, topicID string) (types.AggregatedValue, bool) {
	v, err := k.Aggregated.Get(ctx, topicID)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("oracle aggregated decode", "topic", topicID, "err", err)
		}
		return types.AggregatedValue{}, false
	}
	return v, true
}

// ---------------------------------------------------------------------------
// Reserve attestations
// ---------------------------------------------------------------------------

func (k Keeper) SetReserveAttestation(ctx sdk.Context, r types.ReserveAttestation) error {
	if err := k.ReserveAttestations.Set(ctx, r.Id, r); err != nil {
		return err
	}
	return k.ReserveAttByAsset.Set(ctx, collections.Join(r.Asset, r.Id))
}

func (k Keeper) GetReserveAttestation(ctx sdk.Context, id string) (types.ReserveAttestation, bool) {
	r, err := k.ReserveAttestations.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("oracle reserve decode", "id", id, "err", err)
		}
		return types.ReserveAttestation{}, false
	}
	return r, true
}

// ---------------------------------------------------------------------------
// Bond release queue
// ---------------------------------------------------------------------------

func (k Keeper) EnqueueBondRelease(ctx sdk.Context, height int64, addr string) error {
	return k.BondReleaseQueue.Set(ctx, collections.Join(height, addr))
}

func (k Keeper) DequeueBondRelease(ctx sdk.Context, height int64, addr string) error {
	return k.BondReleaseQueue.Remove(ctx, collections.Join(height, addr))
}

// ---------------------------------------------------------------------------
// Bank wrappers — escrow / release of bonds.
// ---------------------------------------------------------------------------

// EscrowFrom moves funds from the provider to the oracle module account.
// nil-safe: when bank is unwired (test mode) the escrow is a no-op so
// keeper-level unit tests can model bond changes without standing up
// x/bank.
func (k Keeper) EscrowFrom(ctx sdk.Context, from sdk.AccAddress, amt sdk.Coins) error {
	if k.bank == nil {
		return nil
	}
	return k.bank.SendCoinsFromAccountToModule(ctx, from, ModuleAccountName, amt)
}

func (k Keeper) ReleaseTo(ctx sdk.Context, to sdk.AccAddress, amt sdk.Coins) error {
	if k.bank == nil {
		return nil
	}
	return k.bank.SendCoinsFromModuleToAccount(ctx, ModuleAccountName, to, amt)
}

// ---------------------------------------------------------------------------
// Genesis
// ---------------------------------------------------------------------------

func (k Keeper) InitGenesis(ctx sdk.Context, gs types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return fmt.Errorf("set params: %w", err)
	}
	for i, t := range gs.Topics {
		if err := k.SetTopic(ctx, t); err != nil {
			return fmt.Errorf("topic %d: %w", i, err)
		}
	}
	for i, p := range gs.Providers {
		if err := k.SetProvider(ctx, p); err != nil {
			return fmt.Errorf("provider %d: %w", i, err)
		}
		if p.Status == types.ProviderStatus_PROVIDER_STATUS_WITHDRAWING && p.BondReleaseHeight > 0 {
			if err := k.EnqueueBondRelease(ctx, p.BondReleaseHeight, p.Address); err != nil {
				return fmt.Errorf("provider %d bond queue: %w", i, err)
			}
		}
	}
	for i, s := range gs.Submissions {
		if err := k.SetSubmission(ctx, s); err != nil {
			return fmt.Errorf("submission %d: %w", i, err)
		}
	}
	for i, a := range gs.Aggregated {
		if err := k.SetAggregated(ctx, a); err != nil {
			return fmt.Errorf("aggregated %d: %w", i, err)
		}
	}
	for i, r := range gs.ReserveAttestations {
		if err := k.SetReserveAttestation(ctx, r); err != nil {
			return fmt.Errorf("reserve %d: %w", i, err)
		}
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	gs := types.DefaultGenesis()
	gs.Params = k.GetParams(ctx)

	_ = k.Topics.Walk(ctx, nil, func(_ string, t types.Topic) (bool, error) {
		gs.Topics = append(gs.Topics, t)
		return false, nil
	})
	_ = k.Providers.Walk(ctx, nil, func(_ string, p types.Provider) (bool, error) {
		gs.Providers = append(gs.Providers, p)
		return false, nil
	})
	_ = k.Submissions.Walk(ctx, nil, func(_ collections.Pair[string, string], s types.Submission) (bool, error) {
		gs.Submissions = append(gs.Submissions, s)
		return false, nil
	})
	_ = k.Aggregated.Walk(ctx, nil, func(_ string, a types.AggregatedValue) (bool, error) {
		gs.Aggregated = append(gs.Aggregated, a)
		return false, nil
	})
	_ = k.ReserveAttestations.Walk(ctx, nil, func(_ string, r types.ReserveAttestation) (bool, error) {
		gs.ReserveAttestations = append(gs.ReserveAttestations, r)
		return false, nil
	})
	return gs
}
