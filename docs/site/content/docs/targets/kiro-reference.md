+++
title = "Kiro native reference"
description = "Native field mappings, config keys, and import coverage for kiro."
weight = 161

[extra]
nav_title = "Kiro reference"
group = "Reference"
+++

# Kiro native reference

Start with the [Kiro setup](@/docs/targets/kiro.md).

## Native mappings


- **Rules**: [steering files](https://kiro.dev/docs/steering/) must start with their YAML frontmatter. Rules without globs use `inclusion: always`. Rules with globs use `fileMatch` with `fileMatchPattern`, a string for one pattern and a list for several. Rules with globs need the Kiro IDE or Kiro CLI V3. CLI V1 and V2 load only `always` files. Sync never writes Kiro's `auto` and `manual` modes.
- **`AGENTS.md`**: Kiro includes it in default resources. When Codex or another tool that copies rules into `AGENTS.md` adds the `## Rules` block, always-on rules normally get no steering file. `fileMatch` rules keep theirs. [Custom agents inherit default resources](https://kiro.dev/docs/custom-agents/configuration-reference/) alongside their own list unless `chat.disableInheritingDefaultResources` is on. Setting `x-kiro.resources` on an agent does not change this. If you turn that setting on, add `file://AGENTS.md` to each agent's `x-kiro.resources`. `agnostic-ai lint` warns (LINT029) when an agent with resources leaves it out. See [target behavior](@/docs/target-behavior.md#entry-point-files).
- **Memory**: with `builtins: [memory]`, `.kiro/steering/kiro-memory-index.md` includes `.agnostic-ai/memory/MEMORY.md` and `.agnostic-ai/local/memory/MEMORY.md` with `#[[file:...]]`, so Kiro loads both [shared memory](@/docs/memory.md) lists every session. No other tool gets the file, and `sync --global` leaves it out. Kiro resolves each path from the workspace root and shows a marker for a list that does not exist yet. It applies its file read permissions and ignore rules to a reference, so a list they block does not load. With `memory.personal: repo`, the file includes only the project list. The shared policy tells Kiro to find personal memory with `agnostic-ai memory path`.
- **Skills**: [native](https://kiro.dev/docs/skills/) at `.kiro/skills/<name>/SKILL.md`, read by the skill picker. The file (`name` and `description` frontmatter) is identical to the shared `.agents/skills/` one. Bundled `scripts/`, `references/`, and `assets/` are copied with it.
- **Agents**: [native custom agents](https://kiro.dev/docs/custom-agents/) in YAML-frontmatter Markdown, read by the agent picker. `description` (default: the agent name) and `model` are copied as written. The spec name stays the file name. `x-kiro.name` sets Kiro's optional display name and survives import.
- **Settings**: `model` writes `chat.defaultModel` in `.kiro/settings/cli.json`, keeping every other key. In terminal UI sessions, Kiro CLI uses this workspace value over the global one in `~/.kiro/settings/cli.json`.

  Kiro ids use dots (`claude-sonnet-4.6`), so set `model: {default: ..., kiro: claude-sonnet-4.6}` when the shared value is a Claude Code id. A Claude alias such as `sonnet` raises a note and writes nothing. `permissions` raise a note, because the file has no allow, deny, or ask list.

  While a spec sets `model`, sync replaces any hand-written `chat.defaultModel`. Removing the model later deletes the key without restoring its earlier value. Set `model: {claude: ...}` to leave Kiro out.

  Settings `x-kiro` raises a note and writes nothing because `cli.json` takes flat dotted keys. Sync rewrites the file with two-space indentation and drops comments.
- **Agent MCP servers**: each server an agent names in `mcpServers` is written in full under the agent's `mcpServers`, the only form Kiro takes. An [agent-only server](@/docs/spec-format/mcps.md#agent-only) stays out of `.kiro/settings/mcp.json`. Set `x-kiro.timeout` (connection handshake) or `x-kiro.requestTimeout` (each call) in milliseconds on the MCP spec to write them in agent files; Kiro's defaults are 60000 and 120000. The portable `timeout` is not written there, with a note, since it does not say which limit it means. `x-kiro.mcpServers` replaces the list.

## Commands

Kiro CLI 2.27.0 and newer shows [workspace prompts](https://kiro.dev/docs/cli/chat/manage-prompts/) as `/name` commands in V3. Sync writes the command body to `.kiro/prompts/<name>.md`, with a generated-file header and no frontmatter.

Kiro expands `$ARGUMENTS`, `${1}` through `${10}`, and `${@}`. Sync keeps these tokens as they are. Use a `::target kiro` fence for Kiro-specific templates. Bare `$1` and `$ARGUMENTS[0]` still get the unsupported-syntax checks.

`outputs.kiro.commands-dir` changes the prompt folder. Otherwise, `outputs.kiro.dir` puts prompts under `<dir>/prompts`. Import reads the same folder into `sources.commands`.

Kiro also has a global prompt folder. `sync --global` does not write commands.

<details>
<summary>Unmanaged AGENTS.md</summary>

Listing `AGENTS.md` under `sync.unmanaged` keeps every steering file. Check [the effect on other targets sharing that file](@/docs/target-behavior.md#inlined-rules).

</details>

<details>
<summary>Old flattened steering files</summary>

Older versions flattened skills into `.kiro/steering/skill-<name>.md` and agents into `.kiro/steering/agent-<name>.md`, which never reached the pickers. `sync` removes them.

</details>

Kiro's [agent schema](https://kiro.dev/docs/custom-agents/configuration-reference/) also carries `tools`, `mcpServers`, `permissions`, `hooks`, `keyboardShortcut` (CLI 2.x only), `welcomeMessage`, `excludedTools`, `includeMcpJson`, and `includePowers`. `tools` is translated from the portable `can` or `tools` field. Kiro's `mcpServers` holds inline definitions, not names, so set `x-kiro.mcpServers`. Set the rest, and any other key, under `x-kiro`, including `resources`.

Kiro groups agent tools into `read`, `write`, `shell`, `web`, and `subagent`. The [configuration reference](https://kiro.dev/docs/custom-agents/configuration-reference/) and [tools page](https://kiro.dev/docs/tools/) list the same categories. It also accepts `@server_name`, `@server_name/tool_name`, `@mcp`, `@builtin`, and `*`.

Built-in tools such as `knowledge` use their exact tool IDs. Sync translates four of the categories, plus named MCP servers and tools:

| Spec `can` values | Spec `tools` values | Kiro category |
| --- | --- | --- |
| `read` | `Read`, `Grep`, `Glob` | `read` |
| `write`, `edit`, `delete` | `Write`, `Edit` | `write` |
| `shell` | `Bash` | `shell` |
| `web` | `WebFetch`, `WebSearch` | `web` |
| `mcp:<server>` | `mcp__<server>` | `@<server>` |
| `mcp:<server>/<tool>` | `mcp__<server>__<tool>` | `@<server>/<tool>` |

Duplicates are merged. A Kiro category covers more than one tool, so access widens: `write` also covers `delete_file`, and `web` covers fetch and search. Sync prints a note naming the extra access, such as `kiro: .agnostic-ai/agents/writer.md: edit becomes Kiro's write category, which also allows delete_file`. `on-unsupported: error` fails when access widens, including for a Claude Code alias such as `Edit`. Scoped values, such as `shell(git diff *)` and `read(src/**)`, are dropped with a sync note. The rest are still written. `x-kiro.tools` takes Kiro's own names and always wins.

**Hooks** are [native](https://kiro.dev/docs/hooks/): one JSON file per spec, `{"version": "v1", "hooks": [{name, trigger, matcher, action, timeout, enabled, description}]}`.

- Portable `on: before-tool` maps to `PreToolUse`, with anchored lists of documented built-in IDs for `shell`, `read`, `web`, and `edit`, and `@<server>/*` for `mcp:<server>`. Built-in kinds exclude MCP tools and need an updated mapping for new IDs. The tool wildcard keeps server names such as `mcp` distinct from source selectors. Other portable events and `decision: stdout` have no Kiro mapping. See [portable hooks](@/docs/spec-format/hook-reference.md#portable-events).
- `event` becomes `trigger`, as written. `validate` flags `AgentSpawn` and `agentSpawn`, Kiro V3's compatibility aliases, and suggests `SessionStart`. `PreCompact` and `PostCompact` run in Kiro CLI 3.1.0 or later, around conversation compaction, and take no matcher ([hook types](https://kiro.dev/docs/hooks/types/#pre-compaction)). Kiro IDE 1.4.121 also has compaction hooks, but does not document their trigger names in the hook file.
- Each `command` (string or list) becomes an `action: {"type": "command", "command": ...}` entry in the same file. The `name` gets a `-2`, `-3`, ... suffix to stay unique.
- `disabled: true` writes `"enabled": false`. `description` is written to the file as documentation only.
- `timeout: 0` disables the timeout; omitting it keeps Kiro's 60-second default.
- `x-kiro` keys are copied to each entry. That is the only way to set `confirm` (ask before a Stop command hook runs, with `question`, `options` of `id`/`label`/`run`, and optional `confirmCommand`).
- `x-kiro.action` takes `{type: agent, prompt: ...}` or `{type: command, command: ...}`. A valid one needs no portable `command` and replaces the whole list with one action. An invalid one fails sync.
- `.agnostic-ai/scripts/<name>` references are copied to `.kiro/scripts/<name>`, outside `.kiro/hooks/` where Kiro reads hook files, and the command is rewritten. See [shared hook scripts](@/docs/spec-format/hook-recipes.md#shared-hook-scripts).
- [`agnostic-ai hook run`](@/docs/cli-reference/hook-diagnostics.md#hook-run) runs a `UserPromptSubmit` or `Stop` hook, or a tool hook with `--payload`, on an assumed `sh -c`, before a session does.

Use your home hook specs in every workspace with [Kiro CLI V3](https://kiro.dev/docs/hooks/#file-naming-and-location):

```bash
agnostic-ai sync --global --only kiro --dry-run
agnostic-ai sync --global --only kiro
agnostic-ai sync --global --only kiro --check
```

Sync writes `~/.kiro/hooks/<name>.json` and copies shared scripts to `~/.kiro/scripts/`. Set `KIRO_HOME` to a valid absolute profile directory to change where agnostic-ai writes these folders and Kiro's other global files. Like [Kiro CLI](https://kiro.dev/docs/reference/settings/#environment-variables), sync uses `~/.kiro` when the value is relative, a filesystem root, your home directory, or one of its ancestors. Global hooks and workspace hooks both run.

Sync removes only recorded files after their specs are removed. A conflicting or edited file stops sync.

`agnostic-ai import --global kiro` imports hook files that one spec can reproduce at the same filename. Files with mixed triggers, names that differ from their filename, or other content the spec cannot preserve are skipped with a warning. Existing specs stay. After importing, run the sync and check commands above.

**MCP** servers go to `.kiro/settings/mcp.json` under `mcpServers`, Kiro's [workspace-level config](https://kiro.dev/docs/mcp/configuration/). Local servers have `command` plus optional `args` and `env`. Remote ones have `url` plus optional `headers` and `env`. Kiro expands a `${NAME}` reference only after you approve the variable under **Mcp Approved Env Vars** in its settings. See [environment references](@/docs/spec-format/mcps.md#environment-references). `disabled` is copied as written (default `false`). Kiro also accepts `autoApprove` (tools approved without prompting, `"*"` for all) and `disabledTools` (tools hidden from the agent).

A remote server can add `oauth` (`{clientId, clientSecret, redirectUri, clientMetadataUrl, oauthScopes}`) and top-level `oauthScopes`. Set only one scopes list. Kiro documents different [scope precedence by client and version](https://kiro.dev/docs/mcp/configuration/#scopes):

| Client | When both lists are set |
| --- | --- |
| CLI V2 sessions | Nested `oauth.oauthScopes` takes priority |
| CLI V3 sessions before 3.4.0 | The nested list is ignored |
| CLI V3 sessions in 3.4.0 or later, IDE 1.4.215 or later | Top-level `oauthScopes` takes priority |

An empty `oauthScopes: []` is written as is, which Kiro documents for scope errors. Kiro's `oauth` differs from Claude Code's, so each tool maps only its own sub-keys. See [`disabled` support by target](@/docs/spec-format/mcps-reference.md#disabled-support-by-target).

**Ignore** specs are joined into the project `.kiroignore` in gitignore syntax ([Kiro ignore](https://kiro.dev/docs/kiroignore/)); override with `outputs.kiro.ignore-file`. The IDE honors the file only when it is listed in `kiroAgent.agentIgnoreFiles` (Kiro suggests `[".gitignore", ".kiroignore"]`). CLI V3 honors workspace `.kiroignore`, `~/.kiro/settings/kiroignore`, and Git's global ignore file (`core.excludesfile`, only in Git repositories). Matching files are blocked from direct reads and hidden from content and filename search.


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
| `.kiro/agents/<name>.md` (native agent profile) | `<agents>/<name>.md`. An inline `mcpServers` map becomes a list of MCP spec names when each server matches its enabled entry in `.kiro/settings/mcp.json` and sets no `timeout` or `requestTimeout`; otherwise the whole map moves to `x-kiro.mcpServers`, with literal credentials turned into `${NAME}` references as in MCP file import. A server whose `oauth` holds a credential is left out with a note. Native `excludedTools`, `includeMcpJson`, `includePowers`, `resources`, `permissions`, `welcomeMessage` and `hooks` move under `x-kiro`, and `sync` writes them back; deleting one from the native file drops it on the next import |
| `.kiro/agents/<name>.json` (JSON agent profile) | `<agents>/<name>.md`. Keys map as in a Markdown profile, since Kiro says both formats hold identical fields. `prompt` becomes the body; a `file://` prompt is read from the profile's folder and must sit inside the project. A profile that is not valid JSON, has a `prompt` that is not text or null, or points at a missing file is skipped with a note. Sync writes `.kiro/agents/<name>.md`, so import prints a note to remove the JSON file: Kiro would read both under the name `<name>` and they would clash |
| `.kiro/agents/<folder>/<name>.md` or `.json` (agent in a subfolder) | `<agents>/<folder>-<name>.md`: path separators become hyphens and the extension is dropped. Sync writes `.kiro/agents/<folder>-<name>.md`, so import prints a note to remove the original: keeping both gives Kiro two agents, `<folder>/<name>` and `<folder>-<name>` |
| Two agent files that give one spec name, such as `a.md` and `a.json`, or `team-planner.md` and `team/planner.md` | the first wins in this order: top-level `.md`, top-level `.json`, subfolder `.md`, then subfolder `.json`. Import prints a note for each file it skips, and no file overwrites another. After a sync, a later import says the original can be removed instead |
| `.kiro/skills/<name>/SKILL.md` (native skill folder) | `<skills>/<name>/SKILL.md`, bundled files next to the skill included |
| `.kiro/steering/<name>.md` (`inclusion: always`) | `<rules>/<name>.md` (project-wide rule) |
| `.kiro/steering/<name>.md` (`inclusion: fileMatch` + `fileMatchPattern`) | `<rules>/<name>.md` with `globs: <fileMatchPattern>` |
| `.kiro/steering/<name>.md` (`inclusion: manual` or `auto`) | skipped with a note. The file stays hand-written, since a rule has no on-demand mode |
| `.kiro/steering/agent-<name>.md` (legacy, pre-native sync) | `<agents>/<name>.md`, body only |
| `.kiro/steering/skill-<name>.md` (legacy, pre-native sync) | `<skills>/<name>/SKILL.md`, body only |
| `.kiro/hooks/<id>.json` (`hooks[]`) | one hook spec per group of entries that differ only in `action.command` |
| `.kiro/settings/mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml`. A server whose `oauth` holds a credential is left out with a note |
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
- An old flattened agent or skill keeps only its body. That form never had a description, model, or bundled files.
- An explicit `enabled: true` leaves no key, since it is the default.
- Two hook files with the same `name` keep both hooks. The second gets a generated name, because spec names are unique file names.
- <code>{&#123;filePath}}</code> stays as written. Kiro documents it as new in 3.0, and it means nothing elsewhere.

## Global voice language

`agnostic-ai sync --global --only kiro` writes home `voice.language` to the literal `"voice.language"` key in `~/.kiro/settings/cli.json`, or `settings/cli.json` under `KIRO_HOME`. Use `auto` to detect the transcription language. Kiro CLI has no on/off or capture mode key, so sync prints a note for `voice.enabled` and `voice.mode`. Other voice settings, including `voice.autoSubmit`, stay as written. Removing the portable language restores its previous value; a value you changed by hand stays yours. See [voice dictation](@/docs/spec-format/settings.md#voice-dictation).
