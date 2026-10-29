#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
METRICS="$SCRIPT_DIR/audit-metrics.sh"
RUN=""

function set_up() {
  RUN=$(mktemp -d)
}

function tear_down() {
  [ -z "$RUN" ] || rm -rf "$RUN"
}

function metrics() {
  bash "$METRICS" "$@"
}

function test_overlapping_stages_and_delivery_keep_the_audit_wall_span() {
  metrics init "$RUN" fixture '{"scenario":"vendor-change","audited_commit":"abc"}' 100
  metrics record "$RUN" stage '{"id":"fetch","phase":"audit","name":"fetch","start":100,"end":110,"status":"completed"}'
  metrics record "$RUN" stage '{"id":"review-a","phase":"audit","name":"review","start":105,"end":125,"status":"completed"}'
  metrics record "$RUN" stage '{"id":"review-b","phase":"audit","name":"review","start":110,"end":120,"status":"interrupted"}'
  metrics record "$RUN" stage '{"id":"fix","phase":"delivery","name":"implementation","start":150,"end":200,"status":"completed"}'
  metrics finish "$RUN" 130
  local summary
  summary=$(metrics report "$RUN")
  assert_contains 'Audit elapsed: 30 seconds; recorded stage union: 25 seconds; overlap: 15 seconds.' "$summary"
  assert_contains 'Delivery: 1 recorded stages, reported separately.' "$summary"
  assert_contains 'review-b | audit | 110 | 120 | 10 | interrupted' "$summary"
  assert_contains 'Auditor input tokens: unknown; output tokens: unknown; effort seconds: unknown.' "$summary"
}

function test_running_stage_stays_unknown_until_an_explicit_end() {
  metrics init "$RUN" fixture '{}' 100
  metrics record "$RUN" stage '{"id":"review","phase":"audit","name":"review","start":105,"end":null,"status":"running"}'
  local summary
  summary=$(metrics report "$RUN")
  assert_contains 'Audit elapsed: unknown' "$summary"
  assert_contains 'review | audit | 105 | unknown | unknown | running' "$summary"
  assert_equals null "$(jq '.audit.end' "$RUN/audit-metrics.json")"
}

function test_routing_counts_keep_samples_and_builtin_overrides_separate() {
  metrics init "$RUN" fixture '{"scenario":"builtin-only"}' 100
  metrics record "$RUN" review '{"target":"codex","decision":"deep","reason":"built-in invalidation","builtin_override":true}'
  metrics record "$RUN" review '{"target":"claude","decision":"deep","reason":"built-in invalidation","builtin_override":true}'
  assert_contains 'Reviews: 2 planned, 0 omitted under an approved fast path, 0 sampled, 2 built-in overrides.' "$(metrics report "$RUN")"
  metrics record "$RUN" review '{"target":"cursor","decision":"omitted","reason":"eligible unchanged coverage","approval":"approved run policy"}'
  metrics record "$RUN" review '{"target":"kiro","decision":"sampled","reason":"random eligible target","approval":"approved run policy"}'
  assert_contains 'Reviews: 4 planned, 1 omitted under an approved fast path, 1 sampled, 2 built-in overrides.' "$(metrics report "$RUN")"
  local code=0
  metrics record "$RUN" review '{"target":"unsafe","decision":"omitted","reason":"clear"}' 2>/dev/null || code=$?
  assert_not_equals 0 "$code"
}

function test_findings_link_known_leads_and_leave_unknown_values_null() {
  metrics init "$RUN" fixture '{}' 100
  metrics record "$RUN" finding '{"id":"confirmed","outcome":"confirmed","lead":{"file":"triage.tsv","line":2},"evidence":"finding.md"}'
  metrics record "$RUN" finding '{"id":"rejected","outcome":"rejected","evidence":"rejection.md"}'
  assert_equals null "$(jq '.findings[1].lead' "$RUN/audit-metrics.json")"
  assert_contains 'Findings: 1 confirmed, 1 rejected, 0 unresolved; 1 linked to a known lead.' "$(metrics report "$RUN")"
  metrics record "$RUN" usage '{"id":"auditor-a","input_tokens":0,"output_tokens":0,"effort_seconds":0}'
  assert_contains 'Auditor input tokens: 0; output tokens: 0; effort seconds: 0.' "$(metrics report "$RUN")"
  metrics record "$RUN" usage '{"id":"auditor-b"}'
  assert_contains 'Auditor input tokens: unknown; output tokens: unknown; effort seconds: unknown.' "$(metrics report "$RUN")"
}

function test_invalid_interval_cannot_replace_the_artifact() {
  metrics init "$RUN" fixture '{}' 100
  local code=0
  metrics record "$RUN" stage '{"id":"bad","phase":"audit","name":"review","start":110,"end":105,"status":"completed"}' 2>/dev/null || code=$?
  assert_not_equals 0 "$code"
  assert_equals 0 "$(jq '.stages | length' "$RUN/audit-metrics.json")"
}

function test_report_rejects_triage_from_another_run_or_schema() {
  metrics init "$RUN" current '{}' 100
  printf '%s\n' '{"schema_version":1,"run":{"id":"old"},"mode":"old-run-mode"}' >"$RUN/triage-metrics.json"
  local summary
  summary=$(metrics report "$RUN")
  assert_contains 'Triage metrics: unavailable.' "$summary"
  assert_contains 'triage metrics run ID does not match' "$summary"
  assert_not_contains old-run-mode "$summary"
  printf '%s\n' '{"schema_version":2,"run":{"id":"current"}}' >"$RUN/triage-metrics.json"
  summary=$(metrics report "$RUN")
  assert_contains 'Triage metrics: unavailable.' "$summary"
  assert_contains 'triage metrics schema does not match' "$summary"
}

function test_report_marks_malformed_triage_unavailable_without_failing() {
  metrics init "$RUN" current '{}' 100
  local fixture summary code
  for fixture in '{' 'null' '{"schema_version":1,"run":{"id":"current"}}'; do
    printf '%s\n' "$fixture" >"$RUN/triage-metrics.json"
    code=0
    summary=$(metrics report "$RUN" 2>/dev/null) || code=$?
    assert_equals 0 "$code"
    assert_contains 'Triage metrics: unavailable.' "$summary"
    assert_contains 'triage metrics are malformed' "$summary"
  done
}

function test_audit_stage_bounds_reject_records_outside_the_run() {
  metrics init "$RUN" fixture '{}' 100
  local code=0
  metrics record "$RUN" stage '{"id":"before","phase":"audit","name":"review","start":99,"end":110,"status":"completed"}' 2>/dev/null || code=$?
  assert_not_equals 0 "$code"
  metrics finish "$RUN" 120
  code=0
  metrics record "$RUN" stage '{"id":"after","phase":"audit","name":"review","start":110,"end":121,"status":"completed"}' 2>/dev/null || code=$?
  assert_not_equals 0 "$code"
  code=0
  metrics record "$RUN" stage '{"id":"running-after","phase":"audit","name":"review","start":121,"end":null,"status":"running"}' 2>/dev/null || code=$?
  assert_not_equals 0 "$code"
  assert_equals 0 "$(jq '.stages | length' "$RUN/audit-metrics.json")"
  metrics record "$RUN" stage '{"id":"delivery","phase":"delivery","name":"implementation","start":90,"end":130,"status":"completed"}'
  assert_equals 1 "$(jq '.stages | length' "$RUN/audit-metrics.json")"
}

function test_audit_finish_cannot_precede_observed_stage_bounds() {
  metrics init "$RUN" fixture '{}' 100
  metrics record "$RUN" stage '{"id":"review","phase":"audit","name":"review","start":100,"end":130,"status":"completed"}'
  local code=0
  metrics finish "$RUN" 110 2>/dev/null || code=$?
  assert_not_equals 0 "$code"
  assert_equals null "$(jq '.audit.end' "$RUN/audit-metrics.json")"
  metrics record "$RUN" stage '{"id":"incomplete","phase":"audit","name":"review","start":140,"end":null,"status":"running"}'
  code=0
  metrics finish "$RUN" 135 2>/dev/null || code=$?
  assert_not_equals 0 "$code"
  metrics finish "$RUN" 150
  assert_equals null "$(jq '.stages[1].end' "$RUN/audit-metrics.json")"
  assert_contains 'incomplete | audit | 140 | unknown | unknown | running' "$(metrics report "$RUN")"
}

function test_restarted_wave_shows_per_batch_attempts_and_their_usage() {
  metrics init "$RUN" fixture '{}' 100
  metrics record "$RUN" stage '{"id":"review-batch-1-w1","phase":"audit","name":"review","batch":"batch-1","wave":1,"start":100,"end":400,"status":"interrupted"}'
  metrics record "$RUN" stage '{"id":"review-batch-2-w1","phase":"audit","name":"review","batch":"batch-2","wave":1,"start":100,"end":300,"status":"completed"}'
  metrics record "$RUN" stage '{"id":"review-batch-1-w2","phase":"audit","name":"review","batch":"batch-1","wave":2,"start":410,"end":500,"status":"completed"}'
  metrics record "$RUN" usage '{"id":"Frodo","batch":"batch-1","wave":1,"input_tokens":900,"output_tokens":90}'
  metrics record "$RUN" usage '{"id":"Frodo-2","batch":"batch-1","wave":2,"input_tokens":200,"output_tokens":20}'
  metrics record "$RUN" usage '{"id":"checker","batch":"batch-1","wave":2,"input_tokens":100,"output_tokens":10}'
  metrics record "$RUN" usage '{"id":"Sam","batch":"batch-2","wave":1}'
  local summary
  summary=$(metrics report "$RUN")
  assert_contains 'Waves: 2 batches, 3 waves, 1 restarted batches.' "$summary"
  assert_contains 'batch-1 | 1 | 300 | interrupted | 900 | 90' "$summary"
  assert_contains 'batch-1 | 2 | 90 | completed | 300 | 30' "$summary"
  assert_contains 'batch-2 | 1 | 200 | completed | unknown | unknown' "$summary"
}

function test_wave_needs_a_batch_and_stays_unique_per_batch() {
  metrics init "$RUN" fixture '{}' 100
  local code=0
  metrics record "$RUN" stage '{"id":"no-batch","phase":"audit","name":"review","wave":1,"start":100,"end":110,"status":"completed"}' 2>/dev/null || code=$?
  assert_not_equals 0 "$code"
  code=0
  metrics record "$RUN" stage '{"id":"zero","phase":"audit","name":"review","batch":"batch-1","wave":0,"start":100,"end":110,"status":"completed"}' 2>/dev/null || code=$?
  assert_not_equals 0 "$code"
  metrics record "$RUN" stage '{"id":"first","phase":"audit","name":"review","batch":"batch-1","wave":1,"start":100,"end":110,"status":"completed"}'
  code=0
  metrics record "$RUN" stage '{"id":"again","phase":"audit","name":"review","batch":"batch-1","wave":1,"start":110,"end":120,"status":"completed"}' 2>/dev/null || code=$?
  assert_not_equals 0 "$code"
  metrics record "$RUN" stage '{"id":"first","phase":"audit","name":"review","batch":"batch-1","wave":1,"start":100,"end":115,"status":"completed"}'
  assert_equals 1 "$(jq '.stages | length' "$RUN/audit-metrics.json")"
  assert_contains 'Waves: 1 batches, 1 waves, 0 restarted batches.' "$(metrics report "$RUN")"
}

function test_report_without_waves_says_so() {
  metrics init "$RUN" fixture '{}' 100
  assert_contains 'Waves: none recorded.' "$(metrics report "$RUN")"
}
