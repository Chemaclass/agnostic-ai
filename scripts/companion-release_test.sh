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

# Records each codesign and security call, one line each, fields split by |.
function stub_signing_tools() {
  SIGN_CALLS="$1"
  : > "$SIGN_CALLS"
  # shellcheck disable=SC2329
  function codesign() { printf 'codesign|%s\n' "$(IFS='|'; echo "$*")" >> "$SIGN_CALLS"; }
  # shellcheck disable=SC2329
  function security() { printf 'security|%s\n' "$(IFS='|'; echo "$*")" >> "$SIGN_CALLS"; }
}

function test_it_signs_ad_hoc_without_a_certificate() {
  local calls
  calls="$(mktemp)"
  stub_signing_tools "$calls"

  sign_companion "my.app" "" "" app

  assert_same 'codesign|--force|--sign|-|my.app' "$(cat "$calls")"
  unset -f codesign security
  rm -f "$calls"
}

function test_it_pins_the_app_to_the_certificate() {
  local calls
  calls="$(mktemp)"
  stub_signing_tools "$calls"

  sign_companion "my.app" "ABC123" "/k.keychain-db" app

  assert_same 'security|unlock-keychain|-p||/k.keychain-db
codesign|--force|--keychain|/k.keychain-db|--sign|ABC123|--identifier|dev.agnostic-ai.companion|-r=designated => identifier "dev.agnostic-ai.companion" and certificate leaf = H"ABC123"|my.app' "$(cat "$calls")"
  unset -f codesign security
  rm -f "$calls"
}

function test_it_signs_the_bundled_cli_with_the_certificate_and_no_requirement() {
  local calls
  calls="$(mktemp)"
  stub_signing_tools "$calls"

  sign_companion "agnostic-ai" "ABC123" "/k.keychain-db"

  assert_same 'security|unlock-keychain|-p||/k.keychain-db
codesign|--force|--keychain|/k.keychain-db|--sign|ABC123|agnostic-ai' "$(cat "$calls")"
  unset -f codesign security
  rm -f "$calls"
}

# Makes a self-signed code-signing .p12 named $2 in folder $1, with the
# release password, and prints its SHA-1 hash.
function make_test_p12() {
  local dir="$1" name="$2"
  printf '[req]\ndistinguished_name=dn\nx509_extensions=ext\nprompt=no\n[dn]\nCN=%s\n[ext]\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=critical,codeSigning\n' "$name" > "$dir/cert.cnf"
  /usr/bin/openssl req -x509 -newkey rsa:2048 -nodes -keyout "$dir/key.pem" -out "$dir/cert.pem" -days 2 -config "$dir/cert.cnf" 2>/dev/null
  /usr/bin/openssl pkcs12 -export -inkey "$dir/key.pem" -in "$dir/cert.pem" -out "$dir/id.p12" -passout pass:agnostic-ai
  /usr/bin/openssl x509 -in "$dir/cert.pem" -outform der | shasum -a 1 | cut -d' ' -f1 | tr '[:lower:]' '[:upper:]'
}

function test_it_imports_the_release_certificate_and_prints_its_hash() {
  if [ "$(uname)" != Darwin ]; then
    bashunit::skip "security runs on macOS only"
    return
  fi
  local tmp want got
  tmp="$(mktemp -d)"
  want="$(make_test_p12 "$tmp" "agnostic-ai companion release")"

  got="$(COMPANION_SIGNING_P12="$(base64 -i "$tmp/id.p12")" import_companion_signer "$tmp/k.keychain-db" 2>/dev/null)"

  security delete-keychain "$tmp/k.keychain-db" 2>/dev/null
  rm -rf "$tmp"
  assert_same "$want" "$got"
}

function test_it_prints_no_hash_for_a_certificate_under_another_name() {
  if [ "$(uname)" != Darwin ]; then
    bashunit::skip "security runs on macOS only"
    return
  fi
  local tmp got
  tmp="$(mktemp -d)"
  make_test_p12 "$tmp" "someone else" > /dev/null

  got="$(COMPANION_SIGNING_P12="$(base64 -i "$tmp/id.p12")" import_companion_signer "$tmp/k.keychain-db" 2>/dev/null)"

  security delete-keychain "$tmp/k.keychain-db" 2>/dev/null
  rm -rf "$tmp"
  assert_empty "$got"
}
