package keeper_test

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/runtime"

	"energychain/testutil"
	"energychain/x/identity/keeper"
	"energychain/x/identity/types"
)

var (
	authority = testutil.DeriveAddr("authority")
	registrar = testutil.DeriveAddr("registrar")
	stranger  = testutil.DeriveAddr("stranger")
	alice     = testutil.DeriveAddr("alice")
	bob       = testutil.DeriveAddr("bob")
	carol     = testutil.DeriveAddr("carol")
	policyOwn = testutil.DeriveAddr("policy-owner")
)

type fixture struct {
	ts  *testutil.TestStore
	k   keeper.Keeper
	srv types.MsgServer
	q   types.QueryServer
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ts := testutil.NewTestStore(t, types.StoreKey)
	k := keeper.NewKeeper(ts.Cdc, runtime.NewKVStoreService(ts.Key(t, types.StoreKey)), authority)
	if err := k.SetParams(ts.Ctx, types.DefaultParams()); err != nil {
		t.Fatalf("set params: %v", err)
	}
	return &fixture{ts: ts, k: k, srv: keeper.NewMsgServerImpl(k), q: keeper.NewQueryServerImpl(k)}
}

func (f *fixture) addRegistrar(t *testing.T) {
	t.Helper()
	if _, err := f.srv.AddRegistrar(f.ts.Ctx, &types.MsgAddRegistrar{
		Authority: authority, Registrar: registrar, DisplayName: "KYC Co",
	}); err != nil {
		t.Fatalf("add registrar: %v", err)
	}
}

func (f *fixture) setKYC(t *testing.T, addr, jurisdiction string, accredited bool) {
	t.Helper()
	if _, err := f.srv.SetAccount(f.ts.Ctx, &types.MsgSetAccount{
		Registrar: registrar, Address: addr, Did: "did:web:ex:" + addr,
		KycCleared: true, Accredited: accredited, Jurisdiction: jurisdiction,
	}); err != nil {
		t.Fatalf("set account %s: %v", addr, err)
	}
}

// ---- Registrar ------------------------------------------------------------

func TestAddRegistrar(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	ok, err := f.k.IsRegistrar(f.ts.Ctx, registrar)
	if err != nil || !ok {
		t.Fatalf("expected registrar present, ok=%v err=%v", ok, err)
	}
}

func TestAddRegistrarUnauthorized(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.AddRegistrar(f.ts.Ctx, &types.MsgAddRegistrar{
		Authority: stranger, Registrar: registrar,
	}); err == nil {
		t.Fatal("expected unauthorized error")
	}
}

func TestAddRegistrarDuplicate(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	if _, err := f.srv.AddRegistrar(f.ts.Ctx, &types.MsgAddRegistrar{
		Authority: authority, Registrar: registrar,
	}); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestRemoveRegistrar(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	if _, err := f.srv.RemoveRegistrar(f.ts.Ctx, &types.MsgRemoveRegistrar{
		Authority: authority, Registrar: registrar,
	}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if ok, _ := f.k.IsRegistrar(f.ts.Ctx, registrar); ok {
		t.Fatal("registrar should be gone")
	}
}

func TestRemoveRegistrarNotFound(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.RemoveRegistrar(f.ts.Ctx, &types.MsgRemoveRegistrar{
		Authority: authority, Registrar: registrar,
	}); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestRegistrarLimit(t *testing.T) {
	f := setup(t)
	p := types.DefaultParams()
	p.MaxRegistrars = 1
	if err := f.k.SetParams(f.ts.Ctx, p); err != nil {
		t.Fatal(err)
	}
	f.addRegistrar(t)
	if _, err := f.srv.AddRegistrar(f.ts.Ctx, &types.MsgAddRegistrar{
		Authority: authority, Registrar: alice,
	}); err == nil {
		t.Fatal("expected limit error")
	}
}

// ---- Account --------------------------------------------------------------

func TestSetAccountRequiresRegistrar(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.SetAccount(f.ts.Ctx, &types.MsgSetAccount{
		Registrar: stranger, Address: alice, KycCleared: true,
	}); err == nil {
		t.Fatal("expected unauthorized for non-registrar")
	}
}

func TestSetAccountAuthorityIsImplicitRegistrar(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.SetAccount(f.ts.Ctx, &types.MsgSetAccount{
		Registrar: authority, Address: alice, KycCleared: true,
	}); err != nil {
		t.Fatalf("authority should act as registrar: %v", err)
	}
}

func TestSetAccountUpsert(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	f.setKYC(t, alice, "US", false)
	acc, found, err := f.k.GetAccountRaw(f.ts.Ctx, alice)
	if err != nil || !found {
		t.Fatalf("account missing: %v", err)
	}
	if !acc.KycCleared || acc.Jurisdiction != "US" {
		t.Fatalf("unexpected account: %+v", acc)
	}
	// update flips accredited
	f.setKYC(t, alice, "US", true)
	acc, _, _ = f.k.GetAccountRaw(f.ts.Ctx, alice)
	if !acc.Accredited {
		t.Fatal("expected accredited after update")
	}
	if acc.CreatedAt == 0 || acc.UpdatedAt < acc.CreatedAt {
		t.Fatalf("timestamps wrong: %+v", acc)
	}
}

func TestFreezeUnfreeze(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	f.setKYC(t, alice, "US", false)
	if _, err := f.srv.FreezeAccount(f.ts.Ctx, &types.MsgFreezeAccount{
		Registrar: registrar, Address: alice, Reason: "review",
	}); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	acc, _, _ := f.k.GetAccountRaw(f.ts.Ctx, alice)
	if acc.Status != types.AccountStatus_ACCOUNT_STATUS_FROZEN {
		t.Fatal("expected frozen")
	}
	if _, err := f.srv.UnfreezeAccount(f.ts.Ctx, &types.MsgUnfreezeAccount{
		Registrar: registrar, Address: alice,
	}); err != nil {
		t.Fatalf("unfreeze: %v", err)
	}
	acc, _, _ = f.k.GetAccountRaw(f.ts.Ctx, alice)
	if acc.Status != types.AccountStatus_ACCOUNT_STATUS_ACTIVE {
		t.Fatal("expected active")
	}
}

func TestFreezeUnknownAccount(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	if _, err := f.srv.FreezeAccount(f.ts.Ctx, &types.MsgFreezeAccount{
		Registrar: registrar, Address: alice,
	}); err == nil {
		t.Fatal("expected not-found")
	}
}

// ---- Sanctions ------------------------------------------------------------

func TestSanctionsLifecycle(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.AddSanction(f.ts.Ctx, &types.MsgAddSanction{
		Authority: authority, Address: bob, ListSource: "OFAC", Reason: "x",
	}); err != nil {
		t.Fatalf("add sanction: %v", err)
	}
	if !f.k.IsSanctioned(f.ts.Ctx, bob) {
		t.Fatal("bob should be sanctioned")
	}
	if f.k.IsSanctioned(f.ts.Ctx, alice) {
		t.Fatal("alice should not be sanctioned")
	}
	if _, err := f.srv.RemoveSanction(f.ts.Ctx, &types.MsgRemoveSanction{
		Authority: authority, Address: bob,
	}); err != nil {
		t.Fatalf("remove sanction: %v", err)
	}
	if f.k.IsSanctioned(f.ts.Ctx, bob) {
		t.Fatal("bob should be clear")
	}
}

func TestSanctionUnauthorized(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.AddSanction(f.ts.Ctx, &types.MsgAddSanction{
		Authority: stranger, Address: bob,
	}); err == nil {
		t.Fatal("expected unauthorized")
	}
}

func TestSanctionDuplicate(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.AddSanction(f.ts.Ctx, &types.MsgAddSanction{Authority: authority, Address: bob}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.AddSanction(f.ts.Ctx, &types.MsgAddSanction{Authority: authority, Address: bob}); err == nil {
		t.Fatal("expected duplicate error")
	}
}

// ---- Policy ---------------------------------------------------------------

func (f *fixture) createKYCPolicy(t *testing.T, id string) {
	t.Helper()
	if _, err := f.srv.CreatePolicy(f.ts.Ctx, &types.MsgCreatePolicy{
		Owner: policyOwn, Id: id, Description: "kyc only",
		RequireKyc: true, DenyFrozen: true,
	}); err != nil {
		t.Fatalf("create policy: %v", err)
	}
}

func TestPolicyCRUD(t *testing.T) {
	f := setup(t)
	f.createKYCPolicy(t, "p1")
	pol, found, err := f.k.GetPolicy(f.ts.Ctx, "p1")
	if err != nil || !found || !pol.RequireKyc {
		t.Fatalf("policy missing: %+v %v", pol, err)
	}
	// duplicate
	if _, err := f.srv.CreatePolicy(f.ts.Ctx, &types.MsgCreatePolicy{Owner: policyOwn, Id: "p1"}); err == nil {
		t.Fatal("expected duplicate")
	}
	// update by non-owner non-authority denied
	if _, err := f.srv.UpdatePolicy(f.ts.Ctx, &types.MsgUpdatePolicy{Owner: stranger, Id: "p1"}); err == nil {
		t.Fatal("expected unauthorized update")
	}
	// authority override allowed
	if _, err := f.srv.UpdatePolicy(f.ts.Ctx, &types.MsgUpdatePolicy{Owner: authority, Id: "p1", RequireAccredited: true}); err != nil {
		t.Fatalf("authority update: %v", err)
	}
	pol, _, _ = f.k.GetPolicy(f.ts.Ctx, "p1")
	if !pol.RequireAccredited {
		t.Fatal("update did not apply")
	}
	// pause
	if _, err := f.srv.PausePolicy(f.ts.Ctx, &types.MsgPausePolicy{Owner: policyOwn, Id: "p1", Paused: true}); err != nil {
		t.Fatalf("pause: %v", err)
	}
	pol, _, _ = f.k.GetPolicy(f.ts.Ctx, "p1")
	if !pol.Paused {
		t.Fatal("expected paused")
	}
	// delete
	if _, err := f.srv.DeletePolicy(f.ts.Ctx, &types.MsgDeletePolicy{Owner: policyOwn, Id: "p1"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, found, _ := f.k.GetPolicy(f.ts.Ctx, "p1"); found {
		t.Fatal("policy should be deleted")
	}
}

func TestPolicyJurisdictionCap(t *testing.T) {
	f := setup(t)
	p := types.DefaultParams()
	p.MaxJurisdictionsPerPolicy = 1
	if err := f.k.SetParams(f.ts.Ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.CreatePolicy(f.ts.Ctx, &types.MsgCreatePolicy{
		Owner: policyOwn, Id: "p2", AllowedJurisdictions: []string{"US", "SG"},
	}); err == nil {
		t.Fatal("expected jurisdiction cap error")
	}
}

// ---- EvaluateTransfer -----------------------------------------------------

func TestEvaluateTransferSanctions(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.AddSanction(f.ts.Ctx, &types.MsgAddSanction{Authority: authority, Address: bob}); err != nil {
		t.Fatal(err)
	}
	// no policy, but sanctions gate still fires on receiver
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "", alice, bob, 10); err == nil {
		t.Fatal("expected sanctions denial")
	}
	// and on sender
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "", bob, alice, 10); err == nil {
		t.Fatal("expected sanctions denial for sender")
	}
}

func TestEvaluateTransferKYC(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	f.createKYCPolicy(t, "p1")
	// neither has identity -> denied
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "p1", alice, bob, 10); err == nil {
		t.Fatal("expected denial: no identity")
	}
	f.setKYC(t, alice, "US", false)
	f.setKYC(t, bob, "US", false)
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "p1", alice, bob, 10); err != nil {
		t.Fatalf("expected allow: %v", err)
	}
}

func TestEvaluateTransferAccredited(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	if _, err := f.srv.CreatePolicy(f.ts.Ctx, &types.MsgCreatePolicy{
		Owner: policyOwn, Id: "acc", RequireKyc: true, RequireAccredited: true,
	}); err != nil {
		t.Fatal(err)
	}
	f.setKYC(t, alice, "US", true)  // accredited sender
	f.setKYC(t, bob, "US", false)   // non-accredited receiver
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "acc", alice, bob, 10); err == nil {
		t.Fatal("expected receiver-accreditation denial")
	}
	// sender need not be accredited; receiver must
	f.setKYC(t, bob, "US", true)
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "acc", alice, bob, 10); err != nil {
		t.Fatalf("expected allow once receiver accredited: %v", err)
	}
}

func TestEvaluateTransferFrozen(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	f.createKYCPolicy(t, "p1")
	f.setKYC(t, alice, "US", false)
	f.setKYC(t, bob, "US", false)
	if _, err := f.srv.FreezeAccount(f.ts.Ctx, &types.MsgFreezeAccount{Registrar: registrar, Address: alice}); err != nil {
		t.Fatal(err)
	}
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "p1", alice, bob, 10); err == nil {
		t.Fatal("expected frozen-sender denial")
	}
}

func TestEvaluateTransferJurisdiction(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	if _, err := f.srv.CreatePolicy(f.ts.Ctx, &types.MsgCreatePolicy{
		Owner: policyOwn, Id: "jur", RequireKyc: true,
		AllowedJurisdictions: []string{"US", "SG"},
		DeniedJurisdictions:  []string{"KP"},
	}); err != nil {
		t.Fatal(err)
	}
	f.setKYC(t, alice, "US", false)
	f.setKYC(t, bob, "CN", false) // not in allow-list
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "jur", alice, bob, 10); err == nil {
		t.Fatal("expected jurisdiction denial (not allowed)")
	}
	f.setKYC(t, bob, "KP", false) // explicitly denied
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "jur", alice, bob, 10); err == nil {
		t.Fatal("expected jurisdiction denial (denied list)")
	}
	f.setKYC(t, bob, "SG", false) // allowed
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "jur", alice, bob, 10); err != nil {
		t.Fatalf("expected allow: %v", err)
	}
}

func TestEvaluateTransferPaused(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	f.createKYCPolicy(t, "p1")
	f.setKYC(t, alice, "US", false)
	f.setKYC(t, bob, "US", false)
	if _, err := f.srv.PausePolicy(f.ts.Ctx, &types.MsgPausePolicy{Owner: policyOwn, Id: "p1", Paused: true}); err != nil {
		t.Fatal(err)
	}
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "p1", alice, bob, 10); err == nil {
		t.Fatal("expected paused-policy denial")
	}
}

// Regression (Bugbot): a deny-list-only policy must not be evadable by
// using an unregistered wallet or one with an empty jurisdiction.
func TestEvaluateTransferDenyListNoBypass(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	if _, err := f.srv.CreatePolicy(f.ts.Ctx, &types.MsgCreatePolicy{
		Owner: policyOwn, Id: "deny", DeniedJurisdictions: []string{"KP", "IR"},
	}); err != nil {
		t.Fatal(err)
	}
	// unknown receiver -> cannot prove not-denied -> blocked
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "deny", alice, bob, 10); err == nil {
		t.Fatal("expected denial: unknown account cannot evade deny list")
	}
	// known but empty jurisdiction -> blocked
	if _, err := f.srv.SetAccount(f.ts.Ctx, &types.MsgSetAccount{Registrar: registrar, Address: bob, KycCleared: true}); err != nil {
		t.Fatal(err)
	}
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "deny", alice, bob, 10); err == nil {
		t.Fatal("expected denial: empty jurisdiction cannot evade deny list")
	}
	// known clean jurisdiction -> allowed (sender alice also needs a record now)
	f.setKYC(t, alice, "US", false)
	f.setKYC(t, bob, "US", false)
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "deny", alice, bob, 10); err != nil {
		t.Fatalf("expected allow for clean jurisdiction: %v", err)
	}
	// explicitly denied jurisdiction -> blocked
	f.setKYC(t, bob, "KP", false)
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "deny", alice, bob, 10); err == nil {
		t.Fatal("expected denial for KP")
	}
}

// Regression (Bugbot): a deny_frozen-only policy must NOT block an
// unregistered wallet (an unknown account is not frozen).
func TestEvaluateTransferDenyFrozenAllowsUnknown(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	if _, err := f.srv.CreatePolicy(f.ts.Ctx, &types.MsgCreatePolicy{
		Owner: policyOwn, Id: "fz", DenyFrozen: true,
	}); err != nil {
		t.Fatal(err)
	}
	// both parties unregistered, no positive requirement -> allowed
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "fz", alice, bob, 10); err != nil {
		t.Fatalf("deny_frozen-only must allow unknown wallets: %v", err)
	}
	// once frozen, it blocks
	f.setKYC(t, alice, "US", false)
	if _, err := f.srv.FreezeAccount(f.ts.Ctx, &types.MsgFreezeAccount{Registrar: registrar, Address: alice}); err != nil {
		t.Fatal(err)
	}
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "fz", alice, bob, 10); err == nil {
		t.Fatal("expected denial for frozen sender")
	}
}

func TestEvaluateTransferUnknownPolicy(t *testing.T) {
	f := setup(t)
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "nope", alice, bob, 10); err == nil {
		t.Fatal("expected unknown-policy error")
	}
}

func TestEvaluateTransferNoPolicyAllows(t *testing.T) {
	f := setup(t)
	if err := f.k.EvaluateTransfer(f.ts.Ctx, "rwa", "1", "", alice, bob, 10); err != nil {
		t.Fatalf("no policy should allow: %v", err)
	}
}

// ---- RequireKYC -----------------------------------------------------------

func TestRequireKYC(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	if err := f.k.RequireKYC(f.ts.Ctx, alice); err == nil {
		t.Fatal("expected error: no record")
	}
	f.setKYC(t, alice, "US", false)
	if err := f.k.RequireKYC(f.ts.Ctx, alice); err != nil {
		t.Fatalf("expected ok: %v", err)
	}
	// expiry in the past
	if _, err := f.srv.SetAccount(f.ts.Ctx, &types.MsgSetAccount{
		Registrar: registrar, Address: alice, KycCleared: true, KycExpiresAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.k.RequireKYC(f.ts.Ctx, alice); err == nil {
		t.Fatal("expected expired error")
	}
	// empty addr is a no-op
	if err := f.k.RequireKYC(f.ts.Ctx, ""); err != nil {
		t.Fatalf("empty addr should be allowed: %v", err)
	}
}

// ---- Audit ----------------------------------------------------------------

func TestAuditMonotonic(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	f.setKYC(t, alice, "US", false)
	f.setKYC(t, bob, "US", false)
	resp, err := f.q.AuditEntries(f.ts.Ctx, &types.QueryAuditEntriesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Entries) < 3 {
		t.Fatalf("expected >=3 audit entries, got %d", len(resp.Entries))
	}
	var prev uint64
	for _, e := range resp.Entries {
		if e.Seq <= prev {
			t.Fatalf("audit seq not monotonic: %d after %d", e.Seq, prev)
		}
		prev = e.Seq
	}
	// module filter
	byMod, err := f.q.AuditEntries(f.ts.Ctx, &types.QueryAuditEntriesRequest{Module: types.ModuleName})
	if err != nil {
		t.Fatal(err)
	}
	if len(byMod.Entries) != len(resp.Entries) {
		t.Fatalf("module filter mismatch: %d vs %d", len(byMod.Entries), len(resp.Entries))
	}
}

// ---- Params ---------------------------------------------------------------

func TestUpdateParamsUnauthorized(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.UpdateParams(f.ts.Ctx, &types.MsgUpdateParams{
		Authority: stranger, Params: types.DefaultParams(),
	}); err == nil {
		t.Fatal("expected unauthorized")
	}
}

func TestUpdateParamsInvalid(t *testing.T) {
	f := setup(t)
	bad := types.DefaultParams()
	bad.MaxPolicies = 0
	if _, err := f.srv.UpdateParams(f.ts.Ctx, &types.MsgUpdateParams{
		Authority: authority, Params: bad,
	}); err == nil {
		t.Fatal("expected invalid-params error")
	}
}

// ---- Genesis --------------------------------------------------------------

func TestGenesisRoundtrip(t *testing.T) {
	f := setup(t)
	f.addRegistrar(t)
	f.setKYC(t, alice, "US", true)
	f.createKYCPolicy(t, "p1")
	if _, err := f.srv.AddSanction(f.ts.Ctx, &types.MsgAddSanction{Authority: authority, Address: bob, ListSource: "OFAC"}); err != nil {
		t.Fatal(err)
	}

	exported, err := f.k.ExportGenesis(f.ts.Ctx)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if err := exported.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}

	// re-import into a fresh keeper
	g := setup(t)
	if err := g.k.InitGenesis(g.ts.Ctx, exported); err != nil {
		t.Fatalf("init: %v", err)
	}
	reexported, err := g.k.ExportGenesis(g.ts.Ctx)
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if len(reexported.Accounts) != len(exported.Accounts) ||
		len(reexported.Policies) != len(exported.Policies) ||
		len(reexported.Sanctions) != len(exported.Sanctions) ||
		len(reexported.Registrars) != len(exported.Registrars) {
		t.Fatalf("genesis roundtrip lost rows: %+v vs %+v", reexported, exported)
	}
	if !g.k.IsSanctioned(g.ts.Ctx, bob) {
		t.Fatal("sanction lost on roundtrip")
	}
	if reexported.AuditSeq != exported.AuditSeq {
		t.Fatalf("audit seq drift: %d vs %d", reexported.AuditSeq, exported.AuditSeq)
	}
}

func TestGenesisValidateRejectsDuplicateAccount(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Accounts = []types.Account{
		{Address: alice, Status: types.AccountStatus_ACCOUNT_STATUS_ACTIVE},
		{Address: alice, Status: types.AccountStatus_ACCOUNT_STATUS_ACTIVE},
	}
	if err := gs.Validate(); err == nil {
		t.Fatal("expected duplicate account rejection")
	}
}
