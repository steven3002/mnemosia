// Package local persists the on-device working copy and read caches.
package local

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	// Registers the "sqlite" driver Open asks for; pure Go, so no cgo.
	_ "modernc.org/sqlite"
)

// A Store is the device's copy of the vault, held in SQLite.
//
// This is the one place plaintext lives. The network sees ciphertext; the
// device sees everything, because search, ranking and decryption all happen
// here and nothing about the design would work if they did not.
type Store struct {
	db *sql.DB
}

// Open prepares the database at path, creating it if absent.
//
// Every MCP client launches its own process against the same vault, so
// concurrent access is a requirement of the first commit rather than a
// hardening step: WAL lets readers and a writer proceed at once, and a busy
// timeout absorbs the writer-versus-writer case.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	store := &Store{db: db}
	if err := store.ensureWAL(); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL on %s: %w", path, err)
	}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("prepare schema in %s: %w", path, err)
	}
	return store, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// dsn carries only per-connection settings.
//
// journal_mode is deliberately absent: database/sql replays these pragmas on
// every connection it opens, and switching a database to WAL needs a lock that
// a second process already holds, so a DSN-level journal_mode turns a
// concurrent open into an outright failure. WAL is a property of the file, set
// once and inherited thereafter.
func dsn(path string) string {
	return path + "?" + strings.Join([]string{
		"_pragma=busy_timeout(10000)",
		"_pragma=synchronous(NORMAL)",
		"_pragma=foreign_keys(ON)",
	}, "&")
}

func (s *Store) ensureWAL() error {
	var mode string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		return err
	}
	if strings.EqualFold(mode, "wal") {
		return nil
	}
	// Another process may hold the lock this needs; it is released quickly, so
	// retrying beats failing an otherwise valid open.
	var last error
	for attempt := range walRetries {
		if err := s.db.QueryRow(`PRAGMA journal_mode = WAL`).Scan(&mode); err == nil {
			return nil
		} else {
			last = err
		}
		time.Sleep(time.Duration(attempt+1) * walRetryStep)
	}
	return last
}

const (
	walRetries   = 20
	walRetryStep = 25 * time.Millisecond
)
