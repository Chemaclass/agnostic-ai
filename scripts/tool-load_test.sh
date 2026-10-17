#!/usr/bin/env bash
#
# bashunit tests for scripts/tool-load.sh
#
# Run:
#   bashunit scripts/tool-load_test.sh
#
# No test installs a tool or reaches the network. The real tools run in
# the weekly Tool load workflow (.github/workflows/tool-load.yml).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/tool-load.sh"

FIXTURES=""

function set_up() {
  FIXTURES=$(mktemp -d)
  TOOL_LOAD_FAILED=0
}

function tear_down() {
  [ -n "$FIXTURES" ] && rm -rf "$FIXTURES"
}

function test_expect_reports_a_found_marker() {
  printf 'skills: probe-skill\n' >"$FIXTURES/out"
  assert_equals "ok	gemini	0.62.0	skills load" "$(tool_load_expect gemini 0.62.0 "skills load" probe-skill "$FIXTURES/out")"
}

function test_expect_fails_on_a_missing_marker() {
  printf 'No skills discovered.\n' >"$FIXTURES/out"
  tool_load_expect gemini 0.62.0 "skills load" probe-skill "$FIXTURES/out" >/dev/null
  assert_equals 1 "$TOOL_LOAD_FAILED"
}

function test_project_syncs_only_the_named_target() {
  printf '#!/bin/sh\necho "$@" >"%s/sync-args"\n' "$FIXTURES" >"$FIXTURES/fake-aai"
  chmod +x "$FIXTURES/fake-aai"
  # shellcheck disable=SC2034
  TOOL_LOAD_BIN="$FIXTURES/fake-aai"
  mkdir -p "$FIXTURES/project"
  tool_load_project "$FIXTURES/project" codex
  assert_equals "version: 1
targets: [codex]" "$(cat "$FIXTURES/project/agnostic-ai.yaml")"
  assert_equals "sync -q" "$(cat "$FIXTURES/sync-args")"
  assert_contains "AAI-PROBE-SKILL" "$(cat "$FIXTURES/project/.agnostic-ai/skills/probe-skill/SKILL.md")"
}

function test_trust_marks_the_project_for_codex_and_gemini() {
  mkdir -p "$FIXTURES/home/.codex" "$FIXTURES/home/.gemini"
  tool_load_trust codex "$FIXTURES/home" /work/p
  tool_load_trust gemini "$FIXTURES/home" /work/p
  assert_contains '[projects."/work/p"]' "$(cat "$FIXTURES/home/.codex/config.toml")"
  assert_contains '"/work/p": "TRUST_FOLDER"' "$(cat "$FIXTURES/home/.gemini/trustedFolders.json")"
}

function test_an_unknown_tool_is_refused() {
  local code=0
  tool_load_checks cursor 1 "$FIXTURES" 2>/dev/null || code=$?
  assert_equals 2 "$code"
}

function test_latest_probe_resolves_an_exact_package_version() {
  # shellcheck disable=SC2329
  function npm() { printf '1.2.3\n'; }
  local out
  out=$(tool_load_resolve_package codex)
  unset -f npm
  assert_equals '@openai/codex@1.2.3' "$out"
}

function test_exact_probe_replay_refuses_registry_version_mismatch() {
  # shellcheck disable=SC2329
  function npm() { printf '1.2.4\n'; }
  local code=0
  TOOL_LOAD_CODEX_VERSION=1.2.3 tool_load_resolve_package codex >/dev/null 2>&1 || code=$?
  unset -f npm
  assert_not_equals 0 "$code"
}

function test_probe_refuses_a_version_range_or_command_text() {
  local code=0
  TOOL_LOAD_CODEX_VERSION='latest; touch marker' tool_load_resolve_package codex >/dev/null 2>&1 || code=$?
  assert_not_equals 0 "$code"
}

function test_saved_lock_replay_does_not_resolve_latest_again() {
  mkdir -p "$FIXTURES/locked"
  printf '{"dependencies":{"@openai/codex":"1.2.3"}}\n' > "$FIXTURES/locked/package.json"
  # shellcheck disable=SC2329
  function npm() { return 1; }
  local out
  out=$(TOOL_LOAD_LOCK_DIR="$FIXTURES/locked" tool_load_resolve_package codex)
  unset -f npm
  assert_equals '@openai/codex@1.2.3' "$out"
}

function test_probe_records_lock_before_installing_the_saved_graph() {
  local original_project original_trust original_run original_checks
  original_project=$(declare -f tool_load_project)
  original_trust=$(declare -f tool_load_trust)
  original_run=$(declare -f tool_load_run)
  original_checks=$(declare -f tool_load_checks)
  local log="$FIXTURES/install.log"
  # shellcheck disable=SC2329
  function npm() {
    printf '%s\n' "$*" >> "$log"
    if [[ "$1" == view ]]; then printf '1.2.3\n'; fi
    if [[ "$1" == install ]]; then
      if [[ "$PWD" == "$TOOL_LOAD_PREFIX" ]] && grep -q '"private":true' package.json; then
        printf 'private manifest in package directory\n' >> "$log"
      fi
      mkdir -p "$TOOL_LOAD_PREFIX"
      printf '{"dependencies":{"@openai/codex":"1.2.3"}}\n' > "$TOOL_LOAD_PREFIX/package.json"
      printf '{"lockfileVersion":3}\n' > "$TOOL_LOAD_PREFIX/package-lock.json"
    fi
  }
  # shellcheck disable=SC2329
  function node() { printf '/usr/bin/node'; }
  # shellcheck disable=SC2329
  function tool_load_project() { :; }
  # shellcheck disable=SC2329
  function tool_load_trust() { :; }
  # shellcheck disable=SC2329
  function tool_load_run() { printf '1.2.3\n' > "$1"; }
  # shellcheck disable=SC2329
  function tool_load_checks() { :; }
  TOOL_LOAD_REPORT_DIR="$FIXTURES/report" tool_load_main --bin /unused codex
  unset -f npm node
  eval "$original_project"
  eval "$original_trust"
  eval "$original_run"
  eval "$original_checks"
  assert_contains '--package-lock-only --ignore-scripts' "$(cat "$log")"
  assert_contains 'ci --prefix' "$(cat "$log")"
  assert_contains 'private manifest in package directory' "$(cat "$log")"
  assert_equals '@openai/codex@1.2.3' "$(cat "$FIXTURES/report/selected-packages.txt")"
  assert_file_exists "$FIXTURES/report/package-lock.json"
}
