+++
title = "Inspect your configuration"
description = "Find a warning in the browser, edit its source, and sync the fix."
weight = 85

[extra]
group = "Workflows"
+++

# Inspect your configuration

Find a warning in the local browser interface, edit its source, and sync the fix.

## Open the project

From the folder containing `agnostic-ai.yaml` or any folder inside it, run:

```bash
agnostic-ai view
```

Keep the terminal running. The browser opens at `http://127.0.0.1:4310`. Outside a project, `view` opens user configuration. In a project, use the header's **Project** / **Global** switch to move to your user configuration and back, or add `--global` to open only the user configuration.

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

Review the plan before syncing. `sync --check` should exit 0. Reload the browser to see the updated rules and findings.

For per-agent permission decisions, use the [Authority API](@/docs/cli-reference/inspect.md#serve).

For browser switches, see [saving changes and undo](@/docs/cli-reference/inspect.md#serve).
