#!/usr/bin/env bash
#
# npm-publish.sh - publish the seven npm packages of a release, in order.
#
# Usage:
#   scripts/npm-publish.sh <version> [platforms-dir] [parent-dir]
#
# The six platform packages go first, with up to NPM_PUBLISH_WORKERS (default
# three) operations at once. The parent pins all six and waits for each one to
# be available with verified provenance.
#
# Publishing is idempotent only after the existing provenance verifies.
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
  (cd "$1" && node -e '
    const { name, version } = require("./package.json")
    if (!name || version !== process.argv[1]) {
      console.error("npm-publish: " + process.cwd() + "/package.json must name a package at version " + process.argv[1])
      process.exit(1)
    }
    console.log(name)
  ' "$2")
}

npm_metadata() {
  local spec="$1" field="$2" result
  if result="$(npm view "$spec" "$field" --json --prefer-online \
    --fetch-retries=1 --fetch-retry-mintimeout=1000 --fetch-timeout=10000 2>/dev/null)"; then
    printf '%s\n' "${result:-null}"
    return 0
  fi
  if node -e '
    try {
      const value = JSON.parse(require("node:fs").readFileSync(0, "utf8"))
      process.exitCode = value?.error?.code === "E404" ? 0 : 1
    } catch { process.exitCode = 1 }
  ' <<< "$result"; then
    printf 'null\n'
    return 0
  fi
  err "cannot read $field for $spec from the registry. Retry the same release after the registry recovers."
  return 1
}

published() {
  local found
  found="$(npm_metadata "$1@$2" version)" || return 2
  case "$found" in
    "\"$2\"") return 0 ;;
    null) return 1 ;;
  esac
  err "unexpected version metadata for $1@$2"
  return 2
}

# Derive the intended dist-tag from a release version.
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
  version="${version%%+*}"
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

# Used for both the last check before writing and the distribution check.
npm_dist_tag_state() {
  local name="$1" version="$2" tag tags npm_root
  tag="$(npm_dist_tag "$version")"
  tags="$(npm_metadata "$name" dist-tags)" || return 1
  npm_root="${NPM_PUBLISH_MODULES:-$(npm root -g)}" || return 1
  # shellcheck disable=SC2016
  node -e '
    const [root, tag, wanted, name] = process.argv.slice(1)
    const requireNpm = require("node:module").createRequire(require("node:path").join(root, "npm", "package.json"))
    const semver = requireNpm("semver")
    try {
      const tags = JSON.parse(require("node:fs").readFileSync(0, "utf8"))
      if (tags !== null && (typeof tags !== "object" || Array.isArray(tags))) throw new Error("invalid dist-tags")
      const current = tags && Object.hasOwn(tags, tag) ? tags[tag] : undefined
      if (!semver.valid(wanted) || (current !== undefined && !semver.valid(current))) throw new Error("invalid version")
      if (current === wanted) console.log("current")
      else if (current && semver.gte(current, wanted)) console.log("superseded")
      else console.log("pending")
    } catch (error) {
      console.error(`npm-publish: cannot determine ${name}@${tag}: ${error.message}`)
      process.exitCode = 1
    }
  ' "$npm_root" "$tag" "$version" "$name" <<< "$tags"
}

recovery_tag() {
  local version="$1"
  printf 'recovery-v%s\n' "${version//+/.}"
}

npm_provenance_verifies() {
  local name="$1" version="$2" commit npm_root
  commit="${GITHUB_SHA:-}"
  if [[ -z "$commit" ]]; then
    commit="$(git -C "$ROOT" rev-parse "v${version}^{commit}")" || return 1
  fi
  npm_root="${NPM_PUBLISH_MODULES:-$(npm root -g)}" || return 1
  node "$ROOT/scripts/npm-verify-provenance.js" "$name" "$version" "$commit" "$npm_root"
}

verify_npm_provenance() {
  # Retry with backoff so a short registry outage does not fail a good version.
  local name="$1" version="$2" attempt
  local retries="${NPM_PROVENANCE_RETRIES:-10}" delay="${NPM_PUBLISH_FIRST_DELAY:-5}"
  local max_delay="${NPM_PUBLISH_MAX_DELAY:-60}"
  for ((attempt = 1; attempt <= retries; attempt++)); do
    if npm_provenance_verifies "$name" "$version"; then
      return 0
    fi
    printf 'attempt %s: %s@%s provenance does not verify yet\n' "$attempt" "$name" "$version"
    if [[ "$attempt" -lt "$retries" ]]; then
      sleep "$delay"
      delay=$((delay * 2))
      [[ "$delay" -le "$max_delay" ]] || delay="$max_delay"
    fi
  done
  err "$name@$version has missing or invalid provenance. Retry after registry availability recovers. If published without provenance, this immutable version cannot be repaired; stop and cut a new version for all seven packages."
  return 1
}

record_timing() {
  local phase="$1" name="$2" version="$3" started="$4"
  printf 'npm-publish: %s %s@%s took %ss\n' "$phase" "$name" "$version" "$((SECONDS - started))"
}

publish_package() {
  local dir="$1" version="$2" tag="$3" name state started
  name="$(package_name "$dir" "$version")" || return 1
  if [[ "$tag" == "$(npm_dist_tag "$version")" ]]; then
    state="$(npm_dist_tag_state "$name" "$version")" || return 1
    case "$state" in
      current) return 0 ;;
      superseded) tag="$(recovery_tag "$version")" ;;
    esac
  fi

  started="$SECONDS"
  if ! (cd "$dir" && npm publish --access public --provenance --tag "$tag"); then
    err "provenance publish failed for $name@$version; no plain publish will be attempted"
    published "$name" "$version" || return 1
  fi
  record_timing publish "$name" "$version" "$started"
}

# npm caches an attestation response for a year, on both registry hosts. A
# read before npm adds its own publish attestation stores an incomplete copy
# that fails every later check at that cache location. npm gives no signal
# that signing is done, so the first read waits well past the minute or so it
# usually takes.
settle_attestations() {
  local settle="${NPM_ATTESTATION_SETTLE:-300}"
  if [[ "$settle" -gt 0 ]]; then
    printf 'waiting %ss for npm to finish signing before reading attestations\n' "$settle"
    sleep "$settle"
  fi
}

verify_with_timing() {
  local name="$1" version="$2" started="$SECONDS"
  verify_npm_provenance "$name" "$version" || return 1
  record_timing provenance "$name" "$version" "$started"
}

verify_existing_package() {
  verify_with_timing "$1" "$2" || return 1
  printf '::notice::%s@%s is already published with verified provenance\n' "$1" "$2"
}

refresh_existing_tag() {
  local name="$1" version="$2" tag="$3" state
  state="$(npm_dist_tag_state "$name" "$version")" || return 1
  if [[ "$state" == pending ]]; then
    npm dist-tag add "$name@$version" "$tag" || return 1
  fi
}

# A release has six platform packages. Reap a bounded cohort before moving on
# so a failure never leaves workers running into the next phase.
run_workers() {
  local operation="$1" workers="$2" version="$3" tag="$4" i pid failed
  shift 4
  while [[ $# -gt 0 ]]; do
    local pids=()
    for ((i = 0; i < workers && $# > 0; i++)); do
      "$operation" "$1" "$version" "$tag" &
      pids+=("$!")
      shift
    done
    failed=0
    for pid in "${pids[@]}"; do
      wait "$pid" || failed=1
    done
    [[ "$failed" -eq 0 ]] || return 1
  done
}

wait_for() {
  # Same bounded backoff the release's distribution guard uses, for the same
  # reason: npm scans a publish before serving it. npm documents about five
  # minutes as typical and 15 minutes or more at peak times, so the defaults
  # cover about 20 minutes without polling the registry aggressively.
  # Read at call time, not load time, so the test suite can shorten it.
  local name="$1" version="$2" attempt started="$SECONDS"
  local retries="${NPM_PUBLISH_RETRIES:-24}" delay="${NPM_PUBLISH_FIRST_DELAY:-5}"
  local max_delay="${NPM_PUBLISH_MAX_DELAY:-60}"
  for ((attempt = 1; attempt <= retries; attempt++)); do
    if published "$name" "$version"; then
      printf 'attempt %s: %s@%s is served\n' "$attempt" "$name" "$version"
      record_timing visibility "$name" "$version" "$started"
      return 0
    elif [[ "$?" -eq 1 ]]; then
      printf 'attempt %s: %s@%s absent\n' "$attempt" "$name" "$version"
    fi
    if [[ "$attempt" -lt "$retries" ]]; then
      sleep "$delay"
      delay=$((delay * 2))
      [[ "$delay" -le "$max_delay" ]] || delay="$max_delay"
    fi
  done
  err "$name@$version did not become visible after $retries attempts. Retry the same release when npm serves it."
  return 1
}

wait_for_dist_tag() {
  local name="$1" version="$2" tag="$3" state attempt started="$SECONDS"
  local retries="${NPM_PUBLISH_RETRIES:-24}" delay="${NPM_PUBLISH_FIRST_DELAY:-5}"
  local max_delay="${NPM_PUBLISH_MAX_DELAY:-60}"
  for ((attempt = 1; attempt <= retries; attempt++)); do
    if state="$(npm_dist_tag_state "$name" "$version")"; then
      case "$state" in
        current|superseded)
          record_timing dist-tag "$name" "$version" "$started" >&2
          printf '%s\n' "$state"
          return 0
          ;;
      esac
    fi
    if [[ "$attempt" -lt "$retries" ]]; then
      sleep "$delay"
      delay=$((delay * 2))
      [[ "$delay" -le "$max_delay" ]] || delay="$max_delay"
    fi
  done
  err "$name@$tag did not point to $version after $retries attempts. Retry the same release after registry availability recovers."
  return 1
}

# Every name the parent pins has to be one of the packages about to be
# published. A pin with no package is a parent that installs nothing on that
# platform, and it is the one mistake ordering alone does not catch.
require_every_pin() {
  local parent="$1" names="$2" version="$3" pinned pin pinned_version pinned_names=""
  pinned="$(cd "$parent" && node -p "Object.entries(require('./package.json').optionalDependencies || {}).map(entry => entry.join(' ')).join('\n')")" || return 1
  while read -r pin pinned_version; do
    [[ -n "$pin" ]] || continue
    if ! grep -qxF "$pin" <<< "$names"; then
      err "the parent pins $pin but no package directory builds it"
      return 1
    fi
    if [[ "$pinned_version" != "$version" ]]; then
      err "the parent pins $pin to $pinned_version, expected $version"
      return 1
    fi
    pinned_names+="$pin"$'\n'
  done <<< "$pinned"
  if [[ "$(sort <<< "$names")" != "$(printf '%s' "$pinned_names" | sort)" ]]; then
    err "the parent must pin every built platform package exactly once"
    return 1
  fi
}

main() {
  local version="${1:-}" platforms="${2:-$ROOT/npm/platforms}" parent="${3:-$ROOT/npm}"
  if [[ -z "$version" ]]; then
    err "usage: scripts/npm-publish.sh <version> [platforms-dir] [parent-dir]"
    return 2
  fi

  # Parallel arrays rather than one associative array: macOS still ships
  # bash 3.2, which has no associative arrays.
  local dirs=() names=() dir i name state intended_tag publish_tag parent_name parent_present=0
  local workers="${NPM_PUBLISH_WORKERS:-3}" started="$SECONDS"
  if [[ ! "$workers" =~ ^[1-6]$ ]]; then
    err "NPM_PUBLISH_WORKERS must be between 1 and 6"
    return 2
  fi
  for dir in "$platforms"/*/; do
    [[ -f "$dir/package.json" ]] || continue
    dirs+=("${dir%/}")
    name="$(package_name "${dir%/}" "$version")" || return 1
    names+=("$name")
  done
  if [[ "${#dirs[@]}" -eq 0 ]]; then
    err "no platform packages in $platforms; run npm/scripts/build-platform-packages.js first"
    return 1
  fi
  require_every_pin "$parent" "$(printf '%s\n' "${names[@]}")" "$version" || return 1
  parent_name="$(package_name "$parent" "$version")" || return 1
  local NPM_PUBLISH_MODULES
  NPM_PUBLISH_MODULES="$(npm root -g)" || return 1
  intended_tag="$(npm_dist_tag "$version")"
  publish_tag="$intended_tag"
  for name in "${names[@]}" "$parent_name"; do
    state="$(npm_dist_tag_state "$name" "$version")" || return 1
    if [[ "$state" == superseded ]]; then
      publish_tag="$(recovery_tag "$version")"
    fi
  done
  if [[ "$publish_tag" != "$intended_tag" ]]; then
    printf '::notice::A newer version owns %s. Missing packages at %s will publish under %s.\n' "$intended_tag" "$version" "$publish_tag"
  fi
  local existing_platforms=() pending_dirs=() pending_names=()
  for i in "${!names[@]}"; do
    if published "${names[$i]}" "$version"; then
      existing_platforms+=("${names[$i]}")
    elif [[ "$?" -eq 1 ]]; then
      pending_dirs+=("${dirs[$i]}")
      pending_names+=("${names[$i]}")
    else
      return 1
    fi
  done
  if published "$parent_name" "$version"; then
    verify_existing_package "$parent_name" "$version" || return 1
    parent_present=1
  elif [[ "$?" -ne 1 ]]; then
    return 1
  fi
  if [[ "${#existing_platforms[@]}" -gt 0 ]]; then
    run_workers verify_existing_package "$workers" "$version" "$publish_tag" "${existing_platforms[@]}" || return 1
  fi
  if [[ "${#pending_dirs[@]}" -gt 0 ]]; then
    run_workers publish_package "$workers" "$version" "$publish_tag" "${pending_dirs[@]}" || return 1
    run_workers wait_for "$workers" "$version" "$publish_tag" "${pending_names[@]}" || return 1
    settle_attestations || return 1
    run_workers verify_with_timing "$workers" "$version" "$publish_tag" "${pending_names[@]}" || return 1
  fi
  if [[ "$publish_tag" == "$intended_tag" && "${#existing_platforms[@]}" -gt 0 ]]; then
    run_workers refresh_existing_tag "$workers" "$version" "$intended_tag" "${existing_platforms[@]}" || return 1
  fi
  if [[ "$parent_present" -eq 0 ]]; then
    publish_package "$parent" "$version" "$publish_tag" || return 1
    wait_for "$parent_name" "$version" || return 1
    settle_attestations || return 1
    verify_with_timing "$parent_name" "$version" || return 1
  elif [[ "$publish_tag" == "$intended_tag" ]]; then
    refresh_existing_tag "$parent_name" "$version" "$intended_tag" || return 1
  fi
  state=superseded
  if [[ "$publish_tag" == "$intended_tag" ]]; then
    local pointers
    pointers="$(run_workers wait_for_dist_tag "$workers" "$version" "$intended_tag" "${names[@]}" "$parent_name")" || return 1
    if ! grep -Fxq superseded <<< "$pointers"; then
      state=verified
    fi
  fi
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    printf 'state=%s\n' "$state" >> "$GITHUB_OUTPUT"
  fi
  printf 'npm-publish: verified %s packages at %s in %ss\n' \
    "$((${#dirs[@]} + 1))" "$version" "$((SECONDS - started))"
}

if [[ -z "${BASH_SOURCE[0]:-}" || "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
