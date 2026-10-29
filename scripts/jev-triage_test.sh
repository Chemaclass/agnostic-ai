#!/usr/bin/env bash
#
# bashunit tests for scripts/jev-triage.sh
#
# Run:
#   bashunit scripts/jev-triage_test.sh
#
# No test reaches TypeSafe. A fake curl on PATH records its arguments and
# stdin, then answers from FAKE_CURL_CODE, or FAKE_CURL_FAIL_CODE on call
# number FAKE_CURL_FAIL_CALL. A 2xx answer picks each Choice from the claim
# its question names in state: "mcp.json" contradicts, "hooks" supports,
# anything else says nothing. The claims come from a stub facts script.
# A fake trim-1 helper ranks first every claim matching FAKE_TRIM_MATCH, or
# exits FAKE_TRIM_EXIT; one test runs the real helper when Go is installed.

# shellcheck disable=SC2016 # backticks in fixtures are Markdown, not commands
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
JEV="$SCRIPT_DIR/jev-triage.sh"
SECRET="ts-test-key-5f3a9c"

FIXTURES=""
RUN=""

function set_up() {
  FIXTURES=$(mktemp -d)
  RUN="$FIXTURES/run"
  mkdir -p "$FIXTURES/bin" "$RUN/pages/copilot"
  write_fake_curl
  write_fake_trim
  write_facts
  write_run
}

function tear_down() {
  [ -n "$FIXTURES" ] && rm -rf "$FIXTURES"
}

function write_fake_curl() {
  cat >"$FIXTURES/bin/curl" <<'EOF'
#!/usr/bin/env bash
out="" data=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    --data-binary) data="${2#@}"; shift 2 ;;
    -w | -H | --max-time | --connect-timeout | --config) shift 2 ;;
    *) shift ;;
  esac
done
printf '%s\n' "$*" >>"$FAKE_CURL_LOG.args"
cat >>"$FAKE_CURL_LOG.stdin"
echo call >>"$FAKE_CURL_LOG.calls"
code="${FAKE_CURL_CODE:-200}"
if [ "${FAKE_CURL_FAIL_CALL:-}" = "$(wc -l <"$FAKE_CURL_LOG.calls" | tr -d ' ')" ]; then
  code="$FAKE_CURL_FAIL_CODE"
fi
if [ "$code" = 000 ]; then
  printf '000'
  exit 7
fi
if [ "${code#2}" = "$code" ]; then
  printf '{"error":"stub"}' >"$out"
  printf '%s' "$code"
  exit 0
fi
cp "$data" "$FAKE_CURL_LOG.request.$(wc -l <"$FAKE_CURL_LOG.calls" | tr -d ' ')"
jq --arg call "$(wc -l <"$FAKE_CURL_LOG.calls" | tr -d ' ')" '. as $r | {model: "jev-stub", usage: {input_tokens: ($ENV.FAKE_INPUT_TOKENS // "100" | tonumber), output_tokens: 10}, answers: (.questions | with_entries(.key as $k | .value |= (
  if .type == "noul" then {type: "noul", noul: ($ENV.FAKE_NOUL // "0.85" | tonumber)}
  else
    (($r.state.claims[$k] | ascii_downcase) as $c
      | (if ($ENV.FAKE_NOTHING // "") != "" then "says_nothing"
         elif ($c | contains("mcp.json")) then "contradicts"
         elif ($c | contains("hooks")) then "supports"
         else "says_nothing" end)) as $pick
    | {type: "choice", choice: $pick,
       probabilities: ({supports: 0.05, contradicts: 0.05, says_nothing: 0.05} + {($pick): 0.9}),
       confidence: 0.8}
  end)))} | if $ENV.FAKE_MISSING_USAGE == "1" or $ENV.FAKE_MISSING_USAGE_CALL == $call then del(.usage) else . end' "$data" >"$out"
printf '%s' "$code"
EOF
  chmod +x "$FIXTURES/bin/curl"
  export FAKE_CURL_LOG="$FIXTURES/curl"
}

function write_fake_trim() {
  cat >"$FIXTURES/bin/trim-leads" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_TRIM_LOG.args"
if [ -n "${FAKE_TRIM_EXIT:-}" ]; then
  echo "trim-leads: boom" >&2
  exit "$FAKE_TRIM_EXIT"
fi
while IFS="$(printf '\t')" read -r id claims text; do
  printf '%s\n' "$id" >>"$FAKE_TRIM_LOG.jobs"
  head -n 1 "$claims" >>"$FAKE_TRIM_LOG.first"
  { cat "$text"; echo; } >>"$FAKE_TRIM_LOG.text"
  [ -n "${FAKE_TRIM_MATCH:-}" ] || continue
  awk -v id="$id" -v re="$FAKE_TRIM_MATCH" '$0 ~ re { printf "%s\t%d\t1\t0.%d\n", id, NR, 90 - NR }' "$claims"
done
EOF
  chmod +x "$FIXTURES/bin/trim-leads"
  export FAKE_TRIM_LOG="$FIXTURES/trim"
}

function write_facts() {
  cat >"$FIXTURES/facts.sh" <<'EOF'
#!/usr/bin/env bash
cat <<'FACTS'
--- declared capabilities ---
Supports: []{MCP, Hook},

--- default output paths ---
target            = "copilot"
defaultCLIMCPFile = ".github/mcp.json"
defaultHooksDir   = ".github/hooks"

--- adapter package doc (what we claim the tool does) ---
// Package copilot emits GitHub Copilot custom instructions.
//
// MCP servers emit to `.github/mcp.json`, the shared Copilot CLI
// configuration committed to the repository.
--- docs/site/content/docs/target-behavior.md lines ---
12:| copilot | rules | always-on instructions |
FACTS
EOF
  chmod +x "$FIXTURES/facts.sh"
  export JEV_TARGET_FACTS="$FIXTURES/facts.sh"
}

function write_run() {
  local p="$RUN/pages/copilot"
  printf '%s\n' 'Copilot CLI reads [-`.github/mcp.json`-]{+`.github/copilot/mcp.json`+} for servers shared with the team.' >"$p/docs-1.delta"
  printf '%s\n' 'Sep.25 Release {+Hooks now also load from .github/hooks/*.json in subfolders+} Sep.24' >"$p/changelog-2.delta"
  printf '%s\n' 'Removes the server. {+A disabled server remains configured but is not used.+} Using servers' >"$p/docs-3.delta"
  {
    printf 'copilot\tdocs\thttps://example.test/mcp\tmentions:.github/mcp.json\t%s\n' "$p/docs-1.delta"
    printf 'copilot\tchangelog\thttps://example.test/changelog\tprose\t%s\n' "$p/changelog-2.delta"
    printf 'copilot\tdocs\thttps://example.test/cli\tprose\t%s\n' "$p/docs-3.delta"
    printf 'copilot\tdocs\thttps://example.test/new\tno-snapshot\t\n'
  } >"$RUN/deltas.tsv"
}

# triage [args...] runs the script with the fake curl first on PATH and the
# fake trim-1 helper, and prints its stdout and stderr together.
function triage() {
  PATH="$FIXTURES/bin:$PATH" TYPESAFE_API_KEY="${KEY-$SECRET}" JEV_BACKOFF=0 \
    JEV_TRIM_HELPER="${TRIM_HELPER-$FIXTURES/bin/trim-leads}" bash "$JEV" "$@" 2>&1
}

function test_metrics_keep_missing_usage_distinct_from_measured_zero() {
  FAKE_MISSING_USAGE=1 triage "$RUN" >/dev/null
  assert_equals 'null 0 3' "$(jq -r '[.usage.input_tokens, .usage.known_input_tokens, .usage.missing_input_requests] | map(tostring) | join(" ")' "$RUN/triage-metrics.json")"
  FAKE_INPUT_TOKENS=0 triage "$RUN" >/dev/null
  assert_equals '0 0 0' "$(jq -r '[.usage.input_tokens, .usage.known_input_tokens, .usage.missing_input_requests] | map(tostring) | join(" ")' "$RUN/triage-metrics.json")"
}

function test_metrics_keep_known_subtotals_when_some_usage_or_answers_are_missing() {
  FAKE_MISSING_USAGE_CALL=2 JEV_JOBS=1 triage "$RUN" >/dev/null
  assert_equals 'null 200 1' "$(jq -r '[.usage.input_tokens, .usage.known_input_tokens, .usage.missing_input_requests] | map(tostring) | join(" ")' "$RUN/triage-metrics.json")"
  rm -f "$FAKE_CURL_LOG.calls"
  FAKE_CURL_FAIL_CALL=2 FAKE_CURL_FAIL_CODE=400 JEV_JOBS=1 triage "$RUN" >/dev/null
  assert_equals 'fallback 2 1 null 200' "$(jq -r '[.mode, .requests.answered, .requests.unanswered, .usage.input_tokens, .usage.known_input_tokens] | map(tostring) | join(" ")' "$RUN/triage-metrics.json")"
}

function test_metrics_failure_preserves_no_key_fallback() {
  printf '#!/usr/bin/env bash\nexit 127\n' >"$FIXTURES/bin/jq"
  chmod +x "$FIXTURES/bin/jq"
  local out code=0
  out=$(KEY='' triage "$RUN") || code=$?
  assert_equals 0 "$code"
  assert_contains 'metrics unavailable (cannot write artifact)' "$out"
  assert_contains 'lexical and trim-1 leads only (no TYPESAFE_API_KEY)' "$out"
  assert_file_exists "$RUN/triage.tsv"
  assert_file_not_exists "$RUN/triage-metrics.json"
}

function test_metrics_interrupted_snapshot_keeps_end_answers_and_usage_unknown() {
  bash -c '. "$1"; jev_metrics "$2" "$3" 100 running "" 3 0 2 0' _ "$JEV" "$RUN" "$FIXTURES"
  assert_equals 'pending null null null' "$(jq -r '[.mode, .stage.end, .requests.answered, .usage.input_tokens] | map(tostring) | join(" ")' "$RUN/triage-metrics.json")"
  assert_equals '3 2' "$(jq -r '[.requests.planned, .requests.capped] | map(tostring) | join(" ")' "$RUN/triage-metrics.json")"
}

function test_metrics_inherit_coordinator_identity_and_render_in_the_report() {
  bash "$SCRIPT_DIR/audit-metrics.sh" init "$RUN" fixture-run '{"scenario":"clean"}'
  KEY='' triage "$RUN" >/dev/null
  assert_equals fixture-run "$(jq -r '.run.id' "$RUN/triage-metrics.json")"
  assert_contains 'Triage: lexical-trim (completed); 0/0 requests answered, 0 capped; input tokens: 0' "$(bash "$SCRIPT_DIR/audit-metrics.sh" report "$RUN")"
}

function test_metrics_record_success_and_fallback_without_changing_tsv() {
  triage "$RUN" >/dev/null
  assert_equals '1 jev 3 3 300 30' "$(jq -r '[.schema_version, .mode, .requests.planned, .requests.answered, .usage.input_tokens, .usage.output_tokens] | map(tostring) | join(" ")' "$RUN/triage-metrics.json")"
  JEV_MAX_REQUESTS=1 triage "$RUN" >/dev/null
  assert_equals '1 1 2' "$(jq -r '[.requests.planned, .requests.answered, .requests.capped] | map(tostring) | join(" ")' "$RUN/triage-metrics.json")"
  JEV_JOBS=1 FAKE_CURL_CODE=401 triage "$RUN" >/dev/null
  assert_equals 'fallback 0 3' "$(jq -r '[.mode, .requests.answered, .requests.unanswered] | map(tostring) | join(" ")' "$RUN/triage-metrics.json")"
  KEY='' triage "$RUN" >/dev/null
  assert_equals 'lexical-trim 0 0' "$(jq -r '[.mode, .requests.planned, .usage.input_tokens] | map(tostring) | join(" ")' "$RUN/triage-metrics.json")"
  assert_equals 'no TYPESAFE_API_KEY' "$(jq -r '.research_limits[0]' "$RUN/triage-metrics.json")"
}

# ---- without Jev -------------------------------------------------------------

function test_no_key_writes_lexical_leads_and_exits_0() {
  local out code
  out=$(KEY='' triage "$RUN")
  code=$?
  assert_equals 0 "$code"
  assert_contains "jev-triage: lexical and trim-1 leads only (no TYPESAFE_API_KEY)" "$out"
  assert_file_not_exists "$FAKE_CURL_LOG.calls"
  assert_equals "paired lexical" "$(cut -f5,8 "$RUN/triage.tsv" | sort -u | tr '\t' ' ')"
  assert_contains 'reads its CLIMCP file from `.github/mcp.json`' "$(grep https://example.test/mcp "$RUN/triage.tsv")"
  assert_contains 'reads its hooks directory from `.github/hooks`' "$(grep https://example.test/changelog "$RUN/triage.tsv")"
}

function test_the_lexical_flag_sends_nothing_even_with_a_key() {
  local out
  out=$(triage --lexical "$RUN")
  assert_contains "lexical and trim-1 leads only (--lexical)" "$out"
  assert_file_not_exists "$FAKE_CURL_LOG.calls"
  assert_file_exists "$RUN/triage.tsv"
}

function test_lexical_leads_skip_unpaired_pages_and_rows_with_no_delta() {
  KEY='' triage "$RUN" >/dev/null
  assert_not_contains "https://example.test/cli" "$(cat "$RUN/triage.tsv")"
  assert_not_contains "https://example.test/new" "$(cat "$RUN/triage.tsv")"
}

function test_without_jev_trim_leads_follow_the_lexical_ones() {
  local out
  out=$(FAKE_TRIM_MATCH='instructions|hooks directory' KEY='' triage "$RUN")
  assert_contains "lexical and trim-1 leads only (no TYPESAFE_API_KEY)" "$out"
  assert_equals "-top 5" "$(cat "$FAKE_TRIM_LOG.args")"
  assert_equals 3 "$(grep -c . "$FAKE_TRIM_LOG.jobs")"
  assert_equals "lexical trim" "$(cut -f8 "$RUN/triage.tsv" | uniq | tr '\n' ' ' | sed 's/ $//')"
  assert_contains $'| copilot | rules | always-on instructions |\tpaired\t0.83\t\ttrim' "$(grep https://example.test/cli "$RUN/triage.tsv")"
  assert_equals "lexical" "$(grep https://example.test/changelog "$RUN/triage.tsv" | grep 'hooks directory' | cut -f8)"
  assert_empty "$(awk -F '\t' 'NF != 8' "$RUN/triage.tsv")"
  assert_equals "lexical" "$(cut -f5 "$RUN/triage-pages.tsv" | sort -u)"
}

function test_the_trim_rank_cut_reaches_the_helper() {
  JEV_TRIM_TOP=3 KEY='' triage "$RUN" >/dev/null
  assert_equals "-top 3" "$(cat "$FAKE_TRIM_LOG.args")"
}

function test_trim_does_not_run_beside_a_working_jev() {
  FAKE_TRIM_MATCH=instructions triage "$RUN" >/dev/null
  assert_file_not_exists "$FAKE_TRIM_LOG.args"
  assert_not_contains "trim" "$(cut -f8 "$RUN/triage.tsv")"
}

function test_trim_ranks_every_part_once_jev_fails_for_the_run() {
  local out
  out=$(FAKE_TRIM_MATCH=instructions FAKE_CURL_CODE=401 JEV_JOBS=1 triage "$RUN")
  assert_contains "(HTTP 401)" "$out"
  assert_equals 3 "$(grep -c . "$FAKE_TRIM_LOG.jobs")"
  assert_equals 3 "$(awk -F '\t' '$8 == "trim" { print $2 }' "$RUN/triage.tsv" | sort -u | grep -c .)"
  assert_equals "unjudged" "$(cut -f5 "$RUN/triage-pages.tsv" | sort -u)"
}

function test_trim_ranks_only_unjudged_parts_and_skips_claims_jev_answered() {
  # Request 1 asks the mcp page about claims 3, 6, and 4; request 2, which
  # fails, about 5, 1, and 2. The other pages are answered.
  FAKE_TRIM_MATCH='hooks directory|hook configuration' FAKE_CURL_FAIL_CALL=2 FAKE_CURL_FAIL_CODE=422 \
    JEV_CLAIMS_PER_REQUEST=3 JEV_JOBS=1 triage "$RUN" >/dev/null
  assert_equals 1 "$(grep -c . "$FAKE_TRIM_LOG.jobs")"
  local trim
  trim=$(awk -F '\t' '$8 == "trim" { print $2 "|" $4 }' "$RUN/triage.tsv")
  assert_equals 'https://example.test/mcp|copilot natively reads project-scoped hook configuration, so agnostic-ai emits hook specs for it.' "$trim"
  assert_equals "unjudged" "$(page_status https://example.test/mcp)"
}

function test_a_missing_trim_helper_falls_back_to_lexical_leads_with_one_note() {
  local out code
  out=$(TRIM_HELPER="$FIXTURES/missing" KEY='' triage "$RUN")
  code=$?
  assert_equals 0 "$code"
  assert_contains "jev-triage: trim-1 skipped ($FIXTURES/missing is not executable)" "$out"
  assert_equals 1 "$(printf '%s\n' "$out" | grep -c 'trim-1 skipped')"
  assert_contains "jev-triage: lexical leads only (no TYPESAFE_API_KEY)" "$out"
  assert_equals "lexical" "$(cut -f8 "$RUN/triage.tsv" | sort -u)"
}

function test_a_failing_trim_helper_falls_back_with_its_error() {
  local out
  out=$(FAKE_TRIM_EXIT=3 FAKE_TRIM_MATCH=instructions KEY='' triage "$RUN")
  assert_contains "jev-triage: trim-1 skipped (trim-leads: boom)" "$out"
  assert_equals "lexical" "$(cut -f8 "$RUN/triage.tsv" | sort -u)"
}

function test_by_default_the_helper_runs_from_its_own_module_with_go() {
  mkdir -p "$FIXTURES/gobin"
  cat >"$FIXTURES/gobin/go" <<'EOF'
#!/usr/bin/env bash
printf '%s|%s|%s\n' "$PWD" "$GOWORK" "$*" >"$FAKE_TRIM_LOG.go"
echo "go: downloading github.com/caveman-ai/trim-1" >&2
echo "no route to proxy.golang.org" >&2
exit 1
EOF
  chmod +x "$FIXTURES/gobin/go"
  local out
  out=$(PATH="$FIXTURES/gobin:$PATH" TRIM_HELPER='' KEY='' triage "$RUN")
  assert_equals "$SCRIPT_DIR/trim-leads|off|run . -top 5" "$(cat "$FAKE_TRIM_LOG.go")"
  assert_contains "jev-triage: trim-1 skipped (no route to proxy.golang.org)" "$out"
  assert_equals "lexical" "$(cut -f8 "$RUN/triage.tsv" | sort -u)"
}

function test_the_real_helper_writes_trim_leads() {
  if ! command -v go >/dev/null 2>&1; then
    bashunit::skip "go is not installed"
    return
  fi
  # With no lexical rows to win the duplicates, every trim-1 lead shows.
  local out
  out=$(TRIM_HELPER='' JEV_LEXICAL_LEADS=0 KEY='' triage "$RUN")
  assert_contains "lexical and trim-1 leads only (no TYPESAFE_API_KEY)" "$out"
  assert_not_contains "trim-1 skipped" "$out"
  assert_contains $'.github/mcp.json`, the shared Copilot CLI configuration committed to the repository.\tpaired' "$(grep -F "$(printf 'https://example.test/mcp\t')" "$RUN/triage.tsv")"
  assert_equals "trim" "$(cut -f8 "$RUN/triage.tsv" | sort -u)"
}

function test_the_real_helper_flags_no_claim_of_a_small_target_for_unrelated_text() {
  if ! command -v go >/dev/null 2>&1; then
    bashunit::skip "go is not installed"
    return
  fi
  bash -c '. "$1"; jev_claims copilot' _ "$JEV" >"$FIXTURES/claims"
  printf '%s\n' 'Pricing {+Team plans now cost $20 per seat each month, billed yearly.+} FAQ' >"$FIXTURES/pricing.delta"
  local out
  out=$(printf 'p1\t%s\t%s\n' "$FIXTURES/claims" "$FIXTURES/pricing.delta" |
    (cd "$SCRIPT_DIR/trim-leads" && GOWORK=off go run . -top 10))
  assert_equals 7 "$(grep -c . "$FIXTURES/claims")"
  assert_empty "$out"
}

function test_missing_deltas_skips_and_removes_a_stale_triage() {
  echo stale >"$RUN/triage.tsv"
  rm "$RUN/deltas.tsv"
  assert_equals "jev-triage: skipped (no deltas.tsv in $RUN)" "$(triage "$RUN")"
  assert_file_not_exists "$RUN/triage.tsv"
}

function test_rows_with_no_delta_leave_an_empty_triage() {
  echo stale >"$RUN/triage.tsv"
  printf 'copilot\tdocs\thttps://example.test/new\tno-snapshot\t\n' >"$RUN/deltas.tsv"
  assert_contains "no changed page with a delta" "$(triage "$RUN")"
  assert_file_exists "$RUN/triage.tsv"
  assert_empty "$(cat "$RUN/triage.tsv")"
  assert_file_not_exists "$FAKE_CURL_LOG.calls"
}

function test_overloaded_api_retries_then_falls_back_to_lexical_leads() {
  local out code
  out=$(FAKE_CURL_CODE=529 JEV_JOBS=1 triage "$RUN")
  code=$?
  assert_equals 0 "$code"
  assert_contains "0 of 3 Jev requests answered, 0 input tokens (HTTP 529 after 3 tries)" "$out"
  assert_equals 3 "$(wc -l <"$FAKE_CURL_LOG.calls" | tr -d ' ')"
  assert_equals "lexical" "$(cut -f8 "$RUN/triage.tsv" | sort -u)"
  assert_file_not_exists "$RUN/triage.tsv.tmp"
}

function test_unauthorized_stops_after_one_call() {
  local out
  out=$(FAKE_CURL_CODE=401 JEV_JOBS=1 triage "$RUN")
  assert_contains "(HTTP 401)" "$out"
  assert_equals 1 "$(wc -l <"$FAKE_CURL_LOG.calls" | tr -d ' ')"
  assert_equals "lexical" "$(cut -f8 "$RUN/triage.tsv" | sort -u)"
}

function test_parallel_requests_failing_together_leave_every_page_unjudged() {
  # GNU cp creates a missing target exclusively, so of two cps onto the same
  # new file one fails. The pause makes every job look before any creates.
  cat >"$FIXTURES/bin/cp" <<'EOF'
#!/usr/bin/env bash
sleep 0.2
set -o noclobber
cat "$1" >"$2"
EOF
  chmod +x "$FIXTURES/bin/cp"
  local out code
  out=$(FAKE_CURL_CODE=401 JEV_JOBS=4 triage "$RUN")
  code=$?
  assert_equals 0 "$code"
  assert_contains "(HTTP 401)" "$out"
  assert_equals 3 "$(wc -l <"$FAKE_CURL_LOG.calls" | tr -d ' ')"
  assert_equals "unjudged" "$(cut -f5 "$RUN/triage-pages.tsv" | sort -u)"
}

function test_unreachable_host_falls_back_with_the_real_curl() {
  local out code
  out=$(TYPESAFE_API_URL=http://127.0.0.1:9/v1/systemone TYPESAFE_API_KEY="$SECRET" \
    JEV_BACKOFF=0 JEV_TRIES=2 JEV_TIMEOUT=5 JEV_JOBS=1 bash "$JEV" "$RUN" 2>&1)
  code=$?
  assert_equals 0 "$code"
  assert_contains "(no response from http://127.0.0.1:9/v1/systemone after 2 tries)" "$out"
  assert_equals "lexical" "$(cut -f8 "$RUN/triage.tsv" | sort -u)"
}

function test_a_rejected_request_leaves_the_other_pages_judged() {
  local out
  out=$(FAKE_CURL_FAIL_CALL=1 FAKE_CURL_FAIL_CODE=422 JEV_JOBS=1 triage "$RUN")
  assert_contains "2 of 3 Jev requests answered, 200 input tokens (HTTP 422)" "$out"
  assert_equals 3 "$(wc -l <"$FAKE_CURL_LOG.calls" | tr -d ' ')"
  assert_equals "lexical" "$(grep https://example.test/mcp "$RUN/triage.tsv" | cut -f8 | sort -u)"
  assert_equals "jev" "$(grep https://example.test/changelog "$RUN/triage.tsv" | cut -f8 | sort -u)"
}

# ---- with Jev ----------------------------------------------------------------

function test_writes_rows_with_strong_contradicts_first() {
  triage "$RUN" >/dev/null
  local first
  first=$(head -n 1 "$RUN/triage.tsv")
  assert_contains $'\tcontradicts\t0.9\t0.9\tjev' "$first"
  assert_equals "contradicts noul" "$(cut -f5 "$RUN/triage.tsv" | awk '!seen[$0]++' | tr '\n' ' ' | sed 's/ $//')"
  assert_empty "$(awk -F '\t' 'NF != 8' "$RUN/triage.tsv")"
}

function test_every_changed_page_is_asked_about_every_claim() {
  JEV_JOBS=1 triage "$RUN" >/dev/null
  assert_equals 3 "$(wc -l <"$FAKE_CURL_LOG.calls" | tr -d ' ')"
  local req="$FAKE_CURL_LOG.request.1"
  assert_equals 7 "$(jq '.state.claims | length' "$req")"
  assert_equals 8 "$(jq '.questions | length' "$req")"
  assert_equals "noul" "$(jq -r '.questions.surface.type' "$req")"
  assert_contains "https://example.test/cli" "$(cat "$RUN/triage.tsv")"
  assert_not_contains "https://example.test/new" "$(cat "$RUN/triage.tsv")"
}

function test_a_request_holds_each_claim_once_and_names_it_by_path() {
  JEV_JOBS=1 triage "$RUN" >/dev/null
  local req="$FAKE_CURL_LOG.request.1"
  assert_equals "jev-latest" "$(jq -r .model "$req")"
  assert_equals "c3" "$(jq -r '.state.claims | to_entries[] | select(.value | contains(".github/mcp.json` in")) | .key' "$req")"
  assert_contains '`claims.c3`' "$(jq -r .questions.c3.instructions "$req")"
  assert_equals "supports,contradicts,says_nothing" "$(jq -r '.questions.c3.criteria | keys_unsorted | join(",")' "$req")"
  assert_contains '[-`.github/mcp.json`-]' "$(jq -r .state.vendor_change "$req")"
}

function test_claims_are_asked_in_chunks() {
  JEV_CLAIMS_PER_REQUEST=3 JEV_JOBS=1 triage "$RUN" >/dev/null
  assert_equals 9 "$(wc -l <"$FAKE_CURL_LOG.calls" | tr -d ' ')"
  assert_equals 4 "$(jq '.questions | length' "$FAKE_CURL_LOG.request.1")"
  assert_equals 3 "$(jq '.questions | length' "$FAKE_CURL_LOG.request.2")"
}

function test_answers_below_the_lead_threshold_are_dropped() {
  triage "$RUN" >/dev/null
  assert_not_contains "says_nothing" "$(cut -f5 "$RUN/triage.tsv")"
  assert_not_contains "supports" "$(cut -f5 "$RUN/triage.tsv")"
  JEV_KEEP_CONTRADICTS=0 triage "$RUN" >/dev/null
  assert_contains "says_nothing" "$(cut -f5 "$RUN/triage.tsv")"
  assert_contains $'\tsupports\t0.9\t0.05\tjev' "$(cat "$RUN/triage.tsv")"
}

function test_the_summary_counts_input_tokens() {
  assert_contains "3 of 3 Jev requests answered, 300 input tokens" "$(triage "$RUN")"
}

function test_every_page_carries_one_surface_row() {
  triage "$RUN" >/dev/null
  local noul
  noul=$(awk -F '\t' '$5 == "noul"' "$RUN/triage.tsv")
  assert_equals 3 "$(printf '%s\n' "$noul" | grep -c .)"
  assert_contains $'https://example.test/changelog\t' "$noul"
  assert_contains $'https://example.test/cli\t' "$noul"
  assert_contains $'\tnoul\t0.85\t\tjev' "$noul"
}

function test_a_weak_surface_answer_is_dropped() {
  FAKE_NOUL=0.2 triage "$RUN" >/dev/null
  assert_not_contains "noul" "$(cut -f5 "$RUN/triage.tsv")"
}

function test_the_target_filter_skips_other_targets() {
  printf 'zed\tdocs\thttps://example.test/zed\tprose\t%s\n' "$RUN/pages/copilot/docs-1.delta" >>"$RUN/deltas.tsv"
  triage "$RUN" copilot >/dev/null
  assert_not_contains "https://example.test/zed" "$(cat "$RUN/triage.tsv")"
}

function test_the_request_cap_bounds_calls_and_leaves_lexical_leads() {
  local out
  out=$(JEV_MAX_REQUESTS=1 triage "$RUN")
  assert_equals 1 "$(wc -l <"$FAKE_CURL_LOG.calls" | tr -d ' ')"
  assert_contains "2 requests over the cap" "$out"
  assert_equals "lexical" "$(grep https://example.test/changelog "$RUN/triage.tsv" | cut -f8 | sort -u)"
}

function test_the_key_never_reaches_argv_or_output() {
  local out
  out=$(triage "$RUN")
  assert_not_contains "$SECRET" "$out"
  assert_not_contains "$SECRET" "$(cat "$RUN/triage.tsv")"
  assert_not_contains "$SECRET" "$(cat "$FAKE_CURL_LOG.args")"
  assert_contains "header = \"Authorization: Bearer $SECRET\"" "$(cat "$FAKE_CURL_LOG.stdin")"
}

# ---- page status -------------------------------------------------------------

function page_status() {
  awk -F '\t' -v url="$1" '$2 == url { print $5 }' "$RUN/triage-pages.tsv"
}

function test_a_page_with_no_jev_lead_is_clear_but_a_mentions_page_is_only_judged() {
  FAKE_NOTHING=1 FAKE_NOUL=0.05 triage "$RUN" >/dev/null
  assert_equals "clear" "$(page_status https://example.test/cli)"
  assert_equals "clear" "$(page_status https://example.test/changelog)"
  assert_equals "judged" "$(page_status https://example.test/mcp)"
  assert_empty "$(page_status https://example.test/new)"
  assert_contains "2 pages clear" "$(FAKE_NOTHING=1 FAKE_NOUL=0.05 triage "$RUN")"
}

function test_a_page_with_an_uncertain_surface_is_only_judged() {
  FAKE_NOTHING=1 FAKE_NOUL=0.3 triage "$RUN" >/dev/null
  assert_equals "judged" "$(page_status https://example.test/cli)"
  assert_not_contains "noul" "$(cut -f5 "$RUN/triage.tsv")"
}

function test_a_page_with_a_lead_or_a_surface_is_not_clear() {
  triage "$RUN" >/dev/null
  assert_equals "lead" "$(page_status https://example.test/cli)"
  FAKE_NOTHING=1 triage "$RUN" >/dev/null
  assert_equals "lead" "$(page_status https://example.test/changelog)"
}

function test_a_failed_or_capped_page_is_unjudged() {
  FAKE_NOTHING=1 FAKE_NOUL=0.05 FAKE_CURL_FAIL_CALL=1 FAKE_CURL_FAIL_CODE=422 JEV_JOBS=1 triage "$RUN" >/dev/null
  assert_equals "unjudged" "$(page_status https://example.test/mcp)"
  assert_equals "clear" "$(page_status https://example.test/changelog)"
  FAKE_NOTHING=1 FAKE_NOUL=0.05 JEV_MAX_REQUESTS=1 triage "$RUN" >/dev/null
  assert_equals "unjudged" "$(page_status https://example.test/cli)"
}

function test_without_jev_every_page_is_lexical() {
  KEY='' triage "$RUN" >/dev/null
  assert_equals "lexical" "$(cut -f5 "$RUN/triage-pages.tsv" | sort -u)"
  assert_equals 3 "$(grep -c . "$RUN/triage-pages.tsv")"
  FAKE_CURL_CODE=401 triage "$RUN" >/dev/null
  assert_equals "unjudged" "$(cut -f5 "$RUN/triage-pages.tsv" | sort -u)"
}

function test_a_missing_deltas_file_removes_a_stale_page_status() {
  echo stale >"$RUN/triage-pages.tsv"
  rm "$RUN/deltas.tsv"
  triage "$RUN" >/dev/null
  assert_file_not_exists "$RUN/triage-pages.tsv"
}

# ---- replay ------------------------------------------------------------------

function write_cases() {
  {
    printf '# case\ttarget\tclaim\texcerpt\texpected\tsource\n'
    printf 'mcp-moved\tcopilot\tCopilot CLI reads `.github/mcp.json`.\tServers now live in .github/copilot/mcp.json.\tcontradicts\t1\n'
    printf 'hooks-kept\tcopilot\tCopilot loads hooks from .github/hooks.\tHooks load from .github/hooks/*.json.\tsupports\t2\n'
    printf 'unrelated\tcopilot\tCopilot reads skills from .github/skills.\tA disabled server remains configured.\tsays_nothing\t3\n'
    printf 'missed\tcopilot\tCopilot reads agents from .github/agents.\tAgents moved to .copilot/agents.\tcontradicts\t4\n'
  } >"$FIXTURES/cases.tsv"
}

function test_replay_prints_recall_false_positives_and_lexical_pairing() {
  write_cases
  local out
  out=$(triage --replay "$FIXTURES/cases.tsv")
  assert_contains "recall on contradicts: 1/2 (0.50)" "$out"
  assert_contains "false positives on says_nothing: 0/1 (0.00)" "$out"
  assert_contains "exact verdicts: 3/4" "$out"
  assert_contains "lexical pairing on contradicts: 1/2 (0.50)" "$out"
  assert_matches "missed +contradicts +says_nothing +0.05 +no +no +MISS +- +4" "$out"
  assert_contains "leads (contradicts or p_contradicts >= 0.4): 1/2 drifts, 0/2 other cases" "$out"
}

function test_replay_asks_each_case_beside_the_target_claims() {
  write_cases
  JEV_JOBS=1 triage --replay "$FIXTURES/cases.tsv" >/dev/null
  local req="$FAKE_CURL_LOG.request.1"
  assert_equals 'Copilot CLI reads `.github/mcp.json`.' "$(jq -r .state.claims.c0 "$req")"
  assert_equals 8 "$(jq '.questions | length' "$req")"
}

function test_replay_without_a_key_reports_lexical_trim_and_union_recall() {
  write_cases
  local out code
  out=$(FAKE_TRIM_MATCH=agents KEY='' triage --replay "$FIXTURES/cases.tsv")
  code=$?
  assert_equals 0 "$code"
  assert_contains "jev-triage: lexical and trim-1 only (no TYPESAFE_API_KEY)" "$out"
  assert_contains "lexical pairing on contradicts: 1/2 (0.50), on says_nothing: 0/1" "$out"
  assert_contains "trim-1 top 5 on contradicts: 1/2 (0.50), on says_nothing: 0/1" "$out"
  assert_contains "lexical or trim-1 on contradicts: 2/2 (1.00), on says_nothing: 0/1" "$out"
  assert_matches "missed +contradicts +- +- +no +yes" "$out"
  assert_not_contains "recall on contradicts" "$out"
  assert_file_not_exists "$FAKE_CURL_LOG.calls"
}

function test_replay_ranks_each_excerpt_against_the_case_claim_and_the_target_claims() {
  write_cases
  KEY='' triage --replay "$FIXTURES/cases.tsv" >/dev/null
  assert_equals 4 "$(grep -c . "$FAKE_TRIM_LOG.jobs")"
  assert_equals 'Copilot CLI reads `.github/mcp.json`.' "$(head -n 1 "$FAKE_TRIM_LOG.first")"
  assert_equals '{+Servers now live in .github/copilot/mcp.json.+}' "$(head -n 1 "$FAKE_TRIM_LOG.text")"
}

function test_replay_without_the_trim_helper_prints_the_lexical_line_only() {
  write_cases
  local out
  out=$(TRIM_HELPER="$FIXTURES/missing" KEY='' triage --replay "$FIXTURES/cases.tsv")
  assert_contains "jev-triage: trim-1 skipped ($FIXTURES/missing is not executable)" "$out"
  assert_contains "jev-triage: lexical only (no TYPESAFE_API_KEY)" "$out"
  assert_contains "lexical pairing on contradicts: 1/2 (0.50)" "$out"
  assert_not_contains "trim-1 top" "$out"
  assert_matches "missed +contradicts +- +- +no +- " "$out"
}

function test_the_committed_cases_file_is_well_formed() {
  local cases="$SCRIPT_DIR/target-audit/triage-cases.tsv"
  assert_file_exists "$cases"
  assert_empty "$(awk -F '\t' '!/^#/ && NF != 6 { print NR ": " NF " columns" }' "$cases")"
  assert_empty "$(awk -F '\t' '!/^#/ && $5 !~ /^(supports|contradicts|says_nothing)$/ { print NR ": " $5 }' "$cases")"
  assert_empty "$(awk -F '\t' '!/^#/ && seen[$1]++ { print "duplicate " $1 }' "$cases")"
}

# ---- surface replay ----------------------------------------------------------

function write_surface_cases() {
  {
    printf '# case\ttarget\texcerpt\texpected\tsource\n'
    printf 'new-dir\tcopilot\tHooks now also load from .github/hooks/*.json.\tyes\t1\n'
    printf 'new-model\tcopilot\tA faster default model is available.\tno\t2\n'
  } >"$FIXTURES/surface.tsv"
}

function test_surface_replay_prints_recall_and_false_positives() {
  write_surface_cases
  local out
  out=$(JEV_JOBS=1 triage --replay-surface "$FIXTURES/surface.tsv")
  assert_contains "surface recall at noul >= 0.5: 1/1 (1.00)" "$out"
  assert_contains "surface false positives: 1/1 (1.00)" "$out"
  assert_matches "new-model +no +0.85 +MISS 2" "$out"
  assert_equals "noul" "$(jq -r '.questions.surface.type' "$FAKE_CURL_LOG.request.1")"
}

function test_surface_replay_without_a_key_skips_and_exits_0() {
  write_surface_cases
  local out code
  out=$(KEY='' triage --replay-surface "$FIXTURES/surface.tsv")
  code=$?
  assert_equals 0 "$code"
  assert_equals "jev-triage: skipped (no TYPESAFE_API_KEY)" "$out"
}

function test_the_committed_surface_cases_file_is_well_formed() {
  local cases="$SCRIPT_DIR/target-audit/surface-cases.tsv"
  assert_file_exists "$cases"
  assert_empty "$(awk -F '\t' '!/^#/ && NF != 5 { print NR ": " NF " columns" }' "$cases")"
  assert_empty "$(awk -F '\t' '!/^#/ && $4 !~ /^(yes|no)$/ { print NR ": " $4 }' "$cases")"
  assert_empty "$(awk -F '\t' '!/^#/ && seen[$1]++ { print "duplicate " $1 }' "$cases")"
}

# local/target-audit/ is ignored by Git, so a path there, a run-relative
# <run>/pages/ or <run>/rows/ path, or an audit:<date> pointer at a date
# report resolves on no fresh checkout. Stored snapshots are ignored and
# pruned too, so no local path is an exception.
function local_audit_citations() {
  local files f
  files=$(find "$1" -type f \( -name '*.tsv' -o -name '*.md' \))
  if [ -z "$files" ]; then
    printf 'no .tsv or .md files under %s\n' "$1"
    return
  fi
  printf '%s\n' "$files" | while IFS= read -r f; do
    grep -HoE 'local/target-audit/[A-Za-z0-9][A-Za-z0-9._/-]*|[0-9]{4}-[0-9]{2}-[0-9]{2}[^[:space:]]*/(pages|rows)/|(^|[^A-Za-z0-9_-])audit:[0-9]{4}-[0-9]{2}-[0-9]{2}' "$f" || true
  done
}

function test_local_audit_citations_flags_every_local_form() {
  mkdir -p "$FIXTURES/audit/nested"
  {
    printf 'a\tlocal/target-audit/snapshots/abc123.txt\n'
    printf 'b\tlocal/target-audit/2026-09-25-run/pages/claude/x.body\n'
    printf 'c\thttps://x.example as of 2026-10-01\n'
    printf 'd\taudit:2026-10-27\n'
    printf 'e\ttarget-capability-audit:2026-09-15\n'
  } >"$FIXTURES/audit/cases.tsv"
  printf 'See 2026-09-22-run-continuation/rows/cline.tsv\n' >"$FIXTURES/audit/nested/notes.md"
  local out
  out=$(local_audit_citations "$FIXTURES/audit")
  assert_contains "local/target-audit/snapshots/abc123.txt" "$out"
  assert_contains "local/target-audit/2026-09-25-run/pages/claude/x.body" "$out"
  assert_contains "audit:2026-10-27" "$out"
  assert_contains "nested/notes.md:2026-09-22-run-continuation/rows/" "$out"
  assert_not_contains "x.example" "$out"
  assert_not_contains "2026-09-15" "$out"
}

function test_local_audit_citations_fails_when_it_finds_no_files() {
  mkdir -p "$FIXTURES/empty"
  assert_contains "no .tsv or .md files" "$(local_audit_citations "$FIXTURES/empty")"
}

function test_the_committed_audit_files_cite_no_local_path_a_checkout_lacks() {
  assert_empty "$(local_audit_citations "$SCRIPT_DIR/target-audit")"
}

function test_claims_ignore_portable_builtin_evidence_and_keep_native_claims() {
  cat >"$FIXTURES/builtin-facts.sh" <<'EOF2'
#!/usr/bin/env bash
cat <<'FACTS'
--- declared capabilities ---
Supports: []{Hook, Skill},
--- default output paths ---
defaultSettingsFile = ".gemini/settings.json"
--- adapter package doc (what we claim the tool does) ---
// Gemini CLI loads `PreCompress` hooks from `.gemini/settings.json`.
--- shipped built-ins this target emits ---
handoff-hook: Hook handoff-pre-compact, event PreCompact (internal/builtins/data/handoff-hook/hooks/handoff-pre-compact.yaml)
event: PreCompact
x-gemini:
  event: PreCompress
targets: [claude, codex, gemini, qoder, factory]
handoff: Skill handoff (internal/builtins/data/handoff/skills/handoff/SKILL.md)
name: handoff
targets: [claude]
--- docs/site/content/docs/target-behavior.md lines ---
12:| gemini | hooks | settings lifecycle hooks |
FACTS
EOF2
  chmod +x "$FIXTURES/builtin-facts.sh"
  local out
  out=$(JEV_TARGET_FACTS="$FIXTURES/builtin-facts.sh" bash -c '. "$1"; jev_claims gemini' _ "$JEV")
  assert_not_contains 'PreCompact' "$out"
  assert_not_contains 'handoff-pre-compact' "$out"
  assert_not_contains 'shipped `handoff` skill' "$out"
  assert_not_contains 'targets:' "$out"
  assert_not_contains 'x-gemini:' "$out"
  assert_contains 'gemini natively reads project-scoped hook configuration' "$out"
  assert_contains 'gemini natively reads project-scoped skill configuration' "$out"
  assert_contains 'gemini reads its settings file from `.gemini/settings.json`' "$out"
  assert_contains 'Gemini CLI loads `PreCompress` hooks from `.gemini/settings.json`.' "$out"
  assert_contains '| gemini | hooks | settings lifecycle hooks |' "$out"
  assert_equals 5 "$(printf '%s\n' "$out" | wc -l | tr -d ' ')"
}
