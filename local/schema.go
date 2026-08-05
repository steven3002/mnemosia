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
	// Location metadata is split in two, because the two halves have wildly
	// different multiplicities. The sector list and coding parameters are
	// identical for every object packed into one slab and run to several
	// kilobytes; the object's own key, signatures and byte ranges are a couple
	// of hundred bytes and unique. Storing the whole thing per object would
	// cost more cache than the data it describes.
	`DROP TABLE IF EXISTS slab_meta`,
	`CREATE TABLE IF NOT EXISTS slab_cache (
		slab_id    TEXT PRIMARY KEY,
		meta       BLOB NOT NULL,
		fetched_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS object_cache (
		object_ref TEXT PRIMARY KEY,
		slab_ids   TEXT NOT NULL,
		meta       BLOB NOT NULL,
		fetched_at TEXT NOT NULL
	)`,
	// Reads counted by the tier that served them. Hit rates are a property of
	// how a vault is used, so they cannot be derived after the fact from
	// anything else; they have to be counted as they happen, and they have to
	// survive the process that counted them.
	`CREATE TABLE IF NOT EXISTS read_stats (
		tier   TEXT PRIMARY KEY,
		reads  INTEGER NOT NULL DEFAULT 0,
		nanos  INTEGER NOT NULL DEFAULT 0,
		misses INTEGER NOT NULL DEFAULT 0
	)`,
	// Slabs this vault has pinned. Without this list nothing can ever be
	// released: the indexer bills a whole slab whether it holds live records or
	// none, and no other party knows which slabs are ours.
	`CREATE TABLE IF NOT EXISTS slabs (
		slab_id   TEXT PRIMARY KEY,
		pinned_at TEXT NOT NULL,
		records   INTEGER NOT NULL DEFAULT 0,
		bytes     INTEGER NOT NULL DEFAULT 0
	)`,
	// Records sealed and waiting for a flush. Holding the queue here rather
	// than in memory is what closes the window in which a record the user was
	// told was saved exists nowhere but a process that is about to exit.
	//
	// A claim marks a batch as being written by one process. It is timestamped
	// so that a process which dies mid-flush releases its work by expiry rather
	// than holding it forever, and it is taken in one statement so two
	// processes over the same vault cannot both pay for the same slab.
	`CREATE TABLE IF NOT EXISTS queue (
		record_id  TEXT PRIMARY KEY,
		kind       TEXT NOT NULL,
		cid        TEXT NOT NULL,
		payload    BLOB NOT NULL,
		queued_at  TEXT NOT NULL,
		seq        INTEGER NOT NULL,
		claimed_at TEXT,
		claimed_by TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS queue_by_seq ON queue(seq)`,
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
