---
name: user-file-edits
description: Safety rules for code that edits files in the user's home under sync --global.
globs: "internal/cli/{sync_global,global_,settings_json_edit,settings_toml_edit,import_global}*.go"
alwaysApply: false
# Only tools that load a rule by its globs, so it costs nothing per session.
targets: [claude, cursor]
---

`sync --global` edits files the user and other tools also write, such as `~/.claude.json` and `~/.codex/config.toml`. Treat every change here as risky:

- Edit text in place. Never re-encode a user file: formatting, comments, and every key sync does not own stay byte for byte.
- Prove add-then-remove restores the original bytes, for a file that existed and for one sync created.
- Own the smallest unit: a key, a server, a hook entry. Record it in `state/global.json`, and adopt a hand-written value that means the same.
- Keep a private file private, and stop if the file changed after sync read it.
- Get a reviewer pass before merging, even for a small change. Every review of this code so far found a real bug.
