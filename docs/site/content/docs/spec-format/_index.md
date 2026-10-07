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

Each AI coding tool keeps its configuration in its own place and format: `.claude/agents/`, `.cursor/rules/`, `.codex/config.toml`, `.gemini/settings.json`. A spec is one file under `.agnostic-ai/` that says what you want, once. `agnostic-ai sync` writes it for every tool you enable, in that tool's own format.

- **One source to review.** A new rule or permission is one diff, not one per tool.
- **No drift.** `sync --check` fails when a tool's file no longer matches its spec.
- **Easy to add tools.** Adding a tool is one entry in `targets:`.
- **Visible gaps.** When a tool cannot hold a field, sync prints a coverage note.

## Folders

Each folder holds one kind of spec. Pick the one that fits what you want the agent to do.

| Folder | Holds | Use it when |
|--------|-------|-------------|
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

Two more entries sit next to them:

- `AGNOSTIC_AI.md` is the instruction text every entry-point file carries, such as `AGENTS.md` and `CLAUDE.md`. See [entry-point files](@/docs/configuration.md#entry-point-files).
- `local/` holds personal specs that stay out of Git. See [local overrides](@/docs/local-overrides.md).

Paths are relative to `.agnostic-ai/` by default, so `rules/*.md` means `.agnostic-ai/rules/*.md`. Override the directories with [`sources`](@/docs/configuration.md#sources). The [capability matrix](@/docs/targets/_index.md#capability-matrix) shows which tools get each kind. Each tool's page shows how it renders the kind.

`agnostic-ai new <kind> <name>` scaffolds an agent, skill, rule, hook, or MCP server. [Getting started](@/docs/getting-started.md) walks through a first rule.

Discovery is recursive:

- Every `.md` under `agents/`, `skills/`, `rules/`, `commands/`, `reviews/`, and `ignore/` loads.
- Every `.yaml` under `hooks/`, `mcps/`, `settings/`, and `environments/` loads.

## Nested layout: per-directory scope

A spec in a subdirectory of its source folder gets a **scope** equal to that subpath.

```
rules/
├── conventional-commits.md      # scope: ""    (root)
└── backend/
    ├── auth.md                  # scope: "backend"
    └── api/limits.md            # scope: "backend/api"
```

For rules, scope decides where the rule applies. A rule can set `scope: services/payments`, which wins over its folder under `rules/`. A folder scopes a rule only when it names a project directory. `agnostic-ai new rule payments-context --scope services/payments` creates one.

Scoped rules stay out of the root instruction file. Supported tools get a path condition or a nested instruction file. Other tools skip the rule with a warning, or fail under `on-unsupported: error`. [Directory-specific instructions](@/docs/scoped-context.md) has the tool matrix and selector limits.

## Target scoping

Four fields limit which tools get a spec of any kind.

| Field | Effect |
|-------|--------|
| `target` | Write only for this target. |
| `targets` | Write only for these targets. |
| `target-exclude` | Write for every target except this one. |
| `targets-exclude` | Write for every target except these. |

With none set, every target that supports the kind gets the spec. `target` wins over `targets`. Exclusion wins over inclusion.

### Import auto-scoping

A hook imported from a tool gets `target: <tool>` (`codex`, `claude`, or `gemini`). Delete the field to let it reach every target.

`import claude` and `import codex` do the same for agents and skills. When both `.claude/` and `.codex/` exist but only one has a spec, it gets `target: <tool>`. A spec in both stays unscoped. So does every spec in a single-tool project, so an import and a sync give identical files.

## Frontmatter rules

- YAML between two `---` lines at the top of the file.
- Empty frontmatter (`---\n---\n`) means no metadata.
- A file without frontmatter still loads; its name defaults to the filename.
- Malformed frontmatter counts as no metadata, and the whole file becomes the body.
- Fields not listed on a kind's page are written as given.

## Per-target body fences

To vary text per tool, wrap the part that differs in `::target` fences. Text outside a fence goes to every tool. Text inside goes only to the listed tools. The marker lines never reach the output.

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
- Fences also work in `.agnostic-ai/AGNOSTIC_AI.md`. A block reaches an entry-point file when any tool that reads the file is listed. Many tools share `AGENTS.md`, so `::target codex` text reaches every reader. A shared file is never split. See [Entry-point files](@/docs/configuration.md#entry-point-files).

## Path variables: `{{$NAME}}`

A spec body can name a directory without hardcoding one tool's layout. `{{$SKILLS_DIR}}` expands to `.claude/skills` for claude, `.agents/skills` for codex, and `.github/skills` for copilot.

| Variable | Resolves to |
|---|---|
| `{{$SKILLS_DIR}}` | the tool's skills directory |
| `{{$AGENTS_DIR}}` | the tool's agents directory |
| `{{$COMMANDS_DIR}}` | the tool's commands directory |
| `{{$RULES_DIR}}` | the tool's rules directory |
| `{{$MCP_FILE}}` | the tool's MCP config file |

- **Bodies only.** Frontmatter values do not expand.
- **An `outputs.<target>.<field>` override wins.** With `outputs.claude.skills-dir: custom/skills`, `{{$SKILLS_DIR}}` follows it.
- **A variable the tool has no directory for stays as written** and raises a coverage note. aider and jules resolve no variables.
- **A shared file resolves for every tool that reads it.** This covers the rules block of an entry point, the review section of `AGENTS.md`, and a nested `AGENTS.md`. `GEMINI.md` has one reader, so it gets `.gemini/skills`. Codex and Amp both read `AGENTS.md` and use `.agents/skills`, so it expands. Codex and OpenCode use different paths, so the variable stays as written and raises one coverage note naming the file.
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
- **A tool with no documented form gets a neutral phrase** and one coverage note. Unlike a path variable, the text is not left as written, because a raw `{{$AGENT:reviewer}}` reads worse to a model than plain words.
- **A rule or review in a shared file**, such as the rules block of `AGENTS.md` or a nested `AGENTS.md`, gets the neutral phrase, because several tools read that file. Codex reads its rules there, so a Codex rule always gets the neutral phrase. The same goes for a skill, agent, or command in a directory two enabled tools write, such as `.agents/skills` for Codex and Windsurf.
- **An unknown name fails `lint`** with LINT033, which lists the known agents or skills. So does a name whose spec does not sync to every target the referring spec reaches. Only project specs count: a teammate without your global agent would get a phrase that points at nothing.

## Target-specific extensions: `x-<target>` namespace

Use an `x-<target>:` block for fields only one tool reads. Other tools ignore it, so the spec stays portable.

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

- Check the keys against the tool's schema yourself. Each tool's page lists the keys it manages.
- A tool that has no output for a spec kind drops custom keys for that kind.
- Gemini TOML accepts only a string, bool, number, or string array, and skips nested tables.

On a settings spec, the block merges into the tool's settings file by the rules in [settings](@/docs/spec-format/settings.md#target-specific-keys).
