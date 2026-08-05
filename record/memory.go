package record

// SchemaVersion is the version stamped on every record this build writes.
//
// Evolution is additive only: readers tolerate unknown fields and old versions
// are upcast in memory on read. Stored blobs are immutable, so a field is never
// removed or repurposed and there is no bulk rewrite.
const SchemaVersion = 1

// A Memory is one atomic proposition plus the context that disambiguates it.
//
// The whole field set is defined now even though early builds populate a subset:
// records are immutable and append-only, so widening the schema is free before
// data exists and costly afterwards.
type Memory struct {
	ID            ID    `json:"id"`
	Kind          Kind  `json:"kind"`
	Type          Type  `json:"type"`
	SchemaVersion int   `json:"schemaVersion"`
	Version       int64 `json:"version"`

	// Statement is a single proposition. Context is what makes it resolvable
	// once it is separated from the conversation it came from; it contributes
	// more to retrieval than any model choice, so it is not optional.
	Statement string   `json:"statement"`
	Context   string   `json:"context"`
	Keywords  []string `json:"keywords,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Entities  []Entity `json:"entities,omitempty"`

	// CreatedAt and UpdatedAt are when the vault learned the statement.
	CreatedAt Time `json:"createdAt"`
	UpdatedAt Time `json:"updatedAt"`
	// ValidFrom and ValidUntil are when the statement is true of the world.
	// ValidUntil is a belief boundary, not a delete flag.
	ValidFrom      *Time `json:"validFrom,omitempty"`
	ValidUntil     *Time `json:"validUntil,omitempty"`
	LastRecalledAt *Time `json:"lastRecalledAt,omitempty"`
	RecallCount    int   `json:"recallCount,omitempty"`

	// Importance and Confidence are supplied by the calling agent; the vault
	// runs no model of its own and does not infer them.
	Importance float64 `json:"importance,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`

	Supersedes   *ID `json:"supersedes,omitempty"`
	SupersededBy *ID `json:"supersededBy,omitempty"`

	Links  []string `json:"links,omitempty"`
	Source Source   `json:"source,omitzero"`

	// Embedding records which model produced the vector, so a model change is
	// detectable and re-indexable instead of silently degrading recall.
	Embedding Embedding `json:"embedding,omitzero"`
}

// An Entity is a typed thing a statement refers to.
type Entity struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// A Source is the provenance of a record: where the vault learned it.
type Source struct {
	Origin    string `json:"origin,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	Span      string `json:"span,omitempty"`
	Client    string `json:"client,omitempty"`
}

// An Embedding identifies the vector held for a record in the index.
type Embedding struct {
	Model string `json:"model,omitempty"`
	Dim   int    `json:"dim,omitempty"`
}

// IndexText is what gets embedded: the statement together with its context.
// Nothing else in the pipeline moves retrieval as much, so the two are never
// embedded apart.
func (m *Memory) IndexText() string {
	if m.Context == "" {
		return m.Statement
	}
	return m.Statement + "\n\n" + m.Context
}
