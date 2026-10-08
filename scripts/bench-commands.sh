#!/usr/bin/env bash
#
# bench-commands.sh - time every read-only command and a no-op sync on the
# benchmark fixture, end to end with a built binary.
#
# The fixture is the one `make bench` uses (benchProject in
# internal/cli/bench_test.go): N rules, N agents, and N skills plus a few
# hooks, MCP servers, and commands, synced to every target. Each command
# runs --runs times and reports its fastest run.
#
# Usage:
#   scripts/bench-commands.sh [--bin <agnostic-ai>] [--specs N] [--runs N]
#
# Prints "<ms>\t<exit>\t<command>" per command. A non-zero exit is reported,
# not fatal: doctor exits 1 when it has findings.
#
# Portable: POSIX-ish bash plus perl for millisecond timing. No GNU-only flags.

set -euo pipefail

BENCH_COMMANDS_ROOT=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)

# bench_commands_list prints one command per line, arguments split by spaces.
bench_commands_list() {
  cat <<'EOF'
list
validate
lint
status
doctor
sync
sync --check
sync --dry-run
sync -t claude
graph
compare claude codex
render .agnostic-ai/rules/rule-0000.md
explain .agnostic-ai/rules/rule-0000.md
why CLAUDE.md
migrate --dry-run
EOF
}

bench_now_ms() {
  perl -MTime::HiRes=time -e 'printf "%.0f\n", time * 1000'
}

# bench_commands_run <bin> <project-dir> <runs> times each command in the dir.
bench_commands_run() {
  local bin="$1" dir="$2" runs="$3" line best best_code start end took code i
  local -a args
  while IFS= read -r line; do
    read -r -a args <<<"$line"
    best=""
    best_code=0
    for ((i = 0; i < runs; i++)); do
      start=$(bench_now_ms)
      code=0
      (cd "$dir" && "$bin" "${args[@]}" >/dev/null 2>&1) || code=$?
      end=$(bench_now_ms)
      took=$((end - start))
      if [ -z "$best" ] || [ "$took" -lt "$best" ]; then
        best=$took
        best_code=$code
      fi
    done
    printf '%s\t%s\t%s\n' "$best" "$best_code" "$line"
  done < <(bench_commands_list)
}

bench_commands_main() {
  local bin="$BENCH_COMMANDS_ROOT/agnostic-ai" specs=500 runs=3 dir
  while [ $# -gt 0 ]; do
    case "$1" in
      --bin) bin="${2:?--bin needs a path}"; shift 2 ;;
      --specs) specs="${2:?--specs needs a number}"; shift 2 ;;
      --runs) runs="${2:?--runs needs a number}"; shift 2 ;;
      *) printf 'error: unknown argument %s\n' "$1" >&2; return 2 ;;
    esac
  done
  case "$specs$runs" in
    *[!0-9]*) printf 'error: --specs and --runs need positive integers\n' >&2; return 2 ;;
  esac
  [ "$specs" -ge 1 ] && [ "$runs" -ge 1 ] || { printf 'error: --specs and --runs need positive integers\n' >&2; return 2; }
  [ -x "$bin" ] || { printf 'error: no binary at %s; run make build\n' "$bin" >&2; return 1; }
  bin="$(CDPATH='' cd -- "$(dirname -- "$bin")" && pwd)/$(basename -- "$bin")"
  dir=$(mktemp -d)
  BENCH_COMMANDS_DIR="$dir"
  trap 'rm -rf "$BENCH_COMMANDS_DIR"' EXIT
  (cd "$BENCH_COMMANDS_ROOT" && AGNOSTIC_AI_BENCH_FIXTURE="$dir" AGNOSTIC_AI_BENCH_SPECS="$specs" \
    go test -count=1 -run '^TestWriteBenchFixture$' ./internal/cli/ >&2)
  (cd "$dir" && git init -q && "$bin" sync -q && git add -A && git -c user.name=bench -c user.email=bench@localhost \
    -c commit.gpgsign=false -c core.hooksPath=/dev/null commit -q --no-verify -m fixture)
  printf '# %s specs, best of %s runs, %s\n' "$specs" "$runs" "$("$bin" --version)"
  bench_commands_run "$bin" "$dir" "$runs"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  bench_commands_main "$@"
fi
