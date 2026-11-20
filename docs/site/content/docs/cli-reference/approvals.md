+++
title = "Approvals commands"
description = "See and stop automatic answers to tool permission prompts."
weight = 59

[extra]
group = "Reference"
+++

# Approvals commands

## approvals

Shows and stops automatic answers to permission prompts. The [`approvals` setting](@/docs/global-configuration-reference.md#global-approvals) takes `review` and `yolo` only from the home config; a project file may set `mode: ask` and nothing else. [Approvals](@/docs/approvals.md) explains how to turn it on. Any other argument is an error.

```bash
agnostic-ai approvals status          # mode and route per tool, here
agnostic-ai approvals status --json   # the same, as JSON
agnostic-ai approvals ask             # show every prompt again
```

## approvals status

Prints one line per tool that `sync --global` writes: the mode for the working directory, the setting that chose it, and how the tool carries it out. A tool with no route for its mode falls back to `ask`, and the line gives the reason. The tools in [the review table](@/docs/global-configuration-reference.md#global-approvals-review) have a route for `review`: their own reviewer, which `sync --global` turns on. Claude Code, Codex and Kiro have a route for `yolo`: a hook. For `yolo` it also checks that the hook is installed and, for Codex, trusted in `/hooks`, and, for Kiro, that no Kiro rule asks; until then the tool falls back to `ask`. On Windows `yolo` falls back to `ask`. [Approvals per tool](@/docs/approvals.md#per-tool) lists the routes.

A `note:` line under a tool says what keeps its reviewer from matching the mode: the key is missing or set to another value in the user settings file (then `effective` is `ask`), Codex `approval_policy = "never"` stops Auto-review from running, or the reviewer stays on where the mode is not `review`, because `sync --global` wrote the key or, for Codex, it was set by hand.

`--json` prints `dir` and a `tools` list. Each item has `tool`, `mode` (what the settings ask for), `effective` (what the tool does), `from`, `yolo` and `review` (the tool's route for each mode, or `ask`), `reason` when the mode falls back to `ask`, `route` when one carries the mode, `min_version` when the route needs a release, and `notes` when there are any.

## approvals ask

Writes `approvals.mode: ask` to `$AGNOSTIC_AI_HOME/local/agnostic-ai.yaml`. That block replaces the shared one, so no tool or project keeps `review` or `yolo`. Its `always-ask` patterns stay, and so do the file's other keys and comments. It refuses a file with more than one YAML document, or an `approvals` block that carries a YAML anchor. The next request reads it; no sync or restart is needed.

A reviewer key that `sync --global` wrote, such as Claude Code auto mode, stays in the tool's user settings until the next `sync --global` removes it, and the command says so. A running session keeps its mode until you switch it in the tool (Shift+Tab in Claude Code).
