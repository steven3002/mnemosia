package sia

import (
	"context"
	"fmt"
	"strings"

	"go.sia.tech/core/types"
	"go.sia.tech/indexd/slabs"
)

// PinSlabs registers the slabs a batch occupies, once for the whole batch.
//
// The SDK's own PinObject performs this step per object. Every object in a
// packed flush shares one slab, so all but the first of those calls is waste,
// which is why the two halves are split here rather than left fused.
func (c *Client) PinSlabs(ctx context.Context, batch *Batch) error {
	if batch == nil || len(batch.objects) == 0 {
		return nil
	}
	seen := make(map[types.Hash256]struct{}, 1)
	var params []slabs.SlabPinParams
	for i := range batch.objects {
		for _, slice := range batch.objects[i].Slabs() {
			digest := types.Hash256(slice.Digest())
			if _, ok := seen[digest]; ok {
				continue
			}
			seen[digest] = struct{}{}
			params = append(params, slabs.SlabPinParams{
				EncryptionKey: slice.EncryptionKey,
				MinShards:     slice.MinShards,
				Sectors:       slice.Sectors,
			})
		}
	}
	if len(params) == 0 {
		return nil
	}
	if _, err := c.app.PinSlabs(ctx, c.appKey, params...); err != nil {
		return fmt.Errorf("pin %d slab(s): %w", len(params), err)
	}
	return nil
}

// PinConcurrency is how many object pins are in flight at once.
//
// Pinning is one indexer round trip per object and nothing else, so the wall
// clock is latency, not work, and the only way to spend less of it is to
// overlap. Measured throughput climbs steeply to this point and then flattens;
// past it the extra requests buy nothing and only widen the blast radius of a
// failure.
const PinConcurrency = 16

// PinObjects records each object's location with the indexer, making the batch
// durable and retrievable by id.
//
// A blob is on hosts after the upload but unreachable until it is pinned: the
// indexer holds the map from object id to slab, offset and length, and without
// that entry there is no way to ask for it back.
func (c *Client) PinObjects(ctx context.Context, batch *Batch) error {
	if batch == nil || len(batch.objects) == 0 {
		return nil
	}
	return inFlight(ctx, len(batch.objects), PinConcurrency, func(ctx context.Context, i int) error {
		sealed := batch.objects[i].Seal(c.appKey)
		if err := c.app.PinObject(ctx, c.appKey, sealed.SealedObject); err != nil {
			return fmt.Errorf("pin object %d of %d (%s): %w",
				i+1, len(batch.objects), batch.placements[i].Ref, err)
		}
		return nil
	})
}

// DeleteObjects removes a set of objects from the indexer.
//
// It frees nothing by itself; quota returns only when a slab has no live
// objects left and is then unpinned. Doing it in that order is not a
// preference: releasing a slab while objects still point into it leaves those
// objects permanently unopenable, because their identity is derived from the
// sectors that just went away.
func (c *Client) DeleteObjects(ctx context.Context, refs []ObjectRef) error {
	return inFlight(ctx, len(refs), PinConcurrency, func(ctx context.Context, i int) error {
		return c.DeleteObject(ctx, refs[i])
	})
}

// UnpinSlab releases a slab and returns its quota immediately.
//
// It goes around the SDK deliberately. The released SDK exposes only a prune
// call that silently ignores anything younger than its own cutoff and offers no
// way to override it, so an app built on the SDK alone cannot free recent
// storage at all and is told it succeeded. Exposing this call was proposed
// upstream and declined, so the detour is the long-term arrangement rather than
// a stopgap.
//
// A slab that is already gone counts as success. The reason is not tidiness: the
// indexer is expected to start releasing slabs by itself once their last object
// is deleted, at which point this call becomes a second opinion on work already
// done. Treating that as a failure would turn a working reclamation into a
// reported error the day the service changes underneath us.
func (c *Client) UnpinSlab(ctx context.Context, id SlabID) error {
	slabID, err := parseSlabID(id)
	if err != nil {
		return err
	}
	if err := c.app.UnpinSlab(ctx, c.appKey, slabID); err != nil {
		if isSlabGone(err, id) {
			return nil
		}
		return fmt.Errorf("unpin slab %s: %w", id, err)
	}
	return nil
}

// isSlabGone recognises the indexer's absent-slab reply.
//
// The test is on the message because that is all that survives the wire: the
// indexer's typed error is rendered into the response body and rebuilt by the
// client as a plain error, so there is nothing to compare against. It cannot be
// a test for the sentinel's own text either. The service interpolates the slab
// id into the middle of that wording, "slab not found" arrives as
// "slab <id> not found", so a search for the sentinel matches nothing, which
// is why the tolerance below it went four scopes without ever firing.
//
// Requiring the id as well as the words keeps it narrow. A transport-level "404
// page not found", or any absence that is not this slab's, is not swallowed.
func isSlabGone(err error, id SlabID) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	if strings.Contains(message, slabs.ErrSlabNotFound.Error()) {
		return true
	}
	return strings.Contains(message, string(id)) && strings.Contains(message, notFoundPhrase)
}

// notFoundPhrase is the tail the indexer keeps whichever entity it names.
const notFoundPhrase = "not found"

// DeleteObject removes an object's entry from the indexer.
//
// It frees nothing on its own: slabs are shared, so quota returns only when
// every object over a slab is gone and the slab itself is unpinned.
//
// Deleting something that is already gone counts as success. Reclamation is
// resumed after interruptions and run against a ledger that can be a step
// behind the indexer, so the alternative is a sweep that cannot finish because
// part of it finished last time.
func (c *Client) DeleteObject(ctx context.Context, ref ObjectRef) error {
	if err := c.sdk.DeleteObject(ctx, ref.ID); err != nil {
		if isNotFound(err, ref) {
			return nil
		}
		return fmt.Errorf("delete object %s: %w", ref, err)
	}
	return nil
}

// isNotFound recognises the indexer's absent-object reply.
//
// Shaped like isSlabGone and for the same reason: the typed error does not
// survive the wire, and the service interpolates the id into the sentinel's own
// wording, so the sentinel is not a substring of what arrives.
func isNotFound(err error, ref ObjectRef) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	if strings.Contains(message, slabs.ErrObjectNotFound.Error()) {
		return true
	}
	return strings.Contains(message, ref.String()) && strings.Contains(message, notFoundPhrase)
}

func parseSlabID(id SlabID) (slabs.SlabID, error) {
	var out slabs.SlabID
	if err := out.UnmarshalText([]byte(id)); err != nil {
		return slabs.SlabID{}, fmt.Errorf("parse slab id %q: %w", id, err)
	}
	return out, nil
}
