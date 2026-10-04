+++
title = "Claude Code"
description = "How agnostic-ai emits Claude Code configuration: native paths, capability limits, and output options."
weight = 10

[extra]
group = "Reference"
target_id = "claude"
+++

# Claude Code (`claude`)

Claude Code reads `CLAUDE.md`, the `.claude/` tree, and `.mcp.json`.

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

- **Instructions**: `CLAUDE.md` carries `.agnostic-ai/AGNOSTIC_AI.md`.
  - When another target writes the root `AGENTS.md` (Codex, Amp, Warp, OpenCode, Zed, and others), `CLAUDE.md` is `@AGENTS.md` plus your `::target claude` blocks. Claude Code expands the import; Cursor, loading both files, reads the text once.
  - A full copy stays when the files would differ (a rules appendix in `AGENTS.md`, or a `::target` block Claude Code skips).
  - If `CLAUDE.md` already imports `AGENTS.md` and no other target writes it, sync keeps that layout and writes the shared body to `AGENTS.md`.
  - Sessions started in a subfolder treat the root `@AGENTS.md` as an outside import: interactive ones ask once, headless `claude -p` skips it.
- **Rules**: Claude Code loads every `.md` file under `.claude/rules/` (recursively) at session start. `globs` (or a native `paths` list) emits `paths:` frontmatter, which scopes the rule to matching files. A comma-separated string gives one entry per pattern. Once translated into `paths`, the portable `scope`, `globs`, and `alwaysApply` keys are dropped.

  Scope and patterns form a union: `scope: src/a` with `globs: tests/a/**` emits `src/a/**` and `tests/a/**`. To filter by file only, omit `scope` and keep the source outside a folder that implies one.
- **Legacy rules modes**:
  - `outputs.claude.rules-mode: import` appends a sentinel-marked block of `@.claude/rules/<name>.md` imports to the pointer body (`import` strips it). Use it only on Claude Code versions that do not load `.claude/rules/` natively.
  - `outputs.claude.rules-file: CLAUDE.md` concatenates rule bodies into that file, with no pointer body.
  - Any other `rules-file` path still writes `CLAUDE.md` with an `@<path>` import: Claude Code does not auto-load files outside `.claude/rules/`, and without `CLAUDE.md` it reads `AGENTS.md`.
- **Skills** (`.claude/skills/<name>/SKILL.md`): every `x-claude` key passes through, such as `disable-model-invocation: true`. Skill `model` and `effort` take a scalar or a per-target map in project and global sync, and `x-claude` wins. Claude applies them for the rest of the turn.
- **Commands** (`.claude/commands/<name>.md`): spec `deploy` becomes `/deploy`. Frontmatter passes through; the body is the prompt template.
- **Agents** (`.claude/agents/<name>.md`): resolved frontmatter passes through, so sync writes a portable `effort` (scalar or per-target map) as is, without validation. Claude Code documents `low`, `medium`, `high`, `xhigh`, and `max`. See [per-target `model` and `effort`](@/docs/spec-format/agents.md#per-target-model-and-effort).
- **Read-only agents**: `readonly: true` becomes `disallowedTools: Write, Edit, NotebookEdit` in project and global agents, and `readonly` itself is dropped. Bash stays allowed. Cursor's read-only mode differs: it also blocks state-changing shell commands. A `disallowedTools` value in the spec or `x-claude` wins; `x-claude.disallowedTools: null` omits it. `readonly: false` writes nothing.
- **Global agents**: `sync --global --only claude` writes `~/.claude/agents/<name>.md` ([global configuration](@/docs/configuration.md#global-configuration)).
- **Environment**: `dev-commands` become `.claude/launch.json` preview servers ([configure preview servers](https://code.claude.com/docs/en/desktop#configure-preview-servers)); `setup` becomes a worktree setup hook.
- **MCP**: `mcpServers`: stdio uses `command`/`args`/`env` with no `type`; remote uses `type` plus `url`/`headers`. All entries take `timeout` (per tool call, milliseconds; under 1000 ignored).
  - `alwaysLoad: true` keeps the server's tools visible without tool search. Since Claude Code v2.1.287, `alwaysLoad: false` defers all of its tools behind tool search. Set `bareElicitationCapability: true` if a server stops connecting after that version's URL elicitation update ([Claude Code v2.1.287 changelog](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md#21287)).
  - Import and sync preserve explicit `true` and `false` for both flags on every transport. An `x-claude` value overrides the matching top-level flag; absent or non-Boolean values are omitted. Other targets omit both flags.
  - `http`, `sse`, and `ws` also take `headersHelper` (a command whose output merges into headers, for non-OAuth auth) and `oauth` `{clientId, callbackPort, authServerMetadataUrl, scopes}`, with space-separated `scopes` ([Claude Code MCP docs](https://code.claude.com/docs/en/mcp)). `oauth.clientSecret` is never written; Claude Code keeps it in the system keychain.
  - `roots` passes through, though Claude Code documents no per-server `roots`; it derives roots from the launch directory plus `--add-dir`, `/add-dir`, or `additionalDirectories` ([Claude Code MCP docs](https://code.claude.com/docs/en/mcp)).
  - `disabled: true` adds the server to `disabledMcpjsonServers` in project settings ([`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target)).
  - Each sync rebuilds `.mcp.json` from MCP specs, so import hand-authored servers first.
- **Settings**: `outputs.claude.settings.*` declares model, outputStyle, statusLine, permissions, enabledPlugins, env, apiKeyHelper, cleanupPeriodDays, attribution, and bashOutputMaxChars (Claude Code v2.1.261 or later). taskOutputMaxChars is accepted but not written, since Claude Code v2.1.277 removed it. Deprecated includeCoAuthoredBy stays for older versions. Copilot CLI also reads enabledPlugins ([Copilot](@/docs/targets/copilot.md)). For precedence, see [Claude settings](#claude-settings).

{% <details summary="Environment and worktree setup"> %}
In `.claude/launch.json`, `command` splits into `runtimeExecutable` and `runtimeArgs`, and `auto-port` becomes `autoPort`. `cwd` is written as given, relative to the project root ([configuration fields](https://code.claude.com/docs/en/desktop#configuration-fields)). `x-claude.autoVerify` sets `autoVerify`. The file follows `outputs.claude.dir`; Claude Code reads it from `.claude/` in the session's start folder.

`import claude` reads a hand-written `launch.json` into `environments/dev.yaml`: `${workspaceFolder}/apps/docs` becomes `cwd: apps/docs`; a bare `${workspaceFolder}` drops `cwd`. A configuration with no command (such as a `url` alone) keeps the whole file as written, with a note, since sync rebuilds `launch.json` from the spec.

`setup` becomes `.claude/hooks/agnostic-ai-worktree-setup.sh`. `SessionStart` (matcher `startup`), `SubagentStart`, and `PostToolUse` (matcher `EnterWorktree`) hooks run it once per new linked worktree, from the root the payload's `cwd` names ([SessionStart](https://code.claude.com/docs/en/hooks#sessionstart), [SubagentStart](https://code.claude.com/docs/en/hooks#subagentstart)). They set `shell: bash`, so Windows needs Git Bash. `x-claude.setup: false` omits them; `import claude` skips them. See [Claude Code worktree setup](@/docs/spec-format/environments.md#claude-code-worktree-setup).

Other fields, such as `install`, `setup-windows`, `cleanup`, `terminals`, or a Cursor `environment.json` key, have no Claude Code file and get a note.
{% </details> %}

{% <details summary="Disabled MCP server tracking"> %}
`.claude/.agnostic-ai-mcp-disabled.json` lists generated rejections only. Re-enabling a server removes its entry and keeps manual entries and unrelated settings. Keep it with the generated settings; both follow `outputs.claude.dir`. Disabling needs the project `.mcp.json` path. A custom path fails with an actionable error. Import restores disabled state from the project rejection list and keeps generated rejections out of the overlay, so a re-enabled server's old rejection stays gone.
{% </details> %}

Hooks support `command`, `http`, `mcp_tool`, and `prompt` handlers:

- HTTP: `url`, optional `headers`, and `allowedEnvVars`.
- MCP: `server`, `tool`, and optional `input`.
- Prompt: `prompt`, optional `model`, and `continueOnBlock`. With `continueOnBlock: true`, a blocking result returns its reason to Claude and the turn continues.
- All stable handlers keep `timeout`, `statusMessage`, `if`, and `once`. `args`, `async`, `asyncRewake`, and `shell` are command-only.
- A `command` list gives one handler per entry. `args` switches to exec form, with the executable in `command`.

With a command hook present, sync adds `AGNOSTIC_AI_TARGET: claude` to the settings `env` (Claude Code hooks have no `env`). Shell, exec, and PowerShell hooks and every other session process see it. `outputs.claude.settings.env` or `x-claude.env` wins. See [which target ran a hook](@/docs/spec-format/hooks.md#hook-target).

When `cursor` is also a target, a portable hook that reaches Cursor gets `[ "$AGNOSTIC_AI_TARGET" = cursor ] && exit 0;` before its command, so Cursor, which also loads `.claude/settings.json`, runs it once. `import claude` drops the check. See [hooks Cursor and Copilot also read](@/docs/spec-format/hooks.md#claude-settings-copies).

`import claude` preserves these handlers. Experimental agent handlers are not emitted. `once` is written but inert, with a sync note: [Claude Code hooks](https://code.claude.com/docs/en/hooks) honor it only in skill frontmatter, and portable hooks land in `.claude/settings.json`. See [hook fields](@/docs/spec-format/hooks.md).

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
| `outputs.claude.settings` | | settings block for `.claude/settings.json` |

`import claude` and `agnostic-ai doctor` read the resolved paths, so a moved directory round-trips and its unmanaged files are still reported.

`dir` moves the whole tool directory: with `dir: vendor/.claude`, rules land in `vendor/.claude/rules/`, commands in `vendor/.claude/commands/`, and <code>{&#123;rules_dir}}</code> and other path variables resolve there. Per-kind keys override their own path. The next full sync sweeps old files as orphans. Claude Code auto-loads only a project-root `.claude/rules/`, so a moved rules directory needs `rules-mode: import`.

With `gitignore.enabled`, the managed `.gitignore` block also lists `/.claude/agent-memory-local/`, `/.claude/settings.local.json`, `/.claude/worktrees/`, and `/.claude/scheduled_tasks.lock`, following `outputs.claude.dir`. It omits `.claude/agent-memory/` (`memory: project`), which Claude Code documents as shareable. Ignoring does not untrack: run `git rm -r --cached .claude/agent-memory-local` if that store is committed.

The block also goes into `.worktreeinclude`, minus the worktree directories and task lock, so new Claude Code worktrees start with the ignored outputs ([gitignore](@/docs/configuration.md#gitignore)). `gitignore.ignore-worktree-include: true` ignores the managed `.worktreeinclude` too; `gitignore.worktree-include: false` leaves it to you.

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

Claude Code creates it on first use; agnostic-ai only emits the key. Session auto memory (`~/.claude/projects/<project>/memory/`) is separate and left alone, but `memory` needs it: with `autoMemoryEnabled` off or `CLAUDE_CODE_DISABLE_AUTO_MEMORY` set, `memory` does nothing. See [Memory and local state](@/docs/target-behavior.md#memory-and-local-state).

## Claude settings

`sync --global --only claude` maps a home settings spec's `permissions.default-mode` to `permissions.defaultMode` in `~/.claude/settings.json`, keeping hand-written permission rules ([global settings](@/docs/configuration.md#global-default-model-and-effort) covers modes, ownership, and conflicts).

`outputs.claude.settings` declares `.claude/settings.json` keys directly. Layers, lowest precedence first:

1. Captured overlay (from `import claude`).
2. Agnostic `settings` specs (`.agnostic-ai/settings/`), the cross-tool source for `permissions`, `model`, and `effort`. `effort` lands as `effortLevel` when it is `low`, `medium`, `high`, or `xhigh`.
3. This `outputs.claude.settings` block.
4. The spec-derived `hooks` block.
5. An `x-claude` block on a settings spec.

Unset keys fall through to the layer below. `x-claude` merges: `x-claude.permissions.deny` adds to the translated deny list, and only scalars such as `model` are replaced.

Without a captured overlay, sync merges permission lists into the on-disk `settings.json` and records the rules it added (portable, protected-path, `outputs.claude.settings`, and `x-claude`) in `.claude/.agnostic-ai-permissions.json`. The next sync removes those first, so a rule dropped from a spec, or moved from `allow` to `deny`, leaves the file. Hand-written and overlay rules stay. The first sync without a record keeps every rule and records the spec ones; a matching hand-written rule then counts as the spec's.

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
| `taskOutputMaxChars` | integer | Retired. Claude Code v2.1.277 removed it with the TaskOutput tool, and every release channel now runs that version or later. Sync no longer writes it, removes the copy an earlier sync wrote, and prints a note; delete it from `agnostic-ai.yaml`. A copy in `.agnostic-ai/overlays/claude.settings.json` is your own content: sync keeps it and names it in a note. |
| `attribution` | object | `commit` and `pr` set the attribution text for commits and pull requests; an empty string disables it. `sessionUrl` controls whether the session URL is included. |
| `includeCoAuthoredBy` | boolean | Deprecated Claude Code setting. Use `attribution`; when both are present, `attribution` takes precedence. |
| `enabledPlugins` | map of string to boolean | `plugin-id@marketplace-id` keys mapped to `true` to enable them, matching Claude Code's settings schema. |
| `env` | map of strings | Environment variables exported into Claude sessions. |
| `statusLine` | object | `type`, `command`, and optional `padding`, `refreshInterval` (seconds, minimum 1), and `hideVimModeIndicator`. |
| `permissions` | object | `allow`, `deny`, `ask` lists of tool-pattern strings. |

Other settings round-trip through the overlay `agnostic-ai import claude` captures (`.agnostic-ai/overlays/claude.settings.json`). For a scalar in both, this block wins. `permissions` `allow`/`deny`/`ask` lists union across the overlay, `settings` specs, and this block, so no layer drops another's rules.

## Import

`agnostic-ai import claude` reads the Claude Code tree:

| Source | Becomes |
|--------|---------|
| `.claude/rules/**/*.md` (preferred) | `<rules>/<sub>/<name>.md` (byte-identical copy, nested subdirectories preserved) |
| `CLAUDE.md` rules block written by `sync` | `<rules>/<name>.md` per rule (only when `.claude/rules/` is absent) |
| `CLAUDE.md` (any form) | `.agnostic-ai/AGNOSTIC_AI.md` (byte-identical copy) |
| `AGENTS.md` or `.claude/AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` (only when no `CLAUDE.md` exists) |
| `CLAUDE.md` that imports `@AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md`: the `AGENTS.md` text, then the rest of `CLAUDE.md` in a `::target claude` fence (no rules) |
| `<dir>/CLAUDE.md` (nested, hand-written) | one rule with the whole file and `scope: <dir>`, named after the scope: `api.md` for `services/api/`, or `services-api.md` when another scope also ends in `api` (only when `.claude/rules/` is absent) |
| `<dir>/CLAUDE.md` that imports `@AGENTS.md` | the same rule, holding the `AGENTS.md` text beside it, then the rest of `CLAUDE.md` in a `::target claude` fence. When `codex` imports in the same run, it owns that `AGENTS.md`, and only the fenced text lands, as `<scope>-claude.md` |
| `.claude/agents/*.md` | `<agents>/<name>.md` (byte-identical copy, except a Claude model name becomes `model: {claude: <name>}`) |
| `.claude/skills/<name>/SKILL.md` | `<skills>/<name>/SKILL.md` (`allowed-tools` moves under `x-claude:`) |
| `.claude/commands/*.md` | `<commands>/<name>.md` (`allowed-tools` moves under `x-claude:`) |
| `.claude/settings.json` hooks | `<hooks>/<event>[-<matcher>]-<command>.yaml`, such as `pretooluse-bash-exit-0.yaml`, with a `description` that says what runs and when (command hooks keep their grouping; HTTP, MCP-tool, and prompt handlers keep their native fields in separate specs; `target: claude` unless the hook can be shared, see below) |
| `.claude/settings.json` `permissions.allow`, `ask`, `deny` | `<settings>/permissions-claude.yaml` (a portable settings spec) |
| `.claude/settings.json` other non-hook keys | `.agnostic-ai/overlays/claude.settings.json` |
| `.mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml` (one spec per server) |

**Instructions.** A hand-written `CLAUDE.md` imports whole. Import does not also split it into rules, since each section would then load twice. When `.claude/rules/` exists (even empty), rule files come only from it. Import follows Claude Code's lookup order: `CLAUDE.md`, `.claude/CLAUDE.md`, `AGENTS.md`, `.claude/AGENTS.md`. Since v2.1.277 (v2.1.281 on Amazon Bedrock or with telemetry disabled), a session with no `CLAUDE.md` at or above the working directory loads `AGENTS.md`, so import captures the real instructions of repos set up for other agents. Only one importer may slice the root `AGENTS.md`, so import skips it when `codex`, `amp`, `warp`, `crush`, `kiro`, or `opencode` imports too. Text from `.claude/CLAUDE.md` syncs to the root `CLAUDE.md`; Claude Code would load both, so delete `.claude/CLAUDE.md` after the next sync, as import says.

**Nested `CLAUDE.md`.** Sync writes nested rules to `.claude/rules/<dir>/`, so import tells you to delete each nested `CLAUDE.md` it read, except a companion that only imports `@AGENTS.md`, which sync removes. `doctor --fix` deletes a nested `CLAUDE.md` still matching its rule, and keeps one you edited.

**Agents, skills, commands.** `allowed-tools`, read only by Claude Code, moves under `x-claude:`, where `lint` accepts it and sync writes it back. An agent `model` naming a Claude model (an alias such as `opus` or `fable`, `inherit`, or a `claude-*` id) imports as `model: {claude: <name>}`, so other targets use their default; sync writes `model: <name>` back.

**Hooks.** A hook's name comes from its script or its first command words. When two hooks would get the same name, a hash tells them apart. A spec an earlier release named `<event>-<matcher>-<hash8>` keeps that name.

**Settings.** Permission lists become a portable settings spec that `lint` checks and every target gets (with a coverage note where unsupported). Import skips rules that another settings spec or `outputs.claude.settings.permissions` declares, since sync wrote them into settings.json. Add `target: claude` to keep them Claude-only. The overlay captures every other non-`hooks` key, including `permissions.defaultMode`. Re-import after editing settings.json by hand. For precedence, see [Claude settings](#claude-settings).

`effortLevel` is the one key import moves out of the overlay. A valid value becomes `effort` in `<settings>/claude.yaml`, which every target syncs, unless another settings spec sets a different effort; then it goes under `x-claude.effortLevel` there. An invalid value stays in the overlay.

**MCP.** Imported MCP specs sync to every MCP-aware target: codex, copilot, cursor, continue, amp, zed, warp, gemini, opencode.

{% <details summary="Unread files and rewritten paths"> %}
The import summary lists each unread file under `.claude/`, such as `.claude/templates/post.md`. Sync leaves them in place. A skill that reads one still finds it, but other tools get no copy. The list skips files sync wrote, hidden files, `settings.local.json`, and Claude Code's `worktrees/` and `agent-memory/` folders.

Imported text naming an imported spec's Claude path, such as `.claude/skills/style/SKILL.md`, now names its source, `.agnostic-ai/skills/style/SKILL.md`, since other targets write it elsewhere. An `@.claude/rules/<name>.md` line for an imported rule is dropped; every target already gets the rule. Import reports each Claude path it leaves alone.
{% </details> %}

{% <details summary="Companion CLAUDE.md files"> %}
A `CLAUDE.md` whose instruction is `@AGENTS.md` is a companion letting Claude Code read `AGENTS.md`; its import line never reaches other tools. Once `.claude/rules/` holds a directory's scoped rules, `sync` deletes a nested companion that only imports its `AGENTS.md` (a `# CLAUDE.md` title aside), so Claude Code does not load them twice. Any other line, headings included, or a `sync.unmanaged` entry keeps the file, and sync reports a conflict with the scoped `AGENTS.md`. `sync --backup` keeps the deleted companion as `CLAUDE.md.bak`; `revert` restores it.
{% </details> %}

{% <details summary="Portable imported hooks"> %}
An imported hook is pinned with `target: claude` unless every other configured target can check it and run it as written. Only Codex can check today, and the hook must:

- use an event Codex has;
- match only tools or sources Codex reports (`Bash`, `apply_patch`, `Edit`, `Write`, `mcp__*`, or a `SessionStart` or compact source);
- use a command or `mcp_tool` handler;
- not set `if`, `shell`, `once`, or `asyncRewake`.

So in a `claude,codex` project a `PreToolUse` hook on `Bash` stays portable; the import summary explains each kept pin. With any target that cannot check, such as `cursor`, every pin stays.
{% </details> %}

{% <details summary="Rebuilding settings.json from the overlay"> %}
Import records the key `hooks` sat next to in `claude.settings.hook-events.json`. `sync -t claude` writes the spec-derived `hooks` back beside it, reproducing the full file after `.claude/` is wiped. An old `hooks: null` in a committed overlay is harmless; re-running `import claude` removes it.
{% </details> %}

## Protected paths

Enforced (permission). Each path in a settings spec's `protected` block becomes an `Edit(/<path>)` rule in `permissions.ask` or `permissions.deny` of `.claude/settings.json`, joined with the portable permission lists. The leading `/` anchors the rule at the project root.

Claude Code checks edits against `Edit` rules only; it accepts but ignores a `Write(<path>)` rule and warns at startup, so sync writes none ([permissions](https://code.claude.com/docs/en/permissions#read-and-edit)). `decision: deny` rules also cover file commands Claude Code recognizes in Bash, such as `sed` and `>` redirections, but not a script that opens files itself. That Bash coverage is documented for deny only; an `ask` rule guards edit tools.

Removing a path, or moving it from `deny` to `ask`, drops its old rule on the next sync ([Claude settings](#claude-settings)). See [Protected paths](@/docs/spec-format/settings.md#protected-paths).

## Verify

1. Install: `npm install -g @anthropic-ai/claude-code` (or the desktop app; both read the same files).
2. Check the tree: `ls CLAUDE.md .claude/agents/ .claude/skills/ .claude/rules/ .claude/commands/ .claude/settings.json .mcp.json`. Check provenance with `grep "Generated by agnostic-ai" .claude/agents/*.md .claude/rules/*.md .claude/commands/*.md` (the header follows the frontmatter, so `head -1` shows only `---`).
3. Validate JSON: `python -m json.tool .claude/settings.json > /dev/null && python -m json.tool .mcp.json > /dev/null`.
4. Run `claude` from the project root. `/agents`, `/skills`, and the slash-command picker list every entry. The MCP picker shows each `.mcp.json` server green.
5. Trigger a matcher action (for example an `Edit` for `PostToolUse`/`Edit`). The hook runs with no "schema mismatch" in the log.
6. Confirm `outputs.claude.settings.*` keys under `/config`.
