+++
title = "Kiro"
description = "What agnostic-ai writes for Kiro: file paths, what Kiro supports, and config options."
weight = 160

[extra]
group = "Reference"
target_id = "kiro"
+++

# Kiro (`kiro`)

agnostic-ai writes AWS Kiro steering files, native skills, agents, commands, and hooks under `.kiro/`, plus MCP servers and `.kiroignore`.

## Output

```
AGENTS.md                          # entry-point pointer body, plus the rules block when another tool adds it
.kiro/steering/<name>.md           # one per rule (inclusion: always, or fileMatch + fileMatchPattern from globs)
.kiro/skills/<name>/SKILL.md       # one per skill (+ bundled scripts/, references/, assets/)
.kiro/agents/<name>.md             # one per agent
.kiro/prompts/<name>.md            # one per command (slash commands in CLI V3)
.kiro/hooks/<name>.json            # one per hook
.kiro/settings/mcp.json            # when MCP entries exist
.kiroignore                        # when ignore entries exist
```

- **Rules**: [steering files](https://kiro.dev/docs/steering/) must start with their YAML frontmatter. Rules without globs use `inclusion: always`. Rules with globs use `fileMatch` with `fileMatchPattern`, a string for one pattern and a list for several. Rules with globs need the Kiro IDE or Kiro CLI V3. CLI V1 and V2 load only `always` files. Sync never writes Kiro's `auto` and `manual` modes.
- **`AGENTS.md`**: Kiro includes it in default resources. When Codex or another tool that copies rules into `AGENTS.md` adds the `## Rules` block, always-on rules normally get no steering file. `fileMatch` rules keep theirs. [Custom agents inherit default resources](https://kiro.dev/docs/custom-agents/configuration-reference/) alongside their own list unless `chat.disableInheritingDefaultResources` is on. Setting `x-kiro.resources` on an agent does not change this. If you turn that setting on, add `file://AGENTS.md` to each agent's `x-kiro.resources`. `agnostic-ai lint` warns (LINT029) when an agent with resources leaves it out. See [target behavior](@/docs/target-behavior.md#entry-point-files).
- **Skills**: [native](https://kiro.dev/docs/skills/) at `.kiro/skills/<name>/SKILL.md`, read by the skill picker. The file (`name` and `description` frontmatter) is identical to the shared `.agents/skills/` one. Bundled `scripts/`, `references/`, and `assets/` are copied with it.
- **Agents**: [native custom agents](https://kiro.dev/docs/custom-agents/) in YAML-frontmatter Markdown, read by the agent picker. `description` (default: the agent name) and `model` are copied as written. The spec name stays the file name. `x-kiro.name` sets Kiro's optional display name and survives import.

## Commands

Kiro CLI 2.27.0 and newer shows [workspace prompts](https://kiro.dev/docs/cli/chat/manage-prompts/) as `/name` commands in V3. Sync writes the command body to `.kiro/prompts/<name>.md`, with a generated-file header and no frontmatter.

Kiro expands `$ARGUMENTS`, `${1}` through `${10}`, and `${@}`. Sync keeps these tokens as they are. Use a `::target kiro` fence for Kiro-specific templates. Bare `$1` and `$ARGUMENTS[0]` still get the unsupported-syntax checks.

`outputs.kiro.commands-dir` changes the prompt folder. Otherwise, `outputs.kiro.dir` puts prompts under `<dir>/prompts`. Import reads the same folder into `sources.commands`.

Kiro also has a global prompt folder. `sync --global` does not write commands.

{% <details summary="Unmanaged AGENTS.md"> %}
Listing `AGENTS.md` under `sync.unmanaged` also keeps every steering file. But sync then stops writing `AGENTS.md`, so Codex and every other tool that reads it miss rule changes.
{% </details> %}

{% <details summary="Old flattened steering files"> %}
Older versions flattened skills into `.kiro/steering/skill-<name>.md` and agents into `.kiro/steering/agent-<name>.md`, which never reached the pickers. `sync` removes them.
{% </details> %}

Kiro's [agent schema](https://kiro.dev/docs/custom-agents/configuration-reference/) also carries `tools`, `mcpServers`, `permissions`, `hooks`, `keyboardShortcut` (CLI 2.x only), `welcomeMessage`, `excludedTools`, `includeMcpJson`, and `includePowers`. `tools` is translated from the portable `can` or `tools` field. Kiro's `mcpServers` holds inline definitions, not names, so set `x-kiro.mcpServers`. Set the rest, and any other key, under `x-kiro`.

Kiro's `tools` takes category tags plus `@server_name`, `@server_name/tool_name`, `@mcp`, `@builtin`, and `*`. The [configuration reference](https://kiro.dev/docs/custom-agents/configuration-reference/) and [tools page](https://kiro.dev/docs/tools/) list five category tags (`read`, `write`, `shell`, `web`, `subagent`); built-ins such as `knowledge` take a direct tool ID. Sync uses four of the tags:

| Spec `can` values | Spec `tools` values | Kiro category |
| --- | --- | --- |
| `read` | `Read`, `Grep`, `Glob` | `read` |
| `write`, `edit`, `delete` | `Write`, `Edit` | `write` |
| `shell` | `Bash` | `shell` |
| `web` | `WebFetch`, `WebSearch` | `web` |
| `mcp:<server>` | `mcp__<server>` | `@<server>` |
| `mcp:<server>/<tool>` | `mcp__<server>__<tool>` | `@<server>/<tool>` |

Duplicates are merged. A Kiro category covers more than one tool, so access widens: `write` also covers `delete_file`, and `web` covers fetch and search. Sync prints a note naming the extra access, such as `kiro: .agnostic-ai/agents/writer.md: edit becomes Kiro's write category, which also allows delete_file`. `on-unsupported: error` fails when access widens, including for a Claude Code alias such as `Edit`. Scoped values, such as `shell(git diff *)` and `read(src/**)`, are dropped with a coverage note. The rest are still written. `x-kiro.tools` takes Kiro's own names and always wins.

**Hooks** are [native](https://kiro.dev/docs/hooks/): one JSON file per spec, `{"version": "v1", "hooks": [{name, trigger, matcher, action, timeout, enabled, description}]}`.

- Portable `on: before-tool` maps to `PreToolUse`, with anchored lists of documented built-in IDs for `shell`, `read`, `web`, and `edit`, and `@<server>/*` for `mcp:<server>`. Built-in kinds exclude MCP tools and need an updated mapping for new IDs. The tool wildcard keeps server names such as `mcp` distinct from source selectors. Other portable events and `decision: stdout` have no Kiro mapping. See [portable hooks](@/docs/spec-format/hooks.md#portable-events).
- `event` becomes `trigger`, as written. `validate` flags `AgentSpawn` and `agentSpawn`, Kiro V3's compatibility aliases, and suggests `SessionStart`.
- Each `command` (string or list) becomes an `action: {"type": "command", "command": ...}` entry in the same file. The `name` gets a `-2`, `-3`, ... suffix to stay unique.
- `disabled: true` writes `"enabled": false`. `description` is written to the file as documentation only.
- `timeout: 0` disables the timeout; omitting it keeps Kiro's 60-second default.
- `x-kiro` keys are copied to each entry. That is the only way to set `confirm` (ask before a Stop command hook runs, with `question`, `options` of `id`/`label`/`run`, and optional `confirmCommand`).
- `x-kiro.action` takes `{type: agent, prompt: ...}` or `{type: command, command: ...}`. A valid one needs no portable `command` and replaces the whole list with one action. An invalid one fails sync.
- `.agnostic-ai/scripts/<name>` references are copied to `.kiro/scripts/<name>`, outside `.kiro/hooks/` where Kiro reads hook files, and the command is rewritten. See [shared hook scripts](@/docs/spec-format/hooks.md#shared-hook-scripts).
- [`agnostic-ai hook run`](@/docs/spec-format/hooks.md#hook-run) runs a `UserPromptSubmit` or `Stop` hook, or a tool hook with `--payload`, on an assumed `sh -c`, before a session does.

**MCP** servers go to `.kiro/settings/mcp.json` under `mcpServers`, Kiro's [workspace-level config](https://kiro.dev/docs/mcp/configuration/). Local servers have `command` plus optional `args` and `env`. Remote ones have `url` plus optional `headers` and `env`. Kiro expands a `${NAME}` reference only after you approve the variable under **Mcp Approved Env Vars** in its settings. See [environment references](@/docs/spec-format/mcps.md#environment-references). `disabled` is copied as written (default `false`). Kiro also accepts `autoApprove` (tools approved without prompting, `"*"` for all) and `disabledTools` (tools hidden from the agent). A remote server can add `oauth` (`{clientId, clientSecret, redirectUri, clientMetadataUrl, oauthScopes}`) and a top-level `oauthScopes` fallback; `oauth.oauthScopes` wins. An empty `oauthScopes: []` is written as is, which is Kiro's documented fix for scope errors. Kiro's `oauth` differs from Claude Code's, so each tool maps only its own sub-keys. See [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target).

**Ignore** specs are joined into `.kiroignore` in gitignore syntax ([Kiro ignore](https://kiro.dev/docs/kiroignore/)); override with `outputs.kiro.ignore-file`. Sync cannot change two limits. The IDE honors the file only when it is listed in `kiroAgent.agentIgnoreFiles` (Kiro suggests `[".gitignore", ".kiroignore"]`). CLI V3 reads only the workspace file, with no global one, and there it blocks direct reads of matches and hides them from content and filename search.

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

`agnostic-ai import kiro` reads the native agent and skill folders first. Then it reads rules and old flattened agents and skills from `.kiro/steering/` (the filename prefix picks the kind):

| Source | Becomes |
|--------|---------|
| `.kiro/prompts/<name>.md` (or the configured command directory) | `<commands>/<name>.md` |
| `.kiro/agents/<name>.md` (native agent profile) | `<agents>/<name>.md` |
| `.kiro/skills/<name>/SKILL.md` (native skill folder) | `<skills>/<name>/SKILL.md`, bundled sibling assets included |
| `.kiro/steering/<name>.md` (`inclusion: always`) | `<rules>/<name>.md` (unscoped rule) |
| `.kiro/steering/<name>.md` (`inclusion: fileMatch` + `fileMatchPattern`) | `<rules>/<name>.md` with `globs: <fileMatchPattern>` |
| `.kiro/steering/<name>.md` (`inclusion: manual` or `auto`) | skipped with a note. The file stays hand-written, since a rule has no on-demand mode |
| `.kiro/steering/agent-<name>.md` (legacy, pre-native sync) | `<agents>/<name>.md`, body only |
| `.kiro/steering/skill-<name>.md` (legacy, pre-native sync) | `<skills>/<name>/SKILL.md`, body only |
| `.kiro/hooks/<id>.json` (`hooks[]`) | one hook spec per group of entries that differ only in `action.command` |
| `.kiro/settings/mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml` |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

Native files win over old steering files with the same name.

Hook import reads every Kiro field:

- `trigger` becomes `event`; `matcher`, `description`, and `timeout` (including `timeout: 0`) carry over; `enabled: false` becomes `disabled: true`.
- `action.type: "agent"` goes under `x-kiro.action`, not the portable prompt handler, which [spec-format.md](@/docs/spec-format/hooks.md) limits to Claude Code, Cursor, and Copilot.
- `confirm` and unknown keys go under `x-kiro`, so new Kiro fields are kept.
- Several `trigger` values split into one spec each.
- Entries that differ only in `action.command` become one `command:` list, without the `-2`/`-3` suffixes.
- A readable name ("Lint on save") becomes the file name in slug form and stays unchanged in `name:`.
- An action with no content (`type: "command"` with no `command`, `type: "agent"` with no `prompt`) is skipped with a warning.

The imported spec loses some data, but the Kiro files sync writes stay the same:

- A rule's folder scope becomes an equivalent `globs:`.
- An old flattened agent or skill keeps only its body. That form never had a description, model, or bundled assets.
- An explicit `enabled: true` leaves no key, since it is the default.
- Two hook files with the same `name` keep both hooks. The second gets a generated name, because spec names are unique file names.
- <code>{&#123;filePath}}</code> stays as written. Kiro documents it as new in 3.0, and it means nothing elsewhere.

## Protected paths

Not enforced. Kiro takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install Kiro from [kiro.dev](https://kiro.dev).
2. Check the tree: `ls AGENTS.md .kiro/steering/ .kiro/skills/ .kiro/agents/ .kiro/hooks/ .kiro/settings/mcp.json .kiroignore`, `head -2 .kiro/steering/*.md .kiro/skills/*/SKILL.md .kiro/agents/*.md` (frontmatter first, no leading blank lines), `python -m json.tool .kiro/settings/mcp.json > /dev/null`, and `python -m json.tool .kiro/hooks/*.json > /dev/null`.
3. Open the project. The steering panel lists every rule with its inclusion mode and no parse warnings; the skill and agent pickers list every `.kiro/skills/<name>/` and `.kiro/agents/<name>.md`.
4. Trigger a hook's event (e.g. save a file for `PostFileSave`). The command runs with no schema warning.
5. In Kiro CLI 2.27.0 or newer, start `kiro-cli chat --v3`. Type `/` and select a synced command. Check that its prompt and native arguments expand.
