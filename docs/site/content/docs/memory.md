+++
title = "Shared memory"
description = "One project memory that every AI tool reads and writes, so what one tool learns the others know next session."
weight = 72

[extra]
group = "Workflows"
+++

# Shared memory

Save useful project facts once so other coding tools can read them next session. Memory is Markdown: your team can inspect and review shared facts in Git.

## Turn it on

Add `memory` to `agnostic-ai.yaml` and sync once:

```yaml
requires: ">=0.81.0"
builtins: [memory]
```

```bash
agnostic-ai sync
```

Saving a fact needs no further sync. Team facts require your confirmation.

Use [global configuration](@/docs/global-configuration.md) to enable it across projects. Choose project or global setup: enabling both can load the same facts twice. Claude Code needs a project sync either way.

## Two memories

| | Project memory | Personal memory |
| --- | --- | --- |
| Folder | `.agnostic-ai/memory/` | `.agnostic-ai/local/memory/` |
| In Git | Yes, shared with the team | No, stays on your machine |
| Holds | Team facts: conventions, decisions, known problems | Your preferences and corrections |
| Tools save | After you confirm | Without asking |

Each folder has one file per fact and a `MEMORY.md` list. Tools read the list at the start of a session and open relevant facts when needed.

```
.agnostic-ai/memory/
  MEMORY.md        # - [CI runs on Ubuntu](ci-ubuntu.md): PR CI skips macOS and Windows
  ci-ubuntu.md
```

## Save and recall a fact

Ask your coding tool: “Remember that this project's integration tests need the local database running.” It proposes the team fact for your approval. Ask it to save a personal preference, such as shorter answers, and it can save that without asking.

Inspect the fact and its `MEMORY.md` entry. Start another tool in the project and ask what it knows about integration tests.

Keep your coding tool's own memory on. Shared memory adds a team layer and facts other tools can read. Your request wins over saved facts.

Ask for the `shared-memory` skill to recall a topic or clean up stale facts. Cleanup needs your confirmation.

See [Gemini CLI](@/docs/targets/gemini.md#shared-memory) for how it keeps its own memory alongside these shared facts.

## Check and repair

`agnostic-ai lint` and `doctor` flag a `MEMORY.md` over 100 lines, two `MEMORY.md` files over 6,000 bytes together, a broken link, a fact the list leaves out, text that looks like a secret, and hidden characters a reviewer cannot see. They skip personal memory when its folder is missing, so CI never checks it.

Run `agnostic-ai memory lint` for only these checks, and `agnostic-ai memory index` to rebuild each `MEMORY.md` after a merge conflict. See [memory commands](@/docs/cli-reference/memory.md#memory) for every subcommand.

<details>
<summary>Write or review a memory fact by hand</summary>

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

The `shared-memory-policy` rule asks tools to save facts absent from code, history, and rules; update rather than duplicate them; verify named files and commands still exist; and exclude secrets. Review shared memory changes like code, because every tool reads them.

</details>

<details>
<summary>How each tool loads memory and keeps its own notes</summary>

## How each tool loads it {#how-each-tool-loads-it}

| Tool | How |
| --- | --- |
| Claude Code | `CLAUDE.md` imports the project list, and [Claude Code's own memory](#claude-code-s-own-memory) loads the personal list |
| Codex, Copilot, Gemini CLI, Qoder, Factory | A command runs when a session starts and adds both lists |
| Devin CLI (`windsurf`) | A SessionStart command adds both lists through tagged context; the rule remains a fallback |
| Cursor local sessions and self-hosted workers | A command runs when a session starts and adds both lists |
| Trae with project hooks enabled | A command runs when a session starts and adds both lists |
| Cline VS Code | A project TaskStart hook returns both lists through `contextModification`; the shared rule remains the fallback |
| Auggie CLI (macOS, Linux, Windows WSL) | An executable `SessionStart` script adds both lists |
| OpenCode, Kilo Code | The config lists both files |
| Kiro | A steering file includes project memory and, in checkout mode, personal memory |
| Antigravity | An always-on rule includes project memory and, in checkout mode, personal memory |
| Every other tool | A rule tells the tool to read both lists before a task |

With `memory.personal: repo`, Kiro includes only the project list and finds personal memory with `agnostic-ai memory path`. Kiro shows a marker for an included list that has no file yet and loads the rest. The marker goes away once a tool saves a fact there. If you added the include to a steering file by hand, remove it. With `sync --global`, Kiro gets only the rule, because a home file cannot name a project's lists.

Antigravity gets `antigravity-memory-index` with native file includes. Repo-personal mode includes only project memory and uses `agnostic-ai memory path` for personal memory. Global sync leaves the project include rule out. Includes are static even before an index exists; `init` does not create empty indexes. Antigravity does not document missing-file behavior. Expanded includes count toward its 24,000-byte per-rule limit and shared 20,000-token rules budget. See [Antigravity shared memory](@/docs/targets/antigravity.md#shared-memory).

Aider gets only the rule, because a missing file shows an error. Once both `MEMORY.md` files exist, add them yourself in `.aider.conf.yml`: `read: [.agnostic-ai/local/memory/MEMORY.md, .agnostic-ai/memory/MEMORY.md]`

### When a session starts {#load-at-session-start}

On Codex, Copilot, Cursor, Gemini CLI, Qoder, Factory, Trae, Devin CLI, and Auggie CLI on Unix, sync adds a hook that runs [`agnostic-ai hook memory`](@/docs/cli-reference/maintain.md#hook-memory) when a session starts. In Trae, enable project hooks in Settings > Hooks first.

Cline VS Code gets a project-only `memory-cline-task-start` hook. It needs Cline for VS Code 4.1.20 or later with hooks enabled in the extension. Earlier versions drop the hook's context. Global sync omits this hook because the default Cline SDK and CLI detach TaskStart hooks and ignore their replies. Native memories keep working. Use `agnostic-ai hook run memory-cline-task-start --target cline --runtime vscode --format json` to check its Unix hook mechanics.

Devin CLI uses the native `memory-windsurf-session-start` hook with an explicit POSIX `sh` launcher and a 10-second timeout. It starts from `DEVIN_PROJECT_DIR` even when its process runs elsewhere. Native Windows hook execution is undocumented.

Cursor's hosted cloud agents do not run `sessionStart` hooks. They use the shared memory policy and skill to read the project index before a task. Local sessions and self-hosted workers run the hook; user-level hooks are unavailable in cloud agents. See [Cursor's cloud hook support](https://cursor.com/docs/hooks#cloud-agent-support).

- It runs the `agnostic-ai` on your PATH. Keep that one at 0.81.0 or newer with [`agnostic-ai upgrade`](@/docs/cli-reference/upgrades.md#upgrade); an older one loads no memory, and with none on PATH nothing loads.
- Codex, Cursor, Gemini CLI, and Qoder run it only in a folder you trust.
- On Windows, Cursor and Gemini CLI need `sh` on PATH, for example from Git Bash. The Claude Code subagent hook needs it too.
- Hooks you wrote yourself stay.
- It keeps the first 6,000 bytes and drops hidden characters first; see [`hook memory`](@/docs/cli-reference/maintain.md#hook-memory). Facts past the cut never load on these tools, and `memory lint` names them (the first 10). Every other tool loads both lists whole, hidden characters included, so `memory lint` also warns there once they pass 6,000 bytes. Remove the hidden characters it reports.

### Claude Code's own memory {#claude-code-s-own-memory}

Claude Code also saves facts by itself. Sync points that memory at your personal memory folder (`autoMemoryDirectory` in `.claude/settings.local.json`), so its saves land where every other tool reads them. Sync keeps a value you set yourself, and Git never commits the file. Claude Code uses the setting once you trust the folder.

Claude Code then loads your personal list as its own memory: the first 200 lines or 25KB, with hidden characters cleaned out. `CLAUDE.md` imports only the project list. Claude Code's own memory never reaches a subagent, so a `SubagentStart` hook runs [`agnostic-ai hook memory`](@/docs/cli-reference/maintain.md#hook-memory) and adds the personal list when each subagent starts. In a linked worktree that uses the main checkout's personal folder, the hook reads that same folder.

`CLAUDE.md` imports both lists, and the hook adds nothing, when auto memory is off (`autoMemoryEnabled: false` in a settings file, or `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1` in your shell or a settings `env` block), or when your own `autoMemoryDirectory` points to another folder. It also keeps the personal import when any settings file sets `permissions.blockReadsOutsideWorkingDirectories: true` and the folder comes from repository-supplied settings (for example, a tracked local file or a local file outside Git), or when `sync.unmanaged` prevents sync from setting the folder. Run `agnostic-ai sync` after you change these settings.

Facts Claude Code saved before come along. The first sync that points Claude at personal memory copies them from Claude's own folder (`~/.claude/projects/<project>/memory/`) when personal memory is still empty, and leaves that folder as it is. Otherwise `agnostic-ai lint` names the folder and how many of its facts no longer load (LINT043), and this copies them over:

```bash
agnostic-ai memory import --from claude --dry-run
agnostic-ai memory import --from claude
```

Every tool reads personal memory, so the dry run lists each fact with its description first; `--only <file>` copies just the ones you pick.

`--path` copies from any folder with the same layout, such as Gemini CLI's `~/.gemini/tmp/<project>/memory/` or Qoder's `~/.qoder/projects/<project>/memory/`. See [memory import](@/docs/cli-reference/maintain.md#memory).

### Each tool's own memory {#each-tools-own-memory}

To share something a tool learned, ask it to save the fact in shared memory. Claude Code's own saves land there already, unless you set `autoMemoryDirectory` yourself.

| Tool | Its own memory | Default |
| --- | --- | --- |
| Claude Code | Auto memory: notes for each repository, on your machine | On (`autoMemoryEnabled`); sync points it at your personal memory folder |
| Codex | Memories: notes built from past sessions, one store for the machine | Off (`memories` under `[features]`) |
| Gemini CLI | Edits its own Markdown memory files: `GEMINI.md` files and private notes for each project. Auto Memory proposes notes from past sessions. Sync generates the project's `GEMINI.md`, so the `gemini-memory-tiers` rule sends Gemini's team facts to shared memory instead, and has it copy private facts there too | Editing is always on, with no setting. Auto Memory is off (`experimental.autoMemory`) |
| Copilot | Copilot Memory, hosted: repository facts and your preferences, dropped after 28 days unused. The VS Code memory tool: notes on your machine | Copilot Memory is on for individual plans; organization admins enable it first. CLI: `/memory on` or `/memory off`. The VS Code tool is built in |
| Qoder | CLI Auto-Memory: notes the agent keeps on your machine. The IDE has a separate memory | CLI: off (`autoMemoryEnabled`). IDE: not documented |
| Kilo Code | Kilo Memory: project notes outside the repository, no personal memory | Off (Settings > Context, or `/memory on`) |
| Kiro | Memory: learned notes stored per user account, shared by the IDE and CLI | IDE: off (the user setting **Kiro Agent: Memory**; a workspace value has no effect). CLI: not documented (`/memories`, then `config`) |
| Augment | Agent Memories in the IDE, which you approve one by one | Not documented |
| Trae | Memories, global and for each project, as local Markdown files | Not documented |
| Goose | Memory extension: files in `.goose/memory/` and `~/.config/goose/memory/` | Not documented (Extensions) |
| Jules | Memory in the Jules cloud for each repository | Not documented (repository settings) |
| Windsurf (Devin) | Devin Local keeps none. Cloud Devin has its own Memory | Cloud Devin Memory is on; whether it reaches Devin Desktop, Local, or the CLI is not documented |
| Cursor | None documented | Not applicable |

</details>

<details>
<summary>Share personal memory across linked worktrees</summary>

## Share personal memory across worktrees {#one-store-per-repository}

For a new Git project, `agnostic-ai init --memory` enables shared memory and starts personal memory in repo mode when managed ignores are on. It keeps an existing `agnostic-ai.local.yaml`. Teammates who clone and sync choose their own mode. Without Git or with `--gitignore=off`, it enables memory in checkout mode.

By default each checkout has its own personal memory, so deleting a worktree deletes what it learned. A worktree Claude Code creates starts with a copy when sync manages `.worktreeinclude`; any other starts empty. Claude Code itself reads the main checkout's `.claude/settings.local.json` in a worktree, so its own memory uses the main checkout's personal memory once sync has run there. To keep one personal memory for every worktree of a repository, add this to `agnostic-ai.local.yaml` and sync:

```yaml
memory:
  personal: repo
```

A linked worktree without its own `agnostic-ai.local.yaml` follows the main checkout's `memory.personal` setting. Its own local file takes precedence. Other settings stay local to each checkout, and teammates choose their own mode. In checkout mode, sync names this option when linked worktrees exist.

Personal memory then lives in `~/.agnostic-ai/local/memory/<repository name>-<hash>/` (under `$AGNOSTIC_AI_HOME` when set). Sync creates it, readable only by you, and it works when `~/.agnostic-ai` is a link to another folder. Moving or recloning the repository starts a new folder. To see it:

```
$ agnostic-ai memory path
project   ~/code/app/.agnostic-ai/memory
personal  ~/.agnostic-ai/local/memory/app-fbcd84e8
```

The first sync after a mode change copies facts into the new store when it holds no Markdown file, and prints the count. The old store stays unchanged. With facts already in the new store, `agnostic-ai lint` names the ones left behind; `agnostic-ai memory import --from checkout` copies them over (`--from repo` when you switch back). Dry runs, checks, and previews never copy facts. Later syncs keep imported facts you deleted out of the store; an explicit `memory import` can bring them back.

That folder is outside the project, so some tools need its full path in their config. Sync writes that path only to files Git ignores, which needs the [`.gitignore` block](@/docs/configuration.md#gitignore) (`gitignore.enabled: true`, which `agnostic-ai init` sets). The block lists those files, such as `.codex/config.toml`, whenever their tool and the `memory` built-in are on, so the committed `.gitignore` and `.worktreeinclude` stay the same for teammates without the setting, and a new Claude Code worktree copies them. If one of those files is already in Git, run `agnostic-ai sync --untrack`. To commit one of them instead, such as a hand-written `.codex/config.toml`, add its path to [`gitignore.allow`](@/docs/configuration.md#gitignore).

What each tool does with personal memory in this mode:

| Tool | Without the `.gitignore` block | With it |
| --- | --- | --- |
| Claude Code | Loads and saves | Same |
| Copilot, Factory | Loads | Loads |
| Codex, Gemini CLI, Qoder | Loads | Loads and saves |
| Cursor | Loads | Loads; the CLI saves, the editor asks first |
| OpenCode | Nothing | Loads and saves |
| Windsurf | Finds it with `agnostic-ai memory path` | Same; Devin CLI saves without asking |
| Kilo Code, Kiro, every other tool | Finds it with `agnostic-ai memory path` | Same |

Project memory loads the same way in both columns.

Notes:

- **Saving outside the project.** Codex, Gemini CLI, Qoder, OpenCode, the Cursor CLI, and Devin CLI write only inside the project without asking, so sync gives them the folder: Codex `writable_roots` (read in `workspace-write` mode, in a trusted project), Gemini CLI `context.includeDirectories`, Qoder `permissions.additionalDirectories`, OpenCode `permission.external_directory` in `opencode.json`, and a `Write(<folder>/**)` rule in `.cursor/cli.json` and in `permissions.allow` of `.devin/config.json`. Every tool but Codex keeps your own entries. If your Codex overlay sets `[sandbox_workspace_write]`, sync leaves it alone; add the folder there yourself.
- **Committed config.** A file you keep in Git through `gitignore.commit` or `gitignore.allow` never gets the path, so that tool works as without the block. `sync --gitignore off` does the same for one run.
- **Kilo Code** ignores project config entries outside the project. To load personal memory every session, add its `MEMORY.md` to `instructions` in `~/.config/kilo/kilo.jsonc`.
- **Tools with only the rule** need `agnostic-ai` on PATH, and may ask before they read or write outside the project.

Cloud agents start from a fresh clone, so they never see personal memory.

</details>
