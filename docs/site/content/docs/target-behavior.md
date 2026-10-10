+++
title = "Cross-target behavior"
description = "How entry-point files, global output, and other behavior work across several AI tools."
weight = 125

[extra]
group = "Reference"
+++

# Cross-target behavior

These behaviors apply to several tools. For one tool's output files and settings, open its name in the [tool support table](@/docs/targets/_index.md#capability-matrix).

## Directory-specific instructions

Rules with `scope` work on 20 tools, through each tool's file conditions or instruction files in subdirectories. Five tools skip them. See the [directory support table](@/docs/scoped-context.md#native-support) before combining tools.

## Entry-point files

`sync` copies `.agnostic-ai/AGNOSTIC_AI.md` into a root entry-point file for each enabled tool. Edit `AGNOSTIC_AI.md`; sync creates it once from a short template and never rewrites it. The next sync overwrites any edit to a root entry-point file.

| Entry-point file | Targets |
|---|---|
| `AGENTS.md` | [codex](@/docs/targets/codex.md), [amp](@/docs/targets/amp.md), [warp](@/docs/targets/warp.md), [cline](@/docs/targets/cline.md), [windsurf](@/docs/targets/windsurf.md), [junie](@/docs/targets/junie.md), [kiro](@/docs/targets/kiro.md), [crush](@/docs/targets/crush.md), [trae](@/docs/targets/trae.md), [jules](@/docs/targets/jules.md), [goose](@/docs/targets/goose.md), [augment](@/docs/targets/augment.md), [qoder](@/docs/targets/qoder.md), [openhands](@/docs/targets/openhands.md), [factory](@/docs/targets/factory.md), [kilo](@/docs/targets/kilo.md), [opencode](@/docs/targets/opencode.md) |
| `CLAUDE.md` | [claude](@/docs/targets/claude.md) |
| `GEMINI.md` | [gemini](@/docs/targets/gemini.md) |
| `CONVENTIONS.md` | [aider](@/docs/targets/aider.md) |
| `.github/copilot-instructions.md` | [copilot](@/docs/targets/copilot.md) |
| `.rules` | [zed](@/docs/targets/zed.md) |
| `.agents/AGENTS.md` | [antigravity](@/docs/targets/antigravity.md) |

Tools that share a path write it once. Cursor and continue have no root entry point. The target pages cover tool-specific files such as Warp's legacy `WARP.md`, Junie's `.junie/AGENTS.md`, and Zed's `.rules`.

### Inlined rules

These tools have no native rules directory: codex, amp, warp, zed, gemini, aider, opencode, crush, jules, goose, openhands, factory, and junie. Sync copies their unscoped rules into the entry-point file, under a `## Rules` heading after the shared instructions. `import` removes that block.

Augment (`.augment/rules/`) and Kilo Code (`.kilo/rules/`) have a rules directory but still inline into `AGENTS.md`. OpenHands inlines always-on rules. Its scoped rules (`globs`, `paths`, a folder, or frontmatter) go to `.agents/skills/<name>/SKILL.md`.

So a rule doesn't load twice, `sync` skips a rule file when its text matches the `## Rules` block of an `AGENTS.md` the tool reads. It keeps path-triggered rules and rules whose text differs for that tool (a `::target` fence or a path variable). This applies to Cline, Kiro, Qoder, Kilo Code, and Augment whenever the root `AGENTS.md` has the block: when codex or another inlining tool is enabled, and always for Kilo Code and Augment.

{% <details summary="When sync keeps every rule file"> %}
- Trae reads `AGENTS.md` only after you turn on **Include AGENTS.md in the context** under Settings > Rules & Memories.
- Windsurf (Devin) keeps rule files for legacy Cascade, which [caps](https://docs.devin.ai/desktop/cascade/memories#rules) each file at 12,000 characters.
- A tool whose `outputs.<target>.file` moves its entry point off the root `AGENTS.md`.
- Any tool, when `AGENTS.md` is under `sync.unmanaged`. Sync stops writing `AGENTS.md` for codex and every other reader too.

If you turn `AGENTS.md` off in the tool itself (Cline's Rules panel, or a Qoder CLI `context.fileName` without it), turn it back on. Rules that skipped their file reach that tool only through `AGENTS.md`.
{% </details> %}

The next full sync removes a rule file it no longer writes, edited or not, if it still has the agnostic-ai header. Until then, `sync --check`, `doctor`, and `status` report `ledger` drift; `doctor --fix` removes it.

### Shared settings files

Some settings files also hold your own keys, such as `.gemini/settings.json`, `.claude/settings.json`, `.cursor/cli.json`, `opencode.json`, or Aider's `.aider.conf.yml`. Sync merges its keys into them and records which keys it set.

- In a permission list you share with it, sync tracks only the rules it added.
- MCP servers merge by name. Servers you add stay. A server whose spec goes away is removed. A spec server with the same name as one of yours replaces it, with a note.
- Once no spec writes the file, or its tool leaves `targets`, sync and `doctor --fix` remove only what they added. The file is deleted only when sync created it and nothing else is left.

{% <details summary="Keys a spec no longer produces"> %}
While a spec still writes the file, the next sync removes a key it set that no spec produces any more. A value you edited stays: sync reports the file as a kept orphan, and `doctor --fix` asks before removing it. A file that no longer parses is kept and reported the same way.
{% </details> %}

### Other entry-point options

- A partial sync (`--only`, `--except`, `--target`) still writes a shared entry point for every configured tool that reads it, so the file matches a full sync.
- `outputs.<target>.rules-file: <path>` selects the legacy layout: one merged rules file at `<path>`, and no shared instructions for that tool.
- `sync.target-overview: true` adds a section to each entry-point file that lists where that tool's generated files live. See [configuration](@/docs/configuration.md#synctarget-overview).

## Spec kind behavior

The [spec format guide](@/docs/spec-format/_index.md) defines every portable kind. The [tool support table](@/docs/targets/_index.md#capability-matrix) shows where each is native, mapped, opt-in, source-only, or has no safe output. For an opt-in or source-only spec, `sync` prints a `note:` with the next step. See [coverage notes](@/docs/configuration.md#coverage-notes).

- **Skills** are written as native skill folders (`SKILL.md` plus assets). Most tools read `.agents/skills/`; codex, windsurf, amp, zed, warp, antigravity, crush, augment, goose, openhands, factory, and kilo write it by default, so identical folders are written once. The rest read their own folder. [`sync.shared-skills`](@/docs/configuration.md#syncshared-skills) replaces byte-identical folders with one copy plus symlinks. Aider writes skill text as rule files.

  Trae can also read `.agents/skills/` when **Enable .agents Skills Directory** is on under Settings > Skills & Commands > Import Settings. Its default is not documented. [`import trae`](@/docs/targets/trae.md#import) reads both paths without changing that switch; sync keeps `.trae/skills/` as its default.
- **Agents** are written as native subagent profiles. Goose and OpenHands write flat files to `.agents/agents/`, Antigravity writes nested files there, and Devin reads that shared folder plus `.devin/agents/`.

  **Syncing `windsurf` with `antigravity`, `goose`, or `openhands` gives Devin two profiles with one name.** Only the `.devin/agents/<name>.md` copy carries translated `allowed-tools`. Devin gives the shared copy every tool. Give the agent spec one `target:`. `sync` names every conflicting file.
- **Hooks** differ per tool in event names, paths, wrappers, and timeout units. A matcher can parse but match nothing when event names differ, so read the target page before sharing a hook. See [project-root paths](@/docs/spec-format/hooks.md#imported-project-root-paths) for imported `$CLAUDE_PROJECT_DIR` paths and [which target ran a hook](@/docs/spec-format/hooks.md#hook-target) for `AGNOSTIC_AI_TARGET`.

  **Syncing `claude` with another tool can run a hook twice.** Copilot, Cursor, and Trae can read `.claude/settings.json` beside their own hook files. Copilot and Cursor (third-party switch) do so by default. Trae requires opting in. See the [Cursor](@/docs/targets/cursor.md) and [Trae](@/docs/targets/trae.md) pages.

  **Cursor may drop a Claude hook's `args`.** An exec-form hook (`command: node`, `args: [guard.js]`) may run as a bare `node`. `sync` notes this when both are configured. Write a shell-form `command`, or turn off Cursor's third-party hooks.
- **MCP servers** reach every tool with a project-level MCP file. OpenHands reads them only from `~/.openhands/mcp.json`, which `sync --global` writes. Aider, Jules, and Goose have no MCP support. Cline reads MCP servers only from a user file (`~/.cline/data/settings/cline_mcp_settings.json`), so it has no project MCP file. See [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target).
- **Settings** translate shared fields into each tool's own settings. Gemini maps the default model to `model.name`, keeps sibling options, and accepts `x-gemini` keys. [Protected paths](@/docs/spec-format/settings.md#protected-paths) are enforced on Claude Code (permission rules), Codex and Gemini CLI (a generated hook), and the Cursor CLI (deny rules for `decision: deny`). For other tools, sync does not enforce them.
- **Commands** are written as native slash-prompt files where supported. Kiro uses `.kiro/prompts/`. Codex project prompts need the legacy opt-in `outputs.codex.commands-dir`. Amp has no file output.
- **Ignore** specs merge into each supported tool's ignore file. Before it replaces a hand-written file, `sync` checks that every existing pattern survives in order. If it can't, `AAI-103` leaves the file untouched, so import existing patterns first. See [overwrite behavior](@/docs/spec-format/ignore.md#overwrite-behaviour).

## Memory and local state

[Claude Code](@/docs/targets/claude.md) (`~/.claude/projects/<project>/memory/`) and [Qoder](@/docs/targets/qoder.md) (`~/.qoder/projects/<project>/memory/`) each keep a memory store on your machine: a `MEMORY.md` index plus one topic file per memory.

With the [`memory` built-in](@/docs/memory.md) on, sync points Claude Code's store at the shared personal store by writing `autoMemoryDirectory` to `.claude/settings.local.json`. Without it, agnostic-ai does not touch either store. The contents are one person's corrections and session context, not a project convention.

Put durable team knowledge in a spec:

- a [rule](@/docs/spec-format/rules.md) for a convention needed every session,
- a [skill](@/docs/spec-format/skills.md) for a procedure loaded on demand,
- an agent's [`memory: project`](@/docs/targets/claude.md#agent-memory) for knowledge one subagent builds up in a directory Git tracks,
- the [`memory` built-in](@/docs/memory.md) for facts any tool learns as it works, in one store every tool reads.

The `memory-curator` skill cleans up a store in place, in that tool's own session, and changes nothing until you confirm. `agnostic-ai init --demo` adds it to `.agnostic-ai/skills/`, and the next `sync` writes it to Claude Code and Qoder.

`init` refuses to run where `agnostic-ai.yaml` already exists. There, run `agnostic-ai new skill memory-curator` and replace the whole generated file, frontmatter included, with [the repository copy](https://github.com/Chemaclass/agnostic-ai/blob/main/.agnostic-ai/skills/memory-curator/SKILL.md). Keep its `targets: [claude, qoder]` line, or the skill reaches every tool.

## Selecting targets

In config:

```yaml
targets:
  - claude
  - cursor
  - copilot
```

Per run:

```bash
agnostic-ai sync -t claude,cursor,copilot
```

The flag overrides config. Unknown targets log a warning and are skipped. Five targets are opt-in. `-t amp,warp,jules,goose,augment` enables them for one run. The [default set](@/docs/configuration.md#targets) lists them, and [`init`](@/docs/cli-reference/start.md#init) pre-ticks the tools it detects, else the CLIs on `PATH`, else `claude` and `codex`.

## New targets

A new adapter is about 50 lines plus one registry entry. See [adding adapters](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/adding-adapters.md).

## Global output

`sync --global` writes user-level configuration for 22 of the 25 targets, separate from project files. A dash means global sync writes nothing for that kind.

| Target | Instructions | Rules | Hooks | Skills | Agents |
|--------|--------------|-------|-------|--------|--------|
| **claude** | `~/.claude/CLAUDE.md` | inlined | `~/.claude/settings.json` | `~/.claude/skills/<name>/` | `~/.claude/agents/<name>.md` |
| **cursor** | `~/.cursor/AGENTS.md` (bridged) | inlined | `~/.cursor/hooks.json` | `~/.cursor/skills/<name>/` | `~/.cursor/agents/<name>.md` |
| **codex** | `~/.codex/AGENTS.md` | inlined | `~/.codex/hooks.json` | `~/.agents/skills/<name>/` | `~/.codex/agents/<name>.toml` |
| **gemini** | `~/.gemini/GEMINI.md` | inlined | `~/.gemini/settings.json` | `~/.gemini/skills/<name>/` | `~/.gemini/agents/<name>.md` |
| **qoder** | `~/.qoder/AGENTS.md` | inlined | `~/.qoder/settings.json` | `~/.qoder/skills/<name>/` | `~/.qoder/agents/<name>.md` |
| **copilot** | `~/.copilot/copilot-instructions.md` | inlined | - | `~/.copilot/skills/<name>/` | `~/.copilot/agents/<name>.agent.md` |
| **cline** | `~/.agents/AGENTS.md` | inlined | - | `~/.cline/skills/<name>/` | `~/.cline/agents/<name>.yml` |
| **windsurf** | `~/.config/devin/AGENTS.md` | inlined | - | `~/.agents/skills/<name>/` | `~/.config/devin/agents/<name>.md` |
| **amp** | `~/.config/amp/AGENTS.md` | inlined | - | `~/.agents/skills/<name>/` | - |
| **zed** | `~/.config/zed/AGENTS.md` | inlined | - | `~/.agents/skills/<name>/` | - |
| **warp** | `~/.agents/AGENTS.md` | inlined | - | `~/.agents/skills/<name>/` | - |
| **opencode** | `~/.config/opencode/AGENTS.md` | inlined | - | `~/.config/opencode/skills/<name>/` | `~/.config/opencode/agents/<name>.md` |
| **antigravity** | `~/.gemini/GEMINI.md` | inlined | - | `~/.gemini/config/skills/<name>/` | `~/.gemini/config/agents/<name>/agent.md` |
| **junie** | `~/.junie/AGENTS.md` | inlined | - | `~/.junie/skills/<name>/` | `~/.junie/agents/<name>.md` |
| **kiro** | `~/.kiro/steering/AGENTS.md` | inlined | - | `~/.kiro/skills/<name>/` | `~/.kiro/agents/<name>.md` |
| **crush** | `~/.config/crush/CRUSH.md` | inlined | - | `~/.config/crush/skills/<name>/` | - |
| **factory** | `~/.factory/AGENTS.md` | inlined | - | `~/.factory/skills/<name>/` | `~/.factory/droids/<name>.md` |
| **kilo** | `~/.config/kilo/AGENTS.md` | inlined | - | `~/.kilo/skills/<name>/` | `~/.config/kilo/agents/<name>.md` |
| **goose** | `~/.config/goose/.goosehints` | inlined | - | `~/.agents/skills/<name>/` | `~/.agents/agents/<name>.md` |
| **openhands** | - | - | - | `~/.agents/skills/<name>/` | `~/.agents/agents/<name>.md` |
| **trae** | - | - | - | `~/.trae/skills/<name>/` | `~/.trae-cn/agents/<name>.md` |
| **augment** | - | `~/.augment/rules/<name>.md` | `~/.augment/settings.json` | `~/.augment/skills/<name>/` | `~/.augment/agents/<name>.md` |

- **MCP** reaches the user MCP files of Antigravity, Augment, Claude Code, Codex, Cursor, Copilot, Gemini, OpenHands, Qoder, and Warp. See [MCP servers](@/docs/configuration.md#global-mcp-servers).
- **Settings** reach the user settings of Claude Code, Codex, Copilot, Qoder, Gemini (model only), and Augment (`x-augment` keys only). Claude also maps `permissions.default-mode`; global permission lists are unsupported. Other tools raise a coverage note. See [default model and effort](@/docs/configuration.md#global-default-model-and-effort).
- **Agents** use the same formats as project agents. Amp, Zed, Warp, and Crush have no global agent output. See [global configuration](@/docs/configuration.md#global-configuration).
- **Rules** are inlined into the instructions file, in the same `## Rules` block as the shared body. Augment has no user-level instructions file for the CLI, so its `~/.augment/rules/` entries are always on.

Paths marked `~/.config/` follow `XDG_CONFIG_HOME` when set. Devin agents use `~/.config/devin/agents/` on Linux and macOS and `%APPDATA%\devin\agents\` on Windows.

### Notes

- **Discovery**: Copilot's path is for its CLI. OpenHands discovery applies to local conversations. Devin custom profiles are experimental. Trae needs **Settings > Beta > Subagents > Enable Subagents Directory**.
- **Configuration roots**: `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `COPILOT_HOME`, `CLINE_DIR`, `QODER_CONFIG_DIR`, `KIRO_HOME`, and `JUNIE_HOME` replace the matching `~/.<tool>/` folder. `GEMINI_CLI_HOME` is the parent of `.gemini/`. Everything under that folder moves together. Paths outside it, such as `~/.agents/skills/`, stay put. Use absolute paths.
- **Shared paths** are written once: `~/.agents/skills/` (codex, windsurf, amp, zed, warp, goose, openhands), `~/.agents/agents/` (goose, openhands), `~/.agents/AGENTS.md` (cline, warp), `~/.gemini/GEMINI.md` (gemini, antigravity). If two tools would write different content to one path, the run stops before writing and names the path.
- **Copilot agent effort**: Copilot agent profiles have no effort key. A portable agent `effort` of `low`, `medium`, `high`, or `xhigh` goes to `subagents.agents.<name>.effortLevel` in `~/.copilot/settings.json`. Sync stops when the file has comments, since a rewrite drops them. Rerun with `--backup` to rewrite it and keep `settings.json.bak`. A hand-set `effortLevel` with a different value stops the run. Other effort values raise a coverage note.
- **Cursor** does not load the home-level `AGENTS.md`. Global sync installs a `sessionStart` hook and a script under `~/.cursor/hooks/` (POSIX shell on macOS and Linux, PowerShell on Windows) that passes the instructions to Cursor. It needs neither agnostic-ai, Python, nor jq, and it adds context without enforcing policy. Existing `sessionStart` entries stay.
- **OpenCode** reads `~/.claude/CLAUDE.md` only when `~/.config/opencode/AGENTS.md` does not exist. Syncing both tools moves OpenCode onto its own file, so text that lived only in the Claude file stops reaching it.
- **Devin CLI and VS Code Copilot** read `~/.claude/` by default, so syncing claude plus windsurf or copilot delivers the same instructions through two paths.

### Targets left out

- Aider reaches a home instructions file only through a `read:` entry in `~/.aider.conf.yml`.
- Continue's one documented home setting is the `rules:` list in the `config.yaml` it rewrites itself.
- Jules documents nothing at user level.

Hooks reach six tools at user level: Claude Code, Codex, Gemini, Qoder, Cursor, and Augment. The other tools have no user-level hook output:

- Factory, Devin CLI, Antigravity, Kiro, Goose, and Crush use hook formats that global sync does not write. Crush supports only `PreToolUse`. Junie's hooks are Early Access.
- Amp, OpenCode, and Kilo expose hooks only as TypeScript plugins. Only OpenCode (`.opencode/plugins/`) and Kilo (`.kilo/plugin/`) get project-level ones.
- Cline and Copilot hooks are project-level only.

An unsupported feature, such as a hook for a tool without hooks, is skipped with a warning by default. Change that with `on-unsupported` in [configuration](@/docs/configuration.md).
