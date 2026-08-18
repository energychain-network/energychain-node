package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/stableusd/types"
)

// bindReserve binds a reserve topic to the test denom at the given ratio/staleness.
func (f *fixture) bindReserve(t *testing.T, topic string, ratioBps uint32, staleness int64) {
	t.Helper()
	if _, err := f.srv.BindReserve(f.ts.Ctx, &types.MsgBindReserve{
		Admin: admin, DenomId: denomID, ReserveTopic: topic, RequiredRatioBps: ratioBps, MaxStalenessSeconds: staleness,
	}); err != nil {
		t.Fatalf("bind reserve: %v", err)
	}
}

func blockNow(f *fixture) int64 { return sdk.UnwrapSDKContext(f.ts.Ctx).BlockTime().Unix() }

// TestBridgeMintRequiresReserve proves the bridge mint path is ALWAYS reserve-
// gated: with no topic bound (and the global MintRequiresReserve toggle off by
// default) it must still fail closed.
func TestBridgeMintRequiresReserve(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	if err := f.k.BridgeMint(f.ts.Ctx, denomID, alice, 100); err == nil {
		t.Fatal("bridge mint must fail without a bound reserve topic")
	}
	if got := f.k.GetBalance(f.ts.Ctx, denomID, alice); got != 0 {
		t.Fatalf("balance changed on failed mint: %d", got)
	}
}

// TestBridgeMintReserveCoverage is the core safety property: bridge issuance can
// never exceed the attested off-chain collateral at the required ratio, even
// though the global MintRequiresReserve param is off.
func TestBridgeMintReserveCoverage(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.bindReserve(t, "reserve.usdc", 10000, 0) // 100%, no staleness window
	f.res.set("reserve.usdc", 1000, blockNow(f))

	// within coverage -> mints 1:1
	if err := f.k.BridgeMint(f.ts.Ctx, denomID, alice, 1000); err != nil {
		t.Fatalf("mint within coverage: %v", err)
	}
	if got := f.k.GetBalance(f.ts.Ctx, denomID, alice); got != 1000 {
		t.Fatalf("alice=%d want 1000", got)
	}
	if got := f.k.GetSupply(f.ts.Ctx, denomID); got != 1000 {
		t.Fatalf("supply=%d want 1000", got)
	}
	// one unit beyond attested reserve -> refused
	if err := f.k.BridgeMint(f.ts.Ctx, denomID, alice, 1); err == nil {
		t.Fatal("mint beyond reserve coverage must fail")
	}
	// top up the attested reserve -> headroom returns
	f.res.set("reserve.usdc", 2000, blockNow(f))
	if err := f.k.BridgeMint(f.ts.Ctx, denomID, alice, 500); err != nil {
		t.Fatalf("mint after reserve top-up: %v", err)
	}
}

// TestBridgeMintStaleReserve refuses a mint backed by a stale attestation.
func TestBridgeMintStaleReserve(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.bindReserve(t, "reserve.usdc", 10000, 60) // 60s staleness window
	f.res.set("reserve.usdc", 1000, blockNow(f)-120)
	if err := f.k.BridgeMint(f.ts.Ctx, denomID, alice, 100); err == nil {
		t.Fatal("mint with stale reserve must fail")
	}
}

// TestBridgeMintCompliance gates the recipient through the same chokepoint as a
// normal mint (sanctions / freeze).
func TestBridgeMintCompliance(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.bindReserve(t, "reserve.usdc", 10000, 0)
	f.res.set("reserve.usdc", 1000, blockNow(f))
	f.cmp.sanctioned[alice] = true
	if err := f.k.BridgeMint(f.ts.Ctx, denomID, alice, 100); err == nil {
		t.Fatal("sanctioned recipient bridge mint must fail")
	}
}

// TestBridgeBurn destroys balance and supply and is allowed during wind-down.
func TestBridgeBurn(t *testing.T) {
	f := setup(t)
	f.createDenom(t)
	f.bindReserve(t, "reserve.usdc", 10000, 0)
	f.res.set("reserve.usdc", 1000, blockNow(f))
	if err := f.k.BridgeMint(f.ts.Ctx, denomID, alice, 800); err != nil {
		t.Fatalf("mint: %v", err)
	}
	// over-burn -> fail closed, no state change
	if err := f.k.BridgeBurn(f.ts.Ctx, denomID, alice, 801); err == nil {
		t.Fatal("over-burn must fail")
	}
	if err := f.k.BridgeBurn(f.ts.Ctx, denomID, alice, 300); err != nil {
		t.Fatalf("burn: %v", err)
	}
	if got := f.k.GetBalance(f.ts.Ctx, denomID, alice); got != 500 {
		t.Fatalf("alice=%d want 500", got)
	}
	if got := f.k.GetSupply(f.ts.Ctx, denomID); got != 500 {
		t.Fatalf("supply=%d want 500", got)
	}
	// pause the denom (wind-down): burns still allowed, mints blocked
	if _, err := f.srv.SetDenomStatus(f.ts.Ctx, &types.MsgSetDenomStatus{
		Admin: admin, DenomId: denomID, Status: types.DenomStatus_DENOM_STATUS_PAUSED,
	}); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if err := f.k.BridgeBurn(f.ts.Ctx, denomID, alice, 100); err != nil {
		t.Fatalf("burn during pause should succeed: %v", err)
	}
	if err := f.k.BridgeMint(f.ts.Ctx, denomID, alice, 100); err == nil {
		t.Fatal("mint during pause must fail")
	}
}
