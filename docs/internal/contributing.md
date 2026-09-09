# Development workflow

[Contributor docs](README.md) · [Setup and first contribution](../../CONTRIBUTING.md)

Use this page after building the CLI. It covers checks, conventions, and generated artifacts without repeating checkout setup.

## Development loop

Run the smallest relevant example while implementing, then batch the required checks once the change is complete.

```bash
go run ./cmd/agnostic-ai validate
go run ./cmd/agnostic-ai list
go run ./cmd/agnostic-ai sync -t claude --dry-run
```

Use a temporary project when experimenting with imported or generated files. Adapter tests should pass a small bundle to `Emit`, then read the resulting files back from disk.

## Choose checks for your change

| Change | Checks |
|---|---|
| Go behavior | Focused package tests during development; `make preflight` before submission |
| Concurrency | `make test-race` |
| Shell scripts or CLI end-to-end behavior | `make test-shell` (requires bashunit on PATH) |
| Config struct or schema fields | `go run ./cmd/schemagen`; include the updated schema |
| Project source specs | `go run ./cmd/agnostic-ai lint`, then `go run ./cmd/agnostic-ai sync` |
| Adapter output | Adapter tests and relevant golden fixtures; preview with `sync --dry-run` |
| Playground or code used by WASM | `make playground-serve`, then exercise affected behavior in a browser |
| Documentation | Check relative links and anchors; execute changed command examples in a temporary project |
| Editor extension | Follow its [development guide](../../editors/README.md) and CI job |

`make preflight` covers formatting, vet, lint, and Go tests. It does not run every job in [CI](../../.github/workflows/ci.yml), including race tests, shell tests, schema drift, WASM builds, and extension builds.

## Conventions

| Topic | Convention |
|---|---|
| Format | `gofmt`; use `goimports` for import grouping |
| Dependencies | Prefer the standard library; justify new dependencies |
| Adapters | Stateless, constructed with `New()`; no imports between target packages |
| Shared output code | Put it in `internal/adapters/internal/emit/` |
| Tests | Behavior names; `t.TempDir()` and `testutil.Chdir(t, dir)` for filesystem tests; no external mocking libraries |
| Errors | Wrap with file or operation context and `%w` |
| Scope | One concern per PR; separate unrelated refactors |

## Generated project configuration

The repository's `.agnostic-ai/` specs generate root entry points and tool folders. Those outputs are ignored. Never edit or commit them as source.

The repository's output ignore block is maintained by hand. Preserve hierarchical patterns such as `**/AGENTS.md` and the `!internal/adapters/*/testdata/**` exception that keeps golden fixtures tracked. Add new adapter paths to [.gitignore](../../.gitignore).

CI lints source specs. It does not run `sync --check` against a fresh checkout because generated files are not committed. After generating locally, `sync --check` can confirm local output matches the specs.

## Documentation checklist

- New or changed flags, targets, or output fields: update [targets](../user/targets.md) and [configuration](../user/configuration.md), plus the [CLI reference](../user/cli-reference.md) for command changes.
- New or changed spec fields: update [spec format](../user/spec-format.md).
- Config struct tag changes: regenerate [config.schema.json](../schemas/config.schema.json).
- New commands or visible behavior: update the matching capability or quickstart explanation in [README](../../README.md).
- User-visible changes: add an `[Unreleased]` entry in [CHANGELOG](../../CHANGELOG.md).

Keep tutorials focused on one working outcome. Put optional workflows in task guides and field details in references. Link to the canonical explanation instead of copying tables or setup instructions. Preserve existing page paths and section anchors when reorganizing docs.

## Before submitting

Review the diff for unrelated edits and generated files. Describe the final behavior and the checks you ran in the PR. Record non-obvious architectural choices in the [decision log](decisions.md).
