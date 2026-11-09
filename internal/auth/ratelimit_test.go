package auth

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	t0 := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	l := NewRateLimiter(60, 3) // 1 per second, bursts of 3

	for i := range 3 {
		if ok, _ := l.Allow("k", t0); !ok {
			t.Fatalf("burst request %d rejected", i+1)
		}
	}
	ok, wait := l.Allow("k", t0)
	if ok || wait != time.Second {
		t.Fatalf("4th request: ok=%v wait=%v", ok, wait)
	}
	// Other keys are independent.
	if ok, _ := l.Allow("other", t0); !ok {
		t.Fatal("other key throttled")
	}
	// One token comes back per second, and never more than the burst.
	if ok, _ := l.Allow("k", t0.Add(time.Second)); !ok {
		t.Fatal("refilled token not available")
	}
	if ok, _ := l.Allow("k", t0.Add(time.Second)); ok {
		t.Fatal("more than the refill was allowed")
	}
	later := t0.Add(time.Hour)
	for i := range 3 {
		if ok, _ := l.Allow("k", later); !ok {
			t.Fatalf("after idle, request %d rejected", i+1)
		}
	}
	if ok, _ := l.Allow("k", later); ok {
		t.Fatal("bucket refilled beyond burst")
	}
	// Refund gives the token back.
	l.Refund("k", later)
	if ok, _ := l.Allow("k", later); !ok {
		t.Fatal("refunded token not available")
	}
}

func TestRateLimiterWaitAndSweep(t *testing.T) {
	t0 := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	l := NewRateLimiter(6, 1) // one every 10 seconds
	l.Allow("k", t0)
	if ok, wait := l.Allow("k", t0.Add(4*time.Second)); ok || wait != 6*time.Second {
		t.Fatalf("wait: ok=%v %v", ok, wait)
	}
	// Idle full buckets are dropped by the periodic sweep.
	l.Allow("k", t0.Add(time.Hour))
	l.Allow("fresh", t0.Add(2*time.Hour))
	if _, kept := l.buckets["k"]; kept {
		t.Fatal("idle bucket not swept")
	}
}

func TestRateLimiterDisabled(t *testing.T) {
	l := NewRateLimiter(0, 0)
	if l != nil {
		t.Fatal("zero rate must disable the limiter")
	}
	for range 1000 {
		if ok, _ := l.Allow("k", time.Now()); !ok {
			t.Fatal("disabled limiter rejected")
		}
	}
	l.Refund("k", time.Now())
}
