+++
title = "Spec format"
description = "What each folder under .agnostic-ai/ holds, and the conventions every spec shares."
weight = 110
sort_by = "weight"
template = "docs/hub.html"
page_template = "docs/page.html"
aliases = ["/docs/spec-format/skills-rules-commands/"]

[extra]
group = "Reference"
hub = true
nav_first = "Reference"

[extra.moved]
mcp-servers = "@/docs/spec-format/mcps.md"
+++

# Spec format

Every AI coding tool keeps its configuration in its own place and shape: `.claude/agents/`, `.cursor/rules/`, `.codex/config.toml`, `.gemini/settings.json`. A spec is one file under `.agnostic-ai/` that says what you want once. `agnostic-ai sync` writes it into every tool you enable, in that tool's native format.

- **One source to review.** A new rule or permission is one diff, not one per tool.
- **No drift.** `sync --check` fails when a native file no longer matches its spec.
- **Free to switch tools.** Adding a tool is one entry in `targets:`, not a rewrite.
- **Visible gaps.** When a tool cannot hold a field, sync prints a coverage note instead of dropping it silently.

## Folders

Each folder holds one spec kind. Pick the page for what you want the agent to do.

| Folder | Holds | Reach for it when |
|--------|-------|-------------------|
| [`agents/`](@/docs/spec-format/agents.md) | Subagents with their own prompt, tools, and model | a task deserves a specialist with a narrow role |
| [`skills/`](@/docs/spec-format/skills.md) | Procedures loaded on demand, with bundled files | a workflow is long, occasional, or needs scripts |
| [`rules/`](@/docs/spec-format/rules.md) | Standing conventions, for the project, a directory, or a file pattern | the agent must follow it every time |
| [`commands/`](@/docs/spec-format/commands.md) | Slash commands | a person starts the workflow by name |
| [`hooks/`](@/docs/spec-format/hooks.md) | Commands the tool runs on lifecycle events | something must happen every time, not when the model remembers |
| [`mcps/`](@/docs/spec-format/mcps.md) | MCP server connections | the agent needs an outside tool or data source |
| [`settings/`](@/docs/spec-format/settings.md) | Permissions, default model, and effort | one policy should decide what agents may run |
| [`reviews/`](@/docs/spec-format/reviews.md) | Guidance for code-review bots | a review bot should check project rules |
| [`environments/`](@/docs/spec-format/environments.md) | Worktree setup, dev servers, and cleanup | agents work in fresh worktrees |
| [`ignore/`](@/docs/spec-format/ignore.md) | Paths the agent must not read | secrets or build output must stay out of context |
| [`overlays/`](@/docs/spec-format/overlays.md) | Native settings and helper files that `import` captured | a tool setting has no portable spec |

Two more entries sit next to them. `AGNOSTIC_AI.md` is the instruction body every entry-point file carries, such as `AGENTS.md` and `CLAUDE.md`; see [entry-point files](@/docs/configuration.md#entry-point-files). `local/` holds personal specs that stay out of Git; see [local overrides](@/docs/local-overrides.md).

Paths are relative to `.agnostic-ai/` by default, so `rules/*.md` means `.agnostic-ai/rules/*.md`. Override the directories with [`sources`](@/docs/configuration.md#sources). The [capability matrix](@/docs/targets/_index.md#capability-matrix) shows which targets receive each kind, and each target page shows how that tool renders it.

`agnostic-ai new <kind> <name>` scaffolds an agent, skill, rule, hook, or MCP server. [Getting started](@/docs/getting-started.md) walks through a first rule.

Discovery is recursive: every `.md` under `agents/`, `skills/`, `rules/`, `commands/`, `reviews/`, and `ignore/` loads, and every `.yaml` under `hooks/`, `mcps/`, `settings/`, and `environments/`.

The sections below apply to every kind.

## Nested layout: per-directory scope

A spec in a subdirectory of its source dir gets an implicit **scope** equal to that subpath.

```
rules/
├── conventional-commits.md      # scope: ""    (root)
└── backend/
    ├── auth.md                  # scope: "backend"
    └── api/limits.md            # scope: "backend/api"
```

For rules, scope controls native activation or directory discovery. A flat rule may set `scope: services/payments`; source-layout scope wins. `agnostic-ai new rule payments-context --scope services/payments` creates one.

Scoped bodies stay out of root instruction appendices. Supported targets get native path conditions or a nested instruction file. Unsupported targets skip the rule with a warning, or fail under `on-unsupported: error`. See [directory-specific instructions](@/docs/scoped-context.md) for the target matrix and selector limits.

## Target scoping

Four fields limit where any spec kind emits.

| Field | Effect |
|-------|--------|
| `target` | Emit only to this one target. |
| `targets` | Emit only to these targets. |
| `target-exclude` | Emit everywhere except this target. |
| `targets-exclude` | Emit everywhere except these targets. |

With none set, the spec emits to every target that supports its kind. `target` beats `targets`. Exclusion wins over inclusion.

### Import auto-scoping

A hook imported from a tool gets `target: <tool>` (`codex`, `claude`, or `gemini`). Delete the field to let it reach every target.

`import claude` and `import codex` do the same for agents and skills. When both `.claude/` and `.codex/` exist but only one has a spec, it gets `target: <tool>`. A spec in both stays unscoped, and so does every spec in a single-tool project, so round-trips stay byte-identical.

## Frontmatter rules

- YAML between two `---` lines at the top of the file.
- Empty frontmatter (`---\n---\n`) means no metadata.
- A file without frontmatter still loads; its name defaults to the filename.
- Malformed frontmatter counts as no metadata, and the whole file becomes the body.
- Fields not listed on a kind's page pass through on emit.

## Per-target body fences

To vary prose per target, wrap the divergent part in `::target` fences. Content outside a fence emits everywhere; content inside emits only to the listed targets. Marker lines never reach the output.

```md
Shared intro paragraph.

::target claude
Claude-only section.
::end

::targets codex gemini
Codex and Gemini section.
::end
```

| Syntax | Meaning |
|--------|---------|
| `::target <name>` | Opens a fence for one target. |
| `::targets <a> <b>` | Opens a fence for several targets. |
| `::end` | Closes the most recent fence. A missing `::end` runs to the end of the body. |

- `import` round-trips keep fences intact. `import codex` builds them when Claude and Codex ship the same agent or skill with different bodies.
- Fences also work in `.agnostic-ai/AGNOSTIC_AI.md`. A block reaches an entry-point file when any target that reads the file is listed. `AGENTS.md` is shared by the whole family, so `::target codex` content reaches every reader; a shared file is never split. See [Entry-point files](@/docs/configuration.md#entry-point-files).

## Path variables: `{{$NAME}}`

A spec body can name a directory without hardcoding one target's layout. `{{$SKILLS_DIR}}` expands to `.claude/skills` for claude, `.agents/skills` for codex, and `.github/skills` for copilot.

| Variable | Resolves to |
|---|---|
| `{{$SKILLS_DIR}}` | the target's skills directory |
| `{{$AGENTS_DIR}}` | the target's agents directory |
| `{{$COMMANDS_DIR}}` | the target's commands directory |
| `{{$RULES_DIR}}` | the target's rules directory |
| `{{$MCP_FILE}}` | the target's MCP config file |

- **Bodies only.** Frontmatter values do not expand.
- **An `outputs.<target>.<field>` override wins.** With `outputs.claude.skills-dir: custom/skills`, `{{$SKILLS_DIR}}` follows it.
- **A variable with no target surface stays verbatim** and raises a coverage note. aider and jules resolve no variables.
- **Only where the target has a dedicated surface.** Targets that flatten agents into rules (continue, trae, windsurf) or commands (gemini) declare no `{{$AGENTS_DIR}}`. Antigravity, Goose, and OpenHands resolve it to `.agents/agents`.
- **The `$` sigil is required.** Plain `{{placeholder}}` stays untouched, so Warp workflow arguments and Handlebars or Jinja survive. Lowercase names such as `{{$skills_dir}}` do not resolve.

## Target-specific extensions: `x-<target>` namespace

Use an `x-<target>:` block for fields only one adapter reads. Other adapters strip it, so the spec stays portable.

```markdown
---
name: code-reviewer
description: Reviews diffs.
model: sonnet
x-claude:
  allowed-tools: [Read, Grep, Bash]
x-cursor:
  globs: "src/**"
  alwaysApply: false
---
```

For each target, all `x-*` keys are dropped, then the matching `x-<target>` block is flattened into the top level, overriding keys with the same name.

| Target | Resulting frontmatter |
|--------|-----------------------|
| `claude` | `name`, `description`, `model`, `allowed-tools` |
| `cursor` | `name`, `description`, `model`, `globs`, `alwaysApply` |
| `gemini` | `description` (`name` becomes the `.toml` filename; `model` is not emitted for commands) |

### Arbitrary custom keys

Any other key under `x-<target>` emits verbatim into that target's output, in sorted order, and never leaks across targets. Validate them against the target's schema yourself. Each target page lists the keys its adapter manages. A target with no surface for a spec kind drops custom keys for that kind. Gemini TOML accepts only a string, bool, number, or string array, and skips nested tables.

On a settings spec the block merges into the target's settings file by the rules in [settings](@/docs/spec-format/settings.md#target-specific-keys).
