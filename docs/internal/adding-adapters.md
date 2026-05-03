# Adding an adapter

Add a new AI CLI target (`foo`):

## 1. Create the package

`internal/adapters/foo/foo.go`:

```go
package foo

import (
    "github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
    "github.com/chemaclass/agnostic-ai/internal/config"
    "github.com/chemaclass/agnostic-ai/internal/spec"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string { return "foo" }

func (a *Adapter) Emit(entries []spec.Entry, cfg *config.Config, dryRun bool) error {
    // 1. Filter entries by kind
    // 2. Build output content
    // 3. Call emit.WriteFile to write
    return nil
}
```

## 2. Register

`internal/adapters/adapter.go`:

```go
import "github.com/chemaclass/agnostic-ai/internal/adapters/foo"

var registry = map[string]Adapter{
    // ...existing
    "foo": foo.New(),
}
```

Update default targets in `internal/config/config.go` and `internal/cli/init.go`.

## 3. Document capabilities

Edit `docs/user/targets.md`:

- Add a row to the capability matrix
- Add a per-target output section

## 4. Add config support

If outputs are user-configurable, add fields to `config.Output` in `internal/config/config.go`.

## 5. Add tests

`internal/adapters/foo/foo_test.go`:

```go
func TestEmit_WritesExpectedFile(t *testing.T) {
    dir := t.TempDir()
    // setup, call Emit, assert files exist with expected content
}
```

## 6. Handle unsupported kinds

For kinds the target lacks (e.g. hooks), log a warning to stderr. See `internal/adapters/codex/codex.go`.

## Conventions

- Adapter packages never import other adapters. Share via `internal/adapters/internal/emit`.
- Adapters are stateless. No globals; construct via `New()`.
- Frontmatter passes through unless the target needs transformation (e.g. Cursor `.mdc`).
- Generated files belong in the project's `.gitignore` template.
