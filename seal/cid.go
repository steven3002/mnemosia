package seal

import (
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/blake2b"
)

// CIDSize is the length of a content address in bytes.
const CIDSize = 32

// A CID is a content address for a record body.
//
// It is a keyed hash, never a plain digest of plaintext. An unkeyed content
// address lets anyone holding a candidate plaintext confirm that this vault
// stores it, which turns a private store into an oracle.
type CID [CIDSize]byte

func (c CID) String() string { return hex.EncodeToString(c[:]) }

// ParseCID reads the hex form produced by String.
func ParseCID(s string) (CID, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return CID{}, fmt.Errorf("parse cid %q: %w", s, err)
	}
	if len(b) != CIDSize {
		return CID{}, fmt.Errorf("parse cid %q: got %d bytes, want %d", s, len(b), CIDSize)
	}
	var cid CID
	copy(cid[:], b)
	return cid, nil
}

// MarshalText renders the address as hex so it round-trips through JSON as a
// string.
func (c CID) MarshalText() ([]byte, error) { return []byte(c.String()), nil }

// UnmarshalText reads the hex form.
func (c *CID) UnmarshalText(text []byte) error {
	parsed, err := ParseCID(string(text))
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}

// CID derives the keyed content address of a record body.
func (s *Sealer) CID(plaintext []byte) (CID, error) {
	h, err := blake2b.New256(s.content[:])
	if err != nil {
		return CID{}, fmt.Errorf("keyed blake2b: %w", err)
	}
	if _, err := h.Write(plaintext); err != nil {
		return CID{}, fmt.Errorf("hash record body: %w", err)
	}
	var cid CID
	copy(cid[:], h.Sum(nil))
	return cid, nil
}
