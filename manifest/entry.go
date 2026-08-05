// Package manifest maps record ids to object locations.
package manifest

import (
	"github.com/steven3002/mnemosia/record"
	"github.com/steven3002/mnemosia/seal"
)

// An Entry is the catalog's record of where one record version lives.
//
// It is keyed on the record id and not on the object ref, because repack
// rewrites every object ref: a catalog keyed on storage location would see
// every repacked record as a new record.
type Entry struct {
	ID        record.ID   `json:"id"`
	Kind      record.Kind `json:"kind"`
	Type      record.Type `json:"type"`
	Version   int64       `json:"version"`
	CID       seal.CID    `json:"cid"`
	ObjectRef string      `json:"objectRef"`
	SlabID    string      `json:"slabId,omitempty"`
	Bytes     int         `json:"bytes,omitempty"`
	Tags      []string    `json:"tags,omitempty"`
	WrittenAt record.Time `json:"writtenAt"`
}
