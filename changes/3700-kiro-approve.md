### By tool

#### Kiro

- With approvals in `yolo`, `sync --global` writes a `PermissionRequest` hook to `~/.kiro/hooks/` that approves safe shell commands and file reads in Kiro CLI 3.4.0 or newer V3 sessions. A Kiro rule with `effect: ask` or `effect: evaluate`, or the `approve-for-me` preset, in the user or a workspace permissions file, or in an agent profile, turns the hook off, and `approvals status` says so (#3700).
