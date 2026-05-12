package keeper

import (
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/meter/types"
)

// Keeper is the meter module's persistent state surface. It models a
// registry of MeteringPoints, time-bucketed Readings (committed via
// internal/commitment), Merkle-rooted Batches, and StreamAuthorizations
// that delegate ongoing submission rights to a device.
//
// Cross-module dependencies are kept behind narrow interfaces (DIDKeeper,
// DeviceKeeper) so the module can be exercised in isolation. Both are
// nil-safe: a nil keeper short-circuits the corresponding check, which
// is what dev/testnet wiring relies on. Production app.go MUST wire
// both.
type Keeper struct {
	cdc          codec.Codec
	storeService corestore.KVStoreService
	authority    string

	did    types.DIDKeeper
	device types.DeviceKeeper

	Schema collections.Schema

	Params collections.Item[types.Params]

	MeteringPoints   collections.Map[string, types.MeteringPoint]
	MPByOwner        collections.KeySet[collections.Pair[string, string]]
	MPByZone         collections.KeySet[collections.Pair[string, string]]

	Readings collections.Map[collections.Pair[string, int64], types.Reading]

	Batches collections.Map[string, types.Batch]
	BatchByMP collections.KeySet[collections.Pair[string, string]]

	StreamAuth        collections.Map[string, types.StreamAuthorization]
	StreamByMP        collections.KeySet[collections.Pair[string, string]]
	StreamByExpiry    collections.KeySet[collections.Pair[int64, string]]
	StreamIDSeq       collections.Sequence
}

func NewKeeper(
	cdc codec.Codec,
	storeService corestore.KVStoreService,
	authority string,
	did types.DIDKeeper,
	device types.DeviceKeeper,
) Keeper {
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		did:          did,
		device:       device,

		Params: collections.NewItem(sb, types.ParamsCollectionPrefix, "params",
			codec.CollValue[types.Params](cdc)),

		MeteringPoints: collections.NewMap(sb, types.MeteringPointCollectionPrefix, "metering_points",
			collections.StringKey, codec.CollValue[types.MeteringPoint](cdc)),
		MPByOwner: collections.NewKeySet(sb, types.MeteringPointByOwnerPrefix, "mp_by_owner",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey)),
		MPByZone: collections.NewKeySet(sb, types.MeteringPointByZonePrefix, "mp_by_zone",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey)),

		Readings: collections.NewMap(sb, types.ReadingCollectionPrefix, "readings",
			collections.PairKeyCodec(collections.StringKey, collections.Int64Key),
			codec.CollValue[types.Reading](cdc)),

		Batches: collections.NewMap(sb, types.BatchCollectionPrefix, "batches",
			collections.StringKey, codec.CollValue[types.Batch](cdc)),
		BatchByMP: collections.NewKeySet(sb, types.BatchByMPPrefix, "batch_by_mp",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey)),

		StreamAuth: collections.NewMap(sb, types.StreamAuthorizationPrefix, "stream_auth",
			collections.StringKey, codec.CollValue[types.StreamAuthorization](cdc)),
		StreamByMP: collections.NewKeySet(sb, types.StreamAuthByMPPrefix, "stream_by_mp",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey)),
		StreamByExpiry: collections.NewKeySet(sb, types.StreamAuthByExpiryPrefix, "stream_by_expiry",
			collections.PairKeyCodec(collections.Int64Key, collections.StringKey)),
		StreamIDSeq: collections.NewSequence(sb, types.StreamAuthIDSeqPrefix, "stream_id_seq"),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(fmt.Errorf("meter keeper: build schema: %w", err))
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
			ctx.Logger().Error("meter params decode", "err", err)
		}
		return types.DefaultParams()
	}
	return p
}

// ---------------------------------------------------------------------------
// Metering points
// ---------------------------------------------------------------------------

func (k Keeper) SetMeteringPoint(ctx sdk.Context, mp types.MeteringPoint) error {
	if err := k.MeteringPoints.Set(ctx, mp.Id, mp); err != nil {
		return err
	}
	if mp.OwnerAddress != "" {
		if err := k.MPByOwner.Set(ctx, collections.Join(mp.OwnerAddress, mp.Id)); err != nil {
			return err
		}
	}
	if mp.GridZone != "" {
		if err := k.MPByZone.Set(ctx, collections.Join(mp.GridZone, mp.Id)); err != nil {
			return err
		}
	}
	return nil
}

// updateMeteringPoint rewrites the primary row and re-indexes the
// secondary maps if the owner / zone changed. Deactivation reuses this
// path with a flipped Active flag.
func (k Keeper) updateMeteringPoint(ctx sdk.Context, prior, next types.MeteringPoint) error {
	if prior.OwnerAddress != next.OwnerAddress && prior.OwnerAddress != "" {
		if err := k.MPByOwner.Remove(ctx, collections.Join(prior.OwnerAddress, prior.Id)); err != nil {
			return err
		}
	}
	if prior.GridZone != next.GridZone && prior.GridZone != "" {
		if err := k.MPByZone.Remove(ctx, collections.Join(prior.GridZone, prior.Id)); err != nil {
			return err
		}
	}
	return k.SetMeteringPoint(ctx, next)
}

func (k Keeper) GetMeteringPoint(ctx sdk.Context, id string) (types.MeteringPoint, bool) {
	mp, err := k.MeteringPoints.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("meter mp decode", "id", id, "err", err)
		}
		return types.MeteringPoint{}, false
	}
	return mp, true
}

func (k Keeper) HasMeteringPoint(ctx sdk.Context, id string) bool {
	has, _ := k.MeteringPoints.Has(ctx, id)
	return has
}

// ---------------------------------------------------------------------------
// Readings
// ---------------------------------------------------------------------------

func (k Keeper) SetReading(ctx sdk.Context, r types.Reading) error {
	return k.Readings.Set(ctx, collections.Join(r.MeteringPointId, r.StartTime), r)
}

func (k Keeper) GetReading(ctx sdk.Context, mpID string, startTime int64) (types.Reading, bool) {
	r, err := k.Readings.Get(ctx, collections.Join(mpID, startTime))
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("meter reading decode", "mp", mpID, "start", startTime, "err", err)
		}
		return types.Reading{}, false
	}
	return r, true
}

// ---------------------------------------------------------------------------
// Batches
// ---------------------------------------------------------------------------

func (k Keeper) SetBatch(ctx sdk.Context, b types.Batch) error {
	if err := k.Batches.Set(ctx, b.Id, b); err != nil {
		return err
	}
	return k.BatchByMP.Set(ctx, collections.Join(b.MeteringPointId, b.Id))
}

func (k Keeper) GetBatch(ctx sdk.Context, id string) (types.Batch, bool) {
	b, err := k.Batches.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("meter batch decode", "id", id, "err", err)
		}
		return types.Batch{}, false
	}
	return b, true
}

// ---------------------------------------------------------------------------
// Stream authorisations
// ---------------------------------------------------------------------------

func (k Keeper) SetStreamAuth(ctx sdk.Context, a types.StreamAuthorization) error {
	if err := k.StreamAuth.Set(ctx, a.Id, a); err != nil {
		return err
	}
	if err := k.StreamByMP.Set(ctx, collections.Join(a.MeteringPointId, a.Id)); err != nil {
		return err
	}
	if a.ExpiresAt > 0 && !a.Revoked {
		if err := k.StreamByExpiry.Set(ctx, collections.Join(a.ExpiresAt, a.Id)); err != nil {
			return err
		}
	}
	return nil
}

func (k Keeper) GetStreamAuth(ctx sdk.Context, id string) (types.StreamAuthorization, bool) {
	a, err := k.StreamAuth.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("meter stream decode", "id", id, "err", err)
		}
		return types.StreamAuthorization{}, false
	}
	return a, true
}

// MarkStreamRevoked toggles the revoked flag and removes the expiry-queue
// entry so the EndBlocker sweep does not re-process it.
func (k Keeper) MarkStreamRevoked(ctx sdk.Context, a types.StreamAuthorization, reason string) error {
	if a.ExpiresAt > 0 {
		if err := k.StreamByExpiry.Remove(ctx, collections.Join(a.ExpiresAt, a.Id)); err != nil &&
			!errors.Is(err, collections.ErrNotFound) {
			return err
		}
	}
	a.Revoked = true
	a.RevokedAt = ctx.BlockTime().Unix()
	a.RevocationReason = reason
	return k.StreamAuth.Set(ctx, a.Id, a)
}

// NextStreamID is a strictly monotonic counter (collections.Sequence is
// shared-prefix safe) used to mint canonical authorisation ids.
func (k Keeper) NextStreamID(ctx sdk.Context) (uint64, error) {
	return k.StreamIDSeq.Next(ctx)
}

// ---------------------------------------------------------------------------
// Cross-module checks
// ---------------------------------------------------------------------------

// requireOwnerHasDID returns an error when the DID gating is wired and
// the owner has no active DID. Nil-safe: tests that don't wire x/did
// pass through.
func (k Keeper) requireOwnerHasDID(ctx sdk.Context, owner string) error {
	if k.did == nil {
		return nil
	}
	if !k.did.IsActive(ctx, owner) {
		return fmt.Errorf("owner %s has no active DID", owner)
	}
	return nil
}

// requireAttestedDevice gates the data path. Behaviour matrix:
//
//	param=false, keeper=nil  → pass  (dev/test, no gating)
//	param=false, keeper set  → pass  (gating opt-in via params)
//	param=true,  keeper set  → enforce IsAttested
//	param=true,  keeper=nil  → FAIL CLOSED  (misconfigured prod)
//
// The fail-closed path on the last row protects against a wiring bug
// where governance enables RequireAttestedDevice but the DeviceKeeper
// dependency is still nil — without the guard, every reading would
// silently bypass the attestation requirement.
func (k Keeper) requireAttestedDevice(ctx sdk.Context, deviceDID string) error {
	if !k.GetParams(ctx).RequireAttestedDevice {
		return nil
	}
	if k.device == nil {
		return fmt.Errorf("attestation required but device keeper not wired (governance must wire x/device or disable require_attested_device)")
	}
	if !k.device.IsAttested(ctx, deviceDID) {
		return fmt.Errorf("device %s not attested", deviceDID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Genesis
// ---------------------------------------------------------------------------

func (k Keeper) InitGenesis(ctx sdk.Context, gs types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return fmt.Errorf("set params: %w", err)
	}
	for i, mp := range gs.MeteringPoints {
		if err := k.SetMeteringPoint(ctx, mp); err != nil {
			return fmt.Errorf("metering_point %d: %w", i, err)
		}
	}
	for i, r := range gs.Readings {
		if err := k.SetReading(ctx, r); err != nil {
			return fmt.Errorf("reading %d: %w", i, err)
		}
	}
	for i, b := range gs.Batches {
		if err := k.SetBatch(ctx, b); err != nil {
			return fmt.Errorf("batch %d: %w", i, err)
		}
	}
	maxSeq := uint64(0)
	for i, a := range gs.Authorizations {
		if err := k.SetStreamAuth(ctx, a); err != nil {
			return fmt.Errorf("authorization %d: %w", i, err)
		}
		if n := parseStreamSeq(a.Id); n > maxSeq {
			maxSeq = n
		}
	}
	if maxSeq > 0 {
		// Seed the sequence so freshly-issued ids don't collide with
		// genesis-imported ones.
		if err := k.StreamIDSeq.Set(ctx, maxSeq+1); err != nil {
			return fmt.Errorf("seed stream sequence: %w", err)
		}
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	gs := types.DefaultGenesis()
	gs.Params = k.GetParams(ctx)

	_ = k.MeteringPoints.Walk(ctx, nil, func(_ string, mp types.MeteringPoint) (bool, error) {
		gs.MeteringPoints = append(gs.MeteringPoints, mp)
		return false, nil
	})
	_ = k.Readings.Walk(ctx, nil, func(_ collections.Pair[string, int64], r types.Reading) (bool, error) {
		gs.Readings = append(gs.Readings, r)
		return false, nil
	})
	_ = k.Batches.Walk(ctx, nil, func(_ string, b types.Batch) (bool, error) {
		gs.Batches = append(gs.Batches, b)
		return false, nil
	})
	_ = k.StreamAuth.Walk(ctx, nil, func(_ string, a types.StreamAuthorization) (bool, error) {
		gs.Authorizations = append(gs.Authorizations, a)
		return false, nil
	})
	return gs
}
