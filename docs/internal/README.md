# Contributor docs

1. [Architecture](architecture.md) — code layout, data flow, core types.
2. [Adding an adapter](adding-adapters.md) — concrete walkthrough.
3. [Plugin protocol](plugin-protocol.md) — JSON-over-stdio contract for out-of-tree adapter binaries.
4. [Contributing](contributing.md) — dev setup, branch flow, test expectations.
5. [Release process](release-process.md) — how a tag becomes a release.
6. [Contributing a preset](contributing-presets.md) — adding an `init --preset <name>` pack.
7. [Decision log](decisions.md) — historical design choices.
8. [Roadmap](roadmap.md) — planned directions.

## House rules

- One concern per PR. Refactors get their own PR.
- Adapter packages stay independent; share via `internal/adapters/internal/emit/`.
- `gofmt` clean, `goimports` for grouping, stdlib first.
- Test names describe behavior (`TestEmit_WritesAgentFile`, not `TestEmit1`).

## Local CI

```bash
make preflight                            # fmt-check + vet + lint + test
go run ./cmd/agnostic-ai sync --check     # dogfood: our own outputs are in sync
```
