# Mnemosia

**User-owned storage for an AI's memory, sessions, and skills — encrypted, on the [Sia](https://sia.tech) network.**

Mnemosia gives an AI agent a long-term memory that belongs to **you** rather than to a vendor: encrypted on your device, stored on decentralized infrastructure that cannot read it, retrieved by meaning, and portable across apps, models and machines.

> **Status: pre-alpha, in active development.** The design is settled and the storage layer is validated against the live Sia network, but **there is no usable release yet.** Nothing here is production-ready. See [Status](#status).

---

## Why

An AI assistant's memory is trapped. It lives inside one provider, it can't move with you, and you can't inspect, own, or truly export it. To have it available everywhere, you normally hand it to a cloud that can read it.

Mnemosia takes the other path:

- **You own it.** Keys are derived from your recovery phrase and never leave your device.
- **Nobody can read it.** Records are encrypted client-side before they touch the network; storage providers and the coordinating indexer see only ciphertext.
- **It's portable.** Memory follows you across agents, tools and machines — not locked to one vendor.
- **It's searched by meaning.** Semantic recall over your own records, computed locally.

This is **storage plumbing**, not an AI assistant. "AI" describes the *data being stored* — AI apps and agents are consumers of it.

## How it works

Three record types on one encrypted, content-addressed, versioned substrate:

| Record | What it holds | Retrieved by |
|---|---|---|
| `memory` | facts, preferences, learned context | **semantic** (vector) search |
| `session` | past conversations you can resume | metadata + version |
| `skill` | reusable procedures an agent can load | name + version |

Agents talk to it over **[MCP](https://modelcontextprotocol.io)** (Model Context Protocol), so any MCP-capable client can use the same memory.

**The search never leaves your machine.** Query embedding and vector search run locally against a local index; only opaque fetches of already-identified records hit the network. Nobody learns what you searched for.

```
remember ──▶ embed + encrypt locally ──▶ pack ──▶ Sia
recall   ──▶ embed query locally ──▶ local vector search ──▶ fetch those records ──▶ decrypt locally
```

## Status

Design and feasibility work is complete and measured against the live network. Implementation of the product itself has not started.

**Validated against live Sia:**
- End-to-end round-trip, byte-exact, with client-side encryption
- Interactive read latency (p50 ~210 ms cold; ~42 ms warm)
- Packed writes — many small records share one storage slab
- Storage reclamation and repack correctness
- Semantic recall quality on a labelled evaluation corpus

**Not yet built:** the substrate library, the MCP server, the CLI, and the local viewer.

There is **no install path yet.** Watch the repository or the [CHANGELOG](CHANGELOG.md) for the first release.

## Design principles

1. **Storage consumer, not AI operator** — Mnemosia puts data *on* Sia; it does not drive Sia with an LLM.
2. **Client-side confidentiality** — encryption, keys and search stay on the device.
3. **User-owned and portable** — memory follows the user across apps, models and devices.
4. **With the grain of Sia** — built on the first-party SDK and indexer.
5. **No LLM in our stack** — Mnemosia embeds, stores, indexes and ranks. The calling agent decides what is worth remembering.

## Built on

- [Sia](https://sia.tech) — decentralized storage with client-side encryption and user-held keys
- [`go.sia.tech/siastorage`](https://pkg.go.dev/go.sia.tech/siastorage) — the first-party Go SDK
- [Model Context Protocol](https://modelcontextprotocol.io) — the open standard for connecting data to AI applications

## Contributing

Early and in flux — the architecture is settled but the code isn't written. See [CONTRIBUTING.md](CONTRIBUTING.md). Bug reports and design discussion are welcome; please open an issue before a large pull request.

## Security

Mnemosia handles encryption keys and personal data. Please report vulnerabilities responsibly — see [SECURITY.md](SECURITY.md). Do not open public issues for security problems.

## License

[MIT](LICENSE) — matching the Sia SDK and the wider Go ecosystem.
