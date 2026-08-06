package record

import "strings"

// spanSeparator joins the two ends of a span in the single string a memory's
// source carries.
const spanSeparator = ".."

// A Span names a run of turns within a session.
//
// It is stored as one string on a memory's source rather than as two fields,
// because the memory schema is already written and on the network: records are
// immutable, so a field cannot be added to one that exists. The parse is here,
// once, instead of in every caller that wants to know which turns a fact came
// from.
type Span struct {
	First string
	Last  string
}

// NewSpan names the run from first to last inclusive. A single turn is a span
// whose ends are the same.
func NewSpan(first, last string) Span { return Span{First: first, Last: last} }

// ParseSpan reads the stored form. An empty or unparseable value is an empty
// span, which means the memory names a conversation but no place in it.
func ParseSpan(s string) Span {
	s = strings.TrimSpace(s)
	if s == "" {
		return Span{}
	}
	first, last, found := strings.Cut(s, spanSeparator)
	if !found {
		return Span{First: s, Last: s}
	}
	first, last = strings.TrimSpace(first), strings.TrimSpace(last)
	if last == "" {
		last = first
	}
	return Span{First: first, Last: last}
}

// String renders the stored form.
func (s Span) String() string {
	switch {
	case s.First == "" && s.Last == "":
		return ""
	case s.First == s.Last:
		return s.First
	default:
		return s.First + spanSeparator + s.Last
	}
}

// Empty reports whether the span names no turn.
func (s Span) Empty() bool { return s.First == "" && s.Last == "" }

// Select returns the messages the span covers.
//
// It selects by position between the two named turns rather than by matching
// each id, so a span over turns that were interleaved with others still returns
// what was actually said between its ends. A span whose first turn is absent
// selects nothing: a provenance edge that cannot be resolved should say so
// rather than return a plausible neighbourhood.
func (s Span) Select(messages []Message) []Message {
	if s.Empty() {
		return nil
	}
	first := -1
	for i := range messages {
		if messages[i].ID == s.First {
			first = i
			break
		}
	}
	if first < 0 {
		return nil
	}
	last := first
	for i := first; i < len(messages); i++ {
		if messages[i].ID == s.Last {
			last = i
			break
		}
	}
	return messages[first : last+1]
}
