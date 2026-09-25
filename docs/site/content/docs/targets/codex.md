+++
title = "Codex"
description = "How agnostic-ai emits Codex configuration: native paths, capability limits, and output options."
weight = 20

[extra]
group = "Reference"
target_id = "codex"
+++

# Codex (`codex`)

## Output

```
AGENTS.md                                    # entry-point pointer body (written by sync)
.codex/agents/<name>.toml                    # one TOML per agent
.agents/skills/<name>/SKILL.md               # one folder per skill (the path Codex scans)
.agents/skills/<name>/agents/openai.yaml     # when x-codex provides UI/policy/deps
.codex/config.toml                           # when settings or MCP entries exist
.codex/hooks.json                            # when hook entries exist
.codex/rules/default.rules                   # opt-in, from outputs.codex.exec-policies
.codex/prompts/<name>.md                     # opt-in via outputs.codex.commands-dir (deprecated by Codex)
```

- **Rules**: unscoped rules inline into the root `AGENTS.md`. A rule with `scope: services/payments` reaches `services/payments/AGENTS.md` instead. A `globs` field alone does not create a directory scope. Remove legacy `outputs.codex.rules-file` overrides before using scoped rules. See [scoped context](@/docs/scoped-context.md) for selector and runtime limits.
- **Agents**: [Codex custom agents](https://learn.chatgpt.com/docs/agent-configuration/subagents) are one TOML file each, with `name`, `description`, and `developer_instructions`. Session config such as `model`, `model_reasoning_effort`, `sandbox_mode`, `mcp_servers`, or any other `config.toml` key goes under `x-codex`. A generic `tools: [Read, Bash, ...]` list is dropped and reported, because Codex `tools` is a config table, not an allowlist. Use `x-codex.tools` for native settings such as `web_search` and `view_image`. Use a per-target `model` map to keep another CLI's model name, such as `sonnet`, out of Codex.

  The portable `effort` field writes `model_reasoning_effort` directly. Codex accepts any non-empty string: the documented names (`minimal`, `low`, `medium`, `high`, `xhigh`, `max`, `ultra`) parse, and anything else loads as a custom label ([config reference](https://learn.chatgpt.com/docs/config-file/config-reference.md)). So `effort: xhigh` and `effort: max` reach Codex unchanged (Factory rejects them). Only Qoder's integer effort budget is dropped, with a coverage note. An explicit `x-codex.model_reasoning_effort` wins.
- **Skills**: [Codex skills](https://learn.chatgpt.com/docs/build-skills) are one folder per skill under `.agents/skills/`, which Codex scans from the cwd up to the repo root. Each needs a `SKILL.md` with `name` and `description` frontmatter. A scoped skill moves under its scope: `skills/services/api/review/SKILL.md` becomes `services/api/.agents/skills/review/SKILL.md`. `import codex` restores the scope and bundled assets.

  When the spec sets `x-codex.interface`, `x-codex.policy`, or `x-codex.dependencies`, sync also writes `agents/openai.yaml` for UI and policy. Amp reads the same root path and the bytes are identical, so enabling both targets is safe. A stale managed tree at the old `.codex/skills/` default is swept.
- **Hooks**: land in `.codex/hooks.json` (override with `outputs.codex.hooks-file`), grouped by `event` (`SessionStart`, `SubagentStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PreCompact`, `PostCompact`, `Stop`, `SubagentStop`, `SessionEnd`, `Interrupt`) with `matcher` and `command`.

  Optional `timeout`, `statusMessage`, `commandWindows`, `additionalContextLimit`, and `async` pass through and survive `import codex`. `async` runs the hook in the background. An explicit `additionalContextLimit: 0` is kept, since Codex uses it to pass the full hook context.

  `import codex` also reads hooks from a hand-authored `.codex/config.toml` in the [documented inline shape](https://learn.chatgpt.com/docs/hooks.md): `[[hooks.<event>]]` holds `matcher`, and a nested `[[hooks.<event>.hooks]]` holds the command fields. The older, undocumented flat table with `matcher` and `command` together still decodes.

  A hook with `type: mcp_tool` calls a tool on a connected MCP server instead of a shell command, with the same trust review and output contract as a command hook ([hooks docs](https://learn.chatgpt.com/docs/hooks.md)). `server` and `tool` are required, `input` (an argument template) is optional, and `timeout`/`statusMessage` apply. It emits as `{type, server, tool, input, timeout, statusMessage}` in `hooks.json` and imports back.
- **Exec policies**: opt-in. Set `outputs.codex.exec-policies` (inline list) or `outputs.codex.exec-policies-file` (external YAML) to write `.codex/rules/default.rules` as Starlark `prefix_rule(...)` calls. Portable `permissions` lists do not feed this, because `prefix_rule` matches token lists, not globs. They raise a coverage note pointing here.
- **MCP**: lands in `.codex/config.toml` as `[mcp_servers.<name>]`. Stdio servers use `command`/`args`/`env`/`cwd` plus `env_vars`, whose entries are names or `{name, source}` objects with `source` set to `local` or `remote`. HTTP/SSE servers use `url`/`bearer_token_env_var`/`http_headers`/`env_http_headers`/`auth` (`oauth` or `chatgpt`)/`http_headers_helper` (a local command printing header JSON, documented for local HTTP servers only). `disabled: true` writes `enabled = false`.

  Server names that are not bare TOML keys are quoted, including package-style names with `:`, `@`, `/`, or `.` (accepted since Codex CLI 0.152.0), such as `npm:@modelcontextprotocol/server-sequential.thinking`. Import stores a slash-bearing name in a percent-encoded YAML filename and keeps the exact name in the spec, so import then sync is lossless.

  These fields emit on any transport: `enabled_tools`/`disabled_tools` ([config reference](https://learn.chatgpt.com/docs/config-file/config-reference.md); `disabled_tools` applies after `enabled_tools`), `required`, `startup_timeout_sec` or its millisecond alias `startup_timeout_ms` (set one), `tool_timeout_sec`, `default_tools_approval_mode`, and `experimental_environment`.

  | Field | Default | Meaning |
  |-------|---------|---------|
  | `required` | `false` | Fail startup or resume if this enabled server cannot initialize. |
  | `startup_timeout_sec` / `startup_timeout_ms` | `10` / `10000` | Server startup timeout. |
  | `tool_timeout_sec` | `60` | Per-tool execution timeout. |
  | `default_tools_approval_mode` | unset | `auto`, `prompt`, `writes`, or `approve`, unless a per-tool override exists. |
  | `experimental_environment` | unset | `local` or `remote`. `remote` starts a stdio server through a remote executor; HTTP remote placement is not implemented yet. |

  `scopes`, `oauth_resource` (the RFC 8707 resource parameter), and an `[mcp_servers.<id>.oauth]` sub-table `{client_id, callback_url, callback_port}` land on the HTTP/SSE shape next to `auth`.

  A `tools` map emits per-tool sub-tables, `[mcp_servers.<name>.tools.<tool>]`, with keys passed through verbatim. Codex documents `output_token_limit` (token budget for one tool's output, since v0.153.0) and a per-tool approval override. Tool names that are not bare TOML keys are quoted. These sub-tables are written last, so later server scalars are not read as tool keys. All of these fields survive `import codex`. The project `config.toml` is overwritten each sync; put unmanaged Codex config in `~/.codex/config.toml`.
- **Settings**: the last portable `model` writes to `.codex/config.toml`, below `outputs.codex.config.model`. A portable `effort` writes `model_reasoning_effort` the same way, below `outputs.codex.config.model-reasoning-effort`. Any string passes, since available levels depend on the model and client. A captured `.agnostic-ai/overlays/codex.config.toml` has the highest precedence, so an imported `model` there wins and is never duplicated. Codex has no `x-<target>` settings passthrough: an `x-codex` block on a settings spec raises a coverage note naming the overlay and `outputs.codex.config` instead.
- **Commands**: not emitted by default. Codex reads custom prompts only from `~/.codex/prompts/` and [deprecates them in favor of skills](https://learn.chatgpt.com/docs/custom-prompts). `sync` prints a coverage note and sweeps a stale managed `.codex/prompts/` tree. Set `outputs.codex.commands-dir` to emit the legacy layout anyway.

## Config keys

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

The `outputs.codex.config` block sets first-class `.codex/config.toml` keys, written to the project config on each sync. It wins over a portable Settings spec `model`. Keys not listed here belong in `~/.codex/config.toml`, which Codex merges last.

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

Sync also reads `.agnostic-ai/overlays/codex.config.toml` (captured by `import codex`) and writes it before the spec-derived `[mcp_servers.*]` sections. The overlay keeps every other `.codex/config.toml` key (`model`, `sandbox`, `approval_policy`, `notify`, `[history]`, `[profiles.*]`, `[model_providers.*]`, ...), so wiping `.codex/` between import and sync loses nothing.

- `model` precedence, low to high: portable Settings spec, `outputs.codex.config.model`, overlay.
- The overlay wins any other conflict with `outputs.codex.config.*`. The lower value is dropped to keep the TOML valid.
- On import, a top-level `model_reasoning_effort` moves to `effort` in `<settings>/codex.yaml` when no settings spec sets `effort`, so every target syncs it. `[profiles.*]` values, and a value another settings spec shadows, stay in the overlay.

### Codex exec-policies

`outputs.codex.exec-policies` (list) or `outputs.codex.exec-policies-file` (path to a YAML list) declares Codex's Starlark exec-policy rules, rendered into `.codex/rules/default.rules`. Each entry allows, forbids, or prompts for a shell command prefix.

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

For many policies, use a separate file: `exec-policies-file: ./.agnostic-ai/codex.exec-policies.yaml`. Inline entries render first, then file entries. Order matters: Codex evaluates rules top-down.

`import codex` captures every `prefix_rule(...)` in `.codex/rules/default.rules` into `.agnostic-ai/overlays/codex.exec-policies.yaml`. Sync loads that overlay when neither an inline list nor `exec-policies-file` is set, so the round-trip preserves content with no extra config.

## Import

`agnostic-ai import codex` finds `AGENTS.md` at any depth and reads the rest of the Codex tree:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` (split on `## headings`) | `<rules>/<slug>.md` per section |
| `AGENTS.md` (no headings) | single `<rules>/<projectname>.md` |
| `<dir>/AGENTS.md` (nested) | `<rules>/<slug>.md` with inferred `globs: <dir>/**` |
| `## Conventions` / `## Agents` / `## Skills` wrapper sections | unwrapped: their `### children` become the rules |
| Single-line italic (`_text_`) immediately under a rule heading | extracted into the rule's `description` (and removed from the body) |
| `.codex/agents/*.toml` and `.agents/agents/*.toml` | `<agents>/<name>.md` |
| `.agents/skills/<name>/SKILL.md` (+ `agents/openai.yaml`, asset folders) | `<skills>/<name>/SKILL.md` (+ nested assets, exec bits preserved) |
| `.codex/config.toml` `[[hooks.<event>]]` | `<hooks>/<event>-<hash8>.yaml` (one spec per entry) |
| `.codex/config.toml` `[mcp_servers.<name>]` | `<mcps>/<name>.yaml` |
| `.codex/config.toml` remaining keys (model, sandbox, approval_policy, notify, `[history]`, `[profiles.*]`, `[model_providers.*]`, …) | `.agnostic-ai/overlays/codex.config.toml` (`hooks` + `mcp_servers` stripped) |
| `.codex/prompts/*.md` | `<commands>/<name>.md` (byte-identical copy, so user-authored prompts round-trip) |

Slug collisions are deduplicated (`style.md`, `style-2.md`). The walk skips hidden directories, the configured source directories, `node_modules/`, and `vendor/`.

`sync -t codex` writes the overlay back, so every captured key survives a `.codex/` wipe. See [Codex config](#codex-config) for conflicts when both sides set a key. The [exec-policies overlay](#codex-exec-policies) works the same way.

## Verify

1. Install: `npm install -g @openai/codex` ([quickstart](https://learn.chatgpt.com/docs/codex/cli)), then `codex --version`.
2. Run `agnostic-ai sync -t codex`, then `ls .codex/agents/ .agents/skills/`, `head -1 .codex/config.toml`, and `jq '.hooks | keys' .codex/hooks.json`. The first line of `config.toml` must be the `# Generated by agnostic-ai` comment.
3. `toml-test .codex/config.toml` and `jq empty .codex/hooks.json` both exit `0`.
4. `codex run "list one rule from this project"` loads `AGENTS.md`, agents, and skills. Look for `loaded N agents` / `loaded N skills`.
5. Fire a hook's `event` (e.g. an `Edit` for a `PostToolUse` hook). The `command` appears in the hook log.
6. `codex mcp list` shows every `[mcp_servers.<name>]`, with disabled servers flagged.
