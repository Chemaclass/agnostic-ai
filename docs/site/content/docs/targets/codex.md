+++
title = "Codex"
description = "How agnostic-ai emits Codex configuration: native paths, capability limits, and output options."
weight = 20

[extra]
group = "Reference"
target_id = "codex"
+++

# Codex (`codex`)

```
AGENTS.md                                    # canonical entry-point pointer body (written by sync)
.codex/agents/<name>.toml                    # one TOML per agent (Codex CLI's native path)
.agents/skills/<name>/SKILL.md               # one folder per skill (the path Codex CLI scans)
.agents/skills/<name>/agents/openai.yaml     # optional, when x-codex provides UI/policy/deps
.codex/config.toml                           # when settings or MCP entries exist
.codex/hooks.json                            # when hook entries exist
.codex/rules/default.rules                   # opt-in, from outputs.codex.exec-policies
.codex/prompts/<name>.md                     # opt-in via outputs.codex.commands-dir (deprecated by Codex)
```

- **Rules**: unscoped rules inline into the root `AGENTS.md`. A rule with `scope: services/payments` instead reaches `services/payments/AGENTS.md`. A `globs` field alone does not create a directory scope. Remove legacy `outputs.codex.rules-file` overrides before using scoped rules. See [scoped context](@/docs/scoped-context.md) for selector and runtime limits.
- **Agents**: [Codex custom agents](https://learn.chatgpt.com/docs/agent-configuration/subagents) use one TOML file per agent with `name`, `description`, and `developer_instructions`, plus optional session config such as `model`, `model_reasoning_effort`, and `sandbox_mode`. A generic `tools: [Read, Bash, ...]` list is not emitted because Codex defines `tools` as a config table, not a tool allowlist. Sync reports the dropped field. Use `x-codex.tools` for native settings such as `web_search` and `view_image`. Use a per-target `model` map when another CLI's model name, such as `sonnet`, must not reach Codex.
- **Skills**: [Codex skills layout](https://learn.chatgpt.com/docs/build-skills), one folder per skill under `.agents/skills/` (the directory Codex scans from the cwd up to the repo root) with a required `SKILL.md` (frontmatter `name` + `description`, plus body). A scoped source skill moves the native directory under that scope: for example `skills/services/api/review/SKILL.md` becomes `services/api/.agents/skills/review/SKILL.md`. `import codex` restores the scope and bundled assets.

  When the spec carries `x-codex.interface`, `x-codex.policy`, or `x-codex.dependencies`, an `agents/openai.yaml` is also written for UI customization and policy declarations. Amp reads the root path, so identical emitted bytes dedupe and enabling both targets is safe. A stale managed tree at the pre-v0.43 `.codex/skills/` default is swept on sync.
- **Hooks**: land in `.codex/hooks.json` (override via `outputs.codex.hooks-file`). Hooks route by `event` frontmatter (`SessionStart`, `SubagentStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PreCompact`, `PostCompact`, `Stop`, `SubagentStop`, `SessionEnd`, `Interrupt`) into per-event arrays with `matcher` and `command`.

  Optional `timeout`, `statusMessage`, `commandWindows`, `additionalContextLimit`, and `async` pass through and survive `import codex`. `async` runs the hook in the background instead of blocking the session. An explicit `additionalContextLimit: 0` is preserved, since Codex uses it to pass complete hook context.

  `import codex` also reads hooks straight out of a hand-authored `.codex/config.toml`, in the vendor's own documented inline shape: `[[hooks.<event>]]` carries `matcher` alone, and a nested `[[hooks.<event>.hooks]]` array carries the command fields ([learn.chatgpt.com/docs/hooks](https://learn.chatgpt.com/docs/hooks.md), "Equivalent inline TOML in config.toml"). A flat `[[hooks.<event>]]` table with `matcher` and `command` on the same table also still decodes, for configs written before this shape was added. The vendor has never documented that flat form (#669).

  A hook spec with `type: mcp_tool` calls a tool on an already-connected MCP server instead of running a shell command. [learn.chatgpt.com/docs/hooks](https://learn.chatgpt.com/docs/hooks.md) says it "sends structured arguments directly to the tool and uses the same trust review and output contract as a command hook." `server` and `tool` are required; `input` (an argument-template object) is optional, and it shares `timeout`/`statusMessage` with the command shape. It emits as `{type, server, tool, input, timeout, statusMessage}` in the same `hooks.json`, and `import codex` reads it back from there (#693).
- **Exec policies**: opt-in. Set `outputs.codex.exec-policies` (inline list) or `outputs.codex.exec-policies-file` (external YAML) to write `.codex/rules/default.rules` in Codex's Starlark `prefix_rule(...)` form. Unset writes nothing.
- **MCP**: lands in `.codex/config.toml`. Servers emit as `[mcp_servers.<name>]`. Stdio servers use `command`/`args`/`env`/`cwd` plus the mixed `env_vars` array. HTTP/SSE servers use `url`/`bearer_token_env_var`/`http_headers`/`env_http_headers`/`auth` (`oauth` or `chatgpt`)/`http_headers_helper` (a local command printing header JSON, documented for a locally connected HTTP server only).

  A server name that TOML cannot carry as a bare key is quoted. Codex CLI 0.152.0 widened the accepted server-name charset to include `:`, `@`, `/`, and `.` for package-style names such as `npm:@modelcontextprotocol/server-sequential.thinking` (openai/codex#41700), and this adapter quotes those too so one such name does not invalidate the whole file (#706). Import stores a slash-bearing name in one percent-encoded YAML filename and keeps the exact name inside the spec, so import followed by sync is lossless (#711).

  These fields carry no transport restriction and emit on either shape: `enabled_tools`/`disabled_tools` ([learn.chatgpt.com/docs/config-file/config-reference](https://learn.chatgpt.com/docs/config-file/config-reference.md), #661), plus `required`, `startup_timeout_sec`, `startup_timeout_ms` (the vendor's own millisecond alias for the same startup timeout; set one or the other, #735), `tool_timeout_sec`, `default_tools_approval_mode`, and `experimental_environment`. `scopes`, `oauth_resource`, and an `[mcp_servers.<name>.oauth]` sub-table (`client_id`, `callback_url`, `callback_port`) authenticate to an MCP HTTP server and land on the http/sse shape alongside `auth` (#693).

  A `tools` map emits the vendor's per-tool sub-tables, `[mcp_servers.<name>.tools.<tool>]`, whose keys pass through verbatim. The vendor documents two today: `output_token_limit` ("Token budget for one MCP tool's output, before the standard 20% serialization allowance", shipped in Codex v0.153.0) and a per-tool approval override; the table gains entries without warning, so nothing is mapped key by key. A tool name that TOML cannot carry as a bare key is quoted.

  These sub-tables are written last in the server's table, because a TOML sub-table header ends its parent: a server-level scalar emitted after one would be read as a key of the tool. The whole block was dropped in silence before #678. All of these fields survive `import codex`. The project-tier config.toml is managed (overwritten each sync); put unmanaged Codex config in `~/.codex/config.toml`.
- **Settings**: the last portable `model` value writes to `.codex/config.toml`. `outputs.codex.config.model` wins over the portable value. A captured `.agnostic-ai/overlays/codex.config.toml` remains the highest-precedence layer for backward compatibility, so an imported `model` there wins over both and is never duplicated.
- **Commands**: not emitted by default. Codex loads custom prompts from `~/.codex/prompts/` only (no project-level discovery) and [deprecates them in favor of skills](https://learn.chatgpt.com/docs/custom-prompts), so a project-tier prompts tree would never be read; `sync` prints a coverage note instead and sweeps a stale managed `.codex/prompts/` tree. Set `outputs.codex.commands-dir` to emit the legacy layout anyway.
- **Import**: `import codex` captures `.codex/config.toml` minus `hooks` and `mcp_servers` into `.agnostic-ai/overlays/codex.config.toml`. `sync -t codex` prepends it before the spec-derived sections, so `model`, `sandbox`, `approval_policy`, `notify`, `[history]`, `[profiles.*]`, `[model_providers.*]`, and any other key survive a `.codex/` wipe. On conflict with `outputs.codex.config.*` the overlay wins and the first-class key is dropped to keep TOML valid. `import codex` also reads `.codex/prompts/*.md` and writes them byte-for-byte to `<commands>/`, so user-authored prompts round-trip.

Config keys:

| Key | Default | Notes |
|---|---|---|
| `outputs.codex.agents-dir` | `.codex/agents` | override to `.agents/agents` for the community shared layout |
| `outputs.codex.skills-dir` | `.agents/skills` | the path Codex scans |
| `outputs.codex.shared-subagents` | `true` | emits the per-skill tree at `skills-dir`; set `false` to skip codex skill emission |
| `outputs.codex.commands-dir` | unset | set to e.g. `.codex/prompts` to emit the deprecated project prompts layout |
| `outputs.codex.mcp-file` | `.codex/config.toml` | |
| `outputs.codex.hooks-file` | `.codex/hooks.json` | |
| `outputs.codex.rules-file` | unset | writes legacy concatenated rules and skips the pointer-body write |
| `outputs.codex.exec-policies` / `outputs.codex.exec-policies-file` | unset | write `.codex/rules/default.rules` |

## Codex config

The `outputs.codex.config` block declares first-class `.codex/config.toml` global keys, written into the project-tier config on each sync. A portable Settings spec can set the same project `model`; `outputs.codex.config.model` wins when both exist. Keys not listed here belong in the user-level `~/.codex/config.toml`, which Codex merges last.

```yaml
outputs:
  codex:
    config:
      model: o4-mini
      sandbox: workspace
      approval-policy: on-failure
      model-reasoning-effort: high
      model-reasoning-summary: auto
      history-persistence: project
      notify: ["python3", "/etc/codex/notify.py"]
      profiles:
        work:
          model: o4-mini
          sandbox: workspace-write
          approval-policy: on-failure
        oss:
          model: gpt-oss-20b
          model-provider: ollama
```

| Field | Type | Notes |
|-------|------|-------|
| `model` | string | Model identifier Codex uses for this project. |
| `sandbox` | string | Sandbox profile (e.g. `workspace`). |
| `approval-policy` | string | When Codex asks for approval: `never`, `on-failure`, or `always`. |
| `model-reasoning-effort` | string | Reasoning effort for o-series models: `low`, `medium`, `high`. |
| `model-reasoning-summary` | string | Reasoning summary verbosity: `auto`, `concise`, `detailed`. |
| `history-persistence` | string | Conversation history scope: `project`, `global`, or `none`. |
| `notify` | string array | External program Codex invokes on session events. First element is the executable; rest are arguments. |
| `profiles` | map | Named `[profiles.<name>]` blocks. Each entry overrides top-level fields when Codex runs with `--profile <name>`. Supported keys: `model`, `sandbox`, `approval-policy`, `model-reasoning-effort`, `model-reasoning-summary`, `model-provider`. |
| `model-providers` | map | Named `[model_providers.<id>]` blocks declaring backends Codex can call. Supported keys: `name`, `base-url`, `wire-api`, `api-key-env`, `env-key`. Reference an `id` from `profiles.<name>.model-provider`. |

### Codex exec-policies

`outputs.codex.exec-policies` (list) or `outputs.codex.exec-policies-file` (path to a YAML list) declares Codex CLI's Starlark exec-policy DSL, rendered into `.codex/rules/default.rules` on sync. Each entry allow- or forbid-lists a shell command prefix.

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
| `justification` | no | Free-form comment emitted above the rule as a `#` line. |
| `match` | no | Example matches rendered as commented `# match: ...` lines below the rule. Documentation only; Codex CLI ignores them. |

For many policies, keep them in a separate YAML file and point `exec-policies-file: ./.agnostic-ai/codex.exec-policies.yaml`. Inline entries render first, then file entries. Order matters: Codex evaluates rules top-down.

`agnostic-ai import codex` against a project that ships `.codex/rules/default.rules` captures every `prefix_rule(...)` call into `.agnostic-ai/overlays/codex.exec-policies.yaml`. The codex emitter auto-loads that overlay when no inline list and no explicit `exec-policies-file` is set, so the round-trip is byte-content-preserving without extra config.

The file is written only when at least one policy is declared. Otherwise nothing under `.codex/rules/` is created.

The codex emitter also reads `.agnostic-ai/overlays/codex.config.toml` (captured by `agnostic-ai import codex`) and prepends its body before the spec-derived `[mcp_servers.*]` sections. The overlay carries every other `.codex/config.toml` key the user has configured (`model`, `sandbox`, `approval_policy`, `notify`, `[history]`, `[profiles.*]`, `[model_providers.*]`, ...) so wiping `.codex/` between `import` and `sync` no longer drops them. For `model`, precedence from low to high is portable Settings spec, `outputs.codex.config.model`, captured overlay. The overlay also wins any other conflict with `outputs.codex.config.*`; the lower value is dropped to keep the TOML valid.

Verify with the real CLI:

1. Install: `npm install -g @openai/codex` ([quickstart](https://learn.chatgpt.com/docs/codex/cli)); `codex --version` to confirm PATH.
2. Check the tree: `agnostic-ai sync -t codex`, then `ls .codex/agents/ .agents/skills/`, `test -f .codex/config.toml && head -1 .codex/config.toml`, `test -f .codex/hooks.json && jq '.hooks | keys' .codex/hooks.json`. First line of config.toml must be the `# Generated by agnostic-ai` provenance comment.
3. Validate syntax: `toml-test .codex/config.toml` and `jq empty .codex/hooks.json` should both exit `0`.
4. `codex run "list one rule from this project"`. Codex picks up `AGENTS.md`, the agents, and skill folders. Look for `loaded N agents` / `loaded N skills`.
5. Trigger a hook by firing the targeted `event` (e.g. an `Edit` for a `PostToolUse` hook); the `command` appears in the hook log.
6. `codex mcp list` shows every `[mcp_servers.<name>]`. Disabled servers appear with the disabled flag.

The audit issue [#329](https://github.com/Chemaclass/agnostic-ai/issues/329) tracks this smoke checklist; close its "Real CLI smoke" box only after every step passes against the live Codex CLI build in the linked PR.
