+++
title = "Reviews"
description = "reviews/: project guidance for code-review bots, for the whole repository or one directory."
weight = 80

[extra]
group = "Reference"
+++

# Reviews

`reviews/` defines project checks for code-review bots, such as dependency boundaries, error handling, and migration risks. One review spec writes that guidance into each bot's own file:

| Target | Reads |
|--------|-------|
| [Cursor](@/docs/targets/cursor.md) (Bugbot) | `.cursor/BUGBOT.md`, and `<scope>/.cursor/BUGBOT.md` for scoped specs |
| [Codex](@/docs/targets/codex.md) (code review) | a `## Code Review Rules` section of the root or scoped `AGENTS.md` |
| [Goose](@/docs/targets/goose.md) (`goose review`) | `.agents/REVIEW.md`, and `<scope>/.agents/REVIEW.md` for scoped specs |
| [Ellipsis](@/docs/targets/ellipsis.md) (code review) | one reviewer's prompt in `.ellipsis/code-review.yaml`; scoped specs become sections that apply only to changes in their directory |

Other tools report review specs as unsupported.

Ellipsis takes the reviewer, its model, and any posting rules from `x-ellipsis` in a [settings spec](@/docs/spec-format/settings.md). Without a reviewer, sync writes no file and says what is missing.

## Write one

```bash
agnostic-ai new review code-quality
```

It creates `.agnostic-ai/reviews/code-quality.md`. Write the checks the bot should apply, then sync and start a review. The spec supplies guidance only; it does not install or start a bot.

```markdown
---
scope: backend
---

Flag any handler that talks to the database directly instead of going through a repository.
```

Reviews follow `scope` and the folder layout like [rules](@/docs/spec-format/rules.md). Specs with the same scope are joined into one file, without frontmatter. The Codex section sits in `AGENTS.md`, so every tool that reads `AGENTS.md` loads it, unless a `targets:` filter leaves out `codex`.

## Include a file

A line holding only `@path` includes that file, read from the project root, so a folder's `README.md` can be its review:

```markdown
---
scope: services/billing
target: cursor
---

@services/billing/README.md
```

- Sync writes the file's text in place of the line, so a README change shows up in `sync --check`.
- A missing file, an absolute path, or a path outside the project fails the load ([AAI-001](@/docs/errors.md#aai-001-spec-parse-failed)).
- A line inside a fenced code block stays as written.
- An included file is not searched for more `@path` lines.
- Only reviews take `@path` lines. On `AGNOSTIC_AI.md` the [`resolve-imports`](@/docs/configuration.md#syncresolve-imports) setting controls them.
