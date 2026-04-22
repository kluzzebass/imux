package tui

import "testing"

func TestLogTimePrecisionCycle(t *testing.T) {
	p := logTimeOff
	order := []logTimePrecision{logTimeOff, logTimeSec, logTimeMSec, logTimeUSec}
	for i := 0; i < 20; i++ {
		want := order[i%len(order)]
		if p != want {
			t.Fatalf("step %d: got %v want %v", i, p, want)
		}
		p = p.next()
	}
	// prev is the inverse of next.
	p = logTimeSec
	if got := p.prev(); got != logTimeOff {
		t.Fatalf("sec.prev want off got %v", got)
	}
	p = logTimeOff
	if got := p.prev(); got != logTimeUSec {
		t.Fatalf("off.prev want usec got %v", got)
	}
	for i := 0; i < 30; i++ {
		q := order[i%len(order)]
		if q.next().prev() != q {
			t.Fatalf("next/prev not inverse at %v", q)
		}
	}
}

func TestLogTimePrecisionLayouts(t *testing.T) {
	cases := []struct {
		p   logTimePrecision
		exp string
	}{
		{logTimeOff, ""},
		{logTimeSec, "15:04:05"},
		{logTimeMSec, "15:04:05.000"},
		{logTimeUSec, "15:04:05.000000"},
	}
	for _, tc := range cases {
		if got := tc.p.goTimeLayout(); got != tc.exp {
			t.Errorf("%v goTimeLayout: got %q want %q", tc.p, got, tc.exp)
		}
	}
}
