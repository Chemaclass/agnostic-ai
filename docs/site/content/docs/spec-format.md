+++
title = "Spec format"
description = "Define agents, skills, rules, hooks, MCP servers, commands, settings, and other portable specs."
weight = 110

[extra]
group = "Reference"
+++

# Spec format

The [capability matrix](@/docs/targets/_index.md#capability-matrix) shows which targets receive each spec kind. Each target page shows how that tool renders it.

Source paths are relative to `.agnostic-ai/` by default, so `rules/*.md` means `.agnostic-ai/rules/*.md`. Override directories with [`sources`](@/docs/configuration.md#sources).

Start with a [rule](#rules) for conventions, a [skill](#skills) for a reusable workflow, or an [MCP server](#mcp-servers) for a tool connection. See [Getting started](@/docs/getting-started.md) for a complete first rule.

| Kind    | Source                                    | Format                      |
|---------|-------------------------------------------|-----------------------------|
| [Agent](#agents) | `agents/*.md`                             | Markdown + YAML frontmatter |
| [Skill](#skills) | `skills/*.md` or `skills/<name>/SKILL.md` | Markdown + YAML frontmatter |
| [Rule](#rules) | `rules/*.md`                              | Markdown + YAML frontmatter |
| [Hook](#hooks) | `hooks/*.yaml`                            | YAML                        |
| [MCP](#mcp-servers) | `mcps/*.yaml`                             | YAML                        |
| [Command](#commands) | `commands/*.md`                           | Markdown + YAML frontmatter |
| [Settings](#settings) | `settings/*.yaml`                      | YAML                        |
| [Review](#reviews) | `reviews/*.md`                         | Markdown + YAML frontmatter |
| [Environment](#environments) | `environments/*.yaml`                  | YAML                        |
| [Ignore](#ignore) | `ignore/*.md`                          | Markdown + YAML frontmatter |

Discovery is recursive: every `.md` under `agents/`, `skills/`, `rules/`, `commands/`, `reviews/`, and `ignore/` loads, and every `.yaml` under `hooks/`, `mcps/`, `settings/`, and `environments/`.

## Nested layout: per-directory scope

A spec in a subdirectory of its source dir gets an implicit **scope** equal to that subpath.

```
rules/
├── conventional-commits.md      # scope: ""    (root)
└── backend/
    ├── auth.md                  # scope: "backend"
    └── api/limits.md            # scope: "backend/api"
```

For rules, scope controls native activation or directory discovery. A flat rule may set `scope: services/payments`; source-layout scope wins. `agnostic-ai new rule payments-context --scope services/payments` creates one.

Scoped bodies stay out of root instruction appendices. Supported targets get native path conditions or a nested instruction file. Unsupported targets skip the rule with a warning, or fail under `on-unsupported: error`. See [directory-specific instructions](@/docs/scoped-context.md) for the target matrix and selector limits.

## Agents

```markdown
---
name: code-reviewer
description: Reviews diffs for bugs, style, and architectural issues.
tools: [Read, Grep, Bash]
model: sonnet
---

You are a code reviewer. Report concise findings with `file:line` references.
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename without `.md` | Agent identifier and output filename. |
| `description` | no | empty | One-liner shown in tool listings. |
| `tools` | no | unset | Tools the agent may invoke. See [`tools` support by target](#tools-support-by-target). |
| `model` | no | unset | A string for every target, or a map per target. See [per-target `model` and `effort`](#per-target-model-and-effort). |
| `effort` | no | unset | A string or integer for every target, or a map per target. See [per-target `model` and `effort`](#per-target-model-and-effort). |
| `color` | no | unset | Badge color. See [`color` support by target](#color-support-by-target). |
| `readonly` | no | unset | `true` restricts the agent to reading. Cursor: restricted. Claude: `disallowedTools: Write, Edit, NotebookEdit` (Bash stays allowed). Codex: `sandbox_mode = "read-only"`. Factory: `tools: read-only` with `mcpServers: []` unless servers are listed (wins over a portable `tools` list). An explicit `x-claude.disallowedTools`, `x-codex.sandbox_mode`, or `x-factory.tools` wins. Other targets report a coverage note. `false` is a no-op. |
| `memory` | no | unset | Persistent memory scope: `user`, `project`, or `local`. |

Any other frontmatter field passes through unchanged.

`memory` gives the agent a directory that survives across sessions. Only [Claude Code](@/docs/targets/claude.md#agent-memory) is confirmed to act on it; [Qoder](@/docs/targets/qoder.md#subagent-memory) gets the key unconfirmed, Junie passes it through, and every other adapter drops it.

### Per-target `model` and `effort` {#per-target-model-and-effort}

`model:` and `effort:` each take a scalar or a map keyed by target name, with an optional `default`. Precedence: `x-<target>.<key>`, then `<key>.<target>`, then `<key>.default`, then the key is not written and the tool uses its own default. `x-<target>.<key>: null` deletes it. A non-scalar value under a target key falls through to `default`.

| Want | Write |
|------|-------|
| Same model everywhere | `model: sonnet` |
| Per target, with a fallback | `model: {claude: sonnet, default: gpt-4o}` |
| Per target, tool default elsewhere | `model: {claude: sonnet}` |
| Different effort per target | `effort: {claude: xhigh, default: high}` |

```yaml
---
name: architect
description: Designs the change before anyone codes it.
model:
  claude: opus
  cursor: "claude-opus-5[effort=high]"
  default: gpt-5.5
effort:
  claude: xhigh
  qoder: 8000
  factory: max
  default: high
x-codex:
  model_reasoning_effort: xhigh
---
```

Result: Claude gets `opus` and `xhigh`; Qoder `gpt-5.5` and `8000`; Junie `gpt-5.5` and `high`; Cursor `claude-opus-5[effort=high]` (resolved effort discarded); Codex `gpt-5.5` with `x-codex` overriding effort to `xhigh`; Factory `gpt-5.5` with no `reasoningEffort` (`max` is outside its enum, coverage note); Trae drops both with notes.

**`effort` values by target.** Only the targets listed were checked. Omitting `effort` inherits the session's level.

| Target | Values | How it lands |
|--------|--------|--------------|
| [Claude Code](@/docs/targets/claude.md) | `low`, `medium`, `high`, `xhigh`, `max`, model dependent | Verbatim `effort`, not validated |
| [Qoder](@/docs/targets/qoder.md) | Same five names, or a positive integer | Verbatim `effort` |
| [Junie](@/docs/targets/junie.md) | Alias of `reasoningLevel` | Verbatim `effort` |
| [Factory](@/docs/targets/factory.md) | `low`, `medium`, `high` | `reasoningEffort`. Other values and integers raise a coverage note |
| [Codex](@/docs/targets/codex.md) | Any string | `model_reasoning_effort`; `x-codex.model_reasoning_effort` wins. An integer raises a note |
| [Cursor](@/docs/targets/cursor.md) | none | Coverage note. Put it in the model id: `model: {cursor: "claude-opus-5[effort=high]"}` |
| [Copilot](@/docs/targets/copilot.md) | none | Coverage note. `x-copilot.effort` still passes through |
| Every other target | none | Coverage note |

Cursor encodes effort in the `model` string, so it rides on the `model` map. Factory ignores `reasoningEffort` when `model` resolves to `inherit`.

### `tools` support by target

Only the targets listed were checked. A target that cannot honor `tools` prints a coverage note at sync time, so `tools: [Read]` never silently becomes an unrestricted agent.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Copilot](@/docs/targets/copilot.md), [Junie](@/docs/targets/junie.md) | Passed through as a YAML list |
| [Qoder](@/docs/targets/qoder.md), [Trae](@/docs/targets/trae.md) | Passed through as a comma-separated string (`tools: Read, Bash`) |
| [Windsurf](@/docs/targets/windsurf.md), [Kiro](@/docs/targets/kiro.md), [Factory](@/docs/targets/factory.md), [Gemini](@/docs/targets/gemini.md) | Translated to native names |
| [Antigravity](@/docs/targets/antigravity.md), [OpenHands](@/docs/targets/openhands.md), [Goose](@/docs/targets/goose.md), [Codex](@/docs/targets/codex.md), [Cursor](@/docs/targets/cursor.md), [Augment](@/docs/targets/augment.md), [Kilo Code](@/docs/targets/kilo.md) | Dropped with a note |

Translation can widen access: on Kiro, `Edit` alone also permits `delete_file`. Most targets accept native names through `x-<target>.tools`, which bypasses translation.

### `mcpServers` support by target {#mcpservers-support-by-target}

Only the targets listed were checked. A top-level `mcpServers` list narrows which MCP servers one agent may reach. Omitting it inherits the session's full set.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Junie](@/docs/targets/junie.md), [Factory](@/docs/targets/factory.md) | Server names, written to the agent file |
| [Qoder](@/docs/targets/qoder.md) | Server names or inline objects, written to the agent file |
| [OpenHands](@/docs/targets/openhands.md) | Inline definitions only. Set `x-openhands.mcp_servers` |
| [Antigravity](@/docs/targets/antigravity.md) | Inline objects only. Set `x-antigravity.mcpServers` |
| [Kiro](@/docs/targets/kiro.md) | Inline definitions only. Set `x-kiro.mcpServers` |

Every other target drops the list with a coverage note; the three inline targets' notes name the `x-<target>` key to set.

**An empty list is not portable.** Junie treats `mcpServers: []` as keeping every configured server. Factory treats it as excluding every server. List the servers you want instead.

### `permissionMode` and agent `hooks` support by target {#agent-policy-support-by-target}

Only the targets listed were checked. `permissionMode` sets one delegated agent's approval boundary; `hooks` scopes lifecycle hooks to it. Omitting either inherits the parent session.

| Target | `permissionMode` | Agent `hooks` |
|--------|------------------|---------------|
| [Claude Code](@/docs/targets/claude.md) | Seven values, `manual` aliases `default` | Written to the agent file |
| [Qoder](@/docs/targets/qoder.md) | Six values, another one is reported | Seven events, a wider one is reported |
| [OpenHands](@/docs/targets/openhands.md) | Different names (`always_confirm`, `never_confirm`, `confirm_risky`). Set `x-openhands` | Six snake_case events, command handlers only. Set `x-openhands` |

On Qoder, `bypassPermissions` is demoted to `acceptEdits` when security policy disables it, and agent scope runs fewer hook events than project scope.

### `color` support by target

Only the targets listed were checked. `color` is written verbatim and not validated. An unrecognized value is cosmetic: the agent still runs.

| Target | Values |
|--------|--------|
| [Augment](@/docs/targets/augment.md) | Free text, an ANSI color name |
| [Kilo Code](@/docs/targets/kilo.md) | Hex or a theme token |
| [Qoder](@/docs/targets/qoder.md) | One of eight names |
| [OpenHands](@/docs/targets/openhands.md) | Dropped with a note. Set `x-openhands.color` to a [Rich color name](https://rich.readthedocs.io/en/stable/appendix/colors.html) |

`color: blue` is valid on Augment and Qoder but is neither hex nor a Kilo Code theme token. OpenHands shares its `.agents/agents/` tree with [Goose](@/docs/targets/goose.md), whose frontmatter has no `color`, so a portable `color` prints a coverage note there.

## Skills

Two layouts:

- **Flat:** `skills/yaml-validator.md`
- **Nested**, for skills with attached resources: `skills/yaml-validator/SKILL.md` next to `skills/yaml-validator/schema.yaml`

```markdown
---
name: yaml-validator
description: Validate YAML against a schema.
---

# YAML Validator

1. Read target file
2. Parse YAML and compare against `schema.yaml`
3. Report violations as `path: message`
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | dir or filename | Skill identifier and output directory. Some targets restrict the format. |
| `description` | no | empty | One-liner the model uses to decide whether to invoke the skill. |
| `model` | no | unset | Claude Code model for the rest of the turn. Scalar or per-target map; `x-claude.model` wins. |
| `effort` | no | unset | Claude Code effort for the rest of the turn. Scalar or per-target map; `x-claude.effort` wins. |

Other targets omit skill `model` and `effort` and report a coverage note when a value resolves for them. Use `{claude: opus}` to choose a model only for Claude. Global sync uses the same renderers; shared global directories omit target overrides.

Only `SKILL.md` and flat `skills/*.md` parse as skills. Every other file in a nested skill directory is a bundled asset (scripts, templates, fixtures, extra `*.md`). Assets copy verbatim to the same relative path under each target's skills dir. Import and sync preserve executable bits.

Most targets write `<dir>/<name>/SKILL.md` with assets. Several share `.agents/skills/`, so identical bytes write once. Targets with no skill surface flatten it to a `skill-<name>.md` rule and raise a coverage note, since assets cannot follow. Set `outputs.<target>.emit-skills-as-commands: true` to also emit a slash command. Each target page gives the exact directory.

### `disable-model-invocation` support by target {#disable-model-invocation-support-by-target}

Only the targets listed were checked. Setting it keeps a skill out of automatic model invocation; the user can still invoke it. Omitting it leaves each target's default, which is model-invocable everywhere below.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Cursor](@/docs/targets/cursor.md) | Written to `SKILL.md` |
| [Codex](@/docs/targets/codex.md) | Written as `allow_implicit_invocation: false` to `agents/openai.yaml`. An explicit value in `x-codex.policy` or a bundled `agents/openai.yaml` wins |
| [Crush](@/docs/targets/crush.md) | Dropped with a note. Set `x-crush.disable-model-invocation` |
| [Factory](@/docs/targets/factory.md) | Dropped with a note. Set `x-factory.disable-model-invocation` |

Crush and Factory skills land in the shared `.agents/skills/` tree, so emitting the key would hand it to targets with no such field. Use the `x-` key: a manual-only skill turning model-invocable is a safety boundary.

OpenHands' `triggers` is unrelated: it injects a skill on a keyword. Devin spells this restriction `triggers: [user]`.

## Rules

```markdown
---
name: conventional-commits
description: Always use Conventional Commits format.
globs: "**/*"
alwaysApply: true
---

Use `feat:`, `fix:`, `docs:`, etc. Subject under 72 chars.
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Rule identifier. |
| `description` | no | empty | Short summary. |
| `scope` | no | project-wide | Project-relative directory and its descendants. Source-layout scope wins. A scope inside `node_modules` is refused. See [scoped context](@/docs/scoped-context.md). |
| `globs` | no | target-dependent; `new rule` seeds `**/*` | Project-relative patterns, as a comma-separated string (`"*.go,*.mod"`) or a list. A comma inside a brace set does not separate patterns, so `"src/**/*.{ts,tsx}"` is one pattern. With `scope`, the selector must stay inside the directory. `new rule --scope` omits it. |
| `paths` | no | unset | File patterns, as a string or list. Scoped rules accept it with or instead of `globs`; see [selector limits](@/docs/scoped-context.md#narrow-a-rule-to-certain-files). |
| `alwaysApply` | no | target-dependent; `new rule` seeds `true` | Requests unconditional activation. With `scope`, only inside the directory. `new rule --scope` omits it. |

## Hooks

Pure YAML, no markdown body.

```yaml
name: session-status
description: Show repository status when a session starts.
targets: [claude, codex]
event: SessionStart
command: "git status --short"
```

Command hooks receive event JSON on stdin; read the edited path or shell command from the target's `tool_input` fields. `AGNOSTIC_AI_TARGET` names the target that ran the hook; see [which target ran a hook](#hook-target).

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Hook identifier. |
| `description` | no | empty | Free-form documentation. |
| `event` | yes | none | Hook event, written verbatim. See [events](#events). |
| `matcher` | no | empty | Regex on the tool name, or another event-specific selector. |
| `command` | command handlers only | none | Shell command, or a list where each entry becomes its own handler. |
| `args` | no | empty | Switches to **exec form**: `command` runs as an executable with `args` as its argument vector and no shell, so spaces, `$`, and backticks pass verbatim. Leave unset when the command needs a pipe or `&&`. Targets with no exec form (Codex, Gemini, Cursor) get the args folded into `command`, each quoted for a POSIX shell. |
| `type` | no | `command` | `command`, `http`, `mcp_tool`, or `prompt`, where the target supports it. |
| `timeout` | no | none | Seconds before the tool cancels the hook. Some targets convert to milliseconds or apply their own default. |
| `disabled` | no | `false` | Keep the hook defined but stop it running. Antigravity and Kiro write `enabled: false`; OpenCode and Kilo write no plugin module; other targets emit the hook unchanged. |

Handler-specific fields emit only where the target's schema defines them:

- `server`, `tool`, `input` (`type: mcp_tool`): Claude Code, Codex.
- `url`, `headers`, `allowedEnvVars` (HTTP handler): Claude Code, Qoder, Copilot.
- `prompt`, `model` (prompt handler): Claude Code, Qoder, Cursor, Copilot (`sessionStart` only).
- `statusMessage`, `async`: Claude Code, Codex, Qoder.
- `asyncRewake`, `shell`, `if`: Claude Code, Qoder.
- `continueOnBlock`: Claude Code. `commandWindows`, `additionalContextLimit`: Codex. `failClosed`: Cursor. `loop_limit`: Cursor, Trae.
- `x-goose.on_failure` (Goose), `x-kiro.action` (Kiro), `x-gemini.hooks`, `x-gemini.sequential`, `x-gemini.name`, `x-gemini.env` (Gemini).

`command` is not needed for a non-command handler, a valid `x-kiro.action`, or a hook that sets `x-gemini.hooks`. Scope a non-command hook to the targets that support it with `target` or `targets`.

### Events

`event` is written verbatim; names are never translated between tools. Claude Code and Codex share `PreToolUse`, `PostToolUse`, and `UserPromptSubmit`, so one spec feeds both. Other tools need their own names, such as Cursor's `beforeShellExecution` or Gemini's `BeforeTool`. `agnostic-ai validate` flags an event a target does not recognize. Targets without hook support log a warning and skip.

### Which target ran a hook {#hook-target}

A shared script reads `AGNOSTIC_AI_TARGET` to pick the reply protocol (Claude Code and Codex read exit code 2 and stderr; Cursor reads JSON).

```sh
case "$AGNOSTIC_AI_TARGET" in
  cursor) echo '{"permission":"deny","agent_message":"Blocked."}' ;;
  *) echo "Blocked." >&2; exit 2 ;;
esac
```

A spec that sets `AGNOSTIC_AI_TARGET` in its own `env` keeps that value. Where sync cannot set it, the parent process's value stays, which can be `claude` for a tool started from Claude Code. Sync sets it per target:

- [Claude Code](@/docs/targets/claude.md): `env` in `.claude/settings.json` (`~/.claude/settings.json` for `sync --global`). Set for the whole session, so the Bash tool sees it too.
- [Cursor](@/docs/targets/cursor.md): a `sessionStart` hook returns the variable. `sessionStart` hooks and hooks that fire before it returns do not see it.
- [Codex](@/docs/targets/codex.md): `export AGNOSTIC_AI_TARGET=codex; ` before `command`. Not set on Windows (`commandWindows`), and needs a POSIX session shell (a `pwsh` or `nu` login shell breaks it).
- [Gemini](@/docs/targets/gemini.md), [Qoder](@/docs/targets/qoder.md), [Copilot](@/docs/targets/copilot.md): `env` on each command handler.
- [Goose](@/docs/targets/goose.md), [Crush](@/docs/targets/crush.md), [Cline](@/docs/targets/cline.md): `export` prefix or line.
- [OpenCode](@/docs/targets/opencode.md), [Kilo](@/docs/targets/kilo.md): `.env()` on each plugin command. [Zed](@/docs/targets/zed.md): `env` on each task.

Trae, Factory, OpenHands, Antigravity, Kiro, Windsurf, and Augment do not get the variable: none has a per-hook `env`, and a prefix would break hooks that work today. Tell them apart by their own variables, such as `TRAE_PROJECT_DIR`, `FACTORY_PROJECT_DIR`, `OPENHANDS_PROJECT_DIR`, `DEVIN_PROJECT_DIR`, or `AUGMENT_PROJECT_DIR`. Do not use `CLAUDE_PROJECT_DIR`: Cursor, Gemini, Qoder, Factory, and Trae set it too.

`sync --global` leaves a matching hand-written hook alone, so an adopted Codex, Gemini, or Qoder entry does not get the variable. Cursor and Copilot also run `.claude/settings.json` hooks but read no `env` from it: Cursor still gets `cursor` from `sessionStart`, Copilot gets nothing.

### Per-target body fences

To vary prose per target, wrap the divergent part in `::target` fences. Content outside a fence emits everywhere; content inside emits only to the listed targets. Marker lines never reach the output.

```md
Shared intro paragraph.

::target claude
Claude-only section.
::end

::targets codex gemini
Codex and Gemini section.
::end
```

| Syntax | Meaning |
|--------|---------|
| `::target <name>` | Opens a fence for one target. |
| `::targets <a> <b>` | Opens a fence for several targets. |
| `::end` | Closes the most recent fence. A missing `::end` runs to the end of the body. |

- `import` round-trips keep fences intact. `import codex` builds them when Claude and Codex ship the same agent or skill with different bodies.
- Fences also work in `.agnostic-ai/AGNOSTIC_AI.md`. A block reaches an entry-point file when any target that reads the file is listed. `AGENTS.md` is shared by the whole family, so `::target codex` content reaches every reader; a shared file is never split. See [Entry-point files](@/docs/configuration.md#entry-point-files).

## Target scoping

Four fields limit where any spec kind emits.

| Field | Effect |
|-------|--------|
| `target` | Emit only to this one target. |
| `targets` | Emit only to these targets. |
| `target-exclude` | Emit everywhere except this target. |
| `targets-exclude` | Emit everywhere except these targets. |

With none set, the spec emits to every target that supports its kind. `target` beats `targets`. Exclusion wins over inclusion.

### Import auto-scoping

A hook imported from a tool gets `target: <tool>` (`codex`, `claude`, or `gemini`). Delete the field to let it reach every target.

`import claude` and `import codex` do the same for agents and skills. When both `.claude/` and `.codex/` exist but only one has a spec, it gets `target: <tool>`. A spec in both stays unscoped, and so does every spec in a single-tool project, so round-trips stay byte-identical.

## MCP servers

Pure YAML, no markdown body, one file per server.

```yaml
name: filesystem
description: Local filesystem access for the model.
type: stdio
command: npx
args:
  - -y
  - "@modelcontextprotocol/server-filesystem"
env:
  ROOT: /tmp
```

`name` is the server identifier, not the filename. It may contain package-style slashes, such as `npm:@modelcontextprotocol/server-sequential.thinking`. Such names are percent-encoded in YAML filenames and kept as-is in every generated config. Other spec kinds need one safe path segment, because their names become output paths.

A server needs `command` (stdio) or `url` (remote). `agnostic-ai lint` reports a missing one as LINT008; `validate` and `sync` do not, and some targets write a server that cannot start. See [lint](@/docs/cli-reference.md#lint).

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | yes | none | Server identifier and key in the generated config. |
| `description` | no | empty | Free-form documentation. Dropped where the target's MCP schema has no such key (Junie, Warp). |
| `type` | no | `stdio` | `stdio`, `http`, `sse`, or `ws`. Remote transports write an explicit `type`; `stdio` stays implicit. A `ws` entry emits no server on Augment, Factory, and Qoder. |
| `command` | stdio only | none | Executable to launch. |
| `args` | no | empty | Argument list for the command. |
| `env` | no | empty | Environment variables for the server. |
| `url` | http/sse/ws only | none | Endpoint URL. |
| `headers` | no | empty | HTTP headers for `http`/`sse`. |
| `cwd` | no | empty | Working directory for a stdio server, where supported. |
| `timeout` | no | empty | Units vary by target: milliseconds on most. |
| `oauth` | no | empty | OAuth settings. The shape is target-specific; see the target page. |
| `disabled` | no | `false` | See [`disabled` support by target](#disabled-support-by-target). |
| `roots` | no | empty | List of `{uri, name}` objects, for targets that support MCP roots. |

These fields apply only to the listed targets and are ignored elsewhere.

| Target | Extra fields |
|--------|--------------|
| [Codex](@/docs/targets/codex.md) | `env_vars`, `env_http_headers`, `http_headers_helper`, `auth`, `required`, `startup_timeout_sec`, `startup_timeout_ms`, `tool_timeout_sec`, `default_tools_approval_mode`, `scopes`, `oauth_resource`, `experimental_environment`, `enabled_tools`, `disabled_tools` |
| [Crush](@/docs/targets/crush.md) | `enabled_tools`, `disabled_tools`, `sessionless` |
| [Gemini](@/docs/targets/gemini.md) | `trust`, `includeTools`, `excludeTools` |
| [Qoder](@/docs/targets/qoder.md) | `trust`, `includeTools`, `excludeTools`, `alwaysAllow` |
| [Kiro](@/docs/targets/kiro.md) | `autoApprove`, `disabledTools`, `oauthScopes` |
| [Factory](@/docs/targets/factory.md) | `disabledTools`, `connectTimeout` |
| [Claude Code](@/docs/targets/claude.md) | `alwaysLoad`, `headersHelper` |
| [Cursor](@/docs/targets/cursor.md) | `envFile`, `auth` |
| [Copilot / VS Code](@/docs/targets/copilot.md) | `envFile`, `dev`, `sandboxEnabled` (VS Code file only), `tools` (Copilot CLI files only) |
| [Continue](@/docs/targets/continue.md) | `connectionTimeout`, `requestOptions` |
| [OpenHands](@/docs/targets/openhands.md) | `auth: oauth`, or a truthy `oauth`, which sets `auth: "oauth"` on a remote server in `~/.openhands/mcp.json` |

On Amp, set `x-amp.includeTools`. Use `x-factory`, `x-kilo`, or `x-continue` to override the matching top-level options for that target.

### `disabled` support by target

Only the targets listed were checked.

| Target | Behavior |
|--------|----------|
| [Antigravity](@/docs/targets/antigravity.md), [Crush](@/docs/targets/crush.md), [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md), [Qoder](@/docs/targets/qoder.md), [Windsurf](@/docs/targets/windsurf.md) | Native `disabled` |
| [Codex](@/docs/targets/codex.md) | Mapped to `enabled = false` |
| [Kilo Code](@/docs/targets/kilo.md), [OpenCode](@/docs/targets/opencode.md), [Zed](@/docs/targets/zed.md) | Mapped to `"enabled": false` |
| [Copilot](@/docs/targets/copilot.md) | A `disabledMcpServers` entry in `.github/copilot/settings.json` for Copilot CLI. Stripped from both MCP files with a note; disable the server in VS Code for that half |
| [Claude Code](@/docs/targets/claude.md) | Mapped to project `disabledMcpjsonServers` in `.claude/settings.json` for servers emitted to `.mcp.json`. Import restores it |
| [Cursor](@/docs/targets/cursor.md), [Augment](@/docs/targets/augment.md), [Junie](@/docs/targets/junie.md), [Trae](@/docs/targets/trae.md), [Warp](@/docs/targets/warp.md) | Stripped with a note. Disable the server in the tool itself |

## Commands

Markdown with optional YAML frontmatter. Each spec becomes one native slash command.

```markdown
---
name: deploy
description: Deploy the app to staging.
argument-hint: <env>
---

Deploy the app to {{env}}.
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Command identifier and slash name, such as `/deploy`. |
| `description` | no | empty | One-liner shown in slash-command pickers. |
| `argument-hint` | no | empty | Hint shown after the command, on Claude Code, Augment, and Factory. |

Any other frontmatter passes through. Put target-specific keys under `x-<target>`, for example `x-claude.allowed-tools`. Codex emits commands only when `outputs.codex.commands-dir` is set. Targets without a command surface log a warning and skip.

## Settings

Pure YAML, one file per settings group. Agent permissions and the default model live here instead of in each tool's settings file.

```yaml
permissions:
  allow:
    - Bash(go test:*)
  deny:
    - Bash(rm:*)
  ask:
    - Bash(git push:*)
model: claude-opus-4-8
effort:
  claude: xhigh
  default: high
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `permissions.allow` | no | empty | Rules approved without prompting. |
| `permissions.deny` | no | empty | Rules always blocked. |
| `permissions.ask` | no | empty | Rules that prompt before running. |
| `permissions.default-mode` | no | unset | Claude Code starting mode for `sync --global`: `default`, `manual`, `acceptEdits`, `plan`, `auto`, `dontAsk`, or `bypassPermissions`. Other targets raise a coverage note. |
| `model` | no | empty | Default model: a string, or a map per target with an optional `default`, like [agent `model`](#per-target-model-and-effort). A target with no entry and no `default` gets no model. |
| `effort` | no | empty | Default reasoning effort: a scalar, or a map per target with an optional `default`, like [agent `effort`](#per-target-model-and-effort). Separate from an agent's own `effort`. |

A rule is a bare tool name (whole tool) or `Scope(argument)`. An MCP tool is `mcp__<server>__<tool>`. `Scope()` with an empty argument is dropped, not read as the bare tool, which would widen it.

Keep a `Bash` wildcard at the end of an `allow` or `deny` rule. `Bash(git * main)` also approves options inserted at the `*`, and Claude Code matches a mid-command `*` in a `deny` rule literally, so it blocks nothing. `agnostic-ai lint` reports both as LINT009.

Multiple files merge: permission lists concatenate, de-duplicated in source order, and the last non-empty `model` and `effort` win. Each target resolves its own map entry first, so `model: {codex: gpt-6-luna}` in a later file changes only Codex.

`sync --global` also reads settings specs from the home, for `model`, `effort`, target-specific keys, and `permissions.default-mode`; see [default model and effort](@/docs/configuration.md#global-default-model-and-effort).

`effort` reaches four targets, each under its own key. A value the target does not accept is not written and raises a coverage note; the other targets still emit. `x-<target>` wins, so `x-claude.effortLevel` overrides the portable value. Every other target reports a coverage note.

`import` fills `effort` from these keys when the target accepts the value and no other settings spec sets it; otherwise the value lands under `x-<target>`.

| Target | Native key | Accepted values |
|---|---|---|
| Claude Code | `effortLevel` in `.claude/settings.json` | `low`, `medium`, `high`, `xhigh` |
| Copilot | `effortLevel` in `.github/copilot/settings.json` | `low`, `medium`, `high`, `xhigh` |
| Codex | `model_reasoning_effort` in `.codex/config.toml` | any string; `outputs.codex.config.model-reasoning-effort` and the captured overlay win |
| Factory | `reasoningEffort` in `.factory/settings.json` | `none`, `dynamic`, `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`; each model accepts a subset |

| Target | `permissions` | `model` |
|---|---|---|
| Claude Code, Qoder, Kilo Code, OpenCode | yes | yes |
| Factory | `Bash` rules only | yes |
| Windsurf | yes | no |
| Augment | `allow` and `deny` only | no |
| Codex, Copilot, Junie, Gemini | no | yes |

Every other target takes neither. A field a target cannot represent produces a coverage note while the others still emit. Copilot, Junie, and Codex report the whole policy (Codex's note points at `outputs.codex.exec-policies`). Rules translate only as far as the vendor allows: Augment gates `read`, `edit`, and `write` as whole tools, and Factory's command lists take shell patterns, so a path-scoped rule raises a note instead of widening. Factory's `commandDenylist` prompts, so portable `ask` goes there and `deny` goes to `commandBlocklist`. Review an imported `model` before enabling more targets, since identifiers differ between vendors.

Target-specific keys go under `x-<target>` and merge into that target's settings file (`x-factory.sandbox` reaches `.factory/settings.json`). Codex is the exception: its `.codex/config.toml` comes from the captured overlay plus `outputs.codex.config`, so an `x-codex` block raises a coverage note naming both routes.

On a key this tool also writes, lists union (translated entries first), objects merge recursively, and a scalar such as `model` is replaced. A list against a string cannot merge: the `x-<target>` value wins with a coverage note. Maps of whole records (`x-qoder.mcpServers`, `x-augment.mcpServers`) merge by name, and a server both sides name comes from the `x-<target>` block entire (#974). Fixed-order blocks (`x-qoder.hooks`, `x-augment.hooks`) keep their order, with your own events appended (#976).

Four keys take one shape each: `x-augment.toolPermissions` a list; `x-windsurf.permissions`, `x-kilo.permission`, `x-opencode.permission` an object. Another shape is skipped, the translated rules ship, and a coverage note names the key (#976).

## Reviews

Markdown with optional YAML frontmatter, one file per group of code-review-bot guidance.

```markdown
---
scope: backend
---

Flag any handler that talks to the database directly instead of going through a repository.
```

Reviews honor `scope` and the source layout like rules do. Specs with the same scope concatenate into one review file, written as a plain body without frontmatter. [Cursor](@/docs/targets/cursor.md) (Bugbot), [Codex](@/docs/targets/codex.md) (code review), and [Goose](@/docs/targets/goose.md) support reviews; other targets report them as unsupported. For Codex the text lands in a `## Code Review Rules` section of the root or scoped `AGENTS.md`, which every `AGENTS.md` reader loads; a `targets:` filter that omits `codex` keeps a spec out of it.

## Environments

Pure YAML, one file per environment group. It describes how a coding agent boots the dev environment: install dependencies, start services, forward ports, open terminals.

```yaml
install: go mod download
terminals:
  - name: dev
    command: go run ./cmd/agnostic-ai
```

`setup` holds the commands a tool runs in a new worktree (one command or a list); `setup-windows` replaces it on Windows.

`dev-commands` lists dev servers a tool can start and preview. Each entry needs a unique `name` and a `command` (a string or a list of words); `cwd` (relative to the project root), `port`, `auto-port`, `env`, and `url` are optional. A string command with shell syntax (pipe, several lines, `VAR=value`, a builtin like `cd`) runs through `sh -c`, which on Windows needs a POSIX shell such as Git Bash on the `PATH`. A list runs with no shell. `env` values are written as text.

```yaml
name: dev
setup: bash scripts/setup-worktree.bash
dev-commands:
  - name: Docs
    command: [pnpm, dev:mintlify]
    cwd: apps/docs
    port: 3000
    auto-port: true
```

Specs merge by top-level key, and the last value wins.

- [Cursor](@/docs/targets/cursor.md): `setup` and `setup-windows` go to `.cursor/worktrees.json`. The rest goes to `environment.json`, except the routing fields (`name`, `scope`, `target(s)`, `target(s)-exclude`, `description`) and `dev-commands`, which get a no-effect note.
- [Claude Code](@/docs/targets/claude.md): `dev-commands` goes to `.claude/launch.json`; every other field gets a no-effect note. Run worktree setup from a `WorktreeCreate` or `SessionStart` [hook](#hooks) instead.
- [OpenHands](@/docs/targets/openhands.md) and [Amp](@/docs/targets/amp.md): `install` becomes a setup script. Amp also turns `terminals` into services. Both note `setup` and `dev-commands` as having no effect.
- Other targets report the spec as unsupported.

`lint` reports a dev command with no `name` or `command`, a repeated name, an unknown key, or a wrong-typed value (LINT016).

## Ignore

Markdown with optional YAML frontmatter, one file per group. The body holds gitignore-syntax patterns for what an agent must not read or index.

````markdown
Secrets and build artifacts the agent should never read.

```gitignore
*.env
secrets/
dist/
```
````

With fenced code blocks, only the lines inside them are patterns and the surrounding text is prose, which formatters like Prettier can rewrite safely. A body without a fence is read whole. Prettier strips trailing spaces inside a block: write a name ending in a space as `name[ ]`.

Specs concatenate into each target's native ignore file under a `#` provenance header, with a blank line between them. Order and whitespace are kept; CRLF becomes LF. Override the path with `outputs.<target>.ignore-file`. Targets without an ignore file report the spec as unsupported.

### Overwrite behaviour

Sync replaces an ignore file with no agnostic-ai provenance header only when every existing exclusion survives, unchanged and in order. Extra patterns are allowed. Missing or reordered patterns, added negations (`!pattern`), and changed whitespace fail with `AAI-103` and leave the file untouched. The check is conservative, so an equivalent rewrite can still fail.

Run `agnostic-ai import <target>` to copy the patterns into `ignore/<target>.md` as a fenced block, with comments, order, and whitespace intact and a leading UTF-8 byte-order mark dropped. The spec sets `target: <target>`; remove the line to share the patterns with every ignore-capable target. Review the combined order if other specs add negations or reorder patterns.

Comment and blank lines exclude nothing. `#` starts a comment only at the start of a line. `outputs.<target>.provenance-header: false` removes the marker and disables this check. Dry-run skips the check; `sync --check` still reports unsafe overwrites.

## Frontmatter rules

- YAML between two `---` lines at the top of the file.
- Empty frontmatter (`---\n---\n`) means no metadata.
- A file without frontmatter still loads; its name defaults to the filename.
- Malformed frontmatter counts as no metadata, and the whole file becomes the body.
- Fields not listed on this page pass through on emit.

## Path variables: `{{$NAME}}`

A spec body can name a directory without hardcoding one target's layout. `{{$SKILLS_DIR}}` expands to `.claude/skills` for claude, `.agents/skills` for codex, and `.github/skills` for copilot.

| Variable | Resolves to |
|---|---|
| `{{$SKILLS_DIR}}` | the target's skills directory |
| `{{$AGENTS_DIR}}` | the target's agents directory |
| `{{$COMMANDS_DIR}}` | the target's commands directory |
| `{{$RULES_DIR}}` | the target's rules directory |
| `{{$MCP_FILE}}` | the target's MCP config file |

- **Bodies only.** Frontmatter values do not expand.
- **An `outputs.<target>.<field>` override wins.** With `outputs.claude.skills-dir: custom/skills`, `{{$SKILLS_DIR}}` follows it.
- **A variable with no target surface stays verbatim** and raises a coverage note. aider and jules resolve no variables.
- **Only where the target has a dedicated surface.** Targets that flatten agents into rules (continue, trae, windsurf) or commands (gemini) declare no `{{$AGENTS_DIR}}`. Antigravity, Goose, and OpenHands resolve it to `.agents/agents`.
- **The `$` sigil is required.** Plain `{{placeholder}}` stays untouched, so Warp workflow arguments and Handlebars or Jinja survive. Lowercase names such as `{{$skills_dir}}` do not resolve.

## Target-specific extensions: `x-<target>` namespace

Use an `x-<target>:` block for fields only one adapter reads. Other adapters strip it, so the spec stays portable.

```markdown
---
name: code-reviewer
description: Reviews diffs.
model: sonnet
x-claude:
  allowed-tools: [Read, Grep, Bash]
x-cursor:
  globs: "src/**"
  alwaysApply: false
---
```

For each target, all `x-*` keys are dropped, then the matching `x-<target>` block is flattened into the top level, overriding keys with the same name.

| Target | Resulting frontmatter |
|--------|-----------------------|
| `claude` | `name`, `description`, `model`, `allowed-tools` |
| `cursor` | `name`, `description`, `model`, `globs`, `alwaysApply` |
| `gemini` | `description` (`name` becomes the `.toml` filename; `model` is not emitted for commands) |

### Arbitrary custom keys

Any other key under `x-<target>` emits verbatim into that target's output, in sorted order, and never leaks across targets. Validate them against the target's schema yourself. Each target page lists the keys its adapter manages. A target with no surface for a spec kind drops custom keys for that kind. Gemini TOML accepts only a string, bool, number, or string array, and skips nested tables.

On a settings spec the block merges into the target's settings file by the rules in [Settings](@/docs/spec-format.md#settings). Codex takes no settings block and says so in a coverage note.
