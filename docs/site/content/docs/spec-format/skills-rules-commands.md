+++
title = "Skills, rules, and commands"
description = "Write skills, rules, and commands, and how each target reads them."
weight = 20

[extra]
group = "Reference"
+++

# Skills, rules, and commands

## Skills

Two layouts:

- **Flat:** `skills/yaml-validator.md`
- **Nested**, for skills with attached resources: `skills/yaml-validator/SKILL.md` next to `skills/yaml-validator/schema.yaml`

```markdown
---
name: yaml-validator
description: Validate YAML against a schema.
---

# YAML Validator

1. Read target file
2. Parse YAML and compare against `schema.yaml`
3. Report violations as `path: message`
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | dir or filename | Skill identifier and output directory. Some targets restrict the format. |
| `description` | no | empty | One-liner the model uses to decide whether to invoke the skill. |
| `model` | no | unset | Claude Code model for the rest of the turn. Scalar or per-target map; `x-claude.model` wins. |
| `effort` | no | unset | Claude Code effort for the rest of the turn. Scalar or per-target map; `x-claude.effort` wins. |
| `license` | no | unset | The Agent Skills license, kept in every target's `SKILL.md`. |
| `workspaces` | no | empty | Project directories where Cursor also gets a copy, such as `[apps/platforma]`. Cursor loads skills only from the workspace it opens, so a session or SDK agent started in `apps/platforma` misses a root skill. The skill stays at the root for every tool. |

A skill's scope comes from its folder: `skills/services/api/review/SKILL.md` moves the skill under `services/api/`, where only sessions in that directory load it. `scope:` in the frontmatter has no effect, and `lint` warns about it (LINT018). `import cursor` writes `workspaces` when a root `.cursor/skills/<name>` links to a skill folder under a project directory.

Other targets omit skill `model` and `effort` and report a coverage note when a value resolves for them. Use `{claude: opus}` to choose a model only for Claude. Global sync uses the same renderers; shared global directories omit target overrides.

Only `SKILL.md` and flat `skills/*.md` parse as skills. Every other file in a nested skill directory is a bundled asset (scripts, templates, fixtures, extra `*.md`). Assets copy verbatim to the same relative path under each target's skills dir. Import and sync preserve executable bits.

Most targets write `<dir>/<name>/SKILL.md` with assets. Several share `.agents/skills/`, so identical bytes write once. Targets with no skill surface flatten it to a `skill-<name>.md` rule and raise a coverage note, since assets cannot follow. Set `outputs.<target>.emit-skills-as-commands: true` to also emit a slash command. Each target page gives the exact directory.

### `disable-model-invocation` support by target {#disable-model-invocation-support-by-target}

Only the targets listed were checked. Setting it keeps a skill out of automatic model invocation; the user can still invoke it. Omitting it leaves each target's default, which is model-invocable everywhere below.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Cursor](@/docs/targets/cursor.md) | Written to `SKILL.md` |
| [Codex](@/docs/targets/codex.md) | Written as `allow_implicit_invocation: false` to `agents/openai.yaml`. An explicit value in `x-codex.policy` or a bundled `agents/openai.yaml` wins |
| [Crush](@/docs/targets/crush.md) | Dropped with a note. Set `x-crush.disable-model-invocation` |
| [Factory](@/docs/targets/factory.md) | Dropped with a note. Set `x-factory.disable-model-invocation` |

Crush and Factory skills land in the shared `.agents/skills/` tree, so emitting the key would hand it to targets with no such field. Use the `x-` key: a manual-only skill turning model-invocable is a safety boundary.

OpenHands' `triggers` is unrelated: it injects a skill on a keyword. Devin spells this restriction `triggers: [user]`.

## Rules

```markdown
---
name: conventional-commits
description: Always use Conventional Commits format.
globs: "**/*"
alwaysApply: true
---

Use `feat:`, `fix:`, `docs:`, etc. Subject under 72 chars.
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Rule identifier. |
| `description` | no | empty | Short summary. |
| `scope` | no | project-wide | Project-relative directory and its descendants. Source-layout scope wins. A scope inside `node_modules` is refused. See [scoped context](@/docs/scoped-context.md). |
| `globs` | no | target-dependent; `new rule` seeds `**/*` | Project-relative patterns, as a comma-separated string (`"*.go,*.mod"`) or a list. A comma inside a brace set does not separate patterns, so `"src/**/*.{ts,tsx}"` is one pattern. With `scope`, the selector must stay inside the directory. `new rule --scope` omits it. |
| `paths` | no | unset | File patterns, as a string or list. Scoped rules accept it with or instead of `globs`; see [selector limits](@/docs/scoped-context.md#narrow-a-rule-to-certain-files). |
| `alwaysApply` | no | target-dependent; `new rule` seeds `true` | Requests unconditional activation. With `scope`, only inside the directory. `new rule --scope` omits it. |

## Commands

Markdown with optional YAML frontmatter. Each spec becomes one native slash command.

```markdown
---
name: deploy
description: Deploy the app to staging.
argument-hint: <env>
---

Deploy the app to {{env}}.
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Command identifier and slash name, such as `/deploy`. |
| `description` | no | empty | One-liner shown in slash-command pickers. |
| `argument-hint` | no | empty | Hint shown after the command, on Claude Code, Augment, and Factory. |

Any other frontmatter passes through. Put target-specific keys under `x-<target>`, for example `x-claude.allowed-tools`. Codex emits commands only when `outputs.codex.commands-dir` is set. Targets without a command surface log a warning and skip.

