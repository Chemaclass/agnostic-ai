+++
title = "Factory"
description = "What agnostic-ai writes for Factory: file paths, what Factory cannot do, and output options."
weight = 210

[extra]
group = "Reference"
target_id = "factory"

[extra.moved]
set-up-this-tool = "@/docs/targets/factory.md#first-rule"
+++

# Factory (`factory`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

Factory [Droid](https://docs.factory.com/harness/subagents) reads the root `AGENTS.md` and custom droids in `.factory/droids/`. Factory has no per-rule directory, so rule bodies go into the `## Rules` block of `AGENTS.md`.

## First rule

Select `factory` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. With the default paths, it reaches `AGENTS.md`.

Custom droids come from agent specs. A read-only droid also needs its MCP access restricted; sync supplies an empty server list unless you set one.

For existing files, check [what `import factory` reads](#import) before syncing.

## Verify

Run `agnostic-ai sync --check` and inspect `AGENTS.md` for the commit-prefix rule. Launch `droid` from the project and ask for a commit subject for a small fix; it should use `fix:`.

Next, [add a skill](@/docs/spec-format/skills.md).

## Output

```
AGENTS.md                          # pointer body + rules (shared path)
.factory/droids/<name>.md          # one custom-droid profile per agent
.agents/skills/<name>/SKILL.md     # one folder per skill (shared tree)
.factory/commands/<name>.md        # one Markdown slash command per command spec
.factory/hooks.json                # when hook entries exist
.factory/mcp.json                  # when MCP entries exist
.factory/settings.json             # when a settings entry carries a model, a shell-command rule, or an x-factory key
```

## Native mappings

<details>
<summary>Field mappings, discovery, and compatibility</summary>

### Agents

Each `<name>.md` carries `name`, `description`, and optional `model` and `tools`. Any `x-factory` key passes through.

- An agent with an empty body is skipped with a sync note, because Droid CLI requires a system prompt.
- A portable `mcpServers` list is written as is. `mcpServers: []` excludes every MCP server, even global ones, so list the ones you want. See [`mcpServers`](@/docs/spec-format/agents.md#mcpservers-support-by-target).
- A portable `effort` becomes `reasoningEffort`, which accepts only `low`, `medium`, and `high`. `xhigh`, `max`, and integer budgets drop with a sync note. Factory ignores the field under `model: inherit`. See [per-target `model` and `effort`](@/docs/spec-format/agents.md#per-target-model-and-effort).
- `readonly: true` writes `tools: read-only`, Factory's read-only tool category. It replaces a portable `tools` list instead of narrowing it, with a sync note that points to `x-factory.tools` for a custom list. `x-factory.tools` wins over `readonly`. `readonly: false` does nothing.
- A droid gets its MCP servers' tools, so a read-only droid with no `mcpServers` list gets `mcpServers: []`. A list you set is kept.

### Tools

The generic `tools` list maps to Droid CLI's IDs (`Read`, `LS`, `Grep`, `Glob`, `Create`, `Edit`, `ApplyPatch`, `Execute`, `WebSearch`, `FetchUrl`). One unknown ID makes Factory reject the droid.

- `Bash` becomes `Execute`, `Write` becomes `Create`, and `WebFetch` becomes `FetchUrl`, as in Factory's Claude Code importer. The rest already match.
- `TodoWrite` and `Skill` drop silently, because every droid gets them. `ExitSpecMode` and `GenerateDroid` drop because custom droids cannot enable them. `tools: all` is never written, because an omitted key allows every tool.
- Any other name drops with a sync note. The rest are still written.
- `x-factory.tools` writes Factory's own names and wins over the translation. Only it can set a category (`read-only`, `edit`, `execute`, `web`, `mcp`) or an MCP tool ID. See [`tools` support by target](@/docs/spec-format/agents.md#tools-support-by-target).

### Skills

Written once to the tree that codex, amp, zed, and crush share. Factory also reads [`.agent/skills/**/SKILL.md`](https://docs.factory.com/harness/skills), which this adapter does not write.

- Scoped skills go to `<scope>/.factory/skills/<name>/SKILL.md` with bundled files. Unscoped skills keep `.agents/skills/`.
- `outputs.factory.skills-dir` replaces the directory at the root and in each scope.

### Commands

`.factory/commands/<name>.md` takes `description` and `argument-hint`. `$ARGUMENTS` is kept. Factory prefers skills for new workflows but still loads commands.

### Hooks

`.factory/hooks.json` is the committed project file ([hooks docs](https://docs.factory.com/harness/hooks)). `sync` keeps the entries it did not write and replaces only its own. With `builtins: [memory]`, a `SessionStart` hook adds the [shared memory](@/docs/memory.md) index to the session.

- Nine events: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Notification`, `Stop`, `SubagentStop`, `PreCompact`, `SessionStart`, `SessionEnd`.
- The file is keyed by event name, `{"<Event>": [{matcher, hooks: [...]}]}`, with no `hooks` wrapper (unlike Claude Code, Codex, Gemini, and Qoder).
- Per entry: `type` (always `"command"`), `command`, and optional `timeout` in seconds (default 60). Factory's `commandRegex` has no spec field and is not written.
- `matcher` is a regex over Factory's tool names, which match Claude's except `Bash`, `Write`, and `WebFetch`. A matcher that uses those is written as is, matches nothing, and raises a sync note.

### MCP

`.factory/mcp.json` under `mcpServers`, the Claude Code and Cursor shape (stdio: `command`, `args`, `env`, no `type`; remote: `type`, `url`, `headers`).

- `sync` rewrites the file whole. Factory's [MCP docs](https://docs.factory.com/harness/mcp) say to remove project servers by editing this file, but sync would undo that edit, so remove the MCP spec instead.
- `disabled: true` becomes Factory's per-server boolean. See [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target).
- Both transports keep `disabledTools`, `timeout`, and `connectTimeout` (milliseconds), zeros included. Remote HTTP and SSE servers also accept `oauth: false` or an OAuth object with `scopes`, `resource`, `authorizationServerIssuer`, `clientId`, `clientSecret`, `clientMetadataUrl`, `tokenEndpointAuthMethod`, and `callbackPort`.
- Set `x-factory.enabledTools` to load only the named tools. An empty list adds no restriction. `disabledTools` excludes named tools, including a name present in both lists. Sync preserves both lists, including explicit `[]`, without removing overlapping names. See [Factory tool filtering](https://docs.factory.com/harness/mcp#tool-filtering).
- `x-factory` overrides each top-level option. A `type: ws` spec writes no server and raises a sync note, because Factory supports only stdio, HTTP, and SSE.

### Settings

A portable `model` merges into `<git-root>/.factory/settings.json`, the project level of Factory's [hierarchical settings](https://docs.factory.com/enterprise/hierarchical-settings-and-org-control).

- A portable `effort` merges as top-level `reasoningEffort` ([CLI settings](https://docs.factory.com/droid-cli/settings)). Values are `none`, `dynamic`, `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, or `max`, and each model accepts a subset. Any other value raises a sync note. The managed `sessionDefaultSettings.reasoningEffort` is not written.
- Sync merges. It sets only `model`, `reasoningEffort`, the command lists, and `x-factory` keys, so `disabledSkills` and other keys stay.
- `x-factory` sets keys that have no portable field, mainly `sandbox` (file and process limits enforced by the operating system). Its command-list entries add to the translated ones: `x-factory.commandBlocklist: ["author-only"]` beside `deny: ["Bash(rm:*)"]` writes both.
- Portable `allow`, `deny`, and `ask` become Factory's shell-command lists. `Bash(npm:*)` becomes `"npm *"` and `Bash(curl)` becomes `"curl"`.

| portable | Factory key | why |
|---|---|---|
| `allow` | `commandAllowlist` | Always runs without approval. |
| `ask` | `commandDenylist` | Always asks; you can approve. |
| `deny` | `commandBlocklist` | Never runs. No approval. |

- Factory's denylist only prompts, so portable `deny` maps to the blocklist. The denylist beats the allowlist, and the blocklist beats both.
- Rules scoping a path, URL, or MCP tool (such as `Read(src/**)`) have no shell form and raise a sync note.
- A list is written only when a rule translates into it. A list you keep by hand stays if no rule feeds it. If a rule does, sync replaces that key.
- Factory deprecates the three lists in favor of `permissionRules` but still reads them ([LLM safety and agent controls](https://docs.factory.com/enterprise/llm-safety-and-agent-controls)). A permission rule needs a stable `id`, a `match.prefix` token list, and `tests.match` and `tests.noMatch` examples, which portable lists lack. Put them under `x-factory.permissionRules`, which is copied into `.factory/settings.json` as written.

### Handoff from a linked worktree

With `handoff` or `handoff-hook` on, sync adds the main checkout's handoff store to `sandbox.filesystem.allowWrite` in `.factory/settings.local.json`, so Factory's [sandbox](https://docs.factory.com/autonomy-and-safety/sandbox) lets handoff saves and the automatic snapshot reach it. The file stays out of Git, and sync never turns the sandbox on. See [Write from a linked worktree](@/docs/handoff.md#write-from-a-linked-worktree).

</details>

## Config keys

| Key | Default |
| --- | --- |
| `outputs.factory.agents-dir` | `.factory/droids` |
| `outputs.factory.skills-dir` | `.agents/skills` |
| `outputs.factory.commands-dir` | `.factory/commands` |
| `outputs.factory.hooks-file` | `.factory/hooks.json` |
| `outputs.factory.mcp-file` | `.factory/mcp.json` |
| `outputs.factory.conf-file` | `.factory/settings.json` |

## Import

`agnostic-ai import factory` reads these back:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` embedded `## Rules` block (`### <name>` children) | `<rules>/<name>.md` per rule |
| `.factory/droids/<name>.md` | `<agents>/<name>.md` |
| `.agents/skills/<name>/SKILL.md` and `.factory/skills/<name>/SKILL.md` | `<skills>/<name>/SKILL.md` |
| `<scope>/.factory/skills/<name>/SKILL.md` | `<skills>/<scope>/<name>/SKILL.md` |
| `.factory/commands/<name>.md` | `<commands>/<name>.md` |
| `.factory/mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml` |
| `.factory/hooks.json`, else the legacy `.factory/hooks/hooks.json` | one hook spec per matcher group |
| `.factory/settings.json` `model`, `reasoningEffort`, and command lists | `<settings>/factory.yaml`; `reasoningEffort` becomes `effort`, or `x-factory.reasoningEffort` when another settings spec sets a different effort |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

- **Droids**: `tools` names map back (`Execute` to `Bash`, `Create` to `Write`, `FetchUrl` to `WebFetch`). A list with a category (`read-only`) or an MCP tool ID goes under `x-factory.tools` as written. `reasoningEffort` becomes `effort`. Other droid keys go under `x-factory`.
- **MCP servers**: `enabledTools` imports under `x-factory.enabledTools`. Empty lists and names shared with `disabledTools` stay intact.
- **Command lists**: these read back as `Bash(...)` rules: `commandAllowlist` to `allow`, `commandDenylist` to `ask`, `commandBlocklist` to `deny`. A pattern ending in ` *` becomes `Bash(x:*)`. Other `settings.json` keys stay in the file.

<details>
<summary>Read-only droids on import</summary>

A `tools: read-only` that came from a portable `readonly: true` also goes under `x-factory.tools`, because import does not turn it back into `readonly`. Syncing again writes the same droid file.

</details>

## Protected paths

Not enforced. Sync cannot write an edit-blocking rule or hook for this tool, so it prints a sync note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.
