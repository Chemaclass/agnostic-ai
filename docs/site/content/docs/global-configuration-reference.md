+++
title = "User configuration reference"
description = "Shared names, default model and effort, user MCP servers, native discovery paths, and tools without user-level output."
weight = 136

[extra]
nav_title = "User configuration reference"
group = "Reference"
+++

# User configuration reference

Setup is in [User configuration](@/docs/global-configuration.md). This page holds the per-key and per-tool detail, and the [list of files `sync --scope user` writes](#global-output).

## Shared names {#global-shared-names}

A project skill or agent can share its `name` with one in the home. Sync writes both, and each tool decides which one it loads.

`sync` and `doctor` print one warning per shared name. It names each target where one copy hides the other, which copy wins, and the fix:

```text
! .agnostic-ai/skills/gh-issue/SKILL.md: skill "gh-issue" also exists in ~/.agnostic-ai/skills/gh-issue/SKILL.md with different content; claude loads the user-level one, which exists only on this machine, gemini loads this one; to load both, rename the user-level one (such as gh-issue-personal), or delete it to drop it, then run `agnostic-ai sync --scope user`; project sync cannot select this skill instead on claude
```

Why sync keeps both copies:

- Claude Code, Cline, and Amp document no project setting that picks the project copy over a home skill and keeps the other home skills.
- Amp's `amp.skills.disableGlobalAgentsSkills` turns off every skill in the user-level agents folders.
- So sync explains the limit and leaves your home files alone. Doctor reports it even when sync warnings are off.
- A winning user-level copy changes what you run, not what others run from the repository.
- When only project copies win, the warning says the user-level one is unused here. When both copies match, it says to delete one before they drift apart.

The check covers these targets:

| Target | Skill with the same name | Agent with the same name |
|--------|--------------------------|--------------------------|
| Amp | User-level wins when both sync `amp`: `~/.agents/skills/` masks `.agents/skills/` ([skills](https://ampcode.com/docs/customize/skills)) | No user-level agents |
| Claude Code | User-level wins ([skills](https://code.claude.com/docs/en/skills)) | Project wins ([subagents](https://code.claude.com/docs/en/sub-agents)) |
| Cline | User-level wins ([skills](https://docs.cline.bot/customization/skills.md)) when the project syncs `cline` and the home syncs `cline`, or any target that writes `~/.agents/skills/`, which Cline also reads ([config](https://docs.cline.bot/getting-started/config.md)) | Not checked |
| Codex | Both can appear in skill selectors, so no warning ([skills](https://learn.chatgpt.com/docs/build-skills)) | Not checked |
| Gemini CLI | Project wins ([skills](https://geminicli.com/docs/cli/skills/)) | Not checked |

Sync and doctor do not check other tools, though some document what happens:

- [Kiro](https://kiro.dev/docs/skills/#skill-scope) prefers workspace skills over user-level skills.
- [Qoder IDE](https://docs.qoder.com/extensions/skills) documents project skills taking priority over user skills. [Qoder CLI](https://docs.qoder.com/cli/Skills.md) documents user skills taking priority over project skills.
- [Warp](https://docs.warp.dev/agents/capabilities/skills#skill-name-conflicts) prefers home skills, then higher directories, when it resolves names in the background. Its slash-command menu shows all matches, and a plain-language request gives the agent every match in scope with its path.

<details>
<summary>Which pairs the check compares</summary>

- Targets that both the project and the home's [`targets`](@/docs/global-configuration.md#source-and-targets) write. A spec whose `target`, `targets`, or `targets-exclude` leaves one of them out is skipped.
- Claude Code skill names match ignoring case, spacing, invisible characters, and fullwidth forms. Other targets compare names exactly.
- A spec shared through a link is not a clash. Neither is a project that is the home.

</details>

To resolve a clash:

- **Layer on purpose**: give the project spec its own name, such as a `gh-issue-project` skill beside a user `gh-issue`. Both then load everywhere.
- **Keep one name in both places**: list it in [`sync.allow-global-names`](@/docs/sync-settings.md#syncallow-global-names).
- **Silence every shared name**: set [`sync.global-name-clash: ignore`](@/docs/sync-settings.md#syncglobal-name-clash).

## Default model and effort {#global-default-model-and-effort}

A settings spec in the home sets each tool's default model and effort in its user settings file. `model` and `effort` each take one string for every target, or a per-target map with an optional `default`:

```yaml
# ~/.agnostic-ai/settings/defaults.yaml
model:
  claude: opus
  codex: gpt-6-luna
effort:
  claude: high
  codex: high
```

| Target | File | Keys |
|---|---|---|
| codex | `~/.codex/config.toml` | `model`, `model_reasoning_effort` |
| claude | `~/.claude/settings.json` | `model`, `effortLevel`, `permissions.defaultMode`, `permissions.allow`, `ask`, `deny` |
| codebuddy | `~/.codebuddy/settings.json` | `model`, `reasoningEffort`, `permissions.defaultMode`, `permissions.allow`, `ask`, `deny` |
| copilot | `~/.copilot/settings.json` | `model`, `effortLevel` |
| qoder | `~/.qoder/settings.json` | `model.name`, `model.reasoningEffort` |
| qwen | `~/.qwen/settings.json` | `model.name`, `model.reasoningEffort` |
| vibe | `~/.vibe/config.toml` | `active_model` |
| kimi | `~/.kimi-code/config.toml` | `default_model`, `default_plan_mode`, `telemetry` |
| gemini | `~/.gemini/settings.json` | `model.name` |
| augment | `~/.augment/settings.json` | `defaultModel`, `defaultThinkingLevel` (Auggie 1.x) |

- Targets not in the table raise a coverage note for `model` and `effort`. So does `effort` on Gemini and Kimi Code.
- Augment also takes `x-augment` keys, such as `shell`.
- A dotted key is a nested JSON object, except for Kiro CLI's literal dotted keys. For `model.name`, sync sets `name` inside `model` and leaves the other keys alone.

<details>
<summary>Details: accepted effort values</summary>

| Target | Accepted `effort` values |
|---|---|
| Claude | `low`, `medium`, `high`, `xhigh` |
| Copilot | `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max` (each model accepts a subset) |
| Qoder | `disabled`, `off`, `none`, `low`, `medium`, `high`, `xhigh`, `max` |
| Augment | `off`, `minimal`, `low`, `medium`, `high`, `xhigh` |
| Qwen | `low`, `medium`, `high`, `xhigh`, `max` (each provider maps the value to its active model) |
| Codex | Any string |
| CodeBuddy Code | See [native fields and limits](@/docs/targets/codebuddy.md#fields-and-limits) |

`lint --scope user` (LINT014) and `validate --scope user` flag a value a target cannot take.

Claude Code and `effortLevel`:

- `~/.claude/settings.json` `effortLevel` is read on Opus 5, Fable 5.1, and earlier models only.
- Opus 5.5 and later models ignore it. They need a level saved per model with `/effort`.
- `sync --scope user` writes the key and prints a note.
- In project settings, `effortLevel` applies to every model.

</details>

### Permissions

Set Claude Code's starting permission mode in a home settings spec:

```yaml
# ~/.agnostic-ai/settings/defaults.yaml
permissions:
  default-mode: acceptEdits
```

- `sync --scope user --only claude` writes `permissions.defaultMode` in `~/.claude/settings.json`, or under `CLAUDE_CONFIG_DIR`.
- Accepted values: `default`, `manual`, `acceptEdits`, `plan`, `auto`, `dontAsk`, `bypassPermissions`.
- A later settings spec wins.
- Removing the field removes the managed mode, unless you changed it by hand.
- Claude's `auto` and `bypassPermissions` need user, managed, or session settings ([Claude's mode reference](https://code.claude.com/docs/en/settings-reference#permissions-defaultmode)).

Home `permissions.allow`, `ask`, and `deny` rules, and the same lists under `x-claude.permissions`, merge into `~/.claude/settings.json` rule by rule.

- Claude Code saves standing approvals to the project's `.claude/settings.local.json`, which user-level import does not read ([Claude's settings scopes](https://code.claude.com/docs/en/settings#find-or-create-your-settings-files)).
- CodeBuddy Code also merges user permission lists and starting modes ([native limits](@/docs/targets/codebuddy.md#fields-and-limits)).
- A `permissions` field on any other target raises a coverage note. For Codex, set `x-codex.approval_policy` and `x-codex.sandbox_mode` instead.

<details>
<summary>Details: how permission rules merge</summary>

- Sync appends each missing rule after the rules already there, and adopts a rule already there.
- Sync records added and adopted rules in `state/global.json`.
- Dropping a rule from the specs removes it, including a rule you wrote before adoption. Other rules keep their order.
- `--check` fails while a spec rule is missing or a dropped one remains.
- `--plan` and `--dry-run` list each rule added or removed.
- `import --scope user claude` copies user-level rules into `settings/permissions-claude.yaml`, so they reach your other machines.

</details>

### Other settings

- `session-retention-days` sets how many days transcripts are kept, for Claude Code, CodeBuddy Code, Gemini CLI, and Qoder CLI ([session retention](@/docs/spec-format/settings-reference.md#session-retention)).
- `voice` configures voice dictation for tools with a native setting ([voice dictation](@/docs/spec-format/settings-reference.md#voice-dictation)).

### Which value wins

Each item wins over the one before it:

1. `settings/*.yaml` in the home.
2. `local/settings/*.yaml`. A same-named file merges into the shared one field by field. A new file comes after the shared ones, so its `model` and `effort` win.
3. The project tier: a project's settings file, such as `.codex/config.toml` or `.claude/settings.json`, wins by the tool's own settings order.
4. An agent's own `model` and `effort`, for that agent.
5. The tool's flag for one run, such as `codex -m` or `claude --model`.

`agnostic-ai explain --scope user settings/defaults.yaml` names the file and key each target gets.

<details>
<summary>Details: how sync edits settings keys</summary>

- Sync edits only the keys it writes.
- Removing the spec removes only those keys and the objects sync created to hold them. An object you wrote stays, even an empty one such as `"env": {}`.
- Sync deletes a file it created once the file is empty.
- A key that already holds the value sync would write is adopted.
- A key with another value stops the run and names the file, key, and both values. This is normal for Codex, whose `/model` picker saves to `config.toml`. `--backup` overwrites the key and keeps `<path>.bak`.
- `--dry-run` lists each key a write sets or removes. `--check` fails on a changed key.
- An `x-<target>` block sets that target's own keys in the same file. Nested objects merge leaf by leaf.
- An `x-claude` key wins over the portable field it shares a key with.
- A later spec wins key by key. `null` drops an earlier key.
- User-level sync writes Codex top-level scalars and arrays, such as `x-codex.notify`, to `~/.codex/config.toml`.
- Tables such as `profiles`, `x-claude.hooks`, and `x-claude.permissions` keys other than `allow`, `ask`, and `deny` raise a coverage note.
- Project sync does not route `x-codex` into `.codex/config.toml`.

</details>

## Approvals {#global-approvals}

`approvals` sets what happens when a tool wants permission for a tool call:

| Mode | What happens |
|---|---|
| `ask` | The tool shows its prompt. The default. |
| `review` | The tool's own reviewer model decides routine requests. The rest still ask. |
| `yolo` | Each request is approved once, unless a fixed always-ask rule matches. |

```yaml
# $AGNOSTIC_AI_HOME/agnostic-ai.yaml or $AGNOSTIC_AI_HOME/local/agnostic-ai.yaml
approvals:
  mode: ask            # ask | review | yolo
  tools:               # per tool, replaces mode
    codex: review
  projects:            # per project folder, replaces mode and tools
    ~/Code/sandbox: yolo
  always-ask:          # extra patterns, added to the fixed list
    - "Bash(make deploy*)"
  companion: false     # Approve and Deny in the companion, Claude Code and Codex
```

- `review` and `yolo` are read only from the home config. A cloned repository, a pull request, or the agent itself can write a project's files, so a project's `agnostic-ai.yaml` or `agnostic-ai.local.yaml` may set `approvals.mode: ask` and nothing else. Any other `approvals` key there is ignored, and `validate` and `lint` report it as [LINT064](@/docs/cli-reference/lint-codes.md).
- An `approvals` block in `local/agnostic-ai.yaml` replaces the shared block whole, except `always-ask`, whose patterns add up, and `companion`, which keeps the shared value unless the local block sets it.
- A change made from the companion lands in one file. **Ask me** writes `mode: ask` to `local/agnostic-ai.yaml`, as `agnostic-ai approvals ask` does. Any other change goes to the file whose `approvals` block is in effect: `local/agnostic-ai.yaml` when it has one, else `agnostic-ai.yaml` when it has one, else a new block in `local/agnostic-ai.yaml`. Comments and other keys stay. When the change needs the hook and `approvals` is not in `builtins`, it adds it to `local/agnostic-ai.yaml`.
- A `projects` key is an absolute path or one under `~`, the home folder. The deepest folder that holds the session wins; links are resolved, and on macOS and Windows case is ignored. Two keys for one folder take the stricter mode.
- The strictest source wins, in this order: `AGNOSTIC_AI_APPROVALS=ask` in the session's environment, a project file that says `ask`, `projects`, `tools`, `mode`. Any value of the environment variable means `ask`: it can only tighten.
- A config without `approvals` behaves as `ask`. `$AGNOSTIC_AI_HOME` must be an absolute path for approvals to read it.
- `companion: true` shows **Approve** and **Deny** in the companion for a Claude Code or Codex request left to you. Home config only. See [Answer from the companion](@/docs/approvals.md#answer-from-the-companion).
  - Claude Code: the hook waits up to 300 seconds for a click while the terminal prompt stays open. Its hook timeout is 310 seconds.
  - Codex: the hook waits up to 60 seconds, since Codex shows its prompt only after the hook ends. The row adds **Answer in terminal** to show the prompt at once.
- Only Claude Code, Codex, Kiro, Qwen Code and Qoder carry out `yolo`, through a hook that `builtins: [approvals]` and `sync --scope user` install ([Approvals](@/docs/approvals.md)). Only the tools in [the review table](#global-approvals-review) carry out `review`. Every other tool falls back to `ask`, and [`approvals status`](@/docs/cli-reference/approvals.md#approvals-status) says so.

### Review through the tool's own reviewer {#global-approvals-review}

When `approvals.tools.<tool>`, or `approvals.mode` without it, says `review`, `sync --scope user` turns on the tool's own reviewer in its user settings:

| Tool | Reviewer | Key `sync --scope user` writes |
|---|---|---|
| Claude Code | [Auto mode](https://code.claude.com/docs/en/permission-modes) | `permissions.defaultMode: "auto"` in `~/.claude/settings.json` |
| CodeBuddy | [Auto mode](https://www.codebuddy.ai/docs/cli/permission-modes), an AI reviewer; not run by us | `permissions.defaultMode: "auto"` in `~/.codebuddy/settings.json` |
| Codex | [Auto-review](https://learn.chatgpt.com/docs/agent-approvals-security) | `approvals_reviewer = "auto_review"` in `~/.codex/config.toml` |
| Cursor (`cursor-agent` only) | [Auto-review](https://cursor.com/docs/agent/security/run-modes), a server classifier | `approvalMode: "auto-review"` in `~/.cursor/cli-config.json` |
| Devin CLI (`windsurf`) | [Smart mode](https://docs.devin.ai/cli/reference/permissions), a fast model; not run by us | `permission_mode: "smart"` in `~/.config/devin/config.json` (`%APPDATA%\devin\config.json` on Windows) |
| Qoder | [Auto mode](https://docs.qoder.com/cli/permissions), a two-stage classifier; not run by us | `general.defaultPermissionMode: "auto"` in `~/.qoder/settings.json` |
| Vibe | [Smart-approve](https://docs.mistral.ai/vibe/code/cli/configuration-reference), a classifier; not run by us | `default_agent = "smart-approve"` in `~/.vibe/config.toml` |
| Kiro | [Approve for me](https://kiro.dev/docs/permissions/#approve-for-me), Kiro's trust classifier; not run by us | `approve-for-me` under `policies:` in `~/.kiro/settings/permissions.yaml` |
| Qwen Code | [Auto mode](https://github.com/QwenLM/qwen-code/blob/main/docs/users/features/approval-mode.md), an LLM classifier; not run by us | `tools.approvalMode: "auto"` in `~/.qwen/settings.json` |

- A home settings spec that sets the same key wins: `permissions.default-mode` where the tool maps it, otherwise the key under `x-<tool>`, such as `x-codex.approvals_reviewer`. A value set outside agnostic-ai stays as it is, and `sync --scope user` warns that the reviewer stays off.
- Remove `review` and run `sync --scope user` again, and the key goes. [`approvals ask`](@/docs/cli-reference/approvals.md#approvals-ask) alone does not remove it.
- The key sits in a user file, so it applies in every folder. A `projects` entry or `AGNOSTIC_AI_APPROVALS=ask` cannot turn the reviewer off for one folder or session; `approvals status` notes it there. A project sync never writes these keys.
- Codex Auto-review answers only prompts Codex would show. With `approval_policy = "never"` there are none, so the reviewer never runs; `approvals status` says so when `~/.codex/config.toml` sets it. It does not read a trusted project's `.codex/config.toml`, which can set either key too.
- Qwen Code already runs auto mode in a trusted folder when `tools.approvalMode` is unset, so `ask` alone does not bring its prompts back; `approvals status` says so. Set `x-qwen.tools.approvalMode: default` in a home settings spec to see them. Qwen Code asks anyway in an untrusted folder and with `--bare` or `--safe-mode`, and a project's `.qwen/settings.json` can set the same key. Checked against the Qwen Code 0.28.0 package.
- CodeBuddy takes `defaultMode: "auto"` only from user settings and the command line; a project file cannot grant it. It falls back to `default` when `permissions.disableAutoMode` is `"disable"` or auto mode is unavailable, and `approvals status` notes the first. `sync --scope user` writes `~/.codebuddy/settings.json` even when `CODEBUDDY_CONFIG_DIR` moves CodeBuddy's settings elsewhere; status notes that too. Checked against the CodeBuddy 2.185.0 package and its permission-modes page.
- cursor-agent reads `cli-config.json` from `CURSOR_CONFIG_DIR`, else `$XDG_CONFIG_HOME/cursor`, else `~/.cursor`, and `sync --scope user` follows the same order. cursor-agent rewrites that file itself, so sync owns only `approvalMode` there. cursor-agent writes `approvalMode: "allowlist"` as its own default, so review replaces that value and gives the key back when review goes; any other value set by hand stays. `x-cursor.approvalMode` in a home settings spec wins and must be `allowlist`, `unrestricted` or `auto-review`, since cursor-agent sets a file with any other value aside and starts over. `CURSOR_CONFIG_DIR` and `XDG_CONFIG_HOME` must be absolute paths. Auto-review needs a classifier-capable model and team permission, or cursor-agent falls back to its allowlist. The Cursor desktop app keeps its own setting, and `approvals status` says to turn on Auto-review there. Checked against cursor-agent 2026.11.11, which kept the key when it rewrote the file.
- Devin CLI reads `permission_mode` only from the user config, so a project cannot set it. `--permission-mode` replaces it for one launch. Devin's docs also show `DEVIN_PERMISSION_MODE=smart devin`, and `approvals status` notes a set variable; that the variable wins over the file is our reading, not stated. A `null` value means normal, so review replaces it. Your organization's policy can remove Smart mode, and sandbox sessions do not offer it; the docs do not say what Devin does with the key then. `approvals status` reports `dangerous` as approving every call. Sync owns only `permission_mode` in that file; `x-windsurf.permission_mode` in a home settings spec wins and must be `normal`, `accept-edits`, `smart` or `dangerous`. Your own `deny` and `ask` rules still come first. From the Devin CLI configuration and permissions pages; Devin CLI is not installed here.
- Qoder reads `general.defaultPermissionMode` from every settings layer, so a project's `.qoder/settings.json` can set it too, and an untrusted folder starts in `default`, as does a session with no classifier model available. An older `tools.approvalMode` key counts as the user's mode, so review leaves it. Checked against the Qoder CLI 1.1.93 package.
- Vibe takes a `default_agent` that a person sets as an opt-in, so smart-approve starts even before the mode reaches Vibe's mode picker. An organization can still exclude it, and a trusted project's `.vibe/config.toml`, `VIBE_DEFAULT_AGENT`, an admin config, or `--agent` replaces the user file's value. In the user file, `x-vibe` cannot set `default_agent`, so only a hand-set value wins; Vibe can also write `default_agent = "ask"` there itself when `disabled_agents` lists `accept-edits`. With `bypass_tool_permissions = true`, Vibe approves every call and the classifier never runs; `approvals status` notes it and a set `VIBE_DEFAULT_AGENT`. Checked against the mistral-vibe 3.1.3 package.
- Kiro turns its reviewer on through a preset in a list, not a key. `sync --scope user` adds one `approve-for-me` line under `policies:` in `~/.kiro/settings/permissions.yaml` (or `$KIRO_HOME/settings/permissions.yaml`), editing the text in place so your rules, comments and other presets stay. Remove `review` and it takes out only that line, plus the `policies:` key and file when it created them. A preset you listed yourself stays. A file it cannot edit line by line, such as one where `policies` is not a list, gets a warning and no change. Kiro's own `/permissions` page lists the preset in a workspace file instead, which `sync --scope user` does not touch. Kiro asks anyway when its trust classifier is unavailable or the session does not offer Approve for me.
- A running session keeps its mode until you switch it in the tool (Shift+Tab in Claude Code). A new session reads the new settings.
- agnostic-ai logs no decision the reviewer makes. Each tool keeps its own record, and each review costs the tool's own model calls.

Run [`approvals status`](@/docs/cli-reference/approvals.md#approvals-status) to see the mode and route per tool, and [`approvals ask`](@/docs/cli-reference/approvals.md#approvals-ask) to show every prompt again.

## Conversations {#global-conversations}

Set how long a Claude Code background shell can run before it appears in the [Conversations list and companion](@/docs/cli-reference/companion-list.md#stop-a-forgotten-shell).

```yaml
# $AGNOSTIC_AI_HOME/agnostic-ai.yaml or $AGNOSTIC_AI_HOME/local/agnostic-ai.yaml
conversations:
  stuckShellAfter: 45m
```

The default is `30m`. Use a nonnegative duration such as `1h30m`, or `0` to turn off the warnings and stop action. Only the home config controls this setting. The local home setting takes precedence over the shared home setting. Restart `agnostic-ai serve` or the companion to use the new value.

## MCP servers {#global-mcp-servers}

MCP specs in the home's `mcps/` install servers in each supported tool's user MCP file, using the same format as project servers:

| Target | File | Where |
|---|---|---|
| claude | `~/.claude.json` (`$CLAUDE_CONFIG_DIR/.claude.json` when set) | top-level `mcpServers.<name>` |
| codex | `~/.codex/config.toml` | `[mcp_servers.<name>]` table |
| codebuddy | `~/.codebuddy/.mcp.json` | `mcpServers.<name>`; [legacy precedence and native limits](@/docs/targets/codebuddy-reference.md#enable-project-mcp-output) |
| vibe | `~/.vibe/config.toml` | `[[mcp_servers]]`, keyed by `name` |
| kimi | `~/.kimi-code/mcp.json` | `mcpServers.<name>`, retaining disabled servers |
| cursor | `~/.cursor/mcp.json` | `mcpServers.<name>` |
| copilot | `~/.copilot/mcp-config.json` | `mcpServers.<name>`, with `tools: ["*"]` when the spec sets none |
| gemini | `~/.gemini/settings.json` | `mcpServers.<name>` |
| qoder | `~/.qoder/settings.json` | `mcpServers.<name>` |
| warp | `~/.warp/.mcp.json` (Warp app only; [the Agent CLI reads its own file](@/docs/targets/warp.md#app-and-cli)) | `mcpServers.<name>`: `{command, args, env, working_directory}` or `{url, headers}` |
| augment | `~/.augment/settings.json` | `mcpServers.<name>` |
| antigravity | `~/.gemini/config/mcp_config.json` | `mcpServers.<name>`: `{command, args, env, cwd}` or `{serverUrl, headers}`, with optional `disabled` |

The **Tools** page in `serve --scope user` lists servers that have no home spec and were not written by sync. Each row has the `import --scope user <target>` command that creates its spec ([serve](@/docs/cli-reference/serve.md#serve)).

OpenHands has no user MCP file sync can write. Sync prints a note that points at Agent Canvas **Customize > MCP Servers**. It also removes the servers an earlier sync recorded as its own in `~/.openhands/mcp.json` ([OpenHands MCP](@/docs/targets/openhands.md#mcp)).

<details>
<summary>Details: how sync edits MCP servers</summary>

- A hand-written server that means the same as the spec is adopted.
- A different server with the same name stops the run. `--backup` overwrites it.
- A server sync wrote is removed when its spec goes. Servers you add under other names stay.
- In `~/.claude.json`, sync edits only its own `mcpServers` entries and creates a missing file at `0600`.
- If `~/.claude.json` changed after sync read it, sync stops without writing. Run it again.
- A spec with `disabled: true` stays out of the Augment, Claude, Cursor, Copilot, and Warp user files, where a listed server is live in every project.
- Antigravity keeps disabled servers in its user file with `disabled: true`.
- When `CLAUDE_CONFIG_DIR` or another root variable moves a file, the next sync removes its entries from the old one.

</details>

## Native discovery and shared paths

### Discovery limits

- Copilot's path is for its CLI.
- OpenHands discovery applies to local conversations.
- Devin custom profiles are experimental.
- Trae needs **Settings > Beta > Subagents > Enable Subagents Directory**.

### Configuration roots

- `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `COPILOT_HOME`, `CLINE_DIR`, `QODER_CONFIG_DIR`, `KIRO_HOME`, `JUNIE_HOME`, and `KIMI_CODE_HOME` replace the matching `~/.<tool>/` folder.
- `GEMINI_CLI_HOME` is the parent of `.gemini/`.
- Everything under that folder moves together. Paths outside it, such as `~/.agents/skills/`, stay put.
- Use absolute paths.

### Shared paths

Shared paths are written once:

| Path | Tools |
|---|---|
| `~/.agents/skills/` | codex, windsurf, amp, zed, warp, goose, openhands |
| `~/.agents/agents/` | goose, openhands |
| `~/.agents/AGENTS.md` | cline, warp |
| `~/.gemini/GEMINI.md` | gemini, antigravity |

If two tools would write different content to one path, the run stops before writing and names the path.

### Per-tool notes

**Copilot agent effort**

- Copilot CLI documents a native profile `reasoningEffort` field, available through `x-copilot.reasoningEffort` ([agent fields](@/docs/targets/copilot-reference.md#agents)).
- Portable agent effort is not mapped to that CLI field.
- A portable agent `effort` of `low`, `medium`, `high`, or `xhigh` goes to `subagents.agents.<name>.effortLevel` in `~/.copilot/settings.json`. Other values raise a coverage note.
- Sync stops when the file has comments, since a rewrite drops them. Rerun with `--backup` to rewrite it and keep `settings.json.bak`.
- Sync also stops when a hand-set `effortLevel` has a different value.

**Cursor**

- Cursor does not load the home-level `AGENTS.md`.
- User-level sync installs a `sessionStart` hook and a script under `~/.cursor/hooks/` that pass the instructions to Cursor. The script is POSIX shell on macOS and Linux, PowerShell on Windows.
- It needs neither agnostic-ai, Python, nor jq, and it adds context without enforcing policy.
- Existing `sessionStart` entries stay.
- Settings: user-level sync writes only `approvalMode` to cursor-agent's `cli-config.json`, for [`approvals`](#global-approvals-review). Other `x-cursor` keys give a note.

**OpenCode**

- `disable-model-invocation` stays in project and user-level skills. V2 honors it; V1 does not ([OpenCode skills](@/docs/targets/opencode.md) covers native metadata precedence).
- V1 reads `~/.claude/CLAUDE.md` only when `~/.config/opencode/AGENTS.md` does not exist.
- The current [V2 migration guide](https://opencode.ai/v2/docs/migrate-v1#instruction-files) documents a `CLAUDE.md` fallback in each directory without `AGENTS.md`, plus `~/.claude/CLAUDE.md` when the user OpenCode `AGENTS.md` is absent.
- V2 chooses per directory, so an `AGENTS.md` at the project root does not hide a nested `CLAUDE.md`.
- In V2 2.3.1 and later, either `OPENCODE_DISABLE_CLAUDE_CODE_PROMPT` or `OPENCODE_DISABLE_CLAUDE_CODE` disables every `CLAUDE.md` fallback, both project and user ([2.3.1 release notes](https://github.com/anomalyco/opencode/releases/tag/v2.3.1)).
- Put OpenCode guidance in its `AGENTS.md` when syncing both tools.

**Devin CLI and VS Code Copilot**

- Both read `~/.claude/` by default. Syncing claude plus windsurf or copilot delivers the same instructions through two paths.

## Targets left out

No user-level instructions file:

- Aider reaches a home instructions file only through a `read:` entry in `~/.aider.conf.yml`.
- Continue's one documented home setting is the `rules:` list in the `config.yaml` it rewrites itself.
- Jules documents nothing at user level.
- Ellipsis reads only files in the repository.

Hooks reach these tools at user level: Claude Code, CodeBuddy Code, Codex, Gemini, Qoder, Cursor, Augment, Kiro CLI V3, Copilot, Cline SDK/CLI, Grok Build, Kimi Code CLI, Qwen Code, Mistral Vibe, and Goose. The others have no user-level hook output:

- Factory, Devin CLI, Antigravity, and Crush use hook formats that user-level sync does not write.
- Junie's hooks are Early Access.
- Amp, OpenCode, and Kilo take hooks only as TypeScript plugins. Sync writes them at project level only, for OpenCode (`.opencode/plugins/`) and Kilo (`.kilo/plugin/`).
- OpenCode defaults to V1. A hook selects [V2](@/docs/targets/opencode.md#opencode-v2-hooks) with `x-opencode.plugin-api: v2` on macOS and Linux.
- On Windows, V2 refuses matching callbacks, so keep the default V1 hooks.

## User-level output {#global-output}

A dash means user-level sync writes nothing for that kind.

| Target | Instructions | Rules | Hooks | Skills | Agents | Session retention |
|--------|--------------|-------|-------|--------|--------|-------------------|
| **claude** | `~/.claude/CLAUDE.md` | inlined | `~/.claude/settings.json` | `~/.claude/skills/<name>/` | `~/.claude/agents/<name>.md` | [days](@/docs/spec-format/settings.md#session-retention) |
| **codebuddy** | `~/.codebuddy/CODEBUDDY.md` | inlined | `~/.codebuddy/settings.json` | `~/.codebuddy/skills/<name>/` | `~/.codebuddy/agents/<name>.md` | [days](@/docs/targets/codebuddy.md#fields-and-limits) |
| **cursor** | `~/.cursor/AGENTS.md` (bridged) | inlined | `~/.cursor/hooks.json` | `~/.cursor/skills/<name>/` | `~/.cursor/agents/<name>.md` | - |
| **codex** | `~/.codex/AGENTS.md` | inlined | `~/.codex/hooks.json` | `~/.agents/skills/<name>/` | `~/.codex/agents/<name>.toml` | - |
| **gemini** | `~/.gemini/GEMINI.md` | inlined | `~/.gemini/settings.json` | `~/.gemini/skills/<name>/` | `~/.gemini/agents/<name>.md` | [days](@/docs/spec-format/settings.md#session-retention) |
| **kimi** | `~/.kimi-code/AGENTS.md` | inlined | `~/.kimi-code/config.toml` | `~/.kimi-code/skills/<name>/` | `~/.kimi-code/agents/<name>.md` | - |
| **vibe** | `~/.vibe/AGENTS.md` | inlined | `~/.vibe/hooks.toml` | `~/.vibe/skills/<name>/` | `~/.vibe/agents/<name>.toml` | - |
| **qoder** | `~/.qoder/AGENTS.md` | inlined | `~/.qoder/settings.json` | `~/.qoder/skills/<name>/` | `~/.qoder/agents/<name>.md` | [days](@/docs/spec-format/settings.md#session-retention) |
| **copilot** | `~/.copilot/copilot-instructions.md` | inlined | `~/.copilot/hooks/agnostic-ai.json` | `~/.copilot/skills/<name>/` | `~/.copilot/agents/<name>.agent.md` | - |
| **cline** | `~/.agents/AGENTS.md` | inlined | `~/.cline/hooks/<event>` | `~/.cline/skills/<name>/` | `~/.cline/agents/<name>.yml` | - |
| **windsurf** | `~/.config/devin/AGENTS.md` | inlined | - | `~/.agents/skills/<name>/` | `~/.config/devin/agents/<name>.md` | - |
| **amp** | `~/.config/amp/AGENTS.md` | inlined | - | `~/.agents/skills/<name>/` | - | - |
| **zed** | `~/.config/zed/AGENTS.md`; Windows: `%APPDATA%\Zed\AGENTS.md` | inlined | - | `~/.agents/skills/<name>/` | - | - |
| **warp** | `~/.agents/AGENTS.md` | inlined | - | `~/.agents/skills/<name>/` | - | - |
| **opencode** | `~/.config/opencode/AGENTS.md` | inlined | - | `~/.config/opencode/skills/<name>/` | `~/.config/opencode/agents/<name>.md` | - |
| **antigravity** | `~/.gemini/GEMINI.md` | inlined | - | `~/.gemini/config/skills/<name>/` | `~/.gemini/config/agents/<name>/agent.md` | - |
| **junie** | `~/.junie/AGENTS.md` | inlined | - | `~/.junie/skills/<name>/` | `~/.junie/agents/<name>.md` | - |
| **kiro** | `~/.kiro/steering/AGENTS.md` | inlined | `~/.kiro/hooks/<name>.json` | `~/.kiro/skills/<name>/` | `~/.kiro/agents/<name>.md` | - |
| **crush** | `~/.config/crush/CRUSH.md` | inlined | - | `~/.config/crush/skills/<name>/` | - | - |
| **factory** | `~/.factory/AGENTS.md` | inlined | - | `~/.factory/skills/<name>/` | `~/.factory/droids/<name>.md` | - |
| **kilo** | `~/.config/kilo/AGENTS.md` | inlined | - | `~/.kilo/skills/<name>/` | `~/.config/kilo/agents/<name>.md` | - |
| **goose** | `~/.config/goose/.goosehints` | inlined | `~/.agents/plugins/agnostic-ai-global/hooks/hooks.json` | `~/.agents/skills/<name>/` | `~/.agents/agents/<name>.md` | - |
| **openhands** | - | - | - | `~/.agents/skills/<name>/` | `~/.agents/agents/<name>.md` | - |
| **trae** | - | - | - | `~/.trae/skills/<name>/` | `~/.trae-cn/agents/<name>.md` | - |
| **grok** | `~/.grok/AGENTS.md` | inlined | `~/.grok/hooks/<name>.json` | `~/.grok/skills/<name>/` | `~/.grok/agents/<name>.md` | - |
| **qwen** | `~/.qwen/QWEN.md` | inlined | `~/.qwen/settings.json` | `~/.qwen/skills/<name>/` | `~/.qwen/agents/<name>.md` | - |
| **augment** | - | `~/.augment/rules/<name>.md` | `~/.augment/settings.json` | `~/.augment/skills/<name>/` | `~/.augment/agents/<name>.md` | - |

- [MCP servers](@/docs/global-configuration-reference.md#global-mcp-servers) and [settings](@/docs/global-configuration-reference.md#global-default-model-and-effort) have their own tables in the reference. Agents use the same formats as project agents ([the agent example](@/docs/global-configuration.md#global-agents)).
- **Rules** are inlined into the instructions file, in the same `## Rules` block as the shared body.
- Augment has no user-level instructions file for the CLI, so its `~/.augment/rules/` entries are always on.
- Existing instruction files keep their personal text and file permissions.
- Sync stops if a file changes while it is preparing the update.

Path notes:

- Paths marked `~/.config/` follow `XDG_CONFIG_HOME` when set, except Zed on macOS.
- Zed on Linux and FreeBSD prefers `FLATPAK_XDG_CONFIG_HOME` ([native instruction roots](@/docs/targets/zed-reference.md#global-instructions)).
- Devin agents use `~/.config/devin/agents/` on Linux and macOS and `%APPDATA%\devin\agents\` on Windows.
