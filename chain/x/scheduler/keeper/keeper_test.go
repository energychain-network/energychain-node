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

	audittypes "energychain/x/audit/types"
	"energychain/x/scheduler/keeper"
	"energychain/x/scheduler/types"
)

// ---- Stubs ---------------------------------------------------------------

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
func (s *stubStablecoin) HasDenom(_ sdk.Context, d string) bool       { return s.denoms[d] }
func (s *stubStablecoin) IsDenomPaused(_ sdk.Context, d string) bool  { return s.pausedDenom[d] }
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

// stubRouter records every routed Msg in order and lets a single
// upcoming call be flipped to "fail" or "panic" mode.
type stubRouter struct {
	calls    []sdk.Msg
	failNext error
	panicNext bool
}

func (r *stubRouter) RouteMsg(_ sdk.Context, msg sdk.Msg) error {
	r.calls = append(r.calls, msg)
	if r.panicNext {
		r.panicNext = false
		panic("payload panic injected")
	}
	if r.failNext != nil {
		err := r.failNext
		r.failNext = nil
		return err
	}
	return nil
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
	authority = mkAddr(1)
	owner     = mkAddr(2)
	owner2    = mkAddr(3)
	stranger  = mkAddr(4)
	collector = mkAddr(5)
)

const usdDenom = "usd"

// ---- Setup --------------------------------------------------------------

type fixture struct {
	k      keeper.Keeper
	ctx    sdk.Context
	srv    types.MsgServer
	sc     *stubStablecoin
	router *stubRouter
	reg    cdctypes.InterfaceRegistry
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
	// Register the scheduler Msg types (so SchedulerSelfReject can
	// see the type_url namespace at runtime) and x/audit's Msgs
	// (used as the non-self payload in tests below). The keeper's
	// UnpackPayload uses the registry to materialise the typed Msg.
	types.RegisterInterfaces(registry)
	audittypes.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	sc := newStubStablecoin()
	sc.denoms[usdDenom] = true
	router := &stubRouter{}

	k := keeper.NewKeeper(cdc, registry, runtime.NewKVStoreService(storeKey), authority, sc, router, nil)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return &fixture{
		k: k, ctx: ctx, srv: keeper.NewMsgServerImpl(k),
		sc: sc, router: router, reg: registry,
	}
}

func (f *fixture) advance(seconds int64) {
	f.ctx = f.ctx.WithBlockTime(f.ctx.BlockTime().Add(time.Duration(seconds) * time.Second))
}

// payloadFor builds an Any containing a non-scheduler Msg
// (audit.MsgRecordAudit) whose signer is the supplied address.
// audit is used because (a) its type_url is outside the
// scheduler namespace (the self-scheduling ban refuses our own
// Msgs) and (b) it's already part of the chain's interface
// registry. The stub router intercepts the call so the audit
// handler itself never runs.
func payloadFor(t *testing.T, _ cdctypes.InterfaceRegistry, who string) *cdctypes.Any {
	t.Helper()
	inner := &audittypes.MsgRecordAudit{
		Creator:   who,
		EventType: "scheduler.tick",
		Target:    "test",
		Action:    "noop",
		Data:      "{}",
	}
	any, err := cdctypes.NewAnyWithValue(inner)
	if err != nil {
		t.Fatalf("pack any: %v", err)
	}
	return any
}

// basicCreate funds owner with budget and creates a 60s-interval
// active job. Returns the job id.
func basicCreate(t *testing.T, f *fixture) uint64 {
	t.Helper()
	f.sc.credit(usdDenom, owner, 10_000)
	r, err := f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
		Owner:           owner,
		Payload:         payloadFor(t, f.reg, owner),
		StartTime:       0,
		IntervalSeconds: 60,
		MaxExecutions:   0,
		FeeDenom:        usdDenom,
		FeePerRun:       100,
		InitialBudget:   1_000,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return r.JobId
}

// ============================================================================
// Tests
// ============================================================================

func TestCreateJobFundsPoolAndIndexesJob(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)

	if got := f.sc.get(usdDenom, owner); got != 9_000 {
		t.Fatalf("owner balance after create: want 9000, got %d", got)
	}
	if got := f.sc.get(usdDenom, keeper.SchedulerFeePool()); got != 1_000 {
		t.Fatalf("pool balance after create: want 1000, got %d", got)
	}
	j, ok, err := f.k.GetJob(f.ctx, id)
	if err != nil || !ok {
		t.Fatalf("job missing: ok=%v err=%v", ok, err)
	}
	if j.Status != types.Status_STATUS_ACTIVE {
		t.Fatalf("status after create: want ACTIVE, got %s", j.Status)
	}
}

func TestCreateJobRejectsBadInterval(t *testing.T) {
	f := setup(t)
	f.sc.credit(usdDenom, owner, 10_000)
	_, err := f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
		Owner:           owner,
		Payload:         payloadFor(t, f.reg, owner),
		IntervalSeconds: 1, // below default min (60)
		FeeDenom:        usdDenom,
		FeePerRun:       10,
		InitialBudget:   100,
	})
	if err == nil {
		t.Fatal("expected interval rejection")
	}
}

func TestCreateJobRejectsSignerMismatch(t *testing.T) {
	f := setup(t)
	f.sc.credit(usdDenom, owner, 10_000)
	_, err := f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
		Owner:           owner,
		Payload:         payloadFor(t, f.reg, stranger), // signer != owner
		IntervalSeconds: 60,
		FeeDenom:        usdDenom,
		FeePerRun:       10,
		InitialBudget:   100,
	})
	if err == nil {
		t.Fatal("expected signer-mismatch rejection")
	}
}

func TestCreateJobRefusesPausedDenom(t *testing.T) {
	f := setup(t)
	f.sc.pausedDenom[usdDenom] = true
	f.sc.credit(usdDenom, owner, 10_000)
	_, err := f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
		Owner: owner, Payload: payloadFor(t, f.reg, owner),
		IntervalSeconds: 60, FeeDenom: usdDenom,
		FeePerRun: 10, InitialBudget: 100,
	})
	if err == nil {
		t.Fatal("expected paused-denom rejection")
	}
}

func TestCreateJobRefusesBlockedOwner(t *testing.T) {
	f := setup(t)
	f.sc.block(usdDenom, owner, true)
	f.sc.credit(usdDenom, owner, 10_000)
	_, err := f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
		Owner: owner, Payload: payloadFor(t, f.reg, owner),
		IntervalSeconds: 60, FeeDenom: usdDenom,
		FeePerRun: 10, InitialBudget: 100,
	})
	if err == nil {
		t.Fatal("expected blocked-owner rejection")
	}
}

func TestTopUpAddsBudget(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	f.sc.credit(usdDenom, owner, 500)
	r, err := f.srv.TopUpJob(f.ctx, &types.MsgTopUpJob{Owner: owner, JobId: id, Amount: 500})
	if err != nil {
		t.Fatal(err)
	}
	if r.NewBudget != 1_500 {
		t.Fatalf("budget after topup: want 1500, got %d", r.NewBudget)
	}
}

func TestPauseAndResumeJob(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)

	if _, err := f.srv.PauseJob(f.ctx, &types.MsgPauseJob{Owner: owner, JobId: id, Reason: "vacation"}); err != nil {
		t.Fatal(err)
	}
	j, _, _ := f.k.GetJob(f.ctx, id)
	if j.Status != types.Status_STATUS_PAUSED {
		t.Fatalf("status after pause: want PAUSED, got %s", j.Status)
	}
	if _, err := f.srv.ResumeJob(f.ctx, &types.MsgResumeJob{Owner: owner, JobId: id}); err != nil {
		t.Fatal(err)
	}
	j, _, _ = f.k.GetJob(f.ctx, id)
	if j.Status != types.Status_STATUS_ACTIVE {
		t.Fatalf("status after resume: want ACTIVE, got %s", j.Status)
	}
}

func TestPauseRejectsStranger(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	if _, err := f.srv.PauseJob(f.ctx, &types.MsgPauseJob{Owner: stranger, JobId: id}); err == nil {
		t.Fatal("expected stranger to be refused")
	}
}

func TestCancelRefundsBudget(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	beforeOwner := f.sc.get(usdDenom, owner)
	r, err := f.srv.CancelJob(f.ctx, &types.MsgCancelJob{
		Owner: owner, JobId: id, Refund: true, Reason: "done",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Refunded != 1_000 {
		t.Fatalf("refunded: want 1000, got %d", r.Refunded)
	}
	if got := f.sc.get(usdDenom, owner); got != beforeOwner+1_000 {
		t.Fatalf("owner balance after cancel-refund: want %d, got %d", beforeOwner+1_000, got)
	}
	j, _, _ := f.k.GetJob(f.ctx, id)
	if j.Status != types.Status_STATUS_CANCELLED {
		t.Fatalf("status: %s", j.Status)
	}
}

func TestWithdrawRefusedOnActiveJob(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	if _, err := f.srv.WithdrawJob(f.ctx, &types.MsgWithdrawJob{Owner: owner, JobId: id, Amount: 100}); err == nil {
		t.Fatal("expected withdraw refusal on ACTIVE job")
	}
}

func TestWithdrawAfterPause(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	if _, err := f.srv.PauseJob(f.ctx, &types.MsgPauseJob{Owner: owner, JobId: id}); err != nil {
		t.Fatal(err)
	}
	r, err := f.srv.WithdrawJob(f.ctx, &types.MsgWithdrawJob{Owner: owner, JobId: id, Amount: 0})
	if err != nil {
		t.Fatal(err)
	}
	if r.Withdrawn != 1_000 {
		t.Fatalf("withdrawn: %d", r.Withdrawn)
	}
}

func TestUpdateJobChangesInterval(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	if _, err := f.srv.UpdateJob(f.ctx, &types.MsgUpdateJob{
		Owner:           owner,
		JobId:           id,
		IntervalSeconds: 120,
	}); err != nil {
		t.Fatal(err)
	}
	j, _, _ := f.k.GetJob(f.ctx, id)
	if j.IntervalSeconds != 120 {
		t.Fatalf("interval: %d", j.IntervalSeconds)
	}
}

func TestUpdateJobSignerMismatchRejected(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	if _, err := f.srv.UpdateJob(f.ctx, &types.MsgUpdateJob{
		Owner:   owner,
		JobId:   id,
		Payload: payloadFor(t, f.reg, stranger),
	}); err == nil {
		t.Fatal("expected signer-mismatch rejection on update")
	}
}

func TestEndBlockerExecutesDueJobs(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)

	// Job's next_run is start_time = now+60. Advance past it.
	f.advance(61)
	if _, _, err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.router.calls) != 1 {
		t.Fatalf("router calls after one tick: want 1, got %d", len(f.router.calls))
	}
	j, _, _ := f.k.GetJob(f.ctx, id)
	if j.ExecutionsDone != 1 || j.Successes != 1 || j.Failures != 0 {
		t.Fatalf("exec/success/fail = %d/%d/%d", j.ExecutionsDone, j.Successes, j.Failures)
	}
	if j.FeeBudget != 900 || j.FeesSpent != 100 {
		t.Fatalf("budget=%d spent=%d", j.FeeBudget, j.FeesSpent)
	}
}

func TestEndBlockerSkipsFuture(t *testing.T) {
	f := setup(t)
	_ = basicCreate(t, f)
	// time has not advanced; next_run is in the future
	if _, _, err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.router.calls) != 0 {
		t.Fatalf("router calls without time advance: want 0, got %d", len(f.router.calls))
	}
}

func TestEndBlockerCatchUpAdvancesOnlyOnce(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	// Skip 10 intervals (600s) — under the AdvanceNextRun fast-
	// forward rule we should still execute exactly once and
	// re-schedule for the next future slot, NOT burst-execute 10x.
	f.advance(601)
	if _, _, err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.router.calls) != 1 {
		t.Fatalf("burst-protected single tick: got %d calls", len(f.router.calls))
	}
	j, _, _ := f.k.GetJob(f.ctx, id)
	if j.NextRunTime <= f.ctx.BlockTime().Unix() {
		t.Fatalf("next_run %d <= now %d", j.NextRunTime, f.ctx.BlockTime().Unix())
	}
}

func TestEndBlockerExhaustsOnZeroBudget(t *testing.T) {
	f := setup(t)
	// budget = 100, fee_per_run = 100 → exactly one execution then EXHAUSTED.
	f.sc.credit(usdDenom, owner, 1_000)
	r, err := f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
		Owner: owner, Payload: payloadFor(t, f.reg, owner),
		IntervalSeconds: 60, FeeDenom: usdDenom,
		FeePerRun: 100, InitialBudget: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := r.JobId

	// Tick 1: enough budget, executes, budget = 0.
	f.advance(61)
	if _, _, err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	j, _, _ := f.k.GetJob(f.ctx, id)
	if j.Status != types.Status_STATUS_ACTIVE || j.FeeBudget != 0 {
		t.Fatalf("after first tick: status=%s budget=%d", j.Status, j.FeeBudget)
	}
	// Tick 2: insufficient budget → soft EXHAUSTED (no execution).
	f.advance(61)
	if _, _, err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	j, _, _ = f.k.GetJob(f.ctx, id)
	if j.Status != types.Status_STATUS_EXHAUSTED {
		t.Fatalf("after exhaust tick: status=%s", j.Status)
	}
	if len(f.router.calls) != 1 {
		t.Fatalf("router should have been called once total, got %d", len(f.router.calls))
	}
}

func TestEndBlockerMaxExecutionsCap(t *testing.T) {
	f := setup(t)
	f.sc.credit(usdDenom, owner, 10_000)
	r, err := f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
		Owner: owner, Payload: payloadFor(t, f.reg, owner),
		IntervalSeconds: 60, MaxExecutions: 2,
		FeeDenom: usdDenom, FeePerRun: 10, InitialBudget: 1_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := r.JobId
	for i := 0; i < 5; i++ {
		f.advance(61)
		if _, _, err := f.k.EndBlock(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	j, _, _ := f.k.GetJob(f.ctx, id)
	if j.ExecutionsDone != 2 {
		t.Fatalf("exec_done: want 2, got %d", j.ExecutionsDone)
	}
	if j.Status != types.Status_STATUS_EXHAUSTED {
		t.Fatalf("status: %s", j.Status)
	}
}

func TestEndBlockerHandlesPanic(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	f.router.panicNext = true
	f.advance(61)
	if _, _, err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	j, _, _ := f.k.GetJob(f.ctx, id)
	if j.Failures != 1 || j.Successes != 0 {
		t.Fatalf("failures=%d successes=%d", j.Failures, j.Successes)
	}
	if j.LastError == "" {
		t.Fatalf("last_error not recorded after panic")
	}
	// Fee still consumed: budget 1000 - 100 = 900.
	if j.FeeBudget != 900 {
		t.Fatalf("budget after panic-tick: want 900, got %d", j.FeeBudget)
	}
}

func TestEndBlockerFeeStillChargedOnError(t *testing.T) {
	f := setup(t)
	id := basicCreate(t, f)
	f.router.failNext = fmt.Errorf("nope")
	f.advance(61)
	if _, _, err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	j, _, _ := f.k.GetJob(f.ctx, id)
	if j.FeeBudget != 900 {
		t.Fatalf("budget after failed tick: want 900, got %d", j.FeeBudget)
	}
}

func TestEndBlockerRespectsMaxJobsPerBlock(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx)
	p.MaxJobsPerBlock = 2
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	// Create 5 jobs all due immediately after a 61s advance.
	f.sc.credit(usdDenom, owner, 10_000)
	for i := 0; i < 5; i++ {
		_, err := f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
			Owner: owner, Payload: payloadFor(t, f.reg, owner),
			IntervalSeconds: 60, FeeDenom: usdDenom,
			FeePerRun: 10, InitialBudget: 100,
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	f.advance(61)
	if _, _, err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.router.calls) != 2 {
		t.Fatalf("expected 2 jobs ticked per block, got %d", len(f.router.calls))
	}
}

func TestExhaustedJobCanResumeAfterTopUp(t *testing.T) {
	f := setup(t)
	f.sc.credit(usdDenom, owner, 10_000)
	r, err := f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
		Owner: owner, Payload: payloadFor(t, f.reg, owner),
		IntervalSeconds: 60, FeeDenom: usdDenom,
		FeePerRun: 100, InitialBudget: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := r.JobId
	// Drive to EXHAUSTED.
	f.advance(61)
	if _, _, err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.advance(61)
	if _, _, err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	j, _, _ := f.k.GetJob(f.ctx, id)
	if j.Status != types.Status_STATUS_EXHAUSTED {
		t.Fatalf("status: %s", j.Status)
	}
	// Top up with resume_if_exhausted.
	resp, err := f.srv.TopUpJob(f.ctx, &types.MsgTopUpJob{
		Owner: owner, JobId: id, Amount: 500, ResumeIfExhausted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != types.Status_STATUS_ACTIVE {
		t.Fatalf("status after atomic top-up+resume: %s", resp.Status)
	}
}

func TestPerOwnerJobCap(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx)
	p.MaxJobsPerOwner = 2
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	f.sc.credit(usdDenom, owner, 10_000)
	for i := 0; i < 2; i++ {
		_, err := f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
			Owner: owner, Payload: payloadFor(t, f.reg, owner),
			IntervalSeconds: 60, FeeDenom: usdDenom,
			FeePerRun: 10, InitialBudget: 100,
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if _, err := f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
		Owner: owner, Payload: payloadFor(t, f.reg, owner),
		IntervalSeconds: 60, FeeDenom: usdDenom,
		FeePerRun: 10, InitialBudget: 100,
	}); err == nil {
		t.Fatal("expected per-owner cap rejection")
	}
	// Other owners are not affected.
	f.sc.credit(usdDenom, owner2, 10_000)
	if _, err := f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
		Owner: owner2, Payload: payloadFor(t, f.reg, owner2),
		IntervalSeconds: 60, FeeDenom: usdDenom,
		FeePerRun: 10, InitialBudget: 100,
	}); err != nil {
		t.Fatalf("other owner refused: %v", err)
	}
}

func TestFeeCollectorReceivesPerTickFee(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx)
	p.FeeCollector = collector
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	_ = basicCreate(t, f)
	f.advance(61)
	if _, _, err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.sc.get(usdDenom, collector); got != 100 {
		t.Fatalf("collector balance after tick: want 100, got %d", got)
	}
	if got := f.sc.get(usdDenom, keeper.SchedulerFeePool()); got != 900 {
		t.Fatalf("pool balance after tick: want 900, got %d", got)
	}
}

func TestUpdateParamsAuthorityGate(t *testing.T) {
	f := setup(t)
	p := types.DefaultParams()
	if _, err := f.srv.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: stranger, Params: p,
	}); err == nil {
		t.Fatal("expected stranger authority rejection")
	}
	if _, err := f.srv.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: authority, Params: p,
	}); err != nil {
		t.Fatalf("authority update: %v", err)
	}
}

func TestGenesisRoundtrip(t *testing.T) {
	f := setup(t)
	_ = basicCreate(t, f)
	gs, err := f.k.ExportGenesis(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs.Jobs) != 1 {
		t.Fatalf("exported jobs: %d", len(gs.Jobs))
	}
	if gs.NextJobId != 2 {
		t.Fatalf("next_job_id: %d", gs.NextJobId)
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("validate genesis: %v", err)
	}
}

// ---- Regression tests ---------------------------------------------------

// Regression: self-scheduling is banned to prevent the EndBlocker's
// cache-then-outer-commit pattern from clobbering a payload's
// mutations to the same row.
func TestCreateRejectsSelfSchedulingPayload(t *testing.T) {
	f := setup(t)
	f.sc.credit(usdDenom, owner, 10_000)
	// PauseJob lives in the scheduler module — its type_url
	// starts with "/energychain.scheduler.v1." and must be
	// rejected as a payload at create time.
	any, err := cdctypes.NewAnyWithValue(&types.MsgPauseJob{Owner: owner, JobId: 1, Reason: "x"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
		Owner: owner, Payload: any,
		IntervalSeconds: 60, FeeDenom: usdDenom,
		FeePerRun: 10, InitialBudget: 100,
	})
	if err == nil {
		t.Fatal("expected self-scheduler payload rejection")
	}
}

// Regression: TopUp + ResumeIfExhausted on a MaxExecutions-exhausted
// job must NOT resurrect it past its cap.
func TestTopUpResumeRefusesMaxExecExhausted(t *testing.T) {
	f := setup(t)
	f.sc.credit(usdDenom, owner, 10_000)
	r, err := f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
		Owner: owner, Payload: payloadFor(t, f.reg, owner),
		IntervalSeconds: 60, MaxExecutions: 1,
		FeeDenom: usdDenom, FeePerRun: 10, InitialBudget: 1_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := r.JobId
	// Tick once to consume the only allotted execution.
	f.advance(61)
	if _, _, err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	j, _, _ := f.k.GetJob(f.ctx, id)
	if j.Status != types.Status_STATUS_EXHAUSTED {
		t.Fatalf("status after max-exec: %s", j.Status)
	}
	// Top-up must NOT revive the job because the cap is binding.
	f.sc.credit(usdDenom, owner, 1_000)
	if _, err := f.srv.TopUpJob(f.ctx, &types.MsgTopUpJob{
		Owner: owner, JobId: id, Amount: 1_000, ResumeIfExhausted: true,
	}); err == nil {
		t.Fatal("expected max-exec resume refusal")
	}
}

// Regression: TopUp + ResumeIfExhausted on an end_time-passed job
// must NOT resurrect it past its horizon.
func TestTopUpResumeRefusesEndTimePassed(t *testing.T) {
	f := setup(t)
	f.sc.credit(usdDenom, owner, 10_000)
	now := f.ctx.BlockTime().Unix()
	r, err := f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
		Owner: owner, Payload: payloadFor(t, f.reg, owner),
		StartTime: now + 60, EndTime: now + 600,
		IntervalSeconds: 60,
		FeeDenom:        usdDenom, FeePerRun: 100, InitialBudget: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := r.JobId
	// Tick once → budget = 0, exhausted by budget.
	f.advance(61)
	if _, _, err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.advance(61) // budget-exhausted tick
	if _, _, err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	// Advance past end_time.
	f.advance(1_000)
	if _, err := f.srv.TopUpJob(f.ctx, &types.MsgTopUpJob{
		Owner: owner, JobId: id, Amount: 500, ResumeIfExhausted: true,
	}); err == nil {
		t.Fatal("expected end_time resume refusal")
	}
}

func TestCountJobsIsCheap(t *testing.T) {
	f := setup(t)
	f.sc.credit(usdDenom, owner, 10_000)
	for i := 0; i < 4; i++ {
		if _, err := f.srv.CreateJob(f.ctx, &types.MsgCreateJob{
			Owner: owner, Payload: payloadFor(t, f.reg, owner),
			IntervalSeconds: 60, FeeDenom: usdDenom,
			FeePerRun: 10, InitialBudget: 100,
		}); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	n, err := f.k.CountJobs(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("CountJobs after 4 creates: want 4, got %d", n)
	}
}
