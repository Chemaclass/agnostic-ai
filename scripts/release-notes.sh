#!/usr/bin/env bash

# Usage: scripts/release-notes.sh vX.Y.Z [CHANGELOG] [owner/repo] [verified-assets-file]
# The optional file lists the verified release asset basenames, one per line.

set -Eeuo pipefail

extract_changelog_section() {
  local file="$1" ver="$2"
  grep -Eq "^## (\[${ver}\]|${ver}( |$))" "$file" || return 1
  awk -v ver="$ver" '
    /^## / {
      if ($0 ~ "^## \\[" ver "\\]" || $0 ~ "^## " ver "( |$)") {
        in_section=1
        next
      }
      if (in_section) exit
    }
    in_section { lines[++n]=$0 }
    END {
      end=n
      while (end > 0 && lines[end] ~ /^[[:space:]]*$/) end--
      start=1
      while (start <= end && lines[start] ~ /^[[:space:]]*$/) start++
      for (i=start; i<=end; i++) print lines[i]
    }
  ' "$file"
}

download_links() {
  local assets="$1" url="$2" label name links=""
  shift 2
  while [[ $# -gt 0 ]]; do
    label="$1"
    name="$2"
    shift 2
    if grep -Fxq -- "$name" <<< "$assets"; then
      links="${links}${links:+ · }[$label]($url/$name)"
    fi
  done
  printf '%s' "$links"
}

format_release_downloads() {
  local assets="$1" url="$2" companion macos linux windows verification
  companion="$(download_links "$assets" "$url" \
    'Apple silicon' agnostic-ai-companion_darwin_arm64.dmg \
    Intel agnostic-ai-companion_darwin_amd64.dmg)"
  macos="$(download_links "$assets" "$url" \
    'Apple silicon' agnostic-ai_darwin_arm64.tar.gz Intel agnostic-ai_darwin_amd64.tar.gz)"
  linux="$(download_links "$assets" "$url" \
    x86-64 agnostic-ai_linux_amd64.tar.gz Arm64 agnostic-ai_linux_arm64.tar.gz)"
  windows="$(download_links "$assets" "$url" \
    x86-64 agnostic-ai_windows_amd64.zip Arm64 agnostic-ai_windows_arm64.zip)"
  verification="$(download_links "$assets" "$url" \
    Checksums checksums.txt 'Build provenance' agnostic-ai.intoto.jsonl)"
  if [[ -z "$companion$macos$linux$windows$verification" ]]; then
    return 0
  fi

  printf '## Downloads\n\n'
  if [[ -n "$companion" ]]; then
    printf '**macOS companion (.dmg)** %s\n\n' "$companion"
    printf '%s\n\n' 'Requires macOS 13 or later. Check **Apple menu > About This Mac** for your chip or processor. [Install and open the companion](https://agnostic-ai.org/docs/cli-reference/companion/#install).'
  fi
  if [[ -n "$macos$linux$windows" ]]; then
    printf '**Command line** · [Installation options](https://agnostic-ai.org/docs/installation/)\n\n'
    if [[ -n "$macos" ]]; then printf -- '- macOS %s\n' "$macos"; fi
    if [[ -n "$linux" ]]; then printf -- '- Linux %s\n' "$linux"; fi
    if [[ -n "$windows" ]]; then printf -- '- Windows %s\n' "$windows"; fi
    printf '\n'
  fi
  printf '[Verify a download](https://agnostic-ai.org/docs/verify-a-release/)%s\n\n' "${verification:+ · $verification}"
  printf '## What changed\n\n'
}

format_release_notes() {
  local ver="$1" changelog="$2" repo="$3" section archive assets
  archive="$(dirname "$changelog")/docs/CHANGELOG-archive.md"
  section="$(extract_changelog_section "$changelog" "$ver" 2>/dev/null)" \
    || section="$(extract_changelog_section "$archive" "$ver" 2>/dev/null)" \
    || { printf 'error: no [%s] section in %s\n' "$ver" "$changelog" >&2; return 1; }
  if [[ $# -eq 4 ]]; then
    assets="$(cat -- "$4")" || return 1
    format_release_downloads "$assets" "https://github.com/$repo/releases/download/$ver"
  fi
  cat <<EOF
$section

---

[Full changelog](https://github.com/$repo/blob/main/CHANGELOG.md) · [README](https://github.com/$repo#readme)
EOF
}

main() {
  local ver="${1:?usage: $0 vX.Y.Z [CHANGELOG] [owner/repo] [verified-assets-file]}"
  local changelog="${2:-CHANGELOG.md}"
  local repo="${3:-}"
  if [[ -z "$repo" ]]; then
    command -v gh >/dev/null 2>&1 || { printf 'error: repo not provided and gh not available\n' >&2; return 1; }
    repo="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
  fi
  format_release_notes "$ver" "$changelog" "$repo" "${@:4}"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
