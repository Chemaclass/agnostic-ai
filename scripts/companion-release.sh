#!/usr/bin/env bash

# Usage: scripts/companion-release.sh <version> <outdir>
#
# Builds macOS disk images with the CLI inside the app, plus the bare
# companion archives that `agnostic-ai companion install` downloads.
# Requires macOS with the Xcode Command Line Tools.

# A busy macOS runner sometimes leaves a new disk image unreadable for a
# moment, so hdiutil says "Resource temporarily unavailable". Only that
# error is retried; any other fails at once.
hdiutil_retrying() {
  local attempts=4 delay=5 attempt=1 output status
  while :; do
    status=0
    output="$(hdiutil "$@" 2>&1)" || status=$?
    printf '%s\n' "$output"
    [ "$status" -eq 0 ] && return 0
    if [[ "$output" != *"Resource temporarily unavailable"* ]] || [ "$attempt" -ge "$attempts" ]; then
      return "$status"
    fi
    echo "hdiutil $1: resource busy, attempt $attempt of $attempts; retrying in ${delay}s" >&2
    sleep "$delay"
    attempt=$((attempt + 1))
    delay=$((delay * 2))
  done
}

# Release builds sign with the project certificate, given base64 as a
# .p12 in COMPANION_SIGNING_P12. macOS keeps a privacy grant, such as
# Accessibility, against the app's designated requirement, so pinning it
# to that certificate carries the grant from one release to the next.
# Without the certificate the app is signed ad hoc. macOS cannot import a
# .p12 without a password, so it has a fixed one; the secret guards the key.
companion_id=dev.agnostic-ai.companion
companion_signer_cn="agnostic-ai companion release"

# Prints the SHA-1 hash of the certificate it imports into keychain $1.
import_companion_signer() {
  local keychain="$1"
  security create-keychain -p "" "$keychain" >&2
  security set-keychain-settings "$keychain" >&2
  printf '%s' "$COMPANION_SIGNING_P12" | base64 --decode > "$keychain.p12"
  security unlock-keychain -p "" "$keychain" >&2
  security import "$keychain.p12" -k "$keychain" -P agnostic-ai -T /usr/bin/codesign >&2
  rm -f "$keychain.p12"
  security find-certificate -c "$companion_signer_cn" -Z "$keychain" | awk '/^SHA-1 hash:/ { print $3 }' || true
}

# sign_companion <path> <hash> <keychain> [app]: signs ad hoc when hash is
# empty. With "app" the signature names the bundle and pins the certificate.
sign_companion() {
  local path="$1" hash="$2" keychain="$3"
  if [ -z "$hash" ]; then
    codesign --force --sign - "$path"
    return
  fi
  security unlock-keychain -p "" "$keychain"
  if [ "${4:-}" = app ]; then
    codesign --force --keychain "$keychain" --sign "$hash" --identifier "$companion_id" \
      "-r=designated => identifier \"$companion_id\" and certificate leaf = H\"$hash\"" "$path"
  else
    codesign --force --keychain "$keychain" --sign "$hash" "$path"
  fi
}

# Sourced by scripts/companion-release_test.sh for the functions above.
if [[ "${BASH_SOURCE[0]}" != "$0" ]]; then
  return 0
fi

set -Eeuo pipefail

version="${1:?usage: $0 <version> <outdir>}"
outdir="${2:?usage: $0 <version> <outdir>}"
root="$(cd "$(dirname "$0")/.." && pwd)"
name=agnostic-ai-companion
app_name="agnostic-ai companion"
if [[ ! "$version" =~ ^[0-9][A-Za-z0-9.+-]*$ ]]; then
  echo "invalid release version: $version" >&2
  exit 1
fi

# Without this the C parts target the build machine's own macOS, and the
# program refuses any older one. 13 is the oldest the Go toolchain in
# companion/go.mod supports. Go keys its build cache on CGO_CFLAGS, not on
# MACOSX_DEPLOYMENT_TARGET.
min_macos=13.0
export MACOSX_DEPLOYMENT_TARGET="$min_macos"
export CGO_CFLAGS="-O2 -g -mmacosx-version-min=$min_macos"
export CGO_LDFLAGS="-O2 -g -mmacosx-version-min=$min_macos"

mkdir -p "$outdir"
outdir="$(cd "$outdir" && pwd)"
work="$(mktemp -d)"
signing_keychain="$work/signing.keychain-db"
signing_hash=""
cleanup() {
  case "$work" in "${TMPDIR:-/tmp/}"*|/tmp/*|/private/tmp/*) ;; *) exit 1 ;; esac
  if [ -n "$signing_hash" ]; then
    security delete-keychain "$signing_keychain" || true
  fi
  echo "Removing $work"
  rm -rf "$work"
}
trap cleanup EXIT
mkdir -p "$work/companion.iconset"
cp "$root/internal/cli/companion-icon.png" "$work/companion.iconset/icon_512x512.png"
iconutil -c icns "$work/companion.iconset" -o "$work/companion.icns"

if [ -n "${COMPANION_SIGNING_P12:-}" ]; then
  signing_hash="$(import_companion_signer "$signing_keychain")"
  if [ -z "$signing_hash" ]; then
    echo "COMPANION_SIGNING_P12 holds no certificate named $companion_signer_cn" >&2
    exit 1
  fi
  echo "Signing with the certificate $signing_hash, valid until $(security find-certificate -c "$companion_signer_cn" -p "$signing_keychain" | openssl x509 -noout -enddate | cut -d= -f2)"
elif [ "${COMPANION_SIGNING_REQUIRED:-}" = 1 ]; then
  echo "COMPANION_SIGNING_P12 is empty; a release must sign with the project certificate" >&2
  exit 1
fi

for arch in arm64 amd64; do
  mkdir -p "$work/$arch"
  (
    cd "$root/companion"
    CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" go build \
      -tags "production private_mac_apis" -trimpath \
      -ldflags "-s -w -X main.version=$version" \
      -o "$work/$arch/$name" .
  )
  want="$arch"
  [ "$arch" = amd64 ] && want=x86_64
  got="$(lipo -archs "$work/$arch/$name")"
  if [ "$got" != "$want" ]; then
    echo "$name for $arch is built for $got" >&2
    exit 1
  fi
  minos="$(otool -l "$work/$arch/$name" | awk '$1 == "minos" { print $2; exit }')"
  if [ "$minos" != "$min_macos" ]; then
    echo "$name for $arch needs macOS ${minos:-<unknown>}, not $min_macos" >&2
    exit 1
  fi
  if [ "$(uname -m)" = "$arch" ] || { [ "$arch" = amd64 ] && [ "$(uname -m)" = x86_64 ]; }; then
    said="$("$work/$arch/$name" --version)"
    if [ "$said" != "$version" ]; then
      echo "$name for $arch says it is $said, not $version" >&2
      exit 1
    fi
  fi
  # Without this, macOS tar adds ._ files that hold extended attributes.
  COPYFILE_DISABLE=1 tar -czf "$outdir/${name}_darwin_${arch}.tar.gz" -C "$work/$arch" "$name"
  echo "$outdir/${name}_darwin_${arch}.tar.gz"

  app="$work/$arch/image/$app_name.app"
  contents="$app/Contents"
  mkdir -p "$contents/MacOS" "$contents/Resources"
  cp "$work/$arch/$name" "$contents/MacOS/$name"
  cp "$work/companion.icns" "$contents/Resources/companion.icns"
  (
    cd "$root"
    CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -trimpath \
      -ldflags "-s -w -X main.version=$version" \
      -o "$contents/Resources/agnostic-ai" ./cmd/agnostic-ai
  )
  got="$(lipo -archs "$contents/Resources/agnostic-ai")"
  if [ "$got" != "$want" ]; then
    echo "bundled agnostic-ai for $arch is built for $got" >&2
    exit 1
  fi
  if [ "$(uname -m)" = "$arch" ] || { [ "$arch" = amd64 ] && [ "$(uname -m)" = x86_64 ]; }; then
    said="$("$contents/Resources/agnostic-ai" --version)"
    if [ "$said" != "agnostic-ai version $version" ]; then
      echo "bundled agnostic-ai for $arch says $said, expected version $version" >&2
      exit 1
    fi
  fi
  cat > "$contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleIdentifier</key><string>dev.agnostic-ai.companion</string>
  <key>CFBundleName</key><string>$app_name</string>
  <key>CFBundleDisplayName</key><string>$app_name</string>
  <key>CFBundleExecutable</key><string>$name</string>
  <key>CFBundleIconFile</key><string>companion</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$version</string>
  <key>CFBundleVersion</key><string>$version</string>
  <key>LSUIElement</key><true/>
  <key>LSMinimumSystemVersion</key><string>$min_macos</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSAppleEventsUsageDescription</key><string>Clicking a session switches to the terminal tab that runs it.</string>
</dict>
</plist>
PLIST
  plutil -lint "$contents/Info.plist"
  sign_companion "$contents/Resources/agnostic-ai" "$signing_hash" "$signing_keychain"
  sign_companion "$app" "$signing_hash" "$signing_keychain" app
  codesign --verify --deep --strict "$app"
  if [ -n "$signing_hash" ]; then
    codesign --verify -R="identifier \"$companion_id\" and certificate leaf = H\"$signing_hash\"" "$app"
  fi
  ln -s /Applications "$work/$arch/image/Applications"
  image="$outdir/${name}_darwin_${arch}.dmg"
  hdiutil_retrying create -ov -format UDZO -volname "$app_name" \
    -srcfolder "$work/$arch/image" "$image"
  hdiutil_retrying verify "$image"
  echo "$image"
done
