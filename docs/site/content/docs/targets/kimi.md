+++
title = "Kimi Code CLI"
description = "Sync and import Kimi Code instructions, agents, skills, MCP connections, and selected user configuration."
weight = 280

[extra]
group = "Reference"
target_id = "kimi"
+++

# Kimi Code CLI (`kimi`)

Sync generates native files for [Kimi Code CLI 2.3.1](https://github.com/MoonshotAI/kimi-code/releases/tag/%40moonshot-ai/kimi-code%402.3.1), the maintained `@moonshot-ai/kimi-code` package. Run Kimi from your synced Git project root. Its own workspace trust choice controls project MCP loading.

Select `kimi` explicitly. It is excluded from the default targets because its native agent tool lists cannot express scoped shell restrictions such as `Bash(git diff *)`. This keeps existing projects working with the default targets.

## Set up this tool

```sh
mkdir -p .agnostic-ai/rules .agnostic-ai/agents
cat > agnostic-ai.yaml <<'YAML'
version: 1
targets: [kimi]
YAML
cat > .agnostic-ai/rules/changes.md <<'MD'
---
name: changes
---
Keep changes small and explain failures.
MD
cat > .agnostic-ai/agents/reviewer.md <<'MD'
---
name: reviewer
description: Review a patch
can: [read]
---
Read the patch and report defects.
MD
agnostic-ai sync --dry-run
agnostic-ai sync
agnostic-ai sync --check
```

## Native paths

| Spec | Project output | User output with `sync --global --only kimi` |
|---|---|---|
| Rules | `.kimi-code/AGENTS.md`, scoped `<scope>/.kimi-code/AGENTS.md` | `~/.kimi-code/AGENTS.md` |
| Agents | `.kimi-code/agents/<name>.md` | `~/.kimi-code/agents/<name>.md` |
| Skills and resources | `.kimi-code/skills/<name>/SKILL.md` | `~/.kimi-code/skills/<name>/SKILL.md` |
| MCP servers | `.kimi-code/mcp.json` | `~/.kimi-code/mcp.json` |
| Hooks | Explicit profile export only | `~/.kimi-code/config.toml`, `[[hooks]]` |
| Settings | Explicit profile export only | Owned scalar keys in `~/.kimi-code/config.toml` |

`KIMI_CODE_HOME` relocates all user paths together. Sync never changes the older `~/.kimi` data directory or runs Kimi's migration.

Kimi-specific instructions and ordinary `AGENTS.md` instructions are additive. Kimi loads instructions from the Git root through the working directory. The uppercase plain filename wins over `agents.md` in one directory. Kimi-only projects use the branded path. When a configured peer already emits the plain `AGENTS.md`, Kimi shares that file and its scoped instruction files so common text is delivered once. Target-specific rules must be compatible with every reader of a shared file. See [shared entry points](@/docs/configuration.md#entry-point-files) for instruction content and path overrides.

Agents and skills are discovered from the nearest Git root, including linked Git worktrees. Kimi's branded MCP file is relative to the session working directory. When launching from a subdirectory, explicitly choose a repository-root MCP mirror if required.

### Output overrides

| Key | Default |
|---|---|
| `agents-dir` | `.kimi-code/agents` |
| `skills-dir` | `.kimi-code/skills` |
| `mcp-file` | `.kimi-code/mcp.json` |
| `root-mcp-file` | Unset, opt-in repository-root mirror |
| `rules-file` | Unset, optional concatenated instructions |
| `conf-file` | Unset, explicit data-root `config.toml` export |

Overrides change generated paths. Kimi must be configured to discover a relocated file. Skills remain at project root; nested skill scopes are refused.

## Agents and skills

Agents use YAML frontmatter and a Markdown prompt. Portable capabilities map to Kimi's `Read`, `Write`, `Edit`, `Bash`, `FetchURL`, and `WebSearch`. `Glob` and `Grep` pass through. An explicit empty tool list denies every tool; an omitted list is unrestricted. Scoped tool rules such as `shell(git diff *)` cannot be represented by this allowlist and stop sync. Native `whenToUse`, `override`, `disallowedTools`, `subagents`, and tool patterns stay under `x-kimi`. Replacing a built-in agent requires an explicit native `override: true`.

Agent `model`, `effort`, and `maxTurns` have no effect in this Kimi release and raise coverage notes. Portable `readonly` has no direct mapping.

Skills preserve their supporting files and map `disable-model-invocation` to `disableModelInvocation`. Native `type`, `whenToUse`, and `arguments` can be set under `x-kimi`. Kimi supports manual invocation as `/skill:<name>`; standalone command specs are unsupported. Portable skill tool restrictions and model fields have no native enforcement.

## MCP connections

MCP supports `stdio`, `http`, and `sse`. A disabled server emits `enabled: false`, retaining its override of a same-name user server. Native `enabledTools: []` exposes no tools. Keep native timeouts in milliseconds, tool filters, OAuth configuration, and `deferred` under `x-kimi`; deferral also depends on native runtime and model support.

Stdio servers preserve `x-kimi.executor: local` and `x-kimi.runtime_id`. Sync and import reject every other executor, including `kaos`, removed in Kimi Code 2.2.0. Remove that option or choose `local` when the server should run locally. Sync cannot choose that execution location for you.

Kimi inherits the process environment and copies configured strings literally. It does not expand general `${NAME}` references in connection fields. A portable `Authorization: Bearer ${TOKEN}` maps exactly to native `bearerTokenEnvVar: TOKEN`. Other unresolved references omit the whole server, so a missing credential never produces an unauthenticated connection. Write `$${NAME}` to request literal text. Sync never reads the named secret or starts a server.

Kimi merges the user MCP file, repository-root `.mcp.json`, then working-directory `.kimi-code/mcp.json`; later same-name entries replace earlier entries. The adapter writes only its branded project file by default.

## Selected settings and hooks {#selected-profile}

Global sync maps `model` to an existing native `default_model` alias and accepts boolean `x-kimi.default_plan_mode` and `x-kimi.telemetry`. It edits only those owned scalar keys and native hook records, preserving other user text and tables. Global effort is unsupported because `[thinking].effort` needs separate leaf ownership; it is available in an explicit profile export.

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

This selects the entire separate data root, including its agents, skills, connections, and credentials. It is not a project settings layer. Configure that root's native model/provider settings yourself. To place the other generated resources there too, set their output paths explicitly.

In this export, `model` writes `default_model`, `effort` writes `[thinking].effort`, and native tables pass through `x-kimi`, including `permission` and `secondary_model`. Portable permission and protected-path fields are unsupported.

Native hooks accept only `event`, optional `matcher`, `command`, and optional integer `timeout` from 1 to 600 seconds. There are no native `env` or `cwd` configuration fields. Arguments fold into the command; unsafe Windows argument folding stops sync. Hooks receive `AGNOSTIC_AI_TARGET=kimi` through their command wrapper. Portable hook lifecycle/decision translation is unsupported; use native Kimi events and response formats.

Native events are `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `PermissionResult`, `UserPromptSubmit`, `UserPromptQueued`, `TurnStarted`, `Stop`, `StopFailure`, `Interrupt`, `SessionStart`, `SessionEnd`, `SessionHeartbeat`, `SubagentStart`, `SubagentStop`, `TaskStarted`, `PreCompact`, `PostCompact`, and `Notification`.

## Protected paths

Not enforced. Sync reports portable protections as unsupported. Native permission rules in an explicit profile constrain individual Kimi tools and do not provide a general filesystem boundary for Bash or MCP tools.

## Import and ownership

Run `agnostic-ai import kimi` before syncing handwritten native files. Import reads the branded instructions, Markdown agents, folder and flat skills, MCP file, and the explicitly configured TOML profile. It preserves safe native fields under `x-kimi`, including empty restrictions and disabled connections, and refuses literal credentials. Folder skills win over same-name flat skills. Native files linked outside the project are refused for agents and flat skills; resource folders follow the shared contained-folder importer.

`agnostic-ai import kimi --global` imports the selected user's model, hooks, and MCP connections. Project and global hook imports check native records and commands inside generated wrappers for literal credentials before writing hook specs. Global sync records ownership so removing the corresponding specs restores user text and leaves unrelated hooks and tables intact.

## Sources

- [Configuration files](https://moonshotai.github.io/kimi-code/en/configuration/config-files)
- [Subagents](https://moonshotai.github.io/kimi-code/en/customization/agents)
- [Skills](https://moonshotai.github.io/kimi-code/en/customization/skills)
- [Released loaders and schema](https://github.com/MoonshotAI/kimi-code/tree/684bae9ea6a437f3bae98bba1dcb797a917d500b/packages/agent-core-v2/src)
