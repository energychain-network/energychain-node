package keeper_test

import (
	"fmt"
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

	"energychain/x/streampay/keeper"
	"energychain/x/streampay/types"
)

// ---- Stubs ---------------------------------------------------------------

type stubSanctions struct{ bad map[string]bool }

func (s *stubSanctions) IsSanctioned(_ sdk.Context, who string) bool { return s.bad[who] }

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
func (s *stubStablecoin) HasDenom(_ sdk.Context, d string) bool      { return s.denoms[d] }
func (s *stubStablecoin) IsDenomPaused(_ sdk.Context, d string) bool { return s.pausedDenom[d] }
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

// ---- Setup --------------------------------------------------------------

func mkAddr(seed byte) string {
	bz := make([]byte, 20)
	for i := range bz {
		bz[i] = seed + byte(i)
	}
	return sdk.AccAddress(bz).String()
}

var (
	authority   = mkAddr(1)
	sender      = mkAddr(2)
	receiver    = mkAddr(3)
	receiver2   = mkAddr(4)
	stranger    = mkAddr(5)
	sender2     = mkAddr(6)
	usdDenom    = "usd"
)

type fixture struct {
	k   keeper.Keeper
	ctx sdk.Context
	srv types.MsgServer
	san *stubSanctions
	sc  *stubStablecoin
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
	sc := newStubStablecoin()
	sc.denoms[usdDenom] = true
	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, san, sc, nil)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return &fixture{k: k, ctx: ctx, srv: keeper.NewMsgServerImpl(k), san: san, sc: sc}
}

func (f *fixture) advance(seconds int64) {
	f.ctx = f.ctx.WithBlockTime(f.ctx.BlockTime().Add(time.Duration(seconds) * time.Second))
}

func basicCreate(t *testing.T, f *fixture, rate uint64, deposit uint64, endTime int64) uint64 {
	t.Helper()
	f.sc.credit(usdDenom, sender, deposit*2)
	r, err := f.srv.CreateStream(f.ctx, &types.MsgCreateStream{
		Sender:        sender,
		Receiver:      receiver,
		Denom:         usdDenom,
		RatePerSecond: rate,
		Deposit:       deposit,
		EndTime:       endTime,
		Memo:          "test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return r.StreamId
}

// ============================================================================
// Tests
// ============================================================================

func TestCreateFundsPool(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	if got := f.sc.get(usdDenom, keeper.PoolAddress()); got != 1_000 {
		t.Fatalf("pool: want 1000, got %d", got)
	}
	st, _, _ := f.k.GetStream(f.ctx, id)
	if st.Status != types.Status_STATUS_ACTIVE {
		t.Fatalf("status: %s", st.Status)
	}
}

func TestCreateRefusesSenderEqReceiver(t *testing.T) {
	f := setup(t)
	f.sc.credit(usdDenom, sender, 1_000)
	if _, err := f.srv.CreateStream(f.ctx, &types.MsgCreateStream{
		Sender: sender, Receiver: sender, Denom: usdDenom,
		RatePerSecond: 10, Deposit: 100,
	}); err == nil {
		t.Fatal("expected sender==receiver rejection")
	}
}

func TestCreateRefusesSanctionedParty(t *testing.T) {
	f := setup(t)
	f.san.bad[receiver] = true
	f.sc.credit(usdDenom, sender, 1_000)
	if _, err := f.srv.CreateStream(f.ctx, &types.MsgCreateStream{
		Sender: sender, Receiver: receiver, Denom: usdDenom,
		RatePerSecond: 10, Deposit: 100,
	}); err == nil {
		t.Fatal("expected sanctioned receiver rejection")
	}
}

func TestCreateRefusesPausedDenom(t *testing.T) {
	f := setup(t)
	f.sc.pausedDenom[usdDenom] = true
	f.sc.credit(usdDenom, sender, 1_000)
	if _, err := f.srv.CreateStream(f.ctx, &types.MsgCreateStream{
		Sender: sender, Receiver: receiver, Denom: usdDenom,
		RatePerSecond: 10, Deposit: 100,
	}); err == nil {
		t.Fatal("expected paused-denom rejection")
	}
}

func TestCreateRefusesBlockedSender(t *testing.T) {
	f := setup(t)
	f.sc.block(usdDenom, sender, true)
	f.sc.credit(usdDenom, sender, 1_000)
	if _, err := f.srv.CreateStream(f.ctx, &types.MsgCreateStream{
		Sender: sender, Receiver: receiver, Denom: usdDenom,
		RatePerSecond: 10, Deposit: 100,
	}); err == nil {
		t.Fatal("expected blocked sender rejection")
	}
}

func TestWithdrawAccrues(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	f.advance(30) // 30s * 10/s = 300
	r, err := f.srv.Withdraw(f.ctx, &types.MsgWithdraw{Receiver: receiver, StreamId: id})
	if err != nil {
		t.Fatal(err)
	}
	if r.Withdrawn != 300 {
		t.Fatalf("withdrawn: want 300, got %d", r.Withdrawn)
	}
	if got := f.sc.get(usdDenom, receiver); got != 300 {
		t.Fatalf("receiver bal: %d", got)
	}
	if got := f.sc.get(usdDenom, keeper.PoolAddress()); got != 700 {
		t.Fatalf("pool bal: %d", got)
	}
}

func TestWithdrawCapsToDeposit(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 100, 0)
	f.advance(1_000) // way more than deposit/rate
	r, err := f.srv.Withdraw(f.ctx, &types.MsgWithdraw{Receiver: receiver, StreamId: id})
	if err != nil {
		t.Fatal(err)
	}
	if r.Withdrawn != 100 {
		t.Fatalf("withdrawn (capped): want 100, got %d", r.Withdrawn)
	}
}

func TestWithdrawRefusesNonReceiver(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	f.advance(30)
	if _, err := f.srv.Withdraw(f.ctx, &types.MsgWithdraw{Receiver: stranger, StreamId: id}); err == nil {
		t.Fatal("expected non-receiver rejection")
	}
}

func TestWithdrawRefusesSanctionedReceiver(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	f.advance(30)
	f.san.bad[receiver] = true
	if _, err := f.srv.Withdraw(f.ctx, &types.MsgWithdraw{Receiver: receiver, StreamId: id}); err == nil {
		t.Fatal("expected sanctioned-receiver rejection")
	}
}

func TestPauseFreezesAccrual(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	f.advance(30)
	if _, err := f.srv.Pause(f.ctx, &types.MsgPause{Sender: sender, StreamId: id, Reason: "x"}); err != nil {
		t.Fatal(err)
	}
	// Advance 100s while paused.
	f.advance(100)
	// Withdrawable should still be exactly what accrued before pause (300).
	st, _, _ := f.k.GetStream(f.ctx, id)
	if got := types.Withdrawable(st, f.ctx.BlockTime().Unix()); got != 300 {
		t.Fatalf("withdrawable during pause: want 300, got %d", got)
	}
}

func TestResumeRestoresAccrual(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	f.advance(30)
	if _, err := f.srv.Pause(f.ctx, &types.MsgPause{Sender: sender, StreamId: id}); err != nil {
		t.Fatal(err)
	}
	f.advance(100)
	if _, err := f.srv.Resume(f.ctx, &types.MsgResume{Sender: sender, StreamId: id}); err != nil {
		t.Fatal(err)
	}
	// 30s pre-pause + 0s during resume == 300; tick another 20s → 500 total
	f.advance(20)
	st, _, _ := f.k.GetStream(f.ctx, id)
	if got := types.Withdrawable(st, f.ctx.BlockTime().Unix()); got != 500 {
		t.Fatalf("withdrawable after resume+20: want 500, got %d", got)
	}
}

func TestPauseRequiresSender(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	if _, err := f.srv.Pause(f.ctx, &types.MsgPause{Sender: stranger, StreamId: id}); err == nil {
		t.Fatal("expected non-sender pause refusal")
	}
}

func TestCancelSettles(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	f.advance(30) // streamed 300, withdrawn 0
	// Withdraw 100 first.
	if _, err := f.srv.Withdraw(f.ctx, &types.MsgWithdraw{Receiver: receiver, StreamId: id, Amount: 100}); err != nil {
		t.Fatal(err)
	}
	// At this exact instant, sender cancels.
	r, err := f.srv.Cancel(f.ctx, &types.MsgCancel{Actor: sender, StreamId: id, Reason: "done"})
	if err != nil {
		t.Fatal(err)
	}
	// Receiver gets streamed - withdrawn = 300 - 100 = 200; sender gets deposit - streamed = 700.
	if r.ReceiverPayout != 200 {
		t.Fatalf("receiver payout: %d", r.ReceiverPayout)
	}
	if r.SenderRefund != 700 {
		t.Fatalf("sender refund: %d", r.SenderRefund)
	}
	if got := f.sc.get(usdDenom, receiver); got != 300 { // 100 + 200
		t.Fatalf("receiver bal: %d", got)
	}
	st, _, _ := f.k.GetStream(f.ctx, id)
	if st.Status != types.Status_STATUS_CANCELLED {
		t.Fatalf("status: %s", st.Status)
	}
}

func TestCancelEitherParty(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	// Receiver cancels.
	if _, err := f.srv.Cancel(f.ctx, &types.MsgCancel{Actor: receiver, StreamId: id}); err != nil {
		t.Fatal(err)
	}
}

func TestCancelRejectsThirdParty(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	if _, err := f.srv.Cancel(f.ctx, &types.MsgCancel{Actor: stranger, StreamId: id}); err == nil {
		t.Fatal("expected third-party cancel refusal")
	}
}

func TestTransferReassignsReceiver(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		Receiver: receiver, StreamId: id, NewReceiver: receiver2,
	}); err != nil {
		t.Fatal(err)
	}
	st, _, _ := f.k.GetStream(f.ctx, id)
	if st.Receiver != receiver2 {
		t.Fatalf("receiver after transfer: %s", st.Receiver)
	}
	// New receiver can withdraw; old cannot.
	f.advance(30)
	if _, err := f.srv.Withdraw(f.ctx, &types.MsgWithdraw{Receiver: receiver, StreamId: id}); err == nil {
		t.Fatal("old receiver should be refused")
	}
	if _, err := f.srv.Withdraw(f.ctx, &types.MsgWithdraw{Receiver: receiver2, StreamId: id}); err != nil {
		t.Fatal(err)
	}
}

func TestTransferRefusesSanctioned(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	f.san.bad[receiver2] = true
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		Receiver: receiver, StreamId: id, NewReceiver: receiver2,
	}); err == nil {
		t.Fatal("expected sanctioned new-receiver rejection")
	}
}

func TestTransferRefusesBlockedDenom(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	f.sc.block(usdDenom, receiver2, true)
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		Receiver: receiver, StreamId: id, NewReceiver: receiver2,
	}); err == nil {
		t.Fatal("expected blocked new-receiver rejection")
	}
}

func TestDepositExtendsRunway(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	r, err := f.srv.DepositToStream(f.ctx, &types.MsgDepositToStream{
		Sender: sender, StreamId: id, Amount: 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.NewDeposit != 1_500 {
		t.Fatalf("new deposit: %d", r.NewDeposit)
	}
}

func TestChangeRateSettlesUnclaimed(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	f.advance(20) // streamed 200, unclaimed 200
	if _, err := f.srv.ChangeRate(f.ctx, &types.MsgChangeRate{
		Sender: sender, StreamId: id, NewRatePerSecond: 5,
	}); err != nil {
		t.Fatal(err)
	}
	// Receiver should have received the unclaimed 200 immediately.
	if got := f.sc.get(usdDenom, receiver); got != 200 {
		t.Fatalf("receiver after settle: %d", got)
	}
	// Stream now has runway = 1000 - 200 = 800 at rate 5.
	st, _, _ := f.k.GetStream(f.ctx, id)
	if st.Deposit != 800 {
		t.Fatalf("deposit after rate change: %d", st.Deposit)
	}
	if st.RatePerSecond != 5 {
		t.Fatalf("rate: %d", st.RatePerSecond)
	}
	if st.Withdrawn != 0 {
		t.Fatalf("withdrawn baseline: %d", st.Withdrawn)
	}
	// Advance 10s: new accrual = 50.
	f.advance(10)
	stp, _, _ := f.k.GetStream(f.ctx, id)
	if got := types.Withdrawable(stp, f.ctx.BlockTime().Unix()); got != 50 {
		t.Fatalf("withdrawable post rate-change: want 50, got %d", got)
	}
}

func TestChangeRateRefusedWhilePaused(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	if _, err := f.srv.Pause(f.ctx, &types.MsgPause{Sender: sender, StreamId: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.ChangeRate(f.ctx, &types.MsgChangeRate{
		Sender: sender, StreamId: id, NewRatePerSecond: 5,
	}); err == nil {
		t.Fatal("expected paused-stream rate-change refusal")
	}
}

func TestEndTimeCapsStreaming(t *testing.T) {
	f := setup(t)
	now := f.ctx.BlockTime().Unix()
	id := basicCreate(t, f, 10, 10_000, now+100) // horizon 100s, total potential 1000
	f.advance(500) // way past end_time
	st, _, _ := f.k.GetStream(f.ctx, id)
	if got := types.StreamedAmount(st, f.ctx.BlockTime().Unix()); got != 1_000 {
		t.Fatalf("streamed capped at end_time: want 1000, got %d", got)
	}
}

func TestWithdrawCompletesAfterEndTime(t *testing.T) {
	f := setup(t)
	now := f.ctx.BlockTime().Unix()
	id := basicCreate(t, f, 10, 1_000, now+100)
	f.advance(200) // past end_time, full streamed = 1000
	if _, err := f.srv.Withdraw(f.ctx, &types.MsgWithdraw{Receiver: receiver, StreamId: id}); err != nil {
		t.Fatal(err)
	}
	st, _, _ := f.k.GetStream(f.ctx, id)
	if st.Status != types.Status_STATUS_COMPLETED {
		t.Fatalf("status after final withdraw: %s", st.Status)
	}
}

func TestPerSenderCap(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx)
	p.MaxStreamsPerSender = 2
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	f.sc.credit(usdDenom, sender, 10_000)
	for i := 0; i < 2; i++ {
		_, err := f.srv.CreateStream(f.ctx, &types.MsgCreateStream{
			Sender: sender, Receiver: receiver, Denom: usdDenom,
			RatePerSecond: 1, Deposit: 100,
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if _, err := f.srv.CreateStream(f.ctx, &types.MsgCreateStream{
		Sender: sender, Receiver: receiver, Denom: usdDenom,
		RatePerSecond: 1, Deposit: 100,
	}); err == nil {
		t.Fatal("expected per-sender cap rejection")
	}
}

func TestRateOverflowGuardCapsAtDeposit(t *testing.T) {
	f := setup(t)
	// Bump max_rate above the default cap so we can exercise the
	// SafeMul overflow guard directly. The guarded path must
	// silently clamp to deposit instead of wrapping.
	p, _ := f.k.GetParams(f.ctx)
	p.MaxRatePerSecond = types.HardMaxRate
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	id := basicCreate(t, f, types.HardMaxRate, 100, 0)
	f.advance(1_000_000_000)
	st, _, _ := f.k.GetStream(f.ctx, id)
	if got := types.StreamedAmount(st, f.ctx.BlockTime().Unix()); got != 100 {
		t.Fatalf("rate-overflow cap: want 100, got %d", got)
	}
}

func TestUpdateParamsAuthorityGate(t *testing.T) {
	f := setup(t)
	p := types.DefaultParams()
	if _, err := f.srv.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: stranger, Params: p,
	}); err == nil {
		t.Fatal("expected stranger rejection")
	}
	if _, err := f.srv.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: authority, Params: p,
	}); err != nil {
		t.Fatalf("authority update: %v", err)
	}
}

func TestGenesisRoundtrip(t *testing.T) {
	f := setup(t)
	_ = basicCreate(t, f, 10, 1_000, 0)
	gs, err := f.k.ExportGenesis(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs.Streams) != 1 || gs.NextStreamId != 2 {
		t.Fatalf("export: streams=%d next=%d", len(gs.Streams), gs.NextStreamId)
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

// ---- Regression tests ---------------------------------------------------

// Regression: a receiver sanctioned mid-stream must not be paid
// out via the other party's Cancel — drainPool only checks the
// per-denom freeze, so the chain-level sanctions list is a
// separate re-check.
func TestCancelRefusesSanctionedReceiverPayout(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	f.advance(30) // streamed 300
	f.san.bad[receiver] = true
	if _, err := f.srv.Cancel(f.ctx, &types.MsgCancel{Actor: sender, StreamId: id, Reason: "x"}); err == nil {
		t.Fatal("expected sanctioned-receiver cancel refusal")
	}
	// Stream stays ACTIVE — funds remain pinned in the pool.
	st, _, _ := f.k.GetStream(f.ctx, id)
	if st.Status != types.Status_STATUS_ACTIVE {
		t.Fatalf("status after refused cancel: %s", st.Status)
	}
}

// Regression: a sender sanctioned post-create must not receive
// the refund leg of Cancel.
func TestCancelRefusesSanctionedSenderRefund(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f, 10, 1_000, 0)
	// No accrual yet — receiver_payout == 0; the only payout leg
	// is the sender_refund.
	f.san.bad[sender] = true
	if _, err := f.srv.Cancel(f.ctx, &types.MsgCancel{Actor: receiver, StreamId: id, Reason: "x"}); err == nil {
		t.Fatal("expected sanctioned-sender refund refusal")
	}
}

func TestCountStreamsIsCheap(t *testing.T) {
	f := setup(t)
	f.sc.credit(usdDenom, sender, 10_000)
	for i := 0; i < 4; i++ {
		if _, err := f.srv.CreateStream(f.ctx, &types.MsgCreateStream{
			Sender: sender, Receiver: receiver, Denom: usdDenom,
			RatePerSecond: 1, Deposit: 100,
		}); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	n, err := f.k.CountStreams(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("count: %d", n)
	}
}

// suppress unused-var warnings for fixtures we keep around for
// future tests but don't yet exercise.
var _ = sender2
