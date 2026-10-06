+++
title = "Kiro"
description = "How agnostic-ai emits Kiro configuration: native paths, capability limits, and output options."
weight = 160

[extra]
group = "Reference"
target_id = "kiro"
+++

# Kiro (`kiro`)

agnostic-ai writes AWS Kiro steering files, native skills, agents, commands, and hooks under `.kiro/`, plus MCP servers and `.kiroignore`.

## Output

```
AGENTS.md                          # entry-point pointer body, plus the rules block when an inlining target shares it
.kiro/steering/<name>.md           # one per rule (inclusion: always, or fileMatch + fileMatchPattern from globs)
.kiro/skills/<name>/SKILL.md       # one per skill (+ bundled scripts/, references/, assets/)
.kiro/agents/<name>.md             # one per agent
.kiro/prompts/<name>.md            # one per command, for CLI V3 slash commands
.kiro/hooks/<name>.json            # one per hook
.kiro/settings/mcp.json            # when MCP entries exist
.kiroignore                        # when ignore entries exist
```

- **Rules**: [steering files](https://kiro.dev/docs/steering/) must start with their YAML frontmatter. Unscoped rules use `inclusion: always`; globbed rules use `fileMatch` with `fileMatchPattern`, a string for one pattern and a list for several. Globbed rules need the Kiro IDE or Kiro CLI V3; CLI V1 and V2 load only `always` files. Kiro's `auto` and `manual` modes go unused.
- **`AGENTS.md`**: Kiro includes it in default resources. When codex or another inlining target adds the `## Rules` block, matching always-on rules normally get no steering file; `fileMatch` rules keep theirs. [Custom agents inherit default resources](https://kiro.dev/docs/custom-agents/configuration-reference/) alongside their own list unless `chat.disableInheritingDefaultResources` is enabled. An agent that sets `x-kiro.resources` does not change this. If you turn that setting on, add `file://AGENTS.md` to each agent's `x-kiro.resources`; `agnostic-ai lint` warns (LINT029) when an agent with resources leaves it out. See [target behavior](@/docs/target-behavior.md#entry-point-files).
- **Skills**: [native](https://kiro.dev/docs/skills/) at `.kiro/skills/<name>/SKILL.md`, read by the skill picker. The render (`name` and `description` frontmatter) is byte-identical with the shared `.agents/skills/` one. Bundled `scripts/`, `references/`, and `assets/` copy alongside.
- **Agents**: [native custom agents](https://kiro.dev/docs/custom-agents/) in YAML-frontmatter Markdown, read by the agent picker. `description` (default: agent name) and `model` pass through. The spec name stays the filename; `x-kiro.name` sets Kiro's optional display name and survives import.

## Commands

Kiro CLI 2.27.0 and newer exposes [workspace prompts](https://kiro.dev/docs/cli/chat/manage-prompts/) as `/name` commands in V3. Sync writes the command body to `.kiro/prompts/<name>.md`, with provenance and no metadata frontmatter.

Kiro expands `$ARGUMENTS`, `${1}` through `${10}`, and `${@}`. Sync preserves these tokens. Use a `::target kiro` fence for Kiro-specific templates; bare `$1` and `$ARGUMENTS[0]` keep the unsupported-syntax checks.

`outputs.kiro.commands-dir` overrides the prompt directory. Otherwise, `outputs.kiro.dir` places prompts under `<dir>/prompts`. Import reads the same configured directory into `sources.commands`.

Kiro also has a native global prompt directory. agnostic-ai's global sync does not emit commands.

{% <details summary="Unmanaged AGENTS.md"> %}
Listing `AGENTS.md` under `sync.unmanaged` also keeps every steering file, but sync then stops writing `AGENTS.md`, so codex and every other reader lose rule changes there.
{% </details> %}

{% <details summary="Old flattened steering files"> %}
Older versions flattened skills into `.kiro/steering/skill-<name>.md` and agents into `.kiro/steering/agent-<name>.md`, which never reached the pickers. `sync` sweeps them.
{% </details> %}

Kiro's [agent schema](https://kiro.dev/docs/custom-agents/configuration-reference/) also carries `tools`, `mcpServers`, `permissions`, `hooks`, `keyboardShortcut`, `welcomeMessage`, `excludedTools`, `includeMcpJson`, and `includePowers`. `tools` translates from the portable `can` or `tools` field. Kiro's `mcpServers` holds inline definitions, not names: set `x-kiro.mcpServers`. The rest, and any arbitrary key, pass through `x-kiro`.

Kiro's `tools` takes category tags plus `@server_name`, `@server_name/tool_name`, `@mcp`, `@builtin`, and `*`. The [configuration reference](https://kiro.dev/docs/custom-agents/configuration-reference/) and [tools page](https://kiro.dev/docs/tools/) disagree on some categories (`knowledge`, `todo_list`, `spec`, `context`) but agree on the four the portable list uses:

| Spec `can` values | Spec `tools` values | Kiro category |
| --- | --- | --- |
| `read` | `Read`, `Grep`, `Glob` | `read` |
| `write`, `edit`, `delete` | `Write`, `Edit` | `write` |
| `shell` | `Bash` | `shell` |
| `web` | `WebFetch`, `WebSearch` | `web` |
| `mcp:<server>` | `mcp__<server>` | `@<server>` |
| `mcp:<server>/<tool>` | `mcp__<server>__<tool>` | `@<server>/<tool>` |

Duplicates collapse. Categories are bundles, so access widens: `write` also covers `delete_file`, and `web` covers fetch and search. Sync prints a note naming the extra access, such as `kiro: .agnostic-ai/agents/writer.md: edit becomes Kiro's write category, which also allows delete_file`. `on-unsupported: error` fails on widening, including a Claude Code alias such as `Edit`. Scoped values, such as `shell(git diff *)` and `read(src/**)`, drop with a coverage note; the rest still emit. `x-kiro.tools` takes Kiro's vocabulary and always wins.

**Hooks** are [native](https://kiro.dev/docs/hooks/): one JSON file per spec, `{"version": "v1", "hooks": [{name, trigger, matcher, action, timeout, enabled, description}]}`.

- `event` becomes `trigger`, verbatim. `validate` flags `AgentSpawn` and `agentSpawn`, Kiro V3's compatibility aliases, and suggests `SessionStart`.
- Each `command` (string or list) becomes an `action: {"type": "command", "command": ...}` entry in the same file, with `name` suffixed `-2`, `-3`, ... to stay unique.
- `disabled: true` writes `"enabled": false`. `description` reaches the file as documentation only.
- `timeout: 0` disables the timeout; omitting it keeps Kiro's 60-second default.
- `x-kiro` keys pass through per entry. That is the only way to set `confirm` (ask before a Stop command hook runs, with `question`, `options` of `id`/`label`/`run`, and optional `confirmCommand`).
- `x-kiro.action` takes `{type: agent, prompt: ...}` or `{type: command, command: ...}`. A valid one needs no generic `command` and replaces the whole list with one action; an invalid one fails sync.
- Neutral `.agnostic-ai/scripts/<name>` references copy to `.kiro/scripts/<name>`, outside `.kiro/hooks/` where Kiro reads definitions, and the command is rewritten. See [shared hook scripts](@/docs/spec-format/hooks.md#shared-hook-scripts).
- [`agnostic-ai hook run`](@/docs/spec-format/hooks.md#hook-run) runs a `UserPromptSubmit` or `Stop` hook, or a tool hook with `--payload`, on an assumed `sh -c` before a session does.

**MCP** servers go to `.kiro/settings/mcp.json` under `mcpServers`, Kiro's [workspace-level config](https://kiro.dev/docs/mcp/configuration/). Local servers carry `command` plus optional `args` and `env`; remote ones carry `url` plus optional `headers` and `env`. Kiro expands a `${NAME}` reference only after you approve the variable under **Mcp Approved Env Vars** in its settings; see [environment references](@/docs/spec-format/mcps.md#environment-references). `disabled` passes through (default `false`). Kiro also accepts `autoApprove` (tools approved without prompting, `"*"` for all) and `disabledTools` (tools hidden from the agent). A remote server can add `oauth` (`{clientId, clientSecret, redirectUri, clientMetadataUrl, oauthScopes}`) and a top-level `oauthScopes` fallback; `oauth.oauthScopes` wins. An empty `oauthScopes: []` emits as written, Kiro's documented fix for scope errors. Kiro's `oauth` differs from Claude Code's, so each target maps only its vendor's sub-keys. See [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target).

**Ignore** specs concatenate into `.kiroignore` in gitignore syntax ([Kiro ignore](https://kiro.dev/docs/kiroignore/)); override with `outputs.kiro.ignore-file`. Sync cannot change two limits. The IDE honors it only when listed in `kiroAgent.agentIgnoreFiles` (Kiro suggests `[".gitignore", ".kiroignore"]`), CLI V3 reads only the workspace file, with no global one, and there it blocks direct reads of matches and filters them from content and filename search.

## Config keys

| Key | Default |
| --- | --- |
| `outputs.kiro.rules-dir` | `.kiro/steering` |
| `outputs.kiro.agents-dir` | `.kiro/agents` |
| `outputs.kiro.skills-dir` | `.kiro/skills` |
| `outputs.kiro.commands-dir` | `.kiro/prompts` |
| `outputs.kiro.hooks-dir` | `.kiro/hooks` |
| `outputs.kiro.mcp-file` | `.kiro/settings/mcp.json` |
| `outputs.kiro.ignore-file` | `.kiroignore` |

## Import

`agnostic-ai import kiro` reads native agent and skill trees first, then fills in rules and legacy flattened agents and skills from `.kiro/steering/` (the filename prefix picks the kind):

| Source | Becomes |
|--------|---------|
| `.kiro/prompts/<name>.md` (or the configured command directory) | `<commands>/<name>.md` |
| `.kiro/agents/<name>.md` (native agent profile) | `<agents>/<name>.md` |
| `.kiro/skills/<name>/SKILL.md` (native skill folder) | `<skills>/<name>/SKILL.md`, bundled sibling assets included |
| `.kiro/steering/<name>.md` (`inclusion: always`) | `<rules>/<name>.md` (unscoped rule) |
| `.kiro/steering/<name>.md` (`inclusion: fileMatch` + `fileMatchPattern`) | `<rules>/<name>.md` with `globs: <fileMatchPattern>` |
| `.kiro/steering/<name>.md` (`inclusion: manual` or `auto`) | skipped with a note; the file stays hand-written, since a rule has no on-demand mode |
| `.kiro/steering/agent-<name>.md` (legacy, pre-native sync) | `<agents>/<name>.md`, body only |
| `.kiro/steering/skill-<name>.md` (legacy, pre-native sync) | `<skills>/<name>/SKILL.md`, body only |
| `.kiro/hooks/<id>.json` (`hooks[]`) | one hook spec per group of entries that differ only in `action.command` |
| `.kiro/settings/mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml` |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

Native copies win over legacy steering of the same name.

Hook import reads every vendor field:

- `trigger` becomes `event`; `matcher`, `description`, and `timeout` (including `timeout: 0`) map across; `enabled: false` becomes `disabled: true`.
- `action.type: "agent"` lands under `x-kiro.action`, not the portable prompt handler, which [spec-format.md](@/docs/spec-format/hooks.md) scopes to Claude Code, Cursor, and Copilot.
- `confirm` and unknown keys land under `x-kiro`, so future fields survive.
- Several `trigger` values split into one spec each.
- Entries differing only in `action.command` recombine into one `command:` list, without the `-2`/`-3` suffixes.
- A human-readable name ("Lint on save") slugs into the filename and stays intact on `name:`.
- An action with no payload (`type: "command"` with no `command`, `type: "agent"` with no `prompt`) skips with a warning.

The rebuilt spec loses some data, but Kiro's output stays the same:

- A rule's source-layout scope collapses into an equivalent `globs:`.
- A legacy flattened agent or skill keeps only its body; that form never carried a description, model, or bundled assets.
- An explicit `enabled: true` leaves no key, since it is the default.
- Two hook files sharing a `name` keep both hooks; the second gets a deterministic generated name, since spec names are unique filenames.
- <code>{&#123;filePath}}</code> stays literal: Kiro documents it as new in 3.0, and it means nothing elsewhere.

## Protected paths

Advisory. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install Kiro from [kiro.dev](https://kiro.dev).
2. Check the tree: `ls AGENTS.md .kiro/steering/ .kiro/skills/ .kiro/agents/ .kiro/hooks/ .kiro/settings/mcp.json .kiroignore`, `head -2 .kiro/steering/*.md .kiro/skills/*/SKILL.md .kiro/agents/*.md` (frontmatter first, no leading blank lines), `python -m json.tool .kiro/settings/mcp.json > /dev/null`, and `python -m json.tool .kiro/hooks/*.json > /dev/null`.
3. Open the project. The steering panel lists every rule with its inclusion mode and no parse warnings; the skill and agent pickers list every `.kiro/skills/<name>/` and `.kiro/agents/<name>.md`.
4. Trigger a hook's event (e.g. save a file for `PostFileSave`). The command runs with no schema warning.
5. In Kiro CLI 2.27.0 or newer, start `kiro-cli chat --v3`. Type `/` and select a synced command. Check that its prompt and native arguments expand.
