package sia

import (
	"context"
	"fmt"
	"time"

	"go.sia.tech/core/types"
	"go.sia.tech/indexd/api"
	"go.sia.tech/indexd/slabs"
	"go.sia.tech/siastorage"
)

// EnumeratePageSize is how many entries one page of the object feed carries.
// The indexer rejects a larger request outright rather than clamping it.
const EnumeratePageSize = 500

// A StoredObject is an object the indexer holds for this account, carrying the
// location metadata needed to read it without a second lookup.
type StoredObject struct {
	Ref  ObjectRef
	Slab SlabID
	// Bytes is the payload length, before erasure coding.
	Bytes uint64
	// UpdatedAt is when the indexer last saw a change to this object.
	UpdatedAt time.Time

	object siastorage.Object
}

// A WalkStats records how a walk was served, so the feed's behaviour is
// observable rather than assumed.
type WalkStats struct {
	// Pages and Events count what the feed returned.
	Pages, Events int
	// Deleted and Superseded count events collapsed away: an object removed,
	// and an earlier version of one that was written again.
	Deleted, Superseded int
	// Live is how many distinct objects the account currently holds.
	Live int
	// Unreadable lists objects the feed returned that could not be opened.
	// They are skipped rather than fatal, so one damaged entry costs its own
	// records and not the whole recovery. They are still billed, so they are
	// reported rather than merely tolerated.
	Unreadable []ObjectRef
}

// WalkObjects calls fn once for every object this account currently has pinned.
//
// It is what makes the catalog a performance index rather than a correctness
// requirement: given the seed and this listing, every record can be rebuilt
// with no local state at all.
//
// The indexer serves a change feed rather than a listing, so a walk from the
// zero cursor replays the account's whole history: an object written twice
// appears at its latest position only, and a removed one appears as a deletion.
// Both are collapsed here, so fn sees each live object exactly once.
func (c *Client) WalkObjects(ctx context.Context, fn func(StoredObject) error) error {
	_, err := c.WalkObjectsStats(ctx, fn)
	return err
}

// WalkObjectsStats enumerates like WalkObjects and additionally reports how the
// feed answered.
func (c *Client) WalkObjectsStats(ctx context.Context, fn func(StoredObject) error) (WalkStats, error) {
	var (
		stats  WalkStats
		cursor slabs.Cursor
		// The feed is ordered by update time, so a later event for a key
		// supersedes an earlier one and the last event for a key is the truth.
		live = make(map[types.Hash256]StoredObject)
		// Keys in the order they were first seen, so two walks of an unchanged
		// account are directly comparable.
		order []types.Hash256
		seen  = make(map[types.Hash256]struct{})
	)
	for {
		// The listing is read through the indexer client rather than the SDK's
		// wrapper because the wrapper opens every object in the page and fails
		// the whole call if any one of them will not open. An account can hold
		// such an object — unpinning a slab out from under a live object leaves
		// exactly that — and one of them must not cost the account its ability
		// to enumerate itself.
		events, err := c.app.ListObjects(ctx, c.appKey, cursor, EnumeratePageSize)
		if err != nil {
			return stats, fmt.Errorf("list objects after %s: %w", cursor.After, err)
		}
		if len(events) == 0 {
			break
		}
		stats.Pages++
		stats.Events += len(events)

		for i := range events {
			event := events[i]
			cursor = slabs.Cursor{Key: event.Key, After: event.UpdatedAt}

			if _, already := seen[event.Key]; already {
				stats.Superseded++
			} else {
				seen[event.Key] = struct{}{}
				order = append(order, event.Key)
			}
			if event.Deleted || event.Object == nil {
				stats.Deleted++
				delete(live, event.Key)
				continue
			}
			sealed := siastorage.SealedObject{SealedObject: *event.Object}
			object, err := sealed.Open(c.appKey)
			if err != nil {
				stats.Unreadable = append(stats.Unreadable, ObjectRef{ID: event.Key})
				delete(live, event.Key)
				continue
			}
			live[event.Key] = StoredObject{
				Ref:       ObjectRef{ID: event.Key},
				Slab:      slabOf(&object),
				Bytes:     object.Size(),
				UpdatedAt: event.UpdatedAt,
				object:    object,
			}
		}
	}

	for _, key := range order {
		object, ok := live[key]
		if !ok {
			continue
		}
		stats.Live++
		if err := fn(object); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

// ReadObject fetches an enumerated object's bytes without asking the indexer
// again, since the walk already carries where it lives.
func (c *Client) ReadObject(object StoredObject) ([]byte, error) {
	return c.read(&object.object, nil)
}

// PinnedSlabs lists every slab the account is being billed for.
//
// This is the indexer's own answer rather than the device's, and the two can
// disagree in the direction that costs money: a slab whose objects have all
// been deleted still bills, and nothing on the device necessarily remembers it.
// A vault that only ever consulted its own ledger could not find storage a
// previous installation stranded.
func (c *Client) PinnedSlabs(ctx context.Context) ([]SlabID, error) {
	const page = 500
	var out []SlabID
	for offset := 0; ; offset += page {
		batch, err := c.app.SlabIDs(ctx, c.appKey, api.WithOffset(offset), api.WithLimit(page))
		if err != nil {
			return nil, fmt.Errorf("list pinned slabs from %d: %w", offset, err)
		}
		for _, id := range batch {
			out = append(out, SlabID(id.String()))
		}
		if len(batch) < page {
			return out, nil
		}
	}
}
