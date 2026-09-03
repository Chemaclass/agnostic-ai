---
name: adapter-pattern
description: Required shape for every adapter under internal/adapters/<target>/.
globs: "internal/adapters/**/*.go"
alwaysApply: true
---

Every adapter follows the same skeleton. Read any existing file under `internal/adapters/<target>/` for a working reference.

```go
package <target>

const (
    target         = "<target>"
    defaultOutFile = "..."
)

var caps = emit.Capabilities{
    Target:   target,
    Supports: []spec.Kind{ /* declare every kind the CLI natively understands */ },
}

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (Adapter) Name() string { return target }

func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
    if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
        return err
    }
    // ... write per-kind output via emit helpers ...
    return nil
}
```

Conventions:

- Stateless. No fields on `Adapter`. Construct via `New()` only.
- One concern per file. If you need shared logic between two adapters, lift it into `internal/adapters/internal/emit/` first.
- Resolve every output path through an `emit.Output*` helper so the user can override via `outputs.<target>.<field>` in the config.
- MCP server files: emit only the target-native path (for example Cursor's `.cursor/mcp.json`). Some hosts such as the VS Code Agent Host read a workspace `.mcp.json` at the project root natively, but VS Code forwards non-interactive MCP servers there, so mirroring the same servers to a root `.mcp.json` by default would be a surprise file. If a portable root file is wanted, make it opt-in via `outputs.<target>.root-mcp-file`.
- Skip silently (return nil) when an output is opt-in and unconfigured. Never write a surprise file.
- Register the adapter in `internal/adapters/adapter.go` and add it to `config.DefaultTargets()`.
