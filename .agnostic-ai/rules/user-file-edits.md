---
name: user-file-edits
description: Safety rules for code that edits files in the user's home under sync --global.
globs: "internal/cli/{sync_global,global_,settings_,grok_settings_,import_global}*.go"
alwaysApply: false
---

`sync --global` edits files the user and other tools also write (`~/.claude.json`, `~/.codex/config.toml`):

- Edit text in place; never re-encode. Formatting, comments and keys sync does not own stay byte for byte.
- Prove add-then-remove restores the original bytes, for an existing file and one sync created.
- Own the smallest piece (key, server or hook entry); record it in `state/global.json`. Take over an equal user value.
- Keep a private file private; stop if it changed after sync read it.
- Get a reviewer pass before merging, even a small change.
