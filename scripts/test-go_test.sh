#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

function set_up() {
  dir="$(mktemp -d)"
  mkdir -p "$dir/bin" "$dir/calls"
  cat > "$dir/bin/go" <<'STUB'
#!/usr/bin/env bash
if [[ "$1" == list ]]; then
  if [[ "${FAIL_LIST:-}" == 1 ]]; then exit 1; fi
  if [[ "$2" == ./internal/cli ]]; then
    printf '%s\n' example/internal/cli
  else
    printf '%s\n' example/cmd example/internal/cli example/tests/integration
  fi
  exit 0
fi
kind=other
for arg in "$@"; do
  case "$arg" in
    '^Test[A-H]') kind=a-h ;;
    '^Test[I-L]') kind=i-l ;;
    '^TestSync') kind=sync ;;
    -skip) kind=rest ;;
  esac
done
printf '%s\n' "$@" > "$CALLS_DIR/$kind"
if [[ "${BLOCK_TEST:-}" == 1 ]]; then
  sleep 60 &
  printf '%s\n' "$!" > "$CALLS_DIR/$kind-child"
  wait "$!"
fi
if [[ "${FAIL_TEST:-}" == "$kind" ]]; then exit 1; fi
STUB
  chmod +x "$dir/bin/go"
}

function tear_down() {
  local root resolved
  root="$(cd "${TMPDIR:-/tmp}" && pwd -P)"
  resolved="$(cd "$dir" && pwd -P)"
  case "$resolved" in "$root"/*) ;; *) return 1 ;; esac
  printf 'Removing %s\n' "$resolved"
  rm -rf "$resolved"
}

function test_runs_all_packages_and_complementary_cli_shards() {
  PATH="$dir/bin:$PATH" CALLS_DIR="$dir/calls" "$SCRIPT_DIR/test-go.sh"
  assert_successful_code "$?"
  assert_equals $'test\n-timeout=20m\nexample/cmd\nexample/tests/integration' "$(cat "$dir/calls/other")"
  assert_equals $'test\n-timeout=20m\n./internal/cli\n-run\n^Test[A-H]' "$(cat "$dir/calls/a-h")"
  assert_equals $'test\n-timeout=20m\n./internal/cli\n-run\n^Test[I-L]' "$(cat "$dir/calls/i-l")"
  assert_equals $'test\n-timeout=20m\n./internal/cli\n-run\n^TestSync' "$(cat "$dir/calls/sync")"
  assert_equals $'test\n-timeout=20m\n./internal/cli\n-skip\n^Test([A-L]|Sync)' "$(cat "$dir/calls/rest")"
}

function test_race_flag_adds_race_and_disables_the_cache() {
  PATH="$dir/bin:$PATH" CALLS_DIR="$dir/calls" "$SCRIPT_DIR/test-go.sh" --race
  assert_successful_code "$?"
  assert_equals $'test\n-count=1\n-race\n-timeout=20m\nexample/cmd\nexample/tests/integration' "$(cat "$dir/calls/other")"
  assert_equals $'test\n-count=1\n-race\n-timeout=20m\n./internal/cli\n-skip\n^Test([A-L]|Sync)' "$(cat "$dir/calls/rest")"
}

function test_rejects_unknown_arguments() {
  local result=0
  PATH="$dir/bin:$PATH" CALLS_DIR="$dir/calls" "$SCRIPT_DIR/test-go.sh" --bogus 2>/dev/null || result=$?
  assert_equals 2 "$result"
  assert_equals 0 "$(find "$dir/calls" -type f | wc -l | tr -d ' ')"
}

function test_reports_failure_from_each_worker() {
  local worker result
  for worker in other a-h i-l sync rest; do
    result=0
    PATH="$dir/bin:$PATH" CALLS_DIR="$dir/calls" FAIL_TEST="$worker" "$SCRIPT_DIR/test-go.sh" || result=$?
    assert_equals 1 "$result"
  done
}

function test_fails_before_running_tests_if_package_listing_fails() {
  local result=0
  PATH="$dir/bin:$PATH" CALLS_DIR="$dir/calls" FAIL_LIST=1 "$SCRIPT_DIR/test-go.sh" || result=$?
  assert_equals 1 "$result"
  assert_equals 0 "$(find "$dir/calls" -type f | wc -l | tr -d ' ')"
}

function test_termination_stops_worker_process_groups() {
  local launcher result pid running=0
  PATH="$dir/bin:$PATH" CALLS_DIR="$dir/calls" BLOCK_TEST=1 "$SCRIPT_DIR/test-go.sh" >/dev/null 2>&1 &
  launcher=$!
  for _ in $(seq 100); do
    [[ $(find "$dir/calls" -name '*-child' | wc -l | tr -d ' ') == 5 ]] && break
    sleep 0.1
  done
  kill -TERM "$launcher"
  result=0
  wait "$launcher" || result=$?
  assert_equals 143 "$result"
  for worker in other a-h i-l sync rest; do
    if [[ ! -f "$dir/calls/$worker-child" ]]; then
      running=1
      continue
    fi
    pid=$(cat "$dir/calls/$worker-child")
    # The signal reaches the child at once, but it may take a moment to exit.
    for _ in $(seq 50); do
      kill -0 "$pid" 2>/dev/null || break
      sleep 0.1
    done
    if kill -0 "$pid" 2>/dev/null; then
      running=1
      kill -TERM "$pid" 2>/dev/null || true
    fi
  done
  assert_equals 0 "$running"
}
