+++
title = "Session handoffs"
description = "Write a portable session handoff and resume work in another AI tool on the same machine."
weight = 78

[extra]
group = "Workflows"
+++

# Session handoffs

The built-in `handoff` skill lets you stop work in one AI tool and pick it up in another. The first tool writes a short Markdown note. The second tool reads it, checks it against git, and continues.

## Enable the skill

Add this to `agnostic-ai.yaml`, then run `agnostic-ai sync`:

```yaml
requires: ">=0.80.0"
builtins: [handoff]
```

New projects created with `init` have it on already. To use it in every project, add the same lines to `~/.agnostic-ai/agnostic-ai.yaml` and run `agnostic-ai sync --global`.

Most tools get it as a skill. Tools set to `emit-skills-as-commands` also get a command. Aider gets the text inline when `outputs.aider.rules-file` is set. Jules has no skill support.

## Leave a session

Ask the current tool to "write a handoff". It saves `.agnostic-ai/local/HANDOFF.md` at the project root, up to about 100 lines:

```markdown
# Handoff
tool: codex | date: 2026-10-05T14:02Z | branch: feat/x | head: abc1234

## Goal
## Acceptance
## Done
## In progress
## Next steps
## Open decisions
## Gotchas
## Verify
```

Each new handoff replaces the old one. It never includes secrets or the tool's own memory.

The project root is the nearest folder with `agnostic-ai.yaml`, or the git root when there is none. A subproject with its own config keeps its own handoff.

## Save what you learned

After writing the handoff, the skill suggests lessons worth keeping. Each one shows its scope and the exact change. You approve each before it is saved.

| Scope | File | Then run |
| --- | --- | --- |
| [project] | `.agnostic-ai/rules/learnings.md` | `agnostic-ai sync` |
| [personal] | `.agnostic-ai/local/rules/learnings-local.md` | `agnostic-ai sync` |
| [global] | `~/.agnostic-ai/rules/learnings.md` | `agnostic-ai sync --global` |

Project learnings can be committed for your team. Personal learnings stay in your checkout and are not committed. Global learnings apply to every project on your machine.

The project file follows `sources.rules` if you changed it, and the global file follows `AGNOSTIC_AI_HOME` if set. If `agnostic-ai` is not on your PATH, the skill prints the sync command for you to run.

## Add automatic Git snapshots

The separate `handoff-hook` built-in saves a git snapshot when a session ends or its context is compacted:

```yaml
requires: ">=0.80.0"
builtins: [handoff, handoff-hook]
```

It is off by default and works with these tools:

| Tool | Saves a snapshot on | Shows a notice on |
| --- | --- | --- |
| [Claude Code](https://code.claude.com/docs/en/hooks) | `PreCompact`, `SessionEnd` | `SessionStart` |
| [Codex](https://learn.chatgpt.com/docs/hooks) | `PreCompact`, `SessionEnd` | `SessionStart` |
| [Gemini CLI](https://geminicli.com/docs/hooks/reference/) | `PreCompress`, `SessionEnd` | `SessionStart` |
| [Qoder CLI](https://docs.qoder.com/cli/hooks-reference.md) | `PreCompact`, `SessionEnd` | `SessionStart` |
| [Factory / Droid](https://docs.factory.com/harness/hooks) | `PreCompact`, `SessionEnd` | `SessionStart` |

The snapshot goes to `.agnostic-ai/local/HANDOFF.auto.md`. It holds the tool, date, branch, HEAD, `git status --short`, and the last five commit messages. It never reads your handoff note, diffs, transcripts, or tool memory. Outside a git repository, or before the first commit, no snapshot is saved.

When a new session starts and a handoff file exists, the tool shows a notice suggesting you resume. Nothing resumes until you ask.

The hooks need `sh` and Git. On Windows, use Git for Windows with its Unix tools on PATH. Start Claude Code from the project root so it loads `.claude/settings.json`.

Each tool's own hook settings still apply: approve Codex hooks with `/hooks`, and approve Gemini project hooks when asked. A tool may stop before the snapshot finishes when it exits or compacts.

### Factory notices

Factory hides hook output by default. To see the startup notice, add this settings spec and sync:

```yaml
# .agnostic-ai/settings/handoff-notices.yaml
name: handoff-notices
targets: [factory]
x-factory:
  showHookOutput: true
```

Factory hooks only work per project, not through `sync --global`.

### Global hooks

Claude Code, Codex, Gemini CLI, and Qoder CLI can also get the hooks through `agnostic-ai sync --global`. They still find the current project at run time.

A project `agnostic-ai sync` adds `.agnostic-ai/local/` to the ignore file. With only global hooks or skills, add that line to the project's `.gitignore` yourself.

## Resume in another tool

Open the other tool in the same checkout and say "resume the handoff" or "continue where Codex left off". The skill reports the goal and next steps, then compares the branch and HEAD with git. If they differ, it asks before continuing.

If `HANDOFF.auto.md` is newer, the skill takes the git state from it and the task details from `HANDOFF.md`.

## Limits and overrides

Handoffs stay on one machine. The `.agnostic-ai/local/` folder is not committed, so teammates and other checkouts never see it.

To replace the built-in, create your own skill named `handoff` in the project. `agnostic-ai list` shows which one wins. If a global built-in clashes with your project skill, rename yours or turn the built-in off in the global config.

## See where it comes from

```bash
agnostic-ai list
agnostic-ai explain builtin:handoff
agnostic-ai why .claude/skills/handoff/SKILL.md
```

Reports name it `builtin:handoff` with the agnostic-ai version that ships it. After an upgrade changes the built-in, `sync --check` reports drift until you run `agnostic-ai sync`.
