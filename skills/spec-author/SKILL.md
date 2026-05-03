---
name: spec-author
description: Author a new agnostic-ai spec (agent, skill, rule, or hook). Use when the user wants to add a new prompt, convention, or hook to their project.
---

# spec-author

Authors a new agnostic-ai spec file from a description.

## Steps

1. Ask the user which kind: agent, skill, rule, or hook.
2. Pick the right directory and extension:
   - agent: `agents/<name>.md`
   - skill: `skills/<name>/SKILL.md`
   - rule: `rules/<name>.md`
   - hook: `hooks/<name>.yaml`
3. Build the YAML frontmatter (or full YAML for hooks) per `docs/user/spec-format.md`.
4. Write the body in plain English. Lead with the action.
5. Run `agnostic-ai validate` to confirm parse.
6. Run `agnostic-ai sync --dry-run` to preview emitted outputs.

## Conventions

- `name` field matches the file name without extension.
- `description` is one short sentence.
- Body is markdown. Avoid em dashes. Avoid filler.
- Hooks include a `command` field with the exact shell to run.
