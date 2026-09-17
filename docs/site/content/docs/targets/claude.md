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
- **First-class settings**: `outputs.claude.settings.*` declares model, outputStyle, statusLine, permissions, enabledPlugins, env, apiKeyHelper, cleanupPeriodDays, attribution, bashOutputMaxChars, and taskOutputMaxChars. The last two raise how much command and background-task output Claude Code takes inline before spilling it to a file, up to 128K characters, and need Claude Code v2.1.261 or later (#679). The deprecated includeCoAuthoredBy key remains available for older Claude Code versions. These settings merge above the captured overlay and below the spec-derived hooks. See [Claude settings](@/docs/configuration.md#claude-settings).

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

Verify with the real CLI:

1. Install: `npm install -g @anthropic-ai/claude-code` (or the desktop app; both read the same files).
2. Check the tree: `ls CLAUDE.md .claude/agents/ .claude/skills/ .claude/rules/ .claude/commands/ .claude/settings.json .mcp.json` and `grep "Generated by agnostic-ai" .claude/agents/*.md .claude/rules/*.md .claude/commands/*.md` for the provenance header (it sits after the YAML frontmatter, so `head -1` would only show `---`).
3. Validate JSON: `python -m json.tool .claude/settings.json > /dev/null && python -m json.tool .mcp.json > /dev/null`.
4. Launch `claude` from the project root. `/agents`, `/skills`, and the slash-command picker should list every entry. The MCP picker shows each `.mcp.json` server green.
5. Trigger a matcher action (e.g. an `Edit` for `PostToolUse`/`Edit`); the hook command runs with no "schema mismatch" in the log.
6. Confirm `outputs.claude.settings.*` keys under `/config`.
