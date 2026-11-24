# Contributing a preset

[Contributor docs](README.md)

`init --preset <name>` writes starter specs for a language or framework into a fresh project. The bundled presets are `go`, `ts-react`, and `python`.

## Layout

```
internal/cli/initdata/presets/<name>/
├── agents/...
├── skills/...
├── rules/...
├── hooks/...
└── mcps/...
```

Include only the spec types the preset needs. [Spec format](../site/content/docs/spec-format/_index.md)

## Steps

1. Create `internal/cli/initdata/presets/<name>/`.
2. Write specs for shared language or framework conventions, without project details.
3. Add an entry to `presetExpectedFiles` in `internal/cli/init_preset_test.go`.
4. Run `go test ./internal/cli/ -run Preset` to check the expected files.
5. Try `go run ./cmd/agnostic-ai init --preset <name>` in a temporary project and inspect the generated specs.
6. Open a PR titled `feat: add <name> init preset`.

`presetFS` includes new directories automatically through `all:initdata/presets`.

## Style

- Write language or framework conventions, not project conventions. `python` says "use type hints", not "use this team's mypy config".
- One topic per spec. `python-style.md`, `pytest.md`, `typing.md`, not a giant `python.md`.
- Scope rules with `globs:` so they apply only to relevant files.
- `alwaysApply: true` for style/testing rules; `false` for narrow scopes.

## Avoid

- Files specific to a coding tool (CLAUDE.md, AGENTS.md). Adapters generate those; presets provide the source specs.
- Strict personal preferences. Presets provide a starting point for other projects.
- Files downloaded separately. Specs ship inside the binary.
