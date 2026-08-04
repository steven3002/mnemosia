# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Pre-alpha. No release yet — the substrate library, MCP server, CLI and viewer are not implemented.

### Added
- Project scaffolding: README, license, contribution guide, security policy, code of conduct.

### Notes

Design and feasibility work completed against the **live Sia network**. Findings that shaped the architecture:

- **Storage is viable.** End-to-end encrypted round-trip is byte-exact; cold read p50 ~210 ms, warm ~42 ms; storage cost measured in the low single digits of dollars per TiB per month.
- **Small records must be packed.** A storage slab is billed whole regardless of how little it holds, and a partially-filled slab can never be extended — so writes are batched into shared slabs, and periodic repack is a requirement rather than housekeeping.
- **Writes parallelise.** Committing a thousand records went from ~361 s to ~4.8 s by pinning slabs once per flush and objects concurrently.
- **Recovery does not depend on the index.** Records are self-describing on disk, so a full vault can be reconstructed from the recovery phrase and the indexer alone.
- **Retrieval quality.** Semantic recall reached 0.949 hit@5 on a labelled corpus with soft metadata filtering. Filtering must *boost* rather than *exclude* — hard filtering fails badly on a single wrong tag. Recall degrades with topical *homogeneity* rather than corpus size.

[Unreleased]: https://github.com/steven3002/mnemosia
