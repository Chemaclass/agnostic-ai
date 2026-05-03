# Contributing (deep dive)

Short version: [/CONTRIBUTING.md](../../CONTRIBUTING.md).

## Dev setup

Requires Go 1.23+.

```bash
git clone https://github.com/Chemaclass/agnostic-ai
cd agnostic-ai
make build      # builds ./agnostic-ai
make test       # runs all tests
```

Optional: install [golangci-lint](https://golangci-lint.run/welcome/install/) to match CI.

```bash
golangci-lint run
```

## Dev loop

```bash
# edit code, then:
make build && ./agnostic-ai validate
go run ./cmd/agnostic-ai sync --dry-run

# focused tests
go test ./internal/adapters/claude -run TestEmit_WritesAgent -v

# before pushing
make test
golangci-lint run
```

## First PR

1. Pick a `good first issue` (typo, error message, missing test, small flag).
2. Branch: `git checkout -b fix/short-description`.
3. Edit code. Add or update `foo_test.go` next to `foo.go`.
4. Run `go test ./...`.
5. Commit with Conventional Commits: `git commit -m "fix(config): include path in error when config file is missing"`.
6. Push, open PR, fill template, link issue with `Closes #123`.

## Code conventions

- `gofmt` clean. `goimports` for imports.
- Stdlib first. New deps must earn their place.
- Adapters are stateless. Construct via `New()`.
- Adapters share helpers via `internal/adapters/internal/emit`. They never import each other.
- Test names describe behavior: `TestEmit_WritesAgentFile`, not `TestEmit1`.
- One concern per PR.
- Wrap errors with file context: `fmt.Errorf("%s: %w", path, err)`.

## Tests

- Unit tests next to code: `foo.go` + `foo_test.go`.
- Adapter tests use `t.TempDir()` and `os.Chdir`. Restore cwd in cleanup.
- No mocks. Test against stdlib and `gopkg.in/yaml.v3` directly.
- Race detector must pass. CI runs `go test -race ./...`.

## Debugging

```bash
go run ./cmd/agnostic-ai sync --dry-run    # output without writing
go run ./cmd/agnostic-ai list              # confirm specs loaded
go run ./cmd/agnostic-ai validate          # parse-check only
go run ./cmd/agnostic-ai sync -t claude --dry-run  # one adapter
```

For unexpected adapter behavior, write a unit test calling its `Emit` with a small spec slice.

## Common pitfalls

| Pitfall | Fix |
|---|---|
| Forgetting to register a new adapter | Update `internal/adapters/adapter.go`, `internal/config/config.go`, `internal/cli/init.go` |
| Importing one adapter from another | Share via `internal/adapters/internal/emit` |
| Testing with live cwd | Use `t.TempDir()` and `os.Chdir` |
| Skipping docs update | Update `docs/user/` or `README.md` in the same PR |
| Drive-by formatting in feature PRs | Open a separate `chore: gofmt` PR |

## Commits

Conventional Commits required:

| Prefix | Use |
|---|---|
| `feat:` | new feature, adapter, spec kind, flag |
| `fix:` | bug fix |
| `docs:` | docs only |
| `refactor:` | no behavior change |
| `test:` | tests only |
| `chore:` | build, deps, CI |

Subject under 72 chars. Body explains *why*.

## Reviewing PRs

Triagers:

- Triage within 7 days. Apply labels (`bug`, `enhancement`, `good first issue`, `help wanted`).
- Require tests for behavior changes, docs for user-visible changes.
- Squash-merge.

## Releasing

See [release-process.md](release-process.md).

## Decision log

Add non-obvious architectural calls to [decisions.md](decisions.md) with context, options, and rationale.

## Questions

Open a Discussion. Do not file an issue.
