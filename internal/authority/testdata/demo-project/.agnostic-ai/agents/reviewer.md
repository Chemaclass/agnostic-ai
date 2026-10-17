---
name: reviewer
description: Reviews pull requests without changing files.
can: [read, shell(git diff *), mcp:github/get_pull_request, mcp:github/list_pull_request_files]
readonly: true
mcpServers: [github]
---

Review the diff against the agreed intent and report findings.
