package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAllow_UnderLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lim := New(ctx, 10, 10)
	for i := range 5 {
		if !lim.Allow("user1") {
			t.Errorf("request %d should be allowed", i)
		}
	}
}

func TestAllow_ExceedsLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lim := New(ctx, 10, 10)

	// Exhaust the burst.
	for range 10 {
		lim.Allow("user1")
	}

	// Next request should be rejected.
	if lim.Allow("user1") {
		t.Error("request after burst exhaustion should be rejected")
	}
}

func TestAllow_DifferentUsers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lim := New(ctx, 1, 1)

	if !lim.Allow("user1") {
		t.Error("user1 first request should be allowed")
	}
	// user1 exhausted.
	if lim.Allow("user1") {
		t.Error("user1 second request should be rejected")
	}
	// user2 has its own bucket.
	if !lim.Allow("user2") {
		t.Error("user2 first request should be allowed (separate bucket)")
	}
}

func TestAllow_Disabled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lim := New(ctx, 0, 0)

	for i := range 100 {
		if !lim.Allow("user1") {
			t.Errorf("request %d should be allowed when disabled", i)
		}
	}
}

func TestAllow_Concurrent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lim := New(ctx, 1000, 1000)

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				lim.Allow("user1")
			}
		}()
	}
	wg.Wait()
}

func TestAllow_RecoveryAfterBurst(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// High rate so recovery is fast: 1000 req/s, burst 1.
	lim := New(ctx, 1000, 1)

	if !lim.Allow("user1") {
		t.Fatal("first request should be allowed")
	}
	if lim.Allow("user1") {
		t.Fatal("second request should be rejected (burst=1)")
	}

	// Wait for token to refill (1ms at 1000 req/s).
	time.Sleep(5 * time.Millisecond)

	if !lim.Allow("user1") {
		t.Error("request after recovery should be allowed")
	}
}

func TestAllow_CustomBurst(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Same rate, different burst.
	limBurst1 := New(ctx, 10, 1)
	limBurst10 := New(ctx, 10, 10)

	// Burst=1: only 1 request passes immediately.
	if !limBurst1.Allow("user1") {
		t.Error("burst=1: first request should pass")
	}
	if limBurst1.Allow("user1") {
		t.Error("burst=1: second request should be rejected")
	}

	// Burst=10: 10 requests pass immediately.
	for i := range 10 {
		if !limBurst10.Allow("user1") {
			t.Errorf("burst=10: request %d should pass", i)
		}
	}
	if limBurst10.Allow("user1") {
		t.Error("burst=10: 11th request should be rejected")
	}
}

func TestCleanup_RemovesInactiveEntries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lim := newWithTTL(ctx, 10, 10, 50*time.Millisecond, 25*time.Millisecond)

	lim.Allow("user1")
	lim.Allow("user2")

	if lim.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", lim.Len())
	}

	// Wait for cleanup to evict both entries.
	time.Sleep(150 * time.Millisecond)

	if lim.Len() != 0 {
		t.Errorf("expected 0 entries after cleanup, got %d", lim.Len())
	}
}

func TestCleanup_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	lim := newWithTTL(ctx, 10, 10, 50*time.Millisecond, 25*time.Millisecond)
	lim.Allow("user1")

	cancel()

	// Give the goroutine time to exit.
	time.Sleep(50 * time.Millisecond)

	// Limiter should still work after context cancel (no panic), just no cleanup.
	if !lim.Allow("user2") {
		t.Error("limiter should still work after context cancel")
	}
}
