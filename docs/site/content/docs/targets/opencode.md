+++
title = "OpenCode"
description = "What agnostic-ai writes for OpenCode: file paths, what OpenCode cannot do, and output options."
weight = 130

[extra]
group = "Reference"
target_id = "opencode"

[extra.moved]
set-up-this-tool = "@/docs/targets/opencode.md#first-rule"
+++

# OpenCode (`opencode`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

OpenCode reads the root `AGENTS.md`, `.opencode/`, and `opencode.json`.

## First rule

Install the client using the [official setup guide](https://opencode.ai/docs/), then open your project in it.

Select `opencode` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. With the default paths, it reaches `AGENTS.md`.

Generated hooks use the V1 plugin API by default. Select [V2 hooks](#opencode-v2-hooks) explicitly for OpenCode V2.

For existing files, check [what `import opencode` reads](#import) before syncing.

## Verify

Run `agnostic-ai sync --check` and inspect `AGENTS.md` for the commit-prefix rule. Launch `opencode` from the project and ask for a commit subject for a small fix; it should use `fix:`.

Next, [add a skill](@/docs/spec-format/skills.md).

## Output

```
AGENTS.md                                 # entry-point pointer body + rules (written by sync)
.opencode/agents/<name>.md                # one subagent per agent
.opencode/skills/<name>/SKILL.md          # one folder per skill, bundled assets included
.opencode/commands/<name>.md              # one per command spec
.opencode/commands/skill-<name>.md        # extra command form, only when emit-skills-as-commands: true
.opencode/plugins/<name>.ts               # one plugin module per hook spec
opencode.json                             # when MCP entries exist (merged with user config)
```

## Native mappings

<details>
<summary>Field mappings, discovery, and compatibility</summary>

### Rules

[OpenCode](https://opencode.ai/docs/rules/) finds the repo-root `AGENTS.md` by walking up from the current directory. Codex, Amp, Warp, and the other `AGENTS.md` tools share the same file, so `sync` writes it once.

### Agents

([docs](https://opencode.ai/docs/agents/)): `.opencode/agents/<name>.md` (the singular directory is legacy), with `description`, `mode`, `model`, `temperature`, `permission`, and any `x-opencode` key. The portable `maxTurns` is written as `steps`, and `x-opencode.steps` wins. The body is the system prompt.

- In legacy agent files, a `can` or `tools` list becomes the agent [`permission`](https://opencode.ai/docs/agents/#permissions) map: `can: [read]` writes `permission: {"*": deny, doom_loop: ask, external_directory: ask, read: {"*": allow, "*.env": ask, "*.env.*": ask, "*.env.example": allow}}`. Every tool not listed is denied. The catch-all comes first because the last matching rule wins.
- Restricted agents keep project permission decisions for their allowed tools, including `.env` prompts, and deny other tools. Sync reads root `opencode.json`, including native V2 `permissions` in order. It does not read `opencode.jsonc`, `.opencode/opencode.json`, or global user policy, so agent rules can override restrictions kept only there.
- Files with `permissions`, `request`, `system`, or `disabled` use V2 rules, including `disabled: false`. Sync writes generated restrictions as an ordered `permissions` array and appends explicit native array rules in their source order. These explicit rules can override generated restrictions. OpenCode 2 ignores a top-level `temperature` in such a file, so sync writes it as `request: {body: {temperature: ...}}`, merged with any `request` you set; a `temperature` already in `request.body` wins. OpenCode 2.2.0 keeps request values but does not yet send them with model requests ([agents](https://opencode.ai/v2/docs/agents#request)); this also applies to `temperature` in a V1 file. Import preserves the V2 fields and rebuilds the agent without its original native file.
- Native `permission` keys retain source order because the last matching rule wins. `x-opencode.permission` overrides the keys it names; an explicit native action wins outright. Malformed project rules or resource rules that cannot fit the agent format fail sync with the affected paths.
- V2 home-relative resources and Windows case differences can make restrictions stricter. Use absolute resources for scoped filesystem grants. Sync rejects grants whose intersection depends on the runtime home. See [native permissions](https://opencode.ai/v2/docs/permissions) for matching semantics.
- These names translate: `Read`, `Edit` and `Write` (both `edit`), `Bash`, `Glob`, `Grep`, `Task`, `Skill`, `WebFetch`, `WebSearch`, and `mcp__<server>__<tool>` as `<server>_<tool>`, and a whole server (`mcp:<server>` or `mcp__<server>`) as `<server>_*`. Other names drop with a sync note. If nothing translates, no map is written, because `{"*": deny}` alone would lock the agent out. `x-opencode: {permission: {...}}` wins for each key it names; a single action such as `permission: ask` wins outright.

### Skills

([docs](https://opencode.ai/docs/skills/)): `.opencode/skills/<name>/SKILL.md` plus bundled files. OpenCode also scans `.claude/skills/` and `.agents/skills/`. A directory scope from the source folder moves the folder under that directory, and import keeps it.

- `disable-model-invocation: true` keeps a skill out of the model's available list in OpenCode V2 (verified with 2.0.26). Explicit loading by ID still works. V1 does not enforce this field. Native `metadata.opencode/autoinvoke` takes precedence when both forms are set, and import preserves both.
- `outputs.opencode.emit-skills-as-commands: true` also writes the command form. Its relative links to bundled files point into the skill folder (`../skills/<name>/references/x.md`). Other links stay as written.
- Names must be 1-64 lowercase letters or digits, with single hyphens between segments (the vendor's `name` rule, which Zed also enforces). `Deploy`, `my_skill`, and `my--skill` fail sync with the required format. Sync does not rename them, because OpenCode would skip the folder.

### Commands

`.opencode/commands/<name>.md` keeps `description`, `agent`, `model`, and `subtask`.

### V1 hooks

One [OpenCode plugin](https://opencode.ai/docs/plugins/) module per hook spec. The generated TypeScript module runs the hook command. OpenCode loads every JS or TS file in `.opencode/plugins/` at startup.

- Modules follow the vendor's example: `import type { Plugin } from "@opencode-ai/plugin"`, then `export const <Name>Plugin: Plugin = async ({ $ }) => { return { ... } }`. The type-only import is removed at runtime, so `@opencode-ai/plugin` is optional. A `//` comment at the top marks the file as generated.
- `PreToolUse` and `PostToolUse` map to `tool.execute.before` and `tool.execute.after`. OpenCode's dotted names are kept. Other documented events (`session.idle`, `file.edited`, `permission.asked`, ...) use the single `event` hook with an `event.type` guard.
- `matcher` becomes an anchored `new RegExp("^(?:...)$")` tested against `input.tool`, so `write` does not match `todowrite`. OpenCode tool names are lowercase, so a Claude-style `Edit` or `Edit|Write` is written as is with a sync note.
- Claude's `*` gets no guard. Neither does a regex that JavaScript reads differently from RE2 (`(?i)`, `\A`, `(?P<name>`), and that case adds a note.
- Bun's `$` runs each command as written. On `tool.execute.before`, exit code 2 blocks the call and stderr is the reason. Other failures log a warning, and the next command runs. This includes commands Bun's shell cannot parse (no `>&2`).
- `disabled: true` writes no module, because OpenCode runs every module in the directory.
- `matcher` on an event hook raises a sync note, because event data has no tool name. Any `timeout` raises one too, because OpenCode sets no time limit for handlers.
- `shell.env` and `experimental.session.compacting` are declined with a note. They rewrite output that a command spec cannot express.
- `import opencode` does not read `.opencode/plugins/` back.

### MCP

`opencode.json` in the project root gets a `$schema` link and the `mcp` map.

- OpenCode V2 stores servers under `mcp.servers`. `import opencode` keeps the complete native MCP configuration in `settings/opencode-mcp.yaml`, under `x-opencode.mcp`. Sync restores it even without the original JSON file. This import and sync behavior is verified against V2 2.0.26.
- Native V2 settings and portable MCP specs cannot both own the MCP configuration. Sync reports the conflicting spec paths. Use one representation for OpenCode.
- Import replaces inline environment and header values with native environment references. It refuses credentials it cannot rewrite, including inline OAuth secrets. Use `{env:NAME}` references before importing those fields.
- Sync manages each native server separately. Servers and defaults added by hand stay; a removed spec server stays when you have edited it.
- Stdio becomes `{type: "local", command: [...], cwd}`, and HTTP or SSE becomes `{type: "remote", url, headers}`. `cwd` (relative to the workspace) keeps its name.
- `timeout` keeps its name and uses milliseconds for MCP requests, including tool calls. OpenCode defaults to `30000` ms when connecting and fetching tools.
- `disabled: true` writes `"enabled": false` ([MCP docs](https://opencode.ai/docs/mcp-servers/)), and import reads `enabled: false` back. An enabled server gets no key.
- Other documented fields, such as the `oauth` object for pre-registered remote servers, go through `x-opencode`.
- Sync changes only `$schema` and `mcp`. `theme`, `model`, and other keys stay. They never show as drift in `sync --check` or `doctor`, and `doctor --fix` keeps them.

### Settings

A settings spec's default `model` merges into `opencode.json`, and import restores it to `settings/opencode.yaml`. An `x-opencode` block merges in too. Its `permission` merges tool by tool with the translated rules.

### Permissions

[OpenCode's V2 migration guide](https://opencode.ai/v2/docs/migrate-v1#permissions-and-tools) distinguishes the defaults: V1 asks before accessing paths outside the project; V2 allows `external_directory` by default.

To request approval in V2, add the rule below to your existing `x-opencode.permissions` array, keeping all its rules and their order. If your native `permissions` array exists only in `opencode.json`, move the full array into one settings spec under `x-opencode.permissions` before adding the rule. Only when neither location has a V2 policy, save this settings spec as `.agnostic-ai/settings/outside-project.yaml`:

```yaml
targets: [opencode]
x-opencode:
  permissions:
    - action: external_directory
      resource: "*"
      effect: ask
```

Keep the native `permissions` array in one settings spec: arrays from separate specs replace each other rather than combine. Put this broad `ask` rule before intentional exceptions. [V2 permissions](https://opencode.ai/v2/docs/permissions#directories) use the last matching rule.

Portable `allow`, `deny`, and `ask` lists become [OpenCode's `permission` map](https://opencode.ai/docs/permissions/).

- A bare tool name covers the whole tool (`Read` becomes `read: allow`). A scoped rule becomes a glob (`Bash(go test:*)` becomes `bash: {"go test *": "allow"}`).
- `Write` and `Edit` both land on `edit`, which covers edit, write, and patch.
- If two lists claim one tool and pattern, the stricter wins.
- OpenCode takes the last match, so inside one tool sync writes allow, then ask, then deny, each group with the `*` catch-all first. A narrower allow never bypasses a broader deny, as on Claude Code.
- `mcp__<server>__<tool>` becomes `<server>_<tool>` (`mcp__github__create_issue` becomes `github_create_issue: deny`), the key OpenCode uses for MCP tools ([agents docs](https://opencode.ai/docs/agents/)). The server name keeps its hyphens.
- Scoped `WebFetch` or `WebSearch` rules (those keys take a bare action) and denying a whole MCP server (`github_*` has no portable spelling) raise a sync note.
- `x-opencode.permission` writes OpenCode's own map and replaces the translated rules for that tool.
- Sync owns only the tool keys your specs produce. A key you wrote in `permission` that no spec sets, such as `"edit": "ask"`, stays, and a bare `"permission": "ask"` stays as the `"*"` entry. A tool key leaves with its spec while you have not edited it. When a spec starts setting a key you wrote, sync saves the file as `opencode.json.bak` first.
- A project synced before this change has one record for the whole `permission` value. If you edit another key in the same sync where a spec drops a tool, that tool stays as yours, since sync kept only a checksum of what it wrote.
- `import opencode` reads the map back, except namespaced MCP keys, where the server name cannot be told from the tool name.

<details>
<summary>Old .opencode/AGENTS.md leftovers</summary>

Sync removes a generated file at the old `.opencode/AGENTS.md` path, which OpenCode never read. It leaves a hand-written one.

</details>

## OpenCode V2 hooks

Set `x-opencode.plugin-api: v2` on each hook for OpenCode V2. This route is verified against 2.0.26. Omitting it, or choosing `v1`, keeps the V1 plugin format. Other values fail sync with the source path.

```yaml
name: guard-shell
targets: [opencode]
event: PreToolUse
matcher: shell
command: .agnostic-ai/scripts/guard-shell.sh
x-opencode:
  plugin-api: v2
```

On macOS and Linux, V2 supports `PreToolUse`, `PostToolUse`, `tool.execute.before`, and `tool.execute.after`. Other events fail sync. Use these native event names; OpenCode has no portable `on` mapping. Explicit matchers stay anchored JavaScript regular expressions: `bash` is not rewritten to `shell`. An invalid expression prevents the plugin from loading.

The plugin registers a native [V2 tool callback](https://opencode.ai/v2/docs/build/plugins) and runs commands in the triggering session's checkout. It sends the native event as one JSON object plus a newline on stdin. Before-tool input includes `tool`, `sessionID`, `agent`, `messageID`, `id`, and `input`. After-tool input adds `status` and either `result` or `error`. `PostToolUse` runs only for completed tools; the native `tool.execute.after` event also runs for failed tools.

Commands run in order. A before-tool command exiting 2 blocks the operation and stops later commands; stderr gives the reason. Other command failures log the hook name, command, and error, then continue. Stdout is ignored. Structured stdout decisions are not supported by this route.

With `args`, the executable and each argument are passed separately. Without `args`, the command runs through `/bin/sh`. Exec-form scripts must be executable. Shared scripts are copied to `.opencode/hooks/` as with V1.

On Windows, use OpenCode V1 with the default V1 hooks. OpenCode V2 cannot run V1 plugins. V2 plugins load with a clear warning and refuse matching callbacks before reading session data or starting a command. A before-tool callback blocks the operation. An after-tool callback reports the refusal after the operation has already run.

`timeout` is a positive whole number of seconds, up to 86400; the default is 60 seconds. A timeout stops the owned process tree and logs a failure. Unloading the plugin also stops active commands and removes its callbacks. `x-opencode.env` adds string values to each command's inherited environment. Each invocation receives `AGNOSTIC_AI_TARGET=opencode`.

Sync writes `.opencode/plugins/<name>.ts`. The type-only `@opencode/plugin` import adds no runtime dependency. Arbitrary plugin code is not imported back into specs.

</details>

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.opencode.agents-dir` | `.opencode/agents` | |
| `outputs.opencode.skills-dir` | `.opencode/skills` | |
| `outputs.opencode.commands-dir` | `.opencode/commands` | |
| `outputs.opencode.hooks-dir` | `.opencode/plugins` | OpenCode only loads plugins from its own directory, so moving this stops the hooks loading |
| `outputs.opencode.mcp-file` | `opencode.json` | |
| `outputs.opencode.emit-skills-as-commands` | `false` | |
| `outputs.opencode.rules-file` | unset | writes legacy joined rules and skips the write of instructions that point to source specs |

With `builtins: [memory]`, sync adds both [shared memory](@/docs/memory.md) indexes to `instructions` and keeps your own entries. Dropping the built-in removes only those two.

With `memory.personal: repo`, sync also adds `"<folder>/**": "allow"` to `permission.external_directory`, so OpenCode saves to the [personal memory folder](@/docs/memory.md#one-store-per-repository) without asking. Your own entries stay in the order you wrote them, and the folder entry goes last when sync first adds it, since OpenCode applies the last rule that matches. A bare action you set there, such as `"deny"`, becomes the `"*"` entry. A bare `"permission": "deny"` becomes `{"*": "deny"}`, which means the same and stays after repo mode ends. Once sync has added the folder entry it stays where it is, so a broader rule you put after it keeps winning, and an entry you edit stays yours. A folder path with `*` or `?`, which OpenCode reads as wildcards, gets no `external_directory` entry and a note; move `AGNOSTIC_AI_HOME` or set `memory.personal: checkout`.

With `handoff` or `handoff-hook` enabled in a linked worktree, sync also allows the main checkout's ignored [handoff store](@/docs/handoff.md#write-from-a-linked-worktree) under `permission.external_directory`. Existing memory and user permission rules stay, and a rule you or a spec wrote for that path, such as `deny` or `ask`, is never replaced. A main checkout path with `*` or `?` gets no grant. For a tracked, hand-written, or unmanaged file sync skips only this grant and reports the permission to add by hand (once you add it, it stays); the rest of its output is unchanged.

## Import

`agnostic-ai import opencode` reads:

- **Instructions and rules**: root `AGENTS.md`, falling back to legacy `.opencode/AGENTS.md` when the root file is absent. The shared body becomes `.agnostic-ai/AGNOSTIC_AI.md`. Rule sections that sync wrote become rule specs. Hand-written instructions stay whole.
- **Agents and commands**: top-level Markdown files in `.opencode/agents/` and `.opencode/commands/`.
- **Skills**: `.opencode/skills/`, `.claude/skills/`, then `.agents/skills/` (first wins within a scope), with scoped paths and bundled files.
- **MCP servers**: the `mcp` map in `opencode.json`.
- **Settings and permissions**: the default `model` becomes `settings/opencode.yaml`; portable entries in the `permission` map become `settings/permissions-opencode.yaml`. Namespaced MCP permission keys have no portable spelling and stay in the file.

Plugin hooks in `.opencode/plugins/` are not imported.

## Protected paths

Not enforced. Sync cannot write an edit-blocking rule or hook for this tool, so it prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).
