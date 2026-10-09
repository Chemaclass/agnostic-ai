+++
title = "Codex"
description = "How agnostic-ai writes Codex configuration: native paths, what Codex cannot hold, and output options."
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

- **Rules**: project-wide rules go into the root `AGENTS.md`. An unscoped rule whose `globs` or `paths` cover whole directories, such as `[src/app/api/**, prisma/**]` or `dir/**/*`, goes into one nested `AGENTS.md` per directory. `alwaysApply: true` keeps an unscoped rule at the root. Root files, filename filters such as `src/api/**/*.ts`, and mixed selectors stay in the root file with a note, or fail with `on-unsupported: error`.
- **Scoped rules**: `scope: services/payments` writes to `services/payments/AGENTS.md`. Adding `globs: tests/payments/**` also writes the rule into `tests/payments/AGENTS.md`. A nested `AGENTS.md` cannot hold file filters outside its directory or root selectors, so those follow `on-unsupported`. Remove legacy `outputs.codex.rules-file` overrides before using explicit scopes.
- **Rule loading**: Codex reads `AGENTS.md` from the session's working directory and its parent directories, so start Codex in a subtree to load its nested `AGENTS.md`. A session started at the root does not load nested files when it later edits there.

  {% <details summary="When nested placement falls back"> %}
  Sync keeps the rule in the root `AGENTS.md`, with a note, when an output override, an unmanaged destination, or another tool that reads the root file cannot keep nested files. A one-off target picked on the command line counts. If tool settings or bodies conflict, sync fails before it writes anything. See the [scoped context safeguards](@/docs/scoped-context.md#shared-files-and-safe-updates).
  {% </details> %}
- **Agents**: [Codex custom agents](https://learn.chatgpt.com/docs/agent-configuration/subagents) hold `name`, `description`, and `developer_instructions`. Any `config.toml` key, such as `model`, `model_reasoning_effort`, or `mcp_servers`, goes under `x-codex`. Sync drops a generic `tools: [Read, Bash, ...]` list with a note, because Codex `tools` is a config table. Use `x-codex.tools` for native settings such as `web_search` and `view_image`.
- **Agent model**: a per-target `model` map keeps another CLI's model name out of Codex. If a shared `model` holds a Claude model name (an alias such as `opus` or `fable`, `inherit`, or a `claude-*` id), sync leaves it out and adds a coverage note naming `model: {claude: <name>}`. `on-unsupported: error` fails the sync. A [model tier](@/docs/spec-format/agents.md#model-tiers) with a `codex` entry sets the Codex model for every agent that names it. The aliases `sol`, `luna`, `astra`, and `terra` resolve to the current ids ([models](@/docs/configuration.md#models)).
- **Agent effort**: `effort` writes `model_reasoning_effort`. Codex accepts `minimal`, `low`, `medium`, `high`, `xhigh`, `max`, and `ultra`, and loads any other text as a custom label ([config reference](https://learn.chatgpt.com/docs/config-file/config-reference.md)). Only Qoder's integer effort budget is dropped, with a note. An explicit `x-codex.model_reasoning_effort` wins.
- **Agent sandbox**: a Codex agent inherits the session sandbox. Codex v0.155.0 and later ignore `sandbox_mode` in an agent file, so `readonly: true` does not make the agent read-only there. Sync still writes `sandbox_mode = "read-only"` for older releases, unless `x-codex.sandbox_mode` overrides it (`null` omits the key). Only session-level limits are enforced: `sandbox_mode` in `config.toml` and [exec policies](#codex-exec-policies).
- **Skills**: Codex scans [skill folders](https://learn.chatgpt.com/docs/build-skills) under `.agents/skills/` from the cwd up to the repo root. Each `SKILL.md` needs `name` and `description`. A scoped skill moves under its scope: `skills/services/api/review/SKILL.md` becomes `services/api/.agents/skills/review/SKILL.md`. Codex reads Claude Code's `` !`command` `` lines and `$ARGUMENTS` as plain text, with a note ([syntax](@/docs/spec-format/skills.md#claude-code-body-syntax)).

  Sync writes `agents/openai.yaml` when the spec sets `x-codex.interface`, `x-codex.policy`, `x-codex.dependencies`, or `disable-model-invocation: true`. A bundled `agents/openai.yaml` is the base, and `x-codex` keys are merged on top. Every tool that writes `.agents/skills/` or the `outputs.codex.skills-dir` folder gets the merged file. Other skill trees, such as `.cursor/skills/`, keep the bundled file.
- **Skill model and effort**: Codex skills have no `model` or `effort` field. Sync omits both, with a coverage note. Codex reads no `disable-model-invocation`, so it becomes `policy.allow_implicit_invocation: false`. An explicit `allow_implicit_invocation` in `x-codex.policy` or a bundled `agents/openai.yaml` wins.
- **Personal memory**: with `memory.personal: repo`, a hand-written `.codex/config.toml` that sync leaves in place needs the personal memory folder in `sandbox_workspace_write.writable_roots`. Sync names the folder and key to add when they are missing. Keep the other writable roots.
- **Hooks**: grouped by `event` (`SessionStart`, `SubagentStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PreCompact`, `PostCompact`, `Stop`, `SubagentStop`, `SessionEnd`, `Interrupt`) with `matcher` and `command`. Optional `timeout`, `statusMessage`, `commandWindows`, `additionalContextLimit` (an explicit `0` is kept), and `async` (run in the background) pass through. With `builtins: [memory]`, a `SessionStart` hook adds the [shared memory](@/docs/memory.md) index to the session.
- **Hook trust**: Codex runs a new command or MCP tool hook from `sync` or `sync --global` only after you trust it with `/hooks`, and again after its handler, matcher, or another setting changes. Project hooks also need a trusted project ([hooks docs](https://learn.chatgpt.com/docs/hooks)).

  Sync names untrusted, modified, and disabled handlers and points to `/hooks`. `agnostic-ai doctor -t codex` fails on untrusted or modified handlers or unreadable trust state, and only reports disabled ones. `doctor --json` adds a `hook_trust` list. The checks read saved trust from `CODEX_HOME`, or `~/.codex`, so they miss a session trust bypass.
- **Hook commands**: each `command` starts with `export AGNOSTIC_AI_TARGET=codex; ` so a shared script knows Codex ran it ([which target ran a hook](@/docs/spec-format/hooks.md#hook-target)). The prefix needs a POSIX session shell; a macOS or Linux login shell of `pwsh` or `nu` breaks it. On Windows, Codex runs `commandWindows` through PowerShell or cmd, and sync fills it with the declared command, without the variable, when the spec sets none. `import codex` strips both.

  Codex hooks have no exec form, so `args` fold into `command` after the prefix, each in POSIX single quotes (`bash 'guard.sh'`). On Windows, PowerShell reads the folded command as intended when no arg holds an apostrophe; cmd does not. In those cases, set `commandWindows`.
- **MCP tool hooks**: `type: mcp_tool` calls a tool on a connected MCP server instead of a shell command, with the same trust review ([hooks docs](https://learn.chatgpt.com/docs/hooks.md)). `server` and `tool` are required, `input` (an argument template) is optional, and `timeout`/`statusMessage` apply.
- **Edit hooks**: Codex accepts `Edit` and `Write` as matcher aliases for `apply_patch`, but reports `tool_name: "apply_patch"` and puts the patch in `tool_input.command`, with no `tool_input.file_path`. A `PreToolUse`, `PostToolUse`, or `PermissionRequest` edit hook whose `command` reads `tool_input.file_path` gets an empty value. `sync` prints a note, or fails with `on-unsupported: error`. Read edited paths with [`agnostic-ai hook paths`](@/docs/spec-format/hooks.md#edited-paths), which parses the patch, or add `target: claude` to the hook. The check does not read scripts you own.

  {% <details summary="Imported project-root paths"> %}
  Imported `$CLAUDE_PROJECT_DIR` and `${CLAUDE_PROJECT_DIR}` paths resolve through the Git worktree root, so hooks run from a subdirectory. This needs a POSIX shell and Git. Unsupported syntax or a project outside Git gets a note, or fails with `on-unsupported: error`. See [project-root paths](@/docs/spec-format/hooks.md#imported-project-root-paths).
  {% </details> %}

  {% <details summary="Hooks inside config.toml"> %}
  `import codex` also reads hooks from a hand-written `.codex/config.toml`: `[[hooks.<event>]]` holds `matcher`, and a nested `[[hooks.<event>.hooks]]` holds the command fields. A flat table with `matcher` and `command` together works too.
  {% </details> %}
- **Reviews**: review specs land in `## Code Review Rules`. [Codex code review](https://learn.chatgpt.com/docs/third-party/github) reads it from the root `AGENTS.md` and the `AGENTS.md` nearest each changed file. An unscoped spec goes to the root. `scope: services/api` goes to `services/api/AGENTS.md`. Other tools that read `AGENTS.md` load the section too; set `targets:` on a spec to keep it out. With `outputs.codex.rules-file` set, sync skips the root `AGENTS.md`, so unscoped reviews get a coverage note.
- **Exec policies**: opt-in, from [declared policies](#codex-exec-policies) or [translated Bash permissions](#translate-bash-permissions). Until a source holds a rule, portable `permissions` lists raise a coverage note naming both routes.
- **Environment**: environment specs write the Codex app's [local environment](https://learn.chatgpt.com/docs/environments/local-environment).
  - `setup`, `cleanup`, and `setup-windows` become the `[setup]`, `[cleanup]`, and `[setup.win32]` scripts (a list runs one command per line). `[setup]` is always written, empty if needed.
  - Each `dev-commands` entry becomes an `[[actions]]` button with its `name`, `command` (a list joins into one shell line), and an `icon` that defaults to `run`. A `cwd` becomes `cd <cwd> &&` before the command.
  - Specs merge by top-level key, and the last value wins.
  - `port`, `auto-port`, `env`, `url`, `install`, and `terminals` have no Codex key and get a no-effect note.
- **MCP**: `[mcp_servers.<name>]` tables. `disabled: true` writes `enabled = false`.
  - Stdio: `command`/`args`/`env`/`cwd` plus `env_vars`, whose entries are names or `{name, source}` objects with `source` set to `local` or `remote`.
  - HTTP/SSE: `url`/`bearer_token_env_var`/`http_headers`/`env_http_headers`/`auth` (`oauth` or `chatgpt`)/`http_headers_helper` (a local command printing header JSON, for local HTTP servers only).
  - A `${NAME}` reference in `env` or `headers` becomes `env_vars`, `bearer_token_env_var`, or `env_http_headers`. A server with a reference in `url` or `args` is left out with a note. See [environment references](@/docs/spec-format/mcps.md#environment-references).
  - Any transport: `enabled_tools`/`disabled_tools` (`disabled_tools` applies after `enabled_tools`) and the fields below. Set only one of `startup_timeout_sec` and its millisecond alias `startup_timeout_ms`.

  | Field | Default | Meaning |
  |-------|---------|---------|
  | `required` | `false` | Fail startup or resume if this enabled server cannot initialize. |
  | `startup_timeout_sec` / `startup_timeout_ms` | `10` / `10000` | Server startup timeout. |
  | `tool_timeout_sec` | `60` | Per-tool execution timeout. |
  | `default_tools_approval_mode` | unset | `auto`, `prompt`, `writes`, or `approve`, unless a per-tool override exists. |
  | `experimental_environment` | unset | `local` or `remote`. `remote` starts a stdio server through a remote executor; HTTP remote placement is not implemented yet. |

  `scopes`, `oauth_resource`, and an `[mcp_servers.<id>.oauth]` sub-table `{client_id, callback_url, callback_port}` go on the HTTP/SSE shape next to `auth`.

  A `tools` map writes per-tool sub-tables, `[mcp_servers.<name>.tools.<tool>]`, with keys passed through as written, such as `output_token_limit` (v0.153.0 and later) and a per-tool approval override.

  Each sync overwrites the project `config.toml`. Put unmanaged config in `~/.codex/config.toml`.

  {% <details summary="Server names with special characters"> %}
  Server names that are not bare TOML keys are quoted, such as `npm:@modelcontextprotocol/server-sequential.thinking` (Codex CLI 0.152.0 and later).
  {% </details> %}
- **Settings**: the last portable `model` and `effort` write `model` and `model_reasoning_effort`. They lose to `outputs.codex.config.model` or `outputs.codex.config.model-reasoning-effort` and to the captured `.agnostic-ai/overlays/codex.config.toml` (see [Codex config](#codex-config)). Codex has no `x-<target>` settings passthrough: an `x-codex` block on a settings spec raises a coverage note naming the overlay and `outputs.codex.config`.
- **Commands**: off by default. Codex reads custom prompts only from `~/.codex/prompts/` and [deprecates them in favor of skills](https://learn.chatgpt.com/docs/custom-prompts). `sync` prints a coverage note and removes a stale managed `.codex/prompts/` tree.

## Config keys

| Key | Default | Notes |
|---|---|---|
| `outputs.codex.agents-dir` | `.codex/agents` | override to `.agents/agents` for the community shared layout |
| `outputs.codex.skills-dir` | `.agents/skills` | the path Codex scans |
| `outputs.codex.nested-glob-rules` | `true` | put rules that cover whole directories in nested `AGENTS.md`; `false` keeps them in the root file |
| `outputs.codex.shared-subagents` | `true` | writes the per-skill tree at `skills-dir`; set `false` to skip Codex skills |
| `outputs.codex.commands-dir` | unset | set to e.g. `.codex/prompts` to write the deprecated project prompts layout |
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

Sync writes the overlay that `import codex` captured, `.agnostic-ai/overlays/codex.config.toml`, before the `[mcp_servers.*]` sections built from your specs. It keeps every other `.codex/config.toml` key (`[history]`, `[tui]`, ...), so wiping `.codex/` between import and sync loses nothing.

### Keys Codex ignores in a project config

Codex drops these top-level keys from a project `.codex/config.toml`, with a startup warning ([docs](https://learn.chatgpt.com/docs/config-file/config-advanced), [source](https://github.com/openai/codex/blob/0b1b78a4f1694e2b9e393d385c7b82ca714ca08a/codex-rs/config/src/loader/mod.rs#L88-L101)): `openai_base_url`, `chatgpt_base_url`, `apps_mcp_product_sku`, `model_provider`, `model_providers`, `notify`, `profile`, `profiles`, `experimental_realtime_ws_base_url`, `otel`, `responses_api_metadata`, and `experimental_realtime_webrtc_call_base_url`.

Sync writes none of them. Set them in `~/.codex/config.toml`. The `notify`, `profiles`, and `model-providers` fields above print a note. When the overlay sets one, sync leaves it out of `.codex/config.toml`, keeps the overlay file, and prints a note naming the key. These notes never fail the sync, even with `on-unsupported: error`.

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

Only plain, unquoted words translate. A Bash rule with quotes, escapes, a `*` other than one trailing ` *` or `:*`, shell operators, expansions, assignments, or shell keywords gets a coverage note. `on-unsupported: error` fails on it; `silent` omits it. Rules for other tools, such as `Read(.env)` or `WebFetch`, share one `permissions` coverage note and never fail the sync. Use explicit `exec-policies` for a command that cannot translate.

An inline policy list (even `exec-policies: []`), `exec-policies-file` (even an empty file), or imported policy overlay wins. Sync skips translation, notes which source won, and never modifies the policy file.

`lint` warns with LINT021 when a supported Bash `allow` or `deny` rule, including declared portable deny and ask exclusions, has no native prefix that covers it with the same decision. Broader native prefixes count. Restrictive descendants of an allowed prefix also warn. It checks declared prefixes only. `lint --strict` fails on the warning.

## Import

`agnostic-ai import codex` finds `AGENTS.md` at any depth and reads the rest of the Codex tree:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` at the root | `.agnostic-ai/AGNOSTIC_AI.md`; the rules block `sync` appends becomes rules |
| `<dir>/AGENTS.md` (nested, hand-written) | one rule with the whole file, named after the scope: `api.md` for `services/api/`, or `services-api.md` when another scope also ends in `api`, with inferred `globs: <dir>/**` |
| `<dir>/AGENTS.md` (nested, with `import.codex.shred: true`, or written by `sync`) | one rule per section (`api-tests.md` for `## Tests` in `services/api/`); text above the first `##`, past the title, becomes one more rule named after the scope |
| `## Code Review Rules` or `## Review guidelines` in a nested `AGENTS.md`, or the review section `sync` writes to any `AGENTS.md` | `<reviews>/<scope-slug>.md` with `scope: <dir>` (`review.md` at the root), not a rule |
| `## Conventions` / `## Agents` / `## Skills` wrapper sections | unwrapped: their `### children` become the rules |
| Single-line italic (`_text_`) under a rule heading | moved into the rule's `description` |
| `.codex/agents/*.toml` and `.agents/agents/*.toml` | `<agents>/<name>.md`. When the agent spec already exists, as after `import claude`, the Codex `model` lands as `model: {codex: <name>}` (or a `codex` entry in an existing per-target map), so Claude Code keeps its own default. A differing shared scalar `model` gets `x-codex.model` |
| `.agents/skills/<name>/SKILL.md` (+ `agents/openai.yaml`, asset folders) | `<skills>/<name>/SKILL.md` (+ nested assets, exec bits preserved) |
| `.codex/hooks.json` and inline hooks in `.codex/config.toml` | `<hooks>/<event>-<hash8>.yaml` (one spec per handler). A duplicate hook in both files imports once; the JSON definition wins |
| `.codex/config.toml` `[mcp_servers.<name>]` | `<mcps>/<name>.yaml` |
| `.codex/config.toml` remaining keys (model, sandbox, approval_policy, `[history]`, ...) | `.agnostic-ai/overlays/codex.config.toml` (`hooks` + `mcp_servers` stripped). The [ignored keys](#keys-codex-ignores-in-a-project-config) stay in the overlay but are left out of `.codex/config.toml` on sync |
| `.codex/prompts/*.md` | `<commands>/<name>.md` (byte-identical copy) |
| `.codex/environments/environment.toml` | `<environments>/codex.yaml`: `[setup]`, `[setup.win32]`, `[cleanup]`, and `[[actions]]` become `setup`, `setup-windows`, `cleanup`, and `dev-commands`. A file with a `[setup.darwin]` script, an action `platform`, or another key stays as written with a note |

Two sections with the same heading in one file get separate names (`style.md`, `style-2.md`). The walk skips hidden directories, the configured source directories, `node_modules/`, `vendor/`, directories git ignores, and directories with their own `.git`.

`sync -t codex` writes the overlay back, so every captured key survives a `.codex/` wipe. See [Codex config](#codex-config) for conflicts.

## Protected paths

Enforced (hook). Codex has no per-tool permission key, so sync writes `.codex/hooks/agnostic-ai-protect.sh` and a `PreToolUse` hook on `apply_patch` in `.codex/hooks.json`. The script checks every `Add File`, `Update File`, `Delete File`, and `Move to` path in the patch. On a protected one it exits 2 with the reason on stderr, and Codex blocks the edit ([hooks docs](https://learn.chatgpt.com/docs/hooks)). It needs only `sh` and `awk`.

Codex does not support an `ask` decision from a `PreToolUse` hook, so `decision: ask` also blocks, with a message telling the agent to ask the user. Like every project hook, it stays inactive until you trust it with `/hooks`. It does not see a shell command that writes a file.

On Windows the script runs through the `sh` on `PATH`, such as Git for Windows provides. Without one, Codex cannot start the hook and the edit goes through. The script blocks the edit when it cannot run, for example without `awk`. See [Protected paths](@/docs/spec-format/settings.md#protected-paths).

## Verify

1. Install: `npm install -g @openai/codex` ([quickstart](https://learn.chatgpt.com/docs/codex/cli)), then `codex --version`.
2. Run `agnostic-ai sync -t codex`, then `ls .codex/agents/ .agents/skills/`, `head -1 .codex/config.toml`, and `jq '.hooks | keys' .codex/hooks.json`. `head -1` must print the `# Generated by agnostic-ai` comment.
3. `python3 -c 'import sys, tomllib; tomllib.load(open(sys.argv[1], "rb"))' .codex/config.toml` (Python 3.11 or later) and `jq empty .codex/hooks.json` both exit `0`.
4. `codex exec "list one rule from this project"` answers from `AGENTS.md`. Ask it to name a synced skill or agent.
5. Fire a hook's `event` (e.g. an `Edit` for a `PostToolUse` hook). The `command` appears in the hook log.
6. `codex mcp list` shows every `[mcp_servers.<name>]`, with disabled servers flagged.
