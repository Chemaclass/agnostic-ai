+++
title = "Windsurf / Devin Desktop"
description = "How agnostic-ai emits Windsurf / Devin Desktop configuration: native paths, capability limits, and output options."
weight = 80

[extra]
group = "Reference"
target_id = "windsurf"
+++

# Windsurf / Devin Desktop (`windsurf`)

Windsurf became Devin Desktop (2026-06). The target keeps its `windsurf` name, so existing `outputs.windsurf.*` keys and `x-windsurf` meta still work.

## Output

```
AGENTS.md                            # shared pointer body, plus the rules block when an inlining target shares it
.devin/rules/<name>.md
<scope>/.devin/rules/<name>.md       # one per scoped rule
.devin/agents/<name>.md              # one per agent (custom subagent profile)
.agents/skills/<name>/SKILL.md       # one folder per skill (shared tree)
.devinignore                         # when ignore entries exist (indexing and agent access)
.windsurfignore                      # when ignore entries exist (legacy name, older builds)
.devin/mcp_config.json               # when MCP entries exist
.devin/hooks.v1.json                 # when hook entries exist
.devin/config.json                   # when settings entries carry permission rules
```

- **Rules**: Devin prefers `.devin/rules/*.md` and still reads `.windsurf/rules/` (`.windsurfrules` is legacy). Set `outputs.windsurf.rules-dir: .windsurf/rules` to keep the old layout. Otherwise sync sweeps managed leftovers there and leaves hand-authored files alone.
- **Entry point**: Devin also reads the root `AGENTS.md`, so `sync` writes the shared pointer body there.
  - When codex or another inlining target adds the `## Rules` block, Devin loads each always-on rule twice, since every rule keeps its `.devin/rules/` file.
  - Legacy Cascade [limits](https://docs.devin.ai/desktop/cascade/memories#rules) each workspace rule file to 12,000 characters and processes `AGENTS.md` through the same Rules engine. The [Devin CLI rules reference](https://docs.devin.ai/cli/extensibility/rules) specifies no size limit. Sync keeps the per-rule files for compatibility; see [target behavior](@/docs/target-behavior.md#entry-point-files).
- **Agents**: custom subagent profiles ([subagents docs](https://docs.devin.ai/cli/subagents)) with `name`, `description`, `model`, `allowed-tools`, and `max-nesting`. The body is the system prompt. A scoped agent lands flat, since Devin discovers sub-directories for rules only.
  - `allowed-tools` maps Claude-style names to Devin's ([permissions reference](https://docs.devin.ai/cli/reference/permissions), [CLI changelog](https://docs.devin.ai/cli/changelog/stable.md)): `Read`/`Grep`/`Glob`/`Bash` to `read`/`grep`/`glob`/`exec`, and `Write`/`Edit` to `write`/`edit`. `write` needs Devin CLI v3000.11.1 or later. `mcp__<server>__<tool>` passes through. Anything else drops with a coverage note.
  - **Devin also reads `.agents/agents/`**, flat `<name>.md` and nested `<name>/agent.md`. With Antigravity, Goose, or OpenHands in `targets`, Devin sees two profiles with one name, and only this one carries `allowed-tools`. See the [cross-cutting agents note](@/docs/targets/_index.md).
  - `x-windsurf.allowed-tools` writes Devin's names directly. `x-windsurf.max-nesting` sets nesting, which has no generic field. `model` passes through verbatim.
  - Devin marks custom subagents experimental; the format may change.
- **Scoped rules**: land at `<scope>/.devin/rules/<name>.md`, since Desktop discovers `.devin/rules` or `.windsurf/rules` in any sub-directory ([Desktop rules reference](https://docs.devin.ai/desktop/cascade/memories)). Devin CLI v3000.11.1 also discovers rules recursively under `.devin/rules/` and `.windsurf/rules/` ([CLI changelog](https://docs.devin.ai/cli/changelog/stable.md)). That is unconfirmed for Desktop, so the layout stays.
  - With `outputs.windsurf.rules-dir` set, the prefix follows it: `<scope>/.windsurf/rules/<name>.md`.
  - Devin CLI loads a sub-directory rules dir once the agent touches files there. Desktop loads all of them at start. The scope narrows only what the CLI sees.
- **Rule activation**: a rule carries a `trigger` frontmatter key when it sets `alwaysApply: false`, or sets `globs` other than a catch-all such as `**/*` without `alwaysApply`.

  | Rule has | Emitted trigger |
  | --- | --- |
  | `globs` | `trigger: glob` (plus the pattern verbatim) |
  | `description` alone | `trigger: model_decision` |
  | neither | `trigger: manual` |

  An always-on rule stays bare: Devin loads a file without frontmatter as always-on, its body in every message's system prompt, so a `description` has no job. Devin's fifth value, `agent`, has no counterpart in the spec format, so it is never emitted.
- **Skills**: `.agents/skills/<name>/SKILL.md`. Devin reads `.agents/skills/`, `.devin/skills/`, and `.windsurf/skills/`. This adapter writes the first, so skills dedupe with every other target that writes `.agents/skills/`. Sibling assets and modes survive.
  - Native `triggers` live under `x-windsurf` in the spec and return to top-level frontmatter on sync. This keeps `[user]`, `[model]`, and combined invocation policy without leaking a Devin-only field to other targets.
- **Ignore**: ignore specs emit as `.devinignore` and `.windsurfignore`, gitignore syntax under a `#` provenance header. Devin skips matched paths when indexing and cannot view, edit, or create them ([`.devinignore` docs](https://docs.devin.ai/desktop/context-awareness/devin-ignore)).
  - Devin still enforces the legacy `.windsurfignore` and `.codeiumignore`. `outputs.windsurf.ignore-file: .codeiumignore` writes that name instead of `.devinignore`.
  - `.windsurfignore` is for older Windsurf builds, where only it blocked agent file access. Sync always writes it, unless `outputs.windsurf.ignore-file` names it. That key moves the main file only.
- **MCP**: merges into `.devin/mcp_config.json` under `mcpServers`, which [Devin Local](https://docs.devin.ai/desktop/devin-local) reads for project scope. Cascade, with its own MCP file, was removed in Devin Desktop v3.9.19 ([changelog](https://docs.devin.ai/desktop/changelog.md), [Cascade MCP page](https://docs.devin.ai/desktop/cascade/mcp)).
  - [The schema](https://docs.devin.ai/cli/extensibility/mcp/configuration) uses `command`/`args`/`env` for stdio and `url`/`transport` (`http` or `sse`)/`headers`/`oauthClientId`/`oauthClientSecret`/`oauthResource` for remote. The key is `transport`, not `type`, so this adapter has its own schema.
  - Both accept `disabled`, also toggled by `devin mcp enable|disable` (see [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target)).
  - A `type: ws` spec emits no server, with a coverage note: Devin remotes are only `http` and `sse`.
  - Cascade's user-tier `~/.codeium/windsurf/mcp_config.json` is out of reach, since agnostic-ai writes project files only.
- **Hooks**: merge into `.devin/hooks.v1.json` ([hooks docs](https://docs.devin.ai/cli/extensibility/hooks/overview)). Eight events: `PreToolUse`, `PostToolUse`, `PermissionRequest`, `UserPromptSubmit`, `Stop`, `PostCompaction`, `SessionStart`, `SessionEnd`.
  - The file is the hooks object, `{"<Event>": [...]}`, with no `hooks` wrapper (unlike Claude Code, Codex, Gemini, and Qoder).
  - Per entry: `type`, `command`, and optional `timeout` (seconds). `type` is `"command"`, or `"prompt"` for an LLM prompt. A hook spec reaches `"prompt"` only through a hand-authored `type`/`prompt` Meta pair.
  - `matcher` is a regex on `tool_name` for `PreToolUse`, `PostToolUse`, and `PermissionRequest`. **Devin CLI names tools in lowercase snake_case** (`exec`, `edit`, `read`, `write`, `apply_patch`, `grep`, `glob`, `webfetch`, ...), so a Claude-style matcher matches nothing. Sync gives it a coverage note, not a rename.
  - [`agnostic-ai hook run`](@/docs/spec-format/hooks.md#hook-run) runs them with Devin CLI's payload on an [assumed shell, working directory, and timeout](@/docs/spec-format/hooks.md#assumed-results). `--edit` is listed as not run, since the edit tools' input is undocumented.
- **Settings**: portable `permissions` merge into `.devin/config.json`, the committed project policy ([permissions reference](https://docs.devin.ai/cli/reference/permissions)).
  - Project configs accept only `permissions`, `read_config_from`, and `hooks` ([config file reference](https://docs.devin.ai/cli/reference/configuration/config-file)). Sync sets only `permissions`, so other keys and siblings inside it survive.
  - An `x-windsurf` block on a settings spec merges into the same file, reaching `read_config_from` and `hooks`. Its `permissions` merge per spec and per list with the translated rules.
  - A portable `model` stays out with a coverage note, since Devin's `agent` block is user-only.
  - Translation: `Read(glob)` and `Write(glob)` pass through, `Edit(glob)` becomes `Write(glob)`, `Bash(prefix:*)` becomes `Exec(prefix)`, `WebFetch(pattern)` becomes `Fetch(pattern)`, and `mcp__<server>__<tool>` passes through.
  - Bare tool names map to Devin's own (`Bash` to `exec`, and so on), separately from `allowed-tools`. `WebSearch` becomes `web_search`, accepted since Devin CLI v3000.10.21 ([CLI changelog](https://docs.devin.ai/cli/changelog/stable.md)), and imports back. A bare `WebFetch` drops, since `webfetch` is not a permissions name.
  - Devin's `Exec` only prefix-matches, so an exact `Bash(...)` rule without `:*` widens. On `allow` and `ask` that would approve unlisted commands, so the rule drops with a coverage note. On `deny` it blocks more, which is the safe direction: `Bash(rm -rf /)` becomes `Exec(rm -rf /)`. A command deny beats a broader `ask` or `allow` (Devin CLI v3000.10.31).
  - `x-windsurf.permissions` writes Devin's rules directly, replacing that spec's portable ones.
- **Workflows**: `outputs.windsurf.workflows-dir` emits nothing and only prints a warning naming the migration to skills. `.devin/agents/<name>.md` emits either way.

{% <details summary="Older paths and removed features"> %}
- `outputs.windsurf.workflows-dir` wrote each agent as a Cascade Workflow. Devin Desktop v3.9.19 removed Cascade ([changelog](https://docs.devin.ai/desktop/changelog.md)). Devin Local does not support Workflows and points users to skills ([Devin Local limitations](https://docs.devin.ai/desktop/devin-local)).
- Sync sweeps a managed `.devin/rules/agent-<name>.md` for every current agent, and the old `.devin/rules/<scope>/<name>.md` tree.
- The MCP file moved here in v3000.3 (Local 3.6). Devin migrates `mcpServers` from the older `.devin/config.json` on startup, so the path works for both.
- Devin CLI also reads a gitignored `.devin/mcp_config.local.json` for personal secrets, which this adapter does not write. The generated `.devin/mcp_config.json` is in the managed `.gitignore` block by default (`gitignore.enabled: true`), so you do not need that split.
{% </details> %}

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.windsurf.rules-dir` | `.devin/rules` | |
| `outputs.windsurf.agents-dir` | `.devin/agents` | |
| `outputs.windsurf.skills-dir` | `.agents/skills` | |
| `outputs.windsurf.workflows-dir` | empty | set, it only warns (see above) |
| `outputs.windsurf.ignore-file` | `.devinignore` | moves the main ignore file only; the legacy `.windsurfignore` copy follows the same spec |
| `outputs.windsurf.mcp-file` | `.devin/mcp_config.json` | |
| `outputs.windsurf.hooks-file` | `.devin/hooks.v1.json` | |
| `outputs.windsurf.conf-file` | `.devin/config.json` | project config; only `permissions` is ever set |

## Import

`agnostic-ai import windsurf` reads:

- **Rules**: from `outputs.windsurf.rules-dir` when set, else `.devin/rules/`, then legacy `.windsurf/rules/`, reclassified by [filename prefix](@/docs/cli-reference/start.md#filename-prefix-reclassification). Scoped copies (`<scope>/<rules-dir>/*.md`) rebuild scoped rules, including scopes like `.github` or `vendor`. It skips every `node_modules/`.
- **Permissions**: from `.devin/config.json`. `Exec` has no exact-command form, so it imports as `Bash(<cmd>:*)`, not `Bash(<cmd>)`.
- **Skills**: from `.agents/skills/`, `.devin/skills/`, then `.windsurf/skills/`; the first same-name skill wins. Bundled assets and executable modes survive. `triggers` move under `x-windsurf`.
- **Ignore**: from `.devinignore`, or `.windsurfignore` when the first is absent. When both files have different hand-written patterns, combine them in the imported ignore spec, in order, before you sync.

## Protected paths

Advisory. This target has no native edit guard that sync writes, so sync prints a coverage note for [protected paths](@/docs/spec-format/settings.md#protected-paths). To tell the agent about them, list the paths in a rule.

## Verify

1. Install Devin Desktop from [devin.ai](https://devin.ai) (formerly windsurf.com).
2. Check the tree: `ls .devin/rules/ .devin/agents/ .agents/skills/`, `grep "Generated by agnostic-ai" .devin/rules/*.md`, `ls .agents/skills/*/SKILL.md >/dev/null`, `python -m json.tool .devin/mcp_config.json > /dev/null` when MCP specs exist, `python -m json.tool .devin/hooks.v1.json > /dev/null` when hook specs exist.
3. Open the project. Devin Local loads every `.devin/rules/*.md` ([rules docs](https://docs.devin.ai/cli/extensibility/rules)) into the Rules panel without "failed to parse" warnings. Each `.agents/skills/<name>/` loads as a skill.
   A scoped rule at `<scope>/.devin/rules/<name>.md` shows there too, and a `trigger:` rule shows that mode instead of Always On.
   Ask Devin CLI for a subagent by name (`review this using the <name> subagent`). Each `.devin/agents/<name>.md` lists beside the built-in `subagent_explore` and `subagent_general` without a "profile skipped" warning.
4. With ignore specs, `cat .devinignore` shows the patterns and indexing skips them. `cat .windsurfignore` matches, and the agent refuses to open those paths.
5. Each `mcpServers.<name>` from `.devin/mcp_config.json` connects in Devin Local, and a disabled spec shows disabled.
6. `/hooks` in Devin CLI lists each `.devin/hooks.v1.json` entry with that file as its source.
7. With settings specs, `python -m json.tool .devin/config.json > /dev/null` parses, a `deny` command is refused, and an `allow` one runs without a prompt.
