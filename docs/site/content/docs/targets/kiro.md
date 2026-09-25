+++
title = "Kiro"
description = "How agnostic-ai emits Kiro configuration: native paths, capability limits, and output options."
weight = 160

[extra]
group = "Reference"
target_id = "kiro"
+++

# Kiro (`kiro`)

## Output

```
AGENTS.md                          # entry-point pointer body (shared path)
.kiro/steering/<name>.md           # one per rule (inclusion: always, or fileMatch + fileMatchPattern from globs)
.kiro/skills/<name>/SKILL.md       # one per skill (+ bundled scripts/, references/, assets/)
.kiro/agents/<name>.md             # one per agent
.kiro/hooks/<name>.json            # one per hook
.kiro/settings/mcp.json            # when MCP entries exist
.kiroignore                        # when ignore entries exist
```

AWS Kiro loads [steering files](https://kiro.dev/docs/steering/) whose YAML frontmatter must come first in the file. Unscoped rules use `inclusion: always`; globbed rules use `fileMatch` with `fileMatchPattern`. Kiro's `auto` and `manual` modes are not used. Kiro also always includes the root `AGENTS.md`, which carries the shared pointer body.

Skills are [native](https://kiro.dev/docs/skills/): one folder per skill at `.kiro/skills/<name>/SKILL.md`, the tree Kiro's skill picker reads. The render (`name` and `description` frontmatter) is byte-identical with the shared `.agents/skills/` render. Bundled `scripts/`, `references/`, and `assets/` copy alongside.

Agents are [native custom agents](https://kiro.dev/docs/custom-agents/): one YAML-frontmatter Markdown file per agent at `.kiro/agents/<name>.md`, the tree Kiro's agent picker reads. `description` (falls back to the agent name) and `model` pass through. The spec name stays the filename; `x-kiro.name` sets Kiro's optional display name and survives import.

Older versions flattened skills into `.kiro/steering/skill-<name>.md` and agents into `.kiro/steering/agent-<name>.md`, which never reached the pickers. `sync` sweeps those stale files.

Kiro's [agent schema](https://kiro.dev/docs/custom-agents/configuration-reference/) also carries `tools`, `mcpServers`, `permissions`, `hooks`, `keyboardShortcut`, `welcomeMessage`, `excludedTools`, `includeMcpJson`, and `includePowers`. `tools` translates from the portable field. Kiro's `mcpServers` holds inline server definitions, so the portable name list does not map; set `x-kiro.mcpServers`. The rest reach the file through `x-kiro`.

Kiro's `tools` vocabulary uses category tags plus `@server_name`, `@server_name/tool_name`, `@mcp`, `@builtin`, and `*`. The [configuration reference](https://kiro.dev/docs/custom-agents/configuration-reference/) and the [tools page](https://kiro.dev/docs/tools/) disagree on some categories (`knowledge`, `todo_list`, `spec`, `context`), but both agree on the four the portable list translates to:

| Spec `tools` values | Kiro category |
| --- | --- |
| `Read`, `Grep`, `Glob` | `read` |
| `Write`, `Edit` | `write` |
| `Bash` | `shell` |
| `WebFetch`, `WebSearch` | `web` |

Duplicates collapse to one category. Each category is a bundle, so access widens: `write` also covers `delete_file` (declaring only `Edit` grants delete), and `web` covers both fetch and search.

Any other `tools` value is dropped with a coverage note; the values that do translate still emit. Set `x-kiro.tools` to use Kiro's vocabulary directly; it always wins over the translated form. Arbitrary `x-kiro` keys always pass through.

Hooks are [native](https://kiro.dev/docs/hooks/): one JSON file per hook spec, `{"version": "v1", "hooks": [{name, trigger, matcher, action, timeout, enabled, description}]}`. `event` becomes `trigger`, verbatim. `command` (string or list) becomes `action: {"type": "command", "command": ...}`, one entry per command in the same file, with `name` suffixed `-2`, `-3`, ... to stay unique.

`disabled: true` writes `"enabled": false`; enabled needs no key. The spec's `description` reaches the file (Kiro treats it as documentation only).

Arbitrary `x-kiro` keys pass through on each entry. That is the only way to set `confirm` (ask before a Stop command hook runs, with `question`, `options` of `id`/`label`/`run`, and optional `confirmCommand`). `x-kiro.action` accepts `{type: agent, prompt: ...}` or `{type: command, command: ...}`. A valid explicit action needs no generic `command` and replaces the whole command list with one native action. An invalid action fails sync instead of falling back.

`timeout: 0` disables the command timeout; omitting it keeps Kiro's 60-second default.

Stashed hook scripts under `.agnostic-ai/scripts/` do not copy into `.kiro/hooks/`, since Kiro reads hook definitions there, not scripts.

MCP servers write to `.kiro/settings/mcp.json` under `mcpServers`, Kiro's [workspace-level config](https://kiro.dev/docs/mcp/configuration/). A local server carries `command` plus optional `args` and `env`; a remote server carries `url` plus optional `headers` and `env`.

`disabled` passes through under that name (default `false`). Kiro also accepts `autoApprove` (tool names to approve without prompting, `"*"` for all) and `disabledTools` (tool names to hide from the agent).

A remote server can add an `oauth` object, `{clientId, clientSecret, redirectUri, clientMetadataUrl, oauthScopes}`, and a top-level `oauthScopes` fallback. `oauth.oauthScopes` wins when both are set. An empty `oauthScopes: []` emits as written, since Kiro documents it as the fix for scope errors.

Kiro's `oauth` differs from Claude Code's, so each target maps only the sub-keys its vendor documents. See [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).

Ignore specs emit as `.kiroignore` in the project root, in gitignore syntax ([Kiro ignore](https://kiro.dev/docs/kiroignore/)). Multiple specs concatenate. Override via `outputs.kiro.ignore-file`.

Two limits `sync` cannot change: the IDE honors `.kiroignore` only when it is listed in the `kiroAgent.agentIgnoreFiles` setting (Kiro suggests `[".gitignore", ".kiroignore"]`), and CLI V3 applies it only to content and filename search results.

## Config keys

| Key | Default |
| --- | --- |
| `outputs.kiro.rules-dir` | `.kiro/steering` |
| `outputs.kiro.agents-dir` | `.kiro/agents` |
| `outputs.kiro.skills-dir` | `.kiro/skills` |
| `outputs.kiro.hooks-dir` | `.kiro/hooks` |
| `outputs.kiro.mcp-file` | `.kiro/settings/mcp.json` |
| `outputs.kiro.ignore-file` | `.kiroignore` |

## Import

`agnostic-ai import kiro` reverses the Kiro layout. Native agent and skill trees import first. Then `.kiro/steering/` files fill in rules plus any legacy flattened agents and skills (the filename prefix picks the kind):

| Source | Becomes |
|--------|---------|
| `.kiro/agents/<name>.md` (native agent profile) | `<agents>/<name>.md` |
| `.kiro/skills/<name>/SKILL.md` (native skill folder) | `<skills>/<name>/SKILL.md`, bundled sibling assets included |
| `.kiro/steering/<name>.md` (`inclusion: always`) | `<rules>/<name>.md` (unscoped rule) |
| `.kiro/steering/<name>.md` (`inclusion: fileMatch` + `fileMatchPattern`) | `<rules>/<name>.md` with `globs: <fileMatchPattern>` |
| `.kiro/steering/agent-<name>.md` (legacy, pre-native sync) | `<agents>/<name>.md`, body only |
| `.kiro/steering/skill-<name>.md` (legacy, pre-native sync) | `<skills>/<name>/SKILL.md`, body only |
| `.kiro/hooks/<id>.json` (`hooks[]`) | one hook spec per group of entries that differ only in `action.command` |
| `.kiro/settings/mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml` |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

A name present in both a native tree and legacy steering keeps the native copy.

Hook import reads every vendor field, not only the ones `sync` writes:

- `trigger` becomes `event`; `matcher`, `description`, and `timeout` (including `timeout: 0`) map straight across; `enabled: false` becomes `disabled: true`.
- An `action.type: "agent"` action lands under `x-kiro.action`, not the portable prompt handler, which [spec-format.md](@/docs/spec-format.md#hooks) scopes to Claude Code, Cursor, and Copilot.
- `confirm` and any other unknown key land under `x-kiro`, so future fields are kept.
- A file with several `trigger` values splits into one spec per trigger.
- Entries that differ only in `action.command` recombine into one spec with a `command:` list, dropping the `-2`/`-3` suffixes.
- A human-readable name ("Lint on save") slugs into the filename and stays intact on `name:`.
- An action with no payload (`type: "command"` with no `command`, `type: "agent"` with no `prompt`) is skipped with a warning.

Some data does not round-trip. Kiro's output stays the same, but the rebuilt spec loses it:

- A rule's source-layout scope collapses into an equivalent `globs:`.
- A legacy flattened steering agent or skill keeps only its body. That form never carried a description, model, or bundled assets.
- A hook's explicit `enabled: true` leaves no key, since it is the default.
- Two hook files with the same `name` keep both hooks, but the second gets a deterministic generated name, because spec names are unique and set the hook filename.
- `{{filePath}}` in a command stays literal text. Kiro documents it as new in 3.0, and it means nothing on other targets.

## Verify

1. Install Kiro from [kiro.dev](https://kiro.dev).
2. Check the tree: `ls AGENTS.md .kiro/steering/ .kiro/skills/ .kiro/agents/ .kiro/hooks/ .kiro/settings/mcp.json .kiroignore`, `head -2 .kiro/steering/*.md .kiro/skills/*/SKILL.md .kiro/agents/*.md` (frontmatter first, no leading blank lines), `python -m json.tool .kiro/settings/mcp.json > /dev/null`, and `python -m json.tool .kiro/hooks/*.json > /dev/null`.
3. Open the project. The steering panel lists every rule with its inclusion mode and no parse warnings. The skill picker lists every `.kiro/skills/<name>/` folder, and the agent picker lists every `.kiro/agents/<name>.md`.
4. Trigger a hook's event (e.g. save a file for `PostFileSave`). The command runs with no schema warning.
