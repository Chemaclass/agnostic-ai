+++
title = "Targets"
description = "Compare supported tools, capability coverage, native output paths, and target-specific behavior."
weight = 120
sort_by = "weight"
template = "docs/targets.html"
page_template = "docs/page.html"

[extra]
group = "Reference"
nav_after = "spec-format"
scripts = ["assets/scripts/capability-matrix.js"]
+++


# Targets


Choose your tools in `targets:` in `agnostic-ai.yaml`. Use the [capability matrix](#capability-matrix) to check a spec kind, then the [per-target output](#per-target-output) sections for exact paths and options. [Target selection](#selecting-targets) covers defaults and filters; [global output](#global-output) covers personal configuration.

## Directory-specific instructions

Rules with `scope` use native file conditions or nested instruction documents on 19 targets. Six targets skip them because native scope or its serialized format is unverified. See the [scope matrix and compatibility rules](@/docs/scoped-context.md#native-support) before combining tools. The per-target layouts below describe ordinary rules unless a scoped case is stated.

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

## Capability matrix

{{ <capability_matrix /> }}

Open a target name for its exact paths, configuration keys, and caveats. `Native` means matching target output is emitted by default. `Mapped` uses another native surface, `Opt-in` requires an output option, and `Source only` keeps the portable spec without default target output. When an opt-in or source-only spec is present, `sync` prints a `note:` with the next step. See [Coverage notes](@/docs/configuration.md#coverage-notes).

Cross-cutting kind notes:

- **Skills**: emit as native skill folders (`SKILL.md` plus bundled assets). Most targets read the shared tree at `.agents/skills/`; the rest read their own. Each target's section below gives its path and why. With [`sync.shared-skills`](@/docs/configuration.md#syncshared-skills), byte-identical folders across targets collapse into one canonical copy plus per-skill symlinks. Aider and Continue have no skill surface, so skills flatten to rule-form files there. Factory scoped skills use `<scope>/.factory/skills/`; root skills retain their compatibility directory.
- **Agents**: emit as native subagent profiles. `.agents/agents/` is a shared tree rather than one target's: Goose and OpenHands write a flat `<name>.md` there through one byte-identical renderer, Antigravity writes the nested `<name>/agent.md`, and Devin reads the whole tree on top of its own `.devin/agents/`.

  **Syncing `windsurf` alongside `antigravity`, `goose`, or `openhands` gives Devin two profiles claiming one name.** Devin lists `.agents/agents/` under "Also supported" for project subagents, accepts `<name>.md` and `<name>/agent.md` alike, and takes a frontmatter `name:` over the path-derived identifier ([docs.devin.ai/cli/subagents](https://docs.devin.ai/cli/subagents)). Only the `.devin/agents/<name>.md` copy carries the translated `allowed-tools`; the shared-tree copy has no key for it, and Devin defaults an absent list to every tool, so a subagent a user restricted for Devin can get an unrestricted twin. The vendor documents a conflict rule for built-in names only, so which profile wins is undocumented. One file cannot serve both readers today, because `model` means a model ID to Devin and the closed `inherit`/`flash`/`pro` tier enum to Antigravity. Until that is resolved, give an agent spec a single `target:` when both are in `targets`. `sync` reports the overlap, naming the shared-tree file and the targets writing it, so the duplicate is not something you have to already know about to notice (#863).
- **Hooks**: shell commands on lifecycle events (`PreToolUse`, `PostToolUse`, `SessionStart`, and others). Event names, file paths, wrapper shapes, and timeout units all differ per target, so read the target's section before authoring one. Matchers fail more quietly: Claude Code's tool vocabulary reaches Qoder and Copilot's PascalCase form unchanged, but OpenHands, Windsurf, Augment, Crush, Factory, and Trae each name tools differently, so a Claude-authored matcher parses there and then matches nothing. Zed runs hooks only through the opt-in `outputs.zed.tasks-file`. Other targets skip with a warning.

  **`.claude/settings.json` is turning into a cross-tool hook file, so syncing `claude` alongside another target can run the same hook twice.** Four vendors read it today: Claude Code, which owns it, plus three that load it on top of their own hook file. Copilot ("all hook entries from all sources are run"), Cursor ("All matching hooks from every source run"), and Trae ("TraeCode will read all enabled hook configurations and execute them in combination"). Only Copilot's read is on by default and ungated; Cursor's and Trae's each sit behind an off-by-default switch, and the per-target sections say which. Until a vendor offers a toggle, give a hook spec a single `target:` rather than two that both read this file.
- **MCP servers**: propagate to every target with a project-scoped MCP file (21 of 25, see the matrix). Aider, Cline, Jules, and Goose have no MCP surface and skip with a warning. Targets with an explicit transport field carry it on remote entries and omit it on stdio; the rest infer the transport from which keys are emitted. Copilot emits twice, since Copilot CLI does not read VS Code's `.vscode/mcp.json`. See [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target) for which targets honor a spec's `disabled: true`.
- **Settings**: Gemini maps the portable default model to `model.name`, preserves sibling options, and accepts `x-gemini` settings keys.
- **Commands**: slash-prompt files authored under `commands/`. Native on ten targets, each with its own directory and frontmatter subset; see the target's section. Codex deprecated project prompts, so its commands stay source-only unless `outputs.codex.commands-dir` opts into the legacy `.codex/prompts/` layout. Amp has no file-based command surface at all, since commands register programmatically from plugin TypeScript, so there is nothing to opt into. Other targets skip with a warning.
- **Ignore**: gitignore-syntax exclusion files, native on ten targets. Every ignore spec concatenates into each of them under a `#` provenance header, preserving pattern order and whitespace. Before replacing a hand-authored file, `sync` requires every existing pattern to survive unchanged and in order, with no added negations; extra exclusions are allowed, and comments and blank lines never block a sync. When preservation cannot be established, `AAI-103` names the risk and leaves the file untouched (#761). Run `agnostic-ai import <target>` to pull existing patterns into a spec first, then review conflicts before syncing. `outputs.<target>.provenance-header: false` removes the marker this check relies on and disables it. See [overwrite behavior](@/docs/spec-format.md#overwrite-behaviour). Other targets skip with a warning.

## Memory and local state

Two targets keep an automatic memory store the tool writes for itself: [Claude Code](@/docs/targets/claude.md) under `~/.claude/projects/<project>/memory/` and [Qoder](@/docs/targets/qoder.md) under `~/.qoder/projects/<project>/memory/`. Each store is a `MEMORY.md` index plus one topic file per memory, and each is machine-local.

agnostic-ai does not sync them, and will not. Claude Code can move its store with `autoMemoryDirectory`, `CLAUDE_CONFIG_DIR`, or `CLAUDE_CODE_PROJECT_DIR_NAME`, and Qoder has no equivalent setting, so a derived path would be wrong for anyone who moved it and a guess everywhere else. The contents are also the wrong shape to share: one person's corrections and session context, not a project convention for a teammate's checkout.

Durable team knowledge belongs in a spec instead. Use a [rule](@/docs/spec-format.md#rules) for a convention that must be in context every session, a [skill](@/docs/spec-format.md#skills) for a procedure that loads on demand, and an agent's [`memory: project`](@/docs/targets/claude.md#agent-memory) when one subagent should accumulate project knowledge in a directory git carries.

To curate a store in place, use the `memory-curator` skill. It edits only that tool's own memory, during that tool's own session, and applies nothing until you confirm. In a new project, `agnostic-ai init --demo` seeds it into `.agnostic-ai/skills/`, and the next `sync` writes it to Claude Code and Qoder. `init` refuses to run where `agnostic-ai.yaml` already exists, so in a project that already has one, run `agnostic-ai new skill memory-curator` and replace the whole scaffolded file, frontmatter included, with [the repository copy](https://github.com/Chemaclass/agnostic-ai/blob/main/.agnostic-ai/skills/memory-curator/SKILL.md). The `targets: [claude, qoder]` line matters: without it the skill reaches every target.

## Per-target output

One page per target: emitted tree, capability notes, config keys, and how to verify it against the real tool.

{{ <target_pages /> }}

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

`sync --global` writes user-level configuration for 22 of the 25 targets. These paths are independent of the project outputs. A dash means the vendor documents no user-level surface of that kind, so nothing is written rather than a path being guessed (target-audit 2026-09-07).

| Target | Instructions | Rules | Hooks | Skills |
|--------|--------------|-------|-------|--------|
| **claude** | `~/.claude/CLAUDE.md` | inlined | `~/.claude/settings.json` | `~/.claude/skills/<name>/` |
| **cursor** | `~/.cursor/AGENTS.md` (bridged) | inlined | `~/.cursor/hooks.json` | `~/.cursor/skills/<name>/` |
| **codex** | `~/.codex/AGENTS.md` | inlined | `~/.codex/hooks.json` | `~/.agents/skills/<name>/` |
| **gemini** | `~/.gemini/GEMINI.md` | inlined | `~/.gemini/settings.json` | `~/.gemini/skills/<name>/` |
| **qoder** | `~/.qoder/AGENTS.md` | inlined | `~/.qoder/settings.json` | `~/.qoder/skills/<name>/` |
| **copilot** | `~/.copilot/copilot-instructions.md` | inlined | - | `~/.copilot/skills/<name>/` |
| **cline** | `~/.agents/AGENTS.md` | inlined | - | `~/.cline/skills/<name>/` |
| **windsurf** | `~/.config/devin/AGENTS.md` | inlined | - | `~/.agents/skills/<name>/` |
| **amp** | `~/.config/amp/AGENTS.md` | inlined | - | `~/.agents/skills/<name>/` |
| **zed** | `~/.config/zed/AGENTS.md` | inlined | - | `~/.agents/skills/<name>/` |
| **warp** | `~/.agents/AGENTS.md` | inlined | - | `~/.agents/skills/<name>/` |
| **opencode** | `~/.config/opencode/AGENTS.md` | inlined | - | `~/.config/opencode/skills/<name>/` |
| **antigravity** | `~/.gemini/GEMINI.md` | inlined | - | `~/.gemini/config/skills/<name>/` |
| **junie** | `~/.junie/AGENTS.md` | inlined | - | `~/.junie/skills/<name>/` |
| **kiro** | `~/.kiro/steering/AGENTS.md` | inlined | - | `~/.kiro/skills/<name>/` |
| **crush** | `~/.config/crush/CRUSH.md` | inlined | - | `~/.config/crush/skills/<name>/` |
| **factory** | `~/.factory/AGENTS.md` | inlined | - | `~/.factory/skills/<name>/` |
| **kilo** | `~/.config/kilo/AGENTS.md` | inlined | - | `~/.kilo/skills/<name>/` |
| **goose** | `~/.config/goose/.goosehints` | inlined | - | `~/.agents/skills/<name>/` |
| **openhands** | - | - | - | `~/.agents/skills/<name>/` |
| **trae** | - | - | - | `~/.trae/skills/<name>/` |
| **augment** | - | `~/.augment/rules/<name>.md` | - | `~/.augment/skills/<name>/` |

Rules inline into the instructions file, under the same sentinel-marked managed block as the shared instructions body. Augment is the one exception: the vendor documents no user-level instructions file for the CLI (`~/.augment/user-guidelines.md` is VS Code only), and its `~/.augment/rules/` entries are "always treated as `always_apply`", which is exactly what a global rule is. Every path marked `~/.config/` follows `XDG_CONFIG_HOME` when that variable is set.

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
