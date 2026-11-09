package auth

import (
	"sync"
	"time"
)

// LoginLimiter throttles password guessing per key (username). After Max
// failures within Window the key is locked until the window passes.
type LoginLimiter struct {
	Max    int
	Window time.Duration
	Now    func() time.Time

	mu      sync.Mutex
	entries map[string]*limitEntry
}

type limitEntry struct {
	failures int
	first    time.Time
}

// NewLoginLimiter returns a limiter allowing max failures per window.
func NewLoginLimiter(maxFailures int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{Max: maxFailures, Window: window, Now: time.Now, entries: map[string]*limitEntry{}}
}

// Allowed reports whether another attempt for key may proceed, and if not,
// how long to wait.
func (l *LoginLimiter) Allowed(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[key]
	if e == nil {
		return true, 0
	}
	elapsed := l.Now().Sub(e.first)
	if elapsed >= l.Window {
		delete(l.entries, key)
		return true, 0
	}
	if e.failures >= l.Max {
		return false, l.Window - elapsed
	}
	return true, 0
}

// Failure records a failed attempt.
func (l *LoginLimiter) Failure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.Now()
	e := l.entries[key]
	if e == nil || now.Sub(e.first) >= l.Window {
		e = &limitEntry{first: now}
		l.entries[key] = e
	}
	e.failures++
	// Opportunistic cleanup keeps the map bounded.
	if len(l.entries) > 10_000 {
		for k, v := range l.entries {
			if now.Sub(v.first) >= l.Window {
				delete(l.entries, k)
			}
		}
	}
}

// Success clears failures for key.
func (l *LoginLimiter) Success(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}
