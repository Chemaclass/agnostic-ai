+++
title = "Amp"
description = "How agnostic-ai emits Amp configuration: native paths, capability limits, and output options."
weight = 100

[extra]
group = "Reference"
target_id = "amp"
+++

# Amp (`amp`)

## Output

```
AGENTS.md                              # canonical entry-point pointer body (written by sync, shared across the AGENTS.md consumers)
.agents/skills/<name>/SKILL.md         # one folder per skill (Amp's native skills path)
.agents/setup                          # executable dependency setup when an environment sets install
.amp/services.yaml                     # supervised services when an environment sets terminals
.amp/settings.json                     # when MCP entries or settings keys exist (merged with existing user config)
```

- **Rules**: root rules inline into `AGENTS.md`. Scoped rules use `<scope>/AGENTS.md`; see [directory-specific instructions](@/docs/scoped-context.md) for shared-reader compatibility.
- **Skills**: one folder per skill at `.agents/skills/<name>/SKILL.md`, Amp's [native skills layout](https://ampcode.com/docs/customize/skills). Frontmatter carries `name` and `description`; sibling assets are copied byte-for-byte. Amp [replaced custom slash commands with skills](https://ampcode.com/news/slashing-custom-commands), so skills no longer emit as `.agents/commands/skill-<name>.md`.
  - Any `x-amp` key passes through. Set `x-amp.mcpServers` to scope MCP servers to one skill: the server's tools stay hidden until that skill loads. Amp also accepts a sibling `mcp.json` (frontmatter wins when both exist) and recommends skill-scoped servers over user settings for most cases ([Amp skills docs](https://ampcode.com/docs/customize/skills)).
  - Amp also loads skills from `.claude/skills/`, `~/.claude/skills/`, and `~/.claude/plugins/cache/` by default. It keeps the first skill with a given `name`, and project `.agents/skills/` comes before `.claude/skills/` ([skill precedence](https://ampcode.com/docs/customize/skills)). A skill synced to both `claude` and `amp` loads once, from `.agents/skills/`; a `claude`-only skill still loads in Amp.
  - User-level `~/.config/agents/skills/`, `~/.agents/skills/`, and `~/.config/amp/skills/` mask a project skill of the same name; `~/.claude/skills/` does not.
  - `amp.skills.disableClaudeCodeSkills` stops loading Claude Code directories, and `amp.skills.disableGlobalAgentsSkills` stops loading `~/.config/agents/skills/` and `~/.agents/skills/` ([settings schema](https://ampcode.com/cli-settings.schema.json)). Neither has a portable spelling; set them with `x-amp` on a settings spec. `amp.skills.path` adds colon- or semicolon-separated directories.
- **Agents**: no output. Amp [removed custom commands](https://ampcode.com/news/slashing-custom-commands), so sync no longer writes `.agents/commands/<name>.md` and removes files a previous sync left there.
  - Agent bodies reach Amp through the merged document when `outputs.amp.rules-file` is set. Otherwise they reach it only through the entry-point pointer to the source specs, and `sync` reports a coverage note.
  - Agents are not written as `.agents/skills/<name>/SKILL.md`: that tree is shared with codex, goose, crush, factory, augment, and antigravity, so it would duplicate the spec in those tools and overwrite a Skill of the same name. Amp's native agent API (`amp.createAgent(...)` / `amp.registerAgentMode(...)` in [plugins](https://ampcode.com/docs/customize/plugins)) is programmatic only.
- **Commands**: not supported. Amp has no file-based command surface; commands register in plugin code via `amp.registerCommand(...)` ([Amp docs index](https://ampcode.com/llms.txt)). A Command spec targeting amp is skipped with a warning (`on-unsupported: warn` by default).
  - `.agents/checks/` is gone from Amp's docs, so there is no reviews output either.
- **MCP**: written to `.amp/settings.json` under `amp.mcpServers` (one dotted key, not a nested object). Stdio emits `command`/`args`/`env`; HTTP/SSE emit `url`/`headers`. Other fields pass through with `x-amp`.
  - Use `x-amp.includeTools` to limit which tools a server exposes (tool names or glob patterns, [Amp skills docs](https://ampcode.com/docs/customize/skills)). It stays under `x-amp` because Amp's MCP page does not list it. Amp recommends exposing fewer tools for better model performance.
  - Existing non-managed keys (theme, editor settings) are kept; only `amp.mcpServers` is overwritten. Workspace MCPs need explicit approval on first open.
  - For a server used by one skill, put it under `x-amp.mcpServers` on the skill spec instead (see **Skills**).
- **Settings**: source only. **No portable field reaches Amp**: `permissions.allow`, `permissions.deny`, `permissions.ask`, and `model` each raise a coverage note on every sync. Only keys you write under `x-amp`, in Amp's spelling, are emitted. Your portable deny list is not enforced on Amp.
  - Output goes to `.amp/settings.json`, Amp's [workspace settings file](https://ampcode.com/docs/cli/settings), found by searching up from the working directory to the repository root. Workspace settings override user settings. MCP servers and settings share one merge: only `amp.mcpServers` and the `x-amp` keys are set, and every other key survives.
  - **`allow` and `ask` are permanently blocked.** Amp's [settings schema](https://ampcode.com/cli-settings.schema.json) has no tool allow-list: `amp.tools.disable` only disables, and `amp.mcpPermissions` matches MCP servers, not tools. There is no `ask` tier: Amp [does not ask for approval by default](https://ampcode.com/docs/tools) and points to a [permissions plugin](https://ampcode.com/docs/customize/plugins#example-plugin-permissions) instead. A portable `model` has no key either; Amp routes models through the Dial.
  - **`deny` is blocked until Amp publishes tool names.** `amp.tools.disable` takes tool names, `builtin:toolname` to disable only the built-in, and `*` globs, but the names come only from `amp tools list`. Guessing a name for `Bash(rm:*)` would write a rule Amp ignores, so sync reports a coverage note instead. Write the list yourself from `amp tools list` output:

    ```yaml
    # .agnostic-ai/settings/policy.yaml
    x-amp:
      amp.tools.disable:
        - "builtin:oracle"
        - "Bash*"
    ```

  - `amp.mcpServers` is refused on a settings spec, because it would replace every server the MCP specs contributed.
  - **Nearest wins, with no merging.** Amp reads only the nearest `.amp/settings.json` up from the working directory. A subdirectory with its own `.amp/` stops the root file being read for that subtree, for both settings and `amp.mcpServers`. Delete the nested `.amp/` or copy the generated keys into it.
- **Environments**: `install` writes executable [`.agents/setup`](https://ampcode.com/docs/orbs/customizing), which Amp runs while preparing a project orb or snapshot. Named `terminals` entries write [`.amp/services.yaml`](https://ampcode.com/docs/orbs/portals) as supervised services that run independently of the setup script. Each service needs `command`; its name must use only lowercase letters, numbers, and hyphens, start with a letter or number, and be at most 32 characters.
  - Other terminal fields pass through, so `x-amp.terminals` can set `cwd`, `port`, `env`, `health`, `portal`, `portals`, `review`, `agent`, and `platforms`. `platforms` is an optional nonempty list of `linux` or `darwin`; Amp skips a service that excludes the executor's OS, and an empty list is invalid.
  - `.agents/resume` is not emitted: it runs after activation and on every wake with thread credentials, which matches neither dependency setup nor a service. Multiple environment specs merge by top-level field, last value wins.
- **Legacy rename**: Amp uses `AGENTS.md` (plural). On first sync after upgrading, an agnostic-generated `AGENT.md` at the configured root is renamed to `AGENT.md.bak`. A user-authored `AGENT.md` (no `Generated by agnostic-ai` marker) is left alone.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.amp.skills-dir` | `.agents/skills` | |
| `outputs.amp.mcp-file` | `.amp/settings.json` | also carries settings; one file, one override |
| `outputs.amp.setup-file` | `.agents/setup` | |
| `outputs.amp.environment-file` | `.amp/services.yaml` | |
| `outputs.amp.rules-file` | unset | writes legacy concatenated rules and skips the pointer-body write |

`outputs.amp.commands-dir` no longer affects Amp: no command surface is left to point at.

## Import

`agnostic-ai import amp` reads `AGENTS.md`, skills from `.agents/skills/`, and MCP servers from `.amp/settings.json`. Settings keys are not imported, since none has a portable spelling. An old `.agents/commands/` tree still imports, one agent per file. Skill folders are restored in full, with bundled assets and executable modes.

## Verify

1. Install with `curl -fsSL https://ampcode.com/install.sh | bash` (Amp's recommended installer), or `npm install -g @ampcode/cli`. The VS Code extension reads the same files.
2. Check the tree: `ls AGENTS.md .agents/skills/ .agents/setup .amp/services.yaml .amp/settings.json`, `grep "Generated by agnostic-ai" .agents/skills/*/SKILL.md .agents/setup .amp/services.yaml`, `test -x .agents/setup`.
3. Confirm no command files came back: `test ! -d .agents/commands`.
4. Validate: `python -m json.tool .amp/settings.json > /dev/null`, and `amp orb services ensure` inside an orb.
5. Open the project. Amp lists `AGENTS.md` under "Project rules", loads each `.agents/skills/<name>/SKILL.md`, runs `.agents/setup` while preparing a new snapshot, and supervises each declared service.
6. The MCP picker lists every `amp.mcpServers` entry green. Workspace MCPs ask for approval the first time; this is expected.
7. Check that no nested workspace settings shadow the generated file: `find . -name settings.json -path '*/.amp/*' -not -path './.amp/*'` prints nothing.
