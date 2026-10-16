---
name: pr-sweep
description: Review every open PR with the code-reviewer agent and an adversarial review, apply the findings, and merge each one once CI is green. Use when asked to ship, sweep, or merge open PRs.
argument-hint: "[PR-number ...] [--no-merge]"
disable-model-invocation: false
---

# PR sweep

Review and finish the requested PRs, or all eligible open PRs when none are specified. Invoking this merge workflow authorizes merging within that scope. With `--no-merge`, or when called from a review-only task, stop at reviewable green PRs.

## List and isolate

List open PRs with number, title, author, draft state, head branch and SHA, and base branch. Paginate until complete. Skip drafts and PRs by other authors and report them.

Fetch the base and PR heads. Stop if local `main` has unpublished commits. Preserve a dirty starting checkout and perform each PR's work in a dedicated worktree. Record its absolute path and branch; reuse an existing task worktree only after checking its head and local changes. Do not discard unpublished work or force a branch held by another worktree. Stop the affected PR if its task worktree is unsafe and continue independent PRs. Run Git commands in the task worktree; a tool wrapper problem does not justify switching the starting checkout.

## Review

Run two independent passes on the current PR diff:

- Use the available `code-reviewer` role for Go changes, or a reviewer against project rules for other changes. Respect the role's configured model; concrete model choices belong in agent metadata.
- Use the available adversarial review role or command to challenge the design, assumptions, and real usage failures.

If either role is unavailable, use an available reviewer with the corresponding brief. If delegation is unavailable, perform two separate passes with those briefs. Read both results before editing. Repeat the adversarial pass after fixes until remaining findings are resolved or explicitly rejected with evidence.

## Verify and apply

A finding is a claim. Reopen its cited file on the current PR head, reproduce behavior claims with a failing test or current binary, and verify vendor claims against current vendor documentation. Record unsupported claims and why they were rejected; report unresolved claims as blockers.

Fix confirmed findings within scope. Use a test that fails before the fix for behavior changes; use existing focused checks for prose or data. Findings needing another design or changes to unrelated targets get a follow-up issue assigned to Chemaclass with the appropriate label. Answer rejected findings with their evidence.

After fixes, post one PR comment with applied commit IDs, follow-up issues, and rejection reasons. Follow the project's comment attribution rule.

## Gate

Before pushing, select and pass checks from repository-root `docs/internal/contributing.md`, section "Choose checks for your change". That policy owns generation, validation selection, serial execution, evidence, and reuse after fixes or merges. Check each command's own exit code. Commit with signed Conventional Commits and push without rewriting published history.

Before merging, confirm required CI and applicable remote OS checks passed on the actual current head SHA. Missing, pending, stale, or skipped required checks are not green.

## Merge and clean up

Process oldest first, prioritizing shared-file changes when that reduces conflicts.

1. Refresh the PR head, checks, review requirements, and merge state. Confirm the reviewed and tested SHA is still current.
2. Update from fresh `origin/main` only when the branch is actually behind a required current base or has merge conflicts. Merge the base into the task branch without rewriting published history, preserve both sides of shared docs, inspect the resulting diff, rerun affected validation, push, and review the new head as needed. A non-clean status alone does not require a base merge: `BLOCKED` can mean pending checks or reviews, and unknown/calculating states need a refresh.
3. Within merge authority, use `gh pr merge <N> --squash --admin --delete-branch --match-head-commit <SHA>`. If administrative merging is rejected, use the same SHA guard with `--auto --squash --delete-branch` and report that merging is queued. A queued request is not a verified merge.
4. Confirm the PR is merged and its merge commit is on fetched `origin/main`. Verify the `CI` workflow for the resulting main head passed. Resolve a red main before continuing merges.
5. Fast-forward local `main` only if its checkout is clean and it has no unpublished commits. Otherwise preserve it and report why it was not advanced. Remove the task worktree and local branch only after verifying the merge and confirming they contain no uncommitted or unpublished work and have no open PR. Delete the remote branch if merge cleanup did not. Run `git fetch --prune` and `git worktree prune`; after removing worktrees, run `~/.agnostic-ai/scripts/reap-stale-agents.sh` if it exists.

## Report blockers and outcomes

Report one table with PR, merge commit or current state, applied findings, follow-up issues, and rejected findings. Include retained task paths and next steps for blocked PRs. Stop work on a PR when a finding remains unverifiable, CI stays red after one fix, or merging remains blocked even with auto-merge. Continue independent PRs when safe.
