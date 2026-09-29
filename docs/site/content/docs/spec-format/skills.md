+++
title = "Skills"
description = "skills/: procedures the agent loads when a task needs them, with their scripts and templates."
weight = 20

[extra]
group = "Reference"
+++

# Skills

`skills/` holds procedures the agent loads only when a task calls for them: cut a release, write a migration, triage a bug report. The tool keeps each skill's `description` in view and reads the body, plus any bundled files, when the skill applies.

- **Small standing context.** A long procedure costs nothing until it is used, unlike a rule, which loads every session.
- **Files that travel with it.** Scripts, templates, and fixtures sit next to `SKILL.md` and land beside it in every tool.
- **Invoked by the model or by name.** The model picks a skill from its description; a person can also call it directly.
- **One format across tools.** Targets that read the `SKILL.md` layout get the skill as written; targets without a skill surface get a rule and a coverage note.

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

A manual-only release skill that runs on a stronger model in Claude Code and ships its own script:

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
| `name` | no | dir or filename | Skill identifier and output directory. Some targets restrict the format. |
| `description` | no | empty | One-liner the model uses to decide whether to invoke the skill. |
| `disable-model-invocation` | no | unset | `true` keeps the skill out of automatic invocation; a person can still call it. See [support by target](#disable-model-invocation-support-by-target). |
| `model` | no | unset | Claude Code model for the rest of the turn. Scalar or per-target map; `x-claude.model` wins. |
| `effort` | no | unset | Claude Code effort for the rest of the turn. Scalar or per-target map; `x-claude.effort` wins. |
| `license` | no | unset | The Agent Skills license, kept in every target's `SKILL.md`. |
| `workspaces` | no | empty | Project directories where Cursor also gets a copy, such as `[apps/web]`. Cursor loads skills only from the workspace it opens, so a session or SDK agent started in `apps/web` misses a root skill. The skill stays at the root for every tool. |

A skill's scope comes from its folder: `skills/services/api/review/SKILL.md` moves the skill under `services/api/`, where only sessions in that directory load it. `scope:` in the frontmatter has no effect, and `lint` warns about it (LINT018). `import cursor` writes `workspaces` when a root `.cursor/skills/<name>` links to a skill folder under a project directory.

Other targets omit skill `model` and `effort` and report a coverage note when a value resolves for them. Use `{claude: opus}` to choose a model only for Claude. Global sync uses the same renderers; shared global directories omit target overrides.

## Bundled files and output

Only `SKILL.md` and flat `skills/*.md` parse as skills. Every other file in a nested skill directory is a bundled asset (scripts, templates, fixtures, extra `*.md`). Assets copy verbatim to the same relative path under each target's skills dir. Import and sync preserve executable bits.

Most targets write `<dir>/<name>/SKILL.md` with assets. Several share `.agents/skills/`, so identical bytes write once. Targets with no skill surface flatten it to a `skill-<name>.md` rule and raise a coverage note, since assets cannot follow. Set `outputs.<target>.emit-skills-as-commands: true` to also emit a slash command. Each target page gives the exact directory.

A body can point at another skill with [`{{$SKILLS_DIR}}`](@/docs/spec-format/_index.md#path-variables-name), which resolves to each target's own skills directory.

## `disable-model-invocation` support by target {#disable-model-invocation-support-by-target}

Only the targets listed were checked. Setting it keeps a skill out of automatic model invocation; the user can still invoke it. Omitting it leaves each target's default, which is model-invocable everywhere below.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Cursor](@/docs/targets/cursor.md) | Written to `SKILL.md` |
| [Codex](@/docs/targets/codex.md) | Written as `allow_implicit_invocation: false` to `agents/openai.yaml`. An explicit value in `x-codex.policy` or a bundled `agents/openai.yaml` wins |
| [Crush](@/docs/targets/crush.md) | Dropped with a note. Set `x-crush.disable-model-invocation` |
| [Factory](@/docs/targets/factory.md) | Dropped with a note. Set `x-factory.disable-model-invocation` |

Crush and Factory skills land in the shared `.agents/skills/` tree, so emitting the key would hand it to targets with no such field. Use the `x-` key: a manual-only skill turning model-invocable is a safety boundary.

OpenHands' `triggers` is unrelated: it injects a skill on a keyword. Devin spells this restriction `triggers: [user]`.
