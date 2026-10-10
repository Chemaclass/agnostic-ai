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

# ---- fixtures ----------------------------------------------------------------

# A parent and two platform packages on disk. Two is enough to prove ordering
# and the wait; the real count is guarded in npm/lib/platforms_test.js.
function fake_tree() {
  local root="$1" name
  mkdir -p "$root/npm/platforms/darwin-arm64" "$root/npm/platforms/linux-x64"
  cat > "$root/npm/package.json" <<'JSON'
{
  "name": "agnostic-ai",
  "version": "1.2.3",
  "optionalDependencies": {
    "@agnostic-ai/darwin-arm64": "1.2.3",
    "@agnostic-ai/linux-x64": "1.2.3"
  }
}
JSON
  for name in darwin-arm64 linux-x64; do
    printf '{"name":"@agnostic-ai/%s","version":"1.2.3"}\n' "$name" \
      > "$root/npm/platforms/$name/package.json"
  done
}

# STUB_LOG records every npm call. The set of published packages lives in a
# file, not a variable: publish_package runs `npm publish` inside a subshell,
# so a variable it assigns never reaches the next call.
function stub_npm() {
  STUB_LOG="$1"
  STUB_PRESENT="$1.present"
  STUB_PUBLISH_FAILS="${3:-}"
  # shellcheck disable=SC2329
  function verify_npm_provenance() {
    printf 'verify %s@%s\n' "$1" "$2" >> "$STUB_LOG"
    [[ "${STUB_UNATTESTED:-}" != "$1" ]]
  }
  : > "$STUB_LOG"
  printf '%s\n' "${2:-}" > "$STUB_PRESENT"
  # shellcheck disable=SC2317,SC2329
  function npm() {
    case "$1" in
      view)
        printf 'view %s\n' "$2" >> "$STUB_LOG"
        grep -qxF "${2%@*}" "$STUB_PRESENT"
        ;;
      publish)
        local name provenance="plain" tag="<none>" arg next=""
        name="$(node -p "require('./package.json').name")"
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
        # A successful publish makes the package visible to later views.
        printf '%s\n' "$name" >> "$STUB_PRESENT"
        ;;
    esac
  }
}

function unstub_npm() {
  unset -f npm verify_npm_provenance
  unset STUB_LOG STUB_PRESENT STUB_PUBLISH_FAILS
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

  assert_same "publish @agnostic-ai/darwin-arm64 provenance latest
publish @agnostic-ai/linux-x64 provenance latest
publish agnostic-ai provenance latest" "$log"
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
      view)
        printf 'view %s\n' "$2" >> "$STUB_LOG"
        [[ "$2" != "@agnostic-ai/linux-x64@1.2.3" ]] && grep -qxF "${2%@*}" "$STUB_PRESENT"
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
      view)
        # linux-x64 shows up only after its publish has been attempted: the
        # tarball landed, attaching the attestation is what failed.
        [[ "$2" == "@agnostic-ai/linux-x64@1.2.3" && -f "$STUB_LOG.attempted" ]] && return 0
        grep -qxF "${2%@*}" "$STUB_PRESENT"
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
  fake_tree "$tmp"
  stub_npm "$tmp/log"
  main 1.2.3-beta.1 "$tmp/npm/platforms" "$tmp/npm" > /dev/null
  log="$(grep '^publish' "$tmp/log")"
  unstub_npm
  rm -rf "$tmp"

  assert_same "publish @agnostic-ai/darwin-arm64 provenance beta
publish @agnostic-ai/linux-x64 provenance beta
publish agnostic-ai provenance beta" "$log"
}

# A failed prerelease must not trigger a second plain publish.
function test_a_failed_prerelease_never_retries_plain() {
  local tmp log
  tmp="$(mktemp -d)"
  fake_tree "$tmp"
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
