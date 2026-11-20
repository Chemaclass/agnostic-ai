+++
title = "Approvals commands"
description = "See and stop automatic answers to tool permission prompts."
weight = 59

[extra]
group = "Reference"
+++

# Approvals commands

## approvals

Shows and stops automatic answers to permission prompts. The [`approvals` setting](@/docs/global-configuration-reference.md#global-approvals) takes `review` and `yolo` only from the home config; a project file may set `mode: ask` and nothing else.

```bash
agnostic-ai approvals status          # mode and route per tool, here
agnostic-ai approvals status --json   # the same, as JSON
agnostic-ai approvals ask             # show every prompt again
```

## approvals status

Prints one line per tool that `sync --global` writes: the mode for the working directory, the setting that chose it, and how the tool carries it out. A tool with no route for its mode falls back to `ask`, and the line says so. No tool has a route yet, so every tool shows its prompt.

`--json` prints `dir` and a `tools` list. Each item has `tool`, `mode` (what the settings ask for), `effective` (what the tool does), `from`, and `route` when one exists.

## approvals ask

Writes `approvals.mode: ask` to `$AGNOSTIC_AI_HOME/local/agnostic-ai.yaml`. That block replaces the shared one, so no tool or project keeps `review` or `yolo`. Its `always-ask` patterns stay, and so do the file's other keys and comments. It refuses a file with more than one YAML document, or an `approvals` block that carries a YAML anchor. The next request reads it; no sync or restart is needed.

A running session that a tool's own reviewer handles, such as Claude Code auto mode, keeps that mode until you switch it in the tool (Shift+Tab in Claude Code). The command says so when a tool was in `review`.
