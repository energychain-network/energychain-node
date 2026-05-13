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

	"energychain/x/eac/keeper"
	"energychain/x/eac/types"
)

// ---- Stub keepers ---------------------------------------------------------

type stubPolicy struct {
	deny    bool
	denyErr string
	calls   int
}

func (s *stubPolicy) EvaluateTransfer(_ sdk.Context, _, _, _, _ string, _ uint64) error {
	s.calls++
	if s.deny {
		return errString(s.denyErr)
	}
	return nil
}

type stubSanctions struct{ bad map[string]bool }

func (s *stubSanctions) IsSanctioned(_ sdk.Context, subject string) bool { return s.bad[subject] }

type stubOracle struct {
	value int64
	ts    int64
	ok    bool
}

func (s *stubOracle) GetAggregatedReserve(_ sdk.Context, _ string) (int64, int64, bool) {
	return s.value, s.ts, s.ok
}

type stubAudit struct{ calls []string }

func (s *stubAudit) RecordEACAction(_ sdk.Context, certID uint64, issuerID, action, actor, beneficiary, detail string) {
	s.calls = append(s.calls, strings.Join([]string{itoa(certID), issuerID, action, actor, beneficiary, detail}, "|"))
}

func itoa(u uint64) string { return formatU(u) }

func formatU(u uint64) string {
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

func errString(s string) error { return &simpleErr{s} }

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }

// ---- Constants ------------------------------------------------------------

const (
	authority   = "cosmos126xkl2rxz08nlv6egudef8dxn6c5suxwnsmrjc"
	issuerAdmin = "cosmos1mc62kvf7adulmqcpgynqupl63hfp68lu82qjlk"
	issuerAuth  = "cosmos150ln4xlvs2dg024ueft43un6e3eav7ca2xhhtf"
	holderAlice = "cosmos1tmxy2mearm0zkw88prwm346aqdqns6m8zlxyrg"
	holderBob   = "cosmos19mup59y4ep02x77uy5vjnq8c4k5a044cwlxkmq"
	holderCarol = "cosmos1lrmkh72j5pqd5rav9wy8tzdq3zlmd3ullte2ev"
	issuerDID   = "did:web:irec.example.com:issuers:1"
)

// ---- Setup ----------------------------------------------------------------

type fixture struct {
	k   keeper.Keeper
	ctx sdk.Context
	srv types.MsgServer
	pol *stubPolicy
	san *stubSanctions
	ora *stubOracle
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
	ora := &stubOracle{ok: true, value: 1_000_000, ts: 1_700_000_000}
	au := &stubAudit{}

	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, pol, san, ora, au)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return &fixture{
		k: k, ctx: ctx, srv: keeper.NewMsgServerImpl(k),
		pol: pol, san: san, ora: ora, au: au,
	}
}

func (f *fixture) registerIssuer(t *testing.T) {
	t.Helper()
	if _, err := f.srv.RegisterIssuer(f.ctx, &types.MsgRegisterIssuer{
		Authority:       authority,
		Id:              "irec",
		Did:             issuerDID,
		DisplayName:     "I-REC Operator",
		Kinds:           []int32{int32(types.CertificateKind_CERTIFICATE_KIND_IREC), int32(types.CertificateKind_CERTIFICATE_KIND_NATIVE)},
		IssuerAuthority: issuerAuth,
		Admin:           issuerAdmin,
	}); err != nil {
		t.Fatalf("register issuer: %v", err)
	}
}

func (f *fixture) issueBatch(t *testing.T, units uint64, recipient string) uint64 {
	t.Helper()
	resp, err := f.srv.IssueBatch(f.ctx, &types.MsgIssueBatch{
		IssuerAuthority: issuerAuth,
		IssuerId:        "irec",
		Kind:            types.CertificateKind_CERTIFICATE_KIND_IREC,
		Technology:      types.Technology_TECHNOLOGY_SOLAR,
		ProjectId:       "proj-001",
		DeviceId:        "dev-001",
		GridZone:        "DE-LU",
		HourStart:       1_700_000_000,
		HourEnd:         1_700_000_000 + 3600,
		VintageYear:     2026,
		SourceRegistry:  "I-REC",
		SourceSerial:    "IREC-001-0001",
		Units:           units,
		Recipient:       recipient,
	})
	if err != nil {
		t.Fatalf("issue batch: %v", err)
	}
	return resp.CertificateId
}

// ---------------------------------------------------------------------------
// Issuer lifecycle
// ---------------------------------------------------------------------------

func TestRegisterIssuerOK(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	is, err := f.k.Issuers.Get(f.ctx, "irec")
	if err != nil || is.Status != types.IssuerStatus_ISSUER_STATUS_ACTIVE {
		t.Fatalf("want active issuer, got %+v err=%v", is, err)
	}
}

func TestRegisterIssuerNonAuthority(t *testing.T) {
	f := setup(t)
	_, err := f.srv.RegisterIssuer(f.ctx, &types.MsgRegisterIssuer{
		Authority: holderAlice, Id: "irec", Did: issuerDID,
		DisplayName: "x", IssuerAuthority: issuerAuth, Admin: issuerAdmin,
	})
	if err == nil {
		t.Fatal("expected unauthorized")
	}
}

func TestRegisterIssuerDuplicate(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	_, err := f.srv.RegisterIssuer(f.ctx, &types.MsgRegisterIssuer{
		Authority: authority, Id: "irec", Did: issuerDID,
		DisplayName: "y", IssuerAuthority: issuerAuth, Admin: issuerAdmin,
	})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestSuspendThenIssueRejected(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	if _, err := f.srv.SuspendIssuer(f.ctx, &types.MsgSuspendIssuer{
		Authority: authority, Id: "irec", Reason: "audit",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := f.srv.IssueBatch(f.ctx, &types.MsgIssueBatch{
		IssuerAuthority: issuerAuth, IssuerId: "irec",
		Kind: types.CertificateKind_CERTIFICATE_KIND_IREC, Technology: types.Technology_TECHNOLOGY_SOLAR,
		ProjectId: "p", GridZone: "g", HourStart: 1_700_000_000, HourEnd: 1_700_003_600,
		VintageYear: 2026, Units: 10, Recipient: holderAlice,
	})
	if err == nil || !strings.Contains(err.Error(), "ACTIVE") {
		t.Fatalf("expected suspended issuer error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Issue & balance
// ---------------------------------------------------------------------------

func TestIssueBatchHappyPath(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 100, holderAlice)
	bal, err := f.k.GetBalance(f.ctx, id, holderAlice)
	if err != nil {
		t.Fatal(err)
	}
	if bal != 100 {
		t.Fatalf("bal want 100 got %d", bal)
	}
	cert, err := f.k.Certificates.Get(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if cert.IssuedUnits != 100 || cert.RetiredUnits != 0 {
		t.Fatalf("cert want issued=100 retired=0, got %+v", cert)
	}
}

func TestIssueBatchKindNotAllowed(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.RegisterIssuer(f.ctx, &types.MsgRegisterIssuer{
		Authority: authority, Id: "irec", Did: issuerDID,
		DisplayName: "I-REC", Kinds: []int32{int32(types.CertificateKind_CERTIFICATE_KIND_IREC)},
		IssuerAuthority: issuerAuth, Admin: issuerAdmin,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := f.srv.IssueBatch(f.ctx, &types.MsgIssueBatch{
		IssuerAuthority: issuerAuth, IssuerId: "irec",
		Kind: types.CertificateKind_CERTIFICATE_KIND_EU_GO, Technology: types.Technology_TECHNOLOGY_SOLAR,
		ProjectId: "p", GridZone: "g", HourStart: 1_700_000_000, HourEnd: 1_700_003_600,
		VintageYear: 2026, Units: 10, Recipient: holderAlice,
	})
	if err == nil || !strings.Contains(err.Error(), "not authorised") {
		t.Fatalf("expected kind disallowed, got %v", err)
	}
}

func TestIssueBatchHourWindowEnforced(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	_, err := f.srv.IssueBatch(f.ctx, &types.MsgIssueBatch{
		IssuerAuthority: issuerAuth, IssuerId: "irec",
		Kind: types.CertificateKind_CERTIFICATE_KIND_IREC, Technology: types.Technology_TECHNOLOGY_SOLAR,
		ProjectId: "p", GridZone: "g",
		HourStart: 1_700_000_000, HourEnd: 1_700_000_000 + 1800, // 30 minutes
		VintageYear: 2026, Units: 10, Recipient: holderAlice,
	})
	if err == nil || !strings.Contains(err.Error(), "hour window") {
		t.Fatalf("expected hour window error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Transfer
// ---------------------------------------------------------------------------

func TestTransferHappyPath(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 100, holderAlice)
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, CertificateId: id, Units: 30,
	}); err != nil {
		t.Fatal(err)
	}
	a, _ := f.k.GetBalance(f.ctx, id, holderAlice)
	b, _ := f.k.GetBalance(f.ctx, id, holderBob)
	if a != 70 || b != 30 {
		t.Fatalf("alice=%d bob=%d, want 70/30", a, b)
	}
}

func TestTransferRejectedWhenSealed(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 100, holderAlice)
	if _, err := f.srv.SealCertificate(f.ctx, &types.MsgSealCertificate{
		Admin: issuerAdmin, CertificateId: id, Reason: "dispute",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, CertificateId: id, Units: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "not transferable") {
		t.Fatalf("expected sealed-block error, got %v", err)
	}
}

func TestTransferRejectedSanctioned(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 100, holderAlice)
	f.san.bad[holderBob] = true
	_, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, CertificateId: id, Units: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "sanctions") {
		t.Fatalf("expected sanctions block, got %v", err)
	}
}

func TestTransferRejectedByPolicy(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 100, holderAlice)
	f.pol.deny = true
	f.pol.denyErr = "policy says no"
	_, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, CertificateId: id, Units: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "policy") {
		t.Fatalf("expected policy block, got %v", err)
	}
}

func TestTransferUnderflow(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 10, holderAlice)
	_, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, CertificateId: id, Units: 11,
	})
	if err == nil {
		t.Fatal("expected underflow")
	}
}

// ---------------------------------------------------------------------------
// Retire
// ---------------------------------------------------------------------------

func TestRetireHappyPath(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 100, holderAlice)
	resp, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderAlice, CertificateId: id, Units: 40, Purpose: "scope2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.RetirementId == 0 {
		t.Fatal("retirement id 0")
	}
	cert, _ := f.k.Certificates.Get(f.ctx, id)
	if cert.RetiredUnits != 40 {
		t.Fatalf("retired want 40 got %d", cert.RetiredUnits)
	}
	bal, _ := f.k.GetBalance(f.ctx, id, holderAlice)
	if bal != 60 {
		t.Fatalf("bal want 60 got %d", bal)
	}
	r, err := f.k.Retirements.Get(f.ctx, resp.RetirementId)
	if err != nil {
		t.Fatal(err)
	}
	if r.Beneficiary != holderAlice || r.Amount != 40 || r.Purpose != "scope2" {
		t.Fatalf("retirement bad: %+v", r)
	}
}

func TestProxyRetireBeneficiaryDifferent(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 50, holderAlice)
	resp, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderAlice, Beneficiary: holderCarol,
		CertificateId: id, Units: 10, Purpose: "rggi",
	})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := f.k.Retirements.Get(f.ctx, resp.RetirementId)
	if r.Retirer != holderAlice || r.Beneficiary != holderCarol {
		t.Fatalf("proxy retire not recorded: %+v", r)
	}
}

func TestRetireFullyRetiresStatus(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 5, holderAlice)
	if _, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderAlice, CertificateId: id, Units: 5, Purpose: "scope2",
	}); err != nil {
		t.Fatal(err)
	}
	cert, _ := f.k.Certificates.Get(f.ctx, id)
	if cert.Status != types.CertificateStatus_CERTIFICATE_STATUS_FULLY_RETIRED {
		t.Fatalf("want FULLY_RETIRED, got %s", cert.Status)
	}
	_, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderAlice, CertificateId: id, Units: 1, Purpose: "scope2",
	})
	if err == nil {
		t.Fatal("expected fully retired block")
	}
}

func TestRetireWhileSealedAllowed(t *testing.T) {
	// Sealed certs MUST still be retirable so holders can liquidate
	// compliance positions even during a freeze.
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 10, holderAlice)
	if _, err := f.srv.SealCertificate(f.ctx, &types.MsgSealCertificate{
		Admin: issuerAdmin, CertificateId: id, Reason: "x",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderAlice, CertificateId: id, Units: 3, Purpose: "scope2",
	}); err != nil {
		t.Fatalf("retire while sealed should succeed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Bridge mint / burn
// ---------------------------------------------------------------------------

func setupBridge(t *testing.T, f *fixture, certID uint64, maxStale uint32) {
	t.Helper()
	if _, err := f.srv.SetBridgeAttestation(f.ctx, &types.MsgSetBridgeAttestation{
		Authority: authority, CertificateId: certID,
		OracleTopicId: "irec.bridge.001", MaxStalenessSeconds: maxStale,
	}); err != nil {
		t.Fatalf("set bridge: %v", err)
	}
}

func TestBridgeMintHappyPath(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 100, holderAlice)
	setupBridge(t, f, id, 3600)
	f.ora.value = 200 // attested locked count
	resp, err := f.srv.BridgeMint(f.ctx, &types.MsgBridgeMint{
		IssuerAuthority: issuerAuth, CertificateId: id, Units: 50, Recipient: holderBob,
	})
	if err != nil {
		t.Fatalf("bridge mint: %v", err)
	}
	if resp.NewIssuedUnits != 150 {
		t.Fatalf("issued want 150 got %d", resp.NewIssuedUnits)
	}
	bob, _ := f.k.GetBalance(f.ctx, id, holderBob)
	if bob != 50 {
		t.Fatalf("bob want 50 got %d", bob)
	}
}

func TestBridgeMintExceedsAttestation(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 100, holderAlice)
	setupBridge(t, f, id, 3600)
	f.ora.value = 120 // only 120 attested
	_, err := f.srv.BridgeMint(f.ctx, &types.MsgBridgeMint{
		IssuerAuthority: issuerAuth, CertificateId: id, Units: 50, Recipient: holderBob,
	})
	if err == nil || !strings.Contains(err.Error(), "attested") {
		t.Fatalf("expected attestation cap, got %v", err)
	}
}

func TestBridgeMintStaleAttestation(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 10, holderAlice)
	setupBridge(t, f, id, 60)
	f.ora.value = 1000
	f.ora.ts = 1_700_000_000 - 3600 // 1h old, max staleness 60s
	_, err := f.srv.BridgeMint(f.ctx, &types.MsgBridgeMint{
		IssuerAuthority: issuerAuth, CertificateId: id, Units: 5, Recipient: holderBob,
	})
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expected stale-attestation block, got %v", err)
	}
}

func TestBridgeMintMissingAttestation(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 10, holderAlice)
	// no SetBridgeAttestation
	_, err := f.srv.BridgeMint(f.ctx, &types.MsgBridgeMint{
		IssuerAuthority: issuerAuth, CertificateId: id, Units: 5, Recipient: holderBob,
	})
	if err == nil || !strings.Contains(err.Error(), "no bridge attestation") {
		t.Fatalf("expected missing attestation, got %v", err)
	}
}

func TestBridgeBurnRequiresAttestation(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 10, holderAlice)
	_, err := f.srv.BridgeBurn(f.ctx, &types.MsgBridgeBurn{
		Holder: holderAlice, CertificateId: id, Units: 1, ExternalRecipient: "ext-account-1",
	})
	if err == nil || !strings.Contains(err.Error(), "no bridge attestation") {
		t.Fatalf("expected attestation-required block, got %v", err)
	}
}

func TestBridgeBurnHappyPath(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 10, holderAlice)
	setupBridge(t, f, id, 3600)
	if _, err := f.srv.BridgeBurn(f.ctx, &types.MsgBridgeBurn{
		Holder: holderAlice, CertificateId: id, Units: 4, ExternalRecipient: "ext-account-1",
	}); err != nil {
		t.Fatal(err)
	}
	cert, _ := f.k.Certificates.Get(f.ctx, id)
	if cert.IssuedUnits != 6 {
		t.Fatalf("issued want 6 got %d", cert.IssuedUnits)
	}
	bal, _ := f.k.GetBalance(f.ctx, id, holderAlice)
	if bal != 6 {
		t.Fatalf("bal want 6 got %d", bal)
	}
}

func TestBridgeBurnRespectsRetiredFloor(t *testing.T) {
	// Cannot burn so much that issued < retired.
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 10, holderAlice)
	setupBridge(t, f, id, 3600)
	if _, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderAlice, CertificateId: id, Units: 6, Purpose: "scope2",
	}); err != nil {
		t.Fatal(err)
	}
	// 4 left in alice's balance, 6 retired. Burning 4 leaves issued=6,
	// equal to retired which is fine. Burning 5 would push issued < retired.
	_, err := f.srv.BridgeBurn(f.ctx, &types.MsgBridgeBurn{
		Holder: holderAlice, CertificateId: id, Units: 5, ExternalRecipient: "ext-1",
	})
	if err == nil {
		t.Fatal("expected balance underflow")
	}
}

// ---------------------------------------------------------------------------
// Genesis round-trip
// ---------------------------------------------------------------------------

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 50, holderAlice)
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, CertificateId: id, Units: 10,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderBob, CertificateId: id, Units: 3, Purpose: "scope2",
	}); err != nil {
		t.Fatal(err)
	}
	setupBridge(t, f, id, 3600)

	gs, err := f.k.ExportGenesis(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}

	g := setup(t)
	if err := g.k.InitGenesis(g.ctx, gs); err != nil {
		t.Fatalf("init: %v", err)
	}
	bal, _ := g.k.GetBalance(g.ctx, id, holderAlice)
	if bal != 40 {
		t.Fatalf("alice want 40 got %d", bal)
	}
	bal, _ = g.k.GetBalance(g.ctx, id, holderBob)
	if bal != 7 {
		t.Fatalf("bob want 7 got %d", bal)
	}
}

// ---------------------------------------------------------------------------
// Defense-in-depth fixes
// ---------------------------------------------------------------------------

func TestIssueBatchSourceSerialUniqueness(t *testing.T) {
	// Two batches sharing (kind, registry, serial) — second must be rejected.
	f := setup(t)
	f.registerIssuer(t)
	if _, err := f.srv.IssueBatch(f.ctx, &types.MsgIssueBatch{
		IssuerAuthority: issuerAuth, IssuerId: "irec",
		Kind: types.CertificateKind_CERTIFICATE_KIND_IREC, Technology: types.Technology_TECHNOLOGY_SOLAR,
		ProjectId: "p", GridZone: "g", HourStart: 1_700_000_000, HourEnd: 1_700_003_600,
		VintageYear: 2026, SourceRegistry: "I-REC", SourceSerial: "IREC-X-1",
		Units: 10, Recipient: holderAlice,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := f.srv.IssueBatch(f.ctx, &types.MsgIssueBatch{
		IssuerAuthority: issuerAuth, IssuerId: "irec",
		Kind: types.CertificateKind_CERTIFICATE_KIND_IREC, Technology: types.Technology_TECHNOLOGY_SOLAR,
		ProjectId: "p2", GridZone: "g", HourStart: 1_700_000_000, HourEnd: 1_700_003_600,
		VintageYear: 2026, SourceRegistry: "I-REC", SourceSerial: "IREC-X-1",
		Units: 5, Recipient: holderBob,
	})
	if err == nil || !strings.Contains(err.Error(), "already issued") {
		t.Fatalf("expected duplicate-serial rejection, got %v", err)
	}
}

func TestIssueBatchEmptySerialNoUniquenessConstraint(t *testing.T) {
	// NATIVE kind with empty serial — multiple batches must be allowed.
	f := setup(t)
	f.registerIssuer(t)
	for i := 0; i < 2; i++ {
		if _, err := f.srv.IssueBatch(f.ctx, &types.MsgIssueBatch{
			IssuerAuthority: issuerAuth, IssuerId: "irec",
			Kind: types.CertificateKind_CERTIFICATE_KIND_NATIVE, Technology: types.Technology_TECHNOLOGY_SOLAR,
			ProjectId: "native-1", GridZone: "g", HourStart: 1_700_000_000, HourEnd: 1_700_003_600,
			VintageYear: 2026, Units: 1, Recipient: holderAlice,
		}); err != nil {
			t.Fatalf("batch %d: %v", i, err)
		}
	}
}

func TestIssueBatchRejectsSanctionedRecipient(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	f.san.bad[holderAlice] = true
	_, err := f.srv.IssueBatch(f.ctx, &types.MsgIssueBatch{
		IssuerAuthority: issuerAuth, IssuerId: "irec",
		Kind: types.CertificateKind_CERTIFICATE_KIND_IREC, Technology: types.Technology_TECHNOLOGY_SOLAR,
		ProjectId: "p", GridZone: "g", HourStart: 1_700_000_000, HourEnd: 1_700_003_600,
		VintageYear: 2026, Units: 1, Recipient: holderAlice,
	})
	if err == nil || !strings.Contains(err.Error(), "sanctions") {
		t.Fatalf("expected recipient sanctions block, got %v", err)
	}
}

func TestBridgeMintRejectsSanctionedRecipient(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueBatch(t, 10, holderAlice)
	setupBridge(t, f, id, 3600)
	f.san.bad[holderBob] = true
	_, err := f.srv.BridgeMint(f.ctx, &types.MsgBridgeMint{
		IssuerAuthority: issuerAuth, CertificateId: id, Units: 5, Recipient: holderBob,
	})
	if err == nil || !strings.Contains(err.Error(), "sanctions") {
		t.Fatalf("expected recipient sanctions block, got %v", err)
	}
}

func TestGenesisInvariantBalanceVsCert(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Issuers = []types.Issuer{{
		Id: "irec", Did: issuerDID, DisplayName: "x",
		Status: types.IssuerStatus_ISSUER_STATUS_ACTIVE,
		Authority: issuerAuth, Admin: issuerAdmin,
	}}
	gs.Certificates = []types.Certificate{{
		Id: 1, IssuerId: "irec",
		Kind: types.CertificateKind_CERTIFICATE_KIND_IREC, Technology: types.Technology_TECHNOLOGY_SOLAR,
		ProjectId: "p", GridZone: "g", HourStart: 1, HourEnd: 2, VintageYear: 2026,
		IssuedUnits: 100, RetiredUnits: 10,
		Status: types.CertificateStatus_CERTIFICATE_STATUS_ACTIVE,
	}}
	// sum(balances) should be 90 = issued-retired; we set 80 to fail.
	gs.Balances = []types.Balance{
		{CertificateId: 1, Account: holderAlice, Amount: 80},
	}
	gs.CertificateIdSeq = 1
	if err := gs.Validate(); err == nil {
		t.Fatal("expected sum(balances) != issued-retired to fail")
	}
}
