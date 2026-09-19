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
.cline/agents/<name>.yml             # frontmatter over a Markdown system prompt
.cline/hooks/<Event>.sh              # one executable script per hook event
.cline/skills/<name>/SKILL.md        # one folder per skill (Cline's recommended skills path)
.clinerules/workflows/<name>.md      # one per agent, only when workflows-dir is set
```

Cline reads the cross-tool root `AGENTS.md`, so `sync` distributes the shared pointer body there (deduplicated with the other AGENTS.md consumers). Rules emit per file into `.clinerules/`. Cline reads two project rules layouts, `.clinerules/` and `.cline/rules/`, and the [cline-rules page](https://docs.cline.bot/customization/cline-rules) is explicit about it: "Both layouts are supported by VS Code, Desktop, and the CLI" and "Both directories are searched when present". The SDK resolver says the same, and adds why it matters: "Every Cline surface (CLI, VS Code extension, desktop app) must honor both."

`.clinerules/` is the default here because the VS Code Rules panel still creates there. Set `outputs.cline.rules-dir: .cline/rules` to use the other layout, which Cline reads just as well. Only one layout is written at a time: a stale managed tree at the layout you are not using is swept on sync, so the same rules never load twice. Neither layout is deprecated. Cline's [deprecations page](https://docs.cline.bot/resources/deprecations) lists `.clineignore`, "Explain Changes" and "Focus Chain", and no rules directory.

Agents always emit at `.cline/agents/`, independent of that override. `resolveAgentConfigSearchPaths` in `cline/cline` returns `<workspace>/.cline/agents`, and all three shipping readers resolve the directory through it, so the path is source-confirmed.

Each agent is a `<name>.yml` file: `---`-delimited frontmatter carrying the required `name` and `description`, then the spec body as the system prompt. The loader accepts only `.yml` and `.yaml`, throws unless the content opens with `---`, and rejects an empty `name` or `description`, so a spec with no description falls back to its name. The provenance comment sits below the closing delimiter, inside the prompt; above it, it would break the anchor and the file would not load. `tools`, `skills`, `providerId`, `modelId`, and `maxIterations` go in through `x-cline`. No heading is synthesized above the body.

Releases before this one wrote `<name>.md` with no frontmatter, which every shipping Cline surface skipped (target-audit 2026-09-19). A stale managed `.md` there is swept on sync; hand-authored files stay.

- **Rule activation**: scoped rules emit conditional `paths` constrained to the directory, even when the source sets `alwaysApply: true`. For unscoped rules, `alwaysApply: false` enables native `paths` derived from the file selector; without a usable selector, the adapter reports a coverage note. See [Cline conditions](https://docs.cline.bot/customization/cline-rules) and [scoped selector limits](@/docs/scoped-context.md#narrow-a-rule-to-certain-files).
- **Skills**: one folder per skill under `.cline/skills/<name>/SKILL.md`, the path [Cline's skills docs](https://docs.cline.bot/customization/skills) recommend and the extension confirms as `GlobalFileNames.clineSkillsDir`. A flat file directly under the rules directory never loads as a skill, so this is a folder, not a rule-form file. The SKILL.md frontmatter carries `name` + `description`; sibling assets next to the source SKILL.md are copied byte-for-byte.

### Hooks

Hooks emit as one executable script per event under `.cline/hooks/`, named after the event. Cline discovers a hook by file name and nothing else: `HookConfigFileName` in `sdk/packages/core/src/hooks/hook-file-config.ts` declares ten names (`TaskStart`, `TaskResume`, `TaskCancel`, `TaskComplete`, `TaskError`, `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `PreCompact`, `SessionShutdown`), an extension allowlist gates the file, and `listHookConfigFiles` scans `resolveHooksConfigSearchPaths`, which returns `<workspace>/.clinerules/hooks` and `<workspace>/.cline/hooks`. The scripts run: `hook-file-hooks.ts` builds a command per file with `inferHookCommand` and `parseShebangCommand`, then runs it through `runSubprocessEvent`.

This surface is source-confirmed and doc-unconfirmed. [`/customization/hooks`](https://docs.cline.bot/customization/hooks) is still a one-line stub pointing at SDK Plugins, where hooks are a TypeScript `AgentPlugin` API rather than shell-command-on-event, and that stub is why agnostic-ai declined hooks for cline until now. The [config reference](https://docs.cline.bot/getting-started/config) corroborates the directory, showing `.cline/  hooks/  # Lifecycle hooks` in its project tree and warning "Hooks and plugins can execute code."

- A script carries no shebang. The provenance comment has to be the first line for sync to recognize the file as managed, and `.sh` runs under `bash` through `inferHookCommand` without one.
- **`matcher` is inert**: a hook file is per event with no matcher, so the script runs on every occurrence and has to filter itself. The adapter reports a note.
- **`timeout` is inert**: Cline times hook subprocesses out on its own runtime setting. The adapter reports a note.
- Two specs on one event share one script, in spec order, under `set -e`. The file name is the event, so there is no second slot. They also share one stdout, which `parseStdout` reads as control JSON, preferring the last `HOOK_CONTROL<TAB><json>` line when one is present. Send anything else to stderr.
- `PreCompact` is a file name Cline lists but maps to no runtime event today, so the script is written at the vendor's own name, reported, and never run until Cline wires the event up.
- An event outside those ten earns a coverage note instead of a file nothing scans for.

### Workflows

When `outputs.cline.workflows-dir` is set, each agent also emits as a Markdown file at `<dir>/<name>.md`, invokable from chat as `/<name>.md`, with the italic description prefixing the body when present.

Set the key to `.clinerules/workflows`. `resolveWorkflowsConfigSearchPaths` returns `<workspace>/.clinerules/workflows` and `<workspace>/.cline/workflows`, but the VS Code extension reads only the first: `workflows.ts` resolves `GlobalFileNames.workflows`, which is `.clinerules/workflows`. The extension also excludes that subdirectory from its rules scan, so a workflow there never doubles as an always-on rule. Point the key at `.cline/workflows` and the workflows reach the CLI and SDK alone; the adapter says so in a note.

The key stays opt-in. A workflow is a second copy of an agent already emitted at `.cline/agents/<name>.yml`, so turning it on by default would double every agent. Cline's doc for the feature, `docs.cline.bot/features/workflows`, still 404s, but the resolver settles the paths (target-audit 2026-09-19, #889; the dead doc was #563). The native `.cline/agents/<name>.yml` emission happens either way.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.cline.rules-dir` | `.clinerules` | set to `.cline/rules` for the other layout Cline reads; both are searched when present |
| `outputs.cline.agents-dir` | `.cline/agents` | |
| `outputs.cline.skills-dir` | `.cline/skills` | set to `.agents/skills` to share one tree with amp, codex, windsurf and zed; Cline scans it too |
| `outputs.cline.hooks-dir` | `.cline/hooks` | set to `.clinerules/hooks` for the other layout Cline resolves |
| `outputs.cline.workflows-dir` | empty | opt-in; set it to `.clinerules/workflows`, the only path both hosts read |

## Import

`agnostic-ai import cline` reads rules from `.clinerules/`, falling back to `.cline/rules/`, the other layout Cline reads, and reclassifies each file by [filename prefix](@/docs/cli-reference.md#filename-prefix-reclassification).

It reconstructs agents from `.cline/agents/<name>.yml`, Cline's native per-agent directory. Each file becomes a `<name>.md` spec, byte-for-byte minus the provenance header: frontmatter and body carry across unchanged, so only the extension moves. `.yaml` is read too, and `.md` last, so a project synced before the format fix still round-trips; a `.yml` wins a same-name collision. There is no `agent-` prefix to strip and no synthesized heading, since sync no longer writes one there. The `agent-<name>.md` prefix only fires when a project still carries the pre-#534 layout, where rules and agents shared `.clinerules/`.

Skills import from all four project paths Cline scans, in this order: `.cline/skills/`, `.clinerules/skills/`, `.claude/skills/`, then `.agents/skills/`. The first same-name skill wins. Bundled assets and executable modes survive. `.clinerules/skills/` is excluded from the rules walk, so a skill folder there never imports as a rule.

`.agents/skills` is source-confirmed and doc-unconfirmed: `getWorkspaceSkillDirectories` maps `.clinerules`, `.cline` and `.agents` onto `<dir>/skills`, and the VS Code extension returns `agentsSkillsDir: ".agents/skills"` independently, while [the skills page](https://docs.cline.bot/customization/skills) still lists three. Emission stays at `.cline/skills`, the documented default. To keep one on-disk copy across targets, either turn on `sync.shared-skills` or set `outputs.cline.skills-dir: .agents/skills`.

## Verify

1. Install the [Cline extension](https://marketplace.visualstudio.com/items?itemName=saoudrizwan.claude-dev) in VS Code.
2. Check the tree: `ls .clinerules/ .cline/agents/ .cline/skills/`, `grep "Generated by agnostic-ai" .clinerules/*.md` for the provenance header, `test -f .cline/skills/*/SKILL.md`.
3. Open the project, open the Cline panel. Cline loads every `.clinerules/*.md`; each appears in the rules list with no "failed to parse" warnings. Each `.cline/agents/<name>.yml` appears wherever Cline surfaces project agent definitions (`cline config`, Agent Teams via `--team-name`, the hub's agent list), and each `.cline/skills/<name>/` loads as a skill. Open a file matching a `paths` rule and the "Conditional rules applied: workspace:&lt;name&gt;.md" notification names it; open an unrelated file and it stays out.
4. Hooks: `ls .cline/hooks/` lists one `<Event>.sh` per hook event. Trigger the event (edit a file for `PostToolUse`, start a task for `TaskStart`) and the script runs.
5. If `outputs.cline.workflows-dir` is set to `.clinerules/workflows`, each `<name>.md` there is invokable as `/<name>.md` in both the extension and the CLI; the italic description previews the workflow.
