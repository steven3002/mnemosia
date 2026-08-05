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

### Notes

Design and feasibility work completed against the **live Sia network**. Findings that shaped the architecture:

- **Storage is viable.** End-to-end encrypted round-trip is byte-exact; cold read p50 ~210 ms, warm ~42 ms; storage cost measured in the low single digits of dollars per TiB per month.
- **Small records must be packed.** A storage slab is billed whole regardless of how little it holds, and a partially-filled slab can never be extended, so writes are batched into shared slabs, and periodic repack is a requirement rather than housekeeping.
- **Writes parallelise.** Committing a thousand records went from ~361 s to ~4.8 s by pinning slabs once per flush and objects concurrently.
- **Recovery does not depend on the index.** Records are self-describing on disk, so a full vault can be reconstructed from the recovery phrase and the indexer alone.
- **Retrieval quality.** Semantic recall reached 0.949 hit@5 on a labelled corpus with soft metadata filtering. Filtering must *boost* rather than *exclude*, hard filtering fails badly on a single wrong tag. Recall degrades with topical *homogeneity* rather than corpus size.

[Unreleased]: https://github.com/steven3002/mnemosia
