#!/usr/bin/env bash
#
# signals-shipped.sh - set shipped-date in scripts/target-audit/signals.tsv
# from the dated sections of CHANGELOG.md.
#
# A signal ships in the release whose changelog section cites one of its
# issues as #NNN. Only rows with an empty shipped-date change, so a date
# once set is never moved. A changelog line that cites the issue without
# delivering support still counts, so review the rows it names.
#
# Usage:
#   scripts/signals-shipped.sh          # the newest dated section only
#   scripts/signals-shipped.sh --all    # every section, earliest release wins
#
# Portable: POSIX-ish bash + awk. No GNU-only flags.

set -euo pipefail

SIGNALS_ROOT=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)

signals_shipped() {
  local all=0 changelog signals tmp
  if [ "${1:-}" = "--all" ]; then
    all=1
    shift
  fi
  changelog="${1:-$SIGNALS_ROOT/CHANGELOG.md}"
  signals="${2:-$SIGNALS_ROOT/scripts/target-audit/signals.tsv}"
  tmp=$(mktemp)
  awk -F '\t' -v OFS='\t' -v all="$all" '
    FNR == NR {
      if ($0 ~ /^## v[0-9]+\.[0-9]+\.[0-9]+ - [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]/) {
        if (date != "" && !all) { done = 1 }
        date = substr($0, length($0) - 9)
        next
      }
      if (/^## /) { if (date != "" && !all) done = 1; next }
      if (date == "" || done) next
      line = $0
      # Sections run newest first, so a later section overwrites: the
      # earliest release that cites an issue wins.
      while (match(line, /#[0-9]+/)) {
        shipped[substr(line, RSTART + 1, RLENGTH - 1)] = date
        line = substr(line, RSTART + RLENGTH)
      }
      next
    }
    /^#/ || NF != 9 || $8 != "" { print; next }
    {
      best = ""
      n = split($5, ids, ",")
      for (i = 1; i <= n; i++) {
        id = ids[i]
        gsub(/[^0-9]/, "", id)
        if (id in shipped && (best == "" || shipped[id] < best)) best = shipped[id]
      }
      if (best != "") { $8 = best; set++ }
      print
    }
    END { printf "signals: set shipped-date on %d row(s)\n", set > "/dev/stderr" }
  ' "$changelog" "$signals" >"$tmp"
  mv "$tmp" "$signals"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  signals_shipped "$@"
fi
