#!/usr/bin/env bash
#
# End-to-end tests for the built agnostic-ai binary.
#
# Run:
#   make build && bashunit scripts/e2e_test.sh
#
# Unlike the Go suite, these drive the real binary against a throwaway
# project on disk: scaffold, sync, re-sync, import, revert. That is the
# only layer where whole-pipeline properties are observable, such as a
# second sync being byte-identical or a malformed spec reaching every
# emitted file. A unit test cannot see either.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$REPO_ROOT/agnostic-ai"
PROJECT=""

function set_up_before_script() {
  if [[ ! -x "$BIN" ]]; then
    printf 'scripts/e2e_test.sh needs the built binary. Run: make build\n' >&2
    exit 1
  fi
}

# Each test gets its own scaffolded project so ordering never matters.
function set_up() {
  PROJECT="$(mktemp -d)"
  cd "$PROJECT" || return 1
  git init -q .
  "$BIN" init --demo >/dev/null 2>&1
}

function tear_down() {
  cd "$REPO_ROOT" || return 1
  [[ -n "$PROJECT" ]] && rm -rf "$PROJECT"
}

# tree_digest hashes emitted output so two runs can be compared.
#
# .agnostic-ai/.sync-state is the one exclusion: it records which warnings have
# already been shown, so it is expected to differ between runs. Everything else
# including .gitignore is covered, which is what guards #580.
function tree_digest() {
  find . -path ./.git -prune -o -type f -print \
    | grep -vE '/\.sync-state$' \
    | sort | xargs shasum 2>/dev/null | shasum | cut -d' ' -f1
}

# ---- scaffold ----------------------------------------------------------------

function test_init_demo_produces_specs_that_validate_and_lint_clean() {
  "$BIN" validate >/dev/null 2>&1
  assert_successful_code
  "$BIN" lint >/dev/null 2>&1
  assert_successful_code
}

# ---- sync --------------------------------------------------------------------

function test_sync_emits_output_for_every_registered_target() {
  local missing="" t
  for t in $("$REPO_ROOT/scripts/target-facts.sh" --list); do
    "$BIN" sync --only "$t" >/dev/null 2>&1 || missing="$missing $t"
  done
  assert_same "" "$missing"
}

function test_sync_check_is_clean_immediately_after_a_sync() {
  "$BIN" sync >/dev/null 2>&1
  "$BIN" sync --check >/dev/null 2>&1
  assert_successful_code
}

function test_second_sync_is_byte_identical() {
  local first second
  "$BIN" sync >/dev/null 2>&1
  first="$(tree_digest)"
  "$BIN" sync >/dev/null 2>&1
  second="$(tree_digest)"
  assert_same "$first" "$second"
}

function test_dry_run_leaves_the_tree_untouched() {
  local before after
  "$BIN" sync >/dev/null 2>&1
  before="$(tree_digest)"
  "$BIN" sync --dry-run >/dev/null 2>&1
  after="$(tree_digest)"
  assert_same "$before" "$after"
}

function test_first_sync_leaves_the_source_entry_point_committable() {
  # sync writes AGNOSTIC_AI.md on the first run only, which used to get it
  # recorded as generated output and ignored. It is a source file: the shared
  # instruction body every target renders from (#580).
  "$BIN" sync >/dev/null 2>&1
  git add -A >/dev/null 2>&1
  assert_contains "AGNOSTIC_AI.md" "$(git status --porcelain)"
}

# ---- import round-trip -------------------------------------------------------

function test_import_then_sync_leaves_no_drift() {
  # Only the sources import supports; the newest targets are emit-only.
  local drifted="" t
  "$BIN" sync >/dev/null 2>&1
  for t in claude codex cursor cline continue windsurf qoder; do
    "$BIN" import "$t" >/dev/null 2>&1 || continue
    "$BIN" sync >/dev/null 2>&1
    "$BIN" sync --check >/dev/null 2>&1 || drifted="$drifted $t"
  done
  assert_same "" "$drifted"
}

function test_import_rejects_an_emit_only_target() {
  assert_contains "AAI-202" "$("$BIN" import kilo 2>&1)"
}

# ---- malformed specs ---------------------------------------------------------

function test_unterminated_frontmatter_is_rejected_by_lint() {
  # The closing `---` is missing, so the block would otherwise survive as
  # body text and be emitted verbatim into every target.
  printf -- '---\nname: broken\ndescription: Reviews code.\n' > .agnostic-ai/agents/broken.md
  assert_contains "LINT006" "$("$BIN" lint 2>&1)"
  "$BIN" lint >/dev/null 2>&1
  assert_general_error
}

function test_invalid_yaml_frontmatter_fails_validate() {
  printf -- '---\nname: [unclosed\ndescription: x\n---\nbody\n' > .agnostic-ai/agents/bad.md
  "$BIN" validate >/dev/null 2>&1
  assert_general_error
}

function test_two_specs_sharing_a_name_are_reported() {
    # The loader collapses them by name, so one body silently never reaches a
    # target. Lint only sees the clash because the loader records the loser
    # as shadowed (#582).
  printf -- '---\nname: dupe\ndescription: First.\n---\nBody one.\n' > .agnostic-ai/rules/x.md
  printf -- '---\nname: dupe\ndescription: Second.\n---\nBody two.\n' > .agnostic-ai/rules/y.md
  assert_contains "LINT003" "$("$BIN" lint 2>&1)"
}

function test_several_hooks_on_one_trigger_are_allowed() {
  # All of them run; this is a supported pattern and must not be flagged.
  printf 'name: fmt-on-save\nevent: PostToolUse\nmatcher: Edit\ncommand: echo fmt\n' > .agnostic-ai/hooks/f.yaml
  printf 'name: vet-on-save\nevent: PostToolUse\nmatcher: Edit\ncommand: echo vet\n' > .agnostic-ai/hooks/v.yaml
  "$BIN" lint >/dev/null 2>&1
  assert_successful_code
}

function test_unknown_target_is_rejected() {
  "$BIN" sync --only not-a-real-target >/dev/null 2>&1
  assert_general_error
}

# ---- backup and revert -------------------------------------------------------

function test_backup_then_revert_then_cleanup_round_trips() {
  "$BIN" sync >/dev/null 2>&1
  printf 'hand edit\n' >> CLAUDE.md
  "$BIN" sync --backup >/dev/null 2>&1
  assert_not_same "0" "$(find . -name '*.bak' | wc -l | tr -d ' ')"

  "$BIN" revert >/dev/null 2>&1
  "$BIN" cleanup --backups >/dev/null 2>&1
  assert_same "0" "$(find . -name '*.bak' | wc -l | tr -d ' ')"
}
