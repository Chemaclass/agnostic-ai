+++
title = "Environments"
description = "environments/: how an agent sets up a worktree or sandbox, starts dev servers, and cleans up."
weight = 90

[extra]
group = "Reference"
+++

# Environments

`environments/` tells a coding tool how to get the project running in a place it just created: a new Git worktree, a cloud sandbox, a preview. Install dependencies, copy local config, start the dev server, stop it afterwards. Each tool has its own file for this, such as `.cursor/worktrees.json`, `.codex/environments/environment.toml`, or `.claude/launch.json`.

- **Parallel agents that work.** Every fresh worktree runs the same setup, so an agent does not start on a tree with no dependencies.
- **Preview buttons.** `dev-commands` become the dev servers a tool can start and preview.
- **Clean exits.** `cleanup` stops what setup started when the worktree goes away.
- **One spec for every tool.** Fields one tool ignores stay silent when another enabled tool reads them.

## Write one

Pure YAML, one file per environment group.

```yaml
name: dev
setup: bash scripts/setup-worktree.bash
cleanup: bash scripts/stop-preview.bash
dev-commands:
  - name: Docs
    command: [pnpm, dev:mintlify]
    cwd: apps/docs
    port: 3000
    auto-port: true
```

A sandbox that installs once and keeps a terminal open, as Cursor, OpenHands, and Amp read it:

```yaml
install: go mod download
terminals:
  - name: dev
    command: go run ./cmd/agnostic-ai
```

## Fields

- `setup`: the commands a tool runs in a new worktree (one command or a list). `setup-windows` replaces it on Windows.
- `cleanup`: the commands a tool runs when it removes the worktree.
- `install` and `terminals`: a sandbox's install step and its long-running processes.
- `dev-commands`: dev servers a tool can start and preview. Each entry needs a unique `name` and a `command` (a string or a list of words). `cwd` (relative to the project root), `port`, `auto-port`, `env`, `url`, and `icon` (the Codex button icon, `run` by default) are optional.

A string command with shell syntax (pipe, several lines, `VAR=value`, a builtin like `cd`) runs through `sh -c`, which on Windows needs a POSIX shell such as Git Bash on the `PATH`. A list runs with no shell. `env` values are written as text.

Specs merge by top-level key, and the last value wins. A field one tool ignores gets no no-effect note when another enabled tool reads it, and a field no enabled tool reads still gets one.

`lint` reports a dev command with no `name` or `command`, a repeated name, an unknown key, or a wrong-typed value (LINT016).

## Support by target

- [Cursor](@/docs/targets/cursor.md): `setup` and `setup-windows` go to `.cursor/worktrees.json`. The rest goes to `environment.json`, except the routing fields (`name`, `scope`, `target(s)`, `target(s)-exclude`, `description`) `dev-commands`, and `cleanup`, which get a no-effect note. `import cursor` reads both files back.
- [Codex](@/docs/targets/codex.md): `setup`, `setup-windows`, `cleanup`, and `dev-commands` go to `.codex/environments/environment.toml` as scripts and action buttons. Other fields get a no-effect note. `import codex` reads the file back.
- [Claude Code](@/docs/targets/claude.md): `dev-commands` goes to `.claude/launch.json`, where Claude Code reads a relative `cwd` from the project root, so the spec needs no `${workspaceFolder}`; every other field gets a no-effect note. Run worktree setup from a `WorktreeCreate` or `SessionStart` [hook](@/docs/spec-format/hooks.md) instead.
- [OpenHands](@/docs/targets/openhands.md) and [Amp](@/docs/targets/amp.md): `install` becomes a setup script. Amp also turns `terminals` into services. Both note `setup`, `cleanup`, and `dev-commands` as having no effect.
- Other targets report the spec as unsupported.

## Import

When several tools keep their own environment file, `import` scopes each spec it writes to its tool with `targets: [<tool>]`, so the next sync reproduces every file as it was instead of merging one tool's dev commands or setup into the others. Remove the line to share a spec across tools. A project with one tool's environment file gets one shared spec.
