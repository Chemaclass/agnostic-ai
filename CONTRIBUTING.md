# Contributing to agnostic-ai

```bash
git clone https://github.com/Chemaclass/agnostic-ai
cd agnostic-ai
make build && make test
```

## PR rules

- [Conventional Commits](https://www.conventionalcommits.org/) subject: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`.
- One concern per PR. Tests for behavior changes.
- User-visible change: add an entry under `[Unreleased]` in `CHANGELOG.md`.

## Where to start

| Goal | Link |
|---|---|
| Report a bug | [Bug issue](.github/ISSUE_TEMPLATE/bug_report.yml) |
| Request a feature | [Feature issue](.github/ISSUE_TEMPLATE/feature_request.yml) |
| Add a new AI CLI target | [Adapter issue](.github/ISSUE_TEMPLATE/new_adapter.yml) · [adding-adapters.md](docs/internal/adding-adapters.md) |
| Deep dive | [docs/internal/](docs/internal/): architecture, decisions, release process |

## AI config is generated

This repo drives its own AI config with agnostic-ai. `.agnostic-ai/` is the source of truth. Every target output is gitignored and regenerated: per-target folders (`.claude/`, `.cursor/`, `.codex/`, ...) and root entry-points (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, ...). The `.gitignore` block lists them all.

After editing a spec, regenerate:

```bash
agnostic-ai sync   # or: go run ./cmd/agnostic-ai sync
```

Never edit or commit a generated file. `sync` overwrites it, and CI runs `sync --check` to fail on drift.

That `.gitignore` block is hand-maintained, not driven by `gitignore.enabled`: it carries hierarchical globs (`**/AGENTS.md`) plus the `!internal/adapters/*/testdata/**` re-allow that keeps golden fixtures tracked. A new adapter needs its output paths added by hand.

## Questions and reports

- General questions: [Discussions](https://github.com/Chemaclass/agnostic-ai/discussions).
- Security: [private advisory](https://github.com/Chemaclass/agnostic-ai/security/advisories/new), never a public issue.
- [Code of Conduct](CODE_OF_CONDUCT.md) · [MIT license](LICENSE), no CLA.

Triage target: 7 days. Ping after two weeks of silence.
