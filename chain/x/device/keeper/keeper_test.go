package keeper_test

import (
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

	"energychain/x/device/keeper"
	"energychain/x/device/types"
)

// stubDID accepts any address; lets tests focus on device-side state
// transitions without spinning up a real x/did keeper.
type stubDID struct{}

func (stubDID) IsActive(_ sdk.Context, _ string) bool { return true }

func setup(t *testing.T) (keeper.Keeper, sdk.Context) {
	t.Helper()
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	st := store.NewCommitMultiStore(db, log.NewNopLogger())
	st.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := st.LoadLatestVersion(); err != nil {
		t.Fatal(err)
	}
	reg := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(reg)

	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), "energy1auth", stubDID{})
	ctx := sdk.NewContext(st, cmtproto.Header{Time: time.Unix(1_700_000_000, 0)}, false, log.NewNopLogger())
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return k, ctx
}

func newDevice(did, owner string) types.Device {
	return types.Device{
		DeviceDid:    did,
		OwnerDid:     owner,
		Class:        types.DeviceClass_DEVICE_CLASS_METER,
		Model:        "x",
		SerialNumber: "y",
		FirmwareHash: "0xabc",
		GridZone:     "PJM.WHUB",
		Status:       types.AttestationStatus_ATTESTATION_STATUS_UNVERIFIED,
		RegisteredAt: 1,
	}
}

func TestSetGetIsAttested(t *testing.T) {
	k, ctx := setup(t)
	d := newDevice("energy1dev01", "energy1own01")
	if err := k.SetDevice(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, ok := k.GetDevice(ctx, "energy1dev01")
	if !ok {
		t.Fatalf("not found")
	}
	if got.OwnerDid != "energy1own01" {
		t.Errorf("owner=%s", got.OwnerDid)
	}
	if k.IsAttested(ctx, "energy1dev01") {
		t.Errorf("unattested device should not pass IsAttested")
	}
}

func TestExpireAttestations(t *testing.T) {
	k, ctx := setup(t)
	now := ctx.BlockTime().Unix()

	d := newDevice("energy1dev02", "energy1own02")
	d.Status = types.AttestationStatus_ATTESTATION_STATUS_ATTESTED
	d.AttestedAt = now - 1000
	d.AttestationExpiresAt = now - 1 // already expired
	if err := k.SetDevice(ctx, d); err != nil {
		t.Fatal(err)
	}
	n, err := k.ExpireAttestations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 hit, got %d", n)
	}
	got, _ := k.GetDevice(ctx, "energy1dev02")
	if got.Status != types.AttestationStatus_ATTESTATION_STATUS_SUSPECT {
		t.Errorf("status = %v after expiry", got.Status)
	}
}

func TestAttestationLifecycle(t *testing.T) {
	k, ctx := setup(t)
	id := k.NextAttestationID(ctx)
	ev := types.AttestationEvidence{
		DeviceDid:   "energy1dev03",
		VerifierDid: "energy1ver",
		Format:      "tdx-quote-v4",
		Evidence:    []byte("evidence"),
		Nonce:       []byte("nonce"),
		SubmittedAt: 1,
		Verdict:     types.VerdictPending,
	}
	if err := k.StoreAttestation(ctx, id, ev); err != nil {
		t.Fatal(err)
	}
	if _, err := k.ResolveAttestation(ctx, id, types.VerdictAccepted, "ok"); err != nil {
		t.Fatal(err)
	}
	got, _ := k.GetAttestation(ctx, id)
	if got.Verdict != types.VerdictAccepted {
		t.Errorf("verdict = %d", got.Verdict)
	}
}

// TestConfirmAcceptDoesNotResurrectRevoked locks in the security fix for
// the submit/revoke/confirm race: once a device is REVOKED, no
// late-arriving verifier accept may flip it back to ATTESTED.
func TestConfirmAcceptDoesNotResurrectRevoked(t *testing.T) {
	k, ctx := setup(t)

	// Configure a known verifier so ConfirmAttestation is authorized.
	params := types.DefaultParams()
	params.AttestationVerifiers = []string{"energy1verifier"}
	if err := k.SetParams(ctx, params); err != nil {
		t.Fatal(err)
	}

	// Register a device, then push it through the same shapes the
	// msg_server would: store, submit attestation, revoke device.
	d := newDevice("energy1dev04", "energy1own04")
	if err := k.SetDevice(ctx, d); err != nil {
		t.Fatal(err)
	}
	id := k.NextAttestationID(ctx)
	if err := k.StoreAttestation(ctx, id, types.AttestationEvidence{
		DeviceDid: "energy1dev04",
		Format:    "tdx-quote-v4",
		Evidence:  []byte("evidence"),
		Nonce:     []byte("nonce"),
		Verdict:   types.VerdictPending,
	}); err != nil {
		t.Fatal(err)
	}
	prev := d
	d.Status = types.AttestationStatus_ATTESTATION_STATUS_REVOKED
	if err := k.ClearDeviceIndexes(ctx, prev); err != nil {
		t.Fatal(err)
	}
	if err := k.SetDevice(ctx, d); err != nil {
		t.Fatal(err)
	}

	// Drive the confirm path through the message server.
	srv := keeper.NewMsgServerImpl(k)
	if _, err := srv.ConfirmAttestation(ctx, &types.MsgConfirmAttestation{
		Verifier:      "energy1verifier",
		AttestationId: id,
		Accept:        true,
		Reason:        "race",
	}); err != nil {
		t.Fatalf("confirm returned error: %v", err)
	}

	got, _ := k.GetDevice(ctx, "energy1dev04")
	if got.Status != types.AttestationStatus_ATTESTATION_STATUS_REVOKED {
		t.Errorf("device status = %v, want REVOKED (must not be resurrected)", got.Status)
	}
	if got.AttestedAt != 0 || got.AttestationExpiresAt != 0 {
		t.Errorf("attested_at=%d expires=%d, both must remain 0", got.AttestedAt, got.AttestationExpiresAt)
	}
}
