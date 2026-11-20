### General

- `init --demo` writes its example agent with `can:` and a per-tool model, so a fresh demo project passes `lint` and `doctor` offers no migration. Only Claude Code now gets the demo agent's `sonnet` model; other tools use their default (#3798).
