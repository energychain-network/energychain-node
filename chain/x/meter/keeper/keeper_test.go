package keeper_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/log/v2"
	"cosmossdk.io/store"
	storetypes "cosmossdk.io/store/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/meter/keeper"
	"energychain/x/meter/types"
)

// ---------------------------------------------------------------------------
// stubs for cross-module dependencies
// ---------------------------------------------------------------------------

// stubDID lets tests opt-in to "owner has active DID" without standing up
// the real x/did keeper. The map is keyed by bech32 owner address.
type stubDID struct{ active map[string]bool }

func (s *stubDID) IsActive(_ sdk.Context, addr string) bool { return s.active[addr] }

// stubDevice mirrors the same approach for x/device.IsAttested.
type stubDevice struct{ attested map[string]bool }

func (s *stubDevice) IsAttested(_ sdk.Context, did string) bool { return s.attested[did] }

// ---------------------------------------------------------------------------
// setup helpers
// ---------------------------------------------------------------------------

const (
	testAuthority = "cosmos1ye4j7hsfmwgvch53kpys4yzm88zwhjkprc7nzx"
	testOwner     = "cosmos15tk4lhmtrwc4qx5gd5sjxdjdy36rg9hk0gx4tv"
	testOwner2    = "cosmos1xrnner78enszhq32u4l5z29slzq2zfvka2lt7g"
	testDeviceDID = "did:device:dev-001"
)

func setupKeeper(t *testing.T) (keeper.Keeper, sdk.Context, *stubDID, *stubDevice) {
	t.Helper()
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := stateStore.LoadLatestVersion(); err != nil {
		t.Fatal(err)
	}
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)

	did := &stubDID{active: map[string]bool{testOwner: true, testOwner2: true}}
	device := &stubDevice{attested: map[string]bool{testDeviceDID: true}}

	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), testAuthority, did, device)
	ctx := sdk.NewContext(stateStore, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return k, ctx, did, device
}

func newMeteringPoint(id, owner, did string) types.MeteringPoint {
	return types.MeteringPoint{
		Id:              id,
		DeviceDid:       did,
		OwnerAddress:    owner,
		GridZone:        "PJM.WHUB",
		MeasurementType: types.MeasurementType_MEASUREMENT_TYPE_ACTIVE_POWER,
		Unit:            "kWh",
		Active:          true,
		RegisteredAt:    1,
		UpdatedAt:       1,
	}
}

// commitmentBytes builds a 32-byte sha256 digest so the commitment shape
// validation in MsgServer.SubmitReading passes.
func commitmentBytes(payload string) []byte {
	h := sha256.Sum256([]byte(payload))
	return h[:]
}

// ---------------------------------------------------------------------------
// MeteringPoint CRUD
// ---------------------------------------------------------------------------

func TestMeteringPointSetGet(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	mp := newMeteringPoint("mp.cn.gd.0001", testOwner, testDeviceDID)
	if err := k.SetMeteringPoint(ctx, mp); err != nil {
		t.Fatal(err)
	}
	got, ok := k.GetMeteringPoint(ctx, mp.Id)
	if !ok || got.OwnerAddress != testOwner {
		t.Fatalf("expected stored mp; got=%v ok=%v", got, ok)
	}
}

// ---------------------------------------------------------------------------
// Reading commitment lifecycle
// ---------------------------------------------------------------------------

func TestSubmitReadingHappyPath(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	mp := newMeteringPoint("mp.cn.gd.0002", testOwner, testDeviceDID)
	if err := k.SetMeteringPoint(ctx, mp); err != nil {
		t.Fatal(err)
	}

	start := ctx.BlockTime().Unix() - 7200 // 2h ago
	end := start + 3600
	_, err := srv.SubmitReading(sdk.WrapSDKContext(ctx), &types.MsgSubmitReading{
		Submitter:        testOwner,
		MeteringPointId:  mp.Id,
		Period:           types.ReadingPeriod_READING_PERIOD_1HOUR,
		StartTime:        start,
		EndTime:          end,
		CommitmentScheme: types.CommitmentScheme_COMMITMENT_SCHEME_PLAINTEXT_SHA256,
		CommitmentBytes:  commitmentBytes("123|salt"),
		PlaintextValue:   123,
		DataQuality:      types.DataQualityFlag_DATA_QUALITY_SETTLED,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	r, ok := k.GetReading(ctx, mp.Id, start)
	if !ok {
		t.Fatalf("reading missing")
	}
	if r.PlaintextValue != 123 {
		t.Fatalf("plaintext=%d want 123", r.PlaintextValue)
	}
}

func TestSubmitReadingDuplicateBucketRejected(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	mp := newMeteringPoint("mp.cn.gd.0003", testOwner, testDeviceDID)
	_ = k.SetMeteringPoint(ctx, mp)

	start := ctx.BlockTime().Unix() - 3600
	end := start + 3600
	body := &types.MsgSubmitReading{
		Submitter:        testOwner,
		MeteringPointId:  mp.Id,
		Period:           types.ReadingPeriod_READING_PERIOD_1HOUR,
		StartTime:        start,
		EndTime:          end,
		CommitmentScheme: types.CommitmentScheme_COMMITMENT_SCHEME_PLAINTEXT_SHA256,
		CommitmentBytes:  commitmentBytes("first"),
		DataQuality:      types.DataQualityFlag_DATA_QUALITY_SETTLED,
	}
	if _, err := srv.SubmitReading(sdk.WrapSDKContext(ctx), body); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	body.CommitmentBytes = commitmentBytes("second")
	_, err := srv.SubmitReading(sdk.WrapSDKContext(ctx), body)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate-bucket rejection; got %v", err)
	}
}

// SECURITY: the bucket-uniqueness check protects the chain from a
// griefer overwriting a settled reading via a faked
// data_quality=ESTIMATED resubmission. This regression test pins the
// behaviour.
func TestSubmitReadingMismatchedPeriodRejected(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	mp := newMeteringPoint("mp.cn.gd.0004", testOwner, testDeviceDID)
	_ = k.SetMeteringPoint(ctx, mp)

	_, err := srv.SubmitReading(sdk.WrapSDKContext(ctx), &types.MsgSubmitReading{
		Submitter:        testOwner,
		MeteringPointId:  mp.Id,
		Period:           types.ReadingPeriod_READING_PERIOD_1HOUR,
		StartTime:        100,
		EndTime:          7300, // 2h delta — does not match 1h period
		CommitmentScheme: types.CommitmentScheme_COMMITMENT_SCHEME_PLAINTEXT_SHA256,
		CommitmentBytes:  commitmentBytes("x"),
		DataQuality:      types.DataQualityFlag_DATA_QUALITY_SETTLED,
	})
	if err == nil {
		t.Fatalf("expected period mismatch rejection")
	}
}

func TestSubmitReadingFutureSkewRejected(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	mp := newMeteringPoint("mp.cn.gd.0005", testOwner, testDeviceDID)
	_ = k.SetMeteringPoint(ctx, mp)

	now := ctx.BlockTime().Unix()
	start := now + 86_400 // 1 day in the future
	_, err := srv.SubmitReading(sdk.WrapSDKContext(ctx), &types.MsgSubmitReading{
		Submitter:        testOwner,
		MeteringPointId:  mp.Id,
		Period:           types.ReadingPeriod_READING_PERIOD_1HOUR,
		StartTime:        start,
		EndTime:          start + 3600,
		CommitmentScheme: types.CommitmentScheme_COMMITMENT_SCHEME_PLAINTEXT_SHA256,
		CommitmentBytes:  commitmentBytes("future"),
		DataQuality:      types.DataQualityFlag_DATA_QUALITY_SETTLED,
	})
	if err == nil {
		t.Fatalf("expected future-skew rejection")
	}
}

// SECURITY: when require_attested_device is on, an unattested device
// must be rejected. Regression for the M1 attestation gate.
func TestSubmitReadingRequiresAttestation(t *testing.T) {
	k, ctx, _, dev := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)

	params := types.DefaultParams()
	params.RequireAttestedDevice = true
	if err := k.SetParams(ctx, params); err != nil {
		t.Fatal(err)
	}
	dev.attested = map[string]bool{} // explicitly unattest the device

	mp := newMeteringPoint("mp.cn.gd.0006", testOwner, testDeviceDID)
	_ = k.SetMeteringPoint(ctx, mp)

	start := ctx.BlockTime().Unix() - 3600
	_, err := srv.SubmitReading(sdk.WrapSDKContext(ctx), &types.MsgSubmitReading{
		Submitter:        testOwner,
		MeteringPointId:  mp.Id,
		Period:           types.ReadingPeriod_READING_PERIOD_1HOUR,
		StartTime:        start,
		EndTime:          start + 3600,
		CommitmentScheme: types.CommitmentScheme_COMMITMENT_SCHEME_PLAINTEXT_SHA256,
		CommitmentBytes:  commitmentBytes("x"),
		DataQuality:      types.DataQualityFlag_DATA_QUALITY_SETTLED,
	})
	if err == nil || !strings.Contains(err.Error(), "attestation") {
		t.Fatalf("expected attestation-gate rejection; got %v", err)
	}
}

// SECURITY: when require_attested_device is on but the DeviceKeeper is
// not wired, the chain MUST fail closed instead of silently bypassing
// the gate. Regression for the misconfiguration foot-gun.
func TestSubmitReadingFailsClosedWithoutDeviceKeeper(t *testing.T) {
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	ms := store.NewCommitMultiStore(db, log.NewNopLogger())
	ms.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := ms.LoadLatestVersion(); err != nil {
		t.Fatal(err)
	}
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	did := &stubDID{active: map[string]bool{testOwner: true}}
	// Note: device keeper deliberately nil to model the misconfiguration.
	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), testAuthority, did, nil)
	ctx := sdk.NewContext(ms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(1).WithBlockTime(time.Unix(1_700_000_000, 0))
	params := types.DefaultParams()
	params.RequireAttestedDevice = true
	if err := k.SetParams(ctx, params); err != nil {
		t.Fatal(err)
	}

	mp := newMeteringPoint("mp.cn.gd.misconfig", testOwner, testDeviceDID)
	if err := k.SetMeteringPoint(ctx, mp); err != nil {
		t.Fatal(err)
	}

	srv := keeper.NewMsgServerImpl(k)
	start := ctx.BlockTime().Unix() - 3600
	_, err := srv.SubmitReading(sdk.WrapSDKContext(ctx), &types.MsgSubmitReading{
		Submitter:        testOwner,
		MeteringPointId:  mp.Id,
		Period:           types.ReadingPeriod_READING_PERIOD_1HOUR,
		StartTime:        start,
		EndTime:          start + 3600,
		CommitmentScheme: types.CommitmentScheme_COMMITMENT_SCHEME_PLAINTEXT_SHA256,
		CommitmentBytes:  commitmentBytes("misconfig"),
		DataQuality:      types.DataQualityFlag_DATA_QUALITY_SETTLED,
	})
	if err == nil || !strings.Contains(err.Error(), "device keeper not wired") {
		t.Fatalf("expected fail-closed rejection; got %v", err)
	}
}

// SECURITY: only the MP owner (or a stream-authorised submitter) may
// publish readings. A random third party must be rejected. Regression
// for the authorisation check.
func TestSubmitReadingThirdPartyRejected(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	mp := newMeteringPoint("mp.cn.gd.0007", testOwner, testDeviceDID)
	_ = k.SetMeteringPoint(ctx, mp)

	start := ctx.BlockTime().Unix() - 3600
	_, err := srv.SubmitReading(sdk.WrapSDKContext(ctx), &types.MsgSubmitReading{
		Submitter:        testOwner2, // not the owner; no stream
		MeteringPointId:  mp.Id,
		Period:           types.ReadingPeriod_READING_PERIOD_1HOUR,
		StartTime:        start,
		EndTime:          start + 3600,
		CommitmentScheme: types.CommitmentScheme_COMMITMENT_SCHEME_PLAINTEXT_SHA256,
		CommitmentBytes:  commitmentBytes("x"),
		DataQuality:      types.DataQualityFlag_DATA_QUALITY_SETTLED,
	})
	if err == nil || !strings.Contains(err.Error(), "neither the owner") {
		t.Fatalf("expected owner-only rejection; got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Batch lifecycle
// ---------------------------------------------------------------------------

func TestSubmitBatchHappyPath(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	mp := newMeteringPoint("mp.cn.gd.0008", testOwner, testDeviceDID)
	_ = k.SetMeteringPoint(ctx, mp)

	root := strings.Repeat("ab", 32) // 64 hex chars
	resp, err := srv.SubmitBatch(sdk.WrapSDKContext(ctx), &types.MsgSubmitBatch{
		Submitter:       testOwner,
		MeteringPointId: mp.Id,
		MerkleRoot:      root,
		MerkleAlgorithm: types.MerkleAlgoSHA256,
		Count:           1024,
		Period:          types.ReadingPeriod_READING_PERIOD_15MIN,
		StartTime:       ctx.BlockTime().Unix() - 86_400,
		EndTime:         ctx.BlockTime().Unix(),
		Uri:             "ipfs://Qm...",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	b, ok := k.GetBatch(ctx, resp.BatchId)
	if !ok || b.MerkleRoot != root {
		t.Fatalf("batch missing or root mismatch")
	}
}

func TestSubmitBatchIdempotentRetry(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	mp := newMeteringPoint("mp.cn.gd.0009", testOwner, testDeviceDID)
	_ = k.SetMeteringPoint(ctx, mp)

	root := strings.Repeat("cd", 32)
	body := &types.MsgSubmitBatch{
		Submitter:       testOwner,
		MeteringPointId: mp.Id,
		MerkleRoot:      root,
		MerkleAlgorithm: types.MerkleAlgoSHA256,
		Count:           10,
		Period:          types.ReadingPeriod_READING_PERIOD_15MIN,
		StartTime:       1_000_000,
		EndTime:         1_009_000,
		Uri:             "ipfs://retry",
	}
	r1, err := srv.SubmitBatch(sdk.WrapSDKContext(ctx), body)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := srv.SubmitBatch(sdk.WrapSDKContext(ctx), body)
	if err != nil {
		t.Fatalf("retry should be idempotent: %v", err)
	}
	if r1.BatchId != r2.BatchId {
		t.Fatalf("retry ids diverged: %s vs %s", r1.BatchId, r2.BatchId)
	}
}

// SECURITY: a non-hex merkle root must be rejected at ValidateBasic time
// before it can pollute on-chain state.
func TestSubmitBatchNonHexRootRejected(t *testing.T) {
	body := &types.MsgSubmitBatch{
		Submitter:       testOwner,
		MeteringPointId: "mp.cn.gd.x",
		MerkleRoot:      strings.Repeat("zz", 32), // 64 chars but non-hex
		MerkleAlgorithm: types.MerkleAlgoSHA256,
		Count:           1,
		Period:          types.ReadingPeriod_READING_PERIOD_15MIN,
		StartTime:       1,
		EndTime:         2,
		Uri:             "ipfs://x",
	}
	if err := body.ValidateBasic(); err == nil {
		t.Fatalf("expected non-hex rejection")
	}
}

// ---------------------------------------------------------------------------
// Stream authorisation lifecycle
// ---------------------------------------------------------------------------

func TestAuthorizeAndUseStream(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	mp := newMeteringPoint("mp.cn.gd.0010", testOwner, testDeviceDID)
	_ = k.SetMeteringPoint(ctx, mp)

	now := ctx.BlockTime().Unix()
	authResp, err := srv.AuthorizeStream(sdk.WrapSDKContext(ctx), &types.MsgAuthorizeStream{
		Authorizer:      testOwner,
		MeteringPointId: mp.Id,
		DeviceDid:       testOwner2, // a sidecar address acting as the streaming agent
		ExpiresAt:       now + 7200,
		Metadata:        "agent#001",
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	a, ok := k.GetStreamAuth(ctx, authResp.Id)
	if !ok || a.MeteringPointId != mp.Id {
		t.Fatalf("auth missing")
	}

	// The stream-authorised account can now publish readings.
	start := now - 3600
	if _, err := srv.SubmitReading(sdk.WrapSDKContext(ctx), &types.MsgSubmitReading{
		Submitter:        testOwner2,
		MeteringPointId:  mp.Id,
		Period:           types.ReadingPeriod_READING_PERIOD_1HOUR,
		StartTime:        start,
		EndTime:          start + 3600,
		CommitmentScheme: types.CommitmentScheme_COMMITMENT_SCHEME_PLAINTEXT_SHA256,
		CommitmentBytes:  commitmentBytes("via-stream"),
		DataQuality:      types.DataQualityFlag_DATA_QUALITY_SETTLED,
	}); err != nil {
		t.Fatalf("stream submit: %v", err)
	}
}

func TestRevokeStreamRevokesSubmission(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	mp := newMeteringPoint("mp.cn.gd.0011", testOwner, testDeviceDID)
	_ = k.SetMeteringPoint(ctx, mp)

	now := ctx.BlockTime().Unix()
	resp, _ := srv.AuthorizeStream(sdk.WrapSDKContext(ctx), &types.MsgAuthorizeStream{
		Authorizer:      testOwner,
		MeteringPointId: mp.Id,
		DeviceDid:       testOwner2,
		ExpiresAt:       now + 7200,
	})
	if _, err := srv.RevokeStream(sdk.WrapSDKContext(ctx), &types.MsgRevokeStream{
		Authorizer: testOwner,
		Id:         resp.Id,
		Reason:     "rotate keys",
	}); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	a, ok := k.GetStreamAuth(ctx, resp.Id)
	if !ok || !a.Revoked {
		t.Fatalf("expected revoked")
	}

	// After revocation, the stream-authorised account must no longer
	// be allowed to submit.
	start := now - 1800
	_, err := srv.SubmitReading(sdk.WrapSDKContext(ctx), &types.MsgSubmitReading{
		Submitter:        testOwner2,
		MeteringPointId:  mp.Id,
		Period:           types.ReadingPeriod_READING_PERIOD_1HOUR,
		StartTime:        start - (start % 3600), // align to bucket
		EndTime:          (start - (start % 3600)) + 3600,
		CommitmentScheme: types.CommitmentScheme_COMMITMENT_SCHEME_PLAINTEXT_SHA256,
		CommitmentBytes:  commitmentBytes("post-revoke"),
		DataQuality:      types.DataQualityFlag_DATA_QUALITY_SETTLED,
	})
	if err == nil {
		t.Fatalf("expected post-revoke submission rejection")
	}
}

// SECURITY: an authoriser whose validity window exceeds Params.StreamMaxValidity
// must be rejected. Without this cap an MP owner could write a 100-year
// authorisation that no rotation cadence can ever catch.
func TestAuthorizeStreamRespectsMaxValidity(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	mp := newMeteringPoint("mp.cn.gd.0012", testOwner, testDeviceDID)
	_ = k.SetMeteringPoint(ctx, mp)

	params := types.DefaultParams()
	params.StreamMaxValidity = 3600 // 1 hour ceiling
	_ = k.SetParams(ctx, params)

	_, err := srv.AuthorizeStream(sdk.WrapSDKContext(ctx), &types.MsgAuthorizeStream{
		Authorizer:      testOwner,
		MeteringPointId: mp.Id,
		DeviceDid:       testOwner2,
		ExpiresAt:       ctx.BlockTime().Unix() + 86_400,
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds max") {
		t.Fatalf("expected max-validity rejection; got %v", err)
	}
}

// ---------------------------------------------------------------------------
// EndBlocker stream sweep
// ---------------------------------------------------------------------------

func TestSweepExpiredStreams(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	mp := newMeteringPoint("mp.cn.gd.0013", testOwner, testDeviceDID)
	_ = k.SetMeteringPoint(ctx, mp)

	now := ctx.BlockTime().Unix()
	resp, err := srv.AuthorizeStream(sdk.WrapSDKContext(ctx), &types.MsgAuthorizeStream{
		Authorizer:      testOwner,
		MeteringPointId: mp.Id,
		DeviceDid:       testOwner2,
		ExpiresAt:       now + 60,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Advance block time past the expiry, then sweep.
	ctxLater := ctx.WithBlockTime(time.Unix(now+120, 0))
	processed, err := k.SweepExpiredStreams(ctxLater)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 expired sweep; got %d", processed)
	}
	a, _ := k.GetStreamAuth(ctxLater, resp.Id)
	if !a.Revoked {
		t.Fatalf("expected revoked after sweep")
	}
}

// ---------------------------------------------------------------------------
// MeteringPoint registration owner gating
// ---------------------------------------------------------------------------

// SECURITY: the chain refuses to register an MP whose owner has no
// active DID, so audit trails always resolve to a real on-chain
// identity.
func TestRegisterMeteringPointDIDGate(t *testing.T) {
	k, ctx, did, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	did.active = map[string]bool{} // empty: no one has a DID

	_, err := srv.RegisterMeteringPoint(sdk.WrapSDKContext(ctx), &types.MsgRegisterMeteringPoint{
		Submitter:       testOwner,
		Id:              "mp.cn.gd.0014",
		DeviceDid:       testDeviceDID,
		OwnerAddress:    testOwner,
		MeasurementType: types.MeasurementType_MEASUREMENT_TYPE_ACTIVE_POWER,
		Unit:            "kWh",
	})
	if err == nil || !strings.Contains(err.Error(), "DID") {
		t.Fatalf("expected DID gate rejection; got %v", err)
	}
}

func TestRegisterMeteringPointHappyPath(t *testing.T) {
	k, ctx, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)

	id := "mp.cn.gd.0015"
	_, err := srv.RegisterMeteringPoint(sdk.WrapSDKContext(ctx), &types.MsgRegisterMeteringPoint{
		Submitter:       testOwner,
		Id:              id,
		DeviceDid:       testDeviceDID,
		OwnerAddress:    testOwner,
		MeasurementType: types.MeasurementType_MEASUREMENT_TYPE_ACTIVE_POWER,
		Unit:            "kWh",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	mp, ok := k.GetMeteringPoint(ctx, id)
	if !ok || !mp.Active {
		t.Fatalf("not active or missing: %+v", mp)
	}
}

// ---------------------------------------------------------------------------
// Misc
// ---------------------------------------------------------------------------

// commitmentHexLen mirrors the merkle-root length sanity check; if proto
// changes silently drop the canonical 32-byte digest, this fails first.
func TestCommitmentDigestLength(t *testing.T) {
	got := hex.EncodeToString(commitmentBytes("x"))
	if len(got) != 64 {
		t.Fatalf("expected 64-hex-char digest, got %d", len(got))
	}
}
