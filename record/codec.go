package record

import (
	"encoding/json"
	"fmt"
)

// Marshal renders a memory as the canonical stored form.
//
// The stored form is JSON because tolerant readers and additive-only evolution
// are schema requirements, and a self-describing encoding is what makes an
// unknown field survive a round trip through an older build.
func Marshal(m *Memory) ([]byte, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal memory %s: %w", m.ID, err)
	}
	return b, nil
}

// Unmarshal reads the stored form. Unknown fields are ignored rather than
// rejected, so a record written by a newer build stays readable.
func Unmarshal(b []byte) (*Memory, error) {
	var m Memory
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("unmarshal memory: %w", err)
	}
	return &m, nil
}

// MarshalSession renders a session head as the canonical stored form.
func MarshalSession(s *Session) ([]byte, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("marshal session %s: %w", s.ID, err)
	}
	return b, nil
}

// UnmarshalSession reads a stored session head.
func UnmarshalSession(b []byte) (*Session, error) {
	var s Session
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &s, nil
}

// MarshalChunk renders a transcript chunk as the canonical stored form.
func MarshalChunk(c *Chunk) ([]byte, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("marshal chunk %s: %w", c.ID, err)
	}
	return b, nil
}

// UnmarshalChunk reads a stored transcript chunk.
func UnmarshalChunk(b []byte) (*Chunk, error) {
	var c Chunk
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("unmarshal chunk: %w", err)
	}
	return &c, nil
}
