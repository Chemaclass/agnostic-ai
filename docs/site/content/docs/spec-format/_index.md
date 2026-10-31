+++
title = "Spec format"
description = "What each folder under .agnostic-ai/ holds, and the rules every spec shares."
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

A spec is a file under `.agnostic-ai/` that defines shared tool configuration. `agnostic-ai sync` writes it for each enabled tool in its native path and format, such as `.claude/agents/`, `.cursor/rules/`, `.codex/config.toml`, or `.gemini/settings.json`.

## Choose what to write

Pick the folder from the [table below](#folders), and write the smallest spec that covers the task: when it applies, what to do, and what to return. Check it with `agnostic-ai lint` and `agnostic-ai sync --dry-run`, then sync and run `agnostic-ai sync --check`.

## Terms used in these pages

- **Target:** an AI coding tool selected in `agnostic-ai.yaml`.
- **Portable:** a setting written once that sync translates for several tools.
- **Native:** a tool's own file format, setting, or feature.
- **Frontmatter:** YAML settings between `---` lines at the start of a Markdown file.
- **Scope:** the directory where a spec applies. A project-wide spec has no directory scope.
- **Glob:** a file pattern such as `src/**/*.go`, rather than one exact path.
- **Sync note:** a message explaining what a tool cannot use, leaves out, or handles differently.
- **MCP:** Model Context Protocol, a way for an agent to use outside tools and data. A `stdio` server runs a local command; HTTP, SSE, and WebSocket (`ws`) servers connect over a network.
- **Matcher:** a pattern that selects which tool calls trigger a hook. A regular expression (regex) is one pattern format.
- **Entry point:** the first instruction file a tool loads, such as `AGENTS.md` or `CLAUDE.md`.

## Folders

| Folder | Holds | Use it when |
|--------|-------|-------------|
| [`agents/`](@/docs/spec-format/agents.md) | Subagents with their own prompt, tools, and model | a task deserves a specialist with a narrow role |
| [`skills/`](@/docs/spec-format/skills.md) | Procedures loaded on demand, with bundled files | a workflow is long, occasional, or needs scripts |
| [`rules/`](@/docs/spec-format/rules.md) | Standing conventions, for the project, a directory, or a file pattern | the agent must follow it every time |
| [`commands/`](@/docs/spec-format/commands.md) | Slash commands | a person starts the workflow by name |
| [`hooks/`](@/docs/spec-format/hooks.md) | Commands the tool runs at events such as session start | something must happen every time, not when the model remembers |
| [`mcps/`](@/docs/spec-format/mcps.md) | MCP server connections | the agent needs an outside tool or data source |
| [`settings/`](@/docs/spec-format/settings.md) | Permissions, default model, and effort | one policy should decide what agents may run |
| [`reviews/`](@/docs/spec-format/reviews.md) | Guidance for code-review bots | a review bot should check project rules |
| [`environments/`](@/docs/spec-format/environments.md) | Worktree setup, dev servers, and cleanup | agents work in fresh worktrees |
| [`ignore/`](@/docs/spec-format/ignore.md) | Context and indexing exclusions for supported tools | irrelevant files crowd search or context |
| [`overlays/`](@/docs/spec-format/overlays.md) | Native settings and helper files that `import` captured | a tool setting has no portable spec |

Beside them, `AGNOSTIC_AI.md` holds the text every [entry-point file](@/docs/configuration.md#entry-point-files) carries, and `local/` holds [personal specs](@/docs/local-overrides.md) that stay out of Git.

- Paths are relative to `.agnostic-ai/`, so `rules/*.md` means `.agnostic-ai/rules/*.md`. [`sources`](@/docs/configuration.md#sources) overrides the directories.
- Subfolders load too: every `.md` under `agents/`, `rules/`, `commands/`, `reviews/`, and `ignore/`, and every `.yaml` under `hooks/`, `mcps/`, `settings/`, and `environments/`. Under `skills/`, Markdown beside a `SKILL.md` is bundled reference material, not another skill.
- `agnostic-ai new <kind> <name>` creates a starter spec for [each supported kind](@/docs/cli-reference/start.md#new). [Getting started](@/docs/getting-started.md) walks through a first rule.
- The [tool support table](@/docs/targets/_index.md#capability-matrix) shows which tools get each kind; each tool's page shows how it writes it.

## Nested layout: per-directory scope

A spec in a subdirectory of its source folder gets a **scope** equal to that subpath.

```
rules/
├── conventional-commits.md      # scope: ""    (root)
└── backend/
    ├── auth.md                  # scope: "backend"
    └── api/limits.md            # scope: "backend/api"
```

For rules, a folder scopes the rule only when it names a project directory, and a `scope:` field wins over the folder ([rule fields](@/docs/spec-format/rules.md#fields)). A scoped rule stays out of the root instruction file: supported tools get a path condition or a nested instruction file, and other tools skip it with a warning, or fail under `on-unsupported: error`. [Directory-specific instructions](@/docs/scoped-context.md) lists the tools and their limits.

## Target scoping

Four fields limit which tools get a spec of any kind. With none set, every target that supports the kind gets it.

| Field | Effect |
|-------|--------|
| `target` | Write only for this target. |
| `targets` | Write only for these targets. |
| `target-exclude` | Write for every target except this one. |
| `targets-exclude` | Write for every target except these. |

`target` wins over `targets`, and exclusion wins over inclusion.

### Import auto-scoping

A hook imported from a tool gets `target: <tool>` (`codex`, `claude`, or `gemini`). Delete the field to let it reach every target.

`import claude` and `import codex` do the same for an agent or skill that only one of `.claude/` and `.codex/` has. A spec in both, or any spec in a single-tool project, stays shared, so an import and a sync give identical files.

## YAML headers (frontmatter) {#frontmatter-rules}

- YAML between two `---` lines at the top of the file.
- Empty frontmatter (`---\n---\n`) means no metadata.
- A file without frontmatter still loads; its name defaults to the filename.
- Malformed YAML inside a closed frontmatter block fails loading. An unclosed block becomes body text, and `lint` reports LINT006.
- A closed opening block that is not a YAML mapping stays in the body; `lint` reports LINT056.
- Fields not listed on a kind's page are written as given.

## Per-target body fences

To vary text per tool, wrap the part that differs in `::target` fences. Text inside goes only to the listed tools; text outside goes to all. Marker lines start at column 0 and never reach the output.

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
| `::end` | Closes the last open fence. A missing `::end` runs to the end of the body. |

- `import` keeps fences intact. `import codex` adds them when Claude and Codex have the same agent or skill with different bodies.
- Fences also work in `.agnostic-ai/AGNOSTIC_AI.md`. A shared file is never split: a block reaches it when any tool that reads it is listed, so `::target codex` text reaches every `AGENTS.md` reader. See [Entry-point files](@/docs/configuration.md#entry-point-files).

## Path variables: `{{$NAME}}`

A spec body can name a tool's directory without hardcoding it. `{{$SKILLS_DIR}}` expands to `.claude/skills` for claude, `.agents/skills` for codex, and `.github/skills` for copilot.

| Variable | Resolves to |
|---|---|
| `{{$SKILLS_DIR}}` | the tool's skills directory |
| `{{$AGENTS_DIR}}` | the tool's agents directory |
| `{{$COMMANDS_DIR}}` | the tool's commands directory |
| `{{$RULES_DIR}}` | the tool's rules directory |
| `{{$MCP_FILE}}` | the tool's MCP config file |

- **Bodies only.** Frontmatter values do not expand.
- **An `outputs.<target>.<field>` override wins.** With `outputs.claude.skills-dir: custom/skills`, `{{$SKILLS_DIR}}` follows it.
- **A variable the tool has no directory for stays as written** and raises a sync note. aider and jules resolve no variables.
- **A shared file expands only when every reader agrees.** This covers the rules block of an entry point, the review section of `AGENTS.md`, and a nested `AGENTS.md`. Codex and Amp both use `.agents/skills`, so it expands; Codex and OpenCode differ, so it stays as written with one sync note naming the file. `GEMINI.md` has one reader and gets `.gemini/skills`.
- **Only where the tool has its own directory.** Tools that turn agents into rules (continue, trae, windsurf) or commands (gemini) have no `{{$AGENTS_DIR}}`. Antigravity, Goose, and OpenHands resolve it to `.agents/agents`.
- **The `$` is required.** Plain `{{placeholder}}` stays untouched, so Warp workflow arguments and Handlebars or Jinja text survive. Lowercase names such as `{{$skills_dir}}` do not resolve.

## Agent and skill references

A spec body can name another agent or skill the way each tool invokes it. `{{$AGENT:reviewer}}` and `{{$SKILL:commit}}` become that tool's documented phrase.

```md
Before pushing, run {{$AGENT:reviewer}}, then {{$SKILL:commit}}.
```

| Target | `{{$AGENT:reviewer}}` | `{{$SKILL:commit}}` | Source |
|---|---|---|---|
| claude | the reviewer subagent | /commit | [subagents](https://code.claude.com/docs/en/sub-agents#invoke-subagents-explicitly), [skills](https://code.claude.com/docs/en/skills#control-who-invokes-a-skill) |
| codex | the reviewer agent (neutral) | $commit | [skills](https://learn.chatgpt.com/docs/build-skills.md) |
| cursor | the reviewer subagent | /commit | [subagents](https://cursor.com/docs/subagents#explicit-invocation), [skills](https://cursor.com/docs/skills) |
| copilot | the reviewer agent | the /commit skill | [custom agents](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/create-custom-agents-for-cli), [skills](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-skills) |
| every other target | the reviewer agent | the commit skill | |

- **Bodies only**, with the `$` and an uppercase keyword, like path variables. `{{AGENT:reviewer}}` stays untouched.
- **A tool with no documented form gets a neutral phrase** and one sync note, not the raw text, which reads worse to a model.
- **A shared file gets the neutral phrase**: a rule or review in the rules block of `AGENTS.md` or a nested `AGENTS.md` (so a Codex rule always does), and a skill, agent, or command in a directory two enabled tools write, such as `.agents/skills` for Codex and Windsurf.
- **`lint` fails with LINT033** on an unknown name, or on a name whose spec does not sync to every target the referring spec reaches, and lists the known names. Only project specs count, since a teammate lacks your global agents.

## Target-specific extensions: `x-<target>` namespace

Use an `x-<target>:` block for fields only one tool reads. Other tools ignore it.

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

For each tool, sync removes all `x-*` keys, then moves the keys of the matching `x-<target>` block to the top level. They replace keys with the same name.

| Target | Resulting frontmatter |
|--------|-----------------------|
| `claude` | `name`, `description`, `model`, `allowed-tools` |
| `cursor` | `name`, `description`, `model`, `globs`, `alwaysApply` |
| `gemini` | `description` (`name` becomes the `.toml` filename; `model` is not written for commands) |

### Arbitrary custom keys

Any other key under `x-<target>` is written as given into that tool's output, in sorted order, and never reaches another tool.

- agnostic-ai does not check them against the tool's schema. Each tool's page lists the keys it manages.
- A tool with no output for a spec kind drops them.
- Gemini TOML accepts only a string, bool, number, or string array, and skips nested tables.

On a settings spec, the block merges into the tool's settings file by the rules in [settings](@/docs/spec-format/settings.md#target-specific-keys).
