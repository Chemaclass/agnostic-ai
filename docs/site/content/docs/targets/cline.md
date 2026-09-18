+++
title = "Cline"
description = "How agnostic-ai emits Cline configuration: native paths, capability limits, and output options."
weight = 70

[extra]
group = "Reference"
target_id = "cline"
+++

# Cline (`cline`)

## Output

```
AGENTS.md                            # canonical entry-point pointer body (written by sync, shared across the AGENTS.md consumers)
.clinerules/<name>.md
.cline/agents/<name>.md
.cline/workflows/<name>.md           # one per agent, only when workflows-dir is set
.cline/skills/<name>/SKILL.md        # one folder per skill (Cline's recommended skills path)
```

Cline reads the cross-tool root `AGENTS.md`, so `sync` distributes the shared pointer body there (deduplicated with the other AGENTS.md consumers). Rules emit per file into `.clinerules/`, the only project rules path any Cline surface reads: the VS Code extension hardcodes it as `GlobalFileNames.clineRules` and resolves it against the workspace root, and Cline's README puts the CLI and the JetBrains plugin on the same path. The [cline-rules page](https://docs.cline.bot/customization/cline-rules) agrees.

`.cline/rules/` appears only in the project tree on the [config reference](https://docs.cline.bot/getting-started/config), which disagrees with the extension it documents. Releases before this one defaulted there on that page alone, so every rule landed where nothing loaded it (target-audit 2026-09-18). Set `outputs.cline.rules-dir: .cline/rules` if you want that path anyway; otherwise a stale managed tree there is swept on sync.

Agents always emit at `.cline/agents/`, independent of that override. That path is absent from `GlobalFileNames` too, but only the config page's rules entry has been contradicted by source, so agents stay where they are until a runtime check settles them. Cline's own file format for that directory has no dedicated doc page, so this adapter writes the spec body verbatim, with no synthesized heading and no invented frontmatter.

- **Rule activation**: scoped rules emit conditional `paths` constrained to the directory, even when the source sets `alwaysApply: true`. For unscoped rules, `alwaysApply: false` enables native `paths` derived from the file selector; without a usable selector, the adapter reports a coverage note. See [Cline conditions](https://docs.cline.bot/customization/cline-rules) and [scoped selector limits](@/docs/scoped-context.md#narrow-a-rule-to-certain-files).
- **Skills**: one folder per skill under `.cline/skills/<name>/SKILL.md`, the path [Cline's skills docs](https://docs.cline.bot/customization/skills) recommend and the extension confirms as `GlobalFileNames.clineSkillsDir`. A flat file directly under the rules directory never loads as a skill, so this is a folder, not a rule-form file. The SKILL.md frontmatter carries `name` + `description`; sibling assets next to the source SKILL.md are copied byte-for-byte.

When `outputs.cline.workflows-dir` is set, each agent also emits as a Markdown file at `<dir>/<name>.md`, invokable from chat as `/<name>.md`, with the italic description prefixing the body when present. Cline's doc for this feature, `docs.cline.bot/features/workflows`, 404s, and `llms.txt` lists no project-scoped replacement in the current `customization/` tree (Rules, `.clineignore`, Hooks, Plugins, Skills; no Workflows entry, target-audit 2026-08-08, #563).

Treat this as an unconfirmed export rather than a vendor-documented surface until a current doc backs it. The native `.cline/agents/<name>.md` emission still happens either way.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.cline.rules-dir` | `.clinerules` | set to `.cline/rules` to emit at the config page's path, which no Cline surface reads |
| `outputs.cline.agents-dir` | `.cline/agents` | |
| `outputs.cline.skills-dir` | `.cline/skills` | |
| `outputs.cline.workflows-dir` | empty | opt-in |

## Import

`agnostic-ai import cline` reads rules from `.clinerules/`, falling back to `.cline/rules/` for a project synced by an earlier release, and reclassifies each file by [filename prefix](@/docs/cli-reference.md#filename-prefix-reclassification).

It reconstructs agents from `.cline/agents/<name>.md`, Cline's native per-agent directory (#534). Each file copies byte-for-byte minus the provenance header. There is no `agent-` prefix to strip and no synthesized heading, since sync no longer writes one there. The `agent-<name>.md` prefix only fires when a project still carries the pre-#534 layout, where rules and agents shared `.clinerules/`.

Skills import from all three documented project paths, in this order: `.cline/skills/`, `.clinerules/skills/`, then `.claude/skills/`. The first same-name skill wins. Bundled assets and executable modes survive. `.clinerules/skills/` is excluded from the rules walk, so a skill folder there never imports as a rule.

## Verify

1. Install the [Cline extension](https://marketplace.visualstudio.com/items?itemName=saoudrizwan.claude-dev) in VS Code.
2. Check the tree: `ls .clinerules/ .cline/agents/ .cline/skills/`, `grep "Generated by agnostic-ai" .clinerules/*.md` for the provenance header, `test -f .cline/skills/*/SKILL.md`.
3. Open the project, open the Cline panel. Cline loads every `.clinerules/*.md`; each appears in the rules list with no "failed to parse" warnings. Each `.cline/agents/<name>.md` appears wherever Cline surfaces project agent definitions, and each `.cline/skills/<name>/` loads as a skill. Open a file matching a `paths` rule and the "Conditional rules applied: workspace:&lt;name&gt;.md" notification names it; open an unrelated file and it stays out.
4. If `outputs.cline.workflows-dir` is set, each `<workflows-dir>/<name>.md` is invokable as `/<name>.md`; the italic description previews the workflow.
