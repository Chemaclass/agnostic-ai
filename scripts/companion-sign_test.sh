#!/usr/bin/env bash
#
# bashunit tests for scripts/companion-sign.sh
#
# Run:
#   bashunit scripts/companion-sign_test.sh
#
# The macOS tests sign with a throwaway certificate in a throwaway keychain.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/companion-sign.sh"

function test_it_reads_the_keychain_search_list_without_quotes() {
  # shellcheck disable=SC2329
  function security() {
    printf '    "/Users/me/Library/Keychains/login.keychain-db"\n    "/Users/me/my [1] keychain.keychain-db"\n'
  }

  assert_same '/Users/me/Library/Keychains/login.keychain-db
/Users/me/my [1] keychain.keychain-db' "$(user_keychains)"
  unset -f security
}

function test_it_signs_the_cli_then_the_app_pinned_to_the_certificate() {
  local calls
  calls="$(mktemp)"
  # shellcheck disable=SC2329
  function codesign() { printf 'codesign|%s\n' "$(IFS='|'; echo "$*")" >> "$calls"; }
  # shellcheck disable=SC2329
  function security() { printf 'security|%s\n' "$(IFS='|'; echo "$*")" >> "$calls"; }
  # shellcheck disable=SC2034 # read by sign_companion_app
  local companion_release_cert=ABC123

  sign_companion_app "my.app" "/k.keychain-db"

  assert_same 'security|unlock-keychain|-p||/k.keychain-db
codesign|--force|--keychain|/k.keychain-db|--sign|ABC123|my.app/Contents/Resources/agnostic-ai
codesign|--force|--keychain|/k.keychain-db|--sign|ABC123|--identifier|dev.agnostic-ai.companion|-r=designated => identifier "dev.agnostic-ai.companion" and certificate leaf = H"ABC123"|my.app
codesign|--verify|--deep|--strict|my.app
codesign|--verify|-R=identifier "dev.agnostic-ai.companion" and certificate leaf = H"ABC123"|my.app' "$(cat "$calls")"
  unset -f codesign security
  rm -f "$calls"
}

function test_it_checks_the_written_image_against_the_pinned_requirement() {
  local calls
  calls="$(mktemp)"
  # shellcheck disable=SC2329
  function hdiutil_retrying() { printf 'hdiutil|%s\n' "$(IFS='|'; echo "$*")" >> "$calls"; }
  # shellcheck disable=SC2329
  function codesign() { printf 'codesign|%s\n' "$(IFS='|'; echo "$*")" >> "$calls"; }
  # shellcheck disable=SC2034 # read by verify_companion_image
  local companion_release_cert=ABC123 mount
  mount="$(mktemp -d)/mount"

  verify_companion_image "a.dmg" "$mount"

  assert_same "hdiutil|attach|-nobrowse|-readonly|-mountpoint|$mount|a.dmg
codesign|--verify|--deep|--strict|-R=identifier \"dev.agnostic-ai.companion\" and certificate leaf = H\"ABC123\"|$mount/agnostic-ai companion.app
hdiutil|detach|$mount" "$(cat "$calls")"
  unset -f hdiutil_retrying codesign
  rm -f "$calls"
}

function test_it_stops_on_an_image_signed_by_another_certificate() {
  # shellcheck disable=SC2329
  function hdiutil_retrying() { :; }
  # shellcheck disable=SC2329
  function codesign() { return 3; }
  local status=0 mount
  mount="$(mktemp -d)/mount"

  verify_companion_image "a.dmg" "$mount" 2> /dev/null || status=$?

  assert_same 3 "$status"
  unset -f hdiutil_retrying codesign
}

# make_test_p12 <dir> <name>: writes <dir>/id.p12, a self-signed code-signing
# certificate with the release password, and prints its SHA-1 hash.
function make_test_p12() {
  local dir="$1" name="$2"
  printf '[req]\ndistinguished_name=dn\nx509_extensions=ext\nprompt=no\n[dn]\nCN=%s\n[ext]\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=critical,codeSigning\n' "$name" > "$dir/cert.cnf"
  /usr/bin/openssl req -x509 -newkey rsa:2048 -nodes -keyout "$dir/key.pem" -out "$dir/cert.pem" -days 2 -config "$dir/cert.cnf" 2> /dev/null
  /usr/bin/openssl pkcs12 -export -inkey "$dir/key.pem" -in "$dir/cert.pem" -out "$dir/id.p12" -passout pass:agnostic-ai
  /usr/bin/openssl x509 -in "$dir/cert.pem" -outform der | shasum -a 1 | cut -d' ' -f1 | tr '[:lower:]' '[:upper:]'
}

function test_it_refuses_a_certificate_other_than_the_pinned_one() {
  if [ "$(uname)" != Darwin ]; then
    bashunit::skip "security runs on macOS only"
    return
  fi
  local tmp status=0
  tmp="$(mktemp -d)"
  make_test_p12 "$tmp" "agnostic-ai companion release staging" > /dev/null

  COMPANION_SIGNING_P12="$(base64 -i "$tmp/id.p12")" companion_release_cert=0E01A50348AA6D59F2CCC808B8CA966BDCD25585 \
    import_companion_signer "$tmp/k.keychain-db" > /dev/null 2>&1 || status=$?

  security delete-keychain "$tmp/k.keychain-db" 2> /dev/null
  rm -rf "$tmp"
  assert_not_same 0 "$status"
}

function test_it_signs_each_disk_image_and_leaves_the_search_list_alone() {
  if [ "$(uname)" != Darwin ]; then
    bashunit::skip "codesign and hdiutil run on macOS only"
    return
  fi
  local tmp hash before after requirement
  tmp="$(mktemp -d)"
  hash="$(make_test_p12 "$tmp" "agnostic-ai companion release")"
  local app="$tmp/image/agnostic-ai companion.app"
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$tmp/dist"
  cp /usr/bin/true "$app/Contents/MacOS/agnostic-ai-companion"
  cp /usr/bin/true "$app/Contents/Resources/agnostic-ai"
  printf '<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>dev.agnostic-ai.companion</string><key>CFBundleExecutable</key><string>agnostic-ai-companion</string></dict></plist>' > "$app/Contents/Info.plist"
  codesign --force --sign - "$app" 2> /dev/null
  ln -s /Applications "$tmp/image/Applications"
  hdiutil create -quiet -format UDZO -volname "agnostic-ai companion" -srcfolder "$tmp/image" "$tmp/dist/agnostic-ai-companion_darwin_arm64.dmg"
  before="$(security list-keychains -d user)"

  COMPANION_SIGNING_P12="$(base64 -i "$tmp/id.p12")" COMPANION_SIGNING_SHA1="$hash" \
    bash "$SCRIPT_DIR/companion-sign.sh" "$tmp/dist" > "$tmp/out" 2>&1

  after="$(security list-keychains -d user)"
  hdiutil attach -quiet -nobrowse -readonly -mountpoint "$tmp/mount" "$tmp/dist/agnostic-ai-companion_darwin_arm64.dmg"
  requirement="$(codesign -d -r- "$tmp/mount/agnostic-ai companion.app" 2>&1 | tail -1)"
  hdiutil detach -quiet "$tmp/mount"
  rm -rf "$tmp"
  assert_same "$before" "$after"
  assert_same "designated => identifier \"dev.agnostic-ai.companion\" and certificate leaf = H\"$(printf '%s' "$hash" | tr '[:upper:]' '[:lower:]')\"" "$requirement"
}
