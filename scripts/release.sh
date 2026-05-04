#!/usr/bin/env bash
#
# release.sh — cut a new agnostic-ai release.
#
# Usage:
#   scripts/release.sh vX.Y.Z              # cut and push
#   scripts/release.sh vX.Y.Z --dry-run    # preview only; touches no files
#   scripts/release.sh vX.Y.Z --no-push    # commit and tag locally; do not push
#
# What it does (in order):
#   1. Validate inputs and git state (clean tree, on main, in sync with origin).
#   2. Run go vet, gofmt -s -l, go test ./..., agnostic-ai sync --check.
#   3. Bump `version` in cmd/agnostic-ai/main.go.
#   4. Promote [Unreleased] in CHANGELOG.md to [vX.Y.Z] with today's date and
#      insert a fresh empty [Unreleased] block above it.
#   5. Commit `chore(release): vX.Y.Z`.
#   6. Create annotated tag vX.Y.Z.
#   7. Push main and tag. CI builds binaries via GoReleaser.

set -Eeuo pipefail

die() { printf 'error: %s\n' "$*" >&2; exit 1; }
note() { printf '→ %s\n' "$*"; }

# --- args ---------------------------------------------------------------------

VERSION=""
DRY_RUN=0
NO_PUSH=0

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    --no-push) NO_PUSH=1 ;;
    -h|--help) sed -n '2,18p' "$0"; exit 0 ;;
    v*) VERSION="$arg" ;;
    *)  die "unknown arg: $arg" ;;
  esac
done

[[ -n "$VERSION" ]] || die "missing version. example: scripts/release.sh v0.2.0"
[[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]] \
  || die "version must match vMAJOR.MINOR.PATCH (or vMAJOR.MINOR.PATCH-pre)"

PLAIN="${VERSION#v}"
DATE="$(date +%Y-%m-%d)"
ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

note "version: $VERSION"
note "date:    $DATE"
[[ $DRY_RUN -eq 1 ]] && note "mode:    dry-run (no writes, no commits, no push)"
[[ $NO_PUSH -eq 1 && $DRY_RUN -eq 0 ]] && note "mode:    --no-push (commit + tag only)"

# --- preflight ----------------------------------------------------------------

note "preflight: git state"
[[ -z "$(git status --porcelain)" ]] || die "working tree dirty. commit or stash first."

BRANCH="$(git rev-parse --abbrev-ref HEAD)"
[[ "$BRANCH" == "main" ]] || die "must release from main, on $BRANCH"

git fetch origin main --quiet
LOCAL="$(git rev-parse @)"
REMOTE="$(git rev-parse @{u})"
[[ "$LOCAL" == "$REMOTE" ]] || die "local main not in sync with origin. pull or push first."

if git rev-parse "$VERSION" >/dev/null 2>&1; then
  die "tag $VERSION already exists"
fi

note "preflight: gofmt"
GOFMT_OUT="$(gofmt -s -l . | grep -v '^vendor/' || true)"
[[ -z "$GOFMT_OUT" ]] || die "gofmt -s issues:"$'\n'"$GOFMT_OUT"

note "preflight: go vet"
go vet ./...

note "preflight: go test"
go test ./...

note "preflight: sync --check (drift gate)"
go run ./cmd/agnostic-ai sync --check

# --- bump version -------------------------------------------------------------

MAIN_FILE="cmd/agnostic-ai/main.go"
CURRENT="$(awk -F'"' '/^var version =/{print $2}' "$MAIN_FILE")"
[[ -n "$CURRENT" ]] || die "could not read current version from $MAIN_FILE"
note "bump: $CURRENT -> $PLAIN"

if [[ $DRY_RUN -eq 0 ]]; then
  # Portable in-place sed (BSD + GNU): write to temp, move back.
  awk -v new="$PLAIN" '
    /^var version =/ { print "var version = \"" new "\""; next }
    { print }
  ' "$MAIN_FILE" > "$MAIN_FILE.tmp" && mv "$MAIN_FILE.tmp" "$MAIN_FILE"
fi

# --- update CHANGELOG ---------------------------------------------------------

CHANGELOG="CHANGELOG.md"
[[ -f "$CHANGELOG" ]] || die "missing $CHANGELOG"

grep -q '^## \[Unreleased\]' "$CHANGELOG" || die "no [Unreleased] section in $CHANGELOG"

note "changelog: promote [Unreleased] -> [$VERSION] $DATE"
if [[ $DRY_RUN -eq 0 ]]; then
  awk -v ver="$VERSION" -v date="$DATE" '
    !done && /^## \[Unreleased\]/ {
      print "## [Unreleased]"
      print ""
      print "## [" ver "] - " date
      done = 1
      next
    }
    { print }
  ' "$CHANGELOG" > "$CHANGELOG.tmp" && mv "$CHANGELOG.tmp" "$CHANGELOG"
fi

# --- commit + tag + push ------------------------------------------------------

if [[ $DRY_RUN -eq 1 ]]; then
  note "dry-run complete. exiting before commit."
  exit 0
fi

note "commit"
git add "$MAIN_FILE" "$CHANGELOG"
git commit -m "chore(release): $VERSION"

note "tag"
git tag -a "$VERSION" -m "Release $VERSION"

if [[ $NO_PUSH -eq 1 ]]; then
  note "skipping push. run \`git push origin main && git push origin $VERSION\` when ready."
  exit 0
fi

note "push main"
git push origin main

note "push tag (CI builds release binaries)"
git push origin "$VERSION"

note "done. watch the release workflow:"
note "  gh run watch \$(gh run list --workflow=release.yml --limit 1 --json databaseId --jq '.[0].databaseId')"
