#!/usr/bin/env bash
#
# jev-triage.sh - rank a target-audit run's changed vendor text against our
# claims, so auditors read the likeliest drift first.
#
# Claims come from scripts/target-facts.sh, split into single checkable
# statements: output paths, supported kinds, adapter package doc paragraphs,
# and target-behavior.md lines. Evidence is the .delta that
# scripts/docfetch.sh wrote for each new or changed row in the run's
# deltas.tsv, one page at a time.
#
# With TYPESAFE_API_KEY set, each page is asked about every claim of its
# target through TypeSafe's Jev: one Choice per claim (supports,
# contradicts, says_nothing), in chunks of JEV_CLAIMS_PER_REQUEST, plus one
# Noul per page: does it change a project-scoped configuration surface? The
# Noul catches a new native surface, which no existing claim can contradict. Without a key, with --lexical, or once the API fails, the script
# still writes the pages' lexical leads: claims that share a path, key, or
# term with the page's moved words.
#
# Output: <run dir>/triage.tsv, one row per lead, likeliest drift first:
#   target, url, delta path, claim, verdict, probability, p_contradicts, via
# verdict is contradicts, supports, says_nothing, noul, or paired (a lexical
# lead with no judgment); via is jev or lexical. A Choice answer is kept only
# when it is contradicts or its p_contradicts reaches JEV_KEEP_CONTRADICTS.
# On the replay cases, with every claim of a target in one request, real
# drift scored p_contradicts 0.63 or more and everything else 0.16 or less.
#
# The triage only orders reading. It never clears a row, and it always
# exits 0, so an audit with no key or no network runs as it would without it.
#
# Usage:
#   scripts/jev-triage.sh [--lexical] <run dir> [target...]
#   scripts/jev-triage.sh [--lexical] --replay <cases.tsv>
#   scripts/jev-triage.sh --replay-surface <surface-cases.tsv>
#
# Environment:
#   TYPESAFE_API_KEY        enables Jev; sent through a curl config on stdin
#   TYPESAFE_API_URL        endpoint override, for tests
#   JEV_MODEL               default jev-latest
#   JEV_TIMEOUT             seconds per request, default 60
#   JEV_JOBS                concurrent requests, default 4
#   JEV_MAX_REQUESTS        request cap per run, default 40
#   JEV_CLAIMS_PER_REQUEST  Choice questions per request, default 40
#   JEV_PAGE_CHARS          delta characters per request, default 8000
#   JEV_LEXICAL_LEADS       lexical leads kept per page, default 4
#   JEV_KEEP_CONTRADICTS    lowest p_contradicts kept as a lead, default 0.4
#   JEV_SURFACE_MIN         lowest Noul that promotes a surface row, default 0.5
#   JEV_CLEAR_SURFACE       Noul a page must stay under to be clear, default 0.1
#   JEV_TRIES               attempts per request on 429, 5xx, or no answer, default 3
#   JEV_BACKOFF             seconds times attempt between retries, default 2
#   JEV_TARGET_FACTS        claim source, default scripts/target-facts.sh
#
# Portable: bash 3.2 + awk + curl + jq. No GNU-only flags.

set -euo pipefail

JEV_ROOT=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
JEV_FACTS="${JEV_TARGET_FACTS:-$JEV_ROOT/scripts/target-facts.sh}"
JEV_API_URL="${TYPESAFE_API_URL:-https://api.typesafe.ai/v1/systemone}"
JEV_MODEL="${JEV_MODEL:-jev-latest}"
JEV_TIMEOUT="${JEV_TIMEOUT:-60}"
JEV_JOBS="${JEV_JOBS:-4}"
JEV_MAX_REQUESTS="${JEV_MAX_REQUESTS:-40}"
JEV_CLAIMS_PER_REQUEST="${JEV_CLAIMS_PER_REQUEST:-40}"
JEV_PAGE_CHARS="${JEV_PAGE_CHARS:-8000}"
JEV_LEXICAL_LEADS="${JEV_LEXICAL_LEADS:-4}"
JEV_KEEP_CONTRADICTS="${JEV_KEEP_CONTRADICTS:-0.4}"
JEV_SURFACE_MIN="${JEV_SURFACE_MIN:-0.5}"
JEV_CLEAR_SURFACE="${JEV_CLEAR_SURFACE:-0.1}"
JEV_TRIES="${JEV_TRIES:-3}"
JEV_BACKOFF="${JEV_BACKOFF:-2}"
JEV_LEXICAL=0
JEV_SURFACE_CLAIM="changes a project-scoped config surface (file path, frontmatter field, MCP key, hook event, spec directory)"
TAB=$(printf '\t')

jev_usage() {
  cat <<'EOF'
Usage: scripts/jev-triage.sh [--lexical] <run dir> [<target>...]
       scripts/jev-triage.sh [--lexical] --replay <cases.tsv>
       scripts/jev-triage.sh --replay-surface <surface-cases.tsv>

  <run dir>      a scripts/docfetch.sh run directory holding deltas.tsv;
                 writes <run dir>/triage.tsv
  <target>...    only triage the named targets
  --lexical      no API calls: write lexical leads only
  --replay       run labeled cases and print recall on contradicts cases,
                 the false-positive rate on says_nothing cases, and how
                 many contradicts cases the lexical pairing alone finds
  --replay-surface
                 run labeled surface cases and print recall on yes cases
                 and the false-positive rate on no cases for the Noul
EOF
}

jev_note() {
  printf 'jev-triage: %s\n' "$1" >&2
}

# jev_ready prints why Jev cannot run and returns 1, or returns 0.
jev_ready() {
  if [ "$JEV_LEXICAL" = 1 ]; then
    printf -- '--lexical'
    return 1
  fi
  if [ -z "${TYPESAFE_API_KEY:-}" ]; then
    printf 'no TYPESAFE_API_KEY'
    return 1
  fi
  local tool
  for tool in curl jq; do
    if ! command -v "$tool" >/dev/null 2>&1; then
      printf '%s not found' "$tool"
      return 1
    fi
  done
}

# jev_claims <target> prints one checkable claim per line from the facts dump.
jev_claims() {
  "$JEV_FACTS" "$1" 2>/dev/null | awk -v t="$1" '
    function words(s,   out, i, c, prev, next_c) {
      out = ""
      for (i = 1; i <= length(s); i++) {
        c = substr(s, i, 1); prev = substr(s, i - 1, 1); next_c = substr(s, i + 1, 1)
        if (i > 1 && c ~ /[A-Z]/ && (prev ~ /[a-z]/ || (prev ~ /[A-Z]/ && next_c ~ /[a-z]/))) out = out " "
        out = out c
      }
      n = split(out, w, " ")
      out = ""
      for (i = 1; i <= n; i++) {
        if (w[i] ~ /^[A-Z][a-z]+$/) w[i] = tolower(w[i])
        if (w[i] == "dir") w[i] = "directory"
        out = out (i > 1 ? " " : "") w[i]
      }
      return out
    }
    function emit(s) {
      gsub(/[ \t]+/, " ", s)
      sub(/^ /, "", s)
      sub(/ $/, "", s)
      if (length(s) < 12) return
      if (length(s) > 1200) s = substr(s, 1, 1200) " ..."
      print s
    }
    function flush() { if (para != "") emit(para); para = "" }
    /^--- / { flush(); sec = $0; next }
    sec ~ /declared capabilities/ && /Supports:/ {
      line = $0
      sub(/^[^{]*\{/, "", line)
      sub(/\}.*$/, "", line)
      n = split(line, kinds, /, */)
      for (i = 1; i <= n; i++) {
        if (kinds[i] == "") continue
        k = tolower(kinds[i])
        emit(t " natively reads project-scoped " k " configuration, so agnostic-ai emits " k " specs for it.")
      }
      next
    }
    sec ~ /default output paths/ && /^default[A-Za-z]*[ \t]*=[ \t]*"/ {
      label = $1
      sub(/^default/, "", label)
      path = $0
      sub(/^[^"]*"/, "", path)
      sub(/".*$/, "", path)
      emit(t " reads its " words(label) " from `" path "` in the project, where agnostic-ai writes it.")
      next
    }
    sec ~ /adapter package doc/ {
      if ($0 ~ /^\/\/[ \t]*$/) { flush(); next }
      if ($0 ~ /^\/\//) {
        l = $0
        sub(/^\/\/ ?/, "", l)
        para = para (para != "" ? " " : "") l
      }
      next
    }
    sec ~ /target-behavior\.md lines/ && /^[0-9]+:/ {
      l = $0
      sub(/^[0-9]+:/, "", l)
      emit(l)
      next
    }
    END { flush() }
  ' || true
}

# jev_pair <claims file> <text file> <target> prints "<claim line>\t<score>"
# for each claim that shares a path, key, or term with the text's moved
# words, best first. A path or key hit weighs 3, a plain word 1, and a
# claim needs 3.
jev_pair() {
  awk -v claims="$1" -v t="$3" '
    BEGIN {
      split("about above after agent agents also always another because been before being both cannot could does done each either every file files first from have here into just like made make many more most much must need needs only other over same should since some such than that their them then there these they this those through under used uses using very were what when where which while will with within without would your agnostic-ai emits emitted writes written reads read project tool tools", sw, " ")
      for (i in sw) stop[sw[i]] = 1
      stop[tolower(t)] = 1
    }
    function tok(raw, out, key, seg,   w) {
      sub(/^[.\/-]+/, "", raw)
      sub(/[.\/:-]+$/, "", raw)
      if (length(raw) < 4) return
      w = tolower(raw)
      if (w in stop) return
      if (w ~ /^[a-z]+s$/ && w !~ /ss$/ && length(w) >= 5) w = substr(w, 1, length(w) - 1)
      out[w] = 1
      if (seg || raw ~ /[\/._]/ || raw ~ /[a-z][A-Z]/ || raw ~ /^[a-z]+-[a-z-]+$/) key[w] = 1
    }
    # A path also counts by its segments, so a moved file still meets its
    # old path: .github/copilot/mcp.json shares mcp.json with .github/mcp.json.
    function toks(s, out, key,   n, i, j, m, parts, segs) {
      n = split(s, parts, /[^A-Za-z0-9_.\/-]+/)
      for (i = 1; i <= n; i++) {
        tok(parts[i], out, key, 0)
        if (index(parts[i], "/") == 0) continue
        m = split(parts[i], segs, "/")
        for (j = 1; j <= m; j++) tok(segs[j], out, key, 1)
      }
    }
    { region = region (NR > 1 ? "\n" : "") $0 }
    END {
      moved = ""
      if (substr(region, 1, 12) == "# whitespace") {
        moved = region
      } else {
        s = region
        while (match(s, /\[-|\{\+/)) {
          closer = (substr(s, RSTART, 2) == "[-") ? "-]" : "+}"
          s = substr(s, RSTART + 2)
          p = index(s, closer)
          if (p == 0) { moved = moved " " s; break }
          moved = moved " " substr(s, 1, p - 1)
          s = substr(s, p + 2)
        }
      }
      toks(moved, mv, mk)
      toks(region, cx, ck)
      idx = 0
      while ((getline c < claims) > 0) {
        idx++
        delete cw
        delete ckey
        toks(c, cw, ckey)
        score = 0
        for (w in cw) {
          if (w in mv) score += ((w in ckey) || (w in mk)) ? 3 : 1
          else if ((w in cx) && (w in ckey)) score += 1
        }
        if (score >= 3) printf "%d\t%d\n", idx, score
      }
    }
  ' "$2" | sort -t "$TAB" -k2,2nr -k1,1n
}

# jev_request <text file> <selected claims tsv> <target> <source> <noul>
# prints one request body: the claims live in state and each Choice names
# its claim by path, so a request carries every claim's text once. Question
# IDs never reach the model, so each instruction names the fields it reads.
jev_request() {
  jq -n \
    --arg model "$JEV_MODEL" \
    --arg target "$3" \
    --arg source "$4" \
    --argjson noul "$5" \
    --rawfile change "$1" \
    --rawfile claims "$2" '
    ($claims | split("\n") | map(select(length > 0) | split("\t"))
      | map({ key: ("c" + .[0]), value: (.[1:] | join("\t")) })) as $c
    | {
      model: $model,
      state: {
        target: $target,
        source: $source,
        notation: "In `vendor_change`, words inside [-...-] were removed, words inside {+...+} were added, and the other words are unchanged context. Read the text as it stands after the change.",
        vendor_change: $change,
        claims: ($c | from_entries)
      },
      questions: (
        ($c | map({
          key: .key,
          value: {
            type: "choice",
            instructions: ("`claims." + .key + "` is a statement the agnostic-ai project makes about how the tool named in `target` reads project configuration. `vendor_change` is a region of that tool'"'"'s documentation (`source`) that changed since the last audit, written in the markup `notation` describes. How does the text after the change relate to `claims." + .key + "`?"),
            criteria: {
              supports: "After the change, the text states the claim or directly implies that it is true",
              contradicts: "After the change, the text states the opposite of the claim or implies it is false, for example by removing, renaming, replacing, or marking legacy a path, key, field, or event the claim names",
              says_nothing: "The text does not address what the claim asserts, either way"
            }
          }
        }) | from_entries)
        + (if $noul then {
          surface: {
            type: "noul",
            instructions: "`vendor_change` is a region of the documentation or changelog of the tool named in `target` (`source`) that changed since the last audit, written in the markup `notation` describes. Does the added or changed text document a project-scoped configuration surface of that tool: something a team commits to its repository to configure the tool, such as a file or directory the tool reads, a frontmatter or settings field, an MCP configuration key, a hook event, or a directory the tool loads skills, rules, agents, or commands from?",
            criteria: {
              "true": "The changed text describes a repository file, directory, frontmatter field, settings key, MCP key, or hook event that configures the tool for a project, including a new field or value of an existing file",
              "false": "The changed text is about UI, CLI commands, models, pricing, packaging, performance, analytics, settings that live only in the user home directory, or fixes that leave project configuration as it was"
            }
          }
        } else {} end)
      )
    }'
}

# jev_post <request> <response> <fail file> <fatal file> sends one request,
# retrying 429, 5xx, and no answer with a linear backoff. A failure that
# will hit every request (no response, 401, 403, retries spent) also writes
# <fatal file>, so no further request is sent; a 4xx for this body alone
# only fails this request.
jev_post() {
  local req="$1" out="$2" fail="$3" fatal="$4" try=1 code key
  # Escape for a quoted curl config value; the key never reaches argv.
  key=$(printf '%s' "$TYPESAFE_API_KEY" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')
  while :; do
    code=$(printf 'header = "Authorization: Bearer %s"\n' "$key" |
      curl -sS --config - --connect-timeout 10 --max-time "$JEV_TIMEOUT" \
        -H 'Content-Type: application/json' --data-binary "@$req" \
        -o "$out" -w '%{http_code}' "$JEV_API_URL" 2>/dev/null) || code=000
    case "$code" in
      2??)
        if jq -e '.answers | type == "object"' "$out" >/dev/null 2>&1; then
          return 0
        fi
        printf 'HTTP %s with no answers' "$code" >"$fail"
        ;;
      429 | 5?? | 000)
        if [ "$try" -lt "$JEV_TRIES" ]; then
          sleep $((JEV_BACKOFF * try))
          try=$((try + 1))
          continue
        fi
        if [ "$code" = 000 ]; then
          printf 'no response from %s after %d tries' "$JEV_API_URL" "$try" >"$fail"
        else
          printf 'HTTP %s after %d tries' "$code" "$try" >"$fail"
        fi
        cp "$fail" "$fatal"
        ;;
      401 | 403)
        printf 'HTTP %s' "$code" >"$fail"
        cp "$fail" "$fatal"
        ;;
      *)
        printf 'HTTP %s' "$code" >"$fail"
        ;;
    esac
    rm -f "$out"
    return 1
  done
}

# jev_run <work dir> posts the requests under <work>/req in name order, at
# most JEV_JOBS at a time, and sends no further batch after a fatal failure.
# Bash 3.2 has no `wait -n`, so it waits on each batch by PID.
jev_run() {
  local work="$1" req n pids="" count=0 pid
  mkdir -p "$work/resp" "$work/fail"
  for req in "$work"/req/*.json; do
    [ -e "$req" ] || continue
    [ -e "$work/fatal" ] && break
    n=$(basename "$req" .json)
    jev_post "$req" "$work/resp/$n.json" "$work/fail/$n" "$work/fatal" &
    pids="$pids $!"
    count=$((count + 1))
    if [ "$count" -ge "$JEV_JOBS" ]; then
      for pid in $pids; do wait "$pid" || true; done
      pids=""
      count=0
    fi
  done
  for pid in $pids; do wait "$pid" || true; done
}

# jev_units <run dir> <work dir> [target...] cuts each changed page's delta
# into texts of at most JEV_PAGE_CHARS at line boundaries and prints
# "prio\tunit\ttarget\tkind\turl\tdelta\tfile\tlabel", mentions first, then
# changelog, prose, whitespace, chrome.
jev_units() {
  local dir="$1" work="$2" target kind url label delta prio page=0 part
  shift 2
  mkdir -p "$work/units"
  while IFS="$TAB" read -r target kind url label delta; do
    [ -n "$delta" ] && [ -f "$delta" ] || continue
    if [ "$#" -gt 0 ]; then
      case " $* " in *" $target "*) ;; *) continue ;; esac
    fi
    case "$label" in
      mentions:*) prio=1 ;;
      prose) prio=3 ;;
      whitespace-only*) prio=4 ;;
      *) prio=5 ;;
    esac
    case "$kind" in changelog | schema | code) [ "$prio" -gt 1 ] && prio=2 ;; esac
    page=$((page + 1))
    awk -v max="$JEV_PAGE_CHARS" -v base="$work/units/$page" '
      function out() { return sprintf("%s.%03d.txt", base, k) }
      BEGIN { k = 1; size = 0 }
      NF == 0 { next }
      {
        line = substr($0, 1, max)
        if (size > 0 && size + length(line) + 1 > max) { close(out()); k++; size = 0 }
        print line > out()
        size += length(line) + 1
      }
    ' "$delta"
    for part in "$work/units/$page".*.txt; do
      [ -e "$part" ] || continue
      printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$prio" "$(basename "$part" .txt)" "$target" "$kind" "$url" "$delta" "$part" "$label"
    done
  done <"$dir/deltas.tsv" | sort -t "$TAB" -k1,1n -k2,2n
}

# jev_answers <response> <meta> prints the rows of one answered request,
# with the claim line as a leading column.
jev_answers() {
  local target url delta sel unit
  IFS="$TAB" read -r target url delta sel unit <"$2"
  jq -r --arg target "$target" --arg url "$url" --arg delta "$delta" \
    --arg surface "$JEV_SURFACE_CLAIM" --rawfile claims "$sel" '
    def p2: . * 100 | round / 100 | tostring;
    ($claims | split("\n") | map(select(length > 0) | split("\t"))
      | map({ key: ("c" + .[0]), value: { line: .[0], text: (.[1:] | join("\t")) } }) | from_entries) as $c
    | .answers | to_entries[]
    | if .value.type == "noul" then
        ["0", $target, $url, $delta, $surface, "noul", (.value.noul | p2), "", "jev"]
      elif $c[.key] then
        [$c[.key].line, $target, $url, $delta, $c[.key].text, .value.choice,
          (.value.probabilities[.value.choice] // 0 | p2),
          (.value.probabilities.contradicts // 0 | p2), "jev"]
      else empty end
    | @tsv' "$1"
}

# jev_finish <rows> <out> keeps Choice answers that are contradicts or reach
# JEV_KEEP_CONTRADICTS and Noul answers that reach JEV_SURFACE_MIN, keeps the
# strongest row per page and claim, and orders Jev leads by p_contradicts,
# then surface rows, then lexical leads.
jev_finish() {
  awk -F '\t' -v keep="$JEV_KEEP_CONTRADICTS" -v surface="$JEV_SURFACE_MIN" '
    ($5 == "supports" || $5 == "says_nothing") && $7 + 0 < keep + 0 { next }
    $5 == "noul" && $6 + 0 < surface + 0 { next }
    {
      r = 1
      if ($5 == "noul") r = 2
      else if ($5 == "paired") r = 3
      k = $3 "\t" $4
      w = ($5 == "noul") ? $6 : $7
      if (k in rank && (rank[k] < r || (rank[k] == r && weight[k] + 0 >= w + 0))) next
      rank[k] = r; weight[k] = w; row[k] = $0
    }
    END { for (k in row) printf "%d\t%s\t%s\n", rank[k], weight[k], row[k] }
  ' "$1" | sort -t "$TAB" -k1,1n -k2,2nr -k3,3 | cut -f3- >"$2.tmp"
  mv "$2.tmp" "$2"
}

# jev_pages <work dir> <triage.tsv> <jev ran> prints one row per changed
# page: target, url, delta path, label, status. clear means Jev answered
# every question for the page, kept no row for it, its Noul stayed under
# JEV_CLEAR_SURFACE, and no path we write moved on it (label mentions:).
# lead means Jev kept a row, judged means no lead but not clear, unjudged
# means a request for the page was capped or failed, and lexical means Jev
# did not run. On the surface replay cases every real surface scored a Noul
# of 0.16 or more, so the clearing cut sits well under the 0.5 lead cut.
jev_pages() {
  local work="$1" out="$2" ran="$3" unit
  : >"$work/unjudged.list"
  for unit in "$work"/unjudged.*; do
    [ -e "$unit" ] || continue
    case "$unit" in *.list) continue ;; esac
    printf '%s\n' "${unit##*/unjudged.}" >>"$work/unjudged.list"
  done
  [ -s "$work/units.tsv" ] || return 0
  { cat "$work"/answered.* 2>/dev/null || true; } | awk -F '\t' '$6 == "noul" { print $4 "\t" $7 }' >"$work/nouls"
  awk -F '\t' -v ran="$ran" -v leads="$out" -v unj="$work/unjudged.list" -v nouls="$work/nouls" -v clearcut="$JEV_CLEAR_SURFACE" '
    BEGIN {
      while ((getline line < leads) > 0) { split(line, f, "\t"); if (f[8] == "jev") lead[f[3]] = 1 }
      while ((getline line < unj) > 0) bad[line] = 1
      while ((getline line < nouls) > 0) { split(line, f, "\t"); if (f[2] + 0 >= clearcut + 0) surface[f[1]] = 1 }
    }
    {
      unit = $1; target = $2; url = $3; delta = $4; label = $5
      if (!(delta in seen)) { seen[delta] = 1; order[++n] = delta; info[delta] = target "\t" url "\t" delta "\t" label }
      if (unit in bad) unjudged[delta] = 1
      if (label ~ /^mentions:/) mentions[delta] = 1
    }
    END {
      for (i = 1; i <= n; i++) {
        d = order[i]
        if (ran != 1) st = "lexical"
        else if (d in unjudged) st = "unjudged"
        else if (d in lead) st = "lead"
        else if (d in mentions || d in surface) st = "judged"
        else st = "clear"
        print info[d] "\t" st
      }
    }
  ' "$work/units.tsv"
}

jev_triage() {
  local dir="$1" reason="" work out prio unit target kind url delta file
  local sel noul n=0 pages=0 capped=0 total i chunk lines label pages_out jev_ran=0
  shift
  out="$dir/triage.tsv"
  pages_out="$dir/triage-pages.tsv"
  rm -f "$out" "$out.tmp" "$pages_out"
  if [ ! -r "$dir/deltas.tsv" ]; then
    jev_note "skipped (no deltas.tsv in $dir)"
    return 0
  fi
  reason=$(jev_ready) || true
  [ -n "$reason" ] || jev_ran=1
  work=$(mktemp -d)
  # shellcheck disable=SC2064 # expand now: $work is local
  trap "rm -rf '$work'" EXIT
  mkdir -p "$work/claims" "$work/lex" "$work/req" "$work/meta" "$work/sel"
  : >"$work/rows"

  while IFS="$TAB" read -r prio unit target kind url delta file label; do
    pages=$((pages + 1))
    printf '%s\t%s\t%s\t%s\t%s\n' "$unit" "$target" "$url" "$delta" "$label" >>"$work/units.tsv"
    [ -f "$work/claims/$target" ] || jev_claims "$target" >"$work/claims/$target"
    jev_pair "$work/claims/$target" "$file" "$target" >"$work/lex/$unit"
    printf '%s\t%s\t%s\n' "$target" "$url" "$delta" >"$work/lex/$unit.meta"
    [ -z "$reason" ] || continue

    noul=true
    total=$(grep -c . "$work/claims/$target" || true)
    lines=$(awk -F '\t' -v total="$total" '
      { print $1; seen[$1] = 1 }
      END { for (i = 1; i <= total; i++) if (!(i in seen)) print i }
    ' "$work/lex/$unit")
    chunk=0
    while :; do
      sel="$work/sel/$unit.$chunk.tsv"
      printf '%s\n' "$lines" | awk -v from=$((chunk * JEV_CLAIMS_PER_REQUEST)) -v k="$JEV_CLAIMS_PER_REQUEST" \
        -v claims="$work/claims/$target" '
        BEGIN { while ((getline c < claims) > 0) text[++m] = c }
        NF && ++seen > from && seen <= from + k { print $1 "\t" text[$1] }
      ' >"$sel"
      [ -s "$sel" ] || [ "$noul" = true ] || break
      if [ "$n" -ge "$JEV_MAX_REQUESTS" ]; then
        capped=$((capped + 1))
        : >"$work/unjudged.$unit"
      else
        n=$((n + 1))
        jev_request "$file" "$sel" "$target" "$kind page $url" "$noul" >"$work/req/$(printf '%05d' "$n").json"
        printf '%s\t%s\t%s\t%s\t%s\n' "$target" "$url" "$delta" "$sel" "$unit" >"$work/meta/$(printf '%05d' "$n").tsv"
      fi
      noul=false
      chunk=$((chunk + 1))
      [ -s "$sel" ] || break
    done
  done < <(jev_units "$dir" "$work" "$@")

  if [ "$pages" -eq 0 ]; then
    : >"$out"
    : >"$pages_out"
    jev_note "no changed page with a delta in $dir, wrote an empty $out"
    return 0
  fi

  local answered=0 failed=0 m r
  if [ -z "$reason" ]; then
    jev_run "$work"
    for m in "$work"/meta/*.tsv; do
      [ -e "$m" ] || continue
      r="$work/resp/$(basename "$m" .tsv).json"
      if [ -s "$r" ]; then
        answered=$((answered + 1))
        IFS="$TAB" read -r _ _ _ sel unit <"$m"
        jev_answers "$r" "$m" | tee -a "$work/answered.$unit" | cut -f2- >>"$work/rows"
      else
        failed=$((failed + 1))
        IFS="$TAB" read -r _ _ _ _ unit <"$m"
        : >"$work/unjudged.$unit"
      fi
    done
    if [ -e "$work/fatal" ]; then
      reason="$(cat "$work/fatal")"
    elif [ "$failed" -gt 0 ]; then
      reason="$(cat "$work"/fail/* 2>/dev/null | head -n 1)"
    fi
  fi

  local lexical=0 lex
  for lex in "$work"/lex/*.meta; do
    unit=$(basename "$lex" .meta)
    IFS="$TAB" read -r target url delta <"$lex"
    while IFS="$TAB" read -r i _; do
      [ -n "$i" ] || continue
      if [ -e "$work/answered.$unit" ] && cut -f1 "$work/answered.$unit" | grep -qx "$i"; then
        continue
      fi
      printf '%s\t%s\t%s\t%s\tpaired\t\t\tlexical\n' "$target" "$url" "$delta" "$(sed -n "${i}p" "$work/claims/$target")" >>"$work/rows"
      lexical=$((lexical + 1))
    done < <(head -n "$JEV_LEXICAL_LEADS" "$work/lex/$unit")
  done

  jev_finish "$work/rows" "$out"
  jev_pages "$work" "$out" "$jev_ran" >"$pages_out"
  local tokens=0
  if [ "$answered" -gt 0 ]; then
    tokens=$(cat "$work"/resp/*.json | jq -s 'map(.usage.input_tokens // 0) | add // 0' 2>/dev/null || echo 0)
  fi
  local clear
  clear=$(awk -F '\t' '$5 == "clear"' "$pages_out" | grep -c . || true)
  awk -F '\t' -v n="$n" -v ok="$answered" -v pages="$pages" -v capped="$capped" -v out="$out" -v why="$reason" -v tokens="$tokens" -v clear="$clear" '
    { rows++; if ($5 == "contradicts") c++; if ($5 == "noul") s++; if ($8 == "lexical") l++ }
    END {
      if (n == 0) head = "lexical leads only (" (why != "" ? why : "no claims to ask about") ")"
      else head = sprintf("%d of %d Jev requests answered, %d input tokens", ok, n, tokens) (why != "" ? " (" why ")" : "")
      printf "jev-triage: %s over %d page parts%s, %d rows (%d contradicts, %d surface, %d lexical) in %s, %d pages clear\n",
        head, pages, (capped ? ", " capped " requests over the cap" : ""), rows, c, s, l, out, clear
    }
  ' "$out" >&2
}

# jev_replay <cases.tsv> asks each labeled case with its excerpt framed as
# added text and the target's current claims beside it as distractors, and
# reports recall, false positives, and what the lexical pairing alone finds.
jev_replay() {
  local cases="$1" reason work id target claim excerpt expected source paired seq=0
  if [ ! -r "$cases" ]; then
    echo "cannot read $cases" >&2
    return 1
  fi
  reason=$(jev_ready) || true
  [ -z "$reason" ] || jev_note "lexical only ($reason)"
  work=$(mktemp -d)
  # shellcheck disable=SC2064 # expand now: $work is local
  trap "rm -rf '$work'" EXIT
  mkdir -p "$work/req" "$work/cases" "$work/claims"
  while IFS="$TAB" read -r id target claim excerpt expected source; do
    case "$id" in '' | '#'*) continue ;; esac
    seq=$((seq + 1))
    printf '{+%s+}' "$excerpt" >"$work/cases/$seq.region"
    printf '%s\n' "$claim" >"$work/cases/$seq.claim"
    if jev_pair "$work/cases/$seq.claim" "$work/cases/$seq.region" "$target" | grep -q .; then
      paired=yes
    else
      paired=no
    fi
    printf '%s\t%s\t%s\t%s\n' "$id" "$expected" "$source" "$paired" >"$work/cases/$seq.meta"
    [ -z "$reason" ] || continue
    [ -f "$work/claims/$target" ] || jev_claims "$target" >"$work/claims/$target"
    {
      printf '0\t%s\n' "$claim"
      awk -v k=$((JEV_CLAIMS_PER_REQUEST - 1)) 'NR <= k { print NR "\t" $0 }' "$work/claims/$target"
    } >"$work/cases/$seq.sel"
    jev_request "$work/cases/$seq.region" "$work/cases/$seq.sel" "$target" "docs page" false >"$work/req/$(printf '%05d' "$seq").json"
  done <"$cases"

  [ -n "$reason" ] || jev_run "$work"
  if [ -z "$reason" ] && [ -e "$work/fatal" ]; then
    jev_note "lexical only ($(cat "$work/fatal"))"
  fi

  local i r
  for i in $(seq 1 "$seq"); do
    IFS="$TAB" read -r id expected source paired <"$work/cases/$i.meta"
    r="$work/resp/$(printf '%05d' "$i").json"
    if [ -s "$r" ]; then
      jq -r --arg id "$id" --arg expected "$expected" --arg source "$source" --arg paired "$paired" \
        --arg keep "$JEV_KEEP_CONTRADICTS" '
        .answers.c0 as $a
        | ($a.probabilities.contradicts // 0) as $pc
        | [$id, $expected, $a.choice, ($pc * 100 | round / 100 | tostring),
           $paired, (if $a.choice == $expected then "ok" else "MISS" end), $source,
           (if $a.choice == "contradicts" or $pc >= ($keep | tonumber) then "lead" else "-" end)]
        | @tsv' "$r"
    else
      printf '%s\t%s\t-\t-\t%s\t-\t%s\t-\n' "$id" "$expected" "$paired" "$source"
    fi
  done | awk -F '\t' -v keep="$JEV_KEEP_CONTRADICTS" '
    BEGIN { printf "%-34s %-13s %-13s %5s  %-6s %-4s %-4s %s\n", "case", "expected", "got", "p_c", "paired", "", "lead", "source" }
    {
      printf "%-34s %-13s %-13s %5s  %-6s %-4s %-4s %s\n", $1, $2, $3, $4, $5, $6, $8, $7
      if ($2 == "contradicts") { pos++; if ($5 == "yes") lex++; if ($3 == "contradicts") hit++; if ($8 == "lead") led++ }
      else if ($8 == "lead") { stray++ }
      if ($2 == "says_nothing") { neg++; if ($3 == "contradicts") fp++ }
      if ($3 != "-") { judged++; if ($6 == "ok") ok++ }
    }
    END {
      if (judged) {
        printf "recall on contradicts: %d/%d", hit, pos
        if (pos) printf " (%.2f)", hit / pos
        printf "\nfalse positives on says_nothing: %d/%d", fp, neg
        if (neg) printf " (%.2f)", fp / neg
        printf "\nexact verdicts: %d/%d\n", ok, judged
        printf "leads (contradicts or p_contradicts >= %s): %d/%d drifts, %d/%d other cases\n", keep, led, pos, stray, judged - pos
      }
      printf "lexical pairing on contradicts: %d/%d", lex, pos
      if (pos) printf " (%.2f)", lex / pos
      printf "\n"
    }
  '
}

# jev_replay_surface <cases.tsv> asks the Noul of each labeled case, with
# the excerpt framed as added text and the target's claims asked beside it
# as a real run does, and prints recall on yes cases and false positives on
# no cases at a Noul of JEV_SURFACE_MIN.
jev_replay_surface() {
  local cases="$1" reason work id target excerpt expected source seq=0 i r
  if [ ! -r "$cases" ]; then
    echo "cannot read $cases" >&2
    return 1
  fi
  if ! reason=$(jev_ready); then
    jev_note "skipped ($reason)"
    return 0
  fi
  work=$(mktemp -d)
  # shellcheck disable=SC2064 # expand now: $work is local
  trap "rm -rf '$work'" EXIT
  mkdir -p "$work/req" "$work/cases" "$work/claims"
  while IFS="$TAB" read -r id target excerpt expected source; do
    case "$id" in '' | '#'*) continue ;; esac
    seq=$((seq + 1))
    printf '{+%s+}' "$excerpt" >"$work/cases/$seq.region"
    [ -f "$work/claims/$target" ] || jev_claims "$target" >"$work/claims/$target"
    awk -v k="$JEV_CLAIMS_PER_REQUEST" 'NR <= k { print NR "\t" $0 }' "$work/claims/$target" >"$work/cases/$seq.sel"
    printf '%s\t%s\t%s\n' "$id" "$expected" "$source" >"$work/cases/$seq.meta"
    jev_request "$work/cases/$seq.region" "$work/cases/$seq.sel" "$target" "docs page" true >"$work/req/$(printf '%05d' "$seq").json"
  done <"$cases"

  jev_run "$work"
  if [ -e "$work/fatal" ]; then
    jev_note "skipped ($(cat "$work/fatal"))"
    return 0
  fi

  for i in $(seq 1 "$seq"); do
    IFS="$TAB" read -r id expected source <"$work/cases/$i.meta"
    r="$work/resp/$(printf '%05d' "$i").json"
    if [ -s "$r" ]; then
      jq -r --arg id "$id" --arg expected "$expected" --arg source "$source" '
        [$id, $expected, (.answers.surface.noul * 100 | round / 100 | tostring), $source] | @tsv' "$r"
    else
      printf '%s\t%s\t-\t%s\n' "$id" "$expected" "$source"
    fi
  done | awk -F '\t' -v min="$JEV_SURFACE_MIN" '
    BEGIN { printf "%-40s %-8s %5s  %-4s %s\n", "case", "expected", "noul", "", "source" }
    {
      got = ($3 != "-" && $3 + 0 >= min + 0) ? "yes" : "no"
      mark = ($3 == "-") ? "-" : (got == $2 ? "ok" : "MISS")
      printf "%-40s %-8s %5s  %-4s %s\n", $1, $2, $3, mark, $4
      if ($2 == "yes") { pos++; if (mark == "ok") hit++ }
      if ($2 == "no") { neg++; if (mark == "MISS") fp++ }
      if ($3 != "-") { if (min_yes == "" && $2 == "yes") min_yes = $3; if ($2 == "yes" && $3 + 0 < min_yes + 0) min_yes = $3
                       if ($2 == "no" && $3 + 0 > max_no + 0) max_no = $3 }
    }
    END {
      printf "surface recall at noul >= %s: %d/%d", min, hit, pos
      if (pos) printf " (%.2f)", hit / pos
      printf "\nsurface false positives: %d/%d", fp, neg
      if (neg) printf " (%.2f)", fp / neg
      printf "\nlowest noul on yes %s, highest on no %s\n", (min_yes == "" ? "-" : min_yes), (max_no == "" ? "0" : max_no)
    }
  '
}

jev_main() {
  if [ "${1:-}" = --lexical ]; then
    JEV_LEXICAL=1
    shift
  fi
  case "${1:-}" in
    -h | --help)
      jev_usage
      return 0
      ;;
    --replay)
      if [ -z "${2:-}" ]; then
        echo "--replay needs a cases.tsv path" >&2
        return 2
      fi
      jev_replay "$2"
      return
      ;;
    --replay-surface)
      if [ -z "${2:-}" ]; then
        echo "--replay-surface needs a cases.tsv path" >&2
        return 2
      fi
      jev_replay_surface "$2"
      return
      ;;
    '')
      jev_usage >&2
      return 2
      ;;
  esac
  local dir="$1"
  shift
  jev_triage "$dir" "$@"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  jev_main "$@"
fi
