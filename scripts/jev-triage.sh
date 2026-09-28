#!/usr/bin/env bash
#
# jev-triage.sh - rank a target-audit run's changed vendor text against our
# claims with TypeSafe's Jev, so auditors read the likeliest drift first.
#
# Claims come from scripts/target-facts.sh, split into single checkable
# statements: output paths, supported kinds, adapter package doc paragraphs,
# and target-behavior.md lines. Evidence is each changed region of each
# .delta that scripts/docfetch.sh wrote for a new or changed row in the run's
# deltas.tsv. Code pairs a region with the same target's claims that share a
# path, key, or term, so each region costs one request carrying one Choice
# question per paired claim (supports, contradicts, says_nothing). A
# changelog region also carries one Noul: does it change a project-scoped
# configuration surface?
#
# Output: <run dir>/triage.tsv, one row per answer, strong contradicts first:
#   target, url, delta path, claim, verdict, probability
# A Noul row has verdict "noul" and the Noul value as its probability.
#
# The triage only orders reading. It never clears a row. With no
# TYPESAFE_API_KEY, no curl or jq, an unreachable API, or a non-2xx answer
# after retries, it prints "jev-triage: skipped (<reason>)" to stderr,
# leaves no triage.tsv, and exits 0, so the audit runs exactly as without it.
#
# Usage:
#   scripts/jev-triage.sh <run dir> [target...]
#   scripts/jev-triage.sh --replay <cases.tsv>
#
# Environment:
#   TYPESAFE_API_KEY        required; sent through a curl config on stdin
#   TYPESAFE_API_URL        endpoint override, for tests
#   JEV_MODEL               default jev-latest
#   JEV_TIMEOUT             seconds per request, default 60
#   JEV_JOBS                concurrent requests, default 4
#   JEV_MAX_REQUESTS        request cap per run, default 40
#   JEV_CLAIMS_PER_REGION   paired claims per region, default 4
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
JEV_CLAIMS_PER_REGION="${JEV_CLAIMS_PER_REGION:-4}"
JEV_TRIES="${JEV_TRIES:-3}"
JEV_BACKOFF="${JEV_BACKOFF:-2}"
JEV_REGION_CHARS=8000
JEV_SURFACE_CLAIM="changes a project-scoped config surface (file path, frontmatter field, MCP key, hook event, spec directory)"

jev_usage() {
  cat <<'EOF'
Usage: scripts/jev-triage.sh <run dir> [<target>...]
       scripts/jev-triage.sh --replay <cases.tsv>

  <run dir>      a scripts/docfetch.sh run directory holding deltas.tsv;
                 writes <run dir>/triage.tsv
  <target>...    only triage the named targets
  --replay       run labeled cases through Jev and print recall on
                 contradicts cases and the false-positive rate on
                 says_nothing cases
EOF
}

jev_skip() {
  printf 'jev-triage: skipped (%s)\n' "$1" >&2
}

jev_ready() {
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

# jev_pair <claims file> <region file> <target> prints the line numbers of
# the claims that share a path, key, or term with the region's moved words,
# best first. A path or key hit weighs 3, a plain word 1, and a claim needs 3.
jev_pair() {
  awk -v claims="$1" -v k="$JEV_CLAIMS_PER_REGION" -v t="$3" '
    BEGIN {
      split("about above after agent agents also always another because been before being both cannot could does done each either every file files first from have here into just like made make many more most much must need needs only other over same should since some such than that their them then there these they this those through under used uses using very were what when where which while will with within without would your agnostic-ai emits emitted writes written reads read project tool tools", sw, " ")
      for (i in sw) stop[sw[i]] = 1
      stop[tolower(t)] = 1
    }
    function toks(s, out, key,   n, i, raw, w, parts) {
      n = split(s, parts, /[^A-Za-z0-9_.\/-]+/)
      for (i = 1; i <= n; i++) {
        raw = parts[i]
        sub(/^[.\/-]+/, "", raw)
        sub(/[.\/:-]+$/, "", raw)
        if (length(raw) < 4) continue
        w = tolower(raw)
        if (w in stop) continue
        out[w] = 1
        if (raw ~ /[\/._]/ || raw ~ /[a-z][A-Z]/ || raw ~ /^[a-z]+-[a-z-]+$/) key[w] = 1
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
        if (score >= 3) printf "%d\t%d\n", score, idx
      }
    }
  ' "$2" | sort -t "$(printf '\t')" -k1,1nr -k2,2n | awk -v k="$JEV_CLAIMS_PER_REGION" 'NR <= k { print $2 }'
}

# jev_request <region file> <selected claims tsv> <target> <source> <noul>
# prints one request body: a Choice per selected claim, plus the Noul when
# <noul> is true. Question IDs never reach the model, so every instruction
# names the fields it reads.
jev_request() {
  jq -n \
    --arg model "$JEV_MODEL" \
    --arg target "$3" \
    --arg source "$4" \
    --argjson noul "$5" \
    --rawfile change "$1" \
    --rawfile claims "$2" '
    {
      model: $model,
      state: { target: $target, source: $source, vendor_change: $change },
      questions: (
        ($claims | split("\n") | map(select(length > 0) | split("\t")) | map({
          key: ("claim_" + .[0]),
          value: {
            type: "choice",
            instructions: {
              claim: (.[1:] | join("\t")),
              question: "`claim` is a statement the agnostic-ai project makes about how the tool named in `target` reads project configuration. `vendor_change` is a region of that tool'"'"'s documentation (`source`) that changed since the last audit: words inside [-...-] were removed, words inside {+...+} were added, and the other words are unchanged context. Reading the text as it stands after the change, how does it relate to `claim`?"
            },
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
            instructions: "`vendor_change` is a region of the changelog of the tool named in `target` that changed since the last audit: words inside [-...-] were removed, words inside {+...+} were added, and the other words are unchanged context. Does the change add, remove, rename, or alter a project-scoped configuration surface of that tool: a file path the tool reads inside a repository, a frontmatter field, an MCP configuration key, a hook event, or a directory the tool loads skills, rules, agents, or commands from?",
            criteria: {
              "true": "It names a project-scoped configuration surface and says it was added, removed, renamed, or changed",
              "false": "It changes only UI, models, pricing, performance, user-level settings, or fixes that leave project configuration as it was"
            }
          }
        } else {} end)
      )
    }'
}

# jev_post <request> <response> <fail file> sends one request, retrying 429,
# 5xx, and no answer with a linear backoff. On failure it writes the reason
# to <fail file> and returns 1.
jev_post() {
  local req="$1" out="$2" fail="$3" try=1 code key
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
        return 1
        ;;
      429 | 5?? | 000)
        if [ "$try" -ge "$JEV_TRIES" ]; then
          if [ "$code" = 000 ]; then
            printf 'no response from %s after %d tries' "$JEV_API_URL" "$try" >"$fail"
          else
            printf 'HTTP %s after %d tries' "$code" "$try" >"$fail"
          fi
          return 1
        fi
        sleep $((JEV_BACKOFF * try))
        try=$((try + 1))
        ;;
      *)
        printf 'HTTP %s' "$code" >"$fail"
        return 1
        ;;
    esac
  done
}

# jev_run <work dir> posts every request under <work>/req at most JEV_JOBS at
# a time, and stops at the first batch with a failure. Bash 3.2 has no
# `wait -n`, so it waits on each batch by PID.
jev_run() {
  local work="$1" req n pids="" count=0 pid
  mkdir -p "$work/resp"
  for req in "$work"/req/*.json; do
    [ -e "$req" ] || continue
    n=$(basename "$req" .json)
    jev_post "$req" "$work/resp/$n.json" "$work/fail.$n" &
    pids="$pids $!"
    count=$((count + 1))
    if [ "$count" -ge "$JEV_JOBS" ]; then
      for pid in $pids; do wait "$pid" || true; done
      pids=""
      count=0
      jev_failed "$work" && return 1
    fi
  done
  for pid in $pids; do wait "$pid" || true; done
  ! jev_failed "$work"
}

jev_failed() {
  local f
  for f in "$1"/fail.*; do
    [ -e "$f" ] && return 0
  done
  return 1
}

jev_fail_reason() {
  local f
  for f in "$1"/fail.*; do
    [ -e "$f" ] && { cat "$f"; return 0; }
  done
}

# jev_regions <run dir> <work dir> [target...] splits each delta into one
# file per changed region and prints "prio\tseq\ttarget\tkind\turl\tdelta\tfile",
# mentions first, then changelog, prose, whitespace, chrome.
jev_regions() {
  local dir="$1" work="$2" target kind url label delta prio seq=0 line n
  shift 2
  mkdir -p "$work/regions"
  while IFS=$'\t' read -r target kind url label delta; do
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
    [ "$kind" = changelog ] && [ "$prio" -gt 1 ] && prio=2
    if [ "$(head -n 1 "$delta")" = "# whitespace" ]; then
      seq=$((seq + 1))
      head -c "$JEV_REGION_CHARS" "$delta" >"$work/regions/$seq.txt"
      printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$prio" "$seq" "$target" "$kind" "$url" "$delta" "$work/regions/$seq.txt"
      continue
    fi
    n=0
    while IFS= read -r line || [ -n "$line" ]; do
      [ -n "$line" ] || continue
      seq=$((seq + 1))
      n=$((n + 1))
      printf '%s' "$line" | head -c "$JEV_REGION_CHARS" >"$work/regions/$seq.txt"
      printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$prio" "$seq" "$target" "$kind" "$url" "$delta" "$work/regions/$seq.txt"
    done <"$delta"
  done <"$dir/deltas.tsv" | sort -t "$(printf '\t')" -k1,1n -k2,2n
}

# jev_rows <response> <meta> prints the triage rows for one answered request.
jev_rows() {
  local target url delta sel
  IFS=$'\t' read -r target url delta sel <"$2"
  jq -r --arg target "$target" --arg url "$url" --arg delta "$delta" \
    --arg surface "$JEV_SURFACE_CLAIM" --rawfile claims "$sel" '
    ($claims | split("\n") | map(select(length > 0) | split("\t"))
      | map({ key: ("claim_" + .[0]), value: (.[1:] | join("\t")) }) | from_entries) as $c
    | .answers | to_entries[]
    | if .value.type == "noul" then
        [$target, $url, $delta, $surface, "noul", (.value.noul * 100 | round / 100 | tostring)]
      else
        [$target, $url, $delta, ($c[.key] // .key), .value.choice,
          (.value.probabilities[.value.choice] // 0 | . * 100 | round / 100 | tostring)]
      end
    | @tsv' "$1"
}

# jev_sort orders rows: contradicts, then noul, supports, says_nothing, each
# by probability, highest first.
jev_sort() {
  awk -F '\t' '
    {
      g = 5
      if ($5 == "contradicts") g = 1
      else if ($5 == "noul") g = 2
      else if ($5 == "supports") g = 3
      else if ($5 == "says_nothing") g = 4
      printf "%d\t%s\t%s\n", g, $6, $0
    }
  ' | sort -t "$(printf '\t')" -k1,1n -k2,2nr | cut -f3-
}

jev_triage() {
  local dir="$1" reason work out target kind url delta file prio seq sel noul n=0 regions=0 capped=0
  shift
  out="$dir/triage.tsv"
  rm -f "$out" "$out.tmp"
  if ! reason=$(jev_ready); then
    jev_skip "$reason"
    return 0
  fi
  if [ ! -r "$dir/deltas.tsv" ]; then
    jev_skip "no deltas.tsv in $dir"
    return 0
  fi
  work=$(mktemp -d)
  # shellcheck disable=SC2064 # expand now: $work is local
  trap "rm -rf '$work'" EXIT
  mkdir -p "$work/claims" "$work/req" "$work/meta" "$work/sel"

  while IFS=$'\t' read -r prio seq target kind url delta file; do
    regions=$((regions + 1))
    [ -f "$work/claims/$target" ] || jev_claims "$target" >"$work/claims/$target"
    sel="$work/sel/$seq.tsv"
    jev_pair "$work/claims/$target" "$file" "$target" |
      while IFS= read -r i; do
        printf '%s\t%s\n' "$i" "$(sed -n "${i}p" "$work/claims/$target")"
      done >"$sel"
    noul=false
    [ "$kind" = changelog ] && noul=true
    if [ ! -s "$sel" ] && [ "$noul" = false ]; then
      continue
    fi
    if [ "$n" -ge "$JEV_MAX_REQUESTS" ]; then
      capped=$((capped + 1))
      continue
    fi
    n=$((n + 1))
    jev_request "$file" "$sel" "$target" "$kind page $url" "$noul" >"$work/req/$seq.json"
    printf '%s\t%s\t%s\t%s\n' "$target" "$url" "$delta" "$sel" >"$work/meta/$seq.tsv"
  done < <(jev_regions "$dir" "$work" "$@")

  if ! jev_run "$work"; then
    rm -f "$out" "$out.tmp"
    jev_skip "$(jev_fail_reason "$work")"
    return 0
  fi

  local resp
  for resp in "$work"/resp/*.json; do
    [ -e "$resp" ] || continue
    jev_rows "$resp" "$work/meta/$(basename "$resp" .json).tsv"
  done | jev_sort >"$out.tmp"
  mv "$out.tmp" "$out"

  awk -F '\t' -v n="$n" -v regions="$regions" -v capped="$capped" -v out="$out" '
    { rows++; if ($5 == "contradicts") c++; if ($5 == "noul" && $6 >= 0.5) s++ }
    END {
      printf "jev-triage: %d requests over %d regions%s, %d rows (%d contradicts, %d surface) in %s\n",
        n, regions, (capped ? ", " capped " over the request cap" : ""), rows, c, s, out
    }
  ' "$out"
}

# jev_replay <cases.tsv> asks each labeled case as its own request, with the
# excerpt framed as added text, and prints per-case results and the rates.
jev_replay() {
  local cases="$1" reason work id target claim excerpt expected source seq=0
  if [ ! -r "$cases" ]; then
    echo "cannot read $cases" >&2
    return 1
  fi
  if ! reason=$(jev_ready); then
    jev_skip "$reason"
    return 0
  fi
  work=$(mktemp -d)
  # shellcheck disable=SC2064 # expand now: $work is local
  trap "rm -rf '$work'" EXIT
  mkdir -p "$work/req" "$work/cases"
  while IFS=$'\t' read -r id target claim excerpt expected source; do
    case "$id" in '' | '#'*) continue ;; esac
    seq=$((seq + 1))
    printf '{+%s+}' "$excerpt" >"$work/cases/$seq.region"
    printf '1\t%s\n' "$claim" >"$work/cases/$seq.sel"
    printf '%s\t%s\t%s\n' "$id" "$expected" "$source" >"$work/cases/$seq.meta"
    jev_request "$work/cases/$seq.region" "$work/cases/$seq.sel" "$target" "docs page" false >"$work/req/$seq.json"
  done <"$cases"

  if ! jev_run "$work"; then
    jev_skip "$(jev_fail_reason "$work")"
    return 0
  fi

  local i
  for i in $(seq 1 "$seq"); do
    IFS=$'\t' read -r id expected source <"$work/cases/$i.meta"
    jq -r --arg id "$id" --arg expected "$expected" --arg source "$source" '
      .answers.claim_1 as $a
      | [$id, $expected, $a.choice, ($a.probabilities[$a.choice] * 100 | round / 100 | tostring),
         (if $a.choice == $expected then "ok" else "MISS" end), $source]
      | @tsv' "$work/resp/$i.json"
  done | awk -F '\t' '
    BEGIN { printf "%-34s %-13s %-13s %5s  %-4s %s\n", "case", "expected", "got", "p", "", "source" }
    {
      printf "%-34s %-13s %-13s %5s  %-4s %s\n", $1, $2, $3, $4, $5, $6
      if ($2 == "contradicts") { pos++; if ($3 == "contradicts") hit++ }
      if ($2 == "says_nothing") { neg++; if ($3 == "contradicts") fp++ }
      total++; if ($5 == "ok") ok++
    }
    END {
      printf "recall on contradicts: %d/%d", hit, pos
      if (pos) printf " (%.2f)", hit / pos
      printf "\nfalse positives on says_nothing: %d/%d", fp, neg
      if (neg) printf " (%.2f)", fp / neg
      printf "\nexact verdicts: %d/%d\n", ok, total
    }
  '
}

jev_main() {
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
