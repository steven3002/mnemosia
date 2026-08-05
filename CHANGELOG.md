# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Pre-alpha. No release yet. The storage substrate and a command-line interface over it exist; the MCP
server and the viewer do not.

### Added
- **Storage substrate.** Records are sealed on the device, batched, and written to Sia in shared
  slabs. The write queue is held on disk, so a record that has been accepted but not yet written to
  the network survives the process that accepted it, and an offline write is owed to the network
  rather than silently kept local.
- **Catalog as an append-only log with periodic snapshots**, compacted on a size ratio. Total bytes
  written stay proportional to the number of records instead of to its square.
- **Recovery from the recovery phrase and the indexer alone.** Every stored record carries its own
  identity beside its ciphertext, so a vault that has lost its catalog can be rebuilt completely.
  Damage is bounded: a truncated or corrupted object yields the records before the break, and the
  authenticated envelope rejects anything that parses but is not what it claims to be.
- **Three-tier reads** — the device's own copy, a cached storage location, and a cold fetch — with
  every read counted by the tier that served it.
- **Reclamation.** Storage that nothing points at is released, including storage stranded by an
  installation that no longer exists. Repack rewrites live records into fewer slabs and runs only
  when asked.
- **Semantic recall with soft metadata filtering.** Tags and a record type supplied with a query
  *prefer* matching records; they never exclude any. A tag guessed wrongly costs a little ranking
  quality and nothing else, which is deliberate — as an exclusion, one wrong tag was measured
  returning nothing at all for a sixth of queries.
- **Hybrid ranking: a full-text index beside the vector one.** Three signals decide the order, in
  one stated sequence — the vector pass ranks the candidate pool by meaning, a BM25 pass reranks it
  by the words the query used, and the metadata filter boosts what the caller asked for. The two
  passes are combined by weighted rank fusion, so no score scale has to be reconciled between them,
  and a record either pass found on its own still ranks.
  The lexical weight is small on purpose. It was chosen as the largest value that costs a query
  sharing *no* word with its answer nothing at all — the case a memory store exists for, and the one
  a term index cannot help and can actively harm. Measured against that criterion, the lexical pass
  buys precision near the top of the list (hit@1 0.627 → 0.678) rather than a better hit@5, and the
  configuration that ships outranks giving the two passes an equal vote on every metric measured.
- **Context is required on every record**, and a record without it is rejected rather than stored
  with a default. It is the single largest measured contributor to whether a record can be found
  again, and records are immutable, so a missing context cannot be repaired afterwards.
- **Supersession.** A record can replace an earlier one. The replaced version stops being the
  current answer and stays retrievable as history.
- **Write-time feedback.** Storing a record reports the records nearest to it, flags any close
  enough to be a restatement, and says how many records already carry each tag used — so a tag too
  common to narrow a search is visible while it is still cheap to change.
- **The search index persists** as an immutable base plus appended deltas, folded together on a size
  ratio, so restarting does not mean re-deriving every vector.
- **A mixed-model index is detected.** Vectors from two embedding models cannot be compared, and
  comparing them yields a plausible number rather than an error; vectors from another model are
  refused, counted, and reported instead of silently searched.
- **A recall regression suite** over a small committed corpus, reporting hit@k and mean reciprocal
  rank on every run, alongside two axes that an aggregate hides: how crowded each query's
  neighbourhood is, and which part of the vault its answers live in.
- **Commands:** `init`, `remember`, `recall`, `flush`, `status`, `reclaim`, `recover`.
- Project scaffolding: README, license, contribution guide, security policy, code of conduct.

### Notes on behaviour worth knowing

- **A saved record is not yet a stored record.** Writes are durable on the device immediately and
  reach the network on a flush. Nothing in the interface conflates the two.
- **Deleting a record frees no space by itself.** Records share a storage slab and a slab is billed
  whole, so space returns when a slab has nothing live left in it and reclamation runs.
- **Repack is manual.** It holds the old and new storage at the same time, and its behaviour under
  concurrent writes and under interruption has not been measured, so nothing triggers it
  automatically.
- **Retrieval quality depends on what else is in the vault.** A vault where nearly everything
  concerns one subject is measurably harder to search than a varied one, because the records
  competing with the right answer share its tags and the filter cannot tell them apart. Quoted
  figures below say which case they describe.
- **Recovering a vault re-derives every vector**, at roughly 0.45 s per record, so a large vault
  takes a while to become searchable again after a recovery. The records themselves come back first.

### Notes

Design and feasibility work completed against the **live Sia network**. Findings that shaped the architecture:

- **Storage is viable.** End-to-end encrypted round-trip is byte-exact; cold read p50 ~210 ms, warm ~42 ms; storage cost measured in the low single digits of dollars per TiB per month.
- **Small records must be packed.** A storage slab is billed whole regardless of how little it holds, and a partially-filled slab can never be extended, so writes are batched into shared slabs, and periodic repack is a requirement rather than housekeeping.
- **Writes parallelise.** Committing a thousand records went from ~361 s to ~4.8 s by pinning slabs once per flush and objects concurrently.
- **Recovery does not depend on the index.** Records are self-describing on disk, so a full vault can be reconstructed from the recovery phrase and the indexer alone.
- **Retrieval quality, and which case each number describes.** On a labelled corpus with metadata
  filtering, semantic recall reached **0.949 hit@5 in a vault of varied subject matter**. In a vault
  of ten thousand records all concerning one subject, the same pipeline holds **0.797**. Filtering
  helps in both cases and helps most where the competition is thickest, but it does not close the
  gap. Both figures are the same measurement reported honestly; the first alone would not be.
- **Filtering must prefer rather than exclude.** As an exclusion, a single wrong tag took hit@5 from
  0.949 to 0.729 — below not filtering at all — and returned nothing whatever for a sixth of
  queries. As a preference, the same wrong filter scores exactly what no filter scores.
- **Recall is uneven inside a single vault**, and an average conceals it: the queries with the most
  competing records scored 0.690 against 0.933 for the rest of the same vault. Quality is reported
  per query as well as in aggregate for that reason.

[Unreleased]: https://github.com/steven3002/mnemosia
