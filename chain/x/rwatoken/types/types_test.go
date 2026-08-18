package types

import (
	"math"
	"math/big"
	"testing"
)

func TestSafeAddSub(t *testing.T) {
	if _, err := SafeAdd(math.MaxUint64, 1); err == nil {
		t.Fatal("expected overflow")
	}
	if v, err := SafeAdd(10, 5); err != nil || v != 15 {
		t.Fatalf("got %d,%v", v, err)
	}
	if _, err := SafeSub(3, 5); err == nil {
		t.Fatal("expected underflow")
	}
	if v, err := SafeSub(10, 4); err != nil || v != 6 {
		t.Fatalf("got %d,%v", v, err)
	}
}

func TestSafeMul(t *testing.T) {
	cases := []struct {
		a, b   uint64
		want   uint64
		errish bool
	}{
		{0, 12345, 0, false},
		{12345, 0, 0, false},
		{1000, 1000, 1_000_000, false},
		{math.MaxUint64, 1, math.MaxUint64, false},
		{math.MaxUint64, 2, 0, true},
		{1 << 32, 1 << 32, 0, true},
	}
	for _, c := range cases {
		got, err := SafeMul(c.a, c.b)
		if c.errish {
			if err == nil {
				t.Fatalf("SafeMul(%d,%d) expected overflow", c.a, c.b)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Fatalf("SafeMul(%d,%d)=%d,%v want %d", c.a, c.b, got, err, c.want)
		}
	}
}

func TestMulDivFloor(t *testing.T) {
	cases := []struct {
		a, b, d uint64
		want    uint64
		errish  bool
	}{
		{100, 30, 100, 30, false},     // 30% pro-rata
		{1, 1, 3, 0, false},           // floors to 0
		{100, 1, 3, 33, false},        // 100/3 floors to 33
		{1000, 999, 1000, 999, false}, // near-full
		{0, 5, 7, 0, false},
		{5, 0, 7, 0, false},
		{100, 5, 0, 0, true}, // div by zero
		// huge product that overflows uint64 intermediate but quotient fits
		{math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64, false},
	}
	for _, c := range cases {
		got, err := MulDivFloor(c.a, c.b, c.d)
		if c.errish {
			if err == nil {
				t.Fatalf("MulDivFloor(%d,%d,%d) expected error", c.a, c.b, c.d)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Fatalf("MulDivFloor(%d,%d,%d)=%d,%v want %d", c.a, c.b, c.d, got, err, c.want)
		}
	}
}

func TestParamsValidate(t *testing.T) {
	p := DefaultParams()
	if err := p.Validate(); err != nil {
		t.Fatalf("default params invalid: %v", err)
	}
	p.MaxTokens = 0
	if err := p.Validate(); err == nil {
		t.Fatal("expected max_tokens=0 invalid")
	}
	p = DefaultParams()
	p.MaxPendingRedemptionsPerHolder = HardMaxPendingRedemptions + 1
	if err := p.Validate(); err == nil {
		t.Fatal("expected pending cap over hard max invalid")
	}
}

// FuzzMulDivFloor checks the floor-division kernel against a big.Int
// reference and asserts the defining inequality
//
//	q*d <= a*b < (q+1)*d
//
// holds for every (a, b, d), so dividend pro-rata can never over-distribute.
func FuzzMulDivFloor(f *testing.F) {
	f.Add(uint64(100), uint64(30), uint64(100))
	f.Add(uint64(math.MaxUint64), uint64(math.MaxUint64), uint64(math.MaxUint64))
	f.Add(uint64(1), uint64(1), uint64(1))
	f.Fuzz(func(t *testing.T, a, b, d uint64) {
		got, err := MulDivFloor(a, b, d)
		if d == 0 {
			if err == nil {
				t.Fatal("expected division-by-zero error")
			}
			return
		}
		ref := new(big.Int).Mul(new(big.Int).SetUint64(a), new(big.Int).SetUint64(b))
		ref.Quo(ref, new(big.Int).SetUint64(d))
		if !ref.IsUint64() {
			if err == nil {
				t.Fatalf("MulDivFloor(%d,%d,%d) should overflow uint64", a, b, d)
			}
			return
		}
		if err != nil {
			t.Fatalf("MulDivFloor(%d,%d,%d) unexpected err %v", a, b, d, err)
		}
		if got != ref.Uint64() {
			t.Fatalf("MulDivFloor(%d,%d,%d)=%d want %d", a, b, d, got, ref.Uint64())
		}
		// q*d <= a*b
		lo := new(big.Int).Mul(new(big.Int).SetUint64(got), new(big.Int).SetUint64(d))
		prod := new(big.Int).Mul(new(big.Int).SetUint64(a), new(big.Int).SetUint64(b))
		if lo.Cmp(prod) > 0 {
			t.Fatalf("q*d > a*b for (%d,%d,%d)", a, b, d)
		}
	})
}

// FuzzSafeMul asserts SafeMul matches a 128-bit reference: it errors iff
// the true product exceeds uint64, and otherwise returns the exact value.
func FuzzSafeMul(f *testing.F) {
	f.Add(uint64(1000), uint64(1000))
	f.Add(uint64(math.MaxUint64), uint64(2))
	f.Fuzz(func(t *testing.T, a, b uint64) {
		got, err := SafeMul(a, b)
		ref := new(big.Int).Mul(new(big.Int).SetUint64(a), new(big.Int).SetUint64(b))
		if !ref.IsUint64() {
			if err == nil {
				t.Fatalf("SafeMul(%d,%d) should overflow", a, b)
			}
			return
		}
		if err != nil || got != ref.Uint64() {
			t.Fatalf("SafeMul(%d,%d)=%d,%v want %d", a, b, got, err, ref.Uint64())
		}
	})
}
