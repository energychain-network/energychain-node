package keeper_test

import (
	"context"
	"testing"

	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/testutil"
	"energychain/x/stableusd/keeper"
	"energychain/x/stableusd/types"
)

var (
	authority = testutil.DeriveAddr("authority")
	admin     = testutil.DeriveAddr("admin")
	minter    = testutil.DeriveAddr("minter")
	alice     = testutil.DeriveAddr("alice")
	bob       = testutil.DeriveAddr("bob")
	carol     = testutil.DeriveAddr("carol")
	recovery  = testutil.DeriveAddr("recovery")
	stranger  = testutil.DeriveAddr("stranger")

	denomID = "weusd"
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

type mockReserve struct {
	values map[string]struct {
		v  uint64
		ts int64
	}
}

func newMockReserve() *mockReserve {
	return &mockReserve{values: map[string]struct {
		v  uint64
		ts int64
	}{}}
}
func (m *mockReserve) set(topic string, v uint64, ts int64) {
	m.values[topic] = struct {
		v  uint64
		ts int64
	}{v, ts}
}
func (m *mockReserve) GetTopicValue(_ context.Context, topic string) (uint64, int64, bool) {
	e, ok := m.values[topic]
	return e.v, e.ts, ok
}

// ---- fixture ---------------------------------------------------------------

type fixture struct {
	ts  *testutil.TestStore
	k   keeper.Keeper
	srv types.MsgServer
	q   types.QueryServer
	cmp *mockCompliance
	res *mockReserve
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ts := testutil.NewTestStore(t, types.StoreKey)
	cmp := newMockCompliance()
	res := newMockReserve()
	k := keeper.NewKeeper(ts.Cdc, runtime.NewKVStoreService(ts.Key(t, types.StoreKey)), authority, cmp, res)
	if err := k.SetParams(ts.Ctx, types.DefaultParams()); err != nil {
		t.Fatalf("set params: %v", err)
	}
	return &fixture{ts: ts, k: k, srv: keeper.NewMsgServerImpl(k), q: keeper.NewQueryServerImpl(k), cmp: cmp, res: res}
}

func (f *fixture) createDenom(t *testing.T) {
	t.Helper()
	if _, err := f.srv.CreateDenom(f.ts.Ctx, &types.MsgCreateDenom{
		Authority: authority, Id: denomID, Symbol: "WeUSD", Decimals: 6, PegCurrency: "USD", Admin: admin,
	}); err != nil {
		t.Fatalf("create denom: %v", err)
	}
}

func (f *fixture) addMinter(t *testing.T) {
	t.Helper()
	if _, err := f.srv.AddMinter(f.ts.Ctx, &types.MsgAddMinter{Admin: admin, DenomId: denomID, Minter: minter}); err != nil {
		t.Fatalf("add minter: %v", err)
	}
}

func (f *fixture) mint(t *testing.T, to string, amt uint64) {
	t.Helper()
	if _, err := f.srv.Mint(f.ts.Ctx, &types.MsgMint{Minter: minter, DenomId: denomID, Recipient: to, Amount: amt}); err != nil {
		t.Fatalf("mint %d to %s: %v", amt, to, err)
	}
}

func (f *fixture) setParams(t *testing.T, p types.Params) {
	t.Helper()
	if _, err := f.srv.UpdateParams(f.ts.Ctx, &types.MsgUpdateParams{Authority: authority, Params: p}); err != nil {
		t.Fatalf("update params: %v", err)
	}
}

// ---- denom lifecycle -------------------------------------------------------

func TestCreateDenomAuthorityAndDuplicate(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.CreateDenom(f.ts.Ctx, &types.MsgCreateDenom{
		Authority: stranger, Id: denomID, Symbol: "X", Decimals: 6, Admin: admin,
	}); err == nil {
		t.Fatal("expected authority gate")
	}
	f.createDenom(t)
	if _, err := f.srv.CreateDenom(f.ts.Ctx, &types.MsgCreateDenom{
		Authority: authority, Id: denomID, Symbol: "X", Decimals: 6, Admin: admin,
	}); err == nil {
		t.Fatal("expected duplicate rejection")
	}
}

func TestMinterManagementAdminGate(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	// non-admin cannot add minter
	if _, err := f.srv.AddMinter(f.ts.Ctx, &types.MsgAddMinter{Admin: stranger, DenomId: denomID, Minter: minter}); err == nil {
		t.Fatal("expected admin gate")
	}
	f.addMinter(t)
	// duplicate
	if _, err := f.srv.AddMinter(f.ts.Ctx, &types.MsgAddMinter{Admin: admin, DenomId: denomID, Minter: minter}); err == nil {
		t.Fatal("expected duplicate minter rejection")
	}
	// remove
	if _, err := f.srv.RemoveMinter(f.ts.Ctx, &types.MsgRemoveMinter{Admin: admin, DenomId: denomID, Minter: minter}); err != nil {
		t.Fatalf("remove minter: %v", err)
	}
	d, _ := f.k.GetDenom(f.ts.Ctx, denomID)
	if len(d.Minters) != 0 {
		t.Fatalf("minter not removed: %+v", d.Minters)
	}
}

// ---- mint / burn -----------------------------------------------------------

func TestMintAuthorisationAndLedger(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	// unauthorised minter
	if _, err := f.srv.Mint(f.ts.Ctx, &types.MsgMint{Minter: stranger, DenomId: denomID, Recipient: alice, Amount: 100}); err == nil {
		t.Fatal("expected unauthorised minter rejection")
	}
	f.addMinter(t)
	f.mint(t, alice, 1000)
	if got := f.k.GetBalance(f.ts.Ctx, denomID, alice); got != 1000 {
		t.Fatalf("balance = %d", got)
	}
	if got := f.k.GetSupply(f.ts.Ctx, denomID); got != 1000 {
		t.Fatalf("supply = %d", got)
	}
}

func TestBurnInsufficient(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.addMinter(t)
	f.mint(t, alice, 100)
	if _, err := f.srv.Burn(f.ts.Ctx, &types.MsgBurn{Holder: alice, DenomId: denomID, Amount: 101}); err == nil {
		t.Fatal("expected insufficient balance")
	}
	if _, err := f.srv.Burn(f.ts.Ctx, &types.MsgBurn{Holder: alice, DenomId: denomID, Amount: 60}); err != nil {
		t.Fatalf("burn: %v", err)
	}
	if got := f.k.GetSupply(f.ts.Ctx, denomID); got != 40 {
		t.Fatalf("supply after burn = %d", got)
	}
}

// ---- reserve gating --------------------------------------------------------

func TestMintReserveCoverage(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.addMinter(t)
	// require reserve, bind topic with 100% ratio
	p := types.DefaultParams()
	p.MintRequiresReserve = true
	f.setParams(t, p)
	if _, err := f.srv.BindReserve(f.ts.Ctx, &types.MsgBindReserve{
		Admin: admin, DenomId: denomID, ReserveTopic: "reserve.weusd", RequiredRatioBps: 10000, MaxStalenessSeconds: 3600,
	}); err != nil {
		t.Fatalf("bind reserve: %v", err)
	}
	// no attestation yet -> fail closed
	if _, err := f.srv.Mint(f.ts.Ctx, &types.MsgMint{Minter: minter, DenomId: denomID, Recipient: alice, Amount: 100}); err == nil {
		t.Fatal("expected reserve gate to fail without attestation")
	}
	// attest 1000 reserve at current block time
	now := f.ts.Ctx.BlockTime().Unix()
	f.res.set("reserve.weusd", 1000, now)
	// minting 1000 is exactly covered
	f.mint(t, alice, 1000)
	// minting 1 more exceeds reserve coverage
	if _, err := f.srv.Mint(f.ts.Ctx, &types.MsgMint{Minter: minter, DenomId: denomID, Recipient: alice, Amount: 1}); err == nil {
		t.Fatal("expected reserve coverage breach")
	}
	// stale attestation rejected
	f.res.set("reserve.weusd", 1_000_000, now-99999)
	if _, err := f.srv.Mint(f.ts.Ctx, &types.MsgMint{Minter: minter, DenomId: denomID, Recipient: alice, Amount: 1}); err == nil {
		t.Fatal("expected stale reserve rejection")
	}
}

// ---- transfers + compliance ------------------------------------------------

func TestTransferComplianceGates(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.addMinter(t)
	f.mint(t, alice, 1000)

	// frozen sender
	if _, err := f.srv.Freeze(f.ts.Ctx, &types.MsgFreeze{Admin: admin, DenomId: denomID, Account: alice}); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if _, err := f.srv.Transfer(f.ts.Ctx, &types.MsgTransfer{From: alice, DenomId: denomID, To: bob, Amount: 1}); err == nil {
		t.Fatal("frozen sender must not transfer")
	}
	_, _ = f.srv.Unfreeze(f.ts.Ctx, &types.MsgUnfreeze{Admin: admin, DenomId: denomID, Account: alice})

	// blacklisted receiver
	_, _ = f.srv.Blacklist(f.ts.Ctx, &types.MsgBlacklist{Admin: admin, DenomId: denomID, Account: bob})
	if _, err := f.srv.Transfer(f.ts.Ctx, &types.MsgTransfer{From: alice, DenomId: denomID, To: bob, Amount: 1}); err == nil {
		t.Fatal("blacklisted receiver must not receive")
	}
	_, _ = f.srv.Unblacklist(f.ts.Ctx, &types.MsgUnblacklist{Admin: admin, DenomId: denomID, Account: bob})

	// sanctioned party
	f.cmp.sanctioned[bob] = true
	if _, err := f.srv.Transfer(f.ts.Ctx, &types.MsgTransfer{From: alice, DenomId: denomID, To: bob, Amount: 1}); err == nil {
		t.Fatal("sanctioned receiver must not receive")
	}
	delete(f.cmp.sanctioned, bob)

	// policy denial
	d, _ := f.k.GetDenom(f.ts.Ctx, denomID)
	d.PolicyId = "kyc-only"
	_ = f.k.Denoms.Set(f.ts.Ctx, denomID, d)
	f.cmp.denied[alice+"|"+bob] = true
	if _, err := f.srv.Transfer(f.ts.Ctx, &types.MsgTransfer{From: alice, DenomId: denomID, To: bob, Amount: 1}); err == nil {
		t.Fatal("policy must deny transfer")
	}
	delete(f.cmp.denied, alice+"|"+bob)

	// success path
	if _, err := f.srv.Transfer(f.ts.Ctx, &types.MsgTransfer{From: alice, DenomId: denomID, To: bob, Amount: 250}); err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if f.k.GetBalance(f.ts.Ctx, denomID, alice) != 750 || f.k.GetBalance(f.ts.Ctx, denomID, bob) != 250 {
		t.Fatal("balances not updated")
	}
}

func TestTransferInsufficient(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.addMinter(t)
	f.mint(t, alice, 10)
	if _, err := f.srv.Transfer(f.ts.Ctx, &types.MsgTransfer{From: alice, DenomId: denomID, To: bob, Amount: 11}); err == nil {
		t.Fatal("expected insufficient funds")
	}
}

func TestApproveAndTransferFrom(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.addMinter(t)
	f.mint(t, alice, 1000)

	if _, err := f.srv.Approve(f.ts.Ctx, &types.MsgApprove{Owner: alice, DenomId: denomID, Spender: bob, Amount: 300}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// over-allowance
	if _, err := f.srv.TransferFrom(f.ts.Ctx, &types.MsgTransferFrom{Spender: bob, DenomId: denomID, From: alice, To: carol, Amount: 301}); err == nil {
		t.Fatal("expected allowance breach")
	}
	if _, err := f.srv.TransferFrom(f.ts.Ctx, &types.MsgTransferFrom{Spender: bob, DenomId: denomID, From: alice, To: carol, Amount: 200}); err != nil {
		t.Fatalf("transferFrom: %v", err)
	}
	if got := f.k.GetAllowance(f.ts.Ctx, denomID, alice, bob); got != 100 {
		t.Fatalf("allowance = %d, want 100", got)
	}
	if f.k.GetBalance(f.ts.Ctx, denomID, carol) != 200 {
		t.Fatal("carol balance wrong")
	}
}

// ---- force transfer --------------------------------------------------------

func TestForceTransferSeizesFrozenFunds(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.addMinter(t)
	f.mint(t, alice, 500)
	_, _ = f.srv.Freeze(f.ts.Ctx, &types.MsgFreeze{Admin: admin, DenomId: denomID, Account: alice})

	// admin seizes frozen funds to recovery
	if _, err := f.srv.ForceTransfer(f.ts.Ctx, &types.MsgForceTransfer{
		Admin: admin, DenomId: denomID, From: alice, To: recovery, Amount: 500, Reason: "court order",
	}); err != nil {
		t.Fatalf("force transfer: %v", err)
	}
	if f.k.GetBalance(f.ts.Ctx, denomID, recovery) != 500 || f.k.GetBalance(f.ts.Ctx, denomID, alice) != 0 {
		t.Fatal("seizure balances wrong")
	}
	// cannot force-transfer into a blocked destination
	_, _ = f.srv.Blacklist(f.ts.Ctx, &types.MsgBlacklist{Admin: admin, DenomId: denomID, Account: bob})
	if _, err := f.srv.ForceTransfer(f.ts.Ctx, &types.MsgForceTransfer{
		Admin: admin, DenomId: denomID, From: recovery, To: bob, Amount: 1,
	}); err == nil {
		t.Fatal("expected blocked destination rejection")
	}
}

// ---- redemption ------------------------------------------------------------

func TestRedemptionLifecycle(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.addMinter(t)
	f.mint(t, alice, 1000)

	res, err := f.srv.RequestRedemption(f.ts.Ctx, &types.MsgRequestRedemption{Holder: alice, DenomId: denomID, Amount: 400, Memo: "bank ref"})
	if err != nil {
		t.Fatalf("request redemption: %v", err)
	}
	// tokens escrowed (holder debited) but supply unchanged until settle
	if f.k.GetBalance(f.ts.Ctx, denomID, alice) != 600 {
		t.Fatalf("holder not debited: %d", f.k.GetBalance(f.ts.Ctx, denomID, alice))
	}
	if f.k.GetSupply(f.ts.Ctx, denomID) != 1000 {
		t.Fatalf("supply should be unchanged on request: %d", f.k.GetSupply(f.ts.Ctx, denomID))
	}
	if f.k.GetBalance(f.ts.Ctx, denomID, types.RedemptionEscrow) != 400 {
		t.Fatalf("escrow not credited: %d", f.k.GetBalance(f.ts.Ctx, denomID, types.RedemptionEscrow))
	}
	// settle burns the escrowed tokens
	if _, err := f.srv.SettleRedemption(f.ts.Ctx, &types.MsgSettleRedemption{Admin: admin, RedemptionId: res.RedemptionId, Memo: "wire-123"}); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if f.k.GetSupply(f.ts.Ctx, denomID) != 600 || f.k.GetBalance(f.ts.Ctx, denomID, types.RedemptionEscrow) != 0 {
		t.Fatalf("settle did not burn escrow: supply=%d escrow=%d",
			f.k.GetSupply(f.ts.Ctx, denomID), f.k.GetBalance(f.ts.Ctx, denomID, types.RedemptionEscrow))
	}
	// double settle rejected
	if _, err := f.srv.SettleRedemption(f.ts.Ctx, &types.MsgSettleRedemption{Admin: admin, RedemptionId: res.RedemptionId}); err == nil {
		t.Fatal("expected double-settle rejection")
	}
}

func TestRedemptionCancelRefunds(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.addMinter(t)
	f.mint(t, alice, 1000)
	res, _ := f.srv.RequestRedemption(f.ts.Ctx, &types.MsgRequestRedemption{Holder: alice, DenomId: denomID, Amount: 400})
	if _, err := f.srv.CancelRedemption(f.ts.Ctx, &types.MsgCancelRedemption{Admin: admin, RedemptionId: res.RedemptionId, Reason: "kyc fail"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if f.k.GetBalance(f.ts.Ctx, denomID, alice) != 1000 || f.k.GetSupply(f.ts.Ctx, denomID) != 1000 {
		t.Fatal("cancel did not refund")
	}
}

func TestRedemptionPendingLimit(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.addMinter(t)
	f.mint(t, alice, 1000)
	p := types.DefaultParams()
	p.MaxPendingRedemptionsPerHolder = 2
	f.setParams(t, p)
	for i := 0; i < 2; i++ {
		if _, err := f.srv.RequestRedemption(f.ts.Ctx, &types.MsgRequestRedemption{Holder: alice, DenomId: denomID, Amount: 1}); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if _, err := f.srv.RequestRedemption(f.ts.Ctx, &types.MsgRequestRedemption{Holder: alice, DenomId: denomID, Amount: 1}); err == nil {
		t.Fatal("expected pending redemption limit")
	}
}

// ---- cross-module ----------------------------------------------------------

func TestMoveBalanceRespectsBlocks(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.addMinter(t)
	f.mint(t, alice, 100)
	_, _ = f.srv.Blacklist(f.ts.Ctx, &types.MsgBlacklist{Admin: admin, DenomId: denomID, Account: bob})
	if err := f.k.MoveBalance(f.ts.Ctx, denomID, alice, bob, 10); err == nil {
		t.Fatal("MoveBalance must refuse blocked destination")
	}
	if err := f.k.MoveBalance(f.ts.Ctx, denomID, alice, carol, 10); err != nil {
		t.Fatalf("MoveBalance: %v", err)
	}
	if f.k.GetBalance(f.ts.Ctx, denomID, carol) != 10 {
		t.Fatal("MoveBalance did not credit")
	}
}

// ---- paused denom ----------------------------------------------------------

func TestPausedDenomBlocksTransferButAllowsBurn(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.addMinter(t)
	f.mint(t, alice, 100)
	_, _ = f.srv.SetDenomStatus(f.ts.Ctx, &types.MsgSetDenomStatus{Admin: admin, DenomId: denomID, Status: types.DenomStatus_DENOM_STATUS_PAUSED})
	if _, err := f.srv.Transfer(f.ts.Ctx, &types.MsgTransfer{From: alice, DenomId: denomID, To: bob, Amount: 1}); err == nil {
		t.Fatal("paused denom must block transfers")
	}
	if _, err := f.srv.Burn(f.ts.Ctx, &types.MsgBurn{Holder: alice, DenomId: denomID, Amount: 10}); err != nil {
		t.Fatalf("burn should be allowed on paused denom: %v", err)
	}
}

// ---- genesis ---------------------------------------------------------------

func TestGenesisRoundtrip(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.addMinter(t)
	f.mint(t, alice, 1000)
	f.mint(t, bob, 500)
	_, _ = f.srv.Approve(f.ts.Ctx, &types.MsgApprove{Owner: alice, DenomId: denomID, Spender: bob, Amount: 100})
	_, _ = f.srv.Freeze(f.ts.Ctx, &types.MsgFreeze{Admin: admin, DenomId: denomID, Account: carol})
	_, _ = f.srv.RequestRedemption(f.ts.Ctx, &types.MsgRequestRedemption{Holder: alice, DenomId: denomID, Amount: 200})

	exported, err := f.k.ExportGenesis(f.ts.Ctx)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if err := exported.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}

	g := setup(t)
	if err := g.k.InitGenesis(g.ts.Ctx, exported); err != nil {
		t.Fatalf("init: %v", err)
	}
	re, err := g.k.ExportGenesis(g.ts.Ctx)
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	// supply invariant: alice 800 + bob 500 + escrow 200 = 1500 (request
	// escrows rather than burns, so total supply is unchanged)
	if g.k.GetSupply(g.ts.Ctx, denomID) != 1500 {
		t.Fatalf("supply after import = %d", g.k.GetSupply(g.ts.Ctx, denomID))
	}
	if g.k.GetBalance(g.ts.Ctx, denomID, types.RedemptionEscrow) != 200 {
		t.Fatalf("escrow after import = %d", g.k.GetBalance(g.ts.Ctx, denomID, types.RedemptionEscrow))
	}
	if len(re.Denoms) != 1 || len(re.Redemptions) != 1 || re.RedemptionIdSeq != exported.RedemptionIdSeq {
		t.Fatalf("roundtrip mismatch: %+v", re)
	}
}

func TestGenesisRejectsPhantomRedemption(t *testing.T) {
	base := func() *types.GenesisState {
		return &types.GenesisState{
			Params: types.DefaultParams(),
			Denoms: []types.StableDenom{{
				Id: denomID, Symbol: "WeUSD", Admin: admin, Status: types.DenomStatus_DENOM_STATUS_ACTIVE,
			}},
			Balances: []types.Balance{{DenomId: denomID, Account: alice, Amount: 1000}},
			Supplies: []types.Supply{{DenomId: denomID, Amount: 1000}},
		}
	}

	// A PENDING redemption with NO matching escrow balance must be rejected.
	phantom := base()
	phantom.Redemptions = []types.Redemption{{
		Id: 1, DenomId: denomID, Holder: alice, Amount: 500,
		Status: types.RedemptionStatus_REDEMPTION_STATUS_PENDING,
	}}
	phantom.RedemptionIdSeq = 1
	if err := phantom.Validate(); err == nil {
		t.Fatal("expected phantom pending redemption to be rejected")
	}

	// The same redemption is valid once escrow holds the locked tokens and
	// supply accounts for them.
	ok := base()
	ok.Balances = []types.Balance{
		{DenomId: denomID, Account: alice, Amount: 1000},
		{DenomId: denomID, Account: types.RedemptionEscrow, Amount: 500},
	}
	ok.Supplies = []types.Supply{{DenomId: denomID, Amount: 1500}}
	ok.Redemptions = []types.Redemption{{
		Id: 1, DenomId: denomID, Holder: alice, Amount: 500,
		Status: types.RedemptionStatus_REDEMPTION_STATUS_PENDING,
	}}
	ok.RedemptionIdSeq = 1
	if err := ok.Validate(); err != nil {
		t.Fatalf("expected valid escrow-backed genesis: %v", err)
	}
}

func TestUpdateParamsAuthority(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.UpdateParams(f.ts.Ctx, &types.MsgUpdateParams{Authority: stranger, Params: types.DefaultParams()}); err == nil {
		t.Fatal("expected authority gate")
	}
	bad := types.DefaultParams()
	bad.MaxDenoms = 0
	if _, err := f.srv.UpdateParams(f.ts.Ctx, &types.MsgUpdateParams{Authority: authority, Params: bad}); err == nil {
		t.Fatal("expected param validation rejection")
	}
}
