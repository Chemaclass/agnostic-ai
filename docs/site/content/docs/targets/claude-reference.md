+++
title = "Claude Code native reference"
description = "Native field mappings, output overrides, and import coverage for claude."
weight = 11

[extra]
nav_title = "Claude Code reference"
group = "Reference"
+++

# Claude Code native reference

Start with the [Claude Code setup](@/docs/targets/claude.md). This page covers native mappings and overrides. For shared home configuration and ownership, see [global configuration](@/docs/global-configuration.md).

## Rules and instruction loading

Claude Code reads `.claude/rules/**/*.md` natively. Sync translates `globs` or `paths` into `paths:` frontmatter and removes portable activation keys. A comma-separated string produces one pattern per entry.

A directory scope and file patterns add together: `scope: src/a` with `globs: tests/a/**` covers both directories. Omit `scope` for a file-only filter, and keep the source outside a folder that implies scope.

`CLAUDE.md` carries the shared instructions. When another target writes an identical root `AGENTS.md`, Claude's file imports it with `@AGENTS.md`, plus Claude-only text. Sync keeps a full copy if a rules appendix or target-specific text would make the files differ. If the existing Claude file already imports `AGENTS.md`, sync can keep that layout without another target.

A session started in a subfolder treats the root `@AGENTS.md` as an outside import. Interactive sessions ask once; non-interactive `claude -p` skips it. Use [explain](@/docs/cli-reference/inspect.md#explain-a-source-file) to inspect planned files, then confirm native loading in that working directory.

Legacy `rules-mode: import` is for versions without native rules loading. `rules-file: CLAUDE.md` joins rule bodies there; another `rules-file` path gets an import from `CLAUDE.md`.

## Agents, skills, and commands

| Source field | Claude Code output |
|---|---|
| Skill `x-claude` | Native skill frontmatter; overrides portable fields |
| Skill `model`, `effort` | Single value or per-target map |
| Command body | `.claude/commands/<name>.md`, invoked as `/<name>` |
| Agent frontmatter | Passed through, including resolved `model` and `effort` |
| Agent `readonly: true` | `disallowedTools: Write, Edit, NotebookEdit`; Bash stays allowed |

A declared `disallowedTools` or `x-claude.disallowedTools` wins over `readonly`. A null override omits it. Claude Code skips names starting with `-` or containing `:`; sync rejects them before writing, including names from `x-claude`.

For global agents and user settings, see [global configuration](@/docs/global-configuration.md).

## Hook mappings

Hooks merge into `.claude/settings.json`. See the [native hook reference](https://code.claude.com/docs/en/hooks) for event names and payloads, and [portable hooks](@/docs/spec-format/hook-reference.md#portable-events) for shared events.

| Handler | Fields |
|---|---|
| Command | `command`; `args` selects exec form; command lists become separate handlers |
| HTTP | `url`, optional `headers`, `allowedEnvVars` |
| MCP tool | `server`, `tool`, optional `input` |
| Prompt | `prompt`, optional `model`, `continueOnBlock` |

Handlers keep `timeout`, `statusMessage`, `if`, and `once`. `args`, `async`, `asyncRewake`, and `shell` are command-only. `once` has no effect in settings hooks; Claude Code honors it only in skill frontmatter. Experimental agent handlers are not emitted.

`failClosed: true` becomes `onFailure: "block"` for command and HTTP handlers. Use synchronous `PreToolUse` or `UserPromptSubmit` when a failed check must stop an action. Claude ignores it for `Stop`, `SubagentStop`, `TaskCompleted`, `TeammateIdle`, or async command handlers. Import retains other native `onFailure` values under `x-claude`; portable `failClosed: true` wins.

With command hooks, sync sets `AGNOSTIC_AI_TARGET: claude` in settings `env`; an explicit settings value wins. When Cursor is also selected, a portable hook's Claude copy exits under Cursor to avoid running twice. Other compatibility reads need [cross-target checks](@/docs/target-behavior.md#spec-kind-behavior).

## MCP mappings

`.mcp.json` is rebuilt from MCP specs on every sync. Import hand-written servers before syncing.

| Transport | Native fields |
|---|---|
| Stdio | `command`, `args`, `env`; no `type` |
| Remote | `type`, `url`, `headers`; also `headersHelper` and `oauth` for HTTP, SSE, and WS |
| Either | `timeout` (milliseconds per tool call; values below 1000 ignored), `roots` |

`alwaysLoad` and `bareElicitationCapability` preserve explicit true and false; `x-claude` overrides them. They are Claude-only fields. See the [MCP reference](https://code.claude.com/docs/en/mcp) for their native behavior.

Remote OAuth keeps `clientId`, `callbackPort`, `authServerMetadataUrl`, and space-separated `scopes`. Sync never writes `oauth.clientSecret`; Claude Code keeps it in the system keychain.

`disabled: true` writes `disabledMcpjsonServers` in project settings. It requires the project `.mcp.json` path; a custom path fails. Sync records only its generated rejections in `.claude/.agnostic-ai-mcp-disabled.json` and removes the entry when re-enabled.

## Environment mappings

`dev-commands` become `.claude/launch.json` preview servers: command splits into `runtimeExecutable` and `runtimeArgs`, `auto-port` becomes `autoPort`, and `cwd` stays project-relative. `x-claude.autoVerify` sets `autoVerify`. Import normalizes `${workspaceFolder}` paths; a URL-only configuration stays native with a note.

`setup` becomes `.claude/hooks/agnostic-ai-worktree-setup.sh`. Session, subagent, and EnterWorktree hooks run it once per new linked worktree, using Bash. Windows needs Git Bash. `x-claude.setup: false` omits them. Other environment fields have no Claude output and get a note. See [worktree setup](@/docs/spec-format/environments.md#claude-code-worktree-setup).

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

`import claude` and `agnostic-ai doctor` read the resolved paths, so a moved directory still imports and syncs back unchanged.

`dir` moves the whole tool directory: with `dir: vendor/.claude`, rules land in `vendor/.claude/rules/`, commands in `vendor/.claude/commands/`, and <code>{&#123;rules_dir}}</code> and other path variables resolve there. Per-kind keys override their own path. The next full sync removes old files as orphans. Claude Code auto-loads only a project-root `.claude/rules/`, so a moved rules directory needs `rules-mode: import`.

With `gitignore.enabled`, the managed `.gitignore` block also lists `/.claude/agent-memory-local/`, `/.claude/settings.local.json`, `/.claude/worktrees/`, and `/.claude/scheduled_tasks.lock`, following `outputs.claude.dir`. It omits `.claude/agent-memory/` (`memory: project`), which is shareable. Ignoring does not untrack: run `git rm -r --cached .claude/agent-memory-local` if that store is committed.

The block also goes into `.worktreeinclude`, so new worktrees start with the ignored outputs ([gitignore](@/docs/configuration.md#gitignore)). `gitignore.ignore-worktree-include: true` ignores the managed `.worktreeinclude` too; `gitignore.worktree-include: false` leaves it to you.

## Agent memory

A top-level `memory` key gives a subagent a directory that persists across sessions. Claude Code acts on it; support in other tools differs. See [agent memory fields](@/docs/spec-format/agents.md).

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

Claude Code creates the directory on first use. Subagent memory is part of auto memory: with `autoMemoryEnabled` off or `CLAUDE_CODE_DISABLE_AUTO_MEMORY` set, the `memory` field has no effect. With auto memory on, Claude Code turns on Read, Write, and Edit for that agent so it can manage its memory files. Session auto memory (`~/.claude/projects/<project>/memory/`) is separate. Sync leaves it alone unless the [`memory` built-in](@/docs/memory.md#claude-code-s-own-memory) points it at the shared personal store. That needs auto memory on: with `autoMemoryEnabled` off or `CLAUDE_CODE_DISABLE_AUTO_MEMORY` set, Claude saves nothing there. See [shared memory](@/docs/memory.md).

## Claude settings

`sync --global --only claude` maps a home settings spec's `permissions.default-mode` to `permissions.defaultMode` in `~/.claude/settings.json`. It merges the home's `allow`, `ask`, and `deny` rules into the lists there. `import --global claude` copies user-level rules into the home. See [global settings and rule ownership](@/docs/configuration.md#global-default-model-and-effort).

`outputs.claude.settings` declares `.claude/settings.json` keys directly. Layers, lowest first (a later layer wins):

1. Captured overlay (from `import claude`).
2. Agnostic `settings` specs (`.agnostic-ai/settings/`), the cross-tool source for `permissions`, `model`, and `effort`. `effort` lands as `effortLevel` when it is `low`, `medium`, `high`, or `xhigh`.
3. This `outputs.claude.settings` block.
4. The `hooks` block built from your hook specs.
5. An `x-claude` block on a settings spec.

Unset keys fall through to the layer below. `x-claude` merges: `x-claude.permissions.deny` adds to the translated deny list, and only single values such as `model` are replaced.

Without a captured overlay, sync merges permission lists into the on-disk `settings.json` and records the rules it added in `.claude/.agnostic-ai-permissions.json`. The next sync removes those first, so a rule dropped from a spec, or moved from `allow` to `deny`, leaves the file. Hand-written and overlay rules stay. The first sync without a record keeps every rule and records the spec ones.

```yaml
outputs:
  claude:
    settings:
      attribution:
        commit: ""
        pr: ""
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

Other settings survive import and sync through the overlay `agnostic-ai import claude` captures (`.agnostic-ai/overlays/claude.settings.json`). For a single value in both, this block wins. `permissions` `allow`/`deny`/`ask` lists combine across the overlay, `settings` specs, and this block.

Claude Code's `/skills` menu saves per-skill toggles as `skillOverrides` in `.claude/settings.local.json`. `agnostic-ai doctor` reports synced skills that this file, the shared `.claude/settings.json`, or your user `~/.claude/settings.json` turns off, naming each file. `doctor --fix` offers to re-enable them in `.claude/settings.local.json` only. See [Claude skill overrides](@/docs/cli-reference/check.md#claude-skill-overrides). `agnostic-ai explain <skill>` shows the entries that name one skill, file by file; see [explain](@/docs/cli-reference/inspect.md#claude-skill-overrides). To set visibility for the whole team, put `skillOverrides` in `.agnostic-ai/overlays/claude.settings.json`; sync writes it to `.claude/settings.json`.

Claude Code ignores some keys in a project `.claude/settings.json`, so a cloned repository cannot change them. Sync still writes them from `x-claude` and prints a note. `sync --global` also writes them, and notes the managed-only keys, which Claude Code ignores in `~/.claude/settings.json` too. Where a key shows a value, only that value is ignored. Scopes come from the [settings reference](https://code.claude.com/docs/en/settings-reference).

| Where Claude Code reads it | Keys |
|---|---|
| User settings (a home settings spec and `sync --global`) or managed settings | `appendPlugins`, `askUserQuestionTimeout`, `autoContinueAtUsageLimit`, `autoMode`, `axScreenReader`, `bashEditDiffEnabled`, `desktopSessionCleanupPeriodDays`, `dialogExpiry`, `feedbackDrafts`, `footerLinksRegexes`, `forceLoginGatewayUrl`, `modelPicker`, `pluginConfigs`, `prependPlugins`, `processWrapper`, `sandbox.allowAppleEvents`, `sandbox.allowNestedUserNamespaces: true`, `sandbox.credentials.allowPlaintextInject`, `sandbox.credentials.awsPairs`, `sandbox.credentials.sigv4`, `sandbox.filesystem.disabled`, `sandbox.network.strictAllowlist`, `sandbox.network.tlsTerminate`, `sandbox.ripgrep`, `skipAutoPermissionPrompt`, `skipDangerousModePermissionPrompt: true`, `spellcheck`, `sshConfigs`, `sshHostCommands`, `syncClaudeAiPlugins: false`, `syncClaudeAiSkills: false`, `useAutoModeDuringPlan: false`, `vimInsertModeRemaps` |
| User settings only (a home settings spec and `sync --global`) | `worktree.location` |
| Managed settings only | `allowAllClaudeAiMcps`, `allowClaudeInChromeWithManagedMcp`, `allowedChannelPlugins`, `allowedProviders`, `allowManagedHooksOnly`, `allowManagedMcpServersOnly`, `allowManagedPermissionRulesOnly`, `availableModelsMatch`, `blockedMarketplaces`, `browserExternalPageTools`, `channelsEnabled`, `claudeMd`, `deniedModels`, `disableBrowserExternalNavigation`, `disableCommandPluginSources`, `disableDesktopLocalSessions`, `disableMobileSimulatorTools`, `disableSideloadFlags`, `forceRemoteSettingsRefresh`, `gatewayInternalNetworks`, `managedMcpServers`, `managedSourcesBehavior`, `modelPricing`, `parentSettingsBehavior`, `pluginSuggestionMarketplaces`, `pluginTrustMessage`, `policyHelper`, `requiredMaximumVersion`, `requiredMinimumVersion`, `sandbox.bwrapPath`, `sandbox.filesystem.allowManagedReadPathsOnly`, `sandbox.network.allowManagedDomainsOnly`, `sandbox.socatPath`, `sshHostAllowlist`, `strictKnownMarketplaces`, `strictPluginOnlyCustomization`, `wslInheritsWindowsSettings` |

## Import

`agnostic-ai import claude` reads the Claude Code files:

| Source | Becomes |
|--------|---------|
| `.claude/rules/**/*.md` (preferred) | `<rules>/<sub>/<name>.md` (unchanged copy) |
| `CLAUDE.md` rules block written by `sync` | `<rules>/<name>.md` per rule (only when `.claude/rules/` is absent) |
| `CLAUDE.md` (any form) | `.agnostic-ai/AGNOSTIC_AI.md` (unchanged copy) |
| `AGENTS.md` or `.claude/AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` (only when no `CLAUDE.md` exists) |
| `CLAUDE.md` that imports `@AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md`: the `AGENTS.md` text, then the rest of `CLAUDE.md` in a `::target claude` fence (no rules) |
| `<dir>/CLAUDE.md` (nested, hand-written) | one rule with the whole file and `scope: <dir>`, named after the scope: `api.md` for `services/api/`, or `services-api.md` when another scope also ends in `api` (only when `.claude/rules/` is absent) |
| `<dir>/CLAUDE.md` that imports `@AGENTS.md` | the same rule, with the `AGENTS.md` text, then the rest of `CLAUDE.md` in a `::target claude` fence. When `codex` imports in the same run, only the fenced text lands, as `<scope>-claude.md` |
| `.claude/agents/*.md` | `<agents>/<name>.md` (unchanged copy, except a Claude model name becomes `model: {claude: <name>}`) |
| `.claude/skills/<name>/SKILL.md` | `<skills>/<name>/SKILL.md` (`allowed-tools` moves under `x-claude:`) |
| `.claude/commands/*.md` | `<commands>/<name>.md` (`allowed-tools` moves under `x-claude:`) |
| `.claude/settings.json` hooks | `<hooks>/<event>[-<matcher>]-<command>.yaml`, such as `pretooluse-bash-exit-0.yaml`, with a `description` (HTTP, MCP-tool, and prompt handlers keep their native fields in separate specs; `target: claude` unless the hook can be shared, see below) |
| `.claude/settings.json` `permissions.allow`, `ask`, `deny` | `<settings>/permissions-claude.yaml` (a portable settings spec) |
| `.claude/settings.json` other non-hook keys | `.agnostic-ai/overlays/claude.settings.json` |
| `.mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml` (one spec per server) |

**Instructions.** A hand-written `CLAUDE.md` imports whole, not split into rules, since each section would then load twice. When `.claude/rules/` exists (even empty), rule files come only from it. Import follows Claude Code's lookup order: `CLAUDE.md`, `.claude/CLAUDE.md`, `AGENTS.md`, `.claude/AGENTS.md`. Only one importer may slice the root `AGENTS.md`, so import skips it when `codex`, `amp`, `warp`, `crush`, `kiro`, or `opencode` imports too. Delete `.claude/CLAUDE.md` after the next sync, as import says, or Claude Code loads both it and the root file.

**Nested `CLAUDE.md`.** Sync writes nested rules to `.claude/rules/<dir>/`, so import tells you to delete each nested `CLAUDE.md` it read. `doctor --fix` deletes one still matching its rule and keeps one you edited. Once `.claude/rules/` holds a directory's scoped rules, `sync` itself deletes a nested `CLAUDE.md` that only imports `@AGENTS.md`. Any other line, or a `sync.unmanaged` entry, keeps the file. `sync --backup` keeps the deleted file as `CLAUDE.md.bak`; `revert` restores it.

**Agents, skills, commands.** `allowed-tools` moves under `x-claude:`, where `lint` accepts it and sync writes it back. An agent `model` naming a Claude model (an alias such as `opus` or `fable`, `inherit`, or a `claude-*` id) imports as `model: {claude: <name>}`, so other targets use their default; sync writes `model: <name>` back.

**Settings.** Permission lists become a portable settings spec that `lint` checks and every target gets (with a sync note where unsupported). Import skips rules that another settings spec or `outputs.claude.settings.permissions` declares. Add `target: claude` to keep them Claude-only. The overlay captures every other non-`hooks` key, including `permissions.defaultMode`. Re-import after editing settings.json by hand.

`effortLevel` is the one key import moves out of the overlay. A valid value becomes `effort` in `<settings>/claude.yaml`, which every target syncs, unless another settings spec sets a different effort; then it goes under `x-claude.effortLevel`. An invalid value stays in the overlay.

<details>
<summary>Unread files and rewritten paths</summary>

The import summary lists each unread file under `.claude/`, such as `.claude/templates/post.md`. Sync leaves them in place. A skill that reads one still finds it, but other tools get no copy.

Imported text naming a Claude path, such as `.claude/skills/style/SKILL.md`, now names its source, `.agnostic-ai/skills/style/SKILL.md`. An `@.claude/rules/<name>.md` line for an imported rule is dropped. Import reports each Claude path it leaves alone.

</details>

<details>
<summary>Portable imported hooks</summary>

An imported hook is pinned with `target: claude` unless every other configured target can check it and run it as written. Only Codex can check today. The hook must use an event Codex has, match only tools Codex reports (`Bash`, `apply_patch`, `Edit`, `Write`, `mcp__*`) or a `SessionStart` or compact source, use a command or `mcp_tool` handler, and not set `if`, `shell`, `once`, or `asyncRewake`. So in a `claude,codex` project a `PreToolUse` hook on `Bash` stays portable. With a tool that cannot check, such as `cursor`, every pin stays. The import summary explains each kept pin.

</details>


For enforcement boundaries, see [protected paths in Claude Code](@/docs/targets/claude.md#protected-paths).
