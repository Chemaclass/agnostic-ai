+++
title = "Google Antigravity"
description = "How agnostic-ai emits Google Antigravity configuration: native paths, capability limits, and output options."
weight = 140

[extra]
group = "Reference"
target_id = "antigravity"
+++

# Google Antigravity (`antigravity`)

## Output

```
.agent/AGENTS.md               # canonical entry-point pointer body (written by sync)
.agents/rules/<name>.md        # one per rule
.agents/agents/<name>/agent.md # one per agent (custom subagent)
.agents/skills/<name>/SKILL.md # one folder per skill (Antigravity's native path)
.agents/mcp_config.json        # when MCP entries exist
```

Antigravity reads project instructions from a top-level AGENTS.md-style file, per-rule files under `.agents/rules/`, and custom subagents under `.agents/agents/`. The adapter emits all three.

The entry-point path stays under `.agent/` (singular) to avoid clashing with codex / amp / warp at the project-root `AGENTS.md`. Rules, skills, and MCP default to the plural `.agents/` form Antigravity itself now prefers ([rules](https://antigravity.google/docs/ide/rules), [skills](https://antigravity.google/docs/ide/skills)), which still "maintains backward support" for the singular paths.

A stale managed tree at the pre-plural `.agent/rules` / `.agent/skills` defaults is swept on sync unless `outputs.antigravity.rules-dir` / `skills-dir` opts back into the legacy path explicitly.

- **Agents**: one custom subagent per agent at `.agents/agents/<name>/agent.md`, the nested workspace form documented alongside the flat form in [Antigravity's subagent reference](https://antigravity.google/docs/subagents).
  - Goose and OpenHands only scan top-level `.md` files in the same root, so the nested form keeps Antigravity's restricted `model` tier separate from their free-form model IDs (#717). Frontmatter carries `name` and `description`, both required; the body defines the system prompt.
  - A managed flat profile from an earlier sync is removed by the sync ledger when no enabled target still writes it. When Goose or OpenHands is enabled, that flat path remains as their current shared output. `import antigravity` prefers nested profiles and falls back to legacy flat files only when no nested profile exists, so it does not ingest co-located Goose or OpenHands agents.
  - Agents previously flattened into `.agents/rules/agent-<name>.md`, a path the subagent loader never reads (#638); a managed copy at the old name is swept for every current agent.
  - **Devin reads this tree too.** With `windsurf` in `targets`, one agent spec produces both this profile and `.devin/agents/<name>.md`, and Devin discovers both under one name. Only the Devin copy carries `allowed-tools`. See the [cross-cutting agents note](@/docs/targets/_index.md) for what that costs and the workaround (#863).
  - A generic `tools` list never reaches this file: Antigravity's vocabulary is its own (`view_file`, `replace_file_content`, `grep_search`, `run_command`, ...) with no name in common with agnostic-ai's Claude-style set, and the vendor warns that "Specifying an unmapped or misspelled tool name in the `tools` list may cause the subagent process to hang during execution". It drops with a coverage note; set `x-antigravity.tools` to write Antigravity's own names.
  - `model` is a three-value tier enum (`inherit`, `flash`, `pro`), not a model ID, so a value outside it drops the same way.
  - Every other documented key (`mainAgent`, `subagent`, `commandExecutionPolicy`, `mcpServers`, `skills`/`plugins`) reaches the file through `x-antigravity` too.
- **Skills**: one folder per skill under `.agents/skills/<name>/SKILL.md`, Antigravity's [native skills layout](https://codelabs.developers.google.com/getting-started-with-antigravity-skills). It's the same tree Codex, Amp, Zed, Crush, and OpenHands share, so identical skill folders dedupe. The SKILL.md frontmatter is reduced to `name` + `description`; the body follows. Sibling files next to the source SKILL.md (helper scripts, fixtures) are copied byte-for-byte into the emitted folder.
- **MCP**: servers land in `.agents/mcp_config.json` under a single `mcpServers` object ([antigravity.google/docs/ide/mcp](https://antigravity.google/docs/ide/mcp)).
  - Remote servers carry `serverUrl`. The vendor doc states the legacy `url` / `httpUrl` field names "are not supported," so this is a dedicated schema, not the shared `mcpServers`-with-`url` shape claude and cursor use.
  - stdio servers carry `command`, `args`, `env`, and `cwd`; remote servers add `headers`.
  - Both transports accept `disabled` under that literal name (see [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target)), unlike codex and kilo which map it onto their own `enabled: false`.
  - Three more documented fields (`authProviderType`, `oauth`, `disabledTools`, same page) have no dedicated mapping. They, `description`, `roots`, and any field the vendor adds next reach the file through `x-antigravity` instead, the same escape hatch Zed and Warp give their own unmapped fields.
  - `import antigravity` reads `.agents/mcp_config.json` back, renaming `serverUrl` to the spec's generic `url` and preserving any other field under `x-antigravity` the same way.
- **Hooks**: merge into `.agents/hooks.json` (override via `outputs.antigravity.hooks-file`): "Hooks are configured in a `hooks.json` file located in your customization directory (e.g., `.agents/` in your workspace)" ([antigravity.google/docs/ide/hooks](https://antigravity.google/docs/ide/hooks), #629).
  - **The file is keyed by hook definition name, not by event**, unlike every other hook target here, so each spec becomes its own top-level definition named after it and holding the one event it names.
  - That is also where `enabled` lives: a spec's `disabled: true` writes `enabled: false` on its own definition ("Set to `false` to disable the hook without removing it"), a sibling of the event key rather than a member of the handler array. This is the inverse spelling of the literal `disabled` this same adapter writes for MCP servers.
  - Five events exist: `PreToolUse`, `PostToolUse`, `PreInvocation`, `PostInvocation`, `Stop`. The first two hold `{matcher, hooks: [...]}` groups; for the other three "the structure is simpler (a list of handlers directly under the event key) and the matcher is ignored". This adapter writes each shape where the vendor documents it and notes a matcher set on the three that ignore it.
  - Per handler: `type` (optional, defaulting to `"command"`, written explicitly here), `command`, and optional `timeout` (seconds, vendor default 30). An event outside the documented five is skipped with a coverage note rather than written as a key no handler backs.
  - **Antigravity names its own tools** (`view_file`, `replace_file_content`, `grep_search`, `run_command`, ...) with none in common with Claude's, so a matcher carried over from a Claude spec parses as a valid regex and then matches nothing. That case emits verbatim with a coverage note rather than a guessed rename, the same line OpenHands, Crush, and Windsurf hold.
  - The hook payload's `transcriptPath` resolves under `~/.gemini/antigravity-ide`, the IDE's own app-data directory, confirming the IDE itself runs them (target-audit 2026-08-27, #563).

Commands are unconfirmed in the public-preview docs and skip with a warning. Add `on-unsupported: silent` to suppress it.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.antigravity.rules-dir` | `.agents/rules` | |
| `outputs.antigravity.agents-dir` | `.agents/agents` | |
| `outputs.antigravity.skills-dir` | `.agents/skills` | |
| `outputs.antigravity.mcp-file` | `.agents/mcp_config.json` | |
| `outputs.antigravity.hooks-file` | `.agents/hooks.json` | |
| `outputs.antigravity.rules-file` | unset | writes a legacy merged document and skips the pointer-body write |

## Import

`agnostic-ai import antigravity` prefers `.agents/rules` and `.agents/skills/`. It falls back to the singular `.agent/rules` and `.agent/skills/` only when the preferred directory is absent. Skill imports include all bundled assets.

Agents and MCP servers import as described under **Agents** and **MCP**: nested profiles win over legacy flat files, and `serverUrl` becomes `url`.

## Verify

1. Install Antigravity from the Google Antigravity public-preview download page.
2. Check the tree: `ls .agent/AGENTS.md .agents/rules/ .agents/agents/ .agents/skills/`, `grep "Generated by agnostic-ai" .agents/rules/*.md` for the provenance header (it sits after the frontmatter), `test -f .agents/skills/*/SKILL.md`, and `python -m json.tool .agents/mcp_config.json > /dev/null` when MCP specs exist.
3. Open the project; it surfaces `.agent/AGENTS.md` in the project-instructions panel with no "unrecognized file" warnings.
4. Open one of `.agents/rules/<name>.md` and verify the per-rule file is picked up; confirm each `.agents/skills/<name>/SKILL.md` loads as a skill, and any `.agents/mcp_config.json` server appears in the MCP panel.
5. Ask the agent to delegate to a custom subagent by name; each `.agents/agents/<name>/agent.md` profile is selectable in the subagent panel and its task runs to Idle rather than hanging (a hang points at an unmapped `tools` name, so check `x-antigravity.tools` against the vendor's vocabulary).
6. Trigger an agent action (e.g. ask for a refactor); the rules apply, with no schema-validation log entries referencing `.agents/`.
