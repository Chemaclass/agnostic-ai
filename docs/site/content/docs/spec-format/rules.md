+++
title = "Rules"
description = "rules/: conventions the agent follows every session, for the whole project, one directory, or matching files."
weight = 30

[extra]
group = "Reference"
+++

# Rules

`rules/` holds the conventions an agent must follow without being asked: commit format, error handling, the money type, the test style. Sync writes each rule in every tool's own format: Cursor `.mdc` files, Claude Code rules, `AGENTS.md` sections.

Keep a rule short and write it as an instruction. Put a multi-step procedure in a [skill](@/docs/spec-format/skills.md), which loads only when needed.

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

A rule that loads when the agent works on matching files:

```markdown
---
name: react-components
description: Conventions for React components.
globs: "src/**/*.{ts,tsx}"
---

Write function components. Keep one exported component per file.
```

A rule for one directory. Create it with `agnostic-ai new rule payments-context --scope services/payments`, or place the file at `rules/services/payments/payments-context.md`:

```markdown
---
name: payments-context
scope: services/payments
---

Use integer minor units for monetary values.
```

Claude Code gets a rule that applies only there. Codex and Cursor share `services/payments/AGENTS.md`. Gemini gets `services/payments/GEMINI.md`. [Directory-specific instructions](@/docs/scoped-context.md) lists every tool and the selector limits.

## Headings in merged files

When several rules share a file, such as Codex's `AGENTS.md` or Gemini's `GEMINI.md`, each rule gets a `### <name>` section. Sync moves all the headings in the rule body down together, so its top heading is at least `####`. A body with `### Doc versioning` and `#### Details` becomes `#### Doc versioning` and `##### Details`.

- The `outputs.claude.rules-file` layout writes `## <name>` sections, so there the top body heading is at least `###`.
- Headings that are already deep enough keep their levels.
- Markdown has six heading levels, so no heading goes past `######`.
- When a body's deepest heading is more than two levels below its top heading (three in the Claude layout), the deepest levels merge at `######`. Import cannot separate them again.

The section's source comment records the shift, as in `<!-- source: .agnostic-ai/rules/content.md headings: +1 -->`. Import moves the headings back, so a rule comes through sync and import unchanged.

<details>
<summary>Edge cases for heading shifts</summary>

- Fenced code (including fences inside lists) and raw HTML blocks keep their headings as written.
- Link reference definitions stay separate from underlined (Setext) headings.
- List items followed by a thematic break remain lists.
- Raw HTML blocks follow CommonMark. A line such as `<b>Note:</b> read first` is paragraph text, so a `---` under it forms a heading that nests too.
- A rule file written on its own keeps its heading levels. So does a nested file with one rule and no section wrapper.
- An underlined (Setext) heading comes back from import as a `#` heading. Hard line breaks in its text become `<br>` tags.
- Headings and source-comment examples inside fenced code and raw HTML stay in their rule body. They do not start another rule.
- When a rule leaves a code or HTML block open, its source comment also records `body-lines: <count>`. Import uses that count to find the next section. Sync updates the count from the spec.

</details>

## Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Rule identifier. |
| `description` | no | empty | Short summary. |
| `scope` | no | project-wide | A project-relative directory and everything under it. It wins over the rule's folder. A folder that names a project directory scopes a rule without `scope`; any other folder only groups rules. A scope inside `node_modules` is refused. See [scoped context](@/docs/scoped-context.md). |
| `globs` | no | depends on the tool; `new rule` seeds `**/*` | Project-relative patterns, as a comma-separated string (`"*.go,*.mod"`) or a list. A comma inside a brace set does not separate patterns, so `"src/**/*.{ts,tsx}"` is one pattern. With `scope`, applies to the whole scope directory and every matching file. `new rule --scope` omits it. |
| `paths` | no | unset | Another name for `globs`, the form Claude Code writes. Every tool reads it as `globs`. Set only one of them; `validate` rejects a rule whose `paths` and `globs` name different patterns. See [selector limits](@/docs/scoped-context.md#narrow-a-rule-to-certain-files). |
| `alwaysApply` | no | `false` with `globs`, else depends on the tool; `new rule` seeds `true` | Asks the tool to always load the rule. Unset on a rule with `globs`, it is `false` in every tool that has an activation field, so the rule loads only on matching files. A catch-all such as `**/*` keeps the rule always-on. With `scope`, the rule stays within the scope plus patterns. `new rule --scope` omits it. |

A rule with neither `globs` nor `alwaysApply` loads differently per tool; each target page says when. Set `alwaysApply: true` when a rule must load everywhere.
