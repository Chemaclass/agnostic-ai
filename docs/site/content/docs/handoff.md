+++
title = "Session handoffs"
description = "Write a portable session handoff and resume work in another AI tool on the same machine."
weight = 74

[extra]
group = "Workflows"
+++

# Session handoffs

The built-in `handoff` skill saves a task note so another coding tool on the same machine can resume, after checking the note against Git. For a summary for people, use a [wrap-up](@/docs/wrap-up.md). For a parallel task, use a [side conversation](@/docs/side-conversation.md).

## Enable the skill

Add this to `agnostic-ai.yaml`, then run `agnostic-ai sync`:

```yaml
requires: ">=0.92.0"
builtins: [handoff]
```

- `init` projects have it on already.
- For every project, add the same lines to `~/.agnostic-ai/agnostic-ai.yaml` and run `agnostic-ai sync --global`.
- Most tools get a skill; tools with `emit-skills-as-commands` also get a command. Aider gets inline text when `outputs.aider.rules-file` is set. Jules has no skill support.

## Leave a session

Say “write a handoff”, or “write a handoff for feat/x” for another branch. The tool saves the goal, progress, and next steps in a short note.

## Resume in another tool

In the same project, say “resume the handoff”. The tool finds the newest note for the branch, reports the next steps, and checks branch, commit, and working changes against Git. It asks you to confirm unexpected differences.

## Limits and overrides

- Handoffs stay on one machine; teammates never see them. Git ignores the notes, and linked worktrees can share them.
- List saved tasks with [`agnostic-ai handoff list`](@/docs/cli-reference/maintain.md#handoff-list).
- To replace the built-in, add a project skill named `handoff`; `agnostic-ai list` shows which wins.
- Some tools load a clashing global built-in first ([shared names](@/docs/global-configuration-reference.md#global-shared-names)). Rename yours or turn the built-in off globally.

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

**Where notes live**

- `.agnostic-ai/local/handoffs/` in the main checkout.
- `feat/x` becomes `feat-x.md`: any character other than a letter, number, `.`, `_`, or `-` becomes `-`.
- A note replaces only its branch's previous note. A filename collision needs confirmation.
- Status is `active`, `blocked`, or `ready`.
- Writes use a private temporary file and an atomic rename, mode 0600.
- Notes exclude secrets and native tool memory.

**Project root and fallback**

- The root is the nearest folder with `agnostic-ai.yaml`, else the Git root. A subproject with its own config keeps its own store at the same relative path in the main checkout.
- If the main checkout is bare, its store is not Git-ignored, or a write fails, the skill uses the current checkout's ignored store. It stops if neither is safe.

**Resuming**

- From the main checkout, ask to resume a named branch.
- With no note for the current branch, the skill lists local task headers (the same tasks as `agnostic-ai handoff list`) and asks which to resume.
- A task whose worktree is gone shows as removed; its note stays available.
- A tool in the task's live worktree resumes its note without a branch mismatch prompt.

**Older files and snapshots**

- `.agnostic-ai/local/HANDOFF.md` and `HANDOFF.auto.md` still work.
- The skill finds automatic snapshots in the shared store, the checkout store, and the old path.
- When the newest snapshot is newer than the manual note, it adds that Git evidence and keeps the note's goal, next steps, status, and PR.
- A shared snapshot survives removal of its worktree.

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

- The absolute path goes only into native config that Git ignores.
- For a tracked file, sync reports the path and asks you to untrack it. For a hand-written or unmanaged file, it reports the access for you to add.
- Existing restrictions still apply. Factory's grant does not turn its sandbox on. An explicit Codex sandbox table in an imported overlay stays unchanged.
- OpenCode and Devin grants go only into files sync created or granted in before. Skipping a grant does not skip the rest of the file's sync.
- OpenCode keeps any existing rule for that path, and gets no grant when the path contains glob characters.
- Hand-written grants always stay. An unchanged generated grant leaves with the built-in, even in a file you later committed.
- An unsafe main store (tracked, not ignored, a symlink, or inaccessible) gets no grant; the skill uses a safe store in the current checkout.
- Dry runs and checks create no store.

</details>

<details>
<summary>Keep useful lessons after a handoff</summary>

## Save what you learned

A lead asks each delegate to write a handoff for its branch before returning. After a handoff, the skill suggests lessons, each with scope and exact change, and saves only what you approve.

| Scope | File | Then run |
| --- | --- | --- |
| [project] | `.agnostic-ai/rules/learnings.md` | `agnostic-ai sync` |
| [personal] | `.agnostic-ai/local/rules/learnings-local.md` | `agnostic-ai sync` |
| [global] | `~/.agnostic-ai/rules/learnings.md` | `agnostic-ai sync --global` |

- Project lessons are shared in Git, personal ones stay in the checkout, global ones apply everywhere.
- The project file follows `sources.rules` if changed; the global file follows `AGNOSTIC_AI_HOME` if set.
- If `agnostic-ai` is not on PATH, the skill prints the sync command.

</details>

<details>
<summary>Save automatic Git snapshots and show startup notices</summary>

## Add automatic Git snapshots

The separate `handoff-hook` built-in, off by default, saves the Git state when a session ends or the tool shortens its conversation history (context compaction):

```yaml
requires: ">=0.92.0"
builtins: [handoff, handoff-hook]
```

| Tool | Saves a snapshot on | Shows a notice on |
| --- | --- | --- |
| [Claude Code](https://code.claude.com/docs/en/hooks) | `PreCompact`, `SessionEnd` | `SessionStart` |
| [Codex](https://learn.chatgpt.com/docs/hooks) | `PreCompact`, `SessionEnd` | `SessionStart` |
| [Gemini CLI](https://geminicli.com/docs/hooks/reference/) | `PreCompress`, `SessionEnd` | `SessionStart` |
| [Qoder CLI](https://docs.qoder.com/cli/hooks-reference.md) | `PreCompact`, `SessionEnd` | `SessionStart` |
| [Factory / Droid](https://docs.factory.com/harness/hooks) | `PreCompact`, `SessionEnd` | `SessionStart` |

**What a snapshot holds**

- File: `handoffs/<branch-slug>.auto.md` in the manual note store, with the same private writes and checkout-store fallback.
- Content: tool, date, branch, HEAD, absolute worktree path, `git status --short`, and the last five commit messages.
- Each hook also keeps `.agnostic-ai/local/HANDOFF.auto.md` as a compatibility copy when that path is ignored.
- It never reads your handoff note, diffs, transcripts, or tool memory.
- Outside a git repository, or before the first commit, no snapshot is saved.

**Startup notice**

- In a linked worktree, it names the branch's handoff. In the main checkout, it says how many branches have handoffs.
- A manual note and a snapshot for one branch count as one task; the notice prefers the manual note.
- Snapshot worktree headers end with `detached: true` or `detached: false`. A detached snapshot uses its HEAD as the task key, so detached tasks and a branch named `detached` stay separate. The final boolean keeps marker-like path text intact.
- With no task handoff, the hooks still show the old messages for `HANDOFF.md` and `HANDOFF.auto.md`.
- Nothing resumes until you ask.

**Claude hooks**

- They use the event's working directory (`cwd`), including in linked worktrees, even when the hook runs in the main checkout. Config discovery stays inside that checkout.
- Empty input or no `cwd` uses the process directory. Invalid or ambiguous input saves nothing.
- Input is limited to 64 KiB, 32 nested containers, and a two-second read deadline.
- They call scripts generated under `.claude/hooks/`, so completion messages show a short command. Project hooks find them through `CLAUDE_PROJECT_DIR`; global hooks use absolute paths.
- Sync tracks the scripts and removes unchanged copies when you disable the built-in.

**Requirements and tool settings**

- Claude hooks need Bash; the others need `sh`. All need Git and `awk`. On Windows, use Git for Windows with its Unix tools on PATH.
- Start Claude Code from the project root so it loads `.claude/settings.json`.
- Each tool's own hook settings apply: approve Codex hooks with `/hooks`, and approve Gemini project hooks when asked.
- A tool may stop before the snapshot finishes when it exits or compacts.

### Factory notices

Factory shows hook rows in the session transcript by default (`hooks.showHookOutput: true`). A successful hook can still hide its output with `suppressOutput: true`. This setting controls transcript visibility, not whether the model receives context ([Factory settings](https://docs.factory.com/droid-cli/settings#hooks)).

If you set it to `false`, change only `hooks.showHookOutput` to `true` in your existing `.factory/settings.json`, keeping the other `hooks` entries.

Factory hooks only work per project, not through `sync --global`.

### Global hooks

- Claude Code, Codex, Gemini CLI, and Qoder CLI can get the hooks through `agnostic-ai sync --global`. They still find the current project at run time.
- A project sync adds `.agnostic-ai/local/` to the ignore file. With only global hooks or skills, add that line to the project's `.gitignore` yourself.

</details>

## See where it comes from

```bash
agnostic-ai list
agnostic-ai explain builtin:handoff
agnostic-ai why .claude/skills/handoff/SKILL.md
```

Reports show `builtin:handoff` and its release version. After an upgrade that changes the built-in, sync again.
