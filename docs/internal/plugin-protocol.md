# Plugin protocol (v1)

[Contributor docs](README.md)

External adapters are programs maintained outside this repository. agnostic-ai finds them by name on `PATH`, sends JSON through standard input, and reads JSON from standard output. You can write an adapter in any language.

## Discovery

Any binary on `PATH` named `agnostic-ai-adapter-<target>` is a candidate for target `<target>`. Enable it in `agnostic-ai.yaml`:

```yaml
targets:
  - claude
  - my-tool   # resolves to agnostic-ai-adapter-my-tool on PATH
```

agnostic-ai runs the program once per target on each `sync`, or when `doctor` / `revert` renders output for comparison.

## Wire format

agnostic-ai writes one JSON document to standard input and reads one from standard output. Keep log messages on standard error; agnostic-ai shows them unchanged when the adapter exits with a nonzero status.

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

| Field | Meaning |
|---|---|
| `protocol_version` | Always `1`. Other values mean the host bumped the protocol; the adapter should refuse via `errors`. |
| `command` | `emit` (only supported command). Future commands use distinct names. |
| `target` | Exact name from `agnostic-ai.yaml`. One program can handle multiple targets through symbolic links with different target names. |
| `dry_run` | Lets the adapter skip actions that a built-in adapter would not perform. Host honors dry-run on its own when writing `files`. |
| `config.sources` / `config.outputs` | Mirror `agnostic-ai.yaml` after defaults. Adapters honoring per-target output paths read `outputs[target]`. |
| `specs.*[].asset_dir` | Folder whose sibling files ship with a skill; `path`'s folder for any folder skill. Absent for flat files. |
| `specs.*[].source_path` | The file the author edits, set only when it differs from `path`. A local skill that edits fields of a shared skill keeps the shared `SKILL.md` as `path`, so an adapter reading assets from `path`'s folder still ships them. |

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

| Field | Meaning |
|---|---|
| `protocol_version` | Must echo `1`. Host rejects other values. |
| `files` | Files to write, each with a project-relative path and full content. agnostic-ai writes them through its shared helpers, preserving comparison, backup, and dry-run behavior. |
| `warnings` | Shown on standard error, prefixed with the target name. |
| `errors` | Non-empty (or non-zero exit) fails the run. Adapter stderr included verbatim. |

## Process model

- The adapter runs as a child process with only the operating system's child-process restrictions.
- agnostic-ai sends input once, reads output until the stream ends, and waits for the program to exit. No interactive terminal.
- Adapter must not write to disk. agnostic-ai handles all file writes so comparison, backup, and dry-run behavior stays consistent.

## Versioning

- `protocol_version` integer is the sole compatibility signal. Changes that break the input or output format increase this number.
- Compatible additions, such as optional fields, keep the same number. Adapters ignore unknown fields rather than fail.
- YAML header fields (frontmatter) under `meta` are stable. Adapter-specific keys (`x-my-tool.<key>`) belong to the adapter; ignore the rest.

## Reference helpers

Define local types for the JSON fields your adapter reads and writes. The Go types under this repository's `internal/` directory are implementation details; Go does not allow unrelated modules to import them. The protocol above works without a library dependency.

## Minimal Go adapter

Save this as `main.go` in a new directory. This example renders rule names and bodies into `MY-TOOL.md`, rejects unsupported protocol versions and commands, and ignores unknown JSON fields.

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type rule struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

type input struct {
	ProtocolVersion int    `json:"protocol_version"`
	Command         string `json:"command"`
	Specs           struct {
		Rules []rule `json:"rules"`
	} `json:"specs"`
}

type file struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type output struct {
	ProtocolVersion int      `json:"protocol_version"`
	Files           []file   `json:"files,omitempty"`
	Errors          []string `json:"errors,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var in input
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		return fmt.Errorf("decode input: %w", err)
	}
	out := output{ProtocolVersion: 1}
	switch {
	case in.ProtocolVersion != 1:
		out.Errors = []string{"unsupported protocol_version"}
	case in.Command != "emit":
		out.Errors = []string{"unsupported command"}
	default:
		var body strings.Builder
		for _, r := range in.Specs.Rules {
			fmt.Fprintf(&body, "## %s\n\n%s\n", r.Name, r.Body)
		}
		out.Files = []file{{Path: "MY-TOOL.md", Content: body.String()}}
	}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		return fmt.Errorf("encode output: %w", err)
	}
	return nil
}
```

Build it as a standalone module:

```sh
go mod init example.com/my-tool
go build -o agnostic-ai-adapter-my-tool .
```

Put the binary on `PATH` and list `my-tool` in `agnostic-ai.yaml`. The next `agnostic-ai sync` runs it and writes the returned file.
