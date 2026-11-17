+++
title = "Jules"
description = "Share project rules with Jules through AGENTS.md, then check the file it reads."
weight = 230

[extra]
group = "Reference"
target_id = "jules"

[extra.moved]
set-up-this-tool = "@/docs/targets/jules.md#first-rule"
+++

# Jules (`jules`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

Google [Jules](https://jules.google/docs) is a cloud agent that reads the root `AGENTS.md`. agnostic-ai writes only the shared instructions that point to source specs and the embedded `## Rules` block there.

Jules has been read-only since November 12, 2026 and closes on December 31, 2026. New tasks cannot start. Save your configuration and history with the [official export instructions](https://jules.google/docs/changelog/#jules-turndown).

## First rule

1. Select `jules` in `agnostic-ai.yaml`.
2. Follow [First rule](@/docs/getting-started.md#first-rule). The commit-prefix rule becomes a section in `AGENTS.md`.

Jules is sync-only. It has no importer, and no output for agents, skills, hooks, or MCP servers. Those are skipped with a warning.

## Output

| Source | Default output |
|---|---|
| Shared instructions and rules | `AGENTS.md` (shared path) |

Jules has no project-level settings, so nothing else is written. It has no config keys. Enable it explicitly ([Selecting targets](@/docs/configuration.md#targets)).

## Verify

1. Run `agnostic-ai sync --check`.
2. Look for the commit-prefix rule in `AGENTS.md`.
3. Commit the file to the branch Jules works on. It reads `AGENTS.md` from there.

## Protected paths

Not enforced. This target takes no settings specs. Sync reports a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [compare another target](@/docs/compare.md) if you also need skills, hooks, or MCP servers.
