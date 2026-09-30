+++
title = "Rules"
description = "rules/: conventions the agent follows every session, for the whole project, one directory, or matching files."
weight = 30

[extra]
group = "Reference"
+++

# Rules

`rules/` holds the conventions an agent must follow without being asked: commit format, error handling, the money type, the test style. Each tool loads rules on its own terms (Cursor `.mdc` files, Claude Code rules, `AGENTS.md` sections); a rule spec is written once and lands in each.

- **Always on, or only where it matters.** A rule can apply to every session, to one directory, or to files that match a pattern, so a payments convention stays out of frontend work.
- **Close to the code.** A rule under `rules/services/payments/` applies inside `services/payments/`, and supported tools load it only there.
- **Checked like code.** `sync --check` fails when a tool's copy drifts from the spec, and `agnostic-ai explain --file <path>` lists which rules apply to a file.

Keep a rule short and stated as an instruction. Put a multi-step procedure in a [skill](@/docs/spec-format/skills.md) instead, so it loads only when needed.

## Write one

`agnostic-ai new rule conventional-commits` creates `rules/conventional-commits.md` with `globs: "**/*"` and `alwaysApply: true`.

```markdown
---
name: conventional-commits
description: Always use Conventional Commits format.
globs: "**/*"
alwaysApply: true
---

Use `feat:`, `fix:`, `docs:`, etc. Subject under 72 chars.
```

A rule for one file type, loaded when the agent works on matching files:

```markdown
---
name: react-components
description: Conventions for React components.
globs: "src/**/*.{ts,tsx}"
---

Write function components. Keep one exported component per file.
```

A rule for one directory. `agnostic-ai new rule payments-context --scope services/payments` creates it, or place the file at `rules/services/payments/payments-context.md`:

```markdown
---
name: payments-context
scope: services/payments
---

Use integer minor units for monetary values.
```

Claude Code gets a conditional rule. Codex and Cursor share `services/payments/AGENTS.md`. Gemini gets `services/payments/GEMINI.md`. See [directory-specific instructions](@/docs/scoped-context.md) for every target and the selector limits.

## Headings in merged files

When several rules share a document, such as Codex's `AGENTS.md` or Gemini's `GEMINI.md`, each rule gets a `### <name>` section. Sync shifts the rule body's heading levels together so its shallowest heading is at least `####`. A body with `### Doc versioning` and `#### Details` becomes `#### Doc versioning` and `##### Details`. Already nested headings keep their levels. Markdown has six heading levels, so deeper headings stop at `######`.

Fenced code keeps its headings as written. Standalone rule files and a nested document containing one rule without a section wrapper also keep the source heading levels.

## Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Rule identifier. |
| `description` | no | empty | Short summary. |
| `scope` | no | project-wide | Project-relative directory and its descendants. It wins over the rule's folder. A folder that names a project directory scopes a rule without `scope`; any other folder only groups rules. A scope inside `node_modules` is refused. See [scoped context](@/docs/scoped-context.md). |
| `globs` | no | target-dependent; `new rule` seeds `**/*` | Project-relative patterns, as a comma-separated string (`"*.go,*.mod"`) or a list. A comma inside a brace set does not separate patterns, so `"src/**/*.{ts,tsx}"` is one pattern. With `scope`, applies to the whole scope directory and every matching file. `new rule --scope` omits it. |
| `paths` | no | unset | File patterns, as a string or list. Adds matching files to the same union as `scope` and `globs`; see [selector limits](@/docs/scoped-context.md#narrow-a-rule-to-certain-files). |
| `alwaysApply` | no | target-dependent; `new rule` seeds `true` | Requests unconditional activation. With `scope`, stays within the scope and pattern union. `new rule --scope` omits it. |

Tools differ on what a rule with neither `globs` nor `alwaysApply` does; each target page says how it activates. Set `alwaysApply: true` when a rule must load everywhere.
