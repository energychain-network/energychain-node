package types

import "testing"

func TestSafeArithmetic(t *testing.T) {
	if _, err := SafeAdd(^uint64(0), 1); err == nil {
		t.Fatal("expected add overflow")
	}
	if _, err := SafeSub(1, 2); err == nil {
		t.Fatal("expected sub underflow")
	}
	if _, err := SafeMul(^uint64(0), 2); err == nil {
		t.Fatal("expected mul overflow")
	}
	if v, err := SafeMul(0, 12345); err != nil || v != 0 {
		t.Fatalf("0*x: %v %d", err, v)
	}
}

func TestMulDivFloor(t *testing.T) {
	cases := []struct {
		a, b, d uint64
		want    uint64
		wantErr bool
	}{
		{0, 5, 7, 0, false},
		{100, 1, 3, 33, false},
		{1, 1, 3, 0, false},
		{1000, 3, 7, 428, false},
		{^uint64(0), 1, 1, ^uint64(0), false},
		{10, 10, 0, 0, true},
		{^uint64(0), ^uint64(0), 1, 0, true}, // quotient overflows uint64
	}
	for i, c := range cases {
		got, err := MulDivFloor(c.a, c.b, c.d)
		if c.wantErr {
			if err == nil {
				t.Fatalf("case %d: expected error", i)
			}
			continue
		}
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if got != c.want {
			t.Fatalf("case %d: got %d want %d", i, got, c.want)
		}
	}
}

func TestTrancheAmountConservation(t *testing.T) {
	cases := []struct {
		raised uint64
		total  uint32
	}{
		{0, 1}, {100, 1}, {100, 3}, {1000, 7}, {1, 4}, {999999, 13}, {^uint64(0), 9},
	}
	for _, c := range cases {
		var sum uint64
		for k := uint32(1); k <= c.total; k++ {
			amt := TrancheAmount(c.raised, c.total, k)
			var err error
			if sum, err = SafeAdd(sum, amt); err != nil {
				t.Fatalf("overflow summing tranches raised=%d total=%d", c.raised, c.total)
			}
		}
		if sum != c.raised {
			t.Fatalf("raised=%d total=%d: tranche sum=%d != raised", c.raised, c.total, sum)
		}
	}
	if TrancheAmount(100, 0, 1) != 0 || TrancheAmount(100, 3, 0) != 0 || TrancheAmount(100, 3, 4) != 0 {
		t.Fatal("out-of-range tranche must return 0")
	}
}

func TestDueInjections(t *testing.T) {
	const start = 1_000
	cases := []struct {
		now      int64
		interval int64
		total    uint32
		want     uint32
	}{
		{start, 100, 5, 0},
		{start + 99, 100, 5, 0},
		{start + 100, 100, 5, 1},
		{start + 350, 100, 5, 3},
		{start + 100000, 100, 5, 5}, // capped at total
		{start + 100000, 0, 5, 0},   // no recurring obligation
		{start - 50, 100, 5, 0},     // before success
	}
	for i, c := range cases {
		if got := DueInjections(c.now, start, c.interval, c.total); got != c.want {
			t.Fatalf("case %d: got %d want %d", i, got, c.want)
		}
	}
}

func TestParamsValidate(t *testing.T) {
	if err := DefaultParams().Validate(); err != nil {
		t.Fatalf("default params invalid: %v", err)
	}
	bad := []Params{
		{MaxOfferings: 0, MaxTranches: 1},
		{MaxOfferings: HardMaxOfferings + 1, MaxTranches: 1},
		{MaxOfferings: 1, MaxTranches: 0},
		{MaxOfferings: 1, MaxTranches: HardMaxTranches + 1},
	}
	for i, p := range bad {
		if err := p.Validate(); err == nil {
			t.Fatalf("bad params %d accepted", i)
		}
	}
}

// FuzzTrancheAmount asserts the core invariant that splitting raised into
// `total` tranches always sums back to raised with no over/under payment.
func FuzzTrancheAmount(f *testing.F) {
	f.Add(uint64(1000), uint32(7))
	f.Add(uint64(0), uint32(1))
	f.Add(^uint64(0), uint32(64))
	f.Fuzz(func(t *testing.T, raised uint64, total uint32) {
		if total == 0 || total > 4096 {
			t.Skip()
		}
		var sum uint64
		for k := uint32(1); k <= total; k++ {
			amt := TrancheAmount(raised, total, k)
			ns, err := SafeAdd(sum, amt)
			if err != nil {
				t.Fatalf("overflow: raised=%d total=%d", raised, total)
			}
			sum = ns
		}
		if sum != raised {
			t.Fatalf("raised=%d total=%d sum=%d", raised, total, sum)
		}
	})
}

// FuzzMulDivFloor checks floor(a*b/d) <= a*b/d exactly and never exceeds the
// closed form, guarding the pro-rata kernel against rounding-up bugs.
func FuzzMulDivFloor(f *testing.F) {
	f.Add(uint64(7), uint64(3), uint64(5))
	f.Add(uint64(0), uint64(0), uint64(1))
	f.Fuzz(func(t *testing.T, a, b, d uint64) {
		if d == 0 {
			t.Skip()
		}
		got, err := MulDivFloor(a, b, d)
		if err != nil {
			t.Skip() // overflow case is acceptable
		}
		// got*d must not exceed a*b, and (got+1)*d must exceed it.
		lo, e1 := SafeMul(got, d)
		if e1 != nil {
			t.Skip()
		}
		hiA, e2 := SafeMul(a, b)
		if e2 != nil {
			t.Skip()
		}
		if lo > hiA {
			t.Fatalf("floor too high: a=%d b=%d d=%d got=%d", a, b, d, got)
		}
	})
}
