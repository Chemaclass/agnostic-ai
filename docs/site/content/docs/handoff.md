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

Ask the current tool to "write a handoff", or "write a handoff for feat/x". It saves one note per branch, up to about 100 lines. Linked worktrees share `.agnostic-ai/local/handoffs/` in the main checkout, so a note survives when its worktree is removed. For `feat/x`, the filename is `feat-x.md`:

```markdown
# Handoff
tool: codex | date: 2026-10-05T14:02Z | branch: feat/x | head: abc1234
worktree: /path/to/checkout | status: active | pr: none

## Goal
## Acceptance
## Done
## In progress
## Next steps
## Open decisions
## Gotchas
## Verify
```

A new handoff replaces only that branch's previous note. Its header names the real branch, worktree, status (`active`, `blocked`, or `ready`), and PR (`#N` or `none`). The filename replaces characters outside letters, numbers, `.`, `_`, and `-` with `-`. If two branch names produce the same filename, the skill asks before replacing a note. It writes through a private temporary file and renames it into place, with mode 0600. It never includes secrets or the tool's own memory.

The project root is the nearest folder with `agnostic-ai.yaml`, or the Git repository root when there is none. A subproject with its own config keeps its own store at the same relative path in the main checkout. If that checkout is bare, its store is not ignored by Git, or a write fails, the skill uses the current checkout's ignored store. It stops if neither destination is safe.

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

The separate `handoff-hook` built-in saves the Git state when a session ends or the tool shortens its conversation history (context compaction):

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

When a new session starts in a linked worktree, the notice names its branch's handoff. In the main checkout, it reports the number of task handoffs. The hooks still show the old messages for `.agnostic-ai/local/HANDOFF.md` and `HANDOFF.auto.md` when no task handoff matches. Nothing resumes until you ask.

Claude hooks use the working directory (`cwd`) supplied by the tool's event, including linked Git worktrees (additional checkouts of the same repository), even when the hook process runs in the main checkout. Config discovery stays inside the selected checkout. Empty input or an event without `cwd` uses the process directory. Invalid or ambiguous input saves nothing. Input is limited to 64 KiB, 32 nested containers, and a two-second read deadline.

Claude hooks need Bash and `awk`; the other hooks need `sh`. All need Git. On Windows, use Git for Windows with its Unix tools on PATH. Start Claude Code from the project root so it loads `.claude/settings.json`.

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

Open the other tool and say "resume the handoff" or "continue where Codex left off". The skill finds notes for the current branch in the shared store, the checkout store, and the old `HANDOFF.md`, choosing the newest match. It reports the goal and next steps, then checks the branch, HEAD, and working changes against Git. Unexpected drift needs confirmation before continuing.

From the main checkout, ask to resume a named branch. If no note matches the current branch, the skill lists local task headers and asks which to resume. A missing worktree appears as removed; its note stays available. A tool in that task's live worktree can resume its matching note without a branch mismatch prompt.

The old `.agnostic-ai/local/HANDOFF.md` still works. If `HANDOFF.auto.md` is newer and belongs to the same branch, the skill uses its Git state alongside the manual task details. Automatic snapshots still stay in each checkout.

## Limits and overrides

Handoffs stay on one machine. The shared store reaches linked worktrees of that repository, but is ignored by Git and does not reach teammates. A fallback note stays in its checkout. Listing task handoffs uses the skill; there is no separate list command.

To replace the built-in, create your own skill named `handoff` in the project. `agnostic-ai list` shows which one wins. If a global built-in clashes with your project skill, rename yours or turn the built-in off in the global config.

## See where it comes from

```bash
agnostic-ai list
agnostic-ai explain builtin:handoff
agnostic-ai why .claude/skills/handoff/SKILL.md
```

Reports name it `builtin:handoff` with the agnostic-ai version that ships it. After an upgrade changes the built-in, `sync --check` reports that generated files are out of date until you run `agnostic-ai sync`.
