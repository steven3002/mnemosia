package recall

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/steven3002/mnemosia/embed"
	"github.com/steven3002/mnemosia/index"
	"github.com/steven3002/mnemosia/record"
)

// A Fetcher resolves a record id to its decrypted body.
//
// Ranking asks for records by id, so everything about where a body actually
// comes from, this device, a cached location, or the network, stays behind
// this one method.
type Fetcher interface {
	Fetch(ctx context.Context, id record.ID) (*record.Memory, Tier, error)
}

// A Catalog answers the questions ranking must settle before it fetches
// anything.
//
// It exists because the order is decided while only ids are in hand: by the time
// a body has been read the ranking has already happened.
//
// It takes the whole candidate pool at once rather than one id at a time. That
// is not only cheaper — it is what lets the answer be read from the device on
// every recall instead of from a cache built when the process started. Another
// process sharing this vault can supersede a record at any moment, and a cached
// answer would keep returning the replaced version until something reopened.
type Catalog interface {
	Describe(ids []record.ID) (Described, error)
}

// Described is what the catalog knows about a candidate pool.
type Described struct {
	// Meta is keyed by record id. A record that is absent is ranked on
	// similarity alone rather than dropped: not knowing a record's tags is a
	// gap in this device's metadata, never evidence about the record.
	Meta map[record.ID]Meta
	// Superseded holds the ids some later record replaces.
	Superseded map[record.ID]bool
}

// A Pipeline runs a query from text to ranked records.
type Pipeline struct {
	embedder *embed.Embedder
	index    *index.Index
	fetcher  Fetcher
	catalog  Catalog
}

// New builds a pipeline over an embedder, an index, a fetcher and a catalog.
func New(embedder *embed.Embedder, idx *index.Index, fetcher Fetcher, catalog Catalog) *Pipeline {
	return &Pipeline{embedder: embedder, index: idx, fetcher: fetcher, catalog: catalog}
}

// A Request is one recall.
type Request struct {
	Query string
	Limit int
	// Filter prefers some records over others. It never removes any.
	Filter Filter
	// Weights override how far the filter may move a record. The zero value
	// means the shipped defaults.
	Weights Weights
	// IncludeSuperseded returns replaced versions alongside current ones.
	//
	// Default recall answers with the vault's current state, which is what
	// makes an append-only store usable: otherwise every correction comes back
	// beside the thing it corrected and the caller has to work out which is
	// which. History is retained and reachable; it is just not the default.
	IncludeSuperseded bool
	// Candidates is how many records are scored before the filter reorders
	// them. Zero means DefaultCandidates.
	Candidates int
}

// A Result is one recall's ranked hits and the time each stage took.
type Result struct {
	Hits []Hit
	// EmbedFor is how long the query took to embed, SearchFor how long the
	// vector scan and ranking took, FetchFor how long resolving and decrypting
	// the hits took.
	EmbedFor  time.Duration
	SearchFor time.Duration
	FetchFor  time.Duration
	// Searched is how many vectors the query was scored against.
	Searched int
	// Considered is how many candidates the filter reordered.
	Considered int
	// SupersededHidden is how many replaced versions were held back.
	SupersededHidden int
}

const (
	// DefaultLimit is how many records a recall returns when the caller does
	// not say.
	DefaultLimit = 5

	// DefaultCandidates is how deep the pool is that the filter reorders.
	//
	// A boost can only reorder what similarity already retrieved, so a pool the
	// size of the answer would make the filter decorative — the records it
	// exists to promote would never have been fetched. This is the depth the
	// measured gain was established at.
	DefaultCandidates = 100
)

// Run executes a recall.
//
// Returning nothing is a legitimate answer, not a failure: a vault that holds no
// relevant record should say so rather than return its least-bad guess.
func (p *Pipeline) Run(ctx context.Context, req Request) (Result, error) {
	if req.Query == "" {
		return Result{}, fmt.Errorf("recall: empty query")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	pool := req.Candidates
	if pool <= 0 {
		pool = DefaultCandidates
	}
	if pool < limit {
		pool = limit
	}

	start := time.Now()
	query, err := p.embedder.EmbedOne(ctx, req.Query)
	if err != nil {
		return Result{}, err
	}
	result := Result{EmbedFor: time.Since(start), Searched: p.index.Len()}

	start = time.Now()
	matches, err := p.index.Search(query, pool)
	if err != nil {
		return Result{}, err
	}
	known, err := p.describe(matches)
	if err != nil {
		return Result{}, err
	}
	ranked, hidden := rank(matches, known, req, limit)
	result.SearchFor = time.Since(start)
	result.Considered = len(matches)
	result.SupersededHidden = hidden

	start = time.Now()
	for _, match := range ranked {
		fetchedAt := time.Now()
		memory, tier, err := p.fetcher.Fetch(ctx, match.ID)
		if err != nil {
			return Result{}, fmt.Errorf("recall %s: %w", match.ID, err)
		}
		result.Hits = append(result.Hits, Hit{
			Memory:     memory,
			Score:      match.Score,
			Similarity: match.Similarity,
			Boost:      match.Boost,
			Tier:       tier,
			FetchedIn:  time.Since(fetchedAt),
		})
	}
	result.FetchFor = time.Since(start)
	return result, nil
}

// A candidate is one record on its way to being ranked.
type candidate struct {
	ID         record.ID
	Score      float32
	Similarity float32
	Boost      float32
}

// describe asks the catalog about the candidate pool, tolerating its absence.
func (p *Pipeline) describe(matches []index.Match) (Described, error) {
	if p.catalog == nil || len(matches) == 0 {
		return Described{}, nil
	}
	ids := make([]record.ID, len(matches))
	for i, match := range matches {
		ids[i] = match.ID
	}
	return p.catalog.Describe(ids)
}

// rank orders the candidate pool and cuts it to k.
//
// The filter is applied here and nowhere else, as an addition to a score. The
// length of what this returns is min(k, len(matches)) whatever the filter says —
// that is the property which makes a wrong filter cost ranking quality instead
// of the answer, and it is the one thing about this function that must not
// change.
//
// Supersession is the single thing that does drop a candidate, and it is not an
// exception to the rule above. It is a statement about which version of a record
// is current, decided by the vault's own history, not a guess about relevance
// supplied by a caller who may be wrong about it.
//
// It is a free function rather than a method so that it can be exercised without
// an embedder, an index or a network.
func rank(matches []index.Match, known Described, req Request, k int) ([]candidate, int) {
	weights := req.Weights.orDefault()
	scored := make([]candidate, 0, len(matches))
	var hidden int

	for _, match := range matches {
		if !req.IncludeSuperseded && known.Superseded[match.ID] {
			hidden++
			continue
		}
		entry := candidate{ID: match.ID, Score: match.Score, Similarity: match.Score}
		if !req.Filter.Empty() {
			if meta, ok := known.Meta[match.ID]; ok {
				entry.Boost = req.Filter.boost(weights, meta)
				entry.Score += entry.Boost
			}
		}
		scored = append(scored, entry)
	}

	sort.SliceStable(scored, func(a, b int) bool {
		if scored[a].Score != scored[b].Score {
			return scored[a].Score > scored[b].Score
		}
		// Ties break on the id so two runs over one vault agree.
		return scored[a].ID.String() < scored[b].ID.String()
	})
	if len(scored) > k {
		scored = scored[:k]
	}
	return scored, hidden
}
