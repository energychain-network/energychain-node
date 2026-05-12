package keeper

import (
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/device/types"
)

// Keeper persists the device registry, attestation evidence, and the
// cross-module hooks (DID lookups). Two index sets:
//
//   - DeviceByOwner  (owner_did, device_did) — list-by-owner pagination.
//   - DeviceByGrid   (grid_zone, device_did) — used by x/cfe247 to enforce
//     "same-grid hour matching" without scanning the entire device store.
//   - DeviceByExpiry (expires_at, device_did) — drives the end-blocker
//     that auto-flips ATTESTED → SUSPECT when an attestation lapses.
//
// Attestation evidence has its own primary store keyed by an
// auto-incrementing uint64 plus a (device_did, id) index for "all
// attestations submitted for device X" queries and a pending-only index
// keyed by id for verifier UIs.
type Keeper struct {
	cdc          codec.Codec
	storeService corestore.KVStoreService
	authority    string

	did types.DIDKeeper

	Schema collections.Schema

	Params         collections.Item[types.Params]
	Devices        collections.Map[string, types.Device]
	ByOwner        collections.KeySet[collections.Pair[string, string]]
	ByGrid         collections.KeySet[collections.Pair[string, string]]
	ByExpiry       collections.KeySet[collections.Pair[int64, string]]

	Attestations  collections.Map[uint64, types.AttestationEvidence]
	AttByDevice   collections.KeySet[collections.Pair[string, uint64]]
	AttIDSeq      collections.Sequence
	AttPending    collections.KeySet[uint64]
}

func NewKeeper(cdc codec.Codec, storeService corestore.KVStoreService, authority string, did types.DIDKeeper) Keeper {
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc: cdc, storeService: storeService, authority: authority, did: did,

		Params: collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Devices: collections.NewMap(sb, types.DeviceCollectionPrefix, "devices",
			collections.StringKey, codec.CollValue[types.Device](cdc)),
		ByOwner: collections.NewKeySet(sb, types.DeviceByOwnerIndexPrefix, "by_owner",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey)),
		ByGrid: collections.NewKeySet(sb, types.DeviceByGridIndexPrefix, "by_grid",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey)),
		ByExpiry: collections.NewKeySet(sb, types.DeviceByExpiryIndexPrefix, "by_expiry",
			collections.PairKeyCodec(collections.Int64Key, collections.StringKey)),

		Attestations: collections.NewMap(sb, types.AttestationCollectionPrefix, "attestations",
			collections.Uint64Key, codec.CollValue[types.AttestationEvidence](cdc)),
		AttByDevice: collections.NewKeySet(sb, types.AttestationByDeviceIndexPrefix, "att_by_device",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		AttIDSeq: collections.NewSequence(sb, types.AttestationIDSequencePrefix, "att_id_seq"),
		AttPending: collections.NewKeySet(sb, types.AttestationPendingIndexPrefix, "att_pending",
			collections.Uint64Key),
	}
	schema, err := sb.Build()
	if err != nil {
		panic(fmt.Errorf("device keeper: build schema: %w", err))
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

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
			ctx.Logger().Error("device params decode", "err", err)
		}
		return types.DefaultParams()
	}
	return p
}

// IsAttestationVerifier checks the verifier whitelist. We compare against
// the params snapshot taken once per call to keep gas accounting stable
// (one read regardless of list length).
func (k Keeper) IsAttestationVerifier(ctx sdk.Context, addr string) bool {
	for _, v := range k.GetParams(ctx).AttestationVerifiers {
		if v == addr {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Devices
// ---------------------------------------------------------------------------

// SetDevice writes the device and refreshes its (owner, grid) index
// entries. Callers MUST first call clearDeviceIndexes when grid_zone or
// owner_did changes — the keeper has no easy way to read the previous
// values without an extra Get, and the msg_server already does that work.
func (k Keeper) SetDevice(ctx sdk.Context, d types.Device) error {
	if err := k.Devices.Set(ctx, d.DeviceDid, d); err != nil {
		return fmt.Errorf("store device %s: %w", d.DeviceDid, err)
	}
	if err := k.ByOwner.Set(ctx, collections.Join(d.OwnerDid, d.DeviceDid)); err != nil {
		return fmt.Errorf("index owner: %w", err)
	}
	if d.GridZone != "" {
		if err := k.ByGrid.Set(ctx, collections.Join(d.GridZone, d.DeviceDid)); err != nil {
			return fmt.Errorf("index grid: %w", err)
		}
	}
	if d.AttestationExpiresAt > 0 && d.Status == types.AttestationStatus_ATTESTATION_STATUS_ATTESTED {
		if err := k.ByExpiry.Set(ctx, collections.Join(d.AttestationExpiresAt, d.DeviceDid)); err != nil {
			return fmt.Errorf("index expiry: %w", err)
		}
	}
	return nil
}

// ClearDeviceIndexes removes the (owner, grid, expiry) index entries so a
// follow-up SetDevice can re-index without leaking stale rows.
func (k Keeper) ClearDeviceIndexes(ctx sdk.Context, prev types.Device) error {
	if err := k.ByOwner.Remove(ctx, collections.Join(prev.OwnerDid, prev.DeviceDid)); err != nil {
		return fmt.Errorf("remove owner index: %w", err)
	}
	if prev.GridZone != "" {
		if err := k.ByGrid.Remove(ctx, collections.Join(prev.GridZone, prev.DeviceDid)); err != nil {
			return fmt.Errorf("remove grid index: %w", err)
		}
	}
	if prev.AttestationExpiresAt > 0 {
		if err := k.ByExpiry.Remove(ctx, collections.Join(prev.AttestationExpiresAt, prev.DeviceDid)); err != nil {
			return fmt.Errorf("remove expiry index: %w", err)
		}
	}
	return nil
}

func (k Keeper) GetDevice(ctx sdk.Context, did string) (types.Device, bool) {
	d, err := k.Devices.Get(ctx, did)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("device decode", "did", did, "err", err)
		}
		return types.Device{}, false
	}
	return d, true
}

// IsAttested is the hot-path used by x/meter and x/eac to gate
// device-signed payloads. Single store read.
func (k Keeper) IsAttested(ctx sdk.Context, did string) bool {
	d, ok := k.GetDevice(ctx, did)
	if !ok {
		return false
	}
	return d.Status == types.AttestationStatus_ATTESTATION_STATUS_ATTESTED
}

// CountByOwner is used by the per-owner cap enforcement. Walks the
// (owner, _) prefix and stops at the cap to keep gas cost bounded.
func (k Keeper) CountByOwner(ctx sdk.Context, owner string, cap uint32) uint32 {
	var n uint32
	rng := collections.NewPrefixedPairRange[string, string](owner)
	_ = k.ByOwner.Walk(ctx, rng, func(_ collections.Pair[string, string]) (bool, error) {
		n++
		return n >= cap, nil
	})
	return n
}

// ---------------------------------------------------------------------------
// Attestations
// ---------------------------------------------------------------------------

func (k Keeper) NextAttestationID(ctx sdk.Context) uint64 {
	id, err := k.AttIDSeq.Next(ctx)
	if err != nil {
		panic(fmt.Errorf("device keeper: advance att id: %w", err))
	}
	return id + 1
}

func (k Keeper) PeekAttestationID(ctx sdk.Context) uint64 {
	id, err := k.AttIDSeq.Peek(ctx)
	if err != nil {
		return 0
	}
	return id
}

func (k Keeper) SetAttestationIDCounter(ctx sdk.Context, id uint64) {
	if err := k.AttIDSeq.Set(ctx, id); err != nil {
		panic(fmt.Errorf("device keeper: set att id counter: %w", err))
	}
}

func (k Keeper) StoreAttestation(ctx sdk.Context, id uint64, ev types.AttestationEvidence) error {
	if err := k.Attestations.Set(ctx, id, ev); err != nil {
		return fmt.Errorf("store attestation %d: %w", id, err)
	}
	if err := k.AttByDevice.Set(ctx, collections.Join(ev.DeviceDid, id)); err != nil {
		return fmt.Errorf("index att by device: %w", err)
	}
	if ev.Verdict == types.VerdictPending {
		if err := k.AttPending.Set(ctx, id); err != nil {
			return fmt.Errorf("index att pending: %w", err)
		}
	}
	return nil
}

func (k Keeper) GetAttestation(ctx sdk.Context, id uint64) (types.AttestationEvidence, bool) {
	ev, err := k.Attestations.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("attestation decode", "id", id, "err", err)
		}
		return types.AttestationEvidence{}, false
	}
	return ev, true
}

// ResolveAttestation flips a pending row to its terminal verdict, trims
// the pending index, and returns the updated evidence. Idempotent: if the
// verdict is already non-pending, the call is a no-op apart from re-
// trimming the pending index.
func (k Keeper) ResolveAttestation(ctx sdk.Context, id uint64, verdict uint32, reason string) (types.AttestationEvidence, error) {
	ev, ok := k.GetAttestation(ctx, id)
	if !ok {
		return types.AttestationEvidence{}, fmt.Errorf("attestation %d not found", id)
	}
	if ev.Verdict != types.VerdictPending {
		// Trim the pending index defensively.
		_ = k.AttPending.Remove(ctx, id)
		return ev, nil
	}
	ev.Verdict = verdict
	ev.Reason = reason
	if err := k.Attestations.Set(ctx, id, ev); err != nil {
		return ev, fmt.Errorf("update attestation: %w", err)
	}
	if err := k.AttPending.Remove(ctx, id); err != nil {
		return ev, fmt.Errorf("remove pending index: %w", err)
	}
	return ev, nil
}

// ---------------------------------------------------------------------------
// End-blocker housekeeping
// ---------------------------------------------------------------------------

// ExpireAttestations walks the (expires_at, device) index up to the
// current block time and flips matching devices ATTESTED → SUSPECT.
// Bounded per-block work; same justification as the did module's
// SweepExpiredCredentials.
func (k Keeper) ExpireAttestations(ctx sdk.Context) (int, error) {
	now := ctx.BlockTime().Unix()
	if now == 0 {
		return 0, nil
	}
	rng := new(collections.Range[collections.Pair[int64, string]]).
		StartInclusive(collections.PairPrefix[int64, string](0)).
		EndExclusive(collections.PairPrefix[int64, string](now + 1))

	type entry struct {
		expiry int64
		did    string
	}
	var hits []entry
	if err := k.ByExpiry.Walk(ctx, rng, func(key collections.Pair[int64, string]) (bool, error) {
		hits = append(hits, entry{expiry: key.K1(), did: key.K2()})
		return len(hits) >= 256, nil
	}); err != nil {
		return 0, fmt.Errorf("walk expiry: %w", err)
	}
	for _, e := range hits {
		d, ok := k.GetDevice(ctx, e.did)
		if !ok {
			_ = k.ByExpiry.Remove(ctx, collections.Join(e.expiry, e.did))
			continue
		}
		if d.Status == types.AttestationStatus_ATTESTATION_STATUS_ATTESTED {
			d.Status = types.AttestationStatus_ATTESTATION_STATUS_SUSPECT
			d.UpdatedAt = now
			if err := k.Devices.Set(ctx, d.DeviceDid, d); err != nil {
				return 0, fmt.Errorf("update device %s: %w", d.DeviceDid, err)
			}
		}
		_ = k.ByExpiry.Remove(ctx, collections.Join(e.expiry, e.did))
	}
	return len(hits), nil
}

// ---------------------------------------------------------------------------
// Iteration helpers (genesis)
// ---------------------------------------------------------------------------

const MaxQueryResults = 1000

func (k Keeper) exportAllDevices(ctx sdk.Context) []types.Device {
	var out []types.Device
	_ = k.Devices.Walk(ctx, nil, func(_ string, v types.Device) (bool, error) {
		out = append(out, v)
		return false, nil
	})
	return out
}

func (k Keeper) exportAllAttestations(ctx sdk.Context) []types.AttestationEvidence {
	var out []types.AttestationEvidence
	_ = k.Attestations.Walk(ctx, nil, func(_ uint64, v types.AttestationEvidence) (bool, error) {
		out = append(out, v)
		return false, nil
	})
	return out
}

func (k Keeper) InitGenesis(ctx sdk.Context, gs types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return fmt.Errorf("set params: %w", err)
	}
	for _, d := range gs.Devices {
		if err := k.SetDevice(ctx, d); err != nil {
			return fmt.Errorf("import device %s: %w", d.DeviceDid, err)
		}
	}
	// Re-import attestations using the genesis-supplied id sequence.
	// We rely on the genesis to pack attestations contiguously starting
	// at id=1; if you exported with gaps, re-export.
	id := uint64(0)
	for _, ev := range gs.Attestations {
		id++
		if err := k.StoreAttestation(ctx, id, ev); err != nil {
			return fmt.Errorf("import attestation %d: %w", id, err)
		}
	}
	if gs.NextAttestationId > 0 {
		k.SetAttestationIDCounter(ctx, gs.NextAttestationId-1)
	} else if id > 0 {
		k.SetAttestationIDCounter(ctx, id)
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	next := k.PeekAttestationID(ctx) + 1
	if next == 0 {
		next = 1
	}
	return &types.GenesisState{
		Params:             k.GetParams(ctx),
		Devices:            k.exportAllDevices(ctx),
		Attestations:       k.exportAllAttestations(ctx),
		NextAttestationId: next,
	}
}

// CheckOwnerHasDID is used by the msg_server before any owner-side
// mutation; routes through the injected DIDKeeper so the dependency
// remains stub-able in tests.
func (k Keeper) CheckOwnerHasDID(ctx sdk.Context, owner string) bool {
	if k.did == nil {
		return true // dev mode: skip the cross-module check
	}
	return k.did.IsActive(ctx, owner)
}
