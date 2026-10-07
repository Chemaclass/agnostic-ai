+++
title = "Shared memory"
description = "One project memory that every AI tool reads and writes, so what one tool learns the others know next session."
weight = 79

[extra]
group = "Workflows"
+++

# Shared memory

The `memory` built-in gives a project one memory that all your AI tools share. If Codex learns that PR CI runs only on Ubuntu, Claude Code knows it next session.

Memory is plain Markdown in your project: one file per fact, plus a short `MEMORY.md` list of the facts that each tool reads at the start of a session. Every tool can read it, and your team reviews it like any other file.

## Turn it on

Add `memory` to `agnostic-ai.yaml`, then run `agnostic-ai sync` once:

```yaml
requires: ">=0.81.0"
builtins: [handoff, memory]
```

Saving a fact later needs no sync, and never makes `sync --check` report drift.

To turn it on in every project, add `memory` to `builtins` in `~/.agnostic-ai/agnostic-ai.yaml` and run `agnostic-ai sync --global`. Each project still keeps its own memory, and Claude Code loads it only after a project sync. Turn it on in one place, not both: with both, tools with a [session-start hook](#load-at-session-start) load memory twice.

## Where facts live

```
.agnostic-ai/memory/           # project memory, committed for the team
  MEMORY.md                    # one "- [Title](file.md): summary" line per fact
  ci-ubuntu.md                 # one fact
.agnostic-ai/local/memory/     # personal memory, never committed
  MEMORY.md
  prefers-tabs.md
```

- **Personal memory** holds your preferences and corrections. Tools save there without asking. It stays on your machine, so cloud agents never see it.
- **Project memory** holds facts for the team. Tools save there only after you confirm.

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

`type` is `user`, `feedback`, `project`, or `reference`. The `sources:` setting does not move these folders.

## How tools save and recall

Every tool gets the `shared-memory-policy` rule. It tells the tool to save what a later session needs and cannot find in the code, Git history, or rules, to update a fact instead of adding a duplicate, and to never save secrets. It also says your request beats memory, and that a file, flag, or command named in a fact must still exist before the tool uses it.

Ask for the `shared-memory` skill to recall what the project knows about a topic, or to clean memory up: merge duplicates, drop stale facts, and fix `MEMORY.md`. You confirm each change.

Anyone who can push can change what every tool reads, so review memory changes in pull requests.

## How each tool loads it

| Tool | How memory loads | With `memory.personal: repo` |
| --- | --- | --- |
| Claude Code | `CLAUDE.md` imports both `MEMORY.md` files. Claude's own memory also saves into personal memory. | Loads and saves |
| Codex, Gemini CLI, Qoder | A [session-start hook](#load-at-session-start) adds both files to the conversation. | Loads; saves only with the [`.gitignore` block](#one-store-per-repository) |
| Copilot, Cursor, Factory | A [session-start hook](#load-at-session-start) adds both files to the conversation. | Loads |
| OpenCode | `opencode.json` lists both files under `instructions`. | Loads only with the [`.gitignore` block](#one-store-per-repository) |
| Kilo Code | `kilo.jsonc` lists both files under `instructions`. | Project memory loads. Kilo also gets the rule, which has it run `agnostic-ai memory path` for the personal folder; whether it reads there without asking is untested. Or [add the file yourself](#one-store-per-repository) |
| Every other tool | The rule names both files, and the tool reads them before a task. | The rule has the tool run `agnostic-ai memory path` to find the personal folder; the tool may ask before it reads or writes outside the project |

Aider and Kiro get the rule only, because they show an error or a marker for a missing file. Once both `MEMORY.md` files exist, add them yourself:

- Aider: `read: [.agnostic-ai/local/memory/MEMORY.md, .agnostic-ai/memory/MEMORY.md]` in `.aider.conf.yml`.
- Kiro: `#[[file:.agnostic-ai/memory/MEMORY.md]]` in a steering file.

### Claude Code's own memory

Claude Code also saves facts on its own, in the same format. Sync sets `autoMemoryDirectory` in `.claude/settings.local.json` to your personal memory folder, so those saves land where every other tool reads them.

- The value is an absolute path on your machine, so sync lists the file in Git's `info/exclude` and Git never commits it.
- A value you set yourself stays.
- Claude Code reads the setting only after you trust the folder.

Codex, Gemini CLI, and Qoder have their own memory too, off by default. Leave it off so they save here. Windsurf's older Cascade agent keeps its memories in `~/.codeium/windsurf/memories/`, and no project setting moves them.

## Load at session start

On Codex, Copilot, Cursor, Gemini CLI, Qoder, and Factory, sync adds a hook that runs when a session starts. It calls [`agnostic-ai hook memory`](@/docs/cli-reference/maintain.md#hook-memory), which adds both `MEMORY.md` files to the conversation.

- It does nothing if `agnostic-ai` is not on your PATH.
- With `memory.personal: repo`, it names the personal memory folder even before the first fact, so the tool knows where to save.
- Codex, Qoder, Gemini CLI, and Cursor run it only in a folder you trust.
- On Windows, Cursor and Gemini CLI need `sh` on PATH, for example from Git Bash.
- Hooks you wrote yourself in the same files stay. Sync replaces only its own.

## One store per repository

By default each checkout has its own personal memory. When sync manages `.worktreeinclude`, a worktree that Claude Code creates starts with a copy; any other starts empty. Removing a worktree deletes what it saved. To share one personal memory across all worktrees of a repository, set this in `agnostic-ai.local.yaml` and run `agnostic-ai sync`:

```yaml
# agnostic-ai.local.yaml
memory:
  personal: repo
```

Personal memory then lives in `~/.agnostic-ai/local/memory/<repository name>-<hash>/` (under `$AGNOSTIC_AI_HOME` when set). Moving or recloning the repository starts a new folder. Sync creates it, readable only by you.

Some tools need that folder's absolute path in their config. Sync never writes it to a file Git could commit, so what each tool gets depends on the [`.gitignore` block](@/docs/configuration.md#gitignore) sync manages in the project root:

- **With the block**, sync also writes the path to `CLAUDE.md`, `opencode.json`, `.codex/config.toml`, `.gemini/settings.json`, and `.qoder/settings.json`. A tool with a `gitignore.commit` kind, or a file matched by `gitignore.allow`, keeps the project path. `--gitignore off` turns this off, including in previews.
- **Without it**, those files keep the project paths. Claude Code still uses personal memory through its own setting, and the hook tools still load it, but OpenCode loads none, and Codex, Gemini CLI, and Qoder cannot save there.
- If one of those files was committed before you added the block, run `agnostic-ai sync --untrack` to stop tracking it.

Codex, Gemini CLI, and Qoder only write inside the project. So they can save to personal memory, sync adds its folder to:

- Codex: `writable_roots` under `[sandbox_workspace_write]` in `.codex/config.toml`. Codex reads it in `workspace-write` mode, in a [trusted project](https://learn.chatgpt.com/docs/config-file/config-reference). If your Codex overlay defines `[sandbox_workspace_write]`, sync leaves that table alone; add the folder to it yourself.
- Gemini CLI: `context.includeDirectories` in `.gemini/settings.json` ([configuration reference](https://github.com/google-gemini/gemini-cli/blob/main/docs/reference/configuration.md)).
- Qoder: `permissions.additionalDirectories` in `.qoder/settings.json` ([permissions](https://docs.qoder.com/cli/permissions.md)).

Gemini CLI and Qoder keep your own entries in those lists.

Three tools have limits in this mode:

- **Claude Code** asks once per project whether to allow the import from outside the project ([external imports](https://code.claude.com/docs/en/memory)). If you decline, it still uses personal memory through its own setting.
- **Kilo Code** ignores files outside the project when the project config lists them. With the block, sync leaves the path out of `kilo.jsonc` and prints a note. To load personal memory, add its `MEMORY.md` to `instructions` in `~/.config/kilo/kilo.jsonc`.
- **Tools with only the rule** (Windsurf, Cline, Trae, Warp, Zed, and the other tools in the last table row) get no session context that names the folder. The rule has them run [`agnostic-ai memory path`](@/docs/cli-reference/maintain.md#memory), which prints it. The tool may ask before it reads or writes outside the project.

Cloud agents start from a fresh clone, so they never see personal memory in either mode.

## Check and repair memory

`agnostic-ai lint` and `doctor` flag a `MEMORY.md` over 100 lines, a line that links a missing file, a fact no line links, and text that looks like a secret. Personal memory is skipped when its folder is missing, so CI never checks it.

```bash
agnostic-ai memory lint     # run only the memory checks
agnostic-ai memory index    # rebuild each MEMORY.md, for example after a merge conflict
agnostic-ai memory list     # print each fact's scope, type, and title
agnostic-ai memory path     # print each memory folder's absolute path
```

See [memory](@/docs/cli-reference/maintain.md#memory) in the CLI reference.
