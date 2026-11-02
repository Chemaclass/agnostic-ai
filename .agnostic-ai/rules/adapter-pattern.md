---
name: adapter-pattern
description: Shape and output boundaries for built-in target adapters.
globs: "internal/adapters/**/*.go"
alwaysApply: false
---

Built-in target adapters are stateless: no fields, `New()`, `Name()` and `Emit(sess, bundle, cfg, dryRun)`. Declare native kinds in `caps` and call `emit.ReportUnsupported(caps, b, cfg.OnUnsupported)` before writing.

- One concern per file. Shared helpers live under `internal/adapters/internal/`, output writers in `emit/`.
- Resolve every output path through an `emit.Output*` helper so `outputs.<target>.<field>` can override it.
- Emit only the target-native MCP path. A portable root `.mcp.json` is opt-in via `outputs.<target>.root-mcp-file`.
- Skip an opt-in output that is not configured and return nil.
- For a native file users write by hand, add an `import` path and test the round trip: `import`, `sync`, same content, `sync --check` and `lint` clean.
- Follow `docs/internal/adding-adapters.md` for registration; decide whether the target belongs in `config.DefaultTargets()`.
