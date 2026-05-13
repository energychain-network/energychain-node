package keeper_test

import (
	"fmt"
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

	"energychain/x/rwa/keeper"
	"energychain/x/rwa/types"
)

// ---- Stubs ---------------------------------------------------------------

type stubPolicy struct {
	deny    bool
	denyErr string
	calls   int
}

func (s *stubPolicy) EvaluateTransfer(_ sdk.Context, _, _, _, _ string, _ uint64) error {
	s.calls++
	if s.deny {
		return fmt.Errorf("%s", s.denyErr)
	}
	return nil
}

type stubSanctions struct{ bad map[string]bool }

func (s *stubSanctions) IsSanctioned(_ sdk.Context, subject string) bool { return s.bad[subject] }

// stubStablecoin is a simple in-memory ledger for test payouts. Missing
// denom / underflows / overflows mirror real x/stablecoin behaviour.
type stubStablecoin struct {
	denoms  map[string]bool
	bal     map[string]map[string]uint64
	failNext bool
}

func newStubStablecoin() *stubStablecoin {
	return &stubStablecoin{
		denoms: map[string]bool{},
		bal:    map[string]map[string]uint64{},
	}
}
func (s *stubStablecoin) HasDenom(_ sdk.Context, d string) bool { return s.denoms[d] }
func (s *stubStablecoin) credit(d, acc string, n uint64) {
	if _, ok := s.bal[d]; !ok {
		s.bal[d] = map[string]uint64{}
	}
	s.bal[d][acc] += n
}
func (s *stubStablecoin) get(d, acc string) uint64 {
	if _, ok := s.bal[d]; !ok {
		return 0
	}
	return s.bal[d][acc]
}
func (s *stubStablecoin) Move(_ sdk.Context, d, from, to string, amount uint64) error {
	if s.failNext {
		s.failNext = false
		return fmt.Errorf("stablecoin failure injected")
	}
	if !s.denoms[d] {
		return fmt.Errorf("denom %s missing", d)
	}
	cur := s.get(d, from)
	if cur < amount {
		return fmt.Errorf("insufficient %s: %d < %d", d, cur, amount)
	}
	s.bal[d][from] = cur - amount
	if s.bal[d][from] == 0 {
		delete(s.bal[d], from)
	}
	s.credit(d, to, amount)
	return nil
}

type stubAudit struct{ calls []string }

func (s *stubAudit) RecordRWAAction(_ sdk.Context, tokenID uint64, issuerID, action, actor, subject, detail string) {
	s.calls = append(s.calls, strings.Join([]string{itoa(tokenID), issuerID, action, actor, subject, detail}, "|"))
}

func itoa(u uint64) string {
	if u == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for u > 0 {
		i--
		buf[i] = byte('0' + u%10)
		u /= 10
	}
	return string(buf[i:])
}

// ---- Constants -----------------------------------------------------------

const (
	authority   = "cosmos126xkl2rxz08nlv6egudef8dxn6c5suxwnsmrjc"
	issuerAdmin = "cosmos1mc62kvf7adulmqcpgynqupl63hfp68lu82qjlk"
	issuerAuth  = "cosmos150ln4xlvs2dg024ueft43un6e3eav7ca2xhhtf"
	holderAlice = "cosmos1tmxy2mearm0zkw88prwm346aqdqns6m8zlxyrg"
	holderBob   = "cosmos19mup59y4ep02x77uy5vjnq8c4k5a044cwlxkmq"
	holderCarol = "cosmos1lrmkh72j5pqd5rav9wy8tzdq3zlmd3ullte2ev"
	issuerDID   = "did:web:rwa.example.com:issuers:1"

	usdDenom = "usd"
	symbol1  = "PPA001"
)

// ---- Setup ---------------------------------------------------------------

type fixture struct {
	k   keeper.Keeper
	ctx sdk.Context
	srv types.MsgServer
	pol *stubPolicy
	san *stubSanctions
	sc  *stubStablecoin
	au  *stubAudit
}

func setup(t *testing.T) *fixture {
	t.Helper()
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	cms := store.NewCommitMultiStore(db, log.NewNopLogger())
	cms.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := cms.LoadLatestVersion(); err != nil {
		t.Fatal(err)
	}
	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)

	pol := &stubPolicy{}
	san := &stubSanctions{bad: map[string]bool{}}
	sc := newStubStablecoin()
	sc.denoms[usdDenom] = true
	au := &stubAudit{}

	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, pol, san, sc, au)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return &fixture{
		k: k, ctx: ctx, srv: keeper.NewMsgServerImpl(k),
		pol: pol, san: san, sc: sc, au: au,
	}
}

func (f *fixture) registerIssuer(t *testing.T) {
	t.Helper()
	if _, err := f.srv.RegisterIssuer(f.ctx, &types.MsgRegisterIssuer{
		Authority:       authority,
		Id:              "verra",
		Did:             issuerDID,
		DisplayName:     "Verra",
		IssuerAuthority: issuerAuth,
		IssuerAdmin:     issuerAdmin,
	}); err != nil {
		t.Fatalf("register issuer: %v", err)
	}
}

func (f *fixture) createToken(t *testing.T, opts ...func(*types.MsgCreateToken)) uint64 {
	t.Helper()
	m := &types.MsgCreateToken{
		Admin:                  issuerAdmin,
		IssuerId:               "verra",
		Symbol:                 symbol1,
		DisplayName:            "PPA Series 1",
		Decimals:               6,
		AssetClass:             types.AssetClass_ASSET_CLASS_PPA,
		Jurisdiction:           "BR",
		PolicyId:               "rwa.standard",
		SettlementDenom:        usdDenom,
		PerHolderCap:           0,
		TotalSupplyCap:         0,
		RedemptionRate:         types.RedemptionRateScale, // 1:1
		RedemptionDelaySeconds: 60,
		RequireKycHolders:      false,
	}
	for _, o := range opts {
		o(m)
	}
	resp, err := f.srv.CreateToken(f.ctx, m)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return resp.TokenId
}

func (f *fixture) mint(t *testing.T, tokenID uint64, recipient string, amount uint64) {
	t.Helper()
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{
		IssuerAuthority: issuerAuth, TokenId: tokenID, Recipient: recipient, Amount: amount, Memo: "mint",
	}); err != nil {
		t.Fatalf("mint: %v", err)
	}
}

func mustBalance(t *testing.T, f *fixture, tokenID uint64, account string, want uint64) {
	t.Helper()
	got, err := f.k.GetBalance(f.ctx, tokenID, account)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("balance(%d, %s): got %d, want %d", tokenID, account, got, want)
	}
}

// ---------------------------------------------------------------------------
// Issuer lifecycle
// ---------------------------------------------------------------------------

func TestRegisterIssuerOK(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	is, ok, err := f.k.GetIssuer(f.ctx, "verra")
	if err != nil || !ok {
		t.Fatalf("get issuer: ok=%v err=%v", ok, err)
	}
	if is.Status != types.IssuerStatus_ISSUER_STATUS_ACTIVE {
		t.Fatalf("status: got %s", is.Status)
	}
}

func TestRegisterIssuerOnlyAuthority(t *testing.T) {
	f := setup(t)
	_, err := f.srv.RegisterIssuer(f.ctx, &types.MsgRegisterIssuer{
		Authority: holderAlice, Id: "verra", Did: issuerDID, DisplayName: "Verra",
		IssuerAuthority: issuerAuth, IssuerAdmin: issuerAdmin,
	})
	if err == nil || !strings.Contains(err.Error(), "expected authority") {
		t.Fatalf("expected authority error, got %v", err)
	}
}

func TestSuspendAndRevokeIssuer(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	if _, err := f.srv.SuspendIssuer(f.ctx, &types.MsgSuspendIssuer{Authority: authority, Id: "verra", Reason: "audit"}); err != nil {
		t.Fatal(err)
	}
	is, _, _ := f.k.GetIssuer(f.ctx, "verra")
	if is.Status != types.IssuerStatus_ISSUER_STATUS_SUSPENDED {
		t.Fatalf("expected suspended, got %s", is.Status)
	}
	if _, err := f.srv.RevokeIssuer(f.ctx, &types.MsgRevokeIssuer{Authority: authority, Id: "verra", Reason: "fraud"}); err != nil {
		t.Fatal(err)
	}
	is, _, _ = f.k.GetIssuer(f.ctx, "verra")
	if is.Status != types.IssuerStatus_ISSUER_STATUS_REVOKED {
		t.Fatalf("expected revoked, got %s", is.Status)
	}
	// after revocation, update is rejected
	if _, err := f.srv.UpdateIssuer(f.ctx, &types.MsgUpdateIssuer{Authority: authority, Id: "verra", DisplayName: "X"}); err == nil {
		t.Fatalf("update on revoked issuer must fail")
	}
}

// ---------------------------------------------------------------------------
// Token lifecycle
// ---------------------------------------------------------------------------

func TestCreateTokenRequiresIssuerAdmin(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	_, err := f.srv.CreateToken(f.ctx, &types.MsgCreateToken{
		Admin: holderAlice, IssuerId: "verra", Symbol: "X", DisplayName: "X",
		AssetClass: types.AssetClass_ASSET_CLASS_PPA, SettlementDenom: usdDenom,
	})
	if err == nil || !strings.Contains(err.Error(), "issuer admin") {
		t.Fatalf("expected admin error, got %v", err)
	}
}

func TestCreateTokenRequiresKnownDenom(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	_, err := f.srv.CreateToken(f.ctx, &types.MsgCreateToken{
		Admin: issuerAdmin, IssuerId: "verra", Symbol: "X", DisplayName: "X",
		AssetClass: types.AssetClass_ASSET_CLASS_PPA, SettlementDenom: "eur",
	})
	if err == nil || !strings.Contains(err.Error(), "settlement_denom") {
		t.Fatalf("expected denom error, got %v", err)
	}
}

func TestCreateTokenSymbolUnique(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	f.createToken(t)
	_, err := f.srv.CreateToken(f.ctx, &types.MsgCreateToken{
		Admin: issuerAdmin, IssuerId: "verra", Symbol: symbol1, DisplayName: "Other",
		AssetClass: types.AssetClass_ASSET_CLASS_PPA, SettlementDenom: usdDenom,
	})
	if err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("expected symbol-uniqueness error, got %v", err)
	}
}

func TestPauseUnpauseAndTerminate(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	if _, err := f.srv.PauseToken(f.ctx, &types.MsgPauseToken{Admin: issuerAdmin, TokenId: tid, Reason: "maintenance"}); err != nil {
		t.Fatal(err)
	}
	t1, _, _ := f.k.GetToken(f.ctx, tid)
	if t1.Status != types.TokenStatus_TOKEN_STATUS_PAUSED {
		t.Fatalf("expected paused")
	}
	// transfers refused while paused (need balance first)
	if _, err := f.srv.UnpauseToken(f.ctx, &types.MsgUnpauseToken{Admin: issuerAdmin, TokenId: tid, Reason: "ok"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.TerminateToken(f.ctx, &types.MsgTerminateToken{Admin: issuerAdmin, TokenId: tid, Reason: "wind down"}); err != nil {
		t.Fatal(err)
	}
	// can't unpause a terminated token
	if _, err := f.srv.UnpauseToken(f.ctx, &types.MsgUnpauseToken{Admin: issuerAdmin, TokenId: tid, Reason: "x"}); err == nil {
		t.Fatalf("unpause-after-terminate should fail")
	}
}

// ---------------------------------------------------------------------------
// Mint / burn
// ---------------------------------------------------------------------------

func TestMintHappyPathTracksSupply(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 1000)
	mustBalance(t, f, tid, holderAlice, 1000)
	tok, _, _ := f.k.GetToken(f.ctx, tid)
	if tok.TotalSupply != 1000 {
		t.Fatalf("supply=%d", tok.TotalSupply)
	}
}

func TestMintRespectsTotalSupplyCap(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t, func(m *types.MsgCreateToken) { m.TotalSupplyCap = 500 })
	f.mint(t, tid, holderAlice, 400)
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{IssuerAuthority: issuerAuth, TokenId: tid, Recipient: holderBob, Amount: 200, Memo: ""}); err == nil {
		t.Fatalf("mint past cap should fail")
	}
}

func TestMintRespectsPerHolderCap(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t, func(m *types.MsgCreateToken) { m.PerHolderCap = 100 })
	f.mint(t, tid, holderAlice, 100)
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{IssuerAuthority: issuerAuth, TokenId: tid, Recipient: holderAlice, Amount: 1, Memo: ""}); err == nil {
		t.Fatalf("over-cap mint should fail")
	}
}

func TestMintRequiresKycWhenFlagSet(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t, func(m *types.MsgCreateToken) { m.RequireKycHolders = true })
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{IssuerAuthority: issuerAuth, TokenId: tid, Recipient: holderAlice, Amount: 10}); err == nil {
		t.Fatalf("mint without KYC must fail")
	}
	if _, err := f.srv.SetAccountFlags(f.ctx, &types.MsgSetAccountFlags{
		Admin: issuerAdmin, TokenId: tid, Account: holderAlice, KycCleared: true,
	}); err != nil {
		t.Fatal(err)
	}
	f.mint(t, tid, holderAlice, 10)
	mustBalance(t, f, tid, holderAlice, 10)
}

func TestMintBlocksSanctionedRecipient(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.san.bad[holderAlice] = true
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{IssuerAuthority: issuerAuth, TokenId: tid, Recipient: holderAlice, Amount: 1}); err == nil {
		t.Fatalf("sanctioned recipient must be rejected")
	}
}

func TestBurnTracksSupply(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 100)
	if _, err := f.srv.Burn(f.ctx, &types.MsgBurn{IssuerAuthority: issuerAuth, TokenId: tid, From: holderAlice, Amount: 30, Reason: "ok"}); err != nil {
		t.Fatal(err)
	}
	mustBalance(t, f, tid, holderAlice, 70)
	tok, _, _ := f.k.GetToken(f.ctx, tid)
	if tok.TotalSupply != 70 {
		t.Fatalf("supply=%d", tok.TotalSupply)
	}
}

// ---------------------------------------------------------------------------
// Transfer
// ---------------------------------------------------------------------------

func TestTransferRejectsPausedToken(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 100)
	if _, err := f.srv.PauseToken(f.ctx, &types.MsgPauseToken{Admin: issuerAdmin, TokenId: tid, Reason: ""}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{From: holderAlice, To: holderBob, TokenId: tid, Amount: 1, Memo: ""}); err == nil {
		t.Fatalf("transfer on paused token must fail")
	}
}

func TestTransferRejectsSanctionedReceiver(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 100)
	f.san.bad[holderBob] = true
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{From: holderAlice, To: holderBob, TokenId: tid, Amount: 1, Memo: ""}); err == nil {
		t.Fatalf("sanctioned receiver must be blocked")
	}
}

func TestTransferRespectsFrozenAndLockup(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 100)

	// freeze 30 units of Alice's 100
	if _, err := f.srv.SetFrozenBalance(f.ctx, &types.MsgSetFrozenBalance{
		IssuerAuthority: issuerAuth, TokenId: tid, Account: holderAlice, Amount: 30, Reason: "court",
	}); err != nil {
		t.Fatal(err)
	}
	// lockup 50 more — leaves only 20 transferable
	if _, err := f.srv.AddLockup(f.ctx, &types.MsgAddLockup{
		IssuerAuthority: issuerAuth, TokenId: tid, Account: holderAlice, Amount: 50,
		UnlockTime: f.ctx.BlockTime().Unix() + 3600, Reason: "vesting",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, TokenId: tid, Amount: 25,
	}); err == nil {
		t.Fatalf("transfer of 25 should fail (transferable=20)")
	}
	// 20 ok
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, TokenId: tid, Amount: 20,
	}); err != nil {
		t.Fatalf("transfer of 20 should succeed: %v", err)
	}
}

func TestLockupOverReserveRejected(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 100)
	if _, err := f.srv.SetFrozenBalance(f.ctx, &types.MsgSetFrozenBalance{
		IssuerAuthority: issuerAuth, TokenId: tid, Account: holderAlice, Amount: 60, Reason: "x",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.AddLockup(f.ctx, &types.MsgAddLockup{
		IssuerAuthority: issuerAuth, TokenId: tid, Account: holderAlice, Amount: 50,
		UnlockTime: f.ctx.BlockTime().Unix() + 3600, Reason: "x",
	}); err == nil {
		t.Fatalf("lockup over-reserve should fail")
	}
}

// ForceTransfer can move frozen units; bypasses the transferable check.
func TestForceTransferBypassesFreeze(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 100)
	if _, err := f.srv.SetFrozenBalance(f.ctx, &types.MsgSetFrozenBalance{
		IssuerAuthority: issuerAuth, TokenId: tid, Account: holderAlice, Amount: 100, Reason: "court",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.ForceTransfer(f.ctx, &types.MsgForceTransfer{
		IssuerAuthority: issuerAuth, TokenId: tid, From: holderAlice, To: holderBob, Amount: 80, Reason: "court order",
	}); err != nil {
		t.Fatalf("force transfer should succeed: %v", err)
	}
	mustBalance(t, f, tid, holderAlice, 20)
	mustBalance(t, f, tid, holderBob, 80)
	// frozen should be trimmed to remaining balance (20)
	frozen, err := f.k.GetFrozen(f.ctx, tid, holderAlice)
	if err != nil {
		t.Fatal(err)
	}
	if frozen != 20 {
		t.Fatalf("frozen should be trimmed to 20, got %d", frozen)
	}
}

// ---------------------------------------------------------------------------
// Snapshot / distribution / claim / finalize
// ---------------------------------------------------------------------------

func TestSnapshotAndDistributionFullFlow(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 700)
	f.mint(t, tid, holderBob, 300)

	snap, err := f.srv.TakeSnapshot(f.ctx, &types.MsgTakeSnapshot{Admin: issuerAdmin, TokenId: tid})
	if err != nil {
		t.Fatal(err)
	}
	if snap.HolderCount != 2 || snap.TotalSupply != 1000 {
		t.Fatalf("snapshot meta wrong: %+v", snap)
	}

	dist, err := f.srv.CreateDistribution(f.ctx, &types.MsgCreateDistribution{
		Admin: issuerAdmin, TokenId: tid, SnapshotId: snap.SnapshotId, TotalAmount: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Fund: requires the issuer-authority to hold 1000 USD already.
	f.sc.credit(usdDenom, issuerAuth, 1000)
	if _, err := f.srv.FundDistribution(f.ctx, &types.MsgFundDistribution{
		IssuerAuthority: issuerAuth, DistributionId: dist.DistributionId,
	}); err != nil {
		t.Fatal(err)
	}
	if f.sc.get(usdDenom, issuerAuth) != 0 {
		t.Fatalf("issuer authority not debited; bal=%d", f.sc.get(usdDenom, issuerAuth))
	}

	// Alice claims 700, Bob claims 300.
	r1, err := f.srv.ClaimDistribution(f.ctx, &types.MsgClaimDistribution{Claimer: holderAlice, DistributionId: dist.DistributionId})
	if err != nil {
		t.Fatal(err)
	}
	if r1.Amount != 700 {
		t.Fatalf("alice claim: got %d, want 700", r1.Amount)
	}
	r2, err := f.srv.ClaimDistribution(f.ctx, &types.MsgClaimDistribution{Claimer: holderBob, DistributionId: dist.DistributionId})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Amount != 300 {
		t.Fatalf("bob claim: got %d, want 300", r2.Amount)
	}
	if f.sc.get(usdDenom, holderAlice) != 700 || f.sc.get(usdDenom, holderBob) != 300 {
		t.Fatalf("payout balances wrong")
	}
	// Double-claim refused
	if _, err := f.srv.ClaimDistribution(f.ctx, &types.MsgClaimDistribution{Claimer: holderAlice, DistributionId: dist.DistributionId}); err == nil {
		t.Fatalf("double claim must fail")
	}
}

func TestDistributionFinalizeSweepsRemainder(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 700)
	f.mint(t, tid, holderBob, 300)
	snap, _ := f.srv.TakeSnapshot(f.ctx, &types.MsgTakeSnapshot{Admin: issuerAdmin, TokenId: tid})
	dist, _ := f.srv.CreateDistribution(f.ctx, &types.MsgCreateDistribution{
		Admin: issuerAdmin, TokenId: tid, SnapshotId: snap.SnapshotId, TotalAmount: 1000,
	})
	f.sc.credit(usdDenom, issuerAuth, 1000)
	if _, err := f.srv.FundDistribution(f.ctx, &types.MsgFundDistribution{
		IssuerAuthority: issuerAuth, DistributionId: dist.DistributionId,
	}); err != nil {
		t.Fatal(err)
	}
	// Only Alice claims; 300 should be swept back.
	if _, err := f.srv.ClaimDistribution(f.ctx, &types.MsgClaimDistribution{Claimer: holderAlice, DistributionId: dist.DistributionId}); err != nil {
		t.Fatal(err)
	}
	res, err := f.srv.FinalizeDistribution(f.ctx, &types.MsgFinalizeDistribution{
		Admin: issuerAdmin, DistributionId: dist.DistributionId,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.SweptBack != 300 {
		t.Fatalf("swept_back: got %d, want 300", res.SweptBack)
	}
	if f.sc.get(usdDenom, issuerAuth) != 300 {
		t.Fatalf("issuer authority should have 300 returned, got %d", f.sc.get(usdDenom, issuerAuth))
	}
	// claims after finalize refused
	if _, err := f.srv.ClaimDistribution(f.ctx, &types.MsgClaimDistribution{Claimer: holderBob, DistributionId: dist.DistributionId}); err == nil {
		t.Fatalf("claim after finalize must fail")
	}
}

// Sanctions hook applies to claimer (defense-in-depth) — even though the
// token already gates new transfers, the snapshot may pre-date the listing.
func TestClaimDistributionRejectsSanctionedClaimer(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 1000)
	snap, _ := f.srv.TakeSnapshot(f.ctx, &types.MsgTakeSnapshot{Admin: issuerAdmin, TokenId: tid})
	dist, _ := f.srv.CreateDistribution(f.ctx, &types.MsgCreateDistribution{
		Admin: issuerAdmin, TokenId: tid, SnapshotId: snap.SnapshotId, TotalAmount: 1000,
	})
	f.sc.credit(usdDenom, issuerAuth, 1000)
	if _, err := f.srv.FundDistribution(f.ctx, &types.MsgFundDistribution{IssuerAuthority: issuerAuth, DistributionId: dist.DistributionId}); err != nil {
		t.Fatal(err)
	}
	f.san.bad[holderAlice] = true
	if _, err := f.srv.ClaimDistribution(f.ctx, &types.MsgClaimDistribution{Claimer: holderAlice, DistributionId: dist.DistributionId}); err == nil {
		t.Fatalf("sanctioned claimer must be blocked")
	}
}

// ---------------------------------------------------------------------------
// Redemption queue
// ---------------------------------------------------------------------------

func TestRedemptionRequestSettleSupplyInvariant(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 1000)

	resp, err := f.srv.RequestRedemption(f.ctx, &types.MsgRequestRedemption{
		Holder: holderAlice, TokenId: tid, Amount: 400, Memo: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Holder balance debited, pending pool credited.
	mustBalance(t, f, tid, holderAlice, 600)
	pending, _ := f.k.GetPendingRedemptionUnits(f.ctx, tid)
	if pending != 400 {
		t.Fatalf("pending=%d", pending)
	}
	// Supply unchanged until settle.
	tok, _, _ := f.k.GetToken(f.ctx, tid)
	if tok.TotalSupply != 1000 {
		t.Fatalf("pre-settle supply changed: %d", tok.TotalSupply)
	}

	// Cannot settle before eligibility window.
	if _, err := f.srv.SettleRedemption(f.ctx, &types.MsgSettleRedemption{IssuerAuthority: issuerAuth, RedemptionId: resp.RedemptionId}); err == nil {
		t.Fatalf("early settle must fail")
	}
	// Advance clock past eligibility.
	ctx2 := f.ctx.WithBlockTime(f.ctx.BlockTime().Add(2 * time.Minute))

	// Issuer authority must hold settlement funds.
	f.sc.credit(usdDenom, issuerAuth, 1000)
	if _, err := f.srv.SettleRedemption(ctx2, &types.MsgSettleRedemption{IssuerAuthority: issuerAuth, RedemptionId: resp.RedemptionId}); err != nil {
		t.Fatalf("settle: %v", err)
	}
	tok, _, _ = f.k.GetToken(ctx2, tid)
	if tok.TotalSupply != 600 {
		t.Fatalf("post-settle supply: got %d, want 600", tok.TotalSupply)
	}
	pending, _ = f.k.GetPendingRedemptionUnits(ctx2, tid)
	if pending != 0 {
		t.Fatalf("pending should clear, got %d", pending)
	}
	if f.sc.get(usdDenom, holderAlice) != resp.SettlementAmount {
		t.Fatalf("alice payout=%d want %d", f.sc.get(usdDenom, holderAlice), resp.SettlementAmount)
	}
}

func TestRedemptionCancelReturnsUnits(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 1000)
	resp, _ := f.srv.RequestRedemption(f.ctx, &types.MsgRequestRedemption{
		Holder: holderAlice, TokenId: tid, Amount: 300, Memo: "",
	})
	if _, err := f.srv.CancelRedemption(f.ctx, &types.MsgCancelRedemption{
		Actor: holderAlice, RedemptionId: resp.RedemptionId, Reason: "changed mind",
	}); err != nil {
		t.Fatal(err)
	}
	mustBalance(t, f, tid, holderAlice, 1000)
	pending, _ := f.k.GetPendingRedemptionUnits(f.ctx, tid)
	if pending != 0 {
		t.Fatalf("pending should clear after cancel, got %d", pending)
	}
}

func TestRedemptionRejectsWhenRateZero(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t, func(m *types.MsgCreateToken) {
		m.RedemptionRate = 0
		m.RedemptionDelaySeconds = 0
	})
	f.mint(t, tid, holderAlice, 100)
	if _, err := f.srv.RequestRedemption(f.ctx, &types.MsgRequestRedemption{
		Holder: holderAlice, TokenId: tid, Amount: 10,
	}); err == nil {
		t.Fatalf("redemption on rate-0 token must fail")
	}
}

func TestRedemptionRespectsTransferable(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 100)
	if _, err := f.srv.SetFrozenBalance(f.ctx, &types.MsgSetFrozenBalance{
		IssuerAuthority: issuerAuth, TokenId: tid, Account: holderAlice, Amount: 80, Reason: "x",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.RequestRedemption(f.ctx, &types.MsgRequestRedemption{
		Holder: holderAlice, TokenId: tid, Amount: 30,
	}); err == nil {
		t.Fatalf("over-transferable redemption must fail")
	}
}

// ---------------------------------------------------------------------------
// Genesis roundtrip
// ---------------------------------------------------------------------------

func TestGenesisRoundtrip(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 700)
	f.mint(t, tid, holderBob, 300)
	if _, err := f.srv.RequestRedemption(f.ctx, &types.MsgRequestRedemption{
		Holder: holderAlice, TokenId: tid, Amount: 100,
	}); err != nil {
		t.Fatal(err)
	}
	exported, err := f.k.ExportGenesis(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := exported.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}

	// Fresh keeper, replay.
	f2 := setup(t)
	if err := f2.k.InitGenesis(f2.ctx, exported); err != nil {
		t.Fatalf("init genesis: %v", err)
	}
	exported2, err := f2.k.ExportGenesis(f2.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if exported2.Tokens[0].TotalSupply != 1000 {
		t.Fatalf("supply lost across roundtrip: %d", exported2.Tokens[0].TotalSupply)
	}
	pending := uint64(0)
	for _, p := range exported2.PendingRedemptionUnits {
		if p.TokenId == tid {
			pending = p.Amount
		}
	}
	if pending != 100 {
		t.Fatalf("pending units lost across roundtrip: %d", pending)
	}
}

// CountPendingRedemptionsForHolder caps abuse; verify it counts only pending.
func TestPendingRedemptionsCountSkipsSettled(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 1000)
	r1, _ := f.srv.RequestRedemption(f.ctx, &types.MsgRequestRedemption{Holder: holderAlice, TokenId: tid, Amount: 100})
	_, _ = f.srv.RequestRedemption(f.ctx, &types.MsgRequestRedemption{Holder: holderAlice, TokenId: tid, Amount: 100})
	if c, _ := f.k.CountPendingRedemptionsForHolder(f.ctx, holderAlice); c != 2 {
		t.Fatalf("count=%d", c)
	}
	ctx2 := f.ctx.WithBlockTime(f.ctx.BlockTime().Add(2 * time.Minute))
	f.sc.credit(usdDenom, issuerAuth, 1000)
	if _, err := f.srv.SettleRedemption(ctx2, &types.MsgSettleRedemption{IssuerAuthority: issuerAuth, RedemptionId: r1.RedemptionId}); err != nil {
		t.Fatal(err)
	}
	if c, _ := f.k.CountPendingRedemptionsForHolder(ctx2, holderAlice); c != 1 {
		t.Fatalf("count after settle=%d", c)
	}
}

// ---------------------------------------------------------------------------
// Authorization / negative paths
// ---------------------------------------------------------------------------

func TestMintRequiresIssuerAuthority(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{IssuerAuthority: holderAlice, TokenId: tid, Recipient: holderAlice, Amount: 1}); err == nil {
		t.Fatalf("non-authority mint must fail")
	}
}

func TestForceTransferRequiresIssuerAuthority(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 100)
	if _, err := f.srv.ForceTransfer(f.ctx, &types.MsgForceTransfer{
		IssuerAuthority: holderAlice, TokenId: tid, From: holderAlice, To: holderBob, Amount: 10, Reason: "x",
	}); err == nil {
		t.Fatalf("non-authority force transfer must fail")
	}
}

// ---------------------------------------------------------------------------
// Regression: security fixes
// ---------------------------------------------------------------------------

// SettleRedemption must re-check the holder's sanctions status; a
// holder clean at request time but listed before settlement must not
// be paid out.
func TestSettleRejectsSanctionedHolderAtSettleTime(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 1000)
	resp, err := f.srv.RequestRedemption(f.ctx, &types.MsgRequestRedemption{
		Holder: holderAlice, TokenId: tid, Amount: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	// List the holder AFTER request, BEFORE settle.
	f.san.bad[holderAlice] = true
	f.sc.credit(usdDenom, issuerAuth, 1000)
	ctx2 := f.ctx.WithBlockTime(f.ctx.BlockTime().Add(2 * time.Minute))
	if _, err := f.srv.SettleRedemption(ctx2, &types.MsgSettleRedemption{
		IssuerAuthority: issuerAuth, RedemptionId: resp.RedemptionId,
	}); err == nil {
		t.Fatalf("settle for sanctioned holder must fail")
	}
}

// FundDistribution must refuse a sanctioned issuer authority.
func TestFundDistributionRejectsSanctionedFunder(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 1000)
	snap, _ := f.srv.TakeSnapshot(f.ctx, &types.MsgTakeSnapshot{Admin: issuerAdmin, TokenId: tid})
	dist, _ := f.srv.CreateDistribution(f.ctx, &types.MsgCreateDistribution{
		Admin: issuerAdmin, TokenId: tid, SnapshotId: snap.SnapshotId, TotalAmount: 1000,
	})
	f.sc.credit(usdDenom, issuerAuth, 1000)
	f.san.bad[issuerAuth] = true
	if _, err := f.srv.FundDistribution(f.ctx, &types.MsgFundDistribution{
		IssuerAuthority: issuerAuth, DistributionId: dist.DistributionId,
	}); err == nil {
		t.Fatalf("fund by sanctioned authority must fail")
	}
}

// ForceTransfer must enforce KYC on the receiver when the token
// requires KYC; otherwise the issuer authority can mint compliance
// holes via court orders.
func TestForceTransferRequiresReceiverKYC(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t, func(m *types.MsgCreateToken) { m.RequireKycHolders = true })
	if _, err := f.srv.SetAccountFlags(f.ctx, &types.MsgSetAccountFlags{
		Admin: issuerAdmin, TokenId: tid, Account: holderAlice, KycCleared: true,
	}); err != nil {
		t.Fatal(err)
	}
	f.mint(t, tid, holderAlice, 100)
	// Bob has no KYC — force-transfer must refuse.
	if _, err := f.srv.ForceTransfer(f.ctx, &types.MsgForceTransfer{
		IssuerAuthority: issuerAuth, TokenId: tid, From: holderAlice, To: holderBob, Amount: 10, Reason: "x",
	}); err == nil {
		t.Fatalf("force-transfer to unKYC'd receiver on a KYC token must fail")
	}
	// Clear Bob and retry.
	if _, err := f.srv.SetAccountFlags(f.ctx, &types.MsgSetAccountFlags{
		Admin: issuerAdmin, TokenId: tid, Account: holderBob, KycCleared: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.ForceTransfer(f.ctx, &types.MsgForceTransfer{
		IssuerAuthority: issuerAuth, TokenId: tid, From: holderAlice, To: holderBob, Amount: 10, Reason: "ok",
	}); err != nil {
		t.Fatalf("force-transfer to KYC'd receiver should succeed: %v", err)
	}
}

// AddLockup must prune matured rows so the per-holder row cap stays
// sustainable across years.
func TestAddLockupPrunesMaturedRows(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t)
	f.mint(t, tid, holderAlice, 1000)
	now := f.ctx.BlockTime().Unix()
	// Add a short lockup that will mature.
	if _, err := f.srv.AddLockup(f.ctx, &types.MsgAddLockup{
		IssuerAuthority: issuerAuth, TokenId: tid, Account: holderAlice,
		Amount: 100, UnlockTime: now + 60, Reason: "near",
	}); err != nil {
		t.Fatal(err)
	}
	// Advance past maturity.
	ctx2 := f.ctx.WithBlockTime(f.ctx.BlockTime().Add(2 * time.Minute))
	// Add a new lockup; the matured row should be pruned away.
	if _, err := f.srv.AddLockup(ctx2, &types.MsgAddLockup{
		IssuerAuthority: issuerAuth, TokenId: tid, Account: holderAlice,
		Amount: 200, UnlockTime: ctx2.BlockTime().Unix() + 3600, Reason: "next",
	}); err != nil {
		t.Fatalf("add second lockup: %v", err)
	}
	// Only the new (unmatured) lockup should be counted; the matured
	// row was pruned before the new add.
	_, _, rowCount, err := f.k.SumLocked(ctx2, tid, holderAlice, ctx2.BlockTime().Unix())
	if err != nil {
		t.Fatal(err)
	}
	if rowCount != 1 {
		t.Fatalf("rowCount after prune: got %d, want 1", rowCount)
	}
}

// DistributionPoolAccount must be a valid bech32 (round-trip).
func TestDistributionPoolAccountIsValid(t *testing.T) {
	if _, err := sdk.AccAddressFromBech32(keeper.DistributionPoolAccount); err != nil {
		t.Fatalf("DistributionPoolAccount %q invalid: %v", keeper.DistributionPoolAccount, err)
	}
}

func TestTransferRespectsPerHolderCap(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	tid := f.createToken(t, func(m *types.MsgCreateToken) { m.PerHolderCap = 50 })
	f.mint(t, tid, holderAlice, 50)
	// Bob has 0; mint to bob another 50 hits cap exactly.
	f.mint(t, tid, holderBob, 50)
	// Alice → Bob would push Bob to 51, fail.
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{From: holderAlice, To: holderBob, TokenId: tid, Amount: 1}); err == nil {
		t.Fatalf("over-cap transfer must fail")
	}
}
