+++
title = "Inspect your configuration"
description = "Find a warning in the browser, edit its source, and sync the fix."
weight = 50

[extra]
group = "Workflows"
+++

# Inspect your configuration

Use the local browser interface to find warnings and the source files to fix.

## Open the project

From the folder containing `agnostic-ai.yaml` or any folder inside it, run:

```bash
agnostic-ai ui
```

Keep the terminal running. The browser opens at `http://127.0.0.1:4310`, or the next free port it prints. Outside a project, or with `--scope user`, it shows your user configuration. Ports and flags: [`view`](@/docs/cli-reference/serve.md#view).

## Fix a warning

Select a section to see its files and the `lint` and `doctor` problems, beside the affected file or at the top when no file matches.

Example: a warning about bare `read` in **Authority** means the rule allows reading every file. Open the path in your editor, or use the edit icon (editor set in Settings > Editor). To cover only `src/`, use `read(src/**)`:

```yaml
permissions:
  allow:
    - read(src/**)
```

Keep the other entries. Other rules and tool defaults still affect access ([permission rules](@/docs/spec-format/settings.md#permission-rules)).

## Sync and check

In another terminal in the project folder:

```bash
agnostic-ai sync --plan
agnostic-ai sync
agnostic-ai sync --check
```

Review the plan first. `sync --check` exits 0 once files match. Reload the page to see updated findings; the view and search stay in the address bar, unsaved input is lost.

- Per-agent permission decisions, browser switches, and undo: [`serve`](@/docs/cli-reference/serve.md#serve).
- Voice dictation: in `agnostic-ai ui --scope user`, open **Settings > Preferences** to turn it on, choose hold or tap, and set the language. Then run `agnostic-ai sync --scope user`.
