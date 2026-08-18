package types

import (
	"math"
	"math/big"
	"testing"
)

func TestCrossVerifiedTable(t *testing.T) {
	cases := []struct {
		name    string
		iot, op uint64
		bps     uint32
		want    bool
	}{
		{"both zero", 0, 0, 200, true},
		{"exact match", 1000, 1000, 0, true},
		{"within 2pct", 1000, 1015, 200, true},
		{"at boundary", 1000, 1020, 200, true},
		{"just over boundary", 1000, 1021, 200, false},
		{"op larger within", 1015, 1000, 200, true},
		{"zero tolerance mismatch", 1000, 1001, 0, false},
		{"one side zero", 0, 1, 5000, false},
		{"large within", 1 << 40, (1 << 40) + (1 << 30), 200, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CrossVerified(c.iot, c.op, c.bps); got != c.want {
				t.Fatalf("CrossVerified(%d,%d,%d)=%v want %v", c.iot, c.op, c.bps, got, c.want)
			}
		})
	}
}

func TestCrossVerifiedSymmetric(t *testing.T) {
	// the result must not depend on argument order
	for _, p := range [][3]uint64{
		{1000, 1100, 200}, {0, 5, 100}, {math.MaxUint64, math.MaxUint64 - 7, 1},
	} {
		a := CrossVerified(p[0], p[1], uint32(p[2]))
		b := CrossVerified(p[1], p[0], uint32(p[2]))
		if a != b {
			t.Fatalf("asymmetric for %v: %v != %v", p, a, b)
		}
	}
}

func TestMulBpsAgainstBigInt(t *testing.T) {
	cases := []struct {
		v   uint64
		bps uint32
	}{
		{0, 500}, {1, 10000}, {1000, 500}, {math.MaxUint64, 500},
		{math.MaxUint64, 10000}, {math.MaxUint64, 1}, {123456789, 333},
	}
	for _, c := range cases {
		want := new(big.Int).Mul(new(big.Int).SetUint64(c.v), big.NewInt(int64(c.bps)))
		want.Div(want, big.NewInt(int64(BpsDenominator)))
		got := MulBps(c.v, c.bps)
		if want.Cmp(new(big.Int).SetUint64(got)) != 0 {
			t.Fatalf("MulBps(%d,%d)=%d want %s", c.v, c.bps, got, want)
		}
		// a slash can never exceed the bonded amount for bps<=10000
		if c.bps <= BpsDenominator && got > c.v {
			t.Fatalf("MulBps(%d,%d)=%d exceeds value", c.v, c.bps, got)
		}
	}
}

// FuzzMulBps checks MulBps matches a big.Int reference and never exceeds the
// input value for valid bps, across the full uint64/bps domain.
func FuzzMulBps(f *testing.F) {
	f.Add(uint64(1000), uint32(500))
	f.Add(uint64(math.MaxUint64), uint32(10000))
	f.Fuzz(func(t *testing.T, v uint64, bps uint32) {
		if bps > BpsDenominator {
			t.Skip()
		}
		want := new(big.Int).Mul(new(big.Int).SetUint64(v), big.NewInt(int64(bps)))
		want.Div(want, big.NewInt(int64(BpsDenominator)))
		got := MulBps(v, bps)
		if want.Cmp(new(big.Int).SetUint64(got)) != 0 {
			t.Fatalf("MulBps(%d,%d)=%d want %s", v, bps, got, want)
		}
		if got > v {
			t.Fatalf("MulBps(%d,%d)=%d exceeds value", v, bps, got)
		}
	})
}

// FuzzCrossVerified checks the cross-verification gate matches an
// independent big.Int relative-difference computation and is symmetric.
func FuzzCrossVerified(f *testing.F) {
	f.Add(uint64(1000), uint64(1015), uint32(200))
	f.Fuzz(func(t *testing.T, iot, op uint64, bps uint32) {
		if bps > BpsDenominator {
			t.Skip()
		}
		got := CrossVerified(iot, op, bps)
		if got != CrossVerified(op, iot, bps) {
			t.Fatalf("asymmetric: iot=%d op=%d bps=%d", iot, op, bps)
		}
		hi, lo := iot, op
		if op > iot {
			hi, lo = op, iot
		}
		var want bool
		if hi == 0 {
			want = true
		} else {
			diff := new(big.Int).SetUint64(hi - lo)
			left := new(big.Int).Mul(diff, big.NewInt(int64(BpsDenominator)))
			right := new(big.Int).Mul(new(big.Int).SetUint64(hi), big.NewInt(int64(bps)))
			want = left.Cmp(right) <= 0
		}
		if got != want {
			t.Fatalf("CrossVerified(%d,%d,%d)=%v want %v", iot, op, bps, got, want)
		}
	})
}

func TestMedianViaParamsSanity(t *testing.T) {
	// DefaultParams must validate.
	if err := DefaultParams().Validate(); err != nil {
		t.Fatalf("default params invalid: %v", err)
	}
}
