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
  assert_contains "raise the budget in $dir/budget" "$out"
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
