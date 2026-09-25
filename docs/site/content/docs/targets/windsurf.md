+++
title = "Windsurf / Devin Desktop"
description = "How agnostic-ai emits Windsurf / Devin Desktop configuration: native paths, capability limits, and output options."
weight = 80

[extra]
group = "Reference"
target_id = "windsurf"
+++

# Windsurf / Devin Desktop (`windsurf`)

## Output

```
AGENTS.md                            # shared pointer body
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

Windsurf became Devin Desktop (2026-06). Devin prefers `.devin/rules/*.md` and still reads `.windsurf/rules/` as a fallback (`.windsurfrules` is legacy), so rules emit at the preferred path. The target keeps its `windsurf` name, so existing `outputs.windsurf.*` keys and `x-windsurf` meta still work.

Set `outputs.windsurf.rules-dir: .windsurf/rules` to keep the old layout. Otherwise sync sweeps managed leftovers at the old path; hand-authored files survive. Devin also reads the root `AGENTS.md`, so `sync` writes the shared pointer body there.

- **Agents**: one custom subagent profile per agent at `.devin/agents/<name>.md` ([subagents docs](https://docs.devin.ai/cli/subagents)). Frontmatter carries `name`, `description`, `model`, `allowed-tools`, and `max-nesting`; the body is the system prompt.
  - A managed copy at the old `.devin/rules/agent-<name>.md` path is swept for every current agent. A scoped agent lands flat, since Devin discovers sub-directories for rules only.
  - `allowed-tools` translates Claude-style names onto Devin's own ([permissions reference](https://docs.devin.ai/cli/reference/permissions), [CLI changelog](https://docs.devin.ai/cli/changelog/stable.md)): `Read`/`Grep`/`Glob`/`Bash` map to `read`/`grep`/`glob`/`exec`, and `Write`/`Edit` map to `write`/`edit`. `write` needs Devin CLI v3000.11.1 or later. `mcp__<server>__<tool>` passes through. Anything else drops with a coverage note. This table is separate from the permissions one below.
  - **Devin also reads `.agents/agents/`**, in flat `<name>.md` and nested `<name>/agent.md` form. Antigravity, Goose, and OpenHands write there, so with any of them in `targets` Devin sees two profiles with one name, and only this one carries `allowed-tools`. See the [cross-cutting agents note](@/docs/targets/_index.md) for the workaround.
  - `x-windsurf.allowed-tools` writes Devin's vocabulary directly, and `x-windsurf.max-nesting` sets nesting, which has no generic field. `model` passes through verbatim. Devin marks custom subagents as experimental, so the format may change.
- **Scoped rules**: a scoped rule lands at `<scope>/.devin/rules/<name>.md`, following Desktop's discovery of `.devin/rules` or `.windsurf/rules` in any sub-directory ([Desktop rules reference](https://docs.devin.ai/desktop/cascade/memories)). Devin CLI v3000.11.1 also discovers rules recursively under `.devin/rules/` and `.windsurf/rules/` ([CLI changelog](https://docs.devin.ai/cli/changelog/stable.md)), but that does not establish the same behavior in Desktop, so the layout stays. Sync still sweeps the old `.devin/rules/<scope>/<name>.md` tree.
  - With `outputs.windsurf.rules-dir` set, the prefix follows it, so the legacy layout scopes to `<scope>/.windsurf/rules/<name>.md`. Devin CLI loads a sub-directory rules dir when the agent touches files there; Desktop loads all of them at session start. The scope narrows what the CLI sees, not what Desktop sees.
- **Rule activation**: a rule that sets `alwaysApply: false` carries a `trigger` frontmatter key.

  | Rule has | Emitted trigger |
  | --- | --- |
  | `globs` | `trigger: glob` (plus the pattern verbatim) |
  | `description` alone | `trigger: model_decision` |
  | neither | `trigger: manual` |

  An always-on rule stays bare: Devin loads a file with no frontmatter as always-on and puts the full body in the system prompt on every message, so a `description` has no job there. Devin's fifth value, `agent`, has no counterpart in the spec format, so it is never emitted.
- **Skills**: one folder per skill at `.agents/skills/<name>/SKILL.md`. Devin reads `.agents/skills/`, `.devin/skills/`, and `.windsurf/skills/`; this adapter writes the first, so skills dedupe with Codex, Amp, Zed, Crush, OpenHands, Antigravity, Augment, and Kilo.
  - Native `triggers` values live under `x-windsurf` in the source spec and return to top-level frontmatter on sync. This keeps `[user]`, `[model]`, and combined invocation policy without leaking a Devin-only field to other targets. Sibling assets and modes survive.
- **Ignore**: ignore specs emit as both `.devinignore` and `.windsurfignore`, gitignore syntax under a `#` provenance header. Matched paths are excluded from indexing, and Devin cannot view, edit, or create them ([`.devinignore` docs](https://docs.devin.ai/desktop/context-awareness/devin-ignore)).
  - `.windsurfignore` and `.codeiumignore` are legacy names Devin still enforces. Set `outputs.windsurf.ignore-file: .codeiumignore` to write that name instead of `.devinignore`.
  - The `.windsurfignore` copy covers older Windsurf builds, where it was the only file that blocked agent file access. `outputs.windsurf.ignore-file` moves the main file only; `.windsurfignore` is always written too, unless the override names `.windsurfignore` itself.
- **MCP**: merges into `.devin/mcp_config.json` under a root `mcpServers` map. [Devin Local](https://docs.devin.ai/desktop/devin-local) reads this file for project scope. Cascade, which used a different MCP file, was removed in Devin Desktop v3.9.19 ([changelog](https://docs.devin.ai/desktop/changelog.md), [Cascade MCP page](https://docs.devin.ai/desktop/cascade/mcp)).
  - [The schema](https://docs.devin.ai/cli/extensibility/mcp/configuration) uses `command`/`args`/`env` for stdio servers and `url`/`transport` (`http` or `sse`)/`headers`/`oauthClientId`/`oauthClientSecret`/`oauthResource` for remote ones. Both accept `disabled`, which `devin mcp enable|disable` also toggles (see [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target)). The key is `transport`, not `type`, so this adapter has its own schema.
  - The file moved here in v3000.3 (Local 3.6), and Devin migrates `mcpServers` from the older `.devin/config.json` on startup, so this path works for both. Cascade's user-tier `~/.codeium/windsurf/mcp_config.json` is out of reach, since agnostic-ai writes project files only.
  - Devin CLI also reads a gitignored `.devin/mcp_config.local.json` for personal secrets, which this adapter does not write. agnostic-ai's generated `.devin/mcp_config.json` is already in the managed `.gitignore` block by default (`gitignore.enabled: true`), so the split does not apply.
  - A `type: ws` spec emits no server and raises a coverage note, since Devin supports only `http` and `sse` remotes.
- **Hooks**: merge into `.devin/hooks.v1.json` ([hooks docs](https://docs.devin.ai/cli/extensibility/hooks/overview)). Eight events: `PreToolUse`, `PostToolUse`, `PermissionRequest`, `UserPromptSubmit`, `Stop`, `PostCompaction`, `SessionStart`, `SessionEnd`.
  - The hooks object is the whole file, `{"<Event>": [...]}`, with no `hooks` wrapper (unlike Claude Code, Codex, Gemini, and Qoder).
  - Per entry: `type`, `command`, and optional `timeout` (seconds). `type` is `"command"`, or `"prompt"` for an LLM prompt, which the generic hook spec can only reach through a hand-authored `type`/`prompt` Meta pair. `matcher` is a regex on `tool_name`, used by `PreToolUse`, `PostToolUse`, and `PermissionRequest`.
  - **Devin CLI names its tools in lowercase snake_case** (`exec`, `edit`, `read`, `write`, `apply_patch`, `grep`, `glob`, `webfetch`, ...). A Claude-style matcher matches nothing, so it raises a coverage note instead of being renamed.
- **Settings**: the portable `permissions` lists merge into `.devin/config.json` under `permissions`, the committed project policy ([permissions reference](https://docs.devin.ai/cli/reference/permissions)). Project configs accept only `permissions`, `read_config_from`, and `hooks` ([config file reference](https://docs.devin.ai/cli/reference/configuration/config-file)). Only `permissions` is set from portable rules, so the other keys and siblings inside `permissions` survive. An `x-windsurf` block on a settings spec merges into the same file, reaching `read_config_from` and `hooks`; `permissions` there merges per spec and per list with the translated rules.
  - A portable `model` stays out with a coverage note, because Devin's `agent` block is user-only.
  - Rules are translated: `Read(glob)` and `Write(glob)` pass through, `Edit(glob)` becomes `Write(glob)`, `Bash(prefix:*)` becomes `Exec(prefix)`, `WebFetch(pattern)` becomes `Fetch(pattern)`, bare tool names map to Devin's own (`Bash` to `exec`, and so on), and `mcp__<server>__<tool>` passes through.
  - The bare-name table is separate from the `allowed-tools` one. `WebSearch` becomes `web_search`, accepted since Devin CLI v3000.10.21 ([CLI changelog](https://docs.devin.ai/cli/changelog/stable.md)), and imports back the same way. A bare `WebFetch` still drops, since `webfetch` is not a permissions name.
  - Devin's `Exec` only prefix-matches, so an exact `Bash(...)` rule without `:*` always widens. On `allow` and `ask` that would approve unlisted commands, so the rule drops with a coverage note. On `deny` it blocks more than asked, the safe direction, so `Bash(rm -rf /)` becomes `Exec(rm -rf /)`. A command deny beats a broader `ask` or `allow` rule (Devin CLI v3000.10.31). `x-windsurf.permissions` writes Devin's rules directly and replaces the portable ones for that spec.

`outputs.windsurf.workflows-dir` no longer emits anything. It wrote each agent as a Cascade Workflow, and Devin Desktop v3.9.19 removed Cascade ([changelog](https://docs.devin.ai/desktop/changelog.md)). Devin Local does not support Workflows and points users to migrate them to skills ([Devin Local limitations](https://docs.devin.ai/desktop/devin-local)). Setting the key only prints a warning naming that migration; `.devin/agents/<name>.md` emits either way.

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

`agnostic-ai import windsurf` reads rules from `outputs.windsurf.rules-dir` when set; otherwise from `.devin/rules/`, then legacy `.windsurf/rules/`. Each file is reclassified by [filename prefix](@/docs/cli-reference.md#filename-prefix-reclassification). It also scans the project for scoped copies (`<scope>/<rules-dir>/*.md`) to rebuild scoped rules, including scopes like `.github`, `vendor`, or `node_modules`.

`permissions` import from `.devin/config.json`. Devin's `Exec` has no exact-command form, so it imports as the prefix rule `Bash(<cmd>:*)`, not `Bash(<cmd>)`.

Skills import from `.agents/skills/`, `.devin/skills/`, then `.windsurf/skills/`; the first same-name skill wins. Bundled assets and executable modes survive, and native `triggers` move under `x-windsurf` as described under **Skills**.

A hand-authored ignore file imports from `.devinignore`, or `.windsurfignore` when the first is absent, so patterns in either survive sync taking both over.

## Verify

1. Install Devin Desktop from [devin.ai](https://devin.ai) (formerly windsurf.com).
2. Check the tree: `ls .devin/rules/ .devin/agents/ .agents/skills/`, `grep "Generated by agnostic-ai" .devin/rules/*.md`, `test -f .agents/skills/*/SKILL.md`, `python -m json.tool .devin/mcp_config.json > /dev/null` when MCP specs exist, `python -m json.tool .devin/hooks.v1.json > /dev/null` when hook specs exist.
3. Open the project. Devin Local loads every `.devin/rules/*.md` ([rules docs](https://docs.devin.ai/cli/extensibility/rules)); each appears in the Rules panel with no "failed to parse" warnings. Each `.agents/skills/<name>/` loads as a skill.
   A scoped rule at `<scope>/.devin/rules/<name>.md` appears in the same panel, and a rule with `trigger:` frontmatter shows that mode instead of Always On.
   Ask Devin CLI to use a subagent by name (`review this using the <name> subagent`); each `.devin/agents/<name>.md` appears next to the built-in `subagent_explore` and `subagent_general`, with no "profile skipped" warning.
4. `outputs.windsurf.workflows-dir` has nothing to verify; setting it only prints a sync warning.
5. With ignore specs, `cat .devinignore` shows the patterns and indexing skips them. `cat .windsurfignore` shows the same patterns; ask the agent to open one of those paths and it refuses.
6. Confirm each `mcpServers.<name>` from `.devin/mcp_config.json` connects in Devin Local, and a disabled spec shows as disabled.
7. Run `/hooks` in Devin CLI; each entry in `.devin/hooks.v1.json` loads with the file listed as its source.
8. With settings specs, `python -m json.tool .devin/config.json > /dev/null` parses, a command covered by `deny` is refused, and one covered by `allow` runs without a prompt.
