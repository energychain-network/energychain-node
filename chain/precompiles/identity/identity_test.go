package identity

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestABILoaded(t *testing.T) {
	for _, name := range []string{
		GetIdentityMethod,
		GetIdentityByEvmAddressMethod,
		HasRoleMethod,
		IsRegisteredMethod,
	} {
		if _, ok := ABI.Methods[name]; !ok {
			t.Fatalf("expected ABI method %q to be present", name)
		}
	}
}

func TestPrecompileAddressIsValid(t *testing.T) {
	addr := common.HexToAddress(PrecompileAddress)
	if addr == (common.Address{}) {
		t.Fatal("precompile address must be non-zero")
	}
	if !strings.EqualFold(addr.Hex(), PrecompileAddress) {
		t.Fatalf("address normalisation drift: got %s want %s", addr.Hex(), PrecompileAddress)
	}
}

func TestRequiredGas(t *testing.T) {
	p := Precompile{ABI: ABI}

	cases := []struct {
		name   string
		method string
		want   uint64
	}{
		{"getIdentity", GetIdentityMethod, GasGetIdentity},
		{"getIdentityByEvmAddress", GetIdentityByEvmAddressMethod, GasGetIdentity},
		{"hasRole", HasRoleMethod, GasHasRole},
		{"isRegistered", IsRegisteredMethod, GasIsRegistered},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			method := ABI.Methods[tc.method]
			input := append([]byte{}, method.ID...)
			input = append(input, make([]byte, 32)...)
			if got := p.RequiredGas(input); got != tc.want {
				t.Fatalf("%s: want %d, got %d", tc.method, tc.want, got)
			}
		})
	}
}

func TestIsTransactionAlwaysFalse(t *testing.T) {
	p := Precompile{ABI: ABI}
	for name, method := range ABI.Methods {
		method := method
		if p.IsTransaction(&method) {
			t.Fatalf("identity precompile method %q must not be a transaction", name)
		}
	}
}
