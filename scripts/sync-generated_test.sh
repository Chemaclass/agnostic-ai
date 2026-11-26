#!/usr/bin/env bash
# shellcheck disable=SC2016 # fake binaries are written as literal shell

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SYNC_SCRIPT="$REPO_ROOT/.agnostic-ai/scripts/sync-generated.sh"

# Each test gets a Git project with the script at its real place and a PATH
# that holds only the utilities the script uses, so no installed
# agnostic-ai answers for the fake one.
function set_up() {
  fixture="$(mktemp -d "${TMPDIR:-/tmp}/agnostic-sync-generated-tests.XXXXXX")"
  fixture="$(cd "$fixture" && pwd -P)"
  project="$fixture/project"
  sys="$fixture/sys"
  calls="$fixture/calls"
  mkdir -p "$project/.agnostic-ai/scripts" "$project/cmd" "$sys"
  local util
  for util in git grep head tail dirname; do
    ln -s "$(command -v "$util")" "$sys/$util"
  done
  cp "$SYNC_SCRIPT" "$project/.agnostic-ai/scripts/"
  printf 'version: 1\n' > "$project/agnostic-ai.yaml"
  printf 'package main\n' > "$project/cmd/main.go"
  printf '/agnostic-ai\n' > "$project/.gitignore"
  git_quiet -C "$project" init
  commit_all "$project" init
}

function tear_down() {
  case "$fixture" in
    */agnostic-sync-generated-tests.*) rm -rf "$fixture" ;;
    *) return 1 ;;
  esac
}

function git_quiet() {
  git -c user.name=test -c user.email=test@example.com -c commit.gpgsign=false "$@" >/dev/null 2>&1
}

function commit_all() {
  git_quiet -C "$1" add -A
  git_quiet -C "$1" commit -m "$2"
}

# fake_bin <path> <revision> <body> writes a fake agnostic-ai that records
# the folder it ran in. Go keeps vcs.revision=<commit> in a real binary's
# build info; the comment stands in for it.
function fake_bin() {
  local path="$1" revision="$2" body="$3"
  {
    printf '#!/bin/sh\n'
    [ -n "$revision" ] && printf '# vcs.revision=%s\n' "$revision"
    printf 'pwd >> "%s"\n' "$calls"
    printf '%s\n' "$body"
  } > "$path"
  chmod +x "$path"
}

function head_commit() {
  git -C "${1:-$project}" rev-parse HEAD
}

# run_script <script> [env assignments...] runs the script with stdout and
# stderr kept apart in $out and $err.
function run_script() {
  local script="$1"
  shift
  out="$fixture/out"
  err="$fixture/err"
  (cd "$fixture" && env -i HOME="$fixture" PATH="$sys" "$@" /bin/sh "$script" >"$out" 2>"$err")
}

function test_a_missing_binary_prints_one_line_and_exits_zero() {
  run_script "$project/.agnostic-ai/scripts/sync-generated.sh"
  assert_successful_code "$?"
  assert_same "agnostic-ai: binary not found; run make build, then agnostic-ai sync" "$(cat "$err")"
  assert_empty "$(cat "$out")"
}

function test_a_missing_binary_in_a_claude_cloud_session_is_silent() {
  run_script "$project/.agnostic-ai/scripts/sync-generated.sh" CLAUDE_CODE_REMOTE=true
  assert_successful_code "$?"
  assert_empty "$(cat "$err")"
}

function test_lock_contention_is_silent() {
  fake_bin "$project/agnostic-ai" "$(head_commit)" \
    'echo "error: project is locked by agnostic-ai (pid 42)" >&2; exit 1'
  run_script "$project/.agnostic-ai/scripts/sync-generated.sh"
  assert_successful_code "$?"
  assert_empty "$(cat "$err")"
  assert_empty "$(cat "$out")"
}

function test_kept_files_are_named_on_stderr() {
  fake_bin "$project/agnostic-ai" "$(head_commit)" \
    'printf "  ~ kept AGENTS.md\n✓ synced 2 targets\n  ~ kept CLAUDE.md\n"'
  run_script "$project/.agnostic-ai/scripts/sync-generated.sh"
  assert_successful_code "$?"
  assert_same "  ~ kept AGENTS.md
  ~ kept CLAUDE.md" "$(cat "$err")"
  assert_empty "$(cat "$out")"
}

function test_a_failed_sync_prints_a_note_and_exits_zero() {
  fake_bin "$project/agnostic-ai" "$(head_commit)" 'echo "error: bad spec" >&2; exit 2'
  run_script "$project/.agnostic-ai/scripts/sync-generated.sh"
  assert_successful_code "$?"
  assert_same "agnostic-ai: sync failed; run agnostic-ai sync to see why
error: bad spec" "$(cat "$err")"
}

function test_a_binary_built_before_the_last_code_change_skips_sync() {
  fake_bin "$project/agnostic-ai" "$(head_commit)" 'exit 0'
  printf 'package main\n\nfunc main() {}\n' > "$project/cmd/main.go"
  commit_all "$project" code
  run_script "$project/.agnostic-ai/scripts/sync-generated.sh"
  assert_successful_code "$?"
  assert_contains "was built from older code than this checkout" "$(cat "$err")"
  assert_file_not_exists "$calls"
}

function test_a_commit_without_code_changes_keeps_the_binary_current() {
  fake_bin "$project/agnostic-ai" "$(head_commit)" 'exit 0'
  printf 'version: 1\ntargets: [claude]\n' > "$project/agnostic-ai.yaml"
  commit_all "$project" specs
  run_script "$project/.agnostic-ai/scripts/sync-generated.sh"
  assert_successful_code "$?"
  assert_empty "$(cat "$err")"
  assert_same "$project" "$(cat "$calls")"
}

function test_a_binary_without_a_known_commit_skips_sync() {
  fake_bin "$project/agnostic-ai" "" 'exit 0'
  run_script "$project/.agnostic-ai/scripts/sync-generated.sh"
  assert_successful_code "$?"
  assert_contains "cannot tell which commit built" "$(cat "$err")"
  assert_file_not_exists "$calls"
}

function test_a_release_build_without_a_commit_syncs() {
  fake_bin "$project/agnostic-ai" "" '# mod	github.com/chemaclass/agnostic-ai	v0.99.0	h1:x'
  run_script "$project/.agnostic-ai/scripts/sync-generated.sh"
  assert_successful_code "$?"
  assert_empty "$(cat "$err")"
  assert_same "$project" "$(cat "$calls")"
}

function test_a_pseudo_version_names_the_commit_when_vcs_info_is_missing() {
  local short
  short="$(head_commit | head -c 12)"
  fake_bin "$project/agnostic-ai" "" "# mod	github.com/chemaclass/agnostic-ai	v0.99.1-0.20261126101813-$short	"
  printf 'package main\n\nfunc main() {}\n' > "$project/cmd/main.go"
  commit_all "$project" code
  run_script "$project/.agnostic-ai/scripts/sync-generated.sh"
  assert_successful_code "$?"
  assert_contains "was built from older code than this checkout" "$(cat "$err")"
}

function test_agnostic_ai_bin_skips_the_commit_check() {
  fake_bin "$fixture/custom-bin" "" 'exit 0'
  run_script "$project/.agnostic-ai/scripts/sync-generated.sh" AGNOSTIC_AI_BIN="$fixture/custom-bin"
  assert_successful_code "$?"
  assert_empty "$(cat "$err")"
  assert_same "$project" "$(cat "$calls")"
}

function test_a_copy_two_folders_deep_syncs_from_the_root() {
  fake_bin "$project/agnostic-ai" "$(head_commit)" 'exit 0'
  mkdir -p "$project/.claude/hooks"
  cp "$SYNC_SCRIPT" "$project/.claude/hooks/"
  run_script "$project/.claude/hooks/sync-generated.sh"
  assert_successful_code "$?"
  assert_same "$project" "$(cat "$calls")"
}

function test_a_copy_four_folders_deep_syncs_from_the_root() {
  fake_bin "$project/agnostic-ai" "$(head_commit)" 'exit 0'
  mkdir -p "$project/a/b/c/d"
  cp "$SYNC_SCRIPT" "$project/a/b/c/d/"
  run_script "$project/a/b/c/d/sync-generated.sh"
  assert_successful_code "$?"
  assert_same "$project" "$(cat "$calls")"
}

function test_a_linked_worktree_uses_the_main_worktree_binary() {
  fake_bin "$project/agnostic-ai" "$(head_commit)" 'exit 0'
  git_quiet -C "$project" worktree add -b other "$fixture/linked"
  run_script "$fixture/linked/.agnostic-ai/scripts/sync-generated.sh"
  assert_successful_code "$?"
  assert_empty "$(cat "$err")"
  assert_same "$fixture/linked" "$(cat "$calls")"
}

# Git runs hooks from the worktree root with GIT_DIR exported.
function test_a_git_hook_in_a_linked_worktree_syncs_from_its_root() {
  fake_bin "$project/agnostic-ai" "$(head_commit)" 'exit 0'
  git_quiet -C "$project" worktree add -b other "$fixture/linked"
  local git_dir
  git_dir="$(git -C "$fixture/linked" rev-parse --absolute-git-dir)"
  out="$fixture/out"
  err="$fixture/err"
  (cd "$fixture/linked" && env -i HOME="$fixture" PATH="$sys" GIT_DIR="$git_dir" \
    /bin/sh .agnostic-ai/scripts/sync-generated.sh >"$out" 2>"$err")
  assert_successful_code "$?"
  assert_empty "$(cat "$err")"
  assert_same "$fixture/linked" "$(cat "$calls")"
}

function test_outside_a_git_checkout_it_exits_zero_quietly() {
  mkdir -p "$fixture/plain"
  cp "$SYNC_SCRIPT" "$fixture/plain/"
  run_script "$fixture/plain/sync-generated.sh" GIT_CEILING_DIRECTORIES="$fixture"
  assert_successful_code "$?"
  assert_empty "$(cat "$err")"
  assert_empty "$(cat "$out")"
}
