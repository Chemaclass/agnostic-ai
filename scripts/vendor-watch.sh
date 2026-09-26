#!/usr/bin/env bash
#
# vendor-watch.sh - report vendor doc pages that moved, with no AI involved.
#
# Reads the docfetch.tsv a `scripts/docfetch.sh` run writes and lists every
# row whose status is new, changed, or failed against the committed lock.
# The daily vendor-watch workflow publishes that list to one rolling GitHub
# issue labeled `vendor-watch`, so a human decides when a `/target-audit`
# run is worth its tokens. The lock is never written here: it moves only
# after an audit reads the pages, as before.
#
# Each reported page is keyed by URL and content hash. The issue body keeps
# the keys it has reported in a hidden marker, so a page that stays changed
# is reported once, and again only when its text moves a second time.
#
# A changed page whose text this runner already fetched (a known-texts file,
# the snapshot hashes present before the fetch) is not news: a stale CDN copy
# or a proxy alternating between renders would otherwise report daily (#1167).
#
# Usage:
#   scripts/vendor-watch.sh report <docfetch.tsv> [seen-keys-file] [known-texts-file]
#   scripts/vendor-watch.sh publish <docfetch.tsv> [known-texts-file]
#
# Portable: bash + awk + sort + the gh CLI. No GNU-only flags.

set -euo pipefail

VENDOR_WATCH_LABEL="vendor-watch"
VENDOR_WATCH_TITLE="Vendor docs changed since the last target audit"
VENDOR_WATCH_MARKER="<!-- vendor-watch:keys"

# vendor_watch_keys <tsv> prints one "<url>\t<hash>" key per moved row. A
# failed row has no hash, so its key carries the HTTP code instead.
vendor_watch_keys() {
  awk -F '\t' '
    $8 == "new" || $8 == "changed" { print $3 "\t" $6 }
    $8 == "failed" { print $3 "\tfailed-" $4 }
  ' "$1" | sort -u
}

# vendor_watch_known_texts <snapshot dir> prints the hash of every text the
# runner has fetched and still holds, one per line.
vendor_watch_known_texts() {
  [ -d "$1" ] || return 0
  find "$1" -maxdepth 1 -type f -name '*.txt' | sed -e 's|.*/||' -e 's|\.txt$||' | sort
}

# vendor_watch_report <tsv> <seen-keys-file> [known-texts-file] prints the
# Markdown report of moved rows whose key is not in the seen file and, for a
# changed row, whose text is not a known one, grouped by target. It prints
# nothing when every moved row was already reported.
# When the run left a deltas.tsv beside it, each moved page also carries
# its delta label (mentions:<paths>, prose, chrome-only, ...), so the reader
# can tell a config change from page chrome before spending an audit.
vendor_watch_report() {
  local deltas
  deltas="$(dirname "$1")/deltas.tsv"
  [ -r "$deltas" ] || deltas=/dev/null
  awk -F '\t' -v seen="$2" -v known="${3:-/dev/null}" -v deltas="$deltas" '
    BEGIN {
      while ((getline line < seen) > 0) done[line] = 1
      while ((getline line < known) > 0) fetched[line] = 1
      while ((getline line < deltas) > 0) {
        split(line, d, "\t")
        if (d[3] != "" && d[4] != "") tag[d[3]] = d[4]
      }
    }
    $8 != "new" && $8 != "changed" && $8 != "failed" { next }
    {
      key = $3 "\t" ($8 == "failed" ? "failed-" $4 : $6)
      if (key in done) next
      if ($8 == "changed" && ($6 in fetched)) next
      label = ($8 == "failed") ? "failed (HTTP " $4 ")" : $8
      if (!($1 in rows)) order[++n] = $1
      rows[$1] = rows[$1] "- " label ": " $3 (($3 in tag) ? " (`" tag[$3] "`)" : "") "\n"
    }
    END {
      if (n == 0) exit
      for (i = 1; i <= n; i++)
        for (j = i + 1; j <= n; j++)
          if (order[j] < order[i]) { t = order[i]; order[i] = order[j]; order[j] = t }
      for (i = 1; i <= n; i++) {
        printf "### %s\n\n%s\n", order[i], rows[order[i]]
        targets = targets (i > 1 ? " " : "") order[i]
      }
      printf "Next: run `/target-audit %s`, then close this issue once the lock moves.\n", targets
    }
  ' "$1"
}

# vendor_watch_marker <keys> wraps keys in the hidden issue-body marker.
vendor_watch_marker() {
  printf '%s\n%s\n-->\n' "$VENDOR_WATCH_MARKER" "$1"
}

# vendor_watch_marker_keys reads an issue body on stdin and prints the keys
# held in its marker. With "rest" it prints the body without the marker.
vendor_watch_marker_keys() {
  awk -v open="$VENDOR_WATCH_MARKER" -v want="${1:-keys}" '
    $0 == open { inside = 1; next }
    inside && $0 == "-->" { inside = 0; next }
    inside { if (want == "keys" && NF) print; next }
    want == "rest" { print }
  '
}

# vendor_watch_publish <tsv> opens the rolling issue, or comments on the open
# one with only the pages it has not reported yet and refreshes its marker.
vendor_watch_publish() {
  local tsv="$1" known="${2:-/dev/null}" number body="" seen="" report keys
  number=$(gh issue list --label "$VENDOR_WATCH_LABEL" --state open \
    --json number --jq '.[0].number // empty')
  if [ -n "$number" ]; then
    body=$(gh issue view "$number" --json body --jq .body)
    seen=$(printf '%s\n' "$body" | vendor_watch_marker_keys)
  fi

  report=$(vendor_watch_report "$tsv" <(printf '%s\n' "$seen") "$known")
  if [ -z "$report" ]; then
    echo "vendor-watch: nothing new to report"
    return 0
  fi
  keys=$(printf '%s\n%s\n' "$seen" "$(vendor_watch_keys "$tsv")" | grep . | sort -u)

  if [ -z "$number" ]; then
    gh label create "$VENDOR_WATCH_LABEL" --color c5def5 \
      --description "Vendor doc pages moved since the last target audit" >/dev/null 2>&1 || true
    # shellcheck disable=SC2016 # the backticks are Markdown, not a command
    gh issue create --title "$VENDOR_WATCH_TITLE" --label "$VENDOR_WATCH_LABEL" \
      --body "$(printf '%s\n\n%s\n\n%s' \
        'The daily `scripts/docfetch.sh` run found vendor pages whose text moved since the last audit (`scripts/target-audit/sources.lock`). No AI read them yet.' \
        "$report" "$(vendor_watch_marker "$keys")")"
    return 0
  fi

  gh issue comment "$number" --body "$report"
  gh issue edit "$number" --body "$(printf '%s\n' "$body" | vendor_watch_marker_keys rest)
$(vendor_watch_marker "$keys")"
}

vendor_watch_main() {
  local cmd="${1:-}"
  shift || true
  case "$cmd" in
    report | publish) ;;
    *)
      echo "Usage: scripts/vendor-watch.sh report <docfetch.tsv> [seen-keys-file] [known-texts-file]" >&2
      echo "       scripts/vendor-watch.sh publish <docfetch.tsv> [known-texts-file]" >&2
      return 1
      ;;
  esac
  if [ ! -r "${1:-}" ]; then
    echo "vendor-watch: cannot read docfetch.tsv '${1:-}'" >&2
    return 1
  fi
  case "$cmd" in
    report) vendor_watch_report "$1" "${2:-/dev/null}" "${3:-/dev/null}" ;;
    publish) vendor_watch_publish "$1" "${2:-/dev/null}" ;;
  esac
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  vendor_watch_main "$@"
fi
