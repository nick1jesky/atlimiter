// Package atlimiter implements a rate limiter pattern based on atomic variables.
// Usement of atomic operations provides efficient lock-free limiting making package ideal for high-concurrency applications.
// This realisаtion allows you to avoid overloading the runtime.
package atlimiter

// Only standart libraries
import (
	"fmt"
	"sync/atomic"
	"time"
)

// - is a base stucture that provides all operations.
type ATLimiter struct {
	// Max quantity of requests per second - base parameter of rate limiter.
	// Stored atomically since SetMaxRPS can mutate it concurrently with reads
	// from Allow/TryAllow/calculateTokenRefill.
	maxRPS atomic.Uint64
	// Max burst of requests - is an option that allows to increase the speed for a limited period of time
	// even if a lower speed is specified in the max-limit parameter in the queue settings.
	// Stored atomically for the same reason as maxRPS.
	capacity atomic.Uint64
	// The current number of tokens in the token container.
	// Tokens are generated every second and spent on request at a one-to-one ratio.
	tokens atomic.Uint64
	// Last refill - is a previous token replenishment in unix nanoseconds
	lastRefill atomic.Int64
}

// - is a constructor of atlimiter copies.
//
// Takes maxRPS, the maximum number of requests per second, as a parameter.
// Takes capacityFactor, capacity increase multiplier in float64 number, as a parameter.
func NewLimiter(maxRPS uint64, capacityFactor float64) *ATLimiter {
	if capacityFactor < 1.0 {
		capacityFactor = 1.0
	}

	capacity := max(max(uint64(float64(maxRPS)*capacityFactor), 1), maxRPS)

	now := time.Now().UnixNano()
	l := &ATLimiter{}

	l.maxRPS.Store(maxRPS)
	l.capacity.Store(capacity)
	l.tokens.Store(capacity)
	l.lastRefill.Store(now)

	return l
}

// - is a private method of ATLimiter that is responsible for calculating and generating new tokens.
//
// Quantity of new tokens calculates using elapsed time and maxRPS.
// For comparing of previous refill of tokens and current time function uses compare-and-swap operation (that realised in sync/atomic/asm.s)
// and realised on Go's assembler.
//
// The token update itself is also CAS-based (not a plain Store): between the
// Load and the write, a concurrent Allow/TryAllow call can decrement tokens.
// A plain Store here would clobber that decrement and let the limiter hand
// out more tokens than the configured rate allows. The retry loop guarantees
// the refill is applied on top of the latest observed value.
func (r *ATLimiter) calculateTokenRefill() {
	now := time.Now().UnixNano()
	prev := r.lastRefill.Load()
	elapsed := float64(now-prev) / 1e9

	// If time went backwards, update lastRefill without adding tokens.
	if elapsed <= 0 {
		r.lastRefill.CompareAndSwap(prev, now)
		return
	}

	// Only one goroutine succeeds in updating lastRefill.
	if r.lastRefill.CompareAndSwap(prev, now) {
		maxRPS := r.maxRPS.Load()
		cap := r.capacity.Load()

		// Safely compute newTokens, avoiding uint64 overflow.
		var newTokens uint64
		if float64(maxRPS) > float64(^uint64(0))/elapsed {
			newTokens = cap // overflow would happen, so cap it.
		} else {
			newTokens = uint64(float64(maxRPS) * elapsed)
		}
		if newTokens == 0 {
			return
		}

		// CAS loop to add newTokens to tokens, capping at capacity.
		for {
			cur := r.tokens.Load()
			newTotal := min(cur+newTokens, cap)
			if cur == newTotal || r.tokens.CompareAndSwap(cur, newTotal) {
				break
			}
		}
	}
}

// - checks the request for available tokens and allows it if tokens are present.
//
// If current quantity of tokens equals zero returns false.
// If tokens available it's compare and swap current quantity and quantity minus one.
func (r *ATLimiter) Allow() bool {
	if r.maxRPS.Load() == 0 {
		return true
	}
	for {
		// Fast path: try to take a token without refilling.
		current := r.tokens.Load()
		if current > 0 && r.tokens.CompareAndSwap(current, current-1) {
			return true
		}
		// No tokens or CAS lost: refill and retry.
		r.calculateTokenRefill()
		// If still no tokens, give up.
		if r.tokens.Load() == 0 {
			return false
		}
	}
}

// - checks and allows N = tokensCount of requests.
func (r *ATLimiter) TryAllow(tokensCount uint64) bool {
	if r.maxRPS.Load() == 0 {
		return true
	}
	if tokensCount == 0 {
		return true
	}
	if tokensCount > r.capacity.Load() {
		return false
	}
	for {
		current := r.tokens.Load()
		if current >= tokensCount && r.tokens.CompareAndSwap(current, current-tokensCount) {
			return true
		}
		// Not enough tokens or CAS lost – refill.
		r.calculateTokenRefill()
		if r.tokens.Load() < tokensCount {
			return false
		}
	}
}

// - returns quantity of available tokens
func (r *ATLimiter) Available() uint64 {
	r.calculateTokenRefill()
	return r.tokens.Load()
}

// - is a function designed to change maxRPS and capacity during execution.
//
// Takes newMaxRPS, the new maximum number of requests per second, as a parameter.
// Takes capacityFactor, new capacity increase multiplier in float64 number, as a parameter.
// If current quantity of tokens is more than new calculated capacity it's compare and swap it with new.
func (r *ATLimiter) SetMaxRPS(newMaxRPS uint64, newCapacityFactor float64) {
	if newCapacityFactor < 1.0 {
		newCapacityFactor = 1.0
	}
	newCapacity := max(max(uint64(float64(newMaxRPS)*newCapacityFactor), 1), newMaxRPS)

	r.maxRPS.Store(newMaxRPS)
	r.capacity.Store(newCapacity)

	// Reduce tokens if they exceed new capacity.
	for {
		current := r.tokens.Load()
		if current <= newCapacity {
			break
		}
		if r.tokens.CompareAndSwap(current, newCapacity) {
			break
		}
	}
}

// - changes only the burst capacity, leaving maxRPS unchanged.
// If current tokens exceed the new capacity, they are reduced.
func (r *ATLimiter) SetCapacity(newCapacity uint64) {
	if newCapacity < r.maxRPS.Load() {
		newCapacity = r.maxRPS.Load() // capacity cannot be less than maxRPS
	}
	r.capacity.Store(newCapacity)
	// Reduce tokens if needed.
	for {
		current := r.tokens.Load()
		if current <= newCapacity {
			break
		}
		if r.tokens.CompareAndSwap(current, newCapacity) {
			break
		}
	}
}

// - returns current max RPS
func (r *ATLimiter) GetMaxRPS() uint64 {
	return r.maxRPS.Load()
}

// - returns current capacity
func (r *ATLimiter) GetCapacity() uint64 {
	return r.capacity.Load()
}

// - restores the limiter to its initial state: tokens = capacity, lastRefill = now.
func (r *ATLimiter) Reset() {
	r.tokens.Store(r.capacity.Load())
	r.lastRefill.Store(time.Now().UnixNano())
}

// String implements fmt.Stringer, giving a compact snapshot of the
// limiter's state for logs and debugging. The snapshot is inherently
// racy with respect to concurrent Allow/TryAllow calls (as is any
// observation of a lock-free structure) and is meant for diagnostics,
// not for making control decisions.
func (r *ATLimiter) String() string {
	return fmt.Sprintf(
		"ATLimiter{maxRPS: %d, capacity: %d, available: %d}",
		r.maxRPS.Load(), r.capacity.Load(), r.tokens.Load(),
	)
}
