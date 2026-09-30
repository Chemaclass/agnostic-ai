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
