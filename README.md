# ATLimiter: Atomic Token-Bucket Based Rate Limiter

ATLimiter is a high-performance rate limiter implementation in Go that utilizes atomic operations to provide lock-free concurrency control. Built on the token bucket algorithm, it is designed for high-load systems where traditional mutex-based limiters become bottlenecks.

## Technical Implementation

It operates on three fundamental params:

* **maxRPS** - maximum requests per second
* **capacity** – maximum burst capacity calculated as `maxRPS * capacityFactor`
* **tokens** – current available tokens managed through atomic operations

This architecture is **lock-free**. Unlike conventional rate limiters that use mutex locks, `atlimiter` employs atomic Compare-And-Swap (CAS) operations for all state modifications. Moreover, `sync/atomic` is implemented efficiently in Go assembler. This approach provides several advantages:

* Eliminates lock contention – no routine blocking during token acquisition.
* Provides wait-free progress with guarantee of completion in finite time.

All progress methods are CAS-based and use the private method `calculateTokenRefill()`:

```go
func (r *ATLimiter) calculateTokenRefill() {
    now := time.Now().UnixNano()
    prev := r.lastRefill.Load()
    elapsed := float64(now-prev) / 1e9
    if elapsed <= 0 {
        r.lastRefill.CompareAndSwap(prev, now)
        return
    }
    if r.lastRefill.CompareAndSwap(prev, now) {
        maxRPS := r.maxRPS.Load()
        cap := r.capacity.Load()
        // safe overflow check
        var newTokens uint64
        if float64(maxRPS) > float64(^uint64(0))/elapsed {
            newTokens = cap
        } else {
            newTokens = uint64(float64(maxRPS) * elapsed)
        }
        if newTokens == 0 {
            return
        }
        for {
            cur := r.tokens.Load()
            newTotal := cur + newTokens
            if newTotal > cap {
                newTotal = cap
            }
            if cur == newTotal || r.tokens.CompareAndSwap(cur, newTotal) {
                break
            }
        }
    }
}
```

## Benchmarks

| Benchmark name                 |       (1) |             (2) |          (3) |             (4) |
| ------------------------------ | --------: | --------------: | -----------: | --------------: |
| Benchmark_Allow/atlimiter	| 58644720 | 39.18 ns/op | 0 B/op | 0 allocs/op |
| Benchmark_Allow/rate.limiter | 16538538 | 71.23 ns/op | 0 B/op | 0 allocs/op |
| Benchmark_Allow/uber.ratelimit | 668773 | 1894 ns/op ns/op | 0 B/op | 0 allocs/op |
| Benchmark_Allow_Parallel/atlimiter | 22051213 | 46.61 ns/op | 0 B/op | 0 allocs/op |
| Benchmark_Allow_Parallel/rate.limiter | 8451982 | 146.8 ns/op | 0 B/op | 0 allocs/op |
| Benchmark_Allow_Parallel/uber.ratelimit | 239368 | 4915 ns/op | 0 B/op | 0 allocs/op |

## Instalation

```bash
go get github.com/nick1jesky/atlimiter
```