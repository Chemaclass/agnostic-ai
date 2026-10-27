+++
title = "Environments"
description = "environments/: how an agent sets up a worktree or sandbox, starts dev servers, and cleans up."
weight = 90

[extra]
group = "Reference"
+++

# Environments

`environments/` defines project setup for a new Git worktree (a separate checkout of the same repository), cloud sandbox, or preview. It covers installing dependencies, copying local config, starting the dev server, and stopping it afterwards. Sync writes each tool's native file, such as `.cursor/worktrees.json`, `.codex/environments/environment.toml`, or `.claude/launch.json`.

## Write one

```bash
agnostic-ai new environment development
```

It creates `.agnostic-ai/environments/development.yaml` with no commands. Add the commands you have reviewed, and create any script they call before syncing:

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

A sandbox that installs once and keeps a terminal open (Cursor, OpenHands, Amp):

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
- `dev-commands`: dev servers a tool can start and preview. Each entry needs a unique `name` and a `command` (a string or a list of words). `cwd` (relative to the project root), `port`, `auto-port`, `env`, `url`, and `icon` (the Codex button icon, `run` by default) are optional. An entry with a `url` and no `command` tells Claude Code to open a server you already run instead of starting one; Codex has no such action, so sync skips the entry there with a note.

- A string command with shell syntax (pipe, several lines, `VAR=value`, a builtin like `cd`) runs through `sh -c`, which on Windows needs a POSIX shell such as Git Bash on the `PATH`. A list runs with no shell. `env` values are written as text.
- Specs merge by top-level key, and the last value wins. A field that no enabled tool uses gets a no-effect note.
- `lint` reports a dev command with no `name`, with neither `command` nor `url`, a repeated name, an unknown key, or a value of the wrong type (LINT016). It also reports a `url` entry with no `command` that reaches Codex, and names the target. Give Codex its own list without that entry under `x-codex.dev-commands`.

## Support by target

Fields a tool does not use get a no-effect note. Other tools report the spec as unsupported. Each tool's page has the key mapping.

| Target | Uses | Writes |
|---|---|---|
| [Cursor](@/docs/targets/cursor.md) | `setup`, `setup-windows`; every other key except `name`, `scope`, `description`, the target fields, `dev-commands`, and `cleanup` | `.cursor/worktrees.json`, `.cursor/environment.json` |
| [Codex](@/docs/targets/codex.md) | `setup`, `setup-windows`, `cleanup`, `dev-commands` (as scripts and action buttons) | `.codex/environments/environment.toml` |
| [Claude Code](@/docs/targets/claude.md) | `setup` ([worktree setup](#claude-code-worktree-setup)), `dev-commands` (a relative `cwd` reads from the project root, so no `${workspaceFolder}`) | `.claude/hooks/agnostic-ai-worktree-setup.sh`, `.claude/launch.json` |
| [OpenHands](@/docs/targets/openhands.md) | `install` (a setup script) | `.openhands/setup.sh` |
| [Amp](@/docs/targets/amp.md) | `install` (a setup script), `terminals` (services) | `.agents/setup`, `.amp/services.yaml` |
| [Ellipsis](@/docs/targets/ellipsis.md) | `install` as `hooks.build_base`, `setup` as `hooks.after_checkout`, each after `cd /sandbox/<repository>`, only for the environment `x-ellipsis.environment` selects | `.ellipsis/environments/<name>.yaml` |

`import cursor`, `import codex`, and `import ellipsis` read their files back. A successful install does not check the generated files: run the [project readiness checks](@/docs/cli-reference/project.md#project-readiness) after installation and sync, before starting a cached environment.

## Claude Code worktree setup {#claude-code-worktree-setup}

Claude Code has no setup step for a worktree it creates ([worktrees](https://code.claude.com/docs/en/worktrees#set-up-the-worktree-environment)), so sync writes `setup` into `.claude/hooks/agnostic-ai-worktree-setup.sh` and runs it from three hooks in `.claude/settings.json`:

- `SessionStart` with matcher `startup`, for `claude --worktree` and Desktop worktree sessions.
- `SubagentStart`, for a subagent with `isolation: worktree`. `SessionStart` does not fire for a subagent.
- `PostToolUse` with matcher `EnterWorktree`, for a worktree Claude enters during a session.

Each hook sets `shell: bash`, so on Windows the script needs Git Bash, or setup does not run. A checkout without the script, such as a worktree created before the first sync, skips the hook. Claude Code runs matching hooks in parallel, so delete a hand-written `SessionStart` hook that installs the same dependencies.

How the script runs:

- It takes the worktree from the `cwd` in the hook's event data, because `$CLAUDE_PROJECT_DIR` stays at the directory the session started in.
- It runs `setup` from the worktree root through `sh`, only in a linked Git worktree, never in the main checkout.
- A successful run leaves a marker in the worktree's own Git directory, so later sessions there skip setup. A failed run, or a worktree made before the first sync with `setup`, has no marker, so the next session, subagent, or worktree entry runs it.
- Claude Code stops a command hook after 600 seconds by default, so a longer setup fails and runs again next time.
- Setup output goes to stderr, because Claude Code adds a `SessionStart` hook's stdout to the session context.
- The script exits when `AGNOSTIC_AI_TARGET` is not `claude`. A tool that also reads the hooks in `.claude/settings.json`, such as Cursor, then does not run setup a second time.

### Refresh an existing worktree

After changing setup or dependencies, sync one worktree and rerun its Claude setup:

```bash
agnostic-ai hook worktree-refresh --repo /path/to/main --worktree "/path/to/linked worktree"
```

It resets only that linked worktree's marker and runs setup again; a failure leaves no marker. See [worktree refresh](@/docs/cli-reference/maintain.md#hook-worktree-refresh) for what it checks and refuses.

### Include setup in new worktrees

A new worktree needs `.claude/settings.json` and the script.

- With `gitignore.enabled`, both are ignored. `.worktreeinclude`, which sync keeps when `claude` is a target, copies them into each worktree Claude Code creates (see [gitignore](@/docs/configuration.md#gitignore)).
- Without it, both files are committed, so the checkout has them.

To keep `setup` for the other tools and set up Claude Code another way, turn the hooks off:

```yaml
setup: composer install
x-claude:
  setup: false
```

### Claude cleanup limits

`cleanup` has no Claude Code equivalent. Claude Code documents `WorktreeRemove` as the removal step for a worktree a `WorktreeCreate` hook made. `SessionEnd` runs at the end of every session with a 1.5-second budget.

## Import

When several tools keep their own environment file, `import` limits each spec to its tool with `targets: [<tool>]`, so the next sync writes every file back as it was. Remove the line to share a spec. A project with one tool's environment file gets one shared spec.
