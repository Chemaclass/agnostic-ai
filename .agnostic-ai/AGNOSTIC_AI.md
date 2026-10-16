# agnostic-ai development

agnostic-ai is a Go CLI that turns portable specs into native configuration for AI coding tools. The CLI lives under `cmd/` and `internal/`; the docs site is under `docs/site/`, and editor extensions are under `editors/`.

This repository uses agnostic-ai for its own agent setup. Tracked source lives in `.agnostic-ai/` and `agnostic-ai.yaml`. Native files such as `AGENTS.md` and `CLAUDE.md` are generated and ignored. `.openhands/setup.sh` is a tracked setup script that sync generates. Edit source specs, run `go run ./cmd/agnostic-ai sync`, and review tracked output changes before committing.

Paths in these instructions are relative to the repository root, including when a tool reads a generated file from a nested directory. Start with `CONTRIBUTING.md` for setup and checks. For adapter changes, read `docs/internal/adding-adapters.md`. Run the relevant checks after a change set is complete; `make ci-local` is the full check that must pass before a release.

Match delegated work to the smallest capable model: fast for lookups, midrange for bounded changes, and higher reasoning for design, debugging, and verification. Run independent delegates in parallel.
