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
//
// Three signals decide the order, and they compose in this order and no other:
//
//  1. the vector pass retrieves the candidate pool and ranks it by meaning;
//  2. the lexical pass reranks that pool by the words the query used, fused with
//     the vector ranking by weighted reciprocal rank fusion;
//  3. the metadata filter boosts what the caller said it was looking for.
//
// Fusion sits beneath the filter because the two answer different questions.
// Fusion decides which records best match the query as asked; the filter
// expresses a preference about the answer that the query text does not carry.
// Applying the filter first would mean fusing a ranking the caller had already
// perturbed, and the lexical pass would be reranking a pool ordered partly by
// something it knows nothing about.
type Pipeline struct {
	embedder *embed.Embedder
	index    *index.Index
	fetcher  Fetcher
	catalog  Catalog
	lexical  Lexical
}

// New builds a pipeline over an embedder, an index, a fetcher, a catalog and a
// lexical index.
//
// A nil lexical index is allowed and leaves the ranking dense-only. That is a
// worse ranking, not a broken one, and it is what a vault opened before the
// lexical index existed has until it is rebuilt.
func New(embedder *embed.Embedder, idx *index.Index, fetcher Fetcher, catalog Catalog, lexical Lexical) *Pipeline {
	return &Pipeline{embedder: embedder, index: idx, fetcher: fetcher, catalog: catalog, lexical: lexical}
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
	// LexicalWeight overrides how much say the lexical pass gets. The zero
	// value means DefaultLexicalWeight; a negative value disables the lexical
	// pass and ranks on meaning alone.
	LexicalWeight float32
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
	// LexicalHits is how many records the term pass matched. Zero means the
	// query shared no indexed word with anything in the vault, and the ranking
	// is the vector pass's alone.
	LexicalHits int
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
	weight := lexicalWeight(req)
	lexical, err := p.searchLexical(req.Query, pool, weight)
	if err != nil {
		return Result{}, err
	}
	fused := fuse(matches, lexical, weight)
	known, err := p.describe(fused)
	if err != nil {
		return Result{}, err
	}
	ranked, hidden := rank(fused, known, req, limit)
	result.SearchFor = time.Since(start)
	result.Considered = len(fused)
	result.LexicalHits = len(lexical)
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
			Lexical:    match.Lexical,
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
	ID record.ID
	// Score is what the order is decided on: the fused rank score plus whatever
	// the filter added.
	Score float32
	// Similarity is the vector pass's own cosine, carried through unchanged so a
	// caller can still see how close the record sits to the query. It is
	// reported, never ranked on, once fusion has run.
	Similarity float32
	// Fused is the rank-fusion score before the filter touched it, and Lexical
	// is the part of it the lexical pass contributed.
	Fused   float32
	Lexical float32
	Boost   float32
}

// lexicalWeight resolves how much say the lexical pass gets for one request.
//
// A negative weight is how a caller asks for the vector pass alone; zero is the
// unset value and means the shipped default, because a Request literal that does
// not mention fusion must get the shipped ranking.
func lexicalWeight(req Request) float32 {
	if req.LexicalWeight == 0 {
		return DefaultLexicalWeight
	}
	if req.LexicalWeight < 0 {
		return 0
	}
	return req.LexicalWeight
}

// searchLexical runs the term pass, tolerating its absence.
//
// A vault whose lexical index is missing or empty ranks on meaning alone rather
// than failing: half a ranking is a worse answer, and no answer at all is not
// one. A caller who has asked for no lexical contribution is not charged for the
// query either, so ranking by meaning alone costs what it did before there was
// a second pass.
func (p *Pipeline) searchLexical(query string, k int, weight float32) ([]record.ID, error) {
	if p.lexical == nil || weight == 0 {
		return nil, nil
	}
	return p.lexical.SearchLexical(query, k)
}

// describe asks the catalog about the candidate pool, tolerating its absence.
func (p *Pipeline) describe(fused []candidate) (Described, error) {
	if p.catalog == nil || len(fused) == 0 {
		return Described{}, nil
	}
	ids := make([]record.ID, len(fused))
	for i, entry := range fused {
		ids[i] = entry.ID
	}
	return p.catalog.Describe(ids)
}

// rank applies the filter to the fused pool and cuts it to k.
//
// The filter is applied here and nowhere else, as an addition to a score. The
// length of what this returns is min(k, len(fused)) whatever the filter says —
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
func rank(fused []candidate, known Described, req Request, k int) ([]candidate, int) {
	weights := req.Weights.orDefault()
	scored := make([]candidate, 0, len(fused))
	var hidden int

	for _, entry := range fused {
		if !req.IncludeSuperseded && known.Superseded[entry.ID] {
			hidden++
			continue
		}
		if !req.Filter.Empty() {
			if meta, ok := known.Meta[entry.ID]; ok {
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
