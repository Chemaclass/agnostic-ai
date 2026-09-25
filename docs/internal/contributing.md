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
| Docs site (`docs/site/`) | `make site-serve` to preview, then `make site-check` and `make site-test`, with the pinned Zola. See [Docs site](#docs-site) |
| Editor extension | Follow its [development guide](../../editors/README.md) and CI job |

`make preflight` covers formatting, lint (including `govet`), and Go tests. It does not run every job in [CI](../../.github/workflows/ci.yml), including race tests, shell tests, schema drift, WASM builds, and extension builds.

`make lint` names the problem when `golangci-lint` is missing or not the pinned version; rerun `make tools` after a pin bump. A linter older than your Go toolchain reports that as a typecheck failure in files you never touched.

A pull request runs the Go tests on Linux only. Windows and macOS run on every push to `main`, once a night, and on demand with `gh workflow run ci.yml --ref main`. Editor jobs on pull requests run when their source or dependencies change. Confirm a full three-OS run before cutting a release.

## Docs site

The site is [Zola](https://www.getzola.org/), and `zola --version` must match `ZOLA_VERSION` in the [Makefile](../../Makefile) exactly. `site-serve`, `site-check`, and `site-build` refuse a different version. To run the pinned build without replacing your install, download that release and put it first on `PATH`:

```bash
gh release download v0.23.6 --repo getzola/zola --pattern '*aarch64-apple-darwin.tar.gz'
tar xzf zola-v0.23.6-aarch64-apple-darwin.tar.gz
PATH="$PWD:$PATH" make site-serve   # http://127.0.0.1:1111
```

**A mismatched Zola does not fail `go test ./...`; it skips the three site build tests.** Run `make site-test` with the pinned binary before trusting a change under `docs/site/`.

Never edit `docs/site/templates/` just to satisfy a newer Zola. Moving versions is its own change: bump `ZOLA_VERSION` in the Makefile and `.github/workflows/playground.yml` with the template edits, and diff `_site/` built on both versions.

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

The repository's `.agnostic-ai/` specs generate root entry points and tool folders. Most output is ignored; `.openhands/setup.sh` is tracked for bootstrap. Edit specs and review any tracked output change after syncing.

The repository's output ignore block is maintained by hand. Preserve hierarchical patterns such as `**/AGENTS.md` and the `!internal/adapters/*/testdata/**` exception that keeps golden fixtures tracked. Add new adapter paths to [.gitignore](../../.gitignore).

CI lints source specs. It does not run `sync --check` against a fresh checkout because generated files are not committed. After generating locally, `sync --check` can confirm local output matches the specs.

## Documentation checklist

- New or changed flags, targets, or output fields: update the target's page under [targets](../site/content/docs/targets/_index.md) and [configuration](../site/content/docs/configuration.md), plus the [CLI reference](../site/content/docs/cli-reference.md) for command changes.
- New or changed spec fields: update [spec format](../site/content/docs/spec-format.md).
- Config struct tag changes: regenerate [config.schema.json](../schemas/config.schema.json).
- New commands or visible behavior: update the matching capability or quickstart explanation in [README](../../README.md).
- User-visible changes: add an `[Unreleased]` entry in [CHANGELOG](../../CHANGELOG.md).

Keep tutorials focused on one working outcome. Put optional workflows in task guides and field details in references. Link to the canonical explanation instead of copying tables or setup instructions. Preserve existing page paths and section anchors when reorganizing docs.

## Before submitting

Review the diff for unrelated edits and generated files. Describe the final behavior and the checks you ran in the PR. Record non-obvious architectural choices in the [decision log](decisions.md).
