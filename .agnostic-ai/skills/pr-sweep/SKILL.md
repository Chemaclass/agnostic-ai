---
name: pr-sweep
description: Review every open PR, apply the review findings, and merge each one once CI is green.
argument-hint: "[PR-number ...] [--no-merge]"
disable-model-invocation: false
---

# PR sweep

Take every open PR from review to merge without asking between steps. With PR numbers, sweep only those. With `--no-merge`, stop at green PRs.

## 1. List

```bash
gh pr list --state open --json number,title,headRefName,additions,deletions,author
```

Skip PRs by other authors and drafts; report them. Empty list: stop.

## 2. Review

Use the `code-reviewer` agent for Go diffs when available. Review other diffs against the project rules. Independent reviews may run in parallel; read every result before editing.

## 3. Verify each finding

A finding is a claim, not a task. Before acting on one:

- Reopen the file and line it cites on the PR head.
- Reproduce behavior claims with a built binary or a failing test.
- Check vendor claims against the vendor page, not memory.

Keep what holds. Record what does not, with the reason.

## 4. Apply

Check out the PR branch in the main checkout (worktree agents cannot run git while the RTK hook rewrites it) and pull. For each confirmed finding:

- Fix it on the branch. Test first for behavior changes: the new test must fail on the old code.
- A finding that needs a different design or touches other targets gets its own issue (`gh issue create --assignee Chemaclass` with a label). Do not widen the PR.
- A finding you reject after verification is answered, not ignored.

Then post one PR comment listing the commit that applied the findings, the issues filed, and each rejection with its reason.

## 5. Gate

Before every push, all green:

- `make ci-local`; use `SKIP_JETBRAINS=1` only when Java or Gradle is unavailable, report the skip, and check JetBrains CI when its inputs changed
- `make site-test` when site content changes, counting skips, not the exit code
- `./agnostic-ai sync --check`. Drift only in gitignored generated files after a fresh checkout is a stale ledger: run `sync`, confirm 0 tracked files change.
- CHANGELOG bullets pass the length check in `.agnostic-ai/agents/changelog-curator.md`.

Commit with conventional commits, GPG-signed, no AI attribution or session links. Push.

## 6. Merge

For each PR, oldest first, or the one that moves shared files (`sources.lock`, `signals.tsv`, `CHANGELOG.md`) first:

1. Wait for checks on the current head SHA: `gh pr checks <N>`. Missing or pending checks are not green.
2. If `mergeStateStatus` is not `CLEAN`, rebase on `origin/main`, keep both sides of shared docs, rerun the gate, force-push with `--force-with-lease`, and wait again.
3. `gh pr merge <N> --squash --admin --delete-branch`.
4. Fast-forward local `main` from `origin/main` and delete the local branch. Stop if `main` has unpublished commits; never reset them away.

PR Go tests run on Linux only. After the last merge, wait for the `CI` workflow on the new `main` head and confirm every job passed, Windows included. A red main is the first thing to fix.

## 7. Report

One table: PR, merge commit, findings applied, findings moved to issues, findings rejected. Then the open follow-up issues.

## Stop

Stop and report when a finding cannot be verified either way, CI stays red after one fix, or a merge is blocked beyond `--admin`.
