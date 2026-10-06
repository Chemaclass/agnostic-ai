#!/usr/bin/env bash
#
# bashunit tests for scripts/target-facts.sh
#
# Run:
#   bashunit scripts/target-facts_test.sh
#
# Pure helpers are tested by sourcing the script (its entry-point guard
# keeps main() from running). The invariant that matters most is that
# every target list derives from the adapter registry: a new adapter must
# show up in --list and in exactly one --batches group without anyone
# editing this script or the target-audit skill.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/target-facts.sh"
FACTS_ROOT="$ROOT"

# ---- list_targets ------------------------------------------------------------

function test_list_targets_matches_the_registry() {
  local from_script from_go
  from_script=$(list_targets | sort | tr '\n' ' ')
  from_go=$(grep -E '^\s+"[a-z]+":\s+[a-z]+\.New\(\),$' "$REGISTRY" |
    sed -e 's/^[[:space:]]*"//' -e 's/".*$//' | sort | tr '\n' ' ')
  assert_equals "$from_go" "$from_script"
}

function test_list_targets_has_no_duplicates() {
  local total unique
  total=$(list_targets | wc -l | tr -d ' ')
  unique=$(list_targets | sort -u | wc -l | tr -d ' ')
  assert_equals "$total" "$unique"
}

function test_list_targets_leads_with_the_highest_churn_vendors() {
  # Registry order is roughly chronological, which is what makes batch 1
  # the fast-moving vendors. Guard that property.
  assert_equals "claude" "$(list_targets | head -1)"
}

# ---- batches -----------------------------------------------------------------

function test_batches_returns_the_requested_group_count() {
  assert_equals 5 "$(batches 5 | wc -l | tr -d ' ')"
}

function test_batches_covers_every_target_exactly_once() {
  local flattened expected
  flattened=$(batches 5 | sed 's/^[0-9]*: //' | tr ' ' '\n' | sort | tr '\n' ' ')
  expected=$(list_targets | sort | tr '\n' ' ')
  assert_equals "$expected" "$flattened"
}

function test_batches_spreads_the_remainder_evenly() {
  # 25 targets over 4 groups is 7/6/6/6, never 6/6/6/7 or a 4-wide gap.
  local sizes
  sizes=$(batches 4 | sed 's/^[0-9]*: //' | awk '{ print NF }' | tr '\n' ' ')
  assert_equals "7 6 6 6 " "$sizes"
}

function test_batches_clamps_a_count_above_the_target_total() {
  local total
  total=$(list_targets | wc -l | tr -d ' ')
  assert_equals "$total" "$(batches 999 | wc -l | tr -d ' ')"
}

function test_batches_clamps_a_zero_count_to_one_group() {
  assert_equals 1 "$(batches 0 | wc -l | tr -d ' ')"
}

# ---- pkg_for / src_for -------------------------------------------------------

function test_pkg_for_resolves_a_matching_package_name() {
  assert_equals "claude" "$(pkg_for claude)"
}

function test_pkg_for_resolves_a_package_that_differs_from_the_target() {
  assert_equals "continueai" "$(pkg_for continue)"
}

function test_pkg_for_is_empty_for_an_unknown_target() {
  assert_empty "$(pkg_for definitely-not-a-target)"
}

function test_src_for_finds_a_non_test_source_file() {
  assert_contains "internal/adapters/claude/claude.go" "$(src_for claude)"
}

# ---- fact extraction ---------------------------------------------------------

function test_caps_extracts_the_declared_spec_kinds() {
  assert_contains "Rule" "$(caps "$(src_for claude)")"
}

function test_defaults_extracts_the_const_block_verbatim() {
  # gofmt aligns the `=` in a const block; the dump keeps that alignment
  # so a reader can match it against the source at a glance.
  local out
  out=$(defaults "$(src_for claude)")
  assert_contains '.claude/rules' "$out"
  assert_contains '.mcp.json' "$out"
}

function test_doc_comment_stops_at_the_package_clause() {
  local out
  out=$(doc_comment "$(src_for claude)")
  assert_contains "Package claude" "$out"
  assert_not_contains "package claude" "$out"
}

function test_doc_section_extracts_only_the_requested_target() {
  local out
  out=$(doc_section zed)
  assert_contains "context_servers" "$out"
  assert_not_contains "### Warp" "$out"
}

# ---- dump_target -------------------------------------------------------------

function test_dump_target_rejects_an_unknown_target() {
  dump_target definitely-not-a-target 2>/dev/null
  assert_equals 1 $?
}

function test_dump_target_emits_every_section_for_a_real_target() {
  local out
  out=$(dump_target kilo)
  assert_contains "TARGET: kilo" "$out"
  assert_contains "declared capabilities" "$out"
  assert_contains "default output paths" "$out"
  assert_contains "docs/site/content/docs/target-behavior.md lines" "$out"
  assert_contains "**kilo**" "$out"
  assert_contains "docs/site/content/docs/targets/kilo.md" "$out"
  assert_contains "# Kilo" "$out"
}

# ---- coverage of the audit source list ---------------------------------------

function test_source_sections_contains_only_requested_targets() {
  local out
  out=$(source_sections zed junie)
  assert_contains "## zed" "$out"
  assert_contains "## junie" "$out"
  assert_contains "docs:" "$out"
  assert_not_contains "## warp" "$out"
  assert_not_contains "## claude" "$out"
}

function test_source_sections_rejects_unknown_target_before_printing() {
  local out status=0
  out=$(source_sections zed definitely-not-a-target 2>/dev/null) || status=$?
  assert_equals 1 "$status"
  assert_empty "$out"
}

function test_source_sections_requires_a_target() {
  local status=0
  source_sections 2>/dev/null || status=$?
  assert_equals 2 "$status"
}

function test_source_sections_does_not_repeat_duplicate_targets() {
  local count
  count=$(source_sections zed zed | grep -c '^## zed$')
  assert_equals 1 "$count"
}

function test_every_registered_target_has_an_upstream_sources_entry() {
  # The Go test in tests/integration owns this invariant; this mirror
  # keeps `make test-shell` honest when the sources file is edited by
  # hand between Go runs.
  local sources missing t
  sources="$ROOT/.agnostic-ai/skills/target-audit/references/sources.md"
  missing=""
  for t in $(list_targets); do
    grep -q "^## $t\$" "$sources" || missing="$missing $t"
  done
  assert_empty "$missing"
}

# ---- batch_list --------------------------------------------------------------

function test_batch_list_spreads_the_remainder_like_batches() {
  local sizes
  sizes=$(batch_list 3 a b c d e f g | sed 's/^[0-9]*: //' | awk '{ print NF }' | tr '\n' ' ')
  assert_equals "3 2 2 " "$sizes"
}

function test_batch_list_clamps_a_count_above_the_input() {
  assert_equals 2 "$(batch_list 9 a b | wc -l | tr -d ' ')"
}

function test_batch_list_prints_nothing_without_targets() {
  assert_empty "$(batch_list 3)"
}

# ---- changed_classes ---------------------------------------------------------

# write_run <row>... builds a docfetch.tsv from "target kind url status" tuples.
function write_run() {
  local row
  : >"$CHANGED_RUN"
  for row in "$@"; do
    set -- $row
    printf '%s\t%s\t%s\t200\thtml\tsha-%s\t2026-09-01\t%s\t%s\t\n' \
      "$1" "$2" "$3" "$3" "$4" "$3" >>"$CHANGED_RUN"
  done
}

function set_up() {
  ROOT="$FACTS_ROOT"
  REGISTRY="$ROOT/internal/adapters/adapter.go"
  TARGETS_DIR="$ROOT/docs/site/content/docs/targets"
  BEHAVIOR_PAGE="$ROOT/docs/site/content/docs/target-behavior.md"
  CHANGED_DIR=$(mktemp -d)
  CHANGED_RUN="$CHANGED_DIR/docfetch.tsv"
  export TARGET_AUDIT_LOCK="$CHANGED_DIR/sources.lock"
  LOCK="$TARGET_AUDIT_LOCK"
  BUILTINS_DIR="$ROOT/internal/builtins/data"
}

function tear_down() {
  [ -n "${CHANGED_DIR:-}" ] && rm -rf "$CHANGED_DIR"
}

function test_changed_classes_marks_a_moved_page_deep() {
  write_run "claude docs https://a/1 changed" "zed docs https://z/1 unchanged" "zed changelog https://z/c unchanged"
  local out
  out=$(changed_classes "$CHANGED_RUN")
  assert_contains "deep: claude" "$out"
  assert_contains "sweep: zed" "$out"
}

function test_changed_classes_marks_a_moved_changelog_deep() {
  write_run "zed docs https://z/1 unchanged" "zed changelog https://z/c changed"
  assert_contains "deep: zed" "$(changed_classes "$CHANGED_RUN")"
}

function test_changed_classes_marks_an_unrecovered_page_deep() {
  write_run "kiro docs https://k/1 failed" "kiro changelog https://k/c unchanged"
  assert_contains "deep: kiro" "$(changed_classes "$CHANGED_RUN")"
}

function test_changed_classes_sweeps_a_fully_unchanged_target() {
  write_run "zed docs https://z/1 unchanged" "zed changelog https://z/c unchanged"
  local out
  out=$(changed_classes "$CHANGED_RUN")
  assert_equals "sweep: zed" "$out"
}

function test_changed_classes_ignores_a_target_absent_from_the_run() {
  write_run "zed docs https://z/1 unchanged" "zed changelog https://z/c unchanged"
  assert_not_contains "claude" "$(changed_classes "$CHANGED_RUN")"
}

function test_changed_classes_derives_status_from_the_lock_when_absent() {
  # A seven-column file predates the status column; the lock decides.
  printf 'zed\tdocs\thttps://z/1\t200\thtml\tsha-1\t2026-09-01\n' >"$CHANGED_RUN"
  assert_contains "deep: zed" "$(changed_classes "$CHANGED_RUN")"
  printf 'zed\tdocs\thttps://z/1\t200\thtml\tsha-1\t2026-09-01\n' >"$LOCK"
  assert_contains "sweep: zed" "$(changed_classes "$CHANGED_RUN")"
}

function test_changed_classes_rejects_an_unreadable_file() {
  local status=0
  changed_classes "$CHANGED_DIR/missing.tsv" 2>/dev/null || status=$?
  assert_equals 1 "$status"
}

# ---- --changed ---------------------------------------------------------------

function test_changed_prints_numbered_deep_batches_then_the_sweep() {
  builtin_regression_fixture
  write_run "claude docs https://a/1 changed" "cursor docs https://c/1 changed" \
    "zed docs https://z/1 unchanged" "zed changelog https://z/c unchanged"
  local out
  out=$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")
  assert_contains "1: claude" "$out"
  assert_contains "2: cursor" "$out"
  assert_contains "sweep: zed" "$out"
}

function test_changed_omits_the_sweep_line_when_every_target_moved() {
  write_run "claude docs https://a/1 changed"
  assert_not_contains "sweep:" "$(main --changed "$CHANGED_RUN")"
}

function test_changed_clamps_the_batch_count_to_the_deep_targets() {
  builtin_regression_fixture
  write_run "claude docs https://a/1 changed" "zed docs https://z/1 unchanged" "zed changelog https://z/c unchanged"
  assert_equals 1 "$(main --changed "$CHANGED_RUN" 5 --builtins-since "$BUILTIN_BASE" | grep -c '^[0-9]*:')"
}

function test_changed_requires_a_file() {
  local status=0
  main --changed 2>/dev/null || status=$?
  assert_equals 2 "$status"
}

function test_changed_rejects_a_missing_file() {
  local status=0
  main --changed "$CHANGED_DIR/missing.tsv" 2>/dev/null || status=$?
  assert_equals 1 "$status"
}

# ---- built-ins ---------------------------------------------------------------

function test_changed_moves_a_builtin_target_out_of_the_sweep() {
  write_run "claude docs https://a/1 unchanged" "claude changelog https://a/c unchanged" \
    "zed docs https://z/1 unchanged" "zed changelog https://z/c unchanged"
  local out
  out=$(changed_batches "$CHANGED_RUN" 5 claude)
  assert_contains "1: claude" "$out"
  assert_contains "sweep: zed" "$out"
}

function test_changed_rejects_builtins_since_without_a_revision() {
  write_run "zed docs https://z/1 unchanged"
  local status=0
  main --changed "$CHANGED_RUN" 5 --builtins-since 2>/dev/null || status=$?
  assert_equals 2 "$status"
}

function test_changed_with_builtins_since_head_keeps_the_sweep() {
  builtin_regression_fixture
  write_run "zed docs https://z/1 unchanged" "zed changelog https://z/c unchanged"
  assert_equals "sweep: zed" "$(main --changed "$CHANGED_RUN" 5 --builtins-since HEAD)"
}

function builtin_regression_fixture() {
  ROOT="$CHANGED_DIR/repo"
  REGISTRY="$ROOT/internal/adapters/adapter.go"
  BUILTINS_DIR="$ROOT/internal/builtins/data"
  mkdir -p "$BUILTINS_DIR/demo-hook/hooks" "$BUILTINS_DIR/demo/skills/demo/references" \
    "$ROOT/internal/adapters/continueai" "$ROOT/internal/adapters/claude" \
    "$ROOT/internal/adapters/factory" "$ROOT/internal/adapters/zed" \
    "$ROOT/internal/cli" "$ROOT/internal/spec" "$ROOT/internal/config" \
    "$ROOT/internal/adapters/internal/emit" "$ROOT/docs/site/content/docs" \
    "$ROOT/scripts/target-audit"
  cat >"$REGISTRY" <<'EOF'
var registry = map[string]Adapter{
  "claude": claude.New(),
  "factory": factory.New(),
  "continue": continueai.New(),
  "zed": zed.New(),
}
EOF
  local pkg
  for pkg in claude factory continueai zed; do
    printf 'package %s\nvar caps = emit.Capabilities{\n Supports: []spec.Kind{spec.KindHook, spec.KindSkill},\n}\n' "$pkg" \
      >"$ROOT/internal/adapters/$pkg/$pkg.go"
  done
  printf 'package builtins\nvar names = []string{"demo-hook", "demo"}\n' >"$ROOT/internal/builtins/builtins.go"
  printf 'name: end\ntargets: [factory, claude]\nevent: PreCompact\nx-gemini:\n  event: PreCompress\ncommand: |\n  echo BODY_HOOK_SENTINEL\n' >"$BUILTINS_DIR/demo-hook/hooks/end.yaml"
  printf -- '---\nname: demo\ntargets: ["claude"]\ntargets-exclude: [factory]\n---\n\nBODY_SKILL_SENTINEL\ntargets: [zed]\n' >"$BUILTINS_DIR/demo/skills/demo/SKILL.md"
  printf 'asset\n' >"$BUILTINS_DIR/demo/skills/demo/references/notes.txt"
  printf 'package cli\n' >"$ROOT/internal/cli/hook_run.go"
  printf 'package spec\n' >"$ROOT/internal/spec/spec.go"
  printf 'package config\n' >"$ROOT/internal/config/config.go"
  printf 'package emit\n' >"$ROOT/internal/adapters/internal/emit/output.go"
  printf 'handoff docs\n' >"$ROOT/docs/site/content/docs/handoff.md"
  printf 'unrelated docs\n' >"$ROOT/docs/site/content/docs/releases.md"
  printf 'vendor lock\n' >"$ROOT/scripts/target-audit/sources.lock"
  local path
  for path in internal/adapters/vars.go internal/adapters/header/header.go internal/markdown/markdown.go internal/mdlink/mdlink.go internal/hookrun/run.go internal/hookpaths/path.go internal/cli/global_layers.go internal/cli/skills_share.go internal/cli/import.go internal/cli/layers.go internal/cli/global_config.go internal/cli/global_targets.go internal/cli/sync_global.go internal/cli/sync_run.go go.mod go.sum tests/integration/builtins_test.go tests/integration/builtin_handoff_hooks_test.go tests/integration/fixtures/golden/builtin-handoff/example.md docs/site/content/docs/configuration.md docs/site/content/docs/spec-format/hooks.md docs/site/content/docs/targets/continue.md; do
    mkdir -p "$(dirname "$ROOT/$path")"
    printf 'baseline\n' >"$ROOT/$path"
  done
  git -C "$ROOT" init -q
  git -C "$ROOT" add .
  git -C "$ROOT" -c user.name=Test -c user.email=test@example.invalid -c commit.gpgsign=false commit -qm baseline
  BUILTIN_BASE=$(git -C "$ROOT" rev-parse HEAD)
  write_run "factory docs https://f/1 unchanged" "claude docs https://c/1 unchanged" "continue docs https://n/1 unchanged"
}

function assert_builtin_regression_shared_deep() {
  local out="$1"
  assert_contains 'builtin-deep: factory claude continue' "$out"
  assert_contains '1: factory claude' "$out"
  assert_contains '2: continue' "$out"
  assert_not_contains 'sweep:' "$out"
  assert_not_contains 'zed' "$out"
}

function test_builtin_regression_removed_selector_forces_all_requested_targets() {
  builtin_regression_fixture
  printf 'name: end\ntargets: [claude]\nevent: PreCompact\ncommand: echo changed\n' >"$BUILTINS_DIR/demo-hook/hooks/end.yaml"
  assert_builtin_regression_shared_deep "$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")"
}

function test_builtin_regression_untracked_asset_is_not_false_clean() {
  builtin_regression_fixture
  printf 'new asset\n' >"$BUILTINS_DIR/demo/skills/demo/references/new asset's"$'\n'"notes.txt"
  assert_builtin_regression_shared_deep "$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")"
}

function test_builtin_regression_staged_deleted_and_renamed_paths_invalidate() {
  builtin_regression_fixture
  git -C "$ROOT" rm -q internal/builtins/data/demo-hook/hooks/end.yaml
  git -C "$ROOT" mv internal/builtins/data/demo/skills/demo/references/notes.txt renamed-notes.txt
  assert_builtin_regression_shared_deep "$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")"
}

function test_builtin_regression_committed_change_is_compared_to_baseline() {
  builtin_regression_fixture
  printf 'changed\n' >>"$BUILTINS_DIR/demo/skills/demo/references/notes.txt"
  git -C "$ROOT" add .
  git -C "$ROOT" -c user.name=Test -c user.email=test@example.invalid -c commit.gpgsign=false commit -qm change
  assert_builtin_regression_shared_deep "$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")"
}

function test_builtin_regression_shared_runtime_selection_config_and_emit_invalidate() {
  builtin_regression_fixture
  local path out
  for path in internal/cli/hook_run.go internal/spec/spec.go internal/config/config.go internal/adapters/internal/emit/output.go internal/adapters/vars.go internal/adapters/header/header.go internal/markdown/markdown.go internal/mdlink/mdlink.go internal/hookrun/run.go internal/hookpaths/path.go internal/cli/global_layers.go internal/cli/skills_share.go internal/cli/import.go internal/cli/layers.go internal/cli/global_config.go internal/cli/global_targets.go internal/cli/sync_global.go internal/cli/sync_run.go go.mod go.sum tests/integration/builtins_test.go tests/integration/builtin_handoff_hooks_test.go tests/integration/fixtures/golden/builtin-handoff/example.md docs/site/content/docs/handoff.md docs/site/content/docs/configuration.md docs/site/content/docs/spec-format/hooks.md; do
    printf 'changed\n' >>"$ROOT/$path"
    out=$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")
    assert_builtin_regression_shared_deep "$out"
    git -C "$ROOT" checkout -- "$path"
  done
}

function test_builtin_regression_adapter_mapping_limits_deep_to_requested_target() {
  builtin_regression_fixture
  printf 'changed\n' >>"$ROOT/internal/adapters/continueai/continueai.go"
  local out
  out=$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")
  assert_contains 'builtin-deep: continue' "$out"
  assert_contains '1: continue' "$out"
  assert_contains 'sweep: factory claude' "$out"
}

function test_builtin_regression_target_docs_limit_deep_to_requested_target() {
  builtin_regression_fixture
  printf 'changed\n' >>"$ROOT/docs/site/content/docs/targets/continue.md"
  local out
  out=$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")
  assert_contains 'builtin-deep: continue' "$out"
  assert_contains '1: continue' "$out"
  assert_contains 'sweep: factory claude' "$out"
}

function test_builtin_regression_unknown_adapter_mapping_is_conservative() {
  builtin_regression_fixture
  mkdir -p "$ROOT/internal/adapters/unmapped"
  printf 'package unmapped\n' >"$ROOT/internal/adapters/unmapped/hooks.go"
  assert_builtin_regression_shared_deep "$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")"
}

function test_builtin_regression_removed_registration_is_conservative() {
  builtin_regression_fixture
  printf 'package builtins\nvar names = []string{"demo"}\n' >"$ROOT/internal/builtins/builtins.go"
  assert_builtin_regression_shared_deep "$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")"
}

function test_builtin_regression_tracked_path_with_newline_is_not_split() {
  builtin_regression_fixture
  local path
  path="internal/builtins/data/demo/skills/demo/references/tracked"$'\n'"asset.txt"
  printf 'original\n' >"$ROOT/$path"
  git -C "$ROOT" add .
  git -C "$ROOT" -c user.name=Test -c user.email=test@example.invalid -c commit.gpgsign=false commit -qm asset
  BUILTIN_BASE=$(git -C "$ROOT" rev-parse HEAD)
  printf 'changed\n' >>"$ROOT/$path"
  assert_builtin_regression_shared_deep "$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")"
}

function test_builtin_regression_ignored_untracked_asset_is_not_a_change() {
  builtin_regression_fixture
  printf 'ignored.txt\n' >"$ROOT/.git/info/exclude"
  printf 'ignored\n' >"$BUILTINS_DIR/demo/skills/demo/references/ignored.txt"
  assert_equals 'sweep: factory claude continue' "$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")"
}

function test_builtin_regression_unrelated_docs_lock_and_absent_adapter_do_not_invalidate() {
  builtin_regression_fixture
  printf 'changed\n' >>"$ROOT/docs/site/content/docs/releases.md"
  printf 'changed\n' >>"$ROOT/scripts/target-audit/sources.lock"
  printf 'changed\n' >>"$ROOT/internal/adapters/zed/zed.go"
  printf 'original lock\n' >"$LOCK"
  assert_equals 'sweep: factory claude continue' "$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")"
  assert_equals 'original lock' "$(cat "$LOCK")"
}

function test_builtin_regression_missing_baseline_is_conservative() {
  builtin_regression_fixture
  assert_builtin_regression_shared_deep "$(main --changed "$CHANGED_RUN" 2 2>"$CHANGED_DIR/reason")"
  assert_equals 1 "$(wc -l <"$CHANGED_DIR/reason" | tr -d ' ')"
}

function test_builtin_regression_invalid_baseline_is_conservative() {
  builtin_regression_fixture
  local out status=0
  out=$(main --changed "$CHANGED_RUN" 2 --builtins-since unavailable-audit-revision 2>"$CHANGED_DIR/reason") || status=$?
  assert_equals 0 "$status"
  assert_builtin_regression_shared_deep "$out"
  assert_equals 1 "$(wc -l <"$CHANGED_DIR/reason" | tr -d ' ')"
}

function test_builtin_regression_failed_history_inspection_is_conservative() {
  builtin_regression_fixture
  local out operation status
  for operation in diff ls-files; do
    status=0
    git() {
      case " $* " in *" $operation "*) return 1 ;; esac
      command git "$@"
    }
    out=$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE" 2>"$CHANGED_DIR/reason") || status=$?
    unset -f git
    assert_equals 0 "$status"
    assert_builtin_regression_shared_deep "$out"
    assert_equals 1 "$(wc -l <"$CHANGED_DIR/reason" | tr -d ' ')"
  done
}

function test_builtin_regression_inventory_is_raw_registered_evidence_without_bodies() {
  builtin_regression_fixture
  mkdir -p "$BUILTINS_DIR/stray/skills/stray"
  printf -- '---\nname: stray\n---\nSTRAY_SENTINEL\n' >"$BUILTINS_DIR/stray/skills/stray/SKILL.md"
  local out status=0
  out=$(main --builtins 2>/dev/null) || status=$?
  assert_equals 0 "$status"
  assert_contains 'internal/builtins/data/demo/skills/demo/SKILL.md' "$out"
  assert_contains 'targets: ["claude"]' "$out"
  assert_contains ':3:' "$out"
  assert_contains 'targets-exclude: [factory]' "$out"
  assert_contains 'x-gemini:' "$out"
  assert_contains 'event: PreCompress' "$out"
  assert_contains 'internal/builtins/builtins.go' "$out"
  assert_contains 'docs/site/content/docs/handoff.md' "$out"
  assert_contains 'tests/integration/builtin_handoff_hooks_test.go' "$out"
  assert_not_contains 'BODY_HOOK_SENTINEL' "$out"
  assert_not_contains 'BODY_SKILL_SENTINEL' "$out"
  assert_not_contains 'targets: [zed]' "$out"
  assert_not_contains 'STRAY_SENTINEL' "$out"
  assert_not_contains 'stray/SKILL.md' "$out"
  assert_not_contains 'this target emits' "$out"
}

function test_builtin_inventory_keeps_block_list_selectors_and_portable_events() {
  builtin_regression_fixture
  printf 'name: end\ntargets:\n  - claude\n  - codex\ntarget-exclude:\n  - gemini\non: pre-compact\nmatch: x\ncommand: |\n  echo BODY_HOOK_SENTINEL\n' >"$BUILTINS_DIR/demo-hook/hooks/end.yaml"
  local out
  out=$(main --builtins)
  assert_contains 'end.yaml:3:  - claude' "$out"
  assert_contains 'end.yaml:4:  - codex' "$out"
  assert_contains 'end.yaml:6:  - gemini' "$out"
  assert_contains 'end.yaml:7:on: pre-compact' "$out"
  assert_contains 'end.yaml:8:match: x' "$out"
  assert_not_contains 'BODY_HOOK_SENTINEL' "$out"
}

function test_builtin_regression_cli_source_invalidates_and_cli_tests_do_not() {
  builtin_regression_fixture
  printf 'package cli\n// resolves builtins layers\n' >"$ROOT/internal/cli/root.go"
  printf 'package cli\n' >"$ROOT/internal/cli/version_test.go"
  printf 'package cli\n' >"$ROOT/internal/cli/settings_json_edit.go"
  git -C "$ROOT" add .
  git -C "$ROOT" -c user.name=Test -c user.email=test@example.invalid -c commit.gpgsign=false commit -qm cli
  BUILTIN_BASE=$(git -C "$ROOT" rev-parse HEAD)
  printf 'changed\n' >>"$ROOT/internal/cli/version_test.go"
  assert_equals 'sweep: factory claude continue' "$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")"
  printf 'changed\n' >>"$ROOT/internal/cli/root.go"
  assert_builtin_regression_shared_deep "$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")"
  git -C "$ROOT" checkout -- internal/cli/root.go
  printf 'changed\n' >>"$ROOT/internal/cli/settings_json_edit.go"
  assert_builtin_regression_shared_deep "$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")"
}

function test_builtin_regression_cli_file_that_stops_naming_builtins_invalidates() {
  builtin_regression_fixture
  printf 'package cli\n// enables builtins in new projects\n' >"$ROOT/internal/cli/init_scaffold.go"
  git -C "$ROOT" add .
  git -C "$ROOT" -c user.name=Test -c user.email=test@example.invalid -c commit.gpgsign=false commit -qm cli
  BUILTIN_BASE=$(git -C "$ROOT" rev-parse HEAD)
  printf 'package cli\n' >"$ROOT/internal/cli/init_scaffold.go"
  assert_builtin_regression_shared_deep "$(main --changed "$CHANGED_RUN" 2 --builtins-since "$BUILTIN_BASE")"
}

function test_builtin_regression_dirty_built_in_against_head_is_deep() {
  # A report may name built-in coverage only when this check is clean, so a
  # run that audited an uncommitted hook edit never becomes a baseline.
  builtin_regression_fixture
  printf 'name: end\ntargets: [claude]\nevent: Stop\n' >"$BUILTINS_DIR/demo-hook/hooks/end.yaml"
  assert_builtin_regression_shared_deep "$(main --changed "$CHANGED_RUN" 2 --builtins-since HEAD)"
}
