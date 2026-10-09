---
name: adapter-pattern
description: Required shape for every adapter under internal/adapters/<target>/.
globs: "internal/adapters/**/*.go"
alwaysApply: false
---

Every adapter has the same shape: a stateless `Adapter` with `New()`, `Name()`, and `Emit(sess, bundle, cfg, dryRun)`, a `caps` value listing every spec kind the tool natively reads, and `emit.ReportUnsupported(caps, b, cfg.OnUnsupported)` first in `Emit`. Copy the shape of an existing adapter under `internal/adapters/<target>/`.

- Keep `Adapter` free of state: no fields. Create it only with `New()`.
- One concern per file. Logic two adapters share goes in `internal/adapters/internal/emit/` first.
- Resolve every output path through an `emit.Output*` helper so `outputs.<target>.<field>` can override it.
- Emit only the target-native MCP path. A portable root `.mcp.json` is opt-in via `outputs.<target>.root-mcp-file`.
- When an output is opt-in and not configured, skip it and return nil. Never write a file the user did not ask for.
- When the tool reads a native file users write by hand, add an `import` path and test the round trip: hand-written file, `import`, `sync`, same content, then `sync --check` and `lint` clean.
- Register the adapter in `internal/adapters/adapter.go` and add it to `config.DefaultTargets()`.
