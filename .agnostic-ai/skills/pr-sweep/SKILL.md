---
name: pr-sweep
description: Review every open PR with the code-reviewer agent and an adversarial review, apply the findings, and merge each one once CI is green. Use when asked to ship, sweep, or merge open PRs.
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

Run two reviews on the PR branch, in parallel:

- The `code-reviewer` agent for Go diffs (other diffs against the project rules). Keep its Opus model; never pass a cheaper override.
- An adversarial review that challenges the design, not only the lines: `/codex:adversarial-review --base origin/main` where the Codex plugin is installed (Claude Code). Without it, run a second reviewer pass told to question the approach, its assumptions, and how it fails in real use.

Read every result before editing. After each round of fixes, run the adversarial review again until it approves or every remaining finding is rejected in step 4.

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

Before every push, select and pass checks under the policy in repository-root `docs/internal/contributing.md`, section "Choose checks for your change". Follow its serial execution, evidence recording, and reuse rules. Apply it again after review fixes or merges change relevant inputs. Check each command's own exit code; a filtering pipe can hide a failure.

Review tracked changes after sync and keep ignored output ignored. CHANGELOG bullets must pass the length check in `.agnostic-ai/agents/changelog-curator.md`.

Commit with conventional commits, GPG-signed, no AI attribution or session links. Push. Before merging, wait for required CI on the current head SHA, including remote OS checks selected by the validation policy. Missing or pending checks are not green.

## 6. Merge

For each PR, oldest first, or the one that changes shared files (`sources.lock`, `signals.tsv`, `CHANGELOG.md`) first:

1. Wait for checks on the current head SHA: `gh pr checks <N>`. Missing or pending checks are not green.
2. If `mergeStateStatus` is not `CLEAN`, merge `origin/main` into the branch (never rewrite pushed history), keep both sides of shared docs, rerun the gate, push, and wait again.
3. `gh pr merge <N> --squash --admin --delete-branch`. If `--admin` is rejected, use `--auto --squash --delete-branch` and report that the PR awaits approval.
4. Fast-forward local `main` from `origin/main` and delete the local branch. Stop if `main` has unpublished commits; never reset them away.

After the last merge, wait for the `CI` workflow on the new `main` head and confirm every job passed. Main pushes run Go tests on Linux. A red main is the first thing to fix.

## 7. Report

One table: PR, merge commit, findings applied, findings moved to issues, findings rejected. Then the open follow-up issues.

## Stop

Stop and report when a finding cannot be verified either way, CI stays red after one fix, or a merge is blocked even for `--auto`.
