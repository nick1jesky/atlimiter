# ATLimiter: Atomic Token-Bucket Based Rate Limiter

ATLimiter - is a high-performance rate limiter implementation in `Go` that utilizes atomic operations to provide lock-free concurrency control.
Built on the `token bucket` algorithm pattern, it's designed for high-load systems where traditional `mutex`-based limiters become bottlenecks.

## Tech implementation

It's operates on three fundamental params:

* **maxRPS** - max requests quantity per second
* **capacity** - max burst capacity calculated as `maxRPS * capacityFactor`
* **tokens** - current available tokens managed through atomic operations

Design of this architecture is **lock-free**. Unlike conventional rate limiters that uses `mutex`-locks, `atlimiter` employs atomic Compare-And-Swap operations for all state modifications. Moreover, `sync.atomic` implemented efficiently in `Go` assembler. This approach provides several technical advantages:

* It's elimenates lock contention - no routine blocking during token acquisition.
* Is's provides `wait-free` progress with guaranty of completion in finite time of each operation.

All progress methods are `CAS`-based uses private method `calculateTokenRefill()` - 

```go
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
```

Only one goroutine successfully updates the refill timestamp using CAS while others procees with current token values ensuring consistent state.

## Use Cases

* **Real-time data processing** pipelines requiring predictable latency
* **Microservices** with strict QoS requirements
* **Load testing** frameworks needing precise rate control

## Instalation

```bash
go get github.com/nick1jesky/atlimiter
```

## *P.S.*

*Benchmarks included in the test suite demonstrate significant performance advantages over traditional mutex-based implementation!*
