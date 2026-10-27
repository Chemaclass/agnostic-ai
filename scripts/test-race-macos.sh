#!/usr/bin/env bash
set -euo pipefail

cli_package=$(go list ./internal/cli)
all_packages=$(go list ./...)
other_packages=()
found_cli=false
while IFS= read -r package; do
  if [[ "$package" == "$cli_package" ]]; then
    found_cli=true
  elif [[ -n "$package" ]]; then
    other_packages+=("$package")
  fi
done <<< "$all_packages"
if [[ "$found_cli" != true || ${#other_packages[@]} -eq 0 ]]; then
  printf 'test-race: could not split CLI and other packages\n' >&2
  exit 1
fi

set -m
pids=()
stop_children() {
  for pid in ${pids[@]+"${pids[@]}"}; do
    kill -TERM -- "-$pid" 2>/dev/null || true
  done
  for pid in ${pids[@]+"${pids[@]}"}; do
    wait "$pid" 2>/dev/null || true
  done
}
trap 'stop_children; exit 130' INT
trap 'stop_children; exit 143' TERM

timeout=${TEST_RACE_TIMEOUT:-20m}
go test -count=1 -race -timeout="$timeout" "${other_packages[@]}" &
pids+=("$!")
go test -count=1 -race -timeout="$timeout" ./internal/cli -run '^Test[A-L]' &
pids+=("$!")
go test -count=1 -race -timeout="$timeout" ./internal/cli -skip '^Test[A-L]' &
pids+=("$!")

status=0
for i in "${!pids[@]}"; do
  if ! wait "${pids[$i]}"; then
    status=1
  fi
  unset 'pids[$i]'
done
exit "$status"
