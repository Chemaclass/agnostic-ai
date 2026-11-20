+++
title = "Approvals"
description = "Let a hook answer routine permission prompts in Claude Code and Codex, while risky calls still ask."
weight = 73

[extra]
group = "Workflows"
+++

# Approvals

With approvals in `yolo` mode, a hook answers routine permission prompts for you: it approves one tool call at a time, and leaves every risky call to you. Claude Code and Codex support it today.

## What the gate does not catch

The hook approves only calls it can read in full, and asks for everything else. It is a seat belt, not a sandbox:

- A script the agent wrote one step earlier can do anything when it runs as `./script.sh`.
- `make test`, `npm test`, `go test` and similar commands run whatever the project's Makefile, package scripts and tests hold.
- A recursive search such as `grep -r` can print a credential file that sits inside the project.

Use `yolo` only inside a sandbox, a container or a throwaway worktree.

## Turn it on

Approvals live in the [home config](@/docs/global-configuration.md) only. Add the built-in and the mode to `$AGNOSTIC_AI_HOME/agnostic-ai.yaml`, then sync once:

```yaml
targets: [claude, codex]
builtins: [approvals]
approvals:
  mode: yolo
```

```bash
agnostic-ai sync --global
agnostic-ai approvals status
```

`sync --global` writes a `PermissionRequest` hook to `~/.claude/settings.json` and `~/.codex/hooks.json`. A project sync never writes it, so a teammate never gets it from a committed file. Changing the mode later needs no sync: the hook reads the setting on each request.

Two steps happen in the tools:

- Codex stops at startup with "Hooks need review" when the hook is new or changed. Open `/hooks` and trust it. Until then the hook does not run and Codex shows its prompts. `approvals status` says when the hook is not trusted yet.
- Claude Code runs no hook in a folder until you accept its workspace trust dialog.

## Modes

`ask` is the default and changes nothing: each tool shows its own prompt.

`yolo` approves one call at a time when no always-ask rule matches. Each approval covers that call only; the hook never saves a rule, never changes the tool's mode and never rewrites the call. The hook took about 11 ms per prompt in our test on macOS; its real cost is the gate's limits above.

`review` hands routine prompts to the tool's own reviewer model: Claude Code auto mode or Codex Auto-review, which `sync --global` turns on in the tool's user settings ([details](@/docs/global-configuration-reference.md#global-approvals-review)). Each review costs the tool's own model calls. In `review` the hook prints nothing, so Codex's reviewer answers instead of the hook.

## Settings

```yaml
approvals:
  mode: ask            # ask | review | yolo
  tools:               # per tool, replaces mode
    codex: ask
  projects:            # per project folder, replaces mode and tools
    ~/Code/sandbox: yolo
  always-ask:          # extra patterns, added to the fixed list
    - "Bash(make deploy*)"
```

- Only the home config may loosen. A project's `agnostic-ai.yaml` or `agnostic-ai.local.yaml` may set `approvals.mode: ask` and nothing else, so a team can turn approvals off for its repository. `builtins: [approvals]` in a project file does nothing; `validate` reports both.
- The strictest source wins, in this order: `AGNOSTIC_AI_APPROVALS=ask` in the session's environment, a project file that says `ask`, `projects`, `tools`, `mode`.
- `always-ask` patterns use the Claude Code permission rule form, such as `Bash(make deploy*)`, for both tools.

Field details: [approvals reference](@/docs/global-configuration-reference.md#global-approvals).

## What always asks

| Request | Why |
|---|---|
| A shell command outside a short list of known safe programs and options | The gate cannot tell what it does |
| Shell text with command substitution, `eval`, `bash -c`, here-documents, `xargs` or inline code such as `python -c` | Hides the real command |
| Deleting or overwriting: `rm`, `git reset --hard`, `git clean`, `git checkout --`, `git restore`, `git branch -D`, a redirect onto a path outside the project | Hard to undo |
| Publishing: `git push`, `gh` writes, `npm publish`, `docker push`, release scripts | Leaves your machine |
| Network and installs: `curl`, `wget`, `ssh`, `npm install <name>`, `pip install`, `go install` | Reaches other hosts |
| Secrets: `~/.ssh`, `~/.aws`, `.env*`, `*.pem`, `*.key`, `env`, `printenv`, `gh auth token` | Can print a credential |
| Privilege and processes: `sudo`, `chmod -R`, `chown`, `kill`, `launchctl`, `crontab` | Changes the system |
| A read or an edit outside the project | Outside the folder you work in |
| An edit to `.git/`, `.github/workflows/`, shell profiles, tool settings and hook files, `agnostic-ai.yaml`, or anything under `$AGNOSTIC_AI_HOME` | Could turn the gate off |
| A command that names `AGNOSTIC_AI_` settings | Could change the policy |
| MCP tools, web fetch and search, questions to you such as `AskUserQuestion` | Not a call the gate reads |
| A Claude Code session in any mode but manual or accept edits, such as auto or plan | In auto or plan the tool's reviewer or a rule already chose to ask you; other modes were not tested |
| A Claude Code call that matches an `ask` rule in your user, project, local or managed settings. A search tool (Grep, Glob, LS) asks whenever a `Read` rule names a path | A hook allow would skip that rule |
| A Claude Code project settings file whose `env` sets `AGNOSTIC_AI_HOME`, `HOME`, `CLAUDE_CONFIG_DIR` or `USERPROFILE` | It would move the files the hook reads |
| A pattern in `always-ask` | Your own rule |

`always-ask` only adds to this list; nothing removes a row. The tool's own deny rules still win: a denied call never reaches the hook. A path in a settings spec's [`protected` block](@/docs/spec-format/settings.md#protected-paths) is held by each tool's own check that a project sync writes: Claude Code permission rules, which the hook reads as `ask` rules, and a Codex hook that blocks the edit.

Claude Code `ask` rules come from `~/.claude/settings.json` (or `$CLAUDE_CONFIG_DIR`), `.claude/settings.json` and `.claude/settings.local.json` in the session folder and every folder above it, the main checkout of a linked worktree, and the managed settings file and `managed-settings.d/` folder. A file there that does not parse makes every request ask. Rules passed on the command line with `--settings` or `--allowedTools`, and managed settings delivered by a device profile or the claude.ai console, cannot be read: put those patterns in `always-ask` too.

## What you see in the companion

The companion does not count approvals yet. Codex shows no prompt for an approved call. Claude Code draws its prompt while the hook runs and closes it when the hook approves, so a session can show as waiting for a moment. A call the gate sends to you shows as "Needs permission", as in `ask`.

## Turn it off fast

Each works on the next request, with no sync and no restart:

1. `agnostic-ai approvals ask` writes `approvals.mode: ask` to `$AGNOSTIC_AI_HOME/local/agnostic-ai.yaml`.
2. `AGNOSTIC_AI_APPROVALS=ask` in a session's environment.
3. `approvals.mode: ask` in a project's `agnostic-ai.yaml`.

To remove the hooks, drop `approvals` from `builtins` and run `sync --global`. A native reviewer that `review` turned on stays in the tool's user settings until the next `sync --global`, and a running session keeps its mode until you switch it in the tool (Shift+Tab in Claude Code).

## The decision log

In `yolo`, every request the hook sees adds one line to `$AGNOSTIC_AI_HOME/local/approvals/decisions.jsonl`: `time`, `tool`, `session`, `cwd`, `call` (the tool name), `detail`, `mode`, `decision` (`approved` or `asked`) and `rule` (the row that asked). `detail` leaves out text that may hold a credential. At 5 MB the file moves to `decisions.jsonl.1`, and one older file is kept. When the hook cannot write the log, it asks.

## What can go wrong

| Question | Answer |
|---|---|
| Can text in a file or a web page make the hook approve a harmful command? | No. The hook reads only the tool name and its input, never the conversation. |
| Can a repository turn on `yolo`? | Not through agnostic-ai: loosening is read only from the home config, a project sync never writes the hook, and a project settings file that points the hook at another home makes it ask. A repository you trust in Claude Code can still ship its own hooks, approvals or not. |
| Can the agent edit the policy or the log to approve itself? | Edits to those files and commands that name them ask. A script that writes them indirectly is not caught. |
| What if the hook hangs or crashes? | It exits 0 and prints nothing on every failure, so the tool shows its prompt. Its timeout is 10 seconds; a hung hook costs a Codex user that wait before the prompt shows. |
| Does the hook ever deny a call? | No. It approves or stays silent. |
| Does it read Codex rules that ask? | No. Codex `prefix_rule` and other approval rules are not read, so a call one of them would ask for can be approved. Put those patterns in `always-ask`. |

## Per tool {#per-tool}

| Tool | `review` | `yolo` | Needs |
|---|---|---|---|
| Claude Code (`claude`) | Claude Code auto mode | PermissionRequest hook | Claude Code 2.0.45 or newer; accept the workspace trust dialog |
| Codex (`codex`) | Codex Auto-review | PermissionRequest hook | Trust the hook in `/hooks`; Auto-review needs an `approval_policy` that prompts |
| Every other tool | asks | asks | |

On Windows `yolo` asks in every tool: the hook has no Windows command yet. `agnostic-ai approvals status` prints the route and the reason per tool.
