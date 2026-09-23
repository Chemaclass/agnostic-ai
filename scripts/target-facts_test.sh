#!/usr/bin/env bash
#
# bashunit tests for scripts/target-facts.sh
#
# Run:
#   bashunit scripts/target-facts_test.sh
#
# Pure helpers are tested by sourcing the script (its entry-point guard
# keeps main() from running). The invariant that matters most is that
# every target list derives from the adapter registry: a new adapter must
# show up in --list and in exactly one --batches group without anyone
# editing this script or the target-audit skill.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/target-facts.sh"

# ---- list_targets ------------------------------------------------------------

function test_list_targets_matches_the_registry() {
  local from_script from_go
  from_script=$(list_targets | sort | tr '\n' ' ')
  from_go=$(grep -E '^\s+"[a-z]+":\s+[a-z]+\.New\(\),$' "$REGISTRY" |
    sed -e 's/^[[:space:]]*"//' -e 's/".*$//' | sort | tr '\n' ' ')
  assert_equals "$from_go" "$from_script"
}

function test_list_targets_has_no_duplicates() {
  local total unique
  total=$(list_targets | wc -l | tr -d ' ')
  unique=$(list_targets | sort -u | wc -l | tr -d ' ')
  assert_equals "$total" "$unique"
}

function test_list_targets_leads_with_the_highest_churn_vendors() {
  # Registry order is roughly chronological, which is what makes batch 1
  # the fast-moving vendors. Guard that property.
  assert_equals "claude" "$(list_targets | head -1)"
}

# ---- batches -----------------------------------------------------------------

function test_batches_returns_the_requested_group_count() {
  assert_equals 5 "$(batches 5 | wc -l | tr -d ' ')"
}

function test_batches_covers_every_target_exactly_once() {
  local flattened expected
  flattened=$(batches 5 | sed 's/^[0-9]*: //' | tr ' ' '\n' | sort | tr '\n' ' ')
  expected=$(list_targets | sort | tr '\n' ' ')
  assert_equals "$expected" "$flattened"
}

function test_batches_spreads_the_remainder_evenly() {
  # 25 targets over 4 groups is 7/6/6/6, never 6/6/6/7 or a 4-wide gap.
  local sizes
  sizes=$(batches 4 | sed 's/^[0-9]*: //' | awk '{ print NF }' | tr '\n' ' ')
  assert_equals "7 6 6 6 " "$sizes"
}

function test_batches_clamps_a_count_above_the_target_total() {
  local total
  total=$(list_targets | wc -l | tr -d ' ')
  assert_equals "$total" "$(batches 999 | wc -l | tr -d ' ')"
}

function test_batches_clamps_a_zero_count_to_one_group() {
  assert_equals 1 "$(batches 0 | wc -l | tr -d ' ')"
}

# ---- pkg_for / src_for -------------------------------------------------------

function test_pkg_for_resolves_a_matching_package_name() {
  assert_equals "claude" "$(pkg_for claude)"
}

function test_pkg_for_resolves_a_package_that_differs_from_the_target() {
  assert_equals "continueai" "$(pkg_for continue)"
}

function test_pkg_for_is_empty_for_an_unknown_target() {
  assert_empty "$(pkg_for definitely-not-a-target)"
}

function test_src_for_finds_a_non_test_source_file() {
  assert_contains "internal/adapters/claude/claude.go" "$(src_for claude)"
}

# ---- fact extraction ---------------------------------------------------------

function test_caps_extracts_the_declared_spec_kinds() {
  assert_contains "Rule" "$(caps "$(src_for claude)")"
}

function test_defaults_extracts_the_const_block_verbatim() {
  # gofmt aligns the `=` in a const block; the dump keeps that alignment
  # so a reader can match it against the source at a glance.
  local out
  out=$(defaults "$(src_for claude)")
  assert_contains '.claude/rules' "$out"
  assert_contains '.mcp.json' "$out"
}

function test_doc_comment_stops_at_the_package_clause() {
  local out
  out=$(doc_comment "$(src_for claude)")
  assert_contains "Package claude" "$out"
  assert_not_contains "package claude" "$out"
}

function test_doc_section_extracts_only_the_requested_target() {
  local out
  out=$(doc_section zed)
  assert_contains "context_servers" "$out"
  assert_not_contains "### Warp" "$out"
}

# ---- dump_target -------------------------------------------------------------

function test_dump_target_rejects_an_unknown_target() {
  dump_target definitely-not-a-target 2>/dev/null
  assert_equals 1 $?
}

function test_dump_target_emits_every_section_for_a_real_target() {
  local out
  out=$(dump_target kilo)
  assert_contains "TARGET: kilo" "$out"
  assert_contains "declared capabilities" "$out"
  assert_contains "default output paths" "$out"
  assert_contains "docs/site/content/docs/target-behavior.md lines" "$out"
  assert_contains "**kilo**" "$out"
  assert_contains "docs/site/content/docs/targets/kilo.md" "$out"
  assert_contains "# Kilo" "$out"
}

# ---- coverage of the audit source list ---------------------------------------

function test_source_sections_contains_only_requested_targets() {
  local out
  out=$(source_sections zed junie)
  assert_contains "## zed" "$out"
  assert_contains "## junie" "$out"
  assert_contains "docs:" "$out"
  assert_not_contains "## warp" "$out"
  assert_not_contains "## claude" "$out"
}

function test_source_sections_rejects_unknown_target_before_printing() {
  local out status=0
  out=$(source_sections zed definitely-not-a-target 2>/dev/null) || status=$?
  assert_equals 1 "$status"
  assert_empty "$out"
}

function test_source_sections_requires_a_target() {
  local status=0
  source_sections 2>/dev/null || status=$?
  assert_equals 2 "$status"
}

function test_source_sections_does_not_repeat_duplicate_targets() {
  local count
  count=$(source_sections zed zed | grep -c '^## zed$')
  assert_equals 1 "$count"
}

function test_every_registered_target_has_an_upstream_sources_entry() {
  # The Go test in tests/integration owns this invariant; this mirror
  # keeps `make test-shell` honest when the sources file is edited by
  # hand between Go runs.
  local sources missing t
  sources="$ROOT/.agnostic-ai/skills/target-audit/references/sources.md"
  missing=""
  for t in $(list_targets); do
    grep -q "^## $t\$" "$sources" || missing="$missing $t"
  done
  assert_empty "$missing"
}

# ---- batch_list --------------------------------------------------------------

function test_batch_list_spreads_the_remainder_like_batches() {
  local sizes
  sizes=$(batch_list 3 a b c d e f g | sed 's/^[0-9]*: //' | awk '{ print NF }' | tr '\n' ' ')
  assert_equals "3 2 2 " "$sizes"
}

function test_batch_list_clamps_a_count_above_the_input() {
  assert_equals 2 "$(batch_list 9 a b | wc -l | tr -d ' ')"
}

function test_batch_list_prints_nothing_without_targets() {
  assert_empty "$(batch_list 3)"
}

# ---- changed_classes ---------------------------------------------------------

# write_run <row>... builds a docfetch.tsv from "target kind url status" tuples.
function write_run() {
  local row
  : >"$CHANGED_RUN"
  for row in "$@"; do
    set -- $row
    printf '%s\t%s\t%s\t200\thtml\tsha-%s\t2026-09-01\t%s\t%s\t\n' \
      "$1" "$2" "$3" "$3" "$4" "$3" >>"$CHANGED_RUN"
  done
}

function set_up() {
  CHANGED_DIR=$(mktemp -d)
  CHANGED_RUN="$CHANGED_DIR/docfetch.tsv"
  export TARGET_AUDIT_LOCK="$CHANGED_DIR/sources.lock"
  LOCK="$TARGET_AUDIT_LOCK"
}

function tear_down() {
  [ -n "${CHANGED_DIR:-}" ] && rm -rf "$CHANGED_DIR"
}

function test_changed_classes_marks_a_moved_page_deep() {
  write_run "claude docs https://a/1 changed" "zed docs https://z/1 unchanged" "zed changelog https://z/c unchanged"
  local out
  out=$(changed_classes "$CHANGED_RUN")
  assert_contains "deep: claude" "$out"
  assert_contains "sweep: zed" "$out"
}

function test_changed_classes_marks_a_moved_changelog_deep() {
  write_run "zed docs https://z/1 unchanged" "zed changelog https://z/c changed"
  assert_contains "deep: zed" "$(changed_classes "$CHANGED_RUN")"
}

function test_changed_classes_marks_an_unrecovered_page_deep() {
  write_run "kiro docs https://k/1 failed" "kiro changelog https://k/c unchanged"
  assert_contains "deep: kiro" "$(changed_classes "$CHANGED_RUN")"
}

function test_changed_classes_sweeps_a_fully_unchanged_target() {
  write_run "zed docs https://z/1 unchanged" "zed changelog https://z/c unchanged"
  local out
  out=$(changed_classes "$CHANGED_RUN")
  assert_equals "sweep: zed" "$out"
}

function test_changed_classes_ignores_a_target_absent_from_the_run() {
  write_run "zed docs https://z/1 unchanged" "zed changelog https://z/c unchanged"
  assert_not_contains "claude" "$(changed_classes "$CHANGED_RUN")"
}

function test_changed_classes_derives_status_from_the_lock_when_absent() {
  # A seven-column file predates the status column; the lock decides.
  printf 'zed\tdocs\thttps://z/1\t200\thtml\tsha-1\t2026-09-01\n' >"$CHANGED_RUN"
  assert_contains "deep: zed" "$(changed_classes "$CHANGED_RUN")"
  printf 'zed\tdocs\thttps://z/1\t200\thtml\tsha-1\t2026-09-01\n' >"$LOCK"
  assert_contains "sweep: zed" "$(changed_classes "$CHANGED_RUN")"
}

function test_changed_classes_rejects_an_unreadable_file() {
  local status=0
  changed_classes "$CHANGED_DIR/missing.tsv" 2>/dev/null || status=$?
  assert_equals 1 "$status"
}

# ---- --changed ---------------------------------------------------------------

function test_changed_prints_numbered_deep_batches_then_the_sweep() {
  write_run "claude docs https://a/1 changed" "cursor docs https://c/1 changed" \
    "zed docs https://z/1 unchanged" "zed changelog https://z/c unchanged"
  local out
  out=$(main --changed "$CHANGED_RUN" 2)
  assert_contains "1: claude" "$out"
  assert_contains "2: cursor" "$out"
  assert_contains "sweep: zed" "$out"
}

function test_changed_omits_the_sweep_line_when_every_target_moved() {
  write_run "claude docs https://a/1 changed"
  assert_not_contains "sweep:" "$(main --changed "$CHANGED_RUN")"
}

function test_changed_clamps_the_batch_count_to_the_deep_targets() {
  write_run "claude docs https://a/1 changed" "zed docs https://z/1 unchanged" "zed changelog https://z/c unchanged"
  assert_equals 1 "$(main --changed "$CHANGED_RUN" 5 | grep -c '^[0-9]*:')"
}

function test_changed_requires_a_file() {
  local status=0
  main --changed 2>/dev/null || status=$?
  assert_equals 2 "$status"
}

function test_changed_rejects_a_missing_file() {
  local status=0
  main --changed "$CHANGED_DIR/missing.tsv" 2>/dev/null || status=$?
  assert_equals 1 "$status"
}
