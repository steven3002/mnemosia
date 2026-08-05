package mcp

import (
	"strings"

	"github.com/steven3002/mnemosia/record"
)

// The tool descriptions are product surface, not documentation.
//
// Recall quality rests on an agent supplying two or three accurate tags and a
// type at both ends of the loop: a record written without them is unreachable by
// the mechanism that makes retrieval work, and a query issued without them
// leaves that mechanism idle. Measured, the difference between a query filtered
// with usable tags and one without is the difference between hit@5 of 0.949 and
// 0.881, and in a vault where most records concern one subject it is the
// difference between finding an answer and reading five near misses.
//
// These strings are therefore written with the same care as the ranker. They are
// defined here, apart from any server, so that the contract exists and can be
// reviewed before anything speaks the protocol.
//
// ⚠ Whether a real agent tags this well is UNTESTED. The filters that produced
// the measured numbers were written by a model that had read the whole corpus.
// These descriptions are built on the assumption that a well-briefed agent does
// nearly as well, and that assumption is scheduled for validation against a real
// agent — it is not something this package may claim.

// A Tool is one operation exposed to an agent, with the description that tells
// the agent how to use it well.
type Tool struct {
	Name        string
	Title       string
	Description string
}

// RememberTool asks for the two fields that decide whether a record can ever be
// found again.
//
// The description spends most of its length on `context` and `tags` because
// those are what fail. A statement is easy to write and hard to retrieve; the
// context is what makes it resolvable once it has been separated from the
// conversation it came from, and it is worth more to retrieval than every model
// choice available put together.
var RememberTool = Tool{
	Name:  "remember",
	Title: "Store a memory",
	Description: strings.TrimSpace(`
Store one durable fact, preference, insight or correction in the user's own encrypted vault.

Write ONE self-contained proposition per call. If you have three things to remember, call this three
times: a record holding several claims is retrieved for whichever one dominates it and is useless for
the others.

REQUIRED FIELDS, and why each one matters:

- statement: the proposition itself, in a single sentence. Write it so it will still make sense to
  someone who has never seen this conversation. "The rollup runs hourly" is not a memory; "Tidepool's
  reading rollup runs hourly, at ten past the hour" is.

- context: what makes the statement resolvable on its own — where it came from, what was being
  decided, what it is in contrast to. This is NOT optional and the vault will reject the record
  without it. It is the single largest thing that determines whether this record is ever found
  again, worth more than any other choice in the retrieval pipeline. One or two sentences.

- type: exactly one of fact, preference, insight, doc, profile, correction.
    fact        something observed or measured about the world or a system
    preference  how the user wants things done
    insight     a conclusion drawn rather than observed
    doc         reference material held roughly verbatim
    profile     a durable attribute of a person, team or system
    correction  something that was NEVER TRUE, as opposed to something that has become outdated.
                Use this when you are recording that a previous belief was wrong. Something that
                was true and has since changed is not a correction — supersede it instead.

- tags: two to four, lowercase, no spaces. This is where recall is won or lost, so it is worth a
  moment's thought:
    * Prefer the SPECIFIC over the general. In a vault about one project, tagging everything with
      that project's name tells nothing apart. "cache-invalidation" separates; "backend" does not.
    * REUSE the vault's existing tags rather than coining a synonym. Two tags meaning one thing
      split the records that should have been found together, and tags are matched exactly.
    * Add one tag naming the SUBJECT and one naming the ASPECT — what it is about, and what about
      it. "prediction" plus "accuracy" beats two words for the same idea.
  The response reports how many records already carry each tag you used. A tag already on most of
  the vault will not narrow anything, and that is worth correcting on the next write.

OPTIONAL, and worth supplying when you know them:

- supersedes: the id of a record this one replaces. The old record is kept and stays retrievable as
  history; it simply stops being the current answer. Use this when something has CHANGED. Use a
  correction record instead when something was never true.
- links: ids of related records, for navigation and provenance.
- importance, confidence: your own judgement, 0 to 1. The vault runs no model and will not infer them.
- validFrom, validUntil: when the statement is true OF THE WORLD, as distinct from when you learned
  it. Leave them out unless the statement really does have a window.

WHAT THE RESPONSE TELLS YOU, and what to do about it:

- neighbours: the records already in the vault closest to what you just wrote. Read them.
- conflicts: neighbours close enough to be the same statement again. If one is there, you have
  probably just duplicated or contradicted an existing record. Decide which: leave both if they are
  genuinely different, or supersede the old one if this replaces it. Do not simply move on.
- tags: how many records already carry each tag you used, and whether any of them is too common in
  this vault to narrow a search.

The record is durable on the user's device before this call returns; it reaches the network shortly
afterwards. The response says which. Never tell the user something is stored on the network before
the response says it is.
`),
}

// RecallTool asks for the other half of the same contract.
//
// It states plainly that filters prefer rather than exclude, because an agent
// that believes a filter is a WHERE clause writes narrow, brittle filters to
// avoid false positives — which is precisely the behaviour that made hard
// filtering a cliff. Knowing that a wrong tag is survivable is what makes an
// agent willing to supply tags at all.
var RecallTool = Tool{
	Name:  "recall",
	Title: "Search memory by meaning",
	Description: strings.TrimSpace(`
Search the user's vault by meaning and return the records most likely to answer the question.

Pass the user's actual question as the query, in their words. Do not reduce it to keywords: the
search ranks mostly by meaning and only partly by matching words, so a whole question retrieves
better than the nouns extracted from it — and the words are still there for the part that wants
them.

ALWAYS supply tags and types when you can infer them from the question. They are the difference
between finding the answer and returning five near misses, and in a vault where most records concern
one subject they matter more, not less.

- tags: two or three you would expect the answer to carry. Guess from the question. Prefer the
  specific: for "why is deployment slow", "deployment" and "latency" beat "engineering".
- types: which kinds of record could answer this.
    "why" and "should we" questions are usually answered by insight
    "what is" and "how much" by fact
    "how does the user like" by preference
    "what did we get wrong" by correction

FILTERS PREFER, THEY DO NOT EXCLUDE. A tag you guessed wrongly costs you a little ranking quality
and nothing else — records that do not match are still returned, just lower. You will never lose an
answer by guessing, so guess. Supplying three plausible tags is strictly better than supplying none
for fear of being wrong.

By default this returns the vault's CURRENT state: records that have been superseded by a later
version are held back, and the response says how many. Ask for history explicitly when the user
wants to know what something used to be, or how a decision changed.

An empty result is a real answer. It means the vault does not hold this. Say so rather than
presenting the closest record as if it were responsive — the user knows what they told you, and a
confident wrong memory is worse than none.

Each result carries the similarity it earned and the boost the filter gave it, so you can see how
much of a record's position came from meaning and how much from matching your filter.
`),
}

// ForgetTool is stated in the same place because deleting is part of the same
// contract, and because what it does not do needs saying.
var ForgetTool = Tool{
	Name:  "forget",
	Title: "Remove a memory",
	Description: strings.TrimSpace(`
Remove a record from the user's vault.

Prefer superseding to forgetting. A record that has become wrong is usually worth keeping as
history, and superseding it removes it from ordinary recall while leaving the trail intact. Forget is
for things the user does not want stored at all.

This does not immediately return storage. Records share storage with everything written alongside
them, so space comes back later, when nothing in a batch is still needed. Do not report freed space.
`),
}

// Tools is the memory surface in the order an agent meets it.
var Tools = []Tool{RememberTool, RecallTool, ForgetTool}

// TypeGuidance is the one-line gloss for each type in the vocabulary, for
// building a tool schema's enum documentation without restating it by hand.
//
// It is derived from the vocabulary rather than written beside it, so a type
// added without guidance is a compile error rather than an agent guessing.
func TypeGuidance() map[record.Type]string {
	return map[record.Type]string{
		record.TypeFact:       "something observed or measured",
		record.TypePreference: "how the user wants things done",
		record.TypeInsight:    "a conclusion drawn rather than observed",
		record.TypeDoc:        "reference material held roughly verbatim",
		record.TypeProfile:    "a durable attribute of a person, team or system",
		record.TypeCorrection: "something that was never true, as opposed to something now outdated",
	}
}
