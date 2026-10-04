---
# Keep the key order, quotes, and inline comment.
name: review
description: Review a diff.
allowed-tools:
  - Read(src/**) # look first
  - 'Edit(docs/**)'
  - "Bash(git diff *)"
  - WebFetch
  - WebSearch
  - mcp__github__get_issue
  - Grep
---

Read the diff with `Bash(git diff)` and report findings.
