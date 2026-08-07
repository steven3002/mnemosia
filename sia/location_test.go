package sia

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"go.sia.tech/core/types"
	"go.sia.tech/indexd/slabs"
)

// slabWithHosts builds a slab slice whose sectors sit on the given hosts.
func slabWithHosts(minShards uint, hostKeys []types.PublicKey) slabs.SlabSlice {
	sectors := make([]slabs.PinnedSector, len(hostKeys))
	for i, key := range hostKeys {
		sectors[i] = slabs.PinnedSector{Root: types.Hash256{byte(i)}, HostKey: key}
	}
	return slabs.SlabSlice{MinShards: minShards, Sectors: sectors, Offset: 0, Length: 1024}
}

func hostKeys(n int) []types.PublicKey {
	out := make([]types.PublicKey, n)
	for i := range out {
		out[i] = types.PublicKey{byte(i + 1)}
	}
	return out
}

// clientKnowing builds a client whose host set is already populated, so the
// reachability check can be exercised without a network.
func clientKnowing(keys []types.PublicKey) *Client {
	known := make(map[types.PublicKey]struct{}, len(keys))
	for _, key := range keys {
		known[key] = struct{}{}
	}
	return &Client{hosts: known, hostsAt: time.Now()}
}

// The check exists because of a specific crash, not as general hygiene: the
// SDK's slab reader counts sectors, then drops the ones on hosts it does not
// recognise, then takes the first MinShards of what is left without looking.
// When too few survive it indexes an empty slice, in a goroutine the caller
// cannot recover from, and the process dies. So the question that has to be
// answered before handing it cached metadata is how many sectors are on hosts
// the network still lists.
func TestReachabilityIsCountedOnHostsTheNetworkStillLists(t *testing.T) {
	const total, minShards = 30, 10
	all := hostKeys(total)

	for _, tc := range []struct {
		name      string
		reachable int
		stale     bool
	}{
		{name: "every host still listed", reachable: total},
		{name: "more than enough survive", reachable: minShards + 1},
		{name: "exactly enough survive", reachable: minShards},
		{name: "one short", reachable: minShards - 1, stale: true},
		{name: "nothing survives", reachable: 0, stale: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := clientKnowing(all[:tc.reachable])
			err := client.checkReachable(t.Context(), []slabs.SlabSlice{slabWithHosts(minShards, all)})

			if tc.stale && !errors.Is(err, ErrStaleLocation) {
				t.Fatalf("%d of %d sectors reachable against %d shards returned %v, want a stale location",
					tc.reachable, total, minShards, err)
			}
			if !tc.stale && err != nil {
				t.Fatalf("%d of %d sectors reachable against %d shards was rejected: %v",
					tc.reachable, total, minShards, err)
			}
		})
	}
}

// Metadata that is malformed rather than merely decayed must be caught by the
// same door, since the reader treats it the same way.
func TestMalformedSlabMetadataIsStale(t *testing.T) {
	all := hostKeys(30)
	client := clientKnowing(all)

	for _, tc := range []struct {
		name string
		slab slabs.SlabSlice
	}{
		{name: "no shards declared", slab: slabWithHosts(0, all)},
		{name: "fewer sectors than shards", slab: slabWithHosts(10, all[:4])},
		{name: "no sectors at all", slab: slabWithHosts(10, nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := client.checkReachable(t.Context(), []slabs.SlabSlice{tc.slab}); !errors.Is(err, ErrStaleLocation) {
				t.Fatalf("got %v, want a stale location", err)
			}
		})
	}
}

// Splitting a location and putting it back together must reproduce the
// original exactly. The object's identity is derived from the slabs it points
// at, so anything less would fail to open rather than read the wrong bytes,
// but a cache that never works is not much better than no cache.
func TestLocationSplitsAndRejoins(t *testing.T) {
	all := hostKeys(30)
	original := slabs.SealedObject{
		EncryptedDataKey:  []byte("an encrypted data key"),
		DataSignature:     types.Signature{1, 2, 3},
		MetadataSignature: types.Signature{4, 5, 6},
		CreatedAt:         time.Now().UTC().Truncate(time.Second),
		UpdatedAt:         time.Now().UTC().Truncate(time.Second),
		Slabs:             []slabs.SlabSlice{slabWithHosts(10, all)},
	}
	original.Slabs[0].Offset, original.Slabs[0].Length = 4096, 897

	location, shared, err := splitLocation(ObjectRef{ID: types.Hash256{9}}, original)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(shared) != 1 {
		t.Fatalf("split produced %d slab entries for one slab", len(shared))
	}

	byID := map[SlabID][]byte{shared[0].ID: shared[0].Bytes}
	rejoined, err := joinLocation(location, byID)
	if err != nil {
		t.Fatalf("rejoin: %v", err)
	}

	if !reflect.DeepEqual(original, rejoined) {
		t.Fatalf("a location did not survive the round trip:\n want %+v\n  got %+v", original, rejoined)
	}
}

// The whole reason for splitting: the sector list is the bulk of the metadata
// and every object in a flush has the same one. Keeping it per object is what
// turns a location cache into something larger than the data it locates.
func TestSplittingLocationsIsWhatMakesTheCacheAffordable(t *testing.T) {
	all := hostKeys(30)
	slab := slabWithHosts(10, all)

	const objects = 1000
	var naive, object, shared int

	for i := range objects {
		sealed := slabs.SealedObject{
			EncryptedDataKey:  make([]byte, 60),
			DataSignature:     types.Signature{byte(i)},
			MetadataSignature: types.Signature{byte(i)},
			Slabs:             []slabs.SlabSlice{slab},
		}
		whole, err := json.Marshal(sealed)
		if err != nil {
			t.Fatalf("encode object %d: %v", i, err)
		}
		naive += len(whole)

		location, slabs, err := splitLocation(ObjectRef{ID: types.Hash256{byte(i)}}, sealed)
		if err != nil {
			t.Fatalf("split object %d: %v", i, err)
		}
		object += len(location.Bytes)
		if i == 0 {
			for _, entry := range slabs {
				shared += len(entry.Bytes)
			}
		}
	}

	deduped := object + shared
	t.Logf("%d objects over one slab: %d B cached naively (%d B/object), %d B split (%.0f B/object), %.0fx smaller",
		objects, naive, naive/objects, deduped, float64(deduped)/objects, float64(naive)/float64(deduped))

	// What is left per object is almost entirely irreducible: two signatures of
	// 64 bytes and an encrypted data key, all of which the SDK verifies when it
	// reopens the object, plus the byte range this object occupies. There is no
	// version of this cache that is much smaller while still producing an object
	// the SDK will open.
	const floor = 60 + 64 + 64 + 40
	perObject := float64(deduped) / objects
	if perObject > 2*floor {
		t.Fatalf("the split cache costs %.0f B per object against a floor of about %d", perObject, floor)
	}
	if naive < 10*deduped {
		t.Fatalf("splitting saved only %.1fx, which does not justify it", float64(naive)/float64(deduped))
	}
}
