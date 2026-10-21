+++
title = "Hooks"
description = "hooks/: commands the tool runs at events such as before a shell command or when a session starts."
weight = 50

[extra]
group = "Reference"

[extra.moved]
per-target-body-fences = "@/docs/spec-format/_index.md#per-target-body-fences"
portable-events = "@/docs/spec-format/hook-reference.md#portable-events"
decision-on-stdout = "@/docs/spec-format/hook-reference.md#decision-on-stdout"
claude-settings-copies = "@/docs/spec-format/hook-reference.md#claude-settings-copies"
native-events = "@/docs/spec-format/hook-reference.md#native-events"
fields = "@/docs/spec-format/hook-reference.md#fields"
events = "@/docs/spec-format/hook-reference.md#events"
imported-project-root-paths = "@/docs/spec-format/hook-reference.md#imported-project-root-paths"
hook-target = "@/docs/spec-format/hook-reference.md#hook-target"
edited-paths = "@/docs/spec-format/hook-reference.md#edited-paths"
spec-guard = "@/docs/spec-format/hook-reference.md#spec-guard"
shared-hook-scripts = "@/docs/spec-format/hook-recipes.md#shared-hook-scripts"
hook-run = "@/docs/cli-reference/hook-diagnostics.md#hook-run"
assumed-results = "@/docs/cli-reference/hook-diagnostics.md#assumed-results"
kiro-hook-run-matchers = "@/docs/cli-reference/hook-diagnostics.md#kiro-hook-run-matchers"
sources = "@/docs/cli-reference/hook-diagnostics.md#sources"
+++

# Hooks

## Per target body fences {#per-target-body-fences .legacy-anchor}

## Portable events {#portable-events .legacy-anchor}

## Decision on stdout {#decision-on-stdout .legacy-anchor}

## Claude settings copies {#claude-settings-copies .legacy-anchor}

## Native events {#native-events .legacy-anchor}

## Fields {#fields .legacy-anchor}

## Events {#events .legacy-anchor}

## Imported project root paths {#imported-project-root-paths .legacy-anchor}

## Hook target {#hook-target .legacy-anchor}

## Edited paths {#edited-paths .legacy-anchor}

## Spec guard {#spec-guard .legacy-anchor}

## Shared hook scripts {#shared-hook-scripts .legacy-anchor}

## Hook run {#hook-run .legacy-anchor}

## Assumed results {#assumed-results .legacy-anchor}

## Kiro hook run matchers {#kiro-hook-run-matchers .legacy-anchor}

## Sources {#sources .legacy-anchor}


`hooks/` holds commands the AI tool runs at fixed points: when a session starts, before a tool call, after a file edit, when the agent stops. Hooks must be enabled and trusted in that tool.

## Write one

Create the source file, then replace its contents with the spec below:

```bash
agnostic-ai new hook session-status
```

`.agnostic-ai/hooks/session-status.yaml`:

```yaml
name: session-status
description: Show the working changes when a session starts.
targets: [claude, codex]
on: session-start
command: git status --short
```

This command reports Git state and blocks nothing. Run it from a Git project. Sync, then test the generated command:

```bash
agnostic-ai lint
agnostic-ai sync
agnostic-ai hook run session-status
agnostic-ai sync --check
```

For a guard, use `on: before-tool` and a script that reads event JSON on stdin. Exit 0 allows the call to continue; exit 2 blocks supported tool calls with stderr as the reason. Read the [event and failure contract](@/docs/spec-format/hook-reference.md#portable-events) before you rely on a guard.

Hook scripts run with your tools' access, so review hook specs like code. Sync never runs a hook; the configured tools do, and [`agnostic-ai hook run`](@/docs/cli-reference/hook-diagnostics.md#hook-run) runs one when you ask. A written configuration does not prove a hook ran.

[`agnostic-ai compare claude cursor`](@/docs/cli-reference/inspect.md#compare) shows how each tool writes hook events, matchers, commands, timeouts, and failure policies. The report names missing mappings, target exclusions, and ignored options.

## Choose the next step

- [Hook reference](@/docs/spec-format/hook-reference.md): portable events, native handlers, fields, scripts, and event input.
- [Hook recipes](@/docs/spec-format/hook-recipes.md): the demo force-push guard and shared script locations.
- [Test a hook](@/docs/cli-reference/hook-diagnostics.md#hook-run): sample payloads, results, and limits of the diagnostic.
- [Target pages](@/docs/targets/_index.md): discovery, trust, and native hook behavior.
