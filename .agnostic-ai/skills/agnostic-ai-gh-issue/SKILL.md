---
name: agnostic-ai-gh-issue
description: Take one GitHub issue from branch to merged PR with TDD. Use when asked to work, fix, or implement an issue by number.
argument-hint: "[issue-number]"
x-claude:
  allowed-tools: "Read, Edit, Write, Bash(gh *), Bash(git *), Bash(go *), Bash(make *), Bash(./agnostic-ai *)"
disable-model-invocation: false
---

# GitHub issue workflow

Take one issue through implementation, review, and merge. This workflow authorizes merging its issue PR unless the user limits the task to a PR or review.

## Understand and isolate

1. Parse the issue number, stripping `#`. Read the body and every comment: `gh issue view <number> --json number,url,title,body,labels,assignees,state,comments`. Fetch remaining comment pages if the result is incomplete. Later maintainer comments resolve conflicts with the body. Verify that old findings still apply.
2. Check the current user's login and assignment. Skip an issue assigned to another person; assign `@me` only when unassigned.
3. Fetch `origin/main`. Stop if local `main` has unpublished commits; never reset, stash, or discard user work. A dirty starting checkout does not prevent isolated work.
4. Look for an existing issue branch, PR, and linked checkout before creating anything. Reuse the task checkout only after checking its branch, remote head, and local changes. Preserve unpublished commits and stop work on an unsafe task checkout rather than overwriting it.
5. Otherwise create a dedicated worktree and branch from fresh `origin/main`, without switching the starting checkout. Use `<fix|feat|docs>/<number>-<slug>` according to issue labels (`feat` by default), for example `git worktree add -b <branch> <absolute-task-path> origin/main`. Record the absolute task path with the issue number and run every task command there.

## Implement

State one acceptance scenario, affected files, and a short implementation plan. Follow the applicable project rules, including adapter independence, error context, and generated output.

For behavior changes, write a test that fails before the fix, implement, then refactor. For prose or data changes, use existing focused checks for the affected inputs instead of inventing behavior tests. Keep the issue's scope; record independent follow-ups separately.

For user-visible changes, including documentation corrections, follow the documentation checklist in `.agnostic-ai/rules/docs-sync.md`. Select generation and validation from the single policy in repository-root `docs/internal/contributing.md`, section "Choose checks for your change". Follow its serial execution, evidence, and reuse rules; complete the selected gate before pushing.

Review the final diff and commit with a signed Conventional Commit and `Related to #<number>` in the body. Push without rewriting published history. Reuse an existing PR or open one assigned to Chemaclass with the matching label. Write its body to a file and use `gh pr create --body-file <path>`; include the outcome, relevant decisions, validation evidence, and `Closes #<number>`.

## Review and finish

Use `.agnostic-ai/skills/pr-sweep/SKILL.md` from "Review" onward for this PR only, passing the recorded task path and the user's merge scope. Its review, CI, merge verification, and cleanup rules apply. If merging is outside the user's scope, stop with the reviewable PR. Report the PR, acceptance evidence, and any concrete blocker.
