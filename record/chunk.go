package record

import (
	"encoding/json"
	"fmt"
)

// ChunkTargetBytes is how large a chunk is allowed to grow before another is
// started.
//
// It is a compromise between two costs that pull in opposite directions. A
// chunk is the unit a reader fetches, so small chunks make a partial read cheap
// and a whole-transcript read a great many fetches; a chunk is also a record in
// a packed slab, so large chunks amortise the per-record overhead and make the
// head's chunk list short. A quarter of a mebibyte sits where a long
// conversation is a few dozen chunks rather than thousands.
const ChunkTargetBytes = 256 << 10

// A Chunk is an immutable, ordered slice of a session's transcript.
//
// Chunks are the only place messages live. They are content-addressed and never
// rewritten: appending to a session writes new chunks and leaves every earlier
// one exactly as it was, which is what makes the cost of a turn proportional to
// the turn instead of to the conversation.
//
// A chunk is never uploaded on its own. A slab is billed whole whatever it
// holds, so a lone quarter-mebibyte object costs a hundred and sixty times its
// own size; chunks ride the same packed batch as every other record.
type Chunk struct {
	ID            ID     `json:"id"`
	Kind          Kind   `json:"kind"`
	Schema        string `json:"schema"`
	SchemaVersion int    `json:"schemaVersion"`
	// Session is the head this chunk belongs to, by record id, so a chunk
	// recovered from storage alone can be attributed without the head.
	Session ID `json:"session"`
	// Seq is the chunk's position in the transcript.
	Seq      int       `json:"seq"`
	Messages []Message `json:"messages"`
}

// A ChunkRef is what the head holds about one chunk: enough to decide whether
// to fetch it, and nothing that requires fetching it.
//
// It names the chunk by record id and not by object ref. Object refs are a
// mutable pointer, a repack rewrites every one of them, so a transcript keyed
// on storage location would break the first time storage was reorganised.
type ChunkRef struct {
	ID  ID  `json:"id"`
	Seq int `json:"seq"`
	// First and Last are the message ids at each end, so a caller can find the
	// chunk holding a message without opening any of them.
	First string `json:"first,omitempty"`
	Last  string `json:"last,omitempty"`
	// N is how many messages the chunk holds and Bytes is its plaintext size.
	N     int `json:"n"`
	Bytes int `json:"bytes"`
	// CID is the chunk's keyed content address. It is stable across a repack,
	// where the object ref is not, so it is what a copy is verified against.
	CID string `json:"cid,omitempty"`
}

// Validate reports whether a chunk is well formed enough to store.
func (c *Chunk) Validate() error {
	switch {
	case c.ID.IsZero():
		return fmt.Errorf("%w: missing id", ErrInvalid)
	case c.Kind != KindChunk:
		return fmt.Errorf("%w: kind is %q, want %q", ErrInvalid, c.Kind, KindChunk)
	case c.Schema != MessageSchema:
		return fmt.Errorf("%w: chunk %s declares schema %q, this build writes %q",
			ErrInvalid, c.ID, c.Schema, MessageSchema)
	case c.SchemaVersion <= 0:
		return fmt.Errorf("%w: missing schemaVersion", ErrInvalid)
	case c.Session.IsZero():
		return fmt.Errorf("%w: chunk %s names no session", ErrInvalid, c.ID)
	case c.Seq < 0:
		return fmt.Errorf("%w: chunk %s is sequenced %d", ErrInvalid, c.ID, c.Seq)
	case len(c.Messages) == 0:
		return fmt.Errorf("%w: chunk %s holds no messages", ErrInvalid, c.ID)
	}
	for i := range c.Messages {
		if err := c.Messages[i].Validate(); err != nil {
			return err
		}
	}
	return nil
}

// SplitMessages divides a run of messages into chunk-sized groups.
//
// A message is never split across two chunks. One oversized message therefore
// gets a chunk to itself and exceeds the target, which is the right trade: a
// chunk is a storage convenience and a message is the unit of meaning, and half
// a tool result is not a smaller tool result.
func SplitMessages(messages []Message, target int) ([][]Message, error) {
	if target <= 0 {
		target = ChunkTargetBytes
	}
	var out [][]Message
	var group []Message
	var groupBytes int

	for i := range messages {
		size, err := messageBytes(&messages[i])
		if err != nil {
			return nil, err
		}
		if len(group) > 0 && groupBytes+size > target {
			out = append(out, group)
			group, groupBytes = nil, 0
		}
		group = append(group, messages[i])
		groupBytes += size
	}
	if len(group) > 0 {
		out = append(out, group)
	}
	return out, nil
}

// messageBytes measures a message as it will be stored, so the split is decided
// on the same figure the chunk is sized by rather than on an estimate.
func messageBytes(m *Message) (int, error) {
	encoded, err := json.Marshal(m)
	if err != nil {
		return 0, fmt.Errorf("measure message %s: %w", m.ID, err)
	}
	return len(encoded), nil
}
