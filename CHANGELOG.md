# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Skill emission: rules-directory adapters (Cursor, Cline, Windsurf, Continue) now write each skill as its own rule file. Merged-document adapters (Codex, Gemini, Copilot, Aider) list skills in a `## Skills` section with name, description, and source path.
- Merged-document outputs (`AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, `.github/copilot-instructions.md`) carry a generated-by header so downstream tools and humans see they are produced by `agnostic-ai sync`.
- `internal/testutil` package with `Chdir` and `TempCwd` helpers for tests that change the process working directory.
- `.github/dependabot.yml`: weekly updates for `gomod` and `github-actions`.
- `SECURITY.md`: vulnerability reporting via GitHub private security advisories, 90-day disclosure.
- Roadmap doc (`docs/internal/roadmap.md`) covering the planned user-global and project-user configuration layers.

### Changed
- README simplified: install, quickstart, build-from-source, commands, and spec-format details moved into `docs/`. README now links into the docs tree.
- README aligned with the AGENTS.md open standard and lists the standards the project rides on.
- `internal/cli/import_claude.go` (308 LOC) split into one file per concern: rules, skills+agents, hooks. `root.go` command builders moved into `sync.go`, `validate.go`, `list.go`, `init.go`. No behavior change.
- CI test step writes a coverage profile and uploads it to Codecov from the Ubuntu runner.
- `make build` now passes `-trimpath -ldflags="-s -w"` for reproducible, smaller binaries.
- `Makefile` adds convenience targets: `lint`, `fmt`, `vet`, `cover`.
- Codex `routeDir` and nested-glob tests converted to `t.Run` subtests for cleaner failure reports.

### Fixed
- CI Test step on the Windows runner: pinned to `bash` so PowerShell stops mis-parsing `-coverprofile=coverage.out`.

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
