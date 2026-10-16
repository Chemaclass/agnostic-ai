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
#   scripts/target-facts.sh --changed <tsv> 5 --builtins-since <rev>  # plus built-in changes
#   scripts/target-facts.sh --review-reasons <tsv> --builtins-since <rev>  # review work TSV
#   scripts/target-facts.sh --builtins   # shipped built-in evidence
#
# Portable: POSIX-ish bash + awk + grep only. No GNU-only flags.

set -euo pipefail

ROOT=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
REGISTRY="$ROOT/internal/adapters/adapter.go"
LOCK="${TARGET_AUDIT_LOCK:-$ROOT/scripts/target-audit/sources.lock}"
TARGETS_DIR="$ROOT/docs/site/content/docs/targets"
BEHAVIOR_PAGE="$ROOT/docs/site/content/docs/target-behavior.md"
BUILTINS_DIR="$ROOT/internal/builtins/data"

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
  --builtins     print shipped built-in selectors, events, overrides,
                 and the docs and tests that describe them
  --changed <docfetch.tsv> [N] [--builtins-since <rev>]
                 classify targets by what scripts/docfetch.sh found this
                 run. Targets whose pages or changelog moved are split
                 into at most N deep batches; the rest print on one
                 "sweep:" line. A target whose shipped built-in behavior
                 may have changed since <rev> is deep too, on a
                 "builtin-deep:" line. Without a usable <rev>, every
                 target in the run is.
  --review-reasons <docfetch.tsv> [--builtins-since <rev>]
                 print a TSV of vendor, built-in, shared repository and
                 source recovery work with baseline evidence and checklist
                 IDs. This does not change --changed or fast-path rules.
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

# BUILTIN_SHARED_PATHS are repository paths whose change can alter what every
# shipped built-in emits or how it runs: the built-ins themselves, spec
# selection, config, shared emit and hook runtime, and their docs and tests.
# CLI source is covered by builtin_cli_path.
# Each entry matches as a prefix.
BUILTIN_SHARED_PATHS="internal/builtins/
internal/cli/builtin
internal/spec/
internal/config/
internal/adapters/internal/
internal/markdown/
internal/mdlink/
internal/hookrun/
internal/hookpaths/
internal/applypatch/
go.mod
go.sum
tests/integration/builtin
tests/integration/fixtures/builtin-
tests/integration/fixtures/golden/builtin-
docs/site/content/docs/handoff.md
docs/site/content/docs/configuration.md
docs/site/content/docs/spec-format/hooks.md"

# builtin_names prints the registered built-in names from the Go registry, so
# a stray data folder the binary never loads is not evidence.
builtin_names() {
  awk '
    /^var names = \[\]string\{/ {
      line = $0
      sub(/^[^{]*\{/, "", line); sub(/\}.*$/, "", line)
      gsub(/[",]/, " ", line)
      n = split(line, names, " ")
      for (i = 1; i <= n; i++) print names[i]
      exit
    }
  ' "$ROOT/internal/builtins/builtins.go"
}

# builtin_inventory prints the evidence an auditor reads for shipped
# built-ins: each registered spec's selector, event, and override lines as
# "<path>:<line>:<text>", then the registry, docs, and tests that exist. It
# never prints a spec body and never decides which target a spec reaches.
builtin_inventory() {
  local name file rel path
  echo "--- registered built-ins ---"
  echo "internal/builtins/builtins.go: $(builtin_names | tr '\n' ' ' | sed 's/ $//')"
  echo
  echo "--- spec selectors, events, and overrides ---"
  for name in $(builtin_names); do
    [ -d "$BUILTINS_DIR/$name" ] || continue
    find "$BUILTINS_DIR/$name" -type f \( -name '*.md' -o -name '*.yaml' -o -name '*.yml' \) | sort |
      while IFS= read -r file; do
        rel=${file#"$ROOT"/}
        case "$file" in
          */hooks/*.yaml | */hooks/*.yml | */SKILL.md | */agents/*.md | */rules/*.md | */commands/*.md) ;;
          *) continue ;;
        esac
        awk -v path="$rel" '
          function show() { print path ":" FNR ":" $0 }
          FNR == 1 && /^---[[:space:]]*$/ { md = 1; next }
          md && /^---[[:space:]]*$/ { exit }
          list && /^[[:space:]]+-/ { show(); next }
          { list = 0 }
          /^(command|args|description):/ { xblock = 0; next }
          /^(name|target|targets|target-exclude|targets-exclude|event|on|matcher|match|async|timeout|shell):/ {
            xblock = 0
            list = /^(target|targets|target-exclude|targets-exclude):[[:space:]]*$/
            show(); next
          }
          /^x-[A-Za-z0-9-]+:/ { xblock = 1; show(); next }
          /^[^[:space:]]/ { xblock = 0; next }
          xblock && /^  [A-Za-z0-9_-]+:/ && !/^  (command|args):/ { show() }
        ' "$file"
      done
  done
  echo
  echo "--- repository evidence ---"
  printf '%s\n' "$BUILTIN_SHARED_PATHS" | while IFS= read -r path; do
    case "$path" in internal/*) continue ;; esac
    if [ -e "$ROOT/$path" ]; then
      printf '%s\n' "$path"
    else
      for file in "$ROOT/$path"*; do
        [ -e "$file" ] && printf '%s\n' "${file#"$ROOT"/}"
      done
    fi
  done
}

# run_targets <docfetch.tsv> prints the targets a run requested, in order.
run_targets() {
  local requested
  requested="$(dirname "$1")/requested-targets.txt"
  if [ -r "$requested" ]; then
    cat "$requested"
    return
  fi
  awk -F '\t' '/^#/ || NF < 6 { next } !seen[$1]++ { print $1 }' "$1"
}

# builtin_changed_paths <rev> prints, NUL-separated, every path that differs
# from <rev>: committed, staged, unstaged, deleted, both sides of a rename,
# untracked files Git does not ignore, and ignored files the binary embeds.
builtin_changed_paths() {
  git -C "$ROOT" diff --no-renames --name-only -z "$1" -- || return 1
  git -C "$ROOT" ls-files -z --others --exclude-standard || return 1
  builtin_embedded_ignored || return 1
}

# builtin_embedded_ignored prints, NUL-separated, ignored files under the
# built-in data folder. go:embed reads that folder whatever .gitignore says,
# skipping only names that start with "." or "_".
builtin_embedded_ignored() {
  local data="${BUILTINS_DIR#"$ROOT"/}" list path
  list=$(mktemp) || return 1
  if ! git -C "$ROOT" ls-files -z --others --ignored --exclude-standard -- "$data" >"$list"; then
    rm -f "$list"
    return 1
  fi
  while IFS= read -r -d '' path; do
    case "/${path#"$data"/}" in */.* | */_*) continue ;; esac
    printf '%s\0' "$path"
  done <"$list"
  rm -f "$list"
}

# tsv_field escapes paths containing delimiters without splitting evidence rows.
tsv_field() {
  local value="$1"
  value=${value//\\/\\\\}
  value=${value//$'\t'/\\t}
  value=${value//$'\n'/\\n}
  value=${value//$'\r'/\\r}
  printf '%s' "$value"
}

review_row() {
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$1" "$2" "$(tsv_field "$3")" \
    "$(tsv_field "$4")" "$5" "$6"
}

# builtin_review_reasons is also the source of --changed's forced target set.
# Selectors never narrow ownership: removed selectors and native overrides
# require reading the baseline blob as well as the current source.
builtin_review_reasons() {
  local file="$1" rev="${2:-}" detailed="${3:-1}" requested baseline="-" failure=""
  local path rest pkg target owners reason checklist state list
  requested=$(run_targets "$file" | tr '\n' ' ')
  [ -n "$requested" ] || return 0
  list=$(mktemp) || return 1
  if [ -z "$rev" ]; then
    echo "built-ins: no --builtins-since baseline, so every requested target is read deep" >&2
    failure=missing
  elif ! baseline=$(git -C "$ROOT" rev-parse -q --verify "$rev^{commit}" 2>/dev/null); then
    echo "built-ins: baseline $rev is not a commit here, so every requested target is read deep" >&2
    baseline="$rev"
    failure=invalid
  elif ! builtin_changed_paths "$rev" >"$list" 2>/dev/null; then
    echo "built-ins: git could not compare with $rev, so every requested target is read deep" >&2
    failure=compare-failed
  fi
  if [ -n "$failure" ]; then
    for target in $requested; do
      if [ "$detailed" -eq 0 ]; then
        printf '%s\n' "$target"
      else
        review_row "$target" built-in "$baseline" baseline-unavailable "$failure" built-in-review,baseline-recovery
      fi
    done
    rm -f "$list"
    return 0
  fi
  while IFS= read -r -d '' path; do
    owners="" reason=shared-repository checklist=target-claims,built-in-review
    if builtin_shared_path "$path" || builtin_cli_path "$path"; then
      owners="$requested"
      case "$path" in
        internal/builtins/*) reason=built-in; checklist=built-in-review ;;
      esac
    else
      case "$path" in
        internal/adapters/*/*)
          rest=${path#internal/adapters/}
          pkg=${rest%%/*}
          owners=$(target_for_pkg "$pkg")
          [ -n "$owners" ] || owners="$requested"
          ;;
        internal/adapters/*) owners="$requested" ;;
        docs/site/content/docs/targets/*.md)
          owners=${path#docs/site/content/docs/targets/}
          owners=${owners%.md}
          ;;
      esac
    fi
    [ -n "$owners" ] || continue
    state=changed
    if [ "$detailed" -eq 1 ]; then
      if [ ! -e "$ROOT/$path" ] && [ ! -L "$ROOT/$path" ]; then
        state=removed
        checklist="$checklist,previous-blob"
      elif ! git -C "$ROOT" cat-file -e "$baseline:$path" 2>/dev/null; then
        state=added
      fi
    fi
    for target in $requested; do
      case " $owners " in
        *" $target "*)
          if [ "$detailed" -eq 0 ]; then
            printf '%s\n' "$target"
          else
            review_row "$target" "$reason" "$baseline" "$path" "$state" "$checklist"
          fi
          ;;
      esac
    done
    if [ "$detailed" -eq 0 ] && [ "$owners" = "$requested" ]; then
      break
    fi
  done <"$list"
  rm -f "$list"
}

# builtin_deep_targets preserves run order and the existing conservative set.
builtin_deep_targets() {
  local file="$1" rev="${2:-}" reasons
  reasons=$(builtin_review_reasons "$file" "$rev" 0) || return 1
  printf '%s\n' "$reasons" | awk -F '\t' -v requested="$(run_targets "$file" | tr '\n' ' ')" '
    NF { deep[$1] = 1 }
    END {
      count = split(requested, targets, " ")
      for (i = 1; i <= count; i++) {
        if (targets[i] in deep) out = out (out ? " " : "") targets[i]
      }
      print out
    }
  '
}

# builtin_shared_path <path> succeeds when the path is one of
# BUILTIN_SHARED_PATHS or sits under one.
builtin_shared_path() {
  local shared
  for shared in $BUILTIN_SHARED_PATHS; do
    case "$1" in "$shared"*) return 0 ;; esac
  done
  return 1
}

# builtin_cli_path <path> succeeds for CLI source outside tests. Loading,
# layering, enabling, and editing built-in output spans the whole sync
# pipeline, so any CLI change can alter what a built-in emits.
builtin_cli_path() {
  case "$1" in
    internal/cli/*_test.go) return 1 ;;
    internal/cli/*.go) return 0 ;;
  esac
  return 1
}

# target_for_pkg <pkg> prints the registered target an adapter package backs.
target_for_pkg() {
  awk -v p="$1" '
    /^var registry = map\[string\]Adapter\{/ { inside = 1; next }
    inside && /^\}/ { exit }
    inside && $0 ~ ("[[:space:]]" p "\\.New\\(\\)") {
      name = $0
      sub(/^[[:space:]]*"/, "", name)
      sub(/".*$/, "", name)
      print name
      exit
    }
  ' "$REGISTRY"
}

# changed_classes <docfetch.tsv> prints "deep: ..." and "sweep: ..." lines.
# A target is deep when any of its pages is new, changed, or unrecovered, or
# when its changelog moved at all, or when it is named in [forced], the
# targets whose shipped built-ins changed. Everything else is hash-identical
# to the committed lock and only needs a coverage row.
changed_classes() {
  local file="$1" forced="${2:-}" reasons="${3:-0}" requested
  if [ ! -r "$file" ]; then
    echo "cannot read $file" >&2
    return 1
  fi
  requested=$(run_targets "$file" | tr '\n' ' ')
  awk -F '\t' -v lock="$LOCK" -v forced="$forced" -v requested="$requested" -v reasons="$reasons" '
    BEGIN {
      count = split(requested, names, /[[:space:]]+/)
      for (i = 1; i <= count; i++) if (names[i] != "") { seen[names[i]] = 1; order[++n] = names[i] }
      count = split(forced, names, /[[:space:]]+/)
      for (i = 1; i <= count; i++) if (names[i] != "") deep[names[i]] = 1
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
      if (!(target in seen)) { print "unrequested fetch target: " target > "/dev/stderr"; invalid = 1; exit 1 }
      observed[target] = 1
      if (reasons) {
        if (status != "unchanged") {
          reason = (status == "new" || status == "changed") ? "vendor-delta" : "source-recovery"
          checklist = (reason == "vendor-delta") ? "vendor-review" : "recover-source"
          baseline = (url in locked && locked[url] != "") ? locked[url] : "-"
          print target "\t" reason "\t" baseline "\t" url "\t" status "\t" checklist
        }
        next
      }
      if (status != "unchanged") deep[target] = 1
      if (kind == "changelog" && status != "unchanged") deep[target] = 1
    }
    END {
      if (invalid) exit 1
      if (reasons) {
        for (i = 1; i <= n; i++) {
          t = order[i]
          if (!(t in observed)) print t "\tsource-recovery\t-\trequested-targets.txt\tmissing-source\trecover-source"
        }
        exit
      }
      for (i = 1; i <= n; i++) {
        t = order[i]
        if (!(t in observed)) deep[t] = 1
        if (t in deep) d = d (d ? " " : "") t
        else sw = sw (sw ? " " : "") t
      }
      if (d != "") print "deep: " d
      if (sw != "") print "sweep: " sw
    }
  ' "$file"
}

# review_reasons prints work items without changing batching or fast paths.
review_reasons() {
  local file="$1" rev="${2:-}"
  if [ ! -r "$file" ]; then
    echo "cannot read $file" >&2
    return 1
  fi
  printf 'target\treason\tbaseline\tevidence\tstate\tchecklist\n'
  changed_classes "$file" "" 1 || return 1
  builtin_review_reasons "$file" "$rev" | awk '!seen[$0]++'
}

# changed_batches <docfetch.tsv> [n] [forced] formats the classification as
# batches.
changed_batches() {
  local file="$1" n="${2:-5}" forced="${3:-}" classes deep sweep
  classes=$(changed_classes "$file" "$forced") || return 1
  deep=$(printf '%s\n' "$classes" | sed -n 's/^deep: //p')
  sweep=$(printf '%s\n' "$classes" | sed -n 's/^sweep: //p')
  [ -n "$forced" ] && echo "builtin-deep: $forced"
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
    --builtins)
      builtin_inventory
      return 0
      ;;
    --sources)
      shift
      source_sections "$@"
      return
      ;;
    --review-reasons)
      if [ -z "${2:-}" ]; then
        echo "--review-reasons needs a docfetch.tsv path" >&2
        return 2
      fi
      local file="$2" rev=""
      shift 2
      if [ "$#" -gt 0 ]; then
        if [ "$#" -ne 2 ] || [ "$1" != --builtins-since ] || [ -z "$2" ]; then
          echo "--review-reasons accepts only --builtins-since <rev>" >&2
          return 2
        fi
        rev="$2"
      fi
      review_reasons "$file" "$rev"
      return
      ;;
    --changed)
      if [ -z "${2:-}" ]; then
        echo "--changed needs a docfetch.tsv path" >&2
        return 2
      fi
      local file="$2" n=5 rev="" forced=""
      shift 2
      if [ "$#" -gt 0 ] && [ "$1" != --builtins-since ]; then
        n="$1"
        shift
      fi
      if [ "${1:-}" = --builtins-since ]; then
        if [ -z "${2:-}" ]; then
          echo "--builtins-since needs a revision" >&2
          return 2
        fi
        rev="$2"
      fi
      if [ ! -r "$file" ]; then
        echo "cannot read $file" >&2
        return 1
      fi
      forced=$(builtin_deep_targets "$file" "$rev")
      changed_batches "$file" "$n" "$forced"
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
