# Release process

## Versioning

Semantic Versioning. Pre-1.0:

- Minor bumps may break spec format
- Patch bumps are bug-fix only
- Document breaking changes in `CHANGELOG.md`

## Cutting a release

Use the script:

```bash
scripts/release.sh vX.Y.Z              # full run: bump, tag, push
scripts/release.sh vX.Y.Z --dry-run    # preview only
scripts/release.sh vX.Y.Z --no-push    # commit + tag locally
```

The script:

1. Validates clean tree, on `main`, in sync with `origin/main`, tag does not exist.
2. Runs `gofmt -s -l`, `go vet`, `go test ./...`, `agnostic-ai sync --check`.
3. Bumps `version` in `cmd/agnostic-ai/main.go`.
4. Promotes `[Unreleased]` in `CHANGELOG.md` to `[vX.Y.Z]` with today's date and inserts a fresh `[Unreleased]` block above.
5. Commits `chore(release): vX.Y.Z` and tags `vX.Y.Z` (annotated).
6. Pushes `main` and the tag. CI fires GoReleaser, which publishes binaries.

Manual fallback (only if the script can't run):

1. Update `version` in `cmd/agnostic-ai/main.go`
2. Update `CHANGELOG.md`
3. `git commit -m "chore(release): vX.Y.Z"`
4. `git tag vX.Y.Z`
5. `git push origin main --tags`

## Cross-compile manually

```bash
make release
ls dist/
```

## Distribution channels

| Channel | Notes |
|---|---|
| GitHub Releases | raw binaries, primary |
| Homebrew tap | `chemaclass/tap/agnostic-ai`, formula updated by CI |
| Go install | `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` |

## Backporting

Cherry-pick fixes for the previous minor onto a `release/X.Y` branch; tag `vX.Y.Zpatch`.
