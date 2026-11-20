#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/changelog-assemble.sh"

function set_up() {
  dir="$(mktemp -d)"
  mkdir "$dir/changes"
  printf 'Read me.\n' > "$dir/changes/README.md"
}

function tear_down() {
  rm -rf "$dir"
}

function write_changelog() {
  printf '%s' "$1" > "$dir/CHANGELOG.md"
}

function test_fills_an_empty_unreleased_section() {
  write_changelog $'# Changelog\n\n## [Unreleased]\n\n## v1.0.0 - 2026-01-01\n\n- Old.\n'
  printf '### General\n\n- New (#1).\n' > "$dir/changes/one.md"
  assemble_changelog "$dir/CHANGELOG.md" "$dir/changes" >/dev/null
  assert_successful_code "$?"
  assert_equals $'# Changelog\n\n## [Unreleased]\n\n### General\n\n- New (#1).\n\n## v1.0.0 - 2026-01-01\n\n- Old.' "$(cat "$dir/CHANGELOG.md")"
}

function test_appends_under_existing_headings_and_orders_new_ones() {
  write_changelog $'## [Unreleased]\n\n### General\n\n- Kept (#1).\n\n### Site\n\n- Docs (#2).\n\n## v1.0.0 - 2026-01-01\n'
  printf '### By tool\n\n#### Codex\n\n- Codex (#3).\n' > "$dir/changes/a.md"
  printf '### General\n\n- Added (#4).\n' > "$dir/changes/b.md"
  assemble_changelog "$dir/CHANGELOG.md" "$dir/changes" >/dev/null
  assert_equals $'## [Unreleased]\n\n### General\n\n- Kept (#1).\n- Added (#4).\n\n### By tool\n\n#### Codex\n\n- Codex (#3).\n\n### Site\n\n- Docs (#2).\n\n## v1.0.0 - 2026-01-01' "$(cat "$dir/CHANGELOG.md")"
}

function test_deletes_assembled_files_and_keeps_the_readme() {
  write_changelog $'## [Unreleased]\n'
  printf '### General\n\n- New (#1).\n' > "$dir/changes/one.md"
  assemble_changelog "$dir/CHANGELOG.md" "$dir/changes" >/dev/null
  assert_file_not_exists "$dir/changes/one.md"
  assert_file_exists "$dir/changes/README.md"
}

function test_check_changes_nothing() {
  write_changelog $'## [Unreleased]\n'
  printf '### General\n\n- New (#1).\n' > "$dir/changes/one.md"
  local out
  out="$(assemble_changelog --check "$dir/CHANGELOG.md" "$dir/changes")"
  assert_successful_code "$?"
  assert_contains "1 change file(s) ready" "$out"
  assert_equals "## [Unreleased]" "$(cat "$dir/CHANGELOG.md")"
  assert_file_exists "$dir/changes/one.md"
}

function test_without_change_files_leaves_the_changelog_alone() {
  write_changelog $'## [Unreleased]\n\n### Site\n\n- A.\n\n- B.\n'
  assemble_changelog "$dir/CHANGELOG.md" "$dir/changes" >/dev/null
  assert_successful_code "$?"
  assert_equals $'## [Unreleased]\n\n### Site\n\n- A.\n\n- B.' "$(cat "$dir/CHANGELOG.md")"
}

function test_rejects_an_entry_without_a_heading() {
  write_changelog $'## [Unreleased]\n'
  printf -- '- Lost (#1).\n' > "$dir/changes/bad.md"
  local out
  out="$(assemble_changelog --check "$dir/CHANGELOG.md" "$dir/changes" 2>&1)"
  assert_general_error "$?"
  assert_contains "$dir/changes/bad.md: an entry needs a ### heading above it" "$out"
}

function test_rejects_a_level_two_heading() {
  write_changelog $'## [Unreleased]\n'
  printf '## [Unreleased]\n\n### General\n\n- X.\n' > "$dir/changes/bad.md"
  assemble_changelog --check "$dir/CHANGELOG.md" "$dir/changes" >/dev/null 2>&1
  assert_general_error "$?"
}

function test_rejects_a_file_with_no_entry() {
  write_changelog $'## [Unreleased]\n'
  printf '### General\n' > "$dir/changes/bad.md"
  local out
  out="$(assemble_changelog --check "$dir/CHANGELOG.md" "$dir/changes" 2>&1)"
  assert_general_error "$?"
  assert_contains "bad.md: no entry" "$out"
}

function test_rejects_an_empty_file() {
  write_changelog $'## [Unreleased]\n'
  : > "$dir/changes/empty.md"
  assemble_changelog --check "$dir/CHANGELOG.md" "$dir/changes" >/dev/null 2>&1
  assert_general_error "$?"
}

function test_a_failed_file_leaves_everything_in_place() {
  write_changelog $'## [Unreleased]\n'
  printf '### General\n\n- Good.\n' > "$dir/changes/a.md"
  printf -- '- Bad.\n' > "$dir/changes/b.md"
  assemble_changelog "$dir/CHANGELOG.md" "$dir/changes" >/dev/null 2>&1
  assert_general_error "$?"
  assert_equals "## [Unreleased]" "$(cat "$dir/CHANGELOG.md")"
  assert_file_exists "$dir/changes/a.md"
}
