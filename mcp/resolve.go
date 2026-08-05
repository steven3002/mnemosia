package mcp

import (
	"fmt"
	"strings"

	"github.com/steven3002/mnemosia/record"
)

// Scheme is the vault's URI scheme. Memory, sessions and skills are three kinds
// in one address space, not three endpoints, so one scheme addresses all of
// them and one function resolves them.
const Scheme = "mnemosia://"

// A Resolved is a parsed vault URI.
type Resolved struct {
	Kind record.Kind
	ID   record.ID
}

// URI renders the canonical address of a record.
func URI(kind record.Kind, id record.ID) string {
	return Scheme + string(kind) + "/" + id.String()
}

// Resolve parses a vault URI.
//
// There is exactly one of these, and every path into the namespace goes through
// it, the tool that opens a record and the protocol's own resource reads
// alike. Two entry points into one address space drift, and they drift quietly,
// because each looks correct on its own.
func Resolve(uri string) (Resolved, error) {
	rest, ok := strings.CutPrefix(uri, Scheme)
	if !ok {
		return Resolved{}, fmt.Errorf("resolve %q: not a %s URI", uri, Scheme)
	}
	kindPart, idPart, ok := strings.Cut(rest, "/")
	if !ok {
		return Resolved{}, fmt.Errorf("resolve %q: expected %s<kind>/<id>", uri, Scheme)
	}
	kind := record.Kind(kindPart)
	switch kind {
	case record.KindMemory, record.KindSession:
	default:
		return Resolved{}, fmt.Errorf("resolve %q: unknown kind %q", uri, kindPart)
	}
	id, err := record.ParseID(idPart)
	if err != nil {
		return Resolved{}, fmt.Errorf("resolve %q: %w", uri, err)
	}
	return Resolved{Kind: kind, ID: id}, nil
}
