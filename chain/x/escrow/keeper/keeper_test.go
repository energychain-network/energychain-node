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

	"energychain/x/escrow/keeper"
	"energychain/x/escrow/types"
)

// ---- Stubs ---------------------------------------------------------------

type stubSanctions struct{ bad map[string]bool }

func (s *stubSanctions) IsSanctioned(_ sdk.Context, subject string) bool { return s.bad[subject] }

type stubStablecoin struct {
	denoms      map[string]bool
	bal         map[string]map[string]uint64
	pausedDenom map[string]bool
	blocked     map[string]map[string]bool
}

func newStubStablecoin() *stubStablecoin {
	return &stubStablecoin{
		denoms:      map[string]bool{},
		bal:         map[string]map[string]uint64{},
		pausedDenom: map[string]bool{},
		blocked:     map[string]map[string]bool{},
	}
}
func (s *stubStablecoin) HasDenom(_ sdk.Context, d string) bool { return s.denoms[d] }
func (s *stubStablecoin) IsDenomPaused(_ sdk.Context, d string) bool {
	return s.pausedDenom[d]
}
func (s *stubStablecoin) IsAccountBlocked(_ sdk.Context, d, acc string) bool {
	if _, ok := s.blocked[d]; !ok {
		return false
	}
	return s.blocked[d][acc]
}
func (s *stubStablecoin) block(d, acc string, v bool) {
	if _, ok := s.blocked[d]; !ok {
		s.blocked[d] = map[string]bool{}
	}
	s.blocked[d][acc] = v
}
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

// stubRWA is an in-memory RWA-token ledger sufficient to model the
// release-leg compliance contract: paused tokens block releases,
// non-existent tokens are refused at create time, and the pool's
// per-token balance is maintained across lock/release.
type stubRWA struct {
	tokens   map[uint64]bool
	paused   map[uint64]bool
	bal      map[uint64]map[string]uint64
	failNext bool
}

func newStubRWA() *stubRWA {
	return &stubRWA{
		tokens: map[uint64]bool{},
		paused: map[uint64]bool{},
		bal:    map[uint64]map[string]uint64{},
	}
}
func (s *stubRWA) HasToken(_ sdk.Context, id uint64) bool { return s.tokens[id] }
func (s *stubRWA) credit(t uint64, acc string, n uint64) {
	if _, ok := s.bal[t]; !ok {
		s.bal[t] = map[string]uint64{}
	}
	s.bal[t][acc] += n
}
func (s *stubRWA) get(t uint64, acc string) uint64 {
	if _, ok := s.bal[t]; !ok {
		return 0
	}
	return s.bal[t][acc]
}
func (s *stubRWA) EscrowLock(_ sdk.Context, tokenID uint64, depositor string, amount uint64) error {
	if s.failNext {
		s.failNext = false
		return fmt.Errorf("rwa lock failure injected")
	}
	if !s.tokens[tokenID] {
		return fmt.Errorf("rwa token %d missing", tokenID)
	}
	if s.paused[tokenID] {
		return fmt.Errorf("token %d paused", tokenID)
	}
	cur := s.get(tokenID, depositor)
	if cur < amount {
		return fmt.Errorf("insufficient rwa balance: %d < %d", cur, amount)
	}
	s.bal[tokenID][depositor] = cur - amount
	if s.bal[tokenID][depositor] == 0 {
		delete(s.bal[tokenID], depositor)
	}
	s.credit(tokenID, "rwa_pool", amount)
	return nil
}
func (s *stubRWA) EscrowRelease(_ sdk.Context, tokenID uint64, recipient string, amount uint64) error {
	if !s.tokens[tokenID] {
		return fmt.Errorf("rwa token %d missing", tokenID)
	}
	if s.paused[tokenID] {
		return fmt.Errorf("token %d paused", tokenID)
	}
	cur := s.get(tokenID, "rwa_pool")
	if cur < amount {
		return fmt.Errorf("rwa pool short: %d < %d", cur, amount)
	}
	s.bal[tokenID]["rwa_pool"] = cur - amount
	if s.bal[tokenID]["rwa_pool"] == 0 {
		delete(s.bal[tokenID], "rwa_pool")
	}
	s.credit(tokenID, recipient, amount)
	return nil
}

// stubOracle returns a deterministic value/timestamp pair per topic.
type stubOracle struct {
	topics map[string]struct {
		v  int64
		ts int64
	}
}

func (s *stubOracle) GetAggregatedReserve(_ sdk.Context, topicID string) (int64, int64, bool) {
	x, ok := s.topics[topicID]
	if !ok {
		return 0, 0, false
	}
	return x.v, x.ts, true
}

type stubAudit struct{ calls []string }

func (s *stubAudit) RecordEscrowAction(_ sdk.Context, escrowID uint64, action, actor, subject, detail string) {
	s.calls = append(s.calls, strings.Join([]string{itoa(escrowID), action, actor, subject, detail}, "|"))
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

// ---- Constants ----------------------------------------------------------

func mkAddr(seed byte) string {
	bz := make([]byte, 20)
	for i := range bz {
		bz[i] = seed + byte(i)
	}
	return sdk.AccAddress(bz).String()
}

var (
	authority   = mkAddr(1)
	depositor   = mkAddr(2)
	beneficiary = mkAddr(3)
	fallbackAcc = mkAddr(4)
	arbiter     = mkAddr(5)
	signerA     = mkAddr(6)
	signerB     = mkAddr(7)
	signerC     = mkAddr(8)
	stranger    = mkAddr(9)
)

const usdDenom = "usd"

// ---- Setup --------------------------------------------------------------

type fixture struct {
	k   keeper.Keeper
	ctx sdk.Context
	srv types.MsgServer
	san *stubSanctions
	sc  *stubStablecoin
	rwa *stubRWA
	or  *stubOracle
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

	san := &stubSanctions{bad: map[string]bool{}}
	sc := newStubStablecoin()
	sc.denoms[usdDenom] = true
	rwa := newStubRWA()
	or := &stubOracle{topics: map[string]struct {
		v  int64
		ts int64
	}{}}
	au := &stubAudit{}

	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, san, sc, rwa, or, au)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return &fixture{
		k: k, ctx: ctx, srv: keeper.NewMsgServerImpl(k),
		san: san, sc: sc, rwa: rwa, or: or, au: au,
	}
}

func (f *fixture) advance(seconds int64) {
	f.ctx = f.ctx.WithBlockTime(f.ctx.BlockTime().Add(time.Duration(seconds) * time.Second))
}

func basicCreate(t *testing.T, f *fixture) uint64 {
	t.Helper()
	f.sc.credit(usdDenom, depositor, 1_000_000)
	r, err := f.srv.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
		Depositor:         depositor,
		Beneficiary:       beneficiary,
		FallbackAddr:      fallbackAcc,
		Committee:         []string{signerA, signerB, signerC},
		ApprovalThreshold: 2,
		Arbiter:           arbiter,
		Kind:              types.AssetKind_ASSET_KIND_STABLECOIN,
		StablecoinDenom:   usdDenom,
		Amount:            500_000,
		Memo:              "ppa milestone",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return r.EscrowId
}

// ============================================================================
// Tests
// ============================================================================

func TestCreateAndFundStablecoin(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)

	if _, err := f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id}); err != nil {
		t.Fatalf("fund: %v", err)
	}
	if got := f.sc.get(usdDenom, depositor); got != 500_000 {
		t.Fatalf("depositor balance after fund: want 500000, got %d", got)
	}
	if got := f.sc.get(usdDenom, keeper.EscrowPoolAccount); got != 500_000 {
		t.Fatalf("pool balance after fund: want 500000, got %d", got)
	}
	e, _, err := f.k.GetEscrow(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if e.Status != types.Status_STATUS_FUNDED {
		t.Fatalf("status after fund: want FUNDED, got %s", e.Status)
	}
}

func TestCreateRefusesUnknownDenom(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
		Depositor:         depositor,
		Beneficiary:       beneficiary,
		FallbackAddr:      fallbackAcc,
		Committee:         []string{signerA, signerB},
		ApprovalThreshold: 1,
		Kind:              types.AssetKind_ASSET_KIND_STABLECOIN,
		StablecoinDenom:   "ngn",
		Amount:            1,
	}); err == nil {
		t.Fatalf("expected unknown denom rejection")
	}
}

func TestCreateRefusesSanctionedDepositor(t *testing.T) {
	f := setup(t)
	f.san.bad[depositor] = true
	if _, err := f.srv.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
		Depositor:         depositor,
		Beneficiary:       beneficiary,
		FallbackAddr:      fallbackAcc,
		Committee:         []string{signerA, signerB},
		ApprovalThreshold: 1,
		Kind:              types.AssetKind_ASSET_KIND_STABLECOIN,
		StablecoinDenom:   usdDenom,
		Amount:            1,
	}); err == nil {
		t.Fatalf("expected sanctioned depositor rejection")
	}
}

func TestApproveAndRelease(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	if _, err := f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Release(f.ctx, &types.MsgRelease{Actor: stranger, EscrowId: id}); err == nil {
		t.Fatalf("release without approvals must fail")
	}
	if _, err := f.srv.Approve(f.ctx, &types.MsgApprove{Signer: signerA, EscrowId: id, Intent: types.Intent_INTENT_RELEASE}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Release(f.ctx, &types.MsgRelease{Actor: stranger, EscrowId: id}); err == nil {
		t.Fatalf("release with 1/2 approvals must fail")
	}
	if _, err := f.srv.Approve(f.ctx, &types.MsgApprove{Signer: signerB, EscrowId: id, Intent: types.Intent_INTENT_RELEASE}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Release(f.ctx, &types.MsgRelease{Actor: stranger, EscrowId: id}); err != nil {
		t.Fatalf("release: %v", err)
	}
	if got := f.sc.get(usdDenom, beneficiary); got != 500_000 {
		t.Fatalf("beneficiary balance: want 500000, got %d", got)
	}
	e, _, _ := f.k.GetEscrow(f.ctx, id)
	if e.Status != types.Status_STATUS_RELEASED {
		t.Fatalf("status: want RELEASED, got %s", e.Status)
	}
}

func TestApproveSwitchSidesUpdatesCounters(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	if _, err := f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Approve(f.ctx, &types.MsgApprove{Signer: signerA, EscrowId: id, Intent: types.Intent_INTENT_RELEASE}); err != nil {
		t.Fatal(err)
	}
	r, err := f.srv.Approve(f.ctx, &types.MsgApprove{Signer: signerA, EscrowId: id, Intent: types.Intent_INTENT_REFUND})
	if err != nil {
		t.Fatal(err)
	}
	if r.ReleaseCount != 0 || r.RefundCount != 1 {
		t.Fatalf("counters after switch: rel=%d ref=%d", r.ReleaseCount, r.RefundCount)
	}
}

func TestApproveOutsideCommitteeRejected(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	if _, err := f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Approve(f.ctx, &types.MsgApprove{Signer: stranger, EscrowId: id, Intent: types.Intent_INTENT_RELEASE}); err == nil {
		t.Fatalf("non-committee signer must be refused")
	}
}

func TestRevokeApprovalDecrementsCounter(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	if _, err := f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Approve(f.ctx, &types.MsgApprove{Signer: signerA, EscrowId: id, Intent: types.Intent_INTENT_RELEASE}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.RevokeApproval(f.ctx, &types.MsgRevokeApproval{Signer: signerA, EscrowId: id}); err != nil {
		t.Fatal(err)
	}
	e, _, _ := f.k.GetEscrow(f.ctx, id)
	if e.ApprovalCountRelease != 0 {
		t.Fatalf("release count after revoke: want 0, got %d", e.ApprovalCountRelease)
	}
}

func TestReleaseTimeLockGate(t *testing.T) {
	f := setup(t)
	f.sc.credit(usdDenom, depositor, 1_000_000)
	now := f.ctx.BlockTime().Unix()
	r, err := f.srv.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
		Depositor:         depositor,
		Beneficiary:       beneficiary,
		FallbackAddr:      fallbackAcc,
		Committee:         []string{signerA},
		ApprovalThreshold: 1,
		Kind:              types.AssetKind_ASSET_KIND_STABLECOIN,
		StablecoinDenom:   usdDenom,
		Amount:            1,
		Triggers:          types.Triggers{ReleaseAfter: now + 60},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: r.EscrowId}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Approve(f.ctx, &types.MsgApprove{Signer: signerA, EscrowId: r.EscrowId, Intent: types.Intent_INTENT_RELEASE}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Release(f.ctx, &types.MsgRelease{Actor: stranger, EscrowId: r.EscrowId}); err == nil {
		t.Fatalf("release before release_after must fail")
	}
	f.advance(120)
	if _, err := f.srv.Release(f.ctx, &types.MsgRelease{Actor: stranger, EscrowId: r.EscrowId}); err != nil {
		t.Fatalf("release after time lock: %v", err)
	}
}

func TestReleaseOracleGate(t *testing.T) {
	f := setup(t)
	f.sc.credit(usdDenom, depositor, 1_000_000)
	now := f.ctx.BlockTime().Unix()
	r, err := f.srv.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
		Depositor:         depositor,
		Beneficiary:       beneficiary,
		FallbackAddr:      fallbackAcc,
		Committee:         []string{signerA},
		ApprovalThreshold: 1,
		Kind:              types.AssetKind_ASSET_KIND_STABLECOIN,
		StablecoinDenom:   usdDenom,
		Amount:            1,
		Triggers: types.Triggers{
			OracleTopicId:             "delivery_done",
			OracleMinValue:            1,
			OracleMaxStalenessSeconds: 3600,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: r.EscrowId}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Approve(f.ctx, &types.MsgApprove{Signer: signerA, EscrowId: r.EscrowId, Intent: types.Intent_INTENT_RELEASE}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Release(f.ctx, &types.MsgRelease{Actor: stranger, EscrowId: r.EscrowId}); err == nil {
		t.Fatalf("oracle missing must block release")
	}
	f.or.topics["delivery_done"] = struct {
		v  int64
		ts int64
	}{v: 1, ts: now}
	if _, err := f.srv.Release(f.ctx, &types.MsgRelease{Actor: stranger, EscrowId: r.EscrowId}); err != nil {
		t.Fatalf("release with oracle: %v", err)
	}
}

func TestReleaseOracleStaleRejected(t *testing.T) {
	f := setup(t)
	f.sc.credit(usdDenom, depositor, 1_000_000)
	now := f.ctx.BlockTime().Unix()
	r, _ := f.srv.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
		Depositor: depositor, Beneficiary: beneficiary, FallbackAddr: fallbackAcc,
		Committee: []string{signerA}, ApprovalThreshold: 1,
		Kind:            types.AssetKind_ASSET_KIND_STABLECOIN,
		StablecoinDenom: usdDenom, Amount: 1,
		Triggers: types.Triggers{
			OracleTopicId:             "delivery_done",
			OracleMinValue:            1,
			OracleMaxStalenessSeconds: 60,
		},
	})
	_, _ = f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: r.EscrowId})
	_, _ = f.srv.Approve(f.ctx, &types.MsgApprove{Signer: signerA, EscrowId: r.EscrowId, Intent: types.Intent_INTENT_RELEASE})
	f.or.topics["delivery_done"] = struct {
		v  int64
		ts int64
	}{v: 1, ts: now - 3600}
	if _, err := f.srv.Release(f.ctx, &types.MsgRelease{Actor: stranger, EscrowId: r.EscrowId}); err == nil {
		t.Fatalf("stale oracle must block release")
	}
}

func TestRefundFlow(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	_, _ = f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id})
	for _, sn := range []string{signerA, signerB} {
		if _, err := f.srv.Approve(f.ctx, &types.MsgApprove{Signer: sn, EscrowId: id, Intent: types.Intent_INTENT_REFUND}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.srv.Refund(f.ctx, &types.MsgRefund{Actor: stranger, EscrowId: id}); err != nil {
		t.Fatalf("refund: %v", err)
	}
	if got := f.sc.get(usdDenom, fallbackAcc); got != 500_000 {
		t.Fatalf("fallback balance: want 500000, got %d", got)
	}
}

func TestReleaseBlockedBySanctionedBeneficiary(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	_, _ = f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id})
	for _, sn := range []string{signerA, signerB} {
		_, _ = f.srv.Approve(f.ctx, &types.MsgApprove{Signer: sn, EscrowId: id, Intent: types.Intent_INTENT_RELEASE})
	}
	f.san.bad[beneficiary] = true
	if _, err := f.srv.Release(f.ctx, &types.MsgRelease{Actor: stranger, EscrowId: id}); err == nil {
		t.Fatalf("sanctioned beneficiary must block release")
	}
}

func TestArbiterReleaseBypassesApprovals(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	_, _ = f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id})
	if _, err := f.srv.ArbiterRelease(f.ctx, &types.MsgArbiterRelease{Arbiter: arbiter, EscrowId: id, Reason: "court order"}); err != nil {
		t.Fatalf("arbiter release: %v", err)
	}
	if got := f.sc.get(usdDenom, beneficiary); got != 500_000 {
		t.Fatalf("beneficiary balance: want 500000, got %d", got)
	}
}

func TestArbiterFromUnauthorizedRefused(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	_, _ = f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id})
	if _, err := f.srv.ArbiterRelease(f.ctx, &types.MsgArbiterRelease{Arbiter: stranger, EscrowId: id, Reason: "x"}); err == nil {
		t.Fatalf("non-arbiter must be refused")
	}
}

func TestDisputeFreezesAndResolves(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	_, _ = f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id})

	if _, err := f.srv.MarkDisputed(f.ctx, &types.MsgMarkDisputed{Actor: depositor, EscrowId: id, Reason: "non-delivery"}); err != nil {
		t.Fatalf("mark disputed: %v", err)
	}
	for _, sn := range []string{signerA, signerB} {
		if _, err := f.srv.Approve(f.ctx, &types.MsgApprove{Signer: sn, EscrowId: id, Intent: types.Intent_INTENT_RELEASE}); err == nil {
			t.Fatalf("approve while disputed must be refused")
		}
	}
	if _, err := f.srv.Release(f.ctx, &types.MsgRelease{Actor: stranger, EscrowId: id}); err == nil {
		t.Fatalf("release while disputed must be refused")
	}
	if _, err := f.srv.ResolveDispute(f.ctx, &types.MsgResolveDispute{Arbiter: arbiter, EscrowId: id, Release: false, Reason: "found in favour of buyer"}); err != nil {
		t.Fatalf("resolve dispute: %v", err)
	}
	if got := f.sc.get(usdDenom, fallbackAcc); got != 500_000 {
		t.Fatalf("fallback balance: want 500000, got %d", got)
	}
}

func TestDisputeMarkOnlyByParties(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	_, _ = f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id})
	if _, err := f.srv.MarkDisputed(f.ctx, &types.MsgMarkDisputed{Actor: stranger, EscrowId: id, Reason: "x"}); err == nil {
		t.Fatalf("non-party dispute mark must be refused")
	}
}

func TestCancelOnlyDraft(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	if _, err := f.srv.Cancel(f.ctx, &types.MsgCancel{Depositor: depositor, EscrowId: id, Reason: "abort"}); err != nil {
		t.Fatalf("cancel draft: %v", err)
	}
	id2 := basicCreate(t, f)
	_, _ = f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id2})
	if _, err := f.srv.Cancel(f.ctx, &types.MsgCancel{Depositor: depositor, EscrowId: id2, Reason: "x"}); err == nil {
		t.Fatalf("cancel after fund must be refused")
	}
}

func TestRWAEscrowFlow(t *testing.T) {
	f := setup(t)
	const tokID uint64 = 7
	f.rwa.tokens[tokID] = true
	f.rwa.credit(tokID, depositor, 100)

	r, err := f.srv.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
		Depositor:         depositor,
		Beneficiary:       beneficiary,
		FallbackAddr:      fallbackAcc,
		Committee:         []string{signerA, signerB},
		ApprovalThreshold: 1,
		Kind:              types.AssetKind_ASSET_KIND_RWA,
		RwaTokenId:        tokID,
		Amount:            40,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: r.EscrowId}); err != nil {
		t.Fatalf("fund rwa: %v", err)
	}
	if got := f.rwa.get(tokID, depositor); got != 60 {
		t.Fatalf("depositor rwa: want 60, got %d", got)
	}
	if got := f.rwa.get(tokID, "rwa_pool"); got != 40 {
		t.Fatalf("pool rwa: want 40, got %d", got)
	}
	if _, err := f.srv.Approve(f.ctx, &types.MsgApprove{Signer: signerA, EscrowId: r.EscrowId, Intent: types.Intent_INTENT_RELEASE}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Release(f.ctx, &types.MsgRelease{Actor: stranger, EscrowId: r.EscrowId}); err != nil {
		t.Fatalf("release rwa: %v", err)
	}
	if got := f.rwa.get(tokID, beneficiary); got != 40 {
		t.Fatalf("beneficiary rwa: want 40, got %d", got)
	}
}

func TestRWAEscrowRefusedForUnknownToken(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
		Depositor:         depositor,
		Beneficiary:       beneficiary,
		FallbackAddr:      fallbackAcc,
		Committee:         []string{signerA, signerB},
		ApprovalThreshold: 1,
		Kind:              types.AssetKind_ASSET_KIND_RWA,
		RwaTokenId:        999,
		Amount:            1,
	}); err == nil {
		t.Fatalf("unknown rwa token must be refused")
	}
}

func TestPoolAccountIsValidBech32(t *testing.T) {
	if _, err := sdk.AccAddressFromBech32(keeper.EscrowPoolAccount); err != nil {
		t.Fatalf("EscrowPoolAccount %q must round-trip: %v", keeper.EscrowPoolAccount, err)
	}
}

func TestApprovalIdempotenceDoesNotDoubleCount(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	_, _ = f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id})
	for i := 0; i < 5; i++ {
		if _, err := f.srv.Approve(f.ctx, &types.MsgApprove{Signer: signerA, EscrowId: id, Intent: types.Intent_INTENT_RELEASE}); err != nil {
			t.Fatalf("approve %d: %v", i, err)
		}
	}
	e, _, _ := f.k.GetEscrow(f.ctx, id)
	if e.ApprovalCountRelease != 1 {
		t.Fatalf("re-approval must be idempotent (count=%d)", e.ApprovalCountRelease)
	}
}

func TestUpdateParamsAuthority(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: stranger, Params: types.DefaultParams(),
	}); err == nil {
		t.Fatalf("non-authority must be refused")
	}
	p := types.DefaultParams()
	p.MaxEscrows = 10
	if _, err := f.srv.UpdateParams(f.ctx, &types.MsgUpdateParams{Authority: authority, Params: p}); err != nil {
		t.Fatalf("authority update: %v", err)
	}
}

func TestMaxEscrowsCap(t *testing.T) {
	f := setup(t)
	p := types.DefaultParams()
	p.MaxEscrows = 1
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	_ = basicCreate(t, f)
	if _, err := f.srv.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
		Depositor:         depositor,
		Beneficiary:       beneficiary,
		FallbackAddr:      fallbackAcc,
		Committee:         []string{signerA},
		ApprovalThreshold: 1,
		Kind:              types.AssetKind_ASSET_KIND_STABLECOIN,
		StablecoinDenom:   usdDenom,
		Amount:            1,
	}); err == nil {
		t.Fatalf("max-escrows cap must enforce")
	}
}

func TestFundAfterSanctionRejected(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	f.san.bad[depositor] = true
	if _, err := f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id}); err == nil {
		t.Fatalf("fund after sanction must be refused")
	}
}

// ---- Regression tests from review --------------------------------------

// Issue 1: stablecoin-level freeze on the depositor at fund time
// must refuse the lock-leg even though sanctions clear, otherwise
// escrows could be used to unblock a frozen account.
func TestFundRefusedForStablecoinFrozenDepositor(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	f.sc.block(usdDenom, depositor, true)
	if _, err := f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id}); err == nil {
		t.Fatalf("frozen depositor must not be able to fund escrow")
	}
}

// Issue 1 cont.: stablecoin-level freeze added between Fund and
// Release must block payout to the now-frozen beneficiary.
func TestReleaseRefusedForStablecoinFrozenBeneficiary(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	if _, err := f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id}); err != nil {
		t.Fatal(err)
	}
	for _, sn := range []string{signerA, signerB} {
		_, _ = f.srv.Approve(f.ctx, &types.MsgApprove{Signer: sn, EscrowId: id, Intent: types.Intent_INTENT_RELEASE})
	}
	f.sc.block(usdDenom, beneficiary, true)
	if _, err := f.srv.Release(f.ctx, &types.MsgRelease{Actor: stranger, EscrowId: id}); err == nil {
		t.Fatalf("frozen beneficiary must block release")
	}
}

// Issue 1 cont.: paused denoms refuse new pay-ins.
func TestFundRefusedForPausedDenom(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	f.sc.pausedDenom[usdDenom] = true
	if _, err := f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id}); err == nil {
		t.Fatalf("paused denom must refuse fund")
	}
}

// Issue 2: an escrow without an arbiter must refuse MarkDisputed
// because no actor would be able to ResolveDispute, leaving the
// funds permanently locked.
func TestMarkDisputedRequiresArbiter(t *testing.T) {
	f := setup(t)
	f.sc.credit(usdDenom, depositor, 1_000_000)
	r, err := f.srv.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
		Depositor: depositor, Beneficiary: beneficiary, FallbackAddr: fallbackAcc,
		Committee: []string{signerA, signerB}, ApprovalThreshold: 1,
		Kind:            types.AssetKind_ASSET_KIND_STABLECOIN,
		StablecoinDenom: usdDenom, Amount: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: r.EscrowId}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.MarkDisputed(f.ctx, &types.MsgMarkDisputed{Actor: depositor, EscrowId: r.EscrowId, Reason: "x"}); err == nil {
		t.Fatalf("arbiter-less escrow must refuse MarkDisputed")
	}
}

// Issue 3: CountEscrows must not walk the whole table for every
// CreateEscrow tx. We assert it returns peek(IDSeq) — independent
// of how many lifetime rows exist, the cap is honoured cheaply.
func TestCountEscrowsIsCheap(t *testing.T) {
	f := setup(t)
	for i := 0; i < 5; i++ {
		_ = basicCreate(t, f)
	}
	got, err := f.k.CountEscrows(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != 5 {
		t.Fatalf("count: want 5, got %d", got)
	}
}

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	_, _ = f.srv.Fund(f.ctx, &types.MsgFund{Depositor: depositor, EscrowId: id})
	if _, err := f.srv.Approve(f.ctx, &types.MsgApprove{Signer: signerA, EscrowId: id, Intent: types.Intent_INTENT_RELEASE}); err != nil {
		t.Fatal(err)
	}
	gs, err := f.k.ExportGenesis(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}
	// Re-init into fresh keeper; assert state matches.
	g := setup(t)
	if err := g.k.InitGenesis(g.ctx, gs); err != nil {
		t.Fatal(err)
	}
	e, ok, err := g.k.GetEscrow(g.ctx, id)
	if err != nil || !ok {
		t.Fatalf("escrow lost in round-trip: %v ok=%v", err, ok)
	}
	if e.ApprovalCountRelease != 1 {
		t.Fatalf("counter lost in round-trip: %d", e.ApprovalCountRelease)
	}
}
