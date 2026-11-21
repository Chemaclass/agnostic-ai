#!/usr/bin/env bash
#
# bashunit tests for scripts/npm-publish.sh
#
# Run:
#   bashunit scripts/npm-publish_test.sh
#
# Seven packages have to reach the registry at one version or installs break on
# whichever platform is missing. `npm` is stubbed as a shell function that logs
# what it was asked to do, so the ordering and the failure handling are checked
# without publishing anything.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/npm-publish.sh"

# The retries are what the release needs and what a test cannot wait for.
export NPM_PUBLISH_RETRIES=2
export NPM_PUBLISH_FIRST_DELAY=0
export NPM_PROVENANCE_RETRIES=3
export NPM_ATTESTATION_SETTLE=0
unset GITHUB_OUTPUT

# ---- fixtures ----------------------------------------------------------------

# A parent and two platform packages on disk. Two is enough to prove ordering
# and the wait; the real count is guarded in npm/lib/platforms_test.js.
function fake_tree() {
  local root="$1" version="${2:-1.2.3}" name
  mkdir -p "$root/npm/platforms/darwin-arm64" "$root/npm/platforms/linux-x64"
  cat > "$root/npm/package.json" <<JSON
{
  "name": "agnostic-ai",
  "version": "$version",
  "optionalDependencies": {
    "@agnostic-ai/darwin-arm64": "$version",
    "@agnostic-ai/linux-x64": "$version"
  }
}
JSON
  for name in darwin-arm64 linux-x64; do
    printf '{"name":"@agnostic-ai/%s","version":"%s"}\n' "$name" "$version" \
      > "$root/npm/platforms/$name/package.json"
  done
}

# STUB_LOG records every npm call. The set of published packages lives in a
# file, not a variable: publish_package runs `npm publish` inside a subshell,
# so a variable it assigns never reaches the next call.
function stub_npm() {
  STUB_LOG="$1"
  STUB_PRESENT="$1.present"
  STUB_TAGS="$1.tags"
  STUB_PUBLISH_FAILS="${3:-}"
  # shellcheck disable=SC2329
  function verify_npm_provenance() {
    printf 'verify %s@%s\n' "$1" "$2" >> "$STUB_LOG"
    [[ "${STUB_UNATTESTED:-}" != "$1" ]]
  }
  : > "$STUB_LOG"
  printf '%s\n' "${2:-}" > "$STUB_PRESENT"
  : > "$STUB_TAGS"
  # shellcheck disable=SC2317,SC2329
  function npm() {
    case "$1" in
      root) command npm "$@" ;;
      view)
        printf 'view %s\n' "$2" >> "$STUB_LOG"
        if [[ "${STUB_METADATA_FAILS:-}" == "$2 $3" ]]; then
          printf '{"error":{"code":"E503"}}\n'
          return 1
        fi
        if [[ "$3" == dist-tags ]]; then
          awk -v name="$2" '
            $1 == name { tags[$2] = $3 }
            END {
              printf "{"
              sep = ""
              for (tag in tags) {
                printf "%s\"%s\":\"%s\"", sep, tag, tags[tag]
                sep = ","
              }
              print "}"
            }
          ' "$STUB_TAGS"
        elif grep -qxF "${2%@*}" "$STUB_PRESENT"; then
          printf '"%s"\n' "${2##*@}"
        else
          printf '{"error":{"code":"E404"}}\n'
          return 1
        fi
        ;;
      publish)
        local name version provenance="plain" tag="<none>" arg next="" attempt
        name="$(node -p "require('./package.json').name")"
        version="$(node -p "require('./package.json').version")"
        [[ "$*" == *--provenance* ]] && provenance="provenance"
        # An untagged publish writes `latest`, so the log records the tag the
        # script asked for, not just that it published.
        for arg in "$@"; do
          [[ -n "$next" ]] && { tag="$arg"; next=""; continue; }
          [[ "$arg" == "--tag" ]] && next=1
        done
        printf 'publish %s %s %s\n' "$name" "$provenance" "$tag" >> "$STUB_LOG"
        if grep -qxF "$name $provenance" <<< "$STUB_PUBLISH_FAILS"; then
          return 1
        fi
        if [[ "${STUB_OVERLAP:-}" == yes && "$name" == @agnostic-ai/* ]]; then
          touch "$STUB_LOG.${name##*/}.started"
          for ((attempt = 0; attempt < 200; attempt++)); do
            if [[ -f "$STUB_LOG.darwin-arm64.started" && -f "$STUB_LOG.linux-x64.started" ]]; then
              break
            fi
            sleep 0.05
          done
          [[ "$attempt" -lt 200 ]] || return 1
        fi
        # A successful publish makes the package visible to later views.
        printf '%s\n' "$name" >> "$STUB_PRESENT"
        if [[ "${STUB_STALE_TAG:-}" != "$name" ]]; then
          if [[ "${STUB_SUPERSEDES:-}" == "$name" ]]; then
            version=1.2.10
          fi
          printf '%s %s %s\n' "$name" "$tag" "$version" >> "$STUB_TAGS"
        fi
        ;;
      dist-tag)
        printf 'tag %s %s\n' "$3" "$4" >> "$STUB_LOG"
        printf '%s %s %s\n' "${3%@*}" "$4" "${3##*@}" >> "$STUB_TAGS"
        ;;
    esac
  }
}

function unstub_npm() {
  unset -f npm verify_npm_provenance
  unset STUB_LOG STUB_PRESENT STUB_PUBLISH_FAILS STUB_TAGS
}

# ---- ordering ----------------------------------------------------------------

function test_it_publishes_every_platform_package_before_the_parent() {
  local tmp log
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  stub_npm "$tmp/log"
  main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" > /dev/null
  log="$(grep '^publish' "$tmp/log")"
  unstub_npm
  rm -rf "$tmp"

  assert_contains "publish @agnostic-ai/darwin-arm64 provenance latest" "$log"
  assert_contains "publish @agnostic-ai/linux-x64 provenance latest" "$log"
  assert_same "publish agnostic-ai provenance latest" "$(tail -1 <<< "$log")"
}

function test_it_waits_for_every_platform_package_before_the_parent() {
  local tmp order
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  stub_npm "$tmp/log"
  main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" > /dev/null
  # The last view of a platform package has to come before the parent publish.
  order="$(grep -n 'view @agnostic-ai/linux-x64@1.2.3\|publish agnostic-ai ' "$tmp/log" | tail -2 | cut -d: -f2- | cut -d' ' -f1)"
  unstub_npm
  rm -rf "$tmp"

  assert_same "view
publish" "$order"
}

function test_it_refuses_to_publish_the_parent_when_a_platform_package_never_lands() {
  local tmp code log
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  # linux-x64 publishes without error and never becomes visible.
  stub_npm "$tmp/log"
  # shellcheck disable=SC2317,SC2329
  function npm() {
    case "$1" in
      root) command npm "$@" ;;
      view)
        printf 'view %s\n' "$2" >> "$STUB_LOG"
        if [[ "$3" == dist-tags ]]; then
          printf '{}\n'
        elif [[ "$2" != "@agnostic-ai/linux-x64@1.2.3" ]] && grep -qxF "${2%@*}" "$STUB_PRESENT"; then
          printf '"%s"\n' "${2##*@}"
        else
          printf '{"error":{"code":"E404"}}\n'
          return 1
        fi
        ;;
      publish)
        local name
        name="$(node -p "require('./package.json').name")"
        printf 'publish %s\n' "$name" >> "$STUB_LOG"
        printf '%s\n' "$name" >> "$STUB_PRESENT"
        ;;
    esac
  }
  code="$(main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" > /dev/null 2>&1; echo $?)"
  log="$(cat "$tmp/log")"
  unstub_npm
  rm -rf "$tmp"

  assert_not_same "0" "$code"
  assert_not_contains "publish agnostic-ai" "$log"
}

# A read before npm finishes signing caches an incomplete attestation for a
# year, so no package is verified until every platform package is up.
function test_it_verifies_only_after_every_platform_package_is_published() {
  local tmp order
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  stub_npm "$tmp/log"
  main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" > /dev/null
  order="$(grep -E '^(publish|verify)' "$tmp/log" | cut -d' ' -f1)"
  unstub_npm
  rm -rf "$tmp"

  assert_same "publish
publish
verify
verify
publish
verify" "$order"
}

function test_it_waits_for_signing_only_after_an_upload() {
  local tmp out
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  stub_npm "$tmp/log" "@agnostic-ai/darwin-arm64
@agnostic-ai/linux-x64"
  out="$(NPM_ATTESTATION_SETTLE=1 main 1.2.3 "$tmp/npm/platforms" "$tmp/npm")"
  unstub_npm
  rm -rf "$tmp"

  assert_same "1" "$(grep -c 'waiting 1s for npm to finish signing' <<< "$out")"
}

# ---- idempotence -------------------------------------------------------------

function test_it_skips_a_package_already_on_the_registry() {
  local tmp log
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  stub_npm "$tmp/log" "@agnostic-ai/darwin-arm64"
  main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" > /dev/null
  log="$(grep '^publish' "$tmp/log")"
  unstub_npm
  rm -rf "$tmp"

  assert_not_contains "publish @agnostic-ai/darwin-arm64" "$log"
  assert_contains "publish @agnostic-ai/linux-x64" "$log"
  assert_contains "publish agnostic-ai" "$log"
}

# ---- provenance --------------------------------------------------------------

function test_a_provenance_failure_never_retries_plain() {
  local tmp log code
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  stub_npm "$tmp/log" "" "@agnostic-ai/darwin-arm64 provenance"
  code="$(main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" > /dev/null 2>&1; echo $?)"
  log="$(cat "$tmp/log")"
  unstub_npm
  rm -rf "$tmp"

  assert_not_same "0" "$code"
  assert_contains "publish @agnostic-ai/darwin-arm64 provenance latest" "$log"
  assert_not_contains " plain " "$log"
}

function test_a_platform_publish_failure_stops_the_release() {
  local tmp code
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  stub_npm "$tmp/log" "" "@agnostic-ai/linux-x64 provenance"
  code="$(main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" > /dev/null 2>&1; echo $?)"
  unstub_npm
  rm -rf "$tmp"

  assert_not_same "0" "$code"
}

# An uploaded tarball without its required attestation is still a failed release.
function test_a_tarball_uploaded_without_attestation_fails() {
  local tmp code
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  stub_npm "$tmp/log"
  STUB_UNATTESTED="@agnostic-ai/linux-x64"
  # shellcheck disable=SC2317,SC2329
  function npm() {
    case "$1" in
      root) command npm "$@" ;;
      view)
        # linux-x64 shows up only after its publish has been attempted: the
        # tarball landed, attaching the attestation is what failed.
        if [[ "$3" == dist-tags ]]; then
          printf '{}\n'
        elif [[ "$2" == "@agnostic-ai/linux-x64@1.2.3" && -f "$STUB_LOG.attempted" ]] || grep -qxF "${2%@*}" "$STUB_PRESENT"; then
          printf '"%s"\n' "${2##*@}"
        else
          printf '{"error":{"code":"E404"}}\n'
          return 1
        fi
        ;;
      publish)
        local name
        name="$(node -p "require('./package.json').name")"
        if [[ "$name" == "@agnostic-ai/linux-x64" ]]; then
          : > "$STUB_LOG.attempted"
          return 1
        fi
        printf '%s\n' "$name" >> "$STUB_PRESENT"
        ;;
    esac
  }
  code="$(main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" > /dev/null 2>&1; echo $?)"
  unset STUB_UNATTESTED
  unstub_npm
  rm -rf "$tmp"

  assert_not_same "0" "$code"
}

# The registry can serve a version before its attestations verify. A check
# that passes on a later attempt is a complete release, not a broken one.
function test_provenance_that_verifies_late_passes() {
  local tmp code
  tmp="$(mktemp -d)"
  # shellcheck disable=SC1091
  source "$SCRIPT_DIR/npm-publish.sh"
  # shellcheck disable=SC2329
  function npm_provenance_verifies() {
    printf 'x' >> "$tmp/attempts"
    [[ "$(cat "$tmp/attempts")" == "xx" ]]
  }
  code="$(verify_npm_provenance agnostic-ai 1.2.3 > /dev/null 2>&1; echo $?)"
  local attempts
  attempts="$(cat "$tmp/attempts")"
  unset -f npm_provenance_verifies
  # shellcheck disable=SC1091
  source "$SCRIPT_DIR/npm-publish.sh"
  rm -rf "$tmp"

  assert_same "0" "$code"
  assert_same "xx" "$attempts"
}

function test_provenance_that_never_verifies_fails_after_the_retries() {
  local tmp code
  tmp="$(mktemp -d)"
  # shellcheck disable=SC1091
  source "$SCRIPT_DIR/npm-publish.sh"
  # shellcheck disable=SC2329
  function npm_provenance_verifies() {
    printf 'x' >> "$tmp/attempts"
    return 1
  }
  code="$(verify_npm_provenance agnostic-ai 1.2.3 > /dev/null 2>&1; echo $?)"
  local attempts
  attempts="$(cat "$tmp/attempts")"
  unset -f npm_provenance_verifies
  # shellcheck disable=SC1091
  source "$SCRIPT_DIR/npm-publish.sh"
  rm -rf "$tmp"

  assert_not_same "0" "$code"
  assert_same "xxx" "$attempts"
}

# ---- dist-tag ----------------------------------------------------------------

# npm defaults an untagged publish to `latest`, which is what an unpinned
# `npm install agnostic-ai` resolves. A prerelease that lands there replaces
# the stable release for everyone who did not pin.
function test_a_stable_version_owns_latest() {
  assert_same "latest" "$(npm_dist_tag 1.2.3)"
  assert_same "latest" "$(npm_dist_tag v1.2.3)"
}

# The tag comes from the suffix, so two prerelease lines never overwrite each
# other's channel.
function test_each_prerelease_line_gets_its_own_tag() {
  assert_same "beta" "$(npm_dist_tag 1.2.3-beta.1)"
  assert_same "rc" "$(npm_dist_tag 1.2.3-rc.1)"
  assert_same "alpha" "$(npm_dist_tag 1.2.3-alpha.0)"
}

# A date stamp, a bare build id, or `latest` itself is not a tag name we can
# hand npm: the registry refuses a dist-tag that parses as a version, and
# `latest` is the one tag a prerelease must never take.
function test_an_unrecognised_suffix_parks_on_next() {
  assert_same "next" "$(npm_dist_tag 1.2.3-20240101)"
  assert_same "next" "$(npm_dist_tag 1.2.3-latest)"
  assert_same "next" "$(npm_dist_tag 1.2.3-foo_bar)"
}

function test_it_publishes_a_prerelease_under_its_own_tag() {
  local tmp log
  tmp="$(mktemp -d)"
  fake_tree "$tmp" 1.2.3-beta.1
  stub_npm "$tmp/log"
  main 1.2.3-beta.1 "$tmp/npm/platforms" "$tmp/npm" > /dev/null
  log="$(grep '^publish' "$tmp/log")"
  unstub_npm
  rm -rf "$tmp"

  assert_contains "publish @agnostic-ai/darwin-arm64 provenance beta" "$log"
  assert_contains "publish @agnostic-ai/linux-x64 provenance beta" "$log"
  assert_same "publish agnostic-ai provenance beta" "$(tail -1 <<< "$log")"
}

# A failed prerelease must not trigger a second plain publish.
function test_a_failed_prerelease_never_retries_plain() {
  local tmp log
  tmp="$(mktemp -d)"
  fake_tree "$tmp" 1.2.3-rc.2
  stub_npm "$tmp/log" "" "@agnostic-ai/darwin-arm64 provenance"
  main 1.2.3-rc.2 "$tmp/npm/platforms" "$tmp/npm" > /dev/null 2>&1 || true
  log="$(cat "$tmp/log")"
  unstub_npm
  rm -rf "$tmp"

  assert_not_contains " plain " "$log"
}

# ---- arguments ---------------------------------------------------------------

function test_it_fails_when_the_generator_has_not_run() {
  local tmp out
  tmp="$(mktemp -d)"
  mkdir -p "$tmp/npm/platforms"
  printf '{"name":"agnostic-ai","version":"1.2.3"}\n' > "$tmp/npm/package.json"
  out="$(main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" 2>&1 || true)"
  rm -rf "$tmp"

  assert_contains "no platform packages" "$out"
}

# Ordering does not help when the parent pins a package no directory builds:
# the six publish, the parent publishes, and one platform installs nothing.
function test_it_refuses_when_the_parent_pins_a_package_that_was_not_built() {
  local tmp out
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  rm -rf "${tmp:?}/npm/platforms/linux-x64"
  stub_npm "$tmp/log"
  out="$(main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" 2>&1 || true)"
  unstub_npm
  rm -rf "$tmp"

  assert_contains "pins @agnostic-ai/linux-x64 but no package directory builds it" "$out"
}

function test_it_needs_a_version() {
  assert_same "2" "$(main > /dev/null 2>&1; echo $?)"
}

function test_an_existing_unattested_version_fails_without_publishing_more() {
  local tmp code log
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  stub_npm "$tmp/log" "@agnostic-ai/darwin-arm64"
  STUB_UNATTESTED="@agnostic-ai/darwin-arm64"
  code="$(main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" >/dev/null 2>&1; echo $?)"
  log="$(cat "$tmp/log")"
  unset STUB_UNATTESTED
  unstub_npm
  rm -rf "$tmp"
  assert_not_same "0" "$code"
  assert_not_contains "publish " "$log"
}

function test_every_parent_and_platform_package_is_verified() {
  local tmp log name
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  for name in darwin-x64 linux-arm64 win32-arm64 win32-x64; do
    mkdir -p "$tmp/npm/platforms/$name"
    printf '{"name":"@agnostic-ai/%s","version":"1.2.3"}\n' "$name" > "$tmp/npm/platforms/$name/package.json"
  done
  (
    cd "$tmp/npm" || exit 1
    node -e '
      const fs = require("node:fs")
      const parent = require("./package.json")
      for (const platform of fs.readdirSync("platforms")) {
        parent.optionalDependencies["@agnostic-ai/" + platform] = "1.2.3"
      }
      fs.writeFileSync("package.json", JSON.stringify(parent))
    '
  )
  stub_npm "$tmp/log"
  main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" >/dev/null
  log="$(cat "$tmp/log")"
  unstub_npm
  rm -rf "$tmp"
  for name in agnostic-ai @agnostic-ai/darwin-arm64 @agnostic-ai/darwin-x64 @agnostic-ai/linux-arm64 @agnostic-ai/linux-x64 @agnostic-ai/win32-arm64 @agnostic-ai/win32-x64; do
    assert_contains "verify $name@1.2.3" "$log"
  done
}

function test_partial_publish_recovery_skips_verified_platforms() {
  local tmp code log
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  stub_npm "$tmp/log" "" "@agnostic-ai/linux-x64 provenance"
  code="$(main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" >/dev/null 2>&1; echo $?)"
  assert_not_same "0" "$code"
  STUB_PUBLISH_FAILS=""
  : > "$tmp/log"
  main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" >/dev/null
  log="$(cat "$tmp/log")"
  unstub_npm
  rm -rf "$tmp"
  assert_not_contains "publish @agnostic-ai/darwin-arm64" "$log"
  assert_contains "verify @agnostic-ai/darwin-arm64@1.2.3" "$log"
  assert_contains "publish @agnostic-ai/linux-x64 provenance latest" "$log"
  assert_contains "verify agnostic-ai@1.2.3" "$log"
}

function test_platform_publishes_overlap_and_finish_before_the_parent() {
  local tmp code out
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  stub_npm "$tmp/log"
  code=0
  out="$(STUB_OVERLAP=yes NPM_PUBLISH_WORKERS=2 main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" 2>&1)" || code=$?
  unstub_npm
  rm -rf "$tmp"

  if [[ "$code" -ne 0 ]]; then printf '%s\n' "$out"; fi
  assert_same "0" "$code"
  assert_contains "publish @agnostic-ai/darwin-arm64@1.2.3 took" "$out"
  assert_contains "visibility @agnostic-ai/linux-x64@1.2.3 took" "$out"
  assert_contains "provenance agnostic-ai@1.2.3 took" "$out"
}

function test_recovery_of_a_superseded_release_keeps_the_newer_pointer() {
  local tmp log tags status
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  stub_npm "$tmp/log"
  printf '%s\n' "agnostic-ai latest 1.2.10" "@agnostic-ai/darwin-arm64 latest 1.2.10" > "$STUB_TAGS"
  GITHUB_OUTPUT="$tmp/status" main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" > /dev/null
  log="$(cat "$tmp/log")"
  tags="$(cat "$STUB_TAGS")"
  status="$(cat "$tmp/status")"
  unstub_npm
  rm -rf "$tmp"

  assert_contains "publish agnostic-ai provenance recovery-v1.2.3" "$log"
  assert_contains "verify agnostic-ai@1.2.3" "$log"
  assert_contains "verify @agnostic-ai/linux-x64@1.2.3" "$log"
  assert_contains "verify @agnostic-ai/darwin-arm64@1.2.3" "$log"
  assert_contains "agnostic-ai latest 1.2.10" "$tags"
  assert_not_contains "latest 1.2.3" "$tags"
  assert_same "state=superseded" "$status"
}

function test_a_newer_prerelease_in_the_same_channel_supersedes_numeric_identifiers() {
  local tmp state
  tmp="$(mktemp -d)"
  stub_npm "$tmp/log"
  printf '%s\n' "agnostic-ai rc 1.2.3-rc.10" "agnostic-ai latest 1.2.2" > "$STUB_TAGS"
  state="$(npm_dist_tag_state agnostic-ai 1.2.3-rc.2)"
  unstub_npm
  rm -rf "$tmp"

  assert_same "superseded" "$state"
}

function test_registry_failure_is_not_mistaken_for_a_missing_package() {
  local tmp code=0 out log
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  stub_npm "$tmp/log"
  out="$(STUB_METADATA_FAILS="agnostic-ai@1.2.3 version" main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" 2>&1)" || code=$?
  log="$(cat "$tmp/log")"
  unstub_npm
  rm -rf "$tmp"

  assert_not_same "0" "$code"
  assert_contains "cannot read version for agnostic-ai@1.2.3" "$out"
  assert_not_contains "publish " "$log"
}

function test_parent_pins_have_to_match_the_release_version() {
  local tmp output
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  (
    cd "$tmp/npm" || exit 1
    node -e '
      const fs = require("node:fs")
      const manifest = require("./package.json")
      manifest.optionalDependencies["@agnostic-ai/darwin-arm64"] = "1.2.2"
      fs.writeFileSync("package.json", JSON.stringify(manifest))
    '
  )
  output="$(main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" 2>&1 || true)"
  rm -rf "$tmp"

  assert_contains "pins @agnostic-ai/darwin-arm64 to 1.2.2, expected 1.2.3" "$output"
}

function test_npm_publish_fails_when_a_pointer_stays_stale() {
  local tmp out code=0
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  stub_npm "$tmp/log"
  printf 'agnostic-ai latest 1.2.2\n' > "$STUB_TAGS"
  out="$(STUB_STALE_TAG=agnostic-ai GITHUB_OUTPUT="$tmp/status" main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" 2>&1)" || code=$?
  unstub_npm

  assert_not_same "0" "$code"
  assert_contains "agnostic-ai@latest did not point to 1.2.3" "$out"
  assert_file_not_exists "$tmp/status"
  rm -rf "$tmp"
}

function test_publisher_accepts_a_newer_pointer_after_publish() {
  local tmp status tags
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
  stub_npm "$tmp/log"
  STUB_SUPERSEDES=agnostic-ai GITHUB_OUTPUT="$tmp/status" main 1.2.3 "$tmp/npm/platforms" "$tmp/npm" > /dev/null
  status="$(cat "$tmp/status")"
  tags="$(cat "$STUB_TAGS")"
  unstub_npm
  rm -rf "$tmp"

  assert_same "state=superseded" "$status"
  assert_contains "agnostic-ai latest 1.2.10" "$tags"
  assert_not_contains "agnostic-ai latest 1.2.3" "$tags"
}
