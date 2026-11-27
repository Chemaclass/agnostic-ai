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

function test_rejects_an_unknown_section() {
  write_changelog $'## [Unreleased]\n'
  printf '### Genral\n\n- X.\n' > "$dir/changes/bad.md"
  local out
  out="$(assemble_changelog --check "$dir/CHANGELOG.md" "$dir/changes" 2>&1)"
  assert_general_error "$?"
  assert_contains "not ### Genral" "$out"
}

function test_a_trailing_space_in_a_heading_joins_the_section() {
  write_changelog $'## [Unreleased]\n\n### General\n\n- A.\n'
  printf '### General \n\n- B.\n' > "$dir/changes/one.md"
  assemble_changelog "$dir/CHANGELOG.md" "$dir/changes" >/dev/null
  assert_equals $'## [Unreleased]\n\n### General\n\n- A.\n- B.' "$(cat "$dir/CHANGELOG.md")"
}

function test_rejects_a_file_that_is_not_markdown() {
  write_changelog $'## [Unreleased]\n'
  printf '### General\n\n- X.\n' > "$dir/changes/entry.txt"
  local out
  out="$(assemble_changelog --check "$dir/CHANGELOG.md" "$dir/changes" 2>&1)"
  assert_general_error "$?"
  assert_contains "entry.txt: name an entry file <slug>.md" "$out"
}

function test_keeps_intro_text_before_the_first_section() {
  write_changelog $'## [Unreleased]\n\nIntro one.\n\nIntro two.\n\n## v1.0.0 - 2026-01-01\n'
  printf '### General\n\n- X.\n' > "$dir/changes/one.md"
  assemble_changelog "$dir/CHANGELOG.md" "$dir/changes" >/dev/null
  assert_equals $'## [Unreleased]\n\nIntro one.\n\nIntro two.\n\n### General\n\n- X.\n\n## v1.0.0 - 2026-01-01' "$(cat "$dir/CHANGELOG.md")"
}

function in_repo() {
  git -C "$dir" -c commit.gpgsign=false -c core.hooksPath=/dev/null \
    -c user.name=t -c user.email=t@example.com "$@"
}

# commit_change commits one change file with the given subject.
function commit_change() {
  in_repo add "$1"
  in_repo commit -q -m "$2" -- "$1"
}

function init_repo() {
  in_repo init -q
  commit_change CHANGELOG.md "chore: start"
}

function test_adds_the_pull_request_from_the_commit_that_added_the_file() {
  write_changelog $'## [Unreleased]\n'
  init_repo
  printf '### General\n\n- New thing.\n- Cited (#7).\n- No period\n' > "$dir/changes/one.md"
  commit_change changes/one.md "feat: new thing (#123)"
  assemble_changelog "$dir/CHANGELOG.md" "$dir/changes" >/dev/null
  assert_equals $'## [Unreleased]\n\n### General\n\n- New thing (#123).\n- Cited (#7).\n- No period (#123)' "$(cat "$dir/CHANGELOG.md")"
}

function test_adds_the_reference_to_the_last_line_of_a_wrapped_bullet() {
  write_changelog $'## [Unreleased]\n'
  init_repo
  printf '### Site\n\n- Long entry\n  wraps here.\n  - Nested detail.\n' > "$dir/changes/one.md"
  commit_change changes/one.md "docs: long (#9)"
  assemble_changelog "$dir/CHANGELOG.md" "$dir/changes" >/dev/null
  assert_equals $'## [Unreleased]\n\n### Site\n\n- Long entry\n  wraps here (#9).\n  - Nested detail.' "$(cat "$dir/CHANGELOG.md")"
}

function test_leaves_a_bullet_alone_without_a_pull_request_number() {
  write_changelog $'## [Unreleased]\n'
  init_repo
  printf '### General\n\n- Direct push.\n' > "$dir/changes/one.md"
  commit_change changes/one.md "fix: pushed to main"
  printf '### General\n\n- Not committed.\n' > "$dir/changes/two.md"
  assemble_changelog "$dir/CHANGELOG.md" "$dir/changes" >/dev/null
  assert_equals $'## [Unreleased]\n\n### General\n\n- Direct push.\n- Not committed.' "$(cat "$dir/CHANGELOG.md")"
}

function test_leaves_entries_already_in_unreleased_alone() {
  write_changelog $'## [Unreleased]\n\n### General\n\n- Old entry.\n'
  init_repo
  printf '### General\n\n- New.\n' > "$dir/changes/one.md"
  commit_change changes/one.md "feat: new (#5)"
  assemble_changelog "$dir/CHANGELOG.md" "$dir/changes" >/dev/null
  assert_equals $'## [Unreleased]\n\n### General\n\n- Old entry.\n- New (#5).' "$(cat "$dir/CHANGELOG.md")"
}
