---
name: agent-context
description: Set up or review project agent instructions using a concise, evidence-based context inventory.
---

# Agent Context

Use this skill when setting up a root agent file or reviewing the context
that reaches coding agents. It works in any project, with or without the
specialist agents below. Work from the project source of truth. Do not edit
generated entry points directly.

## Delegate deliberately

Read the shared [context checklist](references/context-checklist.md) for every
task. If the specialist agents are available and delegation helps, use them:

| Need | Delegate to |
|---|---|
| Create or replace a root agent setup | `agent-config-bootstrapper` |
| Review existing agent context without edits | `agent-context-reviewer` |
| Review and edit existing agent context | Reviewer, then bootstrapper with its findings |

If either agent is unavailable, do its part yourself using the steps below.
Global sync installs skills but not agents, so this fallback is required for
global use. The reviewer reports evidence; the bootstrapper makes source
changes and validates them when delegated. Do not duplicate the checklist in
either agent.

## Setup

1. Inspect the repository before writing instructions. Read its existing
   agent files, contributor guidance, build commands, and source layout.
2. Identify whether the project uses agnostic-ai. When it does, place shared
   instructions in `.agnostic-ai/AGNOSTIC_AI.md`, rules in
   `.agnostic-ai/rules/`, and reusable workflows in `.agnostic-ai/skills/`.
   Use the root entry point only when the project has no generated source.
3. Write the smallest useful root context. State the project purpose, source
   of truth, required checks, code boundaries, and generated-file policy.
   Link to detailed documents instead of copying them.
4. Add rules only for stable constraints that apply repeatedly. Put task
   procedures in skills, not in the root context.
5. Run the project's validation and, for agnostic-ai projects,
   `agnostic-ai validate` followed by `agnostic-ai sync`.

## Review

1. Build the inventory in [references/context-checklist.md](references/context-checklist.md).
   Record each file's owner, audience, scope, and whether it is generated.
2. Flag duplication, contradictions, stale commands, broad permissions, and
   instructions that cannot be verified from the repository.
3. Keep root context short. Move target-specific details into target fences,
   scoped rules, agents, or skills as appropriate.
4. Preserve user-owned and security-sensitive configuration. Never copy
   credentials, tokens, private URLs, or machine-specific paths into shared
   instructions.
5. For a review-only request, propose a focused edit. For a review-and-edit
   request, make focused source changes, regenerate outputs where applicable,
   and validate.
   Report which source files changed and which generated outputs followed.

## Decision guide

| Need | Put it in |
|---|---|
| A project-wide fact or non-negotiable convention | Root context or a rule |
| A reusable multi-step workflow | Skill |
| A delegated role with a clear responsibility | Agent |
| A directory-specific convention | Scoped rule |
| A tool-specific difference | Target fence or target override |

## Exit criteria

- Every instruction has one owner and one source of truth.
- The root context links to detail instead of repeating it.
- Generated outputs match their source specs.
- Validation commands are current and runnable from the repository root.
