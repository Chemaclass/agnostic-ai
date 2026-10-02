#!/usr/bin/env bash
#
# bashunit tests for scripts/signals-shipped.sh
#
# Run:
#   bashunit scripts/signals-shipped_test.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/signals-shipped.sh"

FIXTURES=""

function set_up() {
  FIXTURES=$(mktemp -d)
  cat >"$FIXTURES/CHANGELOG.md" <<'EOF'
# Changelog

## [Unreleased]

- Pending work (#900).

## v0.76.0 - 2026-10-02

- Agents take a portable effort (#824).

## v0.75.0 - 2026-09-29

- Rules activate by policy (#1029).
- An earlier note on effort (#824).
EOF
  printf '# signal-id\tfirst-seen\tlast-seen\tdisposition\tissues\tvendor-date\tfirst-source\tshipped-date\tnote\n' >"$FIXTURES/signals.tsv"
  printf 'cap-effort\t2026-09-16\t2026-09-23\tspec-candidate\t824\t\t\t\tnote\n' >>"$FIXTURES/signals.tsv"
  printf 'cap-rules\t2026-09-16\t2026-09-23\tadapter-gap\t1029,1113\t\t\t\t\n' >>"$FIXTURES/signals.tsv"
  printf 'cap-pending\t2026-09-16\t2026-09-23\twatch\t900\t\t\t\t\n' >>"$FIXTURES/signals.tsv"
  printf 'cap-kept\t2026-09-16\t2026-09-23\twatch\t824\t\t\t2026-09-01\t\n' >>"$FIXTURES/signals.tsv"
}

function tear_down() {
  [ -n "$FIXTURES" ] && rm -rf "$FIXTURES"
}

function shipped_of() {
  awk -F '\t' -v id="$1" '$1 == id { print $8 }' "$FIXTURES/signals.tsv"
}

function write_archive() {
  mkdir -p "$FIXTURES/docs"
  cat >"$FIXTURES/docs/CHANGELOG-archive.md" <<'EOF'
# Changelog archive

- An undated reference (#900).

## v0.66.0 - 2026-09-23

- Effort was supported earlier (#824).
- Archived support (#700).

## v0.65.0 - 2026-09-22

- The first effort release (#824).
EOF
  printf 'cap-archive\t2026-09-16\t2026-09-23\tadapter-gap\t700\t\t\t\tarchived note\n' >>"$FIXTURES/signals.tsv"
}

function test_the_newest_section_sets_the_rows_it_cites() {
  signals_shipped "$FIXTURES/CHANGELOG.md" "$FIXTURES/signals.tsv" 2>/dev/null
  assert_equals "2026-10-02" "$(shipped_of cap-effort)"
  assert_empty "$(shipped_of cap-rules)"
}

function test_all_sections_pick_the_earliest_release() {
  signals_shipped --all "$FIXTURES/CHANGELOG.md" "$FIXTURES/signals.tsv" 2>/dev/null
  assert_equals "2026-09-29" "$(shipped_of cap-effort)"
  assert_equals "2026-09-29" "$(shipped_of cap-rules)"
}

function test_unreleased_entries_do_not_ship() {
  signals_shipped --all "$FIXTURES/CHANGELOG.md" "$FIXTURES/signals.tsv" 2>/dev/null
  assert_empty "$(shipped_of cap-pending)"
}

function test_a_set_date_is_never_moved() {
  signals_shipped --all "$FIXTURES/CHANGELOG.md" "$FIXTURES/signals.tsv" 2>/dev/null
  assert_equals "2026-09-01" "$(shipped_of cap-kept)"
}

function test_rows_keep_every_other_column() {
  signals_shipped "$FIXTURES/CHANGELOG.md" "$FIXTURES/signals.tsv" 2>/dev/null
  assert_equals "cap-effort	2026-09-16	2026-09-23	spec-candidate	824			2026-10-02	note" \
    "$(grep '^cap-effort' "$FIXTURES/signals.tsv")"
}

function test_all_sections_include_the_archive() {
  write_archive
  signals_shipped --all "$FIXTURES/CHANGELOG.md" "$FIXTURES/signals.tsv" 2>/dev/null
  assert_equals "2026-09-23" "$(shipped_of cap-archive)"
  assert_equals "2026-09-22" "$(shipped_of cap-effort)"
  assert_equals "2026-09-29" "$(shipped_of cap-rules)"
  assert_empty "$(shipped_of cap-pending)"
  assert_equals "2026-09-01" "$(shipped_of cap-kept)"
  assert_equals "cap-effort	2026-09-16	2026-09-23	spec-candidate	824			2026-09-22	note" \
    "$(grep '^cap-effort' "$FIXTURES/signals.tsv")"
}

function test_the_newest_section_does_not_read_the_archive() {
  write_archive
  signals_shipped "$FIXTURES/CHANGELOG.md" "$FIXTURES/signals.tsv" 2>/dev/null
  assert_equals "2026-10-02" "$(shipped_of cap-effort)"
  assert_empty "$(shipped_of cap-archive)"
}

function test_all_sections_pick_the_earliest_date_regardless_of_order() {
  cat >"$FIXTURES/CHANGELOG.md" <<'EOF'
## v0.65.0 - 2026-09-22

- The first effort release (#824).

## v0.66.0 - 2026-09-23

- A later effort update (#824).
EOF
  mkdir -p "$FIXTURES/docs"
  printf '## v0.76.0 - 2026-10-02\n\n- The newest effort update (#824).\n' >"$FIXTURES/docs/CHANGELOG-archive.md"
  signals_shipped --all "$FIXTURES/CHANGELOG.md" "$FIXTURES/signals.tsv" 2>/dev/null
  assert_equals "2026-09-22" "$(shipped_of cap-effort)"
}
