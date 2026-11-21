### General

- `--scope user` replaces `--global` on `sync`, `list`, `import`, `migrate`, `validate`, `lint`, `explain`, `install-hook`, `view`, `serve` and `mcp tools`. `--global` still works and prints a deprecation warning on stderr. Run `install-hook --scope user` again to update an installed hook (#3856).
