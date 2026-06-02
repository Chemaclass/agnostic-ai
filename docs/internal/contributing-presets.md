# Contributing a preset

Presets are stack-flavored starter packs that `init --preset <name>` writes into a fresh project. Today: `go`, `ts-react`, `python`.

## Layout

```
internal/cli/initdata/presets/<name>/
├── agents/...
├── skills/...
├── rules/...
├── hooks/...
└── mcps/...
```

Omit kinds you don't need. Spec format: [docs/user/spec-format.md](../user/spec-format.md).

## Steps

1. Create `internal/cli/initdata/presets/<name>/`.
2. Write specs capturing house style (not project specifics).
3. Add an entry to `presetExpectedFiles` in `internal/cli/init_preset_test.go`.
4. `go test ./internal/cli/ -run Preset`.
5. Open a PR titled `feat: add <name> init preset`.

The `presetFS` `embed.FS` uses `all:initdata/presets`, so new dirs are picked up automatically. No registry to maintain.

## Style

- Stack-shaped, not project-shaped. `python` says "use type hints", not "use this team's mypy config".
- One topic per spec. `python-style.md`, `pytest.md`, `typing.md`, not a giant `python.md`.
- Scope rules with `globs:` so they fire only on relevant files.
- `alwaysApply: true` for style/testing rules; `false` for narrow scopes.

## Avoid

- Tooling-specific output (CLAUDE.md, AGENTS.md). Adapters generate those; presets seed sources.
- Hot opinions. Presets are a starting point, not a manifesto.
- External assets. Specs ship inside the binary.
