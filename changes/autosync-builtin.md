### General

- `builtins: [autosync]` adds a session-start hook to every tool that has one. It runs `sync --keep-edits`, so a fresh clone, pull, rebase or new worktree starts with current tool files. It never fails the session, stays quiet when another sync runs, and names only the files it kept. See [sync on session start](https://agnostic-ai.org/docs/autosync/).
- `install-hook --post-checkout` also writes `post-rewrite`, so a rebase, `pull --rebase` included, syncs the rebased tree.
- `install-hook` writes no hook in a project that uses lefthook, husky or pre-commit. It prints the lines to add to that tool's config instead. `--force` writes the plain hooks anyway.
