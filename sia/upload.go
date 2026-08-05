package sia

import (
	"bytes"
	"context"
	"fmt"

	"go.sia.tech/siastorage"
)

// A Placement is where one uploaded blob landed.
type Placement struct {
	Ref  ObjectRef
	Slab SlabID
	// Bytes is the payload length as written, before erasure coding.
	Bytes int
}

// A Batch is one finalized packed upload, not yet pinned.
//
// It holds the SDK's object descriptors so pinning can be split, the slabs
// once for the whole batch, then the objects, without those descriptors
// escaping this package.
type Batch struct {
	objects    []siastorage.Object
	placements []Placement
}

// Len reports how many blobs the batch carries.
func (b *Batch) Len() int { return len(b.objects) }

// Placements reports where each blob landed, in the order it was added.
func (b *Batch) Placements() []Placement { return b.placements }

// Slabs lists the distinct slabs the batch occupies.
func (b *Batch) Slabs() []SlabID {
	seen := make(map[SlabID]struct{}, 1)
	var out []SlabID
	for _, p := range b.placements {
		if p.Slab == "" {
			continue
		}
		if _, ok := seen[p.Slab]; ok {
			continue
		}
		seen[p.Slab] = struct{}{}
		out = append(out, p.Slab)
	}
	return out
}

// UploadPacked writes a batch of blobs into shared slabs and returns where each
// landed, unpinned.
//
// There is no single-blob variant on purpose. The indexer bills a whole slab
// however little it holds, so a lone small record costs roughly four orders of
// magnitude more than its own size. Packing is the write path, not an
// optimisation of it.
func (c *Client) UploadPacked(ctx context.Context, payloads [][]byte, opts ...UploadOption) (*Batch, error) {
	if len(payloads) == 0 {
		return &Batch{}, nil
	}
	options := make([]siastorage.UploadOption, 0, len(opts))
	for _, opt := range opts {
		options = append(options, siastorage.UploadOption(opt))
	}

	upload, err := c.sdk.UploadPacked(options...)
	if err != nil {
		return nil, fmt.Errorf("open packed upload: %w", err)
	}
	defer upload.Close()

	for i, payload := range payloads {
		if _, err := upload.Add(ctx, bytes.NewReader(payload)); err != nil {
			return nil, fmt.Errorf("pack blob %d of %d: %w", i+1, len(payloads), err)
		}
	}

	objects, err := upload.Finalize(ctx)
	if err != nil {
		return nil, fmt.Errorf("finalize packed upload: %w", err)
	}
	if len(objects) != len(payloads) {
		return nil, fmt.Errorf("packed upload returned %d objects for %d blobs", len(objects), len(payloads))
	}

	batch := &Batch{objects: objects, placements: make([]Placement, len(objects))}
	for i := range objects {
		batch.placements[i] = Placement{
			Ref:   ObjectRef{ID: objects[i].ID()},
			Slab:  slabOf(&objects[i]),
			Bytes: len(payloads[i]),
		}
	}
	return batch, nil
}

// SlabPayloadSize reports how many payload bytes one slab holds, which is what
// a flush is billed for whether it uses them or not.
func (c *Client) SlabPayloadSize() (int64, error) {
	upload, err := c.sdk.UploadPacked()
	if err != nil {
		return 0, fmt.Errorf("open packed upload: %w", err)
	}
	defer upload.Close()
	return upload.OptimalDataSize(), nil
}

// An UploadOption tunes a write. Erasure-coding parameters are the dominant
// cost lever, so they stay reachable from above without exposing the SDK.
type UploadOption siastorage.UploadOption

// WithRedundancy overrides the erasure coding for one upload. The slab quantum
// is dataShards x 4 MiB, so this changes what a flush costs.
func WithRedundancy(dataShards, parityShards uint8) UploadOption {
	return UploadOption(siastorage.WithRedundancy(dataShards, parityShards))
}

// slabOf names the slab a packed object sits in. Every object in one flush
// shares it, which is what makes pinning slabs once per flush correct.
func slabOf(obj *siastorage.Object) SlabID {
	slices := obj.Slabs()
	if len(slices) == 0 {
		return ""
	}
	return SlabID(slices[0].Digest().String())
}
