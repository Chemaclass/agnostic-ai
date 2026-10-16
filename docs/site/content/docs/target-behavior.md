+++
title = "Cross-target behavior"
description = "Understand shared instruction files, machine-wide settings, and differences between tools."
weight = 125

[extra]
group = "Reference"

[extra.moved]
notes = "@/docs/global-configuration.md#native-discovery-and-shared-paths"
targets-left-out = "@/docs/global-configuration.md#targets-left-out"
+++

# Cross-target behavior

## Notes {#notes .legacy-anchor}

## Targets left out {#targets-left-out .legacy-anchor}

One portable source can produce different native files. A rule becomes `.claude/rules/<name>.md` for Claude Code and a section of `AGENTS.md` for Codex. Tools still differ in discovery, permissions, and supported fields. Use [Compare targets](@/docs/compare.md) to inspect sample output, and `agnostic-ai compare` to check your own specs.

## Directory-specific instructions

A rule's `scope` limits it to a directory, using native file conditions or nested instruction files. Support and loading time differ by tool. Check [directory-specific instructions](@/docs/scoped-context.md) before sharing scoped rules, especially when one tool starts at the repository root and another loads rules while editing.

## Entry-point files

Write shared project instructions in `.agnostic-ai/AGNOSTIC_AI.md`. Sync creates it once and preserves your edits. It copies that body into each selected tool's instruction file; edits to the generated copies are overwritten by the next sync.

| Instruction file | Targets |
|---|---|
| `AGENTS.md` | Codex, Amp, Warp, Cline, Windsurf, Junie, Kiro, Crush, Trae, Jules, Goose, Augment, Qoder, OpenHands, Factory, Kilo, OpenCode |
| `CLAUDE.md` | Claude Code |
| `GEMINI.md` | Gemini CLI |
| `CONVENTIONS.md` | Aider |
| `.github/copilot-instructions.md` | Copilot |
| `.rules` | Zed |
| `.agents/AGENTS.md` | Antigravity |

Tools sharing a path get one write. Junie also has its preferred `.junie/AGENTS.md`. Claude can import identical shared instructions from `AGENTS.md`. Cursor and Continue use rule files instead of a generated root instruction file. Each [target page](@/docs/targets/_index.md) explains its discovery rules.

### Inlined rules

Some tools read no separate rule directory. Sync puts their project-wide rules below the shared body in a marked `## Rules` block. Augment and Kilo also use that block despite having rule directories. OpenHands uses it for always-on rules; path-triggered rules become skills.

Cline, Kiro, Qoder, Kilo, and Augment skip a native rule file when its body already appears in the root `AGENTS.md` they read. File-conditioned rules and tool-specific bodies keep their own files. Keep `AGENTS.md` enabled in Cline's Rules panel and Qoder's context file list, or those tools miss the shared rules.

Trae keeps every rule file because reading `AGENTS.md` is optional. Windsurf keeps them for legacy compatibility. Enabling those tools beside an inlining target can load the same rule twice.

Moving a tool's entry point off root `AGENTS.md` keeps its separate rule files. Listing `AGENTS.md` in `sync.unmanaged` stops writes for every target sharing that path. It is a shared file, not a per-tool switch.

The next full sync removes obsolete generated rule files, even edited ones that still carry the generated header. Before that cleanup, `sync --check`, `doctor`, and `status` report `ledger` drift; `doctor --fix` removes it.

### Shared settings files

For mergeable files such as `.claude/settings.json`, `.gemini/settings.json`, `.cursor/cli.json`, and `opencode.json`, sync records the keys it owns and preserves unrelated keys. An unchanged generated key leaves when no spec produces it. A value you edited becomes a kept orphan; `doctor --fix` asks before removing it.

MCP servers in mergeable maps are tracked by name. A same-name spec can replace your server with a note. This does not apply to files rebuilt wholesale, such as Claude's `.mcp.json` or Cursor's `.cursor/mcp.json`. Check the target's import and overwrite behavior before the first sync.

When a target leaves the setup, cleanup removes only its recorded values. A shared settings file is deleted only if sync created it and no other content remains. Invalid or edited orphan files stay for review.

### Other entry-point options

- A partial sync still refreshes shared instruction files for all configured readers, so the body matches a full sync.
- `outputs.<target>.rules-file` selects a legacy merged-rules layout instead of that tool's normal shared-instructions output.
- `sync.target-overview: true` adds generated-file locations to each entry point.

See [configuration](@/docs/configuration.md) for these options.

## Spec kind behavior

The [tool support table](@/docs/targets/_index.md#capability-matrix) describes output support, not complete native feature parity. Native fields can still be omitted, a client can require trust, and a shared prompt is not necessarily an enforced restriction.

| State | What to do |
|---|---|
| Native | Use the target's own format; check field and client limits on its page. |
| Mapped | Check what feature carries the content. An agent prompt written as a skill is not a native subagent. |
| Opt-in | Enable the named output setting, then check native discovery. A legacy file can be written without the tool loading it. |
| Source only | Keep the portable source; no default native artifact is written. A pointer does not activate the feature. |
| No output | Remove that target from the spec or use a supported native route. Sync warns by default. |

For details, read the matching [spec reference](@/docs/spec-format/_index.md). Change unsupported-kind handling with [`on-unsupported`](@/docs/configuration.md#on-unsupported); field-level coverage notes also explain conversions and omissions.

### Shared skills and agent discovery

Many targets share `.agents/skills/`. Byte-identical folders are written once. [`sync.shared-skills`](@/docs/configuration.md#syncshared-skills) can replace identical copies in other directories with symlinks. A tool can also discover folders written for another selected target, even when the spec excludes it.

**Devin discovers `.agents/agents/` profiles written for Goose, OpenHands, and Antigravity.** Those profiles can lack Devin's `allowed-tools`, which defaults to all tools. A same-name `.devin/agents/` profile can also appear. Check duplicate names and restrictions in the discovered profiles; `targets:` controls which adapter writes a file, not which tools read it. Sync reports shared-profile writers. See [Windsurf / Devin](@/docs/targets/windsurf.md).

### Hooks loaded through another tool

Cursor, Copilot, and Trae can read `.claude/settings.json` beside their own hook files. Sharing a hook with Claude can run it twice. Cursor enables compatibility reads by default; Trae requires opting in. Copilot has no switch to stop these reads. Use the target pages for the remedy: [Cursor](@/docs/targets/cursor.md#third-party-reads), [Copilot](@/docs/targets/copilot.md#hooks), [Trae](@/docs/targets/trae.md).

A portable hook's Claude copy exits under Cursor when both targets are selected. Other hooks still need review. Cursor can also ignore a Claude hook's `args`, leaving a bare interpreter; use a shell-form command or turn off compatibility reads.

Event names, tool matcher names, timeout units, and failure behavior differ. Prefer [portable events](@/docs/spec-format/hook-reference.md#portable-events) where supported, and test the actual native event before depending on a guard.

## Memory and local state

[Shared memory](@/docs/memory.md) adds a common store alongside each tool's own memory. It does not replace every native memory feature. Claude Code can use the shared personal folder for its own store; other tools have different integrations. Keep facts that should reach all tools in shared memory, and project conventions in rules.

For native stores, defaults, and importing facts, see [each tool's own memory](@/docs/memory.md#each-tools-own-memory). For scoped Claude agent stores, see [agent memory](@/docs/targets/claude-reference.md#agent-memory).

## Selecting targets

Select only the tools you use in `agnostic-ai.yaml`. A per-run `-t` list replaces that selection for the run. `init` offers detected tools, or Claude and Codex when none are detected. See [target selection](@/docs/configuration.md#targets).

## Global output

Home instructions, shared global paths, supported user-level outputs, and configuration-root environment variables live in [global configuration](@/docs/global-configuration.md). Global and project settings have different ownership and native precedence.

## New targets

To request a target, [open an issue](https://github.com/Chemaclass/agnostic-ai/issues). To implement one, follow [adding adapters](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/adding-adapters.md).
