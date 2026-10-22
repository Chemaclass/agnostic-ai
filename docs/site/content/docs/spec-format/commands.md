+++
title = "Commands"
description = "commands/: slash commands a person runs by name, written once for every tool's command picker."
weight = 40

[extra]
group = "Reference"
+++

# Commands

`commands/` holds slash commands: saved prompts a person starts by name, such as `/deploy` or `/review-pr`. The body is the prompt the tool sends when someone runs it.

Use a [skill](@/docs/spec-format/skills.md) instead when the model should pick the workflow itself, or when it needs bundled files. Set `outputs.<target>.emit-skills-as-commands: true` to get both from one skill.

## Write one

```bash
agnostic-ai new command review-changes
```

It creates `.agnostic-ai/commands/review-changes.md`. Replace the description and prompt as below, run `agnostic-ai sync`, then type `/review-changes` in a supported tool.

```markdown
---
name: review-changes
description: Review the current branch against main.
---

Diff the branch against `main` and report bugs with `file:line`, most severe first.
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

- Gemini writes each command as a `.toml` file named after `name`.
- Codex without `outputs.codex.commands-dir`, and Copilot, get each command as a skill. See [tools without commands](#tools-without-commands).
- Other tools without command support log a note and skip them.

Each target page lists the exact directory.

## Tools without commands {#tools-without-commands}

A tool that has skills but no commands gets each command as a skill only the user starts, when the tool can keep the model from starting it. Today that is [Codex](@/docs/targets/codex.md) and [Copilot](@/docs/targets/copilot.md). Copilot qualifies only while no other tool writes its skills folder, since a shared `SKILL.md` cannot keep the field. The skill:

- is written at the top level of the skills folder, whatever folder the command sits in under `commands/`;
- sets `disable-model-invocation: true`, which Codex reads as `allow_implicit_invocation: false` in `agents/openai.yaml`, and Copilot keeps in `SKILL.md`;
- keeps `name`, `description`, the target fields, and `argument-hint` where the tool reads it, and notes any other field it leaves out;
- opens with one line asking the model to run it only when called, and to treat the text typed after its name as its arguments, since neither tool fills in `$ARGUMENTS` in a skill. Sync notes each `$ARGUMENTS` or `$1` line, and `on-unsupported: error` does not fail on them.

A skill spec with the same name wins, ignoring case, and so does a hand-written skill folder of that name in a skills folder the tool reads. Sync notes the skipped command. Another tool that reads the same skills folder, such as Gemini CLI reading `.agents/skills`, also sees the skill; sync notes that it lists the command twice, or that its model can start it. Cline, Windsurf, Amp, Zed, Warp, Antigravity, Continue, Goose, OpenHands, and Crush have no switch sync writes to keep a skill user-only, so they skip commands. `target-exclude` stops sync from writing a command for a tool, but a tool that reads another tool's skills folder still sees the skill written there, such as Gemini CLI reading the copy Codex writes to `.agents/skills`.

## Claude Code body syntax {#claude-code-body-syntax}

A Claude Code command body can use `` !`command` `` lines, ` ```! ` blocks, `$ARGUMENTS`, and `$1`, `$2`, ... ([skill syntax](@/docs/spec-format/skills.md#claude-code-body-syntax)). Sync copies the body as written. Support follows each tool's docs:

| Target | `` !`command` `` | `$ARGUMENTS` | `$1`, `$2`, ... |
|--------|------------------|--------------|-----------------|
| [Claude Code](@/docs/targets/claude.md) | yes | yes | yes |
| [OpenCode](@/docs/targets/opencode.md) | yes | yes | yes |
| [Codex](@/docs/targets/codex.md) | no | yes | yes |
| [Augment](@/docs/targets/augment.md), [Factory](@/docs/targets/factory.md) | no | yes | no |
| [Kiro CLI V3](@/docs/targets/kiro.md#commands) | no | yes | no |
| Cursor, Gemini, Junie, Kilo, Qoder, Trae | no | no | no |

A tool that reads the syntax as plain text gets a note naming each line. `on-unsupported: error` fails the sync, and `lint` reports LINT019. Gemini has its own argument and shell placeholders ([custom commands](https://geminicli.com/docs/cli/custom-commands)). Write them in a `::target gemini` fence.

Kiro uses `${1}` through `${10}` and `${@}` for its arguments. Put them in a `::target kiro` fence. Sync keeps them as written and does not map a bare `$1`.
