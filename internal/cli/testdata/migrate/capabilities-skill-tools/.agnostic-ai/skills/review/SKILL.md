---
# Keep this comment and the body's tool spelling.
name: review
description: Review a diff.
allowed-tools: [Read(src/**), 'Edit(docs/**)', "Bash(git log --format=%h,%s)", WebFetch, WebSearch, mcp__github__get_issue, Grep] # keep the list comment
---

Read the diff with `Bash(git diff)` and report findings.
