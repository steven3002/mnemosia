package manifest

import (
	"errors"
	"fmt"
	"sync"

	"github.com/steven3002/mnemosia/record"
)

// ErrNotFound reports that the catalog holds no entry for a record.
var ErrNotFound = errors.New("no manifest entry")

// A Manifest is the catalog of record ids to object locations.
//
// It is a performance index, not a correctness requirement: every stored blob
// carries its own record identity, so a lost catalog costs a rebuild rather
// than the data. That is what allows the on-disk form to be an append-only log
// instead of a structure that must be rewritten to stay correct.
type Manifest struct {
	mu      sync.RWMutex
	log     *Log
	entries map[record.ID]Entry
	order   []record.ID
}

// Load opens the catalog at path and replays it.
func Load(log *Log) (*Manifest, error) {
	entries, order, err := log.Replay()
	if err != nil {
		return nil, err
	}
	return &Manifest{log: log, entries: entries, order: order}, nil
}

// Append records where a record version landed.
func (m *Manifest) Append(entry Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.log.Append(entry); err != nil {
		return err
	}
	if _, seen := m.entries[entry.ID]; !seen {
		m.order = append(m.order, entry.ID)
	}
	m.entries[entry.ID] = entry
	return nil
}

// Lookup resolves a record id to its location.
func (m *Manifest) Lookup(id record.ID) (Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.entries[id]
	if !ok {
		return Entry{}, fmt.Errorf("%w for %s", ErrNotFound, id)
	}
	return entry, nil
}

// Entries lists every catalogued record in the order it was first written.
func (m *Manifest) Entries() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Entry, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.entries[id])
	}
	return out
}

// Len reports how many records the catalog holds.
func (m *Manifest) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

// Close releases the underlying log.
func (m *Manifest) Close() error { return m.log.Close() }
