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
  assert_contains "docs/user/targets.md rows" "$out"
}

# ---- coverage of the audit source list ---------------------------------------

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
