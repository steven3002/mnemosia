package record

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// IDSize is the length of a record id in bytes.
const IDSize = 16

// An ID identifies one immutable record version.
//
// It is deliberately independent of any storage address. Repack rewrites every
// Sia object id, so a record id is the only identifier that survives it, and
// therefore the only safe key for merging two devices or for linking records to
// one another.
type ID [IDSize]byte

// NewID returns a fresh random record id.
func NewID() (ID, error) {
	var id ID
	if _, err := rand.Read(id[:]); err != nil {
		return ID{}, fmt.Errorf("generate record id: %w", err)
	}
	return id, nil
}

func (id ID) String() string { return hex.EncodeToString(id[:]) }

// IsZero reports whether the id is unset.
func (id ID) IsZero() bool { return id == ID{} }

// ParseID reads the hex form produced by String.
func ParseID(s string) (ID, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return ID{}, fmt.Errorf("parse record id %q: %w", s, err)
	}
	if len(b) != IDSize {
		return ID{}, fmt.Errorf("parse record id %q: got %d bytes, want %d", s, len(b), IDSize)
	}
	var id ID
	copy(id[:], b)
	return id, nil
}

// MarshalText renders the id as hex so it round-trips through JSON as a string.
func (id ID) MarshalText() ([]byte, error) { return []byte(id.String()), nil }

// UnmarshalText reads the hex form.
func (id *ID) UnmarshalText(text []byte) error {
	parsed, err := ParseID(string(text))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}
