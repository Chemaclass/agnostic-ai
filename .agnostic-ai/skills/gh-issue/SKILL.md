---
name: gh-issue
description: Take one GitHub issue from branch to merged PR with TDD. Use when asked to work, fix, or implement an issue by number.
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

7. **Run the gate**: `make preflight`, checked by its own exit code (never through `| grep | head`). Fix every failure before continuing.

8. **Regenerate derived artifacts when touched**:
   - Edited `internal/config/config.go` struct tags → `go run ./cmd/schemagen` (see `.agnostic-ai/skills/regen-schema/SKILL.md`).
   - Edited specs under `.agnostic-ai/` or any adapter → `go run ./cmd/agnostic-ai sync` then `go run ./cmd/agnostic-ai sync --check` (see `.agnostic-ai/skills/run-sync-check/SKILL.md`).
   - Changed what a target writes → `.agnostic-ai/rules/capability-fixtures.md`.
   - Touched code reachable from `cmd/agnostic-ai-wasm` → rebuild the playground (see `.agnostic-ai/skills/playground-rebuild/SKILL.md`).

### Phase 4: Ship

9. **Docs and changelog** for user-visible changes, per `.agnostic-ai/rules/docs-sync.md` and `.agnostic-ai/agents/changelog-curator.md`.

10. **Commit** per `.agnostic-ai/rules/conventional-commits.md`, GPG-signed, with `Related to #<issue-number>` in the body.

11. **Push and open the PR** with a body file: a short summary, decisions worth challenging, checks run, and `Closes #<issue-number>`.
    ```bash
    git push -u origin <branch-name>
    gh pr create --assignee Chemaclass --label "<bug|enhancement|documentation>" \
      --title "<type>(<scope>): <description>" --body-file "$body_file"
    ```

### Phase 5: Review and merge

12. **Review, gate, and merge** the PR with `.agnostic-ai/skills/pr-sweep/SKILL.md` steps 2 to 6: the code-reviewer agent and the Codex adversarial review until it approves, the full OS matrix when paths, permissions, or import change, then squash-merge on green and fast-forward local `main`. If `--admin` is rejected, fall back to `--auto --squash --delete-branch` and report that the PR awaits approval.
