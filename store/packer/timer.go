package packer

import (
	"context"
	"time"
)

// Start begins firing the age deadlines in the background.
//
// Without it the idle and maximum-age triggers are inert: a record written and
// then left alone would sit on the device until something else happened to
// write, which is precisely the case the deadlines exist for. A vault that is
// open but quiet is the normal case, not an edge one.
//
// Errors from a background flush are handed to onError rather than dropped; the
// records stay queued, so the next tick tries again.
func (p *Packer) Start(ctx context.Context, onError func(error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stop != nil || p.store == nil {
		return
	}
	p.stop, p.done = make(chan struct{}), make(chan struct{})
	go p.run(ctx, p.stop, p.done, onError)
}

// Stop ends background flushing and waits for an in-flight one to finish.
// Whatever is still queued stays queued and durable.
func (p *Packer) Stop() {
	p.mu.Lock()
	stop, done := p.stop, p.done
	p.stop, p.done = nil, nil
	p.mu.Unlock()

	if stop == nil {
		return
	}
	close(stop)
	<-done
}

func (p *Packer) run(ctx context.Context, stop <-chan struct{}, done chan<- struct{}, onError func(error)) {
	defer close(done)

	ticker := time.NewTicker(p.policy.tick())
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			reason, due := p.Due(now)
			if !due {
				continue
			}
			if _, err := p.flush(ctx, reason); err != nil && onError != nil {
				onError(err)
			}
		}
	}
}
