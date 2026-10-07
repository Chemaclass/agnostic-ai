+++
title = "Skills"
description = "skills/: procedures the agent loads when a task needs them, with their scripts and templates."
weight = 20

[extra]
group = "Reference"
+++

# Skills

`skills/` holds procedures the agent loads only when a task needs them: cut a release, write a migration, triage a bug report. The tool keeps each skill's `description` in view. It reads the body and any bundled files only when the skill applies.

- **Small standing context.** A long procedure costs nothing until it is used. A rule, by contrast, loads every session.
- **Files that travel with it.** Scripts, templates, and fixtures sit next to `SKILL.md` and are copied beside it for every tool.
- **Used by the model or by name.** The model picks a skill from its description. A person can also call it directly.
- **One format across tools.** Tools that read the `SKILL.md` layout get the skill as written. Tools without skill support get a rule and a coverage note.

Use a [rule](@/docs/spec-format/rules.md) for what applies to every task, and a skill for a procedure some tasks need.

## Write one

`agnostic-ai new skill release-notes` scaffolds one. Two layouts:

- **Flat:** `skills/yaml-validator.md`
- **Nested**, for skills with attached files: `skills/yaml-validator/SKILL.md` next to `skills/yaml-validator/schema.yaml`

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

Write `description` as the trigger: what the skill does and when to use it. The model decides from that line alone.

This manual-only release skill runs on a stronger model in Claude Code and ships its own script:

```
skills/cut-release/
├── SKILL.md
└── scripts/bump-version.sh
```

```markdown
---
name: cut-release
description: Cut a release. Run only when the user asks to release or tag.
disable-model-invocation: true
model: {claude: opus}
---

1. Run `make ci-local` and stop on any failure.
2. Run `scripts/bump-version.sh <version>` from this skill's folder.
3. Commit `chore(release): v<version>` and tag it.
```

## Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | dir or filename | Skill name and output directory. Some tools restrict the format. |
| `description` | no | empty | One line the model uses to decide whether to use the skill. |
| `argument-hint` | no | unset | Hint for the arguments to a Claude Code slash command. Other tools leave it out unless you set it under their `x-<target>` block. |
| `disable-model-invocation` | no | unset | `true` stops the model from using the skill on its own. A person can still call it. See [support by target](#disable-model-invocation-support-by-target). |
| `model` | no | unset | Claude Code model for the rest of the turn. Scalar, per-target map, or [tier](@/docs/spec-format/agents.md#model-tiers) name; `x-claude.model` wins. |
| `effort` | no | unset | Claude Code effort for the rest of the turn. Scalar or per-target map; `x-claude.effort` wins. |
| `allowed-tools` | no | unset | Tool-neutral capabilities the skill may use, such as `[read, shell(git diff *)]`. See [allowed tools](#allowed-tools). |
| `license` | no | unset | The Agent Skills license, kept in every tool's `SKILL.md`. |
| `workspaces` | no | empty | Project directories where Cursor also gets a copy, such as `[apps/web]`. Cursor loads skills only from the workspace it opens, so a session or SDK agent started in `apps/web` misses a root skill. The skill stays at the root for every tool. |

A skill's scope comes from its folder. `skills/services/api/review/SKILL.md` puts the skill under `services/api/`, where only sessions in that directory load it. `scope:` in the frontmatter has no effect, and `lint` warns about it (LINT018). `import cursor` writes `workspaces` when a root `.cursor/skills/<name>` links to a skill folder under a project directory.

Sync reports a coverage note for each skill field a tool leaves out, including `argument-hint`, `model`, and `effort`.

- A field that is kept in the tool's own frontmatter, in a command copy of the skill, or in a policy file gets no note.
- `agnostic-ai compare claude codex` shows which skill fields each tool keeps, translates, or drops.
- Use `{claude: opus}` to choose a model only for Claude.
- Global sync works the same way. Shared global directories leave out per-target overrides.

## Allowed tools {#allowed-tools}

Write `allowed-tools` with the same [capabilities](@/docs/spec-format/agents.md#capabilities) as an agent's `can`:

```yaml
allowed-tools: [read(src/**), edit(src/**), shell(git diff *), mcp:github]
```

The names are `read`, `write`, `edit`, `delete`, `shell`, `web`, `mcp:<server>`, and `mcp:<server>/<tool>`. `read` and `edit` take path patterns. `shell` takes a command pattern. Each tool gets its own names where it supports this field. A tool that drops a restriction or grants more access raises a coverage note. `on-unsupported: error` fails when a tool would grant more access.

Claude Code names also work: `Read`, `Write`, `Edit`, `Bash`, `WebFetch`, `WebSearch`, and `mcp__<server>__<tool>`. So do scoped names such as `Read(src/**)` and `Bash(git diff *)`. A list can mix neutral and Claude Code names.

`agnostic-ai migrate --only capabilities` rewrites Claude Code names that map exactly, including an adjacent `WebFetch, WebSearch` pair as `web`. `import` writes neutral names where the tool's own names map one to one. Neither guesses a narrower restriction from a tool's bundle of tools. `lint --suggest-capabilities` suggests neutral names for Claude Code names (LINT037). It suggests nothing by default.

## Bundled files and output

Only `SKILL.md` and flat `skills/*.md` files are read as skills. Every other file in a nested skill directory is a bundled file (scripts, templates, fixtures, extra `*.md`). Bundled files are copied unchanged to the same relative path under each tool's skills directory. Import and sync keep executable bits.

Most tools get `<dir>/<name>/SKILL.md` with the bundled files. Several share `.agents/skills/`, so identical files are written once. Tools without skill support get the skill as a `skill-<name>.md` rule. Bundled files cannot follow, so they raise a coverage note. Set `outputs.<target>.emit-skills-as-commands: true` to also write a slash command. Each target page gives the exact directory.

A body can point at another skill with [`{{$SKILLS_DIR}}`](@/docs/spec-format/_index.md#path-variables-name), which becomes each tool's own skills directory.

## Claude Code body syntax {#claude-code-body-syntax}

Claude Code replaces some syntax in a skill body before the model reads it ([skills docs](https://code.claude.com/docs/en/skills)):

- `` !`command` `` lines and ` ```! ` blocks run the command and insert its output.
- `$ARGUMENTS` becomes the text typed after the skill name.
- `$0`, `$1`, ... and `$ARGUMENTS[N]` become one argument each.

No other tool documents this syntax for skills, so each one reads it as plain text. Sync copies the body as written. It prints one note per tool and syntax, naming each line:

```
  note: `!`command`` on 1 skill has no effect on codex (the command does not run at .agnostic-ai/skills/pr/SKILL.md:8; put the line in a ::target claude fence)
```

`on-unsupported: error` fails the sync instead, and `silent` hides the note. `lint` reports each line as LINT019. `sync --global` prints the same notes for the skills it writes to user-level directories. It reads `on-unsupported` from the `agnostic-ai.yaml` in the source root.

Put the Claude line in a fence and give other tools their own text:

```markdown
::target claude
!`git log main..HEAD --oneline`
::end
::target codex
Run `git log main..HEAD --oneline` first. `$ARGUMENTS` below means the text passed after the skill name.
::end
```

A `$1` or `` !`command` `` inside a fenced code block counts as an example and is not reported. `$ARGUMENTS` is still reported there. Escape a literal dollar as `\$1`, as Claude Code expects. `import claude` keeps the body as written, so Claude Code keeps its dynamic context.

[Commands](@/docs/spec-format/commands.md#claude-code-body-syntax) use the same syntax, and a few tools support part of it there.

## `disable-model-invocation` support by target {#disable-model-invocation-support-by-target}

Only the tools listed were checked. Setting it stops the model from using a skill on its own. The user can still call it. Without it, each tool uses its default, which lets the model use the skill, in every tool below.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Cursor](@/docs/targets/cursor.md) | Written to `SKILL.md` |
| [Codex](@/docs/targets/codex.md) | Written as `allow_implicit_invocation: false` to `agents/openai.yaml`. An explicit value in `x-codex.policy` or a bundled `agents/openai.yaml` wins |
| [Crush](@/docs/targets/crush.md) | Dropped with a note. Set `x-crush.disable-model-invocation` |
| [Factory](@/docs/targets/factory.md) | Dropped with a note. Set `x-factory.disable-model-invocation` |

Crush and Factory skills go in the shared `.agents/skills/` folder, so writing the key there would hand it to tools with no such field. Use the `x-` key. A manual-only skill that becomes usable by the model crosses a safety boundary.

OpenHands' `triggers` is unrelated: it loads a skill when a keyword appears. Devin spells this restriction `triggers: [user]`.
