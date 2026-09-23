+++
title = "Claude Code"
description = "How agnostic-ai emits Claude Code configuration: native paths, capability limits, and output options."
weight = 10

[extra]
group = "Reference"
target_id = "claude"
+++

# Claude Code (`claude`)

## Output

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

- **Rules**: one file per spec under `.claude/rules/`. Claude Code discovers every `.md` file under that directory (recursively) at session start, so emitted rules load with no extra wiring. A spec with the cross-tool `globs` field (or a native `paths` list) emits `paths:` frontmatter, which scopes the rule to matching files.
- **Legacy rules modes**: `outputs.claude.rules-mode: import` appends a sentinel-marked block of `@.claude/rules/<name>.md` imports to the pointer body, round-trip-stripped on `import`. It predates native rules loading, so keep it only for Claude Code versions older than the `.claude/rules/` rollout. `outputs.claude.rules-file: CLAUDE.md` concatenates rule bodies into a single file instead, and skips the pointer-body write for `claude` because the adapter owns that exact path. Point `rules-file` at any other path and `CLAUDE.md` is still written, with an `@<path>` import wiring the merged file in: a file outside `.claude/rules/` is on no Claude Code auto-load path, and a project with no `CLAUDE.md` at all makes Claude Code read `AGENTS.md` instead.
- **Skills**: one folder per skill at `.claude/skills/<name>/SKILL.md`. The adapter manages no frontmatter keys, so every `x-claude` key passes through, for example `disable-model-invocation: true`.
- **Commands**: one file per spec at `.claude/commands/<name>.md`. Spec `deploy` becomes `/deploy`. Frontmatter passes through; body is the prompt template.
- **Agents**: one file per spec at `.claude/agents/<name>.md`. Resolved frontmatter passes through, so a portable `effort` (scalar or per-target map) is written verbatim and not validated; Claude Code documents `low`, `medium`, `high`, `xhigh`, and `max`. See [per-target `model` and `effort`](@/docs/spec-format.md#per-target-model-and-effort).
- **Global agents**: `sync --global --only claude` writes `~/.claude/agents/<name>.md`. See [global configuration](@/docs/configuration.md#global-configuration) for source paths and setup.
- **MCP**: written into `.mcp.json` under the standard `mcpServers` map. Stdio entries use `command`/`args`/`env` with no `type`; remote entries use `type` plus `url`/`headers`. Every entry also accepts `timeout` (per-tool-call execution timeout in milliseconds; values under 1000 are ignored) and `alwaysLoad` (load the server's tools at session start instead of deferring them behind tool search, "available on all server types").

  An `http`, `sse`, or `ws` entry additionally accepts `headersHelper`, a command run at connection time whose output merges into the connection headers, for "an authentication scheme other than OAuth, such as Kerberos, short-lived tokens, or an internal SSO". It also accepts an `oauth` object, `{clientId, callbackPort, authServerMetadataUrl, scopes}`, where `scopes` is one space-separated string ([code.claude.com/docs/en/mcp](https://code.claude.com/docs/en/mcp), target-audit 2026-08-27, #634). `oauth.clientSecret` is never written: the vendor keeps the secret in the system keychain, "not in your config". A `roots` list still emits when a spec declares one, but no Claude Code page documents `roots` as a per-server `.mcp.json` key, so treat it as passthrough rather than a supported field. Claude Code derives roots itself: it "answers `roots/list` with the session's launch directory plus every additional working directory you've granted with `--add-dir`, `/add-dir`, or the `additionalDirectories` setting" ([code.claude.com/docs/en/mcp](https://code.claude.com/docs/en/mcp), target-audit 2026-09-20). `disabled: true` adds the server to `disabledMcpjsonServers` in project settings; see [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).
- **First-class settings**: `outputs.claude.settings.*` declares model, outputStyle, statusLine, permissions, enabledPlugins, env, apiKeyHelper, cleanupPeriodDays, attribution, bashOutputMaxChars, and taskOutputMaxChars. The last two need Claude Code v2.1.261 or later (#679), and taskOutputMaxChars is a no-op from Claude Code v2.1.277 on. The deprecated includeCoAuthoredBy key remains available for older Claude Code versions. enabledPlugins reaches more than Claude Code: Copilot CLI reads the same file, see [Copilot](@/docs/targets/copilot.md). These settings merge above the captured overlay and below the spec-derived hooks. See [Claude settings](#claude-settings).

Hooks support `command`, `http`, `mcp_tool`, and `prompt` handlers. HTTP uses `url`, optional `headers`, and `allowedEnvVars`. MCP uses `server`, `tool`, and optional `input`. Prompt uses `prompt`, optional `model`, and `continueOnBlock`. When `continueOnBlock: true`, a blocking prompt result returns its reason to Claude and the turn continues. `import claude` preserves these handlers alongside command hooks. Experimental agent handlers are not emitted. See [hook fields](@/docs/spec-format.md#hooks).

Stable handlers keep `timeout`, `statusMessage`, `if`, and `once`. `args`, `async`, `asyncRewake`, and `shell` apply to command handlers only. A `command` list becomes one handler per entry. Setting `args` switches to exec form, which keeps the executable in `command`.

`once` writes through but never deregisters a hook here: Claude Code "only honors it for hooks declared in skill frontmatter; ignored in settings files and agent frontmatter" ([code.claude.com/docs/en/hooks](https://code.claude.com/docs/en/hooks)), and `.claude/settings.json` is this adapter's only hook sink. `sync` prints a note saying so.

`event` passes through verbatim. Common events:

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

Claude Code defines more events (`Setup`, `InstructionsLoaded`, `TaskCompleted`, `TeammateIdle`, `FileChanged`, ...), and any documented name works. See the [Claude Code hooks reference](https://code.claude.com/docs/en/hooks) for the full list.

The MCP file is managed as a whole document. Each sync replaces `.mcp.json` from MCP specs. Import hand-authored servers before syncing.

## Config keys

| Key | Default | Notes |
|---|---|---|
| `outputs.claude.dir` | `.claude` | |
| `outputs.claude.rules-dir` | `.claude/rules` | auto-loaded by Claude Code |
| `outputs.claude.rules-mode` | unset | set to `import` to also wire `.claude/rules/*.md` into `CLAUDE.md` via `@`-imports, only needed on Claude Code versions without native rules loading |
| `outputs.claude.rules-file` | unset | switches to legacy concatenated single-file layout, typically `CLAUDE.md`; any other path keeps `CLAUDE.md` and wires the file in with an `@`-import |
| `outputs.claude.commands-dir` | `.claude/commands` | |
| `outputs.claude.agents-dir` | `.claude/agents` | |
| `outputs.claude.skills-dir` | `.claude/skills` | |
| `outputs.claude.mcp-file` | `.mcp.json` | |
| `outputs.claude.settings` | | first-class settings block |

`import claude` and `agnostic-ai doctor` read the same resolved paths, so a moved directory round-trips and an unmanaged file under it is still reported (#852).

`dir` moves the whole tool directory: with `dir: vendor/.claude`, rules land in `vendor/.claude/rules/` and commands in `vendor/.claude/commands/`, and `{{rules_dir}}` and the other path variables resolve there too. A per-kind key overrides that path on its own. Moving `dir` after a sync leaves the old rules and commands behind until the next full sync sweeps them as orphans, and Claude Code auto-loads only a project-root `.claude/rules/`, so a moved rules directory needs `rules-mode: import` to reach the session.

With `gitignore.enabled`, the managed `.gitignore` block also lists `/.claude/agent-memory-local/` and `/.claude/settings.local.json`, following `outputs.claude.dir`. Subagent memory written under `memory: project` lives in `.claude/agent-memory/` and stays out of the block because Claude Code documents it as shareable via version control, while `memory: local` is machine-local. A store already committed before this changed stays tracked until `git rm -r --cached .claude/agent-memory-local` removes it, because an ignore line does not untrack files.

## Agent memory

A top-level `memory` key gives a subagent a directory that survives across sessions. Claude Code is the only target that acts on it. Junie copies the key into its own agent file unchanged, and every other adapter drops it, so the same spec stays portable.

```yaml
---
name: code-reviewer
description: Reviews diffs for bugs and style.
memory: project
---
```

| Scope | Directory | Git |
|---|---|---|
| `user` | `~/.claude/agent-memory/<name>/` | outside the repository, so git never sees it |
| `project` | `.claude/agent-memory/<name>/` | documented as shareable via version control, so commit it if the team wants it shared |
| `local` | `.claude/agent-memory-local/<name>/` | documented as not to be checked into version control |

Claude Code creates and writes the directory on first use. agnostic-ai emits the frontmatter key and never reads or writes the store.

This is subagent memory, separate from the session auto memory store under `~/.claude/projects/<project>/memory/`, which agnostic-ai leaves alone. It still depends on auto memory being enabled: with `autoMemoryEnabled` off, or `CLAUDE_CODE_DISABLE_AUTO_MEMORY` set, the `memory` key has no effect. See [Memory and local state](@/docs/target-behavior.md#memory-and-local-state).

Disabled MCP policy uses `.claude/.agnostic-ai-mcp-disabled.json` to track only generated rejection entries. Re-enabling a server removes its generated rejection while preserving manual entries and unrelated settings. Keep this state file with generated settings. Both paths follow `outputs.claude.dir` and must be managed together. A disabled server requires the project `.mcp.json` path; custom MCP paths fail with an actionable error. Import restores disabled state from the project rejection list and excludes generated rejections from the settings overlay, so later syncs cannot restore a re-enabled server's old rejection.

## Claude settings

The `outputs.claude.settings` block declares first-class `.claude/settings.json` keys. The full layering, low to high precedence, is: captured overlay (from `import claude`) < agnostic `settings` specs (`.agnostic-ai/settings/`, the cross-tool source for `permissions` + `model`) < this `outputs.claude.settings` config < spec-derived `hooks` block < an `x-claude` block on a settings spec. Keys you do not set fall through to the lower layers. The `x-claude` block is last because it is the most specific statement of intent: an author writing Claude Code's own spelling means that key. On a key a lower layer already wrote, the two merge instead of the block replacing it: `x-claude.permissions.deny` adds its rules to the translated deny list, and only a scalar such as `model` is replaced outright.

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
| `taskOutputMaxChars` | integer | Deprecated. The same budget for background-task output, up to 128000. Claude Code v2.1.277 removed the TaskOutput tool, and its changelog says the setting and `TASK_MAX_OUTPUT_LENGTH` "no longer have any effect". It still emits, because the `stable` dist-tag was 2.1.267 when this was checked and the key works there. Needed Claude Code v2.1.261 or later. |
| `attribution` | object | Current attribution controls. `commit` and `pr` set the text for commits and pull requests; an explicit empty string disables that attribution. `sessionUrl` controls whether the Claude session URL is included. |
| `includeCoAuthoredBy` | boolean | Deprecated Claude Code setting. Use `attribution`; when both are present, `attribution` takes precedence. |
| `enabledPlugins` | map of string to boolean | `plugin-id@marketplace-id` keys mapped to `true` to enable them. Matches the `enabledPlugins` object in Claude Code's settings schema; a plain list cannot express the required `@marketplace-id` qualifier. |
| `env` | map of strings | Environment variables exported into Claude sessions. |
| `statusLine` | object | `type`, `command`, and optional `padding`, `refreshInterval` (seconds, minimum 1), and `hideVimModeIndicator`. |
| `permissions` | object | `allow`, `deny`, `ask` lists of tool-pattern strings. |

Any setting not declared here round-trips through the overlay captured during `agnostic-ai import claude` (written to `.agnostic-ai/overlays/claude.settings.json`). When both the overlay and `outputs.claude.settings.*` declare the same scalar key, the first-class config block wins. The `permissions` lists are the exception: their `allow`/`deny`/`ask` entries are unioned across the overlay, any `settings` spec, and this config, so no layer silently drops another's rules.

## Import

`agnostic-ai import claude` reads the Claude Code tree:

| Source | Becomes |
|--------|---------|
| `.claude/rules/**/*.md` (preferred) | `<rules>/<sub>/<name>.md` (byte-identical copy, nested subdirectories preserved) |
| `CLAUDE.md` (split on `## headings`) | `<rules>/<slug>.md` per section (only when `.claude/rules/` is absent) |
| `CLAUDE.md` (no headings) | single `<rules>/<projectname>.md` (only when `.claude/rules/` is absent) |
| `CLAUDE.md` (any form) | `.agnostic-ai/AGNOSTIC_AI.md` (byte-identical copy) |
| `AGENTS.md` or `.claude/AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` (only when no `CLAUDE.md` exists) |
| `.claude/agents/*.md` | `<agents>/<name>.md` (byte-identical copy) |
| `.claude/skills/<name>/SKILL.md` | `<skills>/<name>/SKILL.md` |
| `.claude/commands/*.md` | `<commands>/<name>.md` (byte-identical copy) |
| `.claude/settings.json` hooks | `<hooks>/<event>[-<matcher-slug>]-<hash8>.yaml` (command hooks retain their command grouping; HTTP, MCP-tool, and prompt handlers retain their native fields in separate stable specs) |
| `.claude/settings.json` non-hook keys | `.agnostic-ai/overlays/claude.settings.json` |
| `.mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml` (one spec per server) |

When `.claude/rules/` exists (even if empty), slicing `CLAUDE.md` is skipped so the on-disk rules layout is the single source of truth for rule files. `.agnostic-ai/AGNOSTIC_AI.md` is still written from `CLAUDE.md` to keep a CLI-agnostic top-level instructions file alongside `CLAUDE.md` / `AGENTS.md` / `GEMINI.md`.

The instructions file is looked up in the order Claude Code itself reads: `CLAUDE.md`, `.claude/CLAUDE.md`, `AGENTS.md`, `.claude/AGENTS.md`. Since v2.1.277 a session with no `CLAUDE.md` at or above the working directory loads `AGENTS.md`, so a repo set up for other coding agents has its real instructions captured instead of the generic pointer template. The root `AGENTS.md` rung is skipped when `codex`, `amp`, `warp`, `crush`, `kiro`, or `opencode` imports in the same invocation, since that file is their own main file and only one importer may slice it.

The settings overlay captures every non-`hooks` key of `.claude/settings.json` (statusLine, enabledPlugins, model overrides, any other top-level key). `sync -t claude` layers the spec-derived `hooks` key on top, so it reproduces the full settings.json after `.claude/` is wiped. Re-run `import claude` after editing settings.json by hand. For precedence against `outputs.claude.settings`, see [Claude settings](#claude-settings).

Each imported MCP spec round-trips to every MCP-aware target on the next `sync`: codex, copilot, cursor, continue, amp, zed, warp, gemini, opencode.

## Verify

1. Install: `npm install -g @anthropic-ai/claude-code` (or the desktop app; both read the same files).
2. Check the tree: `ls CLAUDE.md .claude/agents/ .claude/skills/ .claude/rules/ .claude/commands/ .claude/settings.json .mcp.json` and `grep "Generated by agnostic-ai" .claude/agents/*.md .claude/rules/*.md .claude/commands/*.md` for the provenance header (it sits after the YAML frontmatter, so `head -1` would only show `---`).
3. Validate JSON: `python -m json.tool .claude/settings.json > /dev/null && python -m json.tool .mcp.json > /dev/null`.
4. Launch `claude` from the project root. `/agents`, `/skills`, and the slash-command picker list every entry. The MCP picker shows each `.mcp.json` server green.
5. Trigger a matcher action (e.g. an `Edit` for `PostToolUse`/`Edit`); the hook command runs with no "schema mismatch" in the log.
6. Confirm `outputs.claude.settings.*` keys under `/config`.
