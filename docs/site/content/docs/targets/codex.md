+++
title = "Codex"
description = "How agnostic-ai emits Codex configuration: native paths, capability limits, and output options."
weight = 20

[extra]
group = "Reference"
target_id = "codex"
+++

# Codex (`codex`)

agnostic-ai writes [Codex](https://learn.chatgpt.com/docs/codex/cli) instructions to `AGENTS.md`, plus agents, skills, hooks, MCP servers, and settings.

## Output

```
AGENTS.md                                    # entry-point pointer body (written by sync)
.codex/agents/<name>.toml                    # one TOML per agent
.agents/skills/<name>/SKILL.md               # one folder per skill (the path Codex scans)
.agents/skills/<name>/agents/openai.yaml     # for x-codex UI/policy/deps or a manual-only skill
.codex/config.toml                           # when settings or MCP entries exist
.codex/hooks.json                            # when hook entries exist
.codex/environments/environment.toml         # when an environment spec sets setup, cleanup, or dev-commands
.codex/rules/default.rules                   # opt-in, from outputs.codex.exec-policies
.codex/prompts/<name>.md                     # opt-in via outputs.codex.commands-dir (deprecated by Codex)
```

- **Rules**: project-wide rules go inline into the root `AGENTS.md`. An unscoped rule with whole-subtree `globs` or `paths`, such as `[src/app/api/**, prisma/**]`, writes one nested `AGENTS.md` per directory instead. `dir/**/*` also counts as a whole subtree. `alwaysApply: true` keeps an unscoped rule at the root. Root files, filename filters such as `src/api/**/*.ts`, and mixed selectors stay inline with a note naming the rule as always loaded. With `on-unsupported: error`, they fail.
- **Scoped rules**: `scope: services/payments` writes to `services/payments/AGENTS.md`. Adding `globs: tests/payments/**` also writes the rule into `tests/payments/AGENTS.md`. Directory documents cannot hold external file filters or root selectors with scope, so those follow `on-unsupported`. Remove legacy `outputs.codex.rules-file` overrides before you use explicit scopes.
- **Rule loading**: Codex reads `AGENTS.md` from the session's working directory and its ancestors. Start Codex in a subtree to load its nested `AGENTS.md`. A session started at the root does not load nested files when it later edits there.

  {% <details summary="When nested placement falls back"> %}
  Automatic glob placement falls back inline with a note when output overrides, an unmanaged destination, or another root reader cannot keep nested delivery. A one-off target picked on the command line counts in that reader check. Compatible configured `AGENTS.md` readers share the same nested files. Incompatible target conditions or bodies fail before any write. User-owned files and alternate instruction files keep the [scoped context safeguards](@/docs/scoped-context.md#shared-files-and-safe-updates).
  {% </details> %}
- **Agents**: [Codex custom agents](https://learn.chatgpt.com/docs/agent-configuration/subagents) hold `name`, `description`, and `developer_instructions`. Any `config.toml` key, such as `model`, `model_reasoning_effort`, or `mcp_servers`, goes under `x-codex`. Sync drops and reports a generic `tools: [Read, Bash, ...]` list, because Codex `tools` is a config table, not an allowlist. Use `x-codex.tools` for native settings such as `web_search` and `view_image`.
- **Agent model**: a per-target `model` map keeps another CLI's model name out of Codex. If a shared `model` holds a Claude model name (an alias such as `opus` or `fable`, `inherit`, or a `claude-*` id), sync leaves it out of the Codex file. A coverage note names `model: {claude: <name>}`, and `on-unsupported: error` fails the sync. A [model tier](@/docs/spec-format/agents.md#model-tiers) with a `codex` entry sets the Codex model for every agent that names it. The aliases `sol`, `luna`, `astra`, and `terra` resolve to the current ids ([models](@/docs/configuration.md#models)).
- **Agent effort**: `effort` writes `model_reasoning_effort` directly. Codex parses the documented names (`minimal`, `low`, `medium`, `high`, `xhigh`, `max`, `ultra`) and loads any other non-empty string as a custom label ([config reference](https://learn.chatgpt.com/docs/config-file/config-reference.md)). So `effort: xhigh` and `effort: max` reach Codex unchanged (Factory rejects them). Only Qoder's integer effort budget is dropped, with a coverage note. An explicit `x-codex.model_reasoning_effort` wins.
- **Agent sandbox**: a Codex agent inherits the session sandbox. Since [openai/codex#39299](https://github.com/openai/codex/pull/39299) (rust-v0.155.0), Codex silently ignores `sandbox_mode` in an agent file, so `readonly: true` does not make the agent read-only. Sync still writes `sandbox_mode = "read-only"` for older releases, unless `x-codex.sandbox_mode` overrides it (`null` omits the key). One coverage note counts those agents, in project and global sync. Only session-level limits are enforced: `sandbox_mode` in `config.toml` and [exec policies](#codex-exec-policies).
- **Skills**: Codex scans [skill folders](https://learn.chatgpt.com/docs/build-skills) under `.agents/skills/` from the cwd up to the repo root. Each `SKILL.md` needs `name` and `description`. A scoped skill moves under its scope: `skills/services/api/review/SKILL.md` becomes `services/api/.agents/skills/review/SKILL.md`. `import codex` restores the scope and bundled assets without generated provenance headers, so a round trip keeps the canonical instructions and native edits. Codex reads Claude Code's `` !`command` `` lines and `$ARGUMENTS` as plain text, so sync notes each one ([Claude Code body syntax](@/docs/spec-format/skills.md#claude-code-body-syntax)).

  Sync writes `agents/openai.yaml` when the spec sets `x-codex.interface`, `x-codex.policy`, `x-codex.dependencies`, or `disable-model-invocation: true`. A bundled `agents/openai.yaml` is the base, with `x-codex` keys layered over it. Every target that writes `.agents/skills/` or the `outputs.codex.skills-dir` folder writes the merged file. Other skill trees, such as `.cursor/skills/`, keep the bundled file verbatim. Amp reads the same root path with identical bytes, so enabling both is safe. Sync sweeps a stale managed tree at the old `.codex/skills/` default.
- **Skill model and effort**: Codex skills have no `model` or `effort` field. Sync omits both, with a coverage note when a value resolves for Codex. Global `~/.agents/skills/` also omits target overrides, because several tools read it. Global sync writes `agents/openai.yaml` too. Codex reads no `disable-model-invocation`, so it becomes `policy.allow_implicit_invocation: false`, in project and global sync. An explicit `allow_implicit_invocation` in `x-codex.policy` or a bundled `agents/openai.yaml` wins, so `true` keeps the skill implicit.
- **Hooks**: grouped by `event` (`SessionStart`, `SubagentStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PreCompact`, `PostCompact`, `Stop`, `SubagentStop`, `SessionEnd`, `Interrupt`) with `matcher` and `command`. Optional `timeout`, `statusMessage`, `commandWindows`, `additionalContextLimit`, and `async` (run in the background) pass through and survive `import codex`. An explicit `additionalContextLimit: 0` is kept, since Codex uses it to pass the full hook context.
- **Hook trust**: Codex runs a new command or MCP tool hook from `sync` or `sync --global` only after you trust it with `/hooks`, and again after any change to its handler, matcher, or other effective field. Project hooks also need a trusted project. Codex keeps trust under `hooks.state` in the user `config.toml`, which project config cannot grant ([hooks docs](https://learn.chatgpt.com/docs/hooks)).

  Sync names untrusted, modified, and disabled handlers and points to `/hooks`. `agnostic-ai doctor -t codex` checks the project hooks file and user `hooks.json`: it fails on untrusted or modified handlers or unreadable trust state, and only reports disabled ones. `doctor --json` adds a `hook_trust` list. The checks read persisted trust from `CODEX_HOME`, or `~/.codex`, so they miss a session trust bypass. Sync and `doctor --fix` never grant trust or change a handler's enabled state.
- **Hook commands**: each `command` starts with `export AGNOSTIC_AI_TARGET=codex; ` so a shared script knows Codex ran it ([which target ran a hook](@/docs/spec-format/hooks.md#hook-target)). The prefix needs a POSIX session shell; a macOS or Linux login shell of `pwsh` or `nu` breaks it. On Windows, Codex runs `commandWindows` through PowerShell or cmd, and sync fills it with the declared command, without the variable, when the spec sets none. `import codex` strips both. `$CLAUDE_PROJECT_DIR` in `commandWindows` becomes the same Git root path as in `command`.

  Codex hooks have no exec form, so `args` fold into `command` after the prefix, each in POSIX single quotes (`bash 'guard.sh'`). A command holding a space or another shell character folds the same way. Without `commandWindows`, Windows runs the folded command. PowerShell reads it as intended when no arg holds an apostrophe; cmd does not. In those cases, set `commandWindows`. `import codex` reads it back as one shell-form `command`.
- **MCP tool hooks**: `type: mcp_tool` calls a tool on a connected MCP server instead of a shell command, with the same trust review and output contract ([hooks docs](https://learn.chatgpt.com/docs/hooks.md)). `server` and `tool` are required, `input` (an argument template) is optional, and `timeout`/`statusMessage` apply. It emits as `{type, server, tool, input, timeout, statusMessage}` in `hooks.json` and imports back.
- **Edit hooks**: Codex accepts `Edit` and `Write` as matcher aliases for `apply_patch`, but reports `tool_name: "apply_patch"` and puts the patch in `tool_input.command`, with no `tool_input.file_path` ([hooks docs](https://learn.chatgpt.com/docs/hooks)). A `PreToolUse`, `PostToolUse`, or `PermissionRequest` edit hook whose `command` reads `tool_input.file_path` gets an empty value. `sync` prints a note, or fails with `on-unsupported: error`. Read edited paths with [`agnostic-ai hook paths`](@/docs/spec-format/hooks.md#edited-paths), which parses the patch, or add `target: claude` to the hook.

  {% <details summary="What the edit-hook check reads"> %}
  The check reads the `command` text and the selected managed script body under `.agnostic-ai/scripts/` that sync copies to Codex. Target-specific script overrides take precedence. User-owned scripts and arbitrary command paths are not read.
  {% </details> %}

  {% <details summary="Imported project-root paths"> %}
  Imported shell-form `$CLAUDE_PROJECT_DIR` and `${CLAUDE_PROJECT_DIR}` paths resolve through the Git worktree root and the configured project's relative path, so hooks run from a subdirectory. This needs a POSIX shell and Git. Unsupported root syntax or a project outside Git gets a named note, or fails with `on-unsupported: error`. See [project-root paths](@/docs/spec-format/hooks.md#imported-project-root-paths).
  {% </details> %}

  {% <details summary="Hooks inside config.toml"> %}
  `import codex` also reads hooks from a hand-authored `.codex/config.toml` in the [documented inline shape](https://learn.chatgpt.com/docs/hooks.md): `[[hooks.<event>]]` holds `matcher`, and a nested `[[hooks.<event>.hooks]]` holds the command fields. The older, undocumented flat table with `matcher` and `command` together still decodes.
  {% </details> %}
- **Reviews**: review specs land in `## Code Review Rules`. [Codex code review](https://learn.chatgpt.com/docs/third-party/github) reads it from the root `AGENTS.md` and the `AGENTS.md` nearest each changed file. An unscoped spec goes to the root. `scope: services/api` goes to `services/api/AGENTS.md`, after that scope's rules. Specs sharing a scope concatenate into the same text Cursor writes to `BUGBOT.md`. Other `AGENTS.md` readers load the section too. Set `targets:` on a spec to keep it out. With `outputs.codex.rules-file` set, sync skips the root `AGENTS.md`, so unscoped reviews get a coverage note.
- **Exec policies**: opt-in, from [declared policies](#codex-exec-policies) or [translated Bash permissions](#translate-bash-permissions). Until a source (inline, file, or the captured overlay) holds a rule, portable `permissions` lists raise a coverage note naming both routes.
- **Environment**: environment specs write the [local environment](https://learn.chatgpt.com/docs/environments/local-environment) the Codex app reads.
  - `setup`, `cleanup`, and `setup-windows` become the `[setup]`, `[cleanup]`, and `[setup.win32]` scripts (a list runs one command per line). `[setup]` is always written, empty if needed, because the app's own file always carries it.
  - Each `dev-commands` entry becomes an `[[actions]]` button with its `name`, `command` (a list joins into one shell line), and an `icon` that defaults to `run`. Actions run from the project root, so a `cwd` becomes `cd <cwd> &&` before the command, which `import codex` reads back as `cwd`.
  - Specs merge by top-level key, the last value wins, and `name` is the environment's name.
  - `port`, `auto-port`, `env`, `url`, `install`, and `terminals` have no Codex key and get a no-effect note.
  - The docs page does not show the file, so the layout follows what the app writes.
- **MCP**: `[mcp_servers.<name>]` tables. `disabled: true` writes `enabled = false`.
  - Stdio: `command`/`args`/`env`/`cwd` plus `env_vars`, whose entries are names or `{name, source}` objects with `source` set to `local` or `remote`.
  - HTTP/SSE: `url`/`bearer_token_env_var`/`http_headers`/`env_http_headers`/`auth` (`oauth` or `chatgpt`)/`http_headers_helper` (a local command printing header JSON, documented for local HTTP servers only).
  - A `${NAME}` reference in `env` or `headers` becomes `env_vars`, `bearer_token_env_var`, or `env_http_headers`, since Codex forwards variables by name. Codex documents no reference in `url` or `args`, so a server with one there is left out with a note. See [environment references](@/docs/spec-format/mcps.md#environment-references).
  - Any transport: `enabled_tools`/`disabled_tools` ([config reference](https://learn.chatgpt.com/docs/config-file/config-reference.md); `disabled_tools` applies after `enabled_tools`) and the fields below. Set only one of `startup_timeout_sec` and its millisecond alias `startup_timeout_ms`.

  | Field | Default | Meaning |
  |-------|---------|---------|
  | `required` | `false` | Fail startup or resume if this enabled server cannot initialize. |
  | `startup_timeout_sec` / `startup_timeout_ms` | `10` / `10000` | Server startup timeout. |
  | `tool_timeout_sec` | `60` | Per-tool execution timeout. |
  | `default_tools_approval_mode` | unset | `auto`, `prompt`, `writes`, or `approve`, unless a per-tool override exists. |
  | `experimental_environment` | unset | `local` or `remote`. `remote` starts a stdio server through a remote executor; HTTP remote placement is not implemented yet. |

  `scopes`, `oauth_resource` (the RFC 8707 resource parameter), and an `[mcp_servers.<id>.oauth]` sub-table `{client_id, callback_url, callback_port}` land on the HTTP/SSE shape next to `auth`.

  A `tools` map emits per-tool sub-tables, `[mcp_servers.<name>.tools.<tool>]`, with keys passed through verbatim. Codex documents `output_token_limit` (token budget for one tool's output, since v0.153.0) and a per-tool approval override. Sync quotes tool names that are not bare TOML keys and writes these sub-tables last, so later server scalars are not read as tool keys. All of these fields survive `import codex`.

  Each sync overwrites the project `config.toml`. Put unmanaged Codex config in `~/.codex/config.toml`.

  {% <details summary="Server names with special characters"> %}
  Server names that are not bare TOML keys are quoted, including package-style names with `:`, `@`, `/`, or `.` (accepted since Codex CLI 0.152.0), such as `npm:@modelcontextprotocol/server-sequential.thinking`. Import stores a slash-bearing name in a percent-encoded YAML filename and keeps the exact name in the spec, so import then sync is lossless.
  {% </details> %}
- **Settings**: the last portable `model` and `effort` write `model` and `model_reasoning_effort`. They rank below `outputs.codex.config.model` or `outputs.codex.config.model-reasoning-effort` and the captured `.agnostic-ai/overlays/codex.config.toml` ([precedence](#codex-config)), so an imported `model` is never duplicated. Any effort string passes, since available levels depend on the model and client. Codex has no `x-<target>` settings passthrough: an `x-codex` block on a settings spec raises a coverage note naming the overlay and `outputs.codex.config`.
- **Commands**: off by default. Codex reads custom prompts only from `~/.codex/prompts/` and [deprecates them in favor of skills](https://learn.chatgpt.com/docs/custom-prompts). `sync` prints a coverage note and sweeps a stale managed `.codex/prompts/` tree.

## Config keys

| Key | Default | Notes |
|---|---|---|
| `outputs.codex.agents-dir` | `.codex/agents` | override to `.agents/agents` for the community shared layout |
| `outputs.codex.skills-dir` | `.agents/skills` | the path Codex scans |
| `outputs.codex.nested-glob-rules` | `true` | place exact whole-subtree rules in nested `AGENTS.md`; `false` keeps root inlining |
| `outputs.codex.shared-subagents` | `true` | emits the per-skill tree at `skills-dir`; set `false` to skip codex skill emission |
| `outputs.codex.commands-dir` | unset | set to e.g. `.codex/prompts` to emit the deprecated project prompts layout |
| `outputs.codex.mcp-file` | `.codex/config.toml` | |
| `outputs.codex.hooks-file` | `.codex/hooks.json` | |
| `outputs.codex.environment-file` | `.codex/environments/environment.toml` | |
| `outputs.codex.rules-file` | unset | writes legacy concatenated rules and skips the pointer-body write |
| `outputs.codex.exec-policies` / `outputs.codex.exec-policies-file` | unset | write `.codex/rules/default.rules` |
| `outputs.codex.exec-policies-from-permissions` | `false` | translate simple Bash permission rules when no native policy source is set |

## Codex config

`outputs.codex.config` sets the listed `.codex/config.toml` keys on each sync. It wins over a portable Settings spec `model`. Put unlisted keys in `~/.codex/config.toml`, which Codex merges last.

```yaml
outputs:
  codex:
    config:
      model: gpt-6-luna
      sandbox: workspace
      approval-policy: on-request
      model-reasoning-effort: high
      model-reasoning-summary: auto
      history-persistence: project
```

| Field | Type | Notes |
|-------|------|-------|
| `model` | string | Model identifier Codex uses for this project. |
| `sandbox` | string | Sandbox profile (e.g. `workspace`). |
| `approval-policy` | string | `on-request` for interactive approvals or `never` to reject approval prompts. `on-failure` is deprecated; `untrusted` is unsupported ([config reference](https://learn.chatgpt.com/docs/config-file/config-reference)). |
| `model-reasoning-effort` | string | Passed through unchanged. Use an effort the selected model and client advertise, such as `low`, `medium`, `high`, `xhigh`, `max`, or `ultra` ([config reference](https://learn.chatgpt.com/docs/config-file/config-reference)). |
| `model-reasoning-summary` | string | Reasoning summary verbosity: `auto`, `concise`, `detailed`. |
| `history-persistence` | string | Conversation history scope: `project`, `global`, or `none`. |
| `notify` | string array | Not written. Codex ignores `notify` in a project config; set it in `~/.codex/config.toml`. |
| `profiles` | map | Not written. Codex ignores `[profiles.*]` in a project config, and 0.134.0 and later read no `[profiles.*]` table from any `config.toml`. Put each profile in `~/.codex/<name>.config.toml` and select it with `--profile <name>`. |
| `model-providers` | map | Not written. Codex ignores `[model_providers.*]` in a project config; set them in `~/.codex/config.toml`. |

Sync writes the overlay that `import codex` captured, `.agnostic-ai/overlays/codex.config.toml`, before the spec-derived `[mcp_servers.*]` sections. It keeps every other `.codex/config.toml` key (`model`, `sandbox`, `approval_policy`, `[history]`, `[tui]`, ...), so wiping `.codex/` between import and sync loses nothing.

### Keys Codex ignores in a project config

Codex drops these top-level keys from a project `.codex/config.toml`, with a startup warning for each ([config-advanced docs](https://learn.chatgpt.com/docs/config-file/config-advanced)): `openai_base_url`, `chatgpt_base_url`, `apps_mcp_product_sku`, `model_provider`, `model_providers`, `notify`, `profile`, `profiles`, `experimental_realtime_ws_base_url`, and `otel`. The Codex source list also has `responses_api_metadata` and `experimental_realtime_webrtc_call_base_url` ([`PROJECT_LOCAL_CONFIG_DENYLIST`](https://github.com/openai/codex/blob/0b1b78a4f1694e2b9e393d385c7b82ca714ca08a/codex-rs/config/src/loader/mod.rs#L88-L101)).

Sync writes none of them. Set them in `~/.codex/config.toml`.

- The `notify`, `profiles`, and `model-providers` fields above print a note.
- When the overlay sets one, sync leaves it out of `.codex/config.toml`, keeps the overlay file as it is, and prints a note naming the key.
- These notes never fail the sync, even with `on-unsupported: error`, which covers only spec kinds a target cannot write.
- Since Codex 0.134.0, `--profile <name>` reads `~/.codex/<name>.config.toml`, and neither `[profiles.<name>]` nor the top-level `profile` selector works in any `config.toml`.

How sync resolves conflicts:

- `model` precedence, low to high: portable Settings spec, `outputs.codex.config.model`, overlay. Either of the last two replaces the settings model, so a Claude model name in a settings spec raises no note; agent model notes still apply. A `[profiles.*]` model does not count, since sync leaves profiles out.
- The overlay wins any other conflict with `outputs.codex.config.*`, and the lower value is dropped to keep the TOML valid.
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

For many policies, use a file: `exec-policies-file: ./.agnostic-ai/codex.exec-policies.yaml`. Inline entries render first. [Codex applies the strictest matching decision](https://learn.chatgpt.com/docs/agent-configuration/rules) (`forbidden`, then `prompt`, then `allow`), so order never overrides a restriction.

`import codex` captures every `prefix_rule(...)` in `.codex/rules/default.rules` into `.agnostic-ai/overlays/codex.exec-policies.yaml`, skipping a file sync generated. Sync loads that overlay when neither an inline list nor `exec-policies-file` is set, so the round trip needs no extra config.

### Translate Bash permissions

Set `outputs.codex.exec-policies-from-permissions: true` to generate command prefixes from portable Settings specs that target Codex and from `outputs.claude.settings.permissions`. The lists combine in source order with duplicates removed per list, as Claude combines them. Hand-written Claude settings and user policy files are not read.

```yaml
targets: [claude, codex]
outputs:
  claude:
    settings:
      permissions:
        allow:
          - Bash(npm run check)
          - Bash(npx vitest run:*)
          - Bash(git diff:*)
  codex:
    exec-policies-from-permissions: true
```

This writes three `prefix_rule` entries. `Bash(a b c)`, `Bash(a b c *)`, and `Bash(a b c:*)` all become `pattern = ["a", "b", "c"]`. `allow`, `deny`, and `ask` become `allow`, `forbidden`, and `prompt`.

Translation is opt-in because Codex has no exact-match rule. A prefix matches extra arguments, even from a bare rule without `:*`. Claude Code allows `Bash(git push)` only as a bare `git push`, but Codex also allows `git push --force origin main`. Translation covers a subset of command prefixes, not exact Claude permission equivalence.

Codex rules govern requests to run outside the sandbox. An `allow` match runs the command without asking, outside the sandbox when every segment matches an `allow` rule. Project rules load only in a trusted project config layer.

Sync names each exact `allow` rule that Codex widens, with its source:

```text
note: codex: agnostic-ai.yaml: permissions.allow rule Bash(git push) becomes a Codex prefix rule, so Codex also allows `git push` with extra arguments; add a deny or ask rule for arguments that need review
```

A deny or ask rule on the same or a shorter prefix silences the note, as does a wildcard `allow` such as `Bash(git:*)` that already allows the extra arguments in Claude Code. `on-unsupported: error` does not fail on it; `silent` omits it. Exact `deny` and `ask` rules only get stricter as a prefix, so they raise no note.

Only plain, unquoted words translate. A Bash rule with quotes, escapes, a `*` other than one trailing ` *` or `:*`, shell operators, expansions, assignments, or shell keywords gets a coverage note naming the rule and source. `on-unsupported: error` fails on it; `silent` omits it. Rules for other tools, such as `Read(.env)` or `WebFetch`, share one `permissions` coverage note and never fail the sync. Use explicit `exec-policies` for a command that cannot translate.

An inline policy list (even `exec-policies: []`), `exec-policies-file` (even an empty file), or imported policy overlay is authoritative. Sync uses it, skips translation, notes which source won, and never modifies the policy file.

`lint` warns with LINT021 when a supported Bash `allow` or `deny` rule lacks a covering native prefix with the same effective decision. This includes declared portable deny and ask exclusions. Broader native prefixes count. Restrictive descendants of an allowed prefix also warn. It checks declared prefixes, not every shell invocation or other Codex config layer. `lint --strict` fails on the warning.

## Import

`agnostic-ai import codex` finds `AGENTS.md` at any depth and reads the rest of the Codex tree:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` at the root | `.agnostic-ai/AGNOSTIC_AI.md`; only the rules block `sync` appends becomes rules |
| `<dir>/AGENTS.md` (nested, hand-written) | one rule with the whole file, named after the scope: `api.md` for `services/api/`, or `services-api.md` when another scope also ends in `api`, with inferred `globs: <dir>/**`. Sync writes a directory with one rule as that rule's text, so the file comes back as it was |
| `<dir>/AGENTS.md` (nested, with `import.codex.shred: true`, or written by `sync`) | one rule per section (`api-tests.md` for `## Tests` in `services/api/`); text above the first `##`, past the title, becomes one more rule named after the scope |
| `## Code Review Rules` or `## Review guidelines` in a nested `AGENTS.md`, or the review section `sync` writes to any `AGENTS.md` | `<reviews>/<scope-slug>.md` with `scope: <dir>` (`review.md` at the root), not a rule |
| `## Conventions` / `## Agents` / `## Skills` wrapper sections | unwrapped: their `### children` become the rules |
| Single-line italic (`_text_`) immediately under a rule heading | extracted into the rule's `description` (and removed from the body) |
| `.codex/agents/*.toml` and `.agents/agents/*.toml` | `<agents>/<name>.md`. When the agent spec already exists, as after `import claude`, the Codex `model` lands as `model: {codex: <name>}`, or as a `codex` entry in an existing per-target map, so Claude Code keeps its own default. A shared scalar `model` that differs gets `x-codex.model` |
| `.agents/skills/<name>/SKILL.md` (+ `agents/openai.yaml`, asset folders) | `<skills>/<name>/SKILL.md` (+ nested assets, exec bits preserved) |
| `.codex/hooks.json` and inline hooks in `.codex/config.toml` | `<hooks>/<event>-<hash8>.yaml` (one spec per handler). Duplicate command hooks match by event, matcher, and command; the JSON definition wins. MCP tool hooks match by event, matcher, server, and tool |
| `.codex/config.toml` `[mcp_servers.<name>]` | `<mcps>/<name>.yaml` |
| `.codex/config.toml` remaining keys (model, sandbox, approval_policy, notify, `[history]`, `[profiles.*]`, `[model_providers.*]`, …) | `.agnostic-ai/overlays/codex.config.toml` (`hooks` + `mcp_servers` stripped). `notify`, `[profiles.*]`, `[model_providers.*]`, and the other [ignored keys](#keys-codex-ignores-in-a-project-config) stay in the overlay but are left out of `.codex/config.toml` on sync |
| `.codex/prompts/*.md` | `<commands>/<name>.md` (byte-identical copy, so user-authored prompts round-trip) |
| `.codex/environments/environment.toml` | `<environments>/codex.yaml`: `[setup]`, `[setup.win32]`, `[cleanup]`, and `[[actions]]` become `setup`, `setup-windows`, `cleanup`, and `dev-commands`. A file with a `[setup.darwin]` script, an action `platform`, or another key stays as written with a note |

Codex [reads both hook formats](https://learn.chatgpt.com/docs/hooks#where-codex-looks-for-hooks). Sync writes `.codex/hooks.json`; import also accepts grouped inline TOML and the older flat form.

Two sections with the same heading in one file get separate names (`style.md`, `style-2.md`). The walk skips hidden directories, the configured source directories, `node_modules/`, `vendor/`, directories git ignores, and directories with their own `.git` (a clone, submodule, or worktree).

`sync -t codex` writes the overlay back, so every captured key survives a `.codex/` wipe; see [Codex config](#codex-config) for conflicts. The [exec-policies overlay](#codex-exec-policies) works the same way.

## Protected paths

Enforced (hook). Codex has no per-tool permission key, so sync writes `.codex/hooks/agnostic-ai-protect.sh` and a `PreToolUse` hook on `apply_patch` in `.codex/hooks.json`. The script checks every `Add File`, `Update File`, `Delete File`, and `Move to` path in the patch. On a protected one it exits 2 with the reason on stderr, and Codex shows it and blocks the edit ([hooks docs](https://learn.chatgpt.com/docs/hooks)). It needs only `sh` and `awk`.

Codex parses but does not yet support an `ask` decision from a `PreToolUse` hook, so `decision: ask` also blocks, with a message telling the agent to ask the user. Like every project hook, it stays inactive until you trust it with `/hooks`. It does not see a shell command that writes a file.

Both commands find the script from the Git root, so a session in a subdirectory still runs it. On Windows it runs through the `sh` on `PATH`, such as Git for Windows provides. Without one, Codex cannot start the hook and the edit goes through. See [Protected paths](@/docs/spec-format/settings.md#protected-paths).

{% <details summary="Script failures and odd paths"> %}
The script blocks the edit when it cannot run: no `awk`, an unreadable project root, or an `awk` failure. It ignores case, reads `\` as a path separator, and blocks a header wrapped in control characters it cannot check.
{% </details> %}

## Verify

1. Install: `npm install -g @openai/codex` ([quickstart](https://learn.chatgpt.com/docs/codex/cli)), then `codex --version`.
2. Run `agnostic-ai sync -t codex`, then `ls .codex/agents/ .agents/skills/`, `head -1 .codex/config.toml`, and `jq '.hooks | keys' .codex/hooks.json`. `head -1` must print the `# Generated by agnostic-ai` comment.
3. `python3 -c 'import sys, tomllib; tomllib.load(open(sys.argv[1], "rb"))' .codex/config.toml` (Python 3.11 or later) and `jq empty .codex/hooks.json` both exit `0`.
4. `codex exec "list one rule from this project"` answers from `AGENTS.md`. Ask it to name a synced skill or agent to confirm they load.
5. Fire a hook's `event` (e.g. an `Edit` for a `PostToolUse` hook). The `command` appears in the hook log.
6. `codex mcp list` shows every `[mcp_servers.<name>]`, with disabled servers flagged.
