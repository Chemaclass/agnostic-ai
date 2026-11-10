#!/usr/bin/env bash
# shellcheck disable=SC2016 # fake tools are written as literal shell

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Each test gets a fixture repo with its own pins and a PATH that holds
# only fake tools and the few system utilities the script uses, so a real
# zola or the macOS java stub never answers for a fake.
function set_up() {
  fixture="$(mktemp -d "${TMPDIR:-/tmp}/agnostic-doctor-dev-tests.XXXXXX")"
  root="$fixture/repo"
  bin="$fixture/bin"
  sys="$fixture/sys"
  gopath="$fixture/gopath"
  mkdir -p "$root/.github/workflows" "$bin" "$sys" "$gopath/bin"
  local util
  for util in sed head awk tr grep cat dirname; do
    ln -s "$(command -v "$util")" "$sys/$util"
  done

  printf 'module example\n\ngo 1.30.0\n\ntoolchain go1.30.1\n' > "$root/go.mod"
  printf '[tools]\ngo = "1.30.1"\nnode = "22"\njava = "temurin-21"\nzola = "0.20.0"\n' > "$root/mise.toml"
  printf 'GOLANGCI_LINT_VERSION := v2.9.0\nLEFTHOOK_VERSION := v1.2.3\nZOLA_VERSION := 0.20.0\n' > "$root/Makefile"
  printf 'jobs:\n  shell:\n    steps:\n      - env:\n          BASHUNIT_VERSION: 0.40.0\n' > "$root/.github/workflows/ci.yml"

  fake_tool go 'case "$1 $2" in
  "env GOVERSION") echo go1.30.1 ;;
  "env GOPATH") echo "$FAKE_GOPATH" ;;
  "version "*) echo "go version go1.30.1 darwin/arm64" ;;
esac'
  fake_tool gofmt 'exit 0'
  fake_tool golangci-lint 'echo 2.9.0'
  fake_tool lefthook 'echo 1.2.3'
  fake_tool node 'echo v22.11.0'
  fake_tool npm 'echo 10.9.0'
  fake_tool java 'echo "openjdk version \"21.0.5\" 2024-10-15" >&2'
  fake_tool zola 'echo "zola 0.20.0"'
  fake_tool bashunit 'printf "\033[1m\033[32mbashunit\033[0m - 0.40.0\n"'
}

function tear_down() {
  case "$fixture" in
    "${TMPDIR:-/tmp}"/agnostic-doctor-dev-tests.*) rm -rf "$fixture" ;;
    *) return 1 ;;
  esac
}

function fake_tool() {
  local dir="${3:-$bin}"
  mkdir -p "$dir"
  printf '#!/bin/sh\n%s\n' "$2" > "$dir/$1"
  chmod +x "$dir/$1"
}

function run_doctor() {
  env -i HOME="$fixture" PATH="${DOCTOR_TEST_PATH:-$bin:$gopath/bin:$sys}" \
    FAKE_GOPATH="$gopath" DOCTOR_DEV_ROOT="$root" "$@" \
    "$BASH" "$SCRIPT_DIR/doctor-dev.sh" 2>&1
}

function test_every_tool_at_its_pin_passes() {
  local out
  out="$(run_doctor)"
  assert_successful_code "$?"
  assert_matches "go +1\.30\.1 +1\.30\.1 +ok" "$out"
  assert_matches "golangci-lint +2\.9\.0 +2\.9\.0 +ok" "$out"
  assert_matches "lefthook +1\.2\.3 +1\.2\.3 +ok" "$out"
  assert_matches "node +22\.11\.0 +22 +ok" "$out"
  assert_matches "java +21\.0\.5 +21 +ok" "$out"
  assert_matches "zola +0\.20\.0 +0\.20\.0 +ok" "$out"
  assert_matches "bashunit +0\.40\.0 +0\.40\.0 +ok" "$out"
  assert_contains "doctor-dev: ok" "$out"
}

function test_a_missing_tool_fails_and_later_rows_still_print() {
  rm "$bin/zola"
  local out
  out="$(run_doctor)"
  assert_general_error "$?"
  assert_matches "zola +- +0\.20\.0 +missing" "$out"
  assert_matches "bashunit +0\.40\.0 +0\.40\.0 +ok" "$out"
  assert_contains "mise install" "$out"
  assert_contains "doctor-dev: 1 problem" "$out"
}

function test_an_exact_pin_mismatch_is_wrong() {
  fake_tool zola 'echo "zola 0.21.0"'
  local out
  out="$(run_doctor)"
  assert_general_error "$?"
  assert_matches "zola +0\.21\.0 +0\.20\.0 +wrong" "$out"
}

function test_a_stale_go_tool_points_at_make_tools() {
  fake_tool golangci-lint 'echo 2.8.0'
  local out
  out="$(run_doctor)"
  assert_general_error "$?"
  assert_matches "golangci-lint +2\.8\.0 +2\.9\.0 +wrong" "$out"
  assert_contains "make tools" "$out"
}

function test_node_below_the_pinned_major_fails() {
  fake_tool node 'echo v20.18.0'
  local out
  out="$(run_doctor)"
  assert_general_error "$?"
  assert_matches "node +20\.18\.0 +22 +wrong" "$out"
}

function test_node_above_the_pinned_major_warns_without_failing() {
  fake_tool node 'echo v26.3.0'
  local out
  out="$(run_doctor)"
  assert_successful_code "$?"
  assert_matches "node +26\.3\.0 +22 +newer" "$out"
  assert_contains "doctor-dev: ok, 1 warning" "$out"
}

function test_java_above_the_pinned_major_warns_without_failing() {
  fake_tool java 'echo "openjdk version \"25.0.1\" 2025-10-21" >&2'
  local out
  out="$(run_doctor)"
  assert_successful_code "$?"
  assert_matches "java +25\.0\.1 +21 +newer" "$out"
}

function test_java_is_found_through_java_home() {
  rm "$bin/java"
  fake_tool java 'echo "openjdk version \"21.0.2\" 2024-01-16" >&2' "$fixture/jdk/bin"
  local out
  out="$(run_doctor JAVA_HOME="$fixture/jdk")"
  assert_successful_code "$?"
  assert_matches "java +21\.0\.2 +21 +ok" "$out"
}

function test_old_style_java_version_reads_as_its_major() {
  fake_tool java 'echo "java version \"1.8.0_292\"" >&2'
  local out
  out="$(run_doctor)"
  assert_general_error "$?"
  assert_matches "java +1\.8\.0_292 +21 +wrong" "$out"
}

function test_a_failing_java_probe_reads_as_missing() {
  fake_tool java 'echo "Unable to locate a Java Runtime." >&2; exit 1'
  local out
  out="$(run_doctor)"
  assert_general_error "$?"
  assert_matches "java +- +21 +missing" "$out"
}

function test_skip_jetbrains_skips_java() {
  rm "$bin/java"
  local out
  out="$(run_doctor SKIP_JETBRAINS=1)"
  assert_successful_code "$?"
  assert_matches "java +- +21 +skipped" "$out"
}

function test_npm_through_a_mise_shim_warns_about_home() {
  rm "$bin/node" "$bin/npm"
  fake_tool node 'echo v22.11.0' "$fixture/mise/shims"
  fake_tool npm 'echo 10.9.0' "$fixture/mise/shims"
  local out
  out="$(DOCTOR_TEST_PATH="$bin:$fixture/mise/shims:$gopath/bin:$sys" run_doctor)"
  assert_successful_code "$?"
  assert_contains "warning: npm runs through a version manager shim" "$out"
  assert_contains "mise exec -- make ci-local" "$out"
}

function test_go_bin_dir_missing_from_path_warns() {
  local out
  out="$(DOCTOR_TEST_PATH="$bin:$sys" run_doctor)"
  assert_successful_code "$?"
  assert_contains "warning: $gopath/bin is not on PATH" "$out"
}

function test_missing_go_does_not_stop_the_report() {
  rm "$bin/go" "$bin/gofmt"
  local out
  out="$(run_doctor)"
  assert_general_error "$?"
  assert_matches "go +- +1\.30\.1 +missing" "$out"
  assert_matches "gofmt +- +any +missing" "$out"
  assert_matches "bashunit +0\.40\.0 +0\.40\.0 +ok" "$out"
  assert_contains "doctor-dev: 2 problems" "$out"
}
