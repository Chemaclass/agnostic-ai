#!/usr/bin/env bash
#
# target-facts.sh - print what agnostic-ai currently claims about a target.
#
# One compact dump per target: registered package, declared capabilities,
# default output paths, the adapter's package doc comment, and the rows
# docs/user/targets.md publishes. Feeds the `target-audit` skill so an
# auditing agent reads the repo's side of the comparison in one call
# instead of grepping Go and a 60 KB markdown file.
#
# Usage:
#   scripts/target-facts.sh              # every registered target
#   scripts/target-facts.sh claude zed   # only the named targets
#   scripts/target-facts.sh --list       # target names, one per line
#
# Portable: POSIX-ish bash + awk + grep only. No GNU-only flags.

set -euo pipefail

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
registry="$root/internal/adapters/adapter.go"
targets_doc="$root/docs/user/targets.md"

# pkg_for <target> -> Go package directory backing that target.
pkg_for() {
  awk -v t="$1" '
    $0 ~ "\"" t "\"" && /\.New\(\)/ {
      line = $0
      sub(/^.*:[[:space:]]*/, "", line)
      sub(/\.New\(\).*$/, "", line)
      print line
      exit
    }
  ' "$registry"
}

list_targets() {
  awk '
    /^var registry = map\[string\]Adapter\{/ { inside = 1; next }
    inside && /^\}/ { exit }
    inside && /\.New\(\)/ {
      name = $0
      sub(/^[[:space:]]*"/, "", name)
      sub(/".*$/, "", name)
      print name
    }
  ' "$registry"
}

# doc_comment <file> -> the `//` block immediately above `package X`.
doc_comment() {
  awk '
    /^\/\// { buf = buf $0 "\n"; next }
    /^package / { printf "%s", buf; exit }
    { buf = "" }
  ' "$1"
}

# defaults <file> -> `defaultX = "..."` const lines, trimmed.
defaults() {
  grep -E '^[[:space:]]*(target|default[A-Za-z]*)[[:space:]]*=' "$1" |
    sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*\/\/.*$//' || true
}

# caps <file> -> the Supports line of the adapter's Capabilities value.
caps() {
  awk '
    /^var caps = emit\.Capabilities\{/ { inside = 1; next }
    inside && /^\}/ { exit }
    inside && /Supports:/ {
      line = $0
      sub(/^[[:space:]]*/, "", line)
      gsub(/spec\.Kind/, "", line)
      print line
    }
  ' "$1"
}

# doc_rows <target> -> every docs/user/targets.md line naming that target.
doc_rows() {
  grep -n "\*\*$1\*\*" "$targets_doc" || true
}

# doc_section <target> -> the `### Name (\`target\`)` section body.
doc_section() {
  awk -v t="$1" '
    $0 ~ "^### .*\\(`" t "`\\)" { inside = 1; print; next }
    inside && /^### / { exit }
    inside { print }
  ' "$targets_doc"
}

if [ "${1:-}" = "--list" ]; then
  list_targets
  exit 0
fi

if [ "$#" -gt 0 ]; then
  wanted="$*"
else
  wanted=$(list_targets)
fi

for t in $wanted; do
  pkg=$(pkg_for "$t")
  if [ -z "$pkg" ]; then
    echo "unknown target: $t (not in the registry)" >&2
    exit 1
  fi
  src="$root/internal/adapters/$pkg/$pkg.go"
  if [ ! -f "$src" ]; then
    for candidate in "$root/internal/adapters/$pkg/"*.go; do
      case "$candidate" in *_test.go) continue ;; esac
      src="$candidate"
      break
    done
  fi

  echo "================================================================"
  echo "TARGET: $t   (package internal/adapters/$pkg)"
  echo "================================================================"
  echo
  echo "--- declared capabilities ---"
  caps "$src"
  echo
  echo "--- default output paths ---"
  defaults "$src"
  echo
  echo "--- adapter package doc (what we claim the tool does) ---"
  doc_comment "$src"
  echo "--- docs/user/targets.md rows ---"
  doc_rows "$t"
  echo
  echo "--- docs/user/targets.md section ---"
  doc_section "$t"
  echo
done
