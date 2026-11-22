+++
title = "Factory native reference"
description = "Native field mappings, output overrides, and import coverage for factory."
weight = 211

[extra]
nav_title = "Factory reference"
group = "Reference"
+++

# Factory native reference

Start with the [Factory setup](@/docs/targets/factory.md). For home configuration and ownership, see [user configuration](@/docs/global-configuration.md).

## Agents

`<name>.md` has `name`, `description`, and optional `model` and `tools`. `x-factory` keys pass through.

- Empty body: skipped with a sync note (Droid CLI needs a system prompt).
- A portable `mcpServers` list is written as is. `mcpServers: []` excludes every MCP server, even user-level ones ([`mcpServers`](@/docs/spec-format/agents-reference.md#mcpservers-support-by-target)).
- A read-only droid with no `mcpServers` list gets `mcpServers: []`. A list you set is kept.
- `effort` becomes `reasoningEffort`: `low`, `medium`, `high` only. `xhigh`, `max`, and integer budgets drop with a sync note. Ignored under `model: inherit` ([`effort`](@/docs/spec-format/agents.md#per-target-model-and-effort)).
- `readonly: true` writes `tools: read-only`, replacing a portable `tools` list. Sync notes `x-factory.tools` for a custom list.
- `x-factory.tools` wins over `readonly`. `readonly: false` does nothing.

## Tools

The generic `tools` list maps to Droid CLI IDs: `Read`, `LS`, `Grep`, `Glob`, `Create`, `Edit`, `ApplyPatch`, `Execute`, `WebSearch`, `FetchUrl`. One unknown ID makes Factory reject the droid.

| Generic name | Result |
|---|---|
| `Bash` | `Execute` |
| `Write` | `Create` |
| `WebFetch` | `FetchUrl` |
| `TodoWrite`, `Skill` | Dropped silently (every droid has them) |
| `ExitSpecMode`, `GenerateDroid` | Dropped (custom droids cannot enable them) |
| Anything else | Dropped with a sync note; the rest are written |

- Other names already match.
- `tools: all` is never written. An omitted key allows every tool.
- `x-factory.tools` writes Factory's own names and wins. Only it can set a category (`read-only`, `edit`, `execute`, `web`, `mcp`) or an MCP tool ID ([`tools`](@/docs/spec-format/agents-reference.md#tools-support-by-target)).

## Skills

Written once to the tree that codex, amp, zed, and crush share.

- Factory also reads [`.agent/skills/**/SKILL.md`](https://docs.factory.com/harness/skills). This adapter does not write it.
- Scoped skills: `<scope>/.factory/skills/<name>/SKILL.md`, with bundled files.
- Unscoped skills: `.agents/skills/`.
- `outputs.factory.skills-dir` replaces the directory at the root and in each scope.

## Commands

`.factory/commands/<name>.md` takes `description` and `argument-hint`. `$ARGUMENTS` is kept. Factory prefers skills for new workflows but still loads commands.

## Hooks

`.factory/hooks.json` is the committed project file ([hooks docs](https://docs.factory.com/harness/hooks)). `sync` replaces its own entries and keeps the rest. With `builtins: [memory]`, a `SessionStart` hook adds the [shared memory](@/docs/memory.md) index.

- Events: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Notification`, `Stop`, `SubagentStop`, `PreCompact`, `SessionStart`, `SessionEnd`.
- Keyed by event, `{"<Event>": [{matcher, hooks: [...]}]}`, with no `hooks` wrapper (unlike Claude Code, Codex, Gemini, and Qoder).
- Each entry: `type` (always `"command"`), `command`, optional `timeout` in seconds (default 60).
- `failClosed: true` writes `"onFailure": "block"`. A hook that fails, times out, or exits other than 0 or 2 then blocks the event.
- Factory honors `onFailure` on `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, and `PreCompact`. Sync notes other events.
- Another `onFailure` value goes in `x-factory.onFailure`.
- `x-factory.preserveRiskLevel: true` keeps the model's `riskLevel` when a `PreToolUse` hook rewrites an `Execute` command. Sync notes it on other events.
- `x-factory.commandRegex` sets the group-level `commandRegex`. Different regexes never share a group. `import factory` fills it.
- Exact-name tool matchers accept Claude aliases: `Bash` selects `Execute`, `Write` selects `Create`, and `WebFetch` selects `FetchUrl`. Sync keeps the matcher as written.
- Regular expressions match Droid tool names only, and hook input reports those names. A script checking `tool_name` must handle `Execute`, `Create`, or `FetchUrl` even under a Claude alias. See [Factory matchers](https://docs.factory.com/harness/hooks#matchers).

## MCP

`.factory/mcp.json` under `mcpServers`, the Claude Code and Cursor shape. Stdio: `command`, `args`, `env`, no `type`. Remote: `type`, `url`, `headers`.

- `sync` rewrites the file whole. Factory's [MCP docs](https://docs.factory.com/harness/mcp) say to edit it to remove servers, but sync undoes that. Remove the spec.
- `disabled: true` becomes the per-server boolean ([`disabled`](@/docs/spec-format/mcps-reference.md#disabled-support-by-target)).
- Both transports keep `disabledTools`, `timeout`, and `connectTimeout` (milliseconds), zeros included.
- Remote HTTP and SSE also take `oauth: false` or an OAuth object: `scopes`, `resource`, `authorizationServerIssuer`, `clientId`, `clientSecret`, `clientMetadataUrl`, `tokenEndpointAuthMethod`, `callbackPort`.
- `x-factory.enabledTools` loads only the named tools. An empty list adds no restriction.
- `disabledTools` excludes named tools, even one also in `enabledTools`.
- Sync keeps both lists as written, including `[]` and overlaps ([tool filtering](https://docs.factory.com/harness/mcp#tool-filtering)).
- `x-factory` overrides each top-level option.
- `type: ws` writes no server and raises a sync note. Factory supports only stdio, HTTP, and SSE.

## Settings

A portable `model` merges into `<git-root>/.factory/settings.json`, the project level of Factory's [hierarchical settings](https://docs.factory.com/enterprise/hierarchical-settings-and-org-control).

- `effort` merges as top-level `reasoningEffort` ([CLI settings](https://docs.factory.com/droid-cli/settings)): `none`, `dynamic`, `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`. Each model accepts a subset. Other values raise a sync note.
- The managed `sessionDefaultSettings.reasoningEffort` is not written.
- Sync sets only `model`, `reasoningEffort`, the command lists, and `x-factory` keys. Others (such as `disabledSkills`) stay.
- `x-factory` sets keys with no portable field, mainly `sandbox` (OS-enforced file and process limits).
- `x-factory` command-list entries add to translated ones: `x-factory.commandBlocklist: ["author-only"]` beside `deny: ["Bash(rm:*)"]` writes both.
- `Bash(npm:*)` becomes `"npm *"`. `Bash(curl)` becomes `"curl"`.

| Portable | Factory key | Effect |
|---|---|---|
| `allow` | `commandAllowlist` | Runs without approval |
| `ask` | `commandDenylist` | Always asks; you can approve |
| `deny` | `commandBlocklist` | Never runs |

- Factory's denylist only prompts, so `deny` maps to the blocklist.
- Precedence: blocklist, then denylist, then allowlist.
- Rules scoping a path, URL, or MCP tool (such as `Read(src/**)`) have no shell form and raise a sync note.
- A list is written only when a rule feeds it. A hand-kept list stays unless a rule feeds that key, then sync replaces it.
- Factory deprecates the three lists for `permissionRules` but still reads them ([safety controls](https://docs.factory.com/enterprise/llm-safety-and-agent-controls)).
- A permission rule needs a stable `id`, a `match.prefix` token list, and `tests.match` and `tests.noMatch` examples. Portable lists lack these. Put them under `x-factory.permissionRules`, copied as written.

## Handoff from a linked worktree

With `handoff` or `handoff-hook` on, sync adds the main checkout's handoff store to `sandbox.filesystem.allowWrite` in `.factory/settings.local.json`. Factory's [sandbox](https://docs.factory.com/autonomy-and-safety/sandbox) then lets handoff saves and the automatic snapshot reach it.

- The file stays out of Git. Sync never turns the sandbox on.
- See [Write from a linked worktree](@/docs/handoff.md#write-from-a-linked-worktree).

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
| `AGENTS.md` `## Rules` block (`### <name>` children) | `<rules>/<name>.md` per rule |
| `.factory/droids/<name>.md` | `<agents>/<name>.md` |
| `.agents/skills/<name>/SKILL.md`, `.factory/skills/<name>/SKILL.md` | `<skills>/<name>/SKILL.md` |
| `<scope>/.factory/skills/<name>/SKILL.md` | `<skills>/<scope>/<name>/SKILL.md` |
| `.factory/commands/<name>.md` | `<commands>/<name>.md` |
| `.factory/mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml` |
| `.factory/hooks.json`, else legacy `.factory/hooks/hooks.json` | One spec per consecutive run of commands with the same `timeout`, `onFailure`, `preserveRiskLevel`. `onFailure: "block"` becomes `failClosed: true`. |
| `.factory/settings.json` `model`, `reasoningEffort`, command lists | `<settings>/factory.yaml`. `reasoningEffort` becomes `effort`, or `x-factory.reasoningEffort` when another settings spec sets a different effort. |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

Imported hook names start with their group and command-run positions, so sync keeps command order in each matcher group. Keep those prefixes when renaming.

- **Droids**: `tools` map back (`Execute` to `Bash`, `Create` to `Write`, `FetchUrl` to `WebFetch`). A category (`read-only`) or MCP tool ID list goes under `x-factory.tools` as written. `reasoningEffort` becomes `effort`. Other keys go under `x-factory`.
- **MCP servers**: `enabledTools` goes under `x-factory.enabledTools`. Empty lists and names shared with `disabledTools` stay.
- **Command lists**: `commandAllowlist` becomes `allow`, `commandDenylist` becomes `ask`, `commandBlocklist` becomes `deny`, as `Bash(...)` rules. A pattern ending in ` *` becomes `Bash(x:*)`. Other `settings.json` keys stay.

{% <details summary="Read-only droids on import"> %}

A `tools: read-only` from `readonly: true` also goes under `x-factory.tools`. Import does not restore `readonly`. Syncing again writes the same droid file.

{% </details> %}

### Adopt a changed hook order

Existing hook specs win on re-import. To adopt an order edited in the native file, import it into a temporary project with no specs. From your project root, run this. Use your `outputs.factory.hooks-file` path in the copy if it differs from `.factory/hooks.json`.

```sh
factory_import_dir=$(mktemp -d) &&
mkdir -p "$factory_import_dir/.factory" &&
printf '%s\n' 'version: 1' 'targets: [factory]' > "$factory_import_dir/agnostic-ai.yaml" &&
cp .factory/hooks.json "$factory_import_dir/.factory/hooks.json" &&
(
  cd "$factory_import_dir" &&
  agnostic-ai import factory &&
    agnostic-ai sync --only factory --dry-run --diff
) &&
printf 'Review hook specs in %s\n' "$factory_import_dir/.agnostic-ai/hooks"
```

Then:

1. Compare the new specs with your hook source folder. Carry over comments, descriptions, target filters, and other spec-only edits.
2. A command inserted inside a run can split one spec into several. Point references to each old hook at its replacements.
3. Copy the affected matcher group's old specs outside your hook folder, remove only those, and copy in the reviewed ones. Keep unrelated specs.
4. In the original project, validate and preview:

```sh
agnostic-ai validate &&
  agnostic-ai sync --only factory --dry-run --diff
```

Check the diff for the order you want, then write and check the native file:

```sh
agnostic-ai sync --only factory &&
  agnostic-ai sync --only factory --check
```
