#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/release-notes.sh"

function test_release_notes_use_only_the_requested_dated_section() {
  local changelog
  changelog="$(mktemp)"
  cat > "$changelog" <<'EOF'
## [Unreleased]

## v0.2.0 - 2026-05-05

### Added
- Current feature.

## v0.1.0 - 2026-05-04

- Previous feature.
EOF
  local notes
  notes="$(format_release_notes v0.2.0 "$changelog" owner/repo)"
  assert_contains "Current feature." "$notes"
  assert_not_contains "Previous feature." "$notes"
  assert_contains "owner/repo/blob/main/CHANGELOG.md" "$notes"
  rm -f "$changelog"
}

function test_release_notes_accept_legacy_bracketed_heading() {
  local changelog
  changelog="$(mktemp)"
  printf '## [v0.1.0] - 2026-05-04\n\n- Previous feature.\n' > "$changelog"
  local notes
  notes="$(format_release_notes v0.1.0 "$changelog" owner/repo)"
  assert_contains "Previous feature." "$notes"
  rm -f "$changelog"
}

function test_release_notes_fall_back_to_archive() {
  local root
  root="$(mktemp -d)"
  mkdir -p "$root/docs"
  printf '## [Unreleased]\n' > "$root/CHANGELOG.md"
  printf '## v0.1.0 - 2026-05-04\n\n- Archived feature.\n' > "$root/docs/CHANGELOG-archive.md"
  local notes
  notes="$(format_release_notes v0.1.0 "$root/CHANGELOG.md" owner/repo)"
  assert_contains "Archived feature." "$notes"
  rm -rf "$root"
}

function test_release_notes_fail_for_missing_version() {
  local changelog
  changelog="$(mktemp)"
  printf '## v0.1.0 - 2026-05-04\n\n- Previous feature.\n' > "$changelog"
  format_release_notes v9.9.9 "$changelog" owner/repo >/dev/null 2>&1
  assert_equals 1 $?
  rm -f "$changelog"
}
