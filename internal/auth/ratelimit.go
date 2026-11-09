package auth

import (
	"math"
	"sync"
	"time"
)

// RateLimiter is a token bucket per key: up to Burst requests at once,
// refilled at PerMinute requests per minute. It is in-memory, so with several
// instances each enforces its own limit.
type RateLimiter struct {
	perMinute int
	burst     float64

	mu        sync.Mutex
	buckets   map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	tokens float64
	at     time.Time // last refill
}

// NewRateLimiter allows perMinute requests per minute with bursts of burst.
// perMinute <= 0 returns nil, which allows everything.
func NewRateLimiter(perMinute, burst int) *RateLimiter {
	if perMinute <= 0 {
		return nil
	}
	if burst <= 0 {
		burst = perMinute
	}
	return &RateLimiter{perMinute: perMinute, burst: float64(burst), buckets: map[string]*bucket{}}
}

func (l *RateLimiter) rate() float64 { return float64(l.perMinute) / 60 } // tokens per second

// refill brings b up to now and returns it (creating a full bucket).
func (l *RateLimiter) refill(key string, now time.Time) *bucket {
	b := l.buckets[key]
	if b == nil {
		b = &bucket{tokens: l.burst, at: now}
		l.buckets[key] = b
		return b
	}
	if elapsed := now.Sub(b.at).Seconds(); elapsed > 0 {
		b.tokens = math.Min(l.burst, b.tokens+elapsed*l.rate())
		b.at = now
	}
	return b
}

// Allow takes one token for key. When none is left it returns false and how
// long until the next token. A nil limiter always allows.
func (l *RateLimiter) Allow(key string, now time.Time) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)
	b := l.refill(key, now)
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	// Whole seconds (Retry-After's unit), rounding away float noise first.
	secs := math.Ceil(math.Round((1-b.tokens)/l.rate()*1000) / 1000)
	return false, max(time.Duration(secs)*time.Second, time.Second)
}

// Refund returns a token taken by Allow, e.g. when a second limit then
// rejected the same request.
func (l *RateLimiter) Refund(key string, now time.Time) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.refill(key, now)
	b.tokens = math.Min(l.burst, b.tokens+1)
}

// sweep drops buckets that have refilled completely; they are equivalent to
// absent ones. It runs at most once a minute.
func (l *RateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	full := time.Duration(l.burst / l.rate() * float64(time.Second))
	for k, b := range l.buckets {
		if now.Sub(b.at) >= full {
			delete(l.buckets, k)
		}
	}
}
