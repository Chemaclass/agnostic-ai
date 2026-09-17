+++
title = "Amp"
description = "How agnostic-ai emits Amp configuration: native paths, capability limits, and output options."
weight = 100

[extra]
group = "Reference"
target_id = "amp"
+++

# Amp (`amp`)

```
AGENTS.md                              # canonical entry-point pointer body (written by sync, shared across the AGENTS.md consumers)
.agents/skills/<name>/SKILL.md         # one folder per skill (Amp's native skills path)
.agents/setup                          # executable dependency setup when an environment sets install
.amp/services.yaml                     # supervised services when an environment sets terminals
.amp/settings.json                     # when MCP entries exist (merged with existing user config)
```

- **Rules**: root rules inline into `AGENTS.md`. Scoped rules use `<scope>/AGENTS.md`; see [directory-specific instructions](@/docs/scoped-context.md) for shared-reader compatibility.
- **Skills**: one folder per skill under `.agents/skills/<name>/SKILL.md` (Amp's [native skills layout](https://ampcode.com/docs/customize/skills)). Amp [removed custom slash commands in favor of skills](https://ampcode.com/news/slashing-custom-commands), so skills no longer emit as `.agents/commands/skill-<name>.md`. The SKILL.md frontmatter carries `name` + `description`; sibling assets next to the source SKILL.md are copied byte-for-byte.
  - Arbitrary `x-amp` keys pass through, which is how a skill scopes its own MCP servers: Amp's docs say "a skill can define MCP servers in a sibling `mcp.json` file or in the `mcpServers` field of its `SKILL.md` frontmatter", and prefers `mcpServers` when both are present. Set `x-amp.mcpServers` on the skill spec and it lands in the emitted frontmatter verbatim, so the server's tools stay hidden until that skill loads. Amp's docs recommend this over user settings "for most use cases" (#591).
- **Agents**: no file of their own. They used to emit as slash commands under `.agents/commands/<name>.md`, until Amp [removed custom commands in favor of skills on 2026-01-29](https://ampcode.com/news/slashing-custom-commands) and its migration steps ended with "Delete the original command file". Emitting there wrote a file Amp never reads, on a green sync with no warning, so sync now writes nothing and sweeps what a previous sync left behind (#727).
  - Agent bodies reach Amp through the merged document when `outputs.amp.rules-file` is set, and otherwise only through the entry-point pointer to the source specs, which `sync` reports as a coverage note.
  - The migration's replacement path, `.agents/skills/<name>/SKILL.md`, is deliberately not reused: that tree is shared with codex, goose, crush, factory, augment and antigravity, most of which already emit the same Agent spec to their own native agent surface. Writing it there as a skill would duplicate one spec inside those tools and would overwrite a Skill spec of the same name. A native agent surface does exist (`amp.createAgent(...)` / `amp.registerAgentMode(...)` in [plugin TypeScript](https://ampcode.com/docs/customize/plugins)), but it is programmatic, out of reach of a declarative emitter.
- **Commands**: not supported. A full sweep of every page in [Amp's docs index](https://ampcode.com/llms.txt) finds no file-based command surface at all: commands register programmatically via `amp.registerCommand(...)` in plugin TypeScript, and the [migration post](https://ampcode.com/news/slashing-custom-commands) tells users to delete the old command file rather than pointing at a replacement path. Since there is no path left to redirect a Command spec to, a Command spec targeting amp is skipped with a warning (`on-unsupported: warn` by default) instead of writing a file Amp never reads.
  - `.agents/checks/`, a code-review surface documented as recently as the 2026-08-20 snapshot, is also gone from the current docs; that surface retires rather than becoming a target for a future reviews emitter.
- **MCP**: written into `.amp/settings.json` under `amp.mcpServers` (a single dotted key, not a nested object). Stdio emits `command`/`args`/`env`; HTTP/SSE emit `url`/`headers`. Any other field reaches the entry through `x-amp`, the same passthrough commands and skills already have; the builder enumerated a fixed set with no escape hatch until #634, so even an explicit `x-amp` key was dropped.
  - The field this unblocks is `includeTools`, "optional but recommended" per [ampcode.com/docs/customize/skills](https://ampcode.com/docs/customize/skills): "tool names or glob patterns used to choose which tools are exposed". It stays namespaced rather than mapped top-level because Amp's MCP page says servers "use the same configuration fields as MCP servers in skills" and then enumerates without naming it, so the clause implies the field and the enumeration does not. Amp's own advice is to trim: "Too many available tools can reduce model performance, so for best results, be selective."
  - Pre-existing non-managed keys (theme, editor settings) are preserved; only `amp.mcpServers` is overwritten. Workspace MCPs require explicit approval on first open (Amp's safety model).
  - This is the project-wide surface; to scope a server to one skill instead, which Amp's manual recommends "for most use cases", put it under `x-amp.mcpServers` on the skill spec (see **Skills** above) rather than emitting an MCP spec here.
- **Environments**: `install` writes executable [`.agents/setup`](https://ampcode.com/docs/orbs/customizing), which Amp runs while preparing a project orb or snapshot. Named `terminals` entries write [`.amp/services.yaml`](https://ampcode.com/docs/orbs/portals) as supervised services, so long-running processes survive independently of the setup script. Each service requires `command`; its name must contain only lowercase letters, numbers, and hyphens, start with a letter or number, and stay within 32 characters.
  - Remaining terminal fields pass through, so an `x-amp.terminals` override can use native `cwd`, `port`, `env`, `health`, `portal`, `portals`, `review`, and `agent` controls. `.agents/resume` is deliberately not emitted because it runs after activation and every wake with thread credentials, which is not equivalent to dependency installation or a supervised service. Multiple environment specs merge by top-level field, last value wins.
- **Legacy rename**: Amp's owner's manual specifies `AGENTS.md` (plural). On first sync after upgrading, any agnostic-generated `AGENT.md` at the configured root is renamed to `AGENT.md.bak`. A user-authored `AGENT.md` (no `Generated by agnostic-ai` marker) is left untouched.

Config keys:

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.amp.skills-dir` | `.agents/skills` | |
| `outputs.amp.mcp-file` | `.amp/settings.json` | |
| `outputs.amp.setup-file` | `.agents/setup` | |
| `outputs.amp.environment-file` | `.amp/services.yaml` | |
| `outputs.amp.rules-file` | unset | writes legacy concatenated rules and skips the pointer-body write |

`outputs.amp.commands-dir` no longer affects Amp now that no command surface is left to point at.

## Import

`agnostic-ai import amp` reads `AGENTS.md`, skills from `.agents/skills/`, and MCP servers from `.amp/settings.json`. A pre-migration `.agents/commands/` tree still imports, one agent per file. Each skill folder is restored in full, bundled assets included, and executable modes are preserved.

Verify with the real CLI:

1. Install with Amp's recommended direct installer: `curl -fsSL https://ampcode.com/install.sh | bash`. If your environment requires npm, use `npm install -g @ampcode/cli`. The VS Code extension reads the same files.
2. Check the tree: `ls AGENTS.md .agents/skills/ .agents/setup .amp/services.yaml .amp/settings.json`, `grep "Generated by agnostic-ai" .agents/skills/*/SKILL.md .agents/setup .amp/services.yaml`, `test -x .agents/setup`.
3. Confirm no command files came back: `test ! -d .agents/commands`.
4. Validate the generated files: `python -m json.tool .amp/settings.json > /dev/null` and `amp orb services ensure` inside an orb.
5. Open the project; Amp indexes `AGENTS.md` under "Project rules", loads each `.agents/skills/<name>/SKILL.md` as a skill, runs `.agents/setup` while preparing a new snapshot, and supervises each declared service.
6. The MCP picker lists every `amp.mcpServers` entry green. Workspace MCPs prompt for approval the first time (expected, not an error).
