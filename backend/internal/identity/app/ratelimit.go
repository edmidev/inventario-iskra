package app

import (
	"sync"
	"time"
)

// MemoryLoginLimiter blocks a key after maxFails failures inside window.
// State is per-process; it is a basic brute-force brake, not a global quota.
type MemoryLoginLimiter struct {
	mu       sync.Mutex
	now      func() time.Time
	maxFails int
	window   time.Duration
	entries  map[string]*limitEntry
}

type limitEntry struct {
	fails int
	until time.Time // end of the current window
}

// NewMemoryLoginLimiter builds a limiter. now may be nil (uses time.Now).
func NewMemoryLoginLimiter(maxFails int, window time.Duration, now func() time.Time) *MemoryLoginLimiter {
	if now == nil {
		now = time.Now
	}
	return &MemoryLoginLimiter{now: now, maxFails: maxFails, window: window, entries: map[string]*limitEntry{}}
}

// Allow implements LoginLimiter.
func (l *MemoryLoginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return true
	}
	if !l.now().Before(e.until) {
		delete(l.entries, key)
		return true
	}
	return e.fails < l.maxFails
}

// Fail implements LoginLimiter.
func (l *MemoryLoginLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.prune(now)
	e, ok := l.entries[key]
	if !ok || !now.Before(e.until) {
		e = &limitEntry{until: now.Add(l.window)}
		l.entries[key] = e
	}
	e.fails++
}

// Reset implements LoginLimiter.
func (l *MemoryLoginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// prune drops expired entries so memory stays bounded. Caller holds the lock.
func (l *MemoryLoginLimiter) prune(now time.Time) {
	if len(l.entries) < 1024 {
		return
	}
	for k, e := range l.entries {
		if !now.Before(e.until) {
			delete(l.entries, k)
		}
	}
}
