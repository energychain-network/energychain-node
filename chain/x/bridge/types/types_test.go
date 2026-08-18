package types

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func addr(seed string) string {
	b := make([]byte, 20)
	copy(b, seed)
	return sdk.AccAddress(b).String()
}

func TestValidateAttestorSet(t *testing.T) {
	a, b, c := addr("a"), addr("b"), addr("c")

	if err := ValidateAttestorSet([]string{a, b, c}, 2, 64); err != nil {
		t.Fatalf("valid set rejected: %v", err)
	}
	// empty set
	if err := ValidateAttestorSet(nil, 1, 64); err == nil {
		t.Fatal("empty set must fail")
	}
	// duplicate attestor
	if err := ValidateAttestorSet([]string{a, a}, 1, 64); err == nil {
		t.Fatal("duplicate attestor must fail")
	}
	// threshold 0
	if err := ValidateAttestorSet([]string{a, b}, 0, 64); err == nil {
		t.Fatal("threshold 0 must fail")
	}
	// threshold > len
	if err := ValidateAttestorSet([]string{a, b}, 3, 64); err == nil {
		t.Fatal("threshold > len must fail")
	}
	// exceeds max
	if err := ValidateAttestorSet([]string{a, b, c}, 1, 2); err == nil {
		t.Fatal("set exceeding max must fail")
	}
	// invalid bech32
	if err := ValidateAttestorSet([]string{"not-an-address"}, 1, 64); err == nil {
		t.Fatal("invalid bech32 must fail")
	}
}

func TestParamsValidate(t *testing.T) {
	if err := DefaultParams().Validate(); err != nil {
		t.Fatalf("default invalid: %v", err)
	}
	bad := []Params{
		{},
		{MaxChains: 1, MaxAssets: 0, MaxAttestorsPerChain: 1},
		{MaxChains: 1, MaxAssets: 1, MaxAttestorsPerChain: 0},
		{MaxChains: HardMaxChains + 1, MaxAssets: 1, MaxAttestorsPerChain: 1},
	}
	for i, p := range bad {
		if err := p.Validate(); err == nil {
			t.Fatalf("bad params %d accepted", i)
		}
	}
}

func TestSafeArith(t *testing.T) {
	if _, err := SafeAdd(^uint64(0), 1); err == nil {
		t.Fatal("add overflow not caught")
	}
	if _, err := SafeSub(1, 2); err == nil {
		t.Fatal("sub underflow not caught")
	}
	if v, _ := SafeAdd(5, 7); v != 12 {
		t.Fatalf("add=%d want 12", v)
	}
	if v, _ := SafeSub(7, 5); v != 2 {
		t.Fatalf("sub=%d want 2", v)
	}
}

func TestContains(t *testing.T) {
	set := []string{"a", "b", "c"}
	if !Contains(set, "b") {
		t.Fatal("should contain b")
	}
	if Contains(set, "z") {
		t.Fatal("should not contain z")
	}
}
