#!/usr/bin/env bash

set -euo pipefail

metrics_usage() {
  cat <<'EOF'
Usage: scripts/audit-metrics.sh init <run-dir> <run-id> [<provenance-json>] [<start-epoch>]
       scripts/audit-metrics.sh record <run-dir> <stage|review|finding|usage|limit> <json>
       scripts/audit-metrics.sh finish <run-dir> [<end-epoch>]
       scripts/audit-metrics.sh report <run-dir>

Records audit-metrics.json. Use one coordinator to write it. JSON may be
read from stdin with -. Times are Unix seconds. See the target-audit
metrics reference for record fields and report instructions.
EOF
}

metrics_main() {
  local action="${1:-}" dir="${2:-}" file tmp value key now triage_error=""
  case "$action" in -h | --help) metrics_usage; return ;; esac
  if [ -z "$dir" ] || ! command -v jq >/dev/null 2>&1; then
    printf 'audit-metrics: run directory and jq are required\n' >&2
    return 2
  fi
  file="$dir/audit-metrics.json"
  case "$action" in
    init)
      [ ! -e "$file" ] || { printf '%s: already exists\n' "$file" >&2; return 1; }
      [ -n "${3:-}" ] || { metrics_usage >&2; return 2; }
      mkdir -p "$dir"
      value="${4:-\{\}}"
      [ "$value" != - ] || value=$(cat)
      now="${5:-$(date +%s)}"
      tmp=$(mktemp "$file.XXXXXX")
      if jq -en --arg id "$3" --argjson provenance "$value" --argjson start "$now" '
        if ($provenance | type) != "object" or ($start | type) != "number" or $start < 0 then error("invalid provenance or start") else
          {schema_version: 1, run: {id: $id, provenance: $provenance},
           audit: {start: $start, end: null}, stages: [], reviews: [], findings: [], auditor_usage: [], research_limits: []}
        end' >"$tmp"; then mv "$tmp" "$file"; else rm -f "$tmp"; return 1; fi
      ;;
    record | finish)
      [ -r "$file" ] || { printf '%s: cannot read metrics\n' "$file" >&2; return 1; }
      if [ "$action" = finish ]; then
        key=finish
        value="${3:-$(date +%s)}"
      else
        key="${3:-}"
        value="${4:-}"
        [ "$value" != - ] || value=$(cat)
      fi
      tmp=$(mktemp "$file.XXXXXX")
      if jq -e --arg key "$key" --argjson v "$value" '
        def text: type == "string" and length > 0;
        def nonnegative: type == "number" and . >= 0;
        def optional_number: . == null or nonnegative;
        def upsert($list; $id): .[$list] = ([.[$list][] | select(.[$id] != $v[$id])] + [$v]);
        if .schema_version != 1 then error("unsupported metrics schema")
        elif $key == "finish" then
          .audit.start as $start
          | if ($v | nonnegative) and $v >= $start
              and all(.stages[] | select(.phase == "audit"); .start >= $start and .start <= $v and (.end == null or .end <= $v))
            then .audit.end = $v else error("invalid audit end or stage outside audit bounds") end
        elif $key == "stage" then
          if ($v.id | text) and ($v.name | text) and ($v.phase == "audit" or $v.phase == "delivery")
            and ($v.start | nonnegative) and ($v.end | optional_number)
            and ($v.end == null or $v.end >= $v.start)
            and ($v.phase != "audit" or ($v.start >= .audit.start
              and (.audit.end == null or ($v.start <= .audit.end and ($v.end == null or $v.end <= .audit.end)))))
            and (($v.status == "running" and $v.end == null) or
              (($v.status == "completed" or $v.status == "interrupted") and $v.end != null))
          then upsert("stages"; "id") else error("invalid stage") end
        elif $key == "review" then
          if ($v.target | text) and ($v.reason | text)
            and ($v.builtin_override == null or ($v.builtin_override | type) == "boolean")
            and ($v.decision == "deep" or $v.decision == "sampled" or $v.decision == "omitted")
            and ($v.decision == "deep" or (($v.approval | text) and $v.builtin_override != true))
          then upsert("reviews"; "target") else error("invalid review or missing fast-path approval") end
        elif $key == "finding" then
          if ($v.id | text) and ($v.evidence | text)
            and ($v.outcome == "confirmed" or $v.outcome == "rejected" or $v.outcome == "unresolved")
            and ($v.lead == null or (($v.lead.file | text) and ($v.lead.line | nonnegative) and $v.lead.line >= 1))
          then upsert("findings"; "id") | .findings |= map(.lead = (.lead // null)) else error("invalid finding") end
        elif $key == "usage" then
          if ($v.id | text) and ($v.input_tokens | optional_number) and ($v.output_tokens | optional_number) and ($v.effort_seconds | optional_number)
          then upsert("auditor_usage"; "id") | .auditor_usage |= map(. + {input_tokens: (.input_tokens // null), output_tokens: (.output_tokens // null), effort_seconds: (.effort_seconds // null)})
          else error("invalid auditor usage") end
        elif $key == "limit" and ($v | text) then .research_limits = ((.research_limits + [$v]) | unique)
        else error("unknown record kind or invalid record") end' "$file" >"$tmp"; then
        mv "$tmp" "$file"
      else
        rm -f "$tmp"
        printf '%s: cannot record %s\n' "$file" "$key" >&2
        return 1
      fi
      ;;
    report)
      [ -r "$file" ] || { printf 'Audit metrics: unavailable (%s).\n' "$file"; return; }
      value=null
      if [ -e "$dir/triage-metrics.json" ]; then
        if ! value=$(jq -cse 'if length == 1 and (.[0] | type) == "object" then .[0] else error("invalid triage metrics") end' "$dir/triage-metrics.json" 2>/dev/null); then
          value=null
          triage_error="triage metrics are malformed or unreadable"
        fi
      fi
      jq -r --argjson triage "$value" --arg triage_error "$triage_error" '
        def show: if . == null then "unknown" else tostring end;
        def nonnegative: type == "number" and . >= 0;
        def optional_number: . == null or nonnegative;
        def valid_triage:
          try ((.mode | type) == "string" and (.stage.status | type) == "string"
            and all(.requests.planned, .requests.capped, .usage.known_input_tokens, .usage.known_output_tokens; nonnegative)
            and all(.requests.answered, .requests.unanswered, .usage.input_tokens, .usage.output_tokens; optional_number)
            and (.research_limits | type) == "array" and all(.research_limits[]; type == "string")) catch false;
        def total($key): if length == 0 or any(.[]; .[$key] == null) then null else map(.[$key]) | add end;
        def union:
          sort_by(.start) | reduce .[] as $s ({end: null, seconds: 0};
            if .end == null or $s.start > .end then .seconds += ($s.end - $s.start) | .end = $s.end
            elif $s.end > .end then .seconds += ($s.end - .end) | .end = $s.end else . end) | .seconds;
        (if $triage_error != "" then $triage_error
          elif $triage == null then "triage metrics unavailable"
          elif $triage.schema_version != 1 then "triage metrics schema does not match"
          elif (try ($triage.run.id == .run.id) catch false) != true then "triage metrics run ID does not match"
          elif ($triage | valid_triage) != true then "triage metrics are malformed"
          else "" end) as $triage_limit
        | (if $triage_limit == "" then $triage else null end) as $triage
        | [.stages[] | select(.phase == "audit" and .end != null)] as $closed
        | ($closed | union) as $union
        | ($closed | map(.end - .start) | add // 0) as $sum
        | "Run: \(.run.id); metrics schema: \(.schema_version).",
          "Provenance: \(.run.provenance | tojson).",
          "Audit elapsed: \((if .audit.end == null then null else .audit.end - .audit.start end) | show) seconds; recorded stage union: \($union) seconds; overlap: \($sum - $union) seconds.",
          "Stage | Phase | Start (epoch) | End (epoch) | Seconds | Status",
          "--- | --- | --- | --- | --- | ---",
          (.stages[] | "\(.id) | \(.phase) | \(.start) | \(.end | show) | \((if .end == null then null else .end - .start end) | show) | \(.status)"),
          "Delivery: \([.stages[] | select(.phase == "delivery")] | length) recorded stages, reported separately.",
          "Reviews: \(.reviews | length) planned, \([.reviews[] | select(.decision == "omitted")] | length) omitted under an approved fast path, \([.reviews[] | select(.decision == "sampled")] | length) sampled, \([.reviews[] | select(.builtin_override == true)] | length) built-in overrides.",
          "Findings: \([.findings[] | select(.outcome == "confirmed")] | length) confirmed, \([.findings[] | select(.outcome == "rejected")] | length) rejected, \([.findings[] | select(.outcome == "unresolved")] | length) unresolved; \([.findings[] | select(.lead != null)] | length) linked to a known lead.",
          "Auditor input tokens: \(.auditor_usage | total("input_tokens") | show); output tokens: \(.auditor_usage | total("output_tokens") | show); effort seconds: \(.auditor_usage | total("effort_seconds") | show).",
          (if $triage == null then "Triage metrics: unavailable."
            else "Triage: \($triage.mode) (\($triage.stage.status)); \($triage.requests.answered | show)/\($triage.requests.planned) requests answered, \($triage.requests.capped) capped; input tokens: \($triage.usage.input_tokens | show) (known subtotal \($triage.usage.known_input_tokens)); output tokens: \($triage.usage.output_tokens | show) (known subtotal \($triage.usage.known_output_tokens))." end),
          ((.research_limits + ($triage.research_limits // []) + [$triage_limit]) | map(select(length > 0)) | unique | if length == 0 then "Research limits: none recorded." else "Research limits: " + join("; ") + "." end)
      ' "$file"
      ;;
    *) metrics_usage >&2; return 2 ;;
  esac
}

metrics_main "$@"
