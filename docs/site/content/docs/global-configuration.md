+++
title = "Global configuration"
description = "Install and manage your own cross-project instructions, skills, settings, and MCP servers."
weight = 135

[extra]
group = "Reference"
+++

# Global configuration

Keep personal instructions, rules, hooks, skills, agents, settings, and MCP servers in one home folder, and run `agnostic-ai sync --global` to install them into each tool's user-level files, so they apply in every project. Use it for what you want everywhere; put team defaults in a project or a pack.

## Source and targets

The source folder is `$AGNOSTIC_AI_HOME`, or `~/.agnostic-ai/` when unset. `sync --global` runs from any directory and loads no packs or project specs. [Global output](#global-output) lists the paths it writes.

```text
~/.agnostic-ai/
├── agnostic-ai.yaml        # optional: targets, builtins, requires, lint, on-unsupported, models
├── AGNOSTIC_AI.md
├── agents/*.md
├── rules/*.md
├── hooks/*.yaml
├── settings/*.yaml         # default model and effort
├── mcps/*.yaml             # user-level MCP servers
├── skills/<name>/SKILL.md
└── local/                  # optional personal layer, same layout
```

`local/` works as in a project: a spec there merges into the shared spec of the same kind and name, and a new name adds one. See [local overrides](@/docs/local-overrides.md#override-fields). `local/AGNOSTIC_AI.md` comes last in the managed instructions block. Add `/local/` to the source root's `.gitignore` before adding personal files.

To sync a fixed set of tools without `--only` on every run, list them in the source root's `agnostic-ai.yaml`:

```yaml
targets: [claude, codex, cursor]
```

- `sync --global`, `lint --global`, and `validate --global` then use only those targets. A `targets` list in `local/agnostic-ai.yaml` replaces the shared one.
- `--only` and `--except` narrow the list for one run and must name configured targets. `--target` replaces it and skips the home config's `targets`.
- A target with no user-level files, such as `aider` or `continue`, is skipped with one warning. An unknown name stops the run.
- [`builtins`](@/docs/configuration.md#built-ins), [`requires`](@/docs/configuration.md#requires), `lint`, and [`models`](@/docs/configuration.md#models) work as in a project. For each key, `local/agnostic-ai.yaml` replaces the shared value, and a `models` tier replaces the same-name tier whole.
- When a file's tiers do not load, `sync --global` stops. With `-t`, it warns, skips that file's tiers, and omits the `model` of any spec that names one.
- [`on-unsupported`](@/docs/configuration.md#on-unsupported) (`warn`, `error`, or `silent`) sets what `sync --global` does with a skill line that Claude Code expands and other targets read as plain text. See [Claude Code body syntax](@/docs/spec-format/skills.md#claude-code-body-syntax).
- Other keys except `version` print a warning and are ignored.
- A target dropped from the list keeps its synced files and ownership records until you remove them by hand.

`list --global` shows the specs in effect with their `builtin`, `global`, or `global-local` layer. `validate --global` and `lint --global` check before a sync writes. `migrate --global` rewrites old spec forms. The [CLI reference](@/docs/cli-reference/sync.md#sync) lists the accepted `sync` flags.

- Nested rules, rules with scope, path, glob, or target conditions, commands, settings `permissions` rule lists for targets other than Claude Code and CodeBuddy Code, inheritance, and merging with project specs are unsupported.
- Kilo inline shell placeholders pass the syntax check at documented trusted user destinations. See [Kilo shell commands](@/docs/targets/kilo.md#shell-commands-in-global-skills) for approval, scope, and the kill switch.
- Skills copy their supporting files unchanged. Shared directories such as `~/.agents/skills/` keep neutral frontmatter. See [skill invocation support](@/docs/spec-format/skills.md#disable-model-invocation-support-by-target) for `disable-model-invocation`, including OpenCode V2.
- Hooks, skills, and settings honor `target`, `targets`, and `targets-exclude`. A settings spec routed away from a tool sets none of its values there and does not override an earlier spec. Supported portable `on` events and `match` values translate for each tool. Native `event` and `matcher` values pass through unchanged; set those per target. See [portable hooks](@/docs/spec-format/hook-reference.md#portable-events).
- Some user hook routes have their own limits. Copilot: `import --global copilot` adopts matching handlers from neighboring user JSON files; see [Copilot hooks](@/docs/targets/copilot.md#hooks). Cline: the VS Code extension does not read the hooks sync writes; see [Cline global hooks](@/docs/targets/cline.md#global-hooks). Kiro CLI V3: one `~/.kiro/hooks/<name>.json` per spec, in the project hook format, with shared scripts in `~/.kiro/scripts/`; unrelated hook files stay, and a conflicting file stops sync.
- Targets without global agent output warn and skip agents.
- Output is real files, never symlinks. A user file that is itself a symlink, such as a dotfiles-managed `CLAUDE.md`, is written through. A symlink inside a skills, agents, or rules directory stops the run.
- Sync records what it wrote in `$AGNOSTIC_AI_HOME/state/global.json` and removes only those files and settings for the targets in the run. With `--only`, other targets' output stays in place.
- A hand edit to a file sync owns, or to an instructions file's managed block, stops the run and names the file. Move the edit into the source, or rerun with `--backup` to overwrite or remove it and keep `<path>.bak`. Text outside the managed block never counts.
- A file or hook that already matches what sync would write is adopted. The run stops on one that differs by even one byte, an unmanaged agent, skill, or rule collision, a damaged marker, invalid native JSON, corrupt state, or state recorded under another `HOME`.
- Without `--only`, explicit targets, or a home `targets` list, a target with a relative root variable or an invalid agent name is skipped with a warning. Naming it makes that an error.
- Sync Goose and OpenHands together to update their shared agent file.

Global specs never merge with project specs. When both exist, each tool decides which settings or same-named specs win; see [shared names](#global-shared-names). Ordinary `agnostic-ai sync` reads only the names of `~/.agnostic-ai/` specs, for that warning.

Inside the global source root, `sync`, `init`, `import`, `new`, `packs`, `cleanup`, `revert`, and `install-hook` stop before any write and point you to the `--global` form. Read-only commands such as `lint`, `validate`, and `doctor` still run.

For a home kept in Git, `install-hook --global` writes a pre-commit hook that runs `lint --global --strict`, `validate --global`, and `sync --global --check`. To start a home from what your tools already hold, run `agnostic-ai import --global`. See [import](@/docs/cli-reference/start.md#import).

### Shared names {#global-shared-names}

A project skill or agent can share its `name` with one in the home. Sync writes both, and each tool decides which one it loads. `sync` and `doctor` print one warning per shared name, with each target where one copy hides the other, which copy wins, and the fix:

```text
! .agnostic-ai/skills/gh-issue/SKILL.md: skill "gh-issue" also exists in ~/.agnostic-ai/skills/gh-issue/SKILL.md with different content; claude loads the global one, which exists only on this machine, gemini loads this one; to load both, rename the global one (such as gh-issue-personal), or delete it to drop it, then run `agnostic-ai sync --global`; project sync cannot select this skill instead on claude
```

Claude Code and Amp document no project setting that picks the project copy over a home skill and keeps the other home skills; Amp's `amp.skills.disableGlobalAgentsSkills` turns off every skill in the global agents folders. So sync keeps both copies, explains the limit, and leaves your home files alone. Doctor reports it even when sync warnings are off. A winning global copy changes what you run, not what others run from the repository. When only project copies win, the warning says the global one is unused here; when both copies match, it says to delete one before they drift apart.

The check covers these targets:

| Target | Skill with the same name | Agent with the same name |
|--------|--------------------------|--------------------------|
| Amp | Global wins when both sync `amp`: `~/.agents/skills/` masks `.agents/skills/` ([skills](https://ampcode.com/docs/customize/skills)) | No global agents |
| Claude Code | Global wins ([skills](https://code.claude.com/docs/en/skills)) | Project wins ([subagents](https://code.claude.com/docs/en/sub-agents)) |
| Codex | Both can appear in skill selectors, so no warning ([skills](https://learn.chatgpt.com/docs/build-skills)) | Not checked |
| Gemini CLI | Project wins ([skills](https://geminicli.com/docs/cli/skills/)) | Not checked |

Sync and doctor do not check other tools, though some document what happens:

- [Kiro](https://kiro.dev/docs/skills/#skill-scope) prefers workspace skills over global skills.
- [Qoder](https://docs.qoder.com/extensions/skills) prefers project skills over user skills.
- [Warp](https://docs.warp.dev/agents/capabilities/skills#skill-name-conflicts) prefers home skills, then higher directories, when it resolves names in the background. Its slash-command menu shows all matches, and a plain-language request gives the agent every match in scope with its path.

<details>
<summary>Which pairs the check compares</summary>

- Targets that both the project and the home's [`targets`](#source-and-targets) write. A spec whose `target`, `targets`, or `targets-exclude` leaves one of them out is skipped.
- Claude Code skill names match ignoring case, spacing, invisible characters, and fullwidth forms. Other targets compare names exactly.
- A spec shared through a link is not a clash. Neither is a project that is the home.

</details>

To layer on purpose, give the project spec its own name, such as a `gh-issue-project` skill beside a global `gh-issue`. Both then load everywhere, under different names.

To keep one name in both places, list it in [`sync.allow-global-names`](@/docs/configuration.md#syncallow-global-names). To stop the warning for every shared name, set [`sync.global-name-clash: ignore`](@/docs/configuration.md#syncglobal-name-clash).

### Default model and effort {#global-default-model-and-effort}

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
| vibe | `~/.vibe/config.toml` | `active_model` |
| kimi | `~/.kimi-code/config.toml` | `default_model`, `default_plan_mode`, `telemetry` |
| gemini | `~/.gemini/settings.json` | `model.name` |

Targets not in the table raise a coverage note for `model` and `effort`, and so does `effort` on Gemini and Kimi Code. Augment has no default model or effort key but takes `x-augment` keys, such as `shell`. A dotted key is a nested JSON object: for `model.name`, sync sets `name` inside `model` and leaves the other keys alone.

Accepted `effort` values: Claude and Copilot take `low`, `medium`, `high`, `xhigh`; Qoder takes `disabled`, `off`, `none`, `low`, `medium`, `high`, `xhigh`, `max`; Codex takes any string; for CodeBuddy Code see [native fields and limits](@/docs/targets/codebuddy.md#fields-and-limits). `lint --global` (LINT014) and `validate --global` flag a value a target cannot take.

Set Claude Code's starting permission mode in a home settings spec:

```yaml
# ~/.agnostic-ai/settings/defaults.yaml
permissions:
  default-mode: acceptEdits
```

`sync --global --only claude` writes `permissions.defaultMode` in `~/.claude/settings.json`, or under `CLAUDE_CONFIG_DIR`. It accepts `default`, `manual`, `acceptEdits`, `plan`, `auto`, `dontAsk`, and `bypassPermissions`. A later settings spec wins. Removing the field removes the managed mode, unless you changed it by hand. Claude's `auto` and `bypassPermissions` need user, managed, or session settings; see [Claude's mode reference](https://code.claude.com/docs/en/settings-reference#permissions-defaultmode).

Home `permissions.allow`, `ask`, and `deny` rules, and the same lists under `x-claude.permissions`, merge into `~/.claude/settings.json` rule by rule (v0.86.0 or later):

- Sync appends each missing rule after the rules already there, and adopts a rule already there.
- Sync records added and adopted rules in `state/global.json`. Dropping a rule from the specs removes it, including a rule you wrote before adoption. Other rules keep their order.
- `--check` fails while a spec rule is missing or a dropped one remains. `--plan` and `--dry-run` list each rule added or removed.
- Run `import --global claude` to copy user-level rules into `settings/permissions-claude.yaml`, so they reach your other machines.

Claude Code saves standing permission approvals to the project's `.claude/settings.local.json`, which global import does not read. See [Claude's settings scopes](https://code.claude.com/docs/en/settings#find-or-create-your-settings-files).

CodeBuddy Code also merges user permission lists and starting modes; see its [native limits](@/docs/targets/codebuddy.md#fields-and-limits). Any `permissions` field on another target raises a coverage note. For Codex, set `x-codex.approval_policy` and `x-codex.sandbox_mode` instead.

`session-retention-days` in a home settings spec sets how many days your transcripts are kept, for Claude Code, CodeBuddy Code, Gemini CLI, and Qoder CLI. See [session retention](@/docs/spec-format/settings.md#session-retention).

Each item wins over the one before it:

1. `settings/*.yaml` in the home.
2. `local/settings/*.yaml`. A same-named file merges into the shared one field by field. A new file comes after the shared ones, so its `model` and `effort` win.
3. The project tier: a project's settings file, such as `.codex/config.toml` or `.claude/settings.json`, wins according to the tool's own settings order.
4. An agent's own `model` and `effort`, for that agent.
5. The tool's flag for one run, such as `codex -m` or `claude --model`.

Sync edits only the keys it writes. Removing the spec removes only those keys and the objects sync created to hold them, so an object you wrote, even an empty one such as `"env": {}`, stays. Sync deletes a file it created once the file is empty.

- A key that already holds the value sync would write is adopted. A key with another value stops the run and names the file, key, and both values; this is normal for Codex, whose `/model` picker saves to `config.toml`. `--backup` overwrites the key and keeps `<path>.bak`.
- `--dry-run` lists each key a write sets or removes, and `--check` fails on a changed key.
- An `x-<target>` block sets that target's own keys in the same file. Nested objects merge leaf by leaf. An `x-claude` key wins over the portable field it shares a key with. A later spec wins key by key, and `null` drops an earlier key.
- Global sync writes Codex top-level scalars and arrays, such as `x-codex.notify`, to `~/.codex/config.toml`. Tables such as `profiles`, `x-claude.hooks`, and `x-claude.permissions` keys other than `allow`, `ask`, and `deny` raise a coverage note. Project sync does not route `x-codex` into `.codex/config.toml`.
- `agnostic-ai explain --global settings/defaults.yaml` names the file and key each target gets.

### Agents {#global-agents}

A personal agent such as `~/.agnostic-ai/agents/reviewer.md` with `targets: [claude, codex]` installs with:

```console
agnostic-ai sync --global --only claude,codex
agnostic-ai sync --global --only claude,codex --check
```

Sync writes `~/.claude/agents/reviewer.md` and `~/.codex/agents/reviewer.toml`. If a hand-copied file sits at an output path, copy its edits into the source and move the file aside first; `--backup` does not override a file sync does not own.

### MCP servers {#global-mcp-servers}

MCP specs in the home's `mcps/` install each server in the user MCP file of every target that has one, rendered as the target's project MCP file renders it:

| Target | File | Where |
|---|---|---|
| claude | `~/.claude.json` (`$CLAUDE_CONFIG_DIR/.claude.json` when set) | top-level `mcpServers.<name>` |
| codex | `~/.codex/config.toml` | `[mcp_servers.<name>]` table |
| codebuddy | `~/.codebuddy/.mcp.json` | `mcpServers.<name>`; [legacy precedence and native limits](@/docs/targets/codebuddy.md#enable-project-mcp-output) |
| vibe | `~/.vibe/config.toml` | `[[mcp_servers]]`, keyed by `name` |
| kimi | `~/.kimi-code/mcp.json` | `mcpServers.<name>`, retaining disabled servers |
| cursor | `~/.cursor/mcp.json` | `mcpServers.<name>` |
| copilot | `~/.copilot/mcp-config.json` | `mcpServers.<name>`, with `tools: ["*"]` when the spec sets none |
| gemini | `~/.gemini/settings.json` | `mcpServers.<name>` |
| qoder | `~/.qoder/settings.json` | `mcpServers.<name>` |
| warp | `~/.warp/.mcp.json` (Warp app only; [the Agent CLI reads its own file](@/docs/targets/warp.md#app-and-cli)) | `mcpServers.<name>`: `{command, args, env, working_directory}` or `{url, headers}` |
| augment | `~/.augment/settings.json` | `mcpServers.<name>` |
| antigravity | `~/.gemini/config/mcp_config.json` | `mcpServers.<name>`: `{command, args, env, cwd}` or `{serverUrl, headers}`, with optional `disabled` |
| openhands | `~/.openhands/mcp.json` (`$OPENHANDS_PERSISTENCE_DIR/mcp.json` when set) | `mcpServers.<name>`: `{command, args, env}` or `{url, transport, headers, auth}` |

Each server follows the per-key rules above:

- A hand-written server that means the same as the spec is adopted. A different one with the same name stops the run, and `--backup` overwrites it.
- A server sync wrote is removed when its spec goes. Servers you add under other names stay.
- In `~/.claude.json`, sync edits only its own `mcpServers` entries and creates a missing file at `0600`. If the file changed after sync read it, sync stops without writing; run it again.
- A spec with `disabled: true` stays out of the Augment, Claude, Cursor, Copilot, OpenHands, and Warp user files, where a listed server is live in every project. Antigravity keeps disabled servers with `disabled: true` in its user file.
- When `CLAUDE_CONFIG_DIR` or another root variable moves a file, the next sync removes its entries from the old one.

## Global output

A dash means global sync writes nothing for that kind.

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
| **zed** | `~/.config/zed/AGENTS.md` | inlined | - | `~/.agents/skills/<name>/` | - | - |
| **warp** | `~/.agents/AGENTS.md` | inlined | - | `~/.agents/skills/<name>/` | - | - |
| **opencode** | `~/.config/opencode/AGENTS.md` | inlined | - | `~/.config/opencode/skills/<name>/` | `~/.config/opencode/agents/<name>.md` | - |
| **antigravity** | `~/.gemini/GEMINI.md` | inlined | - | `~/.gemini/config/skills/<name>/` | `~/.gemini/config/agents/<name>/agent.md` | - |
| **junie** | `~/.junie/AGENTS.md` | inlined | - | `~/.junie/skills/<name>/` | `~/.junie/agents/<name>.md` | - |
| **kiro** | `~/.kiro/steering/AGENTS.md` | inlined | `~/.kiro/hooks/<name>.json` | `~/.kiro/skills/<name>/` | `~/.kiro/agents/<name>.md` | - |
| **crush** | `~/.config/crush/CRUSH.md` | inlined | - | `~/.config/crush/skills/<name>/` | - | - |
| **factory** | `~/.factory/AGENTS.md` | inlined | - | `~/.factory/skills/<name>/` | `~/.factory/droids/<name>.md` | - |
| **kilo** | `~/.config/kilo/AGENTS.md` | inlined | - | `~/.kilo/skills/<name>/` | `~/.config/kilo/agents/<name>.md` | - |
| **goose** | `~/.config/goose/.goosehints` | inlined | - | `~/.agents/skills/<name>/` | `~/.agents/agents/<name>.md` | - |
| **openhands** | - | - | - | `~/.agents/skills/<name>/` | `~/.agents/agents/<name>.md` | - |
| **trae** | - | - | - | `~/.trae/skills/<name>/` | `~/.trae-cn/agents/<name>.md` | - |
| **grok** | `~/.grok/AGENTS.md` | inlined | `~/.grok/hooks/<name>.json` | `~/.grok/skills/<name>/` | `~/.grok/agents/<name>.md` | - |
| **qwen** | `~/.qwen/QWEN.md` | inlined | `~/.qwen/settings.json` | `~/.qwen/skills/<name>/` | `~/.qwen/agents/<name>.md` | - |
| **augment** | - | `~/.augment/rules/<name>.md` | `~/.augment/settings.json` | `~/.augment/skills/<name>/` | `~/.augment/agents/<name>.md` | - |

- [MCP servers](#global-mcp-servers) and [settings](#global-default-model-and-effort) have their own tables above. Agents use the same formats as project agents; see [the agent example](#global-agents).
- **Rules** are inlined into the instructions file, in the same `## Rules` block as the shared body. Augment has no user-level instructions file for the CLI, so its `~/.augment/rules/` entries are always on.

Paths marked `~/.config/` follow `XDG_CONFIG_HOME` when set. Devin agents use `~/.config/devin/agents/` on Linux and macOS and `%APPDATA%\devin\agents\` on Windows.

### Native discovery and shared paths

- **Discovery**: Copilot's path is for its CLI. OpenHands discovery applies to local conversations. Devin custom profiles are experimental. Trae needs **Settings > Beta > Subagents > Enable Subagents Directory**.
- **Configuration roots**: `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `COPILOT_HOME`, `CLINE_DIR`, `QODER_CONFIG_DIR`, `KIRO_HOME`, `JUNIE_HOME`, and `KIMI_CODE_HOME` replace the matching `~/.<tool>/` folder, and `GEMINI_CLI_HOME` is the parent of `.gemini/`. Everything under that folder moves together; paths outside it, such as `~/.agents/skills/`, stay put. Use absolute paths.
- **Shared paths** are written once: `~/.agents/skills/` (codex, windsurf, amp, zed, warp, goose, openhands), `~/.agents/agents/` (goose, openhands), `~/.agents/AGENTS.md` (cline, warp), `~/.gemini/GEMINI.md` (gemini, antigravity). If two tools would write different content to one path, the run stops before writing and names the path.
- **Copilot agent effort**: Copilot agent profiles have no effort key. A portable agent `effort` of `low`, `medium`, `high`, or `xhigh` goes to `subagents.agents.<name>.effortLevel` in `~/.copilot/settings.json`; other values raise a coverage note. Sync stops when the file has comments, since a rewrite drops them (rerun with `--backup` to rewrite it and keep `settings.json.bak`), or when a hand-set `effortLevel` has a different value.
- **Cursor** does not load the home-level `AGENTS.md`. Global sync installs a `sessionStart` hook and a script under `~/.cursor/hooks/` (POSIX shell on macOS and Linux, PowerShell on Windows) that passes the instructions to Cursor. It needs neither agnostic-ai, Python, nor jq, adds context without enforcing policy, and keeps existing `sessionStart` entries.
- **OpenCode** keeps `disable-model-invocation` in project and global skills. V2 honors it; V1 does not. See [OpenCode skills](@/docs/targets/opencode.md) for native metadata precedence.
- **OpenCode V1** reads `~/.claude/CLAUDE.md` only when `~/.config/opencode/AGENTS.md` does not exist, so syncing both tools stops Claude-only text from reaching it. [V2 discovers only `AGENTS.md`](https://opencode.ai/v2/docs/migrate-v1#instruction-files). Move any guidance from the Claude fallback into OpenCode's `AGENTS.md`.
- **Devin CLI and VS Code Copilot** read `~/.claude/` by default, so syncing claude plus windsurf or copilot delivers the same instructions through two paths.

### Targets left out

- Aider reaches a home instructions file only through a `read:` entry in `~/.aider.conf.yml`.
- Continue's one documented home setting is the `rules:` list in the `config.yaml` it rewrites itself.
- Jules documents nothing at user level.
- Ellipsis reads only files in the repository.

Hooks reach these tools at user level: Claude Code, CodeBuddy Code, Codex, Gemini, Qoder, Cursor, Augment, Kiro CLI V3, Copilot, Cline SDK/CLI, Grok Build, Kimi Code CLI, Qwen Code, and Mistral Vibe. The other tools have no user-level hook output:

- Factory, Devin CLI, Antigravity, Goose, and Crush use hook formats that global sync does not write. Crush supports only `PreToolUse`.
- Junie's hooks are Early Access.
- Amp, OpenCode, and Kilo take hooks only as TypeScript plugins. Sync writes them for OpenCode (`.opencode/plugins/`) and Kilo (`.kilo/plugin/`) at project level only. OpenCode defaults to V1; a hook selects [V2](@/docs/targets/opencode.md#opencode-v2-hooks) with `x-opencode.plugin-api: v2` on macOS and Linux. On Windows, V2 refuses matching callbacks, so keep the default V1 hooks.
