---
# Keep this comment: migrations must not reformat frontmatter.
name: reviewer
description: Reviews a diff.
tools: [Read, Grep, Bash(git diff *), mcp__github, mcp__github__get_issue, WebFetch, WebSearch]   # least privilege
---

Review the diff and report findings with `file:line`.
