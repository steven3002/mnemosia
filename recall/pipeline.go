package recall

import (
	"context"
	"fmt"
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

// A Pipeline runs a query from text to ranked records.
type Pipeline struct {
	embedder *embed.Embedder
	index    *index.Index
	fetcher  Fetcher
}

// New builds a pipeline over an embedder, an index and a fetcher.
func New(embedder *embed.Embedder, idx *index.Index, fetcher Fetcher) *Pipeline {
	return &Pipeline{embedder: embedder, index: idx, fetcher: fetcher}
}

// A Request is one recall.
type Request struct {
	Query string
	Limit int
	// Filter narrows the ranking. Ranking-side filtering is not applied yet:
	// the measurement that decides how much a tag is worth against a vault of
	// closely related records has not been run, and guessing the weight is how
	// a filter turns into an exclusion by accident.
	Filter Filter
}

// A Result is one recall's ranked hits and the time each stage took.
type Result struct {
	Hits []Hit
	// EmbedFor is how long the query took to embed, SearchFor how long the
	// vector scan took, FetchFor how long resolving and decrypting the hits
	// took.
	EmbedFor  time.Duration
	SearchFor time.Duration
	FetchFor  time.Duration
	// Searched is how many vectors the query was scored against.
	Searched int
}

// DefaultLimit is how many records a recall returns when the caller does not
// say.
const DefaultLimit = 5

// Run executes a recall.
//
// Returning nothing is a legitimate answer, not a failure: a vault that holds
// no relevant record should say so rather than return its least-bad guess.
func (p *Pipeline) Run(ctx context.Context, req Request) (Result, error) {
	if req.Query == "" {
		return Result{}, fmt.Errorf("recall: empty query")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}

	start := time.Now()
	query, err := p.embedder.EmbedOne(ctx, req.Query)
	if err != nil {
		return Result{}, err
	}
	result := Result{EmbedFor: time.Since(start), Searched: p.index.Len()}

	start = time.Now()
	matches, err := p.index.Search(query, limit)
	if err != nil {
		return Result{}, err
	}
	result.SearchFor = time.Since(start)

	start = time.Now()
	for _, match := range matches {
		fetchedAt := time.Now()
		memory, tier, err := p.fetcher.Fetch(ctx, match.ID)
		if err != nil {
			return Result{}, fmt.Errorf("recall %s: %w", match.ID, err)
		}
		result.Hits = append(result.Hits, Hit{
			Memory:    memory,
			Score:     match.Score,
			Tier:      tier,
			FetchedIn: time.Since(fetchedAt),
		})
	}
	result.FetchFor = time.Since(start)
	return result, nil
}
