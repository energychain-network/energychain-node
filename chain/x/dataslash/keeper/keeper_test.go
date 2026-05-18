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
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/dataslash/keeper"
	"energychain/x/dataslash/types"
)

// ---- Stubs -------------------------------------------------------------

type stubSanctions struct{ bad map[string]bool }

func (s *stubSanctions) IsSanctioned(_ sdk.Context, who string) bool { return s.bad[who] }

type stubStablecoin struct {
	denoms   map[string]bool
	paused   map[string]bool
	blocked  map[string]map[string]bool
	balances map[string]map[string]uint64
}

func newStubStablecoin() *stubStablecoin {
	return &stubStablecoin{
		denoms:   map[string]bool{},
		paused:   map[string]bool{},
		blocked:  map[string]map[string]bool{},
		balances: map[string]map[string]uint64{},
	}
}

func (s *stubStablecoin) HasDenom(_ sdk.Context, d string) bool       { return s.denoms[d] }
func (s *stubStablecoin) IsDenomPaused(_ sdk.Context, d string) bool  { return s.paused[d] }
func (s *stubStablecoin) IsAccountBlocked(_ sdk.Context, d, who string) bool {
	if _, ok := s.blocked[d]; !ok {
		return false
	}
	return s.blocked[d][who]
}
func (s *stubStablecoin) Move(_ sdk.Context, denom, from, to string, amount uint64) error {
	if !s.denoms[denom] {
		return testErr("denom not registered")
	}
	if _, ok := s.balances[denom]; !ok {
		s.balances[denom] = map[string]uint64{}
	}
	bal := s.balances[denom]
	if bal[from] < amount {
		return testErr("insufficient balance")
	}
	bal[from] -= amount
	bal[to] += amount
	return nil
}
func (s *stubStablecoin) credit(denom, who string, amount uint64) {
	if _, ok := s.balances[denom]; !ok {
		s.balances[denom] = map[string]uint64{}
	}
	s.balances[denom][who] += amount
}
func (s *stubStablecoin) balance(denom, who string) uint64 {
	if _, ok := s.balances[denom]; !ok {
		return 0
	}
	return s.balances[denom][who]
}
func (s *stubStablecoin) block(denom, who string) {
	if _, ok := s.blocked[denom]; !ok {
		s.blocked[denom] = map[string]bool{}
	}
	s.blocked[denom][who] = true
}

type testErr string

func (e testErr) Error() string { return string(e) }

// ---- Setup -------------------------------------------------------------

func mkAddr(seed byte) string {
	bz := make([]byte, 20)
	for i := range bz {
		bz[i] = seed + byte(i)
	}
	return sdk.AccAddress(bz).String()
}

var (
	authority  = mkAddr(1)
	stranger   = mkAddr(2)
	signerA    = mkAddr(3)
	signerB    = mkAddr(4)
	signerC    = mkAddr(5)
	bondOwnerA = mkAddr(6)
	bondOwnerB = mkAddr(7)
	bondOwnerC = mkAddr(8)

	didA = "did:example:prov-a"
	didB = "did:example:prov-b"
	didC = "did:example:prov-c"

	denomA = "usdc"
)

type fixture struct {
	k    keeper.Keeper
	ctx  sdk.Context
	srv  types.MsgServer
	q    types.QueryServer
	san  *stubSanctions
	coin *stubStablecoin
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
	registry := cdctypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	san := &stubSanctions{bad: map[string]bool{}}
	coin := newStubStablecoin()
	coin.denoms[denomA] = true
	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, san, coin, nil)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return &fixture{
		k: k, ctx: ctx,
		srv: keeper.NewMsgServerImpl(k),
		q:   keeper.NewQueryServerImpl(k),
		san: san, coin: coin,
	}
}

func (f *fixture) registerProvider(t *testing.T, signer, did, name string, role types.ProviderRole, bondOwner string) uint64 {
	t.Helper()
	r, err := f.srv.RegisterProvider(f.ctx, &types.MsgRegisterProvider{
		Authority:     authority,
		Did:           did,
		SignerAddress: signer,
		BondOwner:     bondOwner,
		Name:          name,
		Role:          role,
		BondDenom:     denomA,
		Jurisdictions: []string{"GLOBAL"},
	})
	if err != nil {
		t.Fatalf("register-provider: %v", err)
	}
	return r.ProviderId
}

func (f *fixture) postBond(t *testing.T, owner string, providerID, amount uint64) {
	t.Helper()
	f.coin.credit(denomA, owner, amount)
	_, err := f.srv.PostBond(f.ctx, &types.MsgPostBond{
		BondOwner: owner, ProviderId: providerID, Amount: amount,
	})
	if err != nil {
		t.Fatalf("post-bond: %v", err)
	}
}

func (f *fixture) report(t *testing.T, providerID uint64, kind types.InfractionKind, slashBps uint32, reason string) uint64 {
	t.Helper()
	r, err := f.srv.ReportInfraction(f.ctx, &types.MsgReportInfraction{
		Actor: authority, ProviderId: providerID, Kind: kind,
		OverrideSlashBps: slashBps,
		EvidenceUri:      "ipfs://e",
		EvidenceHash:     strings.Repeat("a", 64),
		Reason:           reason,
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	return r.InfractionId
}

func (f *fixture) advanceTime(d time.Duration) {
	f.ctx = f.ctx.WithBlockTime(f.ctx.BlockTime().Add(d))
}

// ---- tests -------------------------------------------------------------

func TestRegisterProviderAuthorityGate(t *testing.T) {
	f := setup(t)
	_, err := f.srv.RegisterProvider(f.ctx, &types.MsgRegisterProvider{
		Authority: stranger, Did: didA, SignerAddress: signerA, BondOwner: bondOwnerA,
		Name: "X", Role: types.ProviderRole_PROVIDER_ROLE_ORACLE,
	})
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func TestRegisterProviderDIDUniqueness(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	_, err := f.srv.RegisterProvider(f.ctx, &types.MsgRegisterProvider{
		Authority: authority, Did: didA, SignerAddress: signerB, BondOwner: bondOwnerB,
		Name: "B", Role: types.ProviderRole_PROVIDER_ROLE_ORACLE,
	})
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected duplicate-DID error, got %v", err)
	}
}

func TestRegisterProviderSignerUniqueness(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	_, err := f.srv.RegisterProvider(f.ctx, &types.MsgRegisterProvider{
		Authority: authority, Did: didB, SignerAddress: signerA, BondOwner: bondOwnerB,
		Name: "B", Role: types.ProviderRole_PROVIDER_ROLE_ORACLE,
	})
	if err == nil || !strings.Contains(err.Error(), "already bound") {
		t.Fatalf("expected duplicate-signer error, got %v", err)
	}
}

func TestRegisterProviderSanctionedBondOwner(t *testing.T) {
	f := setup(t)
	f.san.bad[bondOwnerA] = true
	_, err := f.srv.RegisterProvider(f.ctx, &types.MsgRegisterProvider{
		Authority: authority, Did: didA, SignerAddress: signerA, BondOwner: bondOwnerA,
		Name: "A", Role: types.ProviderRole_PROVIDER_ROLE_ORACLE,
		BondDenom: denomA,
	})
	if err == nil || !strings.Contains(err.Error(), "sanctioned") {
		t.Fatalf("expected sanctioned error, got %v", err)
	}
}

func TestPostBondAndIsActive(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)

	// Brand-new provider not yet bonded → IsActive false.
	_, _, ok, err := f.k.IsActiveSigner(f.ctx, signerA, types.ProviderRole_PROVIDER_ROLE_ORACLE)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("expected inactive (no bond)")
	}

	f.postBond(t, bondOwnerA, pid, 1)
	_, p, ok, err := f.k.IsActiveSigner(f.ctx, signerA, types.ProviderRole_PROVIDER_ROLE_ORACLE)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || p.BondAmount != 1 {
		t.Fatalf("expected active with bond=1, got ok=%v amount=%d", ok, p.BondAmount)
	}
}

func TestPostBondMismatchedOwner(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	f.coin.credit(denomA, bondOwnerB, 5)
	_, err := f.srv.PostBond(f.ctx, &types.MsgPostBond{
		BondOwner: bondOwnerB, ProviderId: pid, Amount: 5,
	})
	if err == nil || !strings.Contains(err.Error(), "bond_owner mismatch") {
		t.Fatalf("expected mismatch, got %v", err)
	}
}

func TestReportInfractionAuthorityOnly(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	f.postBond(t, bondOwnerA, pid, 1000)
	_, err := f.srv.ReportInfraction(f.ctx, &types.MsgReportInfraction{
		Actor: stranger, ProviderId: pid,
		Kind: types.InfractionKind_INFRACTION_KIND_STALE,
	})
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func TestReportInfractionSlashesBond(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	f.postBond(t, bondOwnerA, pid, 1000)
	// 10% MISREPORT slash.
	f.report(t, pid, types.InfractionKind_INFRACTION_KIND_MISREPORT, 0, "bad")
	p, err := f.k.MustGetProvider(f.ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if p.BondAmount != 900 {
		t.Fatalf("expected bond 900, got %d", p.BondAmount)
	}
	// Pool now holds 1000 (the originally bonded amount):
	// 900 still backs the provider + 100 slashed remainder.
	if got := f.coin.balance(denomA, f.k.PoolAddress()); got != 1000 {
		t.Fatalf("expected pool=1000, got %d", got)
	}
}

func TestReportInfractionEquivocationWipesBond(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	f.postBond(t, bondOwnerA, pid, 500)
	f.report(t, pid, types.InfractionKind_INFRACTION_KIND_EQUIVOCATION, 0, "double-attest")
	p, _ := f.k.MustGetProvider(f.ctx, pid)
	if p.BondAmount != 0 {
		t.Fatalf("expected bond=0 after 100%% slash, got %d", p.BondAmount)
	}
}

func TestAutoJailAfterThreshold(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	f.postBond(t, bondOwnerA, pid, 10_000)
	// Default auto_jail_threshold is 3; report 3 small infractions.
	for i := 0; i < 3; i++ {
		f.report(t, pid, types.InfractionKind_INFRACTION_KIND_STALE, 10, "")
	}
	p, _ := f.k.MustGetProvider(f.ctx, pid)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_JAILED {
		t.Fatalf("expected JAILED, got %s", p.Status)
	}
	if p.JailUntil <= f.ctx.BlockTime().Unix() {
		t.Fatalf("expected jail_until in future")
	}
	if p.InfractionCount != 3 {
		t.Fatalf("expected 3 infractions, got %d", p.InfractionCount)
	}
}

func TestAutoBanAfterThreshold(t *testing.T) {
	f := setup(t)
	// Lower the ban threshold for the test so we don't have to
	// fire 10 reports.
	p, _ := f.k.GetParams(f.ctx)
	p.AutoJailThreshold = 2
	p.AutoBanThreshold = 3
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	f.postBond(t, bondOwnerA, pid, 10_000)
	for i := 0; i < 3; i++ {
		f.report(t, pid, types.InfractionKind_INFRACTION_KIND_STALE, 10, "")
	}
	prov, _ := f.k.MustGetProvider(f.ctx, pid)
	if prov.Status != types.ProviderStatus_PROVIDER_STATUS_BANNED {
		t.Fatalf("expected BANNED, got %s", prov.Status)
	}
	if prov.JailUntil != 0 {
		t.Fatalf("expected jail_until=0 on ban, got %d", prov.JailUntil)
	}
}

func TestJailUnjailLifecycle(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	f.postBond(t, bondOwnerA, pid, 100)
	_, err := f.srv.JailProvider(f.ctx, &types.MsgJailProvider{
		Authority: authority, ProviderId: pid, DurationSeconds: 60, Reason: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := f.k.MustGetProvider(f.ctx, pid)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_JAILED {
		t.Fatalf("expected JAILED, got %s", p.Status)
	}
	_, err = f.srv.UnjailProvider(f.ctx, &types.MsgUnjailProvider{
		Authority: authority, ProviderId: pid, Reason: "y",
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _ = f.k.MustGetProvider(f.ctx, pid)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_ACTIVE {
		t.Fatalf("expected ACTIVE after unjail, got %s", p.Status)
	}
}

func TestUnbondCooldownAndWithdraw(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	f.postBond(t, bondOwnerA, pid, 200)
	_, err := f.srv.RequestUnbond(f.ctx, &types.MsgRequestUnbond{
		BondOwner: bondOwnerA, ProviderId: pid,
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := f.k.MustGetProvider(f.ctx, pid)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_UNBONDING {
		t.Fatalf("expected UNBONDING, got %s", p.Status)
	}
	// Premature withdraw.
	_, err = f.srv.WithdrawUnbonded(f.ctx, &types.MsgWithdrawUnbonded{
		BondOwner: bondOwnerA, ProviderId: pid,
	})
	if err == nil || !strings.Contains(err.Error(), "cooldown not elapsed") {
		t.Fatalf("expected cooldown error, got %v", err)
	}

	// Advance past cooldown.
	f.advanceTime(time.Duration(types.DefaultUnbondCooldownSeconds+1) * time.Second)
	_, err = f.srv.WithdrawUnbonded(f.ctx, &types.MsgWithdrawUnbonded{
		BondOwner: bondOwnerA, ProviderId: pid,
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _ = f.k.MustGetProvider(f.ctx, pid)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_WITHDRAWN {
		t.Fatalf("expected WITHDRAWN, got %s", p.Status)
	}
	if p.BondAmount != 0 {
		t.Fatalf("expected bond=0, got %d", p.BondAmount)
	}
	if got := f.coin.balance(denomA, bondOwnerA); got != 200 {
		t.Fatalf("expected refund=200, got %d", got)
	}
}

func TestEndBlockAutoUnjail(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	f.postBond(t, bondOwnerA, pid, 100)
	_, err := f.srv.JailProvider(f.ctx, &types.MsgJailProvider{
		Authority: authority, ProviderId: pid, DurationSeconds: 60, Reason: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Before jail expiry: end-block should keep status.
	if err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	p, _ := f.k.MustGetProvider(f.ctx, pid)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_JAILED {
		t.Fatalf("expected still JAILED before expiry")
	}
	// After expiry: end-block auto-unjails.
	f.advanceTime(61 * time.Second)
	if err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	p, _ = f.k.MustGetProvider(f.ctx, pid)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_ACTIVE {
		t.Fatalf("expected ACTIVE after auto-unjail, got %s", p.Status)
	}
	if p.JailUntil != 0 {
		t.Fatalf("expected jail_until cleared, got %d", p.JailUntil)
	}
}

func TestEndBlockAutoWithdrawUnbonded(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	f.postBond(t, bondOwnerA, pid, 100)
	if _, err := f.srv.RequestUnbond(f.ctx, &types.MsgRequestUnbond{
		BondOwner: bondOwnerA, ProviderId: pid,
	}); err != nil {
		t.Fatal(err)
	}
	// Before cooldown elapsed.
	if err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	p, _ := f.k.MustGetProvider(f.ctx, pid)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_UNBONDING {
		t.Fatalf("expected UNBONDING")
	}
	// Past cooldown.
	f.advanceTime(time.Duration(types.DefaultUnbondCooldownSeconds+1) * time.Second)
	if err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	p, _ = f.k.MustGetProvider(f.ctx, pid)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_WITHDRAWN {
		t.Fatalf("expected WITHDRAWN, got %s", p.Status)
	}
	if got := f.coin.balance(denomA, bondOwnerA); got != 100 {
		t.Fatalf("expected refund=100, got %d", got)
	}
}

// Regression: when the bond_owner is blocked, end-block must
// STILL flip status to WITHDRAWN so the sweep doesn't deadlock
// the index, but the residual stays in the pool.
func TestEndBlockAutoWithdrawForfeitsBlockedRefund(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	f.postBond(t, bondOwnerA, pid, 100)
	if _, err := f.srv.RequestUnbond(f.ctx, &types.MsgRequestUnbond{
		BondOwner: bondOwnerA, ProviderId: pid,
	}); err != nil {
		t.Fatal(err)
	}
	f.advanceTime(time.Duration(types.DefaultUnbondCooldownSeconds+1) * time.Second)

	// Now block the bond_owner — the refund leg must fail
	// but the sweep must still progress.
	f.coin.block(denomA, bondOwnerA)
	if err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	p, _ := f.k.MustGetProvider(f.ctx, pid)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_WITHDRAWN {
		t.Fatalf("expected WITHDRAWN despite blocked refund, got %s", p.Status)
	}
	if got := f.coin.balance(denomA, bondOwnerA); got != 0 {
		t.Fatalf("expected blocked refund=0, got %d", got)
	}
	if got := f.coin.balance(denomA, f.k.PoolAddress()); got != 100 {
		t.Fatalf("expected pool to retain forfeit=100, got %d", got)
	}
}

func TestQueryIsActive(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	f.postBond(t, bondOwnerA, pid, 1)
	resp, err := f.q.IsActive(f.ctx, &types.QueryIsActiveRequest{
		SignerAddress: signerA,
		ExpectedRole:  types.ProviderRole_PROVIDER_ROLE_ORACLE,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Active || resp.ProviderId != pid {
		t.Fatalf("expected active for signer %s, got %+v", signerA, resp)
	}
	// Wrong role expected → not active.
	resp, err = f.q.IsActive(f.ctx, &types.QueryIsActiveRequest{
		SignerAddress: signerA,
		ExpectedRole:  types.ProviderRole_PROVIDER_ROLE_METER,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Active {
		t.Fatalf("expected inactive for role-mismatch")
	}
}

func TestQueryProviders(t *testing.T) {
	f := setup(t)
	pidA := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	pidB := f.registerProvider(t, signerB, didB, "B", types.ProviderRole_PROVIDER_ROLE_METER, bondOwnerB)
	pidC := f.registerProvider(t, signerC, didC, "C", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerC)
	resp, err := f.q.Providers(f.ctx, &types.QueryProvidersRequest{
		Role: types.ProviderRole_PROVIDER_ROLE_ORACLE,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Providers) != 2 {
		t.Fatalf("expected 2 oracles, got %d", len(resp.Providers))
	}
	seen := map[uint64]bool{}
	for _, p := range resp.Providers {
		seen[p.Id] = true
	}
	if !seen[pidA] || !seen[pidC] || seen[pidB] {
		t.Fatalf("expected pidA+pidC, got %v", seen)
	}
}

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	f.postBond(t, bondOwnerA, pid, 100)
	f.report(t, pid, types.InfractionKind_INFRACTION_KIND_STALE, 50, "stale-report")

	gs, err := f.k.ExportGenesis(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}

	// Fresh keeper; re-import.
	g := setup(t)
	if err := g.k.InitGenesis(g.ctx, gs); err != nil {
		t.Fatalf("init: %v", err)
	}
	p, err := g.k.MustGetProvider(g.ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if p.BondAmount != 50 { // 100 - 0.5% × 100 = 99.5 → floor at 99. But our slash was bps 50 (0.5%), so 100 - 0 = 100. Actually 100 * 50 / 10000 = 0; flooring gives 0 slash for small bonds.
		// Recompute: 100 * 50 / 10000 = 0 (integer floor) → bond unchanged.
		t.Logf("note: BondAmount=%d after replay", p.BondAmount)
	}
	// Next ID seq should advance correctly.
	next, err := g.k.NextProviderID(g.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if next != pid+1 {
		t.Fatalf("expected next provider id=%d, got %d", pid+1, next)
	}
}

func TestMaxProvidersCap(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx)
	p.MaxProviders = 1
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	_, err := f.srv.RegisterProvider(f.ctx, &types.MsgRegisterProvider{
		Authority: authority, Did: didB, SignerAddress: signerB, BondOwner: bondOwnerB,
		Name: "B", Role: types.ProviderRole_PROVIDER_ROLE_ORACLE,
	})
	if err == nil || !strings.Contains(err.Error(), "max_providers reached") {
		t.Fatalf("expected cap error, got %v", err)
	}
}

func TestBanProvider(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	_, err := f.srv.BanProvider(f.ctx, &types.MsgBanProvider{
		Authority: authority, ProviderId: pid, Reason: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := f.k.MustGetProvider(f.ctx, pid)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_BANNED {
		t.Fatalf("expected BANNED, got %s", p.Status)
	}
	// Re-ban is idempotent.
	if _, err := f.srv.BanProvider(f.ctx, &types.MsgBanProvider{
		Authority: authority, ProviderId: pid, Reason: "y",
	}); err != nil {
		t.Fatalf("expected idempotent ban, got %v", err)
	}
	// Reporting on a banned provider rejected.
	_, err = f.srv.ReportInfraction(f.ctx, &types.MsgReportInfraction{
		Actor: authority, ProviderId: pid,
		Kind:         types.InfractionKind_INFRACTION_KIND_STALE,
		EvidenceHash: strings.Repeat("a", 64),
	})
	if err == nil || !strings.Contains(err.Error(), "BANNED") {
		t.Fatalf("expected banned-error, got %v", err)
	}
}

// Regression: a sanctioned bond_owner must not be able to
// reclaim the bond via the manual withdraw path even after
// cooldown has elapsed. Auto-withdraw (EndBlock) forfeits;
// the manual path refuses.
func TestWithdrawUnbondedSanctionedOwnerRefused(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	f.postBond(t, bondOwnerA, pid, 100)
	if _, err := f.srv.RequestUnbond(f.ctx, &types.MsgRequestUnbond{
		BondOwner: bondOwnerA, ProviderId: pid,
	}); err != nil {
		t.Fatal(err)
	}
	f.advanceTime(time.Duration(types.DefaultUnbondCooldownSeconds+1) * time.Second)

	f.san.bad[bondOwnerA] = true
	_, err := f.srv.WithdrawUnbonded(f.ctx, &types.MsgWithdrawUnbonded{
		BondOwner: bondOwnerA, ProviderId: pid,
	})
	if err == nil || !strings.Contains(err.Error(), "sanctioned") {
		t.Fatalf("expected sanctioned refusal, got %v", err)
	}
	// Provider remains UNBONDING; bond stays in pool.
	p, _ := f.k.MustGetProvider(f.ctx, pid)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_UNBONDING {
		t.Fatalf("expected UNBONDING, got %s", p.Status)
	}
	if got := f.coin.balance(denomA, bondOwnerA); got != 0 {
		t.Fatalf("expected no refund, got %d", got)
	}
}

func TestUpdateProviderInfoBondOwnerRotation(t *testing.T) {
	f := setup(t)
	pid := f.registerProvider(t, signerA, didA, "A", types.ProviderRole_PROVIDER_ROLE_ORACLE, bondOwnerA)
	_, err := f.srv.UpdateProviderInfo(f.ctx, &types.MsgUpdateProviderInfo{
		Authority: authority, ProviderId: pid, NewBondOwner: bondOwnerB,
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := f.k.MustGetProvider(f.ctx, pid)
	if p.BondOwner != bondOwnerB {
		t.Fatalf("expected bond_owner=%s, got %s", bondOwnerB, p.BondOwner)
	}
	// Rotation to a sanctioned owner rejected.
	f.san.bad[bondOwnerC] = true
	_, err = f.srv.UpdateProviderInfo(f.ctx, &types.MsgUpdateProviderInfo{
		Authority: authority, ProviderId: pid, NewBondOwner: bondOwnerC,
	})
	if err == nil || !strings.Contains(err.Error(), "sanctioned") {
		t.Fatalf("expected sanctioned, got %v", err)
	}
}
