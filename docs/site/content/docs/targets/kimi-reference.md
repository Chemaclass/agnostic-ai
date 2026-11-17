+++
title = "Kimi Code CLI native reference"
description = "Native field mappings, output overrides, MCP, profile export, and import coverage for kimi."
weight = 281

[extra]
nav_title = "Kimi Code CLI reference"
group = "Reference"
+++

# Kimi Code CLI native reference

Start with the [Kimi setup](@/docs/targets/kimi.md).

## Output overrides

| Key | Default |
|---|---|
| `agents-dir` | `.kimi-code/agents` |
| `skills-dir` | `.kimi-code/skills` |
| `mcp-file` | `.kimi-code/mcp.json` |
| `root-mcp-file` | Unset, opt-in repository-root mirror |
| `rules-file` | Unset, optional concatenated instructions |
| `conf-file` | Unset, explicit data-root `config.toml` export |

Overrides change generated paths. Configure Kimi to find a relocated file. Skills stay at project root, and nested skill scopes are refused.

## Agents and skills

### Agents

Agents use YAML frontmatter and a Markdown prompt.

- Portable capabilities map to Kimi's `Read`, `Write`, `Edit`, `Bash`, `FetchURL`, and `WebSearch`. `Glob` and `Grep` pass through.
- An explicit empty tool list denies every tool. An omitted list is unrestricted.
- Scoped tool rules such as `shell(git diff *)` cannot be represented by the allowlist and stop sync.
- `whenToUse`, `override`, `disallowedTools`, `subagents`, and tool patterns stay under `x-kimi`.
- Replacing a built-in agent needs an explicit native `override: true`.
- `model`, `effort`, and `maxTurns` have no effect in this Kimi release and raise coverage notes. Portable `readonly` has no mapping.

### Skills

- Supporting files are preserved. `disable-model-invocation` maps to `disableModelInvocation`.
- Native `type`, `whenToUse`, and `arguments` can be set under `x-kimi`.
- Kimi runs skills by hand as `/skill:<name>`. Standalone command specs are unsupported.
- Portable skill tool restrictions and model fields have no native enforcement.

## MCP connections

MCP supports `stdio`, `http`, and `sse`.

- A disabled server emits `enabled: false`, which keeps its override of a same-name user server.
- Native `enabledTools: []` exposes no tools.
- Keep native timeouts (milliseconds), tool filters, OAuth configuration, and `deferred` under `x-kimi`. Deferral also depends on native runtime and model support.

Stdio servers keep `x-kimi.executor: local` and the remote environment that starts the server:

| Key | Kimi Code versions |
|---|---|
| `x-kimi.environment_id` | 2.5.0 and later |
| `x-kimi.runtime_id` | 2.2.0 to 2.4.x |

- Each release ignores the other key, so a team on mixed releases can set both.
- Sync reads `environment_id` only from `x-kimi`. A top-level `environment_id` belongs to Codex and never reaches Kimi.
- Environments are defined in the user's Kimi configuration.
- Sync and import reject every other executor, including `kaos` (removed in Kimi Code 2.2.0). Remove the option, or choose `local` to run the server locally. Sync cannot choose the execution location for you.

Environment references:

- Kimi inherits the process environment and copies configured strings literally. It does not expand general `${NAME}` references in connection fields.
- A portable `Authorization: Bearer ${TOKEN}` maps exactly to native `bearerTokenEnvVar: TOKEN`.
- Other unresolved references omit the whole server, so a missing credential never gives an unauthenticated connection.
- Write `$${NAME}` for literal text.
- Sync never reads the named secret or starts a server.

Kimi merges the user MCP file, repository-root `.mcp.json`, then working-directory `.kimi-code/mcp.json`. Later same-name entries replace earlier ones. Sync writes only the branded project file by default.

## Selected settings and hooks {#selected-profile}

Global sync:

- Maps `model` to an existing native `default_model` alias.
- Accepts boolean `x-kimi.default_plan_mode` and `x-kimi.telemetry`.
- Edits only those owned scalar keys and native hook records. Other user text and tables stay.
- Does not support global effort, because it does not track ownership of `[thinking].effort`. Set it in an explicit profile export.

To export settings and hooks, opt into a separate Kimi data root:

```yaml
outputs:
  kimi:
    conf-file: kimi-profile/config.toml
```

```sh
agnostic-ai sync -t kimi
KIMI_CODE_HOME="$PWD/kimi-profile" kimi
```

This selects a separate data root for agents, skills, connections, and credentials. It does not add project settings. Configure its native model and provider settings yourself. To place the other generated resources there, set their output paths explicitly.

In this export:

- `model` writes `default_model`. `effort` writes `[thinking].effort`.
- Native tables pass through `x-kimi`, including `permission` and `secondary_model`.
- Portable permission and protected-path fields are unsupported.

Native hooks accept only these fields:

- `event`
- `matcher` (optional)
- `command`
- `timeout` (optional integer, 1 to 600 seconds)

There are no native `env` or `cwd` fields. Arguments fold into the command, and unsafe Windows argument folding stops sync. Hooks receive `AGNOSTIC_AI_TARGET=kimi` through their command wrapper. Portable hook lifecycle and decision translation is unsupported; use native Kimi events and response formats.

Native events: `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `PermissionResult`, `UserPromptSubmit`, `UserPromptQueued`, `TurnStarted`, `Stop`, `StopFailure`, `Interrupt`, `SessionStart`, `SessionEnd`, `SessionHeartbeat`, `SubagentStart`, `SubagentStop`, `TaskStarted`, `PreCompact`, `PostCompact`, `Notification`.

## Import and ownership

Run `agnostic-ai import kimi` before syncing handwritten native files. It reads:

- Branded instructions.
- Markdown agents.
- Folder and flat skills. Folder skills win over same-name flat skills.
- The MCP file.
- The explicitly configured TOML profile.

It keeps safe native fields under `x-kimi`, including empty restrictions and disabled connections, and refuses literal credentials. Native files linked outside the project are refused for agents and flat skills. Resource folders follow the shared contained-folder importer.

`agnostic-ai import kimi --global` imports the selected user's model, hooks, and MCP connections.

- Project and global hook imports check native records, and commands inside generated wrappers, for literal credentials before writing hook specs.
- Global sync records ownership. Removing the specs restores user text and leaves unrelated hooks and tables intact.

## Sources

- [Configuration files](https://moonshotai.github.io/kimi-code/en/configuration/config-files)
- [Subagents](https://moonshotai.github.io/kimi-code/en/customization/agents)
- [Skills](https://moonshotai.github.io/kimi-code/en/customization/skills)
- [Released loaders and schema](https://github.com/MoonshotAI/kimi-code/tree/684bae9ea6a437f3bae98bba1dcb797a917d500b/packages/agent-core-v2/src)
