package sia

import (
	"context"
	"encoding/json"
	"fmt"

	"go.sia.tech/siastorage"
)

// SlabMeta is an object's location, sealed under the app key, in a form that
// survives a process restart.
//
// Holding it turns a read from two round trips into one. It is a fast path with
// an expiry, never an indefinite cache: a stale host set does not degrade the
// download, it crashes the process, because the SDK's slab reader checks its
// bounds before it filters unusable hosts and then indexes the empty result
// from a goroutine the caller cannot recover.
type SlabMeta struct {
	Ref   ObjectRef
	Bytes []byte
}

// SlabMetaFor captures the location metadata for an object.
func (c *Client) SlabMetaFor(ctx context.Context, ref ObjectRef) (SlabMeta, error) {
	object, err := c.sdk.Object(ctx, ref.ID)
	if err != nil {
		return SlabMeta{}, fmt.Errorf("look up object %s: %w", ref, err)
	}
	sealed := object.Seal(c.appKey)
	encoded, err := json.Marshal(sealed)
	if err != nil {
		return SlabMeta{}, fmt.Errorf("encode slab metadata for %s: %w", ref, err)
	}
	return SlabMeta{Ref: ref, Bytes: encoded}, nil
}

// DownloadCached fetches a blob using previously captured metadata, skipping
// the indexer lookup. Callers must be prepared to fall back to Download when
// the metadata is too old to trust.
func (c *Client) DownloadCached(meta SlabMeta) ([]byte, error) {
	var sealed siastorage.SealedObject
	if err := json.Unmarshal(meta.Bytes, &sealed); err != nil {
		return nil, fmt.Errorf("decode slab metadata for %s: %w", meta.Ref, err)
	}
	object, err := sealed.Open(c.appKey)
	if err != nil {
		return nil, fmt.Errorf("open slab metadata for %s: %w", meta.Ref, err)
	}
	return c.read(&object, nil)
}
