package local

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/steven3002/mnemosia/record"
)

// A CachedSlabMeta is an object's location together with when it was captured.
type CachedSlabMeta struct {
	ObjectRef string
	SlabID    string
	Meta      []byte
	FetchedAt time.Time
}

// Age reports how long ago the metadata was captured.
func (c CachedSlabMeta) Age() time.Duration { return time.Since(c.FetchedAt) }

// PutSlabMeta caches an object's location.
func (s *Store) PutSlabMeta(objectRef, slabID string, meta []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO slab_meta (object_ref, slab_id, meta, fetched_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(object_ref) DO UPDATE SET slab_id = excluded.slab_id, meta = excluded.meta, fetched_at = excluded.fetched_at`,
		objectRef, slabID, meta, record.Now().String())
	if err != nil {
		return fmt.Errorf("cache slab metadata for %s: %w", objectRef, err)
	}
	return nil
}

// GetSlabMeta reads cached location metadata.
//
// Callers must check Age and fall back to the indexer past their own limit.
// Serving an unbounded-age entry is not a slow path but a fatal one: the SDK's
// slab reader crashes the process when the cached host set has decayed too far.
func (s *Store) GetSlabMeta(objectRef string) (CachedSlabMeta, error) {
	var out CachedSlabMeta
	var fetchedAt string
	err := s.db.QueryRow(
		`SELECT object_ref, slab_id, meta, fetched_at FROM slab_meta WHERE object_ref = ?`, objectRef).
		Scan(&out.ObjectRef, &out.SlabID, &out.Meta, &fetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CachedSlabMeta{}, fmt.Errorf("slab metadata for %s: %w", objectRef, ErrNotFound)
	}
	if err != nil {
		return CachedSlabMeta{}, fmt.Errorf("read slab metadata for %s: %w", objectRef, err)
	}
	parsed, err := time.Parse(time.RFC3339, fetchedAt)
	if err != nil {
		return CachedSlabMeta{}, fmt.Errorf("slab metadata for %s has an unreadable timestamp %q: %w", objectRef, fetchedAt, err)
	}
	out.FetchedAt = parsed
	return out, nil
}
