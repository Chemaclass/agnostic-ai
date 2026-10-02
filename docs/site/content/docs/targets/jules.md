+++
title = "Jules"
description = "How agnostic-ai emits Jules configuration: native paths, capability limits, and output options."
weight = 230

[extra]
group = "Reference"
target_id = "jules"
+++

# Jules (`jules`)

Google [Jules](https://jules.google/docs) is a cloud agent that reads the root `AGENTS.md`. agnostic-ai writes only the shared pointer body and the inlined `## Rules` block there.

## Output

```
AGENTS.md                     # canonical entry-point pointer body + inlined rules (written by sync, shared path)
```

Jules has no project-local surface. It adds no unique output, so it is opt-in (see [Selecting targets](@/docs/configuration.md#targets)). Agents, skills, hooks, and MCP skip with a warning.

## Config keys

None.

## Protected paths

Advisory. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Sign in to Jules ([docs](https://jules.google/docs)).
2. Check the tree: `ls AGENTS.md`.
3. Point Jules at the repo; it reads `AGENTS.md` as project context.
