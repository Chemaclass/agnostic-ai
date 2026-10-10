+++
title = "Claude Code"
description = "How agnostic-ai writes Claude Code configuration: file paths, what Claude Code cannot hold, and output options."
weight = 10

[extra]
group = "Reference"
target_id = "claude"
+++

# Claude Code (`claude`)

Claude Code reads `CLAUDE.md`, the `.claude/` tree, and `.mcp.json`.

## Output

```
CLAUDE.md                # entry-point pointer body (written by sync)
.claude/
├── agents/<name>.md
├── skills/<name>/SKILL.md
├── rules/<name>.md
├── commands/<name>.md
└── settings.json
.mcp.json
```

- **Instructions**: `CLAUDE.md` carries `.agnostic-ai/AGNOSTIC_AI.md`.
  - When another target writes the root `AGENTS.md` (Codex, Amp, Warp, OpenCode, Zed, and others), `CLAUDE.md` is `@AGENTS.md` plus your `::target claude` blocks. A full copy stays when the files would differ (a rules appendix in `AGENTS.md`, or a `::target` block Claude Code skips).
  - If `CLAUDE.md` already imports `AGENTS.md` and no other target writes it, sync keeps that layout and writes the shared body to `AGENTS.md`.
  - A session started in a subfolder treats the root `@AGENTS.md` as an outside import: interactive ones ask once, headless `claude -p` skips it.
  - With `builtins: [memory]`, `CLAUDE.md` ends with imports of the project and personal memory indexes. See [shared memory](@/docs/memory.md).
- **Rules**: Claude Code loads every `.md` file under `.claude/rules/` (recursively) at session start. `globs` (or a native `paths` list) writes `paths:` frontmatter, which scopes the rule to matching files. A comma-separated string gives one entry per pattern. The portable `scope`, `globs`, and `alwaysApply` keys are dropped once translated.

  Scope and patterns combine: `scope: src/a` with `globs: tests/a/**` writes `src/a/**` and `tests/a/**`. To filter by file only, omit `scope` and keep the source outside a folder that implies one.
- **Legacy rules modes**:
  - `outputs.claude.rules-mode: import` appends a block of `@.claude/rules/<name>.md` imports to the pointer body. Use it only on Claude Code versions that do not load `.claude/rules/` natively.
  - `outputs.claude.rules-file: CLAUDE.md` concatenates rule bodies into that file.
  - Any other `rules-file` path still writes `CLAUDE.md` with an `@<path>` import, since Claude Code does not auto-load files outside `.claude/rules/`.
- **Skills** (`.claude/skills/<name>/SKILL.md`): every `x-claude` key passes through, such as `disable-model-invocation: true`. Skill `model` and `effort` take a scalar or a per-target map, and `x-claude` wins.
- **Commands** (`.claude/commands/<name>.md`): spec `deploy` becomes `/deploy`. Frontmatter passes through.
- **Agents** (`.claude/agents/<name>.md`): frontmatter passes through, so sync writes a portable `effort` (scalar or per-target map) as is. Claude Code accepts `low`, `medium`, `high`, `xhigh`, and `max`. See [per-target `model` and `effort`](@/docs/spec-format/agents.md#per-target-model-and-effort).
- **Agent names**: Claude Code skips an agent whose name starts with `-` or contains `:` (the separator of plugin-scoped names such as `my-plugin:reviewer`). Sync checks the name Claude Code sees, including an `x-claude` override, and stops with an error before writing anything, for every tool. Rename the agent to continue.
- **Read-only agents**: `readonly: true` becomes `disallowedTools: Write, Edit, NotebookEdit` in project and global agents. Bash stays allowed. A `disallowedTools` value in the spec or `x-claude` wins; `x-claude.disallowedTools: null` omits it. `readonly: false` writes nothing.
- **Global agents**: `sync --global --only claude` writes `~/.claude/agents/<name>.md` ([global configuration](@/docs/configuration.md#global-configuration)).
- **Environment**: `dev-commands` become `.claude/launch.json` [preview servers](https://code.claude.com/docs/en/desktop#configure-preview-servers); `setup` becomes a worktree setup hook.
- **MCP**: `mcpServers`: stdio uses `command`/`args`/`env` with no `type`; remote uses `type` plus `url`/`headers`. All entries take `timeout` (per tool call, milliseconds; under 1000 ignored).
  - `alwaysLoad: true` keeps the server's tools visible without tool search. Since Claude Code v2.1.287, `alwaysLoad: false` defers all of its tools behind tool search. Set `bareElicitationCapability: true` if a server stops connecting after that version's URL elicitation update ([changelog](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md#21287)).
  - Import and sync keep explicit `true` and `false` for both flags. An `x-claude` value overrides the matching top-level flag. Other targets omit both flags.
  - `http`, `sse`, and `ws` also take `headersHelper` (a command whose output merges into headers, for non-OAuth auth) and `oauth` `{clientId, callbackPort, authServerMetadataUrl, scopes}`, with space-separated `scopes` ([Claude Code MCP docs](https://code.claude.com/docs/en/mcp)). `oauth.clientSecret` is never written; Claude Code keeps it in the system keychain.
  - `roots` passes through.
  - `disabled: true` adds the server to `disabledMcpjsonServers` in project settings ([`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target)).
  - Each sync rebuilds `.mcp.json` from MCP specs, so import hand-written servers first.
- **Settings**: `outputs.claude.settings.*` declares `.claude/settings.json` keys. See [Claude settings](#claude-settings) for the keys and which layer wins. Copilot CLI also reads `enabledPlugins` ([Copilot](@/docs/targets/copilot.md)).

{% <details summary="Environment and worktree setup"> %}
In `.claude/launch.json`, `command` splits into `runtimeExecutable` and `runtimeArgs`, and `auto-port` becomes `autoPort`. `cwd` is written as given, relative to the project root ([configuration fields](https://code.claude.com/docs/en/desktop#configuration-fields)). `x-claude.autoVerify` sets `autoVerify`. The file follows `outputs.claude.dir`.

`import claude` reads a hand-written `launch.json` into `environments/dev.yaml`: `${workspaceFolder}/apps/docs` becomes `cwd: apps/docs`; a bare `${workspaceFolder}` drops `cwd`. A configuration with no command (such as a `url` alone) keeps the whole file as written, with a note.

`setup` becomes `.claude/hooks/agnostic-ai-worktree-setup.sh`. [`SessionStart`](https://code.claude.com/docs/en/hooks#sessionstart) (matcher `startup`), [`SubagentStart`](https://code.claude.com/docs/en/hooks#subagentstart), and `PostToolUse` (matcher `EnterWorktree`) hooks run it once per new linked worktree. They set `shell: bash`, so Windows needs Git Bash. `x-claude.setup: false` omits them. See [Claude Code worktree setup](@/docs/spec-format/environments.md#claude-code-worktree-setup).

Other fields, such as `install`, `setup-windows`, `cleanup`, `terminals`, or a Cursor `environment.json` key, have no Claude Code file and get a note.
{% </details> %}

{% <details summary="Disabled MCP server tracking"> %}
`.claude/.agnostic-ai-mcp-disabled.json` lists generated rejections only. Re-enabling a server removes its entry. Disabling needs the project `.mcp.json` path; a custom path fails with an error.
{% </details> %}

Hooks support `command`, `http`, `mcp_tool`, and `prompt` handlers:

- HTTP: `url`, optional `headers`, and `allowedEnvVars`.
- MCP: `server`, `tool`, and optional `input`.
- Prompt: `prompt`, optional `model`, and `continueOnBlock`. With `continueOnBlock: true`, a blocking result returns its reason to Claude and the turn continues.
- All handlers keep `timeout`, `statusMessage`, `if`, and `once`. `args`, `async`, `asyncRewake`, and `shell` are command-only.
- `failClosed: true` writes `onFailure: "block"`: a command or HTTP hook failure applies the event's exit-2 behavior. On `PermissionRequest`, it denies the request. Import reads the key back as `failClosed: true`.
- Claude Code ignores `failClosed` on `Stop`, `SubagentStop`, `TaskCompleted`, `TeammateIdle`, and command handlers with `async: true` or `asyncRewake: true`. Sync notes these cases and keeps the key for import round trips. Use a synchronous `PreToolUse` or `UserPromptSubmit` hook when a failed check must block an action. See [hook failure behavior](https://code.claude.com/docs/en/hooks#block-the-action-when-a-hook-fails).
- Claude Code documents `onFailure` for command and HTTP hooks only. Import keeps any other value, or the key on an MCP-tool or prompt handler, as `x-claude.onFailure`, and sync writes it back as written. `failClosed: true` wins over it.
- A `command` list gives one handler per entry. `args` switches to exec form, with the executable in `command`.

With a command hook present, sync adds `AGNOSTIC_AI_TARGET: claude` to the settings `env`. Every hook and other session process sees it. `outputs.claude.settings.env` or `x-claude.env` wins. See [which target ran a hook](@/docs/spec-format/hooks.md#hook-target).

When `cursor` is also a target, a portable hook that reaches Cursor gets `[ "$AGNOSTIC_AI_TARGET" = cursor ] && exit 0;` before its command, so Cursor, which also loads `.claude/settings.json`, runs it once. See [hooks Cursor and Copilot also read](@/docs/spec-format/hooks.md#claude-settings-copies).

Sync does not write experimental agent handlers. `once` is written but has no effect, with a sync note: Claude Code honors it only in skill frontmatter. See [hook fields](@/docs/spec-format/hooks.md).

`event` passes through as written. Common events:

| Event | When it fires |
|-------|---------------|
| `PreToolUse` | Before a tool call. Matcher is the tool name regex. |
| `PostToolUse` | After a tool call. Matcher is the tool name regex. |
| `PostToolUseFailure` | After a tool call fails. |
| `PermissionRequest` | When a permission dialog appears. |
| `UserPromptSubmit` | Before the model reads a new user message. |
| `SubagentStart` / `SubagentStop` | When a subagent starts / finishes. |
| `Stop` | When the model stops generating. |
| `Notification` | When Claude Code shows a system notification. |
| `SessionStart` / `SessionEnd` | When a session begins / ends. |
| `PreCompact` / `PostCompact` | Around context compaction. |

Any other event in the [Claude Code hooks reference](https://code.claude.com/docs/en/hooks) works too, such as `Setup` or `FileChanged`.

## Config keys

| Key | Default | Notes |
|---|---|---|
| `outputs.claude.dir` | `.claude` | |
| `outputs.claude.rules-dir` | `.claude/rules` | auto-loaded by Claude Code |
| `outputs.claude.rules-mode` | unset | set to `import` to wire `.claude/rules/*.md` into `CLAUDE.md` via `@`-imports; only for Claude Code versions without native rules loading |
| `outputs.claude.rules-file` | unset | legacy single-file layout, typically `CLAUDE.md`; any other path keeps `CLAUDE.md` and wires the file in with an `@`-import |
| `outputs.claude.commands-dir` | `.claude/commands` | |
| `outputs.claude.agents-dir` | `.claude/agents` | |
| `outputs.claude.skills-dir` | `.claude/skills` | |
| `outputs.claude.mcp-file` | `.mcp.json` | |
| `outputs.claude.settings` | | settings block for `.claude/settings.json` |

`import claude` and `agnostic-ai doctor` read the resolved paths, so a moved directory still round-trips.

`dir` moves the whole tool directory: with `dir: vendor/.claude`, rules land in `vendor/.claude/rules/`, commands in `vendor/.claude/commands/`, and <code>{&#123;rules_dir}}</code> and other path variables resolve there. Per-kind keys override their own path. The next full sync removes old files as orphans. Claude Code auto-loads only a project-root `.claude/rules/`, so a moved rules directory needs `rules-mode: import`.

With `gitignore.enabled`, the managed `.gitignore` block also lists `/.claude/agent-memory-local/`, `/.claude/settings.local.json`, `/.claude/worktrees/`, and `/.claude/scheduled_tasks.lock`, following `outputs.claude.dir`. It omits `.claude/agent-memory/` (`memory: project`), which is shareable. Ignoring does not untrack: run `git rm -r --cached .claude/agent-memory-local` if that store is committed.

The block also goes into `.worktreeinclude`, so new worktrees start with the ignored outputs ([gitignore](@/docs/configuration.md#gitignore)). `gitignore.ignore-worktree-include: true` ignores the managed `.worktreeinclude` too; `gitignore.worktree-include: false` leaves it to you.

## Agent memory

A top-level `memory` key gives a subagent a directory that persists across sessions. Only Claude Code acts on it; Junie copies the key unchanged, and other adapters drop it.

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

Claude Code creates the directory on first use. Session auto memory (`~/.claude/projects/<project>/memory/`) is separate. Sync leaves it alone unless the [`memory` built-in](@/docs/memory.md#claude-code-s-own-memory) points it at the shared personal store. That needs auto memory on: with `autoMemoryEnabled` off or `CLAUDE_CODE_DISABLE_AUTO_MEMORY` set, Claude saves nothing there. See [Memory and local state](@/docs/target-behavior.md#memory-and-local-state).

## Claude settings

`sync --global --only claude` maps a home settings spec's `permissions.default-mode` to `permissions.defaultMode` in `~/.claude/settings.json`, keeping hand-written permission rules ([global settings](@/docs/configuration.md#global-default-model-and-effort)).

`outputs.claude.settings` declares `.claude/settings.json` keys directly. Layers, lowest first (a later layer wins):

1. Captured overlay (from `import claude`).
2. Agnostic `settings` specs (`.agnostic-ai/settings/`), the cross-tool source for `permissions`, `model`, and `effort`. `effort` lands as `effortLevel` when it is `low`, `medium`, `high`, or `xhigh`.
3. This `outputs.claude.settings` block.
4. The `hooks` block built from your hook specs.
5. An `x-claude` block on a settings spec.

Unset keys fall through to the layer below. `x-claude` merges: `x-claude.permissions.deny` adds to the translated deny list, and only scalars such as `model` are replaced.

Without a captured overlay, sync merges permission lists into the on-disk `settings.json` and records the rules it added in `.claude/.agnostic-ai-permissions.json`. The next sync removes those first, so a rule dropped from a spec, or moved from `allow` to `deny`, leaves the file. Hand-written and overlay rules stay. The first sync without a record keeps every rule and records the spec ones.

```yaml
outputs:
  claude:
    settings:
      model: claude-opus-4-7
      outputStyle: verbose
      apiKeyHelper: ./bin/keyhelper.sh
      cleanupPeriodDays: 30
      bashOutputMaxChars: 64000
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
| `taskOutputMaxChars` | integer | Retired, not written. Sync removes a copy an earlier sync wrote and prints a note; delete it from `agnostic-ai.yaml`. A copy in `.agnostic-ai/overlays/claude.settings.json` is yours: sync keeps it and names it in a note. |
| `attribution` | object | `commit` and `pr` set the attribution text for commits and pull requests; an empty string disables it. `sessionUrl` controls whether the session URL is included. |
| `includeCoAuthoredBy` | boolean | Deprecated. Use `attribution`; when both are present, `attribution` wins. |
| `enabledPlugins` | map of string to boolean | `plugin-id@marketplace-id` keys mapped to `true` to enable them. |
| `env` | map of strings | Environment variables exported into Claude sessions. |
| `statusLine` | object | `type`, `command`, and optional `padding`, `refreshInterval` (seconds, minimum 1), and `hideVimModeIndicator`. |
| `permissions` | object | `allow`, `deny`, `ask` lists of tool-pattern strings. |

Other settings survive import and sync through the overlay `agnostic-ai import claude` captures (`.agnostic-ai/overlays/claude.settings.json`). For a scalar in both, this block wins. `permissions` `allow`/`deny`/`ask` lists combine across the overlay, `settings` specs, and this block.

## Import

`agnostic-ai import claude` reads the Claude Code tree:

| Source | Becomes |
|--------|---------|
| `.claude/rules/**/*.md` (preferred) | `<rules>/<sub>/<name>.md` (byte-identical copy) |
| `CLAUDE.md` rules block written by `sync` | `<rules>/<name>.md` per rule (only when `.claude/rules/` is absent) |
| `CLAUDE.md` (any form) | `.agnostic-ai/AGNOSTIC_AI.md` (byte-identical copy) |
| `AGENTS.md` or `.claude/AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` (only when no `CLAUDE.md` exists) |
| `CLAUDE.md` that imports `@AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md`: the `AGENTS.md` text, then the rest of `CLAUDE.md` in a `::target claude` fence (no rules) |
| `<dir>/CLAUDE.md` (nested, hand-written) | one rule with the whole file and `scope: <dir>`, named after the scope: `api.md` for `services/api/`, or `services-api.md` when another scope also ends in `api` (only when `.claude/rules/` is absent) |
| `<dir>/CLAUDE.md` that imports `@AGENTS.md` | the same rule, with the `AGENTS.md` text, then the rest of `CLAUDE.md` in a `::target claude` fence. When `codex` imports in the same run, only the fenced text lands, as `<scope>-claude.md` |
| `.claude/agents/*.md` | `<agents>/<name>.md` (byte-identical copy, except a Claude model name becomes `model: {claude: <name>}`) |
| `.claude/skills/<name>/SKILL.md` | `<skills>/<name>/SKILL.md` (`allowed-tools` moves under `x-claude:`) |
| `.claude/commands/*.md` | `<commands>/<name>.md` (`allowed-tools` moves under `x-claude:`) |
| `.claude/settings.json` hooks | `<hooks>/<event>[-<matcher>]-<command>.yaml`, such as `pretooluse-bash-exit-0.yaml`, with a `description` (HTTP, MCP-tool, and prompt handlers keep their native fields in separate specs; `target: claude` unless the hook can be shared, see below) |
| `.claude/settings.json` `permissions.allow`, `ask`, `deny` | `<settings>/permissions-claude.yaml` (a portable settings spec) |
| `.claude/settings.json` other non-hook keys | `.agnostic-ai/overlays/claude.settings.json` |
| `.mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml` (one spec per server) |

**Instructions.** A hand-written `CLAUDE.md` imports whole, not split into rules, since each section would then load twice. When `.claude/rules/` exists (even empty), rule files come only from it. Import follows Claude Code's lookup order: `CLAUDE.md`, `.claude/CLAUDE.md`, `AGENTS.md`, `.claude/AGENTS.md`. Only one importer may slice the root `AGENTS.md`, so import skips it when `codex`, `amp`, `warp`, `crush`, `kiro`, or `opencode` imports too. Delete `.claude/CLAUDE.md` after the next sync, as import says, or Claude Code loads both it and the root file.

**Nested `CLAUDE.md`.** Sync writes nested rules to `.claude/rules/<dir>/`, so import tells you to delete each nested `CLAUDE.md` it read. `doctor --fix` deletes one still matching its rule and keeps one you edited. Once `.claude/rules/` holds a directory's scoped rules, `sync` itself deletes a nested `CLAUDE.md` that only imports `@AGENTS.md`. Any other line, or a `sync.unmanaged` entry, keeps the file. `sync --backup` keeps the deleted file as `CLAUDE.md.bak`; `revert` restores it.

**Agents, skills, commands.** `allowed-tools` moves under `x-claude:`, where `lint` accepts it and sync writes it back. An agent `model` naming a Claude model (an alias such as `opus` or `fable`, `inherit`, or a `claude-*` id) imports as `model: {claude: <name>}`, so other targets use their default; sync writes `model: <name>` back.



**Settings.** Permission lists become a portable settings spec that `lint` checks and every target gets (with a coverage note where unsupported). Import skips rules that another settings spec or `outputs.claude.settings.permissions` declares. Add `target: claude` to keep them Claude-only. The overlay captures every other non-`hooks` key, including `permissions.defaultMode`. Re-import after editing settings.json by hand.

`effortLevel` is the one key import moves out of the overlay. A valid value becomes `effort` in `<settings>/claude.yaml`, which every target syncs, unless another settings spec sets a different effort; then it goes under `x-claude.effortLevel`. An invalid value stays in the overlay.



{% <details summary="Unread files and rewritten paths"> %}
The import summary lists each unread file under `.claude/`, such as `.claude/templates/post.md`. Sync leaves them in place. A skill that reads one still finds it, but other tools get no copy.

Imported text naming a Claude path, such as `.claude/skills/style/SKILL.md`, now names its source, `.agnostic-ai/skills/style/SKILL.md`. An `@.claude/rules/<name>.md` line for an imported rule is dropped. Import reports each Claude path it leaves alone.
{% </details> %}



{% <details summary="Portable imported hooks"> %}
An imported hook is pinned with `target: claude` unless every other configured target can check it and run it as written. Only Codex can check today. The hook must use an event Codex has, match only tools Codex reports (`Bash`, `apply_patch`, `Edit`, `Write`, `mcp__*`) or a `SessionStart` or compact source, use a command or `mcp_tool` handler, and not set `if`, `shell`, `once`, or `asyncRewake`. So in a `claude,codex` project a `PreToolUse` hook on `Bash` stays portable. With a tool that cannot check, such as `cursor`, every pin stays. The import summary explains each kept pin.
{% </details> %}

## Protected paths

Enforced (permission). Each path in a settings spec's `protected` block becomes an `Edit(/<path>)` rule in `permissions.ask` or `permissions.deny` of `.claude/settings.json`, joined with the portable permission lists. The leading `/` anchors the rule at the project root.

Claude Code ignores a `Write(<path>)` rule and warns at startup, so sync writes only `Edit` rules ([permissions](https://code.claude.com/docs/en/permissions#read-and-edit)). `decision: deny` rules also cover file commands Claude Code recognizes in Bash, such as `sed` and `>` redirections, but not a script that opens files itself. An `ask` rule guards edit tools only.

Removing a path, or moving it from `deny` to `ask`, drops its old rule on the next sync ([Claude settings](#claude-settings)). See [Protected paths](@/docs/spec-format/settings.md#protected-paths).

## Verify

1. Install: `npm install -g @anthropic-ai/claude-code`.
2. Check the tree: `ls CLAUDE.md .claude/agents/ .claude/skills/ .claude/rules/ .claude/commands/ .claude/settings.json .mcp.json`. Check the generated-file header with `grep "Generated by agnostic-ai" .claude/agents/*.md .claude/rules/*.md .claude/commands/*.md`.
3. Validate JSON: `python -m json.tool .claude/settings.json > /dev/null && python -m json.tool .mcp.json > /dev/null`.
4. Run `claude` from the project root. `/agents`, `/skills`, and the slash-command picker list every entry. The MCP picker shows each `.mcp.json` server green.
5. Trigger a matcher action (for example an `Edit` for `PostToolUse`/`Edit`). The hook runs with no "schema mismatch" in the log.
6. Confirm `outputs.claude.settings.*` keys under `/config`.
