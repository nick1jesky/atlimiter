// Package atlimiter implements a rate limiter pattern based on atomic variables.
// Usement of atomic operations provides efficient lock-free limiting making package ideal for high-concurrency applications.
// This realisаtion allows you to avoid overloading the runtime.
package atlimiter

// Only standart libraries
import (
	"sync/atomic"
	"time"
)

// - is a base stucture that provides all operations.
type ATLimiter struct {
	// Max quantity of requests per second - base parameter of rate limiter
	maxRPS uint64
	// Max burst of requests - is an option that allows to increase the speed for a limited period of time
	// even if a lower speed is specified in the max-limit parameter in the queue settings.
	capacity uint64
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
	l := &ATLimiter{
		maxRPS:   maxRPS,
		capacity: capacity,
	}

	l.tokens.Store(capacity)
	l.lastRefill.Store(now)

	return l
}

// - is a private method of ATLimiter that is responsible for calculating and generating new tokens.
//
// Quantity of new tokens calculates using elapsed time and maxRPS.
// For comparing of previous refill of tokens and current time function uses compare-and-swap operation (that realised in sync/atomic/asm.s)
// and realised on Go's assembler
func (r *ATLimiter) calculateTokenRefill() {
	now := time.Now().UnixNano()
	previousRefill := r.lastRefill.Load()

	elapsed := float64(now-previousRefill) / 1e9

	if elapsed > 0 {
		if r.lastRefill.CompareAndSwap(previousRefill, now) {
			newTokens := uint64(float64(r.maxRPS) * elapsed)
			if newTokens > 0 {
				current := r.tokens.Load()
				newTotal := min(current+newTokens, r.capacity)
				r.tokens.Store(newTotal)
			}
		}
	}
}

// - checks the request for available tokens and allows it if tokens are present.
//
// If current quantity of tokens equals zero returns false.
// If tokens available it's compare and swap current quantity and quantity minus one.
func (r *ATLimiter) Allow() bool {
	if r.maxRPS == 0 {
		return true
	}

	r.calculateTokenRefill()

	for {
		current := r.tokens.Load()
		if current == 0 {
			return false
		}
		if r.tokens.CompareAndSwap(current, current-1) {
			return true
		}
	}
}

// - checks and allows N = tokensCount of requests.
func (r *ATLimiter) TryAllow(tokensCount uint64) bool {
	if r.maxRPS == 0 {
		return true
	}
	if tokensCount == 0 {
		return true
	}
	if tokensCount > r.capacity {
		return false
	}

	r.calculateTokenRefill()

	for {
		current := r.tokens.Load()
		if current < tokensCount {
			return false
		}
		if r.tokens.CompareAndSwap(current, current-tokensCount) {
			return true
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

	atomic.StoreUint64(&r.maxRPS, newMaxRPS)
	atomic.StoreUint64(&r.capacity, newCapacity)

	current := r.tokens.Load()
	if current > newCapacity {
		r.tokens.CompareAndSwap(current, newCapacity)
	}
}

// - returns current max RPS
func (r *ATLimiter) GetMaxRPS() uint64 {
	return atomic.LoadUint64(&r.maxRPS)
}

// - returns current capacity
func (r *ATLimiter) GetCapacity() uint64 {
	return r.capacity
}
