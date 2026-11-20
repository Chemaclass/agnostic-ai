### By tool

#### Qoder

- With approvals in `yolo`, `sync --global` writes a `PermissionRequest` hook to `~/.qoder/settings.json` that approves safe `Bash`, `Read`, `Edit` and `Write` calls in `default` and `accept_edits` mode. A `permissions.ask` rule in the user or project settings turns the hook off, since a hook allow skips its prompt, and `approvals status` says so (#3700).
