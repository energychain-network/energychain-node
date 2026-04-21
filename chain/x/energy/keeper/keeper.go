package keeper

import (
	"encoding/binary"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	corestore "cosmossdk.io/core/store"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/energy/types"
)

// Keeper holds the declarative collections that back the energy module's
// persistent state. Every persistent KV interaction goes through one of
// the typed collections; raw KV access is reserved for the per-block
// transient store used by rate limiting.
type Keeper struct {
	cdc          codec.Codec
	storeService corestore.KVStoreService
	tStoreKey    storetypes.StoreKey // transient store, wiped per block
	authority    string

	Schema collections.Schema

	// Params holds module governance parameters.
	Params collections.Item[types.Params]

	// IDSequence is the auto-increment counter feeding GenerateID.
	IDSequence collections.Sequence

	// EnergyData stores records keyed by their string ID (e.g. "energy-42").
	EnergyData collections.Map[string, types.EnergyData]

	// ByCategory is the secondary index (category, id). Walk on a fixed
	// category prefix to list every record in that category in deterministic
	// (insertion) order.
	ByCategory collections.KeySet[collections.Pair[string, string]]

	// BySubmitter is the secondary index (submitter, id).
	BySubmitter collections.KeySet[collections.Pair[string, string]]

	// Batches stores batch-submission metadata keyed by batch ID.
	Batches collections.Map[string, types.BatchSubmission]
}

// NewKeeper builds a Keeper. tStoreKey is the per-block transient store used
// by rate-limiting counters; persistent state lives behind storeService.
func NewKeeper(cdc codec.Codec, storeService corestore.KVStoreService, tStoreKey storetypes.StoreKey, authority string) Keeper {
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		tStoreKey:    tStoreKey,
		authority:    authority,

		Params: collections.NewItem(
			sb,
			types.ParamsCollectionPrefix,
			"params",
			codec.CollValue[types.Params](cdc),
		),
		IDSequence: collections.NewSequence(
			sb,
			types.IDSequenceCollectionPrefix,
			"id_sequence",
		),
		EnergyData: collections.NewMap(
			sb,
			types.EnergyDataCollectionPrefix,
			"energy_data",
			collections.StringKey,
			codec.CollValue[types.EnergyData](cdc),
		),
		ByCategory: collections.NewKeySet(
			sb,
			types.ByCategoryCollectionPrefix,
			"by_category",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
		),
		BySubmitter: collections.NewKeySet(
			sb,
			types.BySubmitterCollectionPrefix,
			"by_submitter",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
		),
		Batches: collections.NewMap(
			sb,
			types.BatchCollectionPrefix,
			"batches",
			collections.StringKey,
			codec.CollValue[types.BatchSubmission](cdc),
		),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(fmt.Errorf("energy keeper: build collections schema: %w", err))
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string {
	return k.authority
}

// ---------------------------------------------------------------------------
// ID generation
// ---------------------------------------------------------------------------

// GenerateID returns the next auto-increment ID, formatted as "energy-N".
// Panics on storage errors (treated as state corruption).
func (k Keeper) GenerateID(ctx sdk.Context) string {
	seq, err := k.IDSequence.Next(ctx)
	if err != nil {
		panic(fmt.Errorf("energy keeper: advance id sequence: %w", err))
	}
	// Sequence starts at 0; surface IDs as 1-indexed for human readability.
	return fmt.Sprintf("energy-%d", seq+1)
}

// GetCurrentID returns the most recently issued numeric ID (1-indexed).
// Returns 0 when no IDs have been issued yet.
func (k Keeper) GetCurrentID(ctx sdk.Context) uint64 {
	seq, err := k.IDSequence.Peek(ctx)
	if err != nil {
		return 0
	}
	return seq
}

// SetCurrentID overwrites the sequence so the next GenerateID call returns
// (seq+1). Used by InitGenesis to restore continuity.
func (k Keeper) SetCurrentID(ctx sdk.Context, seq uint64) {
	if err := k.IDSequence.Set(ctx, seq); err != nil {
		panic(fmt.Errorf("energy keeper: set id sequence: %w", err))
	}
}

// ---------------------------------------------------------------------------
// EnergyData CRUD
// ---------------------------------------------------------------------------

func (k Keeper) SubmitEnergyData(ctx sdk.Context, data types.EnergyData) error {
	if err := k.EnergyData.Set(ctx, data.ID, data); err != nil {
		return fmt.Errorf("store energy data: %w", err)
	}
	if err := k.ByCategory.Set(ctx, collections.Join(data.Category, data.ID)); err != nil {
		return fmt.Errorf("index by category: %w", err)
	}
	if err := k.BySubmitter.Set(ctx, collections.Join(data.Submitter, data.ID)); err != nil {
		return fmt.Errorf("index by submitter: %w", err)
	}
	return nil
}

func (k Keeper) GetEnergyData(ctx sdk.Context, id string) (types.EnergyData, bool) {
	data, err := k.EnergyData.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.EnergyData{}, false
		}
		ctx.Logger().Error("energy data decode failed", "id", id, "err", err)
		return types.EnergyData{}, false
	}
	return data, true
}

// MaxQueryResults caps any single list response. The pagination layer also
// enforces a per-page limit, but this constant guards keeper-level callers
// (e.g. genesis import sanity checks) that don't go through pagination.
const MaxQueryResults = 1000

// IterateEnergyData returns every record in the store. Intended for genesis
// export, off-chain indexers, and tests. RPC paths MUST go through the
// paginated query server instead.
func (k Keeper) IterateEnergyData(ctx sdk.Context) []types.EnergyData {
	return k.getAllDataUnbounded(ctx)
}

// getAllDataUnbounded is for genesis export only.
func (k Keeper) getAllDataUnbounded(ctx sdk.Context) []types.EnergyData {
	var out []types.EnergyData
	if err := k.EnergyData.Walk(ctx, nil, func(_ string, v types.EnergyData) (bool, error) {
		out = append(out, v)
		return false, nil
	}); err != nil {
		ctx.Logger().Error("energy data walk failed", "err", err)
	}
	return out
}

// ---------------------------------------------------------------------------
// Batch CRUD
// ---------------------------------------------------------------------------

func (k Keeper) SubmitBatch(ctx sdk.Context, batch types.BatchSubmission) error {
	if err := k.Batches.Set(ctx, batch.ID, batch); err != nil {
		return fmt.Errorf("store batch: %w", err)
	}
	return nil
}

func (k Keeper) GetBatch(ctx sdk.Context, id string) (types.BatchSubmission, bool) {
	batch, err := k.Batches.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.BatchSubmission{}, false
		}
		ctx.Logger().Error("batch decode failed", "id", id, "err", err)
		return types.BatchSubmission{}, false
	}
	return batch, true
}

// getAllBatchesUnbounded is for genesis export only.
func (k Keeper) getAllBatchesUnbounded(ctx sdk.Context) []types.BatchSubmission {
	var out []types.BatchSubmission
	if err := k.Batches.Walk(ctx, nil, func(_ string, v types.BatchSubmission) (bool, error) {
		out = append(out, v)
		return false, nil
	}); err != nil {
		ctx.Logger().Error("batch walk failed", "err", err)
	}
	return out
}

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (k Keeper) SetParams(ctx sdk.Context, params types.Params) error {
	if err := k.Params.Set(ctx, params); err != nil {
		return fmt.Errorf("store energy params: %w", err)
	}
	return nil
}

func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	params, err := k.Params.Get(ctx)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("energy params decode failed", "err", err)
		}
		return types.DefaultParams()
	}
	return params
}

// IsAllowedSubmitter returns true when the address may submit energy data.
// In permissionless mode every address is allowed; otherwise the address must
// be present in the AllowedSubmitters whitelist (empty whitelist => deny all).
func (k Keeper) IsAllowedSubmitter(ctx sdk.Context, address string) bool {
	params := k.GetParams(ctx)
	if params.Permissionless {
		return true
	}
	for _, allowed := range params.AllowedSubmitters {
		if allowed == address {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Per-block rate limiting (transient store)
// ---------------------------------------------------------------------------

// CheckAndIncrSubmitCount returns an error if the address has already landed
// MaxSubmissionsPerBlockPerSubmitter txs in the current block; otherwise it
// increments the per-block counter. The transient store is wiped at the
// start of every block so this is naturally bounded.
//
// When the param is 0, rate limiting is disabled.
//
// NOTE: collections does not model transient stores cleanly (its assumption
// is that data persists across blocks), so we deliberately keep this path on
// raw KV access. The transient KV store has no persistence layer, so the
// extra complexity of a Schema-backed counter would buy nothing.
func (k Keeper) CheckAndIncrSubmitCount(ctx sdk.Context, addr string) error {
	limit := k.GetParams(ctx).MaxSubmissionsPerBlockPerSubmitter
	if limit == 0 {
		return nil
	}
	store := ctx.TransientStore(k.tStoreKey)
	key := types.GetSubmitCountByAddrKey(addr)

	var count uint32
	if bz := store.Get(key); bz != nil && len(bz) >= 4 {
		count = binary.BigEndian.Uint32(bz)
	}
	if count >= limit {
		return fmt.Errorf("address %s exceeded per-block submission limit %d", addr, limit)
	}
	count++
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, count)
	store.Set(key, out)
	return nil
}

// ---------------------------------------------------------------------------
// Genesis helpers
// ---------------------------------------------------------------------------

func (k Keeper) InitGenesis(ctx sdk.Context, gs types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return fmt.Errorf("setting energy params: %w", err)
	}
	for i, data := range gs.DataRecords {
		if err := k.SubmitEnergyData(ctx, data); err != nil {
			return fmt.Errorf("restoring energy data record %d: %w", i, err)
		}
	}
	for i, batch := range gs.Batches {
		if err := k.SubmitBatch(ctx, batch); err != nil {
			return fmt.Errorf("restoring batch %d: %w", i, err)
		}
	}

	// Restore the auto-increment counter so future IDs do not collide with
	// existing records. Take the max of the explicit NextID and any value
	// derivable from existing IDs (defensive: handles older genesis files
	// that omitted NextID).
	highest := gs.NextID
	for _, d := range gs.DataRecords {
		if n, ok := parseEnergyID(d.ID); ok && n > highest {
			highest = n
		}
	}
	for _, b := range gs.Batches {
		if n, ok := parseEnergyID(b.ID); ok && n > highest {
			highest = n
		}
	}
	if highest > 0 {
		k.SetCurrentID(ctx, highest)
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	return &types.GenesisState{
		Params:      k.GetParams(ctx),
		DataRecords: k.getAllDataUnbounded(ctx),
		Batches:     k.getAllBatchesUnbounded(ctx),
		NextID:      k.GetCurrentID(ctx),
	}
}

// parseEnergyID extracts the numeric suffix from IDs of the form
// "energy-<n>". Returns (0, false) for any other format so callers can
// safely ignore externally-supplied custom IDs.
func parseEnergyID(id string) (uint64, bool) {
	const prefix = "energy-"
	if len(id) <= len(prefix) || id[:len(prefix)] != prefix {
		return 0, false
	}
	var n uint64
	for _, c := range id[len(prefix):] {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + uint64(c-'0')
	}
	return n, true
}
