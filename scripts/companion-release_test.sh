#!/usr/bin/env bash
#
# bashunit tests for scripts/companion-release.sh
#
# Run:
#   bashunit scripts/companion-release_test.sh
#
# `hdiutil` is stubbed as a shell function that answers from a script of
# outcomes, one line per call, so the retry is checked without macOS.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/companion-release.sh"

# Each line of $1 is one call's outcome: "ok", "busy" or "broken". The calls
# are counted in a file because the stub runs in a command substitution.
function stub_hdiutil() {
  HDIUTIL_OUTCOMES="$1"
  HDIUTIL_CALLS="$2"
  : > "$HDIUTIL_CALLS"
  # shellcheck disable=SC2329
  function hdiutil() {
    echo "$*" >> "$HDIUTIL_CALLS"
    case "$(sed -n "$(wc -l < "$HDIUTIL_CALLS")p" "$HDIUTIL_OUTCOMES")" in
      ok) echo "verified CRC32 \$1234" ;;
      busy) echo "hdiutil: verify failed - Resource temporarily unavailable" >&2; return 35 ;;
      *) echo "hdiutil: verify failed - image not recognized" >&2; return 1 ;;
    esac
  }
}

# The waits are recorded instead of slept.
function stub_sleep() {
  SLEPT="$1"
  : > "$SLEPT"
  # shellcheck disable=SC2329
  function sleep() {
    echo "$1" >> "$SLEPT"
  }
}

function unstub_hdiutil() {
  unset -f hdiutil sleep
}

function run_with_outcomes() {
  local tmp="$1"
  shift
  printf '%s\n' "$@" > "$tmp/outcomes"
  stub_hdiutil "$tmp/outcomes" "$tmp/calls"
  stub_sleep "$tmp/slept"
  status=0
  output="$(hdiutil_retrying verify image.dmg 2>&1)" || status=$?
  calls="$(wc -l < "$tmp/calls" | tr -d ' ')"
  slept="$(tr '\n' ' ' < "$tmp/slept")"
  unstub_hdiutil
}

function test_it_runs_hdiutil_once_when_it_succeeds() {
  local tmp status output calls slept
  tmp="$(mktemp -d)"
  run_with_outcomes "$tmp" ok
  rm -rf "$tmp"

  assert_same "0" "$status"
  assert_same "1" "$calls"
  assert_contains "verified CRC32" "$output"
}

function test_it_retries_a_busy_resource_until_it_clears() {
  local tmp status output calls slept
  tmp="$(mktemp -d)"
  run_with_outcomes "$tmp" busy busy ok
  rm -rf "$tmp"

  assert_same "0" "$status"
  assert_same "3" "$calls"
  assert_same "5 10 " "$slept"
  assert_contains "hdiutil verify: resource busy, attempt 2 of 4" "$output"
}

function test_it_fails_with_the_hdiutil_status_when_the_resource_stays_busy() {
  local tmp status output calls slept
  tmp="$(mktemp -d)"
  run_with_outcomes "$tmp" busy busy busy busy ok
  rm -rf "$tmp"

  assert_same "35" "$status"
  assert_same "4" "$calls"
  assert_same "5 10 20 " "$slept"
  assert_contains "Resource temporarily unavailable" "$output"
}

function test_it_fails_at_once_on_any_other_error() {
  local tmp status output calls slept
  tmp="$(mktemp -d)"
  run_with_outcomes "$tmp" broken ok
  rm -rf "$tmp"

  assert_same "1" "$status"
  assert_same "1" "$calls"
  assert_empty "$slept"
  assert_not_contains "resource busy" "$output"
}

function test_it_signs_the_app_with_a_requirement_on_its_identifier_alone() {
  local tmp calls
  tmp="$(mktemp -d)"
  CODESIGN_CALLS="$tmp/calls"
  # shellcheck disable=SC2329
  function codesign() {
    printf '%s|' "$@" >> "$CODESIGN_CALLS"
    echo >> "$CODESIGN_CALLS"
  }
  sign_companion_app "my.app"
  calls="$(cat "$tmp/calls")"
  unset -f codesign
  rm -rf "$tmp"

  assert_same '--force|--sign|-|--identifier|dev.agnostic-ai.companion|-r=designated => identifier "dev.agnostic-ai.companion"|my.app|' "$calls"
}
