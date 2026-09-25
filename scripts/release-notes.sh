#!/usr/bin/env bash

# Usage: scripts/release-notes.sh vX.Y.Z [CHANGELOG] [owner/repo]

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

format_release_notes() {
  local ver="$1" changelog="$2" repo="$3" section archive
  archive="$(dirname "$changelog")/docs/CHANGELOG-archive.md"
  section="$(extract_changelog_section "$changelog" "$ver" 2>/dev/null)" \
    || section="$(extract_changelog_section "$archive" "$ver" 2>/dev/null)" \
    || { printf 'error: no [%s] section in %s\n' "$ver" "$changelog" >&2; return 1; }
  cat <<EOF
$section

---

[Full changelog](https://github.com/$repo/blob/main/CHANGELOG.md) · [README](https://github.com/$repo#readme)
EOF
}

main() {
  local ver="${1:?usage: $0 vX.Y.Z [CHANGELOG] [owner/repo]}"
  local changelog="${2:-CHANGELOG.md}"
  local repo="${3:-}"
  if [[ -z "$repo" ]]; then
    command -v gh >/dev/null 2>&1 || { printf 'error: repo not provided and gh not available\n' >&2; return 1; }
    repo="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
  fi
  format_release_notes "$ver" "$changelog" "$repo"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
