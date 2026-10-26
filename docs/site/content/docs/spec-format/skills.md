+++
title = "Skills"
description = "skills/: procedures the agent loads when a task needs them, with their scripts and templates."
weight = 20

[extra]
group = "Reference"
+++

# Skills

`skills/` holds procedures the agent loads only when a task needs them: cut a release, write a migration, investigate a bug report. The tool keeps each skill's `description` in view and reads the body and bundled files only when the skill applies. Use a [rule](@/docs/spec-format/rules.md) for what applies to every task.

## Write one

`agnostic-ai new skill yaml-validator` creates a starter under `.agnostic-ai/skills/`. Use a flat `skills/yaml-validator.md`, or a nested `skills/yaml-validator/SKILL.md` when the skill has files, such as `schema.yaml` beside it.

```markdown
---
name: yaml-validator
description: Validate YAML against a schema. Use when a YAML file changes or the user asks to check one.
---

# YAML Validator

1. Read target file
2. Parse YAML and compare against `schema.yaml`
3. Report violations as `path: message`
```

Run `agnostic-ai lint`, `agnostic-ai sync`, and `agnostic-ai sync --check`. Then ask Claude Code to use `/yaml-validator`, or Codex to use `$yaml-validator`, with a YAML file and its schema.

## Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | dir or filename | Skill name and output directory. Some tools restrict the format. |
| `description` | no | empty | One line the model uses to decide whether to use the skill. |
| `argument-hint` | no | unset | Hint for the arguments to a Claude Code slash command. Other tools leave it out unless you set it under their `x-<target>` block. |
| `disable-model-invocation` | no | unset | `true` stops the model from using the skill on its own. A person can still call it. See [support by target](#disable-model-invocation-support-by-target). |
| `model` | no | unset | Claude Code model for the rest of the turn. Single value, per-target map, or [tier](@/docs/spec-format/agents.md#model-tiers) name; `x-claude.model` wins. |
| `effort` | no | unset | Claude Code effort for the rest of the turn. Single value or per-target map; `x-claude.effort` wins. |
| `allowed-tools` | no | unset | Tool-neutral capabilities the skill may use, such as `[read, shell(git diff *)]`. See [allowed tools](#allowed-tools). |
| `license` | no | unset | The Agent Skills license, kept in every tool's `SKILL.md`. |
| `workspaces` | no | empty | Project directories where Cursor also gets a copy, such as `[apps/web]`. Cursor loads skills only from the workspace it opens, so a session or SDK agent started in `apps/web` misses a root skill. The skill stays at the root for every tool. |

A skill's scope comes from its folder. `skills/services/api/review/SKILL.md` puts the skill under `services/api/`, where only sessions in that directory load it. `scope:` in the frontmatter has no effect, and `lint` warns about it (LINT018). `import cursor` writes `workspaces` when a root `.cursor/skills/<name>` links to a skill folder under a project directory.

Sync, including global sync, prints a note for each skill field a tool leaves out, such as `argument-hint`, `model`, or `effort`; a field kept in the tool's frontmatter, a command copy of the skill, or a policy file gets none. `agnostic-ai compare claude codex` shows which fields each tool keeps, translates, or drops. Shared global directories leave out per-target overrides.

## Allowed tools {#allowed-tools}

Write `allowed-tools` with the same [capabilities](@/docs/spec-format/agents.md#capabilities) as an agent's `can`:

```yaml
allowed-tools: [read(src/**), edit(src/**), shell(git diff *), mcp:github]
```

Unlike an agent's tool list, `allowed-tools` keeps paths and patterns, so `shell(git diff *)` pre-approves only those commands. Each tool gets its own names where it supports this field. A tool that drops a restriction or grants more access raises a sync note, and `on-unsupported: error` fails when a tool would grant more access.

Claude Code names, scoped ones such as `Read(src/**)` and `Bash(git diff *)` included, work as aliases, and a list can mix both. `migrate --only capabilities`, `import`, and `lint --suggest-capabilities` (LINT037) convert names as they do for [agents](@/docs/spec-format/agents.md#capabilities). Neither `migrate` nor `import` guesses a narrower restriction from a tool's bundle of tools.

## Bundled files and output

Only `SKILL.md` and flat `skills/*.md` files are read as skills. Every other file in a nested skill directory is a bundled file (scripts, templates, fixtures, extra `*.md`), copied unchanged to the same relative path under each tool's skills directory. Import and sync keep executable bits.

Most tools get `<dir>/<name>/SKILL.md` with the bundled files. Several share `.agents/skills/`, so identical files are written once. Tools without skill support get the skill as a `skill-<name>.md` rule. Bundled files cannot follow, so they raise a sync note. Set `outputs.<target>.emit-skills-as-commands: true` to also write a slash command. Each target page gives the exact directory.

A body can point at another skill with [`{{$SKILLS_DIR}}`](@/docs/spec-format/_index.md#path-variables-name), which becomes each tool's own skills directory.

## Claude Code body syntax {#claude-code-body-syntax}

Claude Code replaces some syntax in a skill body before the model reads it ([skills docs](https://code.claude.com/docs/en/skills)):

- `` !`command` `` lines and ` ```! ` blocks run the command and insert its output.
- `$ARGUMENTS` becomes the text typed after the skill name.
- `$0`, `$1`, ... and `$ARGUMENTS[N]` become one argument each.

Other targets do not expand this syntax in project skills. Sync copies the body as written and prints one note per tool and syntax, naming each line:

```
  note: `!`command`` on 1 skill has no effect on codex (the command does not run at .agnostic-ai/skills/pr/SKILL.md:8; put the line in a ::target claude fence)
```

`on-unsupported: error` fails the sync instead, and `silent` hides the note. `lint` reports each line as LINT019. `sync --global` checks the user-level destination and reads `on-unsupported` from the `agnostic-ai.yaml` in the source root. There, Kilo's [inline shell placeholders](@/docs/targets/kilo.md#shell-commands-in-global-skills) in trusted user skill directories pass; argument placeholders and Claude dynamic code blocks still raise notes.

Put the Claude line in a fence and give other tools their own text:

```markdown
::target claude
!`git log main..HEAD --oneline`
::end
::target codex
Run `git log main..HEAD --oneline` first. `$ARGUMENTS` below means the text passed after the skill name.
::end
```

A `$1` or `` !`command` `` inside a fenced code block counts as an example and is not reported. `$ARGUMENTS` is still reported there. Escape a literal dollar as `\$1`, as Claude Code expects. `import claude` keeps the body as written, so Claude Code keeps its dynamic context.

[Commands](@/docs/spec-format/commands.md#claude-code-body-syntax) use the same syntax, and a few tools support part of it there.

## `disable-model-invocation` support by target {#disable-model-invocation-support-by-target}

Without the field, every tool lets the model use the skill on its own. Each target page gives the native details and links the vendor docs.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Cursor](@/docs/targets/cursor.md), [Zed](@/docs/targets/zed.md) | Written to `SKILL.md` |
| [Copilot](@/docs/targets/copilot.md) | Written to `SKILL.md`, with `argument-hint` |
| [OpenCode](@/docs/targets/opencode.md) | Written to `SKILL.md`. V2 honors it; V1 does not |
| [Codex](@/docs/targets/codex.md) | `allow_implicit_invocation: false` in `agents/openai.yaml` |
| [Windsurf](@/docs/targets/windsurf.md) | `triggers: [user]` in `SKILL.md` |
| [Crush](@/docs/targets/crush.md) | Written to `SKILL.md` with `user-invocable: true` |
| [Factory](@/docs/targets/factory.md) | Dropped with a note. Set `x-factory.disable-model-invocation` |
| Cline, Amp, Warp, Antigravity, Continue, Goose, OpenHands | Dropped with a note: no documented field keeps a skill user-only |

Copilot, Zed, Windsurf, and Crush write it only while no other target writes the same skills folder, since every writer of a shared `SKILL.md` must agree on its bytes. Otherwise the field is dropped with a note; point the tool's `skills-dir` at a folder of its own to keep it.

Factory skills go in the shared `.agents/skills/` folder, where the key would reach tools that ignore it and let their models use a manual-only skill. So while another target writes that folder, `x-factory.disable-model-invocation` also needs `outputs.factory.skills-dir: .factory/skills`; otherwise sync stops with `AAI-102`.
