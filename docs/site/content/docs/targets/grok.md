+++
title = "Grok Build"
description = "Set up Grok Build with readable native rules, agents, skills, commands, hooks, and MCP servers."
weight = 265
[extra]
group = "Reference"
target_id = "grok"
+++

# Grok Build (`grok`)

Select Grok explicitly and keep its project instructions visible to Git. Grok skips ignored rule files, so it is excluded from the default targets.

## First rule

Create `agnostic-ai.yaml`:

```yaml
version: 1
targets: [grok]
gitignore:
  enabled: true
  commit: [grok:instructions]
```

Create `.agnostic-ai/rules/style.md`:

```markdown
---
name: style
---

Use Conventional Commits for commit subjects.
```

Run `agnostic-ai sync`, then `agnostic-ai sync --check`. Read `.grok/rules/style.md` and run `git check-ignore --no-index .grok/rules/style.md`: it should report no matching ignored path. A root or global ignore pattern can still hide the file. Sync rejects that case before emitting Grok output; remove the matching pattern yourself. Explicitly disabling agnostic-ai's ignore management is also supported when your own ignore policy keeps the rules visible. The check uses Grok's root `.gitignore` and global ignore file, not nested ignore files or `.git/info/exclude`. It conservatively requires parent directories to be re-included before a file below an ignored directory.

Install and open the CLI using the [official setup guide](https://docs.x.ai/build/overview). Grant project trust through Grok, then verify the rule in a session. Sync does not grant trust or run Grok.

## Output

| Spec | Native path |
|---|---|
| Rule | `.grok/rules/<name>.md` |
| Directory rule | `<scope>/.grok/rules/<name>.md` |
| Agent | `.grok/agents/<name>.md` |
| Skill and resources | `.grok/skills/<name>/SKILL.md` and sibling files |
| Command | `.grok/commands/<name>.md` |
| Command hook | `.grok/hooks/<name>.json` |
| Shared hook scripts | `.grok/hook-scripts/` |
| MCP and permission lists | `.grok/config.toml` |

Scoped skills, agents, and commands use the corresponding directory below their source scope. Rules apply when Grok discovers that directory while walking from its working directory to the repository root. File glob conditions have no native rule equivalent; use a whole directory scope.

Output overrides use `outputs.grok.dir`, `rules-dir`, `agents-dir`, `skills-dir`, `commands-dir`, `hooks-dir`, `mcp-file`, and `conf-file`. `conf-file` separates permission settings from the MCP destination. A custom destination must still be a location Grok reads. Rule visibility checks include custom paths.

## Native mappings

Agents carry `name`, `description`, `model`, native `tools`, `disallowedTools`, `skills`, `effort`, `maxTurns`, `background`, and the supported native fields under `x-grok`. Effort accepts `low`, `medium`, `high`, `xhigh`, or `max`. Portable `can` and `tools` use verified native allowlist aliases and disable unrequested MCP bridge tools. Explicit bridge IDs and `x-grok.tools` retain their native choices; a capability without an exact mapping fails with the agent name. Unknown native tool entries fail because Grok restores its full toolset for an unresolved name. Empty restrictions are rejected because native empty tools inherit all tools. To preserve an existing Grok restriction, use verified native aliases or IDs in `tools` or `x-grok.tools`.

Skills keep `name`, `description`, `when-to-use`, `paths`, `allowed-tools`, `argument-hint`, `user-invocable`, `disable-model-invocation`, `metadata`, `license`, and `compatibility`. Bundled files are copied unchanged. `allowed-tools` describes use and does not enforce permission. `user-invocable: false` also hides a skill from model discovery. Skill model and effort overrides are not applied by the verified contract.

Grok converts skill and command names to lowercase `a-z`, digits and hyphens, with a 64-character limit. Skills can fall back to their folder name. Sync rejects names with no valid native form before writing files.

MCP servers become `[mcp_servers.<name>]`. Stdio keeps `command`, `args`, `env`, and `cwd`. Remote servers keep `url`, `headers`, and explicit HTTP or SSE transport. `disabled` becomes the inverse native `enabled`. Native `startup_timeout_sec` and `tool_timeout_sec` stay seconds. Environment references remain text for Grok to expand. Native extras under `x-grok` include the source-backed authentication and timeout fields. Portable `description` and `timeout` produce coverage notes because they have no exact native mapping.

Project settings write only `permissions.allow`, `permissions.ask`, and `permissions.deny` to `[permission]`; `x-grok.permission` and `x-grok.plugins` carry native project configuration. Existing MCP servers and unrelated configuration keys are retained. Project model, effort, permission default mode, and session retention are unsupported. Ignore, review, and environment specs have no native output.

Hooks use native PascalCase `event` and optional `matcher`, command text, safely quoted `args`, timeout in seconds, and per-handler `env`. Sync adds `AGNOSTIC_AI_TARGET=grok` to generated handlers. Grok provides `CLAUDE_PROJECT_DIR` and `GROK_WORKSPACE_ROOT`. A matcher on `Stop` or `UserPromptSubmit` has no effect and produces a note. Hook errors fail open; native explicit denies block according to the event. Portable `on`/`match` hooks are not translated in this adapter. Use Grok-native events and its documented reply protocol. Project hook execution also requires Grok's hook trust approval.

On Windows, shared script paths that need shell-specific quoting, such as paths with spaces, require an explicit native wrapper command. Sync refuses an automatic rewrite it cannot preserve. Existing native invocation commands keep their quoting.

## Import

Run `agnostic-ai import grok` before the first sync of a native project. It reads root and scoped rule, agent, skill, and command directories, resources, native hook JSON files, MCP tables, compact permission lists, and plugins. Plugins are retained under `x-grok.plugins` from the configured settings destination. `x-grok.hooks` preserves each native hook file's groups and handler fields. Unsupported hook handler types fail import. Imported standalone MCP credentials become environment references through the shared credential filter.

Inline agent MCP and hook metadata keep their native form. Import refuses literal credentials there, and in standalone hooks, before writing the source file. Hook `env` values are passed through literally, so putting `${TOKEN}` there does not read the process environment. Inherit credentials from the process and reference them in hook `command` or `url`, where Grok expands variables. Import errors identify the native path without printing credential values.

Import uses the configured output directories. Importing emitted files into fresh source directories, syncing again, linting, and running `sync --check` are covered by the adapter tests.

## Global configuration

`agnostic-ai sync --global --only grok` writes `$GROK_HOME/AGENTS.md`, `skills/`, `agents/`, `hooks/`, `hook-scripts/`, and `config.toml`. `GROK_HOME` defaults to `~/.grok` and must be absolute when set. Global commands and permission lists are not emitted. The portable global model maps to `[models].default`; global effort is unsupported.

Global MCP servers and the default model share one planned config write. Sync owns individual servers and the model key, preserves other text and private file modes, checks for concurrent edits, and removes only unchanged owned values. `agnostic-ai import --global grok` reads the supported settings, MCP, and hook surfaces. Literal credentials are skipped by global import.

## Protected paths

Not enforced. Use an instruction rule or explicit permission rules. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

<details>
<summary>Native evidence and runtime limits</summary>

Paths and fields were checked against the [official project rules](https://docs.x.ai/build/features/project-rules), [MCP](https://docs.x.ai/build/features/mcp-servers), [hooks](https://docs.x.ai/build/features/hooks), [subagents](https://docs.x.ai/build/features/subagents), [skills](https://docs.x.ai/build/features/skills-plugins-marketplaces), and [settings](https://docs.x.ai/build/settings) documentation, plus public source snapshot [2bdd1d6a](https://github.com/xai-org/grok-build/tree/2bdd1d6a6369de0e8c68132ea4539e9abd9e14a8). The stable installer endpoint reported 1.0.67 on October 23, 2026. The source snapshot is an independent format reference, not proof of exact binary source parity. Validation uses native-format fixtures and isolated filesystem tests; it does not install, authenticate, approve trust, or run a model session. Grok also reads compatibility files from other enabled tools, so those can contribute additional instructions, hooks, or servers.

</details>
