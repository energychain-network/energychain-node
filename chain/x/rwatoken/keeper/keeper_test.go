package keeper_test

import (
	"context"
	"testing"

	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/testutil"
	"energychain/x/rwatoken/keeper"
	"energychain/x/rwatoken/types"
)

var (
	authority = testutil.DeriveAddr("authority")
	admin     = testutil.DeriveAddr("admin")
	alice     = testutil.DeriveAddr("alice")
	bob       = testutil.DeriveAddr("bob")
	carol     = testutil.DeriveAddr("carol")
	recovery  = testutil.DeriveAddr("recovery")

	settleDenom = "weusd"
)

// ---- mocks -----------------------------------------------------------------

type mockCompliance struct {
	sanctioned map[string]bool
	kyc        map[string]bool
	denied     map[string]bool // "from|to"
	requireKYC bool
	actions    int
}

func newMockCompliance() *mockCompliance {
	return &mockCompliance{sanctioned: map[string]bool{}, kyc: map[string]bool{}, denied: map[string]bool{}}
}
func (m *mockCompliance) IsSanctioned(_ sdk.Context, addr string) bool { return m.sanctioned[addr] }
func (m *mockCompliance) RequireKYC(_ sdk.Context, addr string) error {
	if m.requireKYC && !m.kyc[addr] {
		return types.ErrCompliance.Wrap("kyc required")
	}
	return nil
}
func (m *mockCompliance) EvaluateTransfer(_ sdk.Context, _, _, _, from, to string, _ uint64) error {
	if m.denied[from+"|"+to] {
		return types.ErrCompliance.Wrap("policy denied")
	}
	return nil
}
func (m *mockCompliance) RecordAction(_ sdk.Context, _, _, _, _, _ string) { m.actions++ }

// mockSettlement is an in-memory stand-in for the x/stableusd ledger
// keyed by (denom, account).
type mockSettlement struct {
	denoms map[string]bool
	bal    map[string]uint64
}

func newMockSettlement() *mockSettlement {
	return &mockSettlement{denoms: map[string]bool{}, bal: map[string]uint64{}}
}
func (m *mockSettlement) key(d, a string) string                    { return d + "|" + a }
func (m *mockSettlement) HasDenom(_ context.Context, d string) bool { return m.denoms[d] }
func (m *mockSettlement) GetBalance(_ context.Context, d, a string) uint64 {
	return m.bal[m.key(d, a)]
}
func (m *mockSettlement) MoveBalance(_ context.Context, d, from, to string, amt uint64) error {
	if !m.denoms[d] {
		return types.ErrSettlement.Wrapf("unknown denom %q", d)
	}
	if m.bal[m.key(d, from)] < amt {
		return types.ErrSettlement.Wrapf("insufficient %s for %s", d, from)
	}
	m.bal[m.key(d, from)] -= amt
	m.bal[m.key(d, to)] += amt
	return nil
}
func (m *mockSettlement) fund(d, a string, amt uint64) { m.bal[m.key(d, a)] += amt }

// mockAssetHub is an in-memory stand-in for the x/assethub device
// registry used to validate token device bindings.
type mockAssetHub struct {
	// id -> (operator, active)
	devices map[string]mockDevice
}
type mockDevice struct {
	operator string
	active   bool
}

func newMockAssetHub() *mockAssetHub { return &mockAssetHub{devices: map[string]mockDevice{}} }
func (m *mockAssetHub) add(id, operator string, active bool) {
	m.devices[id] = mockDevice{operator: operator, active: active}
}
func (m *mockAssetHub) DeviceOperator(_ context.Context, id string) (string, bool, bool) {
	d, ok := m.devices[id]
	if !ok {
		return "", false, false
	}
	return d.operator, d.active, true
}

// ---- fixture ---------------------------------------------------------------

type fixture struct {
	ts  *testutil.TestStore
	k   keeper.Keeper
	srv types.MsgServer
	q   types.QueryServer
	cmp *mockCompliance
	set *mockSettlement
	ah  *mockAssetHub
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ts := testutil.NewTestStore(t, types.StoreKey)
	cmp := newMockCompliance()
	set := newMockSettlement()
	set.denoms[settleDenom] = true
	ah := newMockAssetHub()
	k := keeper.NewKeeper(ts.Cdc, runtime.NewKVStoreService(ts.Key(t, types.StoreKey)), authority, cmp, set, ah)
	if err := k.SetParams(ts.Ctx, types.DefaultParams()); err != nil {
		t.Fatalf("set params: %v", err)
	}
	// The default token admin must be a whitelisted issuer for CreateToken.
	if err := k.Issuers.Set(ts.Ctx, admin); err != nil {
		t.Fatalf("whitelist admin: %v", err)
	}
	return &fixture{ts: ts, k: k, srv: keeper.NewMsgServerImpl(k), q: keeper.NewQueryServerImpl(k), cmp: cmp, set: set, ah: ah}
}

func (f *fixture) ctx() context.Context { return f.ts.Ctx }

func (f *fixture) createToken(t *testing.T, m *types.MsgCreateToken) uint64 {
	t.Helper()
	if m.Admin == "" {
		m.Admin = admin
	}
	if m.Symbol == "" {
		m.Symbol = "LXP"
	}
	if m.AssetClass == "" {
		m.AssetClass = "charging_pile_revenue"
	}
	if m.SettlementDenom == "" {
		m.SettlementDenom = settleDenom
	}
	resp, err := f.srv.CreateToken(f.ctx(), m)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return resp.TokenId
}

func (f *fixture) mint(t *testing.T, id uint64, to string, amt uint64) {
	t.Helper()
	if _, err := f.srv.Mint(f.ctx(), &types.MsgMint{Admin: admin, TokenId: id, Recipient: to, Amount: amt}); err != nil {
		t.Fatalf("mint: %v", err)
	}
}

func (f *fixture) bal(t *testing.T, id uint64, holder string) uint64 {
	t.Helper()
	v, err := f.k.GetBalance(f.ctx(), id, holder)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	return v
}

func (f *fixture) token(t *testing.T, id uint64) types.Token {
	t.Helper()
	tok, ok, err := f.k.GetToken(f.ctx(), id)
	if err != nil || !ok {
		t.Fatalf("get token %d: ok=%v err=%v", id, ok, err)
	}
	return tok
}

// ---- token lifecycle -------------------------------------------------------

func TestCreateTokenAndUniqueness(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP"})
	if id != 1 {
		t.Fatalf("want id 1 got %d", id)
	}
	if tok := f.token(t, id); tok.Status != types.TokenStatus_TOKEN_STATUS_ACTIVE || tok.Admin != admin {
		t.Fatalf("unexpected token %+v", tok)
	}
	// duplicate symbol rejected
	if _, err := f.srv.CreateToken(f.ctx(), &types.MsgCreateToken{
		Admin: admin, Symbol: "LXP", AssetClass: "x", SettlementDenom: settleDenom,
	}); err == nil {
		t.Fatal("expected duplicate symbol error")
	}
}

func TestSnapshotHolderCap(t *testing.T) {
	f := setup(t)
	p := types.DefaultParams()
	p.MaxHoldersPerSnapshot = 2
	if err := f.k.SetParams(f.ctx(), p); err != nil {
		t.Fatalf("set params: %v", err)
	}
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "CAP"})
	f.mint(t, id, alice, 10)
	f.mint(t, id, bob, 10)
	if _, err := f.srv.TakeSnapshot(f.ctx(), &types.MsgTakeSnapshot{Admin: admin, TokenId: id}); err != nil {
		t.Fatalf("snapshot at cap: %v", err)
	}
	f.mint(t, id, carol, 10)
	if _, err := f.srv.TakeSnapshot(f.ctx(), &types.MsgTakeSnapshot{Admin: admin, TokenId: id}); err == nil {
		t.Fatal("expected snapshot above holder cap to be rejected")
	}
}

func TestCreateTokenSanctionedAdmin(t *testing.T) {
	f := setup(t)
	f.cmp.sanctioned[admin] = true
	if _, err := f.srv.CreateToken(f.ctx(), &types.MsgCreateToken{
		Admin: admin, Symbol: "LXP", AssetClass: "x", SettlementDenom: settleDenom,
	}); err == nil {
		t.Fatal("expected sanctioned admin rejected")
	}
}

func TestMaxTokens(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx())
	p.MaxTokens = 1
	if err := f.k.SetParams(f.ctx(), p); err != nil {
		t.Fatal(err)
	}
	f.createToken(t, &types.MsgCreateToken{Symbol: "A"})
	if _, err := f.srv.CreateToken(f.ctx(), &types.MsgCreateToken{
		Admin: admin, Symbol: "B", AssetClass: "x", SettlementDenom: settleDenom,
	}); err == nil {
		t.Fatal("expected max_tokens limit")
	}
}

// ---- mint / supply ---------------------------------------------------------

func TestMintSupplyAndPerHolderCap(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP", PerHolderCap: 100})
	f.mint(t, id, alice, 60)
	if f.bal(t, id, alice) != 60 || f.token(t, id).TotalSupply != 60 {
		t.Fatal("mint accounting wrong")
	}
	// exceeding per-holder cap rejected
	if _, err := f.srv.Mint(f.ctx(), &types.MsgMint{Admin: admin, TokenId: id, Recipient: alice, Amount: 41}); err == nil {
		t.Fatal("expected per-holder cap error")
	}
	f.mint(t, id, alice, 40) // exactly cap
	if f.bal(t, id, alice) != 100 {
		t.Fatal("cap-edge mint failed")
	}
}

func TestMintComplianceGates(t *testing.T) {
	f := setup(t)
	f.cmp.requireKYC = true
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP", RequireKyc: true})
	// recipient lacks KYC
	if _, err := f.srv.Mint(f.ctx(), &types.MsgMint{Admin: admin, TokenId: id, Recipient: alice, Amount: 10}); err == nil {
		t.Fatal("expected KYC gate")
	}
	f.cmp.kyc[alice] = true
	f.mint(t, id, alice, 10)
	// sanctioned recipient
	f.cmp.sanctioned[bob] = true
	f.cmp.kyc[bob] = true
	if _, err := f.srv.Mint(f.ctx(), &types.MsgMint{Admin: admin, TokenId: id, Recipient: bob, Amount: 10}); err == nil {
		t.Fatal("expected sanctioned recipient gate")
	}
	// sanctioned admin (issuer)
	f.cmp.sanctioned[admin] = true
	if _, err := f.srv.Mint(f.ctx(), &types.MsgMint{Admin: admin, TokenId: id, Recipient: alice, Amount: 10}); err == nil {
		t.Fatal("expected sanctioned admin gate")
	}
}

func TestBurn(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP"})
	f.mint(t, id, alice, 50)
	if _, err := f.srv.Burn(f.ctx(), &types.MsgBurn{Admin: admin, TokenId: id, Holder: alice, Amount: 20}); err != nil {
		t.Fatalf("burn: %v", err)
	}
	if f.bal(t, id, alice) != 30 || f.token(t, id).TotalSupply != 30 {
		t.Fatal("burn accounting wrong")
	}
	// burn more than balance fails
	if _, err := f.srv.Burn(f.ctx(), &types.MsgBurn{Admin: admin, TokenId: id, Holder: alice, Amount: 31}); err == nil {
		t.Fatal("expected underflow")
	}
}

// ---- transfer / freeze -----------------------------------------------------

func TestTransferAndCompliance(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP", PolicyId: "p1"})
	f.mint(t, id, alice, 100)

	// happy path
	if _, err := f.srv.Transfer(f.ctx(), &types.MsgTransfer{From: alice, TokenId: id, To: bob, Amount: 40}); err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if f.bal(t, id, alice) != 60 || f.bal(t, id, bob) != 40 {
		t.Fatal("transfer accounting")
	}
	// insufficient balance
	if _, err := f.srv.Transfer(f.ctx(), &types.MsgTransfer{From: alice, TokenId: id, To: bob, Amount: 61}); err == nil {
		t.Fatal("expected insufficient funds")
	}
	// policy denial
	f.cmp.denied[alice+"|"+carol] = true
	if _, err := f.srv.Transfer(f.ctx(), &types.MsgTransfer{From: alice, TokenId: id, To: carol, Amount: 1}); err == nil {
		t.Fatal("expected policy denial")
	}
	// sanctioned receiver
	f.cmp.sanctioned[bob] = true
	if _, err := f.srv.Transfer(f.ctx(), &types.MsgTransfer{From: alice, TokenId: id, To: bob, Amount: 1}); err == nil {
		t.Fatal("expected sanctioned receiver block")
	}
}

func TestFreezeBlocksTransfer(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP"})
	f.mint(t, id, alice, 100)
	if _, err := f.srv.Freeze(f.ctx(), &types.MsgFreeze{Admin: admin, TokenId: id, Holder: alice}); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if _, err := f.srv.Transfer(f.ctx(), &types.MsgTransfer{From: alice, TokenId: id, To: bob, Amount: 1}); err == nil {
		t.Fatal("expected frozen sender block")
	}
	// receiver frozen also blocks
	if _, err := f.srv.Unfreeze(f.ctx(), &types.MsgUnfreeze{Admin: admin, TokenId: id, Holder: alice}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Freeze(f.ctx(), &types.MsgFreeze{Admin: admin, TokenId: id, Holder: bob}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Transfer(f.ctx(), &types.MsgTransfer{From: alice, TokenId: id, To: bob, Amount: 1}); err == nil {
		t.Fatal("expected frozen receiver block")
	}
}

func TestPausedAndMatured(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP"})
	f.mint(t, id, alice, 100)
	if _, err := f.srv.SetTokenStatus(f.ctx(), &types.MsgSetTokenStatus{Admin: admin, TokenId: id, Status: types.TokenStatus_TOKEN_STATUS_PAUSED}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Transfer(f.ctx(), &types.MsgTransfer{From: alice, TokenId: id, To: bob, Amount: 1}); err == nil {
		t.Fatal("expected paused transfer block")
	}
}

func TestForceTransfer(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP"})
	f.mint(t, id, alice, 100)
	// freeze alice; force-transfer out of her still works
	if _, err := f.srv.Freeze(f.ctx(), &types.MsgFreeze{Admin: admin, TokenId: id, Holder: alice}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.ForceTransfer(f.ctx(), &types.MsgForceTransfer{
		Admin: admin, TokenId: id, From: alice, To: recovery, Amount: 30, Reason: "court order",
	}); err != nil {
		t.Fatalf("force transfer: %v", err)
	}
	if f.bal(t, id, alice) != 70 || f.bal(t, id, recovery) != 30 {
		t.Fatal("force transfer accounting")
	}
	// destination sanctioned is blocked
	f.cmp.sanctioned[carol] = true
	if _, err := f.srv.ForceTransfer(f.ctx(), &types.MsgForceTransfer{
		Admin: admin, TokenId: id, From: alice, To: carol, Amount: 1,
	}); err == nil {
		t.Fatal("expected sanctioned destination block")
	}
}

// ---- snapshot / dividend ---------------------------------------------------

func TestSnapshotAndDividend(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP"})
	f.mint(t, id, alice, 70)
	f.mint(t, id, bob, 30)

	snapResp, err := f.srv.TakeSnapshot(f.ctx(), &types.MsgTakeSnapshot{Admin: admin, TokenId: id})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	snap, _, _ := f.k.GetSnapshot(f.ctx(), snapResp.SnapshotId)
	if snap.TotalSupply != 100 || snap.HolderCount != 2 {
		t.Fatalf("snapshot meta wrong: %+v", snap)
	}

	// fund admin so the dividend can be escrowed
	f.set.fund(settleDenom, admin, 1000)
	distResp, err := f.srv.CreateDistribution(f.ctx(), &types.MsgCreateDistribution{
		Admin: admin, TokenId: id, SnapshotId: snapResp.SnapshotId, TotalAmount: 1000,
	})
	if err != nil {
		t.Fatalf("create distribution: %v", err)
	}
	// dividend pool funded, admin debited
	if f.set.GetBalance(f.ctx(), settleDenom, keeper.DividendPoolAccount(id)) != 1000 {
		t.Fatal("dividend pool not funded")
	}

	// alice claims 700, bob claims 300
	r1, err := f.srv.ClaimDistribution(f.ctx(), &types.MsgClaimDistribution{Holder: alice, DistributionId: distResp.DistributionId})
	if err != nil || r1.Amount != 700 {
		t.Fatalf("alice claim: %d %v", r1.Amount, err)
	}
	r2, err := f.srv.ClaimDistribution(f.ctx(), &types.MsgClaimDistribution{Holder: bob, DistributionId: distResp.DistributionId})
	if err != nil || r2.Amount != 300 {
		t.Fatalf("bob claim: %d %v", r2.Amount, err)
	}
	if f.set.GetBalance(f.ctx(), settleDenom, alice) != 700 || f.set.GetBalance(f.ctx(), settleDenom, bob) != 300 {
		t.Fatal("dividend payouts wrong")
	}
	// double claim rejected
	if _, err := f.srv.ClaimDistribution(f.ctx(), &types.MsgClaimDistribution{Holder: alice, DistributionId: distResp.DistributionId}); err == nil {
		t.Fatal("expected double-claim rejection")
	}
}

func TestDividendDustStaysInPool(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP"})
	f.mint(t, id, alice, 1)
	f.mint(t, id, bob, 1)
	f.mint(t, id, carol, 1)
	snap, _ := f.srv.TakeSnapshot(f.ctx(), &types.MsgTakeSnapshot{Admin: admin, TokenId: id})
	f.set.fund(settleDenom, admin, 100)
	// 100 / 3 holders -> each floor(100*1/3)=33, 1 dust remains
	dist, _ := f.srv.CreateDistribution(f.ctx(), &types.MsgCreateDistribution{
		Admin: admin, TokenId: id, SnapshotId: snap.SnapshotId, TotalAmount: 100,
	})
	total := uint64(0)
	for _, h := range []string{alice, bob, carol} {
		r, err := f.srv.ClaimDistribution(f.ctx(), &types.MsgClaimDistribution{Holder: h, DistributionId: dist.DistributionId})
		if err != nil {
			t.Fatalf("claim %s: %v", h, err)
		}
		total += r.Amount
	}
	if total != 99 {
		t.Fatalf("claimed %d want 99 (1 dust)", total)
	}
	if f.set.GetBalance(f.ctx(), settleDenom, keeper.DividendPoolAccount(id)) != 1 {
		t.Fatal("expected 1 dust left in pool")
	}
}

// snapshot must exclude escrowed (pending-redemption) units.
func TestSnapshotExcludesEscrow(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP", RedemptionPrice: 1})
	f.mint(t, id, alice, 100)
	if _, err := f.srv.RequestRedemption(f.ctx(), &types.MsgRequestRedemption{Holder: alice, TokenId: id, Units: 40}); err != nil {
		t.Fatalf("request: %v", err)
	}
	snap, _ := f.srv.TakeSnapshot(f.ctx(), &types.MsgTakeSnapshot{Admin: admin, TokenId: id})
	s, _, _ := f.k.GetSnapshot(f.ctx(), snap.SnapshotId)
	if s.TotalSupply != 60 || s.HolderCount != 1 {
		t.Fatalf("snapshot should exclude escrow: %+v", s)
	}
}

// ---- redemption ------------------------------------------------------------

func TestRedemptionFullCycle(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP", RedemptionPrice: 2, RedemptionDelaySeconds: 100})
	f.mint(t, id, alice, 100)

	resp, err := f.srv.RequestRedemption(f.ctx(), &types.MsgRequestRedemption{Holder: alice, TokenId: id, Units: 10})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.Payout != 20 {
		t.Fatalf("payout %d want 20", resp.Payout)
	}
	// units escrowed; supply unchanged
	if f.bal(t, id, alice) != 90 || f.bal(t, id, types.RedemptionEscrow) != 10 || f.token(t, id).TotalSupply != 100 {
		t.Fatal("escrow accounting wrong")
	}
	// too early
	if _, err := f.srv.ExecuteRedemption(f.ctx(), &types.MsgExecuteRedemption{Executor: bob, RedemptionId: resp.RedemptionId}); err == nil {
		t.Fatal("expected window-not-reached")
	}
	// fund pool and advance time
	f.set.fund(settleDenom, admin, 1000)
	if _, err := f.srv.FundPool(f.ctx(), &types.MsgFundPool{Admin: admin, TokenId: id, Amount: 100}); err != nil {
		t.Fatalf("fund pool: %v", err)
	}
	f.ts.Advance(101)
	if _, err := f.srv.ExecuteRedemption(f.ctx(), &types.MsgExecuteRedemption{Executor: bob, RedemptionId: resp.RedemptionId}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	// holder paid, escrow burned, supply reduced
	if f.set.GetBalance(f.ctx(), settleDenom, alice) != 20 {
		t.Fatal("holder not paid")
	}
	if f.bal(t, id, types.RedemptionEscrow) != 0 || f.token(t, id).TotalSupply != 90 {
		t.Fatal("escrow burn / supply wrong")
	}
}

func TestExecuteRedemptionUnderfundedFailsClosed(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP", RedemptionPrice: 5})
	f.mint(t, id, alice, 100)
	resp, _ := f.srv.RequestRedemption(f.ctx(), &types.MsgRequestRedemption{Holder: alice, TokenId: id, Units: 10})
	// no pool funding; payout 50 cannot be covered
	if _, err := f.srv.ExecuteRedemption(f.ctx(), &types.MsgExecuteRedemption{Executor: admin, RedemptionId: resp.RedemptionId}); err == nil {
		t.Fatal("expected underfunded failure")
	}
	// state untouched: still escrowed, still pending
	if f.bal(t, id, types.RedemptionEscrow) != 10 {
		t.Fatal("escrow should be intact after failed execute")
	}
	r, _, _ := f.k.GetRedemption(f.ctx(), resp.RedemptionId)
	if r.Status != types.RedemptionStatus_REDEMPTION_STATUS_PENDING {
		t.Fatal("redemption should still be pending")
	}
}

func TestCancelRedemptionReturnsUnits(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP", RedemptionPrice: 2})
	f.mint(t, id, alice, 100)
	resp, _ := f.srv.RequestRedemption(f.ctx(), &types.MsgRequestRedemption{Holder: alice, TokenId: id, Units: 30})
	if _, err := f.srv.CancelRedemption(f.ctx(), &types.MsgCancelRedemption{Admin: admin, RedemptionId: resp.RedemptionId, Reason: "withdrawn"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if f.bal(t, id, alice) != 100 || f.bal(t, id, types.RedemptionEscrow) != 0 || f.token(t, id).TotalSupply != 100 {
		t.Fatal("cancel should restore units supply-neutrally")
	}
}

func TestRedemptionDisabledAndPaused(t *testing.T) {
	f := setup(t)
	// price 0 disables redemption
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP", RedemptionPrice: 0})
	f.mint(t, id, alice, 10)
	if _, err := f.srv.RequestRedemption(f.ctx(), &types.MsgRequestRedemption{Holder: alice, TokenId: id, Units: 1}); err == nil {
		t.Fatal("expected redemption-disabled")
	}
	// paused blocks redemption
	id2 := f.createToken(t, &types.MsgCreateToken{Symbol: "LX2", RedemptionPrice: 1})
	f.mint(t, id2, alice, 10)
	if _, err := f.srv.SetTokenStatus(f.ctx(), &types.MsgSetTokenStatus{Admin: admin, TokenId: id2, Status: types.TokenStatus_TOKEN_STATUS_PAUSED}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.RequestRedemption(f.ctx(), &types.MsgRequestRedemption{Holder: alice, TokenId: id2, Units: 1}); err == nil {
		t.Fatal("expected paused redemption block")
	}
}

func TestMaturedAllowsRedemptionBlocksTransfer(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP", RedemptionPrice: 1})
	f.mint(t, id, alice, 50)
	if _, err := f.srv.SetTokenStatus(f.ctx(), &types.MsgSetTokenStatus{Admin: admin, TokenId: id, Status: types.TokenStatus_TOKEN_STATUS_MATURED}); err != nil {
		t.Fatal(err)
	}
	// transfer blocked
	if _, err := f.srv.Transfer(f.ctx(), &types.MsgTransfer{From: alice, TokenId: id, To: bob, Amount: 1}); err == nil {
		t.Fatal("expected matured transfer block")
	}
	// redemption allowed
	if _, err := f.srv.RequestRedemption(f.ctx(), &types.MsgRequestRedemption{Holder: alice, TokenId: id, Units: 5}); err != nil {
		t.Fatalf("matured redemption should work: %v", err)
	}
}

func TestUnauthorizedAdminOps(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP"})
	if _, err := f.srv.Mint(f.ctx(), &types.MsgMint{Admin: bob, TokenId: id, Recipient: alice, Amount: 1}); err == nil {
		t.Fatal("expected non-admin mint rejected")
	}
	if _, err := f.srv.SetTokenStatus(f.ctx(), &types.MsgSetTokenStatus{Admin: bob, TokenId: id, Status: types.TokenStatus_TOKEN_STATUS_PAUSED}); err == nil {
		t.Fatal("expected non-admin status rejected")
	}
}

// ---- genesis ---------------------------------------------------------------

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP", RedemptionPrice: 2, RedemptionDelaySeconds: 50})
	f.mint(t, id, alice, 70)
	f.mint(t, id, bob, 30)
	snap, _ := f.srv.TakeSnapshot(f.ctx(), &types.MsgTakeSnapshot{Admin: admin, TokenId: id})
	f.set.fund(settleDenom, admin, 1000)
	dist, _ := f.srv.CreateDistribution(f.ctx(), &types.MsgCreateDistribution{Admin: admin, TokenId: id, SnapshotId: snap.SnapshotId, TotalAmount: 500})
	if _, err := f.srv.ClaimDistribution(f.ctx(), &types.MsgClaimDistribution{Holder: alice, DistributionId: dist.DistributionId}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.RequestRedemption(f.ctx(), &types.MsgRequestRedemption{Holder: bob, TokenId: id, Units: 10}); err != nil {
		t.Fatal(err)
	}

	gs, err := f.k.ExportGenesis(f.ctx())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}

	// re-import into a fresh keeper
	g := setup(t)
	if err := g.k.InitGenesis(g.ctx(), gs); err != nil {
		t.Fatalf("init: %v", err)
	}
	gs2, err := g.k.ExportGenesis(g.ctx())
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if len(gs2.Tokens) != 1 || len(gs2.Balances) != len(gs.Balances) || len(gs2.Redemptions) != 1 {
		t.Fatalf("roundtrip mismatch: %+v", gs2)
	}
	if gs2.TokenIdSeq != gs.TokenIdSeq || gs2.RedemptionIdSeq != gs.RedemptionIdSeq {
		t.Fatal("sequence mismatch on roundtrip")
	}
}

// A crafted genesis with a pending redemption but no matching escrow
// balance must be rejected (phantom redeemable units).
func TestGenesisRejectsPhantomEscrow(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Tokens = []types.Token{{
		Id: 1, Symbol: "LXP", Admin: admin, AssetClass: "charging_pile_revenue",
		SettlementDenom: settleDenom, Status: types.TokenStatus_TOKEN_STATUS_ACTIVE,
		TotalSupply: 100,
	}}
	gs.Balances = []types.Balance{{TokenId: 1, Holder: alice, Amount: 100}}
	gs.Redemptions = []types.Redemption{{
		Id: 1, TokenId: 1, Holder: alice, Units: 10, Denom: settleDenom,
		Status: types.RedemptionStatus_REDEMPTION_STATUS_PENDING,
	}}
	gs.TokenIdSeq = 1
	gs.RedemptionIdSeq = 1
	if err := gs.Validate(); err == nil {
		t.Fatal("expected phantom-escrow genesis rejection")
	}
}

func TestGenesisEscrowInvariantHolds(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Tokens = []types.Token{{
		Id: 1, Symbol: "LXP", Admin: admin, AssetClass: "charging_pile_revenue",
		SettlementDenom: settleDenom, Status: types.TokenStatus_TOKEN_STATUS_ACTIVE,
		TotalSupply: 100,
	}}
	gs.Balances = []types.Balance{
		{TokenId: 1, Holder: alice, Amount: 90},
		{TokenId: 1, Holder: types.RedemptionEscrow, Amount: 10},
	}
	gs.Redemptions = []types.Redemption{{
		Id: 1, TokenId: 1, Holder: alice, Units: 10, Denom: settleDenom,
		Status: types.RedemptionStatus_REDEMPTION_STATUS_PENDING,
	}}
	gs.TokenIdSeq = 1
	gs.RedemptionIdSeq = 1
	if err := gs.Validate(); err != nil {
		t.Fatalf("valid escrow genesis rejected: %v", err)
	}
}

// A distribution must price against a snapshot of its OWN token; a genesis
// cross-referencing another token's snapshot must be rejected.
func TestGenesisRejectsCrossTokenDistribution(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Tokens = []types.Token{
		{Id: 1, Symbol: "AAA", Admin: admin, AssetClass: "x", SettlementDenom: settleDenom, Status: types.TokenStatus_TOKEN_STATUS_ACTIVE, TotalSupply: 100},
		{Id: 2, Symbol: "BBB", Admin: admin, AssetClass: "x", SettlementDenom: settleDenom, Status: types.TokenStatus_TOKEN_STATUS_ACTIVE, TotalSupply: 100},
	}
	gs.Balances = []types.Balance{
		{TokenId: 1, Holder: alice, Amount: 100},
		{TokenId: 2, Holder: bob, Amount: 100},
	}
	gs.Snapshots = []types.Snapshot{{Id: 1, TokenId: 2, TotalSupply: 100, HolderCount: 1}}
	gs.SnapshotBalances = []types.SnapshotBalanceRow{{SnapshotId: 1, Holder: bob, Amount: 100}}
	// distribution for token 1 references token 2's snapshot
	gs.Distributions = []types.Distribution{{Id: 1, TokenId: 1, SnapshotId: 1, Denom: settleDenom, TotalAmount: 100}}
	gs.TokenIdSeq = 2
	gs.SnapshotIdSeq = 1
	gs.DistributionIdSeq = 1
	if err := gs.Validate(); err == nil {
		t.Fatal("expected cross-token distribution/snapshot rejection")
	}
}

// ---- compliance on settlement legs (regression for review findings) -------

// Dividend claims must enforce the full holder compliance gate (freeze,
// KYC, policy), not just sanctions, so a holder who could not receive a
// transfer cannot extract a dividend either.
func TestClaimDistributionComplianceGate(t *testing.T) {
	setupDist := func(t *testing.T, kyc bool) (*fixture, uint64) {
		f := setup(t)
		f.cmp.requireKYC = kyc
		id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP", RequireKyc: kyc})
		if kyc {
			f.cmp.kyc[alice] = true
			f.cmp.kyc[bob] = true
		}
		f.mint(t, id, alice, 70)
		f.mint(t, id, bob, 30)
		snap, _ := f.srv.TakeSnapshot(f.ctx(), &types.MsgTakeSnapshot{Admin: admin, TokenId: id})
		f.set.fund(settleDenom, admin, 1000)
		if _, err := f.srv.CreateDistribution(f.ctx(), &types.MsgCreateDistribution{Admin: admin, TokenId: id, SnapshotId: snap.SnapshotId, TotalAmount: 1000}); err != nil {
			t.Fatalf("create distribution: %v", err)
		}
		return f, id
	}

	t.Run("frozen", func(t *testing.T) {
		f, id := setupDist(t, false)
		if _, err := f.srv.Freeze(f.ctx(), &types.MsgFreeze{Admin: admin, TokenId: id, Holder: alice}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.srv.ClaimDistribution(f.ctx(), &types.MsgClaimDistribution{Holder: alice, DistributionId: 1}); err == nil {
			t.Fatal("frozen holder must not claim dividends")
		}
	})
	t.Run("no_kyc", func(t *testing.T) {
		f, _ := setupDist(t, true)
		f.cmp.kyc[alice] = false // revoke after snapshot
		if _, err := f.srv.ClaimDistribution(f.ctx(), &types.MsgClaimDistribution{Holder: alice, DistributionId: 1}); err == nil {
			t.Fatal("non-KYC holder must not claim dividends")
		}
	})
	t.Run("policy_denied", func(t *testing.T) {
		f := setup(t)
		id := f.createToken(t, &types.MsgCreateToken{Symbol: "POL", PolicyId: "p1"})
		f.mint(t, id, alice, 100)
		snap, _ := f.srv.TakeSnapshot(f.ctx(), &types.MsgTakeSnapshot{Admin: admin, TokenId: id})
		f.set.fund(settleDenom, admin, 1000)
		if _, err := f.srv.CreateDistribution(f.ctx(), &types.MsgCreateDistribution{Admin: admin, TokenId: id, SnapshotId: snap.SnapshotId, TotalAmount: 1000}); err != nil {
			t.Fatal(err)
		}
		// policy denies a receipt to alice ("" -> alice)
		f.cmp.denied["|"+alice] = true
		if _, err := f.srv.ClaimDistribution(f.ctx(), &types.MsgClaimDistribution{Holder: alice, DistributionId: 1}); err == nil {
			t.Fatal("policy-denied holder must not claim dividends")
		}
	})
}

// A holder frozen after requesting redemption must not be paid on execute;
// the admin can still cancel to return the escrowed units.
func TestExecuteRedemptionFrozenBlocked(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP", RedemptionPrice: 2})
	f.mint(t, id, alice, 100)
	resp, err := f.srv.RequestRedemption(f.ctx(), &types.MsgRequestRedemption{Holder: alice, TokenId: id, Units: 10})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	f.set.fund(settleDenom, admin, 1000)
	if _, err := f.srv.FundPool(f.ctx(), &types.MsgFundPool{Admin: admin, TokenId: id, Amount: 100}); err != nil {
		t.Fatalf("fund: %v", err)
	}
	// freeze holder, then attempt execute -> blocked
	if _, err := f.srv.Freeze(f.ctx(), &types.MsgFreeze{Admin: admin, TokenId: id, Holder: alice}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.ExecuteRedemption(f.ctx(), &types.MsgExecuteRedemption{Executor: admin, RedemptionId: resp.RedemptionId}); err == nil {
		t.Fatal("frozen holder must not be paid on execute")
	}
	// escrow + supply untouched
	if f.bal(t, id, types.RedemptionEscrow) != 10 || f.token(t, id).TotalSupply != 100 {
		t.Fatal("failed execute must not mutate escrow/supply")
	}
}

// A frozen holder must not be able to escrow units for redemption.
func TestRequestRedemptionFrozenBlocked(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP", RedemptionPrice: 1})
	f.mint(t, id, alice, 100)
	if _, err := f.srv.Freeze(f.ctx(), &types.MsgFreeze{Admin: admin, TokenId: id, Holder: alice}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.RequestRedemption(f.ctx(), &types.MsgRequestRedemption{Holder: alice, TokenId: id, Units: 10}); err == nil {
		t.Fatal("frozen holder must not request redemption")
	}
}

// A sanctioned admin must not be able to fund the dividend or redemption
// pools (regression for the sanctioned-issuer pool-funding bypass).
func TestPoolFundingSanctionedAdmin(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "LXP", RedemptionPrice: 1})
	f.mint(t, id, alice, 100)
	snap, _ := f.srv.TakeSnapshot(f.ctx(), &types.MsgTakeSnapshot{Admin: admin, TokenId: id})
	f.set.fund(settleDenom, admin, 1000)
	f.cmp.sanctioned[admin] = true

	if _, err := f.srv.CreateDistribution(f.ctx(), &types.MsgCreateDistribution{Admin: admin, TokenId: id, SnapshotId: snap.SnapshotId, TotalAmount: 100}); err == nil {
		t.Fatal("sanctioned admin must not fund dividend pool")
	}
	if _, err := f.srv.FundPool(f.ctx(), &types.MsgFundPool{Admin: admin, TokenId: id, Amount: 100}); err == nil {
		t.Fatal("sanctioned admin must not fund redemption pool")
	}
}

// ---- issuer whitelist --------------------------------------------------------

// CreateToken must be rejected for accounts outside the governance-managed
// issuer whitelist; whitelisting (and the authority itself) unlocks it.
func TestCreateTokenIssuerWhitelist(t *testing.T) {
	f := setup(t)

	// carol is not whitelisted.
	if _, err := f.srv.CreateToken(f.ctx(), &types.MsgCreateToken{
		Admin: carol, Symbol: "NOPE", AssetClass: "x", SettlementDenom: settleDenom,
	}); err == nil {
		t.Fatal("non-whitelisted issuer must not create tokens")
	}

	// the authority is implicitly an issuer.
	if _, err := f.srv.CreateToken(f.ctx(), &types.MsgCreateToken{
		Admin: authority, Symbol: "GOV", AssetClass: "x", SettlementDenom: settleDenom,
	}); err != nil {
		t.Fatalf("authority create: %v", err)
	}

	// whitelisting carol via governance unlocks CreateToken.
	if _, err := f.srv.AddIssuer(f.ctx(), &types.MsgAddIssuer{Authority: authority, Issuer: carol}); err != nil {
		t.Fatalf("add issuer: %v", err)
	}
	if _, err := f.srv.CreateToken(f.ctx(), &types.MsgCreateToken{
		Admin: carol, Symbol: "OKAY", AssetClass: "x", SettlementDenom: settleDenom,
	}); err != nil {
		t.Fatalf("whitelisted create: %v", err)
	}

	// removal closes the gate again.
	if _, err := f.srv.RemoveIssuer(f.ctx(), &types.MsgRemoveIssuer{Authority: authority, Issuer: carol}); err != nil {
		t.Fatalf("remove issuer: %v", err)
	}
	if _, err := f.srv.CreateToken(f.ctx(), &types.MsgCreateToken{
		Admin: carol, Symbol: "AGAIN", AssetClass: "x", SettlementDenom: settleDenom,
	}); err == nil {
		t.Fatal("removed issuer must not create tokens")
	}
}

// Only the authority may mutate the whitelist; duplicates and missing
// entries are rejected.
func TestIssuerWhitelistAuthorityGate(t *testing.T) {
	f := setup(t)

	if _, err := f.srv.AddIssuer(f.ctx(), &types.MsgAddIssuer{Authority: alice, Issuer: carol}); err == nil {
		t.Fatal("non-authority must not add issuers")
	}
	if _, err := f.srv.RemoveIssuer(f.ctx(), &types.MsgRemoveIssuer{Authority: alice, Issuer: admin}); err == nil {
		t.Fatal("non-authority must not remove issuers")
	}

	if _, err := f.srv.AddIssuer(f.ctx(), &types.MsgAddIssuer{Authority: authority, Issuer: carol}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.AddIssuer(f.ctx(), &types.MsgAddIssuer{Authority: authority, Issuer: carol}); err == nil {
		t.Fatal("duplicate add must fail")
	}
	if _, err := f.srv.RemoveIssuer(f.ctx(), &types.MsgRemoveIssuer{Authority: authority, Issuer: bob}); err == nil {
		t.Fatal("removing a non-issuer must fail")
	}

	// queries reflect the state (admin whitelisted in setup, carol added).
	resp, err := f.q.Issuers(f.ctx(), &types.QueryIssuersRequest{})
	if err != nil || len(resp.Issuers) != 2 {
		t.Fatalf("issuers query: %v %+v", err, resp)
	}
	if is, err := f.q.IsIssuer(f.ctx(), &types.QueryIsIssuerRequest{Address: carol}); err != nil || !is.IsIssuer {
		t.Fatalf("carol should be an issuer: %v %+v", err, is)
	}
	if is, _ := f.q.IsIssuer(f.ctx(), &types.QueryIsIssuerRequest{Address: bob}); is.IsIssuer {
		t.Fatal("bob should not be an issuer")
	}
	if is, _ := f.q.IsIssuer(f.ctx(), &types.QueryIsIssuerRequest{Address: authority}); !is.IsIssuer {
		t.Fatal("authority is implicitly an issuer")
	}
}
