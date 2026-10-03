+++
title = "Amp"
description = "How agnostic-ai emits Amp configuration: native paths, capability limits, and output options."
weight = 100

[extra]
group = "Reference"
target_id = "amp"
+++

# Amp (`amp`)

Amp gets `AGENTS.md`, skills, MCP servers and `x-amp` settings, and orb setup from environment specs. It has no file-based agents or commands.

## Output

```
AGENTS.md                              # canonical entry-point pointer body (written by sync, shared across the AGENTS.md consumers)
.agents/skills/<name>/SKILL.md         # one folder per skill (Amp's native skills path)
.agents/setup                          # executable dependency setup when an environment sets install
.amp/services.yaml                     # supervised services when an environment sets terminals
.amp/settings.json                     # when MCP entries or settings keys exist (merged with existing user config)
```

- **Rules**: root rules inline into `AGENTS.md`. Scoped rules use `<scope>/AGENTS.md`; see [directory-specific instructions](@/docs/scoped-context.md) for shared-reader compatibility.
- **Skills**: Amp's [native skills layout](https://ampcode.com/docs/customize/skills). Frontmatter carries `name` and `description`; sibling assets copy byte-for-byte. Amp [replaced custom slash commands with skills](https://ampcode.com/news/slashing-custom-commands), so skills no longer emit as `.agents/commands/skill-<name>.md`.
  - Any `x-amp` key passes through. `x-amp.mcpServers` scopes MCP servers to one skill, hiding their tools until it loads; Amp recommends this over user settings for most cases ([Amp skills docs](https://ampcode.com/docs/customize/skills)). A sibling `mcp.json` also works (frontmatter wins).
  - Amp also loads `.claude/skills/`, `~/.claude/skills/`, and `~/.claude/plugins/cache/`. The first skill with a given `name` wins, and `.agents/skills/` comes before `.claude/skills/` ([skill precedence](https://ampcode.com/docs/customize/skills)). A skill synced to both `claude` and `amp` loads once; a `claude`-only skill still loads.
  - User-level `~/.config/agents/skills/`, `~/.agents/skills/`, and `~/.config/amp/skills/` mask a same-name project skill. `~/.claude/skills/` does not.
  - `amp.skills.disableClaudeCodeSkills` skips the Claude Code directories. `amp.skills.disableGlobalAgentsSkills` skips `~/.config/agents/skills/` and `~/.agents/skills/` ([settings schema](https://ampcode.com/cli-settings.schema.json)). Set both with `x-amp` on a settings spec; neither is portable. `amp.skills.path` adds colon- or semicolon-separated directories.
- **Agents**: no output. Amp [removed custom commands](https://ampcode.com/news/slashing-custom-commands), so sync no longer writes `.agents/commands/<name>.md` and removes old ones.
  - With `outputs.amp.rules-file` set, agent bodies reach Amp in the merged document. Otherwise only the entry-point pointer to the source specs reaches it, and `sync` notes it.
  - Agents do not go to `.agents/skills/<name>/SKILL.md`: in that tree, shared with the other `.agents/skills/` targets, they would duplicate the spec and overwrite a same-name Skill.
  - Amp's native agent API (`amp.createAgent(...)` / `amp.registerAgentMode(...)` in [plugins](https://ampcode.com/docs/customize/plugins)) is programmatic only.
- **Commands**: not supported. Commands register in plugin code via `amp.registerCommand(...)` ([Amp docs index](https://ampcode.com/llms.txt)). A Command spec for amp skips with a warning (`on-unsupported: warn` by default). `.agents/checks/` is gone from Amp's docs, so there is no reviews output either.
- **MCP**: under `amp.mcpServers` (one dotted key, not a nested object). Stdio emits `command`/`args`/`env`; HTTP/SSE emit `url`/`headers`. Other fields pass through with `x-amp`.
  - `x-amp.includeTools` limits the tools a server exposes (tool names or glob patterns, [Amp skills docs](https://ampcode.com/docs/customize/skills)); Amp recommends fewer tools for better model performance. It stays under `x-amp` because Amp's MCP page omits it.
  - Sync overwrites only `amp.mcpServers`; other keys (theme, editor settings) stay.
  - Workspace MCPs need explicit approval on first open.
- **Settings**: source only. **No portable field reaches Amp**: `permissions.allow`, `permissions.deny`, `permissions.ask`, and `model` each raise a coverage note on every sync, and your portable deny list is not enforced. Only `x-amp` keys, in Amp's spelling, are emitted.
  - They go to `.amp/settings.json`, Amp's [workspace settings file](https://ampcode.com/docs/cli/settings), found by searching up from the working directory to the repository root. Workspace settings override user settings. MCP servers and settings share one merge, and every other key survives.
  - **`allow` and `ask` are permanently blocked.** Amp's [settings schema](https://ampcode.com/cli-settings.schema.json) has no tool allow-list: `amp.tools.disable` only disables, and `amp.mcpPermissions` matches MCP servers, not tools. Amp [does not ask for approval by default](https://ampcode.com/docs/tools) and points to a [permissions plugin](https://ampcode.com/docs/customize/plugins#example-plugin-permissions). `model` has no key either; Amp routes models through the Dial.
  - **`deny` is blocked until Amp publishes tool names.** `amp.tools.disable` takes tool names, `builtin:toolname` to disable only the built-in, and `*` globs, but only `amp tools list` gives the names. A guessed name for `Bash(rm:*)` would be ignored, so sync reports a coverage note. Write the list yourself:

    ```yaml
    # .agnostic-ai/settings/policy.yaml
    x-amp:
      amp.tools.disable:
        - "builtin:oracle"
        - "Bash*"
    ```

  - A settings spec cannot set `amp.mcpServers`; it would replace every server from MCP specs.
  - **Nearest wins, with no merging.** A subdirectory with its own `.amp/` hides the root file for that subtree, for settings and `amp.mcpServers`. Delete the nested `.amp/` or copy the generated keys into it.
- **Environments**: `install` writes executable [`.agents/setup`](https://ampcode.com/docs/orbs/customizing), which Amp runs while preparing a project orb or snapshot. Named `terminals` write [`.amp/services.yaml`](https://ampcode.com/docs/orbs/portals): supervised services that run apart from the setup script.
  - Each service needs `command`. Its name uses only lowercase letters, numbers, and hyphens, starts with a letter or number, and has at most 32 characters.
  - `x-amp.terminals` can set `cwd`, `port`, `env`, `health`, `portal`, `portals`, `review`, `agent`, and `platforms`.
  - `platforms` is an optional nonempty list of `linux` or `darwin`. Amp skips a service that excludes the executor's OS. An empty list is invalid.
  - No `.agents/resume`: it runs after activation and on every wake with thread credentials, so it is neither dependency setup nor a service.
  - Environment specs merge by top-level field; the last wins.

{% <details summary="Old AGENT.md files"> %}
Amp uses `AGENTS.md` (plural). On first sync after upgrading, an agnostic-generated `AGENT.md` at the configured root is renamed to `AGENT.md.bak`. A user-authored `AGENT.md` (no `Generated by agnostic-ai` marker) is left alone.
{% </details> %}

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.amp.skills-dir` | `.agents/skills` | |
| `outputs.amp.mcp-file` | `.amp/settings.json` | also carries settings; one file, one override |
| `outputs.amp.setup-file` | `.agents/setup` | |
| `outputs.amp.environment-file` | `.amp/services.yaml` | |
| `outputs.amp.rules-file` | unset | writes legacy concatenated rules and skips the pointer-body write |

`outputs.amp.commands-dir` no longer has an effect.

## Import

`agnostic-ai import amp` reads `AGENTS.md`, skills from `.agents/skills/`, and MCP servers from `.amp/settings.json`. Skill folders are restored in full, with bundled assets and executable modes. Settings keys are not imported, since none has a portable spelling. An old `.agents/commands/` tree still imports, one agent per file.

## Protected paths

Advisory. This target has no native edit guard that sync writes, so sync prints a coverage note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install with `curl -fsSL https://ampcode.com/install.sh | bash` (Amp's recommended installer), or `npm install -g @ampcode/cli`. The VS Code extension reads the same files.
2. Check the tree: `ls AGENTS.md .agents/skills/ .agents/setup .amp/services.yaml .amp/settings.json`, `grep "Generated by agnostic-ai" .agents/skills/*/SKILL.md .agents/setup .amp/services.yaml`, `test -x .agents/setup`.
3. Confirm no command files came back: `test ! -d .agents/commands`.
4. Validate: `python -m json.tool .amp/settings.json > /dev/null`, and `amp orb services ensure` inside an orb.
5. Open the project. Amp lists `AGENTS.md` under "Project rules", loads each `.agents/skills/<name>/SKILL.md`, runs `.agents/setup` while preparing a new snapshot, and supervises each declared service.
6. The MCP picker lists every `amp.mcpServers` entry green, after the first-open approval.
7. No nested workspace settings shadow the generated file: `find . -name settings.json -path '*/.amp/*' -not -path './.amp/*'` prints nothing.
