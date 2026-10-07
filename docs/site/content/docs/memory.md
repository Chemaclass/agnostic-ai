+++
title = "Shared memory"
description = "One project memory that every AI tool reads and writes, so what one tool learns the others know next session."
weight = 79

[extra]
group = "Workflows"
+++

# Shared memory

The built-in `memory` gives a project one memory that all your AI tools share. If Codex learns that PR CI runs only on Ubuntu, Claude Code knows it next session.

It works like Claude Code's own memory: a short index loaded each session, one Markdown file per fact, and a rule that tells the tool when to save. The files live in your project, so every tool can read them and your team can review them.

## Enable it

Add this to `agnostic-ai.yaml`, then run `agnostic-ai sync`:

```yaml
requires: ">=0.81.0"
builtins: [handoff, memory]
```

You only sync once. Saving a fact later needs no sync.

To use it in every project, add `memory` to `builtins` in `~/.agnostic-ai/agnostic-ai.yaml` and run `agnostic-ai sync --global`. Each project still keeps its own memory. Claude Code loads the index only after a project sync.

## Where facts live

```
.agnostic-ai/memory/           # project memory, committed for the team
  MEMORY.md                    # index: one "- [Title](slug.md): hook" line per fact
  ci-ubuntu.md                 # one fact
.agnostic-ai/local/memory/     # personal memory, not committed
  MEMORY.md
  prefers-tabs.md
```

Your preferences and corrections go to personal memory right away. Team facts go to project memory only after you confirm. Personal memory stays in this checkout, so cloud agents never see it. A new Claude Code worktree gets a copy through `.worktreeinclude`.

A fact file looks like this:

```markdown
---
name: ci-ubuntu
description: PR CI runs on Ubuntu alone
metadata:
  type: project
---

PR CI runs on Ubuntu alone.

**Why:** the matrix job costs too much to run on every push.

**How to apply:** dispatch the full OS matrix before merging a change to paths or file watching.
```

The type is `user`, `feedback`, `project`, or `reference`. The two folders are fixed; `sources:` does not move them.

## How tools save

Every tool gets the `shared-memory-policy` rule. It tells the tool to save facts a later session needs and cannot find in the code, git history, or rules. The tool updates an existing fact instead of adding a duplicate, and never saves secrets.

Ask for the `shared-memory` skill to recall what the project knows about a topic, or to clean up memory. Cleanup merges duplicates, drops stale facts, and fixes the index. You confirm before it changes anything.

## How tools load it

| Tool | How the index loads |
| --- | --- |
| Claude Code | `CLAUDE.md` imports both indexes at session start. Sync also points Claude's own auto memory at the personal store, so Claude saves there too. |
| Codex, Copilot, Cursor, Gemini CLI, Qoder, Factory | A session-start hook loads both indexes. See [Load at session start](#load-at-session-start). |
| OpenCode, Kilo Code | `opencode.json` or `kilo.jsonc` lists both indexes under `instructions`. A missing index is skipped. |
| Other tools | The rule names both indexes, and the tool reads them before a task. |

Aider and Kiro get the rule only, because a missing index file causes an error or a visible marker there. Once both indexes exist, add them yourself:

- Aider: `read: [.agnostic-ai/local/memory/MEMORY.md, .agnostic-ai/memory/MEMORY.md]` in `.aider.conf.yml`.
- Kiro: `#[[file:.agnostic-ai/memory/MEMORY.md]]` in a steering file.

Saving a fact never makes `sync --check` report drift.

## Claude Code's own memory

Claude Code keeps an auto memory that it writes without being asked, in the same `MEMORY.md` plus topic-file format. With `memory` on, sync sets `autoMemoryDirectory` in `.claude/settings.local.json` to the absolute path of `.agnostic-ai/local/memory/`. Claude's own saves then land in the personal store, and every other tool reads them.

- The file holds this checkout's absolute path, so sync adds it to the repository's `info/exclude` and Git never commits it.
- A value you set yourself stays. Sync writes the key only when the file lacks it or sync wrote it before.
- Claude Code uses the setting only after you trust the workspace.
- Codex, Gemini CLI, and Qoder keep their own memory off by default. Leave it off so they save here. Windsurf's legacy Cascade agent keeps memories in `~/.codeium/windsurf/memories/`, which no project setting moves.

## Load at session start

On Codex, Copilot, Cursor, Gemini CLI, Qoder, and Factory, the `memory` built-in adds a session-start hook. It runs [`agnostic-ai hook memory`](@/docs/cli-reference/maintain.md#hook-memory), which adds the index to the model's context.

- Hooks you wrote by hand in those files stay. Sync replaces only its own entries.
- The hook does nothing if `agnostic-ai` is not on your PATH.
- Codex runs it only after you trust the project's hooks.
- Turn on `memory` in your home config or in the project, not both. With both, `sync --global` also writes the hook into your home tool settings, and each session loads the index twice.
- On Windows, Cursor and Gemini CLI need `sh` on PATH, for example from Git Bash. They have no separate Windows command.

## Check and repair memory

`agnostic-ai lint` and `doctor` flag an index over 100 lines, an index line whose file is missing, a fact no index line links, and a line that looks like a secret. A missing store is skipped, so CI never checks personal memory.

```bash
agnostic-ai memory lint     # run only the memory checks
agnostic-ai memory index    # rebuild each MEMORY.md, for example after a merge conflict
agnostic-ai memory list     # print each fact's scope, type, and title
```

See [memory](@/docs/cli-reference/maintain.md#memory) in the CLI reference.

## Memory never overrides you

The rule tells each tool that your request beats memory, and to check that a file, flag, or command named in a fact still exists before using it.

Review memory changes in pull requests like any other file. Anyone who can push can change what every tool reads.
