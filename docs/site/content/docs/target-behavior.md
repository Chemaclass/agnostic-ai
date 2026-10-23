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

One spec can become different files for each tool. A rule becomes `.claude/rules/<name>.md` for Claude Code and a section of `AGENTS.md` for Codex. Tools also differ in how they find files, in permissions, and in the fields they support. [Compare targets](@/docs/compare.md) shows sample output; `agnostic-ai compare` checks your own specs.

## Directory-specific instructions

A rule's `scope` limits it to a directory, through native file conditions or nested instruction files. Tools differ in support and in when they load the rule: some at the repository root, some while editing. Check [directory-specific instructions](@/docs/scoped-context.md) before you share scoped rules.

## Entry-point files

Sync copies `.agnostic-ai/AGNOSTIC_AI.md` into each selected tool's instruction file. The next sync overwrites edits to those copies. See [entry-point files](@/docs/configuration.md#entry-point-files) for what to write in the source.

| Instruction file | Targets |
|---|---|
| `CODEBUDDY.md` | [CodeBuddy Code](@/docs/targets/codebuddy.md) |
| `AGENTS.md` | Codex, Amp, Warp, Cline, Windsurf, Junie, Kiro, Crush, Trae, Jules, Goose, Augment, Qoder, OpenHands, Factory, Kilo, OpenCode |
| `CLAUDE.md` | Claude Code |
| `GEMINI.md` | Gemini CLI |
| `CONVENTIONS.md` | Aider |
| `.github/copilot-instructions.md` | Copilot |
| `.rules` | Zed |
| `.agents/AGENTS.md` | Antigravity |

Tools that share a path get one write. Junie also has its preferred `.junie/AGENTS.md`. Claude can import identical shared instructions from `AGENTS.md`. Cursor and Continue get rule files, not a root instruction file. Each [target page](@/docs/targets/_index.md) explains how that tool finds its files.

### Inlined rules

Some tools read no separate rule directory. Sync puts their project-wide rules below the shared body in a marked `## Rules` block. Augment and Kilo use that block too, though they have rule directories. OpenHands uses it for always-on rules; path-triggered rules become skills.

Cline, Kiro, Qoder, Kilo, and Augment skip a native rule file when its body already appears in the root `AGENTS.md` they read. File-conditioned rules and tool-specific bodies keep their own files. Keep `AGENTS.md` enabled in Cline's Rules panel and Qoder's context file list, or those tools miss the shared rules.

Trae keeps every rule file because reading `AGENTS.md` is optional. Windsurf keeps them for legacy compatibility. Enabling those tools beside an inlining target can load the same rule twice.

A tool whose entry point moves off root `AGENTS.md` keeps its separate rule files. Listing `AGENTS.md` in `sync.unmanaged` stops writes for every target that shares the file, not for one tool.

The next full sync removes obsolete generated rule files, even edited ones that still carry the generated header. Before that cleanup, `sync --check`, `doctor`, and `status` report `ledger` drift; `doctor --fix` removes it.

### Shared settings files

In mergeable files such as `.claude/settings.json`, `.gemini/settings.json`, `.cursor/cli.json`, and `opencode.json`, sync records the keys it owns and keeps the rest. A generated key you left unchanged goes away when no spec produces it. A value you edited stays as an orphan; `doctor --fix` asks before removing it.

Sync tracks MCP servers in mergeable maps by name. A spec with the same name can replace your server and print a note. Name tracking does not apply to files sync rebuilds whole, such as Claude's `.mcp.json` or Cursor's `.cursor/mcp.json`. Check the target's import and overwrite behavior before the first sync.

When a target leaves the setup, cleanup removes only its recorded values. Sync deletes a shared settings file only when it created the file and nothing else remains. Invalid or edited orphan files stay for you to review.

### Other entry-point options

- A partial sync still refreshes shared instruction files for all configured readers, so the body matches a full sync.
- `outputs.<target>.rules-file` selects a legacy merged-rules layout instead of that tool's normal shared-instructions output.
- `sync.target-overview: true` adds generated-file locations to each entry point.

See [configuration](@/docs/configuration.md) for these options.

## Spec kind behavior

The [tool support table](@/docs/targets/_index.md#capability-matrix) shows what sync writes, not that the native feature matches in full. Sync can still leave out native fields, a client can require trust, and a shared prompt does not always enforce a restriction.

| State | What to do |
|---|---|
| Native | Use the target's own format; check field and client limits on its page. |
| Mapped | Check what feature carries the content. An agent prompt written as a skill is not a native subagent. |
| Opt-in | Enable the named output setting, then check native discovery. A legacy file can be written without the tool loading it. |
| Source only | Keep the portable source; no default native artifact is written. A pointer does not activate the feature. |
| No output | Remove that target from the spec or use a supported native route. Sync warns by default. |

Each [spec reference](@/docs/spec-format/_index.md) has the details. [`on-unsupported`](@/docs/configuration.md#on-unsupported) sets what sync does with an unsupported kind. Coverage notes name the fields a tool converts or leaves out.

### Shared skills and agent discovery

Many targets share `.agents/skills/`. Sync writes byte-identical folders once. Crush and Goose share neutral skill frontmatter; Crush commands remain skipped at that shared path because their user-only flag cannot be kept. [`sync.shared-skills`](@/docs/configuration.md#syncshared-skills) can replace identical copies in other directories with symlinks. A tool can also find folders written for another selected target, even when the spec excludes that tool.

**Devin discovers `.agents/agents/` profiles written for Goose, OpenHands, and Antigravity.** Those profiles can lack Devin's `allowed-tools`, which defaults to all tools. A `.devin/agents/` profile with the same name can also appear. Check the profiles Devin finds for duplicate names and tool limits: `targets:` controls which adapter writes a file, not which tools read it. Sync reports which targets write shared profiles. See [Windsurf / Devin](@/docs/targets/windsurf.md).

### Hooks loaded through another tool

Cursor, Copilot, and Trae can read `.claude/settings.json` beside their own hook files, so a hook shared with Claude can run twice. Cursor reads it by default; Trae only when you opt in. Copilot has no switch to stop these reads. The target pages give the fix: [Cursor](@/docs/targets/cursor.md#third-party-reads), [Copilot](@/docs/targets/copilot.md#hooks), [Trae](@/docs/targets/trae.md).

When both targets are selected, a portable hook's Claude copy exits under Cursor. Review other hooks yourself. Cursor can also ignore a Claude hook's `args` and run a bare interpreter; use a shell-form command or turn off compatibility reads.

Event names, tool matcher names, timeout units, and failure behavior differ by tool. Use [portable events](@/docs/spec-format/hook-reference.md#portable-events) where a tool supports them, and test the native event before you rely on a guard.

## Memory and local state

[Shared memory](@/docs/memory.md) adds one store that every tool reads, beside each tool's own memory. Claude Code can keep its own store in the shared personal folder; other tools connect in other ways. Put facts every tool needs in shared memory, and project conventions in rules.

For native stores, defaults, and importing facts, see [each tool's own memory](@/docs/memory.md#each-tools-own-memory). For scoped Claude agent stores, see [agent memory](@/docs/targets/claude-reference.md#agent-memory).

[Pi](@/docs/targets/pi.md) reads ancestor `AGENTS.md` files at session startup. Its target page explains directory scope and project trust.

## Selecting targets

List only the tools you use in `agnostic-ai.yaml`. A `-t` list replaces that list for one run. `init` offers the tools it detects, or Claude and Codex when it finds none. See [target selection](@/docs/configuration.md#targets).

## Global output

[Global configuration](@/docs/global-configuration.md) covers home instructions, shared global paths, user-level outputs, and the environment variables that move a tool's config folder. Global and project settings differ in who owns them and in which one the tool prefers.

## New targets

To request a target, [open an issue](https://github.com/Chemaclass/agnostic-ai/issues). To implement one, follow [adding adapters](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/adding-adapters.md).
