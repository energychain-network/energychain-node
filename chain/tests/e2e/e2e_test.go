// Package e2e wires multiple real keepers against a shared
// multi-store to exercise cross-module flows end-to-end.
//
// Each per-module test suite under chain/x/*/keeper covers its own
// keeper in isolation with stubs for cross-module dependencies. This
// package complements those tests by replacing those stubs with the
// *real* peer keepers, so that interface drift, cross-module audit
// recording, and integration-only behaviours (EAC<->Carbon claim
// accounting, sanctions cascades across modules, audit fan-out)
// are exercised in the same way they run in production.
//
// New scenarios should follow the four-section template demonstrated
// in fixtureT:
//
//  1. mount stores for every module in scope on a single MS;
//  2. instantiate each module's real keeper, wiring real peers for
//     dependencies that are themselves modules in the bundle and
//     hand-rolled stubs only for the modules NOT in scope;
//  3. seed default params + minimal genesis state via SetParams;
//  4. drive the flow through MsgServers (never direct keeper
//     mutations), so the test exercises the exact path a relayer
//     would take.
package e2e_test

import (
	"context"
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

	auditkeeper "energychain/x/audit/keeper"
	audittypes "energychain/x/audit/types"
	carbonkeeper "energychain/x/carbon/keeper"
	carbontypes "energychain/x/carbon/types"
	eackeeper "energychain/x/eac/keeper"
	eactypes "energychain/x/eac/types"
	marketkeeper "energychain/x/market/keeper"
	markettypes "energychain/x/market/types"
	sanctionskeeper "energychain/x/sanctions/keeper"
	sanctionstypes "energychain/x/sanctions/types"
	stablecoinkeeper "energychain/x/stablecoin/keeper"
	stablecointypes "energychain/x/stablecoin/types"
)

// ---------------------------------------------------------------------------
// Shared test addresses
//
// Reused across every test in this package so any new flow can
// reference the same set of named participants without re-deriving
// bech32 addresses. All are valid bech32 secp256k1 addresses (length
// 39 + "cosmos1" prefix; the checksum is correct as taken from the
// per-module fixtures in chain/x/*/keeper).
// ---------------------------------------------------------------------------

// All bech32 addresses below pass `sdk.AccAddressFromBech32` against
// the SDK's default config (no custom prefix). They are reused from
// per-module fixtures (chain/x/eac/keeper/keeper_test.go etc.) so
// the keeper-level bech32 validation that runs in NewKeeper succeeds
// when wiring multiple real keepers together. Mixing in addresses
// from other fixtures with mismatched checksums would crash the
// fixture setup with a "decoding bech32 failed" panic.
const (
	authorityAddr = "cosmos126xkl2rxz08nlv6egudef8dxn6c5suxwnsmrjc"
	issuerAdmin   = "cosmos1mc62kvf7adulmqcpgynqupl63hfp68lu82qjlk"
	issuerAuth    = "cosmos150ln4xlvs2dg024ueft43un6e3eav7ca2xhhtf"
	mintAuthority = "cosmos1uu89qg49evj5pkj2u3xwapdjwt7ymsljt7epry"
	holderAlice   = "cosmos1tmxy2mearm0zkw88prwm346aqdqns6m8zlxyrg"
	holderBob     = "cosmos19mup59y4ep02x77uy5vjnq8c4k5a044cwlxkmq"
	holderCarol   = "cosmos1lrmkh72j5pqd5rav9wy8tzdq3zlmd3ullte2ev"
	sanctionedBad = "cosmos1xrnner78enszhq32u4l5z29slzq2zfvka2lt7g"
)

// ---------------------------------------------------------------------------
// Stubs for the slice of modules NOT yet wired in the e2e bundle.
// Kept intentionally minimal — every method satisfies an interface
// the bundled keepers expect and nothing more. A new e2e scenario
// that needs richer behaviour should replace the stub with the real
// module keeper rather than thickening the stub here.
// ---------------------------------------------------------------------------

type stubDID struct {
	active     map[string]bool
	creds      map[string]map[string]bool
	jurisdict  map[string]string
}

func newStubDID() *stubDID {
	return &stubDID{
		active:    map[string]bool{},
		creds:     map[string]map[string]bool{},
		jurisdict: map[string]string{},
	}
}

func (s *stubDID) IsActive(_ sdk.Context, subject string) bool { return s.active[subject] }
func (s *stubDID) HasCredential(_ sdk.Context, subject, ct string) bool {
	if s.creds[subject] == nil {
		return false
	}
	return s.creds[subject][ct]
}
func (s *stubDID) Jurisdiction(_ sdk.Context, subject string) string {
	return s.jurisdict[subject]
}

// stubPolicy implements every PolicyKeeper interface shape used by
// the bundled modules. Default behaviour is allow-all so e2e tests
// focus on the cross-module gates wired through real keepers; flip
// deny=true to verify the keeper actually short-circuits on a
// policy denial.
type stubPolicy struct {
	deny       bool
	denyDetail string
	calls      int
}

func (s *stubPolicy) EvaluateTransfer(_ sdk.Context, _, _, _, _ string, _ uint64) error {
	s.calls++
	if s.deny {
		return &simpleErr{msg: "POLICY_DENIED: " + s.denyDetail}
	}
	return nil
}

type stubOracle struct {
	value int64
	ts    int64
	ok    bool
}

func (s *stubOracle) GetAggregatedReserve(_ sdk.Context, _ string) (int64, int64, bool) {
	return s.value, s.ts, s.ok
}

type simpleErr struct{ msg string }

func (e *simpleErr) Error() string { return e.msg }

// ---------------------------------------------------------------------------
// Fixture: shared multi-store mounting every module's KV store + the
// audit transient store, with real keepers wired against each other.
// ---------------------------------------------------------------------------

type fixtureT struct {
	ctx sdk.Context

	auditKeeper      auditkeeper.Keeper
	sanctionsKeeper  sanctionskeeper.Keeper
	stablecoinKeeper stablecoinkeeper.Keeper
	eacKeeper        eackeeper.Keeper
	carbonKeeper     carbonkeeper.Keeper
	marketKeeper     marketkeeper.Keeper

	eacMsg        eactypes.MsgServer
	carbonMsg     carbontypes.MsgServer
	stablecoinMsg stablecointypes.MsgServer
	sanctionsMsg  sanctionstypes.MsgServer
	marketMsg     markettypes.MsgServer

	stubPolicy *stubPolicy
	stubDID    *stubDID
	stubOracle *stubOracle
}

// stablecoinAdapter satisfies market.types.StablecoinKeeper by
// translating its narrow surface into the wider x/stablecoin.Keeper
// API. Mirror of escrowStablecoinAdapter from chain/keeper_adapters.go
// — the e2e package re-defines it locally so the test bundle does
// not import the chain top-level package (which would create a cycle
// via the app wiring).
type stablecoinMarketAdapter struct {
	k stablecoinkeeper.Keeper
}

func (a stablecoinMarketAdapter) HasDenom(ctx sdk.Context, denomID string) bool {
	return a.k.HasDenom(ctx, denomID)
}

func (a stablecoinMarketAdapter) Move(ctx sdk.Context, denomID, from, to string, amount uint64) error {
	return a.k.MoveBalance(ctx, denomID, from, to, amount)
}

func (a stablecoinMarketAdapter) IsAccountBlocked(ctx sdk.Context, denomID, account string) bool {
	return a.k.IsAccountBlocked(ctx, denomID, account)
}

func (a stablecoinMarketAdapter) IsDenomPaused(ctx sdk.Context, denomID string) bool {
	return a.k.IsDenomPaused(ctx, denomID)
}

func setupFixture(t *testing.T) *fixtureT {
	t.Helper()

	// Per-module KV store keys + audit's transient key. Mounting
	// each on the same CommitMultiStore is what makes the keepers
	// see a coherent state when called in sequence.
	auditKey := storetypes.NewKVStoreKey(audittypes.StoreKey)
	auditTKey := storetypes.NewTransientStoreKey(audittypes.TStoreKey)
	sanctionsKey := storetypes.NewKVStoreKey(sanctionstypes.StoreKey)
	eacKey := storetypes.NewKVStoreKey(eactypes.StoreKey)
	carbonKey := storetypes.NewKVStoreKey(carbontypes.StoreKey)
	stablecoinKey := storetypes.NewKVStoreKey(stablecointypes.StoreKey)
	marketKey := storetypes.NewKVStoreKey(markettypes.StoreKey)

	db := dbm.NewMemDB()
	ms := store.NewCommitMultiStore(db, log.NewNopLogger())
	for _, k := range []storetypes.StoreKey{
		auditKey, sanctionsKey, eacKey, carbonKey, stablecoinKey, marketKey,
	} {
		ms.MountStoreWithDB(k, storetypes.StoreTypeIAVL, db)
	}
	ms.MountStoreWithDB(auditTKey, storetypes.StoreTypeTransient, db)
	if err := ms.LoadLatestVersion(); err != nil {
		t.Fatalf("ms load: %v", err)
	}

	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)

	did := newStubDID()
	// Mark every named participant as DID-active so any
	// DID-gated keeper path proceeds. Sanctioned participants
	// are intentionally DID-active too: sanctions list membership
	// is independent of DID liveness.
	for _, a := range []string{
		issuerAdmin, issuerAuth, mintAuthority,
		holderAlice, holderBob, holderCarol, sanctionedBad,
	} {
		did.active[a] = true
	}

	pol := &stubPolicy{}
	ora := &stubOracle{
		value: 1_000_000_000,
		ts:    1_700_000_000,
		ok:    true,
	}

	// audit -> downstream: real audit keeper consumed as the
	// AuditKeeper interface by every other module.
	auditK := auditkeeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(auditKey),
		auditTKey,
		authorityAddr,
		did,
	)

	sanctionsK := sanctionskeeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(sanctionsKey),
		authorityAddr,
		auditK,
	)

	stablecoinK := stablecoinkeeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(stablecoinKey),
		authorityAddr,
		pol, sanctionsK, did, ora, auditK,
	)

	eacK := eackeeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(eacKey),
		authorityAddr,
		pol, sanctionsK, ora, auditK,
	)

	carbonK := carbonkeeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(carbonKey),
		authorityAddr,
		pol, sanctionsK, ora, eacK, auditK,
	)

	marketK := marketkeeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(marketKey),
		authorityAddr,
		sanctionsK, stablecoinMarketAdapter{k: stablecoinK}, auditK,
	)

	ctx := sdk.NewContext(ms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))

	// Seed default params for every module so the keepers behave
	// the same way they would after a chain genesis.
	mustSetParams(t, "audit", auditK.SetParams(ctx, audittypes.DefaultParams()))
	mustSetParams(t, "stablecoin", stablecoinK.SetParams(ctx, stablecointypes.DefaultParams()))
	mustSetParams(t, "eac", eacK.SetParams(ctx, eactypes.DefaultParams()))
	mustSetParams(t, "carbon", carbonK.SetParams(ctx, carbontypes.DefaultParams()))
	mustSetParams(t, "market", marketK.SetParams(ctx, markettypes.DefaultParams()))

	return &fixtureT{
		ctx:              ctx,
		auditKeeper:      auditK,
		sanctionsKeeper:  sanctionsK,
		stablecoinKeeper: stablecoinK,
		eacKeeper:        eacK,
		carbonKeeper:     carbonK,
		marketKeeper:     marketK,

		eacMsg:        eackeeper.NewMsgServerImpl(eacK),
		carbonMsg:     carbonkeeper.NewMsgServerImpl(carbonK),
		stablecoinMsg: stablecoinkeeper.NewMsgServerImpl(stablecoinK),
		sanctionsMsg:  sanctionskeeper.NewMsgServerImpl(sanctionsK),
		marketMsg:     marketkeeper.NewMsgServerImpl(marketK),

		stubPolicy: pol,
		stubDID:    did,
		stubOracle: ora,
	}
}

func mustSetParams(t *testing.T, mod string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("set %s params: %v", mod, err)
	}
}

// goCtx lifts the SDK context for MsgServer calls that take
// context.Context (every generated MsgServer).
func (f *fixtureT) goCtx() context.Context { return sdk.WrapSDKContext(f.ctx) }

// ---------------------------------------------------------------------------
// Seed helpers used across scenarios.
// ---------------------------------------------------------------------------

func (f *fixtureT) seedEACIssuer(t *testing.T) {
	t.Helper()
	if _, err := f.eacMsg.RegisterIssuer(f.goCtx(), &eactypes.MsgRegisterIssuer{
		Authority:       authorityAddr,
		Id:              "irec",
		Did:             "did:web:irec.example.com:1",
		DisplayName:     "I-REC Operator",
		Kinds: []int32{
			int32(eactypes.CertificateKind_CERTIFICATE_KIND_IREC),
			int32(eactypes.CertificateKind_CERTIFICATE_KIND_NATIVE),
		},
		IssuerAuthority: issuerAuth,
		Admin:           issuerAdmin,
	}); err != nil {
		t.Fatalf("register eac issuer: %v", err)
	}
}

func (f *fixtureT) seedCarbonIssuer(t *testing.T) {
	t.Helper()
	if _, err := f.carbonMsg.RegisterIssuer(f.goCtx(), &carbontypes.MsgRegisterIssuer{
		Authority:   authorityAddr,
		Id:          "verra",
		Did:         "did:web:verra.org:1",
		DisplayName: "Verra Registry",
		Categories: []int32{
			int32(carbontypes.AssetCategory_ASSET_CATEGORY_ALLOWANCE),
			int32(carbontypes.AssetCategory_ASSET_CATEGORY_OFFSET),
		},
		IssuerAuthority: issuerAuth,
		Admin:           issuerAdmin,
	}); err != nil {
		t.Fatalf("register carbon issuer: %v", err)
	}
}

func (f *fixtureT) issueEAC(t *testing.T, units uint64, recipient string) uint64 {
	t.Helper()
	resp, err := f.eacMsg.IssueBatch(f.goCtx(), &eactypes.MsgIssueBatch{
		IssuerAuthority: issuerAuth,
		IssuerId:        "irec",
		Kind:            eactypes.CertificateKind_CERTIFICATE_KIND_NATIVE,
		Technology:      eactypes.Technology_TECHNOLOGY_SOLAR,
		ProjectId:       "proj-001",
		DeviceId:        "dev-001",
		GridZone:        "DE-LU",
		HourStart:       1_700_000_000,
		HourEnd:         1_700_000_000 + 3600,
		VintageYear:     2026,
		Units:           units,
		Recipient:       recipient,
	})
	if err != nil {
		t.Fatalf("issue eac batch: %v", err)
	}
	return resp.CertificateId
}

func (f *fixtureT) seedUSD(t *testing.T, ceiling uint64) {
	t.Helper()
	if _, err := f.stablecoinMsg.RegisterDenom(f.goCtx(), &stablecointypes.MsgRegisterDenom{
		Authority:    authorityAddr,
		Id:           "usd",
		Symbol:       "scnUSD",
		Name:         "EnergyChain USD",
		Decimals:     6,
		Jurisdiction: "US",
	}); err != nil {
		t.Fatalf("register denom: %v", err)
	}
	if _, err := f.stablecoinMsg.RegisterIssuer(f.goCtx(), &stablecointypes.MsgRegisterIssuer{
		Authority:       authorityAddr,
		Id:              "alpha",
		Did:             "did:web:bank.example.com:1",
		DisplayName:     "Alpha Bank",
		MintAuthorities: []string{mintAuthority},
		Admin:           issuerAdmin,
	}); err != nil {
		t.Fatalf("register issuer: %v", err)
	}
	if _, err := f.stablecoinMsg.SetMintQuota(f.goCtx(), &stablecointypes.MsgSetMintQuota{
		Authority: authorityAddr, IssuerId: "alpha", DenomId: "usd", Ceiling: ceiling,
	}); err != nil {
		t.Fatalf("set quota: %v", err)
	}
	if _, err := f.stablecoinMsg.SetReserveRequirement(f.goCtx(), &stablecointypes.MsgSetReserveRequirement{
		Authority:           authorityAddr,
		DenomId:             "usd",
		OracleTopicId:       "reserve.usd",
		RequiredRatioBps:    10_000,
		MaxStalenessSeconds: 3600,
	}); err != nil {
		t.Fatalf("set reserve: %v", err)
	}
}

func (f *fixtureT) mintUSD(t *testing.T, recipient string, amount uint64) {
	t.Helper()
	if _, err := f.stablecoinMsg.Mint(f.goCtx(), &stablecointypes.MsgMint{
		Minter:    mintAuthority,
		IssuerId:  "alpha",
		DenomId:   "usd",
		Recipient: recipient,
		Amount:    amount,
	}); err != nil {
		t.Fatalf("mint usd: %v", err)
	}
}

// ensureOFACList registers the default OFAC list under the
// authority. Idempotent — re-calls return without error.
func (f *fixtureT) ensureOFACList(t *testing.T) {
	t.Helper()
	if _, err := f.sanctionsMsg.RegisterList(f.goCtx(), &sanctionstypes.MsgRegisterList{
		Authority:     authorityAddr,
		Id:            "ofac",
		Name:          "OFAC SDN",
		Description:   "US Treasury OFAC Specially Designated Nationals",
		SourceUri:     "https://home.treasury.gov/ofac",
		Jurisdiction:  "US",
		ListAuthority: authorityAddr,
	}); err != nil && !strings.Contains(err.Error(), "already") {
		t.Fatalf("register sanctions list: %v", err)
	}
}

// addSanction adds `subject` to the OFAC list via the real
// sanctions keeper MsgServer. Mirrors the exact path a governance
// proposal or list-authority push would take in production.
func (f *fixtureT) addSanction(t *testing.T, subject, reason string) {
	t.Helper()
	f.ensureOFACList(t)
	now := f.ctx.BlockTime().Unix()
	if _, err := f.sanctionsMsg.AddEntry(f.goCtx(), &sanctionstypes.MsgAddEntry{
		Authority: authorityAddr,
		Entry: sanctionstypes.SanctionEntry{
			ListId:      "ofac",
			Subject:     subject,
			SubjectKind: sanctionstypes.SubjectKind_SUBJECT_KIND_ADDRESS,
			Reason:      reason,
			Status:      sanctionstypes.EntryStatus_ENTRY_STATUS_ACTIVE,
			AddedAt:     now,
			AddedBy:     authorityAddr,
		},
	}); err != nil {
		t.Fatalf("add sanction entry: %v", err)
	}
}
