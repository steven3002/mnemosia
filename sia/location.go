package sia

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.sia.tech/core/types"
	"go.sia.tech/indexd/api"
	"go.sia.tech/indexd/slabs"
	"go.sia.tech/siastorage"
)

// ErrStaleLocation reports that cached location metadata can no longer be
// trusted and the indexer must be asked again.
var ErrStaleLocation = errors.New("cached object location is stale")

// A SlabLocation is the part of an object's location that every object packed
// into the same slab shares: the coding parameters and the sector list.
//
// It is the bulk of the metadata — thirty sectors of root and host key — and
// keeping one copy per slab rather than one per object is the difference
// between a cache that grows with records and one that grows with flushes.
type SlabLocation struct {
	ID    SlabID
	Bytes []byte
}

// A Location is where one object's bytes live, minus everything its slab-mates
// already hold. It is a few hundred bytes: the object's own key, its
// signatures, and the ranges it occupies.
type Location struct {
	Ref   ObjectRef
	Slabs []SlabID
	Bytes []byte
}

// LocationFor asks the indexer where an object lives and splits the answer
// into the part unique to the object and the parts shared with its slab.
func (c *Client) LocationFor(ctx context.Context, ref ObjectRef) (Location, []SlabLocation, error) {
	sealed, err := c.app.Object(ctx, c.appKey, ref.ID)
	if err != nil {
		return Location{}, nil, fmt.Errorf("look up object %s: %w", ref, err)
	}
	return splitLocation(ref, sealed)
}

func splitLocation(ref ObjectRef, sealed slabs.SealedObject) (Location, []SlabLocation, error) {
	location := Location{
		Ref:   ref,
		Slabs: make([]SlabID, 0, len(sealed.Slabs)),
		Bytes: encodeObject(sealed),
	}
	shared := make([]SlabLocation, 0, len(sealed.Slabs))
	for _, slice := range sealed.Slabs {
		id := SlabID(slice.Digest().String())
		location.Slabs = append(location.Slabs, id)
		shared = append(shared, SlabLocation{ID: id, Bytes: encodeSlab(slice)})
	}
	return location, shared, nil
}

// DownloadAt fetches a blob from cached location metadata, skipping the
// indexer lookup that otherwise dominates a read.
//
// The reassembled metadata is verified before use, twice over. Structurally,
// because the SDK's slab reader checks its bounds before it filters unusable
// hosts and then indexes the empty result from a goroutine the caller cannot
// recover — a sufficiently decayed sector list does not slow a read down, it
// ends the process. Cryptographically, because an object's identity is derived
// from the slabs it points at, so a mismatched reassembly fails to open rather
// than quietly reading the wrong bytes.
//
// A cache that cannot be trusted reports ErrStaleLocation, and the caller asks
// the indexer again.
func (c *Client) DownloadAt(ctx context.Context, location Location, shared map[SlabID][]byte) ([]byte, error) {
	sealed, err := joinLocation(location, shared)
	if err != nil {
		return nil, err
	}
	if err := c.checkReachable(ctx, sealed.Slabs); err != nil {
		return nil, err
	}
	wrapped := siastorage.SealedObject{SealedObject: sealed}
	object, err := wrapped.Open(c.appKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStaleLocation, err)
	}
	return c.read(&object, nil)
}

func joinLocation(location Location, shared map[SlabID][]byte) (slabs.SealedObject, error) {
	if len(location.Bytes) == 0 {
		return slabs.SealedObject{}, fmt.Errorf("%w: nothing cached for %s", ErrStaleLocation, location.Ref)
	}
	sealed, ids, err := decodeObject(location.Bytes)
	if err != nil {
		return slabs.SealedObject{}, err
	}

	for i, id := range ids {
		encoded, ok := shared[id]
		if !ok {
			return slabs.SealedObject{}, fmt.Errorf("%w: no cached metadata for slab %s", ErrStaleLocation, id)
		}
		slice, err := decodeSlab(encoded)
		if err != nil {
			return slabs.SealedObject{}, fmt.Errorf("slab %s: %w", id, err)
		}
		// The offset and length belong to this object; everything else in the
		// slice is shared with every other object over the same slab.
		slice.Offset, slice.Length = sealed.Slabs[i].Offset, sealed.Slabs[i].Length
		sealed.Slabs[i] = slice
	}
	return sealed, nil
}

// checkReachable refuses metadata the SDK's slab reader would crash on.
//
// The reader drops hosts it does not recognise and then takes the first
// MinShards of what is left without checking that there are that many. So the
// question that has to be answered here is not how many sectors the slab has
// but how many of them sit on hosts the network still knows about.
func (c *Client) checkReachable(ctx context.Context, slices []slabs.SlabSlice) error {
	known, err := c.knownHosts(ctx)
	if err != nil {
		return err
	}
	for i, slice := range slices {
		if slice.MinShards == 0 || len(slice.Sectors) < int(slice.MinShards) {
			return fmt.Errorf("%w: slab %d has %d sectors for %d shards",
				ErrStaleLocation, i, len(slice.Sectors), slice.MinShards)
		}
		var reachable int
		for _, sector := range slice.Sectors {
			if _, ok := known[sector.HostKey]; ok {
				reachable++
			}
		}
		if reachable < int(slice.MinShards) {
			return fmt.Errorf("%w: slab %d has %d of %d shards on hosts the network still lists",
				ErrStaleLocation, i, reachable, slice.MinShards)
		}
	}
	return nil
}

// HostSetTTL is how long a fetched host set is reused.
//
// The set is account-wide rather than per-object, so one fetch covers every
// cached read in the window and the round trip the cache exists to avoid is
// still avoided.
const HostSetTTL = 5 * time.Minute

func (c *Client) knownHosts(ctx context.Context) (map[types.PublicKey]struct{}, error) {
	c.hostsMu.Lock()
	defer c.hostsMu.Unlock()

	if c.hosts != nil && time.Since(c.hostsAt) < HostSetTTL {
		return c.hosts, nil
	}

	const page = 100
	known := make(map[types.PublicKey]struct{})
	for offset := 0; ; offset += page {
		batch, err := c.app.Hosts(ctx, c.appKey, api.WithOffset(offset), api.WithLimit(page))
		if err != nil {
			return nil, fmt.Errorf("read host set: %w", err)
		}
		for _, host := range batch {
			known[host.PublicKey] = struct{}{}
		}
		if len(batch) < page {
			break
		}
	}
	c.hosts, c.hostsAt = known, time.Now()
	return known, nil
}
