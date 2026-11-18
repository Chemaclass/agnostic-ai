+++
title = "Windsurf / Devin Desktop"
description = "Windsurf / Devin Desktop: generated files, supported features, and output settings."
weight = 80

[extra]
group = "Reference"
target_id = "windsurf"

[extra.moved]
set-up-this-tool = "@/docs/targets/windsurf.md#first-rule"
native-mappings = "@/docs/targets/windsurf-reference.md"
rules = "@/docs/targets/windsurf-reference.md"
entry-point = "@/docs/targets/windsurf-reference.md"
agents = "@/docs/targets/windsurf-reference.md"
scoped-rules = "@/docs/targets/windsurf-reference.md"
rule-activation = "@/docs/targets/windsurf-reference.md"
skills = "@/docs/targets/windsurf-reference.md"
ignore = "@/docs/targets/windsurf-reference.md"
mcp = "@/docs/targets/windsurf-reference.md"
hooks = "@/docs/targets/windsurf-reference.md"
settings = "@/docs/targets/windsurf-reference.md"
workflows = "@/docs/targets/windsurf-reference.md"
config-keys = "@/docs/targets/windsurf-reference.md"
import = "@/docs/targets/windsurf-reference.md"
+++

# Windsurf / Devin Desktop (`windsurf`)

## Native mappings {#native-mappings .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Import {#import .legacy-anchor}

## Set up this tool {#set-up-this-tool .legacy-anchor}

Windsurf became Devin Desktop in June 2026. The target keeps the `windsurf` name, so existing `outputs.windsurf.*` keys and `x-windsurf` fields still work.

## First rule

Follow the [official setup guide](https://devin.ai/desktop), then open your project in the tool.

Select `windsurf` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. By default, sync writes it to `.devin/rules/conventional-commits.md`.

Sync writes `.devin/` paths. See [scoped rules](@/docs/targets/windsurf-reference.md#scoped-rules) for directory discovery.

For existing files, check [what `import windsurf` reads](@/docs/targets/windsurf-reference.md#import) before syncing.

## Output

```
AGENTS.md                            # shared pointer body, plus the rules block when another tool adds it
.devin/rules/<name>.md
<scope>/.devin/rules/<name>.md       # one per scoped rule
.devin/agents/<name>.md              # one per agent (custom subagent profile)
.agents/skills/<name>/SKILL.md       # one folder per skill (shared folder)
.devinignore                         # when ignore entries exist (indexing, Tab, DeepWiki)
.windsurfignore                      # when ignore entries exist (legacy name, older builds)
.devin/mcp_config.json               # when MCP entries exist
.devin/hooks.v1.json                 # when hook entries exist
.devin/config.json                   # when settings entries carry permission rules, or with repo-mode memory
```

For field mappings, output overrides, and import coverage, see the [Windsurf native reference](@/docs/targets/windsurf-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Check `.devin/rules/conventional-commits.md` for the commit-prefix rule.
3. Open the project in Devin Desktop or CLI and ask for a commit subject for a small fix. It should use `fix:`.

With Codex or another tool that puts rules in `AGENTS.md`, the rule sits in that file's `## Rules` block instead. See [loaded at session start](#loaded-at-session-start).

## Loaded at session start

Devin reads the root `AGENTS.md` and its always-on rule files in every session. The [Devin Local customizations page](https://docs.devin.ai/desktop/devin-local/customizations.md) says "`AGENTS.md`, `AGENTS.local.md` ... and `AGENT.md` are all read as always-on rules" and "Rule files in both `.devin/rules/` and `.windsurf/rules/` load". The [Devin CLI rules reference](https://docs.devin.ai/cli/extensibility/rules.md) says "Files at the workspace root are loaded at session start."

When another tool writes the `## Rules` block in `AGENTS.md`, sync skips the `.devin/rules/` file of each always-on rule that block holds, so the rule loads once (#3230).

{% <details summary="Size limit, scoped rules, and turning AGENTS.md off"> %}
- With `outputs.codex.nested-glob-rules: false`, scoped rules also stay in the root `AGENTS.md`, so Devin loads them there at start.
- Neither page states a size limit. Older Cascade docs capped a rule file at 12,000 characters; that page now redirects to Devin Local.
- A client that still applies the cap may cut a long `AGENTS.md`.
- `"agents_standard": false` in Devin's config stops it reading `AGENTS.md`. Sync does not read that setting.
- If you turn it off, set `outputs.windsurf.file` to another path, such as `.devin/unread-instructions.md`.
- Windsurf then keeps a `.devin/rules/` file for every rule.
- Sync writes the shared instructions to that unread path, and the sync summary still lists it as the file Windsurf reads.
{% </details> %}

## Shared memory

With `builtins: [memory]`, a Devin CLI `SessionStart` hook adds the shared memory indexes. It needs POSIX `sh`. Native memory stays available. See [shared memory](@/docs/memory.md#load-at-session-start) for which agnostic-ai it runs.

- Devin does not document the hook's default shell, working directory, timeout, or Windows execution.
- Local tests simulate the hook. They do not verify memory loading in Devin or Windows support.
- The memory rule stays available when hooks do not run.

## Protected paths

Not enforced. This target cannot write a rule or hook that blocks edits. Sync prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
