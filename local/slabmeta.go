package local

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// A CachedLocation is an object's own location metadata with the time it was
// captured.
type CachedLocation struct {
	ObjectRef string
	SlabIDs   []string
	Meta      []byte
	FetchedAt time.Time
}

// Age reports how long ago the metadata was captured.
func (c CachedLocation) Age() time.Duration { return time.Since(c.FetchedAt) }

// A CachedSlab is one slab's sector list with the time it was captured.
type CachedSlab struct {
	SlabID    string
	Meta      []byte
	FetchedAt time.Time
}

// Age reports how long ago the metadata was captured.
func (c CachedSlab) Age() time.Duration { return time.Since(c.FetchedAt) }

// PutLocation caches an object's own location metadata.
func (s *Store) PutLocation(objectRef string, slabIDs []string, meta []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO object_cache (object_ref, slab_ids, meta, fetched_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(object_ref) DO UPDATE SET
		     slab_ids = excluded.slab_ids, meta = excluded.meta, fetched_at = excluded.fetched_at`,
		objectRef, strings.Join(slabIDs, ","), meta, stamp(time.Now()))
	if err != nil {
		return fmt.Errorf("cache location for %s: %w", objectRef, err)
	}
	return nil
}

// GetLocation reads an object's cached location metadata.
//
// Callers must check Age and fall back to the indexer past their own limit.
// Serving an unbounded-age entry is not a slow path but a fatal one: the SDK's
// slab reader crashes the process when the cached host set has decayed too far.
func (s *Store) GetLocation(objectRef string) (CachedLocation, error) {
	var (
		out     CachedLocation
		slabIDs string
		fetched string
	)
	err := s.db.QueryRow(
		`SELECT object_ref, slab_ids, meta, fetched_at FROM object_cache WHERE object_ref = ?`, objectRef).
		Scan(&out.ObjectRef, &slabIDs, &out.Meta, &fetched)
	if errors.Is(err, sql.ErrNoRows) {
		return CachedLocation{}, fmt.Errorf("location for %s: %w", objectRef, ErrNotFound)
	}
	if err != nil {
		return CachedLocation{}, fmt.Errorf("read location for %s: %w", objectRef, err)
	}
	if out.FetchedAt, err = time.Parse(time.RFC3339, fetched); err != nil {
		return CachedLocation{}, fmt.Errorf("location for %s has an unreadable timestamp %q: %w", objectRef, fetched, err)
	}
	if slabIDs != "" {
		out.SlabIDs = strings.Split(slabIDs, ",")
	}
	return out, nil
}

// PutSlabMeta caches one slab's sector list, shared by every object in it.
func (s *Store) PutSlabMeta(slabID string, meta []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO slab_cache (slab_id, meta, fetched_at) VALUES (?, ?, ?)
		 ON CONFLICT(slab_id) DO UPDATE SET meta = excluded.meta, fetched_at = excluded.fetched_at`,
		slabID, meta, stamp(time.Now()))
	if err != nil {
		return fmt.Errorf("cache slab %s: %w", slabID, err)
	}
	return nil
}

// GetSlabMeta reads one slab's cached sector list.
func (s *Store) GetSlabMeta(slabID string) (CachedSlab, error) {
	var (
		out     CachedSlab
		fetched string
	)
	err := s.db.QueryRow(
		`SELECT slab_id, meta, fetched_at FROM slab_cache WHERE slab_id = ?`, slabID).
		Scan(&out.SlabID, &out.Meta, &fetched)
	if errors.Is(err, sql.ErrNoRows) {
		return CachedSlab{}, fmt.Errorf("slab metadata for %s: %w", slabID, ErrNotFound)
	}
	if err != nil {
		return CachedSlab{}, fmt.Errorf("read slab metadata for %s: %w", slabID, err)
	}
	if out.FetchedAt, err = time.Parse(time.RFC3339, fetched); err != nil {
		return CachedSlab{}, fmt.Errorf("slab %s has an unreadable timestamp %q: %w", slabID, fetched, err)
	}
	return out, nil
}

// ForgetLocation drops an object's cached location, so the next read asks the
// indexer instead of trusting metadata that has already failed once.
func (s *Store) ForgetLocation(objectRef string) error {
	if _, err := s.db.Exec(`DELETE FROM object_cache WHERE object_ref = ?`, objectRef); err != nil {
		return fmt.Errorf("forget location for %s: %w", objectRef, err)
	}
	return nil
}

// ForgetSlabMeta drops a slab's cached sector list. Every object over that slab
// falls back to the indexer on its next read.
func (s *Store) ForgetSlabMeta(slabID string) error {
	if _, err := s.db.Exec(`DELETE FROM slab_cache WHERE slab_id = ?`, slabID); err != nil {
		return fmt.Errorf("forget slab metadata for %s: %w", slabID, err)
	}
	return nil
}

// A CacheSize reports what the location cache costs on disk.
type CacheSize struct {
	Objects, Slabs         int
	ObjectBytes, SlabBytes int64
}

// Total is the whole cache.
func (c CacheSize) Total() int64 { return c.ObjectBytes + c.SlabBytes }

// PerObject is what the cache costs for each object it can locate, which is
// the figure that decides whether caching locations is affordable at all.
func (c CacheSize) PerObject() float64 {
	if c.Objects == 0 {
		return 0
	}
	return float64(c.Total()) / float64(c.Objects)
}

// CacheSize measures the location cache.
func (s *Store) CacheSize() (CacheSize, error) {
	var out CacheSize
	var objectBytes, slabBytes sql.NullInt64

	if err := s.db.QueryRow(`SELECT COUNT(*), SUM(LENGTH(meta) + LENGTH(slab_ids)) FROM object_cache`).
		Scan(&out.Objects, &objectBytes); err != nil {
		return CacheSize{}, fmt.Errorf("measure object cache: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*), SUM(LENGTH(meta)) FROM slab_cache`).
		Scan(&out.Slabs, &slabBytes); err != nil {
		return CacheSize{}, fmt.Errorf("measure slab cache: %w", err)
	}
	out.ObjectBytes, out.SlabBytes = objectBytes.Int64, slabBytes.Int64
	return out, nil
}
