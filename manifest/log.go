package manifest

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/steven3002/mnemosia/record"
	"github.com/steven3002/mnemosia/seal"
)

// A Log is the catalog's durable form: one sealed entry per line, appended
// never rewritten.
//
// Rewriting a whole catalog on every write costs time quadratic in the record
// count, and the bytes it churns are charged for. Appending a delta per flush
// is what keeps the catalog's cost proportional to what changed.
type Log struct {
	mu     sync.Mutex
	path   string
	file   *os.File
	sealer *Sealer
}

// A Sealer encrypts catalog entries. The catalog names every record the vault
// holds, so it is at least as sensitive as the records themselves.
type Sealer = seal.Sealer

// OpenLog opens or creates the log at path.
func OpenLog(path string, sealer *Sealer) (*Log, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open manifest log %s: %w", path, err)
	}
	return &Log{path: path, file: file, sealer: sealer}, nil
}

// Close releases the log file.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// Append writes one sealed entry and flushes it to disk.
func (l *Log) Append(entry Entry) error {
	plaintext, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode manifest entry %s: %w", entry.ID, err)
	}
	sealed, err := l.sealer.Seal(plaintext)
	if err != nil {
		return fmt.Errorf("seal manifest entry %s: %w", entry.ID, err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	line := base64.StdEncoding.EncodeToString(sealed) + "\n"
	if _, err := l.file.WriteString(line); err != nil {
		return fmt.Errorf("append manifest entry %s: %w", entry.ID, err)
	}
	if err := l.file.Sync(); err != nil {
		return fmt.Errorf("sync manifest log: %w", err)
	}
	return nil
}

// Replay reads the whole log, returning the current entry per record and the
// order records first appeared.
//
// A truncated final line is tolerated: an append interrupted by a crash leaves
// a partial record, and losing that one entry is correct where refusing to open
// the vault would not be.
func (l *Log) Replay() (map[record.ID]Entry, []record.ID, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, err := l.file.Seek(0, io.SeekStart); err != nil {
		return nil, nil, fmt.Errorf("rewind manifest log: %w", err)
	}
	entries := make(map[record.ID]Entry)
	var order []record.ID

	scanner := bufio.NewScanner(l.file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for line := 1; scanner.Scan(); line++ {
		text := scanner.Bytes()
		if len(text) == 0 {
			continue
		}
		sealed, err := base64.StdEncoding.DecodeString(string(text))
		if err != nil {
			return nil, nil, fmt.Errorf("manifest log %s line %d: %w", l.path, line, err)
		}
		plaintext, err := l.sealer.Open(sealed)
		if err != nil {
			return nil, nil, fmt.Errorf("manifest log %s line %d: %w", l.path, line, err)
		}
		var entry Entry
		if err := json.Unmarshal(plaintext, &entry); err != nil {
			return nil, nil, fmt.Errorf("manifest log %s line %d: %w", l.path, line, err)
		}
		if _, seen := entries[entry.ID]; !seen {
			order = append(order, entry.ID)
		}
		entries[entry.ID] = entry
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read manifest log %s: %w", l.path, err)
	}
	if _, err := l.file.Seek(0, io.SeekEnd); err != nil {
		return nil, nil, fmt.Errorf("seek to end of manifest log: %w", err)
	}
	return entries, order, nil
}

const maxLineBytes = 1 << 20
