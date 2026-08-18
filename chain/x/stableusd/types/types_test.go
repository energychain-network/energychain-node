package types

import (
	"math"
	"math/big"
	"testing"
)

func TestCoverageOKTable(t *testing.T) {
	cases := []struct {
		name             string
		outstanding, res uint64
		ratioBps         uint32
		want             bool
	}{
		{"zero outstanding", 0, 0, 10000, true},
		{"exact 100pct", 1000, 1000, 10000, true},
		{"over 100pct breach", 1001, 1000, 10000, false},
		{"150pct over-collateral", 1000, 1500, 10000, true},
		{"50pct ratio ok", 2000, 1000, 5000, true},
		{"50pct ratio breach", 2001, 1000, 5000, false},
		{"large no overflow", math.MaxUint64, math.MaxUint64, 10000, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CoverageOK(c.outstanding, c.res, c.ratioBps); got != c.want {
				t.Fatalf("CoverageOK(%d,%d,%d)=%v want %v", c.outstanding, c.res, c.ratioBps, got, c.want)
			}
		})
	}
}

// FuzzCoverageOK checks the 128-bit coverage comparison matches a big.Int
// reference across the full domain (no overflow, no false accept/reject).
func FuzzCoverageOK(f *testing.F) {
	f.Add(uint64(1000), uint64(1000), uint32(10000))
	f.Add(uint64(math.MaxUint64), uint64(1), uint32(10000))
	f.Fuzz(func(t *testing.T, outstanding, reserve uint64, ratioBps uint32) {
		got := CoverageOK(outstanding, reserve, ratioBps)
		lhs := new(big.Int).Mul(new(big.Int).SetUint64(outstanding), big.NewInt(int64(ratioBps)))
		rhs := new(big.Int).Mul(new(big.Int).SetUint64(reserve), big.NewInt(int64(BpsDenominator)))
		want := lhs.Cmp(rhs) <= 0
		if got != want {
			t.Fatalf("CoverageOK(%d,%d,%d)=%v want %v", outstanding, reserve, ratioBps, got, want)
		}
	})
}

func TestDefaultParamsValid(t *testing.T) {
	if err := DefaultParams().Validate(); err != nil {
		t.Fatalf("default params invalid: %v", err)
	}
}

func TestSafeAddSub(t *testing.T) {
	if _, err := SafeAdd(math.MaxUint64, 1); err == nil {
		t.Fatal("expected add overflow")
	}
	if _, err := SafeSub(1, 2); err == nil {
		t.Fatal("expected sub underflow")
	}
	if v, _ := SafeAdd(2, 3); v != 5 {
		t.Fatal("add wrong")
	}
}
