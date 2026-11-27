#!/usr/bin/env bash
#
# release-verify.sh - check a published release from this machine.
#
# Usage:
#   scripts/release-verify.sh vX.Y.Z [owner/repo]
#
# In a new temporary folder it downloads this machine's archive and
# checksums.txt with `gh release download`, checks the archive's SHA-256,
# extracts it, and compares the binary's --version with vX.Y.Z. It then checks
# that the GitHub release has notes and that the release briefing page answers
# 200. The briefing date comes from docs/site/content/updates/*-vX.Y.Z.md.
#
# RELEASE_ROOT points it at another checkout; RELEASE_SITE_URL at another site.
# Portable: bash 3.2, shasum on macOS, sha256sum on Linux.

set -euo pipefail

VERIFY_SCRIPTS=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

verify_fail() { printf 'FAIL %s\n' "$*" >&2; failures=$((failures + 1)); }
verify_ok() { printf 'ok   %s\n' "$*"; }

verify_platform() {
  local os arch
  case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) printf 'error: unsupported system %s\n' "$(uname -s)" >&2; return 1 ;;
  esac
  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *) printf 'error: unsupported machine %s\n' "$(uname -m)" >&2; return 1 ;;
  esac
  printf '%s_%s' "$os" "$arch"
}

verify_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

release_verify() {
  local ver="${1:-}" repo="${2:-}" root site tmp archive want got out briefing date url code f
  failures=0
  printf '%s\n' "$ver" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' || {
    printf 'usage: release-verify.sh vX.Y.Z [owner/repo]\n' >&2
    return 2
  }
  root="${RELEASE_ROOT:-$(CDPATH='' cd -- "$VERIFY_SCRIPTS/.." && pwd)}"
  site="${RELEASE_SITE_URL:-https://agnostic-ai.org}"
  set -- --repo "${repo:-Chemaclass/agnostic-ai}"

  archive="agnostic-ai_$(verify_platform).tar.gz"
  tmp=$(mktemp -d)
  # shellcheck disable=SC2064
  trap "rm -rf '$tmp'" EXIT
  mkdir "$tmp/download" "$tmp/extract"

  if gh release download "$ver" "$@" --dir "$tmp/download" --pattern "$archive" --pattern checksums.txt; then
    want=$(awk -v n="$archive" '$2 == n || $2 == "*" n { print $1 }' "$tmp/download/checksums.txt" 2>/dev/null || true)
    got=$(verify_sha256 "$tmp/download/$archive")
    if [ -n "$want" ] && [ "$want" = "$got" ]; then
      verify_ok "checksum $archive"
      if tar -xzf "$tmp/download/$archive" -C "$tmp/extract"; then
        out=$("$tmp/extract/agnostic-ai" --version 2>&1 || true)
        case "$out" in
          *" ${ver#v}" | *" $ver") verify_ok "version: $out" ;;
          *) verify_fail "version: got '$out', want ${ver#v}" ;;
        esac
      else
        verify_fail "extract $archive"
      fi
    else
      verify_fail "checksum $archive: got ${got:-none}, checksums.txt has ${want:-none}"
    fi
  else
    verify_fail "download $archive and checksums.txt for $ver"
  fi

  out=$(gh release view "$ver" "$@" --json body --jq .body 2>/dev/null || true)
  if [ -n "$(printf '%s' "$out" | tr -d '[:space:]')" ]; then
    verify_ok "release notes are not empty"
  else
    verify_fail "release $ver has no notes"
  fi

  briefing=""
  for f in "$root"/docs/site/content/updates/*-"$ver".md; do
    if [ -f "$f" ]; then briefing="$f"; break; fi
  done
  if [ -z "$briefing" ]; then
    verify_fail "no briefing docs/site/content/updates/*-$ver.md in $root"
  else
    date=$(basename -- "$briefing" "-$ver.md")
    url="$site/updates/$date-$ver/"
    code=$(curl -sS -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || true)
    if [ "$code" = 200 ]; then
      verify_ok "briefing $url"
    else
      verify_fail "briefing $url answered ${code:-nothing}"
    fi
  fi

  if [ "$failures" -gt 0 ]; then
    printf '%s: %d check(s) failed\n' "$ver" "$failures" >&2
    return 1
  fi
  printf '%s: published release verified\n' "$ver"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  release_verify "$@"
fi
