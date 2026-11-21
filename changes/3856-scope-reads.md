### General

- `list --scope local` lists only your personal specs in a project. `list` adds a fourth column, `overrides:<layer>`, to a local spec that changes a shared one, and `--json` adds an `overrides` object (#3856).
- Layer names changed in `list`, `status`, `explain --layers` and the web interface: `project-user` is `local`, `global` is `user`, and `global-local` is `user-local`. Update scripts that match the old names. External adapters receive the new names in each spec's `layer` (#3856).
- `why`, `status`, `render`, `compare`, `graph`, `revert` and `cleanup` take `--scope project`, and name the user-level command to run instead when asked for `--scope user` (#3856).
