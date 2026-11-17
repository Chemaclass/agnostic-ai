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

## Memory and local state {#memory-and-local-state .legacy-anchor}

## Global output {#global-output .legacy-anchor}

One spec becomes different files per tool: a rule is `.claude/rules/<name>.md` for Claude Code and a section of `AGENTS.md` for Codex. Discovery, permissions, and fields differ too. [Compare targets](@/docs/compare.md) shows samples; `agnostic-ai compare` checks your specs.

## Directory-specific instructions

A rule's `scope` limits it to a directory through native file conditions or nested instruction files. Support and load timing differ by tool (repository root or while editing). See [directory-specific instructions](@/docs/scoped-context.md) before sharing scoped rules.

## Entry-point files

Sync copies `.agnostic-ai/AGNOSTIC_AI.md` into each selected tool's instruction file and overwrites edits to the copies. See [entry-point files](@/docs/configuration.md#entry-point-files).

| Instruction file | Targets |
|---|---|
| `CODEBUDDY.md` | [CodeBuddy Code](@/docs/targets/codebuddy.md) |
| `AGENTS.md` | Codex, Amp, Warp, Cline, Windsurf, Junie, Kiro, Crush, Trae, Jules, Goose, Augment, Qoder, OpenHands, Factory, Kilo, OpenCode, Mistral Vibe |
| `CLAUDE.md` | Claude Code |
| `GEMINI.md` | Gemini CLI |
| `QWEN.md` | [Qwen Code](@/docs/targets/qwen.md) |
| `CONVENTIONS.md` | Aider |
| `.github/copilot-instructions.md` | Copilot |
| `.rules` | Zed |
| `.agents/AGENTS.md` | Antigravity |
| `.kimi-code/AGENTS.md` | Kimi Code CLI |

- Tools that share a path get one write.
- Junie also has its preferred `.junie/AGENTS.md`.
- Claude can import identical shared instructions from `AGENTS.md`.
- Cursor and Continue get rule files, not a root instruction file.
- Each [target page](@/docs/targets/_index.md) explains how that tool finds its files.

### Inlined rules

Tools with no rule directory get project-wide rules in a marked `## Rules` block below the shared body.

| Tool | Rules block holds |
|---|---|
| Augment, Kilo | Project-wide rules (they have rule directories too) |
| OpenHands | Always-on rules; path-triggered rules become skills |
| Qwen Code | Rules without file patterns; a rule with `globs` goes to `.qwen/rules/` and loads when a matching file is read or edited |

<details>
<summary>Skipped rule files, Trae, and cleanup</summary>

- Cline, Kiro, Qoder, Kilo, Augment, Cursor, and Windsurf skip a native rule file when its body already appears in the root `AGENTS.md` they read. File-conditioned rules and tool-specific bodies keep their own files.
- Keep `AGENTS.md` enabled in Cline's Rules panel and Qoder's context file list, or they miss the shared rules.
- Trae reads `AGENTS.md` only after a settings switch is on, so it keeps every rule file by default. With the switch on, set `outputs.trae.rules-mode: entry-point` to skip them. See [Trae](@/docs/targets/trae.md#loading-and-limits).
- A tool whose entry point moves off root `AGENTS.md` keeps its own rule files.
- `AGENTS.md` in `sync.unmanaged` stops writes for every target that shares the file.
- The next full sync removes obsolete generated rule files, even edited ones that keep the generated header. Until then `sync --check`, `doctor`, and `status` report `ledger` drift; `doctor --fix` removes it.

</details>

### Shared settings files

In mergeable files such as `.claude/settings.json`, `.gemini/settings.json`, `.cursor/cli.json`, and `opencode.json`, sync records the keys it owns and keeps the rest. A generated key you left unchanged goes away when no spec produces it. A value you edited stays as an orphan; `doctor --fix` asks before removing it.

<details>
<summary>MCP names, Warp, and cleanup</summary>

- Warp global voice settings use the same per-key ownership in its platform settings file ([Warp voice dictation](@/docs/targets/warp.md#voice-dictation)).
- Sync tracks MCP servers in mergeable maps by name. A spec with the same name can replace your server and print a note.
- Name tracking does not apply to files sync rebuilds whole, such as Claude's `.mcp.json` or Cursor's `.cursor/mcp.json`. Check the target's import and overwrite behavior before the first sync.
- When a target leaves the setup, cleanup removes only its recorded values.
- Sync deletes a shared settings file only if it created it and nothing else remains. Invalid or edited orphan files stay for review.

</details>

### Other entry-point options

A partial sync still refreshes shared instruction files for all configured readers. For the [legacy merged rules file](@/docs/configuration.md#entry-point-files) and [`sync.target-overview`](@/docs/sync-settings.md#synctarget-overview), see those pages.

## Spec kind behavior

The [tool support table](@/docs/targets/_index.md#capability-matrix) lists generated output. Check field support, trust requirements, and permission enforcement per target.

| State | What to do |
|---|---|
| Native | Use the target's own format; check field and client limits on its page. |
| Mapped | Check what feature carries the content. An agent prompt written as a skill is not a native subagent. |
| Opt-in | Enable the named output setting, then check native discovery. A legacy file can be written without the tool loading it. |
| Source only | Keep the portable source; no default native artifact is written. A pointer does not activate the feature. |
| No output | Remove that target from the spec or use a supported native route. Sync warns by default. |

See each [spec reference](@/docs/spec-format/_index.md). [`on-unsupported`](@/docs/configuration-reference.md#on-unsupported) sets what sync does with an unsupported kind.

### Shared skills and agent discovery

- Many targets share `.agents/skills/`; sync writes identical folders once.
- Crush and Goose share neutral skill frontmatter. Crush commands stay skipped at that path because their user-only flag cannot be kept.
- [`sync.shared-skills`](@/docs/sync-settings.md#syncshared-skills) can replace identical copies elsewhere with symlinks.
- A tool can find folders written for another selected target, even if the spec excludes that tool.

<details>
<summary>Devin reads profiles written for other tools</summary>

- Devin discovers `.agents/agents/` profiles written for Goose, OpenHands, and Antigravity.
- Those can lack Devin's `allowed-tools`, which defaults to all tools. A same-name `.devin/agents/` profile can also appear.
- `targets:` controls which adapter writes a file, not which tools read it. Check what Devin finds for duplicate names and tool limits.
- Sync reports which targets write shared profiles. See [Windsurf / Devin](@/docs/targets/windsurf.md).

</details>

### Hooks loaded through another tool

Cursor, Copilot, and Trae can read `.claude/settings.json` beside their own hooks, so a shared hook can run twice. Cursor reads it by default, Trae only on opt-in, and Copilot has no switch. Fixes: [Cursor](@/docs/targets/cursor-reference.md#third-party-reads), [Copilot](@/docs/targets/copilot-reference.md#hooks), [Trae](@/docs/targets/trae.md).

<details>
<summary>Hook differences between tools</summary>

- With both targets selected, a portable hook's Claude copy exits under Cursor. Review other hooks yourself.
- Cursor can ignore a Claude hook's `args` and run a bare interpreter. Use a shell-form command or turn off compatibility reads.
- Event names, matcher names, timeout units, and failure behavior differ. Use [portable events](@/docs/spec-format/hook-reference.md#portable-events) where supported, and test the native event before relying on a guard.

</details>

## Related

- [Shared memory](@/docs/memory.md): one store every tool reads, beside each tool's own memory.
- [Target selection](@/docs/configuration.md#targets): which tools sync writes by default.
- [Global output](@/docs/global-configuration.md#global-output): user-level files and the variables that move a tool's config folder.
- [Adding adapters](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/adding-adapters.md), or [open an issue](https://github.com/Chemaclass/agnostic-ai/issues) to request a target.
