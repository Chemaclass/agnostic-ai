### By tool

#### Qwen Code

- With approvals in `yolo`, `sync --global` writes a `PermissionRequest` hook to `~/.qwen/settings.json` that approves safe shell commands, file reads, edits and writes in `default` and `auto_edit` mode; set `tools.approvalMode` to one of them, since Qwen Code runs auto mode in a trusted folder otherwise. A `permissions.ask` rule in the user, project or system settings turns the hook off, since a hook allow skips its prompt, and `approvals status` says so (#3700).
