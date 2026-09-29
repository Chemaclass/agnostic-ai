+++
title = "Spec format"
description = "Define agents, skills, rules, hooks, MCP servers, commands, settings, and other portable specs."
weight = 110
sort_by = "weight"
template = "docs/hub.html"
page_template = "docs/page.html"

[extra]
group = "Reference"
hub = true
nav_first = "Reference"
+++

# Spec format

The [capability matrix](@/docs/targets/_index.md#capability-matrix) shows which targets receive each spec kind. Each target page shows how that tool renders it.

Source paths are relative to `.agnostic-ai/` by default, so `rules/*.md` means `.agnostic-ai/rules/*.md`. Override directories with [`sources`](@/docs/configuration.md#sources).

Start with a [rule](@/docs/spec-format/skills-rules-commands.md#rules) for conventions, a [skill](@/docs/spec-format/skills-rules-commands.md#skills) for a reusable workflow, or an [MCP server](@/docs/spec-format/mcp-servers.md#mcp-servers) for a tool connection. See [Getting started](@/docs/getting-started.md) for a complete first rule.

| Kind    | Source                                    | Format                      |
|---------|-------------------------------------------|-----------------------------|
| [Agent](@/docs/spec-format/agent-specs.md#agents) | `agents/*.md`                             | Markdown + YAML frontmatter |
| [Skill](@/docs/spec-format/skills-rules-commands.md#skills) | `skills/*.md` or `skills/<name>/SKILL.md` | Markdown + YAML frontmatter |
| [Rule](@/docs/spec-format/skills-rules-commands.md#rules) | `rules/*.md`                              | Markdown + YAML frontmatter |
| [Hook](@/docs/spec-format/hooks.md#hooks) | `hooks/*.yaml`                            | YAML                        |
| [MCP](@/docs/spec-format/mcp-servers.md#mcp-servers) | `mcps/*.yaml`                             | YAML                        |
| [Command](@/docs/spec-format/skills-rules-commands.md#commands) | `commands/*.md`                           | Markdown + YAML frontmatter |
| [Settings](@/docs/spec-format/settings.md#settings) | `settings/*.yaml`                      | YAML                        |
| [Review](@/docs/spec-format/settings.md#reviews) | `reviews/*.md`                         | Markdown + YAML frontmatter |
| [Environment](@/docs/spec-format/settings.md#environments) | `environments/*.yaml`                  | YAML                        |
| [Ignore](@/docs/spec-format/settings.md#ignore) | `ignore/*.md`                          | Markdown + YAML frontmatter |

Discovery is recursive: every `.md` under `agents/`, `skills/`, `rules/`, `commands/`, `reviews/`, and `ignore/` loads, and every `.yaml` under `hooks/`, `mcps/`, `settings/`, and `environments/`.

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
- Fields not listed on this page pass through on emit.

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

On a settings spec the block merges into the target's settings file by the rules in [Settings](@/docs/spec-format/settings.md#settings). Codex takes no settings block and says so in a coverage note.
