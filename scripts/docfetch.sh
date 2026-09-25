#!/usr/bin/env bash
#
# docfetch.sh - fetch every vendor page the target-audit skill watches.
#
# Reads the URLs out of .agnostic-ai/skills/target-audit/references/sources.md,
# fetches each one fresh, runs the recovery ladder when a page does not serve
# usable text, hashes the result, and compares it against the committed lock at
# scripts/target-audit/sources.lock. Auditors then read only the pages whose
# content moved. Every URL is still fetched on every run, so the lock decides
# what a model reads, never whether the evidence is current.
#
# Each new or changed row also gets a word diff against the text the last
# audit read (local/target-audit/snapshots/<lock sha>.txt), saved as
# <page>.delta, and a label in the run's deltas.tsv: mentions:<paths>,
# prose, chrome-only, whitespace-only, or no-snapshot.
#
# Usage:
#   scripts/docfetch.sh                      # every target, into today's run dir
#   scripts/docfetch.sh claude zed           # only the named targets
#   scripts/docfetch.sh --out DIR [target...]
#   scripts/docfetch.sh --urls claude        # resolved URLs, no network
#   scripts/docfetch.sh --update RUN/docfetch.tsv [target...]
#
# Portable: POSIX-ish bash + awk + grep + diff + curl only. No GNU-only flags.

set -euo pipefail

DOCFETCH_ROOT=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
# shellcheck source=scripts/target-facts.sh
. "$DOCFETCH_ROOT/scripts/target-facts.sh"

UA="agnostic-ai-target-audit/1 (+https://github.com/Chemaclass/agnostic-ai)"
READER_PROXY="https://r.jina.ai"
TIMEOUT="${DOCFETCH_TIMEOUT:-25}"

docfetch_usage() {
  cat <<'EOF'
Usage: scripts/docfetch.sh [--out DIR] [<target>...]
       scripts/docfetch.sh --urls <target>...
       scripts/docfetch.sh --update <docfetch.tsv> [<target>...]
       scripts/docfetch.sh --compare-mirrors <run dir a> <run dir b>

  (no args)      fetch every registered target's docs and changelog URLs
  <target>...    fetch only the named targets
  --out DIR      run directory (default local/target-audit/<utc date>-run)
  --urls         print "<target>\t<kind>\t<url>" without fetching
  --update FILE  merge a run's rows into scripts/target-audit/sources.lock
  --mirrors      also fetch each scraped page's .md copy into mirrors.tsv
                 (measurement only, never hashed into the lock)
  --compare-mirrors A B
                 per host, how many mirrored rows moved as scraped text and
                 as Markdown copy between two runs
  -h, --help     this message
EOF
}

# resolve_urls <target> prints "<kind>\t<url>" for one target's source lines.
# Shorthand on those lines is URL-relative: "/x" against the last absolute
# URL's origin, ".../x" against its directory. Parenthesised text is
# commentary and may quote URLs that must not be fetched.
resolve_urls() {
  source_sections "$1" | awk '
    function emit(u,   k) {
      sub(/[.,;:]+$/, "", u)
      if (u == "" || seen[u]) return
      seen[u] = 1
      print kind "\t" u
    }
    function strip_parens(s,   out, depth, i, c) {
      out = ""; depth = 0
      for (i = 1; i <= length(s); i++) {
        c = substr(s, i, 1)
        if (c == "(") { depth++; continue }
        if (c == ")") { if (depth > 0) depth--; continue }
        if (depth == 0) out = out c
      }
      return out
    }
    /^- docs: / { kind = "docs"; line = substr($0, 9) }
    /^- changelog: / { kind = "changelog"; line = substr($0, 14) }
    /^- (docs|changelog): / {
      n = split(strip_parens(line), w, /[ \t]+/)
      for (i = 1; i <= n; i++) {
        t = w[i]
        if (index(t, "`") > 0) continue
        if (t ~ /^https?:\/\//) {
          origin = (match(t, /^https?:\/\/[^\/]+/) ? substr(t, RSTART, RLENGTH) : "")
          dir = t
          sub(/[^\/]*$/, "", dir)
          emit(t)
        } else if (t ~ /^\.\.\.\//) {
          if (dir != "") emit(dir substr(t, 5))
        } else if (t ~ /^\//) {
          if (origin != "") emit(origin t)
        }
      }
    }
  '
}

# github_rewrite <url> prints "<fetch-url>\t<mode>". Rendered GitHub pages are
# mostly chrome; the raw and API forms are the same content without it.
github_rewrite() {
  local url="$1" path
  case "$url" in
    https://github.com/*/*/blob/*)
      path=${url#https://github.com/}
      printf 'https://raw.githubusercontent.com/%s/%s\t%s\n' "${path%%/blob/*}" "${path#*/blob/}" raw-github
      ;;
    https://github.com/*/*/releases)
      path=${url#https://github.com/}
      printf 'https://api.github.com/repos/%s?per_page=10\t%s\n' "$path" github-api
      ;;
    https://github.com/*/*)
      path=${url#https://github.com/}
      case "$path" in
        */*/*) printf '%s\t%s\n' "$url" "" ;;
        *) printf 'https://raw.githubusercontent.com/%s/HEAD/README.md\t%s\n' "${path%/}" raw-github ;;
      esac
      ;;
    *) printf '%s\t%s\n' "$url" "" ;;
  esac
}

# docfetch_curl <url> <outfile> prints "<code>\t<final-url>\t<content-type>".
# Every network read goes through here so tests can replace it.
docfetch_curl() {
  local url="$1" out="$2" auth=()
  case "$url" in
    https://api.github.com/*) [ -n "${GH_TOKEN:-}" ] && auth=(-H "Authorization: Bearer $GH_TOKEN") ;;
  esac
  curl -sS -L --compressed --max-time "$TIMEOUT" -A "$UA" "${auth[@]+"${auth[@]}"}" \
    -o "$out" -w '%{http_code}\t%{url_effective}\t%{content_type}' "$url" 2>/dev/null ||
    printf '000\t%s\t\n' "$url"
}

# proxy_curl <url> <out> fetches <url> through the reader proxy. The free
# tier answers 429 once a runner sends a burst of pages, so a rate-limited
# fetch waits and retries, three times, with a growing pause. The pauses
# draw on one budget per target (DOCFETCH_RETRY_BUDGET seconds, kept in
# DOCFETCH_RETRY_STATE), so a proxy that stays down fails the remaining
# rows fast instead of outliving the workflow timeout with no report.
proxy_curl() {
  local url="$1" out="$2" attempt line pause spent
  for attempt in 1 2 3 4; do
    line=$(docfetch_curl "$READER_PROXY/$url" "$out")
    [ "${line%%	*}" = "429" ] && [ "$attempt" -lt 4 ] || break
    pause=$((${DOCFETCH_RETRY_SLEEP:-15} * attempt))
    spent=$(cat "${DOCFETCH_RETRY_STATE:-/dev/null}" 2>/dev/null || true)
    spent=${spent:-0}
    [ $((spent + pause)) -lt "${DOCFETCH_RETRY_BUDGET:-120}" ] || break
    [ -n "${DOCFETCH_RETRY_STATE:-}" ] && echo $((spent + pause)) >"$DOCFETCH_RETRY_STATE"
    sleep "$pause"
  done
  printf '%s\n' "$line"
}

# strip_html <file> prints the page's visible text, so a nonce or a rebuilt
# script bundle does not read as a documentation change. Navigation, footers,
# the <head>, and a "last modified" stamp are site chrome: a reordered sidebar
# or a rebuild date would otherwise mark every page on the host as changed.
# The separator is the class "[<]", not the string "<": macOS awk also
# splits on newlines for a one-character string, which dropped every line
# after the first of a multi-line text node there but not on Linux.
# A tag ends at the first ">" outside a quoted attribute: utility-class sites
# put ">" inside class values ("[&>*:first-child]:rounded-r-none"), and
# cutting there leaked the class list into the text on every deploy.
strip_html() {
  awk '
    function tag_end(s,   i, c, q) {
      q = ""
      for (i = 1; i <= length(s); i++) {
        c = substr(s, i, 1)
        if (q != "") { if (c == q) q = ""; continue }
        if (c == "\"" || c == "\047") q = c
        else if (c == ">") return i
      }
      return 0
    }
    { all = all $0 "\n" }
    END {
      n = split(all, parts, "[<]")
      out = parts[1]
      for (i = 2; i <= n; i++) {
        p = parts[i]
        gt = tag_end(p)
        if (gt == 0) continue
        tag = tolower(substr(p, 1, gt - 1))
        text = substr(p, gt + 1)
        if (tag ~ /^(script|style)[ \t>]?/ || tag ~ /^(script|style)$/) skip = 1
        else if (tag ~ /^\/(script|style)$/) { skip = 0; continue }
        else if (tag ~ /^(nav|footer|head|title)([ \t]|$)/) chrome++
        else if (tag ~ /^\/(nav|footer|head|title)$/) { if (chrome > 0) chrome--; continue }
        if (tag ~ /class="[^"]*last-(modified|updated)/) continue
        if (!skip && !chrome) out = out " " text
      }
      gsub(/[ \t\r\n]+/, " ", out)
      sub(/^ /, "", out)
      sub(/ $/, "", out)
      print out
    }
  ' "$1"
}

# reader_text prints a reader-proxy body without the proxy's own header, so
# a fresh "Published Time:" stamp or a blank-line reflow does not read as a
# documentation change. The header ends at "Markdown Content:"; a body
# without that line is kept whole. The proxy also renders an embedded video
# as "Video unavailable" and a lazy image as "Loading image..." on some
# fetches only, so both drop, and so does image markup: an image is not a
# config claim. Kiro's "Page updated: <date>" stamp drops too; the proxy
# renders it with and without the space after the colon. Prose whitespace
# collapses to one space, never to none, so "foo bar" and "foobar" differ.
# Code keeps its lines and indentation verbatim, whether fenced with
# backticks or tildes or indented four spaces: in YAML or shell a newline
# or an indent is part of the claim.
reader_text() {
  awk '
    function flush(   p) {
      p = prose
      gsub(/Loading image\.\.\./, "", p)
      gsub(/!\[[^]]*\]\([^)]*\)/, "", p)
      gsub(/[ \t\r\n]+/, " ", p)
      sub(/^ /, "", p)
      sub(/ $/, "", p)
      if (p != "") out = out (out != "" ? " " : "") p
      prose = ""
    }
    { all[++na] = $0 }
    /^Markdown Content:/ && !seen { seen = 1; nb = 0; next }
    seen { body[++nb] = $0 }
    END {
      n = seen ? nb : na
      for (i = 1; i <= n; i++) {
        l = seen ? body[i] : all[i]
        sub(/\r$/, "", l)
        if (match(l, /^[ \t]*(```+|~~~+)/)) {
          run = substr(l, RSTART, RLENGTH)
          sub(/^[ \t]*/, "", run)
          if (fence == "") { flush(); fence = run; out = out (out != "" ? "\n" : "") l; continue }
          # A fence closes only on its own character, at least as long,
          # with nothing after it: "```yaml" inside a "````" block is text.
          rest = substr(l, RSTART + RLENGTH)
          if (substr(run, 1, 1) == substr(fence, 1, 1) && length(run) >= length(fence) && rest ~ /^[ \t]*$/) {
            fence = ""; out = out "\n" l; continue
          }
        }
        if (fence != "") { out = out "\n" l; continue }
        if (l ~ /^(    |\t)/ && l ~ /[^ \t]/) { flush(); out = out (out != "" ? "\n" : "") l; continue }
        if (l ~ /^Video unavailable[ \t]*$/ || l ~ /^Page updated:/) continue
        prose = prose l "\n"
      }
      flush()
      print out
    }
  ' "$@"
}

# url_origin <url> prints the scheme and host. BSD sed has no \? operator, so
# the origin is cut with awk's ERE instead.
url_origin() {
  printf '%s' "$1" | awk '{ print (match($0, /^https?:\/\/[^\/]+/) ? substr($0, RSTART, RLENGTH) : $0) }'
}

# meta_refresh_target <file> <base> prints the URL a client-side redirect points
# at. WebFetch follows HTTP redirects but not this tag, so a moved page looks
# like a short live page instead of a move.
meta_refresh_target() {
  local file="$1" base="$2" target
  target=$(tr 'A-Z' 'a-z' <"$file" | tr -d '\n' |
    sed -n 's/.*http-equiv="refresh"[^>]*content="[^"]*url=\([^";]*\).*/\1/p' | head -1)
  [ -n "$target" ] || return 1
  case "$target" in
    http://* | https://*) printf '%s\n' "$target" ;;
    /*) printf '%s%s\n' "$(url_origin "$base")" "$target" ;;
    *) printf '%s%s\n' "$(printf '%s' "$base" | sed -e 's|[^/]*$||')" "$target" ;;
  esac
}

# router_payload <file> <out> extracts a server-rendered CMS payload. WebFetch
# truncates these, so the page reads as empty while the text is in the response.
router_payload() {
  awk -v out="$2" '
    { all = all $0 "\n" }
    END {
      markers["window._ROUTER_DATA"] = 1
      markers["__NEXT_DATA__"] = 1
      markers["__INITIAL_STATE__"] = 1
      for (m in markers) {
        p = index(all, m)
        if (p == 0) continue
        rest = substr(all, p)
        b = index(rest, "{")
        if (b == 0) continue
        rest = substr(rest, b)
        e = index(rest, "</script>")
        if (e > 0) rest = substr(rest, 1, e - 1)
        sub(/[ \t\r\n;]+$/, "", rest)
        if (length(rest) < 200) continue
        printf "%s", rest > out
        print "ok"
        exit
      }
    }
  ' "$1"
}

# llms_entry <llms.txt> <slug> prints the index entry for one page. The index
# itself is never the page: hashing it would give every page on the host the
# same digest and hide each one's own changes.
llms_entry() {
  awk -v slug="$2" '
    {
      n = split($0, parts, "(")
      for (i = 2; i <= n; i++) {
        u = parts[i]
        sub(/\).*$/, "", u)
        if (u !~ /^https?:\/\//) continue
        p = u
        sub(/[?#].*$/, "", p)
        sub(/\/$/, "", p)
        sub(/\.md$/, "", p)
        m = p
        sub(/^.*\//, "", m)
        if (m == slug) { print u; exit }
      }
    }
  ' "$1"
}

# sha256_of <file> prints the digest, using whichever tool this host ships.
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{ print $1 }'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{ print $1 }'
  else
    openssl dgst -sha256 "$1" | awk '{ print $NF }'
  fi
}

# delta_text <file> prints the Quill delta text of a router payload, one ops
# list per line. The payload also embeds the whole site's sidebar with each
# page's publish time, so hashing it marks every page changed when any one is.
delta_text() {
  command -v jq >/dev/null 2>&1 &&
    jq -r '[.. | objects | select(has("ops")) | [.ops[]? | .insert? | strings] | join("")] | join("\n")' "$1" 2>/dev/null
}

# json_sum <file> hashes a JSON document with its object keys sorted, since
# key order carries no meaning and a vendor API may reorder a map between
# fetches. It hashes the raw bytes when jq is missing or the body is not JSON.
json_sum() {
  local sorted="$1.sorted"
  if command -v jq >/dev/null 2>&1 && jq -S -c . "$1" >"$sorted" 2>/dev/null; then
    sha256_of "$sorted"
  else
    sha256_of "$1"
  fi
  rm -f "$sorted"
}

# Snapshots hold fetched text by the hash docfetch took of it, so a later
# run can show what moved instead of only that something did. A delta is
# taken against snapshots/<the lock row's sha>.txt: content-addressed, it
# can never disagree with the lock, whatever branch or pull moved the lock,
# and any machine that fetched the same text can serve it (the vendor
# watcher caches the directory between CI runs). Every fetch stores what it
# hashed; --update prunes files the lock no longer names after 30 days.
snapshot_dir() {
  printf '%s\n' "${DOCFETCH_SNAPSHOTS:-$DOCFETCH_ROOT/local/target-audit/snapshots}"
}

# snapshot_file <sha> prints where the text with that hash lives.
snapshot_file() {
  printf '%s/%s.txt\n' "$(snapshot_dir)" "$1"
}

# snapshot_valid <file> <sha> succeeds when the file's text hashes to sha,
# by either normalization docfetch uses (raw bytes, or sorted JSON). The
# name alone is not proof: a partial write or a damaged cache restore keeps
# the name and loses the bytes.
snapshot_valid() {
  [ -f "$1" ] || return 1
  [ "$(sha256_of "$1")" = "$2" ] || [ "$(json_sum "$1")" = "$2" ]
}

# locked_sha <url> prints the committed lock row's hash, if any.
locked_sha() {
  [ -r "$LOCK" ] || return 0
  awk -F '\t' -v u="$1" '$3 == u { print $6; exit }' "$LOCK"
}

# hashed_file <run dir> <body path> prints the file a row's hash was taken
# from: the extracted text when the mode writes one, else the body.
hashed_file() {
  local stem="$1/${2%.body}"
  if [ -f "$stem.txt" ]; then
    printf '%s\n' "$stem.txt"
  else
    printf '%s\n' "$1/$2"
  fi
}

# word_delta <old> <new> prints one line per changed region, words compared
# with whitespace ignored: context, then [-removed-] and {+added+}. Most
# extracted pages are one long line, so a line diff would print the page.
# When no word moved but the hash did, the change sits in line breaks or
# indentation, which modes that keep code verbatim preserve because YAML
# and shell read them. Then it prints the line diff instead, marked with a
# "# whitespace" header, capped so a reflowed page cannot flood it.
word_delta() {
  local words
  words=$(word_delta_words "$1" "$2")
  if [ -n "$words" ]; then
    printf '%s\n' "$words"
  elif ! cmp -s "$1" "$2"; then
    printf '# whitespace\n'
    # Past the cap, say so: a silent cut could hide the one indent that
    # matters, and the label carries it so the auditor opens the page.
    { diff -U2 "$1" "$2" || true; } | awk '
      NR <= 2 { next }
      ++n <= 200 { print substr($0, 1, 400); next }
      { more++ }
      END { if (more) printf "# truncated: %d more diff lines, read the page\n", more }
    '
  fi
}

word_delta_words() {
  local a b
  a=$(mktemp)
  b=$(mktemp)
  tr -s '[:space:]' '\n' <"$1" | grep . >"$a" || true
  tr -s '[:space:]' '\n' <"$2" | grep . >"$b" || true
  # diff exits 1 when the files differ; under pipefail that would abort the run.
  { diff -U8 "$a" "$b" || true; } | awk '
    function close_run() {
      if (run == "-") line = line "-]"
      else if (run == "+") line = line "+}"
      run = ""
    }
    function flush() {
      close_run()
      if (line != "") print line
      line = ""
    }
    NR <= 2 && /^(---|\+\+\+) / { next }
    /^@@/ { flush(); next }
    {
      c = substr($0, 1, 1)
      w = substr($0, 2)
      if (c == " ") {
        close_run()
        line = line (line != "" ? " " : "") w
      } else if (c == "-" || c == "+") {
        if (run != c) {
          # A replacement reads as one unit: "[-old-]{+new+}".
          sep = (line != "" && !(run == "-" && c == "+")) ? " " : ""
          close_run()
          line = line sep (c == "-" ? "[-" : "{+")
          run = c
        } else {
          line = line " "
        }
        line = line w
      }
    }
    END { flush() }
  '
  rm -f "$a" "$b"
}

# delta_vocab <target> prints the paths and keys our claims about a target
# name: backticked tokens from target-facts.sh that carry a path or key
# shape. Plain words ("model", "description") would match every page.
delta_vocab() {
  # shellcheck disable=SC2016 # literal backticks, not a command substitution
  dump_target "$1" 2>/dev/null | grep -o '`[^`]*`' | tr -d '`' | tr '<>*{}()[]", ' '\n' |
    awk '
      { sub(/[\/.:]+$/, "") }
      length($0) >= 5 && ($0 ~ /[\/._]/ || $0 ~ /^[a-z]+-[a-z-]+$/ || $0 ~ /^[a-z]+[A-Z][A-Za-z]+$/) && !seen[$0]++
    ' || true
}

# delta_label <delta> <vocab> prints one label for a delta, judged on the
# removed and added words only, never on context:
#   mentions:<terms>  a path or key we write moved (first three terms)
#   chrome-only       nothing but known page chrome moved
#   whitespace-only   no word moved; only spacing or line breaks did
#   prose             anything else
# A label ranks what an auditor reads first. It never skips a row.
delta_label() {
  if [ "$(head -n 1 "$1")" = "# whitespace" ]; then
    if grep -q '^# truncated: ' "$1"; then
      printf 'whitespace-only:truncated\n'
    else
      printf 'whitespace-only\n'
    fi
    return 0
  fi
  awk -v vocab="$2" '
    BEGIN {
      while ((getline t < vocab) > 0) if (t != "") terms[++nt] = t
      close(vocab)
      chrome[1] = "Loading image..."
      chrome[2] = "Loading diagram..."
      chrome[3] = "Ask Assistant"
      chrome[4] = "Assistant Responses are generated using AI and may contain mistakes."
      chrome[5] = "On this page"
      chrome[6] = "Copy page"
      chrome[7] = "\342\214\230 I"
      chrome[8] = "\342\214\230 K"
      nc = 8
    }
    {
      rest = $0
      while (match(rest, /\[-[^]]*-\]|\{\+[^}]*\+\}/)) {
        seg = substr(rest, RSTART + 2, RLENGTH - 4)
        rest = substr(rest, RSTART + RLENGTH)
        moved = moved " " seg
        for (i = 1; i <= nc; i++) {
          while ((p = index(seg, chrome[i])) > 0)
            seg = substr(seg, 1, p - 1) " " substr(seg, p + length(chrome[i]))
        }
        gsub(/\[\]\([^)]*\)/, " ", seg)
        gsub(/(^|[ ])K([ ]|$)/, " ", seg)
        gsub(/[ \t]+/, "", seg)
        if (seg != "") prose = 1
        segs++
      }
    }
    END {
      if (segs == 0) { print "whitespace-only"; exit }
      hit = ""
      for (i = 1; i <= nt && n < 3; i++)
        if (index(moved, terms[i]) > 0) { hit = hit (n++ ? "," : "") terms[i] }
      if (hit != "") print "mentions:" hit
      else if (prose) print "prose"
      else print "chrome-only"
    }
  ' "$1"
}

# write_deltas <run dir> writes <stem>.delta for every new or changed row
# that has a snapshot, and deltas.tsv with one labelled row each, then
# prints a one-line count by label.
write_deltas() {
  local dir="$1" target kind url status body snap hashed label vocab locked
  snapshot_store "$dir/docfetch.tsv"
  : >"$dir/deltas.tsv"
  vocab=$(mktemp -d)
  while IFS=$'\t' read -r target kind url _ _ _ _ status _ body; do
    case "$status" in new | changed) ;; *) continue ;; esac
    [ -n "$body" ] || continue
    locked=$(locked_sha "$url")
    snap=$(snapshot_file "${locked:--}")
    if [ -z "$locked" ] || [ "$locked" = "-" ] || ! snapshot_valid "$snap" "$locked"; then
      rm -f "$snap"
      printf '%s\t%s\t%s\tno-snapshot\t\n' "$target" "$kind" "$url" >>"$dir/deltas.tsv"
      continue
    fi
    [ -f "$vocab/$target" ] || delta_vocab "$target" >"$vocab/$target"
    hashed=$(hashed_file "$dir" "$body")
    word_delta "$snap" "$hashed" >"${hashed%.*}.delta"
    label=$(delta_label "${hashed%.*}.delta" "$vocab/$target")
    printf '%s\t%s\t%s\t%s\t%s\n' "$target" "$kind" "$url" "$label" "${hashed%.*}.delta" >>"$dir/deltas.tsv"
  done <"$dir/docfetch.tsv"
  rm -rf "$vocab"
  awk -F '\t' '
    { l = $4; sub(/:.*/, "", l); n[l]++; total++ }
    END {
      out = ""
      split("mentions prose chrome-only whitespace-only no-snapshot", order, " ")
      for (i = 1; i <= 5; i++) if (n[order[i]]) out = out (out != "" ? ", " : "") n[order[i]] " " order[i]
      printf "deltas: %d%s\n", total, (out != "" ? " (" out ")" : "")
    }
  ' "$dir/deltas.tsv"
}

# snapshot_store <docfetch.tsv> saves each fetched row's hashed text under
# its hash. A file named by a hash only ever holds that text, so storing
# every fetch is safe: the lock decides which one a delta reads.
snapshot_store() {
  local file="$1" dir target url code mode sha status body snap
  dir=$(dirname "$file")
  mkdir -p "$(snapshot_dir)"
  while IFS=$'\t' read -r target _ url code mode sha _ status _ body; do
    case "$target" in '#'* | '') continue ;; esac
    # An app shell or soft 404 answers 200 with no page in it: no hash, no file.
    [ "$code" = "200" ] && [ "$status" != "failed" ] && [ -n "$sha" ] && [ "$sha" != "-" ] || continue
    case "$mode" in app-shell | soft-404 | failed) continue ;; esac
    [ -n "$body" ] && [ -f "$dir/$body" ] || continue
    snap=$(snapshot_file "$sha")
    snapshot_valid "$snap" "$sha" && continue
    # Through a temp file and a rename, so a killed run never leaves half a
    # snapshot under a good name; a text that does not hash to sha is not kept.
    cp "$(hashed_file "$dir" "$body")" "$snap.tmp.$$"
    if snapshot_valid "$snap.tmp.$$" "$sha"; then
      mv "$snap.tmp.$$" "$snap"
    else
      rm -f "$snap.tmp.$$"
    fi
  done <"$file"
}

# snapshot_prune deletes snapshots the lock no longer names once they are
# older than DOCFETCH_SNAPSHOT_DAYS (30): recent ones may still be the
# base of an audit branch that has not merged yet.
snapshot_prune() {
  local sdir f sha
  sdir=$(snapshot_dir)
  [ -d "$sdir" ] && [ -r "$LOCK" ] || return 0
  find "$sdir" -type f -name '*.txt' -mtime +"${DOCFETCH_SNAPSHOT_DAYS:-30}" | while read -r f; do
    sha=$(basename "$f" .txt)
    awk -F '\t' -v s="$sha" '$6 == s { found = 1; exit } END { exit !found }' "$LOCK" || rm -f "$f"
  done
}

# lock_prune drops lock rows whose URL no registered target lists any more,
# so a moved or removed source stops counting as audited.
lock_prune() {
  local current tmp t
  [ -r "$LOCK" ] || return 0
  current=$(mktemp)
  for t in $(list_targets); do
    resolve_urls "$t" | cut -f2
  done >"$current"
  tmp=$(mktemp)
  awk -F '\t' -v cur="$current" '
    BEGIN { while ((getline u < cur) > 0) keep[u] = 1 }
    /^#/ || ($3 in keep) { print; next }
    { dropped++ }
    END { if (dropped) printf "lock: dropped %d row(s) for URLs no source lists\n", dropped > "/dev/stderr" }
  ' "$LOCK" >"$tmp"
  mv "$tmp" "$LOCK"
  rm -f "$current"
}

# json_text <body> <out> writes a JSON body key-sorted and indented, one
# value per line, so a snapshot and a delta see what json_sum hashed and a
# minified document does not diff as one word. No jq, or not JSON: no file,
# and hashed_file falls back to the body, which is what was hashed then.
json_text() {
  command -v jq >/dev/null 2>&1 && jq -S . "$1" >"$2" 2>/dev/null || rm -f "$2"
}

# row_status <url> <mode> <sha> compares one row against the committed lock.
row_status() {
  local url="$1" mode="$2" sha="$3" locked
  case "$mode" in
    failed | app-shell | soft-404)
      printf 'failed\n'
      return 0
      ;;
  esac
  [ -r "$LOCK" ] || {
    printf 'new\n'
    return 0
  }
  locked=$(awk -F '\t' -v u="$url" '$3 == u { print $6; exit }' "$LOCK")
  if [ -z "$locked" ]; then printf 'new\n'
  elif [ "$locked" = "$sha" ]; then printf 'unchanged\n'
  else printf 'changed\n'
  fi
}

# locked_date <url> prints the date the page's hash last moved.
locked_date() {
  [ -r "$LOCK" ] || return 0
  awk -F '\t' -v u="$1" '$3 == u { print $7; exit }' "$LOCK"
}

slugify() {
  printf '%s' "$1" | sed -e 's|^https://||' -e 's|^http://||' -e 's|[^A-Za-z0-9._-]|-|g' -e 's|^-*||' -e 's|-*$||' |
    cut -c1-80
}

# fetch_one <target> <kind> <url> <dir> <index> prints one run row.
fetch_one() {
  local target="$1" kind="$2" url="$3" dir="$4" idx="$5" force_proxy="${6:-}"
  local fetch_url mode rewrite body ctype code final text_len refresh alt host stem
  rewrite=$(github_rewrite "$url")
  fetch_url=${rewrite%%	*}
  mode=${rewrite##*	}

  stem="$dir/pages/$target/$kind-$idx-$(slugify "$url")"
  mkdir -p "$dir/pages/$target"
  body="$stem.body"
  # A rerun into the same run directory must not leave an earlier fetch's
  # extracted text or delta behind: hashed_file trusts that .txt exists
  # only when this fetch's mode wrote it.
  rm -f "$stem.txt" "$stem.delta"

  local result code_line
  if [ -n "$force_proxy" ]; then
    code_line=$(proxy_curl "$fetch_url" "$body")
    mode=reader-proxy
  else
    code_line=$(docfetch_curl "$fetch_url" "$body")
  fi
  code=$(printf '%s' "$code_line" | cut -f1)
  final=$(printf '%s' "$code_line" | cut -f2)
  ctype=$(printf '%s' "$code_line" | cut -f3)

  case "$force_proxy:$code" in
    :403 | :429 | :000 | :5??)
      code_line=$(proxy_curl "$fetch_url" "$body")
      code=$(printf '%s' "$code_line" | cut -f1)
      final=$(printf '%s' "$code_line" | cut -f2)
      ctype=$(printf '%s' "$code_line" | cut -f3)
      [ "$code" = "200" ] && mode=reader-proxy
      ;;
  esac

  if [ "$code" != "200" ]; then
    printf '%s\t%s\t%s\t%s\t%s\t-\t%s\tfailed\t%s\t\n' \
      "$target" "$kind" "$url" "$code" "${mode:-failed}" "$(date -u +%Y-%m-%d)" "$final"
    return 0
  fi

  if [ -z "$mode" ]; then
    if head -c 4096 "$body" | grep -qi 'http-equiv="refresh"' && [ "$(wc -c <"$body")" -lt 4096 ]; then
      if refresh=$(meta_refresh_target "$body" "$final"); then
        code_line=$(docfetch_curl "$refresh" "$body")
        code=$(printf '%s' "$code_line" | cut -f1)
        final=$refresh
        mode=meta-refresh
      fi
    fi
  fi

  if [ -z "$mode" ]; then
    case "$ctype" in
      *json*) mode=json ;;
      *markdown* | *plain*) mode=text ;;
    esac
    case "$url" in *.md) mode=markdown-mirror ;; esac
  fi

  if [ -z "$mode" ]; then
    text_len=$(strip_html "$body" | wc -c | tr -d ' ')
    # A documentation page under 500 bytes of visible text is a shell, not a page.
    if [ "$text_len" -lt 500 ]; then
      alt="${fetch_url%/}.md"
      if [ "$(docfetch_curl "$alt" "$stem.md" | cut -f1)" = "200" ] &&
        ! head -c 200 "$stem.md" | grep -qi '<html\|<!doctype'; then
        mv "$stem.md" "$body"
        mode=markdown-mirror
        final=$alt
      else
        rm -f "$stem.md"
        host=$(url_origin "$fetch_url")
        alt=""
        if [ "$(docfetch_curl "$host/llms.txt" "$stem.llms" | cut -f1)" = "200" ]; then
          alt=$(llms_entry "$stem.llms" "$(basename "${fetch_url%/}")")
        fi
        rm -f "$stem.llms"
        if [ -n "$alt" ] && [ "$(docfetch_curl "$alt" "$stem.llms" | cut -f1)" = "200" ]; then
          mode=llms-txt
          final=$alt
          mv "$stem.llms" "$body"
        else
          rm -f "$stem.llms"
          if [ "$(router_payload "$body" "$stem.json")" = "ok" ]; then
            mv "$stem.json" "$body"
            mode=router-data
          else
            mode=app-shell
          fi
        fi
      fi
    fi
  fi

  if [ -z "$mode" ]; then
    if strip_html "$body" | head -c 400 | grep -qi '404\|not found'; then
      mode=soft-404
    else
      mode=html
    fi
  fi

  case "$mode" in
    html | meta-refresh)
      strip_html "$body" >"$stem.txt"
      result=$(sha256_of "$stem.txt")
      ;;
    github-api)
      # The API answers on one line, so match values rather than lines: a
      # line filter keeps the whole payload and hashes its download counts.
      grep -oE '"(tag_name|published_at)":"[^"]*"' "$body" >"$stem.txt" || true
      result=$(sha256_of "$stem.txt")
      ;;
    json)
      result=$(json_sum "$body")
      json_text "$body" "$stem.txt"
      ;;
    router-data)
      if delta_text "$body" >"$stem.txt" && [ -s "$stem.txt" ]; then
        result=$(sha256_of "$stem.txt")
      else
        rm -f "$stem.txt"
        result=$(json_sum "$body")
        json_text "$body" "$stem.txt"
      fi
      ;;
    reader-proxy)
      reader_text "$body" >"$stem.txt"
      result=$(sha256_of "$stem.txt")
      ;;
    app-shell | soft-404)
      result="-"
      ;;
    *)
      result=$(sha256_of "$body")
      ;;
  esac

  local status changed
  status=$(row_status "$url" "$mode" "$result")
  if [ "$status" = "unchanged" ]; then
    changed=$(locked_date "$url")
  else
    changed=$(date -u +%Y-%m-%d)
  fi

  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$target" "$kind" "$url" "$code" "$mode" "$result" "${changed:-$(date -u +%Y-%m-%d)}" \
    "$status" "$final" "${body#"$dir"/}"
}

# mirror_url <url> prints the Markdown copy many docs hosts serve next to
# a page: the path without its query, fragment, or trailing slash, plus .md.
mirror_url() {
  local u="${1%%#*}"
  u="${u%%\?*}"
  printf '%s.md\n' "${u%/}"
}

# mirror_one <target> <kind> <url> <dir> <index> <mode> fetches the Markdown
# copy of a scraped page and prints "<target> <kind> <url> <mirror url>
# <http> <sha>", sha "-" when no usable copy answers. It prints nothing for
# rows that are not scraped HTML. This is the measurement for #1127: does
# the copy move less often than the scraped page? It never feeds the lock.
mirror_one() {
  local target="$1" kind="$2" url="$3" dir="$4" idx="$5" mode="$6" murl stem code sha="-"
  case "$mode" in html | meta-refresh | reader-proxy) ;; *) return 0 ;; esac
  murl=$(mirror_url "$url")
  stem="$dir/pages/$target/$kind-$idx-$(slugify "$url")"
  mkdir -p "$dir/pages/$target"
  code=$(docfetch_curl "$murl" "$stem.mirror.md" | cut -f1)
  if [ "$code" = "200" ] && [ "$(wc -c <"$stem.mirror.md")" -ge 200 ] &&
    ! head -c 300 "$stem.mirror.md" | grep -qi '<html\|<!doctype'; then
    reader_text "$stem.mirror.md" >"$stem.mirror.txt"
    sha=$(sha256_of "$stem.mirror.txt")
  else
    rm -f "$stem.mirror.md"
  fi
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$target" "$kind" "$url" "$murl" "$code" "$sha"
}

# compare_mirrors <run a> <run b> prints, per host and in total, how many
# mirrored rows each representation saw move between two runs:
# "<host> <rows> <scraped moved> <mirror moved>". Only rows with a usable
# copy in both runs count. Feed it consecutive daily vendor-watch artifacts.
compare_mirrors() {
  # Files are told apart by position, not name: the same run given twice
  # is a valid (all-zero) comparison.
  awk -F '\t' '
    FNR == 1 { f++ }
    f == 1 { ha[$3] = $6; next }
    f == 2 { if ($6 != "-") ma[$3] = $6; next }
    f == 3 { hb[$3] = $6; next }
    f == 4 {
      if ($6 == "-" || !($3 in ma) || !($3 in ha) || !($3 in hb)) next
      host = $3
      sub(/^https?:\/\//, "", host)
      sub(/\/.*$/, "", host)
      rows[host]++; total++
      if (ha[$3] != hb[$3]) { hm[host]++; th++ }
      if (ma[$3] != $6) { mm[host]++; tm++ }
    }
    END {
      print "host\trows\tscraped_moved\tmirror_moved"
      for (h in rows) printf "%s\t%d\t%d\t%d\n", h, rows[h], hm[h], mm[h] | "sort"
      close("sort")
      printf "total\t%d\t%d\t%d\n", total, th, tm
    }
  ' "$1/docfetch.tsv" "$1/mirrors.tsv" "$2/docfetch.tsv" "$2/mirrors.tsv"
}

# fetch_target <target> <dir> fetches one target's URLs. A "- fetch:
# reader-proxy" line in its source section sends every URL through the
# proxy: a host that blocks some networks (kiro.dev, cursor.com) otherwise
# serves HTML to one machine and a proxy copy to another, and the two
# representations never hash the same.
fetch_target() {
  local target="$1" dir="$2" idx=0 kind url proxy="" row
  if source_sections "$target" | grep -q '^- fetch: reader-proxy'; then
    proxy=1
  fi
  mkdir -p "$dir/rows"
  # Mirror rows append per URL; a rerun into the same run dir starts clean.
  rm -f "$dir/rows/$target.mirrors"
  export DOCFETCH_RETRY_STATE="$dir/rows/.retry-$target"
  rm -f "$DOCFETCH_RETRY_STATE"
  while IFS=$'\t' read -r kind url; do
    [ -n "$url" ] || continue
    idx=$((idx + 1))
    row=$(fetch_one "$target" "$kind" "$url" "$dir" "$idx" "$proxy")
    printf '%s\n' "$row"
    if [ -n "${DOCFETCH_MIRRORS:-}" ]; then
      mirror_one "$target" "$kind" "$url" "$dir" "$idx" "$(printf '%s' "$row" | cut -f5)" >>"$dir/rows/$target.mirrors"
    fi
  done < <(resolve_urls "$target")
}

# lock_merge <docfetch.tsv> [target...] rewrites the lock from a finished run.
# It runs after the report, never before: a crash mid-run must not mark pages
# as seen that no auditor read.
lock_merge() {
  local file="$1"
  shift
  if [ ! -r "$file" ]; then
    echo "cannot read $file" >&2
    return 1
  fi
  mkdir -p "$(dirname "$LOCK")"
  local tmp
  tmp=$(mktemp)
  {
    echo "# target-audit sources lock. Written by scripts/docfetch.sh --update. Do not hand-edit rows."
    printf '# target\tkind\turl\thttp\tmode\tsha256\tchanged\n'
    awk -F '\t' -v targets="$*" -v lock="$LOCK" '
      BEGIN {
        count = split(targets, names, " ")
        for (i = 1; i <= count; i++) selected[names[i]] = 1
        while ((getline line < lock) > 0) {
          if (substr(line, 1, 1) == "#") continue
          split(line, f, "\t")
          if (f[3] == "") continue
          keep[f[3]] = f[1] "\t" f[2] "\t" f[3] "\t" f[4] "\t" f[5] "\t" f[6] "\t" f[7]
          owner[f[3]] = f[1]
        }
        close(lock)
      }
      /^#/ || NF < 7 { next }
      count > 0 && !($1 in selected) { next }
      { keep[$3] = $1 "\t" $2 "\t" $3 "\t" $4 "\t" $5 "\t" $6 "\t" $7; owner[$3] = $1 }
      END { for (u in keep) print keep[u] }
    ' "$file" | sort -t"$(printf '\t')" -k1,1 -k2,2 -k3,3
  } >"$tmp"
  mv "$tmp" "$LOCK"
  snapshot_store "$file"
}

docfetch_main() {
  local out="" mode=fetch
  while [ "$#" -gt 0 ]; do
    case "$1" in
      -h | --help)
        docfetch_usage
        return 0
        ;;
      --urls)
        mode=urls
        shift
        ;;
      --update)
        mode=update
        shift
        ;;
      --mirrors)
        export DOCFETCH_MIRRORS=1
        shift
        ;;
      --compare-mirrors)
        mode=compare
        shift
        ;;
      --out)
        if [ -z "${2:-}" ]; then
          echo "--out needs a directory" >&2
          return 2
        fi
        out="$2"
        shift 2
        ;;
      *) break ;;
    esac
  done

  case "$mode" in
    urls)
      if [ "$#" -eq 0 ]; then
        echo "--urls needs at least one target" >&2
        return 2
      fi
      local t
      for t in "$@"; do
        resolve_urls "$t" | awk -v t="$t" -F '\t' '{ print t "\t" $1 "\t" $2 }'
      done
      return 0
      ;;
    compare)
      if [ -z "${1:-}" ] || [ -z "${2:-}" ]; then
        echo "--compare-mirrors needs two run directories" >&2
        return 2
      fi
      local d
      for d in "$1" "$2"; do
        if [ ! -r "$d/docfetch.tsv" ] || [ ! -r "$d/mirrors.tsv" ]; then
          echo "no docfetch.tsv and mirrors.tsv in $d (fetch with --mirrors)" >&2
          return 1
        fi
      done
      compare_mirrors "$1" "$2"
      return 0
      ;;
    update)
      if [ -z "${1:-}" ]; then
        echo "--update needs a docfetch.tsv path" >&2
        return 2
      fi
      local file="$1"
      shift
      lock_merge "$file" "$@" || return
      lock_prune
      snapshot_prune
      return
      ;;
  esac

  local targets
  if [ "$#" -eq 0 ]; then
    targets=$(list_targets)
  else
    targets="$*"
  fi
  [ -n "$out" ] || out="$ROOT/local/target-audit/$(date -u +%Y-%m-%d)-run"
  mkdir -p "$out/rows"

  # Wait on each worker by PID: a bare `wait` returns 0 even when one
  # failed, and the run would then publish a docfetch.tsv missing a target.
  # A worker that exits 0 with fewer rows than its URLs is incomplete too.
  local t pids=() failed=""
  for t in $targets; do
    fetch_target "$t" "$out" >"$out/rows/$t.tsv" &
    pids+=("$!")
  done
  local i=0
  for t in $targets; do
    if ! wait "${pids[$i]}"; then
      failed="$failed $t"
    elif [ "$(grep -c . <"$out/rows/$t.tsv")" -lt "$(resolve_urls "$t" | grep -c .)" ]; then
      failed="$failed $t"
    fi
    i=$((i + 1))
  done
  if [ -n "$failed" ]; then
    echo "docfetch: incomplete run for:$failed" >&2
    return 1
  fi

  cat "$out/rows/"*.tsv >"$out/docfetch.tsv"
  sort -t"$(printf '\t')" -k8,8 -k1,1 "$out/docfetch.tsv" |
    awk -F '\t' '{ print $8 "\t" $1 "\t" $2 "\t" $5 "\t" $4 "\t" $3 }'
  write_deltas "$out"
  if [ -n "${DOCFETCH_MIRRORS:-}" ]; then
    cat "$out/rows/"*.mirrors >"$out/mirrors.tsv" 2>/dev/null || : >"$out/mirrors.tsv"
    awk -F '\t' '{ n++; if ($6 != "-") ok++ } END { printf "mirrors: %d of %d scraped rows serve a Markdown copy\n", ok, n }' "$out/mirrors.tsv"
  fi
  awk -F '\t' '
    { n[$8]++; total++; t[$1] = 1 }
    END {
      count = 0
      for (k in t) count++
      printf "%d targets, %d urls: %d new, %d changed, %d unchanged, %d failed\n",
        count, total, n["new"], n["changed"], n["unchanged"], n["failed"]
    }
  ' "$out/docfetch.tsv"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  docfetch_main "$@"
fi
