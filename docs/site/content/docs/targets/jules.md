+++
title = "Jules"
description = "What agnostic-ai writes for Jules: file paths, what it supports, and output settings."
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

Jules has no project-level settings, so agnostic-ai writes no file for it beyond `AGENTS.md`. Jules must be enabled explicitly (see [Selecting targets](@/docs/configuration.md#targets)). Agents, skills, hooks, and MCP are skipped with a warning.

## Config keys

None.

## Protected paths

Not enforced. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Sign in to Jules ([docs](https://jules.google/docs)).
2. Check the file: `ls AGENTS.md`.
3. Point Jules at the repo. It reads `AGENTS.md` as project context.
