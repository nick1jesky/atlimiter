package atlimiter

import (
	"context"
	"time"
)

// Wait blocks until a single token is available or ctx is done, whichever
// comes first. It returns ctx.Err() if the context is cancelled or its
// deadline is exceeded before a token could be reserved.
//
// Unlike Allow, Wait never busy-spins: while it waits it sleeps for the
// estimated time until enough tokens will have refilled, re-checking on
// each wake-up (and immediately on ctx.Done()).
func (r *ATLimiter) Wait(ctx context.Context) error {
	return r.WaitN(ctx, 1)
}

// WaitN blocks until tokensCount tokens are available or ctx is done.
//
// It returns ErrExceedsCapacity immediately, without touching ctx, if
// tokensCount exceeds the limiter's capacity, since no amount of waiting
// could ever satisfy the request.
func (r *ATLimiter) WaitN(ctx context.Context, tokensCount uint64) error {
	if tokensCount == 0 {
		return nil
	}

	if r.maxRPS.Load() == 0 {
		return nil
	}

	capacity := r.capacity.Load()
	if tokensCount > capacity {
		return ErrExceedsCapacity
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		r.calculateTokenRefill()

		current := r.tokens.Load()
		if current >= tokensCount {
			if r.tokens.CompareAndSwap(current, current-tokensCount) {
				return nil
			}
			// Lost the CAS race to another caller; re-check immediately
			// rather than sleeping, since tokens may already be sufficient.
			continue
		}

		maxRPS := r.maxRPS.Load()
		deficit := tokensCount - current
		waitFor := time.Duration(float64(deficit) / float64(maxRPS) * float64(time.Second))
		if waitFor <= 0 {
			waitFor = time.Millisecond
		}

		timer := time.NewTimer(waitFor)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
