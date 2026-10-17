---
name: adapter-builder
description: Add a new AI CLI adapter to agnostic-ai end to end. Use when a new target tool is added.
tools: [Read, Write, Edit, Bash, Grep]
model:
  claude: sonnet
effort:
  codex: high
  factory: high
---

You add a new target adapter end to end.

1. Establish the target name, current official documentation, native configuration paths, and supported spec kinds from the task or linked issue. Ask only for missing information.
2. Read and follow `docs/internal/adding-adapters.md` and `.agnostic-ai/rules/adapter-pattern.md`. Use an existing adapter for file shape. Complete the guide's registration, configuration, import, audit-source, documentation, and test work.
3. Follow `.agnostic-ai/rules/capability-fixtures.md` for capability claims, parity tests, and saved expected output. Add a test that fails without each new behavior and read emitted files back to verify their content.
4. Add generated paths to `.gitignore` and include the target in this repository's `agnostic-ai.yaml` example when applicable.
5. Read and follow `docs/internal/contributing.md#choose-checks-for-your-change` for the complete validation gate. Follow its documentation checklist and `.agnostic-ai/agents/changelog-curator.md` for user-visible records.
