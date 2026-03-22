package ratelimit

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultTTL      = 10 * time.Minute
	defaultInterval = 1 * time.Minute
)

// Limiter controls per-key request rates.
type Limiter interface {
	Allow(key string) bool
}

// New creates a Limiter. Returns a NoopLimiter if ratePerSec is 0.
func New(ctx context.Context, ratePerSec float64, burst int) Limiter {
	if ratePerSec <= 0 {
		return noopLimiter{}
	}
	return newKeyed(ctx, ratePerSec, burst, defaultTTL, defaultInterval)
}

// newWithTTL creates a keyedLimiter with custom cleanup settings. Used by tests.
func newWithTTL(ctx context.Context, ratePerSec float64, burst int, ttl, cleanupInterval time.Duration) *keyedLimiter {
	return newKeyed(ctx, ratePerSec, burst, ttl, cleanupInterval)
}

// Len returns the number of tracked keys. Exposed for testing.
func (kl *keyedLimiter) Len() int {
	kl.mu.RLock()
	defer kl.mu.RUnlock()
	return len(kl.limiters)
}

// --- NoopLimiter ---

type noopLimiter struct{}

func (noopLimiter) Allow(string) bool { return true }

// --- KeyedLimiter ---

type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type keyedLimiter struct {
	rate     rate.Limit
	burst    int
	ttl      time.Duration
	mu       sync.RWMutex
	limiters map[string]*entry
}

func newKeyed(ctx context.Context, ratePerSec float64, burst int, ttl, cleanupInterval time.Duration) *keyedLimiter {
	kl := &keyedLimiter{
		rate:     rate.Limit(ratePerSec),
		burst:    burst,
		ttl:      ttl,
		limiters: make(map[string]*entry),
	}
	go kl.cleanup(ctx, cleanupInterval)
	return kl
}

func (kl *keyedLimiter) Allow(key string) bool {
	now := time.Now()

	kl.mu.Lock()
	e, ok := kl.limiters[key]
	if !ok {
		e = &entry{
			limiter:  rate.NewLimiter(kl.rate, kl.burst),
			lastSeen: now,
		}
		kl.limiters[key] = e
	}
	e.lastSeen = now
	kl.mu.Unlock()

	// rate.Limiter is internally thread-safe.
	return e.limiter.Allow()
}

func (kl *keyedLimiter) cleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			kl.evict()
		}
	}
}

func (kl *keyedLimiter) evict() {
	now := time.Now()
	kl.mu.Lock()
	defer kl.mu.Unlock()

	for key, e := range kl.limiters {
		if now.Sub(e.lastSeen) > kl.ttl {
			delete(kl.limiters, key)
		}
	}
}
