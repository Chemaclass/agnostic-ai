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

# lsof reports $1 as holding the image until it is signalled, then for
# $HOLDER_LINGERS more checks. Its full command is $2, signals go to $3,
# and hdiutil info lists $HOLDER_ATTACHED as attached, when set.
function stub_image_holder() {
  HOLDER_PID="$1"
  HOLDER_NAME="$2"
  KILLED="$3"
  HOLDER_CHECKS="$KILLED.checks"
  DETACHED="$KILLED.detached"
  : > "$KILLED"
  : > "$HOLDER_CHECKS"
  : > "$DETACHED"
  # shellcheck disable=SC2329
  function lsof() {
    if [ "$1" = -t ]; then
      [ -s "$KILLED" ] && echo x >> "$HOLDER_CHECKS"
      if [ -n "$HOLDER_PID" ] && { [ ! -s "$KILLED" ] || [ "$(wc -l < "$HOLDER_CHECKS")" -le "${HOLDER_LINGERS:-0}" ]; }; then
        echo "$HOLDER_PID"
        return 0
      fi
      return 1
    fi
    echo "lsof $*"
  }
  # shellcheck disable=SC2329
  function ps() { echo "diskimages-help"; }
  # shellcheck disable=SC2329
  function pgrep() {
    if [ "$1" = -lf ]; then
      echo "$HOLDER_PID $HOLDER_NAME"
    elif [ "$1" = -f ] && [[ "$HOLDER_NAME" =~ $2 ]]; then
      echo "$HOLDER_PID"
    else
      return 1
    fi
  }
  # shellcheck disable=SC2329
  function hdiutil() {
    case "$*" in
      "info -plist")
        case "${HOLDER_INFO:-ok}" in
          unavailable) return 1 ;;
          malformed) echo '<plist><dict>'; return 0 ;;
        esac
        cat <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict><key>images</key><array>
<dict>
  <key>image-path</key><string>/different/image.dmg</string>
  <key>system-entities</key><array><dict><key>dev-entry</key><string>/dev/disk2</string></dict></array>
</dict>
PLIST
        if [ -n "${HOLDER_ATTACHED:-}" ]; then
          local n
          for ((n=0; n<${HOLDER_MATCHES:-1}; n++)); do
            cat <<PLIST
<dict>
  <key>image-path</key><string>$HOLDER_ATTACHED</string>
  <key>system-entities</key><array>
    <dict><key>dev-entry</key><string>${HOLDER_DEVICE-/dev/disk7}</string></dict>
    <dict><key>dev-entry</key><string>/dev/disk7s1</string></dict>
    <dict><key>dev-entry</key><string>/dev/disk8</string></dict>
  </array>
</dict>
PLIST
          done
        fi
        echo '</array></dict></plist>'
        ;;
      detach\ *)
        echo "$*" >> "$DETACHED"
        return "${HOLDER_DETACH_STATUS:-0}"
        ;;
      *) echo "hdiutil $*" ;;
    esac
  }
  # shellcheck disable=SC2329
  function kill() { echo "$*" >> "$KILLED"; }
}

function unstub_image_holder() {
  unset -f lsof ps pgrep hdiutil kill sleep
  unset HOLDER_LINGERS HOLDER_ATTACHED HOLDER_DEVICE HOLDER_MATCHES HOLDER_INFO HOLDER_DETACH_STATUS GITHUB_ACTIONS IMAGE
}

function run_release_image() {
  local tmp="$1"
  shift
  stub_image_holder "$@" "$tmp/killed"
  stub_sleep "$tmp/slept"
  status=0
  output="$(release_image "${IMAGE:-image.dmg}" 2>&1)" || status=$?
  killed="$(tr '\n' '|' < "$tmp/killed")"
  detached="$(cat "$tmp/killed.detached")"
  slept="$(wc -l < "$tmp/slept" | tr -d ' ')"
  unstub_image_holder
}

function test_release_image_asks_the_helper_once_to_stop_on_ci() {
  local tmp output killed slept status
  tmp="$(mktemp -d)"
  export GITHUB_ACTIONS=true
  run_release_image "$tmp" 4242 "/System/Library/PrivateFrameworks/DiskImages.framework/Resources/diskimages-helper -uuid fixture -post-exec 4"
  rm -rf "$tmp"

  assert_contains "hdiutil create left image.dmg open" "$output"
  assert_contains "lsof -- image.dmg" "$output"
  assert_contains "hdiutil info" "$output"
  assert_contains "stopping the disk image helper: 4242" "$output"
  assert_same "0" "$status"
  assert_same "4242|" "$killed"
  assert_same "0" "$slept"
  assert_not_contains "still open" "$output"
}

function test_release_image_waits_up_to_ten_seconds_without_a_second_signal() {
  local tmp output killed slept status
  tmp="$(mktemp -d)"
  export GITHUB_ACTIONS=true
  HOLDER_LINGERS=20
  run_release_image "$tmp" 4242 /System/Library/PrivateFrameworks/DiskImages.framework/Resources/diskimages-helper
  rm -rf "$tmp"

  assert_same "0" "$status"
  assert_same "4242|" "$killed"
  assert_same "10" "$slept"
  assert_contains "image.dmg is still open after 10s" "$output"
}

function test_release_image_detaches_only_the_staged_whole_disk_on_ci() {
  local tmp output killed detached slept status
  tmp="$(mktemp -d)"
  export GITHUB_ACTIONS=true
  HOLDER_ATTACHED=image.dmg
  run_release_image "$tmp" 4242 /System/Library/PrivateFrameworks/DiskImages.framework/Resources/diskimages-helper
  rm -rf "$tmp"

  assert_same "0" "$status"
  assert_same "detach /dev/disk7" "$detached"
  assert_empty "$killed"
}

function test_release_image_matches_an_attached_image_under_private() {
  local tmp output killed detached slept status
  tmp="$(mktemp -d)"
  export GITHUB_ACTIONS=true
  HOLDER_ATTACHED=/private/var/x/image.dmg
  IMAGE=/var/x/image.dmg
  run_release_image "$tmp" 4242 /System/Library/PrivateFrameworks/DiskImages.framework/Resources/diskimages-helper
  rm -rf "$tmp"

  assert_same "0" "$status"
  assert_same "detach /dev/disk7" "$detached"
  assert_empty "$killed"
}

function test_release_image_matches_an_attached_image_under_private_tmp() {
  local tmp output killed detached slept status
  tmp="$(mktemp -d)"
  export GITHUB_ACTIONS=true
  HOLDER_ATTACHED=/private/tmp/x/image.dmg
  IMAGE=/tmp/x/image.dmg
  run_release_image "$tmp" 4242 /System/Library/PrivateFrameworks/DiskImages.framework/Resources/diskimages-helper
  rm -rf "$tmp"

  assert_same "0" "$status"
  assert_same "detach /dev/disk7" "$detached"
  assert_empty "$killed"
}

function test_release_image_only_reports_the_holder_off_ci() {
  local tmp output killed detached slept status
  tmp="$(mktemp -d)"
  export GITHUB_ACTIONS=false
  HOLDER_ATTACHED=image.dmg
  run_release_image "$tmp" 4242 /System/Library/PrivateFrameworks/DiskImages.framework/Resources/diskimages-helper
  rm -rf "$tmp"

  assert_contains "lsof -- image.dmg" "$output"
  assert_not_contains "stopping" "$output"
  assert_empty "$detached"
  assert_empty "$killed"
}

function test_release_image_leaves_the_helper_alone_if_detach_fails() {
  local tmp output killed detached slept status
  tmp="$(mktemp -d)"
  export GITHUB_ACTIONS=true
  HOLDER_ATTACHED=image.dmg
  HOLDER_DETACH_STATUS=16
  run_release_image "$tmp" 4242 /System/Library/PrivateFrameworks/DiskImages.framework/Resources/diskimages-helper
  rm -rf "$tmp"

  assert_contains "could not detach" "$output"
  assert_same "detach /dev/disk7" "$detached"
  assert_empty "$killed"
}

function test_release_image_leaves_uncertain_attachments_alone() {
  local tmp output killed detached slept status uncertainty
  for uncertainty in malformed unavailable duplicate missing partition; do
    tmp="$(mktemp -d)"
    export GITHUB_ACTIONS=true
    HOLDER_ATTACHED=image.dmg
    case "$uncertainty" in
      malformed|unavailable) HOLDER_INFO="$uncertainty" ;;
      duplicate) HOLDER_MATCHES=2 ;;
      missing) HOLDER_DEVICE= ;;
      partition) HOLDER_DEVICE=/dev/disk7s1 ;;
    esac
    run_release_image "$tmp" 4242 /System/Library/PrivateFrameworks/DiskImages.framework/Resources/diskimages-helper
    rm -rf "$tmp"

    assert_empty "$detached"
    assert_empty "$killed"
  done
}

function test_release_image_leaves_other_holders_alone() {
  local tmp output killed slept status
  tmp="$(mktemp -d)"
  export GITHUB_ACTIONS=true
  run_release_image "$tmp" 99 /usr/libexec/mds
  rm -rf "$tmp"

  assert_contains "hdiutil create left image.dmg open" "$output"
  assert_not_contains "stopping" "$output"
  assert_empty "$killed"
}

function test_release_image_does_nothing_when_the_image_is_closed() {
  local tmp output killed slept status
  tmp="$(mktemp -d)"
  export GITHUB_ACTIONS=true
  run_release_image "$tmp" "" ""
  rm -rf "$tmp"

  assert_same "0" "$status"
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
