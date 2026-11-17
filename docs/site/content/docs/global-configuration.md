+++
title = "Global configuration"
description = "Install and manage your own cross-project instructions, skills, settings, and MCP servers."
weight = 135

[extra]
group = "Reference"

[extra.moved]
global-shared-names = "@/docs/global-configuration-reference.md#global-shared-names"
global-default-model-and-effort = "@/docs/global-configuration-reference.md#global-default-model-and-effort"
global-mcp-servers = "@/docs/global-configuration-reference.md#global-mcp-servers"
native-discovery-and-shared-paths = "@/docs/global-configuration-reference.md#native-discovery-and-shared-paths"
targets-left-out = "@/docs/global-configuration-reference.md#targets-left-out"
+++

# Global configuration

Keep instructions, rules, hooks, skills, agents, settings, and MCP servers for all your projects in one home folder. Run `agnostic-ai sync --global` to install them in each tool's user files. Put team defaults in a project or pack.

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
<details>
<summary>Limits, output rules and what sync owns</summary>

- Nested rules, rules with scope, path, glob, or target conditions, commands, settings `permissions` rule lists for targets other than Claude Code and CodeBuddy Code, inheritance, and merging with project specs are unsupported.
- Kilo inline shell placeholders pass the syntax check at documented trusted user destinations. See [Kilo shell commands](@/docs/targets/kilo-reference.md#shell-commands-in-global-skills) for approval, scope, and the kill switch.
- Skills copy their supporting files unchanged. Shared directories such as `~/.agents/skills/` keep neutral frontmatter. See [skill invocation support](@/docs/spec-format/skills-reference.md#disable-model-invocation-support-by-target) for `disable-model-invocation`, including OpenCode V2.
- Hooks, skills, and settings honor `target`, `targets`, and `targets-exclude`. A settings spec routed away from a tool sets none of its values there and does not override an earlier spec. Supported portable `on` events and `match` values translate for each tool. Native `event` and `matcher` values pass through unchanged; set those per target. See [portable hooks](@/docs/spec-format/hook-reference.md#portable-events).
- Some user hook routes have their own limits. Copilot: `import --global copilot` adopts matching handlers from neighboring user JSON files; see [Copilot hooks](@/docs/targets/copilot-reference.md#hooks). Cline: the VS Code extension does not read the hooks sync writes; see [Cline global hooks](@/docs/targets/cline-reference.md#global-hooks). Kiro CLI V3: one `~/.kiro/hooks/<name>.json` per spec, in the project hook format, with shared scripts in `~/.kiro/scripts/`; unrelated hook files stay, and a conflicting file stops sync.
- Targets without global agent output warn and skip agents.
- Output is real files, never symlinks. A user file that is itself a symlink, such as a dotfiles-managed `CLAUDE.md`, is written through. A symlink inside a skills, agents, or rules directory stops the run.
- Sync records what it wrote in `$AGNOSTIC_AI_HOME/state/global.json` and removes only those files and settings for the targets in the run. With `--only`, other targets' output stays in place. When the record is missing or lacks newly imported specs, `--check` notes it and fails only when a tool file differs too.
- A hand edit to a file sync owns, or to an instructions file's managed block, stops the run and names the file. Move the edit into the source, or rerun with `--backup` to overwrite or remove it and keep `<path>.bak`. Text outside the managed block never counts.
- A file or hook that already matches what sync would write is adopted. The run stops on one that differs by even one byte, an unmanaged agent, skill, or rule collision, a damaged marker, invalid native JSON, corrupt state, or state recorded under another `HOME`.
- Without `--only`, explicit targets, or a home `targets` list, a target with a relative root variable or an invalid agent name is skipped with a warning. Naming it makes that an error.
- Sync Goose and OpenHands together to update their shared agent file.

</details>

Global specs never merge with project specs. When both exist, each tool decides which wins; see [shared names](@/docs/global-configuration-reference.md#global-shared-names). Ordinary `agnostic-ai sync` reads only the names of your global specs, to warn about clashes.

<details>
<summary>Start the folder, target rules, commands, backup and import</summary>

To start the folder from the web interface, click **Open global settings** on a project page when none exists. The panel creates an empty `AGNOSTIC_AI.md` and `agnostic-ai.yaml`, makes sure `.gitignore` lists `/local/`, and can run `git init`. It does not run `sync --global`. See [the web interface](@/docs/cli-reference/serve.md#serve).

- A target with no user-level files, such as `aider` or `continue`, is skipped with one warning. An unknown name stops the run.
- [`builtins`](@/docs/configuration.md#built-ins), [`requires`](@/docs/configuration.md#requires), `lint`, and [`models`](@/docs/configuration-reference.md#models) work as in a project. For each key, `local/agnostic-ai.yaml` replaces the shared value, and a `models` tier replaces the entire shared tier of the same name. `sync --global` lists the tiers that have a `description` in the instructions block it manages.
- When a file's tiers do not load, `sync --global` stops. With `-t`, it warns, skips that file's tiers, and omits the `model` of any spec that names one.
- [`on-unsupported`](@/docs/configuration-reference.md#on-unsupported) (`warn`, `error`, or `silent`) sets what `sync --global` does with a skill line that Claude Code expands and other targets read as plain text. See [Claude Code body syntax](@/docs/spec-format/skills.md#claude-code-body-syntax).
- Other keys except `version` print a warning and are ignored.
- A target dropped from the list keeps its synced files and ownership records until you remove them by hand.

`list --global` shows the specs in effect with their `builtin`, `global`, or `global-local` layer. `validate --global` and `lint --global` check before a sync writes. `migrate --global` rewrites old spec forms. The [CLI reference](@/docs/cli-reference/sync.md#sync) lists the accepted `sync` flags.

Inside the global source root, `sync`, `init`, `import`, `new`, `packs`, `cleanup`, `revert`, and `install-hook` stop before any write and point you to the `--global` form. Read-only commands such as `lint`, `validate`, and `doctor` still run.

Keep the home in its own Git repository and push it to a remote to back it up. For a home kept in Git, `install-hook --global` writes a pre-commit hook that runs `lint --global --strict`, `validate --global`, and `sync --global --check`. To start a home from what your tools already hold, run `agnostic-ai import --global`. See [import](@/docs/cli-reference/import.md#import).

`import --global` reads [voice dictation](@/docs/spec-format/settings.md#voice-dictation) keys into shared `settings/imported.yaml`. Tool arguments select which native files to read. Existing home voice specs keep their scope; see [voice import](@/docs/spec-format/settings.md#voice-dictation) for exclusions and skipped values.

</details>

### Agents {#global-agents}

A personal agent such as `~/.agnostic-ai/agents/reviewer.md` with `targets: [claude, codex]` installs with:

```console
agnostic-ai sync --global --only claude,codex
agnostic-ai sync --global --only claude,codex --check
```

Sync writes `~/.claude/agents/reviewer.md` and `~/.codex/agents/reviewer.toml`. If a hand-copied file sits at an output path, copy its edits into the source and move the file aside first; `--backup` does not override a file sync does not own.

## More global settings

The [reference](@/docs/global-configuration-reference.md) covers:

- [Shared names](@/docs/global-configuration-reference.md#global-shared-names): what happens when a project and your home use the same skill or agent name.
- [Default model and effort](@/docs/global-configuration-reference.md#global-default-model-and-effort), permission defaults and session retention.
- [MCP servers](@/docs/global-configuration-reference.md#global-mcp-servers) in each tool's user file.
- [Native discovery and shared paths](@/docs/global-configuration-reference.md#native-discovery-and-shared-paths), and the [tools left out](@/docs/global-configuration-reference.md#targets-left-out).

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

- [MCP servers](@/docs/global-configuration-reference.md#global-mcp-servers) and [settings](@/docs/global-configuration-reference.md#global-default-model-and-effort) have their own tables above. Agents use the same formats as project agents; see [the agent example](#global-agents).
- **Rules** are inlined into the instructions file, in the same `## Rules` block as the shared body. Augment has no user-level instructions file for the CLI, so its `~/.augment/rules/` entries are always on.
- Existing instruction files keep their personal text and file permissions. Sync stops if a file changes while it is preparing the update.

Paths marked `~/.config/` follow `XDG_CONFIG_HOME` when set, except Zed on macOS. Zed on Linux and FreeBSD prefers `FLATPAK_XDG_CONFIG_HOME`; see [its native instruction roots](@/docs/targets/zed-reference.md#global-instructions). Devin agents use `~/.config/devin/agents/` on Linux and macOS and `%APPDATA%\devin\agents\` on Windows.

