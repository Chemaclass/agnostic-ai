#!/usr/bin/env bash
# Prints the module's packages, one per line, without the packages given as
# arguments. An argument ending in /... also drops its sub-packages.
# Usage: scripts/go-packages.sh ./internal/cli ./tests/integration/...
# Runs under macOS bash 3.2 and Git Bash on Windows.
set -euo pipefail

module=$(go list -m)
all_packages=$(go list ./...)

excluded=()
for arg in "$@"; do
  case "$arg" in
    ./*) excluded+=("$module/${arg#./}") ;;
    *) excluded+=("$arg") ;;
  esac
done

matched=()
kept=()
for i in ${excluded[@]+"${!excluded[@]}"}; do
  matched[i]=false
done

while IFS= read -r package; do
  package=${package%$'\r'}
  [ -n "$package" ] || continue
  keep=true
  for i in ${excluded[@]+"${!excluded[@]}"}; do
    pattern=${excluded[i]}
    case "$pattern" in
      */...)
        root=${pattern%/...}
        if [ "$package" = "$root" ] || [ "${package#"$root"/}" != "$package" ]; then
          keep=false
          matched[i]=true
        fi
        ;;
      *)
        if [ "$package" = "$pattern" ]; then
          keep=false
          matched[i]=true
        fi
        ;;
    esac
  done
  if [ "$keep" = true ]; then
    kept+=("$package")
  fi
done <<< "$all_packages"

# A misspelled argument would otherwise drop nothing and pass unnoticed.
for i in ${excluded[@]+"${!excluded[@]}"}; do
  if [ "${matched[i]}" != true ]; then
    printf 'go-packages: %s matches no package\n' "${excluded[i]}" >&2
    exit 1
  fi
done

if [ ${#kept[@]} -gt 0 ]; then
  printf '%s\n' "${kept[@]}"
fi
