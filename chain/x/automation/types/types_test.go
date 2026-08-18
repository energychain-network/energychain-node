package types

import "testing"

func TestCeilDiv(t *testing.T) {
	cases := []struct {
		a, b, want uint64
		err        bool
	}{
		{0, 5, 0, false},
		{10, 5, 2, false},
		{11, 5, 3, false},
		{1, 1, 1, false},
		{99, 100, 1, false},
		{100, 100, 1, false},
		{101, 100, 2, false},
		{5, 0, 0, true},
	}
	for i, c := range cases {
		got, err := CeilDiv(c.a, c.b)
		if c.err {
			if err == nil {
				t.Fatalf("case %d: want error", i)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Fatalf("case %d: got %d err %v want %d", i, got, err, c.want)
		}
	}
}

func TestStopTime(t *testing.T) {
	// deposit 100, rate 10 => 10s
	s, err := StopTime(1000, 100, 10)
	if err != nil || s != 1010 {
		t.Fatalf("stop=%d err %v want 1010", s, err)
	}
	// deposit 105, rate 10 => ceil 11s
	s, _ = StopTime(0, 105, 10)
	if s != 11 {
		t.Fatalf("stop=%d want 11", s)
	}
}

func TestVestedAmount(t *testing.T) {
	const dep, rate, start = uint64(100), uint64(10), int64(1000)
	stop, _ := StopTime(start, dep, rate) // 1010
	cases := []struct {
		now  int64
		want uint64
	}{
		{500, 0},    // before start
		{1000, 0},   // at start
		{1001, 10},  // 1s
		{1005, 50},  // 5s
		{1010, 100}, // at stop
		{2000, 100}, // past stop, capped
	}
	for _, c := range cases {
		if got := VestedAmount(dep, rate, start, stop, c.now); got != c.want {
			t.Fatalf("vested(now=%d)=%d want %d", c.now, got, c.want)
		}
	}
}

func TestAdvanceNextRun(t *testing.T) {
	// next=100, interval=10, now=100 -> 110 (one slot forward)
	if got := AdvanceNextRun(100, 10, 100); got != 110 {
		t.Fatalf("got %d want 110", got)
	}
	// missed many slots: next=100, interval=10, now=145 -> 150
	if got := AdvanceNextRun(100, 10, 145); got != 150 {
		t.Fatalf("got %d want 150", got)
	}
	// future next stays
	if got := AdvanceNextRun(200, 10, 100); got != 200 {
		t.Fatalf("got %d want 200", got)
	}
}

func TestParamsValidate(t *testing.T) {
	if err := DefaultParams().Validate(); err != nil {
		t.Fatalf("default invalid: %v", err)
	}
	bad := []Params{
		{},
		{MaxSchedules: 1, MaxStreams: 1, MaxRunsPerBlock: 0, MaxFinalizePerBlock: 1, MaxClosePerRun: 1, MinIntervalSeconds: 1},
		{MaxSchedules: 1, MaxStreams: 1, MaxRunsPerBlock: 1, MaxFinalizePerBlock: 1, MaxClosePerRun: 1, MinIntervalSeconds: 0},
	}
	for i, p := range bad {
		if err := p.Validate(); err == nil {
			t.Fatalf("bad params %d accepted", i)
		}
	}
}

// FuzzVestedBounds asserts the core stream invariants: vesting stays within
// [0, deposit], is monotonic non-decreasing in time, and equals the full
// deposit at/after the stop time. These guarantee a receiver can never vest
// more than the escrowed deposit (value conservation) and never un-vest.
func FuzzVestedBounds(f *testing.F) {
	f.Add(uint64(100), uint64(10), int64(1000), int64(1005))
	f.Add(uint64(1), uint64(1), int64(0), int64(0))
	f.Add(uint64(1_000_000_000), uint64(7), int64(5), int64(999999))
	f.Fuzz(func(t *testing.T, deposit, rate uint64, start, now int64) {
		if deposit == 0 {
			deposit = 1
		}
		if rate == 0 {
			rate = 1
		}
		if start < 0 {
			start = -start
		}
		stop, err := StopTime(start, deposit, rate)
		if err != nil {
			t.Skip()
		}
		v := VestedAmount(deposit, rate, start, stop, now)
		if v > deposit {
			t.Fatalf("vested %d > deposit %d", v, deposit)
		}
		if now <= start && v != 0 {
			t.Fatalf("vested %d before start should be 0", v)
		}
		if now >= stop && v != deposit {
			t.Fatalf("vested %d at/after stop should equal deposit %d", v, deposit)
		}
		// monotonic: one second later never decreases vesting
		if now < 1<<62 {
			v2 := VestedAmount(deposit, rate, start, stop, now+1)
			if v2 < v {
				t.Fatalf("vesting decreased: %d -> %d", v, v2)
			}
		}
	})
}
