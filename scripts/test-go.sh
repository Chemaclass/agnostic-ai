#!/usr/bin/env bash
# Serial, internal/cli outlasts the default 10 minute go test timeout.
set -euo pipefail

flags=()
case "${1:-}" in
  "") ;;
  --race) flags=(-count=1 -race) ;;
  *) printf 'usage: %s [--race]\n' "$0" >&2; exit 2 ;;
esac

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
  printf 'test-go: could not split CLI and other packages\n' >&2
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

run() {
  go test ${flags[@]+"${flags[@]}"} -timeout="${TEST_GO_TIMEOUT:-20m}" "$@" &
  pids+=("$!")
}

# The last shard skips every name the others run, so together they cover
# every test in the package.
run "${other_packages[@]}"
run ./internal/cli -run '^Test[A-H]'
run ./internal/cli -run '^Test[I-L]'
run ./internal/cli -run '^TestSync'
run ./internal/cli -skip '^Test([A-L]|Sync)'

status=0
for i in "${!pids[@]}"; do
  if ! wait "${pids[$i]}"; then
    status=1
  fi
  unset 'pids[$i]'
done
exit "$status"
