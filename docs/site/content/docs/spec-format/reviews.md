+++
title = "Reviews"
description = "reviews/: project guidance for code-review bots, for the whole repository or one directory."
weight = 80

[extra]
group = "Reference"
+++

# Reviews

`reviews/` tells code-review bots what to check in this project: the layering rule, the error handling, the migrations that need a second look. Each review bot reads its guidance from its own files. A review spec feeds all of them from one source.

| Target | Reads |
|--------|-------|
| [Cursor](@/docs/targets/cursor.md) (Bugbot) | `.cursor/BUGBOT.md`, and `<scope>/.cursor/BUGBOT.md` for scoped specs |
| [Codex](@/docs/targets/codex.md) (code review) | a `## Code Review Rules` section of the root or scoped `AGENTS.md` |
| [Goose](@/docs/targets/goose.md) (`goose review`) | `.agents/REVIEW.md`, and `<scope>/.agents/REVIEW.md` for scoped specs |
| [Ellipsis](@/docs/targets/ellipsis.md) (code review) | one reviewer's prompt in `.ellipsis/code-review.yaml`; scoped specs become sections that apply only to changes in their directory |

Other tools report review specs as unsupported.

Ellipsis also needs the reviewer, its model, and any review posting rules from a [settings spec](@/docs/spec-format/settings.md) under `x-ellipsis`. Without a reviewer, sync writes no file and says what is missing. See [Ellipsis](@/docs/targets/ellipsis.md).

## Write one

```bash
agnostic-ai new review code-quality
```

Replace the description and body with the checks your review bot should apply.

Save Markdown with optional YAML settings as `.agnostic-ai/reviews/code-quality.md`. Sync, check the output, then start a review in a supported tool. These specs supply guidance; they do not install or start a review bot.

```markdown
---
scope: backend
---

Flag any handler that talks to the database directly instead of going through a repository.
```

Reviews follow `scope` and the folder layout like [rules](@/docs/spec-format/rules.md) do. Specs with the same scope are joined into one review file, written as plain text without frontmatter. For Codex, the section sits in `AGENTS.md`, so every tool that reads `AGENTS.md` loads it. A `targets:` filter that leaves out `codex` keeps a spec out of it.

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

- A missing file, an absolute path, or a path outside the project fails the load ([AAI-001](@/docs/errors.md#aai-001-spec-parse-failed)).
- A line inside a fenced code block stays as written.
- An included file is not searched for more `@path` lines.
- Only reviews take `@path` lines. On `AGNOSTIC_AI.md` the [`resolve-imports`](@/docs/configuration.md#syncresolve-imports) setting controls them.
