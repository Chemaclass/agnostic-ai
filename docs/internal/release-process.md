# Release process

## Versioning

Semantic Versioning. Pre-1.0: minor bumps may break spec format; patches are bug-fix only. Breaking changes go in `CHANGELOG.md`.

## Cut a release

```bash
scripts/release.sh vX.Y.Z              # bump + tag + push
scripts/release.sh vX.Y.Z --dry-run    # preview only
scripts/release.sh vX.Y.Z --no-push    # commit + tag locally
```

The script:

1. Validates clean tree, on `main`, in sync with `origin/main`, tag absent.
2. Runs `gofmt -s -l`, `go vet`, `go test ./...`, `agnostic-ai sync --check`.
3. Bumps `version` in `cmd/agnostic-ai/main.go`.
4. Promotes `[Unreleased]` to `## vX.Y.Z - YYYY-MM-DD` (no brackets), inserts a fresh empty `[Unreleased]` block.
5. Commits `chore(release): vX.Y.Z`, creates annotated tag `vX.Y.Z`.
6. Pushes `main` + tag. CI runs GoReleaser; release notes come from `scripts/release-notes.sh` against the matching `CHANGELOG.md` section.

## Distribution

| Channel | Notes |
|---|---|
| GitHub Releases | raw binaries, primary |
| Homebrew tap | `chemaclass/tap/agnostic-ai`, formula auto-updated by CI |
| `go install` | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |

## Backporting

Cherry-pick fixes for the previous minor onto `release/X.Y`. Tag `vX.Y.Zpatch`.
