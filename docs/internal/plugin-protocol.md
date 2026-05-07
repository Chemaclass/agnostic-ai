# Plugin protocol (v1)

External adapters live outside this repo as standalone binaries. The
host (`agnostic-ai`) discovers them by name on `PATH` and drives them
through a JSON-over-stdin/stdout protocol. The protocol is
language-agnostic: any binary that can read stdin and write stdout can
implement an adapter.

## Discovery

Any binary on `PATH` named:

```
agnostic-ai-adapter-<target>
```

is a candidate adapter for the target `<target>`. To opt in, list the
target in `agnostic.config.yaml`:

```yaml
targets:
  - claude
  - my-tool   # resolves to agnostic-ai-adapter-my-tool on PATH
```

The host calls the binary once per target on each `sync` (or `doctor`,
`revert` capture pass).

## Wire format

The host writes a single JSON document to the adapter's stdin and reads
a single JSON document from stdout. Stderr is reserved for adapter
diagnostics; the host surfaces it verbatim if the adapter exits
non-zero.

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

Field notes:

- `protocol_version` is always `1` for this revision. A non-1 value
  signals the host has bumped the protocol; the adapter should refuse
  by returning an `errors` entry.
- `command` is `emit` for the only currently-supported operation. New
  commands will use distinct names rather than reshape the envelope.
- `target` is the exact target name from `agnostic.config.yaml`. An
  adapter can multiplex on multiple names by using the same binary at
  different `<target>` symlinks.
- `dry_run` lets the adapter skip side effects an in-tree adapter
  would not normally perform. Most adapters can ignore this field; the
  host writes `files` itself, and dry-run is honored at the host
  level.
- `config.sources` and `config.outputs` mirror the user's
  `agnostic.config.yaml` after defaults are applied. Adapters that
  honor per-target output paths read them from `outputs[target]`.

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

Field notes:

- `protocol_version` must echo `1` from the input. The host rejects
  responses with any other value.
- `files` is the only side-effect surface. Each entry is a
  project-relative path and the full file content. The host writes the
  files through its own emit layer, which honors capture mode (used by
  `sync --check`/`doctor`), backup mode (`sync --backup`), and dry-run.
- `warnings` are surfaced on stderr prefixed with the target name.
- `errors` non-empty (or a non-zero exit code) fails the run. The
  adapter's stderr is included verbatim in the host's error message.

## Process model

- The adapter runs as a subprocess, not in-process. It is sandboxed by
  whatever the host OS already enforces for child processes.
- The host pipes stdin once, reads stdout to EOF, and waits for exit.
  Adapters must not require an interactive terminal.
- The adapter must not read or write outside what the protocol
  defines. In particular, the adapter must never write files to disk;
  the host owns all on-disk state so capture/backup/dry-run modes
  remain consistent.

## Versioning and stability

- The integer `protocol_version` is the sole compatibility signal.
  Wire-incompatible changes bump the integer.
- Backwards-compatible additions (new optional fields on Input or
  Output) do not bump the version. Adapters should ignore unknown
  fields rather than fail.
- The host ships a stable set of fields under `meta` (frontmatter from
  the spec file). Adapter-specific keys may appear here; an adapter
  reads its own namespace (e.g. `x-my-tool.<key>`) and ignores the
  rest.

## Reference helpers

Go authors implementing this protocol can import
`github.com/chemaclass/agnostic-ai/internal/adapters/external` for the
typed `Input`/`Output` structs and the `DecodeInput` / `EncodeOutput`
helpers. Other languages re-implement the JSON shape directly; the
schema is small and the field set is stable.

## Example minimal adapter (Go)

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

Build as `agnostic-ai-adapter-my-tool`, drop on `PATH`, list `my-tool`
in `agnostic.config.yaml`. The next `agnostic-ai sync` picks it up.
