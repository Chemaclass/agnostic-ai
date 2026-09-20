#!/usr/bin/env bash
#
# npm-publish.sh - publish the seven npm packages of a release, in order.
#
# Usage:
#   scripts/npm-publish.sh <version> [platforms-dir] [parent-dir]
#
# The six platform packages go first, the parent last. The parent pins exact
# versions of all six, so a parent that lands while one of them is missing
# breaks `npm install` on that platform until the next release. Ordering plus
# the wait between the two halves is what prevents it.
#
# Publishing is idempotent: a version already on the registry is skipped, so a
# re-run after a partial failure finishes the release instead of failing on a
# conflict.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

err() { printf 'npm-publish: %s\n' "$*" >&2; }

package_name() {
  node -p "require('$1/package.json').name"
}

published() {
  npm view "$1@$2" version >/dev/null 2>&1
}

# Provenance is signed through Sigstore, a service outside this release.
# Shipping the package matters more than the attestation, so a provenance
# failure downgrades to a plain publish. The warning is the signal to
# investigate; `npm view --json <pkg>@<version>` shows whether a version
# carries one.
publish_package() {
  local dir="$1" name version="$2"
  name="$(package_name "$dir")"

  if published "$name" "$version"; then
    printf '::notice::%s@%s is already published\n' "$name" "$version"
    return 0
  fi

  if (cd "$dir" && npm publish --access public --provenance); then
    return 0
  fi
  printf '::warning::npm publish --provenance failed for %s; retrying without it, so this version ships unattested\n' "$name"
  if (cd "$dir" && npm publish --access public); then
    return 0
  fi
  # The first attempt can upload the tarball and still fail while attaching
  # the attestation, which makes the retry a version conflict, not an error.
  if published "$name" "$version"; then
    printf '::warning::%s@%s is on the registry; the failed retry was a version conflict\n' "$name" "$version"
    return 0
  fi
  printf '::error::npm publish failed for %s@%s both with and without provenance\n' "$name" "$version"
  return 1
}

wait_for() {
  # Same bounded backoff the release's distribution guard uses, for the same
  # reason: the read hits a registry replica that lags the write by seconds.
  # Read at call time, not load time, so the test suite can shorten it.
  local name="$1" version="$2" attempt
  local retries="${NPM_PUBLISH_RETRIES:-6}" delay="${NPM_PUBLISH_FIRST_DELAY:-5}"
  for ((attempt = 1; attempt <= retries; attempt++)); do
    if published "$name" "$version"; then
      printf 'attempt %s: %s@%s is served\n' "$attempt" "$name" "$version"
      return 0
    fi
    printf 'attempt %s: %s@%s absent\n' "$attempt" "$name" "$version"
    if [[ "$attempt" -lt "$retries" ]]; then
      sleep "$delay"
      delay=$((delay * 2))
    fi
  done
  return 1
}

main() {
  local version="${1:-}" platforms="${2:-$ROOT/npm/platforms}" parent="${3:-$ROOT/npm}"
  if [[ -z "$version" ]]; then
    err "usage: scripts/npm-publish.sh <version> [platforms-dir] [parent-dir]"
    return 2
  fi

  local dirs=() dir name
  for dir in "$platforms"/*/; do
    [[ -f "$dir/package.json" ]] || continue
    dirs+=("${dir%/}")
  done
  if [[ "${#dirs[@]}" -eq 0 ]]; then
    err "no platform packages in $platforms; run npm/scripts/build-platform-packages.js first"
    return 1
  fi

  for dir in "${dirs[@]}"; do
    publish_package "$dir" "$version"
  done

  # The parent is unusable without all six, so do not publish it until the
  # registry serves every one of them.
  for dir in "${dirs[@]}"; do
    name="$(package_name "$dir")"
    if ! wait_for "$name" "$version"; then
      printf '::error::%s@%s never appeared; not publishing the parent, which would pin a package nobody can install\n' "$name" "$version"
      return 1
    fi
  done

  publish_package "$parent" "$version"
  printf 'npm-publish: published %s packages at %s\n' "$((${#dirs[@]} + 1))" "$version"
}

if [[ -z "${BASH_SOURCE[0]:-}" || "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
