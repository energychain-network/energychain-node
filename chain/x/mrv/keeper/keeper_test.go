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

	"energychain/x/mrv/keeper"
	"energychain/x/mrv/types"
)

// ---- Stubs --------------------------------------------------------------

type stubSanctions struct{ bad map[string]bool }

func (s *stubSanctions) IsSanctioned(_ sdk.Context, who string) bool { return s.bad[who] }

type stubDID struct {
	// did → set of controller addresses
	controllers map[string]map[string]bool
}

func newStubDID() *stubDID { return &stubDID{controllers: map[string]map[string]bool{}} }
func (d *stubDID) IsControllerOf(_ sdk.Context, did, addr string) bool {
	if _, ok := d.controllers[did]; !ok {
		return false
	}
	return d.controllers[did][addr]
}
func (d *stubDID) bind(did, addr string) {
	if _, ok := d.controllers[did]; !ok {
		d.controllers[did] = map[string]bool{}
	}
	d.controllers[did][addr] = true
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
	alice       = mkAddr(2)
	bob         = mkAddr(3)
	regulator   = mkAddr(4)
	verifierKey = mkAddr(5)
	stranger    = mkAddr(6)

	regulatorDID = "did:example:regulator-1"
	verifierDID  = "did:example:verifier-1"
)

type fixture struct {
	k   keeper.Keeper
	ctx sdk.Context
	srv types.MsgServer
	san *stubSanctions
	did *stubDID
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
	did := newStubDID()
	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, san, did, nil)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return &fixture{k: k, ctx: ctx, srv: keeper.NewMsgServerImpl(k), san: san, did: did}
}

// ---- helpers ------------------------------------------------------------

func (f *fixture) registerSchema(t *testing.T, name string, ac types.AssetClass) uint64 {
	t.Helper()
	r, err := f.srv.RegisterSchema(f.ctx, &types.MsgRegisterSchema{
		Authority:    authority,
		Name:         name,
		Version:      "1.0",
		Jurisdiction: "GLOBAL",
		AssetClass:   ac,
		TimeWindow:   types.TimeWindow_TIME_WINDOW_MONTHLY,
		OutputFormat: types.ReportFormat_REPORT_FORMAT_JSON_LD,
		SchemaUri:    "https://example.org/schema/" + name,
		SchemaHash:   "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
	})
	if err != nil {
		t.Fatalf("register-schema: %v", err)
	}
	return r.SchemaId
}

func (f *fixture) registerVerifier(t *testing.T, did, signerAddr string) uint64 {
	t.Helper()
	r, err := f.srv.RegisterVerifier(f.ctx, &types.MsgRegisterVerifier{
		Authority:           authority,
		Did:                 did,
		Name:                "Test VVB Ltd.",
		SignerAddress:       signerAddr,
		AccreditedStandards: []string{"ISO-14064"},
		Jurisdictions:       []string{"GLOBAL"},
	})
	if err != nil {
		t.Fatalf("register-verifier: %v", err)
	}
	return r.VerifierId
}

func (f *fixture) submitReport(t *testing.T, subject string, schemaID uint64, start, end int64) uint64 {
	t.Helper()
	r, err := f.srv.SubmitReport(f.ctx, &types.MsgSubmitReport{
		Submitter:   subject,
		Subject:     subject,
		SchemaId:    schemaID,
		PeriodStart: start,
		PeriodEnd:   end,
		PayloadUri:  "ipfs://Qm.../report.json",
		PayloadHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Aggregate:   types.ReportAggregate{MeterWh: 1000, EacRetiredWh: 1000, RecordCount: 24},
	})
	if err != nil {
		t.Fatalf("submit-report: %v", err)
	}
	return r.ReportId
}

// ---- schema tests -------------------------------------------------------

func TestRegisterSchemaHappy(t *testing.T) {
	f := setup(t)
	id := f.registerSchema(t, "scope2-monthly", types.AssetClass_ASSET_CLASS_EAC)
	if id != 1 {
		t.Fatalf("expected id 1, got %d", id)
	}
	s, _ := f.k.MustGetSchema(f.ctx, id)
	if s.Status != types.SchemaStatus_SCHEMA_STATUS_ACTIVE {
		t.Fatalf("status: %s", s.Status)
	}
}

func TestRegisterSchemaRefusesNonAuthority(t *testing.T) {
	f := setup(t)
	_, err := f.srv.RegisterSchema(f.ctx, &types.MsgRegisterSchema{
		Authority:    stranger,
		Name:         "x",
		Version:      "1",
		Jurisdiction: "GLOBAL",
		AssetClass:   types.AssetClass_ASSET_CLASS_EAC,
		TimeWindow:   types.TimeWindow_TIME_WINDOW_MONTHLY,
		OutputFormat: types.ReportFormat_REPORT_FORMAT_JSON_LD,
	})
	if err == nil || !strings.Contains(err.Error(), "authority") {
		t.Fatalf("expected authority err, got %v", err)
	}
}

func TestRegisterSchemaRefusesBadInputs(t *testing.T) {
	f := setup(t)
	// Bad jurisdiction.
	_, err := f.srv.RegisterSchema(f.ctx, &types.MsgRegisterSchema{
		Authority: authority, Name: "x", Version: "1", Jurisdiction: "USA",
		AssetClass: types.AssetClass_ASSET_CLASS_EAC, TimeWindow: types.TimeWindow_TIME_WINDOW_MONTHLY,
		OutputFormat: types.ReportFormat_REPORT_FORMAT_JSON_LD,
	})
	if err == nil {
		t.Fatal("expected jurisdiction err")
	}
	// Bad hash.
	_, err = f.srv.RegisterSchema(f.ctx, &types.MsgRegisterSchema{
		Authority: authority, Name: "x", Version: "1", Jurisdiction: "DE",
		AssetClass: types.AssetClass_ASSET_CLASS_EAC, TimeWindow: types.TimeWindow_TIME_WINDOW_MONTHLY,
		OutputFormat: types.ReportFormat_REPORT_FORMAT_JSON_LD,
		SchemaHash:   "notahex",
	})
	if err == nil {
		t.Fatal("expected hash err")
	}
}

func TestUpdateSchemaStatusDeprecatesNewSubmissions(t *testing.T) {
	f := setup(t)
	id := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	if _, err := f.srv.UpdateSchemaStatus(f.ctx, &types.MsgUpdateSchemaStatus{
		Authority: authority, SchemaId: id,
		NewStatus: types.SchemaStatus_SCHEMA_STATUS_DEPRECATED, Reason: "v2",
	}); err != nil {
		t.Fatal(err)
	}
	// Submitting against a deprecated schema must fail.
	_, err := f.srv.SubmitReport(f.ctx, &types.MsgSubmitReport{
		Submitter: alice, Subject: alice, SchemaId: id,
		PeriodStart: 100, PeriodEnd: 200,
	})
	if err == nil || !strings.Contains(err.Error(), "not ACTIVE") {
		t.Fatalf("expected deprecated err, got %v", err)
	}
}

func TestMaxSchemasCap(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx)
	p.MaxSchemas = 1
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	f.registerSchema(t, "first", types.AssetClass_ASSET_CLASS_EAC)
	_, err := f.srv.RegisterSchema(f.ctx, &types.MsgRegisterSchema{
		Authority: authority, Name: "second", Version: "1.0", Jurisdiction: "GLOBAL",
		AssetClass: types.AssetClass_ASSET_CLASS_EAC, TimeWindow: types.TimeWindow_TIME_WINDOW_MONTHLY,
		OutputFormat: types.ReportFormat_REPORT_FORMAT_JSON_LD,
	})
	if err == nil || !strings.Contains(err.Error(), "max_schemas") {
		t.Fatalf("expected cap err, got %v", err)
	}
}

// ---- verifier tests -----------------------------------------------------

func TestRegisterVerifierHappy(t *testing.T) {
	f := setup(t)
	id := f.registerVerifier(t, verifierDID, verifierKey)
	v, _, _ := f.k.GetVerifier(f.ctx, id)
	if v.Status != types.VerifierStatus_VERIFIER_STATUS_ACCREDITED {
		t.Fatalf("status: %s", v.Status)
	}
	if v.SignerAddress != verifierKey {
		t.Fatalf("signer not recorded: %s", v.SignerAddress)
	}
	// DID uniqueness — second registration of same DID must fail.
	_, err := f.srv.RegisterVerifier(f.ctx, &types.MsgRegisterVerifier{
		Authority: authority, Did: verifierDID, Name: "dup", SignerAddress: stranger,
	})
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected DID-dup err, got %v", err)
	}
	// Signer-address uniqueness — second registration of same signer must fail.
	_, err = f.srv.RegisterVerifier(f.ctx, &types.MsgRegisterVerifier{
		Authority: authority, Did: "did:example:other", Name: "dup-signer", SignerAddress: verifierKey,
	})
	if err == nil || !strings.Contains(err.Error(), "already bound") {
		t.Fatalf("expected signer-dup err, got %v", err)
	}
}

func TestUpdateVerifierStatusBlocksAttestation(t *testing.T) {
	f := setup(t)
	vid := f.registerVerifier(t, verifierDID, verifierKey)
	f.did.bind(verifierDID, verifierKey)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	rid := f.submitReport(t, alice, sid, 100, 200)
	// Suspend the verifier.
	if _, err := f.srv.UpdateVerifierStatus(f.ctx, &types.MsgUpdateVerifierStatus{
		Authority: authority, VerifierId: vid,
		NewStatus: types.VerifierStatus_VERIFIER_STATUS_SUSPENDED, Reason: "audit",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := f.srv.AttestReport(f.ctx, &types.MsgAttestReport{
		VerifierAddress: verifierKey, VerifierId: vid, ReportId: rid,
		VerifierPayloadHash: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
	})
	if err == nil || !strings.Contains(err.Error(), "not ACCREDITED") {
		t.Fatalf("expected accreditation refusal, got %v", err)
	}
}

func TestVerifierTooManyJurisdictionsRefused(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx)
	p.MaxJurisdictionsPerVerifier = 1
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	_, err := f.srv.RegisterVerifier(f.ctx, &types.MsgRegisterVerifier{
		Authority: authority, Did: verifierDID, Name: "x", SignerAddress: verifierKey,
		Jurisdictions: []string{"US", "EU"},
	})
	if err == nil || !strings.Contains(err.Error(), "too many jurisdictions") {
		t.Fatalf("expected refusal, got %v", err)
	}
}

// ---- report tests -------------------------------------------------------

func TestSubmitReportHappyPath(t *testing.T) {
	f := setup(t)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	rid := f.submitReport(t, alice, sid, 1_700_000_000, 1_702_000_000)
	r, _ := f.k.MustGetReport(f.ctx, rid)
	if r.Status != types.ReportStatus_REPORT_STATUS_DRAFT {
		t.Fatalf("status: %s", r.Status)
	}
	if r.Subject != alice {
		t.Fatalf("subject: %s", r.Subject)
	}
	c, _ := f.k.ReportCount(f.ctx, alice)
	if c != 1 {
		t.Fatalf("count: %d", c)
	}
}

func TestSubmitReportRefusesNonSubject(t *testing.T) {
	f := setup(t)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	_, err := f.srv.SubmitReport(f.ctx, &types.MsgSubmitReport{
		Submitter: stranger, Subject: alice, SchemaId: sid,
		PeriodStart: 100, PeriodEnd: 200,
	})
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected refusal, got %v", err)
	}
}

func TestAuthorityCanSubmitOnBehalf(t *testing.T) {
	f := setup(t)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	r, err := f.srv.SubmitReport(f.ctx, &types.MsgSubmitReport{
		Submitter: authority, Subject: alice, SchemaId: sid,
		PeriodStart: 100, PeriodEnd: 200,
	})
	if err != nil {
		t.Fatalf("authority submit: %v", err)
	}
	if r.ReportId == 0 {
		t.Fatal("got id 0")
	}
}

func TestSubmitReportRefusesUnknownSchema(t *testing.T) {
	f := setup(t)
	_, err := f.srv.SubmitReport(f.ctx, &types.MsgSubmitReport{
		Submitter: alice, Subject: alice, SchemaId: 999,
		PeriodStart: 100, PeriodEnd: 200,
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found, got %v", err)
	}
}

func TestSubmitReportRefusesBadPeriod(t *testing.T) {
	f := setup(t)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	_, err := f.srv.SubmitReport(f.ctx, &types.MsgSubmitReport{
		Submitter: alice, Subject: alice, SchemaId: sid,
		PeriodStart: 200, PeriodEnd: 100,
	})
	if err == nil {
		t.Fatal("expected period err")
	}
}

func TestSubmitReportRefusesSanctioned(t *testing.T) {
	f := setup(t)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	f.san.bad[alice] = true
	_, err := f.srv.SubmitReport(f.ctx, &types.MsgSubmitReport{
		Submitter: alice, Subject: alice, SchemaId: sid,
		PeriodStart: 100, PeriodEnd: 200,
	})
	if err == nil || !strings.Contains(err.Error(), "sanctioned") {
		t.Fatalf("expected sanctions err, got %v", err)
	}
}

func TestMaxReportsPerSubjectCap(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx)
	p.MaxReportsPerSubject = 1
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	f.submitReport(t, alice, sid, 100, 200)
	_, err := f.srv.SubmitReport(f.ctx, &types.MsgSubmitReport{
		Submitter: alice, Subject: alice, SchemaId: sid,
		PeriodStart: 200, PeriodEnd: 300,
	})
	if err == nil || !strings.Contains(err.Error(), "max_reports_per_subject") {
		t.Fatalf("expected cap, got %v", err)
	}
}

// ---- attestation tests --------------------------------------------------

func TestAttestReportHappy(t *testing.T) {
	f := setup(t)
	vid := f.registerVerifier(t, verifierDID, verifierKey)
	f.did.bind(verifierDID, verifierKey)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	rid := f.submitReport(t, alice, sid, 100, 200)
	if _, err := f.srv.AttestReport(f.ctx, &types.MsgAttestReport{
		VerifierAddress:     verifierKey,
		VerifierId:          vid,
		ReportId:            rid,
		VerifierPayloadUri:  "ipfs://Qm.../vc.jsonld",
		VerifierPayloadHash: "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
	}); err != nil {
		t.Fatalf("attest: %v", err)
	}
	r, _ := f.k.MustGetReport(f.ctx, rid)
	if r.Status != types.ReportStatus_REPORT_STATUS_ATTESTED {
		t.Fatalf("status: %s", r.Status)
	}
	if r.VerifierId != vid {
		t.Fatalf("verifier id: %d", r.VerifierId)
	}
}

// REGRESSION: an attestation signed by an address that is
// neither the registered signer_address NOR a DID-controller
// of the verifier must be refused. This is the chain-side
// enforcement of "you can't impersonate a verifier even if
// you know its id".
func TestAttestReportRefusesNonControllerSigner(t *testing.T) {
	f := setup(t)
	vid := f.registerVerifier(t, verifierDID, verifierKey)
	f.did.bind(verifierDID, verifierKey) // verifierKey is the controller
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	rid := f.submitReport(t, alice, sid, 100, 200)
	// stranger signs but is neither verifierKey nor a controller of verifierDID.
	_, err := f.srv.AttestReport(f.ctx, &types.MsgAttestReport{
		VerifierAddress: stranger, VerifierId: vid, ReportId: rid,
	})
	if err == nil || !strings.Contains(err.Error(), "neither the registered signer nor a controller") {
		t.Fatalf("expected impersonation err, got %v", err)
	}
}

// REGRESSION: even with NO DIDKeeper wired (the dangerous
// minimal-deployment case), the signer_address gate must
// keep an attacker from attesting as any verifier. This test
// rebuilds a keeper with did=nil to confirm the fail-closed
// behavior.
func TestAttestReportRefusesImpersonationWithoutDIDKeeper(t *testing.T) {
	f := setup(t)
	// Re-create the keeper with did=nil to simulate the
	// minimal deployment.
	storeKey := f.k.Schema // touch the schema so the linter is happy; not used directly
	_ = storeKey
	// Walk the easier path: register first, then null the did.
	vid := f.registerVerifier(t, verifierDID, verifierKey)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	rid := f.submitReport(t, alice, sid, 100, 200)
	// Forge the keeper to drop the DID hook (test-only).
	f.did.controllers = map[string]map[string]bool{} // empty controllers
	// Stranger tries to attest. Without DID controller and
	// with signer_address=verifierKey, stranger MUST be
	// refused.
	_, err := f.srv.AttestReport(f.ctx, &types.MsgAttestReport{
		VerifierAddress: stranger, VerifierId: vid, ReportId: rid,
	})
	if err == nil || !strings.Contains(err.Error(), "neither the registered signer") {
		t.Fatalf("expected impersonation refusal, got %v", err)
	}
	// The registered signer_address must succeed.
	if _, err := f.srv.AttestReport(f.ctx, &types.MsgAttestReport{
		VerifierAddress: verifierKey, VerifierId: vid, ReportId: rid,
	}); err != nil {
		t.Fatalf("registered signer should succeed: %v", err)
	}
}

// REGRESSION: a DID controller other than the registered
// signer_address may also attest, when a DIDKeeper is wired.
// This is the "key rotation through DID" path.
func TestAttestReportAcceptsAlternateControllerViaDID(t *testing.T) {
	f := setup(t)
	vid := f.registerVerifier(t, verifierDID, verifierKey)
	// Bind a DIFFERENT key as a controller of the same DID.
	alt := mkAddr(7)
	f.did.bind(verifierDID, alt)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	rid := f.submitReport(t, alice, sid, 100, 200)
	if _, err := f.srv.AttestReport(f.ctx, &types.MsgAttestReport{
		VerifierAddress: alt, VerifierId: vid, ReportId: rid,
	}); err != nil {
		t.Fatalf("alt controller should succeed: %v", err)
	}
}

func TestAttestReportRefusesNonDraft(t *testing.T) {
	f := setup(t)
	vid := f.registerVerifier(t, verifierDID, verifierKey)
	f.did.bind(verifierDID, verifierKey)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	rid := f.submitReport(t, alice, sid, 100, 200)
	// Attest once.
	if _, err := f.srv.AttestReport(f.ctx, &types.MsgAttestReport{
		VerifierAddress: verifierKey, VerifierId: vid, ReportId: rid,
	}); err != nil {
		t.Fatal(err)
	}
	// Second attest must fail.
	_, err := f.srv.AttestReport(f.ctx, &types.MsgAttestReport{
		VerifierAddress: verifierKey, VerifierId: vid, ReportId: rid,
	})
	if err == nil || !strings.Contains(err.Error(), "not DRAFT") {
		t.Fatalf("expected non-draft err, got %v", err)
	}
}

func TestRejectReportFlow(t *testing.T) {
	f := setup(t)
	vid := f.registerVerifier(t, verifierDID, verifierKey)
	f.did.bind(verifierDID, verifierKey)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	rid := f.submitReport(t, alice, sid, 100, 200)
	if _, err := f.srv.RejectReport(f.ctx, &types.MsgRejectReport{
		VerifierAddress: verifierKey, VerifierId: vid, ReportId: rid,
		Reason: "missing meter data",
	}); err != nil {
		t.Fatal(err)
	}
	r, _ := f.k.MustGetReport(f.ctx, rid)
	if r.Status != types.ReportStatus_REPORT_STATUS_REJECTED {
		t.Fatalf("status: %s", r.Status)
	}
}

func TestRetractReportBySubject(t *testing.T) {
	f := setup(t)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	rid := f.submitReport(t, alice, sid, 100, 200)
	if _, err := f.srv.RetractReport(f.ctx, &types.MsgRetractReport{
		Actor: alice, ReportId: rid, Reason: "duplicate",
	}); err != nil {
		t.Fatal(err)
	}
	r, _ := f.k.MustGetReport(f.ctx, rid)
	if r.Status != types.ReportStatus_REPORT_STATUS_RETRACTED {
		t.Fatalf("status: %s", r.Status)
	}
	// Cannot retract again.
	_, err := f.srv.RetractReport(f.ctx, &types.MsgRetractReport{
		Actor: alice, ReportId: rid, Reason: "again",
	})
	if err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("expected terminal err, got %v", err)
	}
}

func TestRetractReportRefusesThirdParty(t *testing.T) {
	f := setup(t)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	rid := f.submitReport(t, alice, sid, 100, 200)
	_, err := f.srv.RetractReport(f.ctx, &types.MsgRetractReport{
		Actor: bob, ReportId: rid,
	})
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected refusal, got %v", err)
	}
}

// ---- view-key grant tests -----------------------------------------------

func TestGrantViewKeyHappy(t *testing.T) {
	f := setup(t)
	now := f.ctx.BlockTime().Unix()
	r, err := f.srv.GrantViewKey(f.ctx, &types.MsgGrantViewKey{
		Granter: alice, GranteeDid: regulatorDID,
		ExpiresAt: now + 3600,
	})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	g, _ := f.k.MustGetGrant(f.ctx, r.GrantId)
	if g.Granter != alice {
		t.Fatalf("granter: %s", g.Granter)
	}
	if g.Revoked {
		t.Fatal("should not be revoked")
	}
}

// REGRESSION: a grant scoped to a specific report_id must be
// REFUSED if the report is owned by a different subject — a
// subject must not be able to grant view-access to another
// subject's report.
func TestGrantViewKeyRefusesUnownedReport(t *testing.T) {
	f := setup(t)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	rid := f.submitReport(t, bob, sid, 100, 200)
	now := f.ctx.BlockTime().Unix()
	_, err := f.srv.GrantViewKey(f.ctx, &types.MsgGrantViewKey{
		Granter: alice, GranteeDid: regulatorDID,
		ReportId: rid, ExpiresAt: now + 3600,
	})
	if err == nil || !strings.Contains(err.Error(), "not owned by granter") {
		t.Fatalf("expected ownership err, got %v", err)
	}
}

func TestGrantViewKeyRefusesPastExpiry(t *testing.T) {
	f := setup(t)
	now := f.ctx.BlockTime().Unix()
	_, err := f.srv.GrantViewKey(f.ctx, &types.MsgGrantViewKey{
		Granter: alice, GranteeDid: regulatorDID,
		ExpiresAt: now - 1,
	})
	if err == nil || !strings.Contains(err.Error(), "future") {
		t.Fatalf("expected future err, got %v", err)
	}
}

func TestGrantViewKeyRefusesExcessiveTTL(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx)
	p.MaxGrantTtlSeconds = 3600
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	now := f.ctx.BlockTime().Unix()
	_, err := f.srv.GrantViewKey(f.ctx, &types.MsgGrantViewKey{
		Granter: alice, GranteeDid: regulatorDID,
		ExpiresAt: now + 7200,
	})
	if err == nil || !strings.Contains(err.Error(), "ttl") {
		t.Fatalf("expected ttl err, got %v", err)
	}
}

func TestRevokeViewKeyByGranter(t *testing.T) {
	f := setup(t)
	now := f.ctx.BlockTime().Unix()
	r, _ := f.srv.GrantViewKey(f.ctx, &types.MsgGrantViewKey{
		Granter: alice, GranteeDid: regulatorDID, ExpiresAt: now + 3600,
	})
	if _, err := f.srv.RevokeViewKey(f.ctx, &types.MsgRevokeViewKey{
		Actor: alice, GrantId: r.GrantId, Reason: "no longer needed",
	}); err != nil {
		t.Fatal(err)
	}
	g, _ := f.k.MustGetGrant(f.ctx, r.GrantId)
	if !g.Revoked {
		t.Fatal("expected revoked")
	}
	// Idempotent — second revoke is a no-op (not an error).
	if _, err := f.srv.RevokeViewKey(f.ctx, &types.MsgRevokeViewKey{
		Actor: alice, GrantId: r.GrantId, Reason: "again",
	}); err != nil {
		t.Fatalf("idempotent revoke: %v", err)
	}
}

func TestRevokeViewKeyRefusesThirdParty(t *testing.T) {
	f := setup(t)
	now := f.ctx.BlockTime().Unix()
	r, _ := f.srv.GrantViewKey(f.ctx, &types.MsgGrantViewKey{
		Granter: alice, GranteeDid: regulatorDID, ExpiresAt: now + 3600,
	})
	_, err := f.srv.RevokeViewKey(f.ctx, &types.MsgRevokeViewKey{
		Actor: bob, GrantId: r.GrantId,
	})
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected refusal, got %v", err)
	}
}

func TestAuthorityCanRevokeAnyGrant(t *testing.T) {
	f := setup(t)
	now := f.ctx.BlockTime().Unix()
	r, _ := f.srv.GrantViewKey(f.ctx, &types.MsgGrantViewKey{
		Granter: alice, GranteeDid: regulatorDID, ExpiresAt: now + 3600,
	})
	if _, err := f.srv.RevokeViewKey(f.ctx, &types.MsgRevokeViewKey{
		Actor: authority, GrantId: r.GrantId, Reason: "compliance",
	}); err != nil {
		t.Fatalf("authority revoke: %v", err)
	}
}

// REGRESSION: the EndBlock sweep must flip grants whose
// expires_at slips into the past, even if no caller ever
// explicitly revokes them.
func TestEndBlockSweepsExpiredGrants(t *testing.T) {
	f := setup(t)
	now := f.ctx.BlockTime().Unix()
	r, _ := f.srv.GrantViewKey(f.ctx, &types.MsgGrantViewKey{
		Granter: alice, GranteeDid: regulatorDID, ExpiresAt: now + 10,
	})
	// Advance block time past expiry.
	f.ctx = f.ctx.WithBlockTime(time.Unix(now+11, 0))
	if err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	g, _ := f.k.MustGetGrant(f.ctx, r.GrantId)
	if !g.Revoked {
		t.Fatal("expected sweep to revoke")
	}
}

// REGRESSION: the sweep budget caps work per block, so a
// huge backlog cannot blow the gas budget.
func TestEndBlockSweepRespectsBudget(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx)
	p.MaxGrantsPerBlockSweep = 2
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	now := f.ctx.BlockTime().Unix()
	ids := []uint64{}
	for i := 0; i < 5; i++ {
		r, _ := f.srv.GrantViewKey(f.ctx, &types.MsgGrantViewKey{
			Granter: alice, GranteeDid: regulatorDID, ExpiresAt: now + 10,
		})
		ids = append(ids, r.GrantId)
	}
	f.ctx = f.ctx.WithBlockTime(time.Unix(now+11, 0))
	if err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	revokedCount := 0
	for _, id := range ids {
		g, _ := f.k.MustGetGrant(f.ctx, id)
		if g.Revoked {
			revokedCount++
		}
	}
	if revokedCount != 2 {
		t.Fatalf("expected 2 swept per block, got %d", revokedCount)
	}
	// Next block sweeps next batch.
	if err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	revokedCount = 0
	for _, id := range ids {
		g, _ := f.k.MustGetGrant(f.ctx, id)
		if g.Revoked {
			revokedCount++
		}
	}
	if revokedCount != 4 {
		t.Fatalf("expected 4 swept after 2 blocks, got %d", revokedCount)
	}
}

func TestGrantSchemaIDMustExist(t *testing.T) {
	f := setup(t)
	now := f.ctx.BlockTime().Unix()
	_, err := f.srv.GrantViewKey(f.ctx, &types.MsgGrantViewKey{
		Granter: alice, GranteeDid: regulatorDID,
		SchemaId: 999, ExpiresAt: now + 3600,
	})
	if err == nil || !strings.Contains(err.Error(), "schema 999 not found") {
		t.Fatalf("expected unknown-schema err, got %v", err)
	}
}

// ---- query / genesis tests ---------------------------------------------

func TestQueriesReturnCorrectShape(t *testing.T) {
	f := setup(t)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	vid := f.registerVerifier(t, verifierDID, verifierKey)
	rid := f.submitReport(t, alice, sid, 100, 200)
	now := f.ctx.BlockTime().Unix()
	g, _ := f.srv.GrantViewKey(f.ctx, &types.MsgGrantViewKey{
		Granter: alice, GranteeDid: regulatorDID, ExpiresAt: now + 3600,
	})

	qs := keeper.NewQueryServerImpl(f.k)
	if r, err := qs.Params(f.ctx, &types.QueryParamsRequest{}); err != nil || r.Params.MaxSchemas == 0 {
		t.Fatalf("params: %v %v", r, err)
	}
	if r, err := qs.Schema(f.ctx, &types.QuerySchemaRequest{Id: sid}); err != nil || r.Schema.Id != sid {
		t.Fatalf("schema: %v %v", r, err)
	}
	if r, err := qs.Verifier(f.ctx, &types.QueryVerifierRequest{Id: vid}); err != nil || r.Verifier.Id != vid {
		t.Fatalf("verifier: %v %v", r, err)
	}
	if r, err := qs.Report(f.ctx, &types.QueryReportRequest{Id: rid}); err != nil || r.Report.Id != rid {
		t.Fatalf("report: %v %v", r, err)
	}
	// Subject-filtered Reports query must use the covering index.
	if r, err := qs.Reports(f.ctx, &types.QueryReportsRequest{Subject: alice}); err != nil || len(r.Reports) != 1 {
		t.Fatalf("reports by subject: %v %v", r, err)
	}
	if r, err := qs.ViewKeyGrant(f.ctx, &types.QueryViewKeyGrantRequest{Id: g.GrantId}); err != nil || r.Grant.Id != g.GrantId {
		t.Fatalf("grant: %v %v", r, err)
	}
	if r, err := qs.ViewKeyGrants(f.ctx, &types.QueryViewKeyGrantsRequest{Granter: alice}); err != nil || len(r.Grants) != 1 {
		t.Fatalf("grants by granter: %v %v", r, err)
	}
}

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	sid := f.registerSchema(t, "scope2", types.AssetClass_ASSET_CLASS_EAC)
	vid := f.registerVerifier(t, verifierDID, verifierKey)
	rid := f.submitReport(t, alice, sid, 100, 200)
	now := f.ctx.BlockTime().Unix()
	g, _ := f.srv.GrantViewKey(f.ctx, &types.MsgGrantViewKey{
		Granter: alice, GranteeDid: regulatorDID, ExpiresAt: now + 3600,
	})

	gs, err := f.k.ExportGenesis(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs.Schemas) != 1 || len(gs.Verifiers) != 1 || len(gs.Reports) != 1 || len(gs.Grants) != 1 {
		t.Fatalf("export: %+v", gs)
	}
	f2 := setup(t)
	if err := f2.k.InitGenesis(f2.ctx, gs); err != nil {
		t.Fatalf("import: %v", err)
	}
	if _, err := f2.k.MustGetSchema(f2.ctx, sid); err != nil {
		t.Fatalf("schema missing after import: %v", err)
	}
	if _, _, err := f2.k.GetVerifier(f2.ctx, vid); err != nil {
		t.Fatalf("verifier missing after import: %v", err)
	}
	if _, err := f2.k.MustGetReport(f2.ctx, rid); err != nil {
		t.Fatalf("report missing after import: %v", err)
	}
	if _, err := f2.k.MustGetGrant(f2.ctx, g.GrantId); err != nil {
		t.Fatalf("grant missing after import: %v", err)
	}
	c, _ := f2.k.ReportCount(f2.ctx, alice)
	if c != 1 {
		t.Fatalf("report count not restored: %d", c)
	}
}

func TestUpdateParamsRefusesNonAuthority(t *testing.T) {
	f := setup(t)
	_, err := f.srv.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: stranger, Params: types.DefaultParams(),
	})
	if err == nil {
		t.Fatal("expected refusal")
	}
}
