+++
title = "Jules"
description = "What agnostic-ai writes for Jules: file paths, what it supports, and output settings."
weight = 230

[extra]
group = "Reference"
target_id = "jules"

[extra.moved]
set-up-this-tool = "@/docs/targets/jules.md#first-rule"
+++

# Jules (`jules`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

Google [Jules](https://jules.google/docs) is a cloud agent that reads the root `AGENTS.md`. agnostic-ai writes only the shared instructions that point to source specs and the embedded `## Rules` block there.

Google announced that Jules becomes read-only after November 11, 2026 and closes on December 31, 2026. Finish tasks before the read-only cutoff and follow the [official export instructions](https://jules.google/docs/changelog/#jules-turndown) to save your Jules configuration and history.

## First rule

Select `jules` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. With the default paths, it reaches `AGENTS.md`.

Jules is sync-only: there is no importer or output for agents, skills, hooks, or MCP servers.

## Verify

Run `agnostic-ai sync --check` and inspect `AGENTS.md` for the commit-prefix rule. Before the read-only cutoff, make sure the generated `AGENTS.md` is committed to the branch Jules reads, then start a Jules task asking for a commit subject for a small fix; it should use `fix:`.

Next, [compare another target](@/docs/compare.md) if you also need skills, hooks, or MCP servers.

## Output

```
AGENTS.md                     # canonical entry-point pointer body + inlined rules (written by sync, shared path)
```

Jules has no project-level settings, so agnostic-ai writes no file for it beyond `AGENTS.md`. Jules must be enabled explicitly (see [Selecting targets](@/docs/configuration.md#targets)). Agents, skills, hooks, and MCP are skipped with a warning.

## Config keys

None.

## Protected paths

Not enforced. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).
