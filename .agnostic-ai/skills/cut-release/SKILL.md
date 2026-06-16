---
name: cut-release
description: Cut a new agnostic-ai release end to end. Use when the user wants to ship a new version (tag + GitHub Release).
---

# cut-release

Cuts a new release of agnostic-ai. GoReleaser publishes artifacts and the GitHub Release once the tag is pushed.

## When to run

The user asks to release, tag, ship, or cut a new version.

## Steps

1. Confirm working tree clean and on `main`. `git pull --ff-only`.
2. `make preflight` (fmt-check + vet + lint + test). Refuse to proceed on any failure.
3. Decide next version per semver:
   - patch: bug fixes only
   - minor: additive features
   - major: breaking changes
4. Update `CHANGELOG.md`: drop empty `### ` subsections from `## [Unreleased]`, then move the remaining lines into a new dated `## vX.Y.Z - YYYY-MM-DD` section (no brackets). The released section must never carry a `### ` heading with no entries. Reset `## [Unreleased]` to empty.
5. Bump `version` in `cmd/agnostic-ai/main.go`.
6. Commit: `chore(release): vX.Y.Z`. GPG-signed.
7. Tag: `git tag -s vX.Y.Z -m "vX.Y.Z"`.
8. Push branch and tag: `git push && git push origin vX.Y.Z`.
9. Watch the GoReleaser workflow. If it fails, fix root cause. Do not delete and retag without clear reason.

## Conventions

- Release notes pipeline reads the latest dated section from `CHANGELOG.md`. Never skip step 4.
- Tag format `vX.Y.Z` (lowercase `v`). GoReleaser matches this prefix.
- Commit message follows Conventional Commits. Never mention AI in the message.
