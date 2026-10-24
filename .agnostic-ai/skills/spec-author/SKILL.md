---
name: spec-author
description: Author an agnostic-ai agent, skill, rule, or hook from the user's requested behavior.
---

# spec-author

Create or update the smallest spec that expresses the requested behavior. Infer the kind from the request and existing specs; ask only when the choice changes the outcome. Reuse a matching spec instead of creating a duplicate.

## Steps

1. Read `agnostic-ai.yaml` for configured source paths and inspect sibling specs. The default root is `.agnostic-ai/`.
2. Read the matching page under `docs/site/content/docs/spec-format/` for fields and native limits. For a new spec, use `go run ./cmd/agnostic-ai new <kind> <name> --dry-run` to inspect the scaffold, then create it. Never overwrite an existing spec with a scaffold.
3. Use `agents/<name>.md`, `rules/<name>.md`, or `hooks/<name>.yaml` under the configured root. Skills can use `<name>.md` or `<name>/SKILL.md`; use the folder layout when bundling references or scripts.
4. Write a short description that tells an agent when to use the spec. Keep the body focused on decisions and actions, with conditional details in existing references. Hooks need the documented event and exact `command`; test the emitted behavior in an isolated project.
5. Run `go run ./cmd/agnostic-ai validate` and `go run ./cmd/agnostic-ai lint`, then preview with `go run ./cmd/agnostic-ai sync --dry-run`. Check the intended targets and output paths.
6. Generate with `go run ./cmd/agnostic-ai sync`, then verify `go run ./cmd/agnostic-ai sync --check`. Review tracked output changes and keep ignored output ignored. Follow `docs/internal/contributing.md#choose-checks-for-your-change` for other checks the change requires.

Use lowercase, hyphenated names. An explicit `name` matches the Markdown file stem, or the containing folder for `SKILL.md`. Paths in shared instructions are repository-root relative; state that convention when generated copies live at different depths.
