package keeper_test

import (
	"context"
	"fmt"
	"testing"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/testutil"
	"energychain/x/assethub/keeper"
	"energychain/x/assethub/types"
)

var (
	authority  = testutil.DeriveAddr("authority")
	meterPro   = testutil.DeriveAddr("meter-provider")
	oraclePro  = testutil.DeriveAddr("oracle-provider")
	oraclePro2 = testutil.DeriveAddr("oracle-provider-2")
	oraclePro3 = testutil.DeriveAddr("oracle-provider-3")
	stranger   = testutil.DeriveAddr("stranger")
	bondDenom  = "uecy"
	startFunds = uint64(1_000_000_000)
)

// ---- mock bank -------------------------------------------------------------

type mockBank struct {
	accounts map[string]sdk.Coins
	modules  map[string]sdk.Coins
	burned   sdk.Coins
}

func newMockBank() *mockBank {
	return &mockBank{accounts: map[string]sdk.Coins{}, modules: map[string]sdk.Coins{}}
}

func (b *mockBank) fund(addr string, amount uint64) {
	b.accounts[addr] = b.accounts[addr].Add(sdk.NewCoin(bondDenom, math.NewIntFromUint64(amount)))
}

func (b *mockBank) SendCoinsFromAccountToModule(_ context.Context, sender sdk.AccAddress, mod string, amt sdk.Coins) error {
	bal := b.accounts[sender.String()]
	if !bal.IsAllGTE(amt) {
		return fmt.Errorf("insufficient funds: have %s need %s", bal, amt)
	}
	b.accounts[sender.String()] = bal.Sub(amt...)
	b.modules[mod] = b.modules[mod].Add(amt...)
	return nil
}

func (b *mockBank) SendCoinsFromModuleToAccount(_ context.Context, mod string, recipient sdk.AccAddress, amt sdk.Coins) error {
	bal := b.modules[mod]
	if !bal.IsAllGTE(amt) {
		return fmt.Errorf("module %s insufficient: have %s need %s", mod, bal, amt)
	}
	b.modules[mod] = bal.Sub(amt...)
	b.accounts[recipient.String()] = b.accounts[recipient.String()].Add(amt...)
	return nil
}

func (b *mockBank) BurnCoins(_ context.Context, mod string, amt sdk.Coins) error {
	bal := b.modules[mod]
	if !bal.IsAllGTE(amt) {
		return fmt.Errorf("module %s insufficient to burn: have %s need %s", mod, bal, amt)
	}
	b.modules[mod] = bal.Sub(amt...)
	b.burned = b.burned.Add(amt...)
	return nil
}

func (b *mockBank) moduleBalance() uint64 {
	return b.modules[types.ModuleName].AmountOf(bondDenom).Uint64()
}
func (b *mockBank) accountBalance(addr string) uint64 {
	return b.accounts[addr].AmountOf(bondDenom).Uint64()
}

// ---- fixture ---------------------------------------------------------------

type fixture struct {
	ts   *testutil.TestStore
	k    keeper.Keeper
	srv  types.MsgServer
	q    types.QueryServer
	bank *mockBank
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ts := testutil.NewTestStore(t, types.StoreKey)
	bank := newMockBank()
	k := keeper.NewKeeper(ts.Cdc, runtime.NewKVStoreService(ts.Key(t, types.StoreKey)), authority, bank, nil)
	if err := k.SetParams(ts.Ctx, types.DefaultParams()); err != nil {
		t.Fatalf("set params: %v", err)
	}
	bank.fund(meterPro, startFunds)
	bank.fund(oraclePro, startFunds)
	bank.fund(oraclePro2, startFunds)
	bank.fund(oraclePro3, startFunds)
	bank.fund(stranger, startFunds)
	return &fixture{ts: ts, k: k, srv: keeper.NewMsgServerImpl(k), q: keeper.NewQueryServerImpl(k), bank: bank}
}

func (f *fixture) registerProvider(t *testing.T, addr string, role types.ProviderRole, bond uint64) {
	t.Helper()
	if _, err := f.srv.RegisterProvider(f.ts.Ctx, &types.MsgRegisterProvider{
		Provider: addr, Role: role, DisplayName: "p", Bond: bond,
	}); err != nil {
		t.Fatalf("register provider %s: %v", addr, err)
	}
}

func (f *fixture) registerDevice(t *testing.T, op, id string) {
	t.Helper()
	if _, err := f.srv.RegisterDevice(f.ts.Ctx, &types.MsgRegisterDevice{
		Operator: op, Id: id, DeviceType: "charging_pile", Jurisdiction: "HK",
	}); err != nil {
		t.Fatalf("register device %s: %v", id, err)
	}
}

// ---- provider lifecycle ----------------------------------------------------

func TestRegisterProvider(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, meterPro, types.ProviderRole_PROVIDER_ROLE_METER, types.DefaultMinBond)

	if got := f.bank.moduleBalance(); got != types.DefaultMinBond {
		t.Fatalf("module bond balance = %d, want %d", got, types.DefaultMinBond)
	}
	if got := f.bank.accountBalance(meterPro); got != startFunds-types.DefaultMinBond {
		t.Fatalf("provider balance = %d, want %d", got, startFunds-types.DefaultMinBond)
	}
	p, ok := f.k.GetProvider(f.ts.Ctx, meterPro)
	if !ok || p.Status != types.ProviderStatus_PROVIDER_STATUS_ACTIVE {
		t.Fatalf("provider not active: %+v ok=%v", p, ok)
	}
}

func TestRegisterProviderBelowMinBond(t *testing.T) {
	f := setup(t)
	_, err := f.srv.RegisterProvider(f.ts.Ctx, &types.MsgRegisterProvider{
		Provider: meterPro, Role: types.ProviderRole_PROVIDER_ROLE_METER, Bond: types.DefaultMinBond - 1,
	})
	if err == nil {
		t.Fatal("expected min-bond rejection")
	}
	if f.bank.moduleBalance() != 0 {
		t.Fatal("no bond should have been collected on failure")
	}
}

func TestRegisterProviderDuplicate(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, meterPro, types.ProviderRole_PROVIDER_ROLE_METER, types.DefaultMinBond)
	_, err := f.srv.RegisterProvider(f.ts.Ctx, &types.MsgRegisterProvider{
		Provider: meterPro, Role: types.ProviderRole_PROVIDER_ROLE_METER, Bond: types.DefaultMinBond,
	})
	if err == nil {
		t.Fatal("expected duplicate rejection")
	}
}

func TestIncreaseAndWithdrawBond(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, meterPro, types.ProviderRole_PROVIDER_ROLE_METER, types.DefaultMinBond)

	if _, err := f.srv.IncreaseBond(f.ts.Ctx, &types.MsgIncreaseBond{Provider: meterPro, Amount: 500}); err != nil {
		t.Fatalf("increase: %v", err)
	}
	if got := f.bank.moduleBalance(); got != types.DefaultMinBond+500 {
		t.Fatalf("module balance = %d", got)
	}
	// withdraw the surplus only
	if _, err := f.srv.WithdrawBond(f.ts.Ctx, &types.MsgWithdrawBond{Provider: meterPro, Amount: 500}); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	// cannot withdraw below min bond
	if _, err := f.srv.WithdrawBond(f.ts.Ctx, &types.MsgWithdrawBond{Provider: meterPro, Amount: 1}); err == nil {
		t.Fatal("expected min-bond floor on withdraw")
	}
}

func TestDeregisterRefundsAndBlocksWithDevices(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, meterPro, types.ProviderRole_PROVIDER_ROLE_METER, types.DefaultMinBond)
	f.registerDevice(t, meterPro, "dev-1")

	if _, err := f.srv.DeregisterProvider(f.ts.Ctx, &types.MsgDeregisterProvider{Provider: meterPro}); err == nil {
		t.Fatal("expected deregister blocked by active device")
	}
	// revoke device, then deregister succeeds and refunds the bond
	if _, err := f.srv.RevokeDevice(f.ts.Ctx, &types.MsgRevokeDevice{Actor: meterPro, Id: "dev-1", Reason: "decommission"}); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := f.srv.DeregisterProvider(f.ts.Ctx, &types.MsgDeregisterProvider{Provider: meterPro}); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	if got := f.bank.accountBalance(meterPro); got != startFunds {
		t.Fatalf("bond not fully refunded: %d", got)
	}
	if _, ok := f.k.GetProvider(f.ts.Ctx, meterPro); ok {
		t.Fatal("provider should be removed")
	}
}

// ---- slashing & jailing ----------------------------------------------------

func TestSlashBurnsBondAndJails(t *testing.T) {
	f := setup(t)
	bond := uint64(10_000_000)
	f.registerProvider(t, oraclePro, types.ProviderRole_PROVIDER_ROLE_ORACLE, bond)

	params, _ := f.k.GetParams(f.ts.Ctx)
	expectSlash := types.MulBps(bond, params.SlashFractionBps)

	res, err := f.srv.SlashProvider(f.ts.Ctx, &types.MsgSlashProvider{Authority: authority, Provider: oraclePro, Reason: "bad data"})
	if err != nil {
		t.Fatalf("slash: %v", err)
	}
	if res.Slashed != expectSlash {
		t.Fatalf("slashed = %d, want %d", res.Slashed, expectSlash)
	}
	if got := f.bank.burned.AmountOf(bondDenom).Uint64(); got != expectSlash {
		t.Fatalf("burned = %d, want %d", got, expectSlash)
	}
	// not yet jailed (1 < threshold 3)
	if res.Jailed {
		t.Fatal("should not jail on first infraction")
	}
	// slash twice more to cross the threshold
	_, _ = f.srv.SlashProvider(f.ts.Ctx, &types.MsgSlashProvider{Authority: authority, Provider: oraclePro})
	res, err = f.srv.SlashProvider(f.ts.Ctx, &types.MsgSlashProvider{Authority: authority, Provider: oraclePro})
	if err != nil {
		t.Fatalf("slash: %v", err)
	}
	if !res.Jailed {
		t.Fatal("expected jail at infraction threshold")
	}
	p, _ := f.k.GetProvider(f.ts.Ctx, oraclePro)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_JAILED {
		t.Fatalf("status = %v", p.Status)
	}
	// jailed provider cannot submit
	if err := f.submitValueExpectErr(t); err == nil {
		t.Fatal("jailed oracle must not submit values")
	}
}

func (f *fixture) submitValueExpectErr(t *testing.T) error {
	t.Helper()
	if _, err := f.srv.CreateTopic(f.ts.Ctx, &types.MsgCreateTopic{Authority: authority, Id: "fx.usd", MinSources: 1}); err != nil {
		// topic may already exist; ignore
		_ = err
	}
	_, err := f.srv.SubmitValue(f.ts.Ctx, &types.MsgSubmitValue{Provider: oraclePro, TopicId: "fx.usd", Value: 7})
	return err
}

func TestUnjailRequiresMinBondAndResets(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, oraclePro, types.ProviderRole_PROVIDER_ROLE_ORACLE, types.DefaultMinBond)
	for i := 0; i < 3; i++ {
		if _, err := f.srv.SlashProvider(f.ts.Ctx, &types.MsgSlashProvider{Authority: authority, Provider: oraclePro}); err != nil {
			t.Fatalf("slash %d: %v", i, err)
		}
	}
	// bond now below min; unjail must fail until topped up
	if _, err := f.srv.UnjailProvider(f.ts.Ctx, &types.MsgUnjailProvider{Authority: authority, Provider: oraclePro}); err == nil {
		t.Fatal("expected unjail blocked below min bond")
	}
	if _, err := f.srv.IncreaseBond(f.ts.Ctx, &types.MsgIncreaseBond{Provider: oraclePro, Amount: types.DefaultMinBond}); err != nil {
		t.Fatalf("topup: %v", err)
	}
	if _, err := f.srv.UnjailProvider(f.ts.Ctx, &types.MsgUnjailProvider{Authority: authority, Provider: oraclePro}); err != nil {
		t.Fatalf("unjail: %v", err)
	}
	p, _ := f.k.GetProvider(f.ts.Ctx, oraclePro)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_ACTIVE || p.Infractions != 0 {
		t.Fatalf("unjail did not reset: %+v", p)
	}
}

func TestSlashRequiresAuthority(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, oraclePro, types.ProviderRole_PROVIDER_ROLE_ORACLE, types.DefaultMinBond)
	if _, err := f.srv.SlashProvider(f.ts.Ctx, &types.MsgSlashProvider{Authority: stranger, Provider: oraclePro}); err == nil {
		t.Fatal("expected authority gate on slash")
	}
}

func TestJailAutoReactivatesAfterWindow(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, meterPro, types.ProviderRole_PROVIDER_ROLE_METER, 100_000_000)
	for i := 0; i < 3; i++ {
		if _, err := f.srv.SlashProvider(f.ts.Ctx, &types.MsgSlashProvider{Authority: authority, Provider: meterPro}); err != nil {
			t.Fatalf("slash %d: %v", i, err)
		}
	}
	p, _ := f.k.GetProvider(f.ts.Ctx, meterPro)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_JAILED {
		t.Fatalf("expected jailed, got %v", p.Status)
	}
	// while jailed, role-gated actions are rejected
	if _, err := f.srv.RegisterDevice(f.ts.Ctx, &types.MsgRegisterDevice{
		Operator: meterPro, Id: "dev-jail", DeviceType: "pv_inverter",
	}); err == nil {
		t.Fatal("jailed provider must not register devices")
	}
	// advance past the jail window: the next role-gated action auto-reactivates
	params, _ := f.k.GetParams(f.ts.Ctx)
	f.ts.Advance(params.JailDurationSeconds + 1)
	f.registerDevice(t, meterPro, "dev-ok")
	p, _ = f.k.GetProvider(f.ts.Ctx, meterPro)
	if p.Status != types.ProviderStatus_PROVIDER_STATUS_ACTIVE {
		t.Fatalf("expected auto-reactivation, got %v", p.Status)
	}
}

// ---- devices ---------------------------------------------------------------

func TestRegisterDeviceRoleGate(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, oraclePro, types.ProviderRole_PROVIDER_ROLE_ORACLE, types.DefaultMinBond)
	// oracle role cannot operate devices
	if _, err := f.srv.RegisterDevice(f.ts.Ctx, &types.MsgRegisterDevice{
		Operator: oraclePro, Id: "dev-x", DeviceType: "pv_inverter",
	}); err == nil {
		t.Fatal("oracle provider must not register devices")
	}
}

func TestDeviceRevokeAuthorisation(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, meterPro, types.ProviderRole_PROVIDER_ROLE_METER, types.DefaultMinBond)
	f.registerDevice(t, meterPro, "dev-1")

	// stranger cannot revoke
	if _, err := f.srv.RevokeDevice(f.ts.Ctx, &types.MsgRevokeDevice{Actor: stranger, Id: "dev-1"}); err == nil {
		t.Fatal("stranger must not revoke device")
	}
	// authority can revoke even though not operator
	if _, err := f.srv.RevokeDevice(f.ts.Ctx, &types.MsgRevokeDevice{Actor: authority, Id: "dev-1", Reason: "regulatory"}); err != nil {
		t.Fatalf("authority revoke: %v", err)
	}
	d, _ := f.k.Devices.Get(f.ts.Ctx, "dev-1")
	if d.Status != types.DeviceStatus_DEVICE_STATUS_REVOKED {
		t.Fatalf("device not revoked: %v", d.Status)
	}
}

func TestAttestDevice(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, meterPro, types.ProviderRole_PROVIDER_ROLE_METER, types.DefaultMinBond)
	f.registerDevice(t, meterPro, "dev-1")
	if _, err := f.srv.AttestDevice(f.ts.Ctx, &types.MsgAttestDevice{
		Operator: meterPro, Id: "dev-1", AttestationHash: "0xabc", Firmware: "v1.2.3",
	}); err != nil {
		t.Fatalf("attest: %v", err)
	}
	d, _ := f.k.Devices.Get(f.ts.Ctx, "dev-1")
	if d.AttestationHash != "0xabc" || d.Firmware != "v1.2.3" {
		t.Fatalf("attestation not stored: %+v", d)
	}
}

// ---- metering readings -----------------------------------------------------

func TestSubmitReadingCrossVerification(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, meterPro, types.ProviderRole_PROVIDER_ROLE_METER, types.DefaultMinBond)
	f.registerDevice(t, meterPro, "dev-1")

	// within 2% tolerance => verified
	res, err := f.srv.SubmitReading(f.ts.Ctx, &types.MsgSubmitReading{
		Provider: meterPro, DeviceId: "dev-1", Unit: "kWh",
		PeriodStart: 1, PeriodEnd: 2, IotValue: 1000, OperationalValue: 1015,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !res.Verified {
		t.Fatal("reading within tolerance should verify")
	}
	if res.ReadingId != 1 {
		t.Fatalf("reading id = %d, want 1", res.ReadingId)
	}

	// outside 2% tolerance => not verified
	res, err = f.srv.SubmitReading(f.ts.Ctx, &types.MsgSubmitReading{
		Provider: meterPro, DeviceId: "dev-1", Unit: "kWh",
		PeriodStart: 2, PeriodEnd: 3, IotValue: 1000, OperationalValue: 1100,
	})
	if err != nil {
		t.Fatalf("submit2: %v", err)
	}
	if res.Verified {
		t.Fatal("reading outside tolerance must not verify")
	}
}

func TestSubmitReadingOwnershipAndRevocation(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, meterPro, types.ProviderRole_PROVIDER_ROLE_METER, types.DefaultMinBond)
	f.registerProvider(t, oraclePro, types.ProviderRole_PROVIDER_ROLE_METER, types.DefaultMinBond) // second meter
	f.registerDevice(t, meterPro, "dev-1")

	// a different provider cannot submit for someone else's device
	if _, err := f.srv.SubmitReading(f.ts.Ctx, &types.MsgSubmitReading{
		Provider: oraclePro, DeviceId: "dev-1", Unit: "kWh", IotValue: 1, OperationalValue: 1,
	}); err == nil {
		t.Fatal("non-operator must not submit readings")
	}
	// revoked device rejects readings
	_, _ = f.srv.RevokeDevice(f.ts.Ctx, &types.MsgRevokeDevice{Actor: meterPro, Id: "dev-1"})
	if _, err := f.srv.SubmitReading(f.ts.Ctx, &types.MsgSubmitReading{
		Provider: meterPro, DeviceId: "dev-1", Unit: "kWh", IotValue: 1, OperationalValue: 1,
	}); err == nil {
		t.Fatal("revoked device must reject readings")
	}
}

// ---- oracle ----------------------------------------------------------------

func TestOracleMedianAggregation(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, oraclePro, types.ProviderRole_PROVIDER_ROLE_ORACLE, types.DefaultMinBond)
	f.registerProvider(t, oraclePro2, types.ProviderRole_PROVIDER_ROLE_ORACLE, types.DefaultMinBond)
	f.registerProvider(t, oraclePro3, types.ProviderRole_PROVIDER_ROLE_ORACLE, types.DefaultMinBond)

	if _, err := f.srv.CreateTopic(f.ts.Ctx, &types.MsgCreateTopic{
		Authority: authority, Id: "fx.usd", Description: "USD/local", MinSources: 3,
	}); err != nil {
		t.Fatalf("create topic: %v", err)
	}

	// first two submissions: below min_sources -> no trusted value
	res, _ := f.srv.SubmitValue(f.ts.Ctx, &types.MsgSubmitValue{Provider: oraclePro, TopicId: "fx.usd", Value: 100})
	if res.HasValue {
		t.Fatal("should not have value before min_sources")
	}
	_, _ = f.srv.SubmitValue(f.ts.Ctx, &types.MsgSubmitValue{Provider: oraclePro2, TopicId: "fx.usd", Value: 300})
	res, _ = f.srv.SubmitValue(f.ts.Ctx, &types.MsgSubmitValue{Provider: oraclePro3, TopicId: "fx.usd", Value: 200})
	if !res.HasValue || res.AggregatedValue != 200 {
		t.Fatalf("median = %d hasValue=%v, want 200/true", res.AggregatedValue, res.HasValue)
	}
	// an updated submission re-aggregates
	res, _ = f.srv.SubmitValue(f.ts.Ctx, &types.MsgSubmitValue{Provider: oraclePro, TopicId: "fx.usd", Value: 500})
	if res.AggregatedValue != 300 { // median(500,300,200)=300
		t.Fatalf("re-aggregated median = %d, want 300", res.AggregatedValue)
	}
}

func TestCreateTopicRequiresAuthority(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.CreateTopic(f.ts.Ctx, &types.MsgCreateTopic{Authority: stranger, Id: "fx.usd", MinSources: 1}); err == nil {
		t.Fatal("expected authority gate on create topic")
	}
}

func TestSubmitValueRoleGate(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, meterPro, types.ProviderRole_PROVIDER_ROLE_METER, types.DefaultMinBond)
	_, _ = f.srv.CreateTopic(f.ts.Ctx, &types.MsgCreateTopic{Authority: authority, Id: "fx.usd", MinSources: 1})
	if _, err := f.srv.SubmitValue(f.ts.Ctx, &types.MsgSubmitValue{Provider: meterPro, TopicId: "fx.usd", Value: 1}); err == nil {
		t.Fatal("meter provider must not submit oracle values")
	}
}

// ---- params ----------------------------------------------------------------

func TestUpdateParamsAuthorityAndValidation(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.UpdateParams(f.ts.Ctx, &types.MsgUpdateParams{Authority: stranger, Params: types.DefaultParams()}); err == nil {
		t.Fatal("expected authority gate")
	}
	bad := types.DefaultParams()
	bad.SlashFractionBps = 20000
	if _, err := f.srv.UpdateParams(f.ts.Ctx, &types.MsgUpdateParams{Authority: authority, Params: bad}); err == nil {
		t.Fatal("expected param validation rejection")
	}
}

// ---- genesis roundtrip -----------------------------------------------------

func TestGenesisRoundtrip(t *testing.T) {
	f := setup(t)
	f.registerProvider(t, meterPro, types.ProviderRole_PROVIDER_ROLE_METER, types.DefaultMinBond)
	f.registerProvider(t, oraclePro, types.ProviderRole_PROVIDER_ROLE_ORACLE, types.DefaultMinBond)
	f.registerDevice(t, meterPro, "dev-1")
	_, _ = f.srv.SubmitReading(f.ts.Ctx, &types.MsgSubmitReading{
		Provider: meterPro, DeviceId: "dev-1", Unit: "kWh", IotValue: 100, OperationalValue: 101,
	})
	_, _ = f.srv.CreateTopic(f.ts.Ctx, &types.MsgCreateTopic{Authority: authority, Id: "fx.usd", MinSources: 1})
	_, _ = f.srv.SubmitValue(f.ts.Ctx, &types.MsgSubmitValue{Provider: oraclePro, TopicId: "fx.usd", Value: 42})

	exported, err := f.k.ExportGenesis(f.ts.Ctx)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if err := exported.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}

	// re-import into a fresh keeper
	g := setup(t)
	if err := g.k.InitGenesis(g.ts.Ctx, exported); err != nil {
		t.Fatalf("init: %v", err)
	}
	reexported, err := g.k.ExportGenesis(g.ts.Ctx)
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if len(reexported.Providers) != 2 || len(reexported.Devices) != 1 ||
		len(reexported.Readings) != 1 || len(reexported.Topics) != 1 || len(reexported.Submissions) != 1 {
		t.Fatalf("roundtrip lost data: %+v", reexported)
	}
	if reexported.ReadingIdSeq != exported.ReadingIdSeq {
		t.Fatalf("reading seq drift: %d vs %d", reexported.ReadingIdSeq, exported.ReadingIdSeq)
	}
}
