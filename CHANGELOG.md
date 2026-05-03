# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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
