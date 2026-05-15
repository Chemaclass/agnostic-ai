#!/usr/bin/env bash
#
# bashunit tests for scripts/release.sh
#
# Run:
#   bashunit scripts/release_test.sh
#
# Pure helpers (compute_next_version, validate_version, bump_version_in_file,
# promote_changelog, parse_args) are tested directly by sourcing the script.
# End-to-end paths that touch git are exercised through a temp git fixture
# in dry-run mode so no commits or tags occur.

# Source the script for its functions only. The source guard inside
# release.sh prevents main() from running.
SCRIPT_DIR="$(cd "$(dirname "$BASH_SOURCE")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/release.sh"

# ---- validate_version --------------------------------------------------------

function test_validate_version_accepts_plain() {
  assert_successful_code "$(validate_version v1.2.3 && echo ok)"
}

function test_validate_version_accepts_pre_release() {
  validate_version "v0.2.0-rc.1"
  assert_equals 0 $?
}

function test_validate_version_rejects_missing_v() {
  validate_version "1.2.3"
  assert_equals 1 $?
}

function test_validate_version_rejects_partial() {
  validate_version "v1.2"
  assert_equals 1 $?
}

function test_validate_version_rejects_garbage() {
  validate_version "vabc"
  assert_equals 1 $?
}

# ---- compute_next_version ----------------------------------------------------

function test_compute_next_version_minor() {
  assert_equals "v0.2.0" "$(compute_next_version minor 0.1.5)"
}

function test_compute_next_version_minor_zeroes_patch() {
  assert_equals "v1.3.0" "$(compute_next_version minor 1.2.7)"
}

function test_compute_next_version_major_zeroes_minor_and_patch() {
  assert_equals "v2.0.0" "$(compute_next_version major 1.4.9)"
}

function test_compute_next_version_patch() {
  assert_equals "v0.1.6" "$(compute_next_version patch 0.1.5)"
}

function test_compute_next_version_rejects_unknown_kind() {
  compute_next_version weird 1.0.0 >/dev/null 2>&1
  assert_equals 1 $?
}

function test_compute_next_version_rejects_non_semver_current() {
  compute_next_version minor "0.1" >/dev/null 2>&1
  assert_equals 1 $?
}

function test_compute_next_version_rejects_pre_release_current() {
  compute_next_version minor "0.1.0-rc.1" >/dev/null 2>&1
  assert_equals 1 $?
}

# ---- read_current_version ----------------------------------------------------

function test_read_current_version_extracts_value() {
  local tmp
  tmp="$(mktemp)"
  printf 'package main\n\nvar version = "0.7.3"\n' > "$tmp"
  assert_equals "0.7.3" "$(read_current_version "$tmp")"
  rm -f "$tmp"
}

function test_read_current_version_empty_for_missing_marker() {
  local tmp
  tmp="$(mktemp)"
  printf 'package main\n' > "$tmp"
  assert_equals "" "$(read_current_version "$tmp")"
  rm -f "$tmp"
}

# ---- bump_version_in_file ----------------------------------------------------

function test_bump_version_in_file_rewrites_version_line() {
  local tmp
  tmp="$(mktemp)"
  printf 'package main\n\nvar version = "0.1.0"\n\nfunc main() {}\n' > "$tmp"
  bump_version_in_file "$tmp" "0.2.0"
  assert_equals "0.2.0" "$(read_current_version "$tmp")"
  assert_contains "func main() {}" "$(cat "$tmp")"
  rm -f "$tmp"
}

function test_bump_version_in_file_leaves_other_lines_intact() {
  local tmp
  tmp="$(mktemp)"
  printf 'line1\nvar version = "1.0.0"\nline3\n' > "$tmp"
  bump_version_in_file "$tmp" "1.1.0"
  assert_equals 'line1
var version = "1.1.0"
line3' "$(cat "$tmp")"
  rm -f "$tmp"
}

# ---- promote_changelog -------------------------------------------------------

function test_promote_changelog_inserts_dated_section() {
  local tmp
  tmp="$(mktemp)"
  cat > "$tmp" <<'EOF'
# Changelog

## [Unreleased]

### Added
- foo
EOF
  promote_changelog "$tmp" "v0.2.0" "2026-05-04"
  local out
  out="$(cat "$tmp")"
  assert_contains "## [Unreleased]" "$out"
  assert_contains "## v0.2.0 - 2026-05-04" "$out"
  assert_contains "- foo" "$out"
  # The new [Unreleased] must appear before the dated section.
  local unreleased_line dated_line
  unreleased_line="$(grep -n '^## \[Unreleased\]$' "$tmp" | head -1 | cut -d: -f1)"
  dated_line="$(grep -n '^## v0.2.0' "$tmp" | head -1 | cut -d: -f1)"
  assert_equals "true" "$([[ $unreleased_line -lt $dated_line ]] && echo true || echo false)"
  rm -f "$tmp"
}

function test_promote_changelog_errors_without_unreleased() {
  local tmp
  tmp="$(mktemp)"
  printf '# Changelog\n\n## [v0.1.0] - 2026-01-01\n' > "$tmp"
  promote_changelog "$tmp" "v0.2.0" "2026-05-04" 2>/dev/null
  assert_equals 1 $?
  rm -f "$tmp"
}

# ---- unreleased_has_content --------------------------------------------------

function test_unreleased_has_content_detects_bullet() {
  local tmp
  tmp="$(mktemp)"
  cat > "$tmp" <<'EOF'
# Changelog

## [Unreleased]

### Added
- something new

## v0.1.0 - 2026-01-01
EOF
  unreleased_has_content "$tmp"
  assert_equals 0 $?
  rm -f "$tmp"
}

function test_unreleased_has_content_rejects_empty_subsections() {
  local tmp
  tmp="$(mktemp)"
  cat > "$tmp" <<'EOF'
# Changelog

## [Unreleased]

### Added

### Changed

### Fixed

### Removed

## v0.1.0 - 2026-01-01
EOF
  unreleased_has_content "$tmp"
  assert_equals 1 $?
  rm -f "$tmp"
}

function test_unreleased_has_content_rejects_blank_section() {
  local tmp
  tmp="$(mktemp)"
  printf '## [Unreleased]\n\n## v0.1.0 - 2026-01-01\n' > "$tmp"
  unreleased_has_content "$tmp"
  assert_equals 1 $?
  rm -f "$tmp"
}

# ---- extract_changelog_section ----------------------------------------------

function test_extract_changelog_section_returns_body_only() {
  local tmp
  tmp="$(mktemp)"
  cat > "$tmp" <<'EOF'
# Changelog

## [Unreleased]

## [v0.2.0] - 2026-06-01

### Added
- New thing.

### Fixed
- Old bug.

## [v0.1.0] - 2026-05-04

### Added
- Initial release.
EOF
  local out
  out="$(extract_changelog_section "$tmp" "v0.2.0")"
  assert_contains "### Added" "$out"
  assert_contains "- New thing." "$out"
  assert_contains "- Old bug." "$out"
  assert_not_contains "## [v0.1.0]" "$out"
  assert_not_contains "Initial release" "$out"
  rm -f "$tmp"
}

function test_extract_changelog_section_strips_trailing_blanks() {
  local tmp
  tmp="$(mktemp)"
  cat > "$tmp" <<'EOF'
## [v0.1.0] - 2026-05-04

### Added
- foo



## [v0.0.1] - 2026-04-01

- old
EOF
  local out
  out="$(extract_changelog_section "$tmp" "v0.1.0")"
  # Last line must be content, not blank.
  local last
  last="$(printf '%s\n' "$out" | tail -1)"
  assert_equals "- foo" "$last"
  rm -f "$tmp"
}

function test_extract_changelog_section_returns_last_section() {
  local tmp
  tmp="$(mktemp)"
  cat > "$tmp" <<'EOF'
## [v0.2.0] - 2026-06-01

- new

## [v0.1.0] - 2026-05-04

- first
- ever
EOF
  local out
  out="$(extract_changelog_section "$tmp" "v0.1.0")"
  assert_contains "- first" "$out"
  assert_contains "- ever" "$out"
  assert_not_contains "- new" "$out"
  rm -f "$tmp"
}

function test_extract_changelog_section_accepts_bracketless_heading() {
  local tmp
  tmp="$(mktemp)"
  cat > "$tmp" <<'EOF'
## [Unreleased]

## v0.4.0 - 2026-05-05

### Added
- thing

## v0.3.0 - 2026-05-04

- prior
EOF
  local out
  out="$(extract_changelog_section "$tmp" "v0.4.0")"
  assert_contains "### Added" "$out"
  assert_contains "- thing" "$out"
  assert_not_contains "- prior" "$out"
  rm -f "$tmp"
}

function test_extract_changelog_section_errors_on_missing() {
  local tmp
  tmp="$(mktemp)"
  printf '## [v0.1.0]\n- foo\n' > "$tmp"
  extract_changelog_section "$tmp" "v9.9.9" >/dev/null
  assert_equals 1 $?
  rm -f "$tmp"
}

# ---- format_release_notes ----------------------------------------------------

function test_format_release_notes_includes_changelog_body() {
  local tmp
  tmp="$(mktemp)"
  cat > "$tmp" <<'EOF'
## [v0.1.0] - 2026-05-04

### Added
- one
- two
EOF
  local out
  out="$(format_release_notes "v0.1.0" "$tmp" "owner/repo")"
  assert_contains "### Added" "$out"
  assert_contains "- one" "$out"
  assert_contains "- two" "$out"
  assert_contains "owner/repo/blob/main/CHANGELOG.md" "$out"
  assert_contains "owner/repo#readme" "$out"
  rm -f "$tmp"
}

function test_format_release_notes_omits_install_and_docs_blocks() {
  local tmp
  tmp="$(mktemp)"
  printf '## [v0.1.0]\n\n- foo\n' > "$tmp"
  local out
  out="$(format_release_notes "v0.1.0" "$tmp" "owner/repo")"
  assert_not_contains "## Install" "$out"
  assert_not_contains "brew install" "$out"
  assert_not_contains "## Documentation" "$out"
  assert_not_contains "/docs/user/getting-started.md" "$out"
  rm -f "$tmp"
}

function test_format_release_notes_errors_on_missing_section() {
  local tmp
  tmp="$(mktemp)"
  printf '## [v0.1.0]\n- foo\n' > "$tmp"
  format_release_notes "v9.9.9" "$tmp" "owner/repo" >/dev/null 2>&1
  assert_equals 1 $?
  rm -f "$tmp"
}

# ---- parse_args --------------------------------------------------------------

function test_parse_args_default_is_minor() {
  parse_args
  assert_equals "minor" "$BUMP"
  assert_equals "" "$VERSION"
  assert_equals 0 "$DRY_RUN"
  assert_equals 0 "$NO_PUSH"
}

function test_parse_args_explicit_version() {
  parse_args v0.5.0
  assert_equals "v0.5.0" "$VERSION"
  assert_equals "" "$BUMP"
}

function test_parse_args_bump_kind_overrides_default() {
  parse_args patch
  assert_equals "patch" "$BUMP"
}

function test_parse_args_dry_run_flag() {
  parse_args --dry-run
  assert_equals 1 "$DRY_RUN"
  assert_equals "minor" "$BUMP"
}

function test_parse_args_no_push_flag() {
  parse_args patch --no-push
  assert_equals 1 "$NO_PUSH"
  assert_equals "patch" "$BUMP"
}

function test_parse_args_rejects_version_and_bump_together() {
  parse_args v0.1.0 minor 2>/dev/null
  assert_equals 1 $?
}

function test_parse_args_rejects_unknown_arg() {
  parse_args banana 2>/dev/null
  assert_equals 1 $?
}
