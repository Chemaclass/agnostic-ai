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
  local tmp calls
  tmp="$(mktemp -d)"
  calls="$tmp/calls"
  # shellcheck disable=SC2329
  function hdiutil_retrying() { printf 'hdiutil|%s\n' "$(IFS='|'; echo "$*")" >> "$calls"; }
  # shellcheck disable=SC2329
  function codesign() { printf 'codesign|%s\n' "$(IFS='|'; echo "$*")" >> "$calls"; }
  # shellcheck disable=SC2034 # read by verify_companion_image
  local companion_release_cert=ABC123 mount="$tmp/mount"

  (
    # shellcheck disable=SC2329
    function check_companion_bundle() { printf 'layout|%s|%s\n' "$1" "$2" >> "$calls"; }
    verify_companion_image "a.dmg" "$mount" agnostic-companion "Agnostic Companion"
  )

  assert_same "hdiutil|attach|-nobrowse|-readonly|-mountpoint|$mount|a.dmg
layout|$mount/Agnostic Companion.app|agnostic-companion
codesign|--verify|--deep|--strict|-R=identifier \"dev.agnostic-ai.companion\" and certificate leaf = H\"ABC123\"|$mount/Agnostic Companion.app
hdiutil|detach|$mount" "$(cat "$calls")"
  unset -f hdiutil_retrying codesign
  rm -rf "$tmp"
}

function test_it_stops_on_an_image_signed_by_another_certificate() {
  local status=0 tmp
  tmp="$(mktemp -d)"

  (
    # shellcheck disable=SC2329
    function hdiutil_retrying() { :; }
    # shellcheck disable=SC2329
    function check_companion_bundle() { :; }
    # shellcheck disable=SC2329
    function codesign() { return 3; }
    verify_companion_image "a.dmg" "$tmp/mount" agnostic-companion "Agnostic Companion"
  ) 2> /dev/null || status=$?

  assert_same 3 "$status"
  rm -rf "$tmp"
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

function make_unsigned_image() {
  local tmp="$1" arch="$2" name="$3" app_name="$4"
  local dir="$tmp/$arch-$name-image"
  local app="$dir/$app_name.app"
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$tmp/dist"
  cp /usr/bin/true "$app/Contents/MacOS/$name"
  cp /usr/bin/true "$app/Contents/Resources/agnostic-ai"
  printf '<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>dev.agnostic-ai.companion</string><key>CFBundleExecutable</key><string>%s</string><key>CFBundleName</key><string>%s</string><key>CFBundleDisplayName</key><string>%s</string></dict></plist>' \
    "$name" "Agnostic Companion" "Agnostic Companion" > "$app/Contents/Info.plist"
  codesign --force --sign - "$app" 2> /dev/null
  ln -s /Applications "$dir/Applications"
  hdiutil create -quiet -format UDZO -volname "$app_name" -srcfolder "$dir" "$tmp/dist/${name}_darwin_${arch}.dmg"
}

function test_it_signs_both_layouts_and_leaves_the_search_list_alone() {
  if [ "$(uname)" != Darwin ]; then
    bashunit::skip "codesign and hdiutil run on macOS only"
    return
  fi
  local tmp hash before after requirement arch app_name name app info
  tmp="$(mktemp -d)"
  hash="$(make_test_p12 "$tmp" "agnostic-ai companion release")"
  for arch in arm64 amd64; do
    make_unsigned_image "$tmp" "$arch" agnostic-companion "Agnostic Companion"
    make_unsigned_image "$tmp" "$arch" agnostic-ai-companion "agnostic-ai companion"
  done
  before="$(security list-keychains -d user)"

  assert_successful_code "$(COMPANION_SIGNING_P12="$(base64 -i "$tmp/id.p12")" COMPANION_SIGNING_SHA1="$hash" \
    bash "$SCRIPT_DIR/companion-sign.sh" "$tmp/dist" 2>&1)"

  after="$(security list-keychains -d user)"
  assert_same "$before" "$after"
  for arch in arm64 amd64; do
    for name in agnostic-companion agnostic-ai-companion; do
      app_name="Agnostic Companion"
      if [ "$name" = agnostic-ai-companion ]; then
        app_name="agnostic-ai companion"
      fi
      hdiutil attach -quiet -nobrowse -readonly -mountpoint "$tmp/mount" "$tmp/dist/${name}_darwin_${arch}.dmg"
      app="$tmp/mount/$app_name.app"
      assert_same "$app" "$(printf '%s\n' "$tmp/mount/"*.app)"
      info="$app/Contents/Info.plist"
      assert_same "dev.agnostic-ai.companion" "$(plutil -extract CFBundleIdentifier raw "$info")"
      assert_same "$name" "$(plutil -extract CFBundleExecutable raw "$info")"
      assert_same "Agnostic Companion" "$(plutil -extract CFBundleName raw "$info")"
      assert_same "Agnostic Companion" "$(plutil -extract CFBundleDisplayName raw "$info")"
      assert_file_exists "$app/Contents/Resources/agnostic-ai"
      requirement="$(codesign -d -r- "$app" 2>&1 | tail -1)"
      assert_same "designated => identifier \"dev.agnostic-ai.companion\" and certificate leaf = H\"$(printf '%s' "$hash" | tr '[:upper:]' '[:lower:]')\"" "$requirement"
      hdiutil detach -quiet "$tmp/mount"
    done
  done
  rm -rf "$tmp"
}
