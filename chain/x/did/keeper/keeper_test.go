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

	"energychain/x/did/keeper"
	"energychain/x/did/types"
)

const testAuthority = "energy1authoritytestauthoritytestauthorityqwer"

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

	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), testAuthority)
	ctx := sdk.NewContext(st, cmtproto.Header{Time: time.Unix(1_700_000_000, 0)}, false, log.NewNopLogger())
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return k, ctx
}

func TestSetGetDocument(t *testing.T) {
	k, ctx := setup(t)
	doc := types.DIDDocument{
		Id:          "energy1alice",
		Controllers: []string{"energy1alice"},
		Verification: []types.VerificationMethod{{
			Id: "key-1", Controller: "energy1alice",
			KeyType: types.KeyType_KEY_TYPE_SECP256K1, PublicKey: make([]byte, 33),
			Purposes: []string{types.PurposeAuthentication},
		}},
		Status: types.StatusActive, Version: 1,
	}
	if err := k.SetDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}
	got, ok := k.GetDocument(ctx, "energy1alice")
	if !ok {
		t.Fatalf("doc not found")
	}
	if got.Status != types.StatusActive {
		t.Errorf("status = %s", got.Status)
	}
	if !k.IsController(ctx, "energy1alice", "energy1alice") {
		t.Errorf("IsController should be true for self")
	}
	if k.IsController(ctx, "energy1alice", "energy1bob") {
		t.Errorf("IsController should be false for non-controller")
	}
}

func TestCredentialIssueAndExpiry(t *testing.T) {
	k, ctx := setup(t)
	now := ctx.BlockTime().Unix()

	c := types.CredentialStatus{
		Id:        "urn:uuid:vc1",
		Issuer:    "energy1issuer",
		Subject:   "energy1alice",
		Type:      "urn:vc:kyc:passed",
		Hash:      "abcd",
		Status:    types.CredentialValid,
		IssuedAt:  now - 100,
		ExpiresAt: now + 3600,
	}
	if err := k.SetCredential(ctx, c); err != nil {
		t.Fatal(err)
	}
	if !k.HasValidCredential(ctx, "energy1alice", "urn:vc:kyc:passed") {
		t.Errorf("HasValidCredential should be true")
	}

	// Move forward past expiry, run sweep — credential should be marked.
	future := ctx.WithBlockTime(time.Unix(now+7200, 0))
	if _, err := k.SweepExpiredCredentials(future); err != nil {
		t.Fatal(err)
	}
	gotC, _ := k.GetCredential(future, "urn:uuid:vc1")
	if gotC.Status != types.CredentialExpired {
		t.Errorf("status after sweep = %s, want %s", gotC.Status, types.CredentialExpired)
	}
	if k.HasValidCredential(future, "energy1alice", "urn:vc:kyc:passed") {
		t.Errorf("expired credential should not count")
	}
}

func TestRevokeCredential(t *testing.T) {
	k, ctx := setup(t)
	c := types.CredentialStatus{
		Id: "urn:uuid:vc2", Issuer: "i", Subject: "s",
		Type: "t", Hash: "h", Status: types.CredentialValid,
	}
	if err := k.SetCredential(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := k.MarkCredentialRevoked(ctx, c, "test"); err != nil {
		t.Fatal(err)
	}
	got, _ := k.GetCredential(ctx, "urn:uuid:vc2")
	if got.Status != types.CredentialRevoked {
		t.Errorf("status = %s, want revoked", got.Status)
	}
	if got.RevocationReason != "test" {
		t.Errorf("reason = %q", got.RevocationReason)
	}
}

func TestActiveAnchorScopes(t *testing.T) {
	k, ctx := setup(t)
	a := types.TrustAnchor{
		Did: "energy1anchor", Tier: types.AnchorTierRoot,
		CredentialTypes: []string{"urn:vc:kyc:passed"},
		Active:          true,
	}
	if err := k.SetAnchor(ctx, a); err != nil {
		t.Fatal(err)
	}
	if !k.IsActiveAnchor(ctx, "energy1anchor", "urn:vc:kyc:passed") {
		t.Errorf("anchor should be active for declared type")
	}
	if k.IsActiveAnchor(ctx, "energy1anchor", "urn:vc:other") {
		t.Errorf("anchor should not be active for unlisted type")
	}
}

// TestAnchorChainCascade verifies the security fix: deactivating a root
// anchor must cause all of its intermediates to lose issuance capability.
func TestAnchorChainCascade(t *testing.T) {
	k, ctx := setup(t)
	root := types.TrustAnchor{
		Did: "energy1root", Tier: types.AnchorTierRoot, Active: true,
	}
	inter := types.TrustAnchor{
		Did: "energy1inter", Tier: types.AnchorTierIntermediate,
		ParentDid: "energy1root", Active: true,
	}
	if err := k.SetAnchor(ctx, root); err != nil {
		t.Fatal(err)
	}
	if err := k.SetAnchor(ctx, inter); err != nil {
		t.Fatal(err)
	}

	if !k.IsActiveAnchor(ctx, "energy1inter", "any") {
		t.Fatalf("intermediate should be active when root is active")
	}

	// Disable the root.
	root.Active = false
	if err := k.SetAnchor(ctx, root); err != nil {
		t.Fatal(err)
	}
	if k.IsActiveAnchor(ctx, "energy1inter", "any") {
		t.Errorf("intermediate must be treated inactive once its root is deactivated")
	}
	if k.IsActiveAnchor(ctx, "energy1root", "any") {
		t.Errorf("deactivated root must not be active")
	}
}

// TestAnchorMissingParent verifies that an intermediate without a
// resolvable parent (e.g. data corruption) is conservatively rejected.
func TestAnchorMissingParent(t *testing.T) {
	k, ctx := setup(t)
	inter := types.TrustAnchor{
		Did: "energy1orphan", Tier: types.AnchorTierIntermediate,
		ParentDid: "energy1ghost", Active: true,
	}
	if err := k.SetAnchor(ctx, inter); err != nil {
		t.Fatal(err)
	}
	if k.IsActiveAnchor(ctx, "energy1orphan", "any") {
		t.Errorf("orphan intermediate must not be active")
	}
}
