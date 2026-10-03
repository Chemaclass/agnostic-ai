#!/usr/bin/env bash
#
# binary-size.sh - fail when the release binary outgrows its budget.
#
# The budget lives in scripts/binary-size-budget so that a change which
# grows the binary past it, usually a new dependency, has to raise the
# number in the same pull request and say why.
#
# Usage:
#   scripts/binary-size.sh <binary> [budget-file]
#
# Measure a linux/amd64 build with the release flags (make size-check).
#
# Portable: POSIX-ish bash. No GNU-only flags.

set -euo pipefail

BINARY_SIZE_ROOT=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)

read_budget() {
  local budget
  budget=$(grep -v -e '^[[:space:]]*#' -e '^[[:space:]]*$' "$1" | tr -d '[:blank:]') || true
  case "$budget" in
    '' | 0* | *[!0-9]* | ?????????????????*)
      printf 'error: %s must hold one byte count: 1 to 16 digits, no leading zero, got "%s"\n' "$1" "$budget" >&2
      return 1
      ;;
  esac
  printf '%s\n' "$budget"
}

check_binary_size() {
  local binary="${1:?usage: $0 <binary> [budget-file]}"
  local budget_file="${2:-$BINARY_SIZE_ROOT/scripts/binary-size-budget}"
  local size budget
  [ -f "$binary" ] || { printf 'error: no binary at %s\n' "$binary" >&2; return 1; }
  size=$(wc -c < "$binary" | tr -d '[:space:]')
  budget=$(read_budget "$budget_file") || return 1
  if [ "$size" -gt "$budget" ]; then
    printf 'binary-size: %s bytes, %s over the %s byte budget\n' "$size" "$((size - budget))" "$budget" >&2
    printf 'If the growth is worth it, raise the budget in %s and say why in the PR.\n' "$budget_file" >&2
    return 1
  fi
  printf 'binary-size: %s bytes, %s under the %s byte budget\n' "$size" "$((budget - size))" "$budget"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  check_binary_size "$@"
fi
