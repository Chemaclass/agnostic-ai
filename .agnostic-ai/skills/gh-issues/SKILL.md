---
name: gh-issues
description: Walk over all open GitHub issues that are unassigned or assigned to the current user, and process each one via the gh-issue skill, sequentially.
argument-hint: "[--limit N] [--label foo] [--dry-run]"
disable-model-invocation: false
x-claude:
  allowed-tools: "Read, Bash(gh *), Bash(git *), Bash(go *), Bash(make *), Bash(./agnostic-ai *), Skill(gh-issue)"
---

# GitHub Issues Watcher

## Purpose

Process every open GitHub issue that is **unassigned** or **assigned to the current user (`@me`)** through the `gh-issue` skill. Record blockers and continue independent issues when the worktree is safe.

## Args

- `--limit N` — process at most N issues this run (default: all).
- `--label foo` — only issues carrying label `foo`.
- `--dry-run` — list issues that would be processed; do not invoke `gh-issue`.

## Phase 1: Discover

Fetch open issues that are unassigned **or** assigned to `@me`, oldest first. GitHub search does not OR these cleanly, so run two queries and merge:

```bash
# Unassigned
gh issue list \
  --state open \
  --search "no:assignee" \
  --json number,title,labels,assignees,createdAt \
  --limit 200

# Assigned to me
gh issue list \
  --state open \
  --assignee "@me" \
  --json number,title,labels,assignees,createdAt \
  --limit 200
```

Merge:
- If either query returns 200 rows, repeat it with a larger `--limit` until the result count falls below that limit.
- Deduplicate by `number`.
- Keep only issues whose `assignees` array is empty **or** contains the current user (`gh api user -q .login`).
- Drop issues assigned to anyone else (defensive).
- Apply `--label` filter if given.
- Apply `--limit` if given.
- Sort ascending by `createdAt` (FIFO).

Print the queue: `#<num> <title> [assignee]` per line, where `[assignee]` is `unassigned` or `@me`. If empty, exit cleanly.

## Phase 2: Worktree Sanity

Before touching any issue:

```bash
test -z "$(git status --porcelain)" || exit 1
git fetch origin main
git switch main
test "$(git rev-list --count origin/main..main)" -eq 0 || exit 1
git pull --ff-only
```

Abort if the worktree is dirty or local `main` is ahead of `origin/main`. Never auto-stash or discard local commits.

## Phase 3: Process Loop

For each issue in the queue:

1. Re-check assignment state (someone else may have grabbed it):
   ```bash
   gh issue view <num> --json assignees -q '.assignees[].login'
   me=$(gh api user -q .login)
   ```
   - Empty output → unassigned, proceed.
   - Only `$me` listed → already mine, proceed (skip self-assign step).
   - Any other login present → skip this issue.

2. Invoke the `gh-issue` skill with the issue number. It owns assignment, implementation, checks, docs, PR creation, CI, merge, and local main sync.

3. Record the outcome. Continue with the next independent issue when local `main` is clean. If a PR awaits approval, a check fails, or an external dependency blocks it, record the blocker and continue other actionable issues.

## Stop Conditions

Stop when the worktree is dirty, local `main` cannot safely advance, or an issue leaves uncommitted changes that prevent switching. Report each blocked issue and its concrete next step. Do not retry blindly.

## Dry Run

With `--dry-run`, only execute Phase 1 and print the queue. No assignment, no branching, no commits.

## Preconditions

- `gh` authenticated, can read issues, open and merge PRs.
- Worktree clean.
- `main` exists and tracks `origin/main`.
- `gh-issue` skill available in this session.

## Notes

- `gh-issue` owns validation and PR scope. Keep this skill focused on discovery, ordering, and progress across issues.
