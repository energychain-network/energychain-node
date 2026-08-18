package keeper_test

import (
	"testing"

	"energychain/x/offering/keeper"
	"energychain/x/offering/types"
)

func (f *fixture) status(t *testing.T, id uint64) types.OfferingStatus {
	t.Helper()
	o, ok, err := f.k.GetOffering(f.ctx(), id)
	if err != nil || !ok {
		t.Fatalf("get offering %d: ok=%v err=%v", id, ok, err)
	}
	return o.Status
}

func TestSubscribeRejectsSanctionedInvestor(t *testing.T) {
	f := setup(t)
	id := f.stdOffering(t, 300, 1000, 1, 0, 0)
	f.cmp.sanctioned[alice] = true
	f.set.fund(denom, alice, 100)
	if _, err := f.srv.Subscribe(f.ctx(), &types.MsgSubscribe{Investor: alice, OfferingId: id, Amount: 100}); err == nil {
		t.Fatal("expected sanctioned investor to be rejected")
	}
	// Funds must remain with the investor (no escrow happened).
	if f.bal(alice) != 100 {
		t.Fatalf("alice balance=%d want 100 (no escrow on rejection)", f.bal(alice))
	}
}

func TestSubscribeRequiresKYCWhenTokenGated(t *testing.T) {
	f := setup(t)
	f.rwa.kyc[tokenID] = true // underlying security requires KYC
	id := f.stdOffering(t, 300, 1000, 1, 0, 0)

	f.cmp.noKyc[alice] = true
	f.set.fund(denom, alice, 100)
	if _, err := f.srv.Subscribe(f.ctx(), &types.MsgSubscribe{Investor: alice, OfferingId: id, Amount: 100}); err == nil {
		t.Fatal("expected non-KYC investor to be rejected on a KYC-gated token")
	}

	// Once KYC clears, the same subscription succeeds.
	delete(f.cmp.noKyc, alice)
	if _, err := f.srv.Subscribe(f.ctx(), &types.MsgSubscribe{Investor: alice, OfferingId: id, Amount: 100}); err != nil {
		t.Fatalf("kyc-cleared subscribe: %v", err)
	}
}

func TestEndBlockAutoCloseSucceedsPastWindow(t *testing.T) {
	f := setup(t)
	id := f.stdOffering(t, 300, 1000, 1, 0, 0)
	f.subscribe(t, id, alice, 600) // >= soft cap

	f.ts.Advance(2000) // past end_time
	if err := f.k.EndBlock(f.ctx()); err != nil {
		t.Fatalf("endblock: %v", err)
	}
	if s := f.status(t, id); s != types.OfferingStatus_OFFERING_STATUS_SUCCEEDED {
		t.Fatalf("status=%v want SUCCEEDED", s)
	}
	// Allocation works after the automatic close.
	if _, err := f.srv.ClaimAllocation(f.ctx(), &types.MsgClaimAllocation{Investor: alice, OfferingId: id}); err != nil {
		t.Fatalf("alloc after auto-close: %v", err)
	}
}

func TestEndBlockAutoCloseFailsBelowSoftCap(t *testing.T) {
	f := setup(t)
	id := f.stdOffering(t, 300, 1000, 1, 0, 0)
	f.subscribe(t, id, alice, 100) // below soft cap

	f.ts.Advance(2000)
	if err := f.k.EndBlock(f.ctx()); err != nil {
		t.Fatalf("endblock: %v", err)
	}
	if s := f.status(t, id); s != types.OfferingStatus_OFFERING_STATUS_FAILED {
		t.Fatalf("status=%v want FAILED", s)
	}
	// Failed raise refunds the full contribution.
	if _, err := f.srv.ClaimRefund(f.ctx(), &types.MsgClaimRefund{Investor: alice, OfferingId: id}); err != nil {
		t.Fatalf("refund: %v", err)
	}
	if f.bal(alice) != 100 {
		t.Fatalf("alice refunded balance=%d want 100", f.bal(alice))
	}
}

func TestEndBlockAutoCloseOnHardCapBeforeWindow(t *testing.T) {
	f := setup(t)
	id := f.stdOffering(t, 300, 1000, 1, 0, 0)
	f.subscribe(t, id, alice, 1000) // hits hard cap, end_time not reached

	if err := f.k.EndBlock(f.ctx()); err != nil {
		t.Fatalf("endblock: %v", err)
	}
	if s := f.status(t, id); s != types.OfferingStatus_OFFERING_STATUS_SUCCEEDED {
		t.Fatalf("status=%v want SUCCEEDED on hard-cap", s)
	}
}

func TestEndBlockAutoDefaultsDelinquent(t *testing.T) {
	f := setup(t)
	// 2 tranches, injection required every 100s.
	id := f.stdOffering(t, 300, 1000, 2, 10, 100)
	f.subscribe(t, id, alice, 600)

	// Close successfully first.
	f.ts.Advance(2000)
	if err := f.k.EndBlock(f.ctx()); err != nil {
		t.Fatalf("endblock close: %v", err)
	}
	if s := f.status(t, id); s != types.OfferingStatus_OFFERING_STATUS_SUCCEEDED {
		t.Fatalf("status=%v want SUCCEEDED before default", s)
	}

	// Advance past the first injection deadline without injecting.
	f.ts.Advance(200)
	if err := f.k.EndBlock(f.ctx()); err != nil {
		t.Fatalf("endblock default: %v", err)
	}
	if s := f.status(t, id); s != types.OfferingStatus_OFFERING_STATUS_DEFAULTED {
		t.Fatalf("status=%v want DEFAULTED", s)
	}
	// Treasury was snapshotted for pro-rata refunds.
	o, _, _ := f.k.GetOffering(f.ctx(), id)
	if o.DefaultTreasury != f.bal(keeper.TreasuryAccount(id)) {
		t.Fatalf("default treasury snapshot=%d, live=%d", o.DefaultTreasury, f.bal(keeper.TreasuryAccount(id)))
	}
}
