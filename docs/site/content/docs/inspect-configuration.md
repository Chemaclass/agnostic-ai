+++
title = "Inspect your configuration"
description = "Find a warning in the browser, edit its source, and sync the fix."
weight = 50

[extra]
group = "Workflows"
+++

# Inspect your configuration

Use the local browser interface to find a warning, edit the source file behind it, and sync the fix.

## Open the project

From the folder containing `agnostic-ai.yaml` or any folder inside it, run:

```bash
agnostic-ai ui
```

Keep the terminal running. The browser opens at `http://127.0.0.1:4310`, or the next free port it prints. Outside a project, or with `--global`, it opens your user configuration. See [`view`](@/docs/cli-reference/inspect.md#view) for ports and flags.

## Fix a warning

Select a section to see its files and findings from `lint` and `doctor`. Findings appear beside the affected file or at the top of the section when no file matches.

For example, a warning about bare `read` in **Authority** means the rule allows reading every file. Open the path beside the warning in your editor, or use the edit icon to open it in Cursor.

If the rule should cover only `src/`, replace `read` with `read(src/**)`:

```yaml
permissions:
  allow:
    - read(src/**)
```

Keep the other entries. Other rules and tool defaults still affect access; see [permission rules](@/docs/spec-format/settings.md#permission-rules).

## Sync and check

In another terminal in the project folder:

```bash
agnostic-ai sync --plan
agnostic-ai sync
agnostic-ai sync --check
```

Review the plan before syncing. `sync --check` should exit 0. Reload the browser to see the updated findings.

For per-agent permission decisions, browser switches, and undo, see [`serve`](@/docs/cli-reference/inspect.md#serve).
