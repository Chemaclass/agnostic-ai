+++
title = "Cross-target behavior"
description = "Understand shared entry points, global output paths, and behavior that spans multiple AI tools."
weight = 125

[extra]
group = "Reference"
+++


# Cross-target behavior


These details apply across multiple adapters. For one tool's output tree and configuration, open its name in the [target matrix](@/docs/targets/_index.md#capability-matrix).

## Directory-specific instructions

Rules with `scope` use native file conditions or nested instruction documents on 19 targets. Six targets skip them because native scope or its serialized format is unverified. See the [scope matrix and compatibility rules](@/docs/scoped-context.md#native-support) before combining tools. Target pages describe ordinary rules unless a scoped case is stated.

## Entry-point files

`sync` writes `.agnostic-ai/AGNOSTIC_AI.md` plus a root entry-point file per enabled target. All entry-point files share the same canonical pointer body, so editing one in place is a no-op: the next sync overwrites it.

| Entry-point file | Targets |
|---|---|
| `AGENTS.md` | [codex](@/docs/targets/codex.md), [amp](@/docs/targets/amp.md), [warp](@/docs/targets/warp.md), [cline](@/docs/targets/cline.md), [windsurf](@/docs/targets/windsurf.md), [junie](@/docs/targets/junie.md), [kiro](@/docs/targets/kiro.md), [crush](@/docs/targets/crush.md), [trae](@/docs/targets/trae.md), [jules](@/docs/targets/jules.md), [goose](@/docs/targets/goose.md), [augment](@/docs/targets/augment.md), [qoder](@/docs/targets/qoder.md), [openhands](@/docs/targets/openhands.md), [factory](@/docs/targets/factory.md), [kilo](@/docs/targets/kilo.md), [opencode](@/docs/targets/opencode.md) |
| `CLAUDE.md` | [claude](@/docs/targets/claude.md) |
| `GEMINI.md` | [gemini](@/docs/targets/gemini.md) |
| `CONVENTIONS.md` | [aider](@/docs/targets/aider.md) |
| `.github/copilot-instructions.md` | [copilot](@/docs/targets/copilot.md) |
| `.rules` | [zed](@/docs/targets/zed.md) |
| `.agent/AGENTS.md` | [antigravity](@/docs/targets/antigravity.md) |

Targets sharing a path write it once; dedup is automatic. Targets absent from the table above (cursor, continue) have no root entry-point: they emit only per-file artifacts under their own directory.

Details that apply to one target live on its page: [Warp](@/docs/targets/warp.md) still honours a legacy `WARP.md`, [Junie](@/docs/targets/junie.md) prefers `.junie/AGENTS.md`, [Zed](@/docs/targets/zed.md) reads `.rules`, and [Windsurf](@/docs/targets/windsurf.md) and [OpenCode](@/docs/targets/opencode.md) moved onto the shared `AGENTS.md`.

Targets with no native rules directory (codex, amp, warp, zed, gemini, aider, opencode, crush, jules, goose, openhands, factory, junie) inline unscoped rule bodies into their entry-point file under a sentinel-marked `## Rules` block, after the pointer body. That file is the only always-on context surface these tools read, so the rule reaches them by default. The block is identical across targets that share a path, so the dedup still holds, and `import` strips it, keeping the AGNOSTIC_AI.md round-trip lossless. Junie and Zed are the exceptions: their entry-point files (`.junie/AGENTS.md` and `.rules`) are not the shared path.

Three targets have a native rules directory and still inline into AGENTS.md as well:

- **Augment** (`.augment/rules/<name>.md`): the vendor does not cleanly establish the two surfaces' relative precedence, so this adapter does not assume the native directory makes the inline copy redundant.
- **Kilo Code** (`.kilo/rules/<name>.md`, referenced from `kilo.jsonc`'s `instructions` array): that array outranks AGENTS.md in Kilo Code's documented precedence order (agent prompt > project `instructions` > AGENTS.md > global), but AGENTS.md is always loaded when present regardless, so the inline copy is a fallback layer rather than dead weight.
- **OpenHands** (`.agents/skills/<name>/SKILL.md`): only a rule carrying `globs`/`paths` or a source-layout/frontmatter scope emits there, as a native path-triggered rule. An always-on rule still inlines into AGENTS.md only, since path-triggering needs the glob to scope it.

Set `outputs.<target>.rules-file: <path>` to use the legacy concatenated rules layout instead. The adapter writes a single merged document at `<path>` and `sync` skips the pointer-body write for that target so they do not collide.

Set `sync.target-overview: true` to append a generated section to each entry-point file listing where that tool's generated artifacts live (rules dir, MCP file, ...). The canonical body stays identical across targets; only the appendix differs per file. See [configuration](@/docs/configuration.md#synctarget-overview).

## Spec kind behavior

The [spec format guide](@/docs/spec-format.md) defines every portable kind. The [target matrix](@/docs/targets/_index.md#capability-matrix) shows where each one is native, mapped, opt-in, source-only, or has no safe output. Open a target name there for its paths, mappings, and caveats.

When an opt-in or source-only spec is present, `sync` prints a `note:` with the next step. See [coverage notes](@/docs/configuration.md#coverage-notes).

- **Skills** emit as native skill folders (`SKILL.md` plus bundled assets). Most targets read `.agents/skills/`; the rest read their own tree. [`sync.shared-skills`](@/docs/configuration.md#syncshared-skills) collapses byte-identical folders into one canonical copy plus per-skill symlinks. Aider flattens skills to rule-form files because it has no skill surface.
- **Agents** emit as native subagent profiles. Goose and OpenHands write flat files to `.agents/agents/`, Antigravity writes nested files there, and Devin reads the whole shared tree on top of `.devin/agents/`.

  **Syncing `windsurf` alongside `antigravity`, `goose`, or `openhands` gives Devin two profiles claiming one name.** Only the `.devin/agents/<name>.md` copy carries translated `allowed-tools`. The shared copy has no equivalent key, so Devin defaults it to every tool. Give an agent spec one `target:` when these tools are combined. `sync` reports the overlap and names every conflicting file (#863).
- **Hooks** use different event names, paths, wrappers, and timeout units per target. Matchers can parse but match nothing when tools use different vocabularies. Read the target page before sharing a hook.

  **Syncing `claude` alongside another target can run the same hook twice.** Copilot, Cursor, and Trae can read `.claude/settings.json` alongside their own hook files. Copilot loads it by default. Cursor's third-party configuration switch is also on by default; Trae requires opting in. See the [Cursor](@/docs/targets/cursor.md) and [Trae](@/docs/targets/trae.md) pages.
- **MCP servers** reach every target with a project-scoped MCP file. Aider, Cline, Jules, and Goose have no MCP surface. Transport fields follow each target's schema. See [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).
- **Settings** map portable fields into the target's native settings. Gemini maps the default model to `model.name`, preserves sibling options, and accepts `x-gemini` keys.
- **Commands** emit as native slash-prompt files where the target supports them. Codex project prompts require the legacy opt-in `outputs.codex.commands-dir`. Amp commands register through TypeScript plugins, so there is no file output to enable.
- **Ignore** specs concatenate into each supported target's exclusion file. Before replacing a hand-written file, `sync` checks that every existing pattern survives in order. If it cannot prove that, `AAI-103` leaves the file untouched. Import existing patterns before syncing. See [overwrite behavior](@/docs/spec-format.md#overwrite-behaviour).

## Memory and local state

Two targets keep an automatic memory store the tool writes for itself: [Claude Code](@/docs/targets/claude.md) under `~/.claude/projects/<project>/memory/` and [Qoder](@/docs/targets/qoder.md) under `~/.qoder/projects/<project>/memory/`. Each store is a `MEMORY.md` index plus one topic file per memory, and each is machine-local.

agnostic-ai does not sync them, and will not. Claude Code can move its store with `autoMemoryDirectory`, `CLAUDE_CONFIG_DIR`, or `CLAUDE_CODE_PROJECT_DIR_NAME`, and Qoder has no equivalent setting, so a derived path would be wrong for anyone who moved it and a guess everywhere else. The contents are also the wrong shape to share: one person's corrections and session context, not a project convention for a teammate's checkout.

Durable team knowledge belongs in a spec instead. Use a [rule](@/docs/spec-format.md#rules) for a convention that must be in context every session, a [skill](@/docs/spec-format.md#skills) for a procedure that loads on demand, and an agent's [`memory: project`](@/docs/targets/claude.md#agent-memory) when one subagent should accumulate project knowledge in a directory git carries.

To curate a store in place, use the `memory-curator` skill. It edits only that tool's own memory, during that tool's own session, and applies nothing until you confirm. In a new project, `agnostic-ai init --demo` seeds it into `.agnostic-ai/skills/`, and the next `sync` writes it to Claude Code and Qoder. `init` refuses to run where `agnostic-ai.yaml` already exists, so in a project that already has one, run `agnostic-ai new skill memory-curator` and replace the whole scaffolded file, frontmatter included, with [the repository copy](https://github.com/Chemaclass/agnostic-ai/blob/main/.agnostic-ai/skills/memory-curator/SKILL.md). The `targets: [claude, qoder]` line matters: without it the skill reaches every target.

## Selecting targets

Persistent (config):

```yaml
targets:
  - claude
  - cursor
  - copilot
```

Per-run (CLI):

```bash
agnostic-ai sync -t claude,cursor,copilot
```

CLI flag overrides config. Unknown targets log a warning and skip. Five targets are opt-in (`-t amp,warp,jules,goose,augment` enables them for one run); the [default set](@/docs/configuration.md#targets) lists them, and [`init`](@/docs/cli-reference.md#init) pre-ticks the tools it detects.

## New targets

See [adding adapters](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/adding-adapters.md). ~50 lines plus one registry entry.

## Global output

`sync --global` writes user-level configuration for 22 of the 25 targets. These paths are independent of the project outputs. A dash means global sync emits nothing for that kind.

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
| **augment** | - | `~/.augment/rules/<name>.md` | - | `~/.augment/skills/<name>/` | `~/.augment/agents/<name>.md` |

Global agents use the same native formats as project agents. Amp, Zed, Warp, and Crush have no supported global agent-file output. See [global configuration](@/docs/configuration.md#global-configuration) for setup and migration.

- **Discovery**: Copilot's path is for its CLI. OpenHands discovery applies to local conversations. Devin custom profiles are experimental. Trae requires **Settings > Beta > Subagents > Enable Subagents Directory**; its English docs specify `~/.trae-cn/agents/`, with no verified international alternative.
- **Configuration roots**: `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `COPILOT_HOME`, `CLINE_DIR`, `QODER_CONFIG_DIR`, `KIRO_HOME`, and `JUNIE_HOME` replace the respective `~/.<tool>/` root. `GEMINI_CLI_HOME` is the parent of `.gemini/`. Every surface under that root moves together: instructions, skills, hooks, and agents. Surfaces outside it, such as `~/.agents/skills/`, stay put. Use absolute paths.
- **Platform paths**: OpenCode and Kilo agent roots follow `XDG_CONFIG_HOME`. Devin agents use `~/.config/devin/agents/` on Linux/macOS and `%APPDATA%\devin\agents\` on Windows.
- **Shared agents**: Goose and OpenHands share `~/.agents/agents/`. Identical output is written once; conflicting content stops the run before writes.
- **Copilot agent effort**: Copilot agent profiles have no effort key, so a portable agent `effort` of `low`, `medium`, `high`, or `xhigh` goes to `subagents.agents.<name>.effortLevel` in `~/.copilot/settings.json`. Sync owns only those keys and keeps the rest of the file. The file accepts JSONC. An unchanged effort leaves it byte for byte. A rewrite keeps key order and indent but cannot keep comments, so when the file has any, sync stops before writing; rerun with `--backup` to rewrite it and keep the original as `settings.json.bak`. An `effortLevel` set by hand to a different value stops the run before writes. Other effort values raise a coverage note.

Rules inline into the instructions file, under the same sentinel-marked managed block as the shared instructions body. Augment is the one exception: the vendor documents no user-level instructions file for the CLI (`~/.augment/user-guidelines.md` is VS Code only), and its `~/.augment/rules/` entries are "always treated as `always_apply`", which is exactly what a global rule is. Except for Devin agents as noted above, paths marked `~/.config/` follow `XDG_CONFIG_HOME` when set.

Three targets are absent on purpose:

- Aider reaches a home instructions file only through a `read:` entry in `~/.aider.conf.yml`, never automatically.
- Continue's one documented home surface is the `rules:` list inside the `config.yaml` that Continue itself rewrites.
- Jules documents nothing at user scope at all: its CLI reference has no config file and no home path.

Several targets share a path on purpose, and the shared write happens once:

- `~/.agents/skills/` is read by codex, windsurf, amp, zed, warp, goose, and openhands.
- `~/.agents/AGENTS.md` is read by cline and warp.
- `~/.gemini/GEMINI.md` is read by gemini and antigravity.

Two targets resolving to one path with different bytes is a hard error naming the path, not a last-writer-wins race.

Hooks reach five targets at user scope: Claude Code, Codex, Gemini, and Qoder all document the Claude-style `{"hooks": {"<Event>": [{"matcher", "hooks": [...]}]}}` shape there, so one renderer serves them, and Cursor keeps its own.

The other seventeen are declined for a stated reason, not for lack of a project surface:

- Factory keys `hooks.json` directly by event with no wrapper.
- Augment measures `timeout` in milliseconds and requires a script extension.
- Devin CLI keys `.devin/hooks.v1.json` as an unwrapped array of eight events, with `type` accepting `prompt` as well as `command`.
- Antigravity nests events under a named hook object.
- Crush supports `PreToolUse` alone.
- Goose needs a wrapping plugin directory plus a manifest.
- Junie's hooks are Early Access.
- Kiro uses a `{"version": "v1", "hooks": [...]}` array.
- Amp and OpenCode expose hooks only as TypeScript plugin modules. OpenCode's project-level directory (`.opencode/plugins/`) is emitted as codegen since #892; its user-level twin stays out of this user-scope pass.
- Cline names a hook by file name, one executable script per event, emitted at project scope since #889. `resolveHooksConfigSearchPaths` also returns `~/.cline/hooks`, which stays out of this user-scope pass.

Copilot also documents a user-level hooks directory (`~/.copilot/hooks/`), left for a future user-scope pass; its `{"version": 1, "hooks": {...}}` shape and `timeoutSec` field are already implemented at project scope. Issue #629 is complete and covers project scope only.

Cursor does not automatically load the home-level `AGENTS.md`. Global sync therefore installs a managed `sessionStart` hook and a self-contained script bridge under `~/.cursor/hooks/` (POSIX shell on macOS and Linux, PowerShell on Windows). The bridge returns the rendered instructions as valid `additional_context` JSON without calling agnostic-ai, Python, or jq. Cursor session-start hooks are fire-and-forget context injection, not enforced policy. Existing `sessionStart` entries remain in place. Every other instructions path in the table above auto-loads.

Two interactions to know before enabling everything at once:

- OpenCode reads `~/.claude/CLAUDE.md` only when `~/.config/opencode/AGENTS.md` does not exist, so syncing both targets moves OpenCode onto its own file and any user text that lived only in the Claude file stops reaching it.
- Devin CLI and VS Code Copilot read `~/.claude/` surfaces by default, so a user syncing claude plus windsurf or copilot gets the same instructions body through two paths.

Each adapter emits in its tool's native format: separate files where the tool supports them, a merged document otherwise. Unsupported features (e.g. hooks for a non-hook-aware target) skip with a warning by default. Override via `on-unsupported` in [configuration](@/docs/configuration.md).
