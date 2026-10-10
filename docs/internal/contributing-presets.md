# Contributing a preset

[Contributor docs](README.md)

Presets are starter specs for a language or framework that `init --preset <name>` writes into a fresh project. Today: `go`, `ts-react`, `python`.

## Layout

```
internal/cli/initdata/presets/<name>/
├── agents/...
├── skills/...
├── rules/...
├── hooks/...
└── mcps/...
```

Leave out spec types you do not need. Spec format: [docs/site/content/docs/spec-format/_index.md](../site/content/docs/spec-format/_index.md).

## Steps

1. Create `internal/cli/initdata/presets/<name>/`.
2. Write specs for shared language or framework conventions, without project details.
3. Add an entry to `presetExpectedFiles` in `internal/cli/init_preset_test.go`.
4. `go test ./internal/cli/ -run Preset`.
5. Open a PR titled `feat: add <name> init preset`.

`presetFS` uses `all:initdata/presets`, so new directories are included automatically. No registration step is needed.

## Style

- Write language or framework conventions, not project conventions. `python` says "use type hints", not "use this team's mypy config".
- One topic per spec. `python-style.md`, `pytest.md`, `typing.md`, not a giant `python.md`.
- Scope rules with `globs:` so they apply only to relevant files.
- `alwaysApply: true` for style/testing rules; `false` for narrow scopes.

## Avoid

- Files specific to a coding tool (CLAUDE.md, AGENTS.md). Adapters generate those; presets provide the source specs.
- Strict personal preferences. Presets provide a starting point for other projects.
- Files downloaded separately. Specs ship inside the binary.
