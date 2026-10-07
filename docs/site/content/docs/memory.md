+++
title = "Shared memory"
description = "One project memory that every AI tool reads and writes, so what one tool learns the others know next session."
weight = 79

[extra]
group = "Workflows"
+++

# Shared memory

The built-in `memory` gives a project one memory that every AI tool shares. When Codex learns that PR CI runs on Ubuntu only, Claude Code knows it in its next session, and the other way round.

It follows the shape of Claude Code's own memory: a short index loaded each session, one Markdown file per fact, and a rule that tells the tool when to save. The difference is that the files live in your project, so every tool can read them and your team can review them.

## Enable it

Add this to `agnostic-ai.yaml`, then run `agnostic-ai sync`:

```yaml
requires: ">=0.81.0"
builtins: [handoff, memory]
```

Saving a fact needs no further sync.

To use it in every project, add `memory` to `builtins` in `~/.agnostic-ai/agnostic-ai.yaml` and run `agnostic-ai sync --global`. The rule then reaches each tool's global instructions, and each project still keeps its own store. Only a project sync adds the Claude Code import.

## The store

```
.agnostic-ai/memory/           # project memory, committed for the team
  MEMORY.md                    # index: one "- [Title](slug.md): hook" line per fact
  ci-ubuntu.md                 # one fact
.agnostic-ai/local/memory/     # personal memory, ignored by Git
  MEMORY.md
  prefers-tabs.md
```

Tools save your preferences and corrections to personal memory without asking. Team facts go to project memory only after you confirm. Personal memory stays in this checkout: cloud agents never see it, and a new Claude Code worktree gets a copy through `.worktreeinclude`.

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

The type is one of `user`, `feedback`, `project`, or `reference`. Both folders are fixed and do not follow `sources:`. Commit `.agnostic-ai/memory/`: it is how the team shares what its tools learn.

## How tools save

Every target gets the always-on `shared-memory-policy` rule. It tells the tool to save a fact a later session needs and cannot get from the code, the git history, or the rules: personal facts right away, team facts after you confirm. It updates an existing fact instead of adding a duplicate, and it never saves secrets.

Ask for the `shared-memory` skill to recall what the project knows about a topic, or to clean up the store. Cleanup merges duplicates, drops stale facts, and fixes the index, and it applies nothing until you confirm.

## How tools load it

| Target | How the index loads |
| --- | --- |
| Claude Code | `CLAUDE.md` imports both `MEMORY.md` indexes, so the tool loads them at session start. |
| Codex, Copilot, Cursor, Gemini CLI, Qoder, Factory | With `memory-hook`, a session-start hook adds the index to the model's context. See [load at session start](#load-at-session-start). |
| Every other target | The `shared-memory-policy` rule names both indexes, and the tool reads them before a task. |

The import goes only into files whose readers all follow `@` lines. A file that `sync.resolve-imports` rewrites never carries memory text, so saves never show up as `sync --check` drift.

## Load at session start

Without a hook, Codex, Copilot, Cursor, Gemini CLI, Qoder, and Factory read the index only when the model follows the rule. Add `memory-hook` to load it at every session start:

```yaml
builtins: [handoff, memory, memory-hook]
```

The hook runs [`agnostic-ai hook memory`](@/docs/cli-reference/maintain.md#hook-memory), which adds the index to the model's context.

- Import hand-written hooks first (`agnostic-ai import <target>`). Sync rewrites each target's hook file and drops entries it did not write (#1858).
- Without `agnostic-ai` on `PATH`, the hook does nothing.
- Codex runs it only after you trust the project's hooks.
- On Windows, Copilot, Cursor, and Gemini CLI need `sh` on `PATH`, such as Git Bash (#1856).

## Memory is background

The rule tells each tool that memory never overrides your request, and to check that a file, flag, or command a fact names still exists before acting on it. Review memory changes in pull requests like any other project file: anyone who can push can change what every tool reads.
