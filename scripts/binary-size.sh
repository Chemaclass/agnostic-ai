#!/usr/bin/env bash
#
# binary-size.sh - keep the release binary from growing unnoticed.
#
# A pull request is checked by growth: its build may be at most
# BINARY_SIZE_GROWTH_DEFAULT bytes larger than its base's build. A pull request
# that needs more sets the bytes it needs in scripts/binary-size-growth and says
# why there; the number counts only while that file differs from the base.
# Pushes to main and releases also check the absolute upper bound in
# scripts/binary-size-budget, which only a release raises.
#
# Usage:
#   scripts/binary-size.sh <binary> [budget-file]
#   scripts/binary-size.sh --growth <head-binary> <base-binary> <base-ref>
#   scripts/binary-size.sh --set-budget <binary> [budget-file]
#
# Measure linux/amd64 builds with the release flags (make size-check,
# make size-growth-check).
#
# Portable: POSIX-ish bash. No GNU-only flags.

set -euo pipefail

BINARY_SIZE_ROOT=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
BINARY_SIZE_GROWTH_DEFAULT=16384
BINARY_SIZE_HEADROOM=2097152

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

file_size() {
  [ -f "$1" ] || { printf 'error: no binary at %s\n' "$1" >&2; return 1; }
  wc -c < "$1" | tr -d '[:space:]'
}

check_binary_size() {
  local binary="${1:?usage: $0 <binary> [budget-file]}"
  local budget_file="${2:-$BINARY_SIZE_ROOT/scripts/binary-size-budget}"
  local size budget
  size=$(file_size "$binary") || return 1
  budget=$(read_budget "$budget_file") || return 1
  if [ "$size" -gt "$budget" ]; then
    printf 'binary-size: %s bytes, %s over the %s byte budget\n' "$size" "$((size - budget))" "$budget" >&2
    printf 'A release raises the budget in %s (make size-budget).\n' "$budget_file" >&2
    return 1
  fi
  printf 'binary-size: %s bytes, %s under the %s byte budget\n' "$size" "$((budget - size))" "$budget"
}

# growth_allowance prints the bytes a change may add over <base-ref>: the
# number in the growth file while the change edits that file, else the default.
growth_allowance() {
  local base_ref="$1"
  local growth_file="${2:-$BINARY_SIZE_ROOT/scripts/binary-size-growth}"
  local status=0
  [ -f "$growth_file" ] || { printf '%s\n' "$BINARY_SIZE_GROWTH_DEFAULT"; return; }
  git -C "$(dirname -- "$growth_file")" diff --quiet "$base_ref" -- "$growth_file" || status=$?
  case "$status" in
    0) printf '%s\n' "$BINARY_SIZE_GROWTH_DEFAULT" ;;
    1) read_budget "$growth_file" ;;
    *)
      printf 'error: cannot compare %s with %s\n' "$growth_file" "$base_ref" >&2
      return 1
      ;;
  esac
}

check_binary_growth() {
  local head="${1:?usage: $0 --growth <head-binary> <base-binary> <base-ref> [growth-file]}"
  local base="${2:?usage: $0 --growth <head-binary> <base-binary> <base-ref> [growth-file]}"
  local base_ref="${3:?usage: $0 --growth <head-binary> <base-binary> <base-ref> [growth-file]}"
  local growth_file="${4:-$BINARY_SIZE_ROOT/scripts/binary-size-growth}"
  local head_size base_size growth allowance
  head_size=$(file_size "$head") || return 1
  base_size=$(file_size "$base") || return 1
  allowance=$(growth_allowance "$base_ref" "$growth_file") || return 1
  growth=$((head_size - base_size))
  if [ "$growth" -gt "$allowance" ]; then
    printf 'binary-size: this change grows the binary by %s bytes (%s to %s), over the %s byte allowance\n' \
      "$growth" "$base_size" "$head_size" "$allowance" >&2
    printf 'If the growth is worth it, set the bytes this change needs in %s and say why there.\n' "$growth_file" >&2
    return 1
  fi
  if [ "$growth" -lt 0 ]; then
    printf 'binary-size: this change shrinks the binary by %s bytes (%s to %s)\n' "$((-growth))" "$base_size" "$head_size"
    return
  fi
  printf 'binary-size: this change grows the binary by %s bytes (%s to %s), within the %s byte allowance\n' \
    "$growth" "$base_size" "$head_size" "$allowance"
}

# set_budget writes the binary's size plus headroom as the new upper bound,
# keeping the file's comments. A release runs it.
set_budget() {
  local binary="${1:?usage: $0 --set-budget <binary> [budget-file]}"
  local budget_file="${2:-$BINARY_SIZE_ROOT/scripts/binary-size-budget}"
  local size tmp
  size=$(file_size "$binary") || return 1
  tmp=$(mktemp)
  if ! { grep -e '^[[:space:]]*#' "$budget_file" || true; printf '%s\n' "$((size + BINARY_SIZE_HEADROOM))"; } > "$tmp" ||
    ! cat "$tmp" > "$budget_file"; then
    rm -f "$tmp"
    return 1
  fi
  rm -f "$tmp"
  printf 'binary-size: budget set to %s bytes (%s measured plus %s headroom)\n' \
    "$((size + BINARY_SIZE_HEADROOM))" "$size" "$BINARY_SIZE_HEADROOM"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  case "${1:-}" in
    --growth) shift; check_binary_growth "$@" ;;
    --set-budget) shift; set_budget "$@" ;;
    *) check_binary_size "$@" ;;
  esac
fi
