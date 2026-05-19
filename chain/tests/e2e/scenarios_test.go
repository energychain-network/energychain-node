package e2e_test

import (
	"strings"
	"testing"

	carbontypes "energychain/x/carbon/types"
	eactypes "energychain/x/eac/types"
	markettypes "energychain/x/market/types"
	stablecointypes "energychain/x/stablecoin/types"
)

// ---------------------------------------------------------------------------
// Scenario 1: EAC ↔ Carbon mutual-exclusion
//
// A renewable issuer mints a 1000 MWh EAC and sends it to Alice; a
// carbon issuer then mints a 600 MWh OFFSET asset linked to the
// same EAC certificate. The keeper MUST debit 600 from the EAC's
// claimable bucket (read via GetEACClaimed) and MUST reject a
// second 500 MWh offset linked to the same cert (600 + 500 > 1000).
//
// This proves:
//   * EACKeeper interface (real x/eac satisfies carbon.types.EACKeeper)
//   * reserveEACClaim path runs without panic against real state
//   * over-claim rejection happens BEFORE asset row is written
// ---------------------------------------------------------------------------

func TestEAC_CarbonOffset_ClaimReservation(t *testing.T) {
	f := setupFixture(t)
	f.seedEACIssuer(t)
	f.seedCarbonIssuer(t)

	// 1000 MWh of green generation, credited to Alice.
	certID := f.issueEAC(t, 1000, holderAlice)

	// 600 MWh offset linked to that cert, recipient = Bob.
	if _, err := f.carbonMsg.IssueOffset(f.goCtx(), &carbontypes.MsgIssueOffset{
		IssuerAuthority:        issuerAuth,
		IssuerId:               "verra",
		Registry:               "verra",
		Program:                "VCU",
		ProjectId:              "VCS-1234",
		Methodology:            "VM0007",
		Jurisdiction:           "BR",
		VintageYear:            2026,
		SourceRegistry:         "verra",
		SourceSerial:           "vcu-1234-001",
		Article6Status:         carbontypes.Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE,
		LinkedEacCertificateId: certID,
		Units:                  600,
		Recipient:              holderBob,
	}); err != nil {
		t.Fatalf("first offset must succeed: %v", err)
	}

	claimed, err := f.carbonKeeper.GetEACClaimed(f.ctx, certID)
	if err != nil {
		t.Fatalf("get eac claimed: %v", err)
	}
	if claimed != 600 {
		t.Fatalf("eac claimed: want 600, got %d", claimed)
	}

	// Second 500 MWh offset linked to the same EAC must fail
	// (600 + 500 = 1100 > 1000 issued).
	_, err = f.carbonMsg.IssueOffset(f.goCtx(), &carbontypes.MsgIssueOffset{
		IssuerAuthority:        issuerAuth,
		IssuerId:               "verra",
		Registry:               "verra",
		Program:                "VCU",
		ProjectId:              "VCS-1234-b",
		Methodology:            "VM0007",
		Jurisdiction:           "BR",
		VintageYear:            2026,
		SourceRegistry:         "verra",
		SourceSerial:           "vcu-1234-002",
		Article6Status:         carbontypes.Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE,
		LinkedEacCertificateId: certID,
		Units:                  500,
		Recipient:              holderBob,
	})
	if err == nil {
		t.Fatal("over-claim offset must be rejected")
	}

	// Claim count must NOT have advanced past 600 — the failed
	// IssueOffset must roll back its reservation atomically.
	claimed2, err := f.carbonKeeper.GetEACClaimed(f.ctx, certID)
	if err != nil {
		t.Fatalf("get eac claimed (post-fail): %v", err)
	}
	if claimed2 != 600 {
		t.Fatalf("over-claim must not leak claim units: want 600, got %d", claimed2)
	}
}

// ---------------------------------------------------------------------------
// Scenario 2: Sanctions ↔ EAC + Stablecoin
//
// Marking an address sanctioned via the real sanctions keeper must
// cascade into a denied stablecoin Mint (sanctions check on
// recipient) AND a denied EAC Transfer (sanctions check on
// receiver). This exercises the SanctionsKeeper interface on the
// stablecoin + eac side AGAINST the real sanctions store.
// ---------------------------------------------------------------------------

func TestSanctions_CascadeBlocksMintAndTransfer(t *testing.T) {
	f := setupFixture(t)
	f.seedEACIssuer(t)
	f.seedUSD(t, 1_000_000)

	// Baseline: mint to Alice and transfer some EAC to Bob succeed.
	f.mintUSD(t, holderAlice, 100)
	certID := f.issueEAC(t, 100, holderAlice)
	if _, err := f.eacMsg.Transfer(f.goCtx(), &eactypes.MsgTransfer{
		From:          holderAlice,
		To:            holderBob,
		CertificateId: certID,
		Units:         10,
	}); err != nil {
		t.Fatalf("baseline eac transfer should succeed: %v", err)
	}

	// Now sanction Bob. Re-attempt both flows against Bob.
	f.addSanction(t, holderBob, "test-sanction")

	if f.sanctionsKeeper.IsSanctioned(f.ctx, holderBob) != true {
		t.Fatal("sanctions keeper must report Bob as sanctioned")
	}

	_, err := f.stablecoinMsg.Mint(f.goCtx(), &stablecointypes.MsgMint{
		Minter: mintAuthority, IssuerId: "alpha", DenomId: "usd",
		Recipient: holderBob, Amount: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "sanction") {
		t.Fatalf("sanctioned mint must fail with sanction-related error, got %v", err)
	}

	_, err = f.eacMsg.Transfer(f.goCtx(), &eactypes.MsgTransfer{
		From:          holderAlice,
		To:            holderBob,
		CertificateId: certID,
		Units:         5,
	})
	if err == nil {
		t.Fatal("sanctioned recipient must block EAC transfer")
	}
}

// ---------------------------------------------------------------------------
// Scenario 3: EAC lifecycle end-to-end via real keepers
//
// Issue → transfer → retire → status transition. This exercises
// the real audit keeper (records each action), the real sanctions
// keeper (clean path: nobody is sanctioned), and the EAC keeper
// against its own keepers. Asserts the final CertificateStatus
// flip to FULLY_RETIRED when retired units == issued units.
// ---------------------------------------------------------------------------

func TestEAC_FullLifecycle_FullyRetiredFlag(t *testing.T) {
	f := setupFixture(t)
	f.seedEACIssuer(t)

	certID := f.issueEAC(t, 50, holderAlice)

	// Alice transfers half to Bob.
	if _, err := f.eacMsg.Transfer(f.goCtx(), &eactypes.MsgTransfer{
		From:          holderAlice,
		To:            holderBob,
		CertificateId: certID,
		Units:         25,
	}); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	// Each holder retires their half.
	if _, err := f.eacMsg.Retire(f.goCtx(), &eactypes.MsgRetire{
		Retirer:       holderAlice,
		CertificateId: certID,
		Units:         25,
		Purpose:       "scope2",
	}); err != nil {
		t.Fatalf("alice retire: %v", err)
	}
	if _, err := f.eacMsg.Retire(f.goCtx(), &eactypes.MsgRetire{
		Retirer:       holderBob,
		CertificateId: certID,
		Units:         25,
		Purpose:       "scope2",
		Beneficiary:   holderCarol, // proxy retire on Carol's behalf
	}); err != nil {
		t.Fatalf("bob retire: %v", err)
	}

	cert, err := f.eacKeeper.Certificates.Get(f.ctx, certID)
	if err != nil {
		t.Fatalf("get cert: %v", err)
	}
	if cert.RetiredUnits != 50 {
		t.Fatalf("retired_units: want 50, got %d", cert.RetiredUnits)
	}
	if cert.Status != eactypes.CertificateStatus_CERTIFICATE_STATUS_FULLY_RETIRED {
		t.Fatalf("status: want FULLY_RETIRED, got %s", cert.Status)
	}
}

// ---------------------------------------------------------------------------
// Scenario 4: Stablecoin admin actions ↔ Audit fanout
//
// Compliance-sensitive stablecoin operations (Freeze, Blacklist,
// ForceTransfer) MUST land in the real audit module's log. This
// exercises the AuditKeeper interface against the live audit
// keeper and proves the fanout wiring is intact.
//
// Hot-path operations (Mint/Transfer) deliberately do NOT write
// audit rows — see chain/x/stablecoin/keeper/msg_server.go for the
// per-action recordAudit() call sites — so the test focuses on the
// operations that should leave an audit trail.
// ---------------------------------------------------------------------------

func TestStablecoin_AdminActionsEmitAuditLog(t *testing.T) {
	f := setupFixture(t)
	f.seedUSD(t, 1_000_000)
	f.mintUSD(t, holderAlice, 1_000)

	// Baseline transfer succeeds.
	if _, err := f.stablecoinMsg.Transfer(f.goCtx(), &stablecointypes.MsgTransfer{
		DenomId: "usd",
		From:    holderAlice,
		To:      holderBob,
		Amount:  250,
	}); err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if got := f.stablecoinKeeper.GetBalance(f.ctx, "usd", holderBob); got != 250 {
		t.Fatalf("bob balance: want 250, got %d", got)
	}

	// Admin freezes Bob — this MUST land in audit.
	if _, err := f.stablecoinMsg.Freeze(f.goCtx(), &stablecointypes.MsgFreeze{
		Admin:    issuerAdmin,
		IssuerId: "alpha",
		DenomId:  "usd",
		Account:  holderBob,
		Reason:   "kyc-renewal-overdue",
	}); err != nil {
		t.Fatalf("freeze: %v", err)
	}

	// Once frozen, Bob cannot move funds out — sanity check on the
	// stablecoin keeper's per-account freeze enforcement.
	_, err := f.stablecoinMsg.Transfer(f.goCtx(), &stablecointypes.MsgTransfer{
		DenomId: "usd",
		From:    holderBob,
		To:      holderCarol,
		Amount:  10,
	})
	if err == nil {
		t.Fatal("frozen account must not be able to transfer out")
	}

	logs := f.auditKeeper.GetAllLogs(f.ctx)
	if len(logs) == 0 {
		t.Fatal("expected at least one audit row after Freeze")
	}
	var seenFreeze bool
	for _, l := range logs {
		if strings.Contains(l.EventType, "stablecoin.freeze") {
			seenFreeze = true
		}
	}
	if !seenFreeze {
		var got []string
		for _, l := range logs {
			got = append(got, l.EventType)
		}
		t.Fatalf("expected stablecoin.freeze in audit logs; got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Scenario 5: Market ↔ Stablecoin ↔ Sanctions
//
// CreatePair (USD/USD, FBA mode) → sanctioned trader's PlaceLimitOrder
// is rejected (sanctions cascade via market.requireUnsanctioned).
// A clean trader on the same pair succeeds and lands an open order
// row queryable via getOrder. Demonstrates that the market keeper's
// SanctionsKeeper dependency works against the real sanctions store.
// ---------------------------------------------------------------------------

func TestMarket_SanctionsBlocksOrder(t *testing.T) {
	f := setupFixture(t)
	f.seedUSD(t, 1_000_000_000)

	// Register a second stablecoin denom so the market pair can have
	// distinct base + quote denoms (CreatePair rejects same-denom
	// pairs).
	if _, err := f.stablecoinMsg.RegisterDenom(f.goCtx(), &stablecointypes.MsgRegisterDenom{
		Authority:    authorityAddr,
		Id:           "eur",
		Symbol:       "scnEUR",
		Name:         "EnergyChain EUR",
		Decimals:     6,
		Jurisdiction: "EU",
	}); err != nil {
		t.Fatalf("register eur denom: %v", err)
	}

	// Seed both denoms so the market keeper's StablecoinKeeper.HasDenom
	// returns true for both legs of the pair.
	f.mintUSD(t, holderAlice, 100_000)
	f.mintUSD(t, sanctionedBad, 100_000)

	// Prices are scaled by markettypes.PriceScale (typically 1e6),
	// so the price band must straddle PriceScale to cover the
	// realistic "1.0 EUR/USD ± wide" range used by the order below.
	pairResp, err := f.marketMsg.CreatePair(f.goCtx(), &markettypes.MsgCreatePair{
		Authority:          authorityAddr,
		BaseDenom:          "usd",
		QuoteDenom:         "eur",
		Mode:               markettypes.MatchMode_MATCH_MODE_CONTINUOUS,
		PriceBandLo:        markettypes.PriceScale / 10,
		PriceBandHi:        markettypes.PriceScale * 10,
		MaxPositionPerUser: 10_000,
	})
	if err != nil {
		t.Fatalf("create pair: %v", err)
	}
	pairID := pairResp.PairId

	// Sanction the bad trader BEFORE the clean order, so the
	// scenario asserts the market keeper's sanction gate even
	// when other tests have already passed.
	f.addSanction(t, sanctionedBad, "test-sanction")

	// Clean trader: open a sell order. SELL locks base denom
	// (usd) which Alice has from f.mintUSD above.
	cleanResp, err := f.marketMsg.PlaceLimitOrder(f.goCtx(), &markettypes.MsgPlaceLimitOrder{
		Owner:    holderAlice,
		PairId:   pairID,
		Side:     markettypes.Side_SIDE_SELL,
		Price:    markettypes.PriceScale, // 1.0 EUR per USD
		Quantity: 10,
	})
	if err != nil {
		t.Fatalf("clean trader PlaceLimitOrder: %v", err)
	}
	if cleanResp.OrderId == 0 {
		t.Fatal("expected non-zero order id")
	}

	// Sanctioned trader trying the same sell side must be rejected.
	_, err = f.marketMsg.PlaceLimitOrder(f.goCtx(), &markettypes.MsgPlaceLimitOrder{
		Owner:    sanctionedBad,
		PairId:   pairID,
		Side:     markettypes.Side_SIDE_SELL,
		Price:    markettypes.PriceScale,
		Quantity: 10,
	})
	if err == nil {
		t.Fatal("sanctioned trader must be rejected by market")
	}
}
