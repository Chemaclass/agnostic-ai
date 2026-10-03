+++
title = "Reviews"
description = "reviews/: project guidance for code-review bots, for the whole repository or one directory."
weight = 80

[extra]
group = "Reference"
+++

# Reviews

`reviews/` tells code-review bots what to check in this project: the layering rule, the error handling, the migrations that need a second look. Each review bot reads guidance from its own files. A review spec feeds all of them from one source.

- **Project rules in every review.** The bot flags what your team cares about, not only generic issues.
- **Scoped guidance.** A review under `reviews/services/api/` applies only to changes in that directory.
- **No copies.** An `@path` line pulls a folder's `README.md` into its review, so the docs and the review stay one text.

| Target | Reads |
|--------|-------|
| [Cursor](@/docs/targets/cursor.md) (Bugbot) | `.cursor/BUGBOT.md`, and `<scope>/.cursor/BUGBOT.md` for scoped specs |
| [Codex](@/docs/targets/codex.md) (code review) | a `## Code Review Rules` section of the root or scoped `AGENTS.md` |
| [Goose](@/docs/targets/goose.md) (`goose review`) | `.agents/REVIEW.md`, and `<scope>/.agents/REVIEW.md` for scoped specs |

Other targets report review specs as unsupported.

## Write one

Write Markdown with optional YAML frontmatter, one file per group of guidance.

```markdown
---
scope: backend
---

Flag any handler that talks to the database directly instead of going through a repository.
```

Reviews honor `scope` and the source layout like [rules](@/docs/spec-format/rules.md) do. Specs with the same scope join into one review file, written as a plain body without frontmatter. For Codex, the section sits in `AGENTS.md`, so every tool that reads `AGENTS.md` loads it. A `targets:` filter that omits `codex` keeps a spec out of it.

## Include a file

A line holding only `@path` includes that file, read from the project root. A folder's `README.md` can then serve as its review, with no symlink or copy:

```markdown
---
scope: services/billing
target: cursor
---

@services/billing/README.md
```

Sync writes the file's text in place of the line. A change to the README then shows up in `sync --check`.

- A missing file, an absolute path, or a path that leaves the project fails the load ([AAI-001](@/docs/errors.md#aai-001-spec-parse-failed)).
- A line inside a fenced code block stays as written.
- An included file is not searched for further includes.
- Only reviews take `@path` lines. On `AGNOSTIC_AI.md` the [`resolve-imports`](@/docs/configuration.md#syncresolve-imports) setting controls them.
