# Architecture

[Contributor docs](README.md)

## Code layout

```
agnostic-ai/
├── cmd/agnostic-ai/main.go         # entry point
├── internal/
│   ├── cli/                        # cobra commands; gitignore helper; watch loop
│   ├── config/                     # agnostic-ai.yaml loader
│   ├── spec/                       # spec loader (md+frontmatter, yaml) with per-dir scope
│   └── adapters/
│       ├── adapter.go              # Adapter interface + registry
│       ├── internal/emit/          # shared write helpers (capture/recording/backup, MCP, output paths)
│       ├── header/                 # provenance-header helper shared across adapters
│       ├── claudehooks/            # Claude settings.json hook schema (shared by emit + import)
│       ├── external/               # plugin-protocol passthrough adapter
│       └── <target>/              # one package per built-in target
├── .agnostic-ai/                   # dogfood source specs
├── docs/                           # user docs, internal docs, examples
└── Makefile
```

## Data flow

The CLI loads config and source specs, selects targets, and orchestrates emission. Project specs combine pack defaults, project sources, and personal project overrides. Native global sync is a separate path. See [layered specs](../user/configuration.md#layered-specs).

### Config layer

```
agnostic-ai.yaml          (committed; team defaults)
  + agnostic-ai.local.yaml (optional, gitignored; per-machine overrides)
        │
        ▼  deep-merge: maps recurse, scalars and slices replace
   config.Load ──► *config.Config (defaults applied)
```

Legacy `agnostic.config.yaml` still loads, with a one-shot stderr rename warning.

### Spec layer

```
agents/*.md   ─┐
skills/*.md   ─┤
rules/*.md    ─┼─► spec.LoadLayered ─► spec.Bundle
hooks/*.yaml  ─┤                       (Entries pre-bucketed by Kind; per-entry Scope from layout)
mcps/*.yaml   ─┘
```

### Emit layer

```
(Config, spec.Bundle) ──► adapter.Emit(bundle, config, dryRun)
```

Per-target outputs documented in [docs/user/targets.md](../user/targets.md).

## Emit modes

The shared `emit` package keeps mode flags and buffers in a mutex-protected `Session` owned by each emission pass. Common modes include:

| Mode | Effect | Used by |
|------|--------|---------|
| capture | suppresses IO; records `(path, content)` pairs | `sync --check`, `doctor`, `revert` |
| recording | does NOT suppress IO; records paths only | `sync` with gitignore enabled |
| backup | copies `<path>` → `<path>.bak` before overwrite | `sync --backup` |

Modes stack independently (e.g. recording + backup during gitignore-managed sync).

`sync --watch` wraps these. Default backend: fsnotify with 50 ms debounce, watching every source dir plus `agnostic-ai.yaml` / `agnostic-ai.local.yaml`. `--watch-poll` forces a 200 ms mtime poll for filesystems where fsnotify is unreliable.

## Core types

### `spec.Entry`

```go
type Entry struct {
    Kind  Kind             // One of the ten kinds in spec.AllKinds
    Name  string            // identifier
    Path  string            // source file path (for errors and provenance)
    Scope string            // implicit per-dir scope from layout
    Layer string            // project source: pack:<name> | project | project-user
    Meta  map[string]any    // frontmatter or YAML fields
    Body  string            // markdown body (empty for hooks/mcps)
}

// Entry also carries MetaKeys and MetaStyles (frontmatter key order
// and YAML scalar styles) so a round-trip keeps the author's formatting.
```

One spec file = one Entry. Adapters consume `spec.Bundle` (Entries bucketed by Kind).

### `Adapter` interface

```go
type Adapter interface {
    Name() string
    Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error
}
```

Stateless. `New()` once, `Emit` per sync.

### `config.Config`

Mirrors `agnostic-ai.yaml`. See [internal/config/config.go](../../internal/config/config.go) for the full type and [Configuration](../user/configuration.md) for user-facing fields.

## Registry

`internal/adapters/adapter.go` holds the `name → Adapter` map. Adding an adapter: one registry line + new package. See [adding-adapters.md](adding-adapters.md).

## Boundaries

| Package | Responsibility |
|---------|---------------|
| `cmd/` | entry point, no logic |
| `internal/cli/` | flag parsing, orchestration |
| `internal/spec/`, `internal/config/` | parsing, no adapter knowledge |
| `internal/adapters/<target>/` | file emit, no spec parsing |
| `internal/adapters/internal/emit/` | shared write helpers |

No cross-adapter logic. Each target independent.
