---
name: handoff
description: Write or resume a session handoff between AI tools.
---

# handoff

Write or resume a portable session handoff when the user requests it. Choose the mode from the request: "write a handoff" writes; "resume" or "continue where Codex left off" reads. An explicit argument can select the mode, but is never required. Do not read a handoff at session start without a request.

Use `.agnostic-ai/local/HANDOFF.md` in the current project root, including when this skill is installed globally. For root lookup, use canonical paths for the current directory, candidate ancestors, and the effective global source root (`AGNOSTIC_AI_HOME` when nonempty, otherwise `~/.agnostic-ai`). Find the nearest ancestor containing `agnostic-ai.yaml` or the legacy `agnostic.config.yaml`, but exclude the effective global source root: its home config never identifies a project. The remaining nearest configured ancestor is the project root. If none exists, use `git rev-parse --show-toplevel` for the current checkout. Do not replace a configured subproject root with its Git worktree root. Keep one file per configured project in each checkout. Writing overwrites the previous handoff. Use portable Markdown.

## Write

1. Gather the current branch with `git branch --show-current`, HEAD with `git rev-parse HEAD`, and working changes with `git status --short`. Record a detached HEAD as `detached`.
2. Create `.agnostic-ai/local/` if needed. Fill the template below from the current session and git state. The header records the current tool, UTC date, branch, and HEAD. Replace the example values with the actual values.
3. State the goal, acceptance criteria, completed work, unfinished work, and next steps. Separate unresolved decisions from settled ones. Include changed files and uncommitted work where they help the arriving tool continue. Under Verify, record checks already run and checks still needed.
4. Keep a soft cap of 100 lines. Cut Gotchas before cutting Next steps. Never write secrets, tokens, or `.env` values. Never read or write any tool's own memory store.
5. Write the file before proposing learnings. Follow Promote learnings below, then report the handoff path.

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

## Promote learnings

1. Propose durable learnings from the session. Skip facts already stated in loaded rules or obvious from the code. If none remain, finish the handoff.
2. Tag each proposal with its scope. These are the default destinations:

   | Tag | Default destination |
   | --- | --- |
   | [project] | `.agnostic-ai/rules/learnings.md` |
   | [personal] | `.agnostic-ai/local/rules/learnings-local.md` |
   | [global] | `~/.agnostic-ai/rules/learnings.md` |

   Resolve project `learnings.md` under the effective `sources.rules` from the project config and any `agnostic-ai.local.yaml` override. Relative paths start at the project root; use absolute paths directly, without expanding `~`. The personal path stays fixed. If `AGNOSTIC_AI_HOME` is nonempty, put global `rules/learnings.md` beneath that directory. Global `sources` config does not change its fixed `rules/` folder. Show the actual resolved destination with each proposal.

   Global scope is available only when the user explicitly picks it for that item.
3. Read each resolved destination if it exists and show one append-only diff per file, naming the actual path before requesting confirmation. Preserve existing entries and frontmatter. For a new file, include YAML frontmatter with `name`, `description`, and `alwaysApply: true` in the diff. Use `name: learnings` for project and global files, and `name: learnings-local` for the personal file so it does not replace the project rule. Global rules are unconditional: omit `scope`, `globs`, and `paths`.
4. Wait for confirmation of each item's content and scope. A handoff request alone does not approve learning writes. Append only approved items; never rewrite existing entries.
5. After project or personal writes, run `agnostic-ai sync` from the project root. After approved global writes, run `agnostic-ai sync --global`. If the binary is not on PATH, print the relevant command for the user and mark it pending. Report failed syncs with their error and leave them as unfinished work.
6. After approved writes and sync attempts, gather branch, HEAD, and `git status --short` again using the commands above. Refresh the handoff header, Done, and Verify with the current git state, learning files changed, and sync results. Record failed or pending syncs separately from completed verification.

## Resume

1. Read `.agnostic-ai/local/HANDOFF.md` when it exists. If `.agnostic-ai/local/HANDOFF.auto.md` exists and is newer, or the manual file is missing, read the snapshot as additional recent git evidence. Keep the manual Goal and Next steps when both files exist. If neither exists, tell the user there is no handoff to resume.
2. Gather the current branch, HEAD, and `git status --short` using the commands above. Compare the manual header and recorded changes with the current git state even when the snapshot is newer, so the snapshot cannot hide drift from the manual task. Also compare the snapshot header when read. Report the goal, next steps, and any drift, including a changed branch or HEAD. The tool and date identify the earlier session; a tool change is expected when handing off.
3. Ask for confirmation before acting on any branch, HEAD, or working-tree mismatch. Do not resume until the user confirms. A copied handoff can belong to a different worktree or task.
4. Continue from the manual Next steps after the comparison and any required confirmation. If only a git snapshot exists without a task goal or Next steps, ask for the user's intended next step before work can continue. Never read or write any tool's own memory store.
