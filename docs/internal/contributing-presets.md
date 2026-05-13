# Contributing a preset

Presets are stack-flavored starter spec packs that `init --preset
<name>` writes into a fresh project. Today: `go`, `ts-react`, `python`.
Adding more is a small, mechanical change.

## Layout

Each preset is a directory under `internal/cli/initdata/presets/<name>/`
with the same shape as `.agnostic-ai/`:

```
internal/cli/initdata/presets/
└── <name>/
    ├── agents/...
    ├── skills/...
    ├── rules/...
    ├── hooks/...
    └── mcps/...
```

Only the kinds you need; empty subfolders are not required. Files use
the same Markdown + YAML frontmatter format as user specs (see
[Spec format](../user/spec-format.md)).

## Step by step

1. **Create the directory** under `internal/cli/initdata/presets/`.
2. **Write specs** that capture house style, not project specifics.
   Keep each spec short. Three good rules beat ten vague ones.
3. **Add an entry to `presetExpectedFiles`** in
   `internal/cli/init_preset_test.go` listing every file the preset
   ships. The snapshot test guards against accidental drift.
4. **Run the suite**: `go test ./internal/cli/ -run Preset`.
5. **Open a PR** with subject `feat: add <name> init preset`.

The `presetFS` `embed.FS` in `init.go` uses `all:initdata/presets`, so a
new directory is picked up automatically by the next build. No
registration list to maintain.

## Style guidance

- **Stack-shaped, not project-shaped.** A `python` preset says "use
  type hints", not "use this team's mypy config".
- **One topic per spec.** `python-style.md`, `pytest.md`, `typing.md`
  — not one giant `python.md`.
- **Globs.** Scope rules with `globs:` so they only fire on the
  relevant file types.
- **alwaysApply.** Default `true` for style/testing rules. Reserve
  `false` for narrow scopes.

## What to avoid

- Tooling-specific output (CLAUDE.md, AGENTS.md). Adapters generate
  those; presets seed sources.
- Opinions a thoughtful contributor would push back on. Presets are a
  starting point, not a manifesto.
- External assets. Keep specs to text. The embedded filesystem ships
  in the binary.
