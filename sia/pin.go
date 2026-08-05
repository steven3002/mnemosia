package sia

import (
	"context"
	"fmt"

	"go.sia.tech/core/types"
	"go.sia.tech/indexd/slabs"
)

// PinSlabs registers the slabs a batch occupies, once for the whole batch.
//
// The SDK's own PinObject performs this step per object. Every object in a
// packed flush shares one slab, so all but the first of those calls is waste —
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
	for i := range batch.objects {
		sealed := batch.objects[i].Seal(c.appKey)
		if err := c.app.PinObject(ctx, c.appKey, sealed.SealedObject); err != nil {
			return fmt.Errorf("pin object %d of %d (%s): %w",
				i+1, len(batch.objects), batch.placements[i].Ref, err)
		}
	}
	return nil
}

// UnpinSlab releases a slab and returns its quota immediately.
//
// It goes around the SDK deliberately. The released SDK exposes only a prune
// call that silently ignores anything younger than its own cutoff and offers no
// way to override it, so an app built on the SDK alone cannot free recent
// storage at all and is told it succeeded.
func (c *Client) UnpinSlab(ctx context.Context, id SlabID) error {
	slabID, err := parseSlabID(id)
	if err != nil {
		return err
	}
	if err := c.app.UnpinSlab(ctx, c.appKey, slabID); err != nil {
		return fmt.Errorf("unpin slab %s: %w", id, err)
	}
	return nil
}

// DeleteObject removes an object's entry from the indexer.
//
// It frees nothing on its own: slabs are shared, so quota returns only when
// every object over a slab is gone and the slab itself is unpinned.
func (c *Client) DeleteObject(ctx context.Context, ref ObjectRef) error {
	if err := c.sdk.DeleteObject(ctx, ref.ID); err != nil {
		return fmt.Errorf("delete object %s: %w", ref, err)
	}
	return nil
}

func parseSlabID(id SlabID) (slabs.SlabID, error) {
	var out slabs.SlabID
	if err := out.UnmarshalText([]byte(id)); err != nil {
		return slabs.SlabID{}, fmt.Errorf("parse slab id %q: %w", id, err)
	}
	return out, nil
}
