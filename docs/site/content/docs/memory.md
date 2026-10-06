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
.agnostic-ai/memory/
  MEMORY.md        # index: one "- [Title](slug.md): hook" line per fact
  ci-ubuntu.md     # one fact
```

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

The type is one of `user`, `feedback`, `project`, or `reference`. The folder is fixed and does not follow `sources:`. Commit it: it is how the team shares what its tools learn.

## How tools save

Every target gets the always-on `shared-memory-policy` rule. It tells the tool to save a fact a later session needs and cannot get from the code, the git history, or the rules. The tool proposes each fact and writes it only after you confirm. It updates an existing fact instead of adding a duplicate, and it never saves secrets.

Ask for the `shared-memory` skill to recall what the project knows about a topic, or to clean up the store. Cleanup merges duplicates, drops stale facts, and fixes the index, and it applies nothing until you confirm.

## How tools load it

| Target | How the index loads |
| --- | --- |
| Claude Code | `CLAUDE.md` imports `.agnostic-ai/memory/MEMORY.md`, so the tool loads it at session start. |
| Codex, Copilot, Cursor, Qoder, Factory | A session-start hook runs [`agnostic-ai hook memory`](@/docs/cli-reference/maintain.md#hook-memory), which adds the index to the model's context. Without `agnostic-ai` on `PATH`, the hook does nothing. Codex runs it only after you trust the project's hooks. On Windows, Copilot and Cursor need `sh` on `PATH`, such as Git Bash (#1856). |
| Every other target | The `shared-memory-policy` rule names the index, and the tool reads it before a task. |

The import goes only into files whose readers all follow `@` lines. A file that `sync.resolve-imports` rewrites never carries memory text, so saves never show up as `sync --check` drift.

## Memory is background

The rule tells each tool that memory never overrides your request, and to check that a file, flag, or command a fact names still exists before acting on it. Review memory changes in pull requests like any other project file: anyone who can push can change what every tool reads.
