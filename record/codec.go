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
