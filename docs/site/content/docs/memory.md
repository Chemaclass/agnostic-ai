+++
title = "Shared memory"
description = "One project memory that every AI tool reads and writes, so what one tool learns the others know next session."
weight = 79

[extra]
group = "Workflows"
+++

# Shared memory

Keep useful project facts in shared Markdown files so other AI coding tools can read them in later sessions. For example, a fact saved while using Codex can tell Claude Code and Cursor which operating systems your project checks.

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

New memory facts are read from these Markdown files. Saving a fact does not need another sync; team facts still need your confirmation as described below.

To turn it on for every project, add `memory` to `builtins` in `~/.agnostic-ai/agnostic-ai.yaml` and run `agnostic-ai sync --global` instead. Pick one place: with both, some tools load memory twice. Claude Code still loads it only after a sync in the project.

## Two memories

| | Project memory | Personal memory |
| --- | --- | --- |
| Folder | `.agnostic-ai/memory/` | `.agnostic-ai/local/memory/` |
| In Git | Yes, shared with the team | No, stays on your machine |
| Holds | Team facts: conventions, decisions, known problems | Your preferences and corrections |
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

Every tool gets the `shared-memory-policy` rule, which tells it when to read, save, and update facts. It has tools save only what the code, Git history, and rules don't already say, update a fact instead of adding a copy, check that a file or command a fact names still exists, and never save secrets. Your request always wins over a saved fact. Anyone who can push can change what every tool reads, so review memory changes in pull requests.

To recall what the project knows about a topic, or to clean memory up (merge duplicates, drop stale facts), ask your tool to use the `shared-memory` skill. You confirm each change.

Gemini CLI also gets the `gemini-memory-tiers` rule. Gemini CLI keeps its own memory, saves team facts here instead of in the `GEMINI.md` that sync writes over, and copies its private facts to personal memory. See [Gemini CLI](@/docs/targets/gemini.md#shared-memory).

## How each tool loads it {#how-each-tool-loads-it}

| Tool | How |
| --- | --- |
| Claude Code | `CLAUDE.md` imports both lists |
| Codex, Copilot, Gemini CLI, Qoder, Factory | A command runs when a session starts and adds both lists |
| Cursor local sessions and self-hosted workers | A command runs when a session starts and adds both lists |
| OpenCode, Kilo Code | The config lists both files |
| Every other tool | A rule tells the tool to read both lists before a task |

Aider and Kiro get only the rule, because both show a missing file: Aider with an error, Kiro with a visible marker in its steering content. Once both `MEMORY.md` files exist, add them yourself:

- Aider, in `.aider.conf.yml`: `read: [.agnostic-ai/local/memory/MEMORY.md, .agnostic-ai/memory/MEMORY.md]`
- Kiro, in a steering file: `#[[file:.agnostic-ai/memory/MEMORY.md]]`

### When a session starts {#load-at-session-start}

On Codex, Copilot, Cursor, Gemini CLI, Qoder, and Factory, sync adds a hook that runs [`agnostic-ai hook memory`](@/docs/cli-reference/maintain.md#hook-memory) when a session starts.

Cursor's hosted cloud agents do not run `sessionStart` hooks. They use the shared memory policy and skill to read the project index before a task. Local sessions and self-hosted workers run the hook; user-level hooks are unavailable in cloud agents. See [Cursor's cloud hook support](https://cursor.com/docs/hooks#cloud-agent-support).

- It runs the `agnostic-ai` on your PATH. Keep that one at 0.81.0 or newer with [`agnostic-ai upgrade`](@/docs/cli-reference/maintain.md#upgrade); an older one loads no memory, and with none on PATH nothing loads.
- Codex, Cursor, Gemini CLI, and Qoder run it only in a folder you trust.
- On Windows, Cursor and Gemini CLI need `sh` on PATH, for example from Git Bash.
- Hooks you wrote yourself stay.
- It keeps the first 6,000 bytes, personal memory first, then project memory. Facts past that never load on these tools, and `memory lint` names them (the first 10). Every other tool loads both lists whole, so `memory lint` also warns there once they pass 6,000 bytes.

### Claude Code's own memory {#claude-code-s-own-memory}

Claude Code also saves facts by itself. Sync points that memory at your personal memory folder (`autoMemoryDirectory` in `.claude/settings.local.json`), so its saves land where every other tool reads them. Sync keeps a value you set yourself, and Git never commits the file. Claude Code uses the setting once you trust the folder.

### Each tool's own memory {#each-tools-own-memory}

Your tool's own memory can stay on. Shared memory adds what none of them gives alone: one set of facts every tool reads, and a team layer you review in pull requests. To share something a tool learned, ask it to save the fact in shared memory. Claude Code's own saves land there already, unless you set `autoMemoryDirectory` yourself.

| Tool | Its own memory | Default |
| --- | --- | --- |
| Claude Code | Auto memory: notes for each repository, on your machine | On (`autoMemoryEnabled`); sync points it at your personal memory folder |
| Codex | Memories: notes built from past sessions, one store for the machine | Off (`memories` under `[features]`) |
| Gemini CLI | Edits its own Markdown memory files: `GEMINI.md` files and private notes for each project. Auto Memory proposes notes from past sessions. Sync generates the project's `GEMINI.md`, so the `gemini-memory-tiers` rule sends Gemini's team facts to shared memory instead, and has it copy private facts there too | Editing is always on, with no setting. Auto Memory is off (`experimental.autoMemory`) |
| Copilot | Copilot Memory, hosted: repository facts and your preferences, dropped after 28 days unused. The VS Code memory tool: notes on your machine | Copilot Memory is on for individual plans; organization admins enable it first. CLI: `/memory on` or `/memory off`. The VS Code tool is built in |
| Qoder | CLI Auto-Memory: notes the agent keeps on your machine. The IDE has a separate memory | CLI: off (`autoMemoryEnabled`). IDE: not documented |
| Kilo Code | Kilo Memory: project notes outside the repository, no personal memory | Off (Settings > Context, or `/memory on`) |
| Kiro | Memory: notes Kiro creates and refines for a repository | Not documented (`/memories`, then `config`) |
| Augment | Agent Memories in the IDE, which you approve one by one | Not documented |
| Trae | Memories, global and for each project, as local Markdown files | Not documented |
| Goose | Memory extension: files in `.goose/memory/` and `~/.config/goose/memory/` | Not documented (Extensions) |
| Jules | Memory in the Jules cloud for each repository | Not documented (repository settings) |
| Windsurf (Devin) | Devin Local keeps none. Cloud Devin has its own Memory | Cloud Devin Memory is on; whether it reaches Devin Desktop, Local, or the CLI is not documented |
| Cursor | None documented | Not applicable |

To keep one store instead, turn a tool's memory off with the setting the Default column names, where there is one.

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

That folder is outside the project, so some tools need its full path in their config. Sync writes that path only to files Git ignores, which needs the [`.gitignore` block](@/docs/configuration.md#gitignore) (`gitignore.enabled: true`, which `agnostic-ai init` sets). The block lists those files, such as `.codex/config.toml`, whenever their tool and the `memory` built-in are on, so the committed `.gitignore` and `.worktreeinclude` stay the same for teammates without the setting, and a new Claude Code worktree copies them. If one of those files is already in Git, run `agnostic-ai sync --untrack`. To commit one of them instead, such as a hand-written `.codex/config.toml`, add its path to [`gitignore.allow`](@/docs/configuration.md#gitignore).

What each tool does with personal memory in this mode:

| Tool | Without the `.gitignore` block | With it |
| --- | --- | --- |
| Claude Code | Loads and saves | Same, after you allow the import once |
| Copilot, Factory | Loads | Loads |
| Codex, Gemini CLI, Qoder | Loads | Loads and saves |
| Cursor | Loads | Loads; the CLI saves, the editor asks first |
| OpenCode | Nothing | Loads and saves |
| Windsurf | Finds it with `agnostic-ai memory path` | Same; Devin CLI saves without asking |
| Kilo Code, every other tool | Finds it with `agnostic-ai memory path` | Same |

Project memory loads the same way in both columns.

Notes:

- **Saving outside the project.** Codex, Gemini CLI, Qoder, OpenCode, the Cursor CLI, and Devin CLI write only inside the project without asking, so sync gives them the folder: Codex `writable_roots` (read in `workspace-write` mode, in a trusted project), Gemini CLI `context.includeDirectories`, Qoder `permissions.additionalDirectories`, OpenCode `permission.external_directory` in `opencode.json`, and a `Write(<folder>/**)` rule in `.cursor/cli.json` and in `permissions.allow` of `.devin/config.json`. Every tool but Codex keeps your own entries. If your Codex overlay sets `[sandbox_workspace_write]`, sync leaves it alone; add the folder there yourself.
- **Committed config.** A file you keep in Git through `gitignore.commit` or `gitignore.allow` never gets the path, so that tool works as without the block. `sync --gitignore off` does the same for one run.
- **Kilo Code** ignores project config entries outside the project. To load personal memory every session, add its `MEMORY.md` to `instructions` in `~/.config/kilo/kilo.jsonc`.
- **Tools with only the rule** need `agnostic-ai` on PATH, and may ask before they read or write outside the project.

Cloud agents start from a fresh clone, so they never see personal memory.

## Check and repair

`agnostic-ai lint` and `doctor` flag a `MEMORY.md` over 100 lines, two `MEMORY.md` files over 6,000 bytes together, a broken link, a fact the list leaves out, and text that looks like a secret. They skip personal memory when its folder is missing, so CI never checks it.

```bash
agnostic-ai memory lint     # run only the memory checks
agnostic-ai memory index    # rebuild each MEMORY.md, for example after a merge conflict
agnostic-ai memory list     # list each fact
agnostic-ai memory path     # print both memory folders
```

See [memory](@/docs/cli-reference/maintain.md#memory) in the CLI reference.
