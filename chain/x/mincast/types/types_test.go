package types

import "testing"

func TestMulDivFloorAndBps(t *testing.T) {
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
		{^uint64(0), ^uint64(0), 1, 0, true},
	}
	for i, c := range cases {
		got, err := MulDivFloor(c.a, c.b, c.d)
		if c.wantErr {
			if err == nil {
				t.Fatalf("case %d: want error", i)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Fatalf("case %d: got %d err %v want %d", i, got, err, c.want)
		}
	}
	if v, _ := MulBps(10_000, 250); v != 250 { // 2.5%
		t.Fatalf("MulBps 2.5%% of 10000 = %d want 250", v)
	}
}

func TestProrateYield(t *testing.T) {
	// 1000 principal at 20% APY for exactly one year = 200.
	y, err := ProrateYield(1000, 2000, DefaultYearSeconds, DefaultYearSeconds)
	if err != nil || y != 200 {
		t.Fatalf("annual yield = %d err %v want 200", y, err)
	}
	// Half a year halves it.
	y, _ = ProrateYield(1000, 2000, DefaultYearSeconds/2, DefaultYearSeconds)
	if y != 100 {
		t.Fatalf("half-year yield = %d want 100", y)
	}
	if _, err := ProrateYield(1000, 2000, 0, DefaultYearSeconds); err == nil {
		t.Fatal("zero term must error")
	}
}

func TestParamsValidate(t *testing.T) {
	if err := DefaultParams().Validate(); err != nil {
		t.Fatalf("default params invalid: %v", err)
	}
	bad := []Params{
		{MaxMarkets: 0, MaxFeeBps: 100, MaxApyBps: 100, MaxInvestTermSeconds: 1, MinInitialPrice: 1, YearSeconds: 1},
		{MaxMarkets: 1, MaxFeeBps: Bps + 1, MaxApyBps: 100, MaxInvestTermSeconds: 1, MinInitialPrice: 1, YearSeconds: 1},
		{MaxMarkets: 1, MaxFeeBps: 100, MaxApyBps: 0, MaxInvestTermSeconds: 1, MinInitialPrice: 1, YearSeconds: 1},
		{MaxMarkets: 1, MaxFeeBps: 100, MaxApyBps: 100, MaxInvestTermSeconds: 0, MinInitialPrice: 1, YearSeconds: 1},
		{MaxMarkets: 1, MaxFeeBps: 100, MaxApyBps: 100, MaxInvestTermSeconds: 1, MinInitialPrice: 0, YearSeconds: 1},
		{MaxMarkets: 1, MaxFeeBps: 100, MaxApyBps: 100, MaxInvestTermSeconds: 1, MinInitialPrice: 1, YearSeconds: 0},
	}
	for i, p := range bad {
		if err := p.Validate(); err == nil {
			t.Fatalf("bad params %d accepted", i)
		}
	}
}

// curveSim mirrors the keeper's exact state transitions over the pure
// helpers, so the monotonicity assertions here hold for the real module.
type curveSim struct {
	treasury, supply, floor uint64
	initPrice               uint64
	mintFee, meltFee        uint32
}

func (s *curveSim) checkFloor(t *testing.T, op string) {
	nf := FloorPrice(s.treasury, s.supply)
	if s.supply > 0 && nf < s.floor {
		t.Fatalf("%s regressed floor %d -> %d (T=%d S=%d)", op, s.floor, nf, s.treasury, s.supply)
	}
	s.floor = nf
}

// returns false if the op should be skipped (overflow / zero output).
func (s *curveSim) mint(t *testing.T, pay uint64) bool {
	fee, _ := MulBps(pay, s.mintFee)
	principal, err := SafeSub(pay, fee)
	if err != nil {
		return false
	}
	units, err := MintUnits(s.treasury, s.supply, s.initPrice, principal)
	if err != nil || units == 0 {
		return false
	}
	nt, err := SafeAdd(s.treasury, pay)
	if err != nil {
		return false
	}
	ns, err := SafeAdd(s.supply, units)
	if err != nil {
		return false
	}
	s.treasury, s.supply = nt, ns
	s.checkFloor(t, "mint")
	return true
}

func (s *curveSim) melt(t *testing.T, units uint64) bool {
	if units == 0 || units > s.supply {
		return false
	}
	gross, err := MeltGross(s.treasury, s.supply, units)
	if err != nil || gross == 0 {
		return false
	}
	fee, _ := MulBps(gross, s.meltFee)
	payout, err := SafeSub(gross, fee)
	if err != nil {
		return false
	}
	s.supply -= units
	nt, err := SafeSub(s.treasury, payout)
	if err != nil {
		return false
	}
	s.treasury = nt
	s.checkFloor(t, "melt")
	return true
}

func (s *curveSim) inject(t *testing.T, amt uint64) bool {
	nt, err := SafeAdd(s.treasury, amt)
	if err != nil {
		return false
	}
	s.treasury = nt
	s.checkFloor(t, "inject")
	return true
}

func TestFloorMonotonicScenario(t *testing.T) {
	s := &curveSim{initPrice: 100, mintFee: 100, meltFee: 100} // 1% fees
	s.mint(t, 100_000)
	s.mint(t, 250_000)
	s.melt(t, 300)
	s.inject(t, 50_000)
	s.melt(t, 100)
	s.mint(t, 1)
	s.mint(t, 7)
	// floor should have strictly risen from the fees + injection
	if s.floor == 0 {
		t.Fatal("floor should be positive")
	}
}

// FuzzFloorNeverDecreases drives a random sequence of mint/melt/inject and
// asserts the floor is monotonic non-decreasing across every operation —
// the core economic invariant of the Origin-Mincast curve.
func FuzzFloorNeverDecreases(f *testing.F) {
	f.Add(uint64(100), uint32(100), uint32(100), []byte{1, 2, 3, 4, 5, 6, 7, 8})
	f.Add(uint64(1), uint32(0), uint32(0), []byte{9, 9, 9, 9})
	f.Add(uint64(1_000_000), uint32(500), uint32(9000), []byte{1, 1, 2, 2, 3, 3, 4, 4, 5, 5})
	f.Fuzz(func(t *testing.T, initPrice uint64, mintFee, meltFee uint32, ops []byte) {
		if initPrice == 0 {
			initPrice = 1
		}
		if mintFee > Bps {
			mintFee = mintFee % (Bps + 1)
		}
		if meltFee > Bps {
			meltFee = meltFee % (Bps + 1)
		}
		s := &curveSim{initPrice: initPrice, mintFee: mintFee, meltFee: meltFee}
		// Each op byte: low 2 bits select action, rest scales the amount.
		for _, b := range ops {
			amt := uint64(b)*7919 + 1 // spread amounts, never zero
			switch b & 0x3 {
			case 0, 1:
				s.mint(t, amt)
			case 2:
				s.melt(t, amt%(s.supply+1))
			case 3:
				s.inject(t, amt)
			}
		}
	})
}
