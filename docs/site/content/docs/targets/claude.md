+++
title = "Claude Code"
description = "How agnostic-ai emits Claude Code configuration: native paths, capability limits, and output options."
weight = 10

[extra]
group = "Reference"
target_id = "claude"
+++

# Claude Code (`claude`)

```
CLAUDE.md                # canonical entry-point pointer body (written by sync)
.claude/
├── agents/<name>.md
├── skills/<name>/SKILL.md
├── rules/<name>.md
├── commands/<name>.md
└── settings.json
.mcp.json
```

- **Rules**: one file per spec under `.claude/rules/`. Claude Code discovers every `.md` file under that directory (recursively) at session start, so the emitted rules load natively with no extra wiring. A spec with the cross-tool `globs` field (or a native `paths` list) emits `paths:` frontmatter, which scopes the rule to matching files.
- **Legacy rules modes**: `outputs.claude.rules-mode: import` appends a sentinel-marked block of `@.claude/rules/<name>.md` imports to the pointer body, round-trip-stripped on `import`. It predates native rules loading, so keep it only for Claude Code versions older than the `.claude/rules/` rollout. `outputs.claude.rules-file: CLAUDE.md` concatenates rule bodies into a single file instead, and skips the pointer-body write for `claude`.
- **Commands**: one file per spec under `.claude/commands/`. Spec `deploy` becomes `/deploy`. Frontmatter passes through; body is the prompt template.
- **Settings overlay**: `agnostic-ai import claude` captures the non-`hooks` portion of `.claude/settings.json` (statusLine, enabledPlugins, any top-level key) into `.agnostic-ai/overlays/claude.settings.json`. `sync -t claude` layers the spec-derived `hooks` key on top, reproducing the full settings.json from a fresh checkout. Re-run `import claude` after editing settings.json by hand.
- **MCP**: written into `.mcp.json` under the standard `mcpServers` map. Stdio entries use `command`/`args`/`env` with no `type`; remote entries use `type` plus `url`/`headers`. Every entry also accepts `timeout` (per-tool-call execution timeout in milliseconds; values under 1000 are ignored) and `alwaysLoad` (load the server's tools at session start instead of deferring them behind tool search, "available on all server types").

  An `http`, `sse`, or `ws` entry additionally accepts `headersHelper`, a command run at connection time whose output merges into the connection headers, for "an authentication scheme other than OAuth, such as Kerberos, short-lived tokens, or an internal SSO". It also accepts an `oauth` object taking `clientId`, `callbackPort`, `authServerMetadataUrl`, and a space-separated `scopes` string ([code.claude.com/docs/en/mcp](https://code.claude.com/docs/en/mcp), target-audit 2026-08-27, #634). `oauth.clientSecret` is never written: the vendor keeps the secret in the system keychain, "not in your config". `disabled: true` has no effect here; see [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).
- **MCP import**: `import claude` reads `.mcp.json` and writes one spec under `<mcps>/<name>.yaml` per `mcpServers.<name>`. The next `sync` distributes them to codex, copilot, cursor, continue, amp, zed, warp, gemini, opencode.
- **First-class settings**: `outputs.claude.settings.*` declares model, outputStyle, statusLine, permissions, enabledPlugins, env, apiKeyHelper, cleanupPeriodDays, attribution, bashOutputMaxChars, and taskOutputMaxChars. The last two raise how much command and background-task output Claude Code takes inline before spilling it to a file, up to 128K characters, and need Claude Code v2.1.261 or later (#679). The deprecated includeCoAuthoredBy key remains available for older Claude Code versions. These settings merge above the captured overlay and below the spec-derived hooks. See [Claude settings](#claude-settings).

Hooks support `command`, `http`, `mcp_tool`, and `prompt` handlers. HTTP uses `url`, optional `headers`, and `allowedEnvVars`. MCP uses `server`, `tool`, and optional `input`. Prompt uses `prompt`, optional `model`, and `continueOnBlock`. When `continueOnBlock: true`, a blocking prompt result returns its reason to Claude and the turn continues. `import claude` preserves these handlers alongside command hooks. Experimental agent handlers are not emitted. See [hook fields](@/docs/spec-format.md#hooks).

The MCP file is managed as a whole document. Each sync replaces `.mcp.json` from MCP specs. Import hand-authored servers before syncing.

Config keys:

| Key | Default | Notes |
|---|---|---|
| `outputs.claude.dir` | `.claude` | |
| `outputs.claude.rules-dir` | `.claude/rules` | auto-loaded by Claude Code |
| `outputs.claude.rules-mode` | unset | set to `import` to also wire `.claude/rules/*.md` into `CLAUDE.md` via `@`-imports, only needed on Claude Code versions without native rules loading |
| `outputs.claude.rules-file` | unset | switches to legacy concatenated single-file layout, typically `CLAUDE.md` |
| `outputs.claude.commands-dir` | `.claude/commands` | |
| `outputs.claude.mcp-file` | `.mcp.json` | |
| `outputs.claude.settings` | | first-class settings block |

## Claude settings

The `outputs.claude.settings` block declares first-class `.claude/settings.json` keys. The full layering, low to high precedence, is: captured overlay (from `import claude`) < agnostic `settings` specs (`.agnostic-ai/settings/`, the cross-tool source for `permissions` + `model`) < this `outputs.claude.settings` config < spec-derived `hooks` block. Keys you do not set fall through to the lower layers, so partial adoption works.

```yaml
outputs:
  claude:
    settings:
      model: claude-opus-4-7
      outputStyle: verbose
      apiKeyHelper: ./bin/keyhelper.sh
      cleanupPeriodDays: 30
      bashOutputMaxChars: 64000
      taskOutputMaxChars: 128000
      attribution:
        commit: ""
        pr: ""
        sessionUrl: false
      enabledPlugins:
        plugin-a@marketplace-a: true
        plugin-b@marketplace-b: true
      env:
        FOO: bar
      statusLine:
        type: command
        command: echo status
        padding: 2
        refreshInterval: 5
        hideVimModeIndicator: true
      permissions:
        allow:
          - "Read(*)"
        deny:
          - "Shell(rm *)"
        ask:
          - "Edit(*)"
```

| Field | Type | Notes |
|-------|------|-------|
| `model` | string | Default model preference for new sessions. |
| `outputStyle` | string | One of the Claude Code output styles. |
| `apiKeyHelper` | string | Path to a script that prints an API key on stdout. |
| `cleanupPeriodDays` | integer | Days of conversation history to retain. |
| `bashOutputMaxChars` | integer | How much command output Claude Code takes inline before spilling it to a file, up to 128000. Needs Claude Code v2.1.261 or later. |
| `taskOutputMaxChars` | integer | The same budget for background-task output, up to 128000. Needs Claude Code v2.1.261 or later. |
| `attribution` | object | Current attribution controls. `commit` and `pr` set the text for commits and pull requests; an explicit empty string disables that attribution. `sessionUrl` controls whether the Claude session URL is included. |
| `includeCoAuthoredBy` | boolean | Deprecated Claude Code setting. Use `attribution`; when both are present, `attribution` takes precedence. |
| `enabledPlugins` | map of string to boolean | `plugin-id@marketplace-id` keys mapped to `true` to enable them. Matches the `enabledPlugins` object in Claude Code's settings schema; a plain list cannot express the required `@marketplace-id` qualifier. |
| `env` | map of strings | Environment variables exported into Claude sessions. |
| `statusLine` | object | `type`, `command`, and optional `padding`, `refreshInterval` (seconds, minimum 1), and `hideVimModeIndicator`. |
| `permissions` | object | `allow`, `deny`, `ask` lists of tool-pattern strings. |

Any setting not declared here round-trips through the overlay captured during `agnostic-ai import claude` (written to `.agnostic-ai/overlays/claude.settings.json`). When both the overlay and `outputs.claude.settings.*` declare the same scalar key, the first-class config block wins. The `permissions` lists are the exception: their `allow`/`deny`/`ask` entries are unioned across the overlay, any `settings` spec, and this config, so no layer silently drops another's rules.

Verify with the real CLI:

1. Install: `npm install -g @anthropic-ai/claude-code` (or the desktop app; both read the same files).
2. Check the tree: `ls CLAUDE.md .claude/agents/ .claude/skills/ .claude/rules/ .claude/commands/ .claude/settings.json .mcp.json` and `grep "Generated by agnostic-ai" .claude/agents/*.md .claude/rules/*.md .claude/commands/*.md` for the provenance header (it sits after the YAML frontmatter, so `head -1` would only show `---`).
3. Validate JSON: `python -m json.tool .claude/settings.json > /dev/null && python -m json.tool .mcp.json > /dev/null`.
4. Launch `claude` from the project root. `/agents`, `/skills`, and the slash-command picker should list every entry. The MCP picker shows each `.mcp.json` server green.
5. Trigger a matcher action (e.g. an `Edit` for `PostToolUse`/`Edit`); the hook command runs with no "schema mismatch" in the log.
6. Confirm `outputs.claude.settings.*` keys under `/config`.
