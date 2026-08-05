// Package recall runs the retrieval pipeline end to end.
package recall

import (
	"time"

	"github.com/steven3002/mnemosia/record"
)

// A Tier names where a record's body came from.
//
// The three differ by roughly an order of magnitude each, so which tier served
// a read is the single most useful thing to know about its latency.
type Tier string

const (
	// TierLocal is this device's own copy.
	TierLocal Tier = "local"
	// TierCached uses cached location metadata, skipping the indexer lookup.
	TierCached Tier = "cached"
	// TierNetwork asks the indexer, then fetches the shards.
	TierNetwork Tier = "network"
)

// A Hit is one recalled record with the evidence for why it was returned.
type Hit struct {
	Memory *record.Memory
	// Score is what the record was ranked on: the fused rank score of the vector
	// and lexical passes, plus any boost.
	//
	// It is a rank-fusion score and not a similarity. The scale is set by the
	// fusion constant rather than by the embedding, so the useful comparison is
	// between two hits in one result and never between two runs or two vaults.
	Score float32
	// Similarity is cosine similarity against the query, in [-1, 1].
	//
	// It is reported separately from Score so a caller can always see how much
	// of a record's position it earned by meaning, how much by the words it
	// used, and how much by matching the filter it was given.
	Similarity float32
	// Lexical is how much of Score the term pass contributed. Zero means the
	// record was retrieved by meaning alone.
	Lexical float32
	// Boost is how far the filter moved this record.
	Boost float32
	// Tier is where the body was read from.
	Tier Tier
	// FetchedIn is how long fetching and decrypting this record took.
	FetchedIn time.Duration
}
