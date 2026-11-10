#!/usr/bin/env bash

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HOOK_SCRIPT="$REPO_ROOT/.agnostic-ai/scripts/gofmt-on-edit.sh"
# make test-shell builds the binary first.
AGNOSTIC_AI_TEST_BIN="$REPO_ROOT/agnostic-ai"

BAD_GO='package demo
type point struct{ x, y int }
var xs = []point{point{1, 2}}
func  f( )  int {
return 1 }
'

GOOD_GO='package demo

type point struct{ x, y int }

var xs = []point{{1, 2}}

func f() int {
	return 1
}
'

# Each test gets a project with the hook script at its real place, so the
# script finds the project root from its own path. PATH holds only what
# the script needs, so a missing tool is really missing.
function set_up() {
  project="$(mktemp -d "${TMPDIR:-/tmp}/agnostic-gofmt-hook-tests.XXXXXX")"
  project="$(cd "$project" && pwd -P)"
  sys="$project/.sys"
  mkdir -p "$project/.agnostic-ai/scripts" "$project/pkg/sub" "$sys"
  cp "$HOOK_SCRIPT" "$project/.agnostic-ai/scripts/"
  ln -s "$(command -v dirname)" "$sys/dirname"
  ln -s "$(command -v gofmt)" "$sys/gofmt"
  ln -s "$(command -v git)" "$sys/git"
  others=()
}

function tear_down() {
  local other
  for other in "${others[@]}"; do
    case "$other" in
      */agnostic-gofmt-hook-other.*) rm -rf "$other" ;;
    esac
  done
  case "$project" in
    */agnostic-gofmt-hook-tests.*) rm -rf "$project" ;;
    *) return 1 ;;
  esac
}

function run_hook() {
  local target="$1" payload="$2"
  shift 2
  local bin_env=(AGNOSTIC_AI_BIN="$AGNOSTIC_AI_TEST_BIN")
  [ "${HOOK_BIN_UNSET:-}" = 1 ] && bin_env=()
  printf '%s' "$payload" | (cd "${HOOK_CWD:-$project}" && env -i HOME="$project" PATH="$sys" \
    AGNOSTIC_AI_TARGET="$target" "${bin_env[@]}" "$@" \
    /bin/sh "$project/.agnostic-ai/scripts/gofmt-on-edit.sh")
}

function other_dir() {
  local dir
  dir="$(mktemp -d "${TMPDIR:-/tmp}/agnostic-gofmt-hook-other.XXXXXX")"
  dir="$(cd "$dir" && pwd -P)"
  others+=("$dir")
  printf '%s\n' "$dir"
}

function git_quiet() {
  git -c user.name=test -c user.email=test@example.com -c commit.gpgsign=false "$@" >/dev/null 2>&1
}

function write_bad() {
  printf '%s' "$BAD_GO" > "$project/$1"
}

function test_claude_edit_formats_the_go_file() {
  write_bad pkg/a.go
  run_hook claude "{\"cwd\":\"$project\",\"tool_name\":\"Edit\",\"tool_input\":{\"file_path\":\"$project/pkg/a.go\"}}"
  assert_successful_code "$?"
  assert_same "$GOOD_GO" "$(cat "$project/pkg/a.go")
"
}

function test_codex_patch_formats_only_its_go_files() {
  write_bad pkg/a.go
  write_bad pkg/b.go
  printf '#  title\n\n\tindented   text\n' > "$project/notes.md"
  local before_md
  before_md="$(cat "$project/notes.md")"
  local patch='*** Begin Patch\n*** Update File: pkg/a.go\n@@\n-x\n+y\n*** Add File: pkg/b.go\n+y\n*** Update File: notes.md\n@@\n-x\n+y\n*** End Patch\n'
  run_hook codex "{\"cwd\":\"$project\",\"tool_name\":\"apply_patch\",\"tool_input\":{\"command\":\"$patch\"}}"
  assert_successful_code "$?"
  assert_same "$GOOD_GO" "$(cat "$project/pkg/a.go")
"
  assert_same "$GOOD_GO" "$(cat "$project/pkg/b.go")
"
  assert_same "$before_md" "$(cat "$project/notes.md")"
}

function test_gemini_write_file_formats_the_go_file() {
  write_bad pkg/a.go
  run_hook gemini "{\"cwd\":\"$project\",\"tool_name\":\"write_file\",\"tool_input\":{\"file_path\":\"$project/pkg/a.go\"}}"
  assert_successful_code "$?"
  assert_same "$GOOD_GO" "$(cat "$project/pkg/a.go")
"
}

function test_a_hook_started_below_the_root_formats_the_file() {
  write_bad pkg/sub/c.go
  local patch='*** Begin Patch\n*** Update File: c.go\n@@\n-x\n+y\n*** End Patch\n'
  HOOK_CWD="$project/pkg/sub" run_hook codex "{\"cwd\":\"$project/pkg/sub\",\"tool_name\":\"apply_patch\",\"tool_input\":{\"command\":\"$patch\"}}"
  assert_successful_code "$?"
  assert_same "$GOOD_GO" "$(cat "$project/pkg/sub/c.go")
"
}

function test_a_path_with_a_space_is_formatted() {
  mkdir -p "$project/my pkg"
  write_bad "my pkg/a.go"
  run_hook claude "{\"cwd\":\"$project\",\"tool_name\":\"Write\",\"tool_input\":{\"file_path\":\"$project/my pkg/a.go\"}}"
  assert_successful_code "$?"
  assert_same "$GOOD_GO" "$(cat "$project/my pkg/a.go")
"
}

function test_a_non_go_edit_changes_nothing() {
  printf 'func  f( )  {\n' > "$project/notes.txt"
  run_hook claude "{\"cwd\":\"$project\",\"tool_name\":\"Edit\",\"tool_input\":{\"file_path\":\"$project/notes.txt\"}}"
  assert_successful_code "$?"
  assert_same "func  f( )  {" "$(cat "$project/notes.txt")"
}

function test_a_go_file_with_a_syntax_error_stays_as_it_is() {
  printf 'package demo\nfunc  f( {\n' > "$project/pkg/broken.go"
  run_hook claude "{\"cwd\":\"$project\",\"tool_name\":\"Edit\",\"tool_input\":{\"file_path\":\"$project/pkg/broken.go\"}}"
  assert_successful_code "$?"
  assert_same "package demo
func  f( {" "$(cat "$project/pkg/broken.go")"
}

function test_a_shell_tool_call_does_nothing() {
  write_bad pkg/a.go
  local out
  out="$(run_hook claude '{"tool_name":"Bash","tool_input":{"command":"gofmt -l ."}}' 2>&1)"
  assert_successful_code "$?"
  assert_empty "$out"
  assert_same "$BAD_GO" "$(cat "$project/pkg/a.go")
"
}

function test_a_hook_started_below_the_root_formats_a_file_above_its_cwd() {
  write_bad pkg/a.go
  HOOK_CWD="$project/pkg/sub" run_hook claude "{\"cwd\":\"$project/pkg/sub\",\"tool_name\":\"Edit\",\"tool_input\":{\"file_path\":\"$project/pkg/a.go\"}}"
  assert_successful_code "$?"
  assert_same "$GOOD_GO" "$(cat "$project/pkg/a.go")
"
}

function test_a_file_named_like_a_flag_is_formatted() {
  write_bad -x.go
  run_hook claude "{\"cwd\":\"$project\",\"tool_name\":\"Write\",\"tool_input\":{\"file_path\":\"$project/-x.go\"}}"
  assert_successful_code "$?"
  assert_same "$GOOD_GO" "$(cat -- "$project/-x.go")
"
}

function test_a_go_file_in_another_worktree_of_the_repo_is_formatted() {
  git_quiet -C "$project" init
  git_quiet -C "$project" commit --allow-empty -m init
  local worktree
  worktree="$(other_dir)/wt"
  git_quiet -C "$project" worktree add -b other "$worktree"
  printf '%s' "$BAD_GO" > "$worktree/a.go"
  run_hook claude "{\"cwd\":\"$project\",\"tool_name\":\"Edit\",\"tool_input\":{\"file_path\":\"$worktree/a.go\"}}"
  assert_successful_code "$?"
  assert_same "$GOOD_GO" "$(cat "$worktree/a.go")
"
}

function test_a_go_file_in_an_unrelated_repo_is_not_touched() {
  git_quiet -C "$project" init
  local repo
  repo="$(other_dir)"
  git_quiet -C "$repo" init
  printf '%s' "$BAD_GO" > "$repo/a.go"
  run_hook claude "{\"cwd\":\"$project\",\"tool_name\":\"Edit\",\"tool_input\":{\"file_path\":\"$repo/a.go\"}}"
  assert_successful_code "$?"
  assert_same "$BAD_GO" "$(cat "$repo/a.go")
"
}

function test_a_go_file_in_a_plain_folder_outside_the_repo_is_not_touched() {
  git_quiet -C "$project" init
  local folder
  folder="$(other_dir)"
  printf '%s' "$BAD_GO" > "$folder/a.go"
  run_hook claude "{\"cwd\":\"$project\",\"tool_name\":\"Edit\",\"tool_input\":{\"file_path\":\"$folder/a.go\"}}"
  assert_successful_code "$?"
  assert_same "$BAD_GO" "$(cat "$folder/a.go")
"
}

function test_without_agnostic_ai_bin_it_uses_the_binary_at_the_root() {
  write_bad pkg/a.go
  printf '#!/bin/sh\nprintf "pkg/a.go\\n"\n' > "$project/agnostic-ai"
  chmod +x "$project/agnostic-ai"
  HOOK_BIN_UNSET=1 run_hook claude '{}'
  assert_successful_code "$?"
  assert_same "$GOOD_GO" "$(cat "$project/pkg/a.go")
"
}

function test_a_go_file_outside_the_project_is_not_touched() {
  local outside
  outside="$(mktemp -d "${TMPDIR:-/tmp}/agnostic-gofmt-hook-outside.XXXXXX")"
  printf '%s' "$BAD_GO" > "$outside/x.go"
  run_hook claude "{\"cwd\":\"$project\",\"tool_name\":\"Edit\",\"tool_input\":{\"file_path\":\"$outside/x.go\"}}"
  assert_successful_code "$?"
  assert_same "$BAD_GO" "$(cat "$outside/x.go")
"
  rm -rf "$outside"
}

function test_a_missing_agnostic_ai_binary_exits_zero() {
  write_bad pkg/a.go
  local out
  out="$(run_hook claude "{\"cwd\":\"$project\",\"tool_name\":\"Edit\",\"tool_input\":{\"file_path\":\"$project/pkg/a.go\"}}" AGNOSTIC_AI_BIN=/nonexistent/agnostic-ai 2>&1)"
  assert_successful_code "$?"
  assert_empty "$out"
  assert_same "$BAD_GO" "$(cat "$project/pkg/a.go")
"
}

function test_a_missing_gofmt_exits_zero() {
  rm "$sys/gofmt"
  write_bad pkg/a.go
  local out
  out="$(run_hook claude "{\"cwd\":\"$project\",\"tool_name\":\"Edit\",\"tool_input\":{\"file_path\":\"$project/pkg/a.go\"}}" 2>&1)"
  assert_successful_code "$?"
  assert_empty "$out"
  assert_same "$BAD_GO" "$(cat "$project/pkg/a.go")
"
}

function test_invalid_json_exits_zero() {
  run_hook claude 'not json'
  assert_successful_code "$?"
}
