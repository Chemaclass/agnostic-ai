---
name: test-writer
description: Writes a failing test for a bug.
tools: [Read, Grep, Edit]
model:
  claude: claude-sonnet-5
  codex: gpt-6-sol
effort: medium
---

Reproduce the bug in a failing test. Do not touch production code.
