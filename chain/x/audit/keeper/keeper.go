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

	"energychain/x/audit/types"
)

// Keeper holds the declarative collections backing the audit module's
// persistent state.
type Keeper struct {
	cdc          codec.Codec
	storeService corestore.KVStoreService
	tStoreKey    storetypes.StoreKey // per-block transient store; rate-limit counters
	authority    string

	Schema collections.Schema

	Params      collections.Item[types.Params]
	IDSequence  collections.Sequence
	Logs        collections.Map[uint64, types.AuditLog]
	ByActor     collections.KeySet[collections.Pair[string, uint64]]
	ByEventType collections.KeySet[collections.Pair[string, uint64]]
	ByTime      collections.KeySet[collections.Pair[uint64, uint64]]
}

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
		Logs: collections.NewMap(
			sb,
			types.AuditLogCollectionPrefix,
			"audit_logs",
			collections.Uint64Key,
			codec.CollValue[types.AuditLog](cdc),
		),
		ByActor: collections.NewKeySet(
			sb,
			types.AuditByActorCollectionPrefix,
			"by_actor",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key),
		),
		ByEventType: collections.NewKeySet(
			sb,
			types.AuditByTypeCollectionPrefix,
			"by_event_type",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key),
		),
		ByTime: collections.NewKeySet(
			sb,
			types.AuditByTimeCollectionPrefix,
			"by_time",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key),
		),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(fmt.Errorf("audit keeper: build collections schema: %w", err))
	}
	k.Schema = schema
	return k
}

// CheckAndIncrAuditCount enforces the per-block per-creator rate limit. When
// the param is 0, rate limiting is disabled. The counter lives in the
// transient store so it auto-resets every block; we keep it on raw KV access
// because collections does not model transient stores.
func (k Keeper) CheckAndIncrAuditCount(ctx sdk.Context, creator string) error {
	limit := k.GetParams(ctx).MaxAuditsPerBlockPerCreator
	if limit == 0 {
		return nil
	}
	store := ctx.TransientStore(k.tStoreKey)
	key := types.GetAuditCountByCreatorKey(creator)

	var count uint32
	if bz := store.Get(key); bz != nil && len(bz) >= 4 {
		count = binary.BigEndian.Uint32(bz)
	}
	if count >= limit {
		return fmt.Errorf("creator %s exceeded per-block audit limit %d", creator, limit)
	}
	count++
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, count)
	store.Set(key, out)
	return nil
}

func (k Keeper) GetAuthority() string {
	return k.authority
}

// ---------------------------------------------------------------------------
// Counter (auto-increment ID)
// ---------------------------------------------------------------------------

// GetNextID returns the ID that the *next* RecordAuditLog should use, based
// on the current sequence value. Sequence is 0-indexed in collections; we
// surface IDs as 1-indexed to preserve historical behaviour, so the next ID
// is sequence.Peek + 1. The msg_server pairs this with IncrementCounter.
func (k Keeper) GetNextID(ctx sdk.Context) uint64 {
	cur, err := k.IDSequence.Peek(ctx)
	if err != nil {
		ctx.Logger().Error("audit id sequence peek failed", "err", err)
		return 1
	}
	return cur + 1
}

// IncrementCounter advances the sequence to id (so the next GetNextID returns
// id + 1). Idempotent if called with the value already stored.
func (k Keeper) IncrementCounter(ctx sdk.Context, id uint64) {
	if err := k.IDSequence.Set(ctx, id); err != nil {
		panic(fmt.Errorf("audit keeper: set id sequence: %w", err))
	}
}

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (k Keeper) SetParams(ctx sdk.Context, params types.Params) error {
	if err := k.Params.Set(ctx, params); err != nil {
		return fmt.Errorf("store audit params: %w", err)
	}
	return nil
}

func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	params, err := k.Params.Get(ctx)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("audit params decode failed", "err", err)
		}
		return types.DefaultParams()
	}
	return params
}

// ---------------------------------------------------------------------------
// Audit Log CRUD
// ---------------------------------------------------------------------------

func (k Keeper) RecordAuditLog(ctx sdk.Context, log types.AuditLog) error {
	if err := k.Logs.Set(ctx, log.ID, log); err != nil {
		return fmt.Errorf("store audit log %d: %w", log.ID, err)
	}
	if err := k.ByActor.Set(ctx, collections.Join(log.Actor, log.ID)); err != nil {
		return fmt.Errorf("index audit log by actor: %w", err)
	}
	if err := k.ByEventType.Set(ctx, collections.Join(log.EventType, log.ID)); err != nil {
		return fmt.Errorf("index audit log by event_type: %w", err)
	}
	if err := k.ByTime.Set(ctx, collections.Join(timestampToKey(log.Timestamp), log.ID)); err != nil {
		return fmt.Errorf("index audit log by time: %w", err)
	}
	return nil
}

// timestampToKey clamps negative timestamps to 0 so the uint64 cast preserves
// ordering with legitimate (non-negative) values.
func timestampToKey(ts int64) uint64 {
	if ts < 0 {
		return 0
	}
	return uint64(ts)
}

// GetAuditLog returns (log, true) when found and decodable.
func (k Keeper) GetAuditLog(ctx sdk.Context, id uint64) (types.AuditLog, bool) {
	log, err := k.Logs.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("audit log decode failed", "id", id, "err", err)
		}
		return types.AuditLog{}, false
	}
	return log, true
}

// ---------------------------------------------------------------------------
// Index-based queries (kept for backward compatibility; pagination-friendly
// alternatives live in the query server via collections + CollectionPaginate).
// ---------------------------------------------------------------------------

// MaxQueryResults caps the number of records any list query may return.
const MaxQueryResults = 1000

func (k Keeper) GetAuditLogsByActor(ctx sdk.Context, actor string) []types.AuditLog {
	logs := make([]types.AuditLog, 0, 16)
	rng := collections.NewPrefixedPairRange[string, uint64](actor)
	if err := k.ByActor.Walk(ctx, rng, func(key collections.Pair[string, uint64]) (bool, error) {
		log, ok := k.GetAuditLog(ctx, key.K2())
		if ok {
			logs = append(logs, log)
		}
		if len(logs) >= MaxQueryResults {
			return true, nil
		}
		return false, nil
	}); err != nil {
		ctx.Logger().Error("audit by-actor walk failed", "actor", actor, "err", err)
	}
	return logs
}

func (k Keeper) GetAuditLogsByType(ctx sdk.Context, eventType string) []types.AuditLog {
	logs := make([]types.AuditLog, 0, 16)
	rng := collections.NewPrefixedPairRange[string, uint64](eventType)
	if err := k.ByEventType.Walk(ctx, rng, func(key collections.Pair[string, uint64]) (bool, error) {
		log, ok := k.GetAuditLog(ctx, key.K2())
		if ok {
			logs = append(logs, log)
		}
		if len(logs) >= MaxQueryResults {
			return true, nil
		}
		return false, nil
	}); err != nil {
		ctx.Logger().Error("audit by-type walk failed", "event_type", eventType, "err", err)
	}
	return logs
}

func (k Keeper) GetAuditLogsByTimeRange(ctx sdk.Context, from, to int64) []types.AuditLog {
	if from < 0 || to < from {
		return nil
	}
	startKey := timestampToKey(from)
	endKey := timestampToKey(to) + 1 // exclusive upper bound

	logs := make([]types.AuditLog, 0, 16)
	rng := new(collections.Range[collections.Pair[uint64, uint64]]).
		StartInclusive(collections.PairPrefix[uint64, uint64](startKey)).
		EndExclusive(collections.PairPrefix[uint64, uint64](endKey))
	if err := k.ByTime.Walk(ctx, rng, func(key collections.Pair[uint64, uint64]) (bool, error) {
		log, ok := k.GetAuditLog(ctx, key.K2())
		if ok {
			logs = append(logs, log)
		}
		if len(logs) >= MaxQueryResults {
			return true, nil
		}
		return false, nil
	}); err != nil {
		ctx.Logger().Error("audit by-time walk failed", "from", from, "to", to, "err", err)
	}
	return logs
}

// ---------------------------------------------------------------------------
// Full iteration
// ---------------------------------------------------------------------------

// GetAllLogs returns up to MaxQueryResults audit logs starting from the
// beginning of the store. Use ExportGenesis for unbounded full dumps.
func (k Keeper) GetAllLogs(ctx sdk.Context) []types.AuditLog {
	logs := make([]types.AuditLog, 0, 16)
	if err := k.Logs.Walk(ctx, nil, func(_ uint64, v types.AuditLog) (bool, error) {
		logs = append(logs, v)
		if len(logs) >= MaxQueryResults {
			return true, nil
		}
		return false, nil
	}); err != nil {
		ctx.Logger().Error("audit logs walk failed", "err", err)
	}
	return logs
}

// exportAllLogs returns every audit log; used only for genesis export.
func (k Keeper) exportAllLogs(ctx sdk.Context) []types.AuditLog {
	var logs []types.AuditLog
	if err := k.Logs.Walk(ctx, nil, func(_ uint64, v types.AuditLog) (bool, error) {
		logs = append(logs, v)
		return false, nil
	}); err != nil {
		ctx.Logger().Error("audit logs export walk failed", "err", err)
	}
	return logs
}

// ---------------------------------------------------------------------------
// Genesis helpers
// ---------------------------------------------------------------------------

func (k Keeper) InitGenesis(ctx sdk.Context, gs types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return fmt.Errorf("setting audit params: %w", err)
	}

	for i, log := range gs.Logs {
		if err := k.RecordAuditLog(ctx, log); err != nil {
			return fmt.Errorf("recording audit log %d: %w", i, err)
		}
	}

	// Counter must be at least the highest log ID we just restored, otherwise
	// new logs would collide with existing ones.
	maxID := gs.Counter
	for _, log := range gs.Logs {
		if log.ID > maxID {
			maxID = log.ID
		}
	}
	if maxID > 0 {
		k.IncrementCounter(ctx, maxID)
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	logs := k.exportAllLogs(ctx)
	counter, err := k.IDSequence.Peek(ctx)
	if err != nil {
		counter = 0
	}
	return &types.GenesisState{
		Logs:    logs,
		Counter: counter,
		Params:  k.GetParams(ctx),
	}
}

// uint64ToBytes / bytesToUint64 retained for any downstream consumers; the
// keeper itself no longer uses them.
func uint64ToBytes(v uint64) []byte {
	bz := make([]byte, 8)
	binary.BigEndian.PutUint64(bz, v)
	return bz
}

func bytesToUint64(bz []byte) uint64 {
	return binary.BigEndian.Uint64(bz)
}

// reference the helpers so go vet doesn't complain when nothing in this
// package calls them; they're exported via deprecated APIs.
var (
	_ = uint64ToBytes
	_ = bytesToUint64
)











