#!/usr/bin/env bash

# Usage: scripts/companion-sign.sh <dist dir>
#
# Signs the companion app in each disk image in <dist dir> with the project
# certificate and writes the disk image again. COMPANION_SIGNING_P12 holds
# the certificate and key, base64, as a .p12 with the password agnostic-ai.
# The release runs this in a job of its own, apart from the build, so no
# dependency code runs beside the key. See docs/internal/companion-signing.md.

# The SHA-1 hash of the project certificate. Each released app pins it, so
# changing it makes every user grant Accessibility again.
companion_release_cert="${COMPANION_SIGNING_SHA1:-0E01A50348AA6D59F2CCC808B8CA966BDCD25585}"
companion_id=dev.agnostic-ai.companion
companion_app_name="agnostic-ai companion"

# import_companion_signer <keychain>: imports COMPANION_SIGNING_P12 into a
# new keychain and fails unless it holds the expected certificate.
import_companion_signer() {
  local keychain="$1"
  security create-keychain -p "" "$keychain"
  security set-keychain-settings "$keychain"
  security unlock-keychain -p "" "$keychain"
  printf '%s' "$COMPANION_SIGNING_P12" | base64 --decode > "$keychain.p12"
  security import "$keychain.p12" -k "$keychain" -P agnostic-ai -T /usr/bin/codesign
  rm -f "$keychain.p12"
  # Lets codesign use the key without a dialog, which CI cannot answer.
  security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "" "$keychain" > /dev/null
  if ! security find-certificate -a -Z "$keychain" | grep -qx "SHA-1 hash: $companion_release_cert"; then
    echo "COMPANION_SIGNING_P12 does not hold the certificate $companion_release_cert" >&2
    return 1
  fi
}

# user_keychains prints the user's keychain search list, one path a line.
user_keychains() {
  security list-keychains -d user | sed -e 's/^[[:space:]]*"//' -e 's/"$//'
}

# sign_companion_app <app> <keychain>: signs the bundled CLI, then the app,
# pinning the certificate in the app's designated requirement.
sign_companion_app() {
  local app="$1" keychain="$2" requirement
  requirement="identifier \"$companion_id\" and certificate leaf = H\"$companion_release_cert\""
  security unlock-keychain -p "" "$keychain"
  codesign --force --keychain "$keychain" --sign "$companion_release_cert" "$app/Contents/Resources/agnostic-ai"
  codesign --force --keychain "$keychain" --sign "$companion_release_cert" --identifier "$companion_id" \
    "-r=designated => $requirement" "$app"
  codesign --verify --deep --strict "$app"
  codesign --verify "-R=$requirement" "$app"
}

# verify_companion_image <image> <mount>: fails unless the app in the written
# disk image meets the pinned requirement, the check each update runs too.
# An image signed ad hoc or with another certificate then stops the release.
verify_companion_image() {
  local image="$1" mount="$2" requirement status=0
  requirement="identifier \"$companion_id\" and certificate leaf = H\"$companion_release_cert\""
  mkdir -p "$mount"
  hdiutil_retrying attach -nobrowse -readonly -mountpoint "$mount" "$image"
  codesign --verify --deep --strict "-R=$requirement" "$mount/$companion_app_name.app" || status=$?
  hdiutil_retrying detach "$mount"
  rmdir "$mount"
  if [ "$status" -ne 0 ]; then
    echo "$image: the app does not meet $requirement" >&2
  fi
  return "$status"
}

# Sourced by scripts/companion-sign_test.sh for the functions above.
if [[ "${BASH_SOURCE[0]}" != "$0" ]]; then
  return 0
fi

set -Eeuo pipefail

dist="${1:?usage: $0 <dist dir>}"
if [ -z "${COMPANION_SIGNING_P12:-}" ]; then
  echo "COMPANION_SIGNING_P12 is empty" >&2
  exit 1
fi
root="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/companion-release.sh
source "$root/scripts/companion-release.sh"

work="$(mktemp -d)"
keychain="$work/signing.keychain-db"
cleanup() {
  security delete-keychain "$keychain" 2> /dev/null || true
  if [ -d "$work/mount" ]; then
    hdiutil detach "$work/mount" -force > /dev/null 2>&1 || true
  fi
  case "$work" in "${TMPDIR:-/tmp/}"*|/tmp/*|/private/tmp/*) rm -rf "$work" ;; esac
}
trap cleanup EXIT

searched_paths="$(user_keychains)"
searched=()
while IFS= read -r line; do
  if [ -n "$line" ]; then
    searched+=("$line")
  fi
done <<< "$searched_paths"
import_companion_signer "$keychain"
# Some macOS versions find a signing identity only in a keychain on the
# search list, whatever codesign --keychain says.
security list-keychains -d user -s ${searched[@]+"${searched[@]}"} "$keychain"
echo "Signing with the certificate $companion_release_cert, valid until $(security find-certificate -a -p "$keychain" | openssl x509 -noout -enddate | cut -d= -f2)"

shopt -s nullglob
images=("$dist"/agnostic-ai-companion_darwin_*.dmg)
if [ "${#images[@]}" -eq 0 ]; then
  echo "no companion disk images in $dist" >&2
  exit 1
fi
for image in "${images[@]}"; do
  rm -rf "$work/image" "$work/mount"
  mkdir -p "$work/image" "$work/mount"
  hdiutil_retrying attach -nobrowse -readonly -mountpoint "$work/mount" "$image"
  ditto "$work/mount/$companion_app_name.app" "$work/image/$companion_app_name.app"
  hdiutil_retrying detach "$work/mount"
  rmdir "$work/mount"
  sign_companion_app "$work/image/$companion_app_name.app" "$keychain"
  ln -s /Applications "$work/image/Applications"
  hdiutil_retrying create -ov -format UDZO -volname "$companion_app_name" -srcfolder "$work/image" "$work/staged.dmg"
  release_image "$work/staged.dmg"
  if ! hdiutil_retrying verify "$work/staged.dmg"; then
    image_diagnostics "$work/staged.dmg"
    exit 1
  fi
  mv -f "$work/staged.dmg" "$image"
  verify_companion_image "$image" "$work/mount"
  echo "$image"
done
