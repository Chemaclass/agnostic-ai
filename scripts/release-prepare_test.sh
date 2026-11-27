#!/usr/bin/env bash
#
# bashunit tests for scripts/release-prepare.sh, run against a temporary
# fixture repository.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PREPARE="$SCRIPT_DIR/release-prepare.sh"

function in_repo() {
  git -C "$dir" -c commit.gpgsign=false -c core.hooksPath=/dev/null \
    -c user.name=t -c user.email=t@example.com "$@"
}

function set_up() {
  dir="$(mktemp -d)"
  mkdir -p "$dir/changes" "$dir/cmd/agnostic-ai" "$dir/docs/site/content/updates" "$dir/scripts/target-audit"
  cat > "$dir/CHANGELOG.md" <<'EOF'
# Changelog

Releases before v1.0.0 are in [docs/CHANGELOG-archive.md](docs/CHANGELOG-archive.md).

## [Unreleased]

### Highlights

### General

- Kept (#10).

### By tool

#### Codex

## v1.10.0 - 2026-11-01

- Ten (#9).

## v1.9.0 - 2026-10-01

- Nine (#8).

## v1.0.0 - 2026-01-01

- One (#1).
EOF
  cat > "$dir/docs/CHANGELOG-archive.md" <<'EOF'
# Changelog archive

Releases before v1.0.0. Everything from v1.0.0 on lives in [CHANGELOG.md](../CHANGELOG.md). Releases up to v0.72.0 group entries as Added, Changed, Fixed, and Removed.

## v0.9.0 - 2025-12-01

- Old (#0).
EOF
  printf 'Read me.\n' > "$dir/changes/README.md"
  printf '### By tool\n\n#### Codex\n\n- Codex entry.\n' > "$dir/changes/codex.md"
  printf 'package main\n\nvar version = "1.10.0"\n' > "$dir/cmd/agnostic-ai/main.go"
  printf '[extra]\nversion = "v1.10.0"\n' > "$dir/docs/site/config.toml"
  cat > "$dir/agnostic-ai.yaml" <<'EOF'
# yaml-language-server: $schema=https://raw.githubusercontent.com/Chemaclass/agnostic-ai/v1.10.0/docs/schemas/config.schema.json
requires: ">=1.10.0"
EOF
  printf '# budget\n100\n' > "$dir/scripts/binary-size-budget"
  printf '# signal-id\tfirst-seen\tlast-seen\tdisposition\tissues\tvendor-date\tfirst-source\tshipped-date\tnote\n' > "$dir/scripts/target-audit/signals.tsv"
  printf 'cap-a\t2026-09-16\t2026-09-23\twatch\t10\t\t\t\t\n' >> "$dir/scripts/target-audit/signals.tsv"
  printf 'size-budget:\n\t@printf "# budget\\n200\\n" > scripts/binary-size-budget\n' > "$dir/Makefile"
  in_repo init -q -b main
  in_repo add -A
  in_repo commit -q -m "chore: start"
  printf '### General\n\n- Added later.\n' > "$dir/changes/later.md"
  in_repo add changes/later.md
  in_repo commit -q -m "feat: later (#42)"
}

function tear_down() {
  rm -rf "$dir"
}

function prepare() {
  RELEASE_ROOT="$dir" "$PREPARE" "$@"
}

function test_prepares_the_release_files() {
  prepare v1.11.0 2026-12-01 >/dev/null
  assert_successful_code "$?"

  assert_contains $'## [Unreleased]\n\n## v1.11.0 - 2026-12-01\n\n### General\n\n- Kept (#10).\n- Added later (#42).\n\n### By tool\n\n#### Codex\n\n- Codex entry.\n\n## v1.10.0' "$(cat "$dir/CHANGELOG.md")"
  assert_not_contains "### Highlights" "$(cat "$dir/CHANGELOG.md")"
  assert_file_not_exists "$dir/changes/codex.md"
  assert_contains 'var version = "1.11.0"' "$(cat "$dir/cmd/agnostic-ai/main.go")"
  assert_contains 'version = "v1.11.0"' "$(cat "$dir/docs/site/config.toml")"
  assert_contains 'requires: ">=1.11.0"' "$(cat "$dir/agnostic-ai.yaml")"
  assert_contains 'agnostic-ai/v1.11.0/docs/schemas/' "$(cat "$dir/agnostic-ai.yaml")"
  assert_contains "200" "$(cat "$dir/scripts/binary-size-budget")"
  assert_contains $'\t10\t\t\t2026-12-01\t' "$(cat "$dir/scripts/target-audit/signals.tsv")"
}

function test_writes_the_briefing_front_matter() {
  prepare v1.11.0 2026-12-01 >/dev/null
  local briefing="$dir/docs/site/content/updates/2026-12-01-v1.11.0.md"
  assert_file_exists "$briefing"
  local text
  text="$(cat "$briefing")"
  assert_contains 'slug = "2026-12-01-v1.11.0"' "$text"
  assert_contains 'aliases = ["updates/2026-12-01-v1.11.0.html"]' "$text"
  assert_contains 'rss_guid = "https://agnostic-ai.org/updates/2026-12-01-v1.11.0.html"' "$text"
  assert_contains 'version = "v1.11.0"' "$text"
  assert_contains 'kind = "release"' "$text"
  assert_contains 'targets = []' "$text"
  assert_contains '[[extra.signals]]' "$text"
  assert_contains '## Upgrading from v1.10.0 {#actions}' "$text"
  assert_matches 'date = 2026-12-01T00:00:00[+-][0-9][0-9]:[0-9][0-9]' "$text"
}

function test_archives_the_previous_group_on_an_x1_release() {
  prepare v1.11.0 2026-12-01 >/dev/null
  local changelog archive
  changelog="$(cat "$dir/CHANGELOG.md")"
  archive="$(cat "$dir/docs/CHANGELOG-archive.md")"
  assert_contains "## v1.10.0 - 2026-11-01" "$changelog"
  assert_not_contains "## v1.9.0" "$changelog"
  assert_not_contains "## v1.0.0" "$changelog"
  assert_contains "Releases before v1.10.0 are in" "$changelog"
  assert_contains $'Releases before v1.10.0. Everything from v1.10.0 on lives' "$archive"
  assert_contains $'Releases up to v0.72.0 group entries as Added, Changed, Fixed, and Removed.\n\n## v1.9.0 - 2026-10-01\n\n- Nine (#8).\n\n## v1.0.0 - 2026-01-01\n\n- One (#1).\n\n## v0.9.0 - 2025-12-01' "$archive"
}

function test_keeps_the_group_on_other_releases() {
  prepare v1.10.1 2026-12-01 >/dev/null
  assert_contains "## v1.9.0 - 2026-10-01" "$(cat "$dir/CHANGELOG.md")"
  assert_contains "Releases before v1.0.0 are in" "$(cat "$dir/CHANGELOG.md")"
}

function test_a_rerun_changes_nothing() {
  prepare v1.11.0 2026-12-01 >/dev/null
  local before after out
  before="$(cd "$dir" && cat CHANGELOG.md docs/CHANGELOG-archive.md agnostic-ai.yaml scripts/binary-size-budget scripts/target-audit/signals.tsv docs/site/content/updates/*.md)"
  out="$(prepare v1.11.0 2026-12-02)"
  assert_successful_code "$?"
  after="$(cd "$dir" && cat CHANGELOG.md docs/CHANGELOG-archive.md agnostic-ai.yaml scripts/binary-size-budget scripts/target-audit/signals.tsv docs/site/content/updates/*.md)"
  assert_equals "$before" "$after"
  assert_contains "keeping the existing date 2026-12-01" "$out"
  assert_contains "already there, skipped" "$out"
  assert_file_not_exists "$dir/docs/site/content/updates/2026-12-02-v1.11.0.md"
}

function test_a_rerun_keeps_a_signal_date_cleared_in_review() {
  prepare v1.11.0 2026-12-01 >/dev/null
  printf '# signal-id\tfirst-seen\tlast-seen\tdisposition\tissues\tvendor-date\tfirst-source\tshipped-date\tnote\n' > "$dir/scripts/target-audit/signals.tsv"
  printf 'cap-a\t2026-09-16\t2026-09-23\twatch\t10\t\t\t\t\n' >> "$dir/scripts/target-audit/signals.tsv"
  printf '\n' >> "$dir/scripts/target-audit/signals.tsv"
  local out
  out="$(prepare v1.11.0)"
  assert_contains "signals: already set, skipped" "$out"
  assert_not_contains "2026-12-01" "$(cat "$dir/scripts/target-audit/signals.tsv")"
}

function test_resumes_after_the_rollover() {
  prepare v1.11.0 2026-12-01 >/dev/null
  rm "$dir/docs/site/content/updates/2026-12-01-v1.11.0.md"
  printf 'package main\n\nvar version = "1.10.0"\n' > "$dir/cmd/agnostic-ai/main.go"
  prepare v1.11.0 >/dev/null
  assert_successful_code "$?"
  assert_contains 'var version = "1.11.0"' "$(cat "$dir/cmd/agnostic-ai/main.go")"
  assert_file_exists "$dir/docs/site/content/updates/2026-12-01-v1.11.0.md"
}

function test_refuses_entries_left_after_the_rollover() {
  prepare v1.11.0 2026-12-01 >/dev/null
  printf '### General\n\n- Late.\n' > "$dir/changes/late.md"
  local out
  out="$(prepare v1.11.0 2>&1)"
  assert_general_error "$?"
  assert_contains "changes/ has entries too" "$out"
}

function test_refuses_an_empty_unreleased_section() {
  printf '# Changelog\n\n## [Unreleased]\n\n### General\n\n## v1.10.0 - 2026-11-01\n\n- Ten.\n' > "$dir/CHANGELOG.md"
  rm "$dir/changes/codex.md" "$dir/changes/later.md"
  local out
  out="$(prepare --skip-git-check v1.11.0 2>&1)"
  assert_general_error "$?"
  assert_contains "has no entries to release" "$out"
}

function test_refuses_another_branch() {
  in_repo switch -q -c topic
  local out
  out="$(prepare v1.11.0 2>&1)"
  assert_general_error "$?"
  assert_contains "on branch topic" "$out"
  assert_contains "## [Unreleased]" "$(cat "$dir/CHANGELOG.md")"
  assert_file_exists "$dir/changes/codex.md"
}

function test_refuses_an_uncommitted_change_before_the_release_starts() {
  printf '\n' >> "$dir/CHANGELOG.md"
  local out
  out="$(prepare v1.11.0 2>&1)"
  assert_general_error "$?"
  assert_contains "uncommitted change before the release started: CHANGELOG.md" "$out"
  assert_file_exists "$dir/changes/codex.md"
}

function test_refuses_an_edit_outside_the_release_files_on_resume() {
  prepare v1.11.0 2026-12-01 >/dev/null
  printf 'x\n' > "$dir/notes.txt"
  local out
  out="$(prepare v1.11.0 2>&1)"
  assert_general_error "$?"
  assert_contains "outside the release files: notes.txt" "$out"
}

function test_skip_git_check_allows_another_branch() {
  in_repo switch -q -c topic
  prepare --skip-git-check v1.11.0 2026-12-01 >/dev/null
  assert_successful_code "$?"
}

function test_rejects_a_bad_version() {
  prepare 1.11.0 >/dev/null 2>&1
  assert_general_error "$?"
}

function write_released() {
  printf '# Changelog\n\n## [Unreleased]\n\n## v1.11.0 - 2026-12-01\n\n%s\n\n## v1.10.0 - 2026-11-01\n\n- Ten.\n' "$1" > "$dir/CHANGELOG.md"
  printf '+++\ntitle = "done"\n+++\n' > "$dir/docs/site/content/updates/2026-12-01-v1.11.0.md"
}

function test_check_passes_a_curated_release() {
  write_released $'### General\n\n- One (#1).\n\n### By tool\n\n#### Codex\n\n- Two (#2).'
  local out
  out="$(prepare --check v1.11.0)"
  assert_successful_code "$?"
  assert_contains "section and briefing pass" "$out"
}

function test_check_reports_shape_problems() {
  local long
  long="- $(printf 'x%.0s' $(seq 1 170))"
  write_released "### General

$long

### Site

#### Codex

- 1.
- 2.
- 3.
- 4.
- 5.
- 6."
  printf 'TODO\n' >> "$dir/docs/site/content/updates/2026-12-01-v1.11.0.md"
  local out
  out="$(prepare --check v1.11.0 2>&1)"
  assert_general_error "$?"
  assert_contains "over 160 characters (172)" "$out"
  assert_contains "over 5 bullets: #### Codex" "$out"
  assert_contains "briefing still has TODO" "$out"
  assert_not_contains "empty heading: ### Site" "$out"
}

function test_check_reports_an_empty_heading_and_a_missing_briefing() {
  write_released $'### General\n\n### By tool\n\n#### Codex\n\n- Two (#2).'
  rm "$dir/docs/site/content/updates/2026-12-01-v1.11.0.md"
  local out
  out="$(prepare --check v1.11.0 2>&1)"
  assert_general_error "$?"
  assert_equals 1 "$(printf '%s\n' "$out" | grep -c 'empty heading: ### General')"
  assert_not_contains "empty heading: ### By tool" "$out"
  assert_contains "no briefing" "$out"
}

function test_check_changelog_skips_the_briefing() {
  write_released $'### General\n\n- One (#1).'
  printf 'TODO\n' >> "$dir/docs/site/content/updates/2026-12-01-v1.11.0.md"
  local out
  out="$(prepare --check-changelog v1.11.0)"
  assert_successful_code "$?"
  assert_contains "v1.11.0 section passes" "$out"
}

function test_check_reports_a_missing_section() {
  local out
  out="$(prepare --check v1.11.0 2>&1)"
  assert_general_error "$?"
  assert_contains 'no "## v1.11.0 - " section' "$out"
}
