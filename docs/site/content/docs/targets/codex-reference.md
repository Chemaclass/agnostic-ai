+++
title = "Codex native reference"
description = "Native field mappings, output overrides, and import coverage for codex."
weight = 21

[extra]
nav_title = "Codex reference"
group = "Reference"
+++

# Codex native reference

Start with the [Codex setup](@/docs/targets/codex.md). This page covers native mappings and overrides. For shared home configuration and ownership, see [global configuration](@/docs/global-configuration.md).

## Rule placement and discovery

Rules without a directory scope go into root `AGENTS.md`. Whole-directory patterns such as `src/api/**` can become nested files. `alwaysApply: true` keeps an unscoped rule at the root. A scope plus patterns covers both: `scope: services/payments` and `globs: tests/payments/**` writes to both directories.

Root file selectors, filename filters such as `src/**/*.ts`, and mixed selectors cannot be represented by directory instruction files. Sync keeps them at root with a note, or fails with `on-unsupported: error`. Shared-file conflicts can also force a root fallback. See [scoped context safeguards](@/docs/scoped-context.md#shared-files-and-safe-updates).

Codex discovers `AGENTS.md` from the session working directory and its parents. Start in the subtree to load its nested file; a root-started session does not discover it merely by editing there. [Explain a source file](@/docs/cli-reference/inspect.md#explain-a-source-file) reports planned instruction paths and discovery limits.

## Agent mappings

| Source | Codex output |
|---|---|
| Name, description, body | `name`, `description`, `developer_instructions` in `.codex/agents/<name>.toml` |
| Per-target `model` | Codex model ID; Claude-only names are omitted with a note |
| `effort` | `model_reasoning_effort`; integer budgets are omitted |
| `readonly: true` | Legacy `sandbox_mode = "read-only"`; current agents inherit the session sandbox |
| `x-codex` model fields | `model`, `model_reasoning_effort`, `model_reasoning_summary`, `model_verbosity` |

Sync keeps `features.*`, including false, and `skills.config` path/enabled entries through import. Agent files cannot override the parent's `approval_policy`, `model_provider`, `mcp_servers`, or `service_tier`; preserved import keys get a no-effect note. Set them in session configuration. A generic `tools` list is dropped because Codex's `tools` is a table.

Codex v0.155.0 and later ignore agent `sandbox_mode`. The legacy key remains for older releases; an `x-codex.sandbox_mode` override wins, and null omits it. Use session limits or exec policies for enforced restrictions.

## Skill mappings

Skills go to `.agents/skills/<name>/SKILL.md`, with required `name` and `description`. A scoped skill goes under `<scope>/.agents/skills/`. Codex scans from its working directory up to the repository root. Claude's dynamic command syntax and `$ARGUMENTS` remain plain text, with a note.

Skill `model` and `effort` have no native field and are omitted. `disable-model-invocation: true` becomes `policy.allow_implicit_invocation: false` in `agents/openai.yaml`. An explicit policy value in `x-codex` or a bundled YAML file wins.

Sync writes or merges `agents/openai.yaml` for that policy or `x-codex.interface`, `policy`, and `dependencies`. Tools sharing the same skill directory get the merged file; separate tool directories keep the bundled version.

## Hook mappings and trust

Hooks go to `.codex/hooks.json`, grouped by event and matcher. [Trust handlers with `/hooks`](https://learn.chatgpt.com/docs/hooks) after sync, and again after their configuration changes. Project hooks also need a trusted project. `agnostic-ai doctor -t codex` checks saved trust in `CODEX_HOME` or `~/.codex`; a session trust bypass is outside that check.

Command hooks keep `timeout`, `statusMessage`, `commandWindows`, `additionalContextLimit` (including zero), and `async`. MCP-tool hooks use `type: mcp_tool`, required `server` and `tool`, optional `input`, `timeout`, and `statusMessage`. See the [native hooks reference](https://learn.chatgpt.com/docs/hooks) for event names and payloads.

`PreModelRequest` and `PostModelRequest` are observer events in Codex 0.164.0. Their output is logged; it cannot block or change a model request. Omit `matcher`, since these events supply no matcher input ([released source](https://github.com/openai/codex/blob/6aaac694803004a63c6f393d153c96d22521a6b9/codex-rs/hooks/src/events/model_request.rs)).

Sync prefixes shell commands with `export AGNOSTIC_AI_TARGET=codex; `. This requires a POSIX session shell on macOS and Linux. On Windows it uses `commandWindows`, or copies the declared command without that prefix. `args` fold into POSIX-quoted shell arguments; supply `commandWindows` when Windows quoting differs.

`Edit` and `Write` aliases report the tool as `apply_patch`. Edited paths live inside `tool_input.command`, not `tool_input.file_path`. Use [`agnostic-ai hook paths`](@/docs/spec-format/hook-reference.md#edited-paths). Sync detects inline reads of the missing field, but does not inspect your scripts.

Imported `$CLAUDE_PROJECT_DIR` paths use the Git worktree root and need Git and a POSIX shell. Unsupported forms get a note or fail under `on-unsupported: error`. Import also reads hooks in `config.toml`, deduplicating them with JSON hooks; JSON wins.

## Reviews and environments

Reviews become `## Code Review Rules` in root or scoped `AGENTS.md`. Other tools reading that file see the review section too. A legacy `rules-file` override prevents root reviews and raises a note.

Environment specs write the Codex app's `.codex/environments/environment.toml`. `setup`, `cleanup`, and `setup-windows` become native scripts; `dev-commands` become action buttons with name, command, and icon (default `run`). `cwd` prefixes the command with `cd`. Last top-level values win. Ports, environment maps, URLs, install, and terminals have no native key and get a note.

## MCP mappings

Servers become `[mcp_servers.<name>]` in project config. `disabled: true` becomes `enabled = false`.

| Transport | Native fields |
|---|---|
| Stdio | `command`, `args`, `env`, `cwd`, `env_vars` |
| HTTP/SSE | `url`, `bearer_token_env_var`, `http_headers`, `env_http_headers`, `auth`, `http_headers_helper` |
| Either | `enabled_tools`, `disabled_tools`, `required`, startup and tool timeouts, `default_tools_approval_mode`, `experimental_environment` |

`${NAME}` in env or headers becomes a native environment reference. A reference in URL or args leaves the server out with a note. See [environment references](@/docs/spec-format/mcps.md#environment-references).

HTTP OAuth supports `scopes`, `oauth_resource`, and an `oauth` table with `client_id`, `callback_url`, and `callback_port`. A per-tool `tools` map writes native sub-tables. Set only one startup timeout unit (`startup_timeout_sec` or `startup_timeout_ms`). See the [Codex config reference](https://learn.chatgpt.com/docs/config-file/config-reference) for native options and defaults.

Sync rebuilds project `config.toml`. Import existing keys into the overlay before syncing; keep unmanaged user configuration in your home config.

## Config keys

| Key | Default | Notes |
|---|---|---|
| `outputs.codex.agents-dir` | `.codex/agents` | override the agent output directory |
| `outputs.codex.skills-dir` | `.agents/skills` | the path Codex scans |
| `outputs.codex.nested-glob-rules` | `true` | put rules that cover whole directories in nested `AGENTS.md`; `false` keeps them in the root file |
| `outputs.codex.shared-subagents` | `true` | writes the per-skill tree at `skills-dir`; set `false` to skip Codex skills |
| `outputs.codex.commands-dir` | unset | set to e.g. `.codex/prompts` to write the deprecated project prompts layout |
| `outputs.codex.mcp-file` | `.codex/config.toml` | |
| `outputs.codex.hooks-file` | `.codex/hooks.json` | |
| `outputs.codex.environment-file` | `.codex/environments/environment.toml` | |
| `outputs.codex.rules-file` | unset | writes legacy concatenated rules and skips the write of instructions that point to source specs |
| `outputs.codex.exec-policies` / `outputs.codex.exec-policies-file` | unset | write `.codex/rules/default.rules` |
| `outputs.codex.exec-policies-from-permissions` | `false` | translate simple Bash permission rules when no native policy source is set |

## Codex config

`outputs.codex.config` sets the listed `.codex/config.toml` keys on each sync. It wins over a portable Settings spec `model`. In a trusted project, [project values take precedence over home values](https://learn.chatgpt.com/docs/config-file/config-basic). Keep user-level keys in your [home configuration](@/docs/global-configuration.md).

```yaml
outputs:
  codex:
    config:
      model: gpt-6-luna
      sandbox: workspace-write
      approval-policy: on-request
      model-reasoning-effort: high
      model-reasoning-summary: auto
      history-persistence: project
```

| Field | Type | Notes |
|-------|------|-------|
| `model` | string | Model identifier Codex uses for this project. |
| `sandbox` | string | Written as `sandbox_mode`: `read-only`, `workspace-write`, or `danger-full-access`. Any other value stops sync. |
| `approval-policy` | string | `on-request` for interactive approvals or `never` to reject approval prompts. `on-failure` is deprecated; `untrusted` is unsupported ([config reference](https://learn.chatgpt.com/docs/config-file/config-reference)). |
| `model-reasoning-effort` | string | Passed through unchanged. Use an effort the selected model and client advertise, such as `low`, `medium`, `high`, `xhigh`, `max`, or `ultra`. |
| `model-reasoning-summary` | string | Reasoning summary verbosity: `auto`, `concise`, `detailed`. |
| `history-persistence` | string | Conversation history scope: `project`, `global`, or `none`. |
| `notify` | string array | Not written. Codex ignores `notify` in a project config; set it in `~/.codex/config.toml`. |
| `profiles` | map | Not written. Codex 0.134.0 and later read no `[profiles.*]` table from any `config.toml`. Put each profile in `~/.codex/<name>.config.toml` and select it with `--profile <name>`. |
| `model-providers` | map | Not written. Codex ignores `[model_providers.*]` in a project config; set them in `~/.codex/config.toml`. |

Sync writes the overlay that `import codex` captured, `.agnostic-ai/overlays/codex.config.toml`, before the `[mcp_servers.*]` sections built from your specs. Captured configuration can be regenerated from that overlay, except for the project-ignored keys below.

### Keys Codex ignores in a project config

Codex drops these top-level keys from a project `.codex/config.toml`, with a startup warning ([docs](https://learn.chatgpt.com/docs/config-file/config-advanced), [source](https://github.com/openai/codex/blob/0b1b78a4f1694e2b9e393d385c7b82ca714ca08a/codex-rs/config/src/loader/mod.rs#L88-L101)): `openai_base_url`, `chatgpt_base_url`, `apps_mcp_product_sku`, `model_provider`, `model_providers`, `notify`, `profile`, `profiles`, `experimental_realtime_ws_base_url`, `otel`, `responses_api_metadata`, and `experimental_realtime_webrtc_call_base_url`.

Sync writes none of them. Keep user-level keys in `~/.codex/config.toml`; profiles use the separate `~/.codex/<name>.config.toml` files described above. The `notify`, `profiles`, and `model-providers` fields above print a note. When the overlay sets one, sync leaves it out of `.codex/config.toml`, keeps the overlay file, and prints a note naming the key. These notes never fail the sync, even with `on-unsupported: error`.

How sync resolves conflicts:

- `model` wins in this order, lowest first: portable Settings spec, `outputs.codex.config.model`, overlay. Either of the last two replaces the settings model, so a Claude model name in a settings spec raises no note.
- The overlay wins any other conflict with `outputs.codex.config.*`, and the lower value is dropped.
- On import, a top-level `model_reasoning_effort` moves to `effort` in `<settings>/codex.yaml` when no settings spec sets `effort`, so every target syncs it. `[profiles.*]` values, and a value another settings spec shadows, stay in the overlay.

### Codex exec-policies

`outputs.codex.exec-policies` (list) or `outputs.codex.exec-policies-file` (path to a YAML list) declares Codex's Starlark exec-policy rules for `.codex/rules/default.rules`. Each entry allows, forbids, or prompts for a shell command prefix.

```yaml
outputs:
  codex:
    exec-policies:
      - pattern: ["composer", "test"]
        decision: allow           # allow | forbidden | prompt
        justification: Composer scripts are project entrypoints.
        match: ["composer test", "composer test -- --filter Foo"]
      - pattern: ["rm", "-rf", "/"]
        decision: forbidden
        justification: Never remove the filesystem root.
```

| Field | Required | Notes |
|-------|----------|-------|
| `pattern` | yes | Shell command prefix tokens (`["composer", "test"]`). Becomes the `prefix_rule(pattern = [...])` argument. |
| `decision` | yes | One of `allow`, `forbidden`, `prompt`. |
| `justification` | no | Human-readable reason passed to Codex as `justification`. |
| `match` | no | Example command strings passed to Codex as `match`; Codex validates them when loading the policy. |

For many policies, use a file: `exec-policies-file: ./.agnostic-ai/codex.exec-policies.yaml`. Inline entries come first. [Codex applies the strictest matching decision](https://learn.chatgpt.com/docs/agent-configuration/rules) (`forbidden`, then `prompt`, then `allow`), so order never matters.

`import codex` captures every `prefix_rule(...)` in `.codex/rules/default.rules` into `.agnostic-ai/overlays/codex.exec-policies.yaml`, skipping a file sync generated. Sync uses that overlay when neither an inline list nor `exec-policies-file` is set.

### Translate Bash permissions

Set `outputs.codex.exec-policies-from-permissions: true` to generate command prefixes from portable Settings specs that target Codex and from `outputs.claude.settings.permissions`. The lists combine in source order, with duplicates removed. Hand-written Claude settings and user policy files are not read.

```yaml
targets: [claude, codex]
outputs:
  claude:
    settings:
      permissions:
        allow:
          - Bash(npm run check:*)
          - Bash(npx vitest run:*)
          - Bash(git diff:*)
  codex:
    exec-policies-from-permissions: true
```

This writes three `prefix_rule` entries. `Bash(a b c *)` and `Bash(a b c:*)` become `pattern = ["a", "b", "c"]`. `allow`, `deny`, and `ask` become `allow`, `forbidden`, and `prompt`.

Codex has no exact-match rule: a prefix matches extra arguments, even from a bare rule without `:*`. Claude Code allows `Bash(git push)` only as a bare `git push`, but a `git push` prefix rule would allow extra flags too. Project rules load only in a trusted project.

Sync leaves out an exact `allow` rule that Codex would widen, and names it with its source:

```text
note: codex: agnostic-ai.yaml: permissions.allow rule Bash(git push) is not written: Codex has no exact-match rule, and a `git push` prefix rule also allows extra arguments; write Bash(git push:*) to allow them, or use outputs.codex.exec-policies
```

The rule is written when a deny or ask rule on the same or a shorter prefix already decides every command it would match, or when a wildcard `allow` such as `Bash(git:*)` already allows the extra arguments. `on-unsupported: error` fails on a left-out rule; `silent` omits the note. Exact `deny` and `ask` rules are always written.

Only plain, unquoted words translate. A Bash rule with quotes, escapes, a `*` other than one trailing ` *` or `:*`, shell operators, expansions, assignments, or shell keywords gets a sync note. `on-unsupported: error` fails on it; `silent` omits it. Rules for other tools, such as `Read(.env)` or `WebFetch`, share one `permissions` sync note and never fail the sync. Use explicit `exec-policies` for a command that cannot translate.

An inline policy list (even `exec-policies: []`), `exec-policies-file` (even an empty file), or imported policy overlay wins. Sync skips translation, notes which source won, and never modifies the policy file.

`lint` warns with LINT021 when a supported Bash `allow` or `deny` rule, including declared portable deny and ask exclusions, has no native prefix that covers it with the same decision. Broader native prefixes count. Restrictive descendants of an allowed prefix also warn. It checks declared prefixes only. `lint --strict` fails on the warning.

## Import

`agnostic-ai import codex` finds `AGENTS.md` at any depth and reads the other Codex files:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` at the root | `.agnostic-ai/AGNOSTIC_AI.md`; the rules block `sync` appends becomes rules |
| `<dir>/AGENTS.md` (nested, hand-written) | one rule with the whole file, named after the scope: `api.md` for `services/api/`, or `services-api.md` when another scope also ends in `api`, with inferred `globs: <dir>/**` |
| `<dir>/AGENTS.md` (nested, with `import.codex.shred: true`, or written by `sync`) | one rule per section (`api-tests.md` for `## Tests` in `services/api/`); text above the first `##`, past the title, becomes one more rule named after the scope |
| `## Code Review Rules` or `## Review guidelines` in a nested `AGENTS.md`, or the review section `sync` writes to any `AGENTS.md` | `<reviews>/<scope-slug>.md` with `scope: <dir>` (`review.md` at the root), not a rule |
| `## Conventions` / `## Agents` / `## Skills` wrapper sections | unwrapped: their `### children` become the rules |
| Single-line italic (`_text_`) under a rule heading | moved into the rule's `description` |
| `.codex/agents/*.toml` and `.agents/agents/*.toml` | `<agents>/<name>.md`. When the agent spec already exists, as after `import claude`, the Codex `model` lands as `model: {codex: <name>}` (or a `codex` entry in an existing per-target map), so Claude Code keeps its own default. A differing shared scalar `model` gets `x-codex.model` |
| `.agents/skills/<name>/SKILL.md` (+ hand-written `agents/openai.yaml`, asset folders) | `<skills>/<name>/SKILL.md` (+ nested assets, exec bits preserved). Assets with the generated header are skipped; sync renders its own `agents/openai.yaml` from the skill fields |
| `.codex/hooks.json` and inline hooks in `.codex/config.toml` | `<hooks>/<event>-<hash8>.yaml` (one spec per handler). A duplicate hook in both files imports once; the JSON definition wins |
| `.codex/config.toml` `[mcp_servers.<name>]` | `<mcps>/<name>.yaml` |
| `.codex/config.toml` remaining keys (model, sandbox, approval_policy, `[history]`, ...) | `.agnostic-ai/overlays/codex.config.toml` (`hooks` + `mcp_servers` stripped). The [ignored keys](#keys-codex-ignores-in-a-project-config) stay in the overlay but are left out of `.codex/config.toml` on sync |
| `.codex/prompts/*.md` | `<commands>/<name>.md` (unchanged copy) |
| `.codex/environments/environment.toml` | `<environments>/codex.yaml`: `[setup]`, `[setup.win32]`, `[cleanup]`, and `[[actions]]` become `setup`, `setup-windows`, `cleanup`, and `dev-commands`. A file with a `[setup.darwin]` script, an action `platform`, or another key stays as written with a note |

Two sections with the same heading in one file get separate names (`style.md`, `style-2.md`). The walk skips hidden directories, the configured source directories, `node_modules/`, `vendor/`, directories git ignores, and directories with their own `.git`.

See [Codex config](#codex-config) for overlay precedence and project-ignored keys.


For enforcement boundaries, see [protected paths in Codex](@/docs/targets/codex.md#protected-paths).
