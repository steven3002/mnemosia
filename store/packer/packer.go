package packer

import (
	"context"
	"sync"
	"time"

	"github.com/steven3002/mnemosia/record"
	"github.com/steven3002/mnemosia/store"
)

// A Queued record is one sealed body awaiting a flush.
type Queued struct {
	ID   record.ID
	Kind record.Kind
	Blob store.Blob
	// Since is when the record was queued, which is when its durability window
	// opened.
	Since time.Time
}

// A Packer accumulates sealed records and writes them as one batch.
//
// Between a record being queued and a flush completing, that record exists only
// on this device. The window is a deliberate trade for the slab economics, but
// it is a real one, and nothing above may report a queued record as stored on
// the network.
type Packer struct {
	mu      sync.Mutex
	store   *store.Store
	policy  Policy
	queue   []Queued
	bytes   int64
	oldest  time.Time
	latest  time.Time
	onFlush func(Result) error
}

// New builds a packer that writes through the given store.
func New(s *store.Store, policy Policy) *Packer {
	return &Packer{store: s, policy: policy}
}

// OnFlush registers a callback invoked after each successful flush, before the
// flush returns. Its error is propagated: a batch that reached the network but
// could not be catalogued is a problem the caller has to hear about, since the
// bytes are now billed and nothing points at them.
func (p *Packer) OnFlush(fn func(Result) error) { p.onFlush = fn }

// A Result reports what one flush wrote.
type Result struct {
	Reason  Reason
	Records []Queued
	Flush   store.Flush
}

// Empty reports whether the flush wrote nothing.
func (r Result) Empty() bool { return len(r.Records) == 0 }

// Add queues a sealed record and flushes if the policy says it is due.
func (p *Packer) Add(ctx context.Context, item Queued) (Result, error) {
	p.mu.Lock()
	if item.Since.IsZero() {
		item.Since = time.Now()
	}
	if len(p.queue) == 0 {
		p.oldest = item.Since
	}
	p.latest = item.Since
	p.queue = append(p.queue, item)
	p.bytes += int64(len(item.Blob.Payload))
	reason, due := p.policy.due(len(p.queue), p.bytes, p.oldest, p.latest, time.Now())
	p.mu.Unlock()

	if !due {
		return Result{}, nil
	}
	return p.flush(ctx, reason)
}

// Due reports whether the queue has become flushable through the passage of
// time alone.
func (p *Packer) Due(now time.Time) (Reason, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.policy.due(len(p.queue), p.bytes, p.oldest, p.latest, now)
}

// Pending reports how many records are queued but not yet on the network.
func (p *Packer) Pending() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.queue)
}

// Flush writes whatever is queued.
func (p *Packer) Flush(ctx context.Context) (Result, error) { return p.flush(ctx, ReasonManual) }

func (p *Packer) flush(ctx context.Context, reason Reason) (Result, error) {
	p.mu.Lock()
	if len(p.queue) == 0 {
		p.mu.Unlock()
		return Result{}, nil
	}
	batch := p.queue
	p.queue, p.bytes = nil, 0
	p.mu.Unlock()

	blobs := make([]store.Blob, len(batch))
	for i, item := range batch {
		blobs[i] = item.Blob
	}

	flushed, err := p.store.PutBatch(ctx, blobs)
	if err != nil {
		// The records are still only on this device, so put them back rather
		// than dropping them on the floor.
		p.requeue(batch)
		return Result{}, err
	}

	result := Result{Reason: reason, Records: batch, Flush: flushed}
	if p.onFlush != nil {
		if err := p.onFlush(result); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (p *Packer) requeue(batch []Queued) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.queue = append(batch, p.queue...)
	p.bytes = 0
	for _, item := range p.queue {
		p.bytes += int64(len(item.Blob.Payload))
	}
	if len(p.queue) > 0 {
		p.oldest = p.queue[0].Since
		p.latest = p.queue[len(p.queue)-1].Since
	}
}
