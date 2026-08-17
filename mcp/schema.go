package mcp

import (
	"bytes"
	_ "embed"
	"encoding/json"
)

//go:embed schema.json
var schemaJSON []byte

// SaveSessionSchema is the input schema for save_session, written by hand.
//
// It is the one schema on this surface that is not inferred from its Go type,
// and there are two reasons, both of which make the hand-written version better
// rather than merely necessary.
//
// The first is that it cannot be inferred. A stored message's content is an
// ordered list of typed parts, and a tool result carries parts of its own, so a
// tool that returned an image is stored as an image rather than as a description
// of one. That recursion is the point of the shape, and schema inference rejects
// a recursive Go type outright rather than emitting the `$ref` that expresses it.
//
// The second is that inference would produce an undocumented schema. Property
// descriptions come from struct tags, and the record package's types carry none:
// they describe how a message is stored, not how an agent should fill one in.
// Every property in schema.json is documented for the agent that has to
// construct one, and the correlation id, the single field no later reader can
// reconstruct, is stated twice, once on each half of the exchange.
//
// A test asserts that every field of record.Message and record.Part appears
// there, so the schema cannot fall behind the type it describes.
var SaveSessionSchema = json.RawMessage(bytes.TrimSpace(schemaJSON))
