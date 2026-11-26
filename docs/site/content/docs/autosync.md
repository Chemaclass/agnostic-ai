+++
title = "Sync on session start"
description = "Regenerate ignored tool files when a coding session starts, so a fresh clone, pull or new worktree never starts with missing or stale config."
weight = 66

[extra]
group = "Workflows"
+++

# Sync on session start

With [ignored outputs](@/docs/getting-started.md#commit-or-ignore-generated-outputs), a fresh clone, branch switch, pull, rebase or new worktree leaves the tool files missing or stale until someone runs `sync`. The `autosync` built-in adds a session-start hook to every tool that has one. The hook runs `sync --keep-edits`, so each session starts from the current specs.

## Turn it on

Add this to `agnostic-ai.yaml`, then run `agnostic-ai sync`:

```yaml
requires: ">=0.100.0"
builtins: [autosync]
```

- Off unless you list it.
- Pair it with [`install-hook --post-checkout`](@/docs/git-hooks.md#regenerate-on-checkout), so files also follow `git checkout`, `git pull` and `git rebase` when no session is running.
- Commit the files a cloud session reads before any hook runs, such as `.claude/settings.json`, with [`gitignore.commit`](@/docs/sync-settings.md#gitignore).

## What the hook does

Each hook runs [`agnostic-ai project -- hook autosync --target <tool>`](@/docs/cli-reference/maintain.md#hook-autosync):

- It looks upward from the session's folder for `node_modules/.bin/agnostic-ai` or the project config, like the [memory hook](@/docs/memory.md#load-at-session-start), and falls back to `agnostic-ai` on `PATH`.
- It syncs the nearest project with `--keep-edits`. A file you edited since the last sync stays as it is, and the hook names it on stderr with `~ kept <path>`.
- It never fails the session. It exits 0 and prints nothing outside a project, without the binary, with a binary older than the `project` command, or while another sync holds the project lock.
- A failed sync prints a one-line note and the last lines of the error on stderr. Run `agnostic-ai sync` to see the full error.
- It prints nothing on stdout, so it adds nothing to the session context.

## Where it runs

Every tool whose session-start event reads exit codes as Claude Code does: Claude Code, Codex, GitHub Copilot, Cursor, Gemini CLI, Qwen Code, CodeBuddy, Factory, Qoder, OpenHands, Goose and Augment. Other enabled tools get no hook.

On Windows, Codex (`commandWindows`) and Copilot (`powershell`) run a PowerShell form of the same command. The other tools run the POSIX form, so on Windows they need Git Bash or WSL.

## Claude Code cloud sessions

Claude Code starts every `SessionStart` hook at once, including the [cloud install hook](@/docs/spec-format/environments.md#claude-code-cloud-install). In a cloud session (`CLAUDE_CODE_REMOTE=true`), autosync waits up to 90 seconds for the install hook to release its lock, then syncs.

When the install itself puts `agnostic-ai` on the machine, autosync finds no binary and exits quietly. End the install commands with `agnostic-ai project`, so the files exist once the install is done:

```yaml
name: dev
install: |
  npm ci
  agnostic-ai project
```

## Scope

`autosync` is a project built-in. `sync --scope user` skips it: a personal list would sync every repository a session opens, including ones whose team commits the outputs. List it in each project's `agnostic-ai.yaml` instead.

## Check it

```bash
agnostic-ai explain builtin:autosync
agnostic-ai why .claude/settings.json
```
