# agnostic-ai development

agnostic-ai is a Go CLI that turns portable specs into native configuration for AI coding tools. The CLI lives under `cmd/` and `internal/`; the docs site is under `docs/site/`, and editor extensions are under `editors/`.

This repository uses agnostic-ai for its own agent setup. Tracked source lives in `.agnostic-ai/` and `agnostic-ai.yaml`. Native files such as `AGENTS.md` and `CLAUDE.md` are generated and ignored. `.openhands/setup.sh` is a tracked setup script that sync generates. Edit source specs, run `go run ./cmd/agnostic-ai sync`, and review tracked output changes before committing.

Paths in these instructions are relative to the repository root, including when a tool reads a generated file from a nested directory. Start with `CONTRIBUTING.md` for setup and checks. For adapter changes, read `docs/internal/adding-adapters.md`. Run the relevant checks after a change set is complete; `make ci-local` is the full check that must pass before a release.

When you hand work to a subagent, pick the cheapest model that keeps the quality: a small fast model (Claude Haiku) for lookups and mechanical edits, a mid model (Claude Sonnet) for bounded research, reviews, and doc rewrites, and the main model for design, debugging, and checking a delegate's result before you act on it. Run independent delegates in parallel.
