package cache

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/dgryski/go-wyhash"
	"github.com/opsramp/tracing-proxy/metrics"
	"github.com/sourcegraph/conc/pool"
)

// genID returns a random hex string of length numChars
func genID(numChars int) string {
	seed := 3565269841805

	const charset = "abcdef0123456789"

	id := make([]byte, numChars)
	for i := 0; i < numChars; i++ {
		id[i] = charset[int(wyhash.Rng(seed))%len(charset)]
	}
	return string(id)
}

// Benchmark the Add function
func BenchmarkCuckooTraceChecker_Add(b *testing.B) {
	traceIDs := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		traceIDs[i] = genID(32)
	}

	c := NewCuckooTraceChecker(1000000, &metrics.NullMetrics{})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Add(traceIDs[i])
	}
}

func BenchmarkCuckooTraceChecker_AddParallel(b *testing.B) {
	traceIDs := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		traceIDs[i] = genID(32)
	}
	const numGoroutines = 70

	p := pool.New().WithMaxGoroutines(numGoroutines + 1)
	stop := make(chan struct{})
	p.Go(func() {
		select {
		case <-stop:
			return
		default:
			rand.Intn(100) // nolint
		}
	})

	c := NewCuckooTraceChecker(1000000, &metrics.NullMetrics{})
	ch := make(chan int, numGoroutines) // nolint:all
	for i := 0; i < numGoroutines; i++ {
		p.Go(func() {
			for n := range ch {
				if i%10000 == 0 {
					c.Maintain()
				}
				c.Add(traceIDs[n])
			}
		})
	}
	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		ch <- j
		if j%1000 == 0 {
			// just give things a moment to run
			time.Sleep(1 * time.Microsecond)
		}
	}
	close(ch)
	close(stop)
	p.Wait()
}

func BenchmarkCuckooTraceChecker_Check(b *testing.B) {
	fmt.Println(b.N)
	traceIDs := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		traceIDs[i] = genID(32)
	}

	c := NewCuckooTraceChecker(1000000, &metrics.NullMetrics{})
	// add every other one to the filter
	for i := 0; i < b.N; i += 2 {
		if i%10000 == 0 {
			c.Maintain()
		}
		c.Add(traceIDs[i])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Check(traceIDs[i])
	}
}

func BenchmarkCuckooTraceChecker_CheckParallel(b *testing.B) {
	fmt.Println(b.N)
	traceIDs := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		traceIDs[i] = genID(32)
	}

	c := NewCuckooTraceChecker(1000000, &metrics.NullMetrics{})
	for i := 0; i < b.N; i += 2 {
		if i%10000 == 0 {
			c.Maintain()
		}
		c.Add(traceIDs[i])
	}

	const numGoroutines = 70

	p := pool.New().WithMaxGoroutines(numGoroutines + 1)
	stop := make(chan struct{})
	ch := make(chan int, numGoroutines) //  nolint:all
	for i := 0; i < numGoroutines; i++ {
		p.Go(func() {
			for n := range ch {
				c.Check(traceIDs[n])
			}
		})
	}
	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		ch <- j
	}
	close(ch)
	close(stop)
	p.Wait()
}
