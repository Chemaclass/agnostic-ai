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
#   AGNOSTIC_AI_VERIFY_ATTESTATION
#                            build provenance is checked whenever gh is
#                            installed and logged in; 1 fails when it cannot
#                            be checked, 0 skips it
#
# Windows has no tar.gz path here: use scripts/install.ps1.

# `curl ... | sh` runs this under POSIX sh, where the bashisms below
# half-execute and leave $version empty, so the script exits 0 having
# installed nothing. Fail loudly instead.
if [ -z "${BASH_VERSION:-}" ]; then
  printf 'install.sh needs bash: run `curl -fsSL <url> | bash`, not `| sh`\n' >&2
  exit 1
fi

set -euo pipefail

REPO="Chemaclass/agnostic-ai"
BINARY="agnostic-ai"
# Same pattern as releaseVersionRE in internal/cli/upgrade.go.
VERSION_RE='^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$'
# Releases before this one were published without build provenance.
FIRST_ATTESTED_MINOR=75
# gh exits 4 when a command needs a login it does not have.
GH_AUTH_REQUIRED=4

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

# Resolved from a redirect, not from the API.
#
# https://api.github.com/repos/<repo>/releases/latest allows 60
# unauthenticated requests per hour per IP. Shared egress (CI runners
# behind NAT, a VPN, an office, a university) burns that without anyone
# doing anything wrong, and the install then dies on a bare 403 that never
# says "rate limit". https://github.com/<repo>/releases/latest answers 302
# to /releases/tag/<tag>, is outside the API, and carries no such limit.
#
# It also removes the JSON parse, and with it the need for jq (absent on
# macOS) or for grep. The lesson the parse left behind still holds for any
# future code here: fetch first, then match. Piping curl straight into
# `grep -m1` makes grep exit on the first match, curl take EPIPE and exit
# 23, and `pipefail` propagate that, so the tag resolves and the function
# still dies depending on who finished writing first. Nothing below pipes
# curl into anything.
latest_version() {
  local url="https://github.com/$REPO/releases/latest" response status location tag

  # No -L: the tag is the redirect target, and following it would fetch a
  # few hundred kilobytes of HTML to learn what the header already said.
  response="$(curl -sS -o /dev/null -w '%{http_code} %{redirect_url}' "$url")" \
    || { die "could not reach $url"; return 1; }
  # "<status> <redirect_url>", with the URL empty when there was no redirect.
  status="${response%% *}"
  location="${response#* }"

  # `die` returns instead of exiting, so a sourced copy stays testable; every
  # branch has to stop the function itself. `esac || return 1` does that for
  # all of them at once, and keeps errexit from firing inside the case.
  case "$status" in
    3??) ;;
    403 | 429)
      die "github rate limited this network while resolving the latest release.
    Wait a minute and retry, or pin the version with AGNOSTIC_AI_VERSION=vX.Y.Z"
      ;;
    404) die "no published release at $url" ;;
    *) die "unexpected HTTP $status from $url" ;;
  esac || return 1

  tag="${location##*/releases/tag/}"
  if [[ "$location" != */releases/tag/* || -z "$tag" ]]; then
    die "no published release: $url redirected to ${location:-nowhere}"
    return 1
  fi
  printf '%s\n' "$tag"
}

# release_tag validates a version before it goes into a download URL and
# prints it as the tag, with its leading v.
release_tag() {
  [[ "$1" =~ $VERSION_RE ]] || { die "'$1' is not a release version; use one like v0.89.0"; return 1; }
  printf 'v%s\n' "${1#v}"
}

# Assumes a 0.x floor: every 1.0 or later release has provenance.
has_provenance() {
  local core="${1#v}" major minor
  core="${core%%-*}"
  major="${core%%.*}"
  minor="${core#*.}"
  minor="${minor%%.*}"
  (( 10#$major > 0 || 10#$minor >= FIRST_ATTESTED_MINOR ))
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
    || { die "checksums.txt unavailable for $version, so $asset cannot be verified"; return 1; }

  expected="$(grep " $asset\$" "$sums" | awk '{print $1}')"
  [[ -n "$expected" ]] || die "$asset missing from checksums.txt"

  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$archive" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
  else
    die "no sha256 tool found (sha256sum or shasum), so $asset cannot be verified"
    return 1
  fi

  [[ "$actual" == "$expected" ]] || die "checksum mismatch for $asset"
  note "checksum verified"
}

# verify_attestation checks the archive's build provenance with the GitHub
# CLI: proof that this repository's release workflow built it from the
# tagged commit, which a matching checksum from the same release page cannot
# give. Pass "pinned" as $3 when the user named the version: only then may a
# release from before provenance existed install without it, since whoever
# can publish release assets could mark such a release latest.
verify_attestation() {
  local archive="$1" tag="$2" pinned="${3:-}" forced=0 status=0 detail
  case "${AGNOSTIC_AI_VERIFY_ATTESTATION:-}" in
    1) forced=1 ;;
    0) note "build provenance not checked: AGNOSTIC_AI_VERIFY_ATTESTATION=0"; return 0 ;;
  esac
  if ! has_provenance "$tag"; then
    if (( forced )); then
      die "$tag predates build provenance, which starts at v0.$FIRST_ATTESTED_MINOR.0, and AGNOSTIC_AI_VERIFY_ATTESTATION=1 requires it"
      return 1
    fi
    if [[ "$pinned" != "pinned" ]]; then
      die "the latest release $tag predates build provenance, which starts at v0.$FIRST_ATTESTED_MINOR.0; set AGNOSTIC_AI_VERSION to install it on the checksum alone"
      return 1
    fi
    note "build provenance not checked: releases before v0.$FIRST_ATTESTED_MINOR.0 do not publish it"
    return 0
  fi
  if ! command -v gh >/dev/null 2>&1; then
    (( forced )) && { die "AGNOSTIC_AI_VERIFY_ATTESTATION=1 needs the GitHub CLI (gh)"; return 1; }
    note "build provenance not checked: install the GitHub CLI (gh) to verify it"
    return 0
  fi
  # The host is pinned: a gh logged in to GitHub Enterprise only would ask
  # that server, which does not hold these attestations.
  detail="$(gh attestation verify "$archive" --hostname github.com --repo "$REPO" \
    --signer-workflow "$REPO/.github/workflows/release.yml" \
    --source-ref "refs/tags/$tag" --deny-self-hosted-runners 2>&1 >/dev/null)" || status=$?
  if (( status != 0 && ! forced )); then
    if (( status == GH_AUTH_REQUIRED )); then
      note "build provenance not checked: gh is not logged in (run gh auth login)"
      return 0
    fi
    if [[ "$detail" == *"unknown flag"* || "$detail" == *"unknown command"* ]]; then
      note "build provenance not checked: this gh cannot verify build provenance; update gh"
      return 0
    fi
  fi
  if (( status != 0 )); then
    die "build provenance did not verify for ${archive##*/}${detail:+: $detail}"
    return 1
  fi
  note "build provenance verified"
}

main() {
  command -v curl >/dev/null 2>&1 || die "curl is required"
  command -v tar >/dev/null 2>&1 || die "tar is required"

  local os arch version asset dir tmp
  os="$(detect_os)"
  arch="$(detect_arch)"
  version="${AGNOSTIC_AI_VERSION:-$(latest_version)}"
  [[ -n "$version" ]] || die "could not resolve a version to install"
  version="$(release_tag "$version")" || return 1
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
  verify_attestation "$tmp/$asset" "$version" "${AGNOSTIC_AI_VERSION:+pinned}"
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
