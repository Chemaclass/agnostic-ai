+++
title = "Shared memory"
description = "One project memory that every AI tool reads and writes, so what one tool learns the others know next session."
weight = 79

[extra]
group = "Workflows"
+++

# Shared memory

What one AI tool learns, your other tools know next session. If Codex learns that CI runs only on Ubuntu, Claude Code and Cursor know it too.

Memory is plain Markdown in your project, so every tool can read it and your team reviews it like any other file.

## Turn it on

Add `memory` to `agnostic-ai.yaml` and sync once:

```yaml
requires: ">=0.81.0"
builtins: [handoff, memory]
```

```bash
agnostic-ai sync
```

That's it. Tools save new facts on their own; you never need to sync again for that.

To turn it on for every project, add `memory` to `builtins` in `~/.agnostic-ai/agnostic-ai.yaml` and run `agnostic-ai sync --global` instead. Pick one place: with both, some tools load memory twice. Claude Code still loads it only after a sync in the project.

## Two memories

| | Project memory | Personal memory |
| --- | --- | --- |
| Folder | `.agnostic-ai/memory/` | `.agnostic-ai/local/memory/` |
| In Git | Yes, shared with the team | No, stays on your machine |
| Holds | Team facts: conventions, decisions, gotchas | Your preferences and corrections |
| Tools save | After you confirm | Without asking |

Each folder has one file per fact and a `MEMORY.md` list with one line per fact. Tools read the list at the start of a session and open a fact only when they need it.

```
.agnostic-ai/memory/
  MEMORY.md        # - [CI runs on Ubuntu](ci-ubuntu.md): PR CI skips macOS and Windows
  ci-ubuntu.md
```

A fact file:

```markdown
---
name: ci-ubuntu
description: PR CI runs on Ubuntu alone
metadata:
  type: project
---

PR CI runs on Ubuntu alone.

**Why:** the full OS matrix costs too much to run on every push.

**How to apply:** run the full matrix before merging a change to file paths.
```

`type` is `user`, `feedback`, `project`, or `reference`. The `sources:` setting does not move these folders.

Every tool gets the `shared-memory-policy` rule. It has tools save only what the code, Git history, and rules don't already say, update a fact instead of adding a copy, check that a file or command a fact names still exists, and never save secrets. Your request always wins over a saved fact. Anyone who can push can change what every tool reads, so review memory changes in pull requests.

To recall what the project knows about a topic, or to clean memory up (merge duplicates, drop stale facts), ask your tool to use the `shared-memory` skill. You confirm each change.

## How each tool loads it {#how-each-tool-loads-it}

| Tool | How |
| --- | --- |
| Claude Code | `CLAUDE.md` imports both lists |
| Codex, Copilot, Cursor, Gemini CLI, Qoder, Factory | A command runs when a session starts and adds both lists |
| OpenCode, Kilo Code | The config lists both files |
| Every other tool | A rule tells the tool to read both lists before a task |

Aider and Kiro get only the rule, because a missing file shows an error. Once both `MEMORY.md` files exist, add them yourself:

- Aider, in `.aider.conf.yml`: `read: [.agnostic-ai/local/memory/MEMORY.md, .agnostic-ai/memory/MEMORY.md]`
- Kiro, in a steering file: `#[[file:.agnostic-ai/memory/MEMORY.md]]`

### When a session starts {#load-at-session-start}

On Codex, Copilot, Cursor, Gemini CLI, Qoder, and Factory, sync adds a hook that runs [`agnostic-ai hook memory`](@/docs/cli-reference/maintain.md#hook-memory) when a session starts.

- It runs the `agnostic-ai` on your PATH. Keep that one at 0.81.0 or newer with [`agnostic-ai upgrade`](@/docs/cli-reference/maintain.md#upgrade); an older one loads no memory, and with none on PATH nothing loads.
- Codex, Cursor, Gemini CLI, and Qoder run it only in a folder you trust.
- On Windows, Cursor and Gemini CLI need `sh` on PATH, for example from Git Bash.
- Hooks you wrote yourself stay.

### Claude Code's own memory {#claude-code-s-own-memory}

Claude Code also saves facts by itself. Sync points that memory at your personal memory folder (`autoMemoryDirectory` in `.claude/settings.local.json`), so its saves land where every other tool reads them. Sync keeps a value you set yourself, and Git never commits the file. Claude Code uses the setting once you trust the folder.

Codex, Gemini CLI, and Qoder have their own memory too, off by default. Leave it off so they save here. Windsurf's older Cascade agent keeps its own memories in `~/.codeium/windsurf/memories/`, and no project setting moves them.

## Share personal memory across worktrees {#one-store-per-repository}

By default each checkout has its own personal memory, so deleting a worktree deletes what it learned. A worktree Claude Code creates starts with a copy when sync manages `.worktreeinclude`; any other starts empty. To keep one personal memory for every worktree of a repository, add this to `agnostic-ai.local.yaml` and sync:

```yaml
memory:
  personal: repo
```

Personal memory then lives in `~/.agnostic-ai/local/memory/<repository name>-<hash>/` (under `$AGNOSTIC_AI_HOME` when set). Sync creates it, readable only by you, and it works when `~/.agnostic-ai` is a link to another folder. Moving or recloning the repository starts a new folder. To see it:

```
$ agnostic-ai memory path
project   ~/code/app/.agnostic-ai/memory
personal  ~/.agnostic-ai/local/memory/app-fbcd84e8
```

That folder is outside the project, so some tools need its full path in their config. Sync writes that path only to files Git ignores, which needs the [`.gitignore` block](@/docs/configuration.md#gitignore) (`gitignore.enabled: true`, which `agnostic-ai init` sets). If one of those files is already in Git, run `agnostic-ai sync --untrack`.

What each tool does with personal memory in this mode:

| Tool | Without the `.gitignore` block | With it |
| --- | --- | --- |
| Claude Code | Loads and saves | Same, after you allow the import once |
| Copilot, Factory | Loads | Loads |
| Codex, Gemini CLI, Qoder | Loads | Loads and saves |
| Cursor | Loads | Loads; the CLI saves, the editor asks first |
| OpenCode | Nothing | Loads |
| Kilo Code, every other tool | Finds it with `agnostic-ai memory path` | Same |

Project memory loads the same way in both columns.

Notes:

- **Saving outside the project.** Codex, Gemini CLI, Qoder, and the Cursor CLI write only inside the project without asking, so sync gives them the folder: Codex `writable_roots` (read in `workspace-write` mode, in a trusted project), Gemini CLI `context.includeDirectories`, Qoder `permissions.additionalDirectories`, and a `Write(<folder>/**)` rule in `.cursor/cli.json`. Gemini CLI, Qoder, and Cursor keep your own entries. If your Codex overlay sets `[sandbox_workspace_write]`, sync leaves it alone; add the folder there yourself.
- **Committed config.** A file you keep in Git through `gitignore.commit` or `gitignore.allow` never gets the path, so that tool works as without the block. `sync --gitignore off` does the same for one run.
- **Kilo Code** ignores project config entries outside the project. To load personal memory every session, add its `MEMORY.md` to `instructions` in `~/.config/kilo/kilo.jsonc`.
- **Tools with only the rule** need `agnostic-ai` on PATH, and may ask before they read or write outside the project.

Cloud agents start from a fresh clone, so they never see personal memory.

## Check and repair

`agnostic-ai lint` and `doctor` flag a `MEMORY.md` over 100 lines, a broken link, a fact the list leaves out, and text that looks like a secret. They skip personal memory when its folder is missing, so CI never checks it.

```bash
agnostic-ai memory lint     # run only the memory checks
agnostic-ai memory index    # rebuild each MEMORY.md, for example after a merge conflict
agnostic-ai memory list     # list each fact
agnostic-ai memory path     # print both memory folders
```

See [memory](@/docs/cli-reference/maintain.md#memory) in the CLI reference.
