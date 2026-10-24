---
name: agent-context
description: Set up or review project agent instructions for clear ownership, useful scope, and current commands.
---

# Agent Context

Review the context that reaches coding agents, then make the requested source changes. Keep a review-only request read-only. Use the project's canonical sources; never edit generated entry points.

## Inventory and review

Read `.agnostic-ai/skills/agent-context/references/context-checklist.md`. Inventory only the requested scope and its dependencies. Inspect repository instructions, contributor guidance, source layout, and validation commands before recommending changes.

Trace generated files to their sources. Report duplication, contradictions, stale commands, broad permissions, and unsupported claims with `path:line` evidence and the smallest useful correction. Preserve user-owned configuration and private material.

When delegation helps, use `agent-context-reviewer` for findings and `agent-config-bootstrapper` for accepted edits. Give each a bounded scope, source paths, ownership, and validation budget. Keep independent edits disjoint and expensive checks with one coordinator. If those roles are unavailable, follow this procedure directly.

## Edit

1. Put shared project context in `.agnostic-ai/AGNOSTIC_AI.md`, stable conventions in `rules/`, procedures in `skills/`, and delegated responsibilities in `agents/`. Check `agnostic-ai.yaml` for source overrides. Use a root native file only when the project has no generated source.
2. Keep root context to project purpose, source of truth, validation routing, and boundaries. Link to existing detailed guidance instead of copying it.
3. Keep each instruction with one owner. Preserve non-obvious safety and compatibility requirements when removing duplication. Read references only when the task needs them.
4. Check referenced files and commands against the repository. Verify target-specific claims from current official documentation only when the recommendation depends on them.
5. Follow the project's validation policy. In this repository, use `go run ./cmd/agnostic-ai validate`, `lint`, `sync`, and `sync --check`; `docs/internal/contributing.md#choose-checks-for-your-change` owns additional checks. Review tracked output changes and keep ignored outputs ignored.

## Placement

| Need | Source |
|---|---|
| Project-wide fact or boundary | Root context or a rule |
| Reusable procedure | Skill |
| Delegated responsibility | Agent |
| Directory-specific convention | Scoped rule |
| Tool-specific difference | Target fence or target override |

Finish with the source files changed, behavior clarified, checks and limits. Confirm that generated guidance matches its sources and that referenced resources remain reachable from each emitted location. A clean sync proves matching output, not useful instructions.
