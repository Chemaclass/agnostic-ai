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
- [Claude Code](@/docs/targets/claude.md): `setup` runs from generated hooks; see [Claude Code worktree setup](#claude-code-worktree-setup). `dev-commands` goes to `.claude/launch.json`, where Claude Code reads a relative `cwd` from the project root, so the spec needs no `${workspaceFolder}`. Every other field, `setup-windows` and `cleanup` included, gets a no-effect note.
- [OpenHands](@/docs/targets/openhands.md) and [Amp](@/docs/targets/amp.md): `install` becomes a setup script. Amp also turns `terminals` into services. Both note `setup`, `cleanup`, and `dev-commands` as having no effect.
- Other targets report the spec as unsupported.

## Claude Code worktree setup {#claude-code-worktree-setup}

Claude Code has no setup step for a worktree it creates ([worktrees](https://code.claude.com/docs/en/worktrees#set-up-the-worktree-environment)), so sync writes `setup` into `.claude/hooks/agnostic-ai-worktree-setup.sh` and runs that script from three hooks in `.claude/settings.json`:

- `SessionStart` with matcher `startup`, for `claude --worktree` and Desktop worktree sessions.
- `SubagentStart`, for a subagent with `isolation: worktree`. `SessionStart` does not fire for a subagent.
- `PostToolUse` with matcher `EnterWorktree`, for a worktree Claude enters during a session.

Each hook sets `shell: bash`, so on Windows the script needs Git Bash; without it the hook fails and setup does not run. A checkout without the script, such as a worktree created before the first sync, skips the hook.

Claude Code runs matching hooks in parallel. Delete a hand-written `SessionStart` hook that installs the same dependencies, or it runs alongside setup.

The script reads the worktree from the hook payload's `cwd`, since `$CLAUDE_PROJECT_DIR` stays at the directory the session started in. It runs `setup` from the worktree root through `sh`, only in a linked Git worktree and never in the main checkout. A successful run leaves a marker in the worktree's own Git directory, so later sessions in that worktree skip setup; a failed run leaves none, and the next new session, subagent, or worktree entry tries again. A linked worktree created before the first sync with `setup` has no marker either, so setup runs there once too. Claude Code stops a command hook after 600 seconds by default, so a longer setup fails and runs again next time. Setup output goes to stderr, because Claude Code adds a `SessionStart` hook's stdout to the session context. The script exits when `AGNOSTIC_AI_TARGET` is not `claude`, so a tool that also reads `.claude/settings.json` hooks, such as Cursor, does not run setup a second time.

A new worktree needs `.claude/settings.json` and the script. With `gitignore.enabled`, both are ignored, and `.worktreeinclude`, which sync keeps when `claude` is a target, copies them into each worktree Claude Code creates (see [gitignore](@/docs/configuration.md#gitignore)). Without it, both files are committed, so the checkout has them.

To keep `setup` for the other tools and bootstrap Claude Code another way, turn the hooks off:

```yaml
setup: composer install
x-claude:
  setup: false
```

`cleanup` has no Claude Code equivalent. Claude Code documents `WorktreeRemove` as the removal step for a worktree a `WorktreeCreate` hook made, and `SessionEnd` runs at the end of every session with a 1.5-second budget.

## Import

When several tools keep their own environment file, `import` scopes each spec it writes to its tool with `targets: [<tool>]`, so the next sync reproduces every file as it was instead of merging one tool's dev commands or setup into the others. Remove the line to share a spec across tools. A project with one tool's environment file gets one shared spec.
