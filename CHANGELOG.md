# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.1.0] - 2026-05-04

### Added
- `agnostic-ai init --from claude`: import existing `CLAUDE.md`, `.claude/agents/`, `.claude/skills/`, and `.claude/settings.json` into agnostic source specs. Lowers adoption friction for users with populated Claude Code configs.
- `agnostic-ai sync --check` and `agnostic-ai doctor`: detect drift between source specs and emitted target files. Use as CI gate.
- `x-<target>` frontmatter namespace: per-adapter extensions (e.g. `x-claude:`, `x-cursor:`) flatten for the matching target and are stripped for others.
- Initial Go scaffold.
- Adapters for Claude Code, Codex, Gemini CLI, Cursor, GitHub Copilot, Aider, Cline, Windsurf, Continue.
- Commands: `init`, `sync`, `validate`, `list`.
- User and contributor docs under `docs/`.
- OSS community files: CONTRIBUTING, CODE_OF_CONDUCT, GOVERNANCE.
- GitHub issue/PR templates, CI and release workflows, GoReleaser, golangci-lint config.
- `Dockerfile` (distroless, static) and `.dockerignore`.
- `lefthook.yml` for git hooks (gofmt, golangci-lint, conventional-commits, pre-push tests).
- `Taskfile.yml` for [go-task](https://taskfile.dev) users.
- `.editorconfig` and `.gitattributes`.
- `.github/CODEOWNERS` for PR review routing.
- `renovate.json` for dependency updates.
- Integration tests under `tests/integration/`.
- Dogfood specs at project root (`agents/`, `skills/`, `rules/`, `hooks/`).
- `docs/examples/agnostic.config.yaml` covering every config knob.

[Unreleased]: https://github.com/Chemaclass/agnostic-ai/compare/HEAD...HEAD
