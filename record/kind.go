package record

// A Kind is the class of record stored on the substrate. Kinds share one
// address space, one index and one manifest; retrieval mode differs by kind.
type Kind string

const (
	// KindMemory is an atomic proposition retrieved by meaning.
	KindMemory Kind = "memory"
	// KindSession is a conversation head plus immutable chunks, retrieved by
	// metadata and version.
	KindSession Kind = "session"
	// KindChunk is an immutable ordered slice of a session's transcript. It is
	// a record of its own so that appending to a conversation never rewrites
	// what was already stored, and so that a transcript can be read a piece at
	// a time.
	KindChunk Kind = "chunk"
)

// A Type classifies a memory record. The vocabulary is closed: soft metadata
// filtering discriminates on it, so an unrecognised value is unreachable by the
// mechanism that makes recall work.
type Type string

const (
	// TypeFact is an objective statement about the world or the project.
	TypeFact Type = "fact"
	// TypePreference is how the user wants things done.
	TypePreference Type = "preference"
	// TypeInsight is a conclusion drawn rather than observed.
	TypeInsight Type = "insight"
	// TypeDoc is reference material held verbatim.
	TypeDoc Type = "doc"
	// TypeProfile is a durable attribute of a person, team or system.
	TypeProfile Type = "profile"
	// TypeCorrection records that a prior claim was never true, which the
	// bi-temporal fields cannot express: they distinguish outdated from
	// current, not false from historical.
	TypeCorrection Type = "correction"
)

// Types lists the vocabulary in its canonical order.
var Types = []Type{TypeFact, TypePreference, TypeInsight, TypeDoc, TypeProfile, TypeCorrection}

// Valid reports whether t is in the vocabulary.
func (t Type) Valid() bool {
	for _, known := range Types {
		if t == known {
			return true
		}
	}
	return false
}

func (t Type) String() string { return string(t) }

// TypeNames returns the vocabulary as strings, for help text and tool schemas.
func TypeNames() []string {
	names := make([]string, len(Types))
	for i, t := range Types {
		names[i] = string(t)
	}
	return names
}
