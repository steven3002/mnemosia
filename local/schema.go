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
	// Vectors used to live here. They moved to a base-plus-delta pair of files
	// beside the catalog, for two reasons. One is write amplification, the same
	// reason the catalog is shaped that way. The other is that a row keyed to
	// bodies with a cascade meant that dropping this device's copy of a record,
	// which is how the lower read tiers are reached at all, silently deleted
	// its vector and made the record unsearchable.
	`DROP TABLE IF EXISTS vectors`,
	// The metadata ranking needs, held apart from the bodies it describes.
	//
	// Ranking reads this before it fetches anything: a boost has to be applied
	// while the candidate list is still being ordered, and by the time a body
	// has been fetched the ordering is already decided. Reading it from the
	// bodies would also make ranking depend on the device still holding them,
	// which it deliberately may not.
	//
	// Deliberately NOT keyed to bodies with a cascade. Dropping the local copy
	// of a record is how the lower read tiers are reached at all; the record is
	// still on the network and still has to rank.
	// kind separates the record classes that share this address space. It is
	// not a second tag: ranking uses it as an explicit scope a caller selects,
	// where type and tags are preferences a caller guesses at.
	`CREATE TABLE IF NOT EXISTS record_meta (
		record_id  TEXT PRIMARY KEY,
		kind       TEXT NOT NULL DEFAULT 'memory',
		type       TEXT NOT NULL,
		supersedes TEXT,
		created_at TEXT NOT NULL
	)`,
	// Which record each supersession points at, so the set of superseded
	// records is a lookup rather than a scan.
	`CREATE INDEX IF NOT EXISTS record_meta_supersedes ON record_meta(supersedes)`,
	// Tags one per row rather than a list in a column, because the two things
	// that read them want opposite shapes: ranking wants a record's tags, and
	// the write path wants a tag's frequency across the vault. A column of JSON
	// serves the first and makes the second a full scan.
	`CREATE TABLE IF NOT EXISTS record_tags (
		record_id TEXT NOT NULL,
		tag       TEXT NOT NULL,
		PRIMARY KEY (record_id, tag)
	)`,
	`CREATE INDEX IF NOT EXISTS record_tags_by_tag ON record_tags(tag)`,
	// The lexical half of ranking: one full-text index over the terms of every
	// record, scored with BM25 and fused with the vector pass.
	//
	// It holds the same plaintext the bodies table already holds, so it widens
	// nothing: this file is the one place plaintext lives, and whatever protects
	// the working copy protects this. What it adds is a second way to reach a
	// record, by the words it uses rather than by what it means, which is the
	// half a vector pass cannot do.
	//
	// record_id is UNINDEXED because it is carried, not searched: matching
	// happens on terms alone, and an indexed id column would put record handles
	// into the term dictionary.
	`CREATE VIRTUAL TABLE IF NOT EXISTS record_lexical USING fts5(
		record_id UNINDEXED,
		terms
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
	//
	// origin separates the two ways a slab reaches this list, because only one
	// of them confers the right to release it. A slab this installation pinned
	// is its to release; a slab it merely found by hydrating is another
	// device's, and unpinning it would take away storage that device is still
	// pointing at.
	`CREATE TABLE IF NOT EXISTS slabs (
		slab_id   TEXT PRIMARY KEY,
		pinned_at TEXT NOT NULL,
		records   INTEGER NOT NULL DEFAULT 0,
		bytes     INTEGER NOT NULL DEFAULT 0,
		origin    TEXT NOT NULL DEFAULT 'pinned'
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
	// Session heads, held apart from the bodies table because the two are read
	// in completely different ways. A body is fetched one at a time by id; a
	// head is listed a thousand at a time, sorted and filtered, and the whole
	// point of the session shape is that listing never touches a transcript.
	// The listing columns are therefore denormalised out of the head so that a
	// list reads them and nothing else.
	//
	// A head is the one record that changes after it is written, which is why
	// it lives here and not in a slab: slabs hold immutable content, and a
	// record rewritten on every turn would strand a copy of itself in one on
	// every write.
	`CREATE TABLE IF NOT EXISTS session_heads (
		session_id  TEXT PRIMARY KEY,
		version     INTEGER NOT NULL,
		title       TEXT NOT NULL,
		summary     TEXT NOT NULL DEFAULT '',
		kind        TEXT NOT NULL,
		project     TEXT NOT NULL DEFAULT '',
		agent       TEXT NOT NULL DEFAULT '',
		parent      TEXT,
		created_at  TEXT NOT NULL,
		updated_at  TEXT NOT NULL,
		archived    INTEGER NOT NULL DEFAULT 0,
		messages    INTEGER NOT NULL DEFAULT 0,
		chunks      INTEGER NOT NULL DEFAULT 0,
		bytes       INTEGER NOT NULL DEFAULT 0,
		head        BLOB NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS session_heads_by_updated ON session_heads(updated_at DESC)`,
	// Sub-agent runs are found from their parent rather than by scanning, since
	// loading a session with its children is the ordinary case and a scan of
	// every session in the vault is not.
	`CREATE INDEX IF NOT EXISTS session_heads_by_parent ON session_heads(parent)`,
	`CREATE TABLE IF NOT EXISTS meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
}

// columnAdditions are columns added to tables that existed before them.
//
// SQLite has no "add column if absent", and a vault written by an earlier build
// is a file on somebody's disk rather than something that can be recreated, so
// the presence of each one is checked before it is added.
var columnAdditions = []struct{ table, column, definition string }{
	{"record_meta", "kind", "TEXT NOT NULL DEFAULT 'memory'"},
	// A ledger written before this column existed predates hydration filing
	// anything into it, so every row in one was pinned by that installation.
	// The default says so; it is the only reading that does not silently strip
	// an existing vault of the right to reclaim its own storage.
	{"slabs", "origin", "TEXT NOT NULL DEFAULT 'pinned'"},
}

func (s *Store) migrate() error {
	for i, stmt := range schema {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("statement %d: %w", i+1, err)
		}
	}
	for _, addition := range columnAdditions {
		present, err := s.hasColumn(addition.table, addition.column)
		if err != nil {
			return err
		}
		if present {
			continue
		}
		if _, err := s.db.Exec(
			`ALTER TABLE ` + addition.table + ` ADD COLUMN ` + addition.column + ` ` + addition.definition); err != nil {
			return fmt.Errorf("add %s.%s: %w", addition.table, addition.column, err)
		}
	}
	return nil
}

func (s *Store) hasColumn(table, column string) (bool, error) {
	rows, err := s.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, fmt.Errorf("inspect %s: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
