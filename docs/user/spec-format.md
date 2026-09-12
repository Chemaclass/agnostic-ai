# Spec format

[User docs](README.md)

Source paths below are relative to `.agnostic-ai/` by default. For example, `rules/*.md` means `.agnostic-ai/rules/*.md`. Override directories with [`sources`](configuration.md#sources).

Start with a [rule](#rules) for conventions, a [skill](#skills) for a reusable workflow, or an [MCP server](#mcp-servers) for a tool connection. See [Getting started](getting-started.md) for a complete first-rule example.

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

Discovery is recursive. Every `.md` under `agents/`, `skills/`, `rules/`, `commands/`, `reviews/`, `ignore/` is picked up; every `.yaml` under `hooks/`, `mcps/`, `settings/`, and `environments/`.

## Nested layout: per-directory scope

A spec in a subdirectory of its source dir carries an implicit **scope** equal to that subpath.

```
rules/
├── conventional-commits.md      # scope: ""    (root)
├── backend/
│   └── auth.md                  # scope: "backend"
└── backend/api/
    └── limits.md                # scope: "backend/api"
```

For rules, scope controls native activation or directory discovery. It does not merely organize generated files. A flat rule may set `scope: services/payments`; source-layout scope takes precedence. Create one with:

```bash
agnostic-ai new rule payments-context --scope services/payments
```

Scoped bodies are excluded from root instruction appendices. Supported targets receive native path conditions or a nested instruction file. Unsupported targets skip the rule with a warning, or fail under `on-unsupported: error`.

See [directory-specific instructions](scoped-context.md) for the complete target matrix, selector rules, runtime limits, and shared-reader compatibility.

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
| `name` | no | filename without `.md` | Agent identifier. Used for the output filename. |
| `description` | no | empty | One-liner shown in tool listings. |
| `tools` | no | unset | Tools the agent may invoke. Behavior varies by target; see [`tools` support by target](#tools-support-by-target) below. |
| `model` | no | unset | Preferred model. String applies everywhere; a map selects per target (see below). |

Any other frontmatter field passes through to the target CLI unchanged.

### Per-target models

`model:` accepts a string (same model everywhere) or a map keyed by target name with an optional `default` fallback.

```yaml
---
name: code-reviewer
model:
  claude: opus
  codex: gpt-5.5
  default: gpt-4o
---
```

Resolution per target: matching key wins, else `default`, else no `model` line is emitted (the CLI uses its own built-in default). An `x-<target>.model` override beats the map; `x-<target>.model: null` deletes it.

| Want | Write |
|------|-------|
| Same model everywhere | `model: sonnet` |
| Per-target, with a concrete fallback | `model: {claude: sonnet, default: gpt-4o}` |
| Per-target, native default elsewhere | `model: {claude: sonnet}` |

## Skills

Two layouts:

**Flat:** `skills/yaml-validator.md`

**Nested** (for skills with attached resources):
```
skills/yaml-validator/SKILL.md
skills/yaml-validator/schema.yaml
```

Only `SKILL.md` and flat `skills/*.md` are parsed as skills. Every other file inside a nested skill directory (scripts, templates, fixtures, subdirectories, and extra `*.md` such as `examples.md`) is a bundled asset: it copies verbatim to the same relative location under each target's skills dir and is never promoted to its own skill. Ship a `check.mjs`, `templates/*.tpl`, or `fixtures/*.json` the skill body references. Executable bits are preserved both directions through import + sync.

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
| `name` | no | dir or filename | Skill identifier. Used for the output directory. |
| `description` | no | empty | One-liner shown when the model decides whether to invoke the skill. |

Emission by target:

- **Native**, one folder per skill at `<dir>/<name>/SKILL.md`, carrying bundled assets verbatim. Most targets. Several share one tree at `.agents/skills/`, so identical bytes dedupe instead of writing another on-disk copy.
- **Flattened to a rule file** (`skill-<name>.md`) on the few targets with no skill surface. Bundled assets cannot follow, so those raise a coverage note.
- **Also as a slash command**, opt-in per target via `outputs.<target>.emit-skills-as-commands: true`.

Which target does which, and the exact directory each reads, is the [Skills row and cross-cutting bullet in targets](targets.md) — that list is kept current per change and this one is deliberately not a second copy of it.

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
| `scope` | no | project-wide | Project-relative directory and descendants. Source-layout scope takes precedence. Native routing and supported targets are listed in [scoped context](scoped-context.md). |
| `globs` | no | target-dependent; `new rule` seeds `**/*` | Project-relative file patterns. With `scope`, the selector must preserve the directory boundary. `new rule --scope` omits this field. |
| `paths` | no | unset | Project-relative file patterns, as a string or list. Scoped rules accept this alongside or instead of `globs`; see [selector limits](scoped-context.md#narrow-a-rule-to-certain-files). |
| `alwaysApply` | no | target-dependent; `new rule` seeds `true` | Requests unconditional activation. With `scope`, applies only within the directory boundary; sync chooses the required native flags. `new rule --scope` omits this field. |

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
| `event` | yes | none | Hook event. See list below. |
| `matcher` | no | empty | Regex on tool name (or other event-specific selector). |
| `command` | yes, unless `type: mcp_tool` or `x-gemini.hooks` is set | none | Shell command to run when triggered. |
| `args` | no | empty | Argument list. Claude Code, Qoder and Copilot. Setting it switches the hook to **exec form**: `command` is resolved as an executable and spawned directly with `args` as the argument vector, no shell involved, so spaces, apostrophes, `$`, and backticks pass through verbatim. Leave it unset for shell form, which is what you want when the command uses a pipe or `&&`. The targets spell the form differently. Claude Code and Qoder keep the executable in `command`, and Qoder ignores `shell` in exec form; Copilot moves it to `exec` and forbids carrying both, and its exec form runs under Copilot CLI only, so a hook that must also run under Copilot cloud agent leaves `args` unset. `sync` says so with a coverage note. |
| `type` | no | `command` | Set to `mcp_tool` for a Codex hook that calls a tool on an already-connected MCP server instead of running a shell command, in place of `command`. Codex. |
| `server` | yes, when `type: mcp_tool` | none | Name of the already-connected MCP server to call. Codex. |
| `tool` | yes, when `type: mcp_tool` | none | Name of the tool to call on that server. Codex. |
| `input` | no, `type: mcp_tool` only | empty | JSON object of argument templates for the tool call. Codex. |
| `timeout` | no | none | Seconds before the tool cancels the hook. Claude + Codex, both shapes, Kiro (`0` disables the timeout there instead of meaning immediate cancellation; kiro.dev's own default when the key is absent is 60), Qoder (default 600 when absent), Crush (default 30 when absent), and Factory (default 60 when absent). Augment and Gemini convert this value to **milliseconds** before writing it (vendor default 60000 when absent). |
| `statusMessage` | no | empty | Spinner message while the hook runs. Claude + Codex, both shapes, and Qoder. |
| `async` | no | `false` | Run in the background without blocking. Claude + Codex, and Qoder. |
| `asyncRewake` | no | `false` | Background run that wakes Claude on exit code 2 (implies `async`). Claude and Qoder. |
| `shell` | no | empty | `bash` or `powershell`. Claude and Qoder. |
| `if` | no | empty | Permission-rule filter (e.g. `Bash(git *)`) gating when the hook fires. Claude and Qoder. |
| `loop_limit` | no | none | How many times a `Stop` hook may block the agent from stopping before it is skipped. Trae only, `Stop` only (vendor default 5 when absent). |
| `commandWindows` | no | empty | Windows-specific command override. Codex. |
| `additionalContextLimit` | no | none | Token threshold for how much hook output reaches the model. Codex. Set `0` to pass the complete additional context. |
| `target` | no | empty | Single target name. Emits only there. |
| `targets` | no | empty | List of target names. Emits only to those. |
| `target-exclude` | no | empty | Single target name to block. Emits everywhere else. |
| `targets-exclude` | no | empty | List of target names to block. Emits to every other configured target. |

Tool-specific fields emit only where that tool's schema defines them; other targets ignore them.

With none of the scoping fields set, the hook emits to every target that supports hooks. `target` takes precedence over `targets` when both appear. Exclude wins: a target in both an include and an exclude list is excluded.

These four scoping fields work on every spec kind (agents, skills, rules, commands, mcps), not only hooks. A `target: codex` agent emits only into `.codex/agents/`; a `targets-exclude: [gemini]` skill emits to every configured target except gemini.

### Per-target body fences

When a spec emits to multiple targets but the prose must diverge (codex wants a "Workflow" section claude does not), wrap the divergent prose in `::target` fences. Outside-fence content emits everywhere; inside-fence content emits only to the listed targets. Marker lines never reach the output.

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

- `::target <name>` and `::targets <a> <b>` open a fence pinned to one or more targets.
- `::end` closes the most recent fence.
- An unterminated fence runs to end-of-body, so a missing `::end` keeps the tail of the file.
- The empty target (the source view used by `import` round-trips) returns the body with fences intact, so a re-emit stays byte-stable.
- `import codex` builds these fences automatically when both tools ship the same agent or skill name with diverging bodies: the longest common prefix and suffix stay un-fenced, and each tool's unique middle gets its own `::target` block.

### Import auto-scoping

Hooks imported from a tool-native source auto-set `target` to that tool (codex import → `target: codex`, claude → `target: claude`, gemini → `target: gemini`). Remove the field by hand to let the hook flow everywhere.

```yaml
event: PostToolUse
matcher: apply_patch|Edit|Write
command: "$(git rev-parse --show-toplevel)/.codex/hooks/format-php.sh"
target: codex   # shell-expanded codex path; do not leak to other tools
```

`import claude` and `import codex` apply the same auto-scoping to agents and skills: when both `.claude/` and `.codex/` exist but only one carries a given spec, the captured frontmatter gains `target: <tool>`. A spec present in both tools stays un-scoped (cross-emit). Pure single-tool projects also stay un-scoped, so byte-identical round-trips hold.

### Supported events (Claude Code)

| Event | When it fires |
|-------|---------------|
| `PreToolUse` | Before any tool call. Matcher is the tool name regex. |
| `PostToolUse` | After any tool call. Matcher is the tool name regex. |
| `PostToolUseFailure` | After a tool call fails. |
| `PermissionRequest` | When a permission dialog appears. |
| `UserPromptSubmit` | Before the model reads a new user message. |
| `SubagentStart` / `SubagentStop` | When a subagent spawns / finishes. |
| `Stop` | When the model stops generating. |
| `Notification` | When Claude Code surfaces a system notification. |
| `SessionStart` / `SessionEnd` | When a session begins / terminates. |
| `PreCompact` / `PostCompact` | Around context compaction. |

Claude Code defines more events (`Setup`, `InstructionsLoaded`, `TaskCompleted`, `TeammateIdle`, `FileChanged`, ...); the `event:` value passes through verbatim, so any documented name works. See the [Claude Code hooks reference](https://code.claude.com/docs/en/hooks) for the full list. Codex shares the `SessionStart`/`SubagentStart`/`UserPromptSubmit`/`PreToolUse`/`PermissionRequest`/`PostToolUse`/`PreCompact`/`PostCompact`/`Stop`/`SubagentStop` vocabulary.

Native emission: Claude Code (`.claude/settings.json`), Codex (`.codex/hooks.json`, per-event arrays), Gemini (`.gemini/settings.json` `hooks`), Cursor (`.cursor/hooks.json`, `version` + per-event arrays), Windsurf / Devin CLI (`.devin/hooks.v1.json`, no wrapper key, `type` accepting `prompt` as well as `command`), Factory (`.factory/hooks.json`, also no wrapper key, nine events, `type` always `"command"`), Kiro (`.kiro/hooks/<name>.json`, a `{"version": "v1", "hooks": [...]}` array), Qoder (`.qoder/settings.json` `hooks`, merged alongside `mcpServers` in the same file), OpenHands (`.openhands/hooks.json`, six events), Augment (`.augment/settings.json` `hooks`, merged alongside `mcpServers` in one write; five events; `timeout` in **milliseconds**, converted from this field's seconds; `command` must be a path ending in `.sh`/`.ps1`/`.cmd`/`.bat`, or it never runs), Crush (`crush.json` `hooks`, `PreToolUse` only, merged alongside `mcp` in the same file), Trae (`.trae/hooks.json`, the same integer `{"version": 1, "hooks": {...}}` wrapper around Claude-shaped `{matcher, hooks: [...]}` groups, six events, plus a `loop_limit` on `Stop`), and Copilot (`.github/hooks/agnostic-ai.json`, an integer `{"version": 1, "hooks": {...}}` wrapper, 14 events, `timeoutSec`; both `PreToolUse` and Copilot's own camelCase `preToolUse` are independently valid event-key spellings in that same file, so this one also passes `event:` through verbatim rather than picking one). Zed is opt-in and not a hooks surface at all: with `outputs.zed.tasks-file` set, hook specs emit as Zed Tasks instead, and without it they raise a coverage note. Other targets log a warning and skip. Event names pass through verbatim, so a Cursor hook sets `event:` to a Cursor name (`beforeShellExecution`, `afterFileEdit`, `beforeSubmitPrompt`, `sessionStart`, `stop`, ...). See each tool's docs for its full event list and matcher semantics.

agnostic-ai emits the `event:` value **verbatim** into each target's schema; it does not translate event names between tools. Claude and Codex share the `PreToolUse` / `PostToolUse` / `UserPromptSubmit` vocabulary, so one hook spec feeds both. Gemini uses its own names (`BeforeTool`, `AfterTool`, `BeforeAgent`, `AfterAgent`, `Notification`, `SessionStart`, `SessionEnd`, `PreCompress`, `BeforeModel`, `AfterModel`, `BeforeToolSelection`), so a Gemini hook must set `event:` to one of those. `agnostic-ai validate` flags any event a target does not recognize.

### Per-target rendering

This spec:

```yaml
event: PostToolUse
matcher: Bash(git commit*)
command: echo "tests please"
```

Renders to `.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Bash(git commit*)",
        "hooks": [
          {"type": "command", "command": "echo \"tests please\""}
        ]
      }
    ]
  }
}
```

Renders to `.codex/hooks.json` (same nested shape as Claude, routed into per-event arrays):

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Bash(git commit*)",
        "hooks": [
          {"type": "command", "command": "echo \"tests please\""}
        ]
      }
    ]
  }
}
```

Gemini uses different event names, so a Gemini hook sets `event:` to one of its own. This spec:

```yaml
event: AfterTool
matcher: run_shell_command
command: echo "tests please"
```

Renders to `.gemini/settings.json` (nested command handlers, event name passed through unchanged):

```json
{
  "hooks": {
    "AfterTool": [
      {"matcher": "run_shell_command", "hooks": [{"type": "command", "command": "echo \"tests please\""}]}
    ]
  }
}
```

When `command` is a list, each entry becomes a separate handler (Claude, Codex, Gemini). Gemini keeps the handlers in one definition. Set `x-gemini.sequential: true` to run them in order. `description` reaches each handler; `x-gemini.name` sets its native display name, and `x-gemini.env` supplies per-handler environment variables.

Gemini hook imports preserve nested definitions and accept old flat files. A single handler imports with a timeout in seconds when exactly representable as a whole second; otherwise `x-gemini.timeout` retains the native milliseconds. A group with multiple handlers uses `x-gemini.hooks`, a native handler array that preserves each command, name, description, environment map, and millisecond timeout. This array replaces `command` emission for Gemini.

## MCP servers

Pure YAML, no markdown body. One file per server.

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

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | yes | none | Server identifier. Becomes the key in the generated config. |
| `description` | no | empty | Free-form documentation. |
| `type` | no | `stdio` | Transport: `stdio`, `http`, `sse`, or `ws`. Remote transports (`http`, `sse`, `ws`) emit an explicit `type`; `stdio` stays type-less since it is the inferred default. |
| `command` | stdio only | none | Executable to launch. |
| `args` | no | empty | Argument list for the command. |
| `env` | no | empty | Environment variables passed to the server. |
| `cwd` | no | empty | Working directory for the stdio server process. Codex, Gemini, OpenCode, Qoder, Copilot/VS Code. Warp maps this to its own `working_directory` field. |
| `env_vars` | no | empty | Extra environment variables allowed for a Codex stdio server. Entries are names or `{name, source}` objects, where `source` is `local` or `remote`. |
| `url` | http/sse/ws only | none | Endpoint URL. |
| `headers` | no | empty | HTTP headers for `http`/`sse` transports. |
| `env_http_headers` | no | empty | Codex HTTP headers mapped to the environment variable that supplies each value. |
| `envFile` | stdio only, Cursor + Copilot/VS Code | empty | Path to an env file loading additional variables (e.g. `.env`, `${workspaceFolder}/.env`). Not supported on a `url` (remote) entry. |
| `dev` | stdio only, Copilot/VS Code | empty | Development-mode settings: `{watch, debug}`. `watch` is a glob pattern or array of glob patterns that restarts the server on change. `debug` is `{type: "node"\|"debugpy", debugpyPath}` for setting up a debugger. |
| `sandboxEnabled` | stdio only, Copilot/VS Code | `false` | Run the server in a sandboxed environment. macOS and Linux only. |
| `auth` | no | empty | Two unrelated shapes by target. Codex HTTP authentication fallback, a string: `oauth` or `chatgpt`. Cursor static OAuth on a remote (`url`) entry, an object: `{CLIENT_ID, CLIENT_SECRET, scopes}` (`CLIENT_ID` required, the other two optional). |
| `http_headers_helper` | http only, Codex | empty | Local command that prints a JSON object of HTTP header names/values, for a locally connected HTTP MCP server. |
| `required` | no, Codex | `false` | Fail startup/resume if this enabled MCP server cannot initialize. |
| `startup_timeout_sec` | no, Codex | `10` | Override the server's startup timeout, in seconds. Distinct from `timeout` below, which is milliseconds on the targets that use it; Codex's own field name says `sec` so the two never conflate. |
| `startup_timeout_ms` | no, Codex | `10000` | The same startup timeout in milliseconds. Codex documents it as an alias, so set one or the other, not both. |
| `tool_timeout_sec` | no, Codex | `60` | Override the per-tool execution timeout, in seconds. Same unit note as `startup_timeout_sec`. |
| `default_tools_approval_mode` | no, Codex | unset | Default approval behavior (`auto`, `prompt`, `writes`, or `approve`) for this server's tools, unless a per-tool override exists. |
| `scopes` | http/sse only, Codex | empty | OAuth scopes to request when authenticating to this MCP server. |
| `oauth_resource` | http/sse only, Codex | empty | RFC 8707 OAuth resource parameter to include during MCP login. |
| `experimental_environment` | no, Codex | unset | `local` or `remote` placement for the server. `remote` starts a stdio server through a remote executor environment; HTTP remote placement is documented as not yet implemented. |
| `enabled_tools` | no, Codex + Crush | empty | Allow list of tool names exposed by the server. |
| `disabled_tools` | no, Codex + Crush | empty | Deny list applied after `enabled_tools`. |
| `sessionless` | no, Crush | `false` | Mark a server that sends no `Mcp-Session-Id` so Crush skips the subscriptions/listen stream it would otherwise reject. Leave unset to let Crush auto-detect known sessionless servers such as GitHub MCP. |
| `trust` | no, Gemini + Qoder | `false` | Bypass all tool-call confirmations for this server. |
| `includeTools` | no, Gemini + Qoder | empty | Allowlist of tool names exposed from this server. On Amp the same concept exists but goes in namespaced as `x-amp.includeTools`: Amp's MCP page enumerates its fields without naming it, so a top-level mapping would assert more than the vendor states. |
| `excludeTools` | no, Gemini + Qoder | empty | Denylist of tool names; takes precedence over `includeTools` on a name in both. |
| `alwaysAllow` | no, Qoder | empty | Tool names always allowed without confirmation. |
| `autoApprove` | no, Kiro | empty | Tool names to auto-approve without prompting. `"*"` auto-approves all of the server's tools. |
| `disabledTools` | no, Kiro | empty | Tool names to omit when calling the agent. |
| `alwaysLoad` | no, Claude Code | `false` | Load every tool from this server into context at session start instead of deferring it behind tool search. Available on all transports. |
| `headersHelper` | http/sse/ws only, Claude Code | empty | Command run at connection time that prints headers to merge into the connection, for a server on Kerberos, short-lived tokens, or internal SSO. |
| `oauthScopes` | http/sse only, Kiro | empty | OAuth scopes to request. Overridden by `oauth.oauthScopes` when both are set; an explicitly empty list emits as written, since Kiro documents `[]` as the remedy for scope errors. |
| `oauth` | no | empty | Six unrelated shapes by target, each mapped to the keys its own vendor documents. Claude Code (http/sse): `{clientId, callbackPort, authServerMetadataUrl, scopes}`, where `scopes` is one space-separated string; `clientSecret` is never written, since Claude Code keeps it in the system keychain. Kiro (http/sse): `{clientId, clientSecret, redirectUri, clientMetadataUrl, oauthScopes}`. Qoder: passed through as declared, since the vendor's own field list is open-ended. Crush: a plain boolean toggle, paired with the separate `oauth_client_id` / `oauth_client_secret` / `oauth_callback_port` fields. Copilot/VS Code (http/sse): `{clientId, enterpriseManaged}`. Codex (http/sse): `{client_id, callback_url, callback_port}`, a nested `[mcp_servers.<id>.oauth]` table rather than a top-level object. |
| `api_key` | no | empty | OpenHands credential for an `http`/`sse` server. Upgrades the emitted `sse_servers`/`shttp_servers` element from a bare URL string to `{ url, api_key }`, OpenHands' own documented object form. `headers` has no equivalent there and surfaces a coverage note instead. |
| `timeout` | no | empty | Two unrelated units by target. OpenHands: tool-execution timeout in seconds (1-3600, default 60) for an `http` server; documented for the SHTTP tab only, so it upgrades `shttp_servers` elements the same way `api_key` does, and an `sse` entry that sets it surfaces a coverage note instead. Gemini, Claude Code, OpenCode, and Qoder: milliseconds, any transport. Claude Code's is a per-tool-call execution timeout, OpenCode's a tool-fetch timeout defaulting to 5000. |
| `disabled` | no | `false` | Support varies by target; see [`disabled` support by target](#disabled-support-by-target) below. |
| `roots` | no | empty | List of `{uri, name}` objects. Passed to targets that support MCP roots (Claude Code, Cursor, Copilot). |

`command` and `url` are the two fields a server cannot work without, and `agnostic-ai lint` reports a missing one as an error (LINT008). Neither `validate` nor `sync` catches it: some targets drop the entry, the rest write a server object with no way to start or reach anything, and both do it silently. See [lint](cli-reference.md#lint).

Targets with native MCP propagation:

| Target | File | Schema |
|--------|------|--------|
| Claude Code | `.mcp.json` | standard `mcpServers` |
| Cursor | `.cursor/mcp.json` | standard `mcpServers` |
| Copilot / VS Code | `.vscode/mcp.json` + `.github/mcp.json` | `servers` with `type` field for VS Code; `mcpServers` for Copilot CLI, which does not read the VS Code file |
| Codex | `.codex/config.toml` | `[mcp_servers.<name>]` table |
| Gemini | `.gemini/settings.json` | `mcpServers` (uses `httpUrl` for HTTP) |
| Continue | `.continue/mcpServers/<name>.yaml` | one YAML per server |
| Amp | `.amp/settings.json` | `amp.mcpServers` (dotted key) |
| Zed | `.zed/settings.json` | `context_servers` (stdio: `command`/`args`/`env`; HTTP/SSE: native `url`/`headers`) |
| Warp | `.warp/.mcp.json` | standard `mcpServers` (stdio `cwd` maps to `working_directory`) |
| OpenCode | `opencode.json` | `mcp` with `type: local\|remote` |
| Antigravity | `.agents/mcp_config.json` | `mcpServers` (remote uses `serverUrl`, not `url`) |
| Factory | `.factory/mcp.json` | standard `mcpServers` |
| Qoder | `.qoder/settings.json` | standard `mcpServers`, merged so unrelated settings keys survive |
| OpenHands | `config.toml` | `[mcp]` table with `stdio_servers` / `sse_servers` / `shttp_servers` arrays, no `type` field; a remote entry is a bare URL string, or `{ url, api_key, timeout }` once `api_key` and/or (shttp only) `timeout` is set |
| Trae | `.trae/mcp.json` | standard `mcpServers`, no `type` field (stdio: `command`/`args`/`env`; HTTP: `url`/`headers`) |
| Windsurf | `.devin/mcp_config.json` | `mcpServers` (Devin Local's file, not Cascade's; remote uses `transport`, not `type`) |
| Augment | `.augment/settings.json` | standard `mcpServers`, merged into the file alongside `shell`, `theme`, and other Auggie CLI settings rather than overwriting it |

Aider, Cline, Jules, and Goose have no project-scoped MCP file and skip with a warning.

### `disabled` support by target

Confirmed per target, never generalized: a target not listed here has not been checked.

| Target | Behavior |
|--------|----------|
| Antigravity | Native `disabled` boolean in `.agents/mcp_config.json` (default `false`); passes through unchanged under that literal name, unlike Codex and Kilo Code which map it to `enabled: false`. |
| Codex | Maps to `enabled = false` in `.codex/config.toml`. |
| Crush | Native `disabled` boolean in `crush.json` (default `false`); passes through unchanged. Confirmed in the vendor's published `schema.json` (`$defs.MCPConfig.properties.disabled`), not in the README. |
| Factory | Native `disabled` boolean in `.factory/mcp.json` (default `false`); passes through unchanged. |
| Kilo Code | Maps to `"enabled": false` in `kilo.jsonc`; an enabled server gets no key at all. |
| Kiro | Native `disabled` boolean in `.kiro/settings/mcp.json` (default `false`); passes through unchanged on both local and remote servers. |
| OpenCode | Maps to `"enabled": false` in `opencode.json`; an enabled server gets no key at all. `import opencode` reads it back into `disabled: true`. |
| Qoder | Native `disabled` boolean in `.qoder/settings.json` (default `false`); passes through unchanged. It had no effect while qoder shared Claude Code's `.mcp.json`, and works now that qoder writes its own file. |
| Windsurf | Native `disabled` boolean in `.devin/mcp_config.json` (default `false`); passes through unchanged. `devin mcp enable`/`disable` toggle the same key. |
| Zed | Maps to `"enabled": false` in `.zed/settings.json`; an enabled server gets no key at all, since Zed defaults it to true. Confirmed in Zed's own settings struct (`crates/settings_content/src/project.rs`), not on its MCP doc page, which names no per-server toggle. |
| Claude Code, Cursor, Copilot, Trae, Augment | No file-based way to pre-disable a project-scoped MCP server; the field has no effect and agnostic-ai does not emit it. Disable the server from the target's own UI instead (Augment: `auggie mcp remove`). |
| Warp | No file-based way to pre-disable a project-scoped MCP server: Warp's two property tables are closed lists and neither carries a disable key. Little is lost, since "project-scoped servers never auto-spawn" there; start the server from the MCP servers page when you want it. |

### `tools` support by target

Confirmed per target, never generalized: a target not listed here has not
been checked. One `tools: [Read, Bash]` spec produces five different
outcomes, so read this before assuming an allowlist restricts anything.

| Target | Behavior |
|--------|----------|
| Claude Code, Copilot | Passes through verbatim as a YAML list. The vendor vocabulary matches agnostic-ai's Claude-style names. |
| Junie | Passes through verbatim as a YAML list, at the agent's native `.junie/agents/<name>.md` file (#604). Junie's own built-in tool group labels (`Read`, `Bash`, `Glob`, `Grep`, `Write`, `Edit`, `WebSearch`, plus `AskUserQuestion`, which has no Claude equivalent) match agnostic-ai's Claude-style names for every group Junie documents, though it documents fewer groups overall (no `WebFetch`, `Task`, `TodoWrite`, or `NotebookEdit`). `disallowedTools` (a denylist applied after `tools`) passes through the same way. |
| Qoder | Passes through as a comma-separated string (`tools: Read, Bash`), the only form Qoder documents. Its built-in vocabulary is Claude-style, so the names carry over unchanged. |
| Trae | Passes through as a comma-separated string (`tools: Read, Bash`), the form Trae's subagent doc documents, at the agent's native `.trae/agents/<name>.md` file (#638). Its vocabulary is Claude-style and covers agnostic-ai's set exactly, plus `Skill`, `LSP`, `TodoWrite`, and `mcp__<server>__<tool>`. `x-trae.disallowedTools` (a denylist that wins over `tools`) reaches the file the same way. |
| Windsurf | **Translated**, not passed through, and written under Devin's own key name `allowed-tools`. Devin publishes a complete five-name vocabulary (`read`, `edit`, `grep`, `glob`, `exec`), so `Read`/`Grep`/`Glob`/`Bash` map one-to-one and `Write`/`Edit` both collapse onto `edit`, which means an agent declaring only `Write` also gains edit capability. An `mcp__<server>__<tool>` name passes through untranslated. Names outside the table surface a coverage note rather than being dropped silently. `x-windsurf.allowed-tools` bypasses translation entirely. |
| Antigravity | No effect. Antigravity's names (`view_file`, `replace_file_content`, `grep_search`, `run_command`, ...) share nothing with agnostic-ai's, and the vendor warns that "Specifying an unmapped or misspelled tool name in the `tools` list may cause the subagent process to hang during execution", so a verbatim passthrough is worse than dropping. Set `x-antigravity.tools`. A coverage note fires. |
| Kiro | **Translated**, not passed through. Kiro's vocabulary is lowercase categories, so `Read`/`Grep`/`Glob` become `read`, `Write`/`Edit` become `write`, `Bash` becomes `shell`, and `WebFetch`/`WebSearch` become `web`. A category grants more than the single name it came from: `Edit` alone also permits `delete_file`. Names outside the table surface a coverage note rather than being dropped. `x-kiro.tools` bypasses translation entirely. |
| Factory | **Translated**, not passed through, onto a different vocabulary than Kiro's. Droid CLI's complete tool-ID table is `Read`, `LS`, `Grep`, `Glob`, `Create`, `Edit`, `ApplyPatch`, `Execute`, `WebSearch`, `FetchUrl`, and "Unknown IDs cause a validation error", so `Bash` becomes `Execute`, `Write` becomes `Create`, and `WebFetch` becomes `FetchUrl`; every other agnostic-ai name is already a valid ID and carries over. `TodoWrite` and `Skill` drop because Factory always grants them, and `ExitSpecMode` and `GenerateDroid` drop because listing either one is a validation error. Any other name drops with a coverage note instead of failing the whole droid at load time. `x-factory.tools` bypasses translation, the only way to reach a category name (`read-only`) or a registered MCP tool ID. |
| Codex | No effect. Codex uses `tools` as a configuration table rather than a Claude-style allowlist, so the field is not written and a coverage note fires. Set `x-codex.tools` for Codex-native settings. |
| Cursor | No effect. Cursor subagents document only `name`, `description`, `model`, `readonly`, and `is_background`. `readonly: true` is the coarse equivalent. A coverage note fires. |
| Augment | No effect. Augment's names (`view`, `codebase-retrieval`, `str-replace-editor`, ...) are its own vocabulary, and no vendor-published mapping exists. Set `x-augment.tools` / `x-augment.disabled_tools`. A coverage note fires. |
| Kilo Code | No effect. Kilo Code's agent schema has no `tools` key at all; access is controlled by a `permission` object. Set `x-kilo.permission`. A coverage note fires. |

Every target that cannot honor the field says so at sync time. A silently
dropped restriction is the failure this table and those notes exist to
prevent: an author who writes `tools: [Read]` and gets an unrestricted
agent has no way to notice.

### `color` support by target

Confirmed per target, never generalized: a target not listed here has not
been checked. `color` is a shared top-level key on three targets, and each
documents its own value space, so a value valid on one may go unrecognized
on another.

| Target | Documented values |
|--------|--------------------|
| Augment | Free text: "should be a valid ANSI color name" ([docs.augmentcode.com/cli/subagents](https://docs.augmentcode.com/cli/subagents)). |
| Kilo Code | Hex (`#FF5733`) or a theme token (`primary`, `accent`, `error`, and others the doc leaves open-ended with "etc.") ([Kilo-Org/kilocode `custom-subagents.md`](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/customize/custom-subagents.md)). |
| Qoder | One of eight named values: `red`, `blue`, `green`, `yellow`, `purple`, `orange`, `pink`, `cyan` ([docs.qoder.com/cli/subagent](https://docs.qoder.com/cli/subagent)). |

agnostic-ai writes `color` verbatim to whichever of these three targets an
agent spec reaches; it does not validate the value against any target's
vocabulary. `color: blue` is a valid ANSI name for Augment and one of
Qoder's eight named values, but is neither hex nor a listed Kilo Code
theme token, so it may not render there as intended (Kilo Code's "etc."
leaves other tokens possible, not confirmed). An unrecognized value is
cosmetic: the agent still runs, only the badge may not show the intended
color.

## Commands

Markdown with optional YAML frontmatter. Each spec becomes one native slash command on supported targets.

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
| `name` | no | filename | Command identifier. Becomes the slash name (e.g. `/deploy`). |
| `description` | no | empty | One-liner shown in slash-command pickers. |
| `argument-hint` | no | empty | Hint string shown after the command. Claude-only; passes through. |

Any other frontmatter passes through unchanged. Use the `x-<target>` namespace for target-specific keys (e.g. `x-claude.allowed-tools`).

Native emission: Claude Code (`.claude/commands/<name>.md`), Cursor (`.cursor/commands/<name>.md`), Gemini (`.gemini/commands/<name>.toml`), OpenCode (`.opencode/commands/<name>.md`), Trae (`.trae/commands/<name>.md`), Junie (`.junie/commands/<name>.md`), Qoder (`.qoder/commands/<name>.md`), and Kilo Code (`.kilo/commands/<name>.md`). Codex deprecated project prompts, so its commands emit only when `outputs.codex.commands-dir` is set; otherwise `sync` prints a coverage note. Amp has no file-based command surface at all (commands register programmatically via `amp.registerCommand(...)`), so, like other targets outside this list, it logs a warning and skips.

## Settings

Pure YAML, one file per settings group under `settings/`. A tool-neutral place to single-source agent permissions and the default model instead of hand-editing each tool's settings file.

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
| `permissions.allow` | no | empty | Permission rules auto-approved without prompting. |
| `permissions.deny` | no | empty | Permission rules always blocked. |
| `permissions.ask` | no | empty | Permission rules that prompt before running. |
| `model` | no | empty | Default model the tool should use. |

Multiple settings files merge: permission lists concatenate (de-duped, source order preserved); the last non-empty `model` wins.

Native emission: Claude Code (`.claude/settings.json` `permissions` + `model`). Other targets have no equivalent settings surface and report the spec as unsupported.

When the Claude-specific `outputs.claude.settings` config in `agnostic-ai.yaml` also sets these fields, scalars like `model` take the config value (it is the more specific source), while `permissions` lists are unioned across the captured overlay, the settings spec, and the config so no layer silently drops another's allow/deny/ask rules.

`settings` specs are hand-authored: `import claude` does not create them. An imported `.claude/settings.json` keeps its `permissions` and `model` in the Claude-only overlay (`.agnostic-ai/overlays/claude.settings.json`), since a value like a Claude model id does not necessarily port to other tools. Author a `settings` spec when you want one source to drive multiple tools; a settings spec's `permissions` still union with the imported overlay on sync.

## Reviews

Markdown with optional YAML frontmatter, one file per guidance group under `reviews/`. A single source for code-review-bot guidance that maps to each ecosystem's native review-rule file.

```markdown
---
scope: backend
---

Flag any handler that talks to the database directly instead of going through a repository.
```

Review specs honor `scope` (and the source-directory layout) exactly like rules, so per-directory guidance is supported. Specs that share a scope concatenate into that scope's single review file.

Native emission: Cursor [Bugbot](https://docs.cursor.com/bugbot) `BUGBOT.md`: the repo root for unscoped specs, `<scope>/BUGBOT.md` for scoped ones. Override the basename with `outputs.cursor.review-file`. Other targets have no equivalent review-rule file yet and report the spec as unsupported.

## Environments

Pure YAML, one file per environment group under `environments/`. A single source for how a coding agent boots its dev env (install dependencies, start services, forward ports, open terminals).

```yaml
install: go mod download
terminals:
  - name: dev
    command: go run ./cmd/agnostic-ai
```

Native emission: Cursor background-agent [environment.json](https://docs.cursor.com/background-agent). The spec body is the `.cursor/environment.json` content: every key except the agnostic-ai routing fields (`name`, `scope`, `target(s)`, `target(s)-exclude`, `description`) passes through verbatim, so you author Cursor's schema while agnostic-ai single-sources it. Multiple environment specs merge by top-level key (last wins). Override the path with `outputs.cursor.environment-file`.

Also native on OpenHands: the `install` field writes [`.openhands/setup.sh`](https://docs.openhands.dev/openhands/usage/customization/repository), the vendor's documented repository bootstrap script that "will run every time OpenHands begins working with your repository". The script body is `install` verbatim under a `#!/bin/bash` shebang. `terminals` has no OpenHands equivalent (the script runs once, synchronously; there is no long-running process surface) and surfaces a coverage note instead. Multiple environment specs merge the same way as Cursor's (last `install` wins). Override the path with `outputs.openhands.setup-file`.

Other targets (devcontainers, Codex setup scripts) have no emitter yet and report the spec as unsupported.

## Ignore

Markdown with optional YAML frontmatter, one file per group under `ignore/`. The body is gitignore-syntax patterns naming what an agent must not read or index. One source fans out to each tool's native ignore file.

```markdown
# Secrets and build artifacts the agent should never read
*.env
secrets/
dist/
```

Native emission (gitignore syntax, under a `#` provenance header): Cursor `.cursorignore`, Gemini `.geminiignore`, Aider `.aiderignore`, Windsurf `.devinignore`, Kiro `.kiroignore`, Trae `.trae/.ignore`, Junie `.aiignore`. Each spec body is trimmed and the specs are concatenated with a blank line between them. Each path is overridable via `outputs.<target>.ignore-file`. Targets without an ignore-file convention report the spec as unsupported.

### Overwrite behaviour

An ignore file you wrote by hand is never silently replaced. When the target's ignore file exists, carries no agnostic-ai provenance header, and holds a pattern the emitted body does not carry, `sync` fails, names the patterns at risk, and writes nothing:

```
.kiroignore: hand-authored, and overwriting it would drop patterns that keep
files out of agent context. Would be dropped: *.key, my-secrets/.
Run `agnostic-ai import kiro` to copy them into an ignore spec, then sync again.
```

`agnostic-ai import <target>` reads the file into `ignore/<target>.md`, comments and all. The next `sync` then emits a file holding both your patterns and the ones your other specs contribute, so no manual cleanup step sits in between. An overwrite that already reproduces every on-disk pattern is not a loss and goes ahead untouched.

Two cases stand down deliberately. Comment and blank lines exclude nothing, so a file holding only those never blocks a sync. And `outputs.<target>.provenance-header: false` removes the marker the check reads to tell agnostic-ai's own output from yours, which disables the check along with it.

## Frontmatter rules

- YAML between two `---` lines at the top of the file.
- Empty (`---\n---\n`) is allowed and treated as no metadata.
- Files without frontmatter still load; name defaults to the filename.
- Malformed frontmatter is treated as no metadata; the entire content becomes the body.
- Any field not listed above passes through on emit. Useful for target-specific extensions.

## Path variables: `{{$NAME}}`

A spec body can name a directory without hardcoding one target's layout. `{{$SKILLS_DIR}}` expands to `.claude/skills` for claude, `.agents/skills` for codex, and `.github/skills` for copilot, from one source file.

```markdown
Put new skills in {{$SKILLS_DIR}} and agent profiles in {{$AGENTS_DIR}}.
```

Five variables are available:

| Variable | Resolves to |
|---|---|
| `{{$SKILLS_DIR}}` | the target's skills directory |
| `{{$AGENTS_DIR}}` | the target's agents directory |
| `{{$COMMANDS_DIR}}` | the target's commands directory |
| `{{$RULES_DIR}}` | the target's rules directory |
| `{{$MCP_FILE}}` | the target's MCP config file |

Rules:

- **Bodies only.** Every spec kind that carries a body is expanded; frontmatter values are not.
- **An `outputs.<target>.<field>` override wins.** Set `outputs.claude.skills-dir: custom/skills` and `{{$SKILLS_DIR}}` follows it, so a body never names a directory the emitted tree does not use.
- **A variable the target has no surface for stays verbatim** and raises a coverage note. It is not blanked, because turning "see {{$COMMANDS_DIR}}" into "see " loses the sentence silently. Targets that carry every kind in one entry-point document (aider, jules, goose) resolve no variables at all.
- **A variable is declared only where the target has a dedicated surface for that kind.** Several targets flatten agents into their rules directory with a filename prefix (antigravity, continue, trae, windsurf) or render them as commands (gemini); those declare no `{{$AGENTS_DIR}}` rather than point at a directory that is not an agents directory.
- **The `$` sigil is required.** Plain `{{placeholder}}` is left alone, so Warp workflow arguments and any Handlebars or Jinja quoted in prose survive untouched. Lowercase names (`{{$skills_dir}}`) do not resolve.

## Target-specific extensions: `x-<target>` namespace

Use `x-<target>:` blocks to attach fields only one adapter consumes. Other adapters strip the block on emit, so the spec stays portable.

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

Resolution per target:

- All `x-*` keys are dropped first.
- The matching `x-<target>` block is flattened into top-level meta.
- Flattened keys override top-level keys with the same name.

| Target | Resulting frontmatter |
|--------|-----------------------|
| `claude` | `name`, `description`, `model`, `allowed-tools` |
| `cursor` | `name`, `description`, `model`, `globs`, `alwaysApply` |
| `gemini` | `description` (`name` becomes the `.toml` filename; `model` has no native command surface and is not emitted) |

For Codex agents, `x-codex` fields (`model`, `model_reasoning_effort`, `sandbox_mode`, `nickname_candidates`) pass through to the generated `.codex/agents/<name>.toml`. For Codex skills, `x-codex.interface`, `x-codex.policy`, and `x-codex.dependencies` trigger an additional `.agents/skills/<name>/agents/openai.yaml` for UI customization, policy, and tool dependencies.

### Arbitrary custom keys

Any other key under `x-<target>` emits verbatim into that target's output surface. Declaring it under `x-<target>` is the opt-in: shared top-level keys stay stripped, so plain specs keep emitting valid files. Keys emit in sorted order and never leak across targets. Validate them against the target's schema yourself.

Per surface:

| Target | Surface | Custom key lands in |
|--------|---------|---------------------|
| `claude` | `SKILL.md` frontmatter | every `x-claude` key (e.g. `disable-model-invocation: true`) |
| `codex` | `SKILL.md` frontmatter | every `x-codex` key except `interface`/`policy`/`dependencies` (those route to `openai.yaml`) |
| `amp`, `zed`, `crush`, `gemini`, `opencode`, `copilot`, `kiro` | `SKILL.md` frontmatter (shared renderer) | every `x-<target>` key beyond `name`/`description` (e.g. crush's `user-invocable: true`, which adds the skill to the command palette) |
| `cursor` | `SKILL.md` frontmatter | every `x-cursor` key beyond `name`/`description`/`paths`/`disable-model-invocation`/`icon`/`color`/`metadata` |
| `cursor` | agent `.md` frontmatter | every `x-cursor` key beyond `name`/`description`/`model`/`readonly`/`is_background` |
| `copilot` | rule `.instructions.md` frontmatter | every `x-copilot` key, alongside `applyTo` |
| `copilot` | `.agent.md` frontmatter | every `x-copilot` key beyond `name`/`description`/`tools`/`model` |
| `opencode` | agent `.md` frontmatter | every `x-opencode` key beyond `description`/`mode`/`model`/`temperature`/`permission` |
| `opencode` | command `.md` frontmatter | every `x-opencode` key beyond `description`/`agent`/`model`/`subtask` |
| `gemini` | command `.toml` | every `x-gemini` key (string, bool, number, or string array) |
| `gemini` | agent `.md` frontmatter | every `x-gemini` key beyond `name`/`description`/`kind`/`model`/`temperature`/`max_turns`/`timeout_mins` (e.g. `mcpServers`, Gemini's inline per-agent MCP servers; `x-gemini.tools` also wins outright over the translated `tools` list) |
| `kiro` | agent `.md` frontmatter | every `x-kiro` key beyond `description`/`model` (`name` is excluded: Kiro's agent schema has none, identity comes from the filename) |
| `kiro` | hook `.json` entry | every `x-kiro` key beyond `name`/`trigger`/`matcher`/`action`/`timeout`/`enabled`/`description` (e.g. `confirm`, Kiro's Stop-hook confirmation block, which has no agnostic-ai spec equivalent) |
| `qoder` | agent `.md` frontmatter | every `x-qoder` key beyond `name`/`description`/`model`/`tools`/`skills`/`mcpServers` |
| `warp` | workflow `.yaml` | every `x-warp` key beyond `name`/`command`/`description`/`tags` (e.g. `shells`, `arguments`, `source_url`, `author`, `author_url`) |
| `zed` | task (in `outputs.zed.tasks-file`) | every `x-zed` key beyond `label`/`command`/`args` (e.g. `cwd`, `env`, `shell`, `reveal`, `hide`, `save`, `allow_concurrent_runs`, `use_new_terminal`, `tags`, `reevaluate_context`) |

Targets that emit no surface for a spec kind drop arbitrary custom keys (kiro rule steering files carry no passthrough; kiro agents, skills, and hooks do, see above). Gemini TOML accepts scalars and string arrays only; nested tables are skipped.
