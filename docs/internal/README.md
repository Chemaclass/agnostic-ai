# Contributor docs

Read in this order.

1. [Architecture](architecture.md): code layout, data flow, core types. Start here.
2. [Adding an adapter](adding-adapters.md): concrete walkthrough; ~50 lines plus one registry entry.
3. [Plugin protocol](plugin-protocol.md): JSON-over-stdio contract for out-of-tree adapter binaries.
4. [Contributing](contributing.md): branch flow, commit style, test expectations.
5. [Release process](release-process.md): how a tag becomes a release.
6. [Contributing a preset](contributing-presets.md): adding a new `init --preset <name>` starter pack.
7. [Decision log](decisions.md): historical record of design choices.
8. [Roadmap](roadmap.md): planned directions (layered config, more adapters).

## House rules

- One concern per PR. Refactors go in their own PR.
- Adapter packages stay independent; share via `internal/adapters/internal/emit/`.
- `gofmt` clean, `goimports` for grouping, no external deps without justification.
- Test names describe behavior (`TestEmit_WritesAgentFile`, not `TestEmit1`).

## Local CI

```bash
make test            # unit + integration
go run ./cmd/agnostic-ai sync --check    # dogfood: verify our own outputs are in sync
```
