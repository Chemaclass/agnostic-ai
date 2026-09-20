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
#
# Every publish carries an explicit --tag. An untagged `npm publish` writes the
# `latest` dist-tag, which is what an unpinned `npm install agnostic-ai`
# resolves, so a prerelease published without one takes over the default
# install for everybody.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

err() { printf 'npm-publish: %s\n' "$*" >&2; }

# Never hand node an absolute path from this shell. On Windows, Git Bash
# reports /d/a/... while node wants D:\a\..., so an interpolated path is a
# module node cannot find. cd first and require relatively: the shell
# translates the cwd and node inherits it, on every platform.
package_name() {
  (cd "$1" && node -p "require('./package.json').name")
}

published() {
  npm view "$1@$2" version >/dev/null 2>&1
}

# npm_dist_tag <version> — echoes the dist-tag <version> has to publish under.
#
# A plain X.Y.Z owns `latest`. A version carrying a prerelease suffix gets its
# own channel, named after the first identifier of that suffix, so `-beta.1`
# and `-rc.2` never share one and neither can move `latest`. A suffix that is
# not a plain word (a date stamp, a build id, `latest` itself) parks on `next`
# rather than inventing a tag npm may reject: the registry refuses a dist-tag
# that parses as a version.
#
# Pure: no registry read, no filesystem. The release's distribution guard
# sources this file to derive the same tag it then asserts.
npm_dist_tag() {
  local version="${1#v}" word
  case "$version" in
    *-*) ;;
    *) printf 'latest\n'; return 0 ;;
  esac
  word="${version#*-}"
  word="${word%%.*}"
  word="$(printf '%s' "$word" | tr '[:upper:]' '[:lower:]')"
  case "$word" in
    latest) printf 'next\n' ;;
    [a-z]*)
      case "$word" in
        *[!a-z0-9-]*) printf 'next\n' ;;
        *) printf '%s\n' "$word" ;;
      esac
      ;;
    *) printf 'next\n' ;;
  esac
}

# Provenance is signed through Sigstore, a service outside this release.
# Shipping the package matters more than the attestation, so a provenance
# failure downgrades to a plain publish. The warning is the signal to
# investigate; `npm view --json <pkg>@<version>` shows whether a version
# carries one.
publish_package() {
  local dir="$1" name version="$2" tag
  name="$(package_name "$dir")"
  tag="$(npm_dist_tag "$version")"

  if published "$name" "$version"; then
    printf '::notice::%s@%s is already published\n' "$name" "$version"
    return 0
  fi

  if (cd "$dir" && npm publish --access public --provenance --tag "$tag"); then
    return 0
  fi
  printf '::warning::npm publish --provenance failed for %s; retrying without it, so this version ships unattested\n' "$name"
  if (cd "$dir" && npm publish --access public --tag "$tag"); then
    return 0
  fi
  # The first attempt can upload the tarball and still fail while attaching
  # the attestation, which makes the retry a version conflict, not an error.
  if published "$name" "$version"; then
    printf '::warning::%s@%s is on the registry; the failed retry was a version conflict\n' "$name" "$version"
    return 0
  fi
  printf '::error::npm publish failed for %s@%s both with and without provenance\n' "$name" "$version"
  # A scoped name cannot be created until its org exists, and npm reports
  # that as a plain 404 or 403 on the first publish, which reads the same as
  # a bad token. Name the likely cause once rather than leaving six identical
  # failures to interpret at release time.
  case "$name" in
    @*/*)
      printf '::error::%s is scoped. If this is the first release to publish it, check the npm org %s exists and that NPM_TOKEN can create packages in it; a token scoped to the agnostic-ai package alone cannot.\n' \
        "$name" "${name%%/*}"
      ;;
  esac
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

# Every name the parent pins has to be one of the packages about to be
# published. A pin with no package is a parent that installs nothing on that
# platform, and it is the one mistake ordering alone does not catch.
require_every_pin() {
  local parent="$1" names="$2" pinned pin
  pinned="$(cd "$parent" && node -p "Object.keys(require('./package.json').optionalDependencies||{}).join('\n')")"
  while IFS= read -r pin; do
    [[ -n "$pin" ]] || continue
    if ! grep -qxF "$pin" <<< "$names"; then
      err "the parent pins $pin but no package directory builds it"
      return 1
    fi
  done <<< "$pinned"
}

main() {
  local version="${1:-}" platforms="${2:-$ROOT/npm/platforms}" parent="${3:-$ROOT/npm}"
  if [[ -z "$version" ]]; then
    err "usage: scripts/npm-publish.sh <version> [platforms-dir] [parent-dir]"
    return 2
  fi

  # Parallel arrays rather than one associative array: macOS still ships
  # bash 3.2, which has no associative arrays.
  local dirs=() names=() dir i
  for dir in "$platforms"/*/; do
    [[ -f "$dir/package.json" ]] || continue
    dirs+=("${dir%/}")
    names+=("$(package_name "${dir%/}")")
  done
  if [[ "${#dirs[@]}" -eq 0 ]]; then
    err "no platform packages in $platforms; run npm/scripts/build-platform-packages.js first"
    return 1
  fi
  require_every_pin "$parent" "$(printf '%s\n' "${names[@]}")"

  for dir in "${dirs[@]}"; do
    publish_package "$dir" "$version"
  done

  # The parent is unusable without all six, so do not publish it until the
  # registry serves every one of them.
  for i in "${!names[@]}"; do
    if ! wait_for "${names[$i]}" "$version"; then
      printf '::error::%s@%s never appeared; not publishing the parent, which would pin a package nobody can install\n' "${names[$i]}" "$version"
      return 1
    fi
  done

  publish_package "$parent" "$version"
  printf 'npm-publish: published %s packages at %s under dist-tag %s\n' \
    "$((${#dirs[@]} + 1))" "$version" "$(npm_dist_tag "$version")"
}

if [[ -z "${BASH_SOURCE[0]:-}" || "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
