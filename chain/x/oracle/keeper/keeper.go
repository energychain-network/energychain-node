package keeper

import (
	"errors"
	"fmt"
	"math"

	"cosmossdk.io/collections"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/oracle/types"
)

// Keeper holds the declarative collections backing the oracle module's
// persistent state.
type Keeper struct {
	cdc          codec.Codec
	storeService corestore.KVStoreService
	authority    string

	Schema collections.Schema

	// Params holds module governance parameters.
	Params collections.Item[types.Params]

	// Oracles maps an address string to its OracleInfo record.
	Oracles collections.Map[string, types.OracleInfo]

	// Data stores historical datapoints keyed by (category, timestamp).
	// Timestamp is encoded as uint64 so big-endian ordering matches numeric
	// ordering — required for prefix-range scans by time.
	Data collections.Map[collections.Pair[string, uint64], types.OracleData]

	// LatestData holds the most recently written record per category;
	// duplicated for O(1) lookup without scanning the time-series.
	LatestData collections.Map[string, types.OracleData]
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
		Oracles: collections.NewMap(
			sb,
			types.OracleInfoCollectionPrefix,
			"oracles",
			collections.StringKey,
			codec.CollValue[types.OracleInfo](cdc),
		),
		Data: collections.NewMap(
			sb,
			types.OracleDataCollectionPrefix,
			"oracle_data",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key),
			codec.CollValue[types.OracleData](cdc),
		),
		LatestData: collections.NewMap(
			sb,
			types.LatestDataCollectionPrefix,
			"latest_data",
			collections.StringKey,
			codec.CollValue[types.OracleData](cdc),
		),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(fmt.Errorf("oracle keeper: build collections schema: %w", err))
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string {
	return k.authority
}

// ---------------------------------------------------------------------------
// Oracle Data
// ---------------------------------------------------------------------------

// timestampToKey clamps negative timestamps (which should never happen at the
// validation layer, but defending in depth) to 0 so the uint64 cast never
// produces a key that sorts after MaxInt64-aligned legitimate keys.
func timestampToKey(ts int64) uint64 {
	if ts < 0 {
		return 0
	}
	return uint64(ts)
}

func (k Keeper) SetOracleData(ctx sdk.Context, data types.OracleData) error {
	tsKey := timestampToKey(data.Timestamp)
	if err := k.Data.Set(ctx, collections.Join(data.Category, tsKey), data); err != nil {
		return fmt.Errorf("store oracle data: %w", err)
	}
	if err := k.LatestData.Set(ctx, data.Category, data); err != nil {
		return fmt.Errorf("store latest oracle data: %w", err)
	}
	return nil
}

func (k Keeper) GetLatestData(ctx sdk.Context, category string) (types.OracleData, bool) {
	d, err := k.LatestData.Get(ctx, category)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("oracle latest data decode failed", "category", category, "err", err)
		}
		return types.OracleData{}, false
	}
	return d, true
}

// GetFreshLatestData returns the latest stored datapoint for a category only
// when its age relative to the current block time is at most params.DataMaxAge.
// Stale entries are returned with stale=true so callers can decide whether to
// reject or surface them.
func (k Keeper) GetFreshLatestData(ctx sdk.Context, category string) (data types.OracleData, found, stale bool) {
	data, found = k.GetLatestData(ctx, category)
	if !found {
		return data, false, false
	}
	maxAge := k.GetParams(ctx).DataMaxAge
	if maxAge <= 0 {
		return data, true, false
	}
	if ctx.BlockTime().Unix()-data.Timestamp > maxAge {
		return data, true, true
	}
	return data, true, false
}

// MaxHistoryResults caps the number of records returned by a single
// GetDataHistory call.
const MaxHistoryResults = 1000

// GetDataHistory returns at most MaxHistoryResults records for the given
// category whose timestamp falls in [fromTime, toTime] (inclusive). Negative
// or inverted ranges return nil.
func (k Keeper) GetDataHistory(ctx sdk.Context, category string, fromTime, toTime int64) []types.OracleData {
	if category == "" || fromTime < 0 || toTime < fromTime {
		return nil
	}
	endTS := uint64(toTime)
	if endTS < math.MaxUint64 {
		endTS++
	}
	rng := collections.NewPrefixedPairRange[string, uint64](category).
		StartInclusive(uint64(fromTime)).
		EndExclusive(endTS)

	results := make([]types.OracleData, 0, 64)
	if err := k.Data.Walk(ctx, rng, func(_ collections.Pair[string, uint64], v types.OracleData) (bool, error) {
		results = append(results, v)
		if len(results) >= MaxHistoryResults {
			return true, nil
		}
		return false, nil
	}); err != nil {
		ctx.Logger().Error("oracle data history walk failed", "category", category, "err", err)
		return results
	}
	return results
}

// ---------------------------------------------------------------------------
// Oracle Management
// ---------------------------------------------------------------------------

func (k Keeper) AddOracle(ctx sdk.Context, oracle types.OracleInfo) error {
	if err := k.Oracles.Set(ctx, oracle.Address, oracle); err != nil {
		return fmt.Errorf("store oracle info: %w", err)
	}
	return nil
}

func (k Keeper) RemoveOracle(ctx sdk.Context, address string) {
	if err := k.Oracles.Remove(ctx, address); err != nil {
		// Removing a non-existent key is not an error in collections; only
		// log unexpected failures.
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("oracle remove failed", "address", address, "err", err)
		}
	}
}

func (k Keeper) GetOracle(ctx sdk.Context, address string) (types.OracleInfo, bool) {
	info, err := k.Oracles.Get(ctx, address)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("oracle decode failed", "address", address, "err", err)
		}
		return types.OracleInfo{}, false
	}
	return info, true
}

func (k Keeper) IsAuthorizedOracle(ctx sdk.Context, address, category string) bool {
	oracle, found := k.GetOracle(ctx, address)
	if !found {
		return false
	}
	return oracle.IsAuthorizedFor(category)
}

// MaxOracles caps the number of oracle entries returned by GetAllOracles.
const MaxOracles = 1000

func (k Keeper) GetAllOracles(ctx sdk.Context) []types.OracleInfo {
	oracles := make([]types.OracleInfo, 0, 16)
	if err := k.Oracles.Walk(ctx, nil, func(_ string, v types.OracleInfo) (bool, error) {
		oracles = append(oracles, v)
		if len(oracles) >= MaxOracles {
			return true, nil
		}
		return false, nil
	}); err != nil {
		ctx.Logger().Error("oracle walk failed", "err", err)
	}
	return oracles
}

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (k Keeper) SetParams(ctx sdk.Context, params types.Params) error {
	if err := k.Params.Set(ctx, params); err != nil {
		return fmt.Errorf("store oracle params: %w", err)
	}
	return nil
}

func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	params, err := k.Params.Get(ctx)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("oracle params decode failed", "err", err)
		}
		return types.DefaultParams()
	}
	return params
}

// ---------------------------------------------------------------------------
// Genesis helpers
// ---------------------------------------------------------------------------

func (k Keeper) InitGenesis(ctx sdk.Context, gs types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return fmt.Errorf("setting oracle params: %w", err)
	}
	for i, oracle := range gs.Oracles {
		if err := k.AddOracle(ctx, oracle); err != nil {
			return fmt.Errorf("restoring oracle %d: %w", i, err)
		}
	}
	for i, data := range gs.Data {
		if err := k.SetOracleData(ctx, data); err != nil {
			return fmt.Errorf("restoring oracle data %d: %w", i, err)
		}
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	var data []types.OracleData
	if err := k.Data.Walk(ctx, nil, func(_ collections.Pair[string, uint64], v types.OracleData) (bool, error) {
		data = append(data, v)
		return false, nil
	}); err != nil {
		ctx.Logger().Error("oracle data walk failed during export", "err", err)
	}
	return &types.GenesisState{
		Params:  k.GetParams(ctx),
		Oracles: k.GetAllOracles(ctx),
		Data:    data,
	}
}
