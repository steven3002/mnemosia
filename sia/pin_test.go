package sia

import (
	"errors"
	"fmt"
	"testing"

	"go.sia.tech/indexd/slabs"
)

// The absent-slab tolerance has to survive the service interpolating the id.
//
// It did not, and the cost was specific: every repack whose old slabs the
// indexer had already released reported a failure after completing its work,
// and left behind a ledger entry that made every later sweep fail on the same
// slab. The wording below is the one the live indexer returned on
// 2026-08-07, the sentinel's own words with the id inserted between them.
func TestAnAlreadyReleasedSlabIsNotAFailure(t *testing.T) {
	t.Parallel()

	const id SlabID = "e190d709e126bc77f6a0a0e01533603452f14df5eb811382cbefa2febf367450"
	other := SlabID("f449da50887f1624990e83168c596b8c8af9c791d85cdd7ed1f2410b7180f8b9")

	cases := []struct {
		name string
		err  error
		gone bool
	}{
		{name: "no error is not an absence", err: nil},
		{
			name: "the sentinel as the package declares it",
			err:  slabs.ErrSlabNotFound,
			gone: true,
		},
		{
			name: "the sentinel with the id interpolated, as the service sends it",
			err:  fmt.Errorf("slab %s not found", id),
			gone: true,
		},
		{
			name: "wrapped the way the api client rebuilds it",
			err:  fmt.Errorf("unpin: %w", fmt.Errorf("slab %s not found", id)),
			gone: true,
		},
		{
			name: "an absence that names a different slab is not this one's",
			err:  fmt.Errorf("slab %s not found", other),
		},
		{
			name: "a transport-level absence is not a released slab",
			err:  errors.New("404 page not found"),
		},
		{
			name: "an ordinary failure is still a failure",
			err:  errors.New("unpin slab: connection reset by peer"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isSlabGone(tc.err, id); got != tc.gone {
				t.Fatalf("isSlabGone(%v) = %v, want %v", tc.err, got, tc.gone)
			}
		})
	}
}

// The same shape for objects, which reach the identical branch from the sweep.
func TestAnAlreadyDeletedObjectIsNotAFailure(t *testing.T) {
	t.Parallel()

	var ref ObjectRef
	ref.ID[0], ref.ID[31] = 0xab, 0xcd
	var other ObjectRef
	other.ID[0] = 0x11

	cases := []struct {
		name string
		err  error
		gone bool
	}{
		{name: "the sentinel as declared", err: slabs.ErrObjectNotFound, gone: true},
		{name: "with the id interpolated", err: fmt.Errorf("object %s not found", ref), gone: true},
		{name: "naming another object", err: fmt.Errorf("object %s not found", other)},
		{name: "an ordinary failure", err: errors.New("delete object: timeout")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isNotFound(tc.err, ref); got != tc.gone {
				t.Fatalf("isNotFound(%v) = %v, want %v", tc.err, got, tc.gone)
			}
		})
	}
}
