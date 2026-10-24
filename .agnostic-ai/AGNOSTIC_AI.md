# agnostic-ai development

agnostic-ai emits native AI tool configuration from portable specs. Go code: `cmd/`, `internal/`. Docs site: `docs/site/`. Editor extensions: `editors/`.

Edit canonical specs in `.agnostic-ai/` and `agnostic-ai.yaml`, then run `go run ./cmd/agnostic-ai sync`. Native files such as `AGENTS.md` and `CLAUDE.md` are generated and ignored; `.openhands/setup.sh` is generated but tracked. Review tracked output before committing.

Paths in these instructions are repository-root relative, including from nested generated files. Start with `CONTRIBUTING.md`; `docs/internal/contributing.md#choose-checks-for-your-change` owns validation selection. Read `docs/internal/adding-adapters.md` for adapter work. Run checks after the change set is complete; releases require `make ci-local`.

Delegate independent work in parallel with disjoint ownership. Use fast models for lookups, midrange for bounded changes, and higher reasoning for design, debugging, and verification. Coordinate expensive checks through one caller.
