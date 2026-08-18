package keeper_test

import (
	"context"
	"testing"

	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/testutil"
	"energychain/x/automation/keeper"
	"energychain/x/automation/types"
)

var (
	authority = testutil.DeriveAddr("authority")
	alice     = testutil.DeriveAddr("alice")
	bob       = testutil.DeriveAddr("bob")
	denom     = "weusd"

	mincastSink = "mincast-treasury-sink"
	rwaSink     = "rwa-dividend-sink"
)

// ---- mocks -----------------------------------------------------------------

type mockCompliance struct{ sanctioned map[string]bool }

func (m *mockCompliance) IsSanctioned(_ sdk.Context, a string) bool { return m.sanctioned[a] }

type mockSettlement struct {
	denoms map[string]bool
	bal    map[string]uint64
}

func newSettlement() *mockSettlement {
	return &mockSettlement{denoms: map[string]bool{denom: true}, bal: map[string]uint64{}}
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
		return types.ErrSettlement.Wrap("insufficient")
	}
	m.bal[m.key(d, from)] -= amt
	m.bal[m.key(d, to)] += amt
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

type mockMincast struct {
	set        *mockSettlement
	markets    map[uint64]bool
	failInject bool
	injects    int
	closes     int
	closeRet   uint32
}

func (m *mockMincast) HasMarket(_ context.Context, id uint64) bool { return m.markets[id] }
func (m *mockMincast) InjectTreasuryFrom(ctx context.Context, funder string, marketID, amount uint64) error {
	if m.failInject {
		return types.ErrActionFailed.Wrap("inject failed")
	}
	if err := m.set.MoveBalance(ctx, denom, funder, mincastSink, amount); err != nil {
		return err
	}
	m.injects++
	return nil
}
func (m *mockMincast) CloseMaturedInvests(_ context.Context, _ string, _ uint64, _ uint32) (uint32, error) {
	m.closes++
	return m.closeRet, nil
}

type mockRWA struct {
	set       *mockSettlement
	tokens    map[uint64]bool
	snapID    uint64
	snapshots int
	dividends int
	failSnap  bool
}

func (m *mockRWA) HasToken(_ context.Context, id uint64) bool { return m.tokens[id] }
func (m *mockRWA) SnapshotHolders(_ context.Context, _ string, _ uint64) (uint64, error) {
	if m.failSnap {
		return 0, types.ErrActionFailed.Wrap("snapshot failed")
	}
	m.snapID++
	m.snapshots++
	return m.snapID, nil
}
func (m *mockRWA) DistributeDividend(ctx context.Context, admin string, _, _, amount uint64) (uint64, error) {
	if err := m.set.MoveBalance(ctx, denom, admin, rwaSink, amount); err != nil {
		return 0, err
	}
	m.dividends++
	return 1, nil
}

// ---- fixture ---------------------------------------------------------------

type fixture struct {
	ts  *testutil.TestStore
	k   keeper.Keeper
	srv types.MsgServer
	q   types.QueryServer
	cmp *mockCompliance
	set *mockSettlement
	mc  *mockMincast
	rwa *mockRWA
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ts := testutil.NewTestStore(t, types.StoreKey)
	cmp := &mockCompliance{sanctioned: map[string]bool{}}
	set := newSettlement()
	mc := &mockMincast{set: set, markets: map[uint64]bool{1: true}}
	rwa := &mockRWA{set: set, tokens: map[uint64]bool{1: true}}
	k := keeper.NewKeeper(ts.Cdc, runtime.NewKVStoreService(ts.Key(t, types.StoreKey)), authority, cmp, set, mc, rwa)
	if err := k.SetParams(ts.Ctx, types.DefaultParams()); err != nil {
		t.Fatalf("set params: %v", err)
	}
	return &fixture{ts: ts, k: k, srv: keeper.NewMsgServerImpl(k), q: keeper.NewQueryServerImpl(k), cmp: cmp, set: set, mc: mc, rwa: rwa}
}

func (f *fixture) ctx() context.Context { return f.ts.Ctx }
func (f *fixture) now() int64           { return f.ts.Ctx.BlockTime().Unix() }
func (f *fixture) bal(a string) uint64  { return f.set.GetBalance(f.ctx(), denom, a) }

func (f *fixture) getSchedule(t *testing.T, id uint64) types.Schedule {
	t.Helper()
	s, ok, err := f.k.GetSchedule(f.ctx(), id)
	if err != nil || !ok {
		t.Fatalf("schedule %d ok=%v err=%v", id, ok, err)
	}
	return s
}

func (f *fixture) getStream(t *testing.T, id uint64) types.Stream {
	t.Helper()
	s, ok, err := f.k.GetStream(f.ctx(), id)
	if err != nil || !ok {
		t.Fatalf("stream %d ok=%v err=%v", id, ok, err)
	}
	return s
}

// ---- cron tests ------------------------------------------------------------

func TestCronInject(t *testing.T) {
	f := setup(t)
	f.set.fund(alice, 10_000)
	start := f.now() + 100
	resp, err := f.srv.CreateSchedule(f.ctx(), &types.MsgCreateSchedule{
		Creator: alice, Action: types.ActionKind_ACTION_KIND_MINCAST_INJECT, TargetId: 1,
		AmountPerRun: 1000, IntervalSeconds: 100, StartTime: start,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// not yet due
	if err := f.k.EndBlock(f.ctx()); err != nil {
		t.Fatal(err)
	}
	if f.mc.injects != 0 {
		t.Fatal("should not have run before start")
	}
	// advance to due
	f.ts.Advance(100)
	if err := f.k.EndBlock(f.ctx()); err != nil {
		t.Fatal(err)
	}
	if f.mc.injects != 1 {
		t.Fatalf("expected 1 inject, got %d", f.mc.injects)
	}
	if f.bal(alice) != 9000 || f.bal(mincastSink) != 1000 {
		t.Fatalf("funds not moved: alice=%d sink=%d", f.bal(alice), f.bal(mincastSink))
	}
	s := f.getSchedule(t, resp.ScheduleId)
	if s.Successes != 1 || s.RunsDone != 1 {
		t.Fatalf("schedule counters: %+v", s)
	}
	if s.NextRun != start+100 {
		t.Fatalf("next_run=%d want %d", s.NextRun, start+100)
	}
}

func TestCronFailureRecordedAndContinues(t *testing.T) {
	f := setup(t)
	f.set.fund(alice, 10_000)
	f.mc.failInject = true
	resp, _ := f.srv.CreateSchedule(f.ctx(), &types.MsgCreateSchedule{
		Creator: alice, Action: types.ActionKind_ACTION_KIND_MINCAST_INJECT, TargetId: 1,
		AmountPerRun: 1000, IntervalSeconds: 100, StartTime: f.now(),
	})
	f.ts.Advance(1)
	if err := f.k.EndBlock(f.ctx()); err != nil {
		t.Fatal(err)
	}
	s := f.getSchedule(t, resp.ScheduleId)
	if s.Failures != 1 || s.Successes != 0 {
		t.Fatalf("expected failure recorded: %+v", s)
	}
	if s.LastError == "" {
		t.Fatal("last_error should be set")
	}
	// funds untouched (action errored atomically), schedule keeps ticking
	if f.bal(alice) != 10_000 {
		t.Fatalf("alice balance changed on failure: %d", f.bal(alice))
	}
	if s.Status != types.ScheduleStatus_SCHEDULE_STATUS_ACTIVE {
		t.Fatal("schedule should still be active")
	}
}

func TestCronSnapshotAndDividend(t *testing.T) {
	f := setup(t)
	f.set.fund(alice, 10_000)
	// snapshot-only schedule
	f.srv.CreateSchedule(f.ctx(), &types.MsgCreateSchedule{
		Creator: alice, Action: types.ActionKind_ACTION_KIND_RWA_SNAPSHOT, TargetId: 1,
		IntervalSeconds: 100, StartTime: f.now(),
	})
	// dividend schedule
	f.srv.CreateSchedule(f.ctx(), &types.MsgCreateSchedule{
		Creator: alice, Action: types.ActionKind_ACTION_KIND_RWA_DIVIDEND, TargetId: 1,
		AmountPerRun: 2000, IntervalSeconds: 100, StartTime: f.now(),
	})
	f.ts.Advance(1)
	if err := f.k.EndBlock(f.ctx()); err != nil {
		t.Fatal(err)
	}
	if f.rwa.snapshots != 2 { // one from snapshot action, one inside dividend
		t.Fatalf("snapshots=%d want 2", f.rwa.snapshots)
	}
	if f.rwa.dividends != 1 {
		t.Fatalf("dividends=%d want 1", f.rwa.dividends)
	}
	if f.bal(rwaSink) != 2000 {
		t.Fatalf("dividend not funded: %d", f.bal(rwaSink))
	}
}

func TestCronCloseMatured(t *testing.T) {
	f := setup(t)
	f.mc.closeRet = 3
	f.srv.CreateSchedule(f.ctx(), &types.MsgCreateSchedule{
		Creator: alice, Action: types.ActionKind_ACTION_KIND_MINCAST_CLOSE_MATURED, TargetId: 1,
		IntervalSeconds: 100, StartTime: f.now(),
	})
	f.ts.Advance(1)
	if err := f.k.EndBlock(f.ctx()); err != nil {
		t.Fatal(err)
	}
	if f.mc.closes != 1 {
		t.Fatalf("closes=%d want 1", f.mc.closes)
	}
}

func TestCronMaxRunsCompletes(t *testing.T) {
	f := setup(t)
	f.set.fund(alice, 10_000)
	resp, _ := f.srv.CreateSchedule(f.ctx(), &types.MsgCreateSchedule{
		Creator: alice, Action: types.ActionKind_ACTION_KIND_MINCAST_INJECT, TargetId: 1,
		AmountPerRun: 1000, IntervalSeconds: 100, StartTime: f.now(), MaxRuns: 1,
	})
	f.ts.Advance(1)
	f.k.EndBlock(f.ctx())
	s := f.getSchedule(t, resp.ScheduleId)
	if s.Status != types.ScheduleStatus_SCHEDULE_STATUS_COMPLETED {
		t.Fatalf("should complete after max_runs: %+v", s)
	}
	// advancing further must not run again
	f.ts.Advance(1000)
	f.k.EndBlock(f.ctx())
	if f.mc.injects != 1 {
		t.Fatalf("ran again after completion: %d", f.mc.injects)
	}
}

func TestPauseResumeSchedule(t *testing.T) {
	f := setup(t)
	f.set.fund(alice, 10_000)
	resp, _ := f.srv.CreateSchedule(f.ctx(), &types.MsgCreateSchedule{
		Creator: alice, Action: types.ActionKind_ACTION_KIND_MINCAST_INJECT, TargetId: 1,
		AmountPerRun: 1000, IntervalSeconds: 100, StartTime: f.now(),
	})
	if _, err := f.srv.PauseSchedule(f.ctx(), &types.MsgPauseSchedule{Creator: alice, ScheduleId: resp.ScheduleId}); err != nil {
		t.Fatal(err)
	}
	f.ts.Advance(1000)
	f.k.EndBlock(f.ctx())
	if f.mc.injects != 0 {
		t.Fatal("paused schedule must not run")
	}
	// non-owner cannot resume
	if _, err := f.srv.ResumeSchedule(f.ctx(), &types.MsgResumeSchedule{Creator: bob, ScheduleId: resp.ScheduleId}); err == nil {
		t.Fatal("non-owner resume must fail")
	}
	if _, err := f.srv.ResumeSchedule(f.ctx(), &types.MsgResumeSchedule{Creator: alice, ScheduleId: resp.ScheduleId}); err != nil {
		t.Fatal(err)
	}
	// resume rolls next_run to the next future slot; advance past it
	next := f.getSchedule(t, resp.ScheduleId).NextRun
	f.ts.Advance(next - f.now() + 1)
	f.k.EndBlock(f.ctx())
	if f.mc.injects != 1 {
		t.Fatalf("resumed schedule should run: %d", f.mc.injects)
	}
}

func TestCreateScheduleGuards(t *testing.T) {
	f := setup(t)
	// unknown market
	if _, err := f.srv.CreateSchedule(f.ctx(), &types.MsgCreateSchedule{
		Creator: alice, Action: types.ActionKind_ACTION_KIND_MINCAST_INJECT, TargetId: 99,
		AmountPerRun: 1, IntervalSeconds: 100,
	}); err == nil {
		t.Fatal("unknown market must fail")
	}
	// interval below min
	if _, err := f.srv.CreateSchedule(f.ctx(), &types.MsgCreateSchedule{
		Creator: alice, Action: types.ActionKind_ACTION_KIND_RWA_SNAPSHOT, TargetId: 1,
		IntervalSeconds: 1,
	}); err == nil {
		t.Fatal("interval below min must fail")
	}
}

// ---- stream tests ----------------------------------------------------------

func TestStreamWithdrawLifecycle(t *testing.T) {
	f := setup(t)
	f.set.fund(alice, 1000)
	start := f.now()
	resp, err := f.srv.CreateStream(f.ctx(), &types.MsgCreateStream{
		Sender: alice, Receiver: bob, Denom: denom, Deposit: 1000, RatePerSec: 10, StartTime: start,
	})
	if err != nil {
		t.Fatalf("create stream: %v", err)
	}
	if f.bal(alice) != 0 || f.bal(keeper.StreamEscrowAccount()) != 1000 {
		t.Fatalf("deposit not escrowed: alice=%d escrow=%d", f.bal(alice), f.bal(keeper.StreamEscrowAccount()))
	}
	// 50s -> 500 vested
	f.ts.Advance(50)
	w, err := f.srv.WithdrawStream(f.ctx(), &types.MsgWithdrawStream{Caller: bob, StreamId: resp.StreamId})
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if w.Withdrawn != 500 || f.bal(bob) != 500 {
		t.Fatalf("withdraw amount wrong: %d bob=%d", w.Withdrawn, f.bal(bob))
	}
	// withdraw again immediately -> nothing
	if _, err := f.srv.WithdrawStream(f.ctx(), &types.MsgWithdrawStream{Caller: bob, StreamId: resp.StreamId}); err == nil {
		t.Fatal("expected nothing-vested error")
	}
	// finish vesting and finalize via EndBlocker
	f.ts.Advance(100)
	if err := f.k.EndBlock(f.ctx()); err != nil {
		t.Fatal(err)
	}
	if f.bal(bob) != 1000 {
		t.Fatalf("bob should be fully paid: %d", f.bal(bob))
	}
	s := f.getStream(t, resp.StreamId)
	if s.Status != types.StreamStatus_STREAM_STATUS_COMPLETED {
		t.Fatalf("stream should be completed: %+v", s)
	}
	if f.bal(keeper.StreamEscrowAccount()) != 0 {
		t.Fatal("escrow should be drained")
	}
}

func TestStreamCancel(t *testing.T) {
	f := setup(t)
	f.set.fund(alice, 1000)
	resp, _ := f.srv.CreateStream(f.ctx(), &types.MsgCreateStream{
		Sender: alice, Receiver: bob, Denom: denom, Deposit: 1000, RatePerSec: 10, StartTime: f.now(),
	})
	f.ts.Advance(30) // 300 vested
	// non-sender cannot cancel
	if _, err := f.srv.CancelStream(f.ctx(), &types.MsgCancelStream{Sender: bob, StreamId: resp.StreamId}); err == nil {
		t.Fatal("non-sender cancel must fail")
	}
	cr, err := f.srv.CancelStream(f.ctx(), &types.MsgCancelStream{Sender: alice, StreamId: resp.StreamId})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cr.ReceiverPaid != 300 || cr.SenderRefunded != 700 {
		t.Fatalf("cancel split wrong: recv=%d refund=%d", cr.ReceiverPaid, cr.SenderRefunded)
	}
	if f.bal(bob) != 300 || f.bal(alice) != 700 {
		t.Fatalf("balances wrong: bob=%d alice=%d", f.bal(bob), f.bal(alice))
	}
	if f.bal(keeper.StreamEscrowAccount()) != 0 {
		t.Fatal("escrow should be empty after cancel")
	}
}

func TestStreamSanctionedReceiverBlocked(t *testing.T) {
	f := setup(t)
	f.set.fund(alice, 1000)
	// cannot even create to a sanctioned receiver
	f.cmp.sanctioned[bob] = true
	if _, err := f.srv.CreateStream(f.ctx(), &types.MsgCreateStream{
		Sender: alice, Receiver: bob, Denom: denom, Deposit: 1000, RatePerSec: 10,
	}); err == nil {
		t.Fatal("create to sanctioned receiver must fail")
	}
	// create while clear, then sanction; withdraw + finalize must be blocked/skipped
	f.cmp.sanctioned[bob] = false
	resp, _ := f.srv.CreateStream(f.ctx(), &types.MsgCreateStream{
		Sender: alice, Receiver: bob, Denom: denom, Deposit: 1000, RatePerSec: 10, StartTime: f.now(),
	})
	f.cmp.sanctioned[bob] = true
	f.ts.Advance(50)
	if _, err := f.srv.WithdrawStream(f.ctx(), &types.MsgWithdrawStream{Caller: bob, StreamId: resp.StreamId}); err == nil {
		t.Fatal("sanctioned receiver withdraw must fail")
	}
	// finalize sweep must skip (not error, stream stays active)
	f.ts.Advance(1000)
	if err := f.k.EndBlock(f.ctx()); err != nil {
		t.Fatalf("endblock must not error on sanctioned stream: %v", err)
	}
	s := f.getStream(t, resp.StreamId)
	if s.Status != types.StreamStatus_STREAM_STATUS_ACTIVE {
		t.Fatal("sanctioned stream should remain active (not force-paid)")
	}
	if f.bal(bob) != 0 {
		t.Fatal("sanctioned receiver must not be paid")
	}
}

func TestStreamSettlementConserved(t *testing.T) {
	f := setup(t)
	f.set.fund(alice, 5000)
	before := f.set.total()
	r1, _ := f.srv.CreateStream(f.ctx(), &types.MsgCreateStream{Sender: alice, Receiver: bob, Denom: denom, Deposit: 2000, RatePerSec: 5, StartTime: f.now()})
	f.srv.CreateStream(f.ctx(), &types.MsgCreateStream{Sender: alice, Receiver: bob, Denom: denom, Deposit: 3000, RatePerSec: 3, StartTime: f.now()})
	f.ts.Advance(100)
	f.srv.WithdrawStream(f.ctx(), &types.MsgWithdrawStream{Caller: bob, StreamId: r1.StreamId})
	f.srv.CancelStream(f.ctx(), &types.MsgCancelStream{Sender: alice, StreamId: r1.StreamId})
	f.ts.Advance(2000)
	f.k.EndBlock(f.ctx())
	if f.set.total() != before {
		t.Fatalf("settlement not conserved: %d != %d", f.set.total(), before)
	}
}

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	f.set.fund(alice, 5000)
	f.srv.CreateSchedule(f.ctx(), &types.MsgCreateSchedule{
		Creator: alice, Action: types.ActionKind_ACTION_KIND_MINCAST_INJECT, TargetId: 1,
		AmountPerRun: 1000, IntervalSeconds: 100, StartTime: f.now() + 50,
	})
	f.srv.CreateStream(f.ctx(), &types.MsgCreateStream{Sender: alice, Receiver: bob, Denom: denom, Deposit: 2000, RatePerSec: 5, StartTime: f.now()})

	gs, err := f.k.ExportGenesis(f.ctx())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}
	g := setup(t)
	// mirror the escrow balance x/stableusd genesis would carry
	g.set.fund(keeper.StreamEscrowAccount(), 2000)
	if err := g.k.InitGenesis(g.ctx(), gs); err != nil {
		t.Fatalf("init: %v", err)
	}
	gs2, err := g.k.ExportGenesis(g.ctx())
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if len(gs2.Schedules) != 1 || len(gs2.Streams) != 1 {
		t.Fatalf("roundtrip mismatch: sched=%d stream=%d", len(gs2.Schedules), len(gs2.Streams))
	}
	if gs2.ScheduleIdSeq != gs.ScheduleIdSeq || gs2.StreamIdSeq != gs.StreamIdSeq {
		t.Fatal("sequence mismatch")
	}
}

func TestCancelStreamSanctionedSenderBlocked(t *testing.T) {
	f := setup(t)
	f.set.fund(alice, 1000)
	resp, _ := f.srv.CreateStream(f.ctx(), &types.MsgCreateStream{
		Sender: alice, Receiver: bob, Denom: denom, Deposit: 1000, RatePerSec: 10, StartTime: f.now(),
	})
	f.ts.Advance(30)
	f.cmp.sanctioned[alice] = true
	if _, err := f.srv.CancelStream(f.ctx(), &types.MsgCancelStream{Sender: alice, StreamId: resp.StreamId}); err == nil {
		t.Fatal("sanctioned sender cancel must be blocked")
	}
	// funds remain escrowed; receiver can still withdraw their vested share
	if f.bal(keeper.StreamEscrowAccount()) != 1000 {
		t.Fatalf("escrow should be intact: %d", f.bal(keeper.StreamEscrowAccount()))
	}
	if _, err := f.srv.WithdrawStream(f.ctx(), &types.MsgWithdrawStream{Caller: bob, StreamId: resp.StreamId}); err != nil {
		t.Fatalf("receiver withdraw should still work: %v", err)
	}
}

func TestGenesisRejectsActiveScheduleAtMaxRuns(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Schedules = []types.Schedule{{
		Id: 1, Creator: alice, Action: types.ActionKind_ACTION_KIND_RWA_SNAPSHOT, TargetId: 1,
		IntervalSeconds: 100, MaxRuns: 3, RunsDone: 3, NextRun: 100,
		Status: types.ScheduleStatus_SCHEDULE_STATUS_ACTIVE,
	}}
	gs.ScheduleIdSeq = 1
	if err := gs.Validate(); err == nil {
		t.Fatal("active schedule at max_runs must be rejected")
	}
}

func TestInitGenesisRejectsOverWithdrawnStream(t *testing.T) {
	f := setup(t)
	// Future-dated stream: nothing is vested at genesis time, so a claimed
	// withdrawn of 900 is impossible and must be rejected.
	start := f.now() + 10_000
	stop, _ := types.StopTime(start, 1000, 10)
	gs := types.DefaultGenesis()
	gs.Streams = []types.Stream{{
		Id: 1, Sender: alice, Receiver: bob, Denom: denom, Deposit: 1000, RatePerSec: 10,
		StartTime: start, StopTime: stop, Withdrawn: 900,
		Status: types.StreamStatus_STREAM_STATUS_ACTIVE,
	}}
	gs.StreamIdSeq = 1
	f.set.fund(keeper.StreamEscrowAccount(), 100)
	if err := f.k.InitGenesis(f.ctx(), gs); err == nil {
		t.Fatal("over-withdrawn active stream must be rejected at init")
	}
}

func TestUpdateParamsAuth(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.UpdateParams(f.ctx(), &types.MsgUpdateParams{Authority: alice, Params: types.DefaultParams()}); err == nil {
		t.Fatal("non-authority update must fail")
	}
	if _, err := f.srv.UpdateParams(f.ctx(), &types.MsgUpdateParams{Authority: authority, Params: types.DefaultParams()}); err != nil {
		t.Fatalf("authority update: %v", err)
	}
}
