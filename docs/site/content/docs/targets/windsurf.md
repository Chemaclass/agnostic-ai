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
AGENTS.md                            # shared pointer body (dedup with the other AGENTS.md consumers)
.devin/rules/<name>.md
<scope>/.devin/rules/<name>.md       # one per scoped rule
.devin/agents/<name>.md              # one per agent (custom subagent profile)
.agents/skills/<name>/SKILL.md       # one folder per skill (shared tree with codex/amp/zed/crush/openhands)
.devinignore                         # when ignore entries exist (indexing)
.windsurfignore                      # when ignore entries exist (agent file access)
.devin/mcp_config.json               # when MCP entries exist
.devin/hooks.v1.json                 # when hook entries exist
```

Windsurf became Devin Desktop (2026-06). Devin Desktop prefers `.devin/rules/*.md` and keeps `.windsurf/rules/` as a backward-compat fallback (`.windsurfrules` is legacy), so rules now emit at the preferred path. The target keeps its `windsurf` name: existing `outputs.windsurf.*` keys and `x-windsurf` meta continue to work.

Set `outputs.windsurf.rules-dir: .windsurf/rules` to stay on the old layout; otherwise sync sweeps managed leftovers at the pre-rename path (hand-authored files survive). Devin also reads the cross-tool root `AGENTS.md`, so `sync` distributes the shared pointer body there (see the entry-point table above, #645).

- **Agents**: one custom subagent profile per agent at `.devin/agents/<name>.md`. Devin's docs call this "Custom subagents are defined as markdown files under `agents/`", project layout `.devin/agents/`, "**Flat file**: `agents/<name>.md`" ([docs.devin.ai/cli/subagents](https://docs.devin.ai/cli/subagents)). Frontmatter carries `name`, `description`, `model`, `allowed-tools`, and `max-nesting`; the body after the closing delimiter is the subagent's system prompt.
  - Agents used to flatten into `.devin/rules/agent-<name>.md`, which reached the rules loader instead of the subagent loader and had no key for any of those five fields (target-audit 2026-08-27, #638). A managed copy at the old name is swept for every current agent. A scoped agent lands flat here, since Devin documents sub-directory discovery for rules only.
  - `allowed-tools` translates agnostic-ai's Claude-style names onto the five Devin publishes as its complete set: `read`, `edit`, `grep`, `glob`, `exec` ([docs.devin.ai/cli/reference/permissions](https://docs.devin.ai/cli/reference/permissions)). `Read`/`Grep`/`Glob`/`Bash` map one-to-one onto `read`/`grep`/`glob`/`exec`. `Write` and `Edit` both collapse onto `edit`, so an agent declaring only `Write` also gains edit capability. An `mcp__<server>__<tool>` name passes through untranslated. Anything else drops with a coverage note rather than shipping a name the vendor never documented.
  - **Devin also reads `.agents/agents/`**, listed under "Also supported" for project subagents, in both the flat `<name>.md` and the nested `<name>/agent.md` form. Antigravity, Goose, and OpenHands write there, so with any of them in `targets` one agent spec gives Devin two profiles claiming one name, and only this one carries `allowed-tools`. See the [cross-cutting agents note](@/docs/targets/_index.md) for the cost and the workaround (#863).
  - Set `x-windsurf.allowed-tools` to write Devin's vocabulary directly, and `x-windsurf.max-nesting` for the nesting override, which has no generic spec field. `model` passes through verbatim, since the vendor's own example pins `model: sonnet`. The vendor caveat holds: "Custom subagents are **experimental**. The format, behavior, and configuration options may change in future releases."
- **Scoped rules**: a scoped rule lands at `<scope>/.devin/rules/<name>.md`, not nested inside the root rules dir. Devin reads "`.devin/rules` or `.windsurf/rules` in any sub-directory of your workspace" ([docs.devin.ai/desktop/cascade/memories](https://docs.devin.ai/desktop/cascade/memories)) and globs each one single-level as `.devin/rules/*.md` ([docs.devin.ai/cli/extensibility/rules](https://docs.devin.ai/cli/extensibility/rules)). The old `.devin/rules/<scope>/<name>.md` reached no documented discovery path (target-audit 2026-08-27, #628), so sync sweeps the old nested tree through the ledger.
  - With `outputs.windsurf.rules-dir` set, the prefix follows it, so the legacy layout scopes to `<scope>/.windsurf/rules/<name>.md`. Devin CLI loads a sub-directory rules dir lazily, when the agent touches files there; Devin Desktop discovers every one of them at session start. The scope narrows what the CLI sees, not what Desktop sees.
- **Rule activation**: a rule that sets `alwaysApply: false` carries a `trigger` frontmatter key, the activation mode Devin reads.

  | Rule has | Emitted trigger |
  | --- | --- |
  | `globs` | `trigger: glob` (plus the pattern verbatim) |
  | `description` alone | `trigger: model_decision` |
  | neither | `trigger: manual` |

  An always-on rule stays bare: Devin loads a file with no frontmatter as always-on, and its Always On mode puts the full body in the system prompt on every message, so a `description` has no job there. Devin's fifth documented value, `agent`, has no counterpart in the spec format and is never emitted. Before this, no rule file carried frontmatter, so `alwaysApply: false` was silently promoted to always-on (#628).
- **Skills**: one folder per skill under `.agents/skills/<name>/SKILL.md`. Devin documents `.agents/skills/`, `.devin/skills/`, and `.windsurf/skills/` as project paths. This adapter writes the shared first path so identical skills dedupe with Codex, Amp, Zed, Crush, OpenHands, Antigravity, Augment, and Kilo.
  - Native `triggers` values move under `x-windsurf` in the source spec, then return to top-level frontmatter on sync. This preserves `[user]`, `[model]`, and combined invocation policy without leaking a Devin-only field to other targets. Sibling assets and modes survive every path.
- **Ignore**: ignore specs emit as both `.devinignore` and `.windsurfignore`, gitignore syntax under a `#` provenance header. The two names are not one path and its legacy alias: the vendor gives them different jobs. "you can add a `.devinignore` file to your repo root, with the same syntax as .gitignore" governs Devin Desktop Indexing, while "The agent additionally respects `.windsurfignore` files when accessing files" ([docs.devin.ai/desktop/context-awareness/windsurf-ignore](https://docs.devin.ai/desktop/context-awareness/windsurf-ignore)).
  - Only `.codeiumignore` is legacy on that page, and it is an alias for the indexing file: "The legacy `.codeiumignore` filename is also supported, and both can be used together." Set `outputs.windsurf.ignore-file: .codeiumignore` to write that name instead.
  - Writing just the indexing file put a user's `secrets/**` out of the index while the agent stayed free to read and edit it (target-audit 2026-09-18, #863). `outputs.windsurf.ignore-file` moves the indexing file only; `.windsurfignore` follows every time, unless the override names `.windsurfignore` itself, in which case the one file covers both surfaces.
- **MCP**: merges into `.devin/mcp_config.json` under a root `mcpServers` map. This is the file [Devin Local](https://docs.devin.ai/desktop/devin-local) reads for project scope, not Cascade: Devin Desktop v3.9.19 removed Cascade, leaving Devin Local as the only agent ([docs.devin.ai/desktop/changelog.md](https://docs.devin.ai/desktop/changelog.md), target-audit 2026-09-09, #707). [Cascade's own MCP page](https://docs.devin.ai/desktop/cascade/mcp) confirms this: "The MCP configuration on this page applies to the legacy Cascade agent only. The Devin Local agent ... configures MCP servers in the Devin CLI config files instead."
  - [The schema](https://docs.devin.ai/cli/extensibility/mcp/configuration) carries `command`/`args`/`env` for local (stdio) servers, and `url`/`transport` (`http` or `sse`)/`headers`/`oauthClientId`/`oauthClientSecret`/`oauthResource` for remote ones. Both accept `disabled`, which `devin mcp enable|disable` also toggles on this file (see [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target)). The field is spelled `transport`, not the `type` key the shared `mcpServers`-with-`type` builder writes for claude, cursor, and the rest, so this adapter holds its own schema.
  - The vendor documents that the file moved here in v3000.3 (Local 3.6); `mcpServers` entries in the older `.devin/config.json` migrate to this dedicated file automatically on startup, so this path is correct for both. Cascade's own MCP file, `~/.codeium/windsurf/mcp_config.json`, is user-tier and out of reach: agnostic-ai only emits project-tier files, so this is a new surface rather than a restored one.
  - Devin CLI also documents a third scope this adapter does not write: `.devin/mcp_config.local.json`, saved there "by default" and "gitignored... use these for personal API keys" ([docs.devin.ai/cli/extensibility/mcp/configuration](https://docs.devin.ai/cli/extensibility/mcp/configuration)), next to the project-committed `.devin/mcp_config.json` above ("shared with your team via version control"). The vendor's local/committed split exists so a team can share servers while keeping personal secrets out of version control. agnostic-ai's own generated `.devin/mcp_config.json` is itself added to the managed `.gitignore` block by default (`gitignore.enabled: true`), so it is never committed either; the split's purpose does not apply to agnostic-ai's own output (target-audit 2026-08-11, #609).
  - A `type: ws` spec emits no server and raises a coverage note because Devin documents only `http` and `sse` remote transports.
- **Hooks**: merge into `.devin/hooks.v1.json`: "Create `.devin/hooks.v1.json` in your project" ([docs.devin.ai/cli/extensibility/hooks/overview](https://docs.devin.ai/cli/extensibility/hooks/overview), #629). Eight events: `PreToolUse`, `PostToolUse`, `PermissionRequest`, `UserPromptSubmit`, `Stop`, `PostCompaction`, `SessionStart`, `SessionEnd`.
  - Unlike Claude Code, Codex, Gemini, and Qoder's shared `{"hooks": {...}}` wrapper, "the hooks object is the entire file (no wrapper key needed)", so this adapter's document renders `{"<Event>": [...]}` at the top level with no wrapper.
  - Per entry: `type` (`"command"` runs a shell command, or `"prompt"` evaluates an LLM prompt instead; agnostic-ai's generic hook spec has no `prompt` field, so that shape only reaches the file through a hand-authored `type`/`prompt` Meta pair), `command`, and optional `timeout` (seconds). `matcher` is a regex on the event's `tool_name`, available on `PreToolUse`, `PostToolUse`, and `PermissionRequest`.
  - **Devin CLI names its own tools in lowercase snake_case** (`exec`, `edit`, `read`, `write`, `apply_patch`, `grep`, `glob`, `webfetch`, ...), not Claude's PascalCase. A matcher carried over from a Claude spec parses as a valid regex and then matches nothing. That case surfaces a coverage note rather than a guessed rename, the same treatment OpenHands and Antigravity give their own mismatched vocabularies.

`outputs.windsurf.workflows-dir` no longer emits anything. It used to write each agent as a Workflow, invokable in Cascade as `/<name>`, but Devin Desktop v3.9.19 ("September 8, 2026") removed Cascade, the only agent that ever read one: "Cascade has been removed. Devin Local is now the only agent available in Devin Desktop" ([docs.devin.ai/desktop/changelog.md](https://docs.devin.ai/desktop/changelog.md)).

Devin Local does not pick the surface back up: "Workflows are not available with the Devin Local agent. Migrate your workflows to skills with the Devin: Open Cascade Migration Wizard command" ([docs.devin.ai/desktop/devin-local](https://docs.devin.ai/desktop/devin-local), Limitations; target-audit 2026-09-09, #707). Setting the key now only prints a warning naming that migration path; the native `.devin/agents/<name>.md` emission happens either way, unaffected.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.windsurf.rules-dir` | `.devin/rules` | |
| `outputs.windsurf.agents-dir` | `.devin/agents` | |
| `outputs.windsurf.skills-dir` | `.agents/skills` | |
| `outputs.windsurf.workflows-dir` | empty | set, it only warns (see above) |
| `outputs.windsurf.ignore-file` | `.devinignore` | moves the indexing ignore file only; `.windsurfignore` follows the same spec for agent file access |
| `outputs.windsurf.mcp-file` | `.devin/mcp_config.json` | |
| `outputs.windsurf.hooks-file` | `.devin/hooks.v1.json` | |

## Import

`agnostic-ai import windsurf` reads rules from `.devin/rules/`, falling back to legacy `.windsurf/rules/`, and reclassifies each file by [filename prefix](@/docs/cli-reference.md#filename-prefix-reclassification).

Skills import from `.agents/skills/`, `.devin/skills/`, then `.windsurf/skills/`. The first same-name skill wins. Bundled assets and executable modes survive, and native `triggers` move under `x-windsurf` as described under **Skills**.

A hand-authored ignore file imports from `.devinignore`, falling back to `.windsurfignore` when the indexing file is absent, so patterns written into either one survive sync taking both files over.

## Verify

1. Install Devin Desktop from [devin.ai](https://devin.ai) (formerly windsurf.com).
2. Check the tree: `ls .devin/rules/ .devin/agents/ .agents/skills/`, `grep "Generated by agnostic-ai" .devin/rules/*.md` for the provenance header, `test -f .agents/skills/*/SKILL.md`, `python -m json.tool .devin/mcp_config.json > /dev/null` when MCP specs exist, `python -m json.tool .devin/hooks.v1.json > /dev/null` when hook specs exist.
3. Open the project. Devin Local loads every `.devin/rules/*.md`; each appears in the Rules panel with no "failed to parse" warnings ([docs.devin.ai/cli/extensibility/rules](https://docs.devin.ai/cli/extensibility/rules) still documents this discovery path; only the agent that reads it changed with Cascade's removal, #707). Each `.agents/skills/<name>/` loads as a skill.
   With a scoped rule, `<scope>/.devin/rules/<name>.md` appears in the same panel, and a rule with `trigger:` frontmatter shows that activation mode rather than Always On.
   Ask Devin CLI to use a subagent by name (`review this using the <name> subagent`); each `.devin/agents/<name>.md` profile appears alongside the built-in `subagent_explore` and `subagent_general`, with no "profile skipped" warning.
4. `outputs.windsurf.workflows-dir` has nothing left to verify: no shipping Devin Desktop agent reads a Workflow file, so setting the key only prints a sync-time warning now (#707).
5. When ignore specs exist, `cat .devinignore` shows the concatenated patterns and indexing skips them. `cat .windsurfignore` shows the same patterns; ask the agent to open one of those paths and it should refuse.
6. Devin Local is the only agent in Devin Desktop (Cascade was removed in v3.9.19, #707); confirm each `mcpServers.<name>` from `.devin/mcp_config.json` connects, with a disabled spec showing as disabled.
7. Run `/hooks` in Devin CLI to confirm each entry in `.devin/hooks.v1.json` loads, with the file listed as its source.
