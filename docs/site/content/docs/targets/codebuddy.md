+++
title = "CodeBuddy Code"
description = "Generate native Tencent CodeBuddy Code configuration."
weight = 127

[extra]
group = "Reference"
target_id = "codebuddy"
+++

# CodeBuddy Code (`codebuddy`)

Use `codebuddy` for Tencent's CodeBuddy Code CLI. This adapter follows the [official CLI documentation](https://www.codebuddy.ai/docs/cli/installation) and npm package `@tencent-ai/codebuddy-code` 2.173.0.

Select it explicitly with `targets: [codebuddy]`. It is outside the default target list because native agent names reject digits, and scoped or empty agent tool lists cannot always be represented.

## First rule

Create one rule, generate its native files, and check them:

```bash
mkdir -p .agnostic-ai/rules
cat > agnostic-ai.yaml <<'YAML'
version: 1
targets: [codebuddy]
YAML
cat > .agnostic-ai/rules/commits.md <<'RULE'
---
name: commits
---
Use Conventional Commits.
RULE
agnostic-ai sync
agnostic-ai sync --check
```

Open `CODEBUDDY.md` and find the rule text. CodeBuddy loads that file as project instructions. Keep editing the source rule, then sync again.

## Native files

| Content | Project output | User output with `sync --global --only codebuddy` |
|---|---|---|
| Instructions and always-on rules | `CODEBUDDY.md` (or imported `.codebuddy/CODEBUDDY.md`) | `~/.codebuddy/CODEBUDDY.md` |
| Directory-scoped rules | `<scope>/CODEBUDDY.md` | Not supported |
| Conditional rules | `.codebuddy/rules/<name>.md` | Not supported |
| Agents | `.codebuddy/agents/<name>.md` | `~/.codebuddy/agents/<name>.md` |
| Skills and bundled files | `.codebuddy/skills/<name>/SKILL.md` | `~/.codebuddy/skills/<name>/SKILL.md` |
| Commands | `.codebuddy/commands/<name>.md` | Project-only in agnostic-ai |
| Settings and hooks | `.codebuddy/settings.json` | `~/.codebuddy/settings.json` |
| MCP servers | Root `.mcp.json`, opt-in | `~/.codebuddy/.mcp.json` |

CodeBuddy loads ancestor instructions and discovers descendant `CODEBUDDY.md` files when it works there. Its modular rules directory is loaded from the current working directory only. Conditional rules carry `alwaysApply: false` and `paths`; paths alone do not make a rule conditional. Use portable `scope` for a whole directory. A scoped rule with narrower file filters is reported as unsupported.

## Import an existing setup

```bash
agnostic-ai import codebuddy
agnostic-ai sync --plan
agnostic-ai sync
agnostic-ai sync --check
agnostic-ai lint
```

Import reads root and scoped instructions, modular rules, agents, skill folders and resources, commands, settings, command hooks, and root MCP files. Native fields stay under `x-codebuddy` when they have no portable equivalent. Top-level environment and credential settings stay in the native file. Nested credentials and inline agent MCP literals fail import with a field-only diagnostic; use native environment references in supported MCP fields. Native MCP import reads `.mcp.json` before deprecated `mcp.json`. Configure the opt-in below before syncing imported project servers.

Instruction frontmatter keeps its native `enabled`, `alwaysApply`, and `paths` values. `enabled: false` and `alwaysApply: false` without paths stay inactive. Import uses the source-only `x-codebuddy.instruction-file` hint, either `CODEBUDDY.md` or `.codebuddy/CODEBUDDY.md`, with validated `scope` to restore the original location. This hint is omitted from native output. If both root alternatives exist, select one with the [entry-point file configuration](@/docs/configuration.md#entry-point-files) before import.

A controlled root file cannot share its instruction policy with effective shared or local instructions, root rules, or the target overview. Sync reports that conflict before writing. Scoped native files likewise reject mixed activation policies for one path.

## Enable project MCP output

```yaml
outputs:
  codebuddy:
    root-mcp-file: .mcp.json
```

Sync writes an explicit `type`: `stdio`, `http`, or `sse`. Import accepts native JSONC and infers omitted or empty types from `command` (stdio) or `url` (http). Stdio servers retain `command`, `args`, `env`, and `cwd`; remote servers retain `url` and `headers`. Both retain `description`. Native `cwd` strings stay unchanged; CodeBuddy expands uppercase `${NAME}` and `${NAME:-default}` references there. Environment references stay unexpanded by sync. `x-codebuddy.defer_loading` and `x-codebuddy.tools` keep native loading controls.

Sync rejects disabled servers and remote-server `cwd`. Sync never approves project servers. CodeBuddy asks the user before their first connection.

At user scope, sync and import use the first existing file: `~/.codebuddy/.mcp.json`, deprecated `~/.codebuddy/mcp.json`, then legacy `~/.codebuddy.json`. New output uses the preferred file. Comments, account state, the global `projects` map, and unmanaged servers stay intact.

## Fields and limits

Agent names use lowercase letters and hyphens. Scoped tool patterns, server-only MCP names, and empty agent tool lists have no verified restrictive exposure mapping, so they fail sync. Keep scoped shell and file policies in permission rules. Use per-spec target filters when an agent requires a policy CodeBuddy cannot represent; `x-codebuddy.tools` supplies an explicit native exposure list.

Agents accept `name`, `description`, `tools`, `disallowedTools`, `model`, `effort`, `skills`, `mcpServers`, `permissionMode`, `maxTurns`, `background`, `initialPrompt`, `memory`, and `disable-model-invocation`. The last field keeps a manual-only agent out of model selection. Native additions belong under `x-codebuddy`. Portable tool names map only to verified native tools; sync fails when a restriction has no mapping, preventing broader access. Native agent memory adds `Read`, `Write`, and `Edit` even to a restrictive tools list. Sync reports that widening through `on-unsupported`; `error` rejects it before writing.

Skills retain native scoped allowed-tool strings such as `Bash(git:*)` and `Edit(src/**/*.ts)`, invocation controls, and native `context`, `agent`, `model`, and `hooks`. CodeBuddy applies agent, model, and hook overrides only with `context: fork`. Bundled files are copied unchanged. Commands retain description, argument hint, model, allowed tools, invocation control, and their body placeholders. Global commands are not managed by agnostic-ai, although CodeBuddy itself has a user command directory.

MCP permission rules accept `mcp__server`, `mcp__server__*`, and exact `mcp__server__tool`. The all-server `mcp__*` form works in `ask` and `deny` only. Partial-segment wildcards are rejected. These permission forms follow the [native permission grammar](https://www.codebuddy.ai/docs/cli/permissions#mcp-tools).

Settings map `model` to `model`, `effort` to `reasoningEffort`, permission lists to `permissions.allow/ask/deny`, and `permissions.default-mode` to `permissions.defaultMode`. Effort accepts `minimal`, `low`, `medium`, `high`, `xhigh`, and `max`. Session retention maps to `cleanupPeriodDays` at user scope. Project `autoMode` and `permissions.defaultMode: auto` are rejected because CodeBuddy ignores them there.

Command hooks retain native event names, matchers, and timeouts in seconds. Arguments are shell-quoted into the command. Settings merge with existing native keys and hook entries. CodeBuddy reviews hook changes itself; frontmatter hooks also require the user's `allowUntrustedFrontmatterHooks` choice. Sync never makes trust or approval decisions.

Ignore files, review configuration, environments, plugins, credentials, sessions, and automatic memories have no output in this adapter.

## Protected paths

Not enforced. CodeBuddy's `protected` paths have no registered enforcement mapping in this adapter. Use native permission rules for the operations you want to deny.

## Config keys

For the shared root instruction file, see [entry-point files](@/docs/configuration.md#entry-point-files).

| Output key | Default |
|---|---|
| `dir` under `outputs.codebuddy` | `.codebuddy` |
| `outputs.codebuddy.rules-dir` | `.codebuddy/rules` |
| `outputs.codebuddy.agents-dir` | `.codebuddy/agents` |
| `outputs.codebuddy.skills-dir` | `.codebuddy/skills` |
| `outputs.codebuddy.commands-dir` | `.codebuddy/commands` |
| `outputs.codebuddy.conf-file` | `.codebuddy/settings.json` |
| `outputs.codebuddy.root-mcp-file` | Unset |

`dir` moves the tool directory; individual keys take precedence. Scoped instruction files keep their native names. Custom paths need CodeBuddy's own discovery configuration where they fall outside its documented paths.
