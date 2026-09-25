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
AGENTS.md                            # pointer body (shared across AGENTS.md consumers)
.clinerules/<name>.md
.cline/agents/<name>.yml             # frontmatter over a Markdown system prompt
.cline/hooks/<Event>.sh              # one executable script per hook event
.cline/skills/<name>/SKILL.md        # one folder per skill (Cline's recommended skills path)
.clinerules/workflows/<name>.md      # one per agent, only when workflows-dir is set
```

Cline reads the root `AGENTS.md`, so `sync` writes the shared pointer body there (deduplicated with other AGENTS.md consumers). Rules emit one file each into `.clinerules/`.

Cline reads two project rules layouts, `.clinerules/` and `.cline/rules/`, and [searches both](https://docs.cline.bot/customization/cline-rules) in VS Code, Desktop, and the CLI. `.clinerules/` is the default because the VS Code Rules panel creates rules there. Set `outputs.cline.rules-dir: .cline/rules` for the other layout. Only one layout is written at a time: sync sweeps a stale managed tree at the other one, so rules never load twice. Neither layout is [deprecated](https://docs.cline.bot/resources/deprecations).

- **Agents**: always emit at `.cline/agents/`, whatever the rules override. Cline's source (`resolveAgentConfigSearchPaths` in `cline/cline`) returns `<workspace>/.cline/agents`, and all three shipping readers use it.
  - Each agent is a `<name>.yml` file: `---`-delimited frontmatter with the required `name` and `description`, then the spec body as the system prompt.
  - The loader accepts only `.yml` and `.yaml`, requires the content to open with `---`, and rejects an empty `name` or `description`. A spec with no description falls back to its name.
  - The provenance comment sits below the closing delimiter, inside the prompt. Above it, the file would not load.
  - `tools`, `skills`, `providerId`, `modelId`, and `maxIterations` go through `x-cline`. No heading is added above the body.
  - Older releases wrote `<name>.md` with no frontmatter, which Cline skipped. Sync sweeps a stale managed `.md`; hand-authored files stay.
- **Rule activation**: scoped rules emit conditional `paths` constrained to the directory, even with `alwaysApply: true`. For unscoped rules, `alwaysApply: false` enables native `paths` derived from the file selector; without a usable selector, the adapter reports a coverage note. See [Cline conditions](https://docs.cline.bot/customization/cline-rules) and [scoped selector limits](@/docs/scoped-context.md#narrow-a-rule-to-certain-files).
- **Skills**: one folder per skill under `.cline/skills/<name>/SKILL.md`, the path [Cline's skills docs](https://docs.cline.bot/customization/skills) recommend (`GlobalFileNames.clineSkillsDir` in the extension). A flat file under the rules directory never loads as a skill. Frontmatter carries `name` and `description`; sibling assets copy byte-for-byte.

### Hooks

Hooks emit as one executable script per event under `.cline/hooks/`, named after the event. Cline finds a hook by file name only. Its source declares ten names (`TaskStart`, `TaskResume`, `TaskCancel`, `TaskComplete`, `TaskError`, `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `PreCompact`, `SessionShutdown`), filters by extension, scans `<workspace>/.clinerules/hooks` and `<workspace>/.cline/hooks`, and runs each file as a subprocess.

This is confirmed in Cline's source but not its docs. The [hooks page](https://docs.cline.bot/customization/hooks) is a stub pointing at SDK Plugins (a TypeScript API). The [config reference](https://docs.cline.bot/getting-started/config) does list `.cline/hooks/` as lifecycle hooks.

- Scripts have no shebang. The provenance comment must be the first line for sync to treat the file as managed, and Cline runs `.sh` under `bash` anyway.
- **`matcher` is inert**: a hook file runs on every occurrence of its event and must filter itself. The adapter reports a note.
- **`timeout` is inert**: Cline uses its own runtime timeout. The adapter reports a note.
- Two specs on one event share one script, in spec order, under `set -e`. They also share stdout, which Cline reads as control JSON (the last `HOOK_CONTROL<TAB><json>` line wins when present). Send anything else to stderr.
- `PreCompact` maps to no runtime event yet, so its script is written, reported, and never run until Cline wires it up.
- Any other event gets a coverage note instead of a file.

### Workflows

When `outputs.cline.workflows-dir` is set, each agent also emits as `<dir>/<name>.md`, invokable from chat as `/<name>.md`, with the italic description before the body when present.

Set the key to `.clinerules/workflows`. Cline's resolver searches `.clinerules/workflows` and `.cline/workflows`, but the VS Code extension reads only `.clinerules/workflows`, and excludes it from the rules scan so a workflow never doubles as a rule. With `.cline/workflows`, only the CLI and SDK see the workflows, and the adapter says so in a note.

The key is opt-in because a workflow duplicates an agent already written to `.cline/agents/<name>.yml`, which is emitted either way. Cline's workflows doc page is currently a 404; the paths come from its resolver.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.cline.rules-dir` | `.clinerules` | set to `.cline/rules` for the other layout Cline reads; both are searched when present |
| `outputs.cline.agents-dir` | `.cline/agents` | |
| `outputs.cline.skills-dir` | `.cline/skills` | set to `.agents/skills` to share one tree with amp, codex, windsurf and zed; Cline scans it too |
| `outputs.cline.hooks-dir` | `.cline/hooks` | set to `.clinerules/hooks` for the other layout Cline resolves |
| `outputs.cline.workflows-dir` | empty | opt-in; set it to `.clinerules/workflows`, the only path both hosts read |

## Import

Import reads both `.clinerules/` and `.cline/rules/`. Identical duplicate rules dedupe; distinct files with the same destination fail with a conflict. The reserved `skills`, `workflows`, and `hooks` subdirectories do not become rules. Native `paths` arrays survive through `x-cline.paths`, including brace globs and empty arrays that disable activation.

A single-file `.clinerules` (still read by Cline) imports as one rule, `clinerules.md`, with its `paths` kept, and the `.clinerules/skills/` lookup is skipped. Sync and `doctor --fix` then replace the file with a `.clinerules/` directory, as Cline does, but only when its content matches a rule spec. Otherwise sync, `--check`, `--dry-run`, and `doctor --fix` fail and leave it alone: run `agnostic-ai import cline` first.

Imported rules keep the [filename prefix](@/docs/cli-reference.md#filename-prefix-reclassification) classification used by older layouts.

Agents come from `.cline/agents/<name>.yml`. Each becomes a `<name>.md` spec, unchanged except for the removed provenance header. `.yaml` is read too, and `.md` last, so older syncs still round-trip; `.yml` wins a same-name clash. The `agent-<name>.md` prefix applies only to the old layout where rules and agents shared `.clinerules/`.

Skills import from the four project paths Cline scans, in order: `.cline/skills/`, `.clinerules/skills/`, `.claude/skills/`, `.agents/skills/`. The first same-name skill wins. Bundled assets and executable modes survive. `.clinerules/skills/` is excluded from the rules walk.

`.agents/skills` is confirmed in Cline's source but not in [the skills page](https://docs.cline.bot/customization/skills), which lists three paths. Emission stays at the documented `.cline/skills`. To share one copy across targets, turn on `sync.shared-skills` or set `outputs.cline.skills-dir: .agents/skills`.

## Verify

1. Install the [Cline extension](https://marketplace.visualstudio.com/items?itemName=saoudrizwan.claude-dev) in VS Code.
2. Check the tree: `ls .clinerules/ .cline/agents/ .cline/skills/`, `grep "Generated by agnostic-ai" .clinerules/*.md` for the provenance header, `test -f .cline/skills/*/SKILL.md`.
3. Open the project and the Cline panel. Each `.clinerules/*.md` appears in the rules list with no "failed to parse" warnings. Each `.cline/agents/<name>.yml` appears where Cline lists project agents (`cline config`, Agent Teams via `--team-name`, the hub's agent list), and each `.cline/skills/<name>/` loads as a skill. Open a file matching a `paths` rule and the "Conditional rules applied: workspace:&lt;name&gt;.md" notification names it; an unrelated file does not.
4. Hooks: `ls .cline/hooks/` lists one `<Event>.sh` per hook event. Trigger the event (edit a file for `PostToolUse`, start a task for `TaskStart`) and the script runs.
5. If `outputs.cline.workflows-dir` is `.clinerules/workflows`, each `<name>.md` there is invokable as `/<name>.md` in the extension and the CLI, with the italic description as the preview.
