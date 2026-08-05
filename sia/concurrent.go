package sia

import (
	"context"
	"errors"
	"sync"
)

// inFlight runs fn for each index in [0, n) with at most limit calls
// outstanding, stopping at the first failure.
//
// Every call here is one indexer round trip and nothing more, so the wall clock
// is dominated by latency rather than by work and overlapping is the only thing
// that shortens it.
//
// A failure cancels the rest rather than pressing on. A half-applied batch is
// not treated as recoverable at this level: the caller knows what the batch was
// for and is better placed to choose between retrying it and reclaiming it.
func inFlight(ctx context.Context, n, limit int, fn func(context.Context, int) error) error {
	if n <= 0 {
		return nil
	}
	if limit < 1 {
		limit = 1
	}
	if limit > n {
		limit = n
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	work := make(chan int)

	wg.Add(limit)
	for range limit {
		go func() {
			defer wg.Done()
			for i := range work {
				if err := fn(ctx, i); err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
					cancel()
					return
				}
			}
		}()
	}

	var stopped bool
dispatch:
	for i := range n {
		select {
		case work <- i:
		case <-ctx.Done():
			stopped = true
			break dispatch
		}
	}
	close(work)
	wg.Wait()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if stopped {
		return ctx.Err()
	}
	return nil
}
