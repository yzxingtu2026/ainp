package authentication

import (
	"context"
	"sync"
	"time"
)

type RateLimiter interface {
	Allow(context.Context, string) bool
}

type limitEntry struct {
	count int
	reset time.Time
}
type MemoryRateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]limitEntry
}

func NewMemoryRateLimiter(limit int, window time.Duration) *MemoryRateLimiter {
	if limit <= 0 {
		limit = 10
	}
	if window <= 0 {
		window = time.Minute
	}
	return &MemoryRateLimiter{limit: limit, window: window, entries: make(map[string]limitEntry)}
}

func (l *MemoryRateLimiter) Allow(_ context.Context, key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	entry, ok := l.entries[key]
	if !ok || !entry.reset.After(now) {
		l.entries[key] = limitEntry{count: 1, reset: now.Add(l.window)}
		return true
	}
	if entry.count >= l.limit {
		return false
	}
	entry.count++
	l.entries[key] = entry
	return true
}
