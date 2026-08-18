package keeper_test

import (
	"testing"

	"energychain/x/rwatoken/types"
)

// ---- device binding (gap A) ------------------------------------------------

func TestCreateTokenDeviceBindingHappyPath(t *testing.T) {
	f := setup(t)
	f.ah.add("pv-1", admin, true)
	f.ah.add("pv-2", admin, true)
	id := f.createToken(t, &types.MsgCreateToken{
		Symbol:    "PV",
		DeviceIds: []string{"pv-1", "pv-2"},
	})
	tok, ok, err := f.k.GetToken(f.ctx(), id)
	if err != nil || !ok {
		t.Fatalf("get token: %v ok=%v", err, ok)
	}
	if len(tok.DeviceIds) != 2 || tok.DeviceIds[0] != "pv-1" || tok.DeviceIds[1] != "pv-2" {
		t.Fatalf("device ids not stored: %+v", tok.DeviceIds)
	}
}

func TestCreateTokenDeviceNotFound(t *testing.T) {
	f := setup(t)
	_, err := f.srv.CreateToken(f.ctx(), &types.MsgCreateToken{
		Admin: admin, Symbol: "PV", AssetClass: "solar_pv_revenue",
		SettlementDenom: settleDenom, DeviceIds: []string{"ghost"},
	})
	if err == nil {
		t.Fatal("expected error binding a non-existent device")
	}
}

func TestCreateTokenDeviceNotActive(t *testing.T) {
	f := setup(t)
	f.ah.add("pv-1", admin, false) // revoked / inactive
	_, err := f.srv.CreateToken(f.ctx(), &types.MsgCreateToken{
		Admin: admin, Symbol: "PV", AssetClass: "solar_pv_revenue",
		SettlementDenom: settleDenom, DeviceIds: []string{"pv-1"},
	})
	if err == nil {
		t.Fatal("expected error binding an inactive device")
	}
}

func TestCreateTokenDeviceWrongOperator(t *testing.T) {
	f := setup(t)
	f.ah.add("pv-1", alice, true) // operated by someone other than admin
	_, err := f.srv.CreateToken(f.ctx(), &types.MsgCreateToken{
		Admin: admin, Symbol: "PV", AssetClass: "solar_pv_revenue",
		SettlementDenom: settleDenom, DeviceIds: []string{"pv-1"},
	})
	if err == nil {
		t.Fatal("expected error binding a device operated by a third party")
	}
}

func TestUpdateTokenRebindsDevices(t *testing.T) {
	f := setup(t)
	f.ah.add("pv-1", admin, true)
	f.ah.add("pv-2", admin, true)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "PV", DeviceIds: []string{"pv-1"}})

	if _, err := f.srv.UpdateToken(f.ctx(), &types.MsgUpdateToken{
		Admin: admin, TokenId: id, DeviceIds: []string{"pv-1", "pv-2"},
	}); err != nil {
		t.Fatalf("update token: %v", err)
	}
	tok, _, _ := f.k.GetToken(f.ctx(), id)
	if len(tok.DeviceIds) != 2 {
		t.Fatalf("rebind failed: %+v", tok.DeviceIds)
	}

	// Rebinding to a device operated by someone else must fail.
	f.ah.add("pv-3", bob, true)
	if _, err := f.srv.UpdateToken(f.ctx(), &types.MsgUpdateToken{
		Admin: admin, TokenId: id, DeviceIds: []string{"pv-3"},
	}); err == nil {
		t.Fatal("expected error rebinding to a third-party device")
	}
}

// ---- maturity auto-expire (gap B) ------------------------------------------

func TestEndBlockAutoMatures(t *testing.T) {
	f := setup(t)
	matureAt := f.ts.Ctx.BlockTime().Unix() + 100
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "PV", MaturityTime: matureAt})
	f.mint(t, id, alice, 1000)

	// Before maturity: EndBlock is a no-op, token stays ACTIVE and
	// transfers work.
	if err := f.k.EndBlock(f.ctx()); err != nil {
		t.Fatalf("endblock: %v", err)
	}
	if tok, _, _ := f.k.GetToken(f.ctx(), id); tok.Status != types.TokenStatus_TOKEN_STATUS_ACTIVE {
		t.Fatalf("token matured early: %v", tok.Status)
	}
	if _, err := f.srv.Transfer(f.ctx(), &types.MsgTransfer{From: alice, TokenId: id, To: bob, Amount: 10}); err != nil {
		t.Fatalf("pre-maturity transfer: %v", err)
	}

	// Cross maturity, run EndBlock: token becomes MATURED.
	f.ts.Advance(200)
	if err := f.k.EndBlock(f.ctx()); err != nil {
		t.Fatalf("endblock after maturity: %v", err)
	}
	tok, _, _ := f.k.GetToken(f.ctx(), id)
	if tok.Status != types.TokenStatus_TOKEN_STATUS_MATURED {
		t.Fatalf("token not matured: %v", tok.Status)
	}

	// Transfers are now blocked...
	if _, err := f.srv.Transfer(f.ctx(), &types.MsgTransfer{From: alice, TokenId: id, To: bob, Amount: 10}); err == nil {
		t.Fatal("expected transfer to fail on matured token")
	}
	// ...but redemption requests remain open (price set).
	if _, err := f.srv.UpdateToken(f.ctx(), &types.MsgUpdateToken{Admin: admin, TokenId: id, RedemptionPrice: 5}); err != nil {
		t.Fatalf("update redemption price: %v", err)
	}
	if _, err := f.srv.RequestRedemption(f.ctx(), &types.MsgRequestRedemption{Holder: alice, TokenId: id, Units: 10}); err != nil {
		t.Fatalf("redemption on matured token should be allowed: %v", err)
	}
}

func TestEndBlockPerpetualNeverMatures(t *testing.T) {
	f := setup(t)
	id := f.createToken(t, &types.MsgCreateToken{Symbol: "PV", MaturityTime: 0})
	f.ts.Advance(10 * 365 * 24 * 3600)
	if err := f.k.EndBlock(f.ctx()); err != nil {
		t.Fatalf("endblock: %v", err)
	}
	if tok, _, _ := f.k.GetToken(f.ctx(), id); tok.Status != types.TokenStatus_TOKEN_STATUS_ACTIVE {
		t.Fatalf("perpetual token matured: %v", tok.Status)
	}
}
