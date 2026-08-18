package types

import "testing"

func TestMulDivFloorCeil(t *testing.T) {
	cases := []struct {
		a, b, d      uint64
		wantF, wantC uint64
	}{
		{10, 3, 4, 7, 8},                // 30/4 = 7.5
		{10, 2, 5, 4, 4},                // exact
		{0, 99, 7, 0, 0},                // zero
		{1_000_000, 7, 1_000_000, 7, 7}, // PriceScale exact
	}
	for i, c := range cases {
		f, err := MulDivFloor(c.a, c.b, c.d)
		if err != nil {
			t.Fatalf("case %d floor err: %v", i, err)
		}
		ce, err := MulDivCeil(c.a, c.b, c.d)
		if err != nil {
			t.Fatalf("case %d ceil err: %v", i, err)
		}
		if f != c.wantF || ce != c.wantC {
			t.Fatalf("case %d: floor=%d ceil=%d want %d/%d", i, f, ce, c.wantF, c.wantC)
		}
	}
	if _, err := MulDivFloor(1, 1, 0); err == nil {
		t.Fatal("division by zero must error")
	}
	// no uint64 overflow at large products
	big := ^uint64(0)
	if _, err := MulDivFloor(big, big, 1); err == nil {
		t.Fatal("overflow result should error (not fit uint64)")
	}
	if v, err := MulDivFloor(big, 1, big); err != nil || v != 1 {
		t.Fatalf("big/big should be 1: v=%d err=%v", v, err)
	}
}

func TestQuoteCeilCoversFloor(t *testing.T) {
	for _, p := range []uint64{1, 250_000, 1_000_000, 3_333_333} {
		for _, q := range []uint64{1, 7, 1000, 999_999} {
			f, _ := QuoteFloor(p, q)
			c, _ := QuoteCeil(p, q)
			if c < f {
				t.Fatalf("ceil<floor for p=%d q=%d", p, q)
			}
			if c > f+1 {
				t.Fatalf("ceil more than floor+1 for p=%d q=%d", p, q)
			}
		}
	}
}

func TestComputeClearingBasic(t *testing.T) {
	// buy 10@5, sell 10@3 -> cross, volume 10. Clearing price is one of the
	// order prices that maximises volume.
	buys := []PriceLevel{{Price: 5, Qty: 10}}
	sells := []PriceLevel{{Price: 3, Qty: 10}}
	p, v, ok := ComputeClearing(buys, sells)
	if !ok || v != 10 {
		t.Fatalf("expected volume 10, got v=%d ok=%v", v, ok)
	}
	if p != 3 && p != 5 {
		t.Fatalf("clearing price %d not a book price", p)
	}
}

func TestComputeClearingNoCross(t *testing.T) {
	buys := []PriceLevel{{Price: 3, Qty: 10}}
	sells := []PriceLevel{{Price: 5, Qty: 10}}
	if _, _, ok := ComputeClearing(buys, sells); ok {
		t.Fatal("non-crossing book must not clear")
	}
	if _, _, ok := ComputeClearing(nil, sells); ok {
		t.Fatal("empty buys must not clear")
	}
}

func TestComputeClearingMaxVolume(t *testing.T) {
	// Two buys (8@4, 6@6) two sells (5@10, 3@4). Demand>=p, supply<=p.
	buys := []PriceLevel{{Price: 8, Qty: 4}, {Price: 6, Qty: 6}}
	sells := []PriceLevel{{Price: 5, Qty: 10}, {Price: 3, Qty: 4}}
	p, v, ok := ComputeClearing(buys, sells)
	if !ok {
		t.Fatal("should cross")
	}
	// verify feasibility at returned price
	var demand, supply uint64
	for _, b := range buys {
		if b.Price >= p {
			demand += b.Qty
		}
	}
	for _, s := range sells {
		if s.Price <= p {
			supply += s.Qty
		}
	}
	if v > demand || v > supply {
		t.Fatalf("volume %d exceeds feasibility demand=%d supply=%d at p=%d", v, demand, supply, p)
	}
}

func TestParamsValidate(t *testing.T) {
	if err := DefaultParams().Validate(); err != nil {
		t.Fatalf("default invalid: %v", err)
	}
	bad := []Params{
		{},
		{MaxMarkets: 1, MaxOpenOrdersPerMarket: 0, MaxFillsPerBatch: 1, MinBatchInterval: 1},
		{MaxMarkets: 1, MaxOpenOrdersPerMarket: 1, MaxFillsPerBatch: 0, MinBatchInterval: 1},
		{MaxMarkets: 1, MaxOpenOrdersPerMarket: 1, MaxFillsPerBatch: 1, MaxFeeBps: HardMaxFeeBps + 1, MinBatchInterval: 1},
		{MaxMarkets: 1, MaxOpenOrdersPerMarket: 1, MaxFillsPerBatch: 1, MinBatchInterval: 0},
	}
	for i, p := range bad {
		if err := p.Validate(); err == nil {
			t.Fatalf("bad params %d accepted", i)
		}
	}
}

// FuzzClearingFeasible checks ComputeClearing never returns an infeasible
// (price, volume): at the returned price both demand and supply must cover the
// volume, and a returned ok implies a real cross.
func FuzzClearingFeasible(f *testing.F) {
	f.Add(uint64(5), uint64(10), uint64(3), uint64(10), uint64(7), uint64(4))
	f.Fuzz(func(t *testing.T, bp1, bq1, sp1, sq1, bp2, sq2 uint64) {
		clamp := func(x uint64) uint64 { return x%1_000_000 + 1 }
		buys := []PriceLevel{{Price: clamp(bp1), Qty: clamp(bq1)}, {Price: clamp(bp2), Qty: clamp(bq1) + 1}}
		sells := []PriceLevel{{Price: clamp(sp1), Qty: clamp(sq1)}, {Price: clamp(sp1) + 1, Qty: clamp(sq2)}}
		p, v, ok := ComputeClearing(buys, sells)
		if !ok {
			return
		}
		if v == 0 {
			t.Fatal("ok but zero volume")
		}
		var demand, supply uint64
		for _, b := range buys {
			if b.Price >= p {
				demand += b.Qty
			}
		}
		for _, s := range sells {
			if s.Price <= p {
				supply += s.Qty
			}
		}
		if v > demand || v > supply {
			t.Fatalf("infeasible: p=%d v=%d demand=%d supply=%d", p, v, demand, supply)
		}
	})
}
