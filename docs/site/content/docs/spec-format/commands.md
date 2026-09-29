+++
title = "Commands"
description = "commands/: slash commands a person runs by name, written once for every tool's command picker."
weight = 40

[extra]
group = "Reference"
+++

# Commands

`commands/` holds slash commands: saved prompts a person starts by name, such as `/deploy` or `/review-pr`. The body is the prompt the tool sends when someone runs the command.

- **Repeatable prompts.** The team runs the same `/review-pr` instead of retyping it slightly differently each time.
- **Started by a person.** A command runs when someone types it, which suits steps with side effects, such as a deploy.
- **Same name everywhere.** Each tool with a command surface lists it in its own picker.

A [skill](@/docs/spec-format/skills.md) is the better fit when the model should pick the workflow itself or when it needs bundled files. Set `outputs.<target>.emit-skills-as-commands: true` to get both from one skill.

## Write one

Markdown with optional YAML frontmatter. Each spec becomes one native slash command.

```markdown
---
name: deploy
description: Deploy the app to staging.
argument-hint: <env>
---

Deploy the app to the environment the user names. Run the smoke tests afterwards and report the result.
```

## Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename | Command identifier and slash name, such as `/deploy`. |
| `description` | no | empty | One-liner shown in slash-command pickers. |
| `argument-hint` | no | empty | Hint shown after the command, on Claude Code, Augment, and Factory. |

Any other frontmatter passes through. Put target-specific keys under `x-<target>`, for example `x-claude.allowed-tools`:

```markdown
---
name: review-pr
description: Review the current branch against main.
x-claude:
  allowed-tools: [Bash(git diff:*), Read, Grep]
---

Diff the branch against `main` and list bugs with `file:line`, most severe first.
```

## Output

Codex emits commands only when `outputs.codex.commands-dir` is set. Gemini writes each command as a `.toml` file named after `name`. Targets without a command surface log a warning and skip. Each target page gives the exact directory.
