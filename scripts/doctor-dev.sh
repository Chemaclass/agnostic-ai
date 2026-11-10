#!/usr/bin/env bash
#
# doctor-dev.sh - list each contributor tool with the version found and
# the version wanted, so a missing tool shows up before make ci-local.
#
# Usage:
#   scripts/doctor-dev.sh        (or: make doctor-dev)
#
# Wanted versions come from the files that pin them: go.mod, mise.toml,
# the Makefile and .github/workflows/ci.yml. Exits 1 when a tool is missing
# or has the wrong version. Node.js and Java are pinned by major version; a
# newer major is a warning, since CI runs the pinned one. SKIP_JETBRAINS=1
# skips Java, as make ci-local does.
#
# Portable: POSIX-ish bash. No GNU-only flags.

set -euo pipefail

DOCTOR_DEV_ROOT=${DOCTOR_DEV_ROOT:-$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)}

doctor_problems=0
doctor_warnings=0

pin_from() {
  { sed -n "$2" "$DOCTOR_DEV_ROOT/$1" 2>/dev/null || true; } | sed -n 1p
}

wanted_go() {
  local want
  want=$(pin_from go.mod 's/^toolchain go\([0-9][0-9.]*\)[[:space:]]*$/\1/p')
  [ -n "$want" ] || want=$(pin_from go.mod 's/^go \([0-9][0-9.]*\)[[:space:]]*$/\1/p')
  printf '%s\n' "$want"
}

wanted_makefile() {
  pin_from Makefile "s/^$1 := v\{0,1\}\([^[:space:]]*\)[[:space:]]*\$/\1/p"
}

wanted_mise_major() {
  local want
  want=$(pin_from mise.toml "s/^$1 = \"\([^\"]*\)\"[[:space:]]*\$/\1/p")
  want=${want##*-}
  printf '%s\n' "${want%%.*}"
}

wanted_bashunit() {
  pin_from .github/workflows/ci.yml 's/^[[:space:]]*BASHUNIT_VERSION:[[:space:]]*\([^[:space:]]*\)[[:space:]]*$/\1/p'
}

has() {
  command -v "$1" >/dev/null 2>&1
}

found_go() {
  has go || return 0
  { (cd "$DOCTOR_DEV_ROOT" && go env GOVERSION 2>/dev/null) || true; } | awk '!n++ { sub(/^go/, "", $1); print $1 }'
}

found_gofmt() {
  if has gofmt; then printf 'yes\n'; fi
}

found_golangci_lint() {
  has golangci-lint || return 0
  { golangci-lint version --short 2>/dev/null || true; } | sed -n 1p
}

found_lefthook() {
  has lefthook || return 0
  { lefthook version 2>/dev/null || true; } | awk '!n++ { sub(/^v/, "", $1); print $1 }'
}

found_node() {
  has node || return 0
  { node --version 2>/dev/null || true; } | awk '!n++ { sub(/^v/, "", $1); print $1 }'
}

found_npm() {
  has npm || return 0
  { npm --version 2>/dev/null || true; } | sed -n 1p
}

# Gradle uses JAVA_HOME when it is set and fails when it is invalid, so a
# set JAVA_HOME is the only Java checked.
java_bin() {
  if [ -n "${JAVA_HOME:-}" ]; then
    if [ -x "$JAVA_HOME/bin/java" ]; then printf '%s\n' "$JAVA_HOME/bin/java"; fi
  elif has java; then
    command -v java
  fi
}

found_java() {
  local java
  java=$(java_bin)
  [ -n "$java" ] || return 0
  { "$java" -version 2>&1 || true; } | sed -n 's/.*version "\([^"]*\)".*/\1/p' | sed -n 1p
}

found_zola() {
  has zola || return 0
  { zola --version 2>/dev/null || true; } | awk '!n++ { print $NF }'
}

found_bashunit() {
  has bashunit || return 0
  { bashunit --version 2>/dev/null || true; } | tr -d '\033' | sed 's/\[[0-9;]*m//g' | awk 'NF && !n++ { print $NF }'
}

# Java 8 and older report 1.N.
major_of() {
  local version="$1"
  case "$version" in 1.*) version=${version#1.} ;; esac
  printf '%s\n' "${version%%[!0-9]*}"
}

row() {
  printf '%-14s %-14s %-10s %s\n' "$1" "$2" "$3" "$4"
}

report() {
  local name="$1" found="$2" wanted="$3" status="$4" hint="$5"
  row "$name" "${found:--}" "$wanted" "$status"
  case "$status" in
    missing | wrong)
      doctor_problems=$((doctor_problems + 1))
      printf '  fix: %s\n' "$hint"
      ;;
    newer)
      doctor_warnings=$((doctor_warnings + 1))
      printf '  note: CI uses %s %s; a newer major works for local checks\n' "$name" "$wanted"
      ;;
  esac
}

check_exact() {
  local name="$1" found="$2" wanted="$3" hint="$4" status=ok
  if [ -z "$found" ]; then
    status=missing
  elif [ "$found" != "$wanted" ]; then
    status=wrong
  fi
  report "$name" "$found" "$wanted" "$status" "$hint"
}

check_present() {
  local status=ok
  [ -n "$2" ] || status=missing
  report "$1" "$2" any "$status" "$3"
}

# A major above the pin is `newer` (a warning) unless $5 names another status.
check_major() {
  local name="$1" found="$2" wanted="$3" hint="$4" above="${5:-newer}" status=ok major
  major=$(major_of "$found")
  if [ -z "$found" ]; then
    status=missing
  elif [ -z "$major" ] || [ "$major" -lt "$wanted" ]; then
    status=wrong
  elif [ "$major" -gt "$wanted" ]; then
    status=$above
  fi
  report "$name" "$found" "$wanted" "$status" "$hint"
}

warn() {
  doctor_warnings=$((doctor_warnings + 1))
  printf 'warning: %s\n' "$1"
}

check_go_bin_on_path() {
  has go || return 0
  local dir
  dir=$(go env GOBIN 2>/dev/null || true)
  if [ -z "$dir" ]; then
    dir=$(go env GOPATH 2>/dev/null || true)
    dir=${dir%%:*}
    [ -n "$dir" ] || return 0
    dir="$dir/bin"
  fi
  case ":$PATH:" in
    *":$dir:"*) ;;
    *) warn "$dir is not on PATH, and 'make tools' installs golangci-lint and lefthook there. Add it to PATH." ;;
  esac
}

# A version manager shim finds its config through HOME, and
# TestProjectLifecycle_RealNpmBootstrap changes HOME, so npm through a shim
# stops resolving in that test.
check_npm_shim() {
  has npm || return 0
  case "$(command -v npm)" in
    */shims/*)
      warn "npm runs through a version manager shim ($(command -v npm)). A test that changes HOME then loses it. Run 'mise exec -- make ci-local', or use 'mise activate' so real install folders are on PATH."
      ;;
  esac
}

main() {
  local go_want lint_want lefthook_want zola_want node_want java_want bashunit_want
  go_want=$(wanted_go)
  lint_want=$(wanted_makefile GOLANGCI_LINT_VERSION)
  lefthook_want=$(wanted_makefile LEFTHOOK_VERSION)
  zola_want=$(wanted_makefile ZOLA_VERSION)
  node_want=$(wanted_mise_major node)
  java_want=$(wanted_mise_major java)
  bashunit_want=$(wanted_bashunit)

  row tool found wanted status
  check_exact go "$(found_go)" "$go_want" "install Go $go_want, or run 'mise install'"
  check_present gofmt "$(found_gofmt)" "gofmt ships with Go; put Go's bin folder on PATH"
  check_exact golangci-lint "$(found_golangci_lint)" "$lint_want" "run 'make tools'"
  check_exact lefthook "$(found_lefthook)" "$lefthook_want" "run 'make tools'"
  check_major node "$(found_node)" "$node_want" "install Node.js $node_want, or run 'mise install'"
  check_present npm "$(found_npm)" "npm ships with Node.js; reinstall Node.js $node_want"
  if [ "${SKIP_JETBRAINS:-}" = 1 ]; then
    report java "" "$java_want" skipped ""
  else
    # The JetBrains build asks Gradle for a Java 21 toolchain and does not download one.
    check_major java "$(found_java)" "$java_want" "install JDK $java_want, or run 'mise install', or set JAVA_HOME to it; without Java, run 'make doctor-dev SKIP_JETBRAINS=1' and 'make ci-local SKIP_JETBRAINS=1'" wrong
  fi
  check_exact zola "$(found_zola)" "$zola_want" "run 'mise install', or download Zola $zola_want from https://github.com/getzola/zola/releases"
  check_exact bashunit "$(found_bashunit)" "$bashunit_want" "download https://github.com/TypedDevs/bashunit/releases/download/$bashunit_want/bashunit to a folder on PATH"

  echo
  check_go_bin_on_path
  check_npm_shim

  local warnings=''
  case "$doctor_warnings" in
    0) ;;
    1) warnings=', 1 warning' ;;
    *) warnings=", $doctor_warnings warnings" ;;
  esac
  case "$doctor_problems" in
    0) printf 'doctor-dev: ok%s\n' "$warnings" ;;
    1) printf 'doctor-dev: 1 problem%s\n' "$warnings"; return 1 ;;
    *) printf 'doctor-dev: %s problems%s\n' "$doctor_problems" "$warnings"; return 1 ;;
  esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
