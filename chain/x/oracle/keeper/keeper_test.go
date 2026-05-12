package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/log/v2"
	sdkmath "cosmossdk.io/math"
	"cosmossdk.io/store"
	storetypes "cosmossdk.io/store/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/oracle/keeper"
	"energychain/x/oracle/types"
)

// setupKeeper builds a fresh, in-memory keeper backed by an IAVL store
// and a no-op logger. We pass nil for the BankKeeper so EscrowFrom /
// ReleaseTo become no-ops; the keeper-level tests reason about state
// transitions, not coin flows.
func setupKeeper(t *testing.T) (keeper.Keeper, sdk.Context) {
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
	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), "authority", nil)
	ctx := sdk.NewContext(stateStore, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(1).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	return k, ctx
}

func mustSetParams(t *testing.T, k keeper.Keeper, ctx sdk.Context, p types.Params) {
	t.Helper()
	if err := k.SetParams(ctx, p); err != nil {
		t.Fatal(err)
	}
}

func newTopic(id string, agg types.AggregationFn, minSubs uint32, band uint32) types.Topic {
	return types.Topic{
		Id:                id,
		Description:       "test topic",
		Kind:              types.TopicKind_TOPIC_KIND_PRICE,
		Aggregation:       agg,
		MinSubmissions:    minSubs,
		MaxDataAgeSeconds: 600,
		OutlierBandBps:    band,
		ValueDecimals:     2,
		Quote:             "USD",
		Enabled:           true,
	}
}

// ---------------------------------------------------------------------------
// Topic CRUD
// ---------------------------------------------------------------------------

func TestTopicSetGet(t *testing.T) {
	k, ctx := setupKeeper(t)
	tp := newTopic("power.spot.PJM.WHUB", types.AggregationFn_AGG_MEDIAN, 3, 0)
	if err := k.SetTopic(ctx, tp); err != nil {
		t.Fatal(err)
	}
	got, ok := k.GetTopic(ctx, tp.Id)
	if !ok || got.Id != tp.Id {
		t.Fatalf("topic not found")
	}
}

// ---------------------------------------------------------------------------
// Provider lifecycle
// ---------------------------------------------------------------------------

func TestProviderRegisterAndStatus(t *testing.T) {
	k, ctx := setupKeeper(t)
	p := types.Provider{
		Address:  "energy1prov",
		Status:   types.ProviderStatus_PROVIDER_STATUS_ACTIVE,
		Bond:     sdk.NewCoin(types.DefaultBondDenom, sdkmath.NewInt(100)),
		JoinedAt: ctx.BlockTime().Unix(),
	}
	if err := k.SetProvider(ctx, p); err != nil {
		t.Fatal(err)
	}
	if !k.IsActiveProvider(ctx, p.Address, "") {
		t.Fatalf("expected active")
	}
}

// ---------------------------------------------------------------------------
// Aggregation correctness
// ---------------------------------------------------------------------------

func TestAggregateMedianHappyPath(t *testing.T) {
	k, ctx := setupKeeper(t)
	mustSetParams(t, k, ctx, types.DefaultParams())

	tp := newTopic("price.eur.usd", types.AggregationFn_AGG_MEDIAN, 3, 0)
	if err := k.SetTopic(ctx, tp); err != nil {
		t.Fatal(err)
	}

	now := ctx.BlockTime().Unix()
	values := []int64{100, 102, 110, 108, 105}
	for i, v := range values {
		addr := "energy1p" + string(rune('a'+i))
		if err := k.SetProvider(ctx, types.Provider{
			Address: addr, Status: types.ProviderStatus_PROVIDER_STATUS_ACTIVE,
			Bond:    sdk.NewCoin(types.DefaultBondDenom, sdkmath.OneInt()),
		}); err != nil {
			t.Fatal(err)
		}
		if err := k.SetSubmission(ctx, types.Submission{
			TopicId: tp.Id, Provider: addr, Value: v,
			Timestamp: now, Height: ctx.BlockHeight(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	count, err := k.AggregateAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 topic aggregated, got %d", count)
	}

	got, ok := k.GetAggregated(ctx, tp.Id)
	if !ok {
		t.Fatalf("aggregated value not stored")
	}
	if got.Value != 105 {
		t.Errorf("median: want 105, got %d", got.Value)
	}
	if got.NumSources != 5 {
		t.Errorf("num_sources: want 5, got %d", got.NumSources)
	}
	if got.SourceMin != 100 || got.SourceMax != 110 {
		t.Errorf("source spread mismatch: min=%d max=%d", got.SourceMin, got.SourceMax)
	}
}

// TestAggregateOutlierBand: one extreme outlier is dropped before
// computing the aggregate.
func TestAggregateOutlierBand(t *testing.T) {
	k, ctx := setupKeeper(t)
	mustSetParams(t, k, ctx, types.DefaultParams())

	// 2000 bps = ±20% of median. Submissions: 100, 102, 105, 108, 999.
	// Median = 105 → band [84, 126]. 999 dropped; the rest aggregate.
	tp := newTopic("price.weird", types.AggregationFn_AGG_MEDIAN, 3, 2000)
	if err := k.SetTopic(ctx, tp); err != nil {
		t.Fatal(err)
	}

	now := ctx.BlockTime().Unix()
	for i, v := range []int64{100, 102, 105, 108, 999} {
		addr := "energy1q" + string(rune('a'+i))
		_ = k.SetProvider(ctx, types.Provider{
			Address: addr, Status: types.ProviderStatus_PROVIDER_STATUS_ACTIVE,
			Bond:    sdk.NewCoin(types.DefaultBondDenom, sdkmath.OneInt()),
		})
		_ = k.SetSubmission(ctx, types.Submission{
			TopicId: tp.Id, Provider: addr, Value: v,
			Timestamp: now, Height: ctx.BlockHeight(),
		})
	}
	if _, err := k.AggregateAll(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := k.GetAggregated(ctx, tp.Id)
	if got.NumSources != 4 {
		t.Errorf("outlier should have been dropped: num_sources=%d", got.NumSources)
	}
	if got.Value < 100 || got.Value > 110 {
		t.Errorf("median after trim should be ~105: got %d", got.Value)
	}
}

// TestAggregateRespectsMaxAge: stale submissions are skipped.
func TestAggregateRespectsMaxAge(t *testing.T) {
	k, ctx := setupKeeper(t)
	mustSetParams(t, k, ctx, types.DefaultParams())
	tp := newTopic("price.stale", types.AggregationFn_AGG_MEDIAN, 2, 0)
	tp.MaxDataAgeSeconds = 60
	if err := k.SetTopic(ctx, tp); err != nil {
		t.Fatal(err)
	}

	now := ctx.BlockTime().Unix()
	// Two stale, one fresh — should fail min_submissions=2 and store nothing.
	for i, v := range []int64{100, 102, 110} {
		addr := "energy1z" + string(rune('a'+i))
		_ = k.SetProvider(ctx, types.Provider{
			Address: addr, Status: types.ProviderStatus_PROVIDER_STATUS_ACTIVE,
			Bond:    sdk.NewCoin(types.DefaultBondDenom, sdkmath.OneInt()),
		})
		ts := now - 3600 // stale by an hour
		if i == 2 {
			ts = now // fresh
		}
		_ = k.SetSubmission(ctx, types.Submission{
			TopicId: tp.Id, Provider: addr, Value: v,
			Timestamp: ts, Height: ctx.BlockHeight(),
		})
	}
	if _, err := k.AggregateAll(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := k.GetAggregated(ctx, tp.Id); ok {
		t.Errorf("should not have aggregated with only 1 fresh source")
	}
}

// TestAggregateSkipsSuspendedProvider locks in that suspending a
// provider drops their submissions from the aggregate.
func TestAggregateSkipsSuspendedProvider(t *testing.T) {
	k, ctx := setupKeeper(t)
	mustSetParams(t, k, ctx, types.DefaultParams())
	tp := newTopic("price.susp", types.AggregationFn_AGG_MEDIAN, 2, 0)
	if err := k.SetTopic(ctx, tp); err != nil {
		t.Fatal(err)
	}
	now := ctx.BlockTime().Unix()
	addrs := []string{"energy1aa", "energy1bb", "energy1cc"}
	for i, a := range addrs {
		st := types.ProviderStatus_PROVIDER_STATUS_ACTIVE
		if i == 0 {
			st = types.ProviderStatus_PROVIDER_STATUS_SUSPENDED
		}
		_ = k.SetProvider(ctx, types.Provider{
			Address: a, Status: st,
			Bond: sdk.NewCoin(types.DefaultBondDenom, sdkmath.OneInt()),
		})
		_ = k.SetSubmission(ctx, types.Submission{
			TopicId: tp.Id, Provider: a, Value: int64(100 + i),
			Timestamp: now,
		})
	}
	_, _ = k.AggregateAll(ctx)
	got, _ := k.GetAggregated(ctx, tp.Id)
	if got.NumSources != 2 {
		t.Errorf("suspended provider must be excluded: num_sources=%d", got.NumSources)
	}
}

// TestAggregatePausedTopicSkipped guards against producing a value while
// a topic is paused.
func TestAggregatePausedTopicSkipped(t *testing.T) {
	k, ctx := setupKeeper(t)
	mustSetParams(t, k, ctx, types.DefaultParams())
	tp := newTopic("price.paused", types.AggregationFn_AGG_MEDIAN, 1, 0)
	tp.Paused = true
	if err := k.SetTopic(ctx, tp); err != nil {
		t.Fatal(err)
	}
	_ = k.SetProvider(ctx, types.Provider{
		Address: "energy1pp", Status: types.ProviderStatus_PROVIDER_STATUS_ACTIVE,
		Bond: sdk.NewCoin(types.DefaultBondDenom, sdkmath.OneInt()),
	})
	_ = k.SetSubmission(ctx, types.Submission{
		TopicId: tp.Id, Provider: "energy1pp", Value: 1, Timestamp: ctx.BlockTime().Unix(),
	})
	_, _ = k.AggregateAll(ctx)
	if _, ok := k.GetAggregated(ctx, tp.Id); ok {
		t.Errorf("paused topic should not aggregate")
	}
}

// TestSweepExpiredSuspensions verifies the EndBlock sweep flips
// providers back to ACTIVE once their suspension window passes.
func TestSweepExpiredSuspensions(t *testing.T) {
	k, ctx := setupKeeper(t)
	now := ctx.BlockTime().Unix()
	_ = k.SetProvider(ctx, types.Provider{
		Address:        "energy1xx",
		Status:         types.ProviderStatus_PROVIDER_STATUS_SUSPENDED,
		Bond:           sdk.NewCoin(types.DefaultBondDenom, sdkmath.OneInt()),
		SuspendedAt:    now - 100,
		SuspendedUntil: now - 1, // already past
		SuspendedReason: "test",
	})
	n, err := k.SweepExpiredSuspensions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("swept = %d, want 1", n)
	}
	got, _ := k.GetProvider(ctx, "energy1xx")
	if got.Status != types.ProviderStatus_PROVIDER_STATUS_ACTIVE {
		t.Errorf("expected ACTIVE after sweep, got %v", got.Status)
	}
}

// TestRequestAndWithdrawBondCooldown locks in the cooldown semantics.
func TestRequestAndWithdrawBondCooldown(t *testing.T) {
	k, ctx := setupKeeper(t)
	params := types.DefaultParams()
	params.BondReleaseCooldown = 10
	mustSetParams(t, k, ctx, params)

	addr := "energy1ee"
	_ = k.SetProvider(ctx, types.Provider{
		Address: addr, Status: types.ProviderStatus_PROVIDER_STATUS_ACTIVE,
		Bond: sdk.NewCoin(types.DefaultBondDenom, sdkmath.NewInt(1000)),
	})
	srv := keeper.NewMsgServerImpl(k)

	resp, err := srv.RequestWithdrawBond(ctx, &types.MsgRequestWithdrawBond{Address: addr})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ReleaseHeight != ctx.BlockHeight()+10 {
		t.Errorf("release_height wrong: got %d", resp.ReleaseHeight)
	}

	// Too early: rejected.
	if _, err := srv.WithdrawBond(ctx, &types.MsgWithdrawBond{Address: addr}); err == nil {
		t.Fatalf("expected cooldown rejection")
	}

	// Advance height past cooldown.
	ctx2 := ctx.WithBlockHeight(ctx.BlockHeight() + 11)
	if _, err := srv.WithdrawBond(ctx2, &types.MsgWithdrawBond{Address: addr}); err != nil {
		t.Fatalf("expected withdraw to succeed, got %v", err)
	}
	p, _ := k.GetProvider(ctx2, addr)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_RETIRED {
		t.Errorf("expected RETIRED, got %v", p.Status)
	}
}

// TestWithdrawBondHandlesZeroBond locks in that a fully-slashed
// provider can still complete the withdrawal flow (transitioning to
// RETIRED) without the keeper attempting an empty bank.Send.
func TestWithdrawBondHandlesZeroBond(t *testing.T) {
	k, ctx := setupKeeper(t)
	params := types.DefaultParams()
	params.BondReleaseCooldown = 1
	mustSetParams(t, k, ctx, params)

	addr := "energy1zz"
	_ = k.SetProvider(ctx, types.Provider{
		Address: addr,
		Status:  types.ProviderStatus_PROVIDER_STATUS_WITHDRAWING,
		Bond:    sdk.NewCoin(types.DefaultBondDenom, sdkmath.ZeroInt()), // already slashed
		BondReleaseHeight: ctx.BlockHeight(),
	})
	srv := keeper.NewMsgServerImpl(k)
	if _, err := srv.WithdrawBond(ctx, &types.MsgWithdrawBond{Address: addr}); err != nil {
		t.Fatalf("zero-bond withdraw should succeed and retire, got %v", err)
	}
	got, _ := k.GetProvider(ctx, addr)
	if got.Status != types.ProviderStatus_PROVIDER_STATUS_RETIRED {
		t.Errorf("expected RETIRED after zero-bond withdraw, got %v", got.Status)
	}
}

// TestSubmitValueRejectsTooOld locks in the per-topic max-age check on
// the submit path so a stale push can't dilute the median.
func TestSubmitValueRejectsTooOld(t *testing.T) {
	k, ctx := setupKeeper(t)
	mustSetParams(t, k, ctx, types.DefaultParams())
	tp := newTopic("price.push", types.AggregationFn_AGG_MEDIAN, 2, 0)
	tp.MaxDataAgeSeconds = 60
	if err := k.SetTopic(ctx, tp); err != nil {
		t.Fatal(err)
	}
	_ = k.SetProvider(ctx, types.Provider{
		Address: "energy1ww", Status: types.ProviderStatus_PROVIDER_STATUS_ACTIVE,
		Bond: sdk.NewCoin(types.DefaultBondDenom, sdkmath.OneInt()),
	})
	srv := keeper.NewMsgServerImpl(k)
	now := ctx.BlockTime().Unix()
	if _, err := srv.SubmitValue(ctx, &types.MsgSubmitValue{
		Provider: "energy1ww", TopicId: tp.Id, Value: 1, Timestamp: now - 3600,
	}); err == nil {
		t.Fatalf("expected too-old rejection")
	}
}

// TestSubmitValueRejectsFutureTimestamp guards against malicious clock
// skew that would otherwise let a provider front-run an aggregation
// window with a value timestamped in the future.
func TestSubmitValueRejectsFutureTimestamp(t *testing.T) {
	k, ctx := setupKeeper(t)
	mustSetParams(t, k, ctx, types.DefaultParams())
	tp := newTopic("price.fut", types.AggregationFn_AGG_MEDIAN, 1, 0)
	if err := k.SetTopic(ctx, tp); err != nil {
		t.Fatal(err)
	}
	_ = k.SetProvider(ctx, types.Provider{
		Address: "energy1ff", Status: types.ProviderStatus_PROVIDER_STATUS_ACTIVE,
		Bond: sdk.NewCoin(types.DefaultBondDenom, sdkmath.OneInt()),
	})
	srv := keeper.NewMsgServerImpl(k)
	if _, err := srv.SubmitValue(ctx, &types.MsgSubmitValue{
		Provider: "energy1ff", TopicId: tp.Id, Value: 1,
		Timestamp: ctx.BlockTime().Unix() + 3600,
	}); err == nil {
		t.Fatalf("expected future-timestamp rejection")
	}
}

// TestReserveAttestationRoundTrip verifies the PoR sub-interface stores
// + retrieves attestations and indexes them by asset for x/stablecoin.
func TestReserveAttestationRoundTrip(t *testing.T) {
	k, ctx := setupKeeper(t)
	mustSetParams(t, k, ctx, types.DefaultParams())
	srv := keeper.NewMsgServerImpl(k)
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if _, err := srv.SubmitReserveAttestation(ctx, &types.MsgSubmitReserveAttestation{
		Custodian:       "energy1custodian",
		Id:              "att-1",
		Asset:           "USDC",
		Amount:          "1000000000",
		AttestationUri:  "ipfs://Qm",
		AttestationHash: hash,
		Signers:         []string{"energy1a", "energy1b", "energy1c"},
		Threshold:       2,
	}); err != nil {
		t.Fatal(err)
	}
	got, ok := k.GetReserveAttestation(ctx, "att-1")
	if !ok {
		t.Fatalf("attestation not stored")
	}
	if got.Asset != "USDC" || got.Threshold != 2 {
		t.Errorf("attestation fields wrong: %+v", got)
	}
}
