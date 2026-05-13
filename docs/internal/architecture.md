# Architecture

## Code layout

```
agnostic-ai/
├── cmd/agnostic-ai/main.go         # entry point
├── internal/
│   ├── cli/                        # cobra commands (sync, validate, list,
│   │                                 init, import, doctor, revert; gitignore
│   │                                 helper, watch loop, auto-sync prompt)
│   ├── config/                     # agnostic-ai.yaml loader
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

Config and specs are loaded independently, then handed to each adapter.

### Config layer

```
agnostic-ai.yaml          (committed; team-wide defaults)
  + agnostic-ai.local.yaml (optional, gitignored; per-machine overrides)
        │
        ▼  deep-merge: maps recurse, scalars and slices replace
   config.Load ──► *config.Config (defaults applied)
```

Legacy projects with `agnostic.config.yaml` still load; a one-shot stderr
warning per process prompts the rename.

### Spec layer

```
agents/*.md   ─┐
skills/*.md   ─┤
rules/*.md    ─┼─► spec.LoadBundle ─► spec.Bundle
hooks/*.yaml  ─┤                       (Agents, Skills, Rules, Hooks, MCPs;
mcps/*.yaml   ─┘                        per-entry Scope from source layout)
```

### Emit layer

```
(Config, spec.Bundle) ──► adapter.Emit(bundle, config, dryRun)
        │
        ├─ claude     .claude/, CLAUDE.md, .mcp.json, .claude/settings.json (hooks)
        ├─ codex      AGENTS.md (hierarchical), .codex/agents/<name>.toml,
        │             .agents/skills/<name>/SKILL.md, .codex/config.toml (hooks + MCP)
        ├─ gemini     GEMINI.md (hierarchical), .gemini/commands/<name>.toml,
        │             .gemini/settings.json (hooks + mcpServers)
        ├─ cursor     .cursor/rules/, .cursor/mcp.json
        ├─ copilot    .github/copilot-instructions.md (always-on),
        │             .github/instructions/<name>.instructions.md (applyTo-scoped),
        │             .vscode/mcp.json
        ├─ aider      CONVENTIONS.md
        ├─ cline      .clinerules/<name>.md
        ├─ windsurf   .windsurf/rules/<name>.md
        ├─ continue   .continue/rules/<name>.md, .continue/mcpServers/<name>.yaml
        ├─ amp        AGENTS.md (hierarchical), .agents/commands/<name>.md,
        │             .amp/settings.json (amp.mcpServers)
        ├─ zed        .rules, .zed/settings.json (context_servers)
        ├─ warp       AGENTS.md (hierarchical; agents inlined), .warp/.mcp.json
        └─ opencode   .opencode/AGENTS.md, .opencode/commands/<name>.md,
                      opencode.json (mcp map; merges with user keys)
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

`sync --watch` wraps these. The default backend is fsnotify with a 50 ms
debounce, watching every source directory plus `agnostic-ai.yaml` and
`agnostic-ai.local.yaml` when present. `--watch-poll` forces the legacy
200 ms mtime poll for filesystems where fsnotify is unreliable (some
network mounts, container volumes).

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

Mirrors `agnostic-ai.yaml`. Defaults applied in `config.Load`.
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
