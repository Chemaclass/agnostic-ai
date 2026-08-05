#!/usr/bin/env bash
#
# install.sh - install the agnostic-ai binary on macOS or Linux.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/scripts/install.sh | bash
#   scripts/install.sh
#
# Environment:
#   AGNOSTIC_AI_VERSION      tag to install (default: latest release)
#   AGNOSTIC_AI_INSTALL_DIR  install directory (default: /usr/local/bin when
#                            writable, else ~/.local/bin)
#
# Windows has no tar.gz path here: use scripts/install.ps1.

set -euo pipefail

REPO="Chemaclass/agnostic-ai"
BINARY="agnostic-ai"

die()  { printf 'error: %s\n' "$*" >&2; return 1; }
note() { printf '→ %s\n' "$*"; }

detect_os() {
  case "$(uname -s)" in
    Darwin) printf 'darwin\n' ;;
    Linux)  printf 'linux\n' ;;
    *) die "unsupported OS: $(uname -s). On Windows run scripts/install.ps1" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64)  printf 'amd64\n' ;;
    arm64 | aarch64) printf 'arm64\n' ;;
    *) die "unsupported architecture: $(uname -m)" ;;
  esac
}

asset_name() {
  printf '%s_%s_%s.tar.gz\n' "$BINARY" "$1" "$2"
}

download_url() {
  printf 'https://github.com/%s/releases/download/%s/%s\n' "$REPO" "$1" "$2"
}

# Parsed with grep, not jq: jq is not installed by default on macOS.
latest_version() {
  local api="https://api.github.com/repos/$REPO/releases/latest" tag
  tag="$(curl -fsSL "$api" | grep -m1 '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
  [[ -n "$tag" ]] || die "could not resolve the latest release from $api"
  printf '%s\n' "$tag"
}

resolve_install_dir() {
  if [[ -n "${AGNOSTIC_AI_INSTALL_DIR:-}" ]]; then
    printf '%s\n' "$AGNOSTIC_AI_INSTALL_DIR"
  elif [[ -w /usr/local/bin ]]; then
    printf '/usr/local/bin\n'
  else
    printf '%s/.local/bin\n' "$HOME"
  fi
}

verify_checksum() {
  local archive="$1" asset="$2" version="$3" sums expected actual
  sums="$(dirname "$archive")/checksums.txt"

  curl -fsSL -o "$sums" "$(download_url "$version" checksums.txt)" \
    || { note "checksums.txt unavailable for $version, skipping verification"; return 0; }

  expected="$(grep " $asset\$" "$sums" | awk '{print $1}')"
  [[ -n "$expected" ]] || die "$asset missing from checksums.txt"

  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$archive" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
  else
    note "no sha256 tool found, skipping verification"
    return 0
  fi

  [[ "$actual" == "$expected" ]] || die "checksum mismatch for $asset"
  note "checksum verified"
}

main() {
  command -v curl >/dev/null 2>&1 || die "curl is required"
  command -v tar >/dev/null 2>&1 || die "tar is required"

  local os arch version asset dir tmp
  os="$(detect_os)"
  arch="$(detect_arch)"
  version="${AGNOSTIC_AI_VERSION:-$(latest_version)}"
  asset="$(asset_name "$os" "$arch")"
  dir="$(resolve_install_dir)"

  note "installing $BINARY $version ($os/$arch) into $dir"

  tmp="$(mktemp -d)"
  # Expanded at trap time on purpose: $tmp is local to main() and already out
  # of scope when the EXIT trap runs, which under `set -u` aborts the script.
  # shellcheck disable=SC2064
  trap "rm -rf '$tmp'" EXIT

  curl -fsSL -o "$tmp/$asset" "$(download_url "$version" "$asset")" \
    || die "download failed: $(download_url "$version" "$asset")"
  verify_checksum "$tmp/$asset" "$asset" "$version"
  tar -xzf "$tmp/$asset" -C "$tmp" "$BINARY"

  mkdir -p "$dir"
  # install(1) replaces a running binary atomically; cp can hit ETXTBSY.
  install -m 0755 "$tmp/$BINARY" "$dir/$BINARY" 2>/dev/null \
    || { mv -f "$tmp/$BINARY" "$dir/$BINARY" && chmod 0755 "$dir/$BINARY"; }

  note "installed: $dir/$BINARY"
  "$dir/$BINARY" --version

  case ":$PATH:" in
    *":$dir:"*) ;;
    *) note "add $dir to PATH: export PATH=\"$dir:\$PATH\"" ;;
  esac
}

# A piped install (curl ... | bash) leaves BASH_SOURCE empty, so treat that
# as a direct run. Sourcing for tests keeps main() dormant.
if [[ -z "${BASH_SOURCE[0]:-}" || "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
