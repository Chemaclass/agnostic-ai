+++
title = "Cross-target behavior"
description = "Understand shared entry points, global output paths, and behavior that spans multiple AI tools."
weight = 125

[extra]
group = "Reference"
+++

# Cross-target behavior

These behaviors span several adapters. For one tool's output tree and configuration, open its name in the [target matrix](@/docs/targets/_index.md#capability-matrix).

## Directory-specific instructions

Rules with `scope` use native file conditions or nested instruction documents on 20 targets. Five targets skip them because their native scope is unverified. Check the [scope matrix and compatibility rules](@/docs/scoped-context.md#native-support) before combining tools.

## Entry-point files

`sync` copies `.agnostic-ai/AGNOSTIC_AI.md` into a root entry-point file for each enabled target. Edit `AGNOSTIC_AI.md`. Sync seeds it once with a short template and never rewrites it. The root entry points are generated, so the next sync overwrites any edit made there.

| Entry-point file | Targets |
|---|---|
| `AGENTS.md` | [codex](@/docs/targets/codex.md), [amp](@/docs/targets/amp.md), [warp](@/docs/targets/warp.md), [cline](@/docs/targets/cline.md), [windsurf](@/docs/targets/windsurf.md), [junie](@/docs/targets/junie.md), [kiro](@/docs/targets/kiro.md), [crush](@/docs/targets/crush.md), [trae](@/docs/targets/trae.md), [jules](@/docs/targets/jules.md), [goose](@/docs/targets/goose.md), [augment](@/docs/targets/augment.md), [qoder](@/docs/targets/qoder.md), [openhands](@/docs/targets/openhands.md), [factory](@/docs/targets/factory.md), [kilo](@/docs/targets/kilo.md), [opencode](@/docs/targets/opencode.md) |
| `CLAUDE.md` | [claude](@/docs/targets/claude.md) |
| `GEMINI.md` | [gemini](@/docs/targets/gemini.md) |
| `CONVENTIONS.md` | [aider](@/docs/targets/aider.md) |
| `.github/copilot-instructions.md` | [copilot](@/docs/targets/copilot.md) |
| `.rules` | [zed](@/docs/targets/zed.md) |
| `.agents/AGENTS.md` | [antigravity](@/docs/targets/antigravity.md) |

Targets that share a path write it once. Cursor and continue have no root entry point. They write only per-file artifacts under their own directory. Per-target quirks, such as Warp's legacy `WARP.md`, Junie's `.junie/AGENTS.md`, and Zed's `.rules`, are on the target pages.

### Inlined rules

Some targets have no native rules directory: codex, amp, warp, zed, gemini, aider, opencode, crush, jules, goose, openhands, factory, and junie. Their unscoped rule bodies are inlined into the entry-point file, in a sentinel-marked `## Rules` block after the pointer body. `import` strips the block, so the `AGNOSTIC_AI.md` round-trip loses nothing. Junie and Zed inline into `.junie/AGENTS.md` and `.rules`, not the shared path.

Augment (`.augment/rules/`) and Kilo Code (`.kilo/rules/`) have a rules directory but still inline into `AGENTS.md`. OpenHands inlines always-on rules. For OpenHands, a rule with `globs`/`paths`, or with a source-layout or frontmatter scope, goes to `.agents/skills/<name>/SKILL.md` as a native path-triggered rule.

So a rule doesn't load twice, `sync` skips a rule file whose text (body and description) matches the `## Rules` block of an `AGENTS.md` the target reads. It keeps files that load only on a path match. It also keeps rules whose text differs for that target, such as a `::target` fence or a path variable. This applies to Cline, Kiro, Qoder, Kilo Code, and Augment whenever the root `AGENTS.md` carries the block: when codex or another inlining target is enabled, and always for Kilo Code and Augment.

{% <details summary="When sync keeps every rule file"> %}
- Trae reads `AGENTS.md` only after you turn on **Include AGENTS.md in the context** under Settings > Rules.
- Windsurf (Devin) keeps rule files for legacy Cascade compatibility. [Cascade](https://docs.devin.ai/desktop/cascade/memories#rules) caps each workspace rule file at 12,000 characters and processes `AGENTS.md` through the same Rules engine.
- A target whose `outputs.<target>.file` moves its entry point off the root `AGENTS.md`.
- Any target, when `AGENTS.md` is under `sync.unmanaged`. Sync then stops writing `AGENTS.md`, so codex and every other reader stop getting rule changes there.

If you turn `AGENTS.md` off in the tool itself (Cline's Rules panel, or a Qoder CLI `context.fileName` without it), turn it back on. Rules that skipped their file reach that tool only through `AGENTS.md`.
{% </details> %}

The next full sync removes a rule file it no longer writes, edited or not, while the file still carries the agnostic-ai header. Until then, `sync --check`, `doctor`, and `status` report it as `ledger` drift, and `doctor --fix` removes it.

### Shared settings files

Some settings files also hold your own keys, such as `.gemini/settings.json`, `.claude/settings.json`, `.cursor/cli.json`, `opencode.json`, or Aider's `.aider.conf.yml`. Sync merges its keys into them and records which keys it set.

- In a permission list shared with you, it records only the rules it added.
- An MCP server map merges by server name. Servers you add stay. A server whose spec goes away is removed. A spec server with the same name as one of yours replaces it, with a note.
- Once no spec writes the file, or its target leaves `targets`, sync and `doctor --fix` remove only those keys and rules. The file is deleted only when sync created it and nothing else is left.

{% <details summary="Keys a spec no longer produces"> %}
While a spec still writes the file, the next sync removes a key it set that no spec produces any more, unless you edited it since. A value you edited after sync wrote it stays. Sync reports the file as a kept orphan, and `doctor --fix` asks before it removes the edited value too. A file that no longer parses is kept and reported the same way.

If the ledger comes from a version that did not record these keys, sync doesn't delete a JSON file or Aider config it no longer writes. It keeps the file as a kept orphan.
{% </details> %}

### Other entry-point options

- A partial sync (`--only`, `--except`, `--target`) renders a shared entry point for every configured target that reads it, so the file matches a full sync.
- `outputs.<target>.rules-file: <path>` selects the legacy layout: one merged rules document at `<path>`, and no pointer-body write for that target.
- `sync.target-overview: true` appends a section to each entry-point file that lists where that tool's generated artifacts live. Only the appendix differs per file. See [configuration](@/docs/configuration.md#synctarget-overview).

## Spec kind behavior

The [spec format guide](@/docs/spec-format/_index.md) defines every portable kind. The [target matrix](@/docs/targets/_index.md#capability-matrix) shows where each is native, mapped, opt-in, source-only, or has no safe output. For an opt-in or source-only spec, `sync` prints a `note:` with the next step. See [coverage notes](@/docs/configuration.md#coverage-notes).

- **Skills** emit as native skill folders (`SKILL.md` plus assets). Most targets read `.agents/skills/`; codex, windsurf, amp, zed, warp, antigravity, crush, augment, goose, openhands, factory, and kilo write it by default, so identical folders dedupe. The rest read their own tree. [`sync.shared-skills`](@/docs/configuration.md#syncshared-skills) collapses byte-identical folders into one copy plus symlinks. Aider flattens skills to rule-form files.
- **Agents** emit as native subagent profiles. Goose and OpenHands write flat files to `.agents/agents/`, Antigravity writes nested files there, and Devin reads the shared tree on top of `.devin/agents/`.

  **Syncing `windsurf` with `antigravity`, `goose`, or `openhands` gives Devin two profiles with one name.** Only the `.devin/agents/<name>.md` copy carries translated `allowed-tools`. Devin gives the shared copy every tool. Give the agent spec one `target:`. `sync` names every conflicting file (#863).
- **Hooks** differ per target in event names, paths, wrappers, and timeout units. When the vocabularies differ, a matcher can parse but match nothing. Read the target page before sharing a hook. Imported shell-form `$CLAUDE_PROJECT_DIR` paths use the target's native root variable or a POSIX Git-root lookup. [Project-root paths](@/docs/spec-format/hooks.md#imported-project-root-paths) lists the limits. A shared script reads `AGNOSTIC_AI_TARGET` to learn which target ran it; see [which target ran a hook](@/docs/spec-format/hooks.md#hook-target).

  **Syncing `claude` with another target can run a hook twice.** Copilot, Cursor, and Trae can read `.claude/settings.json` beside their own hook files. Copilot loads it by default. Cursor's third-party switch is on by default. Trae requires opting in. See the [Cursor](@/docs/targets/cursor.md) and [Trae](@/docs/targets/trae.md) pages.

  **Cursor may drop a Claude hook's `args`.** Its third-party hooks docs don't list `args`, so an exec-form hook (`command: node`, `args: [guard.js]`) may run as a bare `node`. `sync` notes this when `claude` runs and `cursor` is configured. Copilot's docs don't cover Claude's `args` either. Write a shell-form `command`, or turn off Cursor's third-party hooks.
- **MCP servers** reach every target with a project-scoped MCP file. OpenHands reads them only from `~/.openhands/mcp.json`, which `sync --global` writes. Aider, Cline, Jules, and Goose have no MCP surface. See [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target).
- **Settings** map portable fields into the target's native settings. Gemini maps the default model to `model.name`, keeps sibling options, and accepts `x-gemini` keys. [Protected paths](@/docs/spec-format/settings.md#protected-paths) are enforced on Claude Code (permission rules), Codex and Gemini CLI (a generated hook), and the Cursor CLI (deny rules for `decision: deny`). Everywhere else they are advisory.
- **Commands** emit as native slash-prompt files where supported. Kiro uses `.kiro/prompts/` for CLI V3 slash commands. Codex project prompts need the legacy opt-in `outputs.codex.commands-dir`. Amp registers commands through TypeScript plugins, so it has no file output.
- **Ignore** specs concatenate into each supported target's exclusion file. Before it replaces a hand-written file, `sync` checks that every existing pattern survives in order. If it can't prove that, `AAI-103` leaves the file untouched. Import existing patterns first. See [overwrite behavior](@/docs/spec-format/ignore.md#overwrite-behaviour).

## Memory and local state

[Claude Code](@/docs/targets/claude.md) (`~/.claude/projects/<project>/memory/`) and [Qoder](@/docs/targets/qoder.md) (`~/.qoder/projects/<project>/memory/`) each keep a machine-local memory store that they write themselves: a `MEMORY.md` index plus one topic file per memory.

agnostic-ai does not sync them, for two reasons:

- A derived path would often be wrong. Claude Code can move its store (`autoMemoryDirectory`, `CLAUDE_CONFIG_DIR`, `CLAUDE_CODE_PROJECT_DIR_NAME`), and Qoder has no such setting.
- The contents are one person's corrections and session context, not a project convention.

Put durable team knowledge in a spec:

- a [rule](@/docs/spec-format/rules.md) for a convention needed every session,
- a [skill](@/docs/spec-format/skills.md) for a procedure loaded on demand,
- an agent's [`memory: project`](@/docs/targets/claude.md#agent-memory) for knowledge one subagent builds up in a directory Git tracks.

The `memory-curator` skill curates a store in place. It edits only that tool's own memory, during that tool's own session, and applies nothing until you confirm. `agnostic-ai init --demo` seeds it into `.agnostic-ai/skills/`, and the next `sync` writes it to Claude Code and Qoder.

`init` refuses to run where `agnostic-ai.yaml` already exists. In that case, run `agnostic-ai new skill memory-curator` and replace the whole scaffolded file, frontmatter included, with [the repository copy](https://github.com/Chemaclass/agnostic-ai/blob/main/.agnostic-ai/skills/memory-curator/SKILL.md). Keep its `targets: [claude, qoder]` line, or the skill reaches every target.

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

`sync --global` writes user-level configuration for 22 of the 25 targets, separate from project outputs. A dash means global sync writes nothing for that kind.

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

- **MCP** reaches the user MCP files of Augment, Claude Code, Codex, Cursor, Copilot, Gemini, OpenHands, and Qoder. See [MCP servers](@/docs/configuration.md#global-mcp-servers).
- **Settings** reach the user settings of Claude Code, Codex, Copilot, Qoder, Gemini (model only), and Augment (`x-augment` keys only). Claude also maps `permissions.default-mode`; global permission lists are unsupported. Other targets raise a coverage note. See [default model and effort](@/docs/configuration.md#global-default-model-and-effort).
- **Agents** use the same native formats as project agents. Amp, Zed, Warp, and Crush have no global agent output. See [global configuration](@/docs/configuration.md#global-configuration) for setup and migration.
- **Rules** inline into the instructions file, in the same sentinel-marked block as the shared body. Augment is the exception. Its vendor documents no user-level instructions file for the CLI (`~/.augment/user-guidelines.md` is VS Code only). Augment's `~/.augment/rules/` entries are "always treated as `always_apply`", which is what a global rule is.

Paths marked `~/.config/` follow `XDG_CONFIG_HOME` when set, except Devin agents.

### Notes

- **Discovery**: Copilot's path is for its CLI. OpenHands discovery applies to local conversations. Devin custom profiles are experimental. Trae requires **Settings > Beta > Subagents > Enable Subagents Directory**. Its English docs give `~/.trae-cn/agents/`, with no verified international alternative.
- **Configuration roots**: `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `COPILOT_HOME`, `CLINE_DIR`, `QODER_CONFIG_DIR`, `KIRO_HOME`, and `JUNIE_HOME` replace the respective `~/.<tool>/` root. `GEMINI_CLI_HOME` is the parent of `.gemini/`. All surfaces under a root move together. Surfaces outside it, such as `~/.agents/skills/`, stay put. Use absolute paths.
- **Platform paths**: OpenCode and Kilo agent roots follow `XDG_CONFIG_HOME`. Devin agents use `~/.config/devin/agents/` on Linux/macOS and `%APPDATA%\devin\agents\` on Windows.
- **Shared paths** are written once: `~/.agents/skills/` (codex, windsurf, amp, zed, warp, goose, openhands), `~/.agents/agents/` (goose, openhands), `~/.agents/AGENTS.md` (cline, warp), `~/.gemini/GEMINI.md` (gemini, antigravity). Two targets resolving to one path with different bytes stop the run before writes, naming the path.
- **Copilot agent effort**: Copilot agent profiles have no effort key. A portable agent `effort` of `low`, `medium`, `high`, or `xhigh` goes to `subagents.agents.<name>.effortLevel` in `~/.copilot/settings.json` instead. Sync owns only those keys and accepts JSONC. An unchanged effort leaves the file byte for byte. A rewrite drops comments, so sync stops when the file has any. Rerun with `--backup` to rewrite it and keep `settings.json.bak`. A hand-set `effortLevel` with a different value stops the run. Other effort values raise a coverage note.
- **Cursor** does not load the home-level `AGENTS.md`. Global sync installs a managed `sessionStart` hook and a self-contained script bridge under `~/.cursor/hooks/`: POSIX shell on macOS and Linux, PowerShell on Windows. The bridge returns the instructions as `additional_context` JSON and needs neither agnostic-ai, Python, nor jq. It injects context. It doesn't enforce policy. Existing `sessionStart` entries stay. Every other instructions path above auto-loads.
- **OpenCode** reads `~/.claude/CLAUDE.md` only when `~/.config/opencode/AGENTS.md` does not exist. Syncing both targets moves OpenCode onto its own file, so text that lived only in the Claude file stops reaching it.
- **Devin CLI and VS Code Copilot** read `~/.claude/` by default, so syncing claude plus windsurf or copilot delivers the same instructions through two paths.

### Targets left out

- Aider reaches a home instructions file only through a `read:` entry in `~/.aider.conf.yml`.
- Continue's one documented home surface is the `rules:` list in the `config.yaml` it rewrites itself.
- Jules documents nothing at user scope.

Hooks reach six targets at user scope: Claude Code, Codex, Gemini, Qoder, Cursor, and Augment. Claude Code, Codex, Gemini, and Qoder share the Claude-style `{"hooks": {"<Event>": [{"matcher", "hooks": [...]}]}}` shape, so one renderer serves them. Cursor keeps its own. [Augment](https://docs.augmentcode.com/cli/hooks#settings-file-locations) uses the grouped shape with narrower fields, millisecond timeouts, and a required script extension. The other targets have no user-level hook output:

- Factory keys `hooks.json` by event with no wrapper.
- Devin CLI keys `.devin/hooks.v1.json` as an unwrapped array of eight events, with `type` accepting `prompt` as well as `command`.
- Antigravity nests events under a named hook object.
- Crush supports `PreToolUse` alone.
- Goose needs a wrapping plugin directory plus a manifest.
- Junie's hooks are Early Access.
- Kiro uses a `{"version": "v1", "hooks": [...]}` array.
- Amp, OpenCode, and Kilo expose hooks only as TypeScript plugin modules. OpenCode's project directory (`.opencode/plugins/`) is emitted as codegen since #892, Kilo's (`.kilo/plugin/`) since #1105. Their user-level twins are out of scope.
- Cline names a hook by file name, one executable script per event, emitted at project scope since #889. `~/.cline/hooks` is out of scope.

Copilot also documents `~/.copilot/hooks/`, which agnostic-ai doesn't write at user scope yet. Its `{"version": 1, "hooks": {...}}` shape and `timeoutSec` field are implemented at project scope (#629).

Each adapter writes its tool's native format: separate files where supported, a merged document otherwise. By default, an unsupported feature, such as a hook for a target without hooks, is skipped with a warning. Change that with `on-unsupported` in [configuration](@/docs/configuration.md).
