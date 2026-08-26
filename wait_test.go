package atlimiter

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitImmediate(t *testing.T) {
	limiter := NewLimiter(10, 1.0)
	ctx := context.Background()

	if err := limiter.Wait(ctx); err != nil {
		t.Fatalf("expected immediate success, got %v", err)
	}
}

func TestWaitBlocksThenSucceeds(t *testing.T) {
	limiter := NewLimiter(10, 1.0)

	// Drain the bucket.
	for range 10 {
		if !limiter.Allow() {
			t.Fatal("initial drain should succeed")
		}
	}

	ctx := context.Background()
	start := time.Now()
	if err := limiter.Wait(ctx); err != nil {
		t.Fatalf("expected Wait to eventually succeed, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("expected Wait to actually block for a refill, only waited %v", elapsed)
	}
}

func TestWaitRespectsContextCancellation(t *testing.T) {
	limiter := NewLimiter(1, 1.0) // 1 rps, so a refill takes ~1s
	for range 1 {
		limiter.Allow()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := limiter.Wait(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestWaitNExceedsCapacity(t *testing.T) {
	limiter := NewLimiter(10, 1.0) // capacity == 10

	err := limiter.WaitN(context.Background(), 100)
	if !errors.Is(err, ErrExceedsCapacity) {
		t.Errorf("expected ErrExceedsCapacity, got %v", err)
	}
}

func TestWaitZeroRPSNeverBlocks(t *testing.T) {
	limiter := NewLimiter(0, 1.0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already-cancelled context

	// maxRPS == 0 means unlimited: should succeed even with a cancelled ctx.
	if err := limiter.Wait(ctx); err != nil {
		t.Errorf("expected unlimited limiter to never block, got %v", err)
	}
}

func TestStringDoesNotPanic(t *testing.T) {
	limiter := NewLimiter(10, 2.0)
	s := limiter.String()
	if s == "" {
		t.Error("expected non-empty String() output")
	}
}
