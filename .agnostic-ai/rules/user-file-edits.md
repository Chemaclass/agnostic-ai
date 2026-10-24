---
name: user-file-edits
description: Safety rules for code that edits files in the user's home under sync --global.
globs: "internal/cli/{sync_global,global_,settings_,grok_settings_,import_global}*.go"
alwaysApply: false
---

`sync --global` edits files the user and other tools also write, such as `~/.claude.json` and `~/.codex/config.toml`. Treat every change here as risky:

- Edit text in place. Never re-encode a user file: formatting, comments, and every key sync does not own stay byte for byte.
- Prove add-then-remove restores the original bytes, for a file that existed and for one sync created.
- Claim ownership of the smallest piece only: a key, a server, or a hook entry. Record it in `state/global.json`. If the user already wrote a value that means the same, take it over.
- Keep a private file private. Stop if the file changed after sync read it.
- Get a reviewer pass before merging, even for a small change.
