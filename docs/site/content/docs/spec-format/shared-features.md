+++
title = "Shared spec features"
description = "Rules every spec kind shares: scope, target fields, frontmatter, body fences, path variables, and x-<target> blocks."
weight = 120
+++

# Shared spec features

These rules apply to every kind in [`.agnostic-ai/`](@/docs/spec-format/_index.md). Each kind's own page lists its fields.

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

<details>
<summary>Details: import and shared files</summary>

- `import` keeps fences intact. `import codex` adds them when Claude and Codex have the same agent or skill with different bodies.
- Fences also work in `.agnostic-ai/AGNOSTIC_AI.md`. A shared file is never split: a block reaches it when any tool that reads it is listed, so `::target codex` text reaches every `AGENTS.md` reader. See [Entry-point files](@/docs/configuration-reference.md#entry-point-files).

</details>

## Path variables: `{{$NAME}}`

A spec body can name a tool's directory without hardcoding it. `{{$SKILLS_DIR}}` expands to `.claude/skills` for claude, `.agents/skills` for codex, and `.github/skills` for copilot.

| Variable | Resolves to |
|---|---|
| `{{$SKILLS_DIR}}` | the tool's skills directory |
| `{{$AGENTS_DIR}}` | the tool's agents directory |
| `{{$COMMANDS_DIR}}` | the tool's commands directory |
| `{{$RULES_DIR}}` | the tool's rules directory |
| `{{$MCP_FILE}}` | the tool's MCP config file |

Variables expand in bodies only, not frontmatter. An `outputs.<target>.<field>` override wins: with `outputs.claude.skills-dir: custom/skills`, `{{$SKILLS_DIR}}` follows it. The `$` is required, so plain `{{placeholder}}` stays untouched.

<details>
<summary>Details: unresolved variables, shared files, tool exceptions</summary>

- **A variable the tool has no directory for stays as written** and raises a sync note. aider and jules resolve no variables.
- **A shared file expands only when every reader agrees.** This covers the rules block of an entry point, the review section of `AGENTS.md`, and a nested `AGENTS.md`. Codex and Amp both use `.agents/skills`, so it expands; Codex and OpenCode differ, so it stays as written with one sync note naming the file. `GEMINI.md` has one reader and gets `.gemini/skills`.
- **Only where the tool has its own directory.** Tools that turn agents into rules (continue, trae, windsurf) or commands (gemini) have no `{{$AGENTS_DIR}}`. Antigravity, Goose, and OpenHands resolve it to `.agents/agents`.

- Frontmatter values do not expand.
- Lowercase names such as `{{$skills_dir}}` do not resolve. Warp workflow arguments and Handlebars or Jinja text survive.

</details>

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

References work in bodies only, with the `$` and an uppercase keyword. `lint` fails with LINT033 on an unknown name. A tool with no documented form gets a neutral phrase and one sync note.

<details>
<summary>Details: shared files and LINT033</summary>

- **Bodies only**, with the `$` and an uppercase keyword, like path variables. `{{AGENT:reviewer}}` stays untouched.
- **A tool with no documented form gets a neutral phrase** and one sync note.
- **A shared file gets the neutral phrase**: a rule or review in the rules block of `AGENTS.md` or a nested `AGENTS.md` (so a Codex rule always does), and a skill, agent, or command in a directory two enabled tools write, such as `.agents/skills` for Codex and Windsurf.
- **`lint` fails with LINT033** on an unknown name, or on a name whose spec does not sync to every target the referring spec reaches, and lists the known names. Only project specs count, since a teammate lacks your user-level agents.

</details>

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

<details>
<summary>Details: custom key limits</summary>

- agnostic-ai does not check them against the tool's schema. Each tool's page lists the keys it manages.
- A tool with no output for a spec kind drops them.
- Gemini TOML accepts only a string, bool, number, or string array, and skips nested tables.

</details>

On a settings spec, the block merges into the tool's settings file by the rules in [settings](@/docs/spec-format/settings-reference.md#target-specific-keys).

## Terms used in these pages {#terms}

<details>
<summary>Glossary</summary>

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

</details>
