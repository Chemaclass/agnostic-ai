+++
title = "Commands"
description = "commands/: slash commands a person runs by name, written once for every tool's command picker."
weight = 40

[extra]
group = "Reference"
+++

# Commands

`commands/` holds slash commands: saved prompts a person starts by name, such as `/deploy` or `/review-pr`. The body is the prompt the tool sends when someone runs it.

- **Repeatable prompts.** The team runs the same `/review-pr` instead of retyping a slightly different version each time.
- **Started by a person.** A command runs when someone types it, which suits steps with side effects, such as a deploy.
- **Same name everywhere.** Each tool that supports commands lists it in its own picker.

Use a [skill](@/docs/spec-format/skills.md) instead when the model should pick the workflow itself, or when it needs bundled files. Set `outputs.<target>.emit-skills-as-commands: true` to get both from one skill.

## Write one

Markdown with optional YAML frontmatter. Each spec becomes one slash command in the tool's own format.

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
| `argument-hint` | no | empty | Hint shown after the command in Claude Code, Augment, and Factory. |

Other frontmatter is kept. Put keys for one tool under `x-<target>`, for example `x-claude.allowed-tools`:

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

- Codex gets commands only when `outputs.codex.commands-dir` is set.
- Gemini writes each command as a `.toml` file named after `name`.
- Tools without command support log a warning and skip them.

Each target page lists the exact directory.

## Claude Code body syntax {#claude-code-body-syntax}

A Claude Code command body can use `` !`command` `` lines, ` ```! ` blocks, `$ARGUMENTS`, and `$1`, `$2`, ... ([skill syntax](@/docs/spec-format/skills.md#claude-code-body-syntax)). Sync copies the body as written. Each tool supports only what its docs show:

| Target | `` !`command` `` | `$ARGUMENTS` | `$1`, `$2`, ... |
|--------|------------------|--------------|-----------------|
| [Claude Code](@/docs/targets/claude.md) | yes | yes | yes |
| [OpenCode](@/docs/targets/opencode.md) | yes | yes | yes |
| [Codex](@/docs/targets/codex.md) | no | yes | yes |
| [Augment](@/docs/targets/augment.md), [Factory](@/docs/targets/factory.md) | no | yes | no |
| [Kiro CLI V3](@/docs/targets/kiro.md#commands) | no | yes | no |
| Cursor, Gemini, Junie, Kilo, Qoder, Trae | no | no | no |

A tool that reads the syntax as plain text gets a note naming each line. `on-unsupported: error` fails the sync, and `lint` reports LINT019. Gemini has its own argument and shell placeholders ([custom commands](https://geminicli.com/docs/cli/custom-commands)). Write them in a `::target gemini` fence.

Kiro uses `${1}` through `${10}` and `${@}` for its own arguments. Put Kiro-specific templates in a `::target kiro` fence. Sync keeps them as written and does not map a bare `$1`.
