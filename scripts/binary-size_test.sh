#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/binary-size.sh"

function set_up() {
  dir="$(mktemp -d)"
  head -c 100 /dev/zero > "$dir/bin"
}

function tear_down() {
  rm -rf "$dir"
}

function test_passes_under_the_budget() {
  printf '# comment\n150\n' > "$dir/budget"
  local out
  out="$(check_binary_size "$dir/bin" "$dir/budget")"
  assert_successful_code "$?"
  assert_contains "100 bytes, 50 under the 150 byte budget" "$out"
}

function test_passes_at_the_budget() {
  printf '100\n' > "$dir/budget"
  check_binary_size "$dir/bin" "$dir/budget" >/dev/null
  assert_successful_code "$?"
}

function test_fails_over_the_budget() {
  printf '90\n' > "$dir/budget"
  local out
  out="$(check_binary_size "$dir/bin" "$dir/budget" 2>&1)"
  assert_general_error "$?"
  assert_contains "100 bytes, 10 over the 90 byte budget" "$out"
  assert_contains "raises the budget in $dir/budget" "$out"
}

function test_rejects_a_budget_that_is_not_a_number() {
  printf '17 MB\n' > "$dir/budget"
  local out
  out="$(check_binary_size "$dir/bin" "$dir/budget" 2>&1)"
  assert_general_error "$?"
  assert_contains "must hold one byte count" "$out"
}

function test_rejects_two_budget_lines() {
  printf '100\n200\n' > "$dir/budget"
  check_binary_size "$dir/bin" "$dir/budget" >/dev/null 2>&1
  assert_general_error "$?"
}

function test_rejects_a_budget_too_large_to_compare() {
  printf '18446744073709551616\n' > "$dir/budget"
  check_binary_size "$dir/bin" "$dir/budget" >/dev/null 2>&1
  assert_general_error "$?"
}

function test_fails_without_a_binary() {
  printf '100\n' > "$dir/budget"
  local out
  out="$(check_binary_size "$dir/missing" "$dir/budget" 2>&1)"
  assert_general_error "$?"
  assert_contains "no binary at" "$out"
}

function test_rejects_a_leading_zero() {
  printf '0150\n' > "$dir/budget"
  check_binary_size "$dir/bin" "$dir/budget" >/dev/null 2>&1
  assert_general_error "$?"
}

function make_repo_with_growth_file() {
  repo="$dir/repo"
  mkdir "$repo"
  git -C "$repo" init -q
  printf '# why\n%s\n' "$1" > "$repo/binary-size-growth"
  git -C "$repo" add binary-size-growth
  git -C "$repo" -c user.name=t -c user.email=t@example.com -c commit.gpgsign=false commit -qm base
}

function test_growth_within_the_default_allowance_passes() {
  make_repo_with_growth_file 50
  head -c 16484 /dev/zero > "$dir/head"
  local out
  out="$(check_binary_growth "$dir/head" "$dir/bin" HEAD "$repo/binary-size-growth")"
  assert_successful_code "$?"
  assert_contains "grows the binary by 16384 bytes (100 to 16484), within the 16384 byte allowance" "$out"
}

function test_growth_over_the_default_allowance_fails() {
  make_repo_with_growth_file 99999
  head -c 16485 /dev/zero > "$dir/head"
  local out
  out="$(check_binary_growth "$dir/head" "$dir/bin" HEAD "$repo/binary-size-growth" 2>&1)"
  assert_general_error "$?"
  assert_contains "grows the binary by 16385 bytes (100 to 16485), over the 16384 byte allowance" "$out"
  assert_contains "set the bytes this change needs in $repo/binary-size-growth" "$out"
}

function test_growth_file_edited_by_the_change_sets_the_allowance() {
  make_repo_with_growth_file 16384
  printf '# a new font\n40000\n' > "$repo/binary-size-growth"
  head -c 40100 /dev/zero > "$dir/head"
  check_binary_growth "$dir/head" "$dir/bin" HEAD "$repo/binary-size-growth" >/dev/null
  assert_successful_code "$?"
}

function test_a_smaller_binary_passes() {
  make_repo_with_growth_file 16384
  head -c 10 /dev/zero > "$dir/head"
  local out
  out="$(check_binary_growth "$dir/head" "$dir/bin" HEAD "$repo/binary-size-growth")"
  assert_successful_code "$?"
  assert_contains "shrinks the binary by 90 bytes (100 to 10)" "$out"
}

function test_set_budget_keeps_comments_and_adds_headroom() {
  printf '# keep me\n50\n' > "$dir/budget"
  set_budget "$dir/bin" "$dir/budget" >/dev/null
  assert_successful_code "$?"
  assert_equals "# keep me
2097252" "$(cat "$dir/budget")"
}
