---
name: adapter-pattern
description: Shape and output boundaries for built-in target adapters.
globs: "internal/adapters/**/*.go"
alwaysApply: false
---

Built-in target adapters are stateless: no fields, constructed with `New()`, and exposing `Name()` and `Emit(sess, bundle, cfg, dryRun)`. Declare native spec kinds in `caps` and call `emit.ReportUnsupported(caps, b, cfg.OnUnsupported)` before writing output. The external adapter bridge has a separate contract.

- One concern per file. Reuse shared helpers under `internal/adapters/internal/`; output-writing helpers belong in `emit/`.
- Resolve every output path through an `emit.Output*` helper so `outputs.<target>.<field>` can override it.
- Emit only the target-native MCP path. A portable root `.mcp.json` is opt-in via `outputs.<target>.root-mcp-file`.
- When an output is opt-in and not configured, skip it and return nil. Never write a file the user did not ask for.
- When the tool reads a native file users write by hand, add an `import` path and test the round trip: hand-written file, `import`, `sync`, same content, then `sync --check` and `lint` clean.
- Follow `docs/internal/adding-adapters.md` for registration and target lists. Decide whether the target is safe in `config.DefaultTargets()` or needs explicit selection because of native restrictions.
