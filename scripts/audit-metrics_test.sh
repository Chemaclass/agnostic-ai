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
