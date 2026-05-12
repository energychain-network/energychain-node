package granularid

import (
	"testing"
	"time"
)

func TestFromUnix(t *testing.T) {
	if got := FromUnix(0); got != 0 {
		t.Errorf("FromUnix(0) = %d, want 0", got)
	}
	if got := FromUnix(3599); got != 0 {
		t.Errorf("FromUnix(3599) = %d, want 0", got)
	}
	if got := FromUnix(3600); got != 1 {
		t.Errorf("FromUnix(3600) = %d, want 1", got)
	}
}

func TestRangeContains(t *testing.T) {
	r := Range{Start: 10, End: 14} // 4 hours
	if r.Hours() != 4 {
		t.Errorf("Hours = %d, want 4", r.Hours())
	}
	if !r.Contains(10) || !r.Contains(13) {
		t.Errorf("Contains: missing endpoint")
	}
	if r.Contains(14) {
		t.Errorf("Contains: half-open violated")
	}
}

func TestStartEnd(t *testing.T) {
	b := FromTime(time.Date(2026, 4, 19, 15, 30, 0, 0, time.UTC))
	if b.StartUnix() > time.Date(2026, 4, 19, 15, 30, 0, 0, time.UTC).Unix() {
		t.Errorf("Start should be ≤ original time")
	}
	if b.EndUnix() <= time.Date(2026, 4, 19, 15, 30, 0, 0, time.UTC).Unix() {
		t.Errorf("End should be > original time")
	}
}
