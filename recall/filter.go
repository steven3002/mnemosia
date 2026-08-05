package recall

import (
	"strings"

	"github.com/steven3002/mnemosia/record"
)

// A Filter narrows a recall by preference.
//
// It boosts and it cannot do anything else. There is deliberately no predicate
// on this type — no Matches, no Passes, no Apply that returns a subset — because
// the difference between preferring and excluding is the difference between a
// slightly worse answer and no answer at all. Applied as an exclusion, a single
// wrong tag drops hit@5 from 0.949 to 0.729, below not filtering at all, and
// returns nothing whatsoever for ten queries in fifty-nine. The identical wrong
// filter applied as a boost scores 0.881, which is what no filter scores.
//
// A future caller who wants a hard filter cannot build one out of what this type
// offers. Boost is a magnitude, not a verdict, and nothing here reports whether
// a record matched.
type Filter struct {
	// Types prefers records of these kinds.
	Types []record.Type
	// Tags prefers records carrying these tags. Matching is graded: carrying
	// two of three requested tags is worth more than one, which is what makes a
	// filter with one wrong tag degrade rather than fall over.
	Tags []string
}

// Empty reports whether the filter would change any ranking.
func (f Filter) Empty() bool { return len(f.Types) == 0 && len(f.Tags) == 0 }

// Weights set how far a filter may move a record.
//
// They are additive on the cosine scale the index scores in, so a boost is
// commensurable with a similarity difference and can be reasoned about: a tag
// weight of 0.05 says "carrying every requested tag is worth as much as five
// hundredths of cosine similarity". A multiplicative form was rejected because
// cosine can be negative, where multiplying by more than one demotes.
type Weights struct {
	Tag  float32
	Type float32
}

// DefaultWeights are the shipped weights, measured on the regression corpus.
//
// They were not carried over from the spike that established filtering works:
// that spike boosted reciprocal-rank-fusion scores, whose spacing has nothing to
// do with cosine's, so its constant would have meant something else here.
//
// Nor were they chosen by maximising the score under a correct filter. That
// number rises monotonically with the weight, and it rises for a bad reason — a
// large enough boost swamps similarity entirely, which is hard filtering under
// another name. The criterion is instead the largest boost that still costs a
// *wrong* filter nothing, since surviving a wrong filter is the entire reason
// soft filtering exists.
//
// Measured across three agent qualities (TestFilterWeightSweep): at this weight
// a correct filter takes hit@5 from 0.950 to 1.000 and MRR from 0.883 to 0.963,
// while a filter with every tag wrong scores 0.950 / 0.883 — the unfiltered
// baseline exactly. At 0.50 the wrong filter's MRR falls to 0.858, below
// baseline, and at 1.00 it collapses to 0.475. The harm begins between the two.
//
// The 3:1 ratio between the tag and type weights was held fixed through the
// sweep rather than optimised on its own; it reflects that tags discriminate
// more than types, which was measured separately, not here.
var DefaultWeights = Weights{Tag: 0.20, Type: 0.067}

func (w Weights) orDefault() Weights {
	if w == (Weights{}) {
		return DefaultWeights
	}
	return w
}

// Meta is what ranking knows about a record before it has fetched it.
//
// Ranking has to decide an order before it reads any bodies — that is the whole
// point of holding an index — so anything the ordering depends on has to be
// available here rather than on the record itself.
type Meta struct {
	Type record.Type
	Tags []string
}

// boost scores how well a record answers the filter, in [0, 1] per component.
//
// It is unexported and takes the metadata rather than hanging off Filter, so
// that the only way to reach it is through the ranking path, which always
// returns as many results as it was asked for.
func (f Filter) boost(w Weights, meta Meta) float32 {
	if f.Empty() {
		return 0
	}
	w = w.orDefault()
	var total float32

	if len(f.Tags) > 0 {
		carried := make(map[string]bool, len(meta.Tags))
		for _, tag := range meta.Tags {
			carried[normalizeTag(tag)] = true
		}
		var matched int
		for _, want := range f.Tags {
			if carried[normalizeTag(want)] {
				matched++
			}
		}
		total += w.Tag * float32(matched) / float32(len(f.Tags))
	}

	for _, want := range f.Types {
		if want == meta.Type {
			total += w.Type
			break
		}
	}
	return total
}

// normalizeTag matches the form the device stores tags in, so a filter and a
// record that mean the same tag agree regardless of how either was typed.
func normalizeTag(tag string) string { return strings.ToLower(strings.TrimSpace(tag)) }
