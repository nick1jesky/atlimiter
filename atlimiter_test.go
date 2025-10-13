package atlimiter

// ok  	atlimiter	3.090s	coverage: 90.4% of statements

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewLimiter(t *testing.T) {
	limiter := NewLimiter(100, 1.5)
	if limiter == nil {
		t.Fatal("NewLimiter returned nil")
	}
	if limiter.GetMaxRPS() != 100 {
		t.Errorf("Expected maxRPS 100, got %d", limiter.GetMaxRPS())
	}
	if limiter.GetCapacity() != 150 {
		t.Errorf("Expected capacity 150, got %d", limiter.GetCapacity())
	}

	limiter2 := NewLimiter(100, 0.2)
	if limiter2.capacity != 100 {
		t.Fatalf("Expected capacity 100, got %d", limiter.capacity)
	}
}

func TestAllow(t *testing.T) {
	limiter := NewLimiter(10, 2.0)

	for i := range 20 {
		if !limiter.Allow() {
			t.Errorf("Request %d should be allowed", i)
		}
	}

	if limiter.Allow() {
		t.Error("Request 21 should be denied")
	}

	time.Sleep(1 * time.Second)

	allowed := 0
	for range 15 {
		if limiter.Allow() {
			allowed++
		}
	}
	if allowed != 10 {
		t.Errorf("Expected 10 allowed requests after 1 second, got %d", allowed)
	}
}

func TestTryAllow(t *testing.T) {
	limiter := NewLimiter(10, 2.0)

	if !limiter.TryAllow(15) {
		t.Error("Should allow 15 tokens")
	}

	if limiter.TryAllow(10) {
		t.Error("Should not allow 10 tokens")
	}

	if !limiter.TryAllow(5) {
		t.Error("Should allow 5 tokens")
	}
}

func TestConcurrentRefill(t *testing.T) {
	limiter := NewLimiter(1000, 2.0)
	var wg sync.WaitGroup
	successCount := atomic.Uint64{}

	for range 2000 {
		limiter.Allow()
	}

	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(10 * time.Millisecond)
			if limiter.Allow() {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	if successCount.Load() == 0 {
		t.Error("At least one goroutine should succeed after refill")
	}
}

func TestMultipleRefillAttempts(t *testing.T) {
	limiter := NewLimiter(100, 1.0)

	var wg sync.WaitGroup
	refillCount := atomic.Uint64{}

	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			time.Sleep(50 * time.Millisecond * time.Duration(id+1))
			if limiter.Available() > 0 {
				refillCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Refill was attempted by %d goroutines", refillCount.Load())
}

func TestAvailable(t *testing.T) {
	limiter := NewLimiter(10, 2.0)

	if available := limiter.Available(); available != 20 {
		t.Errorf("Initial available should be 20, got %d", available)
	}

	limiter.TryAllow(15)
	if available := limiter.Available(); available != 5 {
		t.Errorf("Available after taking 15 should be 5, got %d", available)
	}

	time.Sleep(1 * time.Second)
	available := limiter.Available()
	if available != 15 {
		t.Errorf("Available after 1 second should be exactly 15, got %d", available)
	}
}

func TestSetMaxRPS(t *testing.T) {
	limiter := NewLimiter(10, 2.0)

	limiter.SetMaxRPS(20, 1.5)
	if limiter.GetMaxRPS() != 20 {
		t.Errorf("Expected maxRPS 20 after SetMaxRPS, got %d", limiter.GetMaxRPS())
	}
	if limiter.GetCapacity() != 30 {
		t.Errorf("Expected capacity 30 after SetMaxRPS, got %d", limiter.GetCapacity())
	}
}

func TestConcurrentAccess(t *testing.T) {
	limiter := NewLimiter(1000, 2.0)
	var wg sync.WaitGroup
	successCount := atomic.Uint64{}

	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 300 {
				if limiter.Allow() {
					successCount.Add(1)
				}
				time.Sleep(time.Microsecond)
			}
		}()
	}

	wg.Wait()

	if successCount.Load() > 2500 {
		t.Errorf("Should not allow more than ~2500 requests, got %d", successCount.Load())
	}
}

func TestZeroRPS(t *testing.T) {
	limiter := NewLimiter(0, 1.0)

	for range 100 {
		if !limiter.Allow() {
			t.Error("Should always allow when maxRPS is 0")
		}
	}
}

func Benchmark_Allow(b *testing.B) {
	b.Run("atlimiter", func(b *testing.B) {
		limiter := NewLimiter(1000000, 1.0)

		for b.Loop() {
			limiter.Allow()
		}
	})
	fmt.Println(`b.Run("rate.limiter", func(b *testing.B) {
		limiter := rate.NewLimiter(1000000, 1000000)

		for b.Loop() {
			limiter.Allow()
		}
	})`)
}

func Benchmark_Allow_Parallel(b *testing.B) {
	b.Run("atlimiter", func(b *testing.B) {
		limiter := NewLimiter(1000000, 1.0)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				limiter.Allow()
			}
		})
	})
	fmt.Println(`b.Run("rate.limiter", func(b *testing.B) {
		limiter := rate.NewLimiter(1000000, 1000000)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				limiter.Allow()
			}
		})
	})`)
}
