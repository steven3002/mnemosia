package sia

import (
	"context"
	"fmt"
	"io"

	"go.sia.tech/siastorage"
)

// Download fetches a whole blob by object ref.
//
// It costs two round trips: one to the indexer to learn where the object lives,
// then the shard reads themselves. The first of the two dominates, which is why
// the layer above caches slab metadata rather than caching bodies alone.
func (c *Client) Download(ctx context.Context, ref ObjectRef) ([]byte, error) {
	object, err := c.sdk.Object(ctx, ref.ID)
	if err != nil {
		return nil, fmt.Errorf("look up object %s: %w", ref, err)
	}
	return c.read(&object, nil)
}

// DownloadRange fetches part of a blob. Reads are genuinely selective down to
// 64-byte leaves, so pulling one record out of a shared slab does not cost the
// slab.
func (c *Client) DownloadRange(ctx context.Context, ref ObjectRef, offset, length uint64) ([]byte, error) {
	object, err := c.sdk.Object(ctx, ref.ID)
	if err != nil {
		return nil, fmt.Errorf("look up object %s: %w", ref, err)
	}
	return c.read(&object, []siastorage.DownloadOption{siastorage.WithDownloadRange(offset, length)})
}

func (c *Client) read(object *siastorage.Object, opts []siastorage.DownloadOption) ([]byte, error) {
	reader, err := c.sdk.Download(*object, opts...)
	if err != nil {
		return nil, fmt.Errorf("open download: %w", err)
	}
	defer reader.Close()

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read object body: %w", err)
	}
	return body, nil
}
