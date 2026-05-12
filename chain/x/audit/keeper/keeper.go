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
//
// The M1 evolution introduces three new families of state on top of the
// original audit-log store:
//
//   - Schemas — descriptors keyed by event_type so off-chain readers can
//     resolve a log to a self-describing JSON-Schema / JSON-LD doc.
//   - ArchiveSegments — immutable digests of evicted log batches plus
//     their off-chain URI; live logs in the segment range are removed.
//   - ViewKeyGrants — opaque encrypted symmetric keys handed to
//     regulators (or other DIDs) so they can decrypt encrypted log
//     payloads off-chain. The chain enforces the grant lifecycle
//     (issuance / revocation / expiry sweep), not the decryption.
//
// did is an optional cross-module hook used to refuse view-key grants
// to grantees without an on-chain DID. nil-safe (dev mode / tests skip).
type Keeper struct {
	cdc          codec.Codec
	storeService corestore.KVStoreService
	tStoreKey    storetypes.StoreKey
	authority    string

	did types.DIDKeeper

	Schema collections.Schema

	Params      collections.Item[types.Params]
	IDSequence  collections.Sequence
	Logs        collections.Map[uint64, types.AuditLog]
	ByActor     collections.KeySet[collections.Pair[string, uint64]]
	ByEventType collections.KeySet[collections.Pair[string, uint64]]
	ByTime      collections.KeySet[collections.Pair[uint64, uint64]]
	BySeverity  collections.KeySet[collections.Pair[int32, uint64]]
	BySchema    collections.KeySet[collections.Pair[string, uint64]]

	Schemas         collections.Map[string, types.SchemaDescriptor]
	ArchiveSegments collections.Map[uint64, types.ArchiveSegment]
	ArchiveIDSeq    collections.Sequence
	ViewKeyGrants   collections.Map[string, types.ViewKeyGrant]
	ViewKeyByGrantee collections.KeySet[collections.Pair[string, string]]
	ViewKeyByExpiry  collections.KeySet[collections.Pair[int64, string]]
	GrantIDSeq      collections.Sequence
}

func NewKeeper(
	cdc codec.Codec,
	storeService corestore.KVStoreService,
	tStoreKey storetypes.StoreKey,
	authority string,
	did types.DIDKeeper,
) Keeper {
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		tStoreKey:    tStoreKey,
		authority:    authority,
		did:          did,

		Params: collections.NewItem(sb, types.ParamsCollectionPrefix, "params",
			codec.CollValue[types.Params](cdc)),
		IDSequence: collections.NewSequence(sb, types.IDSequenceCollectionPrefix, "id_sequence"),
		Logs: collections.NewMap(sb, types.AuditLogCollectionPrefix, "audit_logs",
			collections.Uint64Key, codec.CollValue[types.AuditLog](cdc)),
		ByActor: collections.NewKeySet(sb, types.AuditByActorCollectionPrefix, "by_actor",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		ByEventType: collections.NewKeySet(sb, types.AuditByTypeCollectionPrefix, "by_event_type",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		ByTime: collections.NewKeySet(sb, types.AuditByTimeCollectionPrefix, "by_time",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),
		BySeverity: collections.NewKeySet(sb, types.AuditBySeverityCollectionPrefix, "by_severity",
			collections.PairKeyCodec(collections.Int32Key, collections.Uint64Key)),
		BySchema: collections.NewKeySet(sb, types.AuditBySchemaCollectionPrefix, "by_schema",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		Schemas: collections.NewMap(sb, types.SchemaCollectionPrefix, "schemas",
			collections.StringKey, codec.CollValue[types.SchemaDescriptor](cdc)),
		ArchiveSegments: collections.NewMap(sb, types.ArchiveSegmentCollectionPrefix, "archive_segments",
			collections.Uint64Key, codec.CollValue[types.ArchiveSegment](cdc)),
		ArchiveIDSeq: collections.NewSequence(sb, types.ArchiveIDSequencePrefix, "archive_id_seq"),
		ViewKeyGrants: collections.NewMap(sb, types.ViewKeyGrantCollectionPrefix, "view_key_grants",
			collections.StringKey, codec.CollValue[types.ViewKeyGrant](cdc)),
		ViewKeyByGrantee: collections.NewKeySet(sb, types.ViewKeyByGranteePrefix, "view_key_by_grantee",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey)),
		ViewKeyByExpiry: collections.NewKeySet(sb, types.ViewKeyByExpiryPrefix, "view_key_by_expiry",
			collections.PairKeyCodec(collections.Int64Key, collections.StringKey)),
		GrantIDSeq: collections.NewSequence(sb, types.GrantIDSequencePrefix, "grant_id_seq"),
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

func (k Keeper) GetAuthority() string { return k.authority }

// IsArchiveAuthority returns true when the signer is allowed to seal
// archive batches: either the chain authority or one of the addresses
// listed in params.archive_authorities.
func (k Keeper) IsArchiveAuthority(ctx sdk.Context, addr string) bool {
	if addr == k.authority {
		return true
	}
	for _, a := range k.GetParams(ctx).ArchiveAuthorities {
		if a == addr {
			return true
		}
	}
	return false
}

// CheckGranteeHasDID gates view-key grants on the grantee having an
// active DID. Returns true if the dependency was not wired (dev mode);
// otherwise delegates to the injected DIDKeeper.
func (k Keeper) CheckGranteeHasDID(ctx sdk.Context, grantee string) bool {
	if k.did == nil {
		return true
	}
	return k.did.IsActive(ctx, grantee)
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

// NextArchiveID advances and returns the next archive segment id. Mirrors
// the semantics of NextAttestationID in x/device.
func (k Keeper) NextArchiveID(ctx sdk.Context) uint64 {
	id, err := k.ArchiveIDSeq.Next(ctx)
	if err != nil {
		panic(fmt.Errorf("audit keeper: advance archive id: %w", err))
	}
	return id + 1
}

// NextGrantID returns a deterministic, monotonically increasing grant id
// formatted as "vk-<sequence>". Stable strings are easier for off-chain
// CLIs to plumb through than raw uint64.
func (k Keeper) NextGrantID(ctx sdk.Context) string {
	id, err := k.GrantIDSeq.Next(ctx)
	if err != nil {
		panic(fmt.Errorf("audit keeper: advance grant id: %w", err))
	}
	return fmt.Sprintf("vk-%d", id+1)
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

// RecordAuditLog persists a log row and refreshes every secondary index
// in one shot. Callers MUST set log.ID via GetNextID first.
func (k Keeper) RecordAuditLog(ctx sdk.Context, log types.AuditLog) error {
	if err := k.Logs.Set(ctx, log.ID, log); err != nil {
		return fmt.Errorf("store audit log %d: %w", log.ID, err)
	}
	if err := k.indexLog(ctx, log); err != nil {
		return err
	}
	return nil
}

// indexLog writes secondary index entries for log. Extracted so
// ArchiveSegment eviction can build a parallel removal helper without
// repeating the field list.
func (k Keeper) indexLog(ctx sdk.Context, log types.AuditLog) error {
	if err := k.ByActor.Set(ctx, collections.Join(log.Actor, log.ID)); err != nil {
		return fmt.Errorf("index audit log by actor: %w", err)
	}
	if err := k.ByEventType.Set(ctx, collections.Join(log.EventType, log.ID)); err != nil {
		return fmt.Errorf("index audit log by event_type: %w", err)
	}
	if err := k.ByTime.Set(ctx, collections.Join(timestampToKey(log.Timestamp), log.ID)); err != nil {
		return fmt.Errorf("index audit log by time: %w", err)
	}
	if err := k.BySeverity.Set(ctx, collections.Join(int32(log.Severity), log.ID)); err != nil {
		return fmt.Errorf("index audit log by severity: %w", err)
	}
	if log.SchemaId != "" {
		if err := k.BySchema.Set(ctx, collections.Join(log.SchemaId, log.ID)); err != nil {
			return fmt.Errorf("index audit log by schema: %w", err)
		}
	}
	return nil
}

// removeLogIndexes mirrors indexLog for archive eviction.
func (k Keeper) removeLogIndexes(ctx sdk.Context, log types.AuditLog) error {
	if err := k.ByActor.Remove(ctx, collections.Join(log.Actor, log.ID)); err != nil {
		return fmt.Errorf("remove actor index: %w", err)
	}
	if err := k.ByEventType.Remove(ctx, collections.Join(log.EventType, log.ID)); err != nil {
		return fmt.Errorf("remove event_type index: %w", err)
	}
	if err := k.ByTime.Remove(ctx, collections.Join(timestampToKey(log.Timestamp), log.ID)); err != nil {
		return fmt.Errorf("remove time index: %w", err)
	}
	if err := k.BySeverity.Remove(ctx, collections.Join(int32(log.Severity), log.ID)); err != nil {
		return fmt.Errorf("remove severity index: %w", err)
	}
	if log.SchemaId != "" {
		if err := k.BySchema.Remove(ctx, collections.Join(log.SchemaId, log.ID)); err != nil {
			return fmt.Errorf("remove schema index: %w", err)
		}
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
// Schema registry
// ---------------------------------------------------------------------------

func (k Keeper) SetSchema(ctx sdk.Context, d types.SchemaDescriptor) error {
	return k.Schemas.Set(ctx, d.EventType, d)
}

func (k Keeper) GetSchema(ctx sdk.Context, eventType string) (types.SchemaDescriptor, bool) {
	d, err := k.Schemas.Get(ctx, eventType)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("audit schema decode", "event_type", eventType, "err", err)
		}
		return types.SchemaDescriptor{}, false
	}
	return d, true
}

func (k Keeper) exportAllSchemas(ctx sdk.Context) []types.SchemaDescriptor {
	var out []types.SchemaDescriptor
	_ = k.Schemas.Walk(ctx, nil, func(_ string, v types.SchemaDescriptor) (bool, error) {
		out = append(out, v)
		return false, nil
	})
	return out
}

// ---------------------------------------------------------------------------
// Archive segments
// ---------------------------------------------------------------------------

func (k Keeper) SetArchiveSegment(ctx sdk.Context, s types.ArchiveSegment) error {
	return k.ArchiveSegments.Set(ctx, s.ID, s)
}

func (k Keeper) GetArchiveSegment(ctx sdk.Context, id uint64) (types.ArchiveSegment, bool) {
	s, err := k.ArchiveSegments.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("audit archive segment decode", "id", id, "err", err)
		}
		return types.ArchiveSegment{}, false
	}
	return s, true
}

func (k Keeper) exportAllArchiveSegments(ctx sdk.Context) []types.ArchiveSegment {
	var out []types.ArchiveSegment
	_ = k.ArchiveSegments.Walk(ctx, nil, func(_ uint64, v types.ArchiveSegment) (bool, error) {
		out = append(out, v)
		return false, nil
	})
	return out
}

// EvictLogsForArchive removes the live log rows AND their secondary index
// entries for ids in [from, to]. Returns the count actually evicted (0
// rows that no longer exist are silently skipped — they may have been
// archived in an earlier run for the same range, which is a no-op).
func (k Keeper) EvictLogsForArchive(ctx sdk.Context, from, to uint64, cap uint32) (uint64, error) {
	var evicted uint64
	if cap == 0 {
		return 0, errors.New("eviction cap must be > 0")
	}
	for id := from; id <= to; id++ {
		log, ok := k.GetAuditLog(ctx, id)
		if !ok {
			continue
		}
		if err := k.removeLogIndexes(ctx, log); err != nil {
			return evicted, err
		}
		if err := k.Logs.Remove(ctx, id); err != nil {
			return evicted, fmt.Errorf("remove log %d: %w", id, err)
		}
		evicted++
		if uint32(evicted) >= cap {
			break
		}
	}
	return evicted, nil
}

// ---------------------------------------------------------------------------
// View-key grants
// ---------------------------------------------------------------------------

func (k Keeper) SetGrant(ctx sdk.Context, g types.ViewKeyGrant) error {
	if err := k.ViewKeyGrants.Set(ctx, g.Id, g); err != nil {
		return fmt.Errorf("store grant: %w", err)
	}
	if err := k.ViewKeyByGrantee.Set(ctx, collections.Join(g.Grantee, g.Id)); err != nil {
		return fmt.Errorf("index grant by grantee: %w", err)
	}
	if g.ExpiresAt > 0 {
		if err := k.ViewKeyByExpiry.Set(ctx, collections.Join(g.ExpiresAt, g.Id)); err != nil {
			return fmt.Errorf("index grant by expiry: %w", err)
		}
	}
	return nil
}

func (k Keeper) GetGrant(ctx sdk.Context, id string) (types.ViewKeyGrant, bool) {
	g, err := k.ViewKeyGrants.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("audit grant decode", "id", id, "err", err)
		}
		return types.ViewKeyGrant{}, false
	}
	return g, true
}

// MarkGrantRevoked flips a grant to revoked, captures reason / time, and
// trims the expiry index (no longer relevant once revoked).
func (k Keeper) MarkGrantRevoked(ctx sdk.Context, g types.ViewKeyGrant, reason string) error {
	if g.ExpiresAt > 0 {
		if err := k.ViewKeyByExpiry.Remove(ctx, collections.Join(g.ExpiresAt, g.Id)); err != nil {
			return fmt.Errorf("trim expiry index: %w", err)
		}
	}
	g.Revoked = true
	g.RevokedAt = ctx.BlockTime().Unix()
	g.RevocationReason = reason
	return k.ViewKeyGrants.Set(ctx, g.Id, g)
}

func (k Keeper) exportAllGrants(ctx sdk.Context) []types.ViewKeyGrant {
	var out []types.ViewKeyGrant
	_ = k.ViewKeyGrants.Walk(ctx, nil, func(_ string, v types.ViewKeyGrant) (bool, error) {
		out = append(out, v)
		return false, nil
	})
	return out
}

// SweepExpiredGrants is the EndBlock counterpart of x/did's credential
// sweep: walk the (expires_at, grant_id) prefix up to now, mark matching
// grants revoked, and trim the expiry index. Bounded per-block work so a
// long downtime cannot blow the gas budget.
func (k Keeper) SweepExpiredGrants(ctx sdk.Context) (int, error) {
	now := ctx.BlockTime().Unix()
	if now == 0 {
		return 0, nil // pre-genesis safety
	}
	rng := new(collections.Range[collections.Pair[int64, string]]).
		StartInclusive(collections.PairPrefix[int64, string](0)).
		EndExclusive(collections.PairPrefix[int64, string](now + 1))

	type entry struct {
		expiry int64
		id     string
	}
	var hits []entry
	if err := k.ViewKeyByExpiry.Walk(ctx, rng, func(key collections.Pair[int64, string]) (bool, error) {
		hits = append(hits, entry{expiry: key.K1(), id: key.K2()})
		return len(hits) >= 256, nil
	}); err != nil {
		return 0, fmt.Errorf("walk expiry index: %w", err)
	}

	for _, e := range hits {
		g, ok := k.GetGrant(ctx, e.id)
		if !ok {
			_ = k.ViewKeyByExpiry.Remove(ctx, collections.Join(e.expiry, e.id))
			continue
		}
		if !g.Revoked {
			g.Revoked = true
			g.RevokedAt = now
			g.RevocationReason = "expired"
			if err := k.ViewKeyGrants.Set(ctx, g.Id, g); err != nil {
				return 0, fmt.Errorf("mark grant expired %s: %w", g.Id, err)
			}
		}
		if err := k.ViewKeyByExpiry.Remove(ctx, collections.Join(e.expiry, e.id)); err != nil {
			return 0, fmt.Errorf("trim expiry index %s: %w", e.id, err)
		}
	}
	return len(hits), nil
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
	maxID := gs.Counter
	for _, log := range gs.Logs {
		if log.ID > maxID {
			maxID = log.ID
		}
	}
	if maxID > 0 {
		k.IncrementCounter(ctx, maxID)
	}

	for _, d := range gs.Schemas {
		if err := k.SetSchema(ctx, d); err != nil {
			return fmt.Errorf("import schema %s: %w", d.EventType, err)
		}
	}

	for _, s := range gs.ArchiveSegments {
		if err := k.SetArchiveSegment(ctx, s); err != nil {
			return fmt.Errorf("import archive segment %d: %w", s.ID, err)
		}
	}
	if gs.ArchiveCounter > 0 {
		if err := k.ArchiveIDSeq.Set(ctx, gs.ArchiveCounter); err != nil {
			return fmt.Errorf("set archive id seq: %w", err)
		}
	}

	for _, g := range gs.ViewKeyGrants {
		if err := k.SetGrant(ctx, g); err != nil {
			return fmt.Errorf("import grant %s: %w", g.Id, err)
		}
	}
	if gs.GrantCounter > 0 {
		if err := k.GrantIDSeq.Set(ctx, gs.GrantCounter); err != nil {
			return fmt.Errorf("set grant id seq: %w", err)
		}
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	logs := k.exportAllLogs(ctx)
	counter, err := k.IDSequence.Peek(ctx)
	if err != nil {
		counter = 0
	}
	archiveCounter, err := k.ArchiveIDSeq.Peek(ctx)
	if err != nil {
		archiveCounter = 0
	}
	grantCounter, err := k.GrantIDSeq.Peek(ctx)
	if err != nil {
		grantCounter = 0
	}
	return &types.GenesisState{
		Logs:            logs,
		Counter:         counter,
		Params:          k.GetParams(ctx),
		Schemas:         k.exportAllSchemas(ctx),
		ArchiveSegments: k.exportAllArchiveSegments(ctx),
		ArchiveCounter:  archiveCounter,
		ViewKeyGrants:   k.exportAllGrants(ctx),
		GrantCounter:    grantCounter,
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

var (
	_ = uint64ToBytes
	_ = bytesToUint64
)
