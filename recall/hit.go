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
	// Score is cosine similarity against the query, in [-1, 1].
	Score float32
	// Tier is where the body was read from.
	Tier Tier
	// FetchedIn is how long fetching and decrypting this record took.
	FetchedIn time.Duration
}

// A Filter narrows a recall.
//
// It boosts, never excludes. Applied as an exclusion, a single wrong tag
// returns nothing at all for a sixth of queries, worse than no filter, while
// the same wrong filter applied as a boost merely stops helping. A filter must
// never become a where clause.
type Filter struct {
	Types []record.Type
	Tags  []string
}

// Empty reports whether the filter would change any ranking.
func (f Filter) Empty() bool { return len(f.Types) == 0 && len(f.Tags) == 0 }
