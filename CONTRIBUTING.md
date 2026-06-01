# Contributing to agnostic-ai

```bash
git clone https://github.com/Chemaclass/agnostic-ai
cd agnostic-ai
make build && make test
```

Open a PR with a [Conventional Commits](https://www.conventionalcommits.org/) subject.

## Where to start

| Goal | Link |
|---|---|
| Report a bug | [Bug issue](.github/ISSUE_TEMPLATE/bug_report.yml) |
| Request a feature | [Feature issue](.github/ISSUE_TEMPLATE/feature_request.yml) |
| Add a new AI CLI target | [Adapter issue](.github/ISSUE_TEMPLATE/new_adapter.yml) · [adding-adapters.md](docs/internal/adding-adapters.md) |
| Deep dive | [docs/internal/](docs/internal/) (architecture, contributing, decisions) |

## AI config is generated (dogfooding)

This repo drives its own AI config with agnostic-ai. The source of truth is `.agnostic-ai/`. Every target output is **gitignored** and regenerated, never committed:

- Per-target folders: `.claude/`, `.cursor/`, `.gemini/`, `.codex/`, `.windsurf/`, `.continue/`, `.clinerules/`, `.opencode/`, `.agent/`.
- Root entry-points: `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, `.rules`, `.github/copilot-instructions.md`.

After editing any spec under `.agnostic-ai/`, regenerate locally:

```bash
agnostic-ai sync   # or: go run ./cmd/agnostic-ai sync
```

Do not edit the generated files directly and do not commit them; they are overwritten on each sync. CI runs `agnostic-ai check` to fail drift.

## PR rules

- Conventional Commits (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`).
- One concern per PR. Tests for behavior changes.
- Update `CHANGELOG.md` under `[Unreleased]` for user-visible changes.

## Other

- [Code of Conduct](CODE_OF_CONDUCT.md) · [MIT license](LICENSE), no CLA.
- General questions: [Discussions](https://github.com/Chemaclass/agnostic-ai/discussions).
- Security: [private advisory](https://github.com/Chemaclass/agnostic-ai/security/advisories/new). Do not file public issues.

Triage target: 7 days. Ping after two weeks of silence.
