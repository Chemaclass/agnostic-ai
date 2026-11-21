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
agnostic-ai approvals log --since 1h  # what the hook decided in the last hour
```

## approvals status

Prints one line per tool that `sync --scope user` writes: the mode for the working directory, the setting that chose it, and how the tool carries it out. A tool with no route for its mode falls back to `ask`, and the line gives the reason. The tools in [the review table](@/docs/global-configuration-reference.md#global-approvals-review) have a route for `review`: their own reviewer, which `sync --scope user` turns on. Claude Code, Codex, Kiro, Qwen Code and Qoder have a route for `yolo`: a hook. For `yolo` it also checks that the hook is installed and, for Codex, trusted in `/hooks`, and, for Kiro, Qwen Code and Qoder, that no rule of theirs asks; until then the tool falls back to `ask`. On Windows `yolo` falls back to `ask`. [Approvals per tool](@/docs/approvals.md#per-tool) lists the routes.

A `note:` line under a tool says what keeps its reviewer from matching the mode: the key is missing or set to another value in the user settings file (then `effective` is `ask`), Codex `approval_policy = "never"` stops Auto-review from running, or the reviewer stays on where the mode is not `review`, because `sync --scope user` wrote the key or, for Codex, it was set by hand. For Claude Code in `yolo`, a note says that ask rules passed with `--settings` or `--allowedTools` are not read.

`--json` prints `dir` and a `tools` list. Each item has `tool`, `mode` (what the settings ask for), `effective` (what the tool does), `from`, `yolo` and `review` (the tool's route for each mode, or `ask`), `reason` when the mode falls back to `ask`, `route` when one carries the mode, `min_version` when the route needs a release, and `notes` when there are any.

## approvals ask

Writes `approvals.mode: ask` to `$AGNOSTIC_AI_HOME/local/agnostic-ai.yaml`. That block replaces the shared one, so no tool or project keeps `review` or `yolo`. Its `always-ask` patterns stay, and so do the file's other keys and comments. It refuses a file with more than one YAML document, or an `approvals` block that carries a YAML anchor. The next request reads it; no sync or restart is needed.

A reviewer key that `sync --scope user` wrote, such as Claude Code auto mode, stays in the tool's user settings until the next `sync --scope user` removes it, and the command says so. A running session keeps its mode until you switch it in the tool (Shift+Tab in Claude Code).

## approvals log

Prints each request the approvals hook saw, oldest first: the time, the tool, `approved` or `asked` with the rule that asked, the call, and a detail with anything that may hold a credential left out. It reads `$AGNOSTIC_AI_HOME/local/approvals/decisions.jsonl` and the older file beside it, as [the decision log](@/docs/approvals.md#the-decision-log) describes.

```bash
agnostic-ai approvals log                   # every logged decision
agnostic-ai approvals log --since 30m       # the last 30 minutes
agnostic-ai approvals log --since 1h --json # as a JSON array of log lines
```

`--since` takes a duration such as `90s`, `30m` or `2h`. `--json` prints the log lines as they are stored, with the same fields.
