+++
title = "Mailbox commands"
description = "Resolve the exact Git commits and changed paths for a session mailbox review request."
weight = 48

[extra]
group = "Reference"
+++

# Mailbox commands

## mailbox refs

Run `agnostic-ai mailbox refs` anywhere inside a Git worktree to preview the JSON used by a [mailbox review request](@/docs/mailbox.md#request-a-code-review). This command only reads Git. Send the request from the companion.

```text
agnostic-ai mailbox refs [--base <ref>] [--commit <ref>]
```

| Option | Result |
|---|---|
| `--base <ref>` | Compare with this ancestor of the review commit. Without a flag the command uses `origin/HEAD` if it exists. Otherwise, name a base. |
| `--commit <ref>` | Review this commit. Defaults to `HEAD`. |

The JSON includes the worktree root, full commit and base IDs, changed paths and the number of files with uncommitted changes in the current checkout. A detached checkout has no `branch` field. Paths describe the committed diff. The command exits with an error outside a worktree or when it cannot resolve a valid comparison.

This run from a subfolder reviews one committed file against `main`. The worktree path is shortened and the JSON is formatted for readability.

```bash
agnostic-ai mailbox refs --base main
```

```json
{
  "worktree": "/tmp/mailbox-review",
  "commit": "77a7058cbd1780498d98ebdef72c162d0c7eed94",
  "base": "c9b462c24d7507ed26c44541ec3e4c4b66498e10",
  "branch": "review-example",
  "paths": ["src/store.go"],
  "uncommitted": 1
}
```
