#!/usr/bin/env bash
#
# release.sh — cut a new agnostic-ai release.
#
# Usage:
#   scripts/release.sh                     # bump minor by default (X.Y.0 -> X.(Y+1).0)
#   scripts/release.sh major               # bump major (X.Y.Z -> (X+1).0.0)
#   scripts/release.sh minor               # bump minor (explicit)
#   scripts/release.sh patch               # bump patch (X.Y.Z -> X.Y.(Z+1))
#   scripts/release.sh vX.Y.Z              # explicit version
#   scripts/release.sh --dry-run           # preview only; touches no files
#   scripts/release.sh patch --no-push     # commit and tag locally; do not push
#
# What it does (in order):
#   1. Validate inputs and git state (clean tree, on main, in sync with origin).
#   2. Run go vet, gofmt -s -l, go test ./..., agnostic-ai sync --check.
#   3. Bump `version` in cmd/agnostic-ai/main.go.
#   4. Promote [Unreleased] in CHANGELOG.md to [vX.Y.Z] with today's date and
#      insert a fresh [Unreleased] block above.
#   5. Commit `chore(release): vX.Y.Z`.
#   6. Create annotated tag vX.Y.Z.
#   7. Push main and tag. CI fires GoReleaser, which builds and uploads
#      binaries plus tarballs to a fresh GitHub Release for the tag.
#   8. Watch the release workflow and print the release URL when done.

set -Eeuo pipefail

# ---- pure helpers (sourced by tests) -----------------------------------------

die()  { printf 'error: %s\n' "$*" >&2; return 1; }
note() { printf '→ %s\n' "$*"; }

# read_current_version <main.go path> — echoes the literal value from
# `var version = "X.Y.Z"`.
read_current_version() {
  local file="$1"
  awk -F'"' '/^var version =/{print $2; exit}' "$file"
}

# validate_version <vX.Y.Z[-pre]> — returns 0 if valid, 1 otherwise.
validate_version() {
  [[ "$1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]]
}

# compute_next_version <major|minor|patch> <current X.Y.Z> — echoes vX.Y.Z.
compute_next_version() {
  local bump="$1" current="$2"
  [[ "$current" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] \
    || { printf 'error: current version %s is not plain MAJOR.MINOR.PATCH\n' "$current" >&2; return 1; }
  local maj="${BASH_REMATCH[1]}" min="${BASH_REMATCH[2]}" pat="${BASH_REMATCH[3]}"
  case "$bump" in
    major) maj=$((maj + 1)); min=0; pat=0 ;;
    minor) min=$((min + 1)); pat=0 ;;
    patch) pat=$((pat + 1)) ;;
    *)     printf 'error: unknown bump kind %s\n' "$bump" >&2; return 1 ;;
  esac
  printf 'v%d.%d.%d\n' "$maj" "$min" "$pat"
}

# bump_version_in_file <main.go path> <new plain version> — rewrites the
# `var version = "..."` line.
bump_version_in_file() {
  local file="$1" new="$2"
  awk -v new="$new" '
    /^var version =/ { print "var version = \"" new "\""; next }
    { print }
  ' "$file" > "$file.tmp" && mv "$file.tmp" "$file"
}

# promote_changelog <CHANGELOG path> <vX.Y.Z> <YYYY-MM-DD> — promotes the
# existing [Unreleased] block to [version] - date and inserts a fresh
# empty [Unreleased] block above.
promote_changelog() {
  local file="$1" ver="$2" date="$3"
  grep -q '^## \[Unreleased\]' "$file" || { printf 'error: no [Unreleased] section in %s\n' "$file" >&2; return 1; }
  awk -v ver="$ver" -v date="$date" '
    !done && /^## \[Unreleased\]/ {
      print "## [Unreleased]"
      print ""
      print "## [" ver "] - " date
      done = 1
      next
    }
    { print }
  ' "$file" > "$file.tmp" && mv "$file.tmp" "$file"
}

# parse_args <argv...> — sets globals VERSION, BUMP, DRY_RUN, NO_PUSH.
parse_args() {
  VERSION=""
  BUMP=""
  DRY_RUN=0
  NO_PUSH=0
  for arg in "$@"; do
    case "$arg" in
      --dry-run)         DRY_RUN=1 ;;
      --no-push)         NO_PUSH=1 ;;
      -h|--help)         sed -n '2,24p' "$SCRIPT_PATH"; exit 0 ;;
      major|minor|patch) BUMP="$arg" ;;
      v*)                VERSION="$arg" ;;
      *)                 die "unknown arg: $arg"; return 1 ;;
    esac
  done
  if [[ -n "$VERSION" && -n "$BUMP" ]]; then
    die "pass either an explicit vX.Y.Z or a bump kind (major|minor|patch), not both"
    return 1
  fi
  if [[ -z "$VERSION" && -z "$BUMP" ]]; then
    BUMP="minor"
  fi
}

# ---- main orchestration ------------------------------------------------------

main() {
  parse_args "$@"

  local main_file="cmd/agnostic-ai/main.go"
  local changelog="CHANGELOG.md"
  local current
  current="$(read_current_version "$main_file")"
  [[ -n "$current" ]] || die "could not read current version from $main_file"

  if [[ -n "$BUMP" ]]; then
    VERSION="$(compute_next_version "$BUMP" "$current")"
  fi
  validate_version "$VERSION" \
    || die "version must match vMAJOR.MINOR.PATCH (or vMAJOR.MINOR.PATCH-pre): $VERSION"

  local plain="${VERSION#v}"
  local date
  date="$(date +%Y-%m-%d)"

  note "version: $VERSION (was $current)"
  note "date:    $date"
  [[ $DRY_RUN -eq 1 ]] && note "mode:    dry-run (no writes, no commits, no push)"
  [[ $NO_PUSH -eq 1 && $DRY_RUN -eq 0 ]] && note "mode:    --no-push (commit + tag only)"

  preflight "$VERSION"

  note "bump: $current -> $plain"
  if [[ $DRY_RUN -eq 0 ]]; then
    bump_version_in_file "$main_file" "$plain"
    note "changelog: promote [Unreleased] -> [$VERSION] $date"
    promote_changelog "$changelog" "$VERSION" "$date"
  else
    note "dry-run complete. exiting before commit."
    return 0
  fi

  note "commit"
  git add "$main_file" "$changelog"
  git commit -m "chore(release): $VERSION"

  note "tag"
  git tag -a "$VERSION" -m "Release $VERSION"

  if [[ $NO_PUSH -eq 1 ]]; then
    note "skipping push. run \`git push origin main && git push origin $VERSION\` when ready."
    return 0
  fi

  note "push main"
  git push origin main

  note "push tag (CI builds release binaries)"
  git push origin "$VERSION"

  watch_release "$VERSION"
}

# preflight gates the release on git state and code health.
preflight() {
  local version="$1"
  note "preflight: git state"
  [[ -z "$(git status --porcelain)" ]] || die "working tree dirty. commit or stash first."

  local branch
  branch="$(git rev-parse --abbrev-ref HEAD)"
  [[ "$branch" == "main" ]] || die "must release from main, on $branch"

  git fetch origin main --quiet
  local local_head remote_head
  local_head="$(git rev-parse @)"
  remote_head="$(git rev-parse @{u})"
  [[ "$local_head" == "$remote_head" ]] || die "local main not in sync with origin. pull or push first."

  if git rev-parse "$version" >/dev/null 2>&1; then
    die "tag $version already exists"
  fi

  note "preflight: gofmt"
  local gofmt_out
  gofmt_out="$(gofmt -s -l . | grep -v '^vendor/' || true)"
  [[ -z "$gofmt_out" ]] || die "gofmt -s issues:"$'\n'"$gofmt_out"

  note "preflight: go vet"
  go vet ./...

  note "preflight: go test"
  go test ./...

  note "preflight: sync --check (drift gate)"
  go run ./cmd/agnostic-ai sync --check
}

# watch_release waits for the release workflow to start, watches it, and
# prints the resulting GitHub Release URL.
watch_release() {
  local version="$1"
  if ! command -v gh >/dev/null 2>&1; then
    note "gh not installed. release workflow runs in GitHub Actions; check manually."
    return 0
  fi

  note "waiting for release workflow to start..."
  local run_id=""
  for _ in $(seq 1 30); do
    run_id="$(gh run list --workflow=release.yml --branch "$version" \
      --limit 1 --json databaseId --jq '.[0].databaseId' 2>/dev/null || true)"
    [[ -n "$run_id" && "$run_id" != "null" ]] && break
    sleep 2
  done

  if [[ -z "$run_id" || "$run_id" == "null" ]]; then
    note "release workflow did not register within 60s. check manually:"
    note "  gh run list --workflow=release.yml"
    return 0
  fi

  note "release workflow run: $run_id"
  gh run watch "$run_id" --exit-status

  local repo
  repo="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
  note "release published:"
  note "  https://github.com/$repo/releases/tag/$version"
  gh release view "$version" --json assets --jq '.assets[].name' \
    | sed 's/^/    asset: /'
}

# ---- entry point -------------------------------------------------------------

SCRIPT_PATH="${BASH_SOURCE[0]}"

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  cd "$(git rev-parse --show-toplevel)"
  main "$@"
fi
