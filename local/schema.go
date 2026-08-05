package local

import "fmt"

// schema is applied on every open. Each statement is idempotent so two
// processes racing to open a fresh vault both succeed.
var schema = []string{
	`CREATE TABLE IF NOT EXISTS bodies (
		record_id  TEXT PRIMARY KEY,
		kind       TEXT NOT NULL,
		body       BLOB NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS vectors (
		record_id TEXT PRIMARY KEY REFERENCES bodies(record_id) ON DELETE CASCADE,
		model     TEXT NOT NULL,
		dim       INTEGER NOT NULL,
		vector    BLOB NOT NULL
	)`,
	// Location metadata is keyed by slab, not by object: every object in a
	// packed flush shares one sector list, so keying by object would store the
	// same few kilobytes once per record.
	`CREATE TABLE IF NOT EXISTS slab_meta (
		object_ref TEXT PRIMARY KEY,
		slab_id    TEXT NOT NULL,
		meta       BLOB NOT NULL,
		fetched_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS slab_meta_by_slab ON slab_meta(slab_id)`,
	// Slabs this vault has pinned. Without this list nothing can ever be
	// released: the indexer bills a whole slab whether it holds live records or
	// none, and no other party knows which slabs are ours.
	`CREATE TABLE IF NOT EXISTS slabs (
		slab_id   TEXT PRIMARY KEY,
		pinned_at TEXT NOT NULL,
		records   INTEGER NOT NULL DEFAULT 0,
		bytes     INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE IF NOT EXISTS meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
}

func (s *Store) migrate() error {
	for i, stmt := range schema {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("statement %d: %w", i+1, err)
		}
	}
	return nil
}
