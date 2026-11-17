+++
title = "Cursor"
description = "Cursor: generated files, supported features, and output settings."
weight = 40

[extra]
group = "Reference"
target_id = "cursor"

[extra.moved]
set-up-this-tool = "@/docs/targets/cursor.md#first-rule"
native-mappings = "@/docs/targets/cursor-reference.md"
rules = "@/docs/targets/cursor-reference.md"
per-file-check = "@/docs/targets/cursor-reference.md"
agents = "@/docs/targets/cursor-reference.md"
commands = "@/docs/targets/cursor-reference.md"
old-copies = "@/docs/targets/cursor-reference.md"
skills = "@/docs/targets/cursor-reference.md"
review = "@/docs/targets/cursor-reference.md"
environment = "@/docs/targets/cursor-reference.md"
ignore = "@/docs/targets/cursor-reference.md"
hooks = "@/docs/targets/cursor-reference.md"
mcp = "@/docs/targets/cursor-reference.md"
third-party-reads = "@/docs/targets/cursor-reference.md"
config-keys = "@/docs/targets/cursor-reference.md"
permissions = "@/docs/targets/cursor-reference.md"
import = "@/docs/targets/cursor-reference.md"
+++

# Cursor (`cursor`)

## Native mappings {#native-mappings .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Permissions {#permissions .legacy-anchor}

## Import {#import .legacy-anchor}

## Set up this tool {#set-up-this-tool .legacy-anchor}

Cursor reads its project configuration from `.cursor/`, including `.cursor/rules/`.

## First rule

Follow the [official setup guide](https://cursor.com), then open your project in the tool.

Select `cursor` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. By default, sync writes it to `.cursor/rules/conventional-commits.mdc`.

The IDE and CLI share rules, but the [permission settings](@/docs/targets/cursor-reference.md#permissions) guard only the Cursor CLI.

For existing files, check [what `import cursor` reads](@/docs/targets/cursor-reference.md#import) before syncing.

## Output

```
.cursor/rules/<name>.mdc
.cursor/agents/<name>.md             # one native subagent per agent spec
.cursor/skills/<name>/SKILL.md       # one folder per skill, bundled assets included
.cursor/commands/<name>.md           # one per command spec
.cursor/hooks.json                   # when hook specs exist (sync updates its entries, keeps yours)
.cursor/hooks/agnostic-ai-portable-hook.sh  # when a portable before-tool hook exists
.cursor/mcp.json                     # when MCP entries exist
.cursor/cli.json                     # from settings permissions and deny protected paths (merged)
```

For field mappings, output overrides, and import coverage, see the [Cursor native reference](@/docs/targets/cursor-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Open the project in Cursor and check `.cursor/rules/conventional-commits.mdc` is Always Apply in the Rules panel.
3. Ask for a commit subject for a small fix. It should use `fix:`.

With Codex or another tool that puts rules in `AGENTS.md`, the rule sits in that file's `## Rules` block and the Rules panel does not list it. See [loaded at session start](#loaded-at-session-start).

## Loaded at session start

Cursor reads the root `AGENTS.md` and its always-on rules in every session. The [CLI docs](https://cursor.com/docs/cli/using) say it "reads `AGENTS.md` and `CLAUDE.md` at the project root (if present) and applies them as rules alongside `.cursor/rules`". The [rules docs](https://cursor.com/docs/rules) say "Cursor supports AGENTS.md in the project root and subdirectories", and an Always Apply rule applies "to every chat session".

Cursor writes no `AGENTS.md`. When another tool such as Codex writes it with the `## Rules` block, sync skips the `.cursor/rules/` file of each always-on rule that block holds, so the rule loads once (#3230).

{% <details summary="Which rules keep their .mdc file"> %}
- `sync --only cursor` writes that `AGENTS.md` too, so a partial run keeps the rule.
- Rules scoped by `globs` keep their `.mdc` file.
- Rules Cursor applies by description or by hand keep it too.
- So do rules whose Cursor text differs.
- With `outputs.codex.nested-glob-rules: false`, scoped rules also stay in the root `AGENTS.md`, so Cursor loads them there at start as well.
{% </details> %}

## Protected paths

Enforced for the Cursor CLI only. Each path of a `decision: deny` block becomes `Write(<path>)` and `Write(<path>/**)` rules in `permissions.deny` of `.cursor/cli.json` ([configuration](https://cursor.com/docs/cli/reference/configuration)).

- A path that already ends in `**` gets the first rule only.
- The rules have no leading `/`. Cursor scopes a relative path to the workspace and reads `/` as an absolute path ([CLI permissions](https://cursor.com/docs/cli/reference/permissions)).
- **IDE agent:** not covered ([permissions](https://cursor.com/docs/reference/permissions.md)). State the paths in a rule if it should know them.
- **`decision: ask`:** not enforced, with a sync note. The CLI has no ask list and already prompts before a write no `allow` rule covers. Use `decision: deny` to block.
- **`reason`:** not written. A CLI permission rule has no message field.
- **`model`, `effort`, `x-cursor`:** raise a sync note, since a project `cli.json` takes `permissions` only.

`cli.json` merges as [Permissions](@/docs/targets/cursor-reference.md#permissions) describes. Your own rules and keys stay, and removing a path or its block removes its rules on the next sync. A matching rule that was in `cli.json` before sync added one stays yours. See [Protected paths](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
