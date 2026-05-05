# Architecture

## Code layout

```
agnostic-ai/
├── cmd/agnostic-ai/main.go         # entry point
├── internal/
│   ├── cli/                        # cobra commands (sync, validate, list,
│   │                                 init, import, doctor, revert; gitignore
│   │                                 helper, watch loop, auto-sync prompt)
│   ├── config/                     # agnostic.config.yaml loader
│   ├── spec/                       # spec file loader (md+frontmatter, yaml)
│   │                                 with per-directory scope assignment
│   └── adapters/
│       ├── adapter.go              # Adapter interface + registry
│       ├── internal/emit/          # shared emit helpers (write file,
│       │                             merged docs, rules dirs, MCP, output
│       │                             path resolvers, capture/recording)
│       ├── claude/  codex/  gemini/  cursor/  copilot/  aider/
│       ├── cline/   windsurf/  continueai/
│       └── amp/  zed/  warp/  opencode/    # 13 adapters total
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
skills/*.md ──────────┤
rules/*.md ───────────┼─► spec.LoadBundle ─► spec.Bundle (Agents, Skills,
hooks/*.yaml ─────────┤                       Rules, Hooks, MCPs; per-entry
mcps/*.yaml ──────────┘                       Scope derived from layout)

spec.Bundle ──► adapter.Emit(bundle, config, dryRun) ──► files written
                 ├─ claude:    .claude/, CLAUDE.md, .mcp.json
                 ├─ codex:     AGENTS.md (lists agents w/ pointers; nested per scope/globs),
                 │             .codex/agents/<name>.toml, .agents/skills/<name>/SKILL.md
                 ├─ cursor:    .cursor/rules/, .cursor/mcp.json
                 ├─ copilot:   .github/..., .vscode/mcp.json
                 ├─ amp/zed/warp/opencode: AGENT.md / .rules / WARP.md / .opencode/AGENTS.md
                 └─ ...
```

## Emit modes

The shared `emit` package keeps three orthogonal modes behind a mutex-
guarded `state` struct:

| Mode      | Effect | Used by |
|-----------|--------|---------|
| capture   | suppresses IO; records `(path, content)` pairs | `sync --check`, `doctor`, `revert` |
| recording | does NOT suppress IO; records paths only      | `sync` with gitignore enabled |
| backup    | copies existing `<path>` to `<path>.bak` before overwriting | `sync --backup` |

Modes are independent and can stack (e.g. recording + backup during a
gitignore-managed sync).

`sync --watch` wraps these by polling the source directories and
`agnostic.config.yaml` every 200 ms and re-running `Emit` whenever an
mtime changes.

## Core types

### `spec.Entry`

```go
type Entry struct {
    Kind  Kind             // KindAgent | KindSkill | KindRule | KindHook | KindMCP
    Name  string            // identifier
    Path  string            // source file path (for error messages and provenance)
    Scope string            // implicit per-directory scope from source layout
    Meta  map[string]any    // frontmatter or YAML fields
    Body  string            // markdown body (empty for hooks/mcps)
}
```

One spec file = one Entry. Adapters consume a `spec.Bundle` (Entries
pre-bucketed by Kind) and emit per target.

### `Adapter` interface

```go
type Adapter interface {
    Name() string
    Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error
}
```

Stateless. Construct once via `New()`, call `Emit` per sync.

### `config.Config`

Mirrors `agnostic.config.yaml`. Defaults applied in `config.Load`.
Holds `Sources` (per-kind source dirs), `Outputs` (per-target path
overrides, including `mcp-file`), the `Gitignore` block, and `AutoSync`
(`*bool`, persisted from the `--auto-sync` prompt; `nil` means the
prompt has not yet fired).

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
