package keeper_test

import (
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

	"energychain/x/stablecoin/keeper"
	"energychain/x/stablecoin/types"
)

// ---- Stub keepers ---------------------------------------------------------

type stubPolicy struct {
	deny    bool
	denyErr string
	calls   int
}

func (s *stubPolicy) EvaluateTransfer(_ sdk.Context, assetClass, assetID, sender, receiver string, amount uint64) error {
	s.calls++
	if s.deny {
		return errString(s.denyErr)
	}
	_ = assetClass
	_ = assetID
	_ = sender
	_ = receiver
	_ = amount
	return nil
}

type stubSanctions struct {
	bad map[string]bool
}

func (s *stubSanctions) IsSanctioned(_ sdk.Context, subject string) bool {
	return s.bad[subject]
}

type stubDID struct {
	creds map[string]map[string]bool
	live  map[string]bool
}

func (s *stubDID) IsActive(_ sdk.Context, subject string) bool { return s.live[subject] }
func (s *stubDID) HasCredential(_ sdk.Context, subject, ct string) bool {
	if s.creds == nil {
		return false
	}
	return s.creds[subject][ct]
}

type stubOracle struct {
	value uint64
	ts    int64
	ok    bool
}

func (s *stubOracle) GetAggregatedReserve(_ sdk.Context, _ string) (int64, int64, bool) {
	return int64(s.value), s.ts, s.ok
}

type stubAudit struct {
	calls []string
}

func (s *stubAudit) RecordStablecoinAction(_ sdk.Context, denom, issuer, action, actor, subject, detail string) {
	s.calls = append(s.calls, strings.Join([]string{denom, issuer, action, actor, subject, detail}, "|"))
}

func errString(s string) error { return &simpleErr{s} }

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }

// ---- Constants ------------------------------------------------------------

const (
	authority    = "cosmos1ye4j7hsfmwgvch53kpys4yzm88zwhjkprc7nzx"
	issuerAdmin  = "cosmos15tk4lhmtrwc4qx5gd5sjxdjdy36rg9hk0gx4tv"
	mintAuthA    = "cosmos1uu89qg49evj5pkj2u3xwapdjwt7ymsljt7epry"
	mintAuthB    = "cosmos1xrnner78enszhq32u4l5z29slzq2zfvka2lt7g"
	holderAlice  = "cosmos1alice000000000000000000000000000000000"
	holderBob    = "cosmos1bob00000000000000000000000000000000000"
	holderCarol  = "cosmos1carol00000000000000000000000000000000"
	otherAddr    = "cosmos1zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz0000"
	issuerDID    = "did:web:bank.example.com:issuers:1"
)

// ---- Setup ----------------------------------------------------------------

type fixture struct {
	k    keeper.Keeper
	ctx  sdk.Context
	srv  types.MsgServer
	pol  *stubPolicy
	san  *stubSanctions
	did  *stubDID
	ora  *stubOracle
	au   *stubAudit
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
	did := &stubDID{creds: map[string]map[string]bool{}, live: map[string]bool{}}
	ora := &stubOracle{ok: true, value: 1_000_000_000, ts: 1_700_000_000}
	au := &stubAudit{}

	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, pol, san, did, ora, au)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return &fixture{
		k:   k,
		ctx: ctx,
		srv: keeper.NewMsgServerImpl(k),
		pol: pol, san: san, did: did, ora: ora, au: au,
	}
}

// registerUSD seeds a denom + issuer + quota + reserve so most tests
// can immediately exercise the hot path.
func (f *fixture) registerUSD(t *testing.T, ceiling uint64) {
	t.Helper()
	if _, err := f.srv.RegisterDenom(f.ctx, &types.MsgRegisterDenom{
		Authority: authority, Id: "usd", Symbol: "scnUSD", Name: "EnergyChain USD",
		Decimals: 6, Jurisdiction: "US",
	}); err != nil {
		t.Fatalf("register denom: %v", err)
	}
	if _, err := f.srv.RegisterIssuer(f.ctx, &types.MsgRegisterIssuer{
		Authority: authority, Id: "alpha", Did: issuerDID,
		DisplayName: "Alpha Bank",
		MintAuthorities: []string{mintAuthA, mintAuthB},
		Admin:           issuerAdmin,
	}); err != nil {
		t.Fatalf("register issuer: %v", err)
	}
	if _, err := f.srv.SetMintQuota(f.ctx, &types.MsgSetMintQuota{
		Authority: authority, IssuerId: "alpha", DenomId: "usd", Ceiling: ceiling,
	}); err != nil {
		t.Fatalf("set quota: %v", err)
	}
	if _, err := f.srv.SetReserveRequirement(f.ctx, &types.MsgSetReserveRequirement{
		Authority: authority, DenomId: "usd",
		OracleTopicId: "reserve.usd", RequiredRatioBps: 10_000, MaxStalenessSeconds: 3600,
	}); err != nil {
		t.Fatalf("set reserve: %v", err)
	}
}

func (f *fixture) mint(t *testing.T, recipient string, amount uint64) {
	t.Helper()
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{
		Minter: mintAuthA, IssuerId: "alpha", DenomId: "usd",
		Recipient: recipient, Amount: amount,
	}); err != nil {
		t.Fatalf("mint: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Denom + issuer lifecycle
// ---------------------------------------------------------------------------

func TestRegisterDenom(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.RegisterDenom(f.ctx, &types.MsgRegisterDenom{
		Authority: authority, Id: "usd", Symbol: "scnUSD", Name: "USD",
		Decimals: 6, Jurisdiction: "US",
	}); err != nil {
		t.Fatal(err)
	}
	d, ok := f.k.GetDenom(f.ctx, "usd")
	if !ok || d.Status != types.DenomStatus_DENOM_STATUS_ACTIVE {
		t.Fatalf("want active denom, got %+v ok=%v", d, ok)
	}
}

func TestRegisterDenomDuplicate(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 1_000)
	_, err := f.srv.RegisterDenom(f.ctx, &types.MsgRegisterDenom{
		Authority: authority, Id: "usd", Symbol: "scnUSD", Name: "USD", Decimals: 6, Jurisdiction: "US",
	})
	if err == nil {
		t.Fatal("expected duplicate denom error")
	}
}

func TestRegisterDenomNonAuthority(t *testing.T) {
	f := setup(t)
	_, err := f.srv.RegisterDenom(f.ctx, &types.MsgRegisterDenom{
		Authority: holderAlice, Id: "usd", Symbol: "scnUSD", Name: "USD", Decimals: 6, Jurisdiction: "US",
	})
	if err == nil {
		t.Fatal("expected unauthorized")
	}
}

func TestPauseResumeRetireDenom(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 1_000_000)

	if _, err := f.srv.PauseDenom(f.ctx, &types.MsgPauseDenom{Authority: authority, Id: "usd", Reason: "audit"}); err != nil {
		t.Fatal(err)
	}
	if d, _ := f.k.GetDenom(f.ctx, "usd"); d.Status != types.DenomStatus_DENOM_STATUS_PAUSED {
		t.Fatalf("expected paused, got %v", d.Status)
	}

	// Mint refused while paused
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{
		Minter: mintAuthA, IssuerId: "alpha", DenomId: "usd", Recipient: holderAlice, Amount: 100,
	}); err == nil {
		t.Fatal("expected mint refused on paused denom")
	}

	// Resume
	if _, err := f.srv.ResumeDenom(f.ctx, &types.MsgResumeDenom{Authority: authority, Id: "usd"}); err != nil {
		t.Fatal(err)
	}
	if d, _ := f.k.GetDenom(f.ctx, "usd"); d.Status != types.DenomStatus_DENOM_STATUS_ACTIVE {
		t.Fatalf("expected active after resume, got %v", d.Status)
	}

	// Retire refused while supply > 0
	f.mint(t, holderAlice, 50)
	if _, err := f.srv.RetireDenom(f.ctx, &types.MsgRetireDenom{Authority: authority, Id: "usd"}); err == nil {
		t.Fatal("expected retire refused while supply > 0")
	}

	// Burn it down
	if _, err := f.srv.Burn(f.ctx, &types.MsgBurn{Holder: holderAlice, DenomId: "usd", Amount: 50}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.RetireDenom(f.ctx, &types.MsgRetireDenom{Authority: authority, Id: "usd"}); err != nil {
		t.Fatal(err)
	}
	if d, _ := f.k.GetDenom(f.ctx, "usd"); d.Status != types.DenomStatus_DENOM_STATUS_RETIRED {
		t.Fatalf("expected retired, got %v", d.Status)
	}

	// SECURITY: retired is terminal
	if _, err := f.srv.UpdateDenom(f.ctx, &types.MsgUpdateDenom{
		Authority: authority, Id: "usd", Symbol: "scnUSD2",
	}); err == nil {
		t.Fatal("expected update refused on retired denom")
	}
}

func TestRegisterIssuerCapEnforced(t *testing.T) {
	f := setup(t)
	p := types.DefaultParams()
	p.MaxIssuers = 1
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.RegisterIssuer(f.ctx, &types.MsgRegisterIssuer{
		Authority: authority, Id: "alpha", Did: issuerDID, DisplayName: "A",
		MintAuthorities: []string{mintAuthA}, Admin: issuerAdmin,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.RegisterIssuer(f.ctx, &types.MsgRegisterIssuer{
		Authority: authority, Id: "beta", Did: "did:web:beta.example.com",
		DisplayName: "B", MintAuthorities: []string{mintAuthB}, Admin: issuerAdmin,
	}); err == nil {
		t.Fatal("expected max_issuers cap to fire")
	}
}

func TestSuspendRevokeIssuer(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 1_000_000)
	if _, err := f.srv.SuspendIssuer(f.ctx, &types.MsgSuspendIssuer{Authority: authority, Id: "alpha"}); err != nil {
		t.Fatal(err)
	}
	// SECURITY: suspended issuer cannot mint
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{
		Minter: mintAuthA, IssuerId: "alpha", DenomId: "usd", Recipient: holderAlice, Amount: 100,
	}); err == nil {
		t.Fatal("expected mint refused on suspended issuer")
	}

	if _, err := f.srv.RevokeIssuer(f.ctx, &types.MsgRevokeIssuer{Authority: authority, Id: "alpha"}); err != nil {
		t.Fatal(err)
	}
	// SECURITY: revoked is terminal
	if _, err := f.srv.UpdateIssuer(f.ctx, &types.MsgUpdateIssuer{Authority: authority, Id: "alpha", DisplayName: "X"}); err == nil {
		t.Fatal("expected update refused on revoked issuer")
	}
}

// ---------------------------------------------------------------------------
// Mint
// ---------------------------------------------------------------------------

func TestMintHappyPath(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	resp, err := f.srv.Mint(f.ctx, &types.MsgMint{
		Minter: mintAuthA, IssuerId: "alpha", DenomId: "usd", Recipient: holderAlice, Amount: 1_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.NewOutstanding != 1_000 {
		t.Fatalf("outstanding=%d", resp.NewOutstanding)
	}
	if got := f.k.GetBalance(f.ctx, "usd", holderAlice); got != 1_000 {
		t.Fatalf("balance=%d", got)
	}
	if got := f.k.GetSupply(f.ctx, "usd"); got != 1_000 {
		t.Fatalf("supply=%d", got)
	}
}

func TestMintExceedsCeiling(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 100)
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{
		Minter: mintAuthA, IssuerId: "alpha", DenomId: "usd", Recipient: holderAlice, Amount: 101,
	}); err == nil {
		t.Fatal("expected ceiling breach")
	}
}

func TestMintUnauthorisedMinter(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 1_000)
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{
		Minter: otherAddr, IssuerId: "alpha", DenomId: "usd", Recipient: holderAlice, Amount: 100,
	}); err == nil {
		t.Fatal("expected unauthorised minter")
	}
}

func TestMintIssuerPausedFromAdmin(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	if _, err := f.srv.SetMintPaused(f.ctx, &types.MsgSetMintPaused{
		Admin: issuerAdmin, IssuerId: "alpha", DenomId: "usd", Paused: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{
		Minter: mintAuthA, IssuerId: "alpha", DenomId: "usd", Recipient: holderAlice, Amount: 100,
	}); err == nil {
		t.Fatal("expected mint refused when issuer paused")
	}
}

func TestMintReserveCoverage(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)

	// Reserve drops below the post-mint outstanding → mint refused.
	f.ora.value = 999
	f.ora.ok = true
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{
		Minter: mintAuthA, IssuerId: "alpha", DenomId: "usd",
		Recipient: holderAlice, Amount: 1_000,
	}); err == nil {
		t.Fatal("expected reserve insufficient")
	}

	// Bump reserve above coverage → succeeds.
	f.ora.value = 2_000
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{
		Minter: mintAuthA, IssuerId: "alpha", DenomId: "usd",
		Recipient: holderAlice, Amount: 1_000,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMintReserveStaleness(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.ora.value = 1_000_000
	// 7200s old, max staleness is 3600s
	f.ora.ts = f.ctx.BlockTime().Unix() - 7200
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{
		Minter: mintAuthA, IssuerId: "alpha", DenomId: "usd",
		Recipient: holderAlice, Amount: 1_000,
	}); err == nil {
		t.Fatal("expected stale reserve rejection")
	}
}

func TestMintReserveDisabledByParam(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	p := f.k.GetParams(f.ctx)
	p.MintRequiresReserve = false
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	f.ora.ok = false // oracle has nothing to say
	if _, err := f.srv.Mint(f.ctx, &types.MsgMint{
		Minter: mintAuthA, IssuerId: "alpha", DenomId: "usd",
		Recipient: holderAlice, Amount: 1_000,
	}); err != nil {
		t.Fatalf("expected mint OK with reserve check disabled, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Compliance pipeline
// ---------------------------------------------------------------------------

func TestTransferHappyPath(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, DenomId: "usd", Amount: 400,
	}); err != nil {
		t.Fatal(err)
	}
	if got := f.k.GetBalance(f.ctx, "usd", holderAlice); got != 600 {
		t.Fatalf("alice=%d", got)
	}
	if got := f.k.GetBalance(f.ctx, "usd", holderBob); got != 400 {
		t.Fatalf("bob=%d", got)
	}
	// supply is invariant under transfer
	if got := f.k.GetSupply(f.ctx, "usd"); got != 1_000 {
		t.Fatalf("supply=%d", got)
	}
}

func TestTransferFrozen(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)
	if _, err := f.srv.Freeze(f.ctx, &types.MsgFreeze{
		Admin: issuerAdmin, IssuerId: "alpha", DenomId: "usd", Account: holderAlice, Reason: "investigation",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, DenomId: "usd", Amount: 100,
	}); err == nil {
		t.Fatal("expected frozen sender to be refused")
	}
	// Receiver freeze also refuses
	if _, err := f.srv.Unfreeze(f.ctx, &types.MsgUnfreeze{
		Admin: issuerAdmin, IssuerId: "alpha", DenomId: "usd", Account: holderAlice,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Freeze(f.ctx, &types.MsgFreeze{
		Admin: issuerAdmin, IssuerId: "alpha", DenomId: "usd", Account: holderBob, Reason: "kyc",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, DenomId: "usd", Amount: 100,
	}); err == nil {
		t.Fatal("expected frozen receiver to be refused")
	}
}

func TestTransferSanctioned(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)
	f.san.bad[holderBob] = true
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, DenomId: "usd", Amount: 100,
	}); err == nil {
		t.Fatal("expected sanctioned receiver refused")
	}
}

func TestTransferPolicyDenied(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)
	f.pol.deny = true
	f.pol.denyErr = "policy: aml gate failed"
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, DenomId: "usd", Amount: 100,
	}); err == nil {
		t.Fatal("expected policy-denied transfer")
	}
	if f.pol.calls == 0 {
		t.Fatal("policy keeper was never consulted")
	}
}

func TestTransferKYCRequired(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)
	p := f.k.GetParams(f.ctx)
	p.RequireKycCredential = "urn:vc:kyc:passed"
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	// Neither has credential
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, DenomId: "usd", Amount: 100,
	}); err == nil {
		t.Fatal("expected KYC gate to refuse")
	}
	f.did.creds[holderAlice] = map[string]bool{"urn:vc:kyc:passed": true}
	f.did.creds[holderBob] = map[string]bool{"urn:vc:kyc:passed": true}
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, DenomId: "usd", Amount: 100,
	}); err != nil {
		t.Fatalf("expected transfer OK after both KYC, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Allowance flow
// ---------------------------------------------------------------------------

func TestApproveAndTransferFrom(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)
	if _, err := f.srv.Approve(f.ctx, &types.MsgApprove{
		Owner: holderAlice, Spender: holderBob, DenomId: "usd", Amount: 500,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.TransferFrom(f.ctx, &types.MsgTransferFrom{
		Spender: holderBob, From: holderAlice, To: holderCarol, DenomId: "usd", Amount: 300,
	}); err != nil {
		t.Fatal(err)
	}
	if got := f.k.GetAllowance(f.ctx, "usd", holderAlice, holderBob); got != 200 {
		t.Fatalf("allowance=%d", got)
	}
	if got := f.k.GetBalance(f.ctx, "usd", holderCarol); got != 300 {
		t.Fatalf("carol=%d", got)
	}
	// Spending more than approved fails
	if _, err := f.srv.TransferFrom(f.ctx, &types.MsgTransferFrom{
		Spender: holderBob, From: holderAlice, To: holderCarol, DenomId: "usd", Amount: 300,
	}); err == nil {
		t.Fatal("expected over-allowance refused")
	}
}

// ---------------------------------------------------------------------------
// Force ops
// ---------------------------------------------------------------------------

func TestForceTransferBypassesFreezeAndSanctions(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)
	if _, err := f.srv.Freeze(f.ctx, &types.MsgFreeze{
		Admin: issuerAdmin, IssuerId: "alpha", DenomId: "usd", Account: holderAlice, Reason: "x",
	}); err != nil {
		t.Fatal(err)
	}
	f.san.bad[holderAlice] = true
	if _, err := f.srv.ForceTransfer(f.ctx, &types.MsgForceTransfer{
		Admin: issuerAdmin, IssuerId: "alpha", DenomId: "usd",
		From: holderAlice, To: holderBob, Amount: 400, Reason: "court order",
	}); err != nil {
		t.Fatalf("force transfer should bypass freeze/sanctions, got %v", err)
	}
	if got := f.k.GetBalance(f.ctx, "usd", holderBob); got != 400 {
		t.Fatalf("bob=%d", got)
	}
	if len(f.au.calls) == 0 {
		t.Fatal("expected audit hook recorded")
	}
}

func TestForceTransferRevokedIssuerRefused(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)
	if _, err := f.srv.RevokeIssuer(f.ctx, &types.MsgRevokeIssuer{Authority: authority, Id: "alpha"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.ForceTransfer(f.ctx, &types.MsgForceTransfer{
		Admin: issuerAdmin, IssuerId: "alpha", DenomId: "usd",
		From: holderAlice, To: holderBob, Amount: 100, Reason: "any",
	}); err == nil {
		t.Fatal("expected revoked-issuer force refused")
	}
}

func TestNonAdminCannotFreezeOrForceTransfer(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)
	if _, err := f.srv.Freeze(f.ctx, &types.MsgFreeze{
		Admin: otherAddr, IssuerId: "alpha", DenomId: "usd", Account: holderAlice, Reason: "x",
	}); err == nil {
		t.Fatal("expected non-admin freeze refused")
	}
	if _, err := f.srv.ForceTransfer(f.ctx, &types.MsgForceTransfer{
		Admin: otherAddr, IssuerId: "alpha", DenomId: "usd",
		From: holderAlice, To: holderBob, Amount: 100, Reason: "x",
	}); err == nil {
		t.Fatal("expected non-admin force-transfer refused")
	}
}

// ---------------------------------------------------------------------------
// Burn invariants
// ---------------------------------------------------------------------------

func TestBurnShrinksOutstanding(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)
	if _, err := f.srv.Burn(f.ctx, &types.MsgBurn{Holder: holderAlice, DenomId: "usd", Amount: 400}); err != nil {
		t.Fatal(err)
	}
	q, _ := f.k.GetQuota(f.ctx, "alpha", "usd")
	if q.Outstanding != 600 {
		t.Fatalf("outstanding=%d", q.Outstanding)
	}
	if f.k.GetSupply(f.ctx, "usd") != 600 {
		t.Fatalf("supply=%d", f.k.GetSupply(f.ctx, "usd"))
	}
}

func TestBurnFrozenRefused(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)
	if _, err := f.srv.Freeze(f.ctx, &types.MsgFreeze{
		Admin: issuerAdmin, IssuerId: "alpha", DenomId: "usd", Account: holderAlice, Reason: "x",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Burn(f.ctx, &types.MsgBurn{Holder: holderAlice, DenomId: "usd", Amount: 100}); err == nil {
		t.Fatal("expected frozen burn refused")
	}
}

func TestQuotaCeilingCannotDropBelowOutstanding(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 5_000)
	if _, err := f.srv.SetMintQuota(f.ctx, &types.MsgSetMintQuota{
		Authority: authority, IssuerId: "alpha", DenomId: "usd", Ceiling: 1_000,
	}); err == nil {
		t.Fatal("expected ceiling-below-outstanding refusal")
	}
}

// ---------------------------------------------------------------------------
// Redemption
// ---------------------------------------------------------------------------

func TestRedemptionLifecycle(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)

	resp, err := f.srv.RequestRedemption(f.ctx, &types.MsgRequestRedemption{
		Holder: holderAlice, IssuerId: "alpha", DenomId: "usd", Amount: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.k.GetBalance(f.ctx, "usd", holderAlice); got != 600 {
		t.Fatalf("after request alice=%d", got)
	}
	q, _ := f.k.GetQuota(f.ctx, "alpha", "usd")
	if q.Outstanding != 600 {
		t.Fatalf("after request outstanding=%d", q.Outstanding)
	}

	if _, err := f.srv.FulfillRedemption(f.ctx, &types.MsgFulfillRedemption{
		Admin: issuerAdmin, RedemptionId: resp.RedemptionId, PayoutRef: "wire:0xabc",
	}); err != nil {
		t.Fatal(err)
	}
	r, _ := f.k.GetRedemption(f.ctx, resp.RedemptionId)
	if r.Status != types.RedemptionStatus_REDEMPTION_STATUS_FULFILLED {
		t.Fatalf("status=%v", r.Status)
	}
	if r.PayoutRef != "wire:0xabc" {
		t.Fatalf("ref=%q", r.PayoutRef)
	}
}

func TestRedemptionCancelReMints(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)
	resp, err := f.srv.RequestRedemption(f.ctx, &types.MsgRequestRedemption{
		Holder: holderAlice, IssuerId: "alpha", DenomId: "usd", Amount: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.CancelRedemption(f.ctx, &types.MsgCancelRedemption{
		Signer: holderAlice, RedemptionId: resp.RedemptionId, Reason: "changed mind",
	}); err != nil {
		t.Fatal(err)
	}
	if got := f.k.GetBalance(f.ctx, "usd", holderAlice); got != 1_000 {
		t.Fatalf("after cancel alice=%d", got)
	}
	q, _ := f.k.GetQuota(f.ctx, "alpha", "usd")
	if q.Outstanding != 1_000 {
		t.Fatalf("after cancel outstanding=%d", q.Outstanding)
	}
}

// REGRESSION: cancel-redemption MUST succeed even when governance has
// dropped the issuer's quota ceiling between request and cancel.
// Without this exception the holder's burned tokens get stranded in
// the PENDING state with no recovery path.
func TestRedemptionCancelBypassesCeiling(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)
	resp, err := f.srv.RequestRedemption(f.ctx, &types.MsgRequestRedemption{
		Holder: holderAlice, IssuerId: "alpha", DenomId: "usd", Amount: 600,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Outstanding is now 400. Governance lowers ceiling to 500 — well
	// above current outstanding but below outstanding + pending (1000).
	if _, err := f.srv.SetMintQuota(f.ctx, &types.MsgSetMintQuota{
		Authority: authority, IssuerId: "alpha", DenomId: "usd", Ceiling: 500,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.CancelRedemption(f.ctx, &types.MsgCancelRedemption{
		Signer: holderAlice, RedemptionId: resp.RedemptionId, Reason: "changed mind",
	}); err != nil {
		t.Fatalf("cancel must succeed even when re-mint exceeds new ceiling, got %v", err)
	}
	if got := f.k.GetBalance(f.ctx, "usd", holderAlice); got != 1_000 {
		t.Fatalf("alice balance after cancel=%d", got)
	}
}

func TestRedemptionBlacklistedRefused(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)
	if _, err := f.srv.Blacklist(f.ctx, &types.MsgBlacklist{
		Admin: issuerAdmin, IssuerId: "alpha", DenomId: "usd", Account: holderAlice, Reason: "tainted",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.RequestRedemption(f.ctx, &types.MsgRequestRedemption{
		Holder: holderAlice, IssuerId: "alpha", DenomId: "usd", Amount: 100,
	}); err == nil {
		t.Fatal("expected blacklisted holder redemption refused")
	}
}

func TestPendingRedemptionCap(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 1_000_000)
	f.mint(t, holderAlice, 1_000_000)
	p := f.k.GetParams(f.ctx)
	p.MaxRedemptionsPendingPerHolder = 2
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := f.srv.RequestRedemption(f.ctx, &types.MsgRequestRedemption{
			Holder: holderAlice, IssuerId: "alpha", DenomId: "usd", Amount: 1,
		}); err != nil {
			t.Fatalf("req %d: %v", i, err)
		}
	}
	if _, err := f.srv.RequestRedemption(f.ctx, &types.MsgRequestRedemption{
		Holder: holderAlice, IssuerId: "alpha", DenomId: "usd", Amount: 1,
	}); err == nil {
		t.Fatal("expected pending cap to fire")
	}
}

// ---------------------------------------------------------------------------
// Genesis round-trip
// ---------------------------------------------------------------------------

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	f.registerUSD(t, 10_000)
	f.mint(t, holderAlice, 1_000)
	if _, err := f.srv.Approve(f.ctx, &types.MsgApprove{
		Owner: holderAlice, Spender: holderBob, DenomId: "usd", Amount: 500,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Freeze(f.ctx, &types.MsgFreeze{
		Admin: issuerAdmin, IssuerId: "alpha", DenomId: "usd", Account: holderCarol, Reason: "x",
	}); err != nil {
		t.Fatal(err)
	}
	gs := f.k.ExportGenesis(f.ctx)
	if err := gs.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	// Replay into a fresh keeper
	g := setup(t)
	if err := g.k.InitGenesis(g.ctx, *gs); err != nil {
		t.Fatalf("init: %v", err)
	}
	if got := g.k.GetBalance(g.ctx, "usd", holderAlice); got != 1_000 {
		t.Fatalf("re-init alice balance=%d", got)
	}
	if got := g.k.GetAllowance(g.ctx, "usd", holderAlice, holderBob); got != 500 {
		t.Fatalf("re-init allowance=%d", got)
	}
	if got := g.k.GetSupply(g.ctx, "usd"); got != 1_000 {
		t.Fatalf("re-init supply=%d", got)
	}
}

// REGRESSION: GenesisState.Validate must enforce the supply invariant
// sum(outstanding per denom) == supply per denom whenever any quota row
// exists for that denom.
func TestGenesisOutstandingSupplyInvariant(t *testing.T) {
	gs := types.GenesisState{
		Params: types.DefaultParams(),
		Denoms: []types.Denom{
			{Id: "usd", Symbol: "scnUSD", Name: "USD", Decimals: 6, Status: types.DenomStatus_DENOM_STATUS_ACTIVE},
		},
		Issuers: []types.Issuer{
			{Id: "alpha", Did: issuerDID, DisplayName: "A", Status: types.IssuerStatus_ISSUER_STATUS_ACTIVE,
				MintAuthorities: []string{mintAuthA}, Admin: issuerAdmin},
		},
		Quotas: []types.MintQuota{
			{IssuerId: "alpha", DenomId: "usd", Ceiling: 10_000, Outstanding: 500},
		},
		Balances: []types.Balance{
			{DenomId: "usd", Account: holderAlice, Amount: 1_000}, // mismatch: supply=1000, outstanding=500
		},
		Supplies: []types.Supply{
			{DenomId: "usd", TotalSupply: 1_000},
		},
	}
	if err := gs.Validate(); err == nil {
		t.Fatal("expected sum(outstanding) != supply to be rejected")
	}

	// Fix the mismatch and confirm it passes
	gs.Quotas[0].Outstanding = 1_000
	if err := gs.Validate(); err != nil {
		t.Fatalf("expected valid genesis, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Math helpers
// ---------------------------------------------------------------------------

func TestSafeAddSubOverflow(t *testing.T) {
	if _, err := types.SafeAdd(^uint64(0), 1); err == nil {
		t.Fatal("expected overflow")
	}
	if _, err := types.SafeSub(1, 2); err == nil {
		t.Fatal("expected underflow")
	}
}

func TestSafeMulU128(t *testing.T) {
	a, _ := types.SafeMulU128(^uint64(0), 2)
	if a.Hi == 0 {
		t.Fatalf("expected hi != 0 for max*2, got %+v", a)
	}
	b, _ := types.SafeMulU128(100, 200)
	if b.Hi != 0 || b.Lo != 20_000 {
		t.Fatalf("100*200 wrong: %+v", b)
	}
}
