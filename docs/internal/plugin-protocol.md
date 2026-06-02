# Plugin protocol (v1)

External adapters live outside this repo as standalone binaries. The host (`agnostic-ai`) discovers them by name on `PATH` and drives them through a JSON-over-stdin/stdout protocol. Language-agnostic: any binary that reads stdin and writes stdout can implement an adapter.

## Discovery

Any binary on `PATH` named `agnostic-ai-adapter-<target>` is a candidate for the target `<target>`. Opt in via `agnostic-ai.yaml`:

```yaml
targets:
  - claude
  - my-tool   # resolves to agnostic-ai-adapter-my-tool on PATH
```

Host calls the binary once per target on each `sync` (or `doctor` / `revert` capture pass).

## Wire format

Host writes one JSON document to stdin and reads one JSON document from stdout. Stderr is reserved for adapter diagnostics; surfaced verbatim on non-zero exit.

### Input

```json
{
  "protocol_version": 1,
  "command": "emit",
  "target": "my-tool",
  "dry_run": false,
  "config": {
    "sources": { "agents": "agents", "skills": "skills", "rules": "rules", "hooks": "hooks", "mcps": "mcps" },
    "outputs": { "my-tool": { "file": "MY-TOOL.md" } },
    "on_unsupported": "warn",
    "targets": ["claude", "my-tool"]
  },
  "specs": {
    "agents": [],
    "skills": [],
    "rules": [
      {
        "kind": "rule",
        "name": "conventional-commits",
        "path": ".agnostic-ai/rules/conventional-commits.md",
        "scope": "",
        "layer": "project",
        "meta": { "description": "...", "globs": "**/*", "alwaysApply": true },
        "body": "Use Conventional Commits...\n"
      }
    ],
    "hooks": [],
    "mcps": []
  }
}
```

- `protocol_version`: always `1`. Other values mean the host bumped the protocol; adapter should refuse via `errors`.
- `command`: `emit` (only supported op). Future commands use distinct names.
- `target`: exact name from `agnostic-ai.yaml`. Multiplex via symlinks at multiple target names.
- `dry_run`: lets the adapter skip side effects an in-tree adapter wouldn't normally do. Host honors dry-run on its own when writing `files`.
- `config.sources` / `config.outputs`: mirror `agnostic-ai.yaml` after defaults. Adapters honoring per-target output paths read `outputs[target]`.

### Output

```json
{
  "protocol_version": 1,
  "files": [
    { "path": "MY-TOOL.md", "content": "# My Tool\n..." }
  ],
  "warnings": ["skipped 1 hook: not supported"],
  "errors": []
}
```

- `protocol_version`: must echo `1`. Host rejects other values.
- `files`: only side-effect surface. Project-relative paths + full content. Host writes through its own emit layer (capture/backup/dry-run preserved).
- `warnings`: surfaced on stderr prefixed with the target name.
- `errors` non-empty (or non-zero exit): fails the run. Adapter stderr included verbatim.

## Process model

- Adapter runs as subprocess, not in-process. Sandboxed by whatever the host OS enforces for children.
- Host pipes stdin once, reads stdout to EOF, waits for exit. No interactive terminal.
- Adapter must not write to disk. Host owns all on-disk state so capture/backup/dry-run stays consistent.

## Versioning

- `protocol_version` integer is the sole compatibility signal. Wire-incompatible changes bump it.
- Backwards-compatible additions (new optional fields) don't bump. Adapters ignore unknown fields rather than fail.
- Frontmatter under `meta` is stable. Adapter-specific keys (`x-my-tool.<key>`) belong to the adapter; ignore the rest.

## Reference helpers

Go authors: `github.com/chemaclass/agnostic-ai/internal/adapters/external` exposes typed `Input` / `Output` structs and `DecodeInput` / `EncodeOutput`. Other languages re-implement the JSON shape directly.

## Minimal Go adapter

```go
package main

import (
    "os"

    "github.com/chemaclass/agnostic-ai/internal/adapters/external"
)

func main() {
    in, err := external.DecodeInput(os.Stdin)
    if err != nil {
        os.Exit(1)
    }
    out := external.Output{
        Files: []external.File{{
            Path:    "MY-TOOL.md",
            Content: render(in.Specs.Rules),
        }},
    }
    _ = external.EncodeOutput(os.Stdout, out)
}

func render(rules []external.SpecEntry) string {
    var b []byte
    for _, r := range rules {
        b = append(b, "## "+r.Name+"\n\n"+r.Body+"\n"...)
    }
    return string(b)
}
```

Build as `agnostic-ai-adapter-my-tool`, drop on `PATH`, list `my-tool` in `agnostic-ai.yaml`. Next `agnostic-ai sync` picks it up.
