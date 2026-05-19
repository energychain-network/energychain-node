package keeper_test

import (
	"errors"
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/carbon/keeper"
	"energychain/x/carbon/types"
)

type stubERC20 struct {
	registered []string
	returnErr  error
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

func TestAutoRegister_Carbon_HappyPath(t *testing.T) {
	f := setup(t)
	hook := &stubERC20{}
	f.k = f.k.WithERC20Keeper(hook)
	srv := keeper.NewMsgServerImpl(f.k)

	if _, err := srv.RegisterIssuer(f.ctx, &types.MsgRegisterIssuer{
		Authority:   authority,
		Id:          "verra",
		Did:         "did:web:verra.org:1",
		DisplayName: "Verra",
		Categories: []int32{
			int32(types.AssetCategory_ASSET_CATEGORY_OFFSET),
		},
		IssuerAuthority: issuerAuth,
		Admin:           issuerAdmin,
	}); err != nil {
		t.Fatalf("register issuer: %v", err)
	}

	if len(hook.registered) != 1 || hook.registered[0] != "carbonverra" {
		t.Fatalf("expected hook to register exactly [carbonverra], got %v", hook.registered)
	}
	if !hasEvent(f.ctx, "carbon_erc20_registered") {
		t.Fatal("expected carbon_erc20_registered event")
	}
}

func TestAutoRegister_Carbon_HookFailureIsNonFatal(t *testing.T) {
	f := setup(t)
	hook := &stubERC20{returnErr: errors.New("evm down")}
	f.k = f.k.WithERC20Keeper(hook)
	srv := keeper.NewMsgServerImpl(f.k)

	if _, err := srv.RegisterIssuer(f.ctx, &types.MsgRegisterIssuer{
		Authority:   authority,
		Id:          "verra",
		Did:         "did:web:verra.org:1",
		DisplayName: "Verra",
		Categories: []int32{
			int32(types.AssetCategory_ASSET_CATEGORY_OFFSET),
		},
		IssuerAuthority: issuerAuth,
		Admin:           issuerAdmin,
	}); err != nil {
		t.Fatalf("register must succeed even when ERC20 hook fails: %v", err)
	}
	if has, err := f.k.Issuers.Has(f.ctx, "verra"); err != nil || !has {
		t.Fatal("issuer must be persisted even when ERC20 hook fails")
	}
	if !hasEvent(f.ctx, "carbon_erc20_register_failed") {
		t.Fatal("expected carbon_erc20_register_failed event")
	}
}

func hasEvent(ctx sdk.Context, typ string) bool {
	for _, e := range ctx.EventManager().Events() {
		if strings.Contains(e.Type, typ) {
			return true
		}
	}
	return false
}
