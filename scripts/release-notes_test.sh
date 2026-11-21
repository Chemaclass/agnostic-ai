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

function test_release_notes_link_the_available_downloads_at_the_exact_tag() {
  local root notes
  root="$(mktemp -d)"
  printf '## v0.2.0 - 2026-05-05\n\n- Current feature.\n' > "$root/CHANGELOG.md"
  cat > "$root/assets" <<'EOF'
agnostic-ai-companion_darwin_arm64.dmg
agnostic-ai-companion_darwin_amd64.dmg
agnostic-ai_darwin_amd64.tar.gz
agnostic-ai_darwin_arm64.tar.gz
agnostic-ai_linux_amd64.tar.gz
agnostic-ai_linux_arm64.tar.gz
agnostic-ai_windows_amd64.zip
agnostic-ai_windows_arm64.zip
checksums.txt
agnostic-ai.intoto.jsonl
EOF
  notes="$(bash "$SCRIPT_DIR/release-notes.sh" v0.2.0 "$root/CHANGELOG.md" owner/repo "$root/assets")"
  assert_contains '[Apple silicon](https://github.com/owner/repo/releases/download/v0.2.0/agnostic-ai-companion_darwin_arm64.dmg)' "$notes"
  assert_contains '[Intel](https://github.com/owner/repo/releases/download/v0.2.0/agnostic-ai-companion_darwin_amd64.dmg)' "$notes"
  assert_contains 'https://agnostic-ai.org/docs/verify-a-release/' "$notes"
  assert_contains 'Current feature.' "$notes"
  assert_contains 'owner/repo/blob/main/CHANGELOG.md' "$notes"
  assert_equals "$(sort "$root/assets")" "$(printf '%s\n' "$notes" | grep -oE 'https://github\.com/owner/repo/releases/download/v0\.2\.0/[^)]+' | cut -d/ -f9 | sort)"
  rm -rf "$root"
}

function test_prerelease_notes_skip_missing_and_similarly_named_assets() {
  local root notes
  root="$(mktemp -d)"
  printf '## v0.3.0-rc.1 - 2026-05-05\n\n- Prerelease feature.\n' > "$root/CHANGELOG.md"
  printf '%s\n' agnostic-ai-companion_darwin_arm64.dmg.sha256 agnostic-ai_linux_arm64.tar.gz > "$root/assets"
  notes="$(format_release_notes v0.3.0-rc.1 "$root/CHANGELOG.md" owner/repo "$root/assets")"
  assert_not_contains 'agnostic-ai-companion_darwin' "$notes"
  assert_not_contains 'agnostic-ai_windows' "$notes"
  assert_not_contains '/checksums.txt' "$notes"
  assert_contains 'https://github.com/owner/repo/releases/download/v0.3.0-rc.1/agnostic-ai_linux_arm64.tar.gz' "$notes"
  rm -rf "$root"
}

function test_release_notes_keep_the_changelog_when_the_inventory_is_empty() {
  local root notes
  root="$(mktemp -d)"
  printf '## v0.2.0 - 2026-05-05\n\n- Current feature.\n' > "$root/CHANGELOG.md"
  touch "$root/assets"
  notes="$(format_release_notes v0.2.0 "$root/CHANGELOG.md" owner/repo "$root/assets")"
  assert_equals "$(format_release_notes v0.2.0 "$root/CHANGELOG.md" owner/repo)" "$notes"
  rm -rf "$root"
}

function test_release_notes_fail_if_the_verified_inventory_cannot_be_read() {
  local root output status=0
  root="$(mktemp -d)"
  printf '## v0.2.0 - 2026-05-05\n\n- Current feature.\n' > "$root/CHANGELOG.md"
  output="$(format_release_notes v0.2.0 "$root/CHANGELOG.md" owner/repo "$root/missing" 2>&1)" || status=$?
  assert_equals 1 "$status"
  assert_contains "$root/missing" "$output"
  rm -rf "$root"
}
