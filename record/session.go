package record

import (
	"fmt"
	"strings"
)

// A SessionKind separates the three things a stored conversation can be.
//
// They are not interchangeable and a flat list conflates them: three
// independent implementations store a sub-agent run as its own transcript
// keyed by agent, and a background run is a distinct class in the one that has
// a name for it. A session record that cannot say which it is has already lost
// the distinction.
type SessionKind string

const (
	// SessionMain is a conversation a user drove directly.
	SessionMain SessionKind = "main"
	// SessionSubagent is work a parent session delegated.
	SessionSubagent SessionKind = "subagent"
	// SessionBackground is a run that was not attended.
	SessionBackground SessionKind = "background"
)

// SessionKinds lists the vocabulary in its canonical order.
var SessionKinds = []SessionKind{SessionMain, SessionSubagent, SessionBackground}

// Valid reports whether k is in the vocabulary.
func (k SessionKind) Valid() bool {
	for _, known := range SessionKinds {
		if k == known {
			return true
		}
	}
	return false
}

func (k SessionKind) String() string { return string(k) }

// A Session is the head of a stored conversation: everything about it except
// the conversation.
//
// Messages live only in chunks, which the head names in order. Real sessions
// reach hundreds of thousands of tokens and hundreds of megabytes, so the whole
// value of the shape is that the head stays small enough to list a thousand of
// them without reading a single transcript.
//
// The head is the one mutable record in the vault. Appending a turn writes new
// chunks and rewrites the head, which is why it is not a packed record: a slab
// holds immutable content, and a record that changes on every turn would strand
// a version of itself in one on each write.
type Session struct {
	ID ID `json:"id"`
	// Schema names the message format the chunks are written in, so a reader
	// can tell whether it understands a transcript before replaying it.
	Schema        string `json:"schema"`
	SchemaVersion int    `json:"schemaVersion"`
	// Version counts head revisions. It increments on every append and is what
	// a merge decides on, since a wall clock cannot order two devices.
	Version int64 `json:"version"`

	// Title and Summary are the human and the semantic surface. Summary is
	// what gets embedded, together with the title and the tags, and it is the
	// only part of a session that ever does.
	Title   string   `json:"title"`
	Summary string   `json:"summary,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Project Project  `json:"project,omitzero"`

	Created  Time `json:"created"`
	Updated  Time `json:"updated"`
	Archived bool `json:"archived,omitempty"`

	// Agent and Models are provenance: which client wrote this, and which
	// models spoke in it. A session may span more than one model.
	Agent  Agent    `json:"agent,omitzero"`
	Models []string `json:"models,omitempty"`
	Counts Counts   `json:"counts,omitzero"`

	Kind SessionKind `json:"kind"`
	// AgentRef identifies the sub-agent when Kind is SessionSubagent.
	AgentRef *AgentRef `json:"agentRef,omitempty"`

	Lineage Lineage `json:"lineage,omitzero"`

	// HeadMessage is the leaf of the message DAG: where a resume continues
	// from.
	HeadMessage string `json:"headMessage,omitempty"`
	// PreservedTail is the messages kept verbatim beside the summary, for a
	// caller that wants a cheap resume without reading the whole transcript.
	// It is an accelerator and never a replacement: the full log is always
	// still there, because a store that silently discards most of the user's
	// data is not a user-owned store.
	PreservedTail []string `json:"preservedTail,omitempty"`

	// Chunks are the transcript, in order, named by record id.
	Chunks []ChunkRef `json:"chunks,omitempty"`

	Links SessionLinks `json:"links,omitzero"`

	// Embedding records which model produced the summary vector, so a model
	// change is detectable rather than a silently wrong ranking.
	Embedding Embedding `json:"embedding,omitzero"`
}

// A Project is where a session happened.
type Project struct {
	CWD    string `json:"cwd,omitempty"`
	Repo   string `json:"repo,omitempty"`
	Branch string `json:"branch,omitempty"`
}

// Empty reports whether the project is unset.
func (p Project) Empty() bool { return p == Project{} }

// An Agent is the client that produced a session.
type Agent struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

// An AgentRef identifies a sub-agent.
type AgentRef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Role string `json:"role,omitempty"`
}

// Counts are the totals a lister wants without opening anything.
type Counts struct {
	Messages   int   `json:"messages,omitempty"`
	Chunks     int   `json:"chunks,omitempty"`
	Bytes      int64 `json:"bytes,omitempty"`
	TokensIn   int   `json:"tokensIn,omitempty"`
	TokensOut  int   `json:"tokensOut,omitempty"`
	DurationMS int64 `json:"durationMs,omitempty"`
}

// A Lineage holds the three edges between sessions, which are three different
// relationships and are deliberately not collapsed into one.
//
// Containment is a sub-agent inside its parent; divergence is a fork from a
// point in another conversation; continuation is this session carrying on from
// an earlier one. An implementation that keeps only one of them cannot answer
// the questions the other two ask.
//
// Every edge is a record id. Object ids are rewritten wholesale by a repack, so
// a structure keyed on them would see every record as new the next time storage
// was reorganised.
type Lineage struct {
	ParentSession *ID   `json:"parentSession,omitempty"`
	ForkedFrom    *Fork `json:"forkedFrom,omitempty"`
	ResumedFrom   *ID   `json:"resumedFrom,omitempty"`
	// Children is denormalised so that loading a session and its sub-agents is
	// one lookup rather than a scan of every session in the vault.
	Children []ID `json:"children,omitempty"`
}

// Empty reports whether a session has no lineage at all.
func (l Lineage) Empty() bool {
	return l.ParentSession == nil && l.ForkedFrom == nil && l.ResumedFrom == nil && len(l.Children) == 0
}

// A Fork is the point another session diverged from.
type Fork struct {
	SessionID ID     `json:"sessionId"`
	MessageID string `json:"messageId,omitempty"`
}

// SessionLinks are the edges out of a session into the rest of the vault.
type SessionLinks struct {
	// Memories were extracted from or injected into this conversation. The
	// reverse edge lives on the memory, in its source, so a fact reaches its
	// conversation and a conversation reaches its facts.
	Memories []ID `json:"memories,omitempty"`
}

// Empty reports whether a session links to nothing.
func (l SessionLinks) Empty() bool { return len(l.Memories) == 0 }

// RecordKind reports what a session is on the substrate.
func (s *Session) RecordKind() Kind { return KindSession }

// IndexText is what gets embedded for a session: its title, summary and tags.
//
// The transcript is never embedded. Embedding every message of a long
// conversation is a large cost for a poor result, transcripts are mostly
// conversational filler and tool noise, and the summary is the surface a
// search over sessions is actually looking for.
func (s *Session) IndexText() string {
	parts := make([]string, 0, 3)
	if s.Title != "" {
		parts = append(parts, s.Title)
	}
	if s.Summary != "" {
		parts = append(parts, s.Summary)
	}
	if len(s.Tags) > 0 {
		parts = append(parts, strings.Join(s.Tags, " "))
	}
	return strings.Join(parts, "\n\n")
}

// Messages reports how many messages the chunks hold.
func (s *Session) Messages() int {
	var n int
	for _, chunk := range s.Chunks {
		n += chunk.N
	}
	return n
}

// ChunkBytes reports the plaintext size of the transcript.
func (s *Session) ChunkBytes() int64 {
	var n int64
	for _, chunk := range s.Chunks {
		n += int64(chunk.Bytes)
	}
	return n
}

// Limits on a session head. The head is loaded on every list, so its size is
// the cost of listing.
const (
	MaxTitleBytes   = 512
	MaxSummaryBytes = 16 << 10
)

// Validate reports whether a session head is well formed enough to store.
func (s *Session) Validate() error {
	switch {
	case s.ID.IsZero():
		return fmt.Errorf("%w: missing id", ErrInvalid)
	case s.Schema != MessageSchema:
		return fmt.Errorf("%w: session %s declares schema %q, this build writes %q",
			ErrInvalid, s.ID, s.Schema, MessageSchema)
	case s.SchemaVersion <= 0:
		return fmt.Errorf("%w: missing schemaVersion", ErrInvalid)
	case s.Version <= 0:
		return fmt.Errorf("%w: session %s has version %d, want at least 1", ErrInvalid, s.ID, s.Version)
	case !s.Kind.Valid():
		return fmt.Errorf("%w: session %s has kind %q, want one of %s",
			ErrInvalid, s.ID, s.Kind, strings.Join(sessionKindNames(), ", "))
	}

	if strings.TrimSpace(s.Title) == "" {
		return fmt.Errorf("%w: session %s has no title, and the title is half of what makes it "+
			"findable and all of what makes a list readable", ErrInvalid, s.ID)
	}
	if len(s.Title) > MaxTitleBytes {
		return fmt.Errorf("%w: title is %d bytes, limit %d", ErrInvalid, len(s.Title), MaxTitleBytes)
	}
	if len(s.Summary) > MaxSummaryBytes {
		return fmt.Errorf("%w: summary is %d bytes, limit %d", ErrInvalid, len(s.Summary), MaxSummaryBytes)
	}
	if len(s.Tags) > MaxTags {
		return fmt.Errorf("%w: %d tags, limit %d", ErrInvalid, len(s.Tags), MaxTags)
	}
	for _, tag := range s.Tags {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("%w: empty tag", ErrInvalid)
		}
		if len(tag) > MaxTagBytes {
			return fmt.Errorf("%w: tag %q is %d bytes, limit %d", ErrInvalid, tag, len(tag), MaxTagBytes)
		}
	}

	if s.Kind == SessionSubagent && s.Lineage.ParentSession == nil {
		return fmt.Errorf("%w: session %s is a sub-agent run with no parent session", ErrInvalid, s.ID)
	}
	for i, chunk := range s.Chunks {
		if chunk.ID.IsZero() {
			return fmt.Errorf("%w: session %s chunk %d has no record id", ErrInvalid, s.ID, i)
		}
		if chunk.Seq != i {
			return fmt.Errorf("%w: session %s chunk %d is sequenced %d; chunks are ordered and the "+
				"order is the transcript", ErrInvalid, s.ID, i, chunk.Seq)
		}
	}
	return nil
}

func sessionKindNames() []string {
	out := make([]string, len(SessionKinds))
	for i, k := range SessionKinds {
		out[i] = string(k)
	}
	return out
}
