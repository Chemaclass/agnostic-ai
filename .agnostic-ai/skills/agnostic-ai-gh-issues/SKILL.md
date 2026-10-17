---
name: agnostic-ai-gh-issues
description: Work the open issues that are unassigned or assigned to you, one at a time through agnostic-ai-gh-issue. Use when asked to work the backlog.
argument-hint: "[--limit N] [--label foo] [--dry-run]"
disable-model-invocation: false
x-claude:
  allowed-tools: "Read, Bash(gh *), Bash(git *), Bash(go *), Bash(make *), Bash(./agnostic-ai *), Skill(agnostic-ai-gh-issue)"
---

# GitHub issues watcher

Work open issues that are unassigned or assigned only to the current user through `.agnostic-ai/skills/agnostic-ai-gh-issue/SKILL.md`, one at a time. That skill owns implementation, validation, review, and authorized merging.

## Discover

- `--limit N`: process at most N issues (default: all).
- `--label foo`: require this label.
- `--dry-run`: list the queue only, with no assignment or repository changes.

Query both unassigned issues (`gh issue list --state open --search "no:assignee"`) and issues assigned to `@me` (`gh issue list --state open --assignee @me`), requesting `number,title,labels,assignees,createdAt` as JSON. Start with `--limit 200`; if a query fills its limit, increase it or paginate until complete.

Deduplicate by number. Resolve the current login with `gh api user -q .login`; exclude any issue assigned to another person, including issues also assigned to the current user. Apply the label filter, sort oldest first by `createdAt`, then apply the requested limit. Print `#<number> <title> [unassigned|@me]`. An empty queue ends cleanly.

## Process

Before implementation, fetch `origin/main` and stop if local `main` has unpublished commits. Preserve changes in the starting checkout; do not require it to be clean or switch it to `main`.

For each issue, recheck its open state and assignment, skipping closed issues and issues now assigned to others. Invoke the issue skill and record the issue's absolute task worktree path, branch, PR, outcome, and blocker. Reuse an existing task worktree or PR only after its safety checks.

Continue independent issues when a PR awaits approval, CI fails, an external dependency blocks progress, or a task worktree contains unfinished work. Keep that worktree intact and record its next step; never switch or clean it to start another issue. Stop only if unpublished local `main` commits or unsafe repository/worktree state prevent safe isolation. Do not retry a blocker blindly.

Report completed PRs and each blocked issue with its retained worktree path and concrete next step.
