+++
title = "Jules"
description = "How agnostic-ai emits Jules configuration: native paths, capability limits, and output options."
weight = 230

[extra]
group = "Reference"
target_id = "jules"
+++

# Jules (`jules`)

## Output

```
AGENTS.md                     # canonical entry-point pointer body + inlined rules (written by sync, shared path)
```

Google [Jules](https://jules.google/docs) is a cloud agent that reads the root `AGENTS.md`. It has no project-local surface, so it gets only the shared pointer body and the inlined `## Rules` block. It adds no unique output, so it is opt-in (see [Selecting targets](@/docs/configuration.md#targets)). Agents, skills, hooks, and MCP skip with a warning.

## Config keys

None.

## Verify

1. Sign in to Jules ([docs](https://jules.google/docs)).
2. Check the tree: `ls AGENTS.md`.
3. Point Jules at the repo; it reads `AGENTS.md` as project context.
