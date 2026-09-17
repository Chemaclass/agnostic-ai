+++
title = "Kilo"
description = "How agnostic-ai emits Kilo configuration: native paths, capability limits, and output options."
weight = 220

[extra]
group = "Reference"
target_id = "kilo"
+++

# Kilo (`kilo`)

```
AGENTS.md                          # canonical entry-point pointer body + inlined rules (written by sync, shared path)
.kilo/rules/<name>.md              # one per rule
.kilo/agents/<name>.md             # one per agent
.agents/skills/<name>/SKILL.md     # one folder per skill, plus any bundled assets (shared cross-tool tree)
.kilo/commands/<name>.md           # one per command
kilo.jsonc                         # instructions array (one entry per rule) and/or mcp map (merged with existing user config)
.kilocodeignore                    # compatibility input for Kilo's permission migrator
```

Kilo [Code](https://kilo.ai/docs) reads the root `AGENTS.md` natively and loads agents from `.kilo/agents/`, one `<name>.md` per agent. Kilo Code takes the agent's name from the filename, so `name:` is never written. Frontmatter carries `description` (falls back to the spec name) plus `color`, `mode`, and `model` when the spec sets them. `color` is the same generic top-level key augment and qoder also promote, written through without per-target validation (the three targets document different value spaces, see [`color` support by target](@/docs/spec-format.md#color-support-by-target)); `mode` shares OpenCode's `primary`/`subagent`/`all` vocabulary under the identical name.

Kilo Code's full [agent Configuration Options table](https://kilo.ai/docs/customize/custom-subagents) also documents `disable`, `hidden`, `steps`, `temperature`, and `top_p`. None has a confirmed counterpart on another registered target, and `temperature`/`top_p` are provider-scaled tuning knobs besides, so all five stay reachable only through `x-kilo` (e.g. `x-kilo: {temperature: 0.1, steps: 15}`) rather than a generic top-level key (target-audit 2026-08-08, #562). There is no `tools:` key: Kilo Code's full agent option table has no such field, so a spec's `tools` allowlist would silently do nothing. An agent with `tools` set surfaces a coverage note instead, and per-tool restriction goes through Kilo Code's native `permission` map via `x-kilo: {permission: {...}}`.

Skills emit into the shared `.agents/skills/<name>/SKILL.md` tree (target-audit 2026-08-01). Kilo Code documents its own `.kilo/skills/` path, but also lists `.agents/skills/` as a compatibility directory "loaded by default", and that is the same tree codex, amp, zed, crush, openhands, windsurf, and augment already write byte-identically, so pointing here dedupes instead of adding a second on-disk copy.

Unscoped rules emit as one file per rule under `.kilo/rules/`, each one also listed by its own path in `kilo.jsonc`'s [`instructions`](https://kilo.ai/docs/customize/custom-rules) array (target-audit 2026-08-01): "Each entry points to a file path or glob pattern". This adapter lists explicit paths rather than a `.kilo/rules/*.md` glob for ordinary rules. Scoped rules use nested `AGENTS.md` and are omitted from this unconditional list.

Kilo Code's own [precedence order](https://kilo.ai/docs/customize/agents-md) is agent prompt > project `instructions` > AGENTS.md > global, so the `instructions` entry outranks the shared `AGENTS.md` block below it. AGENTS.md is always loaded when present regardless, so this adapter keeps inlining full rule bodies there too as a fallback, rather than treating `instructions` as a replacement. The legacy `.kilocode/rules/` tree (the pre-rename Kilo Code branding) is separate and still auto-included for backward compatibility, but this adapter never emits it.

Commands emit as one Markdown file per command spec at `.kilo/commands/<name>.md`, the new Kilo Code extension's slash-command path: "Workflows are Markdown files stored as slash commands in `.kilo/commands/`" ([packages/kilo-docs/pages/customize/workflows.md](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/customize/workflows.md)), mirrored on GitHub since kilo.ai's rendered docs defeat fetching. Kilo Code takes the command name from the filename, so `name` is never written. Frontmatter carries `description`, `agent`, `model`, `variant`, and `subtask` when set, a field list near-identical to OpenCode's own command frontmatter; `variant` (a reasoning-effort override, e.g. `low` or `high`) is the one extra key this vendor documents (#630).

One name is taken. Kilo v7.6.0 made `goal` reserved: "A custom command or an MCP prompt named `goal` is reserved. Kilo rejects it and reports an error; rename it" ([code-with-ai/agents/goals](https://kilo.ai/docs/code-with-ai/agents/goals), verified 2026-09-11). A command spec called `goal` still emits, because dropping it would lose the spec in silence, and surfaces a coverage note telling you to rename it (#736). The same sentence also reserves an MCP **prompt** named `goal`. An MCP spec names a server, not a prompt, and prompt names come from the server itself at runtime, so nothing agnostic-ai writes can collide there.

MCP servers merge into the `mcp` map of `kilo.jsonc`, not `mcpServers`, which is the deprecated form. Stdio combines `command`+`args` into one `command` array and sets `type: "local"`, using `environment` for env vars; remote sets `type: "remote"` and uses `url`/`headers`. A spec's `disabled: true` writes `"enabled": false`, the key Kilo Code's own documented MCP example carries; an enabled server gets no key at all. Both transports preserve `timeout` in milliseconds, including zero. Remote servers accept `oauth: false` to disable automatic OAuth; `x-kilo` can override both options.

`instructions` and `mcp` merge into `kilo.jsonc` together; user-managed keys there survive every sync. That holds for a JSONC file too, which matters most here because the vendor documents comments on this exact file: "Disable a rule temporarily: Comment out the line in `kilo.jsonc` (JSONC supports `//` comments)" ([kilo.ai/docs/customize/custom-rules](https://kilo.ai/docs/customize/custom-rules)), and the page's own worked example carries a comment plus two trailing commas. `sync` strips both before reading. Keys survive, comments do not, and the sync that drops them prints a one-line warning (target-audit 2026-09-11, #725). Hooks have no Kilo surface yet and skip with a warning.

A settings spec's default `model` merges into the top level of `kilo.jsonc` alongside `instructions` and `mcp`. `import kilo` restores that portable field to `settings/imported.yaml` and continues to import `.kilocodeignore`.

Kilo Code also reads a second project-tier file, `.kilo/kilo.jsonc`, which this adapter does not write. The vendor's [documented 8-level config precedence](https://kilo.ai/docs/getting-started/settings#config-file-precedence) places `.kilo/kilo.jsonc` above the root `kilo.jsonc`. It describes this as higher levels overriding lower ones, a merge rather than an exclusive first-match read: any key the root file sets and `.kilo/kilo.jsonc` does not still reaches Kilo Code. A hand-authored `.kilo/kilo.jsonc` that redeclares `mcp` or `instructions` itself would shadow this adapter's output for those two keys specifically (target-audit 2026-08-27, #644).

Ignore specs write project-root `.kilocodeignore`. Kilo's [compatibility migrator](https://kilo.ai/docs/customize/context/kilocodeignore) converts its patterns into read/edit permission denials. This adapter writes the native input file and does not translate patterns into permission maps. `import kilo` imports this ignore file and the portable default model; other Kilo configuration is not imported. The shared hand-authored-file protection applies.

Config keys:

| Key | Default |
|-----|---------|
| `outputs.kilo.rules-dir` | `.kilo/rules` |
| `outputs.kilo.agents-dir` | `.kilo/agents` |
| `outputs.kilo.skills-dir` | `.agents/skills` |
| `outputs.kilo.commands-dir` | `.kilo/commands` |
| `outputs.kilo.mcp-file` | `kilo.jsonc` |
| `outputs.kilo.ignore-file` | `.kilocodeignore` |

Verify with the real IDE:

1. Install Kilo Code ([docs](https://kilo.ai/docs)).
2. Check the tree: `ls AGENTS.md .kilo/rules/ .kilo/agents/ .agents/skills/ .kilo/commands/ kilo.jsonc`, `grep "Generated by agnostic-ai" .kilo/rules/*.md .kilo/agents/*.md .kilo/commands/*.md` for the provenance header.
3. Open the project; each `.kilo/rules/<name>.md` listed in `kilo.jsonc`'s `instructions` array appears in the loaded-rules list, each `.kilo/agents/<name>.md` appears in the agent picker, each `.agents/skills/<name>/` folder loads as a skill, each `.kilo/commands/<name>.md` runs as `/<name>`, and each `mcp.<name>` from `kilo.jsonc` connects, with a disabled spec showing as disabled.
