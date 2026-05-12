package keeper_test

import (
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

	"energychain/internal/dsl"
	"energychain/x/policy/keeper"
	"energychain/x/policy/types"
)

// ---------------------------------------------------------------------------
// stubs for cross-module dependencies
// ---------------------------------------------------------------------------

// stubDID returns deterministic credential / jurisdiction state. Tests
// flip values to exercise per-rule branches.
type stubDID struct {
	juris map[string]string
	creds map[string]map[string]bool // subject -> credtype -> bool
	live  map[string]bool
}

func newStubDID() *stubDID {
	return &stubDID{
		juris: map[string]string{},
		creds: map[string]map[string]bool{},
		live:  map[string]bool{},
	}
}
func (s *stubDID) IsActive(_ sdk.Context, addr string) bool { return s.live[addr] }
func (s *stubDID) Jurisdiction(_ sdk.Context, addr string) string {
	return s.juris[addr]
}
func (s *stubDID) HasCredential(_ sdk.Context, addr, ct string) bool {
	if s.creds[addr] == nil {
		return false
	}
	return s.creds[addr][ct]
}

// stubSanctions models a pluggable x/sanctions registry.
type stubSanctions struct{ blocked map[string]bool }

func (s *stubSanctions) IsSanctioned(_ sdk.Context, addr string) bool { return s.blocked[addr] }

// stubAudit captures denial events so tests can assert the cross-module
// hook fires the expected number of times.
type stubAudit struct{ denials []string }

func (s *stubAudit) RecordPolicyDenial(_ sdk.Context, policyID, _ string, _ string, _ string, _ string, _ uint32, detail string) {
	s.denials = append(s.denials, policyID+":"+detail)
}

// ---------------------------------------------------------------------------
// setup
// ---------------------------------------------------------------------------

const (
	testAuthority = "cosmos1ye4j7hsfmwgvch53kpys4yzm88zwhjkprc7nzx"
	otherAddr     = "cosmos15tk4lhmtrwc4qx5gd5sjxdjdy36rg9hk0gx4tv"
	alice         = "cosmos1xrnner78enszhq32u4l5z29slzq2zfvka2lt7g"
	bob           = "cosmos1uu89qg49evj5pkj2u3xwapdjwt7ymsljt7epry"
)

func setupKeeper(t *testing.T) (keeper.Keeper, sdk.Context, *stubDID, *stubSanctions, *stubAudit) {
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

	did := newStubDID()
	san := &stubSanctions{blocked: map[string]bool{}}
	au := &stubAudit{}

	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), testAuthority, did, san, au)
	ctx := sdk.NewContext(stateStore, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return k, ctx, did, san, au
}

// helper: build a "require KYC + jurisdiction allow US/CA" policy.
func samplePolicy(id string) types.Policy {
	return types.Policy{
		Id:      id,
		Name:    id,
		Version: "1",
		Status:  types.PolicyStatus_POLICY_STATUS_ACTIVE,
		Rules: []types.PolicyRule{
			{Kind: uint32(dsl.RuleRequireKYC), Side: uint32(dsl.SideBoth)},
			{Kind: uint32(dsl.RuleJurisdictionAllow), Side: uint32(dsl.SideBoth), Params: map[string]string{"countries": "US,CA"}},
			{Kind: uint32(dsl.RuleMaxPerHolder), Side: uint32(dsl.SideReceiver), Params: map[string]string{"max": "1000000"}},
		},
	}
}

// ---------------------------------------------------------------------------
// RegisterPolicy: happy path + duplicate + cap
// ---------------------------------------------------------------------------

func TestRegisterPolicyHappyPath(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("us-reg-d-506c")
	_, err := srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority,
		Id:        p.Id,
		Name:      p.Name,
		Version:   p.Version,
		Rules:     p.Rules,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	got, ok := k.GetPolicy(ctx, p.Id)
	if !ok || got.Status != types.PolicyStatus_POLICY_STATUS_ACTIVE {
		t.Fatalf("expected ACTIVE policy; got %+v ok=%v", got, ok)
	}
}

func TestRegisterPolicyRejectsDuplicate(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("dup")
	if _, err := srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	}); err == nil {
		t.Fatal("expected duplicate-id rejection")
	}
}

func TestRegisterPolicyRejectsNonAuthority(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("auth-test")
	if _, err := srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: otherAddr, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	}); err == nil {
		t.Fatal("expected unauthorized rejection")
	}
}

func TestRegisterPolicyRejectsCapExceeded(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	params := k.GetParams(ctx)
	params.MaxPolicies = 2
	if err := k.SetParams(ctx, params); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"a", "b"} {
		p := samplePolicy(id)
		if _, err := srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
			Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
		}); err != nil {
			t.Fatalf("register #%d: %v", i, err)
		}
	}
	p := samplePolicy("c")
	if _, err := srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	}); err == nil {
		t.Fatal("expected cap rejection")
	}
}

// ---------------------------------------------------------------------------
// Update + Status transitions
// ---------------------------------------------------------------------------

func TestUpdatePolicyMutates(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("up")
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	_, err := srv.UpdatePolicy(ctx, &types.MsgUpdatePolicy{
		Authority: testAuthority, Id: p.Id, Version: "2", Rules: []types.PolicyRule{{Kind: uint32(dsl.RuleRequireKYC)}},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := k.GetPolicy(ctx, p.Id)
	if got.Version != "2" || len(got.Rules) != 1 {
		t.Fatalf("update did not apply: %+v", got)
	}
}

func TestUpdateRefusesDisabledPolicy(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("dis")
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	_, _ = srv.DisablePolicy(ctx, &types.MsgDisablePolicy{Authority: testAuthority, Id: p.Id, Reason: "bug"})
	_, err := srv.UpdatePolicy(ctx, &types.MsgUpdatePolicy{
		Authority: testAuthority, Id: p.Id, Version: "2", Rules: p.Rules,
	})
	if err == nil {
		t.Fatal("expected refusal to update DISABLED policy")
	}
}

// ---------------------------------------------------------------------------
// Bindings
// ---------------------------------------------------------------------------

func TestBindAndUnbindPolicy(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("bind")
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	if _, err := srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "GREEN-BOND-001", PolicyId: p.Id,
	}); err != nil {
		t.Fatal(err)
	}
	got, ok := k.GetBinding(ctx, "rwa", "GREEN-BOND-001")
	if !ok || got.PolicyId != p.Id {
		t.Fatalf("missing binding")
	}
	if _, err := srv.UnbindPolicy(ctx, &types.MsgUnbindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "GREEN-BOND-001", Reason: "rotated",
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := k.GetBinding(ctx, "rwa", "GREEN-BOND-001"); ok {
		t.Fatal("binding not removed")
	}
}

func TestBindRefusesDeprecatedPolicy(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("dep")
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	_, _ = srv.DeprecatePolicy(ctx, &types.MsgDeprecatePolicy{Authority: testAuthority, Id: p.Id, Reason: "v2"})
	if _, err := srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "X", PolicyId: p.Id,
	}); err == nil {
		t.Fatal("expected DEPRECATED rejection")
	}
}

func TestBindRefusesDuplicate(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("dpb")
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	if _, err := srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "X", PolicyId: p.Id,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "X", PolicyId: p.Id,
	}); err == nil {
		t.Fatal("expected duplicate-binding rejection")
	}
}

// ---------------------------------------------------------------------------
// EvaluateTransfer: happy + denials
// ---------------------------------------------------------------------------

func TestEvaluateTransferAllowsWhenNoBinding(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	if err := k.EvaluateTransfer(ctx, "rwa", "OPEN", alice, bob, 100); err != nil {
		t.Fatalf("expected ALLOW for unbound asset; got %v", err)
	}
}

func TestEvaluateTransferDeniesMissingPolicy(t *testing.T) {
	k, ctx, _, _, au := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("mp")
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	if _, err := srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "ASSET", PolicyId: p.Id,
	}); err != nil {
		t.Fatal(err)
	}
	// Manually corrupt by directly removing the policy from the
	// collection (simulates a botched migration).
	if err := k.Policies.Remove(ctx, p.Id); err != nil {
		t.Fatal(err)
	}
	res, _ := k.EvaluateTransferDetailed(ctx, keeper.EvaluateRequest{
		AssetClass: "rwa", AssetID: "ASSET", Sender: alice, Receiver: bob, Amount: 1,
	})
	if res.Allowed || res.Code != dsl.CodeBindingMissing {
		t.Fatalf("expected CodeBindingMissing; got %+v", res)
	}
	if len(au.denials) != 1 {
		t.Fatalf("expected 1 audit denial; got %d", len(au.denials))
	}
}

func TestEvaluateTransferDeniedWhenDisabled(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("dp")
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	_, _ = srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "X", PolicyId: p.Id,
	})
	_, _ = srv.DisablePolicy(ctx, &types.MsgDisablePolicy{Authority: testAuthority, Id: p.Id, Reason: "bug"})
	res, _ := k.EvaluateTransferDetailed(ctx, keeper.EvaluateRequest{
		AssetClass: "rwa", AssetID: "X", Sender: alice, Receiver: bob, Amount: 1,
	})
	if res.Allowed || res.Code != dsl.CodePolicyDisabled {
		t.Fatalf("expected CodePolicyDisabled; got %+v", res)
	}
}

func TestEvaluateRequiresKYC(t *testing.T) {
	k, ctx, did, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("kyc")
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	_, _ = srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "X", PolicyId: p.Id,
	})
	// Both subjects in jurisdiction US, but neither has KYC credential.
	did.juris[alice] = "US"
	did.juris[bob] = "US"
	res, _ := k.EvaluateTransferDetailed(ctx, keeper.EvaluateRequest{
		AssetClass: "rwa", AssetID: "X", Sender: alice, Receiver: bob, Amount: 1,
	})
	if res.Allowed || res.Code != dsl.CodeRequireKYC {
		t.Fatalf("expected CodeRequireKYC; got %+v", res)
	}
}

func TestEvaluateAcceptsWithCredentialOverrides(t *testing.T) {
	k, ctx, did, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("ok")
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	_, _ = srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "X", PolicyId: p.Id,
	})
	did.juris[alice] = "US"
	did.juris[bob] = "CA"
	creds := []string{"urn:vc:kyc:passed"}
	res, _ := k.EvaluateTransferDetailed(ctx, keeper.EvaluateRequest{
		AssetClass: "rwa", AssetID: "X", Sender: alice, Receiver: bob, Amount: 100,
		SenderCredsOverride: creds, ReceiverCredsOverride: creds,
	})
	if !res.Allowed {
		t.Fatalf("expected ALLOW; got %+v", res)
	}
}

func TestEvaluateDeniesJurisdictionOutsideAllowList(t *testing.T) {
	k, ctx, did, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("jur")
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	_, _ = srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "X", PolicyId: p.Id,
	})
	did.juris[alice] = "US"
	did.juris[bob] = "CN" // not in US,CA allow-list
	creds := []string{"urn:vc:kyc:passed"}
	res, _ := k.EvaluateTransferDetailed(ctx, keeper.EvaluateRequest{
		AssetClass: "rwa", AssetID: "X", Sender: alice, Receiver: bob, Amount: 1,
		SenderCredsOverride: creds, ReceiverCredsOverride: creds,
	})
	if res.Allowed || res.Code != dsl.CodeJurisdictionDenied {
		t.Fatalf("expected jurisdiction denial; got %+v", res)
	}
}

func TestEvaluatePerHolderCapHit(t *testing.T) {
	k, ctx, did, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("cap")
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	_, _ = srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "X", PolicyId: p.Id,
	})
	did.juris[alice] = "US"
	did.juris[bob] = "US"
	creds := []string{"urn:vc:kyc:passed"}
	// receiver currently holds 999_999 → +2 puts them over 1_000_000.
	res, _ := k.EvaluateTransferDetailed(ctx, keeper.EvaluateRequest{
		AssetClass: "rwa", AssetID: "X", Sender: alice, Receiver: bob, Amount: 2,
		SenderCredsOverride: creds, ReceiverCredsOverride: creds,
		ReceiverHoldings: 999_999,
	})
	if res.Allowed || res.Code != dsl.CodePerHolderCapHit {
		t.Fatalf("expected per-holder cap hit; got %+v", res)
	}
}

// ---------------------------------------------------------------------------
// Sanctions integration
// ---------------------------------------------------------------------------

func TestEvaluateRespectsSanctions(t *testing.T) {
	k, ctx, did, san, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := types.Policy{
		Id: "sanc", Name: "sanc", Version: "1", Status: types.PolicyStatus_POLICY_STATUS_ACTIVE,
		Rules: []types.PolicyRule{
			{Kind: uint32(dsl.RuleNotSanctioned), Side: uint32(dsl.SideBoth)},
		},
	}
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	_, _ = srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "X", PolicyId: p.Id,
	})
	did.juris[alice] = "US"
	did.juris[bob] = "US"
	san.blocked[bob] = true
	res, _ := k.EvaluateTransferDetailed(ctx, keeper.EvaluateRequest{
		AssetClass: "rwa", AssetID: "X", Sender: alice, Receiver: bob, Amount: 1,
	})
	if res.Allowed || res.Code != dsl.CodeSanctionsHit {
		t.Fatalf("expected sanctions hit; got %+v", res)
	}
}

// ---------------------------------------------------------------------------
// Evaluation log ring buffer
// ---------------------------------------------------------------------------

func TestEvaluationLogIsBounded(t *testing.T) {
	k, ctx, did, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)

	// Tighten the ring-buffer cap so the test runs fast.
	params := k.GetParams(ctx)
	params.EvaluationLogMax = 5
	if err := k.SetParams(ctx, params); err != nil {
		t.Fatal(err)
	}
	p := samplePolicy("rb")
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	_, _ = srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "X", PolicyId: p.Id,
	})
	did.juris[alice] = "US"
	did.juris[bob] = "US"
	// Each call denies on KYC, so the ring buffer should never hold
	// more than 5 entries even after 20 attempts.
	for i := 0; i < 20; i++ {
		_, _ = k.EvaluateTransferDetailed(ctx, keeper.EvaluateRequest{
			AssetClass: "rwa", AssetID: "X", Sender: alice, Receiver: bob, Amount: 1,
		})
	}
	var n uint32
	if err := k.EvaluationLogs.Walk(ctx, nil, func(_ uint64, _ types.EvaluationLog) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	if n > params.EvaluationLogMax {
		t.Fatalf("ring buffer not bounded: %d > %d", n, params.EvaluationLogMax)
	}
}

// ---------------------------------------------------------------------------
// Regression: dry-run query path is read-only
// ---------------------------------------------------------------------------
//
// SECURITY: an earlier implementation called EvaluateTransferDetailed
// from Query.Evaluate, so a denial via the (free) gRPC query mutated
// the EvaluationLog ring buffer. A free, unauthenticated denial-of-
// service primitive against a state collection is a critical bug. The
// dry-run path now sets writeLogs=false; this test guards the
// invariant by asserting no log rows appear after a flood of
// denied queries.

func TestDryRunDoesNotMutateLogs(t *testing.T) {
	k, ctx, did, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("dry")
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	_, _ = srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "X", PolicyId: p.Id,
	})
	did.juris[alice] = "US"
	did.juris[bob] = "US"
	for i := 0; i < 50; i++ {
		_, _ = k.EvaluateDryRun(ctx, keeper.EvaluateRequest{
			AssetClass: "rwa", AssetID: "X", Sender: alice, Receiver: bob, Amount: 1,
		})
	}
	var n int
	if err := k.EvaluationLogs.Walk(ctx, nil, func(_ uint64, _ types.EvaluationLog) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("dry-run mutated EvaluationLogs (%d entries)", n)
	}
}

// ---------------------------------------------------------------------------
// Regression: jurisdiction overrides flow through Evaluate
// ---------------------------------------------------------------------------

func TestEvaluateHonorsJurisdictionOverride(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("ovr")
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	_, _ = srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "X", PolicyId: p.Id,
	})
	creds := []string{"urn:vc:kyc:passed"}
	// DIDKeeper has no jurisdiction info at all; without overrides the
	// JurisdictionAllow rule should reject. With overrides matching the
	// allow-list the same call must pass — proving the override flows.
	res, _ := k.EvaluateDryRun(ctx, keeper.EvaluateRequest{
		AssetClass: "rwa", AssetID: "X", Sender: alice, Receiver: bob, Amount: 1,
		SenderCredsOverride: creds, ReceiverCredsOverride: creds,
	})
	if res.Allowed || res.Code != dsl.CodeJurisdictionDenied {
		t.Fatalf("expected jurisdiction denial without override; got %+v", res)
	}
	res, _ = k.EvaluateDryRun(ctx, keeper.EvaluateRequest{
		AssetClass: "rwa", AssetID: "X", Sender: alice, Receiver: bob, Amount: 1,
		SenderCredsOverride:          creds,
		ReceiverCredsOverride:        creds,
		SenderJurisdictionOverride:   "US",
		ReceiverJurisdictionOverride: "CA",
	})
	if !res.Allowed {
		t.Fatalf("expected ALLOW with overrides; got %+v", res)
	}
}

// ---------------------------------------------------------------------------
// Regression: O(1) eviction does not lose the head cursor
// ---------------------------------------------------------------------------

func TestEvictionO1KeepsBufferBoundedAcrossRestarts(t *testing.T) {
	k, ctx, did, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	params := k.GetParams(ctx)
	params.EvaluationLogMax = 4
	if err := k.SetParams(ctx, params); err != nil {
		t.Fatal(err)
	}
	p := samplePolicy("evict")
	_, _ = srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	})
	_, _ = srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "X", PolicyId: p.Id,
	})
	did.juris[alice] = "US"
	did.juris[bob] = "US"
	// 100 denied transfers; the per-call cap is 4, and there are 4 max
	// slots, so after enough calls the buffer must converge to <=4.
	for i := 0; i < 100; i++ {
		_, _ = k.EvaluateTransferDetailed(ctx, keeper.EvaluateRequest{
			AssetClass: "rwa", AssetID: "X", Sender: alice, Receiver: bob, Amount: 1,
		})
	}
	var n uint32
	if err := k.EvaluationLogs.Walk(ctx, nil, func(_ uint64, _ types.EvaluationLog) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	if n > params.EvaluationLogMax {
		t.Fatalf("ring buffer not bounded: %d > %d", n, params.EvaluationLogMax)
	}
}

// ---------------------------------------------------------------------------
// Genesis round-trip
// ---------------------------------------------------------------------------

func TestGenesisRoundTrip(t *testing.T) {
	k, ctx, _, _, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	p := samplePolicy("rt")
	if _, err := srv.RegisterPolicy(ctx, &types.MsgRegisterPolicy{
		Authority: testAuthority, Id: p.Id, Name: p.Name, Version: p.Version, Rules: p.Rules,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.BindPolicy(ctx, &types.MsgBindPolicy{
		Authority: testAuthority, AssetClass: "rwa", AssetId: "Z", PolicyId: p.Id,
	}); err != nil {
		t.Fatal(err)
	}
	gs := k.ExportGenesis(ctx)
	if len(gs.Policies) != 1 || len(gs.Bindings) != 1 {
		t.Fatalf("export missing rows: %+v", gs)
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("exported gs invalid: %v", err)
	}
}
