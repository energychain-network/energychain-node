package keeper_test

import (
	"context"
	"testing"

	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/testutil"
	"energychain/x/bridge/keeper"
	"energychain/x/bridge/types"
)

var (
	authority = testutil.DeriveAddr("authority")
	alice     = testutil.DeriveAddr("alice")
	bob       = testutil.DeriveAddr("bob")
	att1      = testutil.DeriveAddr("attestor-1")
	att2      = testutil.DeriveAddr("attestor-2")
	att3      = testutil.DeriveAddr("attestor-3")
	outsider  = testutil.DeriveAddr("outsider")
	denom     = "weusd"
)

// ---- mocks -----------------------------------------------------------------

type mockCompliance struct{ sanctioned map[string]bool }

func (m *mockCompliance) IsSanctioned(_ sdk.Context, a string) bool { return m.sanctioned[a] }

// mockSettlement stands in for x/stableusd. In the mint/burn bridge model the
// bridge issues the native stablecoin on inbound release (BridgeMint) and burns
// it on outbound lock (BridgeBurn); the reserve gate, compliance, and supply
// accounting are x/stableusd's responsibility, modelled here by `blocked`,
// `paused`, and `reserveShort` toggles so the bridge-level logic (quorum, rate
// caps, error propagation) can be exercised in isolation.
type mockSettlement struct {
	denoms       map[string]bool
	paused       map[string]bool // ledger PAUSED: blocks mint, still allows burn (wind-down)
	bal          map[string]uint64
	blocked      map[string]bool // compliance-blocked accounts
	reserveShort bool            // simulate the reserve gate refusing a mint
}

func newSettlement() *mockSettlement {
	return &mockSettlement{
		denoms:  map[string]bool{denom: true},
		paused:  map[string]bool{},
		bal:     map[string]uint64{},
		blocked: map[string]bool{},
	}
}
func (m *mockSettlement) key(d, a string) string                    { return d + "|" + a }
func (m *mockSettlement) HasDenom(_ context.Context, d string) bool { return m.denoms[d] }
func (m *mockSettlement) DenomActive(_ context.Context, d string) bool {
	return m.denoms[d] && !m.paused[d]
}
func (m *mockSettlement) GetBalance(_ context.Context, d, a string) uint64 {
	return m.bal[m.key(d, a)]
}

func (m *mockSettlement) BridgeMint(_ context.Context, d, recipient string, amt uint64) error {
	if amt == 0 {
		return nil
	}
	if !m.denoms[d] {
		return types.ErrSettlement.Wrapf("unknown denom %q", d)
	}
	if m.paused[d] {
		return types.ErrSettlement.Wrapf("denom %q paused", d)
	}
	if m.blocked[recipient] {
		return types.ErrSettlement.Wrapf("%s blocked", recipient)
	}
	if m.reserveShort {
		return types.ErrSettlement.Wrap("reserve coverage insufficient")
	}
	m.bal[m.key(d, recipient)] += amt
	return nil
}

func (m *mockSettlement) BridgeBurn(_ context.Context, d, holder string, amt uint64) error {
	if amt == 0 {
		return nil
	}
	if !m.denoms[d] {
		return types.ErrSettlement.Wrapf("unknown denom %q", d)
	}
	if m.blocked[holder] {
		return types.ErrSettlement.Wrapf("%s blocked", holder)
	}
	if m.bal[m.key(d, holder)] < amt {
		return types.ErrSettlement.Wrap("insufficient")
	}
	m.bal[m.key(d, holder)] -= amt
	return nil
}

func (m *mockSettlement) fund(a string, amt uint64) { m.bal[m.key(denom, a)] += amt }
func (m *mockSettlement) total() uint64 {
	var s uint64
	for _, v := range m.bal {
		s += v
	}
	return s
}

// ---- fixture ---------------------------------------------------------------

type fixture struct {
	ts  *testutil.TestStore
	k   keeper.Keeper
	srv types.MsgServer
	cmp *mockCompliance
	set *mockSettlement
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ts := testutil.NewTestStore(t, types.StoreKey)
	cmp := &mockCompliance{sanctioned: map[string]bool{}}
	set := newSettlement()
	k := keeper.NewKeeper(ts.Cdc, runtime.NewKVStoreService(ts.Key(t, types.StoreKey)), authority, cmp, set)
	if err := k.SetParams(ts.Ctx, types.DefaultParams()); err != nil {
		t.Fatalf("set params: %v", err)
	}
	return &fixture{ts: ts, k: k, srv: keeper.NewMsgServerImpl(k), cmp: cmp, set: set}
}

func (f *fixture) ctx() context.Context { return f.ts.Ctx }
func (f *fixture) bal(a string) uint64  { return f.set.GetBalance(f.ctx(), denom, a) }

func (f *fixture) setParams(t *testing.T, p types.Params) {
	t.Helper()
	if _, err := f.srv.UpdateParams(f.ctx(), &types.MsgUpdateParams{Authority: authority, Params: p}); err != nil {
		t.Fatalf("update params: %v", err)
	}
}

// registerChainAsset registers a chain with the given attestors+threshold and
// a single asset on `denom`, returning (chainID, assetID).
func (f *fixture) registerChainAsset(t *testing.T, attestors []string, threshold uint32) (uint64, uint64) {
	t.Helper()
	cr, err := f.srv.RegisterChain(f.ctx(), &types.MsgRegisterChain{
		Authority: authority, Name: "eth", ChainRef: "ethereum:1", Attestors: attestors, Threshold: threshold,
	})
	if err != nil {
		t.Fatalf("register chain: %v", err)
	}
	ar, err := f.srv.RegisterAsset(f.ctx(), &types.MsgRegisterAsset{Authority: authority, Denom: denom})
	if err != nil {
		t.Fatalf("register asset: %v", err)
	}
	return cr.ChainId, ar.AssetId
}

// release attests an inbound to quorum (single-attestor convenience) and
// releases it, returning the inbound id.
func (f *fixture) attestAndRelease(t *testing.T, chainID, assetID, nonce uint64, recipient string, amount uint64) uint64 {
	t.Helper()
	r, err := f.srv.Attest(f.ctx(), &types.MsgAttest{
		Attestor: att1, SrcChainId: chainID, SrcNonce: nonce, Recipient: recipient, AssetId: assetID, Amount: amount,
	})
	if err != nil {
		t.Fatalf("attest: %v", err)
	}
	if _, err := f.srv.Release(f.ctx(), &types.MsgRelease{Caller: outsider, InboundId: r.InboundId}); err != nil {
		t.Fatalf("release: %v", err)
	}
	return r.InboundId
}

// ---- admin -----------------------------------------------------------------

func TestAdminAuthGated(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.RegisterChain(f.ctx(), &types.MsgRegisterChain{
		Authority: alice, Name: "eth", ChainRef: "ethereum:1", Attestors: []string{att1}, Threshold: 1,
	}); err == nil {
		t.Fatal("non-authority register must fail")
	}
	if _, err := f.srv.RegisterAsset(f.ctx(), &types.MsgRegisterAsset{Authority: alice, Denom: denom}); err == nil {
		t.Fatal("non-authority asset register must fail")
	}
}

func TestRegisterAssetDuplicateDenom(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.RegisterAsset(f.ctx(), &types.MsgRegisterAsset{Authority: authority, Denom: denom}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.RegisterAsset(f.ctx(), &types.MsgRegisterAsset{Authority: authority, Denom: denom}); err == nil {
		t.Fatal("duplicate denom must fail")
	}
}

// ---- lock (outbound burn) --------------------------------------------------

func TestLockBurns(t *testing.T) {
	f := setup(t)
	chainID, assetID := f.registerChainAsset(t, []string{att1}, 1)
	f.set.fund(alice, 1000)
	r, err := f.srv.Lock(f.ctx(), &types.MsgLock{Sender: alice, AssetId: assetID, Amount: 600, DestChainId: chainID, DestAddr: "0xabc"})
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	// 600 burned from alice; nothing escrowed anywhere.
	if f.bal(alice) != 400 || f.set.total() != 400 {
		t.Fatalf("burn wrong: alice=%d total=%d", f.bal(alice), f.set.total())
	}
	o, ok, _ := f.k.GetOutbound(f.ctx(), r.Nonce)
	if !ok || o.Amount != 600 || o.DestChainId != chainID {
		t.Fatalf("outbound record wrong: %+v", o)
	}
}

func TestLockGuards(t *testing.T) {
	f := setup(t)
	chainID, assetID := f.registerChainAsset(t, []string{att1}, 1)
	f.set.fund(alice, 1000)
	// blocked sender (compliance handled in x/stableusd BridgeBurn)
	f.set.blocked[alice] = true
	if _, err := f.srv.Lock(f.ctx(), &types.MsgLock{Sender: alice, AssetId: assetID, Amount: 1, DestChainId: chainID, DestAddr: "x"}); err == nil {
		t.Fatal("blocked sender must fail")
	}
	f.set.blocked[alice] = false
	// zero amount
	if _, err := f.srv.Lock(f.ctx(), &types.MsgLock{Sender: alice, AssetId: assetID, Amount: 0, DestChainId: chainID, DestAddr: "x"}); err == nil {
		t.Fatal("zero amount lock must fail")
	}
	// over per-lock cap
	p := types.DefaultParams()
	p.MaxLockAmount = 100
	f.setParams(t, p)
	if _, err := f.srv.Lock(f.ctx(), &types.MsgLock{Sender: alice, AssetId: assetID, Amount: 101, DestChainId: chainID, DestAddr: "x"}); err == nil {
		t.Fatal("over max_lock_amount must fail")
	}
	f.setParams(t, types.DefaultParams())
	// paused asset
	f.srv.SetAssetStatus(f.ctx(), &types.MsgSetAssetStatus{Authority: authority, AssetId: assetID, Status: types.AssetStatus_ASSET_STATUS_PAUSED})
	if _, err := f.srv.Lock(f.ctx(), &types.MsgLock{Sender: alice, AssetId: assetID, Amount: 1, DestChainId: chainID, DestAddr: "x"}); err == nil {
		t.Fatal("paused asset lock must fail")
	}
	f.srv.SetAssetStatus(f.ctx(), &types.MsgSetAssetStatus{Authority: authority, AssetId: assetID, Status: types.AssetStatus_ASSET_STATUS_ACTIVE})
	// paused chain
	f.srv.SetChainStatus(f.ctx(), &types.MsgSetChainStatus{Authority: authority, ChainId: chainID, Status: types.ChainStatus_CHAIN_STATUS_PAUSED})
	if _, err := f.srv.Lock(f.ctx(), &types.MsgLock{Sender: alice, AssetId: assetID, Amount: 1, DestChainId: chainID, DestAddr: "x"}); err == nil {
		t.Fatal("paused chain lock must fail")
	}
}

// ---- attest + release (inbound mint) ---------------------------------------

func TestInboundQuorumAndRelease(t *testing.T) {
	f := setup(t)
	chainID, assetID := f.registerChainAsset(t, []string{att1, att2, att3}, 2)

	// first attestation -> PENDING
	r1, err := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att1, SrcChainId: chainID, SrcNonce: 7, Recipient: bob, AssetId: assetID, Amount: 500})
	if err != nil {
		t.Fatalf("attest1: %v", err)
	}
	if r1.Status != types.InboundStatus_INBOUND_STATUS_PENDING {
		t.Fatalf("status1=%v want PENDING", r1.Status)
	}
	// same attestor again -> rejected
	if _, err := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att1, SrcChainId: chainID, SrcNonce: 7, Recipient: bob, AssetId: assetID, Amount: 500}); err == nil {
		t.Fatal("double-attest must fail")
	}
	// non-attestor -> rejected
	if _, err := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: outsider, SrcChainId: chainID, SrcNonce: 7, Recipient: bob, AssetId: assetID, Amount: 500}); err == nil {
		t.Fatal("non-attestor must fail")
	}
	// mismatched payload -> rejected
	if _, err := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att2, SrcChainId: chainID, SrcNonce: 7, Recipient: bob, AssetId: assetID, Amount: 999}); err == nil {
		t.Fatal("payload mismatch must fail")
	}
	// second valid attestation -> ATTESTED (quorum 2)
	r2, err := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att2, SrcChainId: chainID, SrcNonce: 7, Recipient: bob, AssetId: assetID, Amount: 500})
	if err != nil {
		t.Fatalf("attest2: %v", err)
	}
	if r2.Status != types.InboundStatus_INBOUND_STATUS_ATTESTED {
		t.Fatalf("status2=%v want ATTESTED", r2.Status)
	}
	// release -> MINTS to recipient
	if _, err := f.srv.Release(f.ctx(), &types.MsgRelease{Caller: outsider, InboundId: r2.InboundId}); err != nil {
		t.Fatalf("release: %v", err)
	}
	if f.bal(bob) != 500 || f.set.total() != 500 {
		t.Fatalf("mint wrong: bob=%d total=%d", f.bal(bob), f.set.total())
	}
	// net bridged outstanding == 500 (minted, nothing burned)
	if got := f.k.EscrowBalance(f.ctx(), denom); got != 500 {
		t.Fatalf("net bridged=%d want 500", got)
	}
	// double release / further attest -> replay rejected
	if _, err := f.srv.Release(f.ctx(), &types.MsgRelease{Caller: outsider, InboundId: r2.InboundId}); err == nil {
		t.Fatal("double release must fail")
	}
	if _, err := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att3, SrcChainId: chainID, SrcNonce: 7, Recipient: bob, AssetId: assetID, Amount: 500}); err == nil {
		t.Fatal("attest after release must fail (replay)")
	}
}

func TestThresholdOneAutoAttested(t *testing.T) {
	f := setup(t)
	chainID, assetID := f.registerChainAsset(t, []string{att1}, 1)
	r, err := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att1, SrcChainId: chainID, SrcNonce: 1, Recipient: bob, AssetId: assetID, Amount: 500})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != types.InboundStatus_INBOUND_STATUS_ATTESTED {
		t.Fatalf("threshold-1 should be immediately ATTESTED, got %v", r.Status)
	}
}

func TestReleaseGuards(t *testing.T) {
	f := setup(t)
	chainID, assetID := f.registerChainAsset(t, []string{att1}, 1)
	r, _ := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att1, SrcChainId: chainID, SrcNonce: 1, Recipient: bob, AssetId: assetID, Amount: 500})
	// reserve gate short -> mint refused, fail closed
	f.set.reserveShort = true
	if _, err := f.srv.Release(f.ctx(), &types.MsgRelease{Caller: outsider, InboundId: r.InboundId}); err == nil {
		t.Fatal("release must fail when reserve coverage is short")
	}
	f.set.reserveShort = false
	// blocked recipient -> refused even with reserve
	f.set.blocked[bob] = true
	if _, err := f.srv.Release(f.ctx(), &types.MsgRelease{Caller: outsider, InboundId: r.InboundId}); err == nil {
		t.Fatal("blocked recipient release must fail")
	}
	// clears -> mints
	f.set.blocked[bob] = false
	if _, err := f.srv.Release(f.ctx(), &types.MsgRelease{Caller: outsider, InboundId: r.InboundId}); err != nil {
		t.Fatalf("release after clear: %v", err)
	}
	if f.bal(bob) != 500 {
		t.Fatalf("bob=%d want 500", f.bal(bob))
	}
}

func TestReleaseRequiresAttested(t *testing.T) {
	f := setup(t)
	chainID, assetID := f.registerChainAsset(t, []string{att1, att2}, 2)
	r, _ := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att1, SrcChainId: chainID, SrcNonce: 1, Recipient: bob, AssetId: assetID, Amount: 500})
	// still PENDING (1 of 2)
	if _, err := f.srv.Release(f.ctx(), &types.MsgRelease{Caller: outsider, InboundId: r.InboundId}); err == nil {
		t.Fatal("release of pending inbound must fail")
	}
}

// ---- mint rate limits (defense-in-depth) -----------------------------------

// TestMintPerTxCap bounds a single release: even an attested inbound cannot mint
// more than max_mint_per_tx, capping the per-event loss on a quorum compromise.
func TestMintPerTxCap(t *testing.T) {
	f := setup(t)
	chainID, assetID := f.registerChainAsset(t, []string{att1}, 1)
	p := types.DefaultParams()
	p.MaxMintPerTx = 100
	f.setParams(t, p)
	// 101 > cap -> rejected
	r, _ := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att1, SrcChainId: chainID, SrcNonce: 1, Recipient: bob, AssetId: assetID, Amount: 101})
	if _, err := f.srv.Release(f.ctx(), &types.MsgRelease{Caller: outsider, InboundId: r.InboundId}); err == nil {
		t.Fatal("release over max_mint_per_tx must fail")
	}
	// 100 == cap -> allowed
	r2, _ := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att1, SrcChainId: chainID, SrcNonce: 2, Recipient: bob, AssetId: assetID, Amount: 100})
	if _, err := f.srv.Release(f.ctx(), &types.MsgRelease{Caller: outsider, InboundId: r2.InboundId}); err != nil {
		t.Fatalf("release at cap: %v", err)
	}
}

// TestMintWindowCapPerChain bounds the total minted per source chain within a
// rolling window, and proves the budget is segregated per chain: exhausting
// chain A's window must not block chain B.
func TestMintWindowCapPerChain(t *testing.T) {
	f := setup(t)
	crA, _ := f.srv.RegisterChain(f.ctx(), &types.MsgRegisterChain{Authority: authority, Name: "A", ChainRef: "a:1", Attestors: []string{att1}, Threshold: 1})
	crB, _ := f.srv.RegisterChain(f.ctx(), &types.MsgRegisterChain{Authority: authority, Name: "B", ChainRef: "b:1", Attestors: []string{att2}, Threshold: 1})
	ar, _ := f.srv.RegisterAsset(f.ctx(), &types.MsgRegisterAsset{Authority: authority, Denom: denom})
	p := types.DefaultParams()
	p.MintWindowSeconds = 3600
	p.MaxMintPerWindow = 1000
	f.setParams(t, p)

	// chain A mints 600 then 400 == 1000 (at cap)
	f.attestAndRelease(t, crA.ChainId, ar.AssetId, 1, bob, 600)
	f.attestAndRelease(t, crA.ChainId, ar.AssetId, 2, bob, 400)
	// chain A's next mint (any amount) exceeds the window -> rejected
	rA, _ := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att1, SrcChainId: crA.ChainId, SrcNonce: 3, Recipient: bob, AssetId: ar.AssetId, Amount: 1})
	if _, err := f.srv.Release(f.ctx(), &types.MsgRelease{Caller: outsider, InboundId: rA.InboundId}); err == nil {
		t.Fatal("chain A over window cap must fail")
	}
	// chain B still has its own full budget
	rB, _ := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att2, SrcChainId: crB.ChainId, SrcNonce: 1, Recipient: bob, AssetId: ar.AssetId, Amount: 1000})
	if _, err := f.srv.Release(f.ctx(), &types.MsgRelease{Caller: outsider, InboundId: rB.InboundId}); err != nil {
		t.Fatalf("chain B within its own window must succeed: %v", err)
	}
}

func TestGlobalPauseBlocks(t *testing.T) {
	f := setup(t)
	chainID, assetID := f.registerChainAsset(t, []string{att1}, 1)
	f.set.fund(alice, 1000)
	p := types.DefaultParams()
	p.Paused = true
	f.setParams(t, p)
	if _, err := f.srv.Lock(f.ctx(), &types.MsgLock{Sender: alice, AssetId: assetID, Amount: 1, DestChainId: chainID, DestAddr: "x"}); err == nil {
		t.Fatal("lock must be blocked when paused")
	}
	if _, err := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att1, SrcChainId: chainID, SrcNonce: 1, Recipient: bob, AssetId: assetID, Amount: 1}); err == nil {
		t.Fatal("attest must be blocked when paused")
	}
}

// TestNetBridgedTracksMintMinusBurn checks the EscrowBalance query reports the
// bridge-issued circulating amount: minted via releases minus burned via locks.
func TestNetBridgedTracksMintMinusBurn(t *testing.T) {
	f := setup(t)
	chainID, assetID := f.registerChainAsset(t, []string{att1}, 1)
	// release 500 -> bob holds 500, net bridged 500
	f.attestAndRelease(t, chainID, assetID, 1, bob, 500)
	if got := f.k.EscrowBalance(f.ctx(), denom); got != 500 {
		t.Fatalf("net bridged=%d want 500", got)
	}
	// bob locks 200 back out (burn) -> net bridged 300
	if _, err := f.srv.Lock(f.ctx(), &types.MsgLock{Sender: bob, AssetId: assetID, Amount: 200, DestChainId: chainID, DestAddr: "x"}); err != nil {
		t.Fatalf("lock: %v", err)
	}
	if got := f.k.EscrowBalance(f.ctx(), denom); got != 300 {
		t.Fatalf("net bridged=%d want 300", got)
	}
	if f.bal(bob) != 300 {
		t.Fatalf("bob=%d want 300", f.bal(bob))
	}
}

// TestStaleAttestorDoesNotCountQuorum is the regression for rotated-out
// attestors: after UpdateChain removes a signer, its stale signature must not
// keep counting toward quorum at release time.
func TestStaleAttestorDoesNotCountQuorum(t *testing.T) {
	f := setup(t)
	chainID, assetID := f.registerChainAsset(t, []string{att1, att2}, 2)
	// reach quorum 2/2
	f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att1, SrcChainId: chainID, SrcNonce: 1, Recipient: bob, AssetId: assetID, Amount: 500})
	r, _ := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att2, SrcChainId: chainID, SrcNonce: 1, Recipient: bob, AssetId: assetID, Amount: 500})
	if r.Status != types.InboundStatus_INBOUND_STATUS_ATTESTED {
		t.Fatalf("want ATTESTED got %v", r.Status)
	}
	// governance rotates att2 out (compromised); threshold stays 2, set now {att1, att3}
	if _, err := f.srv.UpdateChain(f.ctx(), &types.MsgUpdateChain{Authority: authority, ChainId: chainID, Attestors: []string{att1, att3}, Threshold: 2}); err != nil {
		t.Fatal(err)
	}
	// release must now fail: only att1 is a current member (1 < 2)
	if _, err := f.srv.Release(f.ctx(), &types.MsgRelease{Caller: outsider, InboundId: r.InboundId}); err == nil {
		t.Fatal("release must fail after attestor rotation drops below quorum")
	}
	// a current member re-attesting restores quorum
	if _, err := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att3, SrcChainId: chainID, SrcNonce: 1, Recipient: bob, AssetId: assetID, Amount: 500}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Release(f.ctx(), &types.MsgRelease{Caller: outsider, InboundId: r.InboundId}); err != nil {
		t.Fatalf("release after restoring quorum: %v", err)
	}
}

// TestThresholdDropUnsticks is the regression for a governance threshold drop:
// release recomputes quorum live, so a PENDING inbound with enough current
// signatures becomes releasable without further attestations.
func TestThresholdDropUnsticks(t *testing.T) {
	f := setup(t)
	chainID, assetID := f.registerChainAsset(t, []string{att1, att2, att3}, 3)
	// only 2 of 3 attest -> PENDING
	f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att1, SrcChainId: chainID, SrcNonce: 1, Recipient: bob, AssetId: assetID, Amount: 500})
	r, _ := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att2, SrcChainId: chainID, SrcNonce: 1, Recipient: bob, AssetId: assetID, Amount: 500})
	if r.Status != types.InboundStatus_INBOUND_STATUS_PENDING {
		t.Fatalf("want PENDING got %v", r.Status)
	}
	// governance lowers threshold to 2
	if _, err := f.srv.UpdateChain(f.ctx(), &types.MsgUpdateChain{Authority: authority, ChainId: chainID, Attestors: []string{att1, att2, att3}, Threshold: 2}); err != nil {
		t.Fatal(err)
	}
	// release now succeeds even though status is still PENDING and no new attest arrived
	if _, err := f.srv.Release(f.ctx(), &types.MsgRelease{Caller: outsider, InboundId: r.InboundId}); err != nil {
		t.Fatalf("release after threshold drop should succeed: %v", err)
	}
	if f.bal(bob) != 500 {
		t.Fatalf("bob=%d want 500", f.bal(bob))
	}
}

// TestPausedDenomBlocksMint is the regression for honoring the x/stableusd denom
// pause on the inbound mint leg: no new issuance while the ledger denom is
// PAUSED, but outbound burns (wind-down withdrawals) are still allowed.
func TestPausedDenomBlocksMint(t *testing.T) {
	f := setup(t)
	chainID, assetID := f.registerChainAsset(t, []string{att1}, 1)
	r, _ := f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att1, SrcChainId: chainID, SrcNonce: 1, Recipient: bob, AssetId: assetID, Amount: 500})
	f.set.paused[denom] = true
	if _, err := f.srv.Release(f.ctx(), &types.MsgRelease{Caller: outsider, InboundId: r.InboundId}); err == nil {
		t.Fatal("release (mint) must fail when settlement denom is paused")
	}
	// burns still allowed during wind-down
	f.set.fund(alice, 100)
	if _, err := f.srv.Lock(f.ctx(), &types.MsgLock{Sender: alice, AssetId: assetID, Amount: 50, DestChainId: chainID, DestAddr: "x"}); err != nil {
		t.Fatalf("lock during pause should succeed (wind-down): %v", err)
	}
	f.set.paused[denom] = false
}

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	chainID, assetID := f.registerChainAsset(t, []string{att1, att2}, 2)
	f.set.fund(alice, 1000)
	f.srv.Lock(f.ctx(), &types.MsgLock{Sender: alice, AssetId: assetID, Amount: 700, DestChainId: chainID, DestAddr: "x"})
	// a pending inbound (1 of 2)
	f.srv.Attest(f.ctx(), &types.MsgAttest{Attestor: att1, SrcChainId: chainID, SrcNonce: 9, Recipient: bob, AssetId: assetID, Amount: 200})

	gs, err := f.k.ExportGenesis(f.ctx())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}
	g := setup(t)
	if err := g.k.InitGenesis(g.ctx(), gs); err != nil {
		t.Fatalf("init: %v", err)
	}
	gs2, err := g.k.ExportGenesis(g.ctx())
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if len(gs2.Chains) != 1 || len(gs2.Assets) != 1 || len(gs2.Outbounds) != 1 || len(gs2.Inbounds) != 1 {
		t.Fatalf("roundtrip mismatch: %+v", gs2)
	}
	if gs2.OutboundNonceSeq != gs.OutboundNonceSeq || gs2.InboundIdSeq != gs.InboundIdSeq {
		t.Fatal("sequence mismatch")
	}
	// replay protection survives roundtrip: the source key is indexed
	if _, err := g.srv.Attest(g.ctx(), &types.MsgAttest{Attestor: att2, SrcChainId: chainID, SrcNonce: 9, Recipient: bob, AssetId: assetID, Amount: 200}); err != nil {
		t.Fatalf("second attest after genesis should accumulate, got %v", err)
	}
}
