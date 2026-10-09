#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/bench-commands.sh"

function set_up() {
  dir="$(mktemp -d)"
  # shellcheck disable=SC2016 # the stub reads its own $1
  printf '#!/usr/bin/env bash\n[ "$1" = doctor ] && exit 1\nexit 0\n' >"$dir/bin"
  chmod +x "$dir/bin"
}

function tear_down() {
  rm -rf "$dir"
}

function test_reports_one_line_per_command() {
  local out
  out="$(bench_commands_run "$dir/bin" "$dir" 1)"
  assert_equals "$(bench_commands_list | wc -l | tr -d ' ')" "$(printf '%s\n' "$out" | wc -l | tr -d ' ')"
}

function test_reports_the_exit_code_without_failing() {
  local out
  out="$(bench_commands_run "$dir/bin" "$dir" 1)"
  assert_successful_code "$?"
  assert_matches $'^[0-9]+\t1\tdoctor$' "$(printf '%s\n' "$out" | grep doctor)"
  assert_matches $'^[0-9]+\t0\tcompare claude codex$' "$(printf '%s\n' "$out" | grep compare)"
}

function test_refuses_a_missing_binary() {
  local out
  out="$(bench_commands_main --bin "$dir/missing" 2>&1)"
  assert_general_error "$?"
  assert_contains "no binary at" "$out"
}

function test_refuses_a_run_count_below_one() {
  local out
  out="$(bench_commands_main --bin "$dir/bin" --runs 0 2>&1)"
  assert_exit_code 2
  assert_contains "positive integers" "$out"
}

function test_no_ledger_reports_read_only_commands() {
  local out
  out="$(bench_commands_run "$dir/bin" "$dir" 1 no-ledger)"
  assert_equals 3 "$(printf '%s\n' "$out" | wc -l | tr -d ' ')"
  assert_matches $'^[0-9]+\t0\tstatus \(no ledger\)$' "$(printf '%s\n' "$out" | grep status)"
  assert_matches $'^[0-9]+\t1\tdoctor \(no ledger\)$' "$(printf '%s\n' "$out" | grep doctor)"
  assert_matches $'^[0-9]+\t0\tsync --dry-run \(no ledger\)$' "$(printf '%s\n' "$out" | grep sync)"
}
