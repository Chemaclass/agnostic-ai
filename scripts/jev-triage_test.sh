#!/usr/bin/env bash
#
# bashunit tests for scripts/jev-triage.sh
#
# Run:
#   bashunit scripts/jev-triage_test.sh
#
# No test reaches TypeSafe. A fake curl on PATH records its arguments and
# stdin, then answers from FAKE_CURL_CODE. A 2xx answer picks each Choice
# from the claim text: "mcp.json" contradicts, "hooks" supports, anything
# else says nothing. The claims come from a stub facts script.

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
jq '{model: "jev-stub", answers: (.questions | with_entries(.value |= (
  if .type == "noul" then {type: "noul", noul: 0.85}
  else
    ((.instructions.claim | ascii_downcase) as $c
      | (if ($c | contains("mcp.json")) then "contradicts"
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

# ---- backward compatibility --------------------------------------------------

function test_no_key_skips_writes_nothing_and_exits_0() {
  local out code
  out=$(KEY='' triage "$RUN")
  code=$?
  assert_equals 0 "$code"
  assert_equals "jev-triage: skipped (no TYPESAFE_API_KEY)" "$out"
  assert_file_not_exists "$RUN/triage.tsv"
  assert_file_not_exists "$FAKE_CURL_LOG.calls"
}

function test_no_key_removes_a_triage_left_by_an_earlier_run() {
  echo stale >"$RUN/triage.tsv"
  KEY='' triage "$RUN" >/dev/null
  assert_file_not_exists "$RUN/triage.tsv"
}

function test_overloaded_api_retries_then_skips_without_a_partial_file() {
  local out code
  out=$(FAKE_CURL_CODE=529 JEV_JOBS=1 triage "$RUN")
  code=$?
  assert_equals 0 "$code"
  assert_contains "jev-triage: skipped (HTTP 529 after 3 tries)" "$out"
  assert_file_not_exists "$RUN/triage.tsv"
  assert_file_not_exists "$RUN/triage.tsv.tmp"
  assert_equals 3 "$(wc -l <"$FAKE_CURL_LOG.calls" | tr -d ' ')"
}

function test_unauthorized_skips_without_retrying() {
  local out
  out=$(FAKE_CURL_CODE=401 JEV_JOBS=1 triage "$RUN")
  assert_contains "jev-triage: skipped (HTTP 401)" "$out"
  assert_equals 1 "$(wc -l <"$FAKE_CURL_LOG.calls" | tr -d ' ')"
  assert_file_not_exists "$RUN/triage.tsv"
}

function test_unreachable_host_skips_with_the_real_curl() {
  local out code
  out=$(TYPESAFE_API_URL=http://127.0.0.1:9/v1/systemone TYPESAFE_API_KEY="$SECRET" \
    JEV_BACKOFF=0 JEV_TRIES=2 JEV_TIMEOUT=5 bash "$JEV" "$RUN" 2>&1)
  code=$?
  assert_equals 0 "$code"
  assert_contains "jev-triage: skipped (no response from http://127.0.0.1:9/v1/systemone after 2 tries)" "$out"
  assert_file_not_exists "$RUN/triage.tsv"
}

function test_missing_deltas_skips() {
  rm "$RUN/deltas.tsv"
  assert_equals "jev-triage: skipped (no deltas.tsv in $RUN)" "$(triage "$RUN")"
  assert_file_not_exists "$RUN/triage.tsv"
}

# ---- happy path --------------------------------------------------------------

function test_writes_rows_with_strong_contradicts_first() {
  triage "$RUN" >/dev/null
  assert_file_exists "$RUN/triage.tsv"
  local first
  first=$(head -n 1 "$RUN/triage.tsv")
  assert_contains $'copilot\thttps://example.test/mcp\t' "$first"
  assert_contains $'\tcontradicts\t0.9' "$first"
  assert_equals "contradicts" "$(cut -f5 "$RUN/triage.tsv" | sed -n 1p)"
  assert_equals "contradicts noul supports" "$(cut -f5 "$RUN/triage.tsv" | awk '!seen[$0]++' | tr '\n' ' ' | sed 's/ $//')"
}

function test_pairs_each_region_only_with_claims_that_share_a_path() {
  triage "$RUN" >/dev/null
  assert_contains 'reads its CLIMCP file from `.github/mcp.json`' "$(grep https://example.test/mcp "$RUN/triage.tsv")"
  assert_not_contains "hooks directory" "$(grep https://example.test/mcp "$RUN/triage.tsv")"
  assert_contains 'reads its hooks directory from `.github/hooks`' "$(grep https://example.test/changelog "$RUN/triage.tsv")"
}

function test_a_changelog_region_carries_one_noul_row() {
  triage "$RUN" >/dev/null
  local noul
  noul=$(awk -F '\t' '$5 == "noul"' "$RUN/triage.tsv")
  assert_equals 1 "$(printf '%s\n' "$noul" | grep -c .)"
  assert_contains $'https://example.test/changelog\t' "$noul"
  assert_contains $'\tnoul\t0.85' "$noul"
}

function test_regions_with_no_paired_claim_and_rows_with_no_delta_send_nothing() {
  triage "$RUN" >/dev/null
  assert_equals 2 "$(wc -l <"$FAKE_CURL_LOG.calls" | tr -d ' ')"
  assert_not_contains "https://example.test/cli" "$(cat "$RUN/triage.tsv")"
  assert_not_contains "https://example.test/new" "$(cat "$RUN/triage.tsv")"
}

function test_one_request_carries_every_paired_claim_as_its_own_choice() {
  JEV_JOBS=1 triage "$RUN" >/dev/null
  local req="$FAKE_CURL_LOG.request.1"
  assert_equals "jev-latest" "$(jq -r .model "$req")"
  assert_equals "choice" "$(jq -r '[.questions[].type] | unique | join(",")' "$req")"
  assert_equals 2 "$(jq '.questions | length' "$req")"
  assert_equals "supports,contradicts,says_nothing" "$(jq -r '[.questions[] | .criteria | keys_unsorted] | first | join(",")' "$req")"
  assert_contains '[-`.github/mcp.json`-]' "$(jq -r .state.vendor_change "$req")"
}

function test_the_target_filter_skips_other_targets() {
  printf 'zed\tdocs\thttps://example.test/zed\tprose\t%s\n' "$RUN/pages/copilot/docs-1.delta" >>"$RUN/deltas.tsv"
  triage "$RUN" copilot >/dev/null
  assert_not_contains "https://example.test/zed" "$(cat "$RUN/triage.tsv")"
}

function test_the_request_cap_bounds_calls() {
  local out
  out=$(JEV_MAX_REQUESTS=1 triage "$RUN")
  assert_equals 1 "$(wc -l <"$FAKE_CURL_LOG.calls" | tr -d ' ')"
  assert_contains "1 over the request cap" "$out"
}

function test_the_key_never_reaches_argv_or_output() {
  local out
  out=$(triage "$RUN")
  assert_not_contains "$SECRET" "$out"
  assert_not_contains "$SECRET" "$(cat "$RUN/triage.tsv")"
  assert_not_contains "$SECRET" "$(cat "$FAKE_CURL_LOG.args")"
  assert_contains "header = \"Authorization: Bearer $SECRET\"" "$(cat "$FAKE_CURL_LOG.stdin")"
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

function test_replay_prints_recall_and_false_positive_rate() {
  write_cases
  local out
  out=$(triage --replay "$FIXTURES/cases.tsv")
  assert_contains "recall on contradicts: 1/2 (0.50)" "$out"
  assert_contains "false positives on says_nothing: 0/1 (0.00)" "$out"
  assert_contains "exact verdicts: 3/4" "$out"
  assert_matches "missed +contradicts +says_nothing +0.9 +MISS 4" "$out"
}

function test_replay_without_a_key_skips_and_exits_0() {
  write_cases
  local out code
  out=$(KEY='' triage --replay "$FIXTURES/cases.tsv")
  code=$?
  assert_equals 0 "$code"
  assert_equals "jev-triage: skipped (no TYPESAFE_API_KEY)" "$out"
}

function test_the_committed_cases_file_is_well_formed() {
  local cases="$SCRIPT_DIR/target-audit/triage-cases.tsv"
  assert_file_exists "$cases"
  assert_empty "$(awk -F '\t' '!/^#/ && NF != 6 { print NR ": " NF " columns" }' "$cases")"
  assert_empty "$(awk -F '\t' '!/^#/ && $5 !~ /^(supports|contradicts|says_nothing)$/ { print NR ": " $5 }' "$cases")"
  assert_empty "$(awk -F '\t' '!/^#/ && seen[$1]++ { print "duplicate " $1 }' "$cases")"
}
