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
├── backend/
│   └── auth.md                  # scope: "backend"
└── backend/api/
    └── limits.md                # scope: "backend/api"
```

For rules, scope controls native activation or directory discovery, not only file organization. A flat rule may set `scope: services/payments`; source-layout scope takes precedence.

```bash
agnostic-ai new rule payments-context --scope services/payments
```

Scoped bodies stay out of root instruction appendices. Supported targets get native path conditions or a nested instruction file. Unsupported targets skip the rule with a warning, or fail under `on-unsupported: error`. See [directory-specific instructions](@/docs/scoped-context.md) for the target matrix and selector limits.

## Agents

```markdown
---
name: code-reviewer
description: Reviews diffs for bugs, style, and architectural issues.
tools: [Read, Grep, Bash]
model: sonnet
---

You are a code reviewer. Examine the diff for:
- Logic bugs and edge cases
- Style consistency
- Security issues

Report concise findings with `file:line` references.
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename without `.md` | Agent identifier and output filename. |
| `description` | no | empty | One-liner shown in tool listings. |
| `tools` | no | unset | Tools the agent may invoke. See [`tools` support by target](#tools-support-by-target). |
| `model` | no | unset | Preferred model: a string for every target, or a map per target. |
| `color` | no | unset | Badge color. See [`color` support by target](#color-support-by-target). |
| `memory` | no | unset | Persistent memory scope for the agent: `user`, `project`, or `local`. |

Any other frontmatter field passes through unchanged.

`memory` gives the agent a directory that survives across sessions. Only [Claude Code](@/docs/targets/claude.md#agent-memory) acts on it today, where `project` is the scope git carries. Junie passes the key through; every other adapter drops it.

### Per-target models

`model:` takes a string or a map keyed by target name, with an optional `default`.

```yaml
---
name: code-reviewer
model:
  claude: opus
  codex: gpt-5.5
  default: gpt-4o
---
```

For each target, the matching key wins, then `default`. With neither, no `model` line is written and the tool uses its own default. `x-<target>.model` beats the map, and `x-<target>.model: null` deletes it.

| Want | Write |
|------|-------|
| Same model everywhere | `model: sonnet` |
| Per target, with a fallback | `model: {claude: sonnet, default: gpt-4o}` |
| Per target, tool default elsewhere | `model: {claude: sonnet}` |

### `tools` support by target

Only the targets listed were checked. `tools: [Read, Bash]` does not restrict every target. A target that cannot honor the field prints a coverage note at sync time, so `tools: [Read]` never silently becomes an unrestricted agent.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Copilot](@/docs/targets/copilot.md), [Junie](@/docs/targets/junie.md) | Passed through as a YAML list |
| [Qoder](@/docs/targets/qoder.md), [Trae](@/docs/targets/trae.md) | Passed through as a comma-separated string (`tools: Read, Bash`) |
| [Windsurf](@/docs/targets/windsurf.md), [Kiro](@/docs/targets/kiro.md), [Factory](@/docs/targets/factory.md), [Gemini](@/docs/targets/gemini.md) | Translated to native names |
| [Antigravity](@/docs/targets/antigravity.md), [OpenHands](@/docs/targets/openhands.md), [Goose](@/docs/targets/goose.md), [Codex](@/docs/targets/codex.md), [Cursor](@/docs/targets/cursor.md), [Augment](@/docs/targets/augment.md), [Kilo Code](@/docs/targets/kilo.md) | Dropped with a note |

Translation can widen access: on Kiro, `Edit` alone also permits `delete_file`. Most targets accept native names through `x-<target>.tools`, which bypasses translation.

### `mcpServers` support by target {#mcpservers-support-by-target}

Only the targets listed were checked. A top-level `mcpServers` list narrows which MCP servers one agent may reach. Omitting it inherits the session's full set on every target below.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md) | Server names, written to the agent file |
| [Junie](@/docs/targets/junie.md) | Server names, written to the agent file |
| [Qoder](@/docs/targets/qoder.md) | Server names or inline objects, written to the agent file |
| [Factory](@/docs/targets/factory.md) | Server names, written to the droid file |
| [OpenHands](@/docs/targets/openhands.md) | Inline server definitions only. Set `x-openhands.mcp_servers` |
| [Antigravity](@/docs/targets/antigravity.md) | Inline server objects only. Set `x-antigravity.mcpServers` |

Two shapes, not one. Claude, Junie, Qoder, and Factory reference servers already configured elsewhere by name, and each writes its own agent file; OpenHands and Antigravity embed the server definition inline. A name list cannot be rewritten into an inline definition without inventing the server's transport, so the two groups stay apart.

**An empty list is not portable.** Junie documents `mcpServers: []` as keeping every configured server available, and Factory documents it as excluding every server, "even globally configured ones". The same two characters mean opposite things, so write the servers you want rather than an empty list.

### `effort` support by target {#effort-support-by-target}

Only the targets listed were checked. A top-level `effort` on an agent asks for deeper reasoning on that agent alone, leaving routine delegated work cheaper. Omitting it inherits the session's level everywhere below.

| Target | Values |
|--------|--------|
| [Claude Code](@/docs/targets/claude.md) | `low`, `medium`, `high`, `xhigh`, `max`, model dependent |
| [Qoder](@/docs/targets/qoder.md) | The same five names, or a positive integer budget |
| [Factory](@/docs/targets/factory.md) | Emitted as `reasoningEffort`. Only `low`, `medium`, `high`; anything wider is dropped with a note |
| [Cursor](@/docs/targets/cursor.md) | No frontmatter key. Write it into the model id: `model: claude-opus-5[effort=high]` |

Cursor is the reason this is not one key everywhere. It encodes per-model options inside the model string rather than as a field, so the same intent is a frontmatter key on three targets and a model-id transformation on the fourth. Factory also ignores the field entirely under `model: inherit`.

### `permissionMode` and agent `hooks` support by target {#agent-policy-support-by-target}

Only the targets listed were checked. Both fields narrow one delegated agent: `permissionMode` sets its approval boundary, `hooks` scopes lifecycle hooks to it. Omitting either inherits the parent session.

| Target | `permissionMode` | Agent `hooks` |
|--------|------------------|---------------|
| [Claude Code](@/docs/targets/claude.md) | Seven values, `manual` aliases `default` | Written to the agent file |
| [Qoder](@/docs/targets/qoder.md) | Six values, another one is reported | Seven events, a wider one is reported |
| [OpenHands](@/docs/targets/openhands.md) | Different names entirely. Set `x-openhands` | Six snake_case events, command handlers only. Set `x-openhands` |

OpenHands names the same intent `always_confirm`, `never_confirm`, and `confirm_risky`, so there is no shared value space to translate into, and its agent file is the shared `.agents/agents/` tree besides. Its route stays `x-openhands`.

Two Qoder behaviors worth knowing before relying on either field. `bypassPermissions` is not always what runs: "Skip permission prompts. If security policy disables it, it is demoted to `acceptEdits`." And a subagent runs a narrower event set than the project hook file's 27, so an event valid at project scope can be inert at agent scope.

### `color` support by target

Only the targets listed were checked. agnostic-ai writes `color` verbatim and does not validate it. A value the target does not recognize is cosmetic: the agent still runs.

| Target | Values |
|--------|--------|
| [Augment](@/docs/targets/augment.md) | Free text, an ANSI color name |
| [Kilo Code](@/docs/targets/kilo.md) | Hex or a theme token |
| [Qoder](@/docs/targets/qoder.md) | One of eight names |
| [OpenHands](@/docs/targets/openhands.md) | Dropped with a note. Set `x-openhands.color` to a [Rich color name](https://rich.readthedocs.io/en/stable/appendix/colors.html) |

`color: blue` is valid on Augment and Qoder but is neither hex nor a Kilo Code theme token.

OpenHands documents `color` on a project agent, but its agent files live in `.agents/agents/`, a tree it shares byte-for-byte with [Goose](@/docs/targets/goose.md), whose frontmatter has no such key. A portable `color` prints a coverage note there instead of reaching the file.

## Skills

Two layouts:

- **Flat:** `skills/yaml-validator.md`
- **Nested**, for skills with attached resources: `skills/yaml-validator/SKILL.md` next to `skills/yaml-validator/schema.yaml`

### `disable-model-invocation` support by target {#disable-model-invocation-support-by-target}

Only the targets listed were checked. Setting it keeps a skill out of automatic model invocation; the user can still invoke it. Omitting it leaves each target's own default, which is model-invocable everywhere below.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md) | Written to `SKILL.md` |
| [Cursor](@/docs/targets/cursor.md) | Written to `SKILL.md` |
| [Crush](@/docs/targets/crush.md) | Dropped with a note. Set `x-crush.disable-model-invocation` |
| [Factory](@/docs/targets/factory.md) | Dropped with a note. Set `x-factory.disable-model-invocation` |

Crush and Factory document the field, but their skills land in the shared `.agents/skills/` tree, written byte-for-byte across every co-writer. Emitting the key for them would also hand it to the targets sharing that path whose frontmatter has no such field, so the drop is reported instead of hidden. A skill marked manual-only becoming model-invocable is a safety boundary, not a cosmetic loss.

Do not confuse this with OpenHands' `triggers`, which is a keyword list that injects a skill when a phrase appears in a user message, not a restriction on who may invoke it. Devin spells the same restriction `triggers: [user]`, so the word means opposite things on the two targets.

Only `SKILL.md` and flat `skills/*.md` parse as skills. Every other file in a nested skill directory is a bundled asset: scripts, templates, fixtures, subdirectories, and extra `*.md` such as `examples.md`. Assets copy verbatim to the same relative path under each target's skills dir. Ship a `check.mjs`, `templates/*.tpl`, or `fixtures/*.json` that the body references. Import and sync preserve executable bits.

```markdown
---
name: yaml-validator
description: Validate YAML against a schema.
---

# YAML Validator

## Steps
1. Read target file
2. Parse YAML
3. Compare against `schema.yaml`
4. Report violations as `path: message`
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | dir or filename | Skill identifier and output directory. Some targets restrict the format. |
| `description` | no | empty | One-liner the model uses to decide whether to invoke the skill. |

Most targets write one native folder per skill at `<dir>/<name>/SKILL.md`, with bundled assets. Several share `.agents/skills/`, so identical bytes write once. Targets with no skill surface flatten it to a `skill-<name>.md` rule file and raise a coverage note, since assets cannot follow. Set `outputs.<target>.emit-skills-as-commands: true` to also emit a slash command. The [Skills notes](@/docs/targets/_index.md#capability-matrix) and each target page give the exact directory.

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
| `scope` | no | project-wide | Project-relative directory and its descendants. Source-layout scope takes precedence. See [scoped context](@/docs/scoped-context.md). |
| `globs` | no | target-dependent; `new rule` seeds `**/*` | Project-relative file patterns. With `scope`, the selector must stay inside the directory. `new rule --scope` omits it. |
| `paths` | no | unset | File patterns, as a string or list. Scoped rules accept it with or instead of `globs`; see [selector limits](@/docs/scoped-context.md#narrow-a-rule-to-certain-files). |
| `alwaysApply` | no | target-dependent; `new rule` seeds `true` | Requests unconditional activation. With `scope`, only inside the directory. `new rule --scope` omits it. |

## Hooks

Pure YAML, no markdown body.

```yaml
name: format-on-save
description: Run formatter after Edit/Write tools modify files.
event: PostToolUse
matcher: "Edit|Write"
command: "npx prettier --write \"$CLAUDE_FILE_PATHS\""
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Hook identifier. |
| `description` | no | empty | Free-form documentation. |
| `event` | yes | none | Hook event, written verbatim. See [events](#events). |
| `matcher` | no | empty | Regex on the tool name, or another event-specific selector. |
| `command` | command handlers only | none | Shell command, or a list where each entry becomes its own handler. |
| `args` | no | empty | Argument list. Switches to **exec form**: `command` runs as an executable with `args` as its argument vector and no shell, so spaces, apostrophes, `$`, and backticks pass through verbatim. Leave it unset when the command needs a pipe or `&&`. |
| `type` | no | `command` | Handler type: `command`, `http`, `mcp_tool`, or `prompt`, where the target supports it. |
| `timeout` | no | none | Seconds before the tool cancels the hook. Some targets convert to milliseconds or apply their own default. |
| `disabled` | no | `false` | Keep the hook defined but stop it running. Antigravity and Kiro write `enabled: false`; other targets emit the hook unchanged. |

Handler-specific and tool-specific fields emit only where the target's schema defines them, and other targets ignore them.

| Fields | Targets |
|--------|---------|
| `server`, `tool`, `input` (for `type: mcp_tool`) | [Claude Code](@/docs/targets/claude.md), [Codex](@/docs/targets/codex.md) |
| `url`, `headers`, `allowedEnvVars` (HTTP handler) | [Claude Code](@/docs/targets/claude.md), [Copilot](@/docs/targets/copilot.md) |
| `prompt`, `model` (prompt handler) | [Claude Code](@/docs/targets/claude.md), [Cursor](@/docs/targets/cursor.md), [Copilot](@/docs/targets/copilot.md) (`sessionStart` only) |
| `continueOnBlock` | [Claude Code](@/docs/targets/claude.md) |
| `statusMessage`, `async` | [Claude Code](@/docs/targets/claude.md), [Codex](@/docs/targets/codex.md), [Qoder](@/docs/targets/qoder.md) |
| `asyncRewake`, `shell`, `if` | [Claude Code](@/docs/targets/claude.md), [Qoder](@/docs/targets/qoder.md) |
| `commandWindows`, `additionalContextLimit` | [Codex](@/docs/targets/codex.md) |
| `failClosed` | [Cursor](@/docs/targets/cursor.md) |
| `loop_limit` | [Cursor](@/docs/targets/cursor.md), [Trae](@/docs/targets/trae.md) |
| `x-goose.on_failure` | [Goose](@/docs/targets/goose.md) |
| `x-kiro.action` | [Kiro](@/docs/targets/kiro.md) |
| `x-gemini.hooks`, `x-gemini.sequential`, `x-gemini.name`, `x-gemini.env` | [Gemini](@/docs/targets/gemini.md) |

`command` is not needed for a non-command handler, a valid `x-kiro.action`, or a hook that sets `x-gemini.hooks`. Scope a non-command hook to the targets that support it with `target` or `targets`.

### Events

agnostic-ai writes `event` verbatim and never translates event names between tools. Claude Code and Codex share `PreToolUse`, `PostToolUse`, and `UserPromptSubmit`, so one spec feeds both. Other tools need their own names, such as Cursor's `beforeShellExecution` or Gemini's `BeforeTool`. `agnostic-ai validate` flags an event a target does not recognize. Each target page lists its events, file, and wrapper shape. Targets without hook support log a warning and skip.

### Per-target body fences

When one spec needs different prose per target, wrap the divergent part in `::target` fences. Content outside a fence emits everywhere; content inside emits only to the listed targets. Marker lines never reach the output.

```md
---
name: test
description: Run the test suite
---

# Test

Shared intro paragraph.

::target claude
## Scope mapping (Claude)

| Changed | Command |
|---------|---------|
| ... | ... |
::end

::target codex
## Workflow (Codex)

1. Choose scope
2. Run `composer test`
::end

Shared outro.
```

| Syntax | Meaning |
|--------|---------|
| `::target <name>` | Opens a fence for one target. |
| `::targets <a> <b>` | Opens a fence for several targets. |
| `::end` | Closes the most recent fence. A missing `::end` runs to the end of the body. |

- The source view used by `import` round-trips keeps fences intact, so a re-emit stays byte-stable.
- `import codex` builds fences when Claude and Codex ship the same agent or skill with different bodies: the common prefix and suffix stay unfenced, and each tool's middle gets its own block.
- Fences also work in `.agnostic-ai/AGNOSTIC_AI.md`. A block reaches an entry-point file when any target that reads the file is listed. `AGENTS.md` is shared by the whole AGENTS.md family, so `::target codex` content reaches every reader of that file. A shared file is never split. See [Entry-point files](@/docs/configuration.md#entry-point-files).

## Target scoping

Four fields limit where a spec emits, for every kind: agents, skills, rules, commands, hooks, and MCP servers.

| Field | Effect |
|-------|--------|
| `target` | Emit only to this one target. |
| `targets` | Emit only to these targets. |
| `target-exclude` | Emit everywhere except this target. |
| `targets-exclude` | Emit everywhere except these targets. |

With none set, the spec emits to every target that supports its kind. `target` beats `targets` when both appear. Exclusion wins over inclusion. A `target: codex` agent emits only into `.codex/agents/`, and a `targets-exclude: [gemini]` skill emits everywhere but Gemini.

### Import auto-scoping

A hook imported from a tool sets `target` to that tool: `target: codex`, `target: claude`, or `target: gemini`. Delete the field to let the hook reach every target.

```yaml
event: PostToolUse
matcher: apply_patch|Edit|Write
command: "$(git rev-parse --show-toplevel)/.codex/hooks/format-php.sh"
target: codex   # shell-expanded codex path; do not leak to other tools
```

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

`name` is the server identifier, not the filename. It may contain package-style slashes, such as `npm:@modelcontextprotocol/server-sequential.thinking`. agnostic-ai percent-encodes such names in YAML filenames, and every generated config keeps the original. Other spec kinds need one safe path segment, because their names become output paths.

A server cannot work without `command` (stdio) or `url` (remote). `agnostic-ai lint` reports a missing one as LINT008. `validate` and `sync` do not: some targets drop the entry, others write a server that cannot start. See [lint](@/docs/cli-reference.md#lint).

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | yes | none | Server identifier and key in the generated config. |
| `description` | no | empty | Free-form documentation. Dropped on a target whose MCP schema has no such key (Junie, Warp). |
| `type` | no | `stdio` | Transport: `stdio`, `http`, `sse`, or `ws`. Remote transports write an explicit `type`; `stdio` stays implicit. A `ws` entry emits no server on Augment, Factory, and Qoder, whose vendors document no WebSocket transport carrying a `url`. |
| `command` | stdio only | none | Executable to launch. |
| `args` | no | empty | Argument list for the command. |
| `env` | no | empty | Environment variables for the server. |
| `url` | http/sse/ws only | none | Endpoint URL. |
| `headers` | no | empty | HTTP headers for `http`/`sse`. |
| `cwd` | no | empty | Working directory for a stdio server, where the target supports it. |
| `timeout` | no | empty | Units vary: milliseconds on most targets, seconds on OpenHands `http` servers. |
| `oauth` | no | empty | OAuth settings. The shape is target-specific; see the target page. |
| `disabled` | no | `false` | See [`disabled` support by target](#disabled-support-by-target). |
| `roots` | no | empty | List of `{uri, name}` objects, for targets that support MCP roots. |

Some fields apply only to certain targets and are ignored elsewhere.

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
| [OpenHands](@/docs/targets/openhands.md) | `api_key`, which turns the entry into `{ url, api_key }` |

On Amp, set `x-amp.includeTools`. Use `x-factory`, `x-kilo`, or `x-continue` to override the matching top-level options for that target.

### `disabled` support by target

Only the targets listed were checked. Native booleans default to `false`.

| Target | Behavior |
|--------|----------|
| [Antigravity](@/docs/targets/antigravity.md), [Crush](@/docs/targets/crush.md), [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md), [Qoder](@/docs/targets/qoder.md), [Windsurf](@/docs/targets/windsurf.md) | Native `disabled` |
| [Codex](@/docs/targets/codex.md) | Mapped to `enabled = false` |
| [Kilo Code](@/docs/targets/kilo.md), [OpenCode](@/docs/targets/opencode.md), [Zed](@/docs/targets/zed.md) | Mapped to `"enabled": false` |
| [Copilot](@/docs/targets/copilot.md) | Written as a `disabledMcpServers` entry in `.github/copilot/settings.json` for Copilot CLI. Stripped from both MCP files with a note, since neither reader has a per-server key; disable the server in VS Code for that half. |
| [Claude Code](@/docs/targets/claude.md), [Cursor](@/docs/targets/cursor.md), [Augment](@/docs/targets/augment.md), [Junie](@/docs/targets/junie.md), [Trae](@/docs/targets/trae.md), [Warp](@/docs/targets/warp.md) | Stripped with a note. Disable the server in the tool itself. |

## Commands

Markdown with optional YAML frontmatter. Each spec becomes one native slash command.

```markdown
---
name: deploy
description: Deploy the app to staging.
argument-hint: <env>
---

Deploy the app to {{env}}.

1. Run tests.
2. Build artifacts.
3. Push to the {{env}} environment.
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
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `permissions.allow` | no | empty | Rules approved without prompting. |
| `permissions.deny` | no | empty | Rules always blocked. |
| `permissions.ask` | no | empty | Rules that prompt before running. |
| `model` | no | empty | Default model. |

Multiple files merge: permission lists concatenate, de-duplicated in source order, and the last non-empty `model` wins. Claude Code and Qoder take `permissions` and `model`; Codex, Copilot, OpenCode, Junie, and Kilo Code take `model` only. A field a target cannot represent produces a coverage note while the others still emit. Model identifiers differ between vendors, so review an imported `model` before enabling more targets.

## Reviews

Markdown with optional YAML frontmatter, one file per group of code-review-bot guidance.

```markdown
---
scope: backend
---

Flag any handler that talks to the database directly instead of going through a repository.
```

Reviews honor `scope` and the source layout like rules do. Specs with the same scope concatenate into that scope's one review file, written as a plain body without frontmatter. [Cursor](@/docs/targets/cursor.md) (Bugbot) and [Goose](@/docs/targets/goose.md) support reviews; other targets report them as unsupported.

## Environments

Pure YAML, one file per environment group. It describes how a coding agent boots the dev environment: install dependencies, start services, forward ports, open terminals.

```yaml
install: go mod download
terminals:
  - name: dev
    command: go run ./cmd/agnostic-ai
```

Specs merge by top-level key, and the last value wins. [Cursor](@/docs/targets/cursor.md) writes the spec as its `environment.json`, passing every key through except the routing fields (`name`, `scope`, `target(s)`, `target(s)-exclude`, `description`). [OpenHands](@/docs/targets/openhands.md) and [Amp](@/docs/targets/amp.md) turn `install` into a setup script, and Amp turns `terminals` into services. Other targets, such as devcontainers or Codex setup scripts, report the spec as unsupported.

## Ignore

Markdown with optional YAML frontmatter, one file per group. The body holds gitignore-syntax patterns for what an agent must not read or index.

```markdown
# Secrets and build artifacts the agent should never read
*.env
secrets/
dist/
```

Specs concatenate into each target's native ignore file under a `#` provenance header, separated by a blank line. Pattern order and whitespace are kept; outer line breaks are trimmed and CRLF becomes LF. Override the path with `outputs.<target>.ignore-file`. Targets without an ignore file report the spec as unsupported.

### Overwrite behaviour

Sync replaces an ignore file without an agnostic-ai provenance header only when every existing exclusion survives. Each pattern must stay unchanged and in order. Extra patterns are allowed. Missing or reordered patterns, added negations (`!pattern`), and changed whitespace fail with `AAI-103` and leave the file untouched. The check is conservative, so an equivalent rewrite can still fail.

```
.kiroignore: hand-authored ignore file cannot be safely overwritten: existing
patterns are missing or reordered: "my-secrets/", "*.key". Run
`agnostic-ai import kiro` to copy its patterns into an ignore spec, then keep
their order and review any added negations before syncing again.
```

`agnostic-ai import <target>` reads the file into `ignore/<target>.md` with comments, order, and whitespace intact. It drops a leading UTF-8 byte-order mark so the header does not turn it into a pattern character. Unchanged imported patterns sync with no cleanup. If other specs add negations or reorder the imported patterns, review the combined order first. Generated files still regenerate from their specs, including intentional removals.

Comment and blank lines exclude nothing, so a file with only those never blocks a sync. `#` starts a comment only at the start of a line; leading spaces and tabs can belong to a pattern. `outputs.<target>.provenance-header: false` removes the marker and disables this check. Dry-run skips the check because it writes nothing; `sync --check` still reports unsafe overwrites.

## Frontmatter rules

- YAML between two `---` lines at the top of the file.
- Empty frontmatter (`---\n---\n`) means no metadata.
- A file without frontmatter still loads; its name defaults to the filename.
- Malformed frontmatter counts as no metadata, and the whole file becomes the body.
- Fields not listed on this page pass through on emit.

## Path variables: `{{$NAME}}`

A spec body can name a directory without hardcoding one target's layout. `{{$SKILLS_DIR}}` expands to `.claude/skills` for claude, `.agents/skills` for codex, and `.github/skills` for copilot.

```markdown
Put new skills in {{$SKILLS_DIR}} and agent profiles in {{$AGENTS_DIR}}.
```

| Variable | Resolves to |
|---|---|
| `{{$SKILLS_DIR}}` | the target's skills directory |
| `{{$AGENTS_DIR}}` | the target's agents directory |
| `{{$COMMANDS_DIR}}` | the target's commands directory |
| `{{$RULES_DIR}}` | the target's rules directory |
| `{{$MCP_FILE}}` | the target's MCP config file |

- **Bodies only.** Every spec body expands; frontmatter values do not.
- **An `outputs.<target>.<field>` override wins.** With `outputs.claude.skills-dir: custom/skills`, `{{$SKILLS_DIR}}` follows it.
- **A variable with no target surface stays verbatim** and raises a coverage note, so "see {{$COMMANDS_DIR}}" never becomes "see ". aider and jules resolve no variables.
- **A variable exists only where the target has a dedicated surface.** Targets that flatten agents into rules (continue, trae, windsurf) or commands (gemini) declare no `{{$AGENTS_DIR}}`. Antigravity, Goose, and OpenHands resolve it to `.agents/agents`.
- **The `$` sigil is required.** Plain `{{placeholder}}` stays untouched, so Warp workflow arguments and quoted Handlebars or Jinja survive. Lowercase names such as `{{$skills_dir}}` do not resolve.

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

Any other key under `x-<target>` emits verbatim into that target's output. That block is the opt-in: shared top-level keys stay stripped, so plain specs keep producing valid files. Keys emit in sorted order and never leak across targets. Validate them against the target's schema yourself.

Each adapter manages some keys itself, and the target page lists them. A target with no surface for a spec kind drops custom keys for that kind. Gemini TOML accepts only a string, bool, number, or string array, and skips nested tables.
