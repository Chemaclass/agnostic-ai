# Contributing (deep dive)

Short version: [/CONTRIBUTING.md](../../CONTRIBUTING.md).

## Setup

Go 1.24+.

```bash
git clone https://github.com/Chemaclass/agnostic-ai
cd agnostic-ai
make tools      # golangci-lint + lefthook at CI-pinned versions
make hooks      # lefthook pre-commit (gofmt + lint + vet) + pre-push (make preflight)
make build      # builds ./agnostic-ai
make test
```

`make tools` puts binaries in `$(go env GOPATH)/bin`. Ensure it is on `$PATH`.

## Dev loop

```bash
go run ./cmd/agnostic-ai sync --dry-run                            # preview emit
go test ./internal/adapters/claude -run TestEmit_WritesAgent -v    # focused test
make preflight                                                     # mirrors CI Lint + Test
```

A green `make preflight` locally means no new lint or test surprise on the PR.

## First PR

1. Pick a `good first issue`.
2. Branch: `fix/short-description`.
3. Edit. Add or update `foo_test.go` next to `foo.go`.
4. `go test ./...`.
5. Commit with Conventional Commits.
6. Push, open PR with template, link issue (`Closes #123`).

## Conventions

| Topic | Rule |
|---|---|
| Format | `gofmt` clean, `goimports` grouping |
| Deps | Stdlib first. New dep needs justification |
| Adapters | Stateless. `New()` constructor. Never import each other; share via `internal/adapters/internal/emit/` |
| Tests | Behavior names. `t.TempDir()` + `testutil.Chdir(t, dir)`. No mocks |
| Errors | Wrap with file/operation context: `fmt.Errorf("%s: %w", path, err)` |
| CHANGELOG | Update `[Unreleased]` for user-visible changes |

## Debugging

```bash
go run ./cmd/agnostic-ai sync --dry-run             # output without writing
go run ./cmd/agnostic-ai list                       # confirm specs loaded
go run ./cmd/agnostic-ai validate                   # parse-check only
go run ./cmd/agnostic-ai sync -t claude --dry-run   # one adapter
```

For unexpected adapter behavior: write a unit test calling `Emit` with a small spec slice.

## Commits

Conventional Commits: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`. Subject under 72 chars. Body explains *why*.

## Triage

- 7-day target. Apply labels (`bug`, `enhancement`, `good first issue`, `help wanted`).
- Tests required for behavior changes. Docs for user-visible changes.
- Squash-merge.

## See also

- [Release process](release-process.md)
- [Decision log](decisions.md): add non-obvious architectural calls.
- Questions: open a Discussion.
