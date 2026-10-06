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
jq '. as $r | {model: "jev-stub", usage: {input_tokens: 100, output_tokens: 10}, answers: (.questions | with_entries(.key as $k | .value |= (
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
  end)))}' "$data" >"$out"
printf '%s' "$code"
EOF
  chmod +x "$FIXTURES/bin/curl"
  export FAKE_CURL_LOG="$FIXTURES/curl"
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

# triage [args...] runs the script with the fake curl first on PATH and
# prints its stdout and stderr together.
function triage() {
  PATH="$FIXTURES/bin:$PATH" TYPESAFE_API_KEY="${KEY-$SECRET}" JEV_BACKOFF=0 \
    bash "$JEV" "$@" 2>&1
}

# ---- without Jev -------------------------------------------------------------

function test_no_key_writes_lexical_leads_and_exits_0() {
  local out code
  out=$(KEY='' triage "$RUN")
  code=$?
  assert_equals 0 "$code"
  assert_contains "jev-triage: lexical leads only (no TYPESAFE_API_KEY)" "$out"
  assert_file_not_exists "$FAKE_CURL_LOG.calls"
  assert_equals "paired lexical" "$(cut -f5,8 "$RUN/triage.tsv" | sort -u | tr '\t' ' ')"
  assert_contains 'reads its CLIMCP file from `.github/mcp.json`' "$(grep https://example.test/mcp "$RUN/triage.tsv")"
  assert_contains 'reads its hooks directory from `.github/hooks`' "$(grep https://example.test/changelog "$RUN/triage.tsv")"
}

function test_the_lexical_flag_sends_nothing_even_with_a_key() {
  local out
  out=$(triage --lexical "$RUN")
  assert_contains "lexical leads only (--lexical)" "$out"
  assert_file_not_exists "$FAKE_CURL_LOG.calls"
  assert_file_exists "$RUN/triage.tsv"
}

function test_lexical_leads_skip_unpaired_pages_and_rows_with_no_delta() {
  KEY='' triage "$RUN" >/dev/null
  assert_not_contains "https://example.test/cli" "$(cat "$RUN/triage.tsv")"
  assert_not_contains "https://example.test/new" "$(cat "$RUN/triage.tsv")"
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
  assert_matches "missed +contradicts +says_nothing +0.05 +no +MISS +- +4" "$out"
  assert_contains "leads (contradicts or p_contradicts >= 0.4): 1/2 drifts, 0/2 other cases" "$out"
}

function test_replay_asks_each_case_beside_the_target_claims() {
  write_cases
  JEV_JOBS=1 triage --replay "$FIXTURES/cases.tsv" >/dev/null
  local req="$FAKE_CURL_LOG.request.1"
  assert_equals 'Copilot CLI reads `.github/mcp.json`.' "$(jq -r .state.claims.c0 "$req")"
  assert_equals 8 "$(jq '.questions | length' "$req")"
}

function test_replay_without_a_key_reports_lexical_pairing_only() {
  write_cases
  local out code
  out=$(KEY='' triage --replay "$FIXTURES/cases.tsv")
  code=$?
  assert_equals 0 "$code"
  assert_contains "jev-triage: lexical only (no TYPESAFE_API_KEY)" "$out"
  assert_contains "lexical pairing on contradicts: 1/2 (0.50)" "$out"
  assert_not_contains "recall on contradicts" "$out"
  assert_file_not_exists "$FAKE_CURL_LOG.calls"
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
