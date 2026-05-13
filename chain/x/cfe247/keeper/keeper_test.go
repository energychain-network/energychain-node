package keeper_test

import (
	"strings"
	"testing"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/log/v2"
	"cosmossdk.io/store"
	storetypes "cosmossdk.io/store/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/cfe247/keeper"
	"energychain/x/cfe247/types"
)

// ---- Stub keepers ---------------------------------------------------------

type stubEAC struct {
	rets map[uint64]types.EACRetirementView
}

func (s *stubEAC) LookupRetirement(_ sdk.Context, id uint64) (types.EACRetirementView, bool) {
	v, ok := s.rets[id]
	return v, ok
}

type stubSanctions struct{ bad map[string]bool }

func (s *stubSanctions) IsSanctioned(_ sdk.Context, addr string) bool { return s.bad[addr] }

type stubAudit struct{ calls []string }

func (s *stubAudit) RecordCFEAction(_ sdk.Context, subjectID, action, actor, detail string) {
	s.calls = append(s.calls, strings.Join([]string{subjectID, action, actor, detail}, "|"))
}

// ---- Constants ------------------------------------------------------------

const (
	authority    = "cosmos126xkl2rxz08nlv6egudef8dxn6c5suxwnsmrjc"
	providerAdm  = "cosmos1mc62kvf7adulmqcpgynqupl63hfp68lu82qjlk"
	providerAtt  = "cosmos150ln4xlvs2dg024ueft43un6e3eav7ca2xhhtf"
	subjectAdmin = "cosmos1tmxy2mearm0zkw88prwm346aqdqns6m8zlxyrg"
	creator      = "cosmos19mup59y4ep02x77uy5vjnq8c4k5a044cwlxkmq"
	allocator    = "cosmos1lrmkh72j5pqd5rav9wy8tzdq3zlmd3ullte2ev"
	providerDID  = "did:web:meterop.example.com"
)

// hour-aligned timestamp for tests (2024-01-01 00:00 UTC)
const hour0 int64 = 1704067200

// ---- Setup ----------------------------------------------------------------

type fixture struct {
	k   keeper.Keeper
	ctx sdk.Context
	srv types.MsgServer
	eac *stubEAC
	san *stubSanctions
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

	eac := &stubEAC{rets: map[uint64]types.EACRetirementView{}}
	san := &stubSanctions{bad: map[string]bool{}}
	au := &stubAudit{}

	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, eac, san, au)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(hour0+12*3600, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return &fixture{k: k, ctx: ctx, srv: keeper.NewMsgServerImpl(k), eac: eac, san: san, au: au}
}

func (f *fixture) registerProvider(t *testing.T) {
	t.Helper()
	if _, err := f.srv.RegisterDataProvider(f.ctx, &types.MsgRegisterDataProvider{
		Authority: authority, Id: "metrop", Did: providerDID,
		DisplayName: "MetrOp", Attestor: providerAtt, Admin: providerAdm,
	}); err != nil {
		t.Fatalf("register provider: %v", err)
	}
}

func (f *fixture) registerZone(t *testing.T, id string) {
	t.Helper()
	if _, err := f.srv.RegisterGridZone(f.ctx, &types.MsgRegisterGridZone{
		Authority: authority, Id: id, DisplayName: id, Country: "DE",
	}); err != nil {
		t.Fatalf("register zone: %v", err)
	}
}

func (f *fixture) registerSubject(t *testing.T) {
	t.Helper()
	if _, err := f.srv.RegisterSubject(f.ctx, &types.MsgRegisterSubject{
		Creator: creator, Id: "dc-frankfurt",
		DisplayName: "Frankfurt DC", Admin: subjectAdmin,
		DefaultGridZone: "de-lu",
	}); err != nil {
		t.Fatalf("register subject: %v", err)
	}
}

func (f *fixture) attestConsumption(t *testing.T, hour int64, wh uint64, zone string) {
	t.Helper()
	if _, err := f.srv.AttestHourlyConsumption(f.ctx, &types.MsgAttestHourlyConsumption{
		Attestor: providerAtt, DataProviderId: "metrop",
		SubjectId: "dc-frankfurt", GridZone: zone,
		HourStart: hour, WhConsumed: wh,
	}); err != nil {
		t.Fatalf("attest %d: %v", hour, err)
	}
}

func mkRet(id, certID uint64, retirer string, hourStart int64, amt uint64, zone string, isStorage bool) types.EACRetirementView {
	return types.EACRetirementView{
		RetirementID: id, CertificateID: certID,
		Retirer: retirer, Beneficiary: retirer, Amount: amt,
		CertHourStart: hourStart, CertHourEnd: hourStart + types.HourSeconds,
		GridZone: zone, IsStorage: isStorage,
	}
}

// ---------------------------------------------------------------------------
// Provider / zone / subject lifecycle
// ---------------------------------------------------------------------------

func TestRegisterProviderUnauthorized(t *testing.T) {
	f := setup(t)
	_, err := f.srv.RegisterDataProvider(f.ctx, &types.MsgRegisterDataProvider{
		Authority: subjectAdmin, Id: "x", Did: providerDID,
		DisplayName: "x", Attestor: providerAtt, Admin: providerAdm,
	})
	if err == nil {
		t.Fatal("expected unauthorized")
	}
}

func TestSuspendBlocksAttestation(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	if _, err := f.srv.SuspendDataProvider(f.ctx, &types.MsgSuspendDataProvider{
		Authority: authority, Id: "metrop", Reason: "audit",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := f.srv.AttestHourlyConsumption(f.ctx, &types.MsgAttestHourlyConsumption{
		Attestor: providerAtt, DataProviderId: "metrop",
		SubjectId: "dc-frankfurt", GridZone: "de-lu",
		HourStart: hour0, WhConsumed: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "ACTIVE") {
		t.Fatalf("expected suspended-provider block, got %v", err)
	}
}

func TestRegisterSubjectRequiresKnownZone(t *testing.T) {
	f := setup(t)
	_, err := f.srv.RegisterSubject(f.ctx, &types.MsgRegisterSubject{
		Creator: creator, Id: "dc1", DisplayName: "DC", Admin: subjectAdmin,
		DefaultGridZone: "ghost",
	})
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("expected unknown-zone error, got %v", err)
	}
}

func TestRegisterSubjectRejectsSanctionedAdmin(t *testing.T) {
	f := setup(t)
	f.registerZone(t, "de-lu")
	f.san.bad[subjectAdmin] = true
	_, err := f.srv.RegisterSubject(f.ctx, &types.MsgRegisterSubject{
		Creator: creator, Id: "dc1", DisplayName: "DC",
		Admin: subjectAdmin, DefaultGridZone: "de-lu",
	})
	if err == nil || !strings.Contains(err.Error(), "sanctions") {
		t.Fatalf("expected sanctions block, got %v", err)
	}
}

func TestRemoveZoneInUseFails(t *testing.T) {
	f := setup(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	_, err := f.srv.RemoveGridZone(f.ctx, &types.MsgRemoveGridZone{
		Authority: authority, Id: "de-lu",
	})
	if err == nil || !strings.Contains(err.Error(), "default for subject") {
		t.Fatalf("expected in-use block, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Consumption
// ---------------------------------------------------------------------------

func TestAttestConsumptionHappyPath(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	f.attestConsumption(t, hour0, 1000, "de-lu")
	c, ok, err := f.k.GetConsumption(f.ctx, "dc-frankfurt", hour0)
	if err != nil || !ok || c.WhConsumed != 1000 {
		t.Fatalf("consumption row wrong: ok=%t err=%v c=%+v", ok, err, c)
	}
	agg, ok, err := f.k.GetAggregate(f.ctx, "dc-frankfurt", hour0)
	if err != nil || !ok || agg.WhConsumed != 1000 {
		t.Fatalf("aggregate wrong: %+v err=%v", agg, err)
	}
}

func TestAttestUnknownSubjectFails(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	_, err := f.srv.AttestHourlyConsumption(f.ctx, &types.MsgAttestHourlyConsumption{
		Attestor: providerAtt, DataProviderId: "metrop",
		SubjectId: "ghost", GridZone: "de-lu",
		HourStart: hour0, WhConsumed: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "subject") {
		t.Fatalf("expected unknown-subject, got %v", err)
	}
}

func TestAttestWrongAttestorFails(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	_, err := f.srv.AttestHourlyConsumption(f.ctx, &types.MsgAttestHourlyConsumption{
		Attestor: subjectAdmin, DataProviderId: "metrop",
		SubjectId: "dc-frankfurt", GridZone: "de-lu",
		HourStart: hour0, WhConsumed: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "not registered for provider") {
		t.Fatalf("expected wrong-attestor block, got %v", err)
	}
}

func TestAttestHourMustBeAligned(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	_, err := f.srv.AttestHourlyConsumption(f.ctx, &types.MsgAttestHourlyConsumption{
		Attestor: providerAtt, DataProviderId: "metrop",
		SubjectId: "dc-frankfurt", GridZone: "de-lu",
		HourStart: hour0 + 1, WhConsumed: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "aligned") {
		t.Fatalf("expected alignment error, got %v", err)
	}
}

func TestConsumptionReattestBeforeMatchAllowed(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	f.attestConsumption(t, hour0, 1000, "de-lu")
	f.attestConsumption(t, hour0, 1500, "de-lu")
	c, _, _ := f.k.GetConsumption(f.ctx, "dc-frankfurt", hour0)
	if c.WhConsumed != 1500 {
		t.Fatalf("re-attest should overwrite when no matches: got %d", c.WhConsumed)
	}
	score, err := f.k.AnnualScores.Get(f.ctx, collections.Join("dc-frankfurt", types.HourYear(hour0)))
	if err != nil {
		t.Fatal(err)
	}
	if score.HoursWithData != 1 || score.TotalWhConsumed != 1500 {
		t.Fatalf("score should track re-attest: %+v", score)
	}
}

func TestConsumptionReattestAfterMatchRejected(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	f.attestConsumption(t, hour0, 1000, "de-lu")
	f.eac.rets[42] = mkRet(42, 7, allocator, hour0, 800, "de-lu", false)
	mustMatch(t, f, hour0, 42, 600)
	_, err := f.srv.AttestHourlyConsumption(f.ctx, &types.MsgAttestHourlyConsumption{
		Attestor: providerAtt, DataProviderId: "metrop",
		SubjectId: "dc-frankfurt", GridZone: "de-lu",
		HourStart: hour0, WhConsumed: 500,
	})
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("expected immutability error, got %v", err)
	}
}

func TestAttestBatchHappyPath(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	resp, err := f.srv.AttestHourlyConsumptionBatch(f.ctx, &types.MsgAttestHourlyConsumptionBatch{
		Attestor: providerAtt, DataProviderId: "metrop",
		Entries: []types.ConsumptionEntry{
			{SubjectId: "dc-frankfurt", GridZone: "de-lu", HourStart: hour0, WhConsumed: 100},
			{SubjectId: "dc-frankfurt", GridZone: "de-lu", HourStart: hour0 + 3600, WhConsumed: 200},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Attested != 2 {
		t.Fatalf("attested want 2 got %d", resp.Attested)
	}
}

// ---------------------------------------------------------------------------
// Allocate match
// ---------------------------------------------------------------------------

func TestAllocateMatchHappyPath(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	f.attestConsumption(t, hour0, 1000, "de-lu")
	f.eac.rets[42] = mkRet(42, 7, allocator, hour0, 800, "de-lu", false)

	resp, err := f.srv.AllocateMatch(f.ctx, &types.MsgAllocateMatch{
		Allocator: allocator, SubjectId: "dc-frankfurt",
		HourStart: hour0, EacRetirementId: 42, WhMatched: 600,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.MatchId == 0 {
		t.Fatal("match id zero")
	}
	agg, _, _ := f.k.GetAggregate(f.ctx, "dc-frankfurt", hour0)
	if agg.WhMatchedTotal != 600 || agg.WhMatchedSameZone != 600 {
		t.Fatalf("agg wrong: %+v", agg)
	}
	score, err := f.k.AnnualScores.Get(f.ctx, collections.Join("dc-frankfurt", types.HourYear(hour0)))
	if err != nil {
		t.Fatal(err)
	}
	if score.TotalWhMatched != 600 || score.TotalWhMatchedSameZone != 600 {
		t.Fatalf("score wrong: %+v", score)
	}
	// hour not fully matched (600 < 1000), so HoursFullyMatched stays 0
	if score.HoursFullyMatched != 0 {
		t.Fatalf("not fully matched yet, want 0, got %d", score.HoursFullyMatched)
	}
	// per-hour min(matched, consumed) = 600
	if score.SumMinMatchConsumed != 600 {
		t.Fatalf("sum_min want 600 got %d", score.SumMinMatchConsumed)
	}
	allocated, _ := f.k.GetRetirementAllocated(f.ctx, 42)
	if allocated != 600 {
		t.Fatalf("allocated want 600 got %d", allocated)
	}
}

func TestAllocateMatchOnlyRetirer(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	f.attestConsumption(t, hour0, 1000, "de-lu")
	f.eac.rets[42] = mkRet(42, 7, "cosmos1otherretirer000000000000000000000000", hour0, 800, "de-lu", false)
	_, err := f.srv.AllocateMatch(f.ctx, &types.MsgAllocateMatch{
		Allocator: allocator, SubjectId: "dc-frankfurt",
		HourStart: hour0, EacRetirementId: 42, WhMatched: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "retirer") {
		t.Fatalf("expected retirer-only block, got %v", err)
	}
}

func TestAllocateMatchHourMustBeContained(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	f.attestConsumption(t, hour0+3600, 1000, "de-lu") // hour 1
	f.eac.rets[42] = mkRet(42, 7, allocator, hour0, 800, "de-lu", false)
	_, err := f.srv.AllocateMatch(f.ctx, &types.MsgAllocateMatch{
		Allocator: allocator, SubjectId: "dc-frankfurt",
		HourStart: hour0 + 3600, EacRetirementId: 42, WhMatched: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "does not contain") {
		t.Fatalf("expected hour-mismatch, got %v", err)
	}
}

func TestAllocateMatchExceedsRetirementCap(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	f.attestConsumption(t, hour0, 1000, "de-lu")
	f.eac.rets[42] = mkRet(42, 7, allocator, hour0, 100, "de-lu", false)
	if _, err := f.srv.AllocateMatch(f.ctx, &types.MsgAllocateMatch{
		Allocator: allocator, SubjectId: "dc-frankfurt",
		HourStart: hour0, EacRetirementId: 42, WhMatched: 80,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := f.srv.AllocateMatch(f.ctx, &types.MsgAllocateMatch{
		Allocator: allocator, SubjectId: "dc-frankfurt",
		HourStart: hour0, EacRetirementId: 42, WhMatched: 50,
	})
	if err == nil || !strings.Contains(err.Error(), "cap") {
		t.Fatalf("expected retirement cap, got %v", err)
	}
}

func TestAllocateMatchRequiresConsumption(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	f.eac.rets[42] = mkRet(42, 7, allocator, hour0, 800, "de-lu", false)
	_, err := f.srv.AllocateMatch(f.ctx, &types.MsgAllocateMatch{
		Allocator: allocator, SubjectId: "dc-frankfurt",
		HourStart: hour0, EacRetirementId: 42, WhMatched: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "no consumption row") {
		t.Fatalf("expected missing-consumption, got %v", err)
	}
}

func TestAllocateMatchSameZoneCredit(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerZone(t, "fr")
	f.registerSubject(t)
	f.attestConsumption(t, hour0, 1000, "de-lu")
	// retirement is from FR zone, so not same-zone
	f.eac.rets[42] = mkRet(42, 7, allocator, hour0, 800, "fr", false)
	if _, err := f.srv.AllocateMatch(f.ctx, &types.MsgAllocateMatch{
		Allocator: allocator, SubjectId: "dc-frankfurt",
		HourStart: hour0, EacRetirementId: 42, WhMatched: 500,
	}); err != nil {
		t.Fatal(err)
	}
	agg, _, _ := f.k.GetAggregate(f.ctx, "dc-frankfurt", hour0)
	if agg.WhMatchedTotal != 500 || agg.WhMatchedSameZone != 0 {
		t.Fatalf("expected total=500 same_zone=0, got %+v", agg)
	}
}

func TestAllocateMatchStorageFlag(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	f.attestConsumption(t, hour0, 1000, "de-lu")
	f.eac.rets[42] = mkRet(42, 7, allocator, hour0, 800, "de-lu", true)
	if _, err := f.srv.AllocateMatch(f.ctx, &types.MsgAllocateMatch{
		Allocator: allocator, SubjectId: "dc-frankfurt",
		HourStart: hour0, EacRetirementId: 42, WhMatched: 500,
	}); err != nil {
		t.Fatal(err)
	}
	agg, _, _ := f.k.GetAggregate(f.ctx, "dc-frankfurt", hour0)
	if agg.WhMatchedStorage != 500 {
		t.Fatalf("storage credit want 500 got %d", agg.WhMatchedStorage)
	}
}

func TestAllocateMatchFullyMatchedFlag(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	f.attestConsumption(t, hour0, 1000, "de-lu")
	f.eac.rets[42] = mkRet(42, 7, allocator, hour0, 1500, "de-lu", false)
	if _, err := f.srv.AllocateMatch(f.ctx, &types.MsgAllocateMatch{
		Allocator: allocator, SubjectId: "dc-frankfurt",
		HourStart: hour0, EacRetirementId: 42, WhMatched: 1200,
	}); err != nil {
		t.Fatal(err)
	}
	score, err := f.k.AnnualScores.Get(f.ctx, collections.Join("dc-frankfurt", types.HourYear(hour0)))
	if err != nil {
		t.Fatal(err)
	}
	if score.HoursFullyMatched != 1 {
		t.Fatalf("expected fully-matched hour, got %d", score.HoursFullyMatched)
	}
	if score.SumMinMatchConsumed != 1000 {
		t.Fatalf("min capped at consumed: want 1000 got %d", score.SumMinMatchConsumed)
	}
}

// ---------------------------------------------------------------------------
// Report
// ---------------------------------------------------------------------------

func TestGenerateReportComputesScores(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerZone(t, "fr")
	f.registerSubject(t)

	for i := int64(0); i < 5; i++ {
		f.attestConsumption(t, hour0+i*3600, 1000, "de-lu")
	}
	// hour 0: 1000 same-zone match (fully)
	f.eac.rets[1] = mkRet(1, 1, allocator, hour0, 1000, "de-lu", false)
	mustMatch(t, f, hour0, 1, 1000)
	// hour 1: 500 same-zone (half)
	f.eac.rets[2] = mkRet(2, 2, allocator, hour0+3600, 500, "de-lu", false)
	mustMatch(t, f, hour0+3600, 2, 500)
	// hour 2: 1000 from FR (cross-zone, fully matched but no location credit)
	f.eac.rets[3] = mkRet(3, 3, allocator, hour0+2*3600, 1000, "fr", false)
	mustMatch(t, f, hour0+2*3600, 3, 1000)
	// hours 3 and 4: no match

	resp, err := f.srv.GenerateReport(f.ctx, &types.MsgGenerateReport{
		Admin: subjectAdmin, SubjectId: "dc-frankfurt",
		Format: types.ReportFormat_REPORT_FORMAT_CFE_COMPACT,
		PeriodStart: hour0, PeriodEnd: hour0 + 5*3600,
		ReportUri:  "https://example.com/r1",
		ReportHash: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
	})
	if err != nil {
		t.Fatal(err)
	}
	rep, _ := f.k.Reports.Get(f.ctx, resp.ReportId)
	// total consumed = 5000, matched = 1000+500+1000 = 2500
	if rep.TotalWhConsumed != 5000 || rep.TotalWhMatched != 2500 {
		t.Fatalf("totals want 5000/2500 got %d/%d", rep.TotalWhConsumed, rep.TotalWhMatched)
	}
	// hourly score = (1000 + 500 + 1000 + 0 + 0) / 5000 = 50%
	if rep.HourlyScoreBps != 5000 {
		t.Fatalf("hourly bps want 5000 got %d", rep.HourlyScoreBps)
	}
	// annual score = 2500 / 5000 = 50%
	if rep.AnnualScoreBps != 5000 {
		t.Fatalf("annual bps want 5000 got %d", rep.AnnualScoreBps)
	}
	// location_matched = 1500 (only de-lu matches) / 5000 = 30%
	if rep.LocationMatchedScoreBps != 3000 {
		t.Fatalf("location bps want 3000 got %d", rep.LocationMatchedScoreBps)
	}
	if rep.HoursFullyMatched != 2 {
		t.Fatalf("fully matched want 2 got %d", rep.HoursFullyMatched)
	}
	if rep.HoursWithData != 5 {
		t.Fatalf("hours with data want 5 got %d", rep.HoursWithData)
	}
}

func mustMatch(t *testing.T, f *fixture, hour int64, retID, wh uint64) {
	t.Helper()
	if _, err := f.srv.AllocateMatch(f.ctx, &types.MsgAllocateMatch{
		Allocator: allocator, SubjectId: "dc-frankfurt",
		HourStart: hour, EacRetirementId: retID, WhMatched: wh,
	}); err != nil {
		t.Fatalf("match: %v", err)
	}
}

func TestGenerateReportPeriodCap(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	_, err := f.srv.GenerateReport(f.ctx, &types.MsgGenerateReport{
		Admin: subjectAdmin, SubjectId: "dc-frankfurt",
		Format:      types.ReportFormat_REPORT_FORMAT_CFE_COMPACT,
		PeriodStart: hour0,
		// 367 days far exceeds DefaultMaxReportPeriodDays=366
		PeriodEnd:  hour0 + 367*86400,
		ReportUri:  "https://example.com/r1",
		ReportHash: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
	})
	if err == nil || !strings.Contains(err.Error(), "max_report_period_days") {
		t.Fatalf("expected period cap, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Genesis
// ---------------------------------------------------------------------------

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	f.registerProvider(t)
	f.registerZone(t, "de-lu")
	f.registerSubject(t)
	f.attestConsumption(t, hour0, 1000, "de-lu")
	f.eac.rets[42] = mkRet(42, 7, allocator, hour0, 800, "de-lu", false)
	mustMatch(t, f, hour0, 42, 600)

	gs, err := f.k.ExportGenesis(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("exported gs invalid: %v", err)
	}
	g := setup(t)
	if err := g.k.InitGenesis(g.ctx, gs); err != nil {
		t.Fatal(err)
	}
	c, ok, err := g.k.GetConsumption(g.ctx, "dc-frankfurt", hour0)
	if err != nil || !ok || c.WhConsumed != 1000 {
		t.Fatalf("consumption round-trip wrong: %+v ok=%t err=%v", c, ok, err)
	}
	allocated, _ := g.k.GetRetirementAllocated(g.ctx, 42)
	if allocated != 600 {
		t.Fatalf("allocated round-trip wrong: %d", allocated)
	}
}

func TestGenesisInvariantAggregateBounds(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Subjects = []types.Subject{{
		Id: "dc-x", DisplayName: "x", Status: types.SubjectStatus_SUBJECT_STATUS_ACTIVE,
		Admin: subjectAdmin,
	}}
	gs.Aggregates = []types.HourlyAggregate{{
		SubjectId: "dc-x", HourStart: hour0,
		WhConsumed: 1000, WhMatchedTotal: 100,
		WhMatchedSameZone: 200, // > total -> invalid
	}}
	if err := gs.Validate(); err == nil {
		t.Fatal("expected aggregate invariant error")
	}
}
