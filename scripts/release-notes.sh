#!/usr/bin/env bash
#
# release-notes.sh — emit GitHub release notes for a tag from CHANGELOG.md.
#
# Usage:
#   scripts/release-notes.sh vX.Y.Z [CHANGELOG path] [owner/repo]
#
# Echoes the body of the matching `## [vX.Y.Z]` section from CHANGELOG.md
# plus a footer linking to the full changelog. Exits non-zero if the
# section is missing.
#
# Defaults: CHANGELOG.md, repo derived from `gh repo view` when unset.
# Used by .github/workflows/release.yml to feed `goreleaser release
# --release-notes=NOTES.md` and by scripts/release.sh as a local sanity
# check.

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/release.sh"

ver="${1:?usage: $0 vX.Y.Z [CHANGELOG] [owner/repo]}"
changelog="${2:-CHANGELOG.md}"
repo="${3:-}"

if [[ -z "$repo" ]]; then
  if command -v gh >/dev/null 2>&1; then
    repo="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
  else
    printf 'error: repo not provided and gh not available\n' >&2
    exit 1
  fi
fi

format_release_notes "$ver" "$changelog" "$repo"
