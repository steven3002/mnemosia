package sia

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestInFlightRunsEveryIndexOnce(t *testing.T) {
	const n = 500
	var mu sync.Mutex
	seen := make(map[int]int, n)

	if err := inFlight(t.Context(), n, PinConcurrency, func(_ context.Context, i int) error {
		mu.Lock()
		defer mu.Unlock()
		seen[i]++
		return nil
	}); err != nil {
		t.Fatalf("inFlight: %v", err)
	}
	if len(seen) != n {
		t.Fatalf("ran %d of %d indexes", len(seen), n)
	}
	for i, count := range seen {
		if count != 1 {
			t.Fatalf("index %d ran %d times", i, count)
		}
	}
}

func TestInFlightRespectsTheLimit(t *testing.T) {
	const limit = 4
	var current, peak atomic.Int64

	if err := inFlight(t.Context(), 200, limit, func(_ context.Context, _ int) error {
		running := current.Add(1)
		defer current.Add(-1)
		for {
			was := peak.Load()
			if running <= was || peak.CompareAndSwap(was, running) {
				break
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("inFlight: %v", err)
	}
	if peak.Load() > limit {
		t.Fatalf("%d calls were in flight at once, limit is %d", peak.Load(), limit)
	}
}

func TestInFlightStopsAtTheFirstFailure(t *testing.T) {
	wanted := errors.New("indexer refused the pin")
	var calls atomic.Int64

	err := inFlight(t.Context(), 10_000, 2, func(_ context.Context, i int) error {
		calls.Add(1)
		if i == 3 {
			return wanted
		}
		return nil
	})
	if !errors.Is(err, wanted) {
		t.Fatalf("got %v, want the underlying failure", err)
	}
	// The point of stopping is to stop: a batch that cannot finish should not
	// keep spending round trips on the indexer.
	if calls.Load() > 1_000 {
		t.Fatalf("%d calls ran after the failure", calls.Load())
	}
}

func TestInFlightReportsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := inFlight(ctx, 32, 4, func(context.Context, int) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestInFlightHandlesAnEmptyBatch(t *testing.T) {
	if err := inFlight(t.Context(), 0, PinConcurrency, func(context.Context, int) error {
		t.Fatal("an empty batch ran work")
		return nil
	}); err != nil {
		t.Fatalf("inFlight: %v", err)
	}
}
