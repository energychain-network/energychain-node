package keeper_test

import (
	"testing"

	"energychain/x/assethub/types"
)

func (f *fixture) setParams(t *testing.T, mutate func(*types.Params)) {
	t.Helper()
	p := types.DefaultParams()
	mutate(&p)
	if err := f.k.SetParams(f.ts.Ctx, p); err != nil {
		t.Fatalf("set params: %v", err)
	}
}

func TestSubmitReadingRejectUnverified(t *testing.T) {
	f := setup(t)
	f.setParams(t, func(p *types.Params) { p.RejectUnverifiedReadings = true })
	f.registerProvider(t, meterPro, types.ProviderRole_PROVIDER_ROLE_METER, types.DefaultMinBond)
	f.registerDevice(t, meterPro, "dev-1")

	// Out of tolerance: with the gate on, this is rejected outright.
	if _, err := f.srv.SubmitReading(f.ts.Ctx, &types.MsgSubmitReading{
		Provider: meterPro, DeviceId: "dev-1", Unit: "kWh",
		PeriodStart: 1, PeriodEnd: 2, IotValue: 1000, OperationalValue: 1100,
	}); err == nil {
		t.Fatal("expected out-of-tolerance reading to be rejected")
	}

	// Within tolerance still succeeds.
	res, err := f.srv.SubmitReading(f.ts.Ctx, &types.MsgSubmitReading{
		Provider: meterPro, DeviceId: "dev-1", Unit: "kWh",
		PeriodStart: 2, PeriodEnd: 3, IotValue: 1000, OperationalValue: 1015,
	})
	if err != nil {
		t.Fatalf("verified reading rejected: %v", err)
	}
	if !res.Verified {
		t.Fatal("reading within tolerance should verify")
	}
}

func TestSubmitReadingRequireDeviceAttestation(t *testing.T) {
	f := setup(t)
	f.setParams(t, func(p *types.Params) { p.RequireDeviceAttestation = true })
	f.registerProvider(t, meterPro, types.ProviderRole_PROVIDER_ROLE_METER, types.DefaultMinBond)
	f.registerDevice(t, meterPro, "dev-1")

	// Unattested device: reading rejected while the gate is on.
	if _, err := f.srv.SubmitReading(f.ts.Ctx, &types.MsgSubmitReading{
		Provider: meterPro, DeviceId: "dev-1", Unit: "kWh", IotValue: 1, OperationalValue: 1,
	}); err == nil {
		t.Fatal("expected unattested device reading to be rejected")
	}

	// After attestation, the same reading is accepted.
	if _, err := f.srv.AttestDevice(f.ts.Ctx, &types.MsgAttestDevice{
		Operator: meterPro, Id: "dev-1", AttestationHash: "0xabc", Firmware: "v1",
	}); err != nil {
		t.Fatalf("attest: %v", err)
	}
	if _, err := f.srv.SubmitReading(f.ts.Ctx, &types.MsgSubmitReading{
		Provider: meterPro, DeviceId: "dev-1", Unit: "kWh", IotValue: 1, OperationalValue: 1,
	}); err != nil {
		t.Fatalf("attested device reading rejected: %v", err)
	}
}
