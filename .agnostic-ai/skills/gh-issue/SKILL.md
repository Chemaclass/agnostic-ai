---
name: gh-issue
description: Fetch a GitHub issue, create a branch, implement with TDD, and open a PR
argument-hint: "[issue-number]"
x-claude:
  allowed-tools: "Read, Edit, Write, Bash(gh *), Bash(git *), Bash(go *), Bash(make *), Bash(./agnostic-ai *)"
disable-model-invocation: false
---

# GitHub Issue Workflow

## Context

Read both the issue body **and every comment** as requirements input. Maintainer follow-ups frequently add scope, edge cases, or override the original description; when a later comment conflicts with the body, prefer the comment.

!`gh issue view ${ARGUMENTS#\#} --json number,url,title,body,labels,assignees,state,comments 2>/dev/null || echo "Provide an issue number"`

## Instructions

### Phase 1: Setup

1. **Parse the issue number** from `$ARGUMENTS` (strip `#` if present).

2. **Assign yourself if unassigned**:
   ```bash
   gh issue edit <number> --add-assignee @me
   ```

3. **Create a branch** from fresh `origin/main` based on the issue type:

   Check that the worktree is clean. Fetch `origin/main`, stop if local `main` has unpublished commits, then fast-forward it. Never discard local work.

   Determine the branch prefix from labels:
   - `bug` → `fix/`
   - `enhancement` → `feat/`
   - `documentation` → `docs/`
   - No label → `feat/` (default)

   Branch name format: `<prefix><issue-number>-<slug>`

   ```bash
   test -z "$(git status --porcelain)" || exit 1
   git fetch origin main
   git switch main
   test "$(git rev-list --count origin/main..main)" -eq 0 || exit 1
   git pull --ff-only
   git switch -c <branch-name>
   ```

### Phase 2: Design

4. **Design the implementation** before editing:
   - Explore the codebase to understand affected areas.
   - Identify files that need changes.
   - Respect adapter independence: `.agnostic-ai/rules/no-cross-adapter-imports.md`.
   - Honor the adapter skeleton: `.agnostic-ai/rules/adapter-pattern.md`.
   - Plan the TDD approach (what tests to write first).

5. **Keep a short implementation plan** with:
   - Summary of what the issue requires.
   - List of files to create/modify.
   - Test strategy (unit per package, integration under `tests/integration`).
   - Step-by-step implementation order.

### Phase 3: Implement

6. **Implement following TDD**:
   - Write failing tests first (`*_test.go` next to the code under test).
   - Implement minimum code to pass.
   - Refactor while keeping tests green.
   - Wrap returned errors per `.agnostic-ai/rules/error-wrapping.md`.
   - Follow `.agnostic-ai/rules/test-conventions.md` (use `t.TempDir()`, `testutil.Chdir`, behavior-named tests).

7. **Run full test suite**:
   ```bash
   go test ./...
   ```
   Fix ALL failures before proceeding.

8. **Regenerate derived artifacts when touched**:
   - Edited `internal/config/config.go` struct tags → `go run ./cmd/schemagen` (see `.agnostic-ai/skills/regen-schema/SKILL.md`).
   - Edited specs under `.agnostic-ai/` or any adapter → `go run ./cmd/agnostic-ai sync` then `go run ./cmd/agnostic-ai sync --check` (see `.agnostic-ai/skills/run-sync-check/SKILL.md`).
   - Touched code reachable from `cmd/agnostic-ai-wasm` → rebuild the playground (see `.agnostic-ai/skills/playground-rebuild/SKILL.md`).

### Phase 4: Ship

9. **Update CHANGELOG.md for user-visible changes**: add one bullet under `## [Unreleased]`, grouped as `Added`, `Changed`, `Fixed`, or `Removed` per `.agnostic-ai/rules/docs-sync.md`. Follow the entry rules in `.agnostic-ai/agents/changelog-curator.md`: one sentence, at most 160 characters, the effect a user sees, then `(#N)`. Detail goes on the docs page, not in the bullet.

10. **Update user docs when behavior is visible**:
    - New or changed flag, target, or output field → `docs/site/content/docs/targets/<target>.md` (and `target-behavior.md` for cross-target notes) and `docs/site/content/docs/configuration.md`.
    - New or changed spec field → `docs/site/content/docs/spec-format.md`.
    - New command or capability → `README.md`.

11. **Review the final diff**. Remove duplication, dead code, debug output, naming drift, and speculative abstractions. Check the rules in `.agnostic-ai/rules/`. Re-run only checks needed for review fixes.

12. **Commit changes** using Conventional Commits (`.agnostic-ai/rules/conventional-commits.md`):
    ```bash
    git add <specific-files>
    git commit -m "<type>(<scope>): <description>

    Related to #<issue-number>"
    ```
    Use `ref:` (not `refactor:`) for refactor commits. Subject under 72 chars. Body explains why, not what. Never mention AI assistance.

13. **Push and create PR**:
    Write a PR body file with a short summary, checks run, and `Closes #<issue-number>`. Set `body_file` to its path and pass it to `gh pr create`:
    ```bash
    git push -u origin <branch-name>
    gh pr create \
      --assignee Chemaclass \
      --label "<bug|enhancement|documentation>" \
      --title "<type>(<scope>): <description>" \
      --body-file "$body_file"
    ```
    Match the label to the issue type. Use `Closes #<num>` so merge auto-closes the issue.

### Phase 5: Verify & Merge

14. **Wait for CI green** on the PR:
    ```bash
    gh pr checks <pr-number-or-branch> --watch
    ```
    Fix red checks on the branch (push fixes; re-watch). Do not proceed while any required check is failing. Never `--no-verify` past a failing required check.

15. **Merge with admin bypass when possible**:
    Once every required check is green:
    ```bash
    gh pr merge <pr-number> --squash --admin --delete-branch
    ```
    If `--admin` is rejected (token lacks admin, branch protection blocks bypass), fall back to `--auto --squash --delete-branch` and surface that the PR is awaiting human approval.

16. **Sync local main** after merge:
    ```bash
    test -z "$(git status --porcelain)" || exit 1
    git fetch origin main
    git switch main
    test "$(git rev-list --count origin/main..main)" -eq 0 || exit 1
    git pull --ff-only
    ```
    Stop and report if local `main` is ahead of `origin/main`; never discard local commits.

## Checklist
- [ ] Issue fetched and understood (body + comments)
- [ ] Self-assigned
- [ ] Branch created from fresh `origin/main`
- [ ] Implementation plan checked against the issue
- [ ] Tests written first (TDD)
- [ ] Implementation complete
- [ ] `go test ./...` passes
- [ ] Derived artifacts regenerated (schema / sync / playground) when applicable
- [ ] Changelog updated under `## [Unreleased]`
- [ ] User docs updated when behavior is visible
- [ ] Final diff reviewed and committed with `Related to #<num>`
- [ ] PR created with `Chemaclass` assignee, matching label, `Closes #<num>`
- [ ] CI green (`gh pr checks --watch`)
- [ ] PR merged via `--admin --squash` (or `--auto` fallback if admin blocked)
- [ ] Local `main` synced to `origin/main`
