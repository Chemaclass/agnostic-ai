+++
title = "Session handoffs"
description = "Write a portable session handoff and resume work in another AI tool on the same machine."
weight = 78

[extra]
group = "Workflows"
+++

# Session handoffs

The built-in `handoff` skill leaves a task note in one coding tool so you can resume the task in another on the same machine. The second tool checks the note against Git before it continues.

## Enable the skill

Add this to `agnostic-ai.yaml`, then run `agnostic-ai sync`:

```yaml
requires: ">=0.80.0"
builtins: [handoff]
```

Projects created with `init` have it on already. For every project, add the same lines to `~/.agnostic-ai/agnostic-ai.yaml` and run `agnostic-ai sync --global`.

Most tools get a skill; tools set to `emit-skills-as-commands` also get a command. Aider gets the text inline when `outputs.aider.rules-file` is set. Jules has no skill support.

## Leave a session

Ask the current tool to “write a handoff”, or “write a handoff for feat/x” for another branch. It saves the goal, progress, and next steps in a short note for the branch.

## Resume in another tool

Open the other tool in the same project and say “resume the handoff”. It finds the newest note for the branch, reports the next steps, and checks the branch, commit, and working changes against Git. Unexpected differences need your confirmation.

## Limits and overrides

Handoffs stay on one machine; teammates never see them. Linked worktrees (extra checkouts of the same repository) can share the notes, which Git ignores. List saved tasks with [`agnostic-ai handoff list`](@/docs/cli-reference/maintain.md#handoff-list).

To replace the built-in, create your own project skill named `handoff`; `agnostic-ai list` shows which one wins. If a global built-in clashes with your project skill, rename yours or turn the built-in off in the global config.

<details>
<summary>Inspect note format, branch matching, and fallback stores</summary>

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

Notes live in `.agnostic-ai/local/handoffs/` in the main checkout. `feat/x` becomes `feat-x.md`: any character other than a letter, number, `.`, `_`, or `-` becomes `-`. A note replaces only its branch's previous note; a filename collision needs confirmation. Status is `active`, `blocked`, or `ready`. Writes go through a private temporary file and an atomic rename, with mode 0600. Notes exclude secrets and native tool memory.

The project root is the nearest folder with `agnostic-ai.yaml`, or the Git repository root when there is none. A subproject with its own config keeps its own store at the same relative path in the main checkout. If that checkout is bare, its store is not ignored by Git, or a write fails, the skill uses the current checkout's ignored store. It stops if neither is safe.

From the main checkout, ask to resume a named branch. If no note matches the current branch, the skill lists local task headers and asks which to resume, the same tasks `agnostic-ai handoff list` shows. A task whose worktree is gone shows as removed, and its note stays available. A tool in the task's live worktree resumes its note without a branch mismatch prompt.

The old `.agnostic-ai/local/HANDOFF.md` and `HANDOFF.auto.md` still work. The skill finds matching automatic snapshots in the shared store, the checkout store, and the old path. When the newest snapshot is newer than the manual note, it adds that Git evidence and keeps the note's goal, next steps, status, and PR. A shared snapshot survives removal of its worktree.

</details>

<details>
<summary>Write notes from linked worktrees</summary>

## Write from a linked worktree

Sync can grant access to the main checkout's ignored handoff store:

| Tool | Native setting |
|---|---|
| Codex | `sandbox_workspace_write.writable_roots` |
| Claude Code | `permissions.additionalDirectories` in `settings.local.json` |
| Qoder | `permissions.additionalDirectories` |
| Factory | `sandbox.filesystem.allowWrite` in `.factory/settings.local.json` |
| OpenCode | `permission.external_directory`, `"<store>/**": "allow"` |
| Devin CLI | `permissions.allow`, `Write(<store>/**)` |

The absolute path goes only into native config that Git ignores. For a tracked file, sync reports the path and asks you to untrack the file; for a hand-written or unmanaged file, it reports the access to add yourself. Existing restrictions still apply, Factory's grant does not turn its sandbox on, and an explicit Codex sandbox table in an imported overlay stays unchanged.

OpenCode and Devin grants go only into files sync created or granted in before; skipping a grant does not skip the rest of the file's sync. OpenCode keeps any existing rule for that path and gets no grant when the path contains glob characters. Hand-written grants always stay. An unchanged generated grant leaves with the built-in, including one in a file you later committed.

An unsafe main store (tracked, not ignored, a symlink, or inaccessible) gets no grant, and the skill uses a safe store in the current checkout. Dry runs and checks create no store.

</details>

<details>
<summary>Keep useful lessons after a handoff</summary>

## Save what you learned

The lead asks each delegate to write a handoff for its branch before returning. After a handoff, the skill suggests lessons worth keeping, each with its scope and exact change, and saves only the ones you approve.

| Scope | File | Then run |
| --- | --- | --- |
| [project] | `.agnostic-ai/rules/learnings.md` | `agnostic-ai sync` |
| [personal] | `.agnostic-ai/local/rules/learnings-local.md` | `agnostic-ai sync` |
| [global] | `~/.agnostic-ai/rules/learnings.md` | `agnostic-ai sync --global` |

Project lessons are shared in Git; personal lessons stay in the checkout; global lessons apply across projects.

The project file follows `sources.rules` if you changed it, and the global file follows `AGNOSTIC_AI_HOME` if set. If `agnostic-ai` is not on your PATH, the skill prints the sync command for you to run.

</details>

<details>
<summary>Save automatic Git snapshots and show startup notices</summary>

## Add automatic Git snapshots

The separate `handoff-hook` built-in, off by default, saves the Git state when a session ends or the tool shortens its conversation history (context compaction):

```yaml
requires: ">=0.80.0"
builtins: [handoff, handoff-hook]
```

| Tool | Saves a snapshot on | Shows a notice on |
| --- | --- | --- |
| [Claude Code](https://code.claude.com/docs/en/hooks) | `PreCompact`, `SessionEnd` | `SessionStart` |
| [Codex](https://learn.chatgpt.com/docs/hooks) | `PreCompact`, `SessionEnd` | `SessionStart` |
| [Gemini CLI](https://geminicli.com/docs/hooks/reference/) | `PreCompress`, `SessionEnd` | `SessionStart` |
| [Qoder CLI](https://docs.qoder.com/cli/hooks-reference.md) | `PreCompact`, `SessionEnd` | `SessionStart` |
| [Factory / Droid](https://docs.factory.com/harness/hooks) | `PreCompact`, `SessionEnd` | `SessionStart` |

The snapshot goes to `handoffs/<branch-slug>.auto.md` in the same store as manual notes, written the same safe way, with the same fallback to the checkout store. It holds the tool, date, branch, HEAD, absolute worktree path, `git status --short`, and the last five commit messages. Each hook also keeps `.agnostic-ai/local/HANDOFF.auto.md` as a compatibility copy when that path is ignored. It never reads your handoff note, diffs, transcripts, or tool memory. Outside a git repository, or before the first commit, no snapshot is saved.

When a session starts in a linked worktree, the notice names its branch's handoff. In the main checkout, it reports how many branches have handoffs. A manual note and a snapshot for the same branch count as one task, and the notice prefers the manual note. Snapshot worktree headers end with `detached: true` or `detached: false`. A detached snapshot uses its HEAD as the task key, so different detached tasks and a branch named `detached` stay separate. The final boolean keeps marker-like path text intact. The hooks still show the old messages for `.agnostic-ai/local/HANDOFF.md` and `HANDOFF.auto.md` when no task handoff matches. Nothing resumes until you ask.

Claude hooks use the working directory (`cwd`) from the tool's event, including in linked worktrees, even when the hook process runs in the main checkout. Config discovery stays inside that checkout. Empty input or an event without `cwd` uses the process directory. Invalid or ambiguous input saves nothing. Input is limited to 64 KiB, 32 nested containers, and a two-second read deadline.

Claude hooks need Bash; the other hooks need `sh`. All need Git and `awk`. On Windows, use Git for Windows with its Unix tools on PATH. Start Claude Code from the project root so it loads `.claude/settings.json`.

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

</details>

## See where it comes from

```bash
agnostic-ai list
agnostic-ai explain builtin:handoff
agnostic-ai why .claude/skills/handoff/SKILL.md
```

Reports show `builtin:handoff` and its release version. After an upgrade that changes the built-in, sync again.
