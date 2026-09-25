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

- **Rules**: one file per spec under `.claude/rules/`. Claude Code loads every `.md` file there (recursively) at session start. A spec with the cross-tool `globs` field (or a native `paths` list) emits `paths:` frontmatter, which scopes the rule to matching files.
- **Legacy rules modes**: `outputs.claude.rules-mode: import` appends a sentinel-marked block of `@.claude/rules/<name>.md` imports to the pointer body, which `import` strips. Use it only on Claude Code versions without native `.claude/rules/` loading. `outputs.claude.rules-file: CLAUDE.md` concatenates rule bodies into that one file and skips the pointer-body write. Any other `rules-file` path still writes `CLAUDE.md` with an `@<path>` import, because a file outside `.claude/rules/` is not auto-loaded, and without `CLAUDE.md` Claude Code reads `AGENTS.md` instead.
- **Skills**: one folder per skill at `.claude/skills/<name>/SKILL.md`. Every `x-claude` key passes through, for example `disable-model-invocation: true`.
- **Commands**: one file per spec at `.claude/commands/<name>.md`. Spec `deploy` becomes `/deploy`. Frontmatter passes through; the body is the prompt template.
- **Agents**: one file per spec at `.claude/agents/<name>.md`. Resolved frontmatter passes through, so a portable `effort` (scalar or per-target map) is written verbatim and not validated. Claude Code documents `low`, `medium`, `high`, `xhigh`, and `max`. See [per-target `model` and `effort`](@/docs/spec-format.md#per-target-model-and-effort).
- **Global agents**: `sync --global --only claude` writes `~/.claude/agents/<name>.md`. See [global configuration](@/docs/configuration.md#global-configuration).
- **MCP**: written to `.mcp.json` under `mcpServers`. Stdio entries use `command`/`args`/`env` with no `type`; remote entries use `type` plus `url`/`headers`. Every entry also accepts `timeout` (per-tool-call timeout in milliseconds; values under 1000 are ignored) and `alwaysLoad` (load the server's tools at session start instead of deferring them behind tool search).
  - `http`, `sse`, and `ws` entries also accept `headersHelper` (a command whose output merges into the connection headers, for non-OAuth auth) and an `oauth` object `{clientId, callbackPort, authServerMetadataUrl, scopes}`, where `scopes` is one space-separated string ([Claude Code MCP docs](https://code.claude.com/docs/en/mcp)). `oauth.clientSecret` is never written; Claude Code keeps it in the system keychain.
  - A spec's `roots` list still emits, but Claude Code documents no per-server `roots` key, so treat it as passthrough. Claude Code derives roots from the launch directory plus directories added with `--add-dir`, `/add-dir`, or `additionalDirectories` ([Claude Code MCP docs](https://code.claude.com/docs/en/mcp)).
  - `disabled: true` adds the server to `disabledMcpjsonServers` in project settings. See [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).
  - Each sync replaces `.mcp.json` as a whole from MCP specs. Import hand-authored servers first.
- **First-class settings**: `outputs.claude.settings.*` declares model, outputStyle, statusLine, permissions, enabledPlugins, env, apiKeyHelper, cleanupPeriodDays, attribution, bashOutputMaxChars, and taskOutputMaxChars. The last two need Claude Code v2.1.261 or later, and taskOutputMaxChars is a no-op from v2.1.277. The deprecated includeCoAuthoredBy stays available for older versions. Copilot CLI also reads enabledPlugins, see [Copilot](@/docs/targets/copilot.md). See [Claude settings](#claude-settings) for precedence.

Hooks support `command`, `http`, `mcp_tool`, and `prompt` handlers:

- HTTP: `url`, optional `headers`, and `allowedEnvVars`.
- MCP: `server`, `tool`, and optional `input`.
- Prompt: `prompt`, optional `model`, and `continueOnBlock`. With `continueOnBlock: true`, a blocking result returns its reason to Claude and the turn continues.
- All stable handlers keep `timeout`, `statusMessage`, `if`, and `once`. `args`, `async`, `asyncRewake`, and `shell` apply to command handlers only. A `command` list becomes one handler per entry; setting `args` switches to exec form, with the executable in `command`.

`import claude` preserves all of these handlers. Experimental agent handlers are not emitted. `once` is written but has no effect, because [Claude Code hooks](https://code.claude.com/docs/en/hooks) honor it only in skill frontmatter and every portable hook lands in `.claude/settings.json`; `sync` prints a note. See [hook fields](@/docs/spec-format.md#hooks).

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

Any other documented event works too (`Setup`, `InstructionsLoaded`, `TaskCompleted`, `TeammateIdle`, `FileChanged`, ...). See the [Claude Code hooks reference](https://code.claude.com/docs/en/hooks).

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

`import claude` and `agnostic-ai doctor` read the same resolved paths, so a moved directory round-trips and unmanaged files under it are still reported.

`dir` moves the whole tool directory: with `dir: vendor/.claude`, rules land in `vendor/.claude/rules/`, commands in `vendor/.claude/commands/`, and `{{rules_dir}}` and the other path variables resolve there. A per-kind key overrides its own path. After moving `dir`, the old files stay until the next full sync sweeps them as orphans. Claude Code auto-loads only a project-root `.claude/rules/`, so a moved rules directory needs `rules-mode: import`.

With `gitignore.enabled`, the managed `.gitignore` block also lists `/.claude/agent-memory-local/` and `/.claude/settings.local.json`, following `outputs.claude.dir`. `.claude/agent-memory/` (`memory: project`) stays out because Claude Code documents it as shareable. An ignore line does not untrack files, so run `git rm -r --cached .claude/agent-memory-local` if that store is already committed.

## Agent memory

A top-level `memory` key gives a subagent a directory that persists across sessions. Only Claude Code acts on it. Junie copies the key into its agent file unchanged, and every other adapter drops it.

```yaml
---
name: code-reviewer
description: Reviews diffs for bugs and style.
memory: project
---
```

| Scope | Directory | Git |
|---|---|---|
| `user` | `~/.claude/agent-memory/<name>/` | outside the repository |
| `project` | `.claude/agent-memory/<name>/` | shareable; commit it if the team wants it shared |
| `local` | `.claude/agent-memory-local/<name>/` | do not commit |

Claude Code creates the directory on first use. agnostic-ai emits the frontmatter key and never reads or writes the store.

This is separate from session auto memory under `~/.claude/projects/<project>/memory/`, which agnostic-ai leaves alone. It still needs auto memory enabled: with `autoMemoryEnabled` off or `CLAUDE_CODE_DISABLE_AUTO_MEMORY` set, `memory` has no effect. See [Memory and local state](@/docs/target-behavior.md#memory-and-local-state).

Disabled MCP servers are tracked in `.claude/.agnostic-ai-mcp-disabled.json`, which lists only generated rejection entries. Re-enabling a server removes its generated rejection and keeps manual entries and unrelated settings. Keep this file with the generated settings; both follow `outputs.claude.dir`. A disabled server requires the project `.mcp.json` path; a custom MCP path fails with an actionable error. Import restores disabled state from the project rejection list and keeps generated rejections out of the settings overlay, so a re-enabled server's old rejection does not come back.

## Claude settings

The `outputs.claude.settings` block declares first-class `.claude/settings.json` keys. Layers, from lowest to highest precedence:

1. Captured overlay (from `import claude`).
2. Agnostic `settings` specs (`.agnostic-ai/settings/`), the cross-tool source for `permissions`, `model`, and `effort`. `effort` lands as `effortLevel` when it is `low`, `medium`, `high`, or `xhigh`.
3. This `outputs.claude.settings` block.
4. The spec-derived `hooks` block.
5. An `x-claude` block on a settings spec.

Unset keys fall through to lower layers. `x-claude` merges with what lower layers wrote: `x-claude.permissions.deny` adds to the translated deny list, and only scalars such as `model` are replaced.

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
| `bashOutputMaxChars` | integer | Inline command output limit before it spills to a file, up to 128000. Needs Claude Code v2.1.261 or later. |
| `taskOutputMaxChars` | integer | Deprecated. The same limit for background-task output, up to 128000. Needs v2.1.261 or later, and has no effect from v2.1.277 (which removed the TaskOutput tool and `TASK_MAX_OUTPUT_LENGTH`). Still emitted because it works on the older `stable` release. |
| `attribution` | object | `commit` and `pr` set the attribution text for commits and pull requests; an empty string disables it. `sessionUrl` controls whether the session URL is included. |
| `includeCoAuthoredBy` | boolean | Deprecated Claude Code setting. Use `attribution`; when both are present, `attribution` takes precedence. |
| `enabledPlugins` | map of string to boolean | `plugin-id@marketplace-id` keys mapped to `true` to enable them, matching Claude Code's settings schema. |
| `env` | map of strings | Environment variables exported into Claude sessions. |
| `statusLine` | object | `type`, `command`, and optional `padding`, `refreshInterval` (seconds, minimum 1), and `hideVimModeIndicator`. |
| `permissions` | object | `allow`, `deny`, `ask` lists of tool-pattern strings. |

Any other setting round-trips through the overlay captured by `agnostic-ai import claude` (`.agnostic-ai/overlays/claude.settings.json`). For a scalar key set in both, this config block wins. `permissions` `allow`/`deny`/`ask` lists are the exception: they are unioned across the overlay, `settings` specs, and this block, so no layer drops another's rules.

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
| `.claude/settings.json` hooks | `<hooks>/<event>[-<matcher-slug>]-<hash8>.yaml` (command hooks keep their grouping; HTTP, MCP-tool, and prompt handlers keep their native fields in separate specs) |
| `.claude/settings.json` non-hook keys | `.agnostic-ai/overlays/claude.settings.json` |
| `.mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml` (one spec per server) |

When `.claude/rules/` exists (even if empty), `CLAUDE.md` is not sliced, so the on-disk rules are the single source for rule files. `.agnostic-ai/AGNOSTIC_AI.md` is still written from `CLAUDE.md`.

The instructions file is looked up in Claude Code's own order: `CLAUDE.md`, `.claude/CLAUDE.md`, `AGENTS.md`, `.claude/AGENTS.md`. Since v2.1.277, a session with no `CLAUDE.md` at or above the working directory loads `AGENTS.md`, so repos set up for other agents get their real instructions captured. The root `AGENTS.md` step is skipped when `codex`, `amp`, `warp`, `crush`, `kiro`, or `opencode` imports in the same run, since only one importer may slice that file.

The settings overlay captures every non-`hooks` key of `.claude/settings.json`. `sync -t claude` layers the spec-derived `hooks` on top, reproducing the full file after `.claude/` is wiped. Re-run `import claude` after editing settings.json by hand. For precedence, see [Claude settings](#claude-settings).

`effortLevel` is the one key import moves out of the overlay. A valid value becomes `effort` in `<settings>/claude.yaml`, so every target syncs it, unless another settings spec already sets a different effort; then it goes under `x-claude.effortLevel` in that file. An invalid value stays in the overlay.

Imported MCP specs sync to every MCP-aware target: codex, copilot, cursor, continue, amp, zed, warp, gemini, opencode.

## Verify

1. Install: `npm install -g @anthropic-ai/claude-code` (or the desktop app; both read the same files).
2. Check the tree: `ls CLAUDE.md .claude/agents/ .claude/skills/ .claude/rules/ .claude/commands/ .claude/settings.json .mcp.json`. Check provenance with `grep "Generated by agnostic-ai" .claude/agents/*.md .claude/rules/*.md .claude/commands/*.md` (the header follows the frontmatter, so `head -1` shows only `---`).
3. Validate JSON: `python -m json.tool .claude/settings.json > /dev/null && python -m json.tool .mcp.json > /dev/null`.
4. Run `claude` from the project root. `/agents`, `/skills`, and the slash-command picker list every entry. The MCP picker shows each `.mcp.json` server green.
5. Trigger a matcher action (for example an `Edit` for `PostToolUse`/`Edit`). The hook runs with no "schema mismatch" in the log.
6. Confirm `outputs.claude.settings.*` keys under `/config`.
