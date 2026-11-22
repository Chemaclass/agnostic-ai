+++
title = "Skills"
description = "skills/: procedures the agent loads when a task needs them, with their scripts and templates."
weight = 20

[extra]
group = "Reference"

[extra.moved]
disable-model-invocation-support-by-target = "@/docs/spec-format/skills-reference.md"
+++

# Skills

`skills/` holds procedures for tasks such as releasing a version or investigating a bug report. The tool keeps each skill's `description` in view and reads the body and bundled files only when the skill applies. Use a [rule](@/docs/spec-format/rules.md) for what applies to every task.

## Write one

`agnostic-ai new skill yaml-validator` creates a starter under `.agnostic-ai/skills/`. Use a flat `skills/yaml-validator.md`, or a nested `skills/yaml-validator/SKILL.md` when the skill has files, such as `schema.yaml` beside it.

```markdown
---
name: yaml-validator
description: Validate YAML against a schema. Use when a YAML file changes or the user asks to check one.
---

# YAML Validator

1. Read target file
2. Parse YAML and compare against `schema.yaml`
3. Report violations as `path: message`
```

Run `agnostic-ai lint`, `agnostic-ai sync`, and `agnostic-ai sync --check`. Then ask Claude Code to use `/yaml-validator`, or Codex to use `$yaml-validator`, with a YAML file and its schema.

## Fields

| Field | Default | Description |
|-------|---------|-------------|
| `name` | dir or filename | Skill name and output directory. Some tools restrict the format. |
| `description` | empty | One line the model uses to decide whether to use the skill. |
| `argument-hint` | unset | Hint for the arguments to a Claude Code slash command. |
| `disable-model-invocation` | unset | `true` stops the model from using the skill on its own. A person can still call it. See [support by target](@/docs/spec-format/skills-reference.md#disable-model-invocation-support-by-target). |
| `model` | unset | Claude Code model for the rest of the turn. A value, a per-target map, or a [tier](@/docs/spec-format/agents.md#model-tiers) name. |
| `effort` | unset | Claude Code effort for the rest of the turn. A value or a per-target map. |
| `allowed-tools` | unset | Capabilities the skill may use, such as `[read, shell(git diff *)]`. See [allowed tools](#allowed-tools). |
| `license` | unset | The Agent Skills license, kept in every tool's `SKILL.md`. |
| `workspaces` | empty | Project directories where Cursor also gets a copy, such as `[apps/web]`. |

All fields are optional. A skill's scope comes from its folder: `skills/services/api/review/SKILL.md` puts the skill under `services/api/`, where only sessions in that directory load it.

{% <details summary="Details: overrides, Cursor workspaces, scope and sync notes"> %}
- `argument-hint`: other tools leave it out unless you set it under their `x-<target>` block.
- `model` and `effort`: `x-claude.model` and `x-claude.effort` win.
- `workspaces`: Cursor loads skills only from the workspace it opens, so a session or SDK agent started in `apps/web` misses a root skill. The skill stays at the root for every tool.
- `import cursor` writes `workspaces` when a root `.cursor/skills/<name>` links to a skill folder under a project directory.
- `scope:` in the frontmatter has no effect, and `lint` warns about it (LINT018).
- Sync, including user-level sync, prints a note for each skill field a tool leaves out, such as `argument-hint`, `model`, or `effort`.
- A field kept in the tool's frontmatter, a command copy of the skill, or a policy file gets no note.
- `agnostic-ai compare claude codex` shows which fields each tool keeps, translates, or drops.
- Shared user directories leave out per-target overrides.
{% </details> %}

## Allowed tools {#allowed-tools}

Write `allowed-tools` with the same [capabilities](@/docs/spec-format/agents.md#capabilities) as an agent's `can`:

```yaml
allowed-tools: [read(src/**), edit(src/**), shell(git diff *), mcp:github]
```

Unlike an agent's tool list, `allowed-tools` keeps paths and patterns, so `shell(git diff *)` pre-approves only those commands. Each tool gets its own names where it supports this field.

{% <details summary="Details: unsupported tools, aliases and conversion"> %}
- A tool that drops a restriction or grants more access raises a sync note. `on-unsupported: error` fails when a tool would grant more access.
- Claude Code names, scoped ones such as `Read(src/**)` and `Bash(git diff *)` included, work as aliases, and a list can mix both.
- `migrate --only capabilities`, `import`, and `lint --suggest-capabilities` (LINT037) convert names as they do for [agents](@/docs/spec-format/agents.md#capabilities).
- Neither `migrate` nor `import` guesses a narrower restriction from a tool's bundle of tools.
{% </details> %}

## Bundled files and output

Only `SKILL.md` and flat `skills/*.md` files are read as skills. Every other file in a nested skill directory is a bundled file (scripts, templates, fixtures, extra `*.md`). It is copied unchanged, with its executable bit, to the same relative path under each tool's skills directory.

A body can point at another skill with [`{{$SKILLS_DIR}}`](@/docs/spec-format/shared-features.md#path-variables-name), which becomes each tool's own skills directory.

| Tool | Gets |
|------|------|
| Most tools | `<dir>/<name>/SKILL.md` with the bundled files. Each target page gives the directory. |
| Several tools | A shared `.agents/skills/`, with identical files written once |
| No skill support | A `skill-<name>.md` rule. Bundled files cannot follow, so sync notes it. Set `outputs.<target>.emit-skills-as-commands: true` to also write a slash command. |

## Claude Code body syntax {#claude-code-body-syntax}

Claude Code replaces some syntax in a skill body before the model reads it ([skills docs](https://code.claude.com/docs/en/skills)):

| Syntax | Becomes |
|--------|---------|
| `` !`command` `` line, ` ```! ` block | The command's output |
| `$ARGUMENTS` | The text typed after the skill name |
| `$0`, `$1`, ... and `$ARGUMENTS[N]` | One argument each |

Other targets do not expand this syntax in project skills. Sync copies the body as written and prints one note per tool and syntax, naming each line:

```
  note: `!`command`` on 1 skill has no effect on codex (the command does not run at .agnostic-ai/skills/pr/SKILL.md:8; put the line in a ::target claude fence)
```

{% <details summary="Details: note options, lint code and user-level sync"> %}
- `on-unsupported: error` fails the sync instead of printing the note. `silent` hides it.
- `lint` reports each line as LINT019.
- `sync --scope user` checks the user-level destination and reads `on-unsupported` from the `agnostic-ai.yaml` in the source root.
- In user-level sync, Kilo's [inline shell placeholders](@/docs/targets/kilo-reference.md#shell-commands-in-global-skills) in trusted user skill directories pass. Argument placeholders and Claude dynamic code blocks still raise notes.

{% </details> %}

Put the Claude line in a fence and give other tools their own text:

```markdown
::target claude
!`git log main..HEAD --oneline`
::end
::target codex
Run `git log main..HEAD --oneline` first. `$ARGUMENTS` below means the text passed after the skill name.
::end
```

{% <details summary="Details: fenced examples, escaping and import"> %}
- A `$1` or `` !`command` `` inside a fenced code block counts as an example and is not reported. `$ARGUMENTS` is still reported there.
- Escape a literal dollar as `\$1`, as Claude Code expects.
- `import claude` keeps the body as written, so Claude Code keeps its dynamic context.

{% </details> %}

[Commands](@/docs/spec-format/commands.md#claude-code-body-syntax) use the same syntax, and a few tools support part of it there.

How each tool handles `disable-model-invocation` is in the [skill reference](@/docs/spec-format/skills-reference.md#disable-model-invocation-support-by-target).
