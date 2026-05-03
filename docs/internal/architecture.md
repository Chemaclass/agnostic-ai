# Architecture

## Code layout

```
agnostic-ai/
├── cmd/agnostic-ai/main.go         # entry point
├── internal/
│   ├── cli/                        # cobra commands (sync, validate, list, init)
│   ├── config/                     # agnostic.config.yaml loader
│   ├── spec/                       # spec file loader (md+frontmatter, yaml)
│   └── adapters/
│       ├── adapter.go              # Adapter interface + registry
│       ├── internal/emit/          # shared emit helpers
│       ├── claude/                 # Claude Code adapter
│       ├── codex/                  # Codex adapter
│       ├── gemini/                 # Gemini CLI adapter
│       └── cursor/                 # Cursor adapter
├── .agnostic-ai/                # dogfood source specs (this repo's own AI config)
├── docs/                        # user docs, internal docs, examples/
└── Makefile
```

## Data flow

```
agnostic.config.yaml ─┐
                      ├─► config.Load ─► Config
                      │
agents/*.md ──────────┤
skills/*.md ──────────┼─► spec.LoadAll ─► []spec.Entry
rules/*.md ───────────┤
hooks/*.yaml ─────────┘

[]spec.Entry ──► adapter.Emit(entries, config) ──► files written
                  ├─ claude:  .claude/, CLAUDE.md
                  ├─ codex:   AGENTS.md
                  ├─ gemini:  GEMINI.md
                  └─ cursor:  .cursor/rules/
```

## Core types

### `spec.Entry`

```go
type Entry struct {
    Kind Kind            // KindAgent | KindSkill | KindRule | KindHook
    Name string          // identifier
    Path string          // source file path (for error messages)
    Meta map[string]any  // frontmatter or YAML fields
    Body string          // markdown body (empty for hooks)
}
```

One spec file = one Entry. Adapters consume `[]Entry` and emit per target.

### `Adapter` interface

```go
type Adapter interface {
    Name() string
    Emit(entries []spec.Entry, cfg *config.Config, dryRun bool) error
}
```

Stateless. Construct once, call `Emit` per sync.

### `config.Config`

Mirrors `agnostic.config.yaml`. Defaults applied in `config.Load`.

## Registry

`internal/adapters/adapter.go` holds a `name -> Adapter` map. Adding an adapter: one registry line plus a new package. See [adding-adapters.md](adding-adapters.md).

## Boundaries

| Package | Responsibility |
|---|---|
| `cmd/` | entry point, no logic |
| `internal/cli/` | flag parsing, orchestration |
| `internal/spec/`, `internal/config/` | parsing, no adapter knowledge |
| `internal/adapters/<target>/` | file emit, no spec parsing |
| `internal/adapters/internal/emit/` | shared write helpers |

No cross-adapter logic. Each target is independent.
