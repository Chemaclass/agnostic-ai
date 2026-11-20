#!/usr/bin/env bash
#
# changelog-assemble.sh - fold changes/*.md into CHANGELOG.md [Unreleased].
#
# Each pull request adds its own changes/<slug>.md instead of editing
# CHANGELOG.md, so two pull requests never touch the same lines. A file holds
# the `### Section` (and `#### Tool`, when it applies) headings it belongs
# under, each followed by its bullets, as they read in CHANGELOG.md. A release
# runs this script: it appends each bullet under its heading in [Unreleased],
# adds a missing heading in the changelog's section order, and deletes the
# files. Entries already in [Unreleased] keep their text and order.
#
# Usage:
#   scripts/changelog-assemble.sh [changelog] [changes-dir]
#   scripts/changelog-assemble.sh --check [changelog] [changes-dir]
#
# --check validates the files and changes nothing.
#
# Portable: POSIX-ish bash and awk. No GNU-only flags.

set -euo pipefail

CHANGELOG_ROOT=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)

change_files() {
  local dir="$1" f
  [ -d "$dir" ] || return 0
  for f in "$dir"/*.md; do
    [ -f "$f" ] || continue
    [ "$(basename -- "$f")" = README.md ] && continue
    [ -s "$f" ] || { printf 'error: %s: no entry\n' "$f" >&2; return 1; }
    printf '%s\n' "$f"
  done
}

# merge_unreleased reads the [Unreleased] body on stdin and prints it with the
# entries of each change file added. It fails, naming the file, on a file with
# no heading, an entry before its first heading, or a level-2 heading.
merge_unreleased() {
  awk '
    function rank(h) {
      if (h == "Highlights") return 1
      if (h == "General") return 2
      if (h == "By tool") return 3
      if (h == "Site") return 4
      return 5
    }
    function bucket(h3, h4,   k) {
      k = h3 SUBSEP h4
      if (!(k in lines)) {
        lines[k] = 0
        if (!(h3 in h3seen)) { h3seen[h3] = 1; h3order[++nh3] = h3 }
        if (h4 != "") { nh4[h3]++; h4order[h3, nh4[h3]] = h4 }
      }
      return k
    }
    function close_bucket() { if (curk != "") pending[curk] = 0; curk = ""; chunk = 0 }
    function add(k, line) {
      curk = k
      if (line ~ /^[[:space:]]*$/) {
        if (chunk) pending[k]++
        return
      }
      chunk = 1
      while (pending[k] > 0) { body[k, ++lines[k]] = ""; pending[k]-- }
      body[k, ++lines[k]] = line
    }
    function fail(msg) { printf "error: %s: %s\n", source, msg > "/dev/stderr"; failed = 1; exit 1 }
    function emit(k,   i) {
      if (lines[k] == 0) return
      for (i = 1; i <= lines[k]; i++) print body[k, i]
      print ""
    }
    FNR == 1 {
      if (fileno > 0 && is_change && entries == 0) fail("no entry")
      fileno++
      source = (FILENAME == "-") ? "[Unreleased]" : FILENAME
      is_change = (FILENAME != "-")
      close_bucket()
      h3 = ""; h4 = ""; entries = 0; prelude = 0
    }
    /^## / {
      if (is_change) fail("use ### and #### headings only")
      next
    }
    /^### / { close_bucket(); h3 = substr($0, 5); h4 = ""; bucket(h3, h4); next }
    /^#### / {
      if (h3 == "") fail("a #### heading needs a ### heading above it")
      close_bucket(); h4 = substr($0, 6); bucket(h3, h4); next
    }
    {
      if (h3 == "") {
        if (is_change && $0 !~ /^[[:space:]]*$/) fail("an entry needs a ### heading above it")
        if (!is_change) { preludeline[++nprelude] = $0 }
        next
      }
      if ($0 !~ /^[[:space:]]*$/) entries++
      add(bucket(h3, h4), $0)
    }
    END {
      if (failed) exit 1
      if (is_change && entries == 0) fail("no entry")
      for (i = 1; i <= nprelude; i++) if (preludeline[i] !~ /^[[:space:]]*$/) print preludeline[i]
      print ""
      for (i = 1; i <= nh3; i++) idx[i] = i
      for (i = 2; i <= nh3; i++) {
        for (j = i; j > 1 && rank(h3order[idx[j]]) < rank(h3order[idx[j - 1]]); j--) {
          t = idx[j]; idx[j] = idx[j - 1]; idx[j - 1] = t
        }
      }
      for (i = 1; i <= nh3; i++) {
        h = h3order[idx[i]]
        print "### " h
        print ""
        emit(h SUBSEP "")
        for (j = 1; j <= nh4[h]; j++) {
          print "#### " h4order[h, j]
          print ""
          emit(h SUBSEP h4order[h, j])
        }
      }
    }
  ' - "$@"
}

assemble_changelog() {
  local check=0
  if [ "${1:-}" = --check ]; then check=1; shift; fi
  local changelog="${1:-$CHANGELOG_ROOT/CHANGELOG.md}"
  local dir="${2:-$CHANGELOG_ROOT/changes}"
  local files=() f merged tmp
  local listed
  listed=$(change_files "$dir") || return 1
  while IFS= read -r f; do [ -n "$f" ] && files+=("$f"); done <<< "$listed"

  grep -q '^## \[Unreleased\]$' "$changelog" || {
    printf 'error: %s has no "## [Unreleased]" heading\n' "$changelog" >&2
    return 1
  }
  merged=$(awk '/^## \[Unreleased\]$/{p=1;next} p && /^## /{exit} p' "$changelog" |
    merge_unreleased ${files[@]+"${files[@]}"}) || return 1

  if [ "$check" = 1 ]; then
    printf 'changelog: %s change file(s) ready to assemble\n' "${#files[@]}"
    return 0
  fi
  [ "${#files[@]}" -gt 0 ] || { printf 'changelog: no change files in %s\n' "$dir"; return 0; }

  tmp=$(mktemp)
  MERGED="$merged" awk '
    /^## \[Unreleased\]$/ {
      print
      if (ENVIRON["MERGED"] != "") print ENVIRON["MERGED"]
      print ""
      skip = 1
      next
    }
    skip && /^## / { skip = 0 }
    !skip
  ' "$changelog" > "$tmp"
  mv "$tmp" "$changelog"
  rm -f -- "${files[@]}"
  printf 'changelog: assembled %s change file(s) into [Unreleased]\n' "${#files[@]}"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  assemble_changelog "$@"
fi
