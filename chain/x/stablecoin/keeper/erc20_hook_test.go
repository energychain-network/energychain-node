package keeper_test

import (
	"errors"
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/stablecoin/keeper"
	"energychain/x/stablecoin/types"
)

// stubERC20 is the test double for types.ERC20Keeper. The single
// stub backs the entire auto-register suite — each subtest mutates
// the flags it cares about.
type stubERC20 struct {
	// registered tracks the hooked denom strings (e.g. "scnusd")
	// the keeper has asked us to reserve. Insertion order matters
	// — TestAutoRegister_Idempotent relies on it to assert a
	// second RegisterDenom for the same id is a no-op.
	registered []string

	// returnErr is the error CreateNewTokenPair returns. Empty
	// means success. Used to verify the failure path is
	// non-fatal to MsgRegisterDenom.
	returnErr error
}

func (s *stubERC20) IsDenomRegistered(_ sdk.Context, denom string) bool {
	for _, d := range s.registered {
		if d == denom {
			return true
		}
	}
	return false
}

func (s *stubERC20) CreateNewTokenPair(_ sdk.Context, denom string) error {
	if s.returnErr != nil {
		return s.returnErr
	}
	s.registered = append(s.registered, denom)
	return nil
}

// TestAutoRegister_HappyPath verifies a fresh denom registration
// triggers exactly one CreateNewTokenPair call with the "scn"-
// prefixed hooked denom and emits the stablecoin_erc20_registered
// event.
func TestAutoRegister_HappyPath(t *testing.T) {
	f := setup(t)
	hook := &stubERC20{}
	f.k = f.k.WithERC20Keeper(hook)
	// MsgServer captures the keeper by value, so re-build it
	// against the hooked keeper.
	srv := newMsgServer(t, f)

	if _, err := srv.RegisterDenom(f.ctx, &types.MsgRegisterDenom{
		Authority: authority, Id: "usd", Symbol: "scnUSD", Name: "USD",
		Decimals: 6, Jurisdiction: "US",
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	if len(hook.registered) != 1 || hook.registered[0] != "scnusd" {
		t.Fatalf("expected hook to register exactly [scnusd], got %v", hook.registered)
	}
	if !hasEvent(f.ctx, "stablecoin_erc20_registered") {
		t.Fatal("expected stablecoin_erc20_registered event")
	}
	if hasEvent(f.ctx, "stablecoin_erc20_register_failed") {
		t.Fatal("did not expect failure event on happy path")
	}
}

// TestAutoRegister_Idempotent verifies a duplicate-denom call is
// short-circuited via IsDenomRegistered so the same TokenPair is
// not re-created on a hook re-fire (e.g. ante handler retries).
func TestAutoRegister_Idempotent(t *testing.T) {
	f := setup(t)
	hook := &stubERC20{registered: []string{"scnusd"}}
	f.k = f.k.WithERC20Keeper(hook)
	srv := newMsgServer(t, f)

	if _, err := srv.RegisterDenom(f.ctx, &types.MsgRegisterDenom{
		Authority: authority, Id: "usd", Symbol: "scnUSD", Name: "USD",
		Decimals: 6, Jurisdiction: "US",
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if len(hook.registered) != 1 {
		t.Fatalf("expected no new registration, got %v", hook.registered)
	}
}

// TestAutoRegister_HookFailureIsNonFatal verifies that an error
// returned by the ERC20 keeper does NOT roll back RegisterDenom:
// the denom is stored, an erc20_register_failed event is emitted
// so operators can replay registration, and the message succeeds.
func TestAutoRegister_HookFailureIsNonFatal(t *testing.T) {
	f := setup(t)
	hook := &stubERC20{returnErr: errors.New("boom")}
	f.k = f.k.WithERC20Keeper(hook)
	srv := newMsgServer(t, f)

	if _, err := srv.RegisterDenom(f.ctx, &types.MsgRegisterDenom{
		Authority: authority, Id: "usd", Symbol: "scnUSD", Name: "USD",
		Decimals: 6, Jurisdiction: "US",
	}); err != nil {
		t.Fatalf("register must succeed even when ERC20 hook fails: %v", err)
	}
	if _, ok := f.k.GetDenom(f.ctx, "usd"); !ok {
		t.Fatal("denom must be persisted even when ERC20 hook fails")
	}
	if !hasEvent(f.ctx, "stablecoin_erc20_register_failed") {
		t.Fatal("expected stablecoin_erc20_register_failed event")
	}
}

// TestAutoRegister_NilHookSkipsAll confirms the keeper behaves
// exactly as before when no ERC20 hook is wired (the production
// default for chains that compile without x/erc20).
func TestAutoRegister_NilHookSkipsAll(t *testing.T) {
	f := setup(t)
	// Leave f.k.erc20 nil (no WithERC20Keeper call).
	if _, err := f.srv.RegisterDenom(f.ctx, &types.MsgRegisterDenom{
		Authority: authority, Id: "usd", Symbol: "scnUSD", Name: "USD",
		Decimals: 6, Jurisdiction: "US",
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if hasEvent(f.ctx, "stablecoin_erc20_registered") ||
		hasEvent(f.ctx, "stablecoin_erc20_register_failed") {
		t.Fatal("no ERC20 events should fire when hook is nil")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newMsgServer rebuilds the MsgServer against the fixture's current
// keeper. Necessary after WithERC20Keeper because the keeper value
// is copied into the MsgServer at construction time.
func newMsgServer(t *testing.T, f *fixture) types.MsgServer {
	t.Helper()
	return keeper.NewMsgServerImpl(f.k)
}

// hasEvent reports whether any event of the given type is in the
// SDK event manager. Compares Type to a substring so callers don't
// need to know the full attribute layout.
func hasEvent(ctx sdk.Context, typ string) bool {
	for _, e := range ctx.EventManager().Events() {
		if strings.Contains(e.Type, typ) {
			return true
		}
	}
	return false
}
