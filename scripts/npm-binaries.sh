#!/usr/bin/env bash
#
# npm-binaries.sh - collect the release binaries the npm platform packages ship.
#
# Usage:
#   scripts/npm-binaries.sh <tag> <destdir>
#
# Downloads the six release archives and checksums.txt, verifies every archive
# against the published digest, and unpacks each binary into
# <destdir>/<goos>_<goarch>/. That is the layout
# npm/scripts/build-platform-packages.js reads.
#
# This runs in the release job, not on a user machine. It is the one place the
# npm distribution still touches the network, and a failure here fails the
# release loudly instead of leaving somebody with a broken install.

set -euo pipefail

REPO="${AGNOSTIC_AI_REPO:-Chemaclass/agnostic-ai}"
BINARY="agnostic-ai"

# Must stay level with PLATFORMS in npm/lib/platforms.js. The Go test
# TestNpmBinariesScript_CoversEveryPlatformPackage holds the two together.
TARGETS=(
  darwin_amd64
  darwin_arm64
  linux_amd64
  linux_arm64
  windows_amd64
  windows_arm64
)

err() { printf 'npm-binaries: %s\n' "$*" >&2; }

archive_name() {
  local target="$1"
  case "$target" in
    windows_*) printf '%s_%s.zip\n' "$BINARY" "$target" ;;
    *) printf '%s_%s.tar.gz\n' "$BINARY" "$target" ;;
  esac
}

binary_name() {
  case "$1" in
    windows_*) printf '%s.exe\n' "$BINARY" ;;
    *) printf '%s\n' "$BINARY" ;;
  esac
}

# gh handles auth, redirects and retries, and it is on every GitHub runner.
# One call for all seven files: seven calls is seven chances to be rate limited.
fetch_release() {
  local tag="$1" dir="$2" patterns=() target
  for target in "${TARGETS[@]}"; do
    patterns+=(--pattern "$(archive_name "$target")")
  done
  gh release download "$tag" --repo "$REPO" --dir "$dir" --clobber \
    "${patterns[@]}" --pattern checksums.txt
}

# checksums.txt covers every release asset, and `shasum -c` fails on a line
# whose file is absent. Check only the archives we pulled.
verify_checksums() {
  local dir="$1" target wanted
  wanted="$dir/wanted.txt"
  : >"$wanted"
  for target in "${TARGETS[@]}"; do
    grep -E "[[:space:]]\*?$(archive_name "$target")\$" "$dir/checksums.txt" >>"$wanted" || {
      err "checksums.txt has no entry for $(archive_name "$target")"
      return 1
    }
  done
  # Linux runners have sha256sum, macOS has shasum. Pick one up front: trying
  # the second after the first fails would turn a real mismatch into a retry.
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$dir" && sha256sum -c wanted.txt)
  else
    (cd "$dir" && shasum -a 256 -c wanted.txt)
  fi
}

# Extract by format, not with one call for both. `tar` on macOS is bsdtar,
# which does read zip, so a single `tar -xf` passes locally and then fails on
# ubuntu-latest, where `tar` is GNU tar and cannot read zip at all. The
# windows targets are the only zips, so that failure was four Windows
# packages missing from an otherwise green release.
unpack() {
  local dir="$1" dest="$2" target out archive
  for target in "${TARGETS[@]}"; do
    out="$dest/$target"
    archive="$dir/$(archive_name "$target")"
    mkdir -p "$out"
    case "$archive" in
      *.zip) unzip -q -o -j "$archive" "$(binary_name "$target")" -d "$out" ;;
      *) tar -xf "$archive" -C "$out" "$(binary_name "$target")" ;;
    esac
    chmod 0755 "$out/$(binary_name "$target")"
  done
}

# A partial set publishes packages for some platforms and pins all six, which
# breaks installs on whatever is missing. Refuse before the generator runs.
require_every_binary() {
  local dest="$1" target missing=0
  for target in "${TARGETS[@]}"; do
    if [[ ! -s "$dest/$target/$(binary_name "$target")" ]]; then
      err "no binary for $target"
      missing=1
    fi
  done
  return "$missing"
}

main() {
  local tag="${1:-}" dest="${2:-}"
  if [[ -z "$tag" || -z "$dest" ]]; then
    err "usage: scripts/npm-binaries.sh <tag> <destdir>"
    return 2
  fi

  local work status=0
  work="$(mktemp -d)"
  if [[ -z "$work" || ! -d "$work" ]]; then
    err "could not create a work directory"
    return 1
  fi

  mkdir -p "$dest"
  # No EXIT trap: this file is sourced by its test suite, and an EXIT trap set
  # here would replace the harness's own.
  {
    fetch_release "$tag" "$work" \
      && verify_checksums "$work" \
      && unpack "$work" "$dest" \
      && require_every_binary "$dest"
  } || status=$?
  rm -rf "$work"
  [[ "$status" -eq 0 ]] || return "$status"

  printf 'npm-binaries: %s binaries from %s in %s\n' "${#TARGETS[@]}" "$tag" "$dest"
}

if [[ -z "${BASH_SOURCE[0]:-}" || "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
