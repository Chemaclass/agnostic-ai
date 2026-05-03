# Release process

## Versioning

Semantic Versioning. Pre-1.0:

- Minor bumps may break spec format
- Patch bumps are bug-fix only
- Document breaking changes in `CHANGELOG.md`

## Cutting a release

1. Update `version` in `cmd/agnostic-ai/main.go`
2. Update `CHANGELOG.md`
3. `git commit -m "chore(release): vX.Y.Z"`
4. `git tag vX.Y.Z`
5. `git push origin main --tags`
6. CI builds binaries and publishes to GitHub Releases

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

Cherry-pick fixes for the previous minor to a `release/X.Y` branch and tag `vX.Y.Zpatch`.
