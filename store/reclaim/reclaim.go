// Package reclaim tracks slabs and reclaims dead ones.
package reclaim

import (
	"context"
	"fmt"
	"time"

	"github.com/steven3002/mnemosia/local"
	"github.com/steven3002/mnemosia/sia"
)

// A Reclaimer knows which slabs this vault has pinned.
//
// Nothing else does. The indexer bills a slab whether it holds live records or
// none, the released SDK offers no way to release a recent one, and a
// partially-filled slab can never be extended, so every flush strands one.
// Without this list that quota is unrecoverable and invisible at the same time.
type Reclaimer struct {
	client *sia.Client
	store  *local.Store
}

// New builds a reclaimer over the device's slab ledger.
func New(client *sia.Client, store *local.Store) *Reclaimer {
	return &Reclaimer{client: client, store: store}
}

// Track records that a flush pinned a slab.
func (r *Reclaimer) Track(slabID sia.SlabID, records int, bytes int64) error {
	if slabID == "" {
		return nil
	}
	return r.store.TrackSlab(string(slabID), records, bytes)
}

// A Slab is one tracked slab.
type Slab struct {
	ID       sia.SlabID
	PinnedAt time.Time
	Records  int
	Bytes    int64
}

// Tracked lists every slab this vault has pinned, oldest first.
func (r *Reclaimer) Tracked() ([]Slab, error) {
	rows, err := r.store.TrackedSlabs()
	if err != nil {
		return nil, err
	}
	out := make([]Slab, len(rows))
	for i, row := range rows {
		out[i] = Slab{
			ID:       sia.SlabID(row.SlabID),
			PinnedAt: row.PinnedAt,
			Records:  row.Records,
			Bytes:    row.Bytes,
		}
	}
	return out, nil
}

// Unpin releases one slab and drops it from the ledger.
//
// This is the only mechanism that returns quota promptly. It is exposed for
// deliberate release; scheduling it is a separate decision, because the
// operation needs spare headroom to run and an account that has run out of room
// can no longer afford the very thing that would give it room back.
func (r *Reclaimer) Unpin(ctx context.Context, slabID sia.SlabID) error {
	if err := r.client.UnpinSlab(ctx, slabID); err != nil {
		return err
	}
	if err := r.store.ForgetSlab(string(slabID)); err != nil {
		return fmt.Errorf("slab %s was released but the ledger still lists it: %w", slabID, err)
	}
	return nil
}
