---
name: adapter-pattern
description: Required shape for every adapter under internal/adapters/<target>/.
globs: "internal/adapters/**/*.go"
alwaysApply: false
---

Every adapter follows the same skeleton: a stateless `Adapter` with `New()`, `Name()`, and `Emit(sess, bundle, cfg, dryRun)`, a `caps` value listing every spec kind the tool natively reads, and `emit.ReportUnsupported(caps, b, cfg.OnUnsupported)` first in `Emit`. Copy the shape of an existing adapter under `internal/adapters/<target>/`.

- Stateless. No fields on `Adapter`. Construct via `New()` only.
- One concern per file. Logic two adapters share goes in `internal/adapters/internal/emit/` first.
- Resolve every output path through an `emit.Output*` helper so `outputs.<target>.<field>` can override it.
- Emit only the target-native MCP path. A portable root `.mcp.json` is opt-in via `outputs.<target>.root-mcp-file`.
- Skip silently (return nil) when an output is opt-in and unconfigured. Never write a surprise file.
- When the tool reads a native file users write by hand, add an `import` path and test the round trip: hand-written file, `import`, `sync`, same content, then `sync --check` and `lint` clean.
- Register the adapter in `internal/adapters/adapter.go` and add it to `config.DefaultTargets()`.
