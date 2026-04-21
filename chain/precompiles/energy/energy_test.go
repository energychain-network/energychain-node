package energy

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// TestABILoaded asserts the embedded ABI parses and exposes ONLY the
// read-only surface. The precompile is intentionally read-only — see the
// package doc for the security rationale.
func TestABILoaded(t *testing.T) {
	if got := len(ABI.Methods); got != 1 {
		t.Fatalf("expected exactly 1 ABI method (read-only precompile), got %d", got)
	}
	if _, ok := ABI.Methods[GetEnergyDataMethod]; !ok {
		t.Fatalf("expected ABI method %q to be present", GetEnergyDataMethod)
	}
	for _, banned := range []string{"submitEnergyData", "batchSubmit", "updateParams"} {
		if _, ok := ABI.Methods[banned]; ok {
			t.Fatalf("method %q must not be exposed by the precompile", banned)
		}
	}
	if len(ABI.Events) != 0 {
		t.Fatalf("read-only precompile must not declare any events, got %d", len(ABI.Events))
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

	t.Run("short input charges nothing", func(t *testing.T) {
		if got := p.RequiredGas([]byte{0x01}); got != 0 {
			t.Fatalf("short input: want 0, got %d", got)
		}
	})

	t.Run("getEnergyData uses query gas", func(t *testing.T) {
		method := ABI.Methods[GetEnergyDataMethod]
		input := append([]byte{}, method.ID...)
		input = append(input, make([]byte, 32)...)
		if got := p.RequiredGas(input); got != GasGetEnergyData {
			t.Fatalf("getEnergyData: want %d, got %d", GasGetEnergyData, got)
		}
	})

	t.Run("unknown selector charges nothing", func(t *testing.T) {
		input := []byte{0xde, 0xad, 0xbe, 0xef}
		if got := p.RequiredGas(input); got != 0 {
			t.Fatalf("unknown selector: want 0, got %d", got)
		}
	})
}

// TestIsTransactionAlwaysFalse pins the read-only invariant: no method on
// this precompile may report itself as a state-modifying tx. cmn.SetupABI
// uses this signal to enforce STATICCALL semantics.
func TestIsTransactionAlwaysFalse(t *testing.T) {
	p := Precompile{ABI: ABI}
	for name, method := range ABI.Methods {
		method := method
		t.Run(name, func(t *testing.T) {
			if p.IsTransaction(&method) {
				t.Fatalf("method %q must not be flagged as a transaction (precompile is read-only)", name)
			}
		})
	}
	// And explicitly cover an unknown method too — defensive default.
	unknown := abi.Method{Name: "doesNotExist"}
	if p.IsTransaction(&unknown) {
		t.Fatalf("unknown method must not be flagged as a transaction")
	}
}
