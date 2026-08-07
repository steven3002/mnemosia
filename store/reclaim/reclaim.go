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

// A Live set names everything a sweep must leave alone.
//
// It is supplied rather than derived here, because what is live is a question
// about records, and this package deliberately knows nothing about them.
type Live struct {
	Objects map[sia.ObjectRef]struct{}
	Slabs   map[sia.SlabID]struct{}
}

// A Sweep reports what a reclamation found and released.
type Sweep struct {
	// ObjectsSeen is how many objects the account held.
	ObjectsSeen int
	// ObjectsDeleted is how many of those sat on this vault's slabs with
	// nothing pointing at them any more.
	ObjectsDeleted int
	// Unreadable is how many objects the account holds that the indexer could
	// not open. They are reported rather than removed: they carry no slab to
	// check them against, so dropping them is a separate, deliberate act.
	Unreadable int
	// SlabsReleased is how many slabs were unpinned.
	SlabsReleased int
	// Before and After are the account's quota either side of the sweep, which
	// is the only figure that says whether anything was actually returned.
	Before, After sia.Account
	Elapsed       time.Duration
}

// Freed reports the quota the sweep returned.
func (s Sweep) Freed() uint64 {
	if s.After.PinnedData >= s.Before.PinnedData {
		return 0
	}
	return s.Before.PinnedData - s.After.PinnedData
}

// Sweep releases the storage this vault pinned and no longer needs.
//
// Deleting a record frees nothing on its own, because a slab is shared and
// billed whole: the space comes back only when a slab has no live object left
// and the slab itself is released. So this runs in two passes, and their order
// is not interchangeable. Releasing a slab while an object still points into it
// does not merely orphan the object, the object's identity is derived from the
// slab's sectors, so it becomes something no key opens, and it goes on billing
// and on being listed. Objects first, then slabs.
//
// The ledger, not the account listing, is what bounds this. An account can hold
// objects this vault did not write, another vault under the same key, or an
// older tool, and a sweep that reasoned from "everything the catalog does not
// name" would delete them. Only slabs this vault pinned are its to release.
func (r *Reclaimer) Sweep(ctx context.Context, live Live) (Sweep, error) {
	start := time.Now()
	report := Sweep{}

	before, err := r.client.Account(ctx)
	if err != nil {
		return report, err
	}
	report.Before = before

	tracked, err := r.Tracked()
	if err != nil {
		return report, err
	}
	ours := make(map[sia.SlabID]struct{}, len(tracked))
	for _, slab := range tracked {
		ours[slab.ID] = struct{}{}
	}

	var dead []sia.ObjectRef
	stats, err := r.client.WalkObjectsStats(ctx, func(object sia.StoredObject) error {
		report.ObjectsSeen++
		if _, mine := ours[object.Slab]; !mine {
			return nil
		}
		if _, alive := live.Objects[object.Ref]; alive {
			return nil
		}
		dead = append(dead, object.Ref)
		return nil
	})
	if err != nil {
		return report, err
	}
	report.Unreadable = len(stats.Unreadable)

	if err := r.client.DeleteObjects(ctx, dead); err != nil {
		return report, fmt.Errorf("delete %d dead object(s): %w", len(dead), err)
	}
	report.ObjectsDeleted = len(dead)

	for slabID := range ours {
		if _, alive := live.Slabs[slabID]; alive {
			continue
		}
		if err := r.Unpin(ctx, slabID); err != nil {
			return report, err
		}
		report.SlabsReleased++
	}

	if report.After, err = r.client.Account(ctx); err != nil {
		return report, err
	}
	report.Elapsed = time.Since(start)
	return report, nil
}

// An Orphan is a slab the account is billed for that holds nothing at all.
type Orphan struct {
	ID sia.SlabID
	// Tracked reports whether this device's ledger knew about it. A slab that
	// was not tracked was stranded by an installation that is gone.
	Tracked bool
}

// Orphans lists slabs the account pays for that hold no object.
//
// The indexer's slab list is the authority here, not the ledger. A slab whose
// objects have all been deleted goes on billing forever, and if the device that
// pinned it is gone, reinstalled, replaced, or simply run from a different
// directory, nothing local remembers it exists. Asking the indexer what it is
// charging for is the only way to find that storage.
func (r *Reclaimer) Orphans(ctx context.Context) ([]Orphan, error) {
	pinned, err := r.client.PinnedSlabs(ctx)
	if err != nil {
		return nil, err
	}
	occupied := make(map[sia.SlabID]struct{}, len(pinned))
	if err := r.client.WalkObjects(ctx, func(object sia.StoredObject) error {
		if object.Slab != "" {
			occupied[object.Slab] = struct{}{}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	tracked, err := r.Tracked()
	if err != nil {
		return nil, err
	}
	known := make(map[sia.SlabID]struct{}, len(tracked))
	for _, slab := range tracked {
		known[slab.ID] = struct{}{}
	}

	var out []Orphan
	for _, id := range pinned {
		if _, holds := occupied[id]; holds {
			continue
		}
		_, isKnown := known[id]
		out = append(out, Orphan{ID: id, Tracked: isKnown})
	}
	return out, nil
}

// ReleaseOrphans unpins slabs that hold nothing and returns their quota.
//
// Nothing readable is lost by construction: a slab with no object over it has
// no way to be read even in principle, since the indexer's object map is the
// only thing that turns a slab into retrievable bytes.
func (r *Reclaimer) ReleaseOrphans(ctx context.Context) (Sweep, error) {
	start := time.Now()
	var report Sweep

	var err error
	if report.Before, err = r.client.Account(ctx); err != nil {
		return report, err
	}
	orphans, err := r.Orphans(ctx)
	if err != nil {
		return report, err
	}
	for _, orphan := range orphans {
		if err := r.Unpin(ctx, orphan.ID); err != nil {
			return report, err
		}
		report.SlabsReleased++
	}
	if report.After, err = r.client.Account(ctx); err != nil {
		return report, err
	}
	report.Elapsed = time.Since(start)
	return report, nil
}

// DropUnreadable deletes objects the indexer will not open.
//
// They hold nothing recoverable and keep their slab alive, so they are pure
// cost, but they carry no slab this vault can check them against, which is
// why removing them is asked for rather than assumed. The state is reached by
// releasing a slab while objects still pointed into it.
func (r *Reclaimer) DropUnreadable(ctx context.Context) ([]sia.ObjectRef, error) {
	stats, err := r.client.WalkObjectsStats(ctx, func(sia.StoredObject) error { return nil })
	if err != nil {
		return nil, err
	}
	if len(stats.Unreadable) == 0 {
		return nil, nil
	}
	if err := r.client.DeleteObjects(ctx, stats.Unreadable); err != nil {
		return nil, fmt.Errorf("delete %d unreadable object(s): %w", len(stats.Unreadable), err)
	}
	return stats.Unreadable, nil
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
	// Every cached location over that slab now points at sectors that are gone.
	return r.store.ForgetSlabMeta(string(slabID))
}

// Account reports the vault's standing with the indexer.
func (r *Reclaimer) Account(ctx context.Context) (sia.Account, error) { return r.client.Account(ctx) }
