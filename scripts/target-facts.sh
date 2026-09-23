#!/usr/bin/env bash
#
# target-facts.sh - print what agnostic-ai currently claims about a target.
#
# One compact dump per target: declared capabilities, default output paths,
# the adapter's package doc comment, the lines docs/site/content/docs/target-behavior.md
# publishes about it, and its own docs/site/content/docs/targets/<target>.md page.
# Feeds the `target-audit` skill so an auditing agent reads the repo's side of
# the comparison in one call instead of grepping Go and the docs tree.
#
# Every target list is derived from the adapter registry, never hardcoded,
# so a newly added adapter is audited without touching this script or the
# skill that drives it.
#
# Usage:
#   scripts/target-facts.sh              # every registered target
#   scripts/target-facts.sh claude zed   # only the named targets
#   scripts/target-facts.sh --list       # target names, one per line
#   scripts/target-facts.sh --batches 5  # registry split into N batches
#   scripts/target-facts.sh --sources zed warp  # selected vendor references
#   scripts/target-facts.sh --changed <run>/docfetch.tsv  # batches sized by drift
#
# Portable: POSIX-ish bash + awk + grep only. No GNU-only flags.

set -euo pipefail

ROOT=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
REGISTRY="$ROOT/internal/adapters/adapter.go"
LOCK="${TARGET_AUDIT_LOCK:-$ROOT/scripts/target-audit/sources.lock}"
TARGETS_DIR="$ROOT/docs/site/content/docs/targets"
BEHAVIOR_PAGE="$ROOT/docs/site/content/docs/target-behavior.md"

usage() {
  cat <<'EOF'
Usage: scripts/target-facts.sh [--list | --batches N | --sources <target>... | <target>...]

  (no args)      dump facts for every registered target
  <target>...    dump facts for the named targets only
  --list         print registered target names, one per line
  --sources <target>...  print only those targets' vendor source sections
  --batches N    split the registry into N batches, one per line as
                 "<n>: <target> <target> ...". Used by the target-audit
                 skill to size its parallel fan-out from the registry
                 rather than a hardcoded table.
  --changed <docfetch.tsv> [N]
                 classify targets by what scripts/docfetch.sh found this
                 run. Targets whose pages or changelog moved are split
                 into at most N deep batches; the rest print on one
                 "sweep:" line.
  -h, --help     this message
EOF
}

# list_targets prints every key of the adapter registry, in source order.
# Registry order is roughly chronological, so the earliest (highest-churn)
# vendors land in the first batch.
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
  ' "$REGISTRY"
}

# batches <n> splits the registry into n near-equal groups, printing one
# group per line as "<index>: <target> <target> ...". A remainder spreads
# across the leading batches so no batch is more than one target larger
# than another.
batches() {
  batch_list "$1" $(list_targets)
}

# batch_list <n> <target>... groups the given targets rather than the whole
# registry, so --changed can size its fan-out from the drifting subset.
batch_list() {
  local n="$1"
  shift
  printf '%s\n' "$@" | awk -v n="$n" '
    NF { t[++count] = $0 }
    END {
      if (count == 0) exit
      if (n < 1) n = 1
      if (n > count) n = count
      base = int(count / n); extra = count % n; i = 1
      for (b = 1; b <= n; b++) {
        size = base + (b <= extra ? 1 : 0)
        line = ""
        for (j = 0; j < size; j++) { line = line (j ? " " : "") t[i]; i++ }
        print b ": " line
      }
    }
  '
}

# pkg_for <target> prints the Go package directory backing that target.
pkg_for() {
  awk -v t="$1" '
    $0 ~ "\"" t "\"" && /\.New\(\)/ {
      line = $0
      sub(/^.*:[[:space:]]*/, "", line)
      sub(/\.New\(\).*$/, "", line)
      print line
      exit
    }
  ' "$REGISTRY"
}

# src_for <pkg> prints the adapter's primary (non-test) source file.
src_for() {
  local pkg="$1" candidate
  if [ -f "$ROOT/internal/adapters/$pkg/$pkg.go" ]; then
    printf '%s\n' "$ROOT/internal/adapters/$pkg/$pkg.go"
    return 0
  fi
  for candidate in "$ROOT/internal/adapters/$pkg/"*.go; do
    case "$candidate" in *_test.go) continue ;; esac
    printf '%s\n' "$candidate"
    return 0
  done
  return 1
}

# doc_comment <file> prints the `//` block immediately above `package X`.
doc_comment() {
  awk '
    /^\/\// { buf = buf $0 "\n"; next }
    /^package / { printf "%s", buf; exit }
    { buf = "" }
  ' "$1"
}

# defaults <file> prints the `target`/`defaultX` const lines, trimmed.
defaults() {
  grep -E '^[[:space:]]*(target|default[A-Za-z]*)[[:space:]]*=' "$1" |
    sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*\/\/.*$//' || true
}

# caps <file> prints the Supports line of the adapter's Capabilities value.
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

# doc_rows <target> prints every target-behavior.md line naming the target.
doc_rows() {
  grep -n -i -w "$1" "$BEHAVIOR_PAGE" || true
}

# doc_section <target> prints the target's own page, without its front matter.
doc_section() {
  local page="$TARGETS_DIR/$1.md"
  [ -f "$page" ] || return 0
  awk 'NR == 1 && /^\+\+\+$/ { fm = 1; next } fm && /^\+\+\+$/ { fm = 0; next } !fm { print }' "$page"
}

# source_sections keeps unrelated vendor history out of each auditor's context.
source_sections() {
  if [ "$#" -eq 0 ]; then
    echo "--sources needs at least one target" >&2
    return 2
  fi
  local target
  for target in "$@"; do
    if [ -z "$(pkg_for "$target")" ]; then
      echo "unknown target: $target (not in the adapter registry)" >&2
      return 1
    fi
  done
  awk -v targets="$*" '
    BEGIN {
      count = split(targets, names, " ")
      for (i = 1; i <= count; i++) selected[names[i]] = 1
    }
    /^## / { include = ($2 in selected) }
    include { print }
  ' "$ROOT/.agnostic-ai/skills/target-audit/references/sources.md"
}

# changed_classes <docfetch.tsv> prints "deep: ..." and "sweep: ..." lines.
# A target is deep when any of its pages is new, changed, or unrecovered, or
# when its changelog moved at all. Everything else is hash-identical to the
# committed lock and only needs a coverage row.
changed_classes() {
  local file="$1"
  if [ ! -r "$file" ]; then
    echo "cannot read $file" >&2
    return 1
  fi
  awk -F '\t' -v lock="$LOCK" '
    BEGIN {
      while ((getline line < lock) > 0) {
        if (substr(line, 1, 1) == "#") continue
        split(line, f, "\t")
        if (f[3] != "") locked[f[3]] = f[6]
      }
      close(lock)
    }
    /^#/ || NF < 6 { next }
    {
      target = $1; kind = $2; url = $3; mode = $5; sha = $6; status = $8
      if (status == "") {
        if (mode == "failed" || mode == "app-shell" || mode == "soft-404") status = "failed"
        else if (!(url in locked)) status = "new"
        else if (locked[url] == sha) status = "unchanged"
        else status = "changed"
      }
      if (!(target in seen)) { seen[target] = 1; order[++n] = target }
      if (status != "unchanged") deep[target] = 1
      if (kind == "changelog" && status != "unchanged") deep[target] = 1
    }
    END {
      for (i = 1; i <= n; i++) {
        t = order[i]
        if (t in deep) d = d (d ? " " : "") t
        else sw = sw (sw ? " " : "") t
      }
      if (d != "") print "deep: " d
      if (sw != "") print "sweep: " sw
    }
  ' "$file"
}

# changed_batches <docfetch.tsv> [n] formats the classification as batches.
changed_batches() {
  local file="$1" n="${2:-5}" classes deep sweep
  classes=$(changed_classes "$file") || return 1
  deep=$(printf '%s\n' "$classes" | sed -n 's/^deep: //p')
  sweep=$(printf '%s\n' "$classes" | sed -n 's/^sweep: //p')
  [ -n "$deep" ] && batch_list "$n" $deep
  [ -n "$sweep" ] && echo "sweep: $sweep"
  return 0
}

# dump_target <target> prints the full fact sheet for one target.
dump_target() {
  local t="$1" pkg src
  pkg=$(pkg_for "$t")
  if [ -z "$pkg" ]; then
    echo "unknown target: $t (not in the adapter registry)" >&2
    return 1
  fi
  src=$(src_for "$pkg")

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
  echo "--- docs/site/content/docs/target-behavior.md lines ---"
  doc_rows "$t"
  echo
  echo "--- docs/site/content/docs/targets/$t.md ---"
  doc_section "$t"
  echo
}

main() {
  case "${1:-}" in
    -h | --help)
      usage
      return 0
      ;;
    --list)
      list_targets
      return 0
      ;;
    --batches)
      if [ -z "${2:-}" ]; then
        echo "--batches needs a count" >&2
        return 2
      fi
      batches "$2"
      return 0
      ;;
    --sources)
      shift
      source_sections "$@"
      return
      ;;
    --changed)
      if [ -z "${2:-}" ]; then
        echo "--changed needs a docfetch.tsv path" >&2
        return 2
      fi
      changed_batches "$2" "${3:-5}"
      return
      ;;
  esac

  if [ "$#" -eq 0 ]; then
    local t
    for t in $(list_targets); do
      dump_target "$t"
    done
    return 0
  fi

  local target
  for target in "$@"; do
    dump_target "$target"
  done
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
