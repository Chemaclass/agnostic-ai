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

# Each line of $1 is one call's outcome. The calls
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
      busy) echo "hdiutil: $1 failed - Resource temporarily unavailable" >&2; return 35 ;;
      resource-busy) echo "hdiutil: $1 failed - Resource busy" >&2; return 16 ;;
      *) echo "hdiutil: $1 failed - image not recognized" >&2; return 1 ;;
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
  local tmp="$1" command="$2"
  shift 2
  printf '%s\n' "$@" > "$tmp/outcomes"
  stub_hdiutil "$tmp/outcomes" "$tmp/calls"
  stub_sleep "$tmp/slept"
  status=0
  output="$(hdiutil_retrying "$command" image.dmg 2>&1)" || status=$?
  calls="$(wc -l < "$tmp/calls" | tr -d ' ')"
  slept="$(tr '\n' ' ' < "$tmp/slept")"
  unstub_hdiutil
}

function test_it_runs_hdiutil_once_when_it_succeeds() {
  local tmp status output calls slept
  tmp="$(mktemp -d)"
  run_with_outcomes "$tmp" verify ok
  rm -rf "$tmp"

  assert_same "0" "$status"
  assert_same "1" "$calls"
  assert_contains "verified CRC32" "$output"
}

function test_it_retries_a_busy_resource_until_it_clears() {
  local tmp status output calls slept
  tmp="$(mktemp -d)"
  run_with_outcomes "$tmp" verify busy busy ok
  rm -rf "$tmp"

  assert_same "0" "$status"
  assert_same "3" "$calls"
  assert_same "5 10 " "$slept"
  assert_contains "hdiutil verify: resource busy, attempt 2 of 6" "$output"
}

function test_it_retries_when_creating_an_image_reports_resource_busy() {
  local tmp status output calls slept
  tmp="$(mktemp -d)"
  run_with_outcomes "$tmp" create resource-busy ok
  rm -rf "$tmp"

  assert_same "0" "$status"
  assert_same "2" "$calls"
  assert_same "5 " "$slept"
  assert_contains "hdiutil create: resource busy, attempt 1 of 6" "$output"
}

function test_it_fails_with_the_hdiutil_status_when_the_resource_stays_busy() {
  local tmp status output calls slept
  tmp="$(mktemp -d)"
  run_with_outcomes "$tmp" verify busy busy busy busy busy busy ok
  rm -rf "$tmp"

  assert_same "35" "$status"
  assert_same "6" "$calls"
  assert_same "5 10 20 40 80 " "$slept"
  assert_contains "Resource temporarily unavailable" "$output"
}

function test_it_fails_at_once_on_any_other_error() {
  local tmp status output calls slept
  tmp="$(mktemp -d)"
  run_with_outcomes "$tmp" verify broken ok
  rm -rf "$tmp"

  assert_same "1" "$status"
  assert_same "1" "$calls"
  assert_empty "$slept"
  assert_not_contains "resource busy" "$output"
}

# lsof reports $1 as holding the image, ps names it $2, and kill is recorded.
function stub_image_holder() {
  HOLDER_PID="$1"
  HOLDER_NAME="$2"
  KILLED="$3"
  : > "$KILLED"
  # shellcheck disable=SC2329
  function lsof() {
    if [ "$1" = -t ]; then
      [ -n "$HOLDER_PID" ] && echo "$HOLDER_PID"
      return 0
    fi
    echo "lsof $*"
  }
  # shellcheck disable=SC2329
  function ps() { echo "$HOLDER_NAME"; }
  # shellcheck disable=SC2329
  function pgrep() { echo "pgrep $*"; }
  # shellcheck disable=SC2329
  function hdiutil() { echo "hdiutil $*"; }
  # shellcheck disable=SC2329
  function kill() {
    echo "$*" >> "$KILLED"
    [ "$1" != -0 ]
  }
}

function unstub_image_holder() {
  unset -f lsof ps pgrep hdiutil kill sleep
}

function test_release_image_stops_the_helper_that_keeps_the_image_open() {
  local tmp output
  tmp="$(mktemp -d)"
  stub_image_holder 4242 /System/Library/PrivateFrameworks/DiskImages2.framework/XPCServices/diskimages-helper "$tmp/killed"
  stub_sleep "$tmp/slept"
  output="$(release_image image.dmg 2>&1)"
  local killed
  killed="$(tr '\n' '|' < "$tmp/killed")"
  unstub_image_holder
  rm -rf "$tmp"

  assert_contains "hdiutil create left image.dmg open" "$output"
  assert_contains "lsof -- image.dmg" "$output"
  assert_contains "hdiutil info" "$output"
  assert_contains "stopping the disk image helper: 4242" "$output"
  assert_same "4242|-0 4242|-9 4242|" "$killed"
}

function test_release_image_leaves_other_holders_alone() {
  local tmp output
  tmp="$(mktemp -d)"
  stub_image_holder 99 /usr/libexec/mds "$tmp/killed"
  output="$(release_image image.dmg 2>&1)"
  local killed
  killed="$(cat "$tmp/killed")"
  unstub_image_holder
  rm -rf "$tmp"

  assert_contains "hdiutil create left image.dmg open" "$output"
  assert_not_contains "stopping" "$output"
  assert_empty "$killed"
}

function test_release_image_does_nothing_when_the_image_is_closed() {
  local tmp output
  tmp="$(mktemp -d)"
  stub_image_holder "" "" "$tmp/killed"
  output="$(release_image image.dmg 2>&1)"
  local killed
  killed="$(cat "$tmp/killed")"
  unstub_image_holder
  rm -rf "$tmp"

  assert_empty "$output"
  assert_empty "$killed"
}

function stub_go_download() {
  GO_OUTCOMES="$1"
  GO_CALLS="$2"
  : > "$GO_CALLS"
  # shellcheck disable=SC2329
  function go() {
    printf '%s\n' "$*" >> "$GO_CALLS"
    case "$(sed -n "$(wc -l < "$GO_CALLS")p" "$GO_OUTCOMES")" in
      ok) return 0 ;;
      timeout) echo 'go: lookup proxy.golang.org: i/o timeout' >&2; return 1 ;;
      checksum) echo 'go: checksum mismatch (previous request: i/o timeout)' >&2; return 2 ;;
    esac
  }
}

function test_go_dependency_fetch_retries_the_release_dns_failure() {
  local tmp status=0 output
  tmp="$(mktemp -d)"
  printf '%s\n' timeout timeout ok > "$tmp/outcomes"
  stub_go_download "$tmp/outcomes" "$tmp/calls"
  stub_sleep "$tmp/slept"
  output="$(download_modules 2>&1)" || status=$?
  assert_same "0" "$status"
  assert_contains "lookup proxy.golang.org: i/o timeout" "$output"
  assert_same "3" "$(wc -l < "$tmp/calls" | tr -d ' ')"
  assert_same "5 10 " "$(tr '\n' ' ' < "$tmp/slept")"
  unset -f go sleep
  rm -rf "$tmp"
}

function test_go_dependency_fetch_does_not_retry_a_checksum_error() {
  local tmp status=0 output
  tmp="$(mktemp -d)"
  printf '%s\n' checksum ok > "$tmp/outcomes"
  stub_go_download "$tmp/outcomes" "$tmp/calls"
  stub_sleep "$tmp/slept"
  output="$(download_modules 2>&1)" || status=$?
  assert_same "2" "$status"
  assert_contains "checksum mismatch" "$output"
  assert_same "1" "$(wc -l < "$tmp/calls" | tr -d ' ')"
  assert_empty "$(cat "$tmp/slept")"
  unset -f go sleep
  rm -rf "$tmp"
}
