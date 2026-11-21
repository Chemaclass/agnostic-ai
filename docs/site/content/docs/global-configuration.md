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
global-output = "@/docs/global-configuration-reference.md#global-output"
native-discovery-and-shared-paths = "@/docs/global-configuration-reference.md#native-discovery-and-shared-paths"
targets-left-out = "@/docs/global-configuration-reference.md#targets-left-out"
+++

# Global configuration

Keep instructions, rules, hooks, skills, agents, settings, and MCP servers for all your projects in one home folder. Run `agnostic-ai sync --scope user` to install them in each tool's user files. Put team defaults in a project or pack.

## Source and targets

The source folder is `$AGNOSTIC_AI_HOME`, or `~/.agnostic-ai/` when unset. `sync --scope user` runs from any directory; `--global` is its older spelling: still accepted, with a deprecation warning on stderr, and the only one releases up to v0.96.0 take. It loads no packs or project specs. [Global output](@/docs/global-configuration-reference.md#global-output) lists the paths it writes.

```text
~/.agnostic-ai/
├── agnostic-ai.yaml        # optional: targets, builtins, requires, lint, on-unsupported, models, approvals
├── AGNOSTIC_AI.md
├── agents/*.md
├── rules/*.md
├── hooks/*.yaml
├── settings/*.yaml         # default model and effort
├── mcps/*.yaml             # user-level MCP servers
├── skills/<name>/SKILL.md
└── local/                  # optional personal layer, same layout
```

Create a spec there from any directory with `agnostic-ai new rule my-style --scope user`. It takes agents, skills, rules, hooks, MCP servers, and settings. A user-level rule starts without `globs`, since it applies everywhere.

`local/` works as in a project ([local overrides](@/docs/local-overrides.md#override-fields)): a spec there merges into the shared spec of the same kind and name, and a new name adds one. `local/AGNOSTIC_AI.md` comes last in the managed instructions block. Add `/local/` to the source root's `.gitignore` before adding personal files.

To sync a fixed set of tools without `--only` on every run, list them in the source root's `agnostic-ai.yaml`:

```yaml
targets: [claude, codex, cursor]
```

- `sync --scope user`, `lint --scope user`, and `validate --scope user` then use only those targets.
- `--only` and `--except` narrow the list for one run. They must name configured targets.
- `--target` replaces the list and skips the home config's `targets`.
- A target with no user-level files, such as `aider` or `continue`, is skipped with one warning. An unknown name stops the run.

Global specs never merge with project specs. When both exist, each tool decides which wins ([shared names](@/docs/global-configuration-reference.md#global-shared-names)). Ordinary `agnostic-ai sync` reads only the names of your global specs, to warn about clashes.

<details>
<summary>Details: home config keys</summary>

- A `targets` list in `local/agnostic-ai.yaml` replaces the shared one.
- [`builtins`](@/docs/configuration.md#built-ins), [`requires`](@/docs/configuration.md#requires), `lint`, and [`models`](@/docs/configuration-reference.md#models) work as in a project.
- For each key, `local/agnostic-ai.yaml` replaces the shared value. A `models` tier replaces the whole shared tier of the same name.
- `sync --scope user` lists the tiers that have a `description` in the instructions block it manages.
- When a file's tiers do not load, `sync --scope user` stops. With `-t`, it warns, skips that file's tiers, and omits the `model` of any spec that names one.
- [`on-unsupported`](@/docs/configuration-reference.md#on-unsupported) (`warn`, `error`, or `silent`) sets what `sync --scope user` does with a skill line that Claude Code expands and other targets read as plain text ([Claude Code body syntax](@/docs/spec-format/skills.md#claude-code-body-syntax)).
- Other keys except `version` print a warning and are ignored.
- A target dropped from the list keeps its synced files and ownership records until you remove them by hand.

</details>

<details>
<summary>Details: start the folder, commands, backup</summary>

- In the web interface, click **Open global settings** on a project page when no folder exists ([serve](@/docs/cli-reference/serve.md#serve)).
- The panel creates an empty `AGNOSTIC_AI.md` and `agnostic-ai.yaml`, makes sure `.gitignore` lists `/local/`, and can run `git init`. It does not run `sync --scope user`.
- To start a home from what your tools already hold, run `agnostic-ai import --scope user` ([import](@/docs/cli-reference/import.md#import)).
- `import --scope user` reads [voice dictation](@/docs/spec-format/settings.md#voice-dictation) keys into shared `settings/imported.yaml`. Tool arguments select which native files to read.
- Existing home voice specs keep their scope ([voice import](@/docs/spec-format/settings.md#voice-dictation) lists exclusions and skipped values).
- `list --scope user` shows the specs in effect with their `builtin`, `user`, or `user-local` layer.
- `validate --scope user` and `lint --scope user` check before a sync writes. `migrate --scope user` rewrites old spec forms.
- The [CLI reference](@/docs/cli-reference/sync.md#sync) lists the accepted `sync` flags.
- Inside the global source root, `sync`, `init`, `import`, `new`, `packs`, `cleanup`, `revert`, and `install-hook` stop before any write and name the user-level command to run instead. Read-only commands such as `lint`, `validate`, and `doctor` still run.
- Keep the home in its own Git repository and push it to a remote to back it up.
- For a home kept in Git, `install-hook --scope user` writes a pre-commit hook that runs `lint --scope user --strict`, `validate --scope user`, and `sync --scope user --check`.

</details>

<details>
<summary>Details: what is unsupported</summary>

- Nested rules, and rules with scope, path, glob, or target conditions.
- Commands, inheritance, and merging with project specs.
- Settings `permissions` rule lists, except for Claude Code and CodeBuddy Code.
- Targets without global agent output warn and skip agents.

</details>

<details>
<summary>Details: hooks, skills and settings routing</summary>

- Hooks, skills, and settings honor `target`, `targets`, and `targets-exclude`.
- A settings spec routed away from a tool sets none of its values there and does not override an earlier spec.
- Supported portable `on` events and `match` values translate for each tool. Native `event` and `matcher` values pass through unchanged, so set those per target ([portable hooks](@/docs/spec-format/hook-reference.md#portable-events)).
- Skills copy their supporting files unchanged. Shared directories such as `~/.agents/skills/` keep neutral frontmatter ([skill invocation support](@/docs/spec-format/skills-reference.md#disable-model-invocation-support-by-target), including OpenCode V2).
- Kilo inline shell placeholders pass the syntax check at documented trusted user destinations ([Kilo shell commands](@/docs/targets/kilo-reference.md#shell-commands-in-global-skills) covers approval, scope, and the kill switch).
- Copilot: `import --scope user copilot` adopts matching handlers from neighboring user JSON files ([Copilot hooks](@/docs/targets/copilot-reference.md)).
- Cline: the VS Code extension does not read the hooks sync writes ([Cline global hooks](@/docs/targets/cline.md#global-hooks)).
- Kiro CLI V3: one `~/.kiro/hooks/<name>.json` per spec, in the project hook format, with shared scripts in `~/.kiro/scripts/`. Unrelated hook files stay, and a conflicting file stops sync.
- Sync Goose and OpenHands together to update their shared agent file.

</details>

<details>
<summary>Details: ownership, state and conflicts</summary>

- Output is real files, never symlinks.
- A user file that is itself a symlink, such as a dotfiles-managed `CLAUDE.md`, is written through.
- A symlink inside a skills, agents, or rules directory stops the run.
- Sync records what it wrote in `$AGNOSTIC_AI_HOME/state/global.json`. It removes only those files and settings, for the targets in the run.
- With `--only`, other targets' output stays in place.
- When the record is missing or lacks newly imported specs, `--check` notes it. It fails only when a tool file differs too.
- A hand edit to a file sync owns, or to an instructions file's managed block, stops the run and names the file. Text outside the managed block never counts.
- To resolve it, move the edit into the source. Or rerun with `--backup` to overwrite or remove the file and keep `<path>.bak`.
- A file or hook that already matches what sync would write is adopted.
- The run stops on a file or hook that differs by even one byte, an unmanaged agent, skill, or rule collision, a damaged marker, invalid native JSON, corrupt state, or state recorded under another `HOME`.
- Without `--only`, explicit targets, or a home `targets` list, a target with a relative root variable or an invalid agent name is skipped with a warning. Naming it makes that an error.

</details>

### Agents {#global-agents}

A personal agent such as `~/.agnostic-ai/agents/reviewer.md` with `targets: [claude, codex]` installs with:

```console
agnostic-ai sync --scope user --only claude,codex
agnostic-ai sync --scope user --only claude,codex --check
```

Sync writes `~/.claude/agents/reviewer.md` and `~/.codex/agents/reviewer.toml`.

If a hand-copied file sits at an output path, copy its edits into the source and move the file aside first. `--backup` does not override a file sync does not own.

## More global settings

The [reference](@/docs/global-configuration-reference.md) covers:

- [Shared names](@/docs/global-configuration-reference.md#global-shared-names): what happens when a project and your home use the same skill or agent name.
- [Default model and effort](@/docs/global-configuration-reference.md#global-default-model-and-effort), permission defaults and session retention.
- [MCP servers](@/docs/global-configuration-reference.md#global-mcp-servers) in each tool's user file.
- [Native discovery and shared paths](@/docs/global-configuration-reference.md#native-discovery-and-shared-paths), and the [tools left out](@/docs/global-configuration-reference.md#targets-left-out).
- [Global output](@/docs/global-configuration-reference.md#global-output): the user-level files `sync --scope user` writes for each tool.
