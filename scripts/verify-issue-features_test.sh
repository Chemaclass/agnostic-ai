#!/usr/bin/env bash

FEATURE_ROOT=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
FEATURE_VERIFY="$FEATURE_ROOT/scripts/verify-issue-features.sh"
FEATURE_MAP="$FEATURE_ROOT/.github/issue-features.json"

function set_up() {
  feature_fixture=$(mktemp -d)
  jq '[.groups[].features[] | {name: .label}]' "$FEATURE_MAP" >"$feature_fixture/labels.json"
  printf '[]\n' >"$feature_fixture/issues.json"
}

function tear_down() {
  rm -f "$feature_fixture/labels.json" "$feature_fixture/issues.json" "$feature_fixture/report.json"
  rmdir "$feature_fixture"
}

function verify_fixture() {
  "$FEATURE_VERIFY" --labels-file "$feature_fixture/labels.json" --issues-file "$feature_fixture/issues.json" "$@" >"$feature_fixture/report.json"
}

function test_approved_map_has_43_features_in_11_groups_and_existing_code_pointers() {
  assert_equals 11 "$(jq '.groups | length' "$FEATURE_MAP")"
  assert_equals 43 "$(jq '[.groups[].features[].label] | unique | length' "$FEATURE_MAP")"
  local pointer
  while IFS= read -r pointer; do
    compgen -G "$FEATURE_ROOT/$pointer" >/dev/null
    assert_successful_code "$?" "missing feature code pointer: $pointer"
  done < <(jq -r '.groups[].features[].paths[]' "$FEATURE_MAP")
}

function test_verifies_rest_and_gh_issue_shapes_and_excludes_pull_requests() {
  cat >"$feature_fixture/issues.json" <<'EOF'
[
  {"number":1,"title":"A","createdAt":"2026-11-18T00:00:00Z","labels":[{"name":"sessions:organize"},{"name":"bug"},{"name":"area:sync"},{"name":"tool-load"}]},
  {"number":2,"title":"B","created_at":"2026-11-24T23:59:59Z","labels":["runtime:approvals","priority:high"]},
  {"number":3,"title":"PR","pull_request":{},"labels":[]},
  {"number":4,"title":"Older","createdAt":"2026-11-17T23:59:59Z","labels":[]},
  {"number":5,"title":"Later","createdAt":"2026-11-25T00:00:00Z","labels":[]}
]
EOF
  verify_fixture --created-from 2026-11-18T00:00:00Z --created-before 2026-11-25T00:00:00Z
  assert_successful_code "$?"
  assert_equals 'true,2,1' "$(jq -r '[.valid,.selectedIssues,.excludedPullRequests] | join(",")' "$feature_fixture/report.json")"
  assert_equals 41 "$(jq '.emptyLabels | length' "$feature_fixture/report.json")"
  assert_equals 2 "$(jq '.concentration | length' "$feature_fixture/report.json")"
}

function test_reports_unmapped_multiple_and_unknown_features_without_counting_operational_labels() {
  cat >"$feature_fixture/issues.json" <<'EOF'
[
  {"number":1,"labels":["area:verification","performance","vendor-watch"]},
  {"number":2,"labels":["cli:sync","cli:checks","bug"]},
  {"number":3,"labels":["cli:sync","sessions:unknown"]}
]
EOF
  verify_fixture
  assert_general_error "$?"
  assert_equals 1 "$(jq '.unmapped[0].number' "$feature_fixture/report.json")"
  assert_equals 2 "$(jq '.multiple[0].number' "$feature_fixture/report.json")"
  assert_equals 'sessions:unknown' "$(jq -r '.unknownFeatures[0].unknown[0]' "$feature_fixture/report.json")"
}

function test_reports_missing_and_unexpected_definitions_but_preserves_source_namespaces() {
  jq 'map(select(.name != "cli:sync")) + [{name:"cli:unknown"},{name:"area:sync"},{name:"target-audit"},{name:"tool-load"},{name:"dependencies"}]' "$feature_fixture/labels.json" >"$feature_fixture/report.json"
  mv "$feature_fixture/report.json" "$feature_fixture/labels.json"
  printf '[{"number":1,"labels":["sessions:catalog"]}]\n' >"$feature_fixture/issues.json"
  verify_fixture
  assert_general_error "$?"
  assert_equals 'cli:sync' "$(jq -r '.definitions.missing | join(",")' "$feature_fixture/report.json")"
  assert_equals 'cli:unknown' "$(jq -r '.definitions.unexpected | join(",")' "$feature_fixture/report.json")"
}

function test_reports_concentration_only_above_one_quarter() {
  printf '[{"number":1,"labels":["cli:sync"]},{"number":2,"labels":["cli:init"]},{"number":3,"labels":["cli:import"]},{"number":4,"labels":["cli:checks"]}]\n' >"$feature_fixture/issues.json"
  verify_fixture
  assert_successful_code "$?"
  assert_equals 0 "$(jq '.concentration | length' "$feature_fixture/report.json")"
}

function test_refuses_an_empty_selection() {
  verify_fixture
  assert_general_error "$?"
  assert_equals false "$(jq '.valid' "$feature_fixture/report.json")"
}

function test_refuses_an_invalid_or_reversed_utc_interval() {
  local output
  output=$(verify_fixture --created-from yesterday 2>&1)
  assert_exit_code 2 "$?"
  assert_contains 'invalid UTC interval' "$output"
  output=$(verify_fixture --created-from 2026-11-25T00:00:00Z --created-before 2026-11-18T00:00:00Z 2>&1)
  assert_exit_code 2 "$?"
  assert_contains 'invalid UTC interval' "$output"
}

function test_refuses_invalid_creation_times_before_filtering_the_window() {
  local created output code
  for created in '"not-a-time"' '"2026-11-18"' '"2026-11-18T00:00:00+00:00"' '"2026-02-30T00:00:00Z"' 'null' '1'; do
    jq -n --argjson created "$created" '[
      {number:1, createdAt:"2026-11-18T00:00:00Z", labels:["cli:sync"]},
      {number:2, createdAt:$created, labels:["cli:sync"]}
    ]' >"$feature_fixture/issues.json"
    output=$(verify_fixture --created-from 2026-11-18T00:00:00Z --created-before 2026-11-25T00:00:00Z 2>&1)
    code=$?
    assert_not_equals 0 "$code"
    assert_contains 'issue #2 needs a canonical UTC creation time' "$output"
  done
}

function test_refuses_impossible_calendar_dates_in_the_interval() {
  local output
  output=$(verify_fixture --created-from 2026-02-30T00:00:00Z 2>&1)
  assert_exit_code 2 "$?"
  assert_contains 'invalid UTC interval' "$output"
}
