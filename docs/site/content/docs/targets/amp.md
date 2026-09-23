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
- **Skills**: one folder per skill under `.agents/skills/<name>/SKILL.md` (Amp's [native skills layout](https://ampcode.com/docs/customize/skills)). Amp [removed custom slash commands in favor of skills](https://ampcode.com/news/slashing-custom-commands), so skills no longer emit as `.agents/commands/skill-<name>.md`. The SKILL.md frontmatter carries `name` + `description`; sibling assets next to the source SKILL.md are copied byte-for-byte.
  - Arbitrary `x-amp` keys pass through, which is how a skill scopes its own MCP servers: Amp's docs say "a skill can define MCP servers in a sibling `mcp.json` file or in the `mcpServers` field of its `SKILL.md` frontmatter", and prefers `mcpServers` when both are present. Set `x-amp.mcpServers` on the skill spec and it lands in the emitted frontmatter verbatim, so the server's tools stay hidden until that skill loads. Amp's docs recommend this over user settings "for most use cases" (#591).
  - Amp also reads skills from Claude Code's directories, and it does so by default. [Amp's settings schema](https://ampcode.com/cli-settings.schema.json) describes `amp.skills.disableClaudeCodeSkills` as disabling "loading skills from Claude Code directories (`.claude/skills/`, `~/.claude/skills/`, and `~/.claude/plugins/cache/`)". Amp keeps the first skill with a given `name`, and project `.agents/skills/` comes before `.claude/skills/` ([skill precedence](https://ampcode.com/docs/customize/skills)). So a skill synced to both `claude` and `amp` loads once, from `.agents/skills/`. A skill that reaches only `claude` still loads in Amp from `.claude/skills/`; set `amp.skills.disableClaudeCodeSkills` to stop that. The user-level `~/.config/agents/skills/`, `~/.agents/skills/`, and `~/.config/amp/skills/` mask a project skill of the same name, while `~/.claude/skills/` does not. A second key, `amp.skills.disableGlobalAgentsSkills`, covers the user-level `~/.config/agents/skills/` and `~/.agents/skills/` directories the same way. Both toggles are reachable today only through `x-amp` on a settings spec, since neither has a portable spelling. `amp.skills.path` adds colon- or semicolon-separated directories rather than removing any.
- **Agents**: no file of their own. They used to emit as slash commands under `.agents/commands/<name>.md`, until Amp [removed custom commands in favor of skills on 2026-01-29](https://ampcode.com/news/slashing-custom-commands) and its migration steps ended with "Delete the original command file". Emitting there wrote a file Amp never reads, on a green sync with no warning, so sync now writes nothing and sweeps what a previous sync left behind (#727).
  - Agent bodies reach Amp through the merged document when `outputs.amp.rules-file` is set, and otherwise only through the entry-point pointer to the source specs, which `sync` reports as a coverage note.
  - The migration's replacement path, `.agents/skills/<name>/SKILL.md`, is deliberately not reused: that tree is shared with codex, goose, crush, factory, augment and antigravity, most of which already emit the same Agent spec to their own native agent surface. Writing it there as a skill would duplicate one spec inside those tools and would overwrite a Skill spec of the same name. A native agent surface does exist (`amp.createAgent(...)` / `amp.registerAgentMode(...)` in [plugin TypeScript](https://ampcode.com/docs/customize/plugins)), but it is programmatic, out of reach of a declarative emitter.
- **Commands**: not supported. [Amp's docs index](https://ampcode.com/llms.txt) has no file-based command surface at all: commands register programmatically via `amp.registerCommand(...)` in plugin TypeScript, and the [migration post](https://ampcode.com/news/slashing-custom-commands) tells users to delete the old command file rather than pointing at a replacement path. With no path left to redirect to, a Command spec targeting amp is skipped with a warning (`on-unsupported: warn` by default) instead of writing a file Amp never reads.
  - `.agents/checks/`, a code-review surface documented as recently as the 2026-08-20 snapshot, is also gone from the current docs; that surface retires rather than becoming a target for a future reviews emitter.
- **MCP**: written into `.amp/settings.json` under `amp.mcpServers` (a single dotted key, not a nested object). Stdio emits `command`/`args`/`env`; HTTP/SSE emit `url`/`headers`. Any other field reaches the entry through `x-amp`, the same passthrough skills already have; the builder enumerated a fixed set with no escape hatch until #634, so even an explicit `x-amp` key was dropped.
  - The field this unblocks is `includeTools`, "optional but recommended" per [ampcode.com/docs/customize/skills](https://ampcode.com/docs/customize/skills): "tool names or glob patterns used to choose which tools are exposed". It stays namespaced rather than mapped top-level because Amp's MCP page says servers "use the same configuration fields as MCP servers in skills" and then enumerates without naming it, so the clause implies the field and the enumeration does not. Amp's own advice is to trim: "Too many available tools can reduce model performance, so for best results, be selective."
  - Pre-existing non-managed keys (theme, editor settings) are preserved; only `amp.mcpServers` is overwritten. Workspace MCPs require explicit approval on first open (Amp's safety model).
  - This is the project-wide surface. To scope a server to one skill instead, put it under `x-amp.mcpServers` on the skill spec (see **Skills** above) rather than emitting an MCP spec here.
- **Settings**: source only. A settings spec is kept as a portable spec, and **no portable field reaches Amp**: `permissions.allow`, `permissions.deny`, `permissions.ask`, and `model` each raise a coverage note on every sync. Only keys you write yourself under `x-amp`, in Amp's own spelling, are emitted. Do not read "Amp supports settings" as "your portable deny list is enforced on Amp", because it is not.
  - What lands, lands in `.amp/settings.json`, which Amp documents as its [workspace settings file](https://ampcode.com/docs/cli/settings): "the nearest `.amp/settings.json` or `.amp/settings.jsonc`, searched upward from your current working directory to the repository root". Workspace settings override user settings. MCP servers and settings share one merge, so only `amp.mcpServers` and the `x-amp` keys are ever set and every other key in the file survives the sync.
  - **`allow` and `ask` are permanently blocked, on vendor posture.** [Amp's settings schema](https://ampcode.com/cli-settings.schema.json) declares more than twenty properties, none of them a tool allow-list; `amp.tools.disable` is the only tool key and it only disables. The one allow-shaped construct, `amp.mcpPermissions`, matches MCP *servers* by `command` or `url`, not tools. There is no `ask` tier either: [Amp's tools page](https://ampcode.com/docs/tools) says "By default, Amp does not ask for approval before running tools", and its permissions section points at a [custom plugin](https://ampcode.com/docs/customize/plugins#example-plugin-permissions) rather than a setting. A portable `model` has no key here either; Amp routes models through the Dial. No future vendor change to the tool list unblocks any of these three.
  - **`deny` is blocked on a missing vocabulary, not on the surface.** `amp.tools.disable` has the right shape: "Disable specific tools by name. Use `builtin:toolname` to disable only the built-in tool with that name while allowing an MCP server to provide a tool by the same name. Glob patterns using `*` are supported." But Amp publishes no tool names: "You can see Amp's builtin tools by running `amp tools list` in the CLI". Translating `Bash(rm:*)` into a guessed name would write a file Amp silently ignores, which looks like a green sync that enforced nothing, so sync reports a coverage note instead. Write the deny-list yourself with the names `amp tools list` prints; the portable mapping lands once those names are confirmed (#950).

    ```yaml
    # .agnostic-ai/settings/policy.yaml
    x-amp:
      amp.tools.disable:
        - "builtin:oracle"
        - "Bash*"
    ```

  - `amp.mcpServers` is the one key the hatch refuses. It is built from MCP specs, each of which carries its own `x-amp` block, so a settings spec setting it would replace every server those specs contributed instead of adding to them.
  - **Nearest wins, and there is no merging.** The vendor sentence is a search path, not a fixed location: Amp takes the nearest `.amp/settings.json` walking up from the working directory, and documents no merging between workspace files. A subdirectory with its own `.amp/` silently stops the repository-root file being read for that subtree. This is not new with settings; `amp.mcpServers` has always shipped on the same path with the same property. If you keep a nested `.amp/`, either delete it or copy the generated keys into it.
- **Environments**: `install` writes executable [`.agents/setup`](https://ampcode.com/docs/orbs/customizing), which Amp runs while preparing a project orb or snapshot. Named `terminals` entries write [`.amp/services.yaml`](https://ampcode.com/docs/orbs/portals) as supervised services, so long-running processes survive independently of the setup script. Each service requires `command`; its name must contain only lowercase letters, numbers, and hyphens, start with a letter or number, and stay within 32 characters.
  - Remaining terminal fields pass through, so an `x-amp.terminals` override can use native `cwd`, `port`, `env`, `health`, `portal`, `portals`, `review`, `agent`, and `platforms` controls. `platforms` is an optional nonempty list of `linux` or `darwin`; Amp skips a service that excludes the executor's operating system, and an explicit empty list is invalid. `.agents/resume` is deliberately not emitted because it runs after activation and every wake with thread credentials, which is not equivalent to dependency installation or a supervised service. Multiple environment specs merge by top-level field, last value wins.
- **Legacy rename**: Amp's owner's manual specifies `AGENTS.md` (plural). On first sync after upgrading, any agnostic-generated `AGENT.md` at the configured root is renamed to `AGENT.md.bak`. A user-authored `AGENT.md` (no `Generated by agnostic-ai` marker) is left untouched.

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

`agnostic-ai import amp` reads `AGENTS.md`, skills from `.agents/skills/`, and MCP servers from `.amp/settings.json`. Settings keys are not imported: nothing in that file has a portable spelling yet, so there is nothing to lift out of it. A pre-migration `.agents/commands/` tree still imports, one agent per file. Each skill folder is restored in full, bundled assets included, and executable modes are preserved.

## Verify

1. Install with Amp's recommended direct installer: `curl -fsSL https://ampcode.com/install.sh | bash`. If your environment requires npm, use `npm install -g @ampcode/cli`. The VS Code extension reads the same files.
2. Check the tree: `ls AGENTS.md .agents/skills/ .agents/setup .amp/services.yaml .amp/settings.json`, `grep "Generated by agnostic-ai" .agents/skills/*/SKILL.md .agents/setup .amp/services.yaml`, `test -x .agents/setup`.
3. Confirm no command files came back: `test ! -d .agents/commands`.
4. Validate the generated files: `python -m json.tool .amp/settings.json > /dev/null` and `amp orb services ensure` inside an orb.
5. Open the project; Amp indexes `AGENTS.md` under "Project rules", loads each `.agents/skills/<name>/SKILL.md` as a skill, runs `.agents/setup` while preparing a new snapshot, and supervises each declared service.
6. The MCP picker lists every `amp.mcpServers` entry green. Workspace MCPs prompt for approval the first time (expected, not an error).
7. Confirm no nested workspace settings shadow the generated file: `find . -name settings.json -path '*/.amp/*' -not -path './.amp/*'` must print nothing.
