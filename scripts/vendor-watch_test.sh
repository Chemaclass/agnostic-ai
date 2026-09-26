#!/usr/bin/env bash
#
# bashunit tests for scripts/vendor-watch.sh
#
# Run:
#   bashunit scripts/vendor-watch_test.sh
#
# No test reaches GitHub. The publish tests replace `gh` with a function that
# records each call and serves a canned issue list and body.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/vendor-watch.sh"

FIXTURES=""

function set_up() {
  FIXTURES=$(mktemp -d)
  GH_CALLS="$FIXTURES/gh.calls"
  GH_OPEN_ISSUE=""
  GH_ISSUE_BODY=""
  : >"$GH_CALLS"
}

function tear_down() {
  [ -n "$FIXTURES" ] && rm -rf "$FIXTURES"
}

# row <target> <url> <sha> <status> [http] prints one ten-column docfetch row.
function row() {
  printf '%s\tdocs\t%s\t%s\thtml\t%s\t2026-09-24\t%s\t%s\tpages/x\n' \
    "$1" "$2" "${5:-200}" "$3" "$4" "$2"
}

function run_tsv() {
  {
    row kiro https://kiro.dev/docs/steering/ aaa changed
    row kiro https://kiro.dev/docs/hooks/ bbb unchanged
    row cursor https://cursor.com/docs/rules ccc new
    row zed https://zed.dev/docs/ai/mcp - failed 404
  } >"$FIXTURES/docfetch.tsv"
  printf '%s' "$FIXTURES/docfetch.tsv"
}

# ---- report ------------------------------------------------------------------

function test_report_lists_moved_rows_grouped_by_target() {
  local out
  out=$(vendor_watch_report "$(run_tsv)" /dev/null)
  assert_contains "### kiro" "$out"
  assert_contains "- changed: https://kiro.dev/docs/steering/" "$out"
  assert_contains "### cursor" "$out"
  assert_contains "- new: https://cursor.com/docs/rules" "$out"
  assert_contains "- failed (HTTP 404): https://zed.dev/docs/ai/mcp" "$out"
}

function test_report_skips_unchanged_rows() {
  assert_not_contains "https://kiro.dev/docs/hooks/" "$(vendor_watch_report "$(run_tsv)" /dev/null)"
}

function test_report_suggests_an_audit_of_the_moved_targets() {
  # shellcheck disable=SC2016 # the backticks are Markdown, not a command
  assert_contains '`/target-audit cursor kiro zed`' "$(vendor_watch_report "$(run_tsv)" /dev/null)"
}

function test_report_is_empty_when_nothing_moved() {
  row kiro https://kiro.dev/docs/hooks/ bbb unchanged >"$FIXTURES/quiet.tsv"
  assert_empty "$(vendor_watch_report "$FIXTURES/quiet.tsv" /dev/null)"
}

function test_report_skips_rows_already_reported() {
  local tsv out
  tsv=$(run_tsv)
  vendor_watch_keys "$tsv" >"$FIXTURES/seen"
  out=$(vendor_watch_report "$tsv" "$FIXTURES/seen")
  assert_empty "$out"
}

function test_report_names_a_page_again_when_its_hash_moves_again() {
  local tsv
  tsv=$(run_tsv)
  vendor_watch_keys "$tsv" >"$FIXTURES/seen"
  row kiro https://kiro.dev/docs/steering/ ddd changed >"$FIXTURES/next.tsv"
  assert_contains "https://kiro.dev/docs/steering/" "$(vendor_watch_report "$FIXTURES/next.tsv" "$FIXTURES/seen")"
}

function test_report_skips_a_changed_page_whose_text_the_runner_fetched_before() {
  printf 'aaa\nccc\n' >"$FIXTURES/known"
  local out
  out=$(vendor_watch_report "$(run_tsv)" /dev/null "$FIXTURES/known")
  assert_not_contains "https://kiro.dev/docs/steering/" "$out"
  assert_contains "- new: https://cursor.com/docs/rules" "$out"
  assert_contains "- failed (HTTP 404): https://zed.dev/docs/ai/mcp" "$out"
}

function test_known_texts_lists_each_stored_snapshot_hash() {
  mkdir -p "$FIXTURES/snapshots"
  : >"$FIXTURES/snapshots/aaa.txt"
  : >"$FIXTURES/snapshots/bbb.txt"
  : >"$FIXTURES/snapshots/ccc.txt.tmp.123"
  assert_equals "$(printf 'aaa\nbbb')" "$(vendor_watch_known_texts "$FIXTURES/snapshots")"
  assert_empty "$(vendor_watch_known_texts "$FIXTURES/missing")"
}

function test_keys_pair_each_moved_url_with_its_hash() {
  local keys
  keys=$(vendor_watch_keys "$(run_tsv)")
  assert_contains "https://kiro.dev/docs/steering/ aaa" "$(printf '%s' "$keys" | tr '\t' ' ')"
  assert_contains "https://zed.dev/docs/ai/mcp failed-404" "$(printf '%s' "$keys" | tr '\t' ' ')"
  assert_not_contains "hooks" "$keys"
}

function test_marker_rest_keeps_the_body_without_the_marker() {
  local body
  body="intro text
$(vendor_watch_marker "$(printf 'u1\ts1')")
outro"
  assert_equals "$(printf 'intro text\noutro')" "$(printf '%s' "$body" | vendor_watch_marker_keys rest)"
}

function test_publish_keeps_every_reported_key_in_the_refreshed_marker() {
  local tsv
  tsv=$(run_tsv)
  GH_OPEN_ISSUE=42
  GH_ISSUE_BODY="old body
$(vendor_watch_marker "$(printf 'https://old.example/page\tzzz')")"
  vendor_watch_publish "$tsv" >/dev/null
  assert_contains "https://old.example/page" "$(grep 'issue edit 42' "$GH_CALLS" -A20)"
  assert_contains "https://cursor.com/docs/rules" "$(grep 'issue edit 42' "$GH_CALLS" -A20)"
}

function test_marker_round_trips_through_an_issue_body() {
  local body
  body="intro text
$(vendor_watch_marker "$(printf 'u1\ts1\nu2\ts2')")"
  assert_equals "$(printf 'u1\ts1\nu2\ts2')" "$(printf '%s' "$body" | vendor_watch_marker_keys)"
}

# ---- publish -----------------------------------------------------------------

# gh stands in for the GitHub CLI. It records every call and answers the two
# reads publish makes: the open issue number and that issue's body.
function gh() {
  printf '%s\n' "$*" >>"$GH_CALLS"
  case "$1 $2" in
    "issue list") printf '%s' "$GH_OPEN_ISSUE" ;;
    "issue view") printf '%s' "$GH_ISSUE_BODY" ;;
  esac
  return 0
}

function test_publish_opens_an_issue_when_none_is_open() {
  vendor_watch_publish "$(run_tsv)" >/dev/null
  assert_contains "issue create" "$(cat "$GH_CALLS")"
  assert_contains "--label vendor-watch" "$(cat "$GH_CALLS")"
}

function test_publish_comments_on_the_open_issue_with_only_new_pages() {
  local tsv
  tsv=$(run_tsv)
  GH_OPEN_ISSUE=42
  GH_ISSUE_BODY="old body
$(vendor_watch_marker "$(vendor_watch_keys "$tsv" | grep -v cursor)")"
  vendor_watch_publish "$tsv" >/dev/null
  assert_contains "issue comment 42" "$(cat "$GH_CALLS")"
  assert_contains "issue edit 42" "$(cat "$GH_CALLS")"
  assert_not_contains "issue create" "$(cat "$GH_CALLS")"
}

function test_publish_does_nothing_when_every_page_was_reported() {
  local tsv
  tsv=$(run_tsv)
  GH_OPEN_ISSUE=42
  GH_ISSUE_BODY="old body
$(vendor_watch_marker "$(vendor_watch_keys "$tsv")")"
  vendor_watch_publish "$tsv" >/dev/null
  assert_not_contains "issue comment" "$(cat "$GH_CALLS")"
  assert_not_contains "issue create" "$(cat "$GH_CALLS")"
  assert_not_contains "issue edit" "$(cat "$GH_CALLS")"
}

function test_publish_stays_quiet_when_every_changed_text_was_fetched_before() {
  row kiro https://kiro.dev/docs/steering/ aaa changed >"$FIXTURES/flap.tsv"
  printf 'aaa\n' >"$FIXTURES/known"
  vendor_watch_publish "$FIXTURES/flap.tsv" "$FIXTURES/known" >/dev/null
  assert_not_contains "issue create" "$(cat "$GH_CALLS")"
}

function test_publish_does_nothing_on_a_quiet_run() {
  row kiro https://kiro.dev/docs/hooks/ bbb unchanged >"$FIXTURES/quiet.tsv"
  vendor_watch_publish "$FIXTURES/quiet.tsv" >/dev/null
  assert_not_contains "issue create" "$(cat "$GH_CALLS")"
}

# ---- main --------------------------------------------------------------------

function test_main_requires_a_readable_tsv() {
  local status=0
  (vendor_watch_main report "$FIXTURES/missing.tsv" 2>/dev/null) || status=$?
  assert_equals 1 "$status"
}

function test_main_rejects_an_unknown_command() {
  local status=0
  (vendor_watch_main nope 2>/dev/null) || status=$?
  assert_equals 1 "$status"
}

function test_report_tags_each_page_with_its_delta_label() {
  run_tsv >/dev/null
  printf 'kiro\tdocs\thttps://kiro.dev/docs/steering/\tchrome-only\tpages/x.delta\n' >"$FIXTURES/deltas.tsv"
  printf 'cursor\tdocs\thttps://cursor.com/docs/rules\tmentions:.cursor/rules\tpages/y.delta\n' >>"$FIXTURES/deltas.tsv"
  local out
  out=$(vendor_watch_report "$FIXTURES/docfetch.tsv" /dev/null)
  assert_contains '- changed: https://kiro.dev/docs/steering/ (`chrome-only`)' "$out"
  assert_contains '- new: https://cursor.com/docs/rules (`mentions:.cursor/rules`)' "$out"
  assert_contains '- failed (HTTP 404): https://zed.dev/docs/ai/mcp' "$out"
}
