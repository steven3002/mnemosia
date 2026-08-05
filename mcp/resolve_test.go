package mcp_test

import (
	"testing"

	"github.com/steven3002/mnemosia/mcp"
	"github.com/steven3002/mnemosia/record"
)

func TestResolveRoundTrip(t *testing.T) {
	id, err := record.NewID()
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	for _, kind := range []record.Kind{record.KindMemory, record.KindSession} {
		resolved, err := mcp.Resolve(mcp.URI(kind, id))
		if err != nil {
			t.Fatalf("resolve %s: %v", kind, err)
		}
		if resolved.Kind != kind || resolved.ID != id {
			t.Fatalf("resolved to %s/%s, want %s/%s", resolved.Kind, resolved.ID, kind, id)
		}
	}
}

func TestResolveRejectsMalformedURIs(t *testing.T) {
	id, _ := record.NewID()
	for _, uri := range []string{
		"",
		"https://example.com/memory/" + id.String(),
		"mnemosia://memory",
		"mnemosia://skill/" + id.String(),
		"mnemosia://memory/not-an-id",
		"mnemosia://memory/" + id.String() + "ff",
	} {
		if _, err := mcp.Resolve(uri); err == nil {
			t.Fatalf("%q was accepted", uri)
		}
	}
}
