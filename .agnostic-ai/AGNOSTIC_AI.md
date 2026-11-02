# agnostic-ai development

agnostic-ai emits native AI tool configuration from portable specs. Go code: `cmd/`, `internal/`. Docs site: `docs/site/`. Editor extensions: `editors/`.

Edit canonical specs in `.agnostic-ai/` and `agnostic-ai.yaml`, then run `go run ./cmd/agnostic-ai sync`. Native files such as `AGENTS.md` and `CLAUDE.md` are generated and ignored; `.openhands/setup.sh` is generated but tracked. Review tracked output before committing.

Paths here are repository-root relative. Start with `CONTRIBUTING.md`; `docs/internal/contributing.md#choose-checks-for-your-change` picks the checks. Read `docs/internal/adding-adapters.md` for adapter work. Run checks once the change set is complete; releases need `make ci-local`.

Delegate independent work with disjoint ownership: fast models for lookups, midrange for bounded changes, high reasoning for design, debugging and verification. One caller runs expensive checks.
