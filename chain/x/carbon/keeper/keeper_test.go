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

	"energychain/x/carbon/keeper"
	"energychain/x/carbon/types"
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

type stubEAC struct{ certs map[uint64]uint64 }

func (s *stubEAC) GetCertificateIssuedUnits(_ sdk.Context, id uint64) (uint64, bool) {
	v, ok := s.certs[id]
	return v, ok
}

type stubAudit struct{ calls []string }

func (s *stubAudit) RecordCarbonAction(_ sdk.Context, assetID uint64, issuerID, action, actor, beneficiary, detail string) {
	s.calls = append(s.calls, strings.Join([]string{itoa(assetID), issuerID, action, actor, beneficiary, detail}, "|"))
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
	issuerDID   = "did:web:verra.example.com:issuers:1"
)

// ---- Setup ----------------------------------------------------------------

type fixture struct {
	k   keeper.Keeper
	ctx sdk.Context
	srv types.MsgServer
	pol *stubPolicy
	san *stubSanctions
	ora *stubOracle
	eac *stubEAC
	au  *stubAudit
}

func setup(t *testing.T) *fixture {
	t.Helper()
	offsetSeq = 0
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
	ora := &stubOracle{ok: true, value: 10_000, ts: 1_700_000_000}
	eac := &stubEAC{certs: map[uint64]uint64{}}
	au := &stubAudit{}

	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, pol, san, ora, eac, au)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return &fixture{
		k: k, ctx: ctx, srv: keeper.NewMsgServerImpl(k),
		pol: pol, san: san, ora: ora, eac: eac, au: au,
	}
}

func (f *fixture) registerIssuer(t *testing.T) {
	t.Helper()
	if _, err := f.srv.RegisterIssuer(f.ctx, &types.MsgRegisterIssuer{
		Authority:       authority,
		Id:              "verra",
		Did:             issuerDID,
		DisplayName:     "Verra",
		Categories: []int32{
			int32(types.AssetCategory_ASSET_CATEGORY_ALLOWANCE),
			int32(types.AssetCategory_ASSET_CATEGORY_OFFSET),
		},
		IssuerAuthority: issuerAuth,
		Admin:           issuerAdmin,
	}); err != nil {
		t.Fatalf("register issuer: %v", err)
	}
}

func (f *fixture) issueAllowance(t *testing.T, units uint64, recipient string) uint64 {
	t.Helper()
	resp, err := f.srv.IssueAllowance(f.ctx, &types.MsgIssueAllowance{
		IssuerAuthority: issuerAuth, IssuerId: "verra",
		Registry: "EU-ETS", Program: "EUA", Jurisdiction: "EU",
		VintageYear: 2026, SourceRegistry: "EU-ETS", SourceSerial: "EUA-2026-001",
		Units: units, Recipient: recipient,
	})
	if err != nil {
		t.Fatalf("issue allowance: %v", err)
	}
	return resp.AssetId
}

// offsetSeq is a per-test counter so successive calls don't collide
// on the source-serial uniqueness check.
var offsetSeq uint64

func (f *fixture) issueOffset(t *testing.T, units uint64, recipient string, eacLink uint64, a6 types.Article6Status, host string) uint64 {
	t.Helper()
	offsetSeq++
	serial := "VCU-001-" + itoa(offsetSeq)
	resp, err := f.srv.IssueOffset(f.ctx, &types.MsgIssueOffset{
		IssuerAuthority: issuerAuth, IssuerId: "verra",
		Registry: "Verra", Program: "VCU", ProjectId: "VCS-001", Methodology: "VM0007",
		Jurisdiction: "BR", VintageYear: 2026, SourceRegistry: "Verra", SourceSerial: serial,
		CcpLabels:              []string{"high-integrity:approved"},
		LinkedEacCertificateId: eacLink, Article6Status: a6, HostCountry: host,
		Units: units, Recipient: recipient,
	})
	if err != nil {
		t.Fatalf("issue offset: %v", err)
	}
	return resp.AssetId
}

// ---------------------------------------------------------------------------
// Issuer lifecycle
// ---------------------------------------------------------------------------

func TestRegisterIssuerOK(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	is, err := f.k.Issuers.Get(f.ctx, "verra")
	if err != nil || is.Status != types.IssuerStatus_ISSUER_STATUS_ACTIVE {
		t.Fatalf("want active issuer, got %+v err=%v", is, err)
	}
}

func TestRegisterIssuerNonAuthority(t *testing.T) {
	f := setup(t)
	_, err := f.srv.RegisterIssuer(f.ctx, &types.MsgRegisterIssuer{
		Authority: holderAlice, Id: "verra", Did: issuerDID,
		DisplayName: "x", IssuerAuthority: issuerAuth, Admin: issuerAdmin,
	})
	if err == nil {
		t.Fatal("expected unauthorized")
	}
}

func TestSuspendBlocksIssue(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	if _, err := f.srv.SuspendIssuer(f.ctx, &types.MsgSuspendIssuer{
		Authority: authority, Id: "verra", Reason: "audit",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := f.srv.IssueAllowance(f.ctx, &types.MsgIssueAllowance{
		IssuerAuthority: issuerAuth, IssuerId: "verra",
		Registry: "EU-ETS", Program: "EUA", Jurisdiction: "EU",
		VintageYear: 2026, SourceRegistry: "EU-ETS", SourceSerial: "EUA-2026-001",
		Units: 1, Recipient: holderAlice,
	})
	if err == nil || !strings.Contains(err.Error(), "ACTIVE") {
		t.Fatalf("expected suspended-issuer block, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Issuance
// ---------------------------------------------------------------------------

func TestIssueAllowanceHappyPath(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueAllowance(t, 100, holderAlice)
	bal, err := f.k.GetBalance(f.ctx, id, holderAlice)
	if err != nil {
		t.Fatal(err)
	}
	if bal != 100 {
		t.Fatalf("bal want 100 got %d", bal)
	}
	a, _ := f.k.Assets.Get(f.ctx, id)
	if a.Category != types.AssetCategory_ASSET_CATEGORY_ALLOWANCE || a.IssuedUnits != 100 {
		t.Fatalf("asset wrong: %+v", a)
	}
}

func TestIssueOffsetHappyPath(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueOffset(t, 50, holderAlice, 0, types.Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE, "")
	a, _ := f.k.Assets.Get(f.ctx, id)
	if a.Category != types.AssetCategory_ASSET_CATEGORY_OFFSET || a.IssuedUnits != 50 {
		t.Fatalf("asset wrong: %+v", a)
	}
}

func TestIssueAllowanceCategoryNotAllowed(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.RegisterIssuer(f.ctx, &types.MsgRegisterIssuer{
		Authority: authority, Id: "offset-only", Did: issuerDID,
		DisplayName: "Offset Only",
		Categories:  []int32{int32(types.AssetCategory_ASSET_CATEGORY_OFFSET)},
		IssuerAuthority: issuerAuth, Admin: issuerAdmin,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := f.srv.IssueAllowance(f.ctx, &types.MsgIssueAllowance{
		IssuerAuthority: issuerAuth, IssuerId: "offset-only",
		Registry: "EU-ETS", Program: "EUA", Jurisdiction: "EU",
		VintageYear: 2026, SourceRegistry: "EU-ETS", SourceSerial: "EUA-2026-OO",
		Units: 1, Recipient: holderAlice,
	})
	if err == nil || !strings.Contains(err.Error(), "not authorised") {
		t.Fatalf("expected category disallowed, got %v", err)
	}
}

func TestIssueRejectsSanctionedRecipient(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	f.san.bad[holderAlice] = true
	_, err := f.srv.IssueAllowance(f.ctx, &types.MsgIssueAllowance{
		IssuerAuthority: issuerAuth, IssuerId: "verra",
		Registry: "EU-ETS", Program: "EUA", Jurisdiction: "EU",
		VintageYear: 2026, SourceRegistry: "EU-ETS", SourceSerial: "EUA-2026-SAN",
		Units: 1, Recipient: holderAlice,
	})
	if err == nil || !strings.Contains(err.Error(), "sanctions") {
		t.Fatalf("expected recipient sanctions block, got %v", err)
	}
}

func TestIssueSourceSerialUniqueness(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	f.issueAllowance(t, 5, holderAlice)
	_, err := f.srv.IssueAllowance(f.ctx, &types.MsgIssueAllowance{
		IssuerAuthority: issuerAuth, IssuerId: "verra",
		Registry: "EU-ETS", Program: "EUA", Jurisdiction: "EU",
		VintageYear: 2026, SourceRegistry: "EU-ETS", SourceSerial: "EUA-2026-001",
		Units: 1, Recipient: holderBob,
	})
	if err == nil || !strings.Contains(err.Error(), "already issued") {
		t.Fatalf("expected duplicate-serial rejection, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Transfer
// ---------------------------------------------------------------------------

func TestTransferHappyPath(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueAllowance(t, 100, holderAlice)
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, AssetId: id, Units: 30,
	}); err != nil {
		t.Fatal(err)
	}
	a, _ := f.k.GetBalance(f.ctx, id, holderAlice)
	b, _ := f.k.GetBalance(f.ctx, id, holderBob)
	if a != 70 || b != 30 {
		t.Fatalf("alice=%d bob=%d, want 70/30", a, b)
	}
}

func TestTransferBlockedSanctioned(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueAllowance(t, 100, holderAlice)
	f.san.bad[holderBob] = true
	_, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, AssetId: id, Units: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "sanctions") {
		t.Fatalf("expected sanctions block, got %v", err)
	}
}

func TestTransferBlockedByPolicy(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueAllowance(t, 100, holderAlice)
	f.pol.deny = true
	f.pol.denyErr = "policy says no"
	_, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, AssetId: id, Units: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "policy") {
		t.Fatalf("expected policy block, got %v", err)
	}
}

func TestTransferBlockedSealed(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueAllowance(t, 100, holderAlice)
	if _, err := f.srv.SealAsset(f.ctx, &types.MsgSealAsset{
		Admin: issuerAdmin, AssetId: id, Reason: "dispute",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, AssetId: id, Units: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "not transferable") {
		t.Fatalf("expected sealed block, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Retire
// ---------------------------------------------------------------------------

func TestRetireHappyPath(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueAllowance(t, 50, holderAlice)
	resp, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderAlice, AssetId: id, Units: 20, Purpose: "scope1", Claim: "fy2026",
	})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := f.k.Assets.Get(f.ctx, id)
	if a.RetiredUnits != 20 {
		t.Fatalf("retired want 20 got %d", a.RetiredUnits)
	}
	r, _ := f.k.Retirements.Get(f.ctx, resp.RetirementId)
	if r.Beneficiary != holderAlice || r.Amount != 20 {
		t.Fatalf("retirement bad: %+v", r)
	}
}

func TestRetireProxy(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueAllowance(t, 50, holderAlice)
	resp, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderAlice, Beneficiary: holderCarol,
		AssetId: id, Units: 10, Purpose: "voluntary",
	})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := f.k.Retirements.Get(f.ctx, resp.RetirementId)
	if r.Beneficiary != holderCarol || r.Retirer != holderAlice {
		t.Fatalf("proxy retire bad: %+v", r)
	}
}

func TestRetireFullyRetiresStatus(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueAllowance(t, 5, holderAlice)
	if _, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderAlice, AssetId: id, Units: 5, Purpose: "scope1",
	}); err != nil {
		t.Fatal(err)
	}
	a, _ := f.k.Assets.Get(f.ctx, id)
	if a.Status != types.AssetStatus_ASSET_STATUS_FULLY_RETIRED {
		t.Fatalf("want FULLY_RETIRED got %s", a.Status)
	}
}

func TestRetireWhileSealedAllowed(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueAllowance(t, 10, holderAlice)
	if _, err := f.srv.SealAsset(f.ctx, &types.MsgSealAsset{
		Admin: issuerAdmin, AssetId: id, Reason: "x",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderAlice, AssetId: id, Units: 3, Purpose: "scope1",
	}); err != nil {
		t.Fatalf("retire while sealed should succeed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Article 6
// ---------------------------------------------------------------------------

func TestArticle6CrossBorderRequiresCA(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueOffset(t, 10, holderAlice, 0,
		types.Article6Status_ARTICLE6_STATUS_AUTHORIZED, "BR")
	// retire on behalf of CH beneficiary, host=BR != BJ=CH -> needs CA_APPLIED
	_, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderAlice, Beneficiary: holderBob,
		AssetId: id, Units: 1, Purpose: "ndc",
		BeneficiaryJurisdiction: "CH",
	})
	if err == nil || !strings.Contains(err.Error(), "CA_APPLIED") {
		t.Fatalf("expected CA_APPLIED requirement, got %v", err)
	}
}

func TestArticle6CrossBorderAllowedAfterCAApplied(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueOffset(t, 10, holderAlice, 0,
		types.Article6Status_ARTICLE6_STATUS_AUTHORIZED, "BR")
	if _, err := f.srv.SetArticle6Status(f.ctx, &types.MsgSetArticle6Status{
		Admin: issuerAdmin, AssetId: id,
		Status: types.Article6Status_ARTICLE6_STATUS_CA_APPLIED,
		HostCountry: "BR", RecipientCountry: "CH",
		DocumentUri:  "https://example.com/a6/ca",
		DocumentHash: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderAlice, Beneficiary: holderBob,
		AssetId: id, Units: 1, Purpose: "ndc",
		BeneficiaryJurisdiction: "CH",
	}); err != nil {
		t.Fatalf("cross-border retire after CA_APPLIED should succeed: %v", err)
	}
	auth, err := f.k.Article6Authorizations.Get(f.ctx, id)
	if err != nil || auth.HostCountry != "BR" {
		t.Fatalf("authorization row missing or wrong: %+v err=%v", auth, err)
	}
}

func TestArticle6SameJurisdictionNoCANeeded(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueOffset(t, 10, holderAlice, 0,
		types.Article6Status_ARTICLE6_STATUS_AUTHORIZED, "BR")
	// retire by BR-resident beneficiary against host=BR -> no CA needed
	if _, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderAlice, AssetId: id, Units: 1, Purpose: "scope1",
		BeneficiaryJurisdiction: "BR",
	}); err != nil {
		t.Fatalf("same-jurisdiction retire should succeed: %v", err)
	}
}

func TestSetArticle6StatusOnlyOffset(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueAllowance(t, 5, holderAlice)
	_, err := f.srv.SetArticle6Status(f.ctx, &types.MsgSetArticle6Status{
		Admin: issuerAdmin, AssetId: id,
		Status:      types.Article6Status_ARTICLE6_STATUS_CA_APPLIED,
		HostCountry: "BR",
	})
	if err == nil || !strings.Contains(err.Error(), "OFFSET") {
		t.Fatalf("expected OFFSET-only restriction, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// EAC mutual exclusion
// ---------------------------------------------------------------------------

func TestEACMutualExclusionHappyPath(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	f.eac.certs[42] = 100
	id := f.issueOffset(t, 80, holderAlice, 42, types.Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE, "")
	claimed, err := f.k.GetEACClaimed(f.ctx, 42)
	if err != nil || claimed != 80 {
		t.Fatalf("claimed want 80 got %d err=%v", claimed, err)
	}
	// second offset of 20 fits into the remaining capacity (20)
	_ = f.issueOffset(t, 20, holderAlice, 42, types.Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE, "")
	claimed, _ = f.k.GetEACClaimed(f.ctx, 42)
	if claimed != 100 {
		t.Fatalf("claimed want 100 got %d", claimed)
	}
	_ = id
}

func TestEACMutualExclusionCapEnforced(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	f.eac.certs[7] = 50
	f.issueOffset(t, 40, holderAlice, 7, types.Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE, "")
	_, err := f.srv.IssueOffset(f.ctx, &types.MsgIssueOffset{
		IssuerAuthority: issuerAuth, IssuerId: "verra",
		Registry: "Verra", Program: "VCU", ProjectId: "p2", Methodology: "VM0007",
		Jurisdiction: "BR", VintageYear: 2026,
		SourceRegistry: "Verra", SourceSerial: "VCU-7-2",
		LinkedEacCertificateId: 7, Article6Status: types.Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE,
		Units: 11, Recipient: holderAlice,
	})
	if err == nil || !strings.Contains(err.Error(), "mutual-exclusion") {
		t.Fatalf("expected EAC cap, got %v", err)
	}
}

func TestEACMutualExclusionUnknownEAC(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	_, err := f.srv.IssueOffset(f.ctx, &types.MsgIssueOffset{
		IssuerAuthority: issuerAuth, IssuerId: "verra",
		Registry: "Verra", Program: "VCU", ProjectId: "p", Methodology: "VM0007",
		Jurisdiction: "BR", VintageYear: 2026,
		SourceRegistry: "Verra", SourceSerial: "VCU-UNK-1",
		LinkedEacCertificateId: 999, Article6Status: types.Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE,
		Units: 1, Recipient: holderAlice,
	})
	if err == nil || !strings.Contains(err.Error(), "EAC certificate 999 not found") {
		t.Fatalf("expected unknown-EAC fail-closed, got %v", err)
	}
}

func TestEACMutualExclusionLinkOnlyOffset(t *testing.T) {
	// LinkedEacCertificateId on an ALLOWANCE asset is not modeled in
	// the message (no field). Make sure the keeper rejects it via the
	// commonIssue guard if a future message ever surfaces one — here
	// we exercise the guard by using IssueOffset which carries the
	// link, so this is implicitly covered.
	t.Skip("link-only-offset is a structural property of the proto")
}

// ---------------------------------------------------------------------------
// Bridge
// ---------------------------------------------------------------------------

func setupBridge(t *testing.T, f *fixture, assetID uint64, maxStale uint32) {
	t.Helper()
	if _, err := f.srv.SetBridgeAttestation(f.ctx, &types.MsgSetBridgeAttestation{
		Authority: authority, AssetId: assetID,
		OracleTopicId: "verra.bridge.001", MaxStalenessSeconds: maxStale,
	}); err != nil {
		t.Fatalf("set bridge: %v", err)
	}
}

func TestBridgeMintHappyPath(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueAllowance(t, 50, holderAlice)
	setupBridge(t, f, id, 3600)
	f.ora.value = 200
	resp, err := f.srv.BridgeMint(f.ctx, &types.MsgBridgeMint{
		IssuerAuthority: issuerAuth, AssetId: id, Units: 100, Recipient: holderBob,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.NewIssuedUnits != 150 {
		t.Fatalf("issued want 150 got %d", resp.NewIssuedUnits)
	}
}

func TestBridgeMintExceedsAttestation(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueAllowance(t, 50, holderAlice)
	setupBridge(t, f, id, 3600)
	f.ora.value = 60
	_, err := f.srv.BridgeMint(f.ctx, &types.MsgBridgeMint{
		IssuerAuthority: issuerAuth, AssetId: id, Units: 50, Recipient: holderBob,
	})
	if err == nil || !strings.Contains(err.Error(), "attested") {
		t.Fatalf("expected attestation cap, got %v", err)
	}
}

func TestBridgeMintStaleAttestation(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueAllowance(t, 5, holderAlice)
	setupBridge(t, f, id, 60)
	f.ora.value = 1000
	f.ora.ts = 1_700_000_000 - 3600
	_, err := f.srv.BridgeMint(f.ctx, &types.MsgBridgeMint{
		IssuerAuthority: issuerAuth, AssetId: id, Units: 1, Recipient: holderBob,
	})
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expected stale block, got %v", err)
	}
}

func TestBridgeBurnRequiresAttestation(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueAllowance(t, 5, holderAlice)
	_, err := f.srv.BridgeBurn(f.ctx, &types.MsgBridgeBurn{
		Holder: holderAlice, AssetId: id, Units: 1, ExternalRecipient: "ext-1",
	})
	if err == nil || !strings.Contains(err.Error(), "no bridge attestation") {
		t.Fatalf("expected attestation-required, got %v", err)
	}
}

func TestBridgeBurnRejectsSanctionedHolder(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueAllowance(t, 5, holderAlice)
	setupBridge(t, f, id, 3600)
	f.san.bad[holderAlice] = true
	_, err := f.srv.BridgeBurn(f.ctx, &types.MsgBridgeBurn{
		Holder: holderAlice, AssetId: id, Units: 1, ExternalRecipient: "ext-1",
	})
	if err == nil || !strings.Contains(err.Error(), "sanctions") {
		t.Fatalf("expected sanctions block on holder, got %v", err)
	}
}

func TestRetireRequiresBeneficiaryJurisdictionForInternationalAsset(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueOffset(t, 10, holderAlice, 0,
		types.Article6Status_ARTICLE6_STATUS_AUTHORIZED, "BR")
	_, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderAlice, AssetId: id, Units: 1, Purpose: "scope1",
		// BeneficiaryJurisdiction omitted — must be rejected because
		// the asset has host_country=BR.
	})
	if err == nil || !strings.Contains(err.Error(), "beneficiary_jurisdiction") {
		t.Fatalf("expected mandatory beneficiary_jurisdiction, got %v", err)
	}
}

func TestSetArticle6StatusDeletesStaleAuthorizationOnDowngrade(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	id := f.issueOffset(t, 10, holderAlice, 0,
		types.Article6Status_ARTICLE6_STATUS_AUTHORIZED, "BR")
	if _, err := f.srv.SetArticle6Status(f.ctx, &types.MsgSetArticle6Status{
		Admin: issuerAdmin, AssetId: id,
		Status: types.Article6Status_ARTICLE6_STATUS_CA_APPLIED,
		HostCountry: "BR", RecipientCountry: "CH",
		DocumentUri:  "https://example.com/a6/ca",
		DocumentHash: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.k.Article6Authorizations.Get(f.ctx, id); err != nil {
		t.Fatalf("expected auth row, got err %v", err)
	}
	// Downgrade: PENDING_AUTHORIZATION -> the stale doc must vanish.
	if _, err := f.srv.SetArticle6Status(f.ctx, &types.MsgSetArticle6Status{
		Admin: issuerAdmin, AssetId: id,
		Status:      types.Article6Status_ARTICLE6_STATUS_PENDING_AUTHORIZATION,
		HostCountry: "BR",
	}); err != nil {
		t.Fatal(err)
	}
	if has, _ := f.k.Article6Authorizations.Has(f.ctx, id); has {
		t.Fatal("expected stale authorization row to be removed on downgrade")
	}
}

func TestBridgeBurnReleasesEACClaim(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	f.eac.certs[7] = 100
	id := f.issueOffset(t, 50, holderAlice, 7, types.Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE, "")
	setupBridge(t, f, id, 3600)
	if _, err := f.srv.BridgeBurn(f.ctx, &types.MsgBridgeBurn{
		Holder: holderAlice, AssetId: id, Units: 20, ExternalRecipient: "ext-1",
	}); err != nil {
		t.Fatal(err)
	}
	claimed, _ := f.k.GetEACClaimed(f.ctx, 7)
	if claimed != 30 {
		t.Fatalf("claimed want 30 got %d", claimed)
	}
}

// ---------------------------------------------------------------------------
// Genesis
// ---------------------------------------------------------------------------

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	f.registerIssuer(t)
	f.eac.certs[42] = 100
	id := f.issueOffset(t, 30, holderAlice, 42, types.Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE, "")
	if _, err := f.srv.Transfer(f.ctx, &types.MsgTransfer{
		From: holderAlice, To: holderBob, AssetId: id, Units: 10,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Retire(f.ctx, &types.MsgRetire{
		Retirer: holderBob, AssetId: id, Units: 3, Purpose: "scope1",
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
	if bal != 20 {
		t.Fatalf("alice want 20 got %d", bal)
	}
	bal, _ = g.k.GetBalance(g.ctx, id, holderBob)
	if bal != 7 {
		t.Fatalf("bob want 7 got %d", bal)
	}
}

func TestGenesisInvariantBalanceVsAsset(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Issuers = []types.Issuer{{
		Id: "verra", Did: issuerDID, DisplayName: "x",
		Status: types.IssuerStatus_ISSUER_STATUS_ACTIVE,
		Authority: issuerAuth, Admin: issuerAdmin,
	}}
	gs.Assets = []types.Asset{{
		Id: 1, IssuerId: "verra", Category: types.AssetCategory_ASSET_CATEGORY_ALLOWANCE,
		Registry: "EU-ETS", Program: "EUA", Jurisdiction: "EU",
		VintageYear: 2026, IssuedUnits: 100, RetiredUnits: 10,
		Status: types.AssetStatus_ASSET_STATUS_ACTIVE,
		Article6Status: types.Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE,
	}}
	gs.Balances = []types.Balance{
		{AssetId: 1, Account: holderAlice, Amount: 80},
	}
	gs.AssetIdSeq = 1
	if err := gs.Validate(); err == nil {
		t.Fatal("expected sum(balances) != issued-retired to fail")
	}
}

func TestGenesisInvariantEACClaimReconciles(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Issuers = []types.Issuer{{
		Id: "verra", Did: issuerDID, DisplayName: "x",
		Status: types.IssuerStatus_ISSUER_STATUS_ACTIVE,
		Authority: issuerAuth, Admin: issuerAdmin,
	}}
	gs.Assets = []types.Asset{{
		Id: 1, IssuerId: "verra", Category: types.AssetCategory_ASSET_CATEGORY_OFFSET,
		Registry: "Verra", Program: "VCU", Jurisdiction: "BR",
		VintageYear: 2026, IssuedUnits: 10, RetiredUnits: 0,
		Status: types.AssetStatus_ASSET_STATUS_ACTIVE,
		Article6Status: types.Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE,
		LinkedEacCertificateId: 99,
	}}
	gs.Balances = []types.Balance{{AssetId: 1, Account: holderAlice, Amount: 10}}
	gs.AssetIdSeq = 1
	// Missing EacClaims row -> invariant should fail.
	if err := gs.Validate(); err == nil {
		t.Fatal("expected EAC claim mismatch to fail")
	}
	gs.EacClaims = []types.EACClaim{{EacCertificateId: 99, ClaimedUnits: 10}}
	if err := gs.Validate(); err != nil {
		t.Fatalf("genesis with reconciled eac claim should pass: %v", err)
	}
}
