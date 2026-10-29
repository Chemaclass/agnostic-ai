#!/usr/bin/env bash
#
# bashunit tests for scripts/docfetch.sh
#
# Run:
#   bashunit scripts/docfetch_test.sh
#
# No test reaches the network. Everything below the URL resolver replaces
# docfetch_curl with a fixture-serving stub, so the recovery ladder is
# exercised without a vendor round trip. The ladder's behaviour against real
# hosts is measured by the bootstrap run, not here.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/docfetch.sh"

FIXTURES=""

function set_up() {
  FIXTURES=$(mktemp -d)
  export TARGET_AUDIT_LOCK="$FIXTURES/sources.lock"
  LOCK="$TARGET_AUDIT_LOCK"
  export DOCFETCH_SNAPSHOTS="$FIXTURES/snapshots"
  export DOCFETCH_RUNS="$FIXTURES/runs"
}

function tear_down() {
  [ -n "$FIXTURES" ] && rm -rf "$FIXTURES"
}

# write_lock <url> <sha> seeds a one-row lock for status comparisons.
function write_lock() {
  {
    echo "# lock"
    printf 'claude\tdocs\t%s\t200\thtml\t%s\t2026-09-01\n' "$1" "$2"
  } >"$LOCK"
}

# ---- resolve_urls ------------------------------------------------------------

function test_resolve_urls_expands_origin_relative_shorthand() {
  local out
  out=$(resolve_urls claude)
  assert_contains "https://code.claude.com/docs/en/memory" "$out"
  assert_contains "https://code.claude.com/docs/en/settings-reference" "$out"
}

function test_resolve_urls_expands_directory_relative_shorthand() {
  # goose lists sibling pages as ".../using-skills.md" off the previous URL.
  assert_contains \
    "https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/using-skills.md" \
    "$(resolve_urls goose)"
}

function test_resolve_urls_reads_both_sides_of_an_and_joiner() {
  local out
  out=$(resolve_urls qoder)
  assert_contains "https://docs.qoder.com/release-notes/qoder-cli.md" "$out"
  assert_contains "https://docs.qoder.com/release-notes/desktop.md" "$out"
}

function test_resolve_urls_ignores_parenthesised_commentary() {
  # The claude docs line names /settings inside a parenthesis discussing it.
  assert_equals 1 "$(resolve_urls claude | grep -c 'docs/en/settings$')"
}

function test_resolve_urls_ignores_backticked_urls() {
  assert_not_contains "windsurf.com/changelog" "$(resolve_urls windsurf)"
}

function test_resolve_urls_tags_each_line_with_its_kind() {
  local out
  out=$(resolve_urls aider)
  assert_contains "changelog	https://aider.chat/HISTORY.html" "$out"
  assert_contains "docs	https://aider.chat/docs/usage/conventions.html" "$out"
}

function test_resolve_urls_keeps_distinct_kinds_for_the_same_url() {
  # shellcheck disable=SC2329
  function source_sections() {
    printf '%s\n' '- docs: https://vendor.example/shared' '- changelog: https://vendor.example/shared'
  }
  assert_equals "$(printf 'docs\thttps://vendor.example/shared\nchangelog\thttps://vendor.example/shared')" "$(resolve_urls vendor)"
}

function test_update_requires_validated_coverage_before_changing_the_lock() {
  # shellcheck disable=SC2329
  function source_sections() { printf '%s\n' '- docs: https://vendor.example/new'; }
  write_lock https://vendor.example/old old
  run_row claude https://vendor.example/new new 2026-10-16 >"$FIXTURES/docfetch.tsv"
  local before err status=0
  before=$(cat "$LOCK")
  err=$(docfetch_main --update "$FIXTURES/docfetch.tsv" 2>&1 >/dev/null) || status=$?
  assert_not_equals 0 "$status"
  assert_contains 'audit.json' "$err"
  assert_not_contains 'registered URLs have no row' "$err"
  assert_equals "$before" "$(cat "$LOCK")"
}

function test_manual_unvalidated_update_is_explicit() {
  run_row claude https://code.claude.com/docs/en/memory new 2026-10-16 >"$FIXTURES/docfetch.tsv"
  local out
  out=$(docfetch_main --update-unvalidated "$FIXTURES/docfetch.tsv" 2>&1)
  assert_contains 'coverage not checked' "$out"
  assert_contains 'new' "$(cat "$LOCK")"
}

function test_every_registered_target_resolves_docs_and_a_changelog() {
  local t missing="" kinds
  for t in $(list_targets); do
    kinds=$(resolve_urls "$t" | cut -f1 | sort -u | tr '\n' ' ')
    case "$kinds" in
      *changelog*docs*) ;;
      *) missing="$missing $t" ;;
    esac
  done
  assert_empty "$missing"
}

function test_every_registered_url_has_a_committed_lock_row() {
  # A registry URL without a lock row is diffed against nothing on the next
  # run. The 2026-10-29 audit merged eight Augment /cli/v0/ URLs that way.
  local t missing
  missing=$(for t in $(list_targets); do resolve_urls "$t" | cut -f2; done | sort -u |
    awk -F '\t' -v lock="$SCRIPT_DIR/target-audit/sources.lock" '
      BEGIN { while ((getline line < lock) > 0) { split(line, f, "\t"); if (substr(line, 1, 1) != "#") have[f[3]] = 1 } }
      !($0 in have)')
  assert_empty "$missing"
}

function test_every_resolved_url_is_absolute_https() {
  local bad
  bad=$(for t in $(list_targets); do resolve_urls "$t"; done | cut -f2 | grep -cv '^https\?://')
  assert_equals 0 "$bad"
}

function test_urls_mode_prefixes_the_target() {
  assert_contains "zed	docs	https://zed.dev/docs/ai/skills" "$(docfetch_main --urls zed)"
}

function test_urls_mode_requires_a_target() {
  local status=0
  docfetch_main --urls 2>/dev/null || status=$?
  assert_equals 2 "$status"
}

# ---- github_rewrite ----------------------------------------------------------

function test_github_rewrite_maps_a_blob_url_to_raw() {
  assert_equals "https://raw.githubusercontent.com/anthropics/claude-code/main/CHANGELOG.md	raw-github" \
    "$(github_rewrite https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md)"
}

function test_github_rewrite_maps_releases_to_the_api() {
  assert_equals "https://api.github.com/repos/cline/cline/releases?per_page=30	github-api" \
    "$(github_rewrite https://github.com/cline/cline/releases)"
}

function test_github_rewrite_maps_a_bare_repo_to_its_readme() {
  assert_equals "https://raw.githubusercontent.com/block/goose/HEAD/README.md	raw-github" \
    "$(github_rewrite https://github.com/block/goose)"
}

function test_github_rewrite_leaves_other_hosts_alone() {
  assert_equals "https://zed.dev/docs/ai/skills	" "$(github_rewrite https://zed.dev/docs/ai/skills)"
}

# ---- strip_html --------------------------------------------------------------

function test_strip_html_ignores_scripts_and_attributes() {
  local a b
  printf '<html><head><script nonce="a1">var x=1</script></head><body><h1>Rules</h1></body></html>' >"$FIXTURES/a.html"
  printf '<html><head><script nonce="zz9">var x=2</script></head><body><h1>Rules</h1></body></html>' >"$FIXTURES/b.html"
  a=$(strip_html "$FIXTURES/a.html")
  b=$(strip_html "$FIXTURES/b.html")
  assert_equals "$a" "$b"
  assert_contains "Rules" "$a"
}

function test_strip_html_collapses_whitespace() {
  printf '<p>one</p>\n\n   <p>two</p>\n' >"$FIXTURES/c.html"
  assert_equals "one two" "$(strip_html "$FIXTURES/c.html")"
}

function test_strip_html_ignores_site_chrome() {
  local a b
  printf '<head><title>Rules - A</title></head><nav>Home Rules</nav><main>Body</main><div class="last-modified">21 September 2026</div><footer>Updated Sep 21</footer>' >"$FIXTURES/a.html"
  printf '<nav>Rules Home Extra</nav><main>Body</main><title>Rules - A</title><div class="last-modified">23 September 2026</div><footer>Updated Sep 23</footer>' >"$FIXTURES/b.html"
  a=$(strip_html "$FIXTURES/a.html")
  b=$(strip_html "$FIXTURES/b.html")
  assert_equals "Body" "$a"
  assert_equals "$a" "$b"
}

function test_strip_html_keeps_a_changed_body() {
  printf '<nav>Home</nav><main>Old <b>text</b></main>' >"$FIXTURES/a.html"
  printf '<nav>Home</nav><main>New <b>text</b></main>' >"$FIXTURES/b.html"
  assert_not_equals "$(strip_html "$FIXTURES/a.html")" "$(strip_html "$FIXTURES/b.html")"
}

# ---- meta_refresh_target -----------------------------------------------------

function test_strip_html_keeps_every_line_of_a_multi_line_text_node() {
  # macOS awk 20200816 also splits on newlines when split() gets a
  # one-character string separator, which dropped all but the first line
  # of every code block there while Linux kept them.
  printf '<div class="code-block">\n.junie/skills/\n  my-skill/\n</div>' >"$FIXTURES/p.html"
  assert_equals ".junie/skills/ my-skill/" "$(strip_html "$FIXTURES/p.html")"
}

function test_strip_html_ignores_a_quoted_greater_than_inside_an_attribute() {
  local out
  out=$(printf '<div class="[&>*:first-child]:rounded-r-none" data-x=\x27a>b\x27>Rules text</div>' >"$FIXTURES/p.html" && strip_html "$FIXTURES/p.html")
  assert_equals "Rules text" "$out"
}

function test_meta_refresh_target_extracts_an_absolute_url() {
  printf '<html><head><meta http-equiv="refresh" content="0; url=https://kiro.dev/docs/reference/configuration/"></head></html>' \
    >"$FIXTURES/stub.html"
  assert_equals "https://kiro.dev/docs/reference/configuration/" \
    "$(meta_refresh_target "$FIXTURES/stub.html" "https://kiro.dev/docs/config/")"
}

function test_meta_refresh_target_resolves_a_root_relative_url() {
  printf '<meta http-equiv="refresh" content="0;url=/docs/new/">' >"$FIXTURES/stub.html"
  assert_equals "https://kiro.dev/docs/new/" \
    "$(meta_refresh_target "$FIXTURES/stub.html" "https://kiro.dev/docs/old/")"
}

function test_meta_refresh_target_fails_on_a_normal_page() {
  meta_refresh_target /dev/null "https://example.com/" >/dev/null 2>&1
  assert_equals 1 $?
}

# ---- router_payload ----------------------------------------------------------

function test_router_payload_extracts_a_server_rendered_blob() {
  {
    printf '<html><body><div id="root"></div><script>window._ROUTER_DATA = {"busStructure":{"updated_at":"2026-09-01"},"ops":['
    printf '{"insert":"mcpServers"},'
    for i in 1 2 3 4 5 6 7 8; do printf '{"insert":"token%d padding padding padding"},' "$i"; done
    printf '{"insert":"end"}]};</script></body></html>'
  } >"$FIXTURES/spa.html"
  assert_equals "ok" "$(router_payload "$FIXTURES/spa.html" "$FIXTURES/spa.json")"
  assert_contains "mcpServers" "$(cat "$FIXTURES/spa.json")"
  assert_not_contains "</script>" "$(cat "$FIXTURES/spa.json")"
}

function test_router_payload_declines_a_page_without_a_payload() {
  printf '<html><body>nothing here</body></html>' >"$FIXTURES/plain.html"
  assert_empty "$(router_payload "$FIXTURES/plain.html" "$FIXTURES/plain.json")"
}

# ---- sha256_of ---------------------------------------------------------------

function test_sha256_of_matches_the_known_digest() {
  printf 'abc' >"$FIXTURES/abc"
  assert_equals "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" \
    "$(sha256_of "$FIXTURES/abc")"
}

function test_sha256_of_falls_back_when_sha256sum_is_absent() {
  printf 'abc' >"$FIXTURES/abc"
  local out
  out=$(
    function command() { [ "$2" = "sha256sum" ] && return 1; builtin command "$@"; }
    sha256_of "$FIXTURES/abc"
  )
  assert_equals "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" "$out"
}

# ---- delta_text ---------------------------------------------------------------

function test_delta_text_ignores_the_sidebar_and_keeps_page_text() {
  printf '{"doc":{"ops":[{"insert":"Rules "},{"insert":"live here"}]},"busStructure":[{"updated_at":"2026-09-21"}]}' >"$FIXTURES/one.json"
  printf '{"busStructure":[{"updated_at":"2026-09-23"}],"doc":{"ops":[{"insert":"Rules "},{"insert":"live here"}]}}' >"$FIXTURES/two.json"
  assert_equals "Rules live here" "$(delta_text "$FIXTURES/one.json")"
  assert_equals "$(delta_text "$FIXTURES/one.json")" "$(delta_text "$FIXTURES/two.json")"
}

function test_delta_text_is_empty_for_a_payload_without_ops() {
  printf '{"props":{"page":"x"}}' >"$FIXTURES/one.json"
  assert_empty "$(delta_text "$FIXTURES/one.json")"
}

# ---- schema_keys ---------------------------------------------------------------

SCHEMA_ONE='{"$defs":{"Hook":{"type":"object","required":["command"],"properties":{"command":{"type":"string","description":"Run this."}}}},"type":"object","properties":{"hooks":{"type":"object","additionalProperties":{"type":"array","items":{"$ref":"#/$defs/Hook"}}},"mode":{"enum":["a","b"],"description":"Pick one."}}}'

function test_schema_keys_lists_paths_types_enums_and_refs() {
  printf '%s' "$SCHEMA_ONE" >"$FIXTURES/s.json"
  schema_keys "$FIXTURES/s.json" "$FIXTURES/s.txt"
  local out
  out=$(cat "$FIXTURES/s.txt")
  assert_contains "hooks.* type=array" "$out"
  assert_contains "hooks.*[] ref=#Hook" "$out"
  assert_contains "#Hook required=command" "$out"
  assert_contains "#Hook.command type=string" "$out"
  assert_contains 'mode enum="b"' "$out"
}

function test_schema_keys_ignores_a_reworded_description() {
  printf '%s' "$SCHEMA_ONE" >"$FIXTURES/one.json"
  printf '%s' "${SCHEMA_ONE/Run this./Run this command.}" >"$FIXTURES/two.json"
  schema_keys "$FIXTURES/one.json" "$FIXTURES/one.txt"
  schema_keys "$FIXTURES/two.json" "$FIXTURES/two.txt"
  assert_equals "$(cat "$FIXTURES/one.txt")" "$(cat "$FIXTURES/two.txt")"
}

function test_schema_keys_declines_a_body_that_is_not_json() {
  printf '<html>moved</html>' >"$FIXTURES/s.json"
  assert_general_error "$(schema_keys "$FIXTURES/s.json" "$FIXTURES/s.txt")"
  assert_file_not_exists "$FIXTURES/s.txt"
}

function test_fetch_one_hashes_a_schema_row_by_its_keys() {
  stub_curl "https://schemas.example/*|200|$SCHEMA_ONE"
  local row
  row=$(fetch_one claude schema https://schemas.example/settings.json "$FIXTURES/run" 1)
  assert_equals "schema-keys" "$(printf '%s' "$row" | cut -f5)"
  assert_equals "$(sha256_of "$FIXTURES/run/pages/claude/schema-1-schemas.example-settings.json.txt")" \
    "$(printf '%s' "$row" | cut -f6)"
}

function test_resolve_urls_tags_a_code_line() {
  assert_contains "code	https://github.com/openai/codex/blob/main/codex-rs/hooks/src/events/mod.rs" "$(resolve_urls codex)"
}

function test_fetch_one_reads_a_code_row_as_raw_text() {
  stub_curl "https://raw.githubusercontent.com/*|200|pub mod session_start;"
  local row
  row=$(fetch_one codex code https://github.com/openai/codex/blob/main/codex-rs/hooks/src/events/mod.rs "$FIXTURES/run" 1)
  assert_equals "raw-github" "$(printf '%s' "$row" | cut -f5)"
  assert_equals "new" "$(printf '%s' "$row" | cut -f8)"
}

function test_resolve_urls_tags_a_schema_line() {
  assert_contains "schema	https://www.schemastore.org/claude-code-settings.json" "$(resolve_urls claude)"
}

# ---- releases ------------------------------------------------------------------

RELEASES='[{"tag_name":"v2","published_at":"2026-09-29T10:00:00Z","prerelease":false,"draft":false,"body":"Adds hooks.Stop."},{"tag_name":"nightly","published_at":"2026-09-30T01:00:00Z","prerelease":true,"draft":false,"body":"build"},{"tag_name":"v1","published_at":"2026-09-20T10:00:00Z","prerelease":false,"draft":false,"body":"Old."},{"tag_name":"v0","published_at":"2026-09-10T10:00:00Z","prerelease":false,"draft":false,"body":"Older."}]'

function test_release_index_skips_prereleases() {
  printf '%s' "$RELEASES" >"$FIXTURES/r.json"
  local out
  out=$(release_index "$FIXTURES/r.json")
  assert_contains '"tag_name":"v2"' "$out"
  assert_contains '"published_at":"2026-09-29T10:00:00Z"' "$out"
  assert_not_contains "nightly" "$out"
}

function test_release_entries_prints_only_releases_newer_than_the_snapshot() {
  printf '%s' "$RELEASES" >"$FIXTURES/r.json"
  printf '"tag_name":"v1"\n"published_at":"2026-09-20T10:00:00Z"\n' >"$FIXTURES/snap.txt"
  local out
  out=$(release_entries "$FIXTURES/r.json" "$FIXTURES/snap.txt")
  assert_contains "## v2 (2026-09-29)" "$out"
  assert_contains "Adds hooks.Stop." "$out"
  assert_not_contains "nightly" "$out"
  # v0 is not in the snapshot but older than it: a longer page, not news.
  assert_not_contains "## v0" "$out"
  assert_not_contains "## v1" "$out"
}

function test_release_entries_counts_every_release_without_a_snapshot() {
  printf '%s' "$RELEASES" >"$FIXTURES/r.json"
  assert_equals 3 "$(release_entries "$FIXTURES/r.json" "$FIXTURES/none.txt" | grep -c '^## ')"
}

# ---- json_sum -----------------------------------------------------------------

function test_json_sum_ignores_object_key_order() {
  printf '{"a":1,"b":{"x":[1,2],"y":"z"}}' >"$FIXTURES/one.json"
  printf '{ "b": { "y": "z", "x": [1, 2] }, "a": 1 }\n' >"$FIXTURES/two.json"
  assert_equals "$(json_sum "$FIXTURES/one.json")" "$(json_sum "$FIXTURES/two.json")"
}

function test_json_sum_still_sees_a_changed_value_or_array_order() {
  printf '{"a":[1,2]}' >"$FIXTURES/one.json"
  printf '{"a":[2,1]}' >"$FIXTURES/two.json"
  assert_not_equals "$(json_sum "$FIXTURES/one.json")" "$(json_sum "$FIXTURES/two.json")"
}

function test_json_sum_hashes_the_raw_body_when_it_is_not_json() {
  printf 'abc' >"$FIXTURES/abc"
  assert_equals "$(sha256_of "$FIXTURES/abc")" "$(json_sum "$FIXTURES/abc")"
}

# ---- row_status --------------------------------------------------------------

function test_row_status_is_new_without_a_lock() {
  assert_equals "new" "$(row_status https://example.com/a html deadbeef)"
}

function test_row_status_is_unchanged_on_a_hash_match() {
  write_lock "https://example.com/a" "deadbeef"
  assert_equals "unchanged" "$(row_status https://example.com/a html deadbeef)"
}

function test_row_status_is_changed_on_a_hash_miss() {
  write_lock "https://example.com/a" "deadbeef"
  assert_equals "changed" "$(row_status https://example.com/a html cafe)"
}

function test_row_status_is_failed_for_an_unrecovered_page() {
  write_lock "https://example.com/a" "deadbeef"
  assert_equals "failed" "$(row_status https://example.com/a app-shell -)"
  assert_equals "failed" "$(row_status https://example.com/a soft-404 -)"
}

# ---- fetch_one ---------------------------------------------------------------

# stub_curl <spec>... installs a docfetch_curl that answers by URL pattern.
# Each spec is "<pattern>|<code>|<body>".
function stub_curl() {
  STUB_SPECS=("$@")
  function docfetch_curl() {
    local url="$1" out="$2" spec pattern code body
    for spec in "${STUB_SPECS[@]}"; do
      pattern=${spec%%|*}
      spec=${spec#*|}
      code=${spec%%|*}
      body=${spec#*|}
      case "$url" in
        $pattern)
          printf '%s' "$body" >"$out"
          printf '%s\t%s\ttext/html\n' "$code" "$url"
          return 0
          ;;
      esac
    done
    : >"$out"
    printf '404\t%s\ttext/html\n' "$url"
  }
}

function test_fetch_one_retries_a_403_through_the_reader_proxy() {
  stub_curl "https://r.jina.ai/*|200|$(printf 'Kiro configuration reference %.0s' 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20)" \
    "https://kiro.dev/*|403|"
  local row
  row=$(fetch_one kiro docs https://kiro.dev/docs/hooks/ "$FIXTURES/run" 1)
  assert_equals "reader-proxy" "$(printf '%s' "$row" | cut -f5)"
  assert_equals "new" "$(printf '%s' "$row" | cut -f8)"
}

function test_fetch_one_goes_straight_to_the_reader_proxy_when_forced() {
  stub_curl "https://r.jina.ai/*|200|$(printf 'Markdown Content:\nKiro steering reference %.0s' 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20)" \
    "https://kiro.dev/*|200|<html><body><p>direct copy served to some networks only</p></body></html>"
  local row
  row=$(fetch_one kiro docs https://kiro.dev/docs/steering/ "$FIXTURES/run" 1 proxy)
  assert_equals "reader-proxy" "$(printf '%s' "$row" | cut -f5)"
}

function test_fetch_one_retries_a_rate_limited_proxy_fetch() {
  PROXY_CALLS="$FIXTURES/proxy.calls"
  : >"$PROXY_CALLS"
  function docfetch_curl() {
    printf 'x\n' >>"$PROXY_CALLS"
    if [ "$(grep -c . <"$PROXY_CALLS")" -lt 3 ]; then
      : >"$2"
      printf '429\t%s\ttext/plain\n' "$1"
      return 0
    fi
    printf 'Markdown Content:\nKiro reference %.0s' 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 >"$2"
    printf '200\t%s\ttext/plain\n' "$1"
  }
  local row
  row=$(DOCFETCH_RETRY_SLEEP=0 fetch_one kiro docs https://kiro.dev/docs/x/ "$FIXTURES/run" 1 proxy)
  assert_equals "200" "$(printf '%s' "$row" | cut -f4)"
  assert_equals "3" "$(grep -c . <"$PROXY_CALLS")"
}

function test_fetch_target_stops_retrying_once_its_retry_budget_is_spent() {
  function docfetch_curl() {
    : >"$2"
    printf '429\t%s\ttext/plain\n' "$1"
  }
  function sleep() { printf '%s\n' "$1" >>"$FIXTURES/slept"; }
  : >"$FIXTURES/slept"
  local rows total
  rows=$(DOCFETCH_RETRY_SLEEP=10 DOCFETCH_RETRY_BUDGET=40 fetch_target kiro "$FIXTURES/run")
  total=$(awk '{ t += $1 } END { print t + 0 }' "$FIXTURES/slept")
  assert_equals "30" "$total"
  assert_equals "failed" "$(printf '%s\n' "$rows" | cut -f8 | sort -u)"
  assert_equals "$(resolve_urls kiro | grep -c .)" "$(printf '%s\n' "$rows" | grep -c .)"
}

function test_fetch_target_forces_the_proxy_for_a_target_marked_fetch_reader_proxy() {
  stub_curl "https://r.jina.ai/*|200|$(printf 'Markdown Content:\nKiro reference %.0s' 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20)" \
    "https://kiro.dev/*|200|<html><body><p>direct copy served to some networks only</p></body></html>"
  local rows
  rows=$(fetch_target kiro "$FIXTURES/run")
  assert_equals "reader-proxy" "$(printf '%s\n' "$rows" | awk -F '\t' '$3 !~ /\.md$/ { print $5 }' | sort -u)"
  assert_equals "markdown-mirror" "$(printf '%s\n' "$rows" | awk -F '\t' '$3 ~ /\.md$/ { print $5 }' | sort -u)"
}

# Cursor's source section is long enough that `grep -q` stops reading
# before it ends; under pipefail the writer's SIGPIPE must not read as "no
# fetch line", or every Cursor page is fetched as HTML and never hashes
# like the locked proxy copy.
function test_fetch_target_forces_the_proxy_for_a_long_source_section() {
  set -o pipefail
  stub_curl "https://r.jina.ai/*|200|$(printf 'Markdown Content:\nCursor reference %.0s' 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20)" \
    "https://cursor.com/*|200|<html><body><p>direct copy served to some networks only</p></body></html>"
  local rows
  rows=$(fetch_target cursor "$FIXTURES/run")
  assert_equals "reader-proxy" "$(printf '%s\n' "$rows" | awk -F '\t' '$2 != "schema" && $3 !~ /\.md$/ { print $5 }' | sort -u)"
  # A schema is JSON; the proxy would wrap it in a Markdown header.
  assert_not_contains "r.jina.ai" "$(printf '%s\n' "$rows" | awk -F '\t' '$2 == "schema" { print $9 }')"
}

function test_fetch_one_follows_a_client_side_meta_refresh() {
  stub_curl \
    "https://kiro.dev/docs/config/|200|<html><meta http-equiv=\"refresh\" content=\"0; url=https://kiro.dev/docs/reference/configuration/\"></html>" \
    "https://kiro.dev/docs/reference/configuration/|200|<html><body>$(printf 'configuration keys %.0s' 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25 26 27 28 29 30)</body></html>"
  local row
  row=$(fetch_one kiro docs https://kiro.dev/docs/config/ "$FIXTURES/run" 1)
  assert_equals "meta-refresh" "$(printf '%s' "$row" | cut -f5)"
  assert_equals "https://kiro.dev/docs/reference/configuration/" "$(printf '%s' "$row" | cut -f9)"
}

function test_fetch_one_falls_back_to_a_markdown_mirror_for_a_thin_page() {
  stub_curl \
    "https://docs.qoder.com/cli/hooks.md|200|# Hooks$(printf ' key %.0s' 1 2 3 4 5 6 7 8 9 10)" \
    "https://docs.qoder.com/cli/hooks|200|<html><body><div id=app></div></body></html>"
  local row
  row=$(fetch_one qoder docs https://docs.qoder.com/cli/hooks "$FIXTURES/run" 1)
  assert_equals "markdown-mirror" "$(printf '%s' "$row" | cut -f5)"
}

function test_fetch_one_reports_an_unrecoverable_shell_without_a_hash() {
  stub_curl "https://example.com/*|200|<html><body><div id=app></div></body></html>"
  local row
  row=$(fetch_one amp docs https://example.com/manual "$FIXTURES/run" 1)
  assert_equals "app-shell" "$(printf '%s' "$row" | cut -f5)"
  assert_equals "-" "$(printf '%s' "$row" | cut -f6)"
  assert_equals "failed" "$(printf '%s' "$row" | cut -f8)"
}

function test_fetch_one_marks_a_dead_url_failed() {
  stub_curl "https://nothing.example/*|404|"
  local row
  row=$(fetch_one zed docs https://nothing.example/docs "$FIXTURES/run" 1)
  assert_equals "failed" "$(printf '%s' "$row" | cut -f8)"
  assert_equals "404" "$(printf '%s' "$row" | cut -f4)"
}

# reader_page <published> <gap> prints a reader-proxy body. The proxy stamps
# a fresh Published Time on some fetches and reflows blank lines on others.
function reader_page() {
  printf 'Title: Steering - Kiro\n\nURL Source: https://kiro.dev/docs/steering/\n\n'
  [ -n "$1" ] && printf 'Published Time: %s\n\n' "$1"
  printf 'Markdown Content:\nSteering gives Kiro persistent knowledge.%s| Capability | IDE |\n' "$2"
}

function test_reader_text_drops_the_proxy_header() {
  local out
  out=$(reader_page "Wed, 23 Sep 2026 09:12:16 GMT" $'\n\n' | reader_text)
  assert_not_contains "Published Time" "$out"
  assert_not_contains "URL Source" "$out"
  assert_contains "Steering gives Kiro persistent knowledge." "$out"
}

function test_reader_text_ignores_a_new_timestamp_and_blank_line_reflow() {
  assert_equals \
    "$(reader_page "" $'\n\n' | reader_text)" \
    "$(reader_page "Thu, 24 Sep 2026 08:00:00 GMT" $'\n' | reader_text)"
}

function test_reader_text_keeps_a_changed_body() {
  assert_not_equals \
    "$(reader_page "" $'\n' | reader_text)" \
    "$(printf 'Markdown Content:\nSteering gives Kiro nothing.\n' | reader_text)"
}

function test_reader_text_keeps_a_body_without_the_proxy_header() {
  assert_equals "plain text body" "$(printf 'plain   text\n\nbody\n' | reader_text)"
}

function test_reader_text_drops_an_embedded_video_placeholder() {
  assert_equals "Hooks run on save." \
    "$(printf 'Markdown Content:\nHooks run\nVideo unavailable\non save.\n' | reader_text)"
}

function test_reader_text_ignores_images_and_their_loading_placeholder() {
  assert_equals \
    "$(printf 'Markdown Content:\nSee [![Image 1: K](https://x/k.png)](https://x) hooks.\n' | reader_text)" \
    "$(printf 'Markdown Content:\nSee [Loading image...![Image 1](https://x/k.png)](https://x) hooks.\n' | reader_text)"
}

function test_fetch_one_hashes_joined_words_differently() {
  local first second
  stub_curl "https://r.jina.ai/*|200|$(printf 'Markdown Content:\nrun foo bar\n')" "https://kiro.dev/*|403|"
  first=$(fetch_one kiro docs https://kiro.dev/docs/x/ "$FIXTURES/run" 1 | cut -f6)
  stub_curl "https://r.jina.ai/*|200|$(printf 'Markdown Content:\nrun foobar\n')" "https://kiro.dev/*|403|"
  second=$(fetch_one kiro docs https://kiro.dev/docs/x/ "$FIXTURES/run" 1 | cut -f6)
  assert_not_equals "$first" "$second"
}

function test_fetch_one_ignores_a_reflowed_page_updated_stamp() {
  local first second
  stub_curl "https://r.jina.ai/*|200|$(printf 'Markdown Content:\nHooks.\nPage updated: September 2, 2026\n')" "https://kiro.dev/*|403|"
  first=$(fetch_one kiro docs https://kiro.dev/docs/x/ "$FIXTURES/run" 1 | cut -f6)
  stub_curl "https://r.jina.ai/*|200|$(printf 'Markdown Content:\nHooks.\nPage updated:September 2, 2026\n')" "https://kiro.dev/*|403|"
  second=$(fetch_one kiro docs https://kiro.dev/docs/x/ "$FIXTURES/run" 1 | cut -f6)
  assert_equals "$first" "$second"
}

function test_fetch_one_hashes_a_reader_proxy_page_without_its_timestamp() {
  local first second
  stub_curl "https://r.jina.ai/*|200|$(reader_page "" $'\n\n')" "https://kiro.dev/*|403|"
  first=$(fetch_one kiro docs https://kiro.dev/docs/steering/ "$FIXTURES/run" 1 | cut -f6)
  stub_curl "https://r.jina.ai/*|200|$(reader_page "Thu, 24 Sep 2026 08:00:00 GMT" $'\n')" "https://kiro.dev/*|403|"
  second=$(fetch_one kiro docs https://kiro.dev/docs/steering/ "$FIXTURES/run" 1 | cut -f6)
  assert_equals "$first" "$second"
}

# ---- docfetch_main: incomplete runs --------------------------------------------

# stub_targets <failing> <short> installs a fetch_target that writes one row
# per resolved URL, exits nonzero for <failing>, and drops a row for <short>.
function stub_targets() {
  STUB_FAIL="$1"
  STUB_SHORT="$2"
  function fetch_target() {
    [ "$1" = "$STUB_FAIL" ] && return 1
    resolve_urls "$1" | awk -F '\t' -v t="$1" -v short="$STUB_SHORT" '
      t == short && NR == 1 { next }
      { print t "\t" $1 "\t" $2 "\t200\thtml\tsha\t2026-09-24\tunchanged\t" $2 "\tpages/x" }
    '
  }
}

function test_main_succeeds_when_every_target_completes() {
  stub_targets "" ""
  local status=0
  docfetch_main --out "$FIXTURES/run" claude zed >/dev/null 2>&1 || status=$?
  assert_equals 0 "$status"
}

function test_main_fails_when_a_target_worker_fails() {
  stub_targets zed ""
  local status=0
  docfetch_main --out "$FIXTURES/run" claude zed >/dev/null 2>&1 || status=$?
  assert_not_equals 0 "$status"
}

function test_main_fails_when_a_target_returns_fewer_rows_than_urls() {
  stub_targets "" zed
  local status=0
  docfetch_main --out "$FIXTURES/run" claude zed >/dev/null 2>&1 || status=$?
  assert_not_equals 0 "$status"
}

function stub_run_pages() {
  function resolve_urls() { printf 'docs\thttps://%s.example/docs\n' "$1"; }
  local text
  text="$(printf 'Hooks run on events. %.0s' $(seq 1 40))"
  stub_curl "https://*.example/*.md|200|$text" \
    "https://*.example/*|200|<html><body><main>$text</main></body></html>"
}

function test_main_reused_directory_reports_only_current_targets() {
  stub_run_pages
  local out old_rows old_page
  out=$(docfetch_main --out "$FIXTURES/run" claude zed)
  assert_contains '2 targets, 2 urls: 2 new' "$out"
  assert_contains 'deltas: 2 (2 no-snapshot)' "$out"
  old_rows=$(cat "$FIXTURES/run/rows/zed.tsv")
  old_page="$FIXTURES/run/$(cut -f10 "$FIXTURES/run/rows/zed.tsv")"

  out=$(docfetch_main --out "$FIXTURES/run" claude)
  assert_equals claude "$(cut -f1 "$FIXTURES/run/docfetch.tsv")"
  assert_equals claude "$(cut -f1 "$FIXTURES/run/deltas.tsv")"
  assert_contains '1 targets, 1 urls: 1 new' "$out"
  assert_contains 'deltas: 1 (1 no-snapshot)' "$out"
  assert_equals "$old_rows" "$(cat "$FIXTURES/run/rows/zed.tsv")"
  assert_file_exists "$old_page"
}

function test_main_repeated_target_does_not_accumulate_rows_or_deltas() {
  stub_run_pages
  docfetch_main --out "$FIXTURES/run" claude >/dev/null
  local out
  out=$(docfetch_main --out "$FIXTURES/run" claude)
  assert_equals 1 "$(wc -l <"$FIXTURES/run/docfetch.tsv" | tr -d ' ')"
  assert_equals 1 "$(wc -l <"$FIXTURES/run/deltas.tsv" | tr -d ' ')"
  assert_contains '1 targets, 1 urls: 1 new' "$out"
}

function test_main_reused_directory_mirrors_only_current_targets() {
  stub_run_pages
  docfetch_main --out "$FIXTURES/run" --mirrors claude zed >/dev/null
  local old_mirrors out
  old_mirrors=$(cat "$FIXTURES/run/rows/zed.mirrors")
  out=$(docfetch_main --out "$FIXTURES/run" --mirrors claude)
  assert_equals claude "$(cut -f1 "$FIXTURES/run/mirrors.tsv")"
  assert_contains 'mirrors: 1 of 1 scraped rows' "$out"
  assert_equals "$old_mirrors" "$(cat "$FIXTURES/run/rows/zed.mirrors")"
}

function test_main_reused_directory_drops_mirrors_when_current_mode_changes() {
  stub_run_pages
  docfetch_main --out "$FIXTURES/run" --mirrors claude zed >/dev/null
  function docfetch_curl() {
    printf 'Plain page text. %.0s' $(seq 1 40) >"$2"
    printf '200\t%s\ttext/plain\n' "$1"
  }
  local out
  out=$(docfetch_main --out "$FIXTURES/run" --mirrors claude)
  assert_equals text "$(cut -f5 "$FIXTURES/run/docfetch.tsv")"
  assert_empty "$(cat "$FIXTURES/run/mirrors.tsv")"
  assert_contains 'mirrors: 0 of 0 scraped rows' "$out"
  assert_file_exists "$FIXTURES/run/rows/zed.mirrors"
}

function test_main_mirror_flag_applies_only_to_its_invocation() {
  stub_run_pages
  docfetch_main --out "$FIXTURES/mirrored" --mirrors claude >/dev/null
  docfetch_main --out "$FIXTURES/plain" claude >/dev/null
  assert_file_not_exists "$FIXTURES/plain/mirrors.tsv"
}

function test_main_nonmirror_rerun_rejects_stale_mirror_comparisons() {
  stub_run_pages
  docfetch_main --out "$FIXTURES/run" --mirrors claude zed >/dev/null
  mkdir -p "$FIXTURES/previous"
  cp "$FIXTURES/run/docfetch.tsv" "$FIXTURES/run/mirrors.tsv" "$FIXTURES/previous/"
  local old_mirrors status=0
  old_mirrors=$(cat "$FIXTURES/run/rows/zed.mirrors")
  stub_curl "https://*.example/*|200|<html><body><main>$(printf 'Changed page text. %.0s' $(seq 1 40))</main></body></html>"
  docfetch_main --out "$FIXTURES/run" claude >/dev/null

  docfetch_main --compare-mirrors "$FIXTURES/previous" "$FIXTURES/run" >/dev/null 2>&1 || status=$?
  assert_equals 1 "$status"
  assert_file_not_exists "$FIXTURES/run/mirrors.tsv"
  assert_equals "$old_mirrors" "$(cat "$FIXTURES/run/rows/zed.mirrors")"
}

function test_main_failed_rerun_keeps_the_last_complete_reports() {
  stub_run_pages
  docfetch_main --out "$FIXTURES/run" --mirrors claude zed >/dev/null
  local rows deltas mirrors status=0
  rows=$(cat "$FIXTURES/run/docfetch.tsv")
  deltas=$(cat "$FIXTURES/run/deltas.tsv")
  mirrors=$(cat "$FIXTURES/run/mirrors.tsv")
  stub_targets zed ""
  docfetch_main --out "$FIXTURES/run" claude zed >/dev/null 2>&1 || status=$?
  assert_not_equals 0 "$status"
  assert_equals "$rows" "$(cat "$FIXTURES/run/docfetch.tsv")"
  assert_equals "$deltas" "$(cat "$FIXTURES/run/deltas.tsv")"
  assert_equals "$mirrors" "$(cat "$FIXTURES/run/mirrors.tsv")"
}

function test_main_short_rerun_keeps_the_last_complete_report() {
  stub_run_pages
  docfetch_main --out "$FIXTURES/run" claude zed >/dev/null
  local rows status=0
  rows=$(cat "$FIXTURES/run/docfetch.tsv")
  stub_targets "" zed
  docfetch_main --out "$FIXTURES/run" claude zed >/dev/null 2>&1 || status=$?
  assert_not_equals 0 "$status"
  assert_equals "$rows" "$(cat "$FIXTURES/run/docfetch.tsv")"
}

function test_reader_text_keeps_the_space_between_words() {
  assert_not_equals \
    "$(printf 'Markdown Content:\nrun foo bar\n' | reader_text)" \
    "$(printf 'Markdown Content:\nrun foobar\n' | reader_text)"
}

function test_reader_text_keeps_whitespace_inside_a_code_fence() {
  assert_not_equals \
    "$(printf 'Markdown Content:\n```yaml\npermissions:\n  allow: x\n```\n' | reader_text)" \
    "$(printf 'Markdown Content:\n```yaml\npermissions:\nallow: x\n```\n' | reader_text)"
}

function test_reader_text_keeps_whitespace_inside_a_tilde_fence() {
  assert_not_equals \
    "$(printf 'Markdown Content:\n~~~yaml\na:\n  allow: x\n~~~\n' | reader_text)" \
    "$(printf 'Markdown Content:\n~~~yaml\na:\nallow: x\n~~~\n' | reader_text)"
}

# The fixture is the head of the GitHub releases API for openai/codex as the
# reader proxy served it. Thirty copies of its body took the quadratic
# extractor 18 seconds on macOS awk, so the limit below caught it.
function test_reader_text_finishes_a_long_release_list_within_the_limit() {
  local fixture="$SCRIPT_DIR/testdata/docfetch/codex-releases.reader.md" i
  {
    cat "$fixture"
    for i in $(seq 29); do sed '1,/^Markdown Content:/d' "$fixture"; done
  } >"$FIXTURES/releases.body"
  READER_TEXT_SECONDS=5 bounded_reader_text "$FIXTURES/releases.body" "$FIXTURES/releases.txt"
  assert_successful_code
  assert_equals 30 "$(grep -c '"tag_name": "rust-v0.166.0-alpha.6"' "$FIXTURES/releases.txt")"
}

# Forty thousand prose lines with no blank line took the quadratic
# extractor 6 seconds on macOS awk.
function test_reader_text_finishes_one_long_paragraph_within_the_limit() {
  awk 'BEGIN { print "Markdown Content:"; for (i = 0; i < 40000; i++) print "word " i " of one paragraph" }' \
    >"$FIXTURES/para.body"
  READER_TEXT_SECONDS=3 bounded_reader_text "$FIXTURES/para.body" "$FIXTURES/para.txt"
  assert_successful_code
  assert_equals 1 "$(wc -l <"$FIXTURES/para.txt" | tr -d ' ')"
}

function test_reader_text_drops_an_image_split_across_lines() {
  assert_equals "See hooks." \
    "$(printf 'Markdown Content:\nSee ![Image 1: K\nalt](https://x/k.png) hooks.\n' | reader_text)"
}

function test_reader_text_is_not_held_by_a_bracket_that_cannot_be_an_image() {
  awk 'BEGIN { print "Markdown Content:"; print "Press ![x] now"; for (i = 0; i < 40000; i++) print "line " i }' \
    >"$FIXTURES/held.body"
  READER_TEXT_SECONDS=3 bounded_reader_text "$FIXTURES/held.body" "$FIXTURES/held.txt"
  assert_successful_code
  assert_equals "Press ![x] now line 0" "$(cut -c1-21 "$FIXTURES/held.txt")"
}

function test_fetch_one_records_an_extraction_past_the_limit() {
  if ! command -v timeout >/dev/null && ! command -v gtimeout >/dev/null; then
    if declare -F bashunit::skip >/dev/null; then bashunit::skip; else skip; fi
    return
  fi
  stub_curl "https://r.jina.ai/*|200|$(printf 'Markdown Content:\nKiro steering reference %.0s' 1 2 3 4 5 6 7 8 9 10)"
  function reader_text() { sleep 5; }
  local row
  row=$(READER_TEXT_SECONDS=1 fetch_one kiro docs https://kiro.dev/docs/steering/ "$FIXTURES/run" 1 proxy)
  assert_equals "extract-limit	-" "$(printf '%s' "$row" | cut -f5-6)"
  assert_equals "failed" "$(printf '%s' "$row" | cut -f8)"
  function reader_text() { return 3; }
  row=$(fetch_one kiro docs https://kiro.dev/docs/steering/ "$FIXTURES/run" 1 proxy)
  assert_exit_code 3
}

function test_bounded_reader_text_refuses_a_large_body_without_timeout() {
  printf 'Markdown Content:\nplenty of text\n' >"$FIXTURES/big.body"
  READER_TEXT_TIMEOUT="" READER_TEXT_BYTES=10 bounded_reader_text "$FIXTURES/big.body" "$FIXTURES/big.txt"
  assert_exit_code 124
  READER_TEXT_TIMEOUT="" bounded_reader_text "$FIXTURES/big.body" "$FIXTURES/big.txt"
  assert_equals "plenty of text" "$(cat "$FIXTURES/big.txt")"
}

function test_reader_text_keeps_indented_code_verbatim() {
  assert_not_equals \
    "$(printf 'Markdown Content:\nConfig:\n\n    a:\n      allow: x\n' | reader_text)" \
    "$(printf 'Markdown Content:\nConfig:\n\n    a:\n    allow: x\n' | reader_text)"
}

function test_reader_text_does_not_close_a_backtick_fence_on_tildes() {
  assert_not_equals \
    "$(printf 'Markdown Content:\n```\n~~~\na:\n  b\n```\n' | reader_text)" \
    "$(printf 'Markdown Content:\n```\n~~~\na:\nb\n```\n' | reader_text)"
}

function test_reader_text_keeps_deeper_indentation_in_indented_code() {
  assert_not_equals \
    "$(printf 'Markdown Content:\nConfig:\n\n    parent:\n      allow: x\n' | reader_text)" \
    "$(printf 'Markdown Content:\nConfig:\n\n    parent:\n        allow: x\n' | reader_text)"
}

function test_reader_text_keeps_a_long_fence_open_past_a_shorter_one() {
  assert_not_equals \
    "$(printf 'Markdown Content:\n````md\n```yaml\n  allow: x\n```\n````\n' | reader_text)" \
    "$(printf 'Markdown Content:\n````md\n```yaml\nallow: x\n```\n````\n' | reader_text)"
}

function test_reader_text_keeps_a_long_tilde_fence_open_past_a_shorter_one() {
  assert_not_equals \
    "$(printf 'Markdown Content:\n~~~~\n~~~\n  allow: x\n~~~\n~~~~\n' | reader_text)" \
    "$(printf 'Markdown Content:\n~~~~\n~~~\nallow: x\n~~~\n~~~~\n' | reader_text)"
}

function test_reader_text_still_collapses_prose_around_a_code_fence() {
  assert_equals \
    "$(printf 'Markdown Content:\nSet it:\n\n\n```\na  b\n```\nDone.\n' | reader_text)" \
    "$(printf 'Markdown Content:\nSet   it:\n```\na  b\n```\n\nDone.\n' | reader_text)"
}

function test_reader_text_drops_the_page_updated_stamp() {
  assert_equals \
    "$(printf 'Markdown Content:\nHooks.\nPage updated:September 2, 2026\n' | reader_text)" \
    "$(printf 'Markdown Content:\nHooks.\nPage updated: September 3, 2026\n' | reader_text)"
}

# ---- lock_merge --------------------------------------------------------------

# run_row <target> <url> <sha> <changed> builds a ten-column run row.
function run_row() {
  printf '%s\tdocs\t%s\t200\thtml\t%s\t%s\tchanged\t%s\tpages/x\n' "$1" "$2" "$3" "$4" "$2"
}

function test_lock_merge_writes_a_header_and_the_rows() {
  run_row claude https://a.example/1 aaa 2026-09-22 >"$FIXTURES/run.tsv"
  lock_merge "$FIXTURES/run.tsv"
  assert_contains "# target-audit sources lock" "$(cat "$LOCK")"
  assert_contains "https://a.example/1	200	html	aaa	2026-09-22" "$(cat "$LOCK")"
}

function test_lock_merge_replaces_a_row_by_url() {
  write_lock "https://a.example/1" "old"
  run_row claude https://a.example/1 new 2026-09-22 >"$FIXTURES/run.tsv"
  lock_merge "$FIXTURES/run.tsv"
  assert_equals 1 "$(grep -c 'https://a.example/1' "$LOCK")"
  assert_contains "new" "$(grep 'https://a.example/1' "$LOCK")"
}

function test_lock_merge_preserves_rows_for_other_targets() {
  write_lock "https://a.example/1" "old"
  run_row zed https://z.example/1 zzz 2026-09-22 >"$FIXTURES/run.tsv"
  lock_merge "$FIXTURES/run.tsv"
  assert_contains "https://a.example/1" "$(cat "$LOCK")"
  assert_contains "https://z.example/1" "$(cat "$LOCK")"
}

function test_lock_merge_honours_a_target_filter() {
  {
    run_row claude https://a.example/1 aaa 2026-09-22
    run_row zed https://z.example/1 zzz 2026-09-22
  } >"$FIXTURES/run.tsv"
  lock_merge "$FIXTURES/run.tsv" claude
  assert_contains "https://a.example/1" "$(cat "$LOCK")"
  assert_not_contains "https://z.example/1" "$(cat "$LOCK")"
}

function test_lock_merge_keeps_the_locked_hash_when_a_row_failed() {
  write_lock "https://a.example/1" "old"
  printf 'claude\tdocs\thttps://a.example/1\t401\treader-proxy\t-\t2026-09-26\tfailed\thttps://r.jina.ai/https://a.example/1\t\n' >"$FIXTURES/run.tsv"
  lock_merge "$FIXTURES/run.tsv"
  assert_contains "https://a.example/1	200	html	old" "$(cat "$LOCK")"
}

function test_lock_merge_sorts_by_target() {
  {
    run_row zed https://z.example/1 zzz 2026-09-22
    run_row claude https://a.example/1 aaa 2026-09-22
  } >"$FIXTURES/run.tsv"
  lock_merge "$FIXTURES/run.tsv"
  assert_equals "claude" "$(grep -v '^#' "$LOCK" | head -1 | cut -f1)"
}

function test_update_requires_a_file() {
  local status=0
  docfetch_main --update 2>/dev/null || status=$?
  assert_equals 2 "$status"
}

function test_update_rejects_an_unreadable_file() {
  local status=0
  docfetch_main --update "$FIXTURES/missing.tsv" 2>/dev/null || status=$?
  assert_equals 1 "$status"
}

# ---- llms_entry --------------------------------------------------------------

function test_llms_entry_finds_the_page_behind_a_slug() {
  cat >"$FIXTURES/llms.txt" <<'EOF'
## Docs
- [Rules](https://ampcode.com/docs/markdown/customize/rules): how rules load
- [Skills](https://ampcode.com/docs/markdown/customize/skills.md): skills
EOF
  assert_equals "https://ampcode.com/docs/markdown/customize/skills.md" \
    "$(llms_entry "$FIXTURES/llms.txt" skills)"
}

function test_llms_entry_is_empty_when_the_index_omits_the_page() {
  printf -- '- [Rules](https://ampcode.com/docs/markdown/rules)\n' >"$FIXTURES/llms.txt"
  assert_empty "$(llms_entry "$FIXTURES/llms.txt" hooks)"
}

function test_fetch_one_hashes_the_indexed_page_not_the_index() {
  local index page_a page_b
  index='- [Rules](https://x.example/raw/rules)
- [Hooks](https://x.example/raw/hooks)'
  page_a="$(printf 'rules body %.0s' 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25 26 27 28 29 30)"
  page_b="$(printf 'hooks body %.0s' 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25 26 27 28 29 30)"
  stub_curl \
    "https://x.example/llms.txt|200|$index" \
    "https://x.example/raw/rules|200|$page_a" \
    "https://x.example/raw/hooks|200|$page_b" \
    "https://x.example/*|200|<html><body><div id=app></div></body></html>"
  local rules hooks
  rules=$(fetch_one trae docs https://x.example/rules "$FIXTURES/run" 1)
  hooks=$(fetch_one trae docs https://x.example/hooks "$FIXTURES/run" 2)
  assert_equals "llms-txt" "$(printf '%s' "$rules" | cut -f5)"
  assert_not_equals "$(printf '%s' "$rules" | cut -f6)" "$(printf '%s' "$hooks" | cut -f6)"
}

function test_fetch_one_ignores_release_asset_download_counters() {
  local a b
  a='[{"tag_name":"v1.0.0","published_at":"2026-09-01T00:00:00Z","assets":[{"download_count":11}]}]'
  b='[{"tag_name":"v1.0.0","published_at":"2026-09-01T00:00:00Z","assets":[{"download_count":9312}]}]'
  stub_curl "https://api.github.com/*|200|$a"
  local first
  first=$(fetch_one crush changelog https://github.com/charmbracelet/crush/releases "$FIXTURES/run" 1)
  stub_curl "https://api.github.com/*|200|$b"
  local second
  second=$(fetch_one crush changelog https://github.com/charmbracelet/crush/releases "$FIXTURES/run" 2)
  assert_equals "github-api" "$(printf '%s' "$first" | cut -f5)"
  assert_equals "$(printf '%s' "$first" | cut -f6)" "$(printf '%s' "$second" | cut -f6)"
}

function test_fetch_one_notices_a_new_release_tag() {
  local a b
  a='[{"tag_name":"v1.0.0","published_at":"2026-09-01T00:00:00Z"}]'
  b='[{"tag_name":"v1.1.0","published_at":"2026-09-20T00:00:00Z"},{"tag_name":"v1.0.0","published_at":"2026-09-01T00:00:00Z"}]'
  stub_curl "https://api.github.com/*|200|$a"
  local first
  first=$(fetch_one crush changelog https://github.com/charmbracelet/crush/releases "$FIXTURES/run" 1)
  stub_curl "https://api.github.com/*|200|$b"
  local second
  second=$(fetch_one crush changelog https://github.com/charmbracelet/crush/releases "$FIXTURES/run" 2)
  assert_not_equals "$(printf '%s' "$first" | cut -f6)" "$(printf '%s' "$second" | cut -f6)"
}

# ---- word_delta --------------------------------------------------------------

function test_word_delta_marks_removed_and_added_words_with_context() {
  printf 'Rules load from .cursor/rules and apply to every file.\n' >"$FIXTURES/old.txt"
  printf 'Rules load from .cursor/rules and apply to matching files.\n' >"$FIXTURES/new.txt"
  local out
  out=$(word_delta "$FIXTURES/old.txt" "$FIXTURES/new.txt")
  assert_contains "[-every file.-]" "$out"
  assert_contains "{+matching files.+}" "$out"
  assert_contains "apply to" "$out"
}

function test_word_delta_is_empty_when_the_texts_match() {
  printf 'a b c\n' >"$FIXTURES/old.txt"
  cp "$FIXTURES/old.txt" "$FIXTURES/new.txt"
  assert_empty "$(word_delta "$FIXTURES/old.txt" "$FIXTURES/new.txt")"
}

function test_word_delta_shows_an_indentation_change_as_lines() {
  # Same words, different YAML: `b` moves from a child of `a` to a sibling.
  printf 'Markdown\n```yaml\na:\n  b: 1\n```\n' >"$FIXTURES/old.txt"
  printf 'Markdown\n```yaml\na:\nb: 1\n```\n' >"$FIXTURES/new.txt"
  local out
  out=$(word_delta "$FIXTURES/old.txt" "$FIXTURES/new.txt")
  assert_contains "# whitespace" "$out"
  assert_contains "-  b: 1" "$out"
  assert_contains "+b: 1" "$out"
}

function test_delta_label_calls_a_line_only_delta_whitespace_only() {
  : >"$FIXTURES/vocab"
  printf 'a:\n  b: 1\n' >"$FIXTURES/old.txt"
  printf 'a:\nb: 1\n' >"$FIXTURES/new.txt"
  assert_equals "whitespace-only" "$(label_of "$FIXTURES/vocab")"
}

function test_fetch_one_drops_a_stale_extract_from_an_earlier_mode() {
  # Same run dir, same stem: an HTML fetch wrote .txt, a later fetch of the
  # same row lands in a body-hashed mode. The old .txt must not survive.
  local stem="$FIXTURES/run/pages/claude/docs-1-x.example-a.md"
  mkdir -p "$(dirname "$stem")"
  printf 'stale extract\n' >"$stem.txt"
  stub_curl "https://x.example/a.md|200|$(printf 'fresh markdown %.0s' $(seq 1 40))"
  fetch_one claude docs https://x.example/a.md "$FIXTURES/run" 1 >/dev/null
  assert_file_not_exists "$stem.txt"
}

function test_word_delta_prints_one_line_per_distant_change() {
  {
    printf 'start '
    printf 'filler %.0s' $(seq 1 60)
    printf 'end\n'
  } >"$FIXTURES/old.txt"
  sed -e 's/^start/begin/' -e 's/end$/finish/' "$FIXTURES/old.txt" >"$FIXTURES/new.txt"
  assert_equals 2 "$(word_delta "$FIXTURES/old.txt" "$FIXTURES/new.txt" | grep -c .)"
}

# ---- delta_label -------------------------------------------------------------

# label_of <vocab> labels the change from old.txt to new.txt the way
# write_deltas does.
label_of() {
  word_delta "$FIXTURES/old.txt" "$FIXTURES/new.txt" >"$FIXTURES/d"
  moved_words "$FIXTURES/old.txt" "$FIXTURES/new.txt" >"$FIXTURES/m"
  delta_label "$FIXTURES/d" "$FIXTURES/m" "$1"
}

function test_moved_words_prints_each_run_from_the_diff_records() {
  printf 'Use array[-1] and keep it\n' >"$FIXTURES/old.txt"
  printf 'Use array[-1] {+x+} and drop it\n' >"$FIXTURES/new.txt"
  assert_equals "$(printf '{+x+}\nkeep\ndrop')" "$(moved_words "$FIXTURES/old.txt" "$FIXTURES/new.txt")"
}

function test_delta_label_names_a_path_we_write() {
  printf '.cursor/BUGBOT.md\nalwaysApply\n' >"$FIXTURES/vocab"
  printf 'Each rule is truncated\n' >"$FIXTURES/old.txt"
  printf 'Each .cursor/BUGBOT.md file is truncated\n' >"$FIXTURES/new.txt"
  assert_equals "mentions:.cursor/BUGBOT.md" "$(label_of "$FIXTURES/vocab")"
}

function test_delta_label_ignores_a_vocabulary_hit_in_context_only() {
  printf '.cursor/BUGBOT.md\n' >"$FIXTURES/vocab"
  printf 'Create .cursor/BUGBOT.md files now\n' >"$FIXTURES/old.txt"
  printf 'Create .cursor/BUGBOT.md files today\n' >"$FIXTURES/new.txt"
  assert_equals "prose" "$(label_of "$FIXTURES/vocab")"
}

function test_delta_label_calls_known_page_chrome_chrome() {
  : >"$FIXTURES/vocab"
  printf 'Search... Ask Assistant \xe2\x8c\x98 I Navigation Dismiss Changelog faster. Instead\n' >"$FIXTURES/old.txt"
  printf 'Search... Navigation Dismiss [](https://kiro.dev/) K Changelog faster. Loading diagram... Instead\n' >"$FIXTURES/new.txt"
  assert_equals "chrome-only" "$(label_of "$FIXTURES/vocab")"
}

function test_delta_label_keeps_prose_that_sits_beside_chrome() {
  : >"$FIXTURES/vocab"
  printf 'x y\n' >"$FIXTURES/old.txt"
  printf 'x Loading diagram... Rules now need a trigger key y\n' >"$FIXTURES/new.txt"
  assert_equals "prose" "$(label_of "$FIXTURES/vocab")"
}

function test_delta_label_reads_a_removed_passage_holding_a_link() {
  : >"$FIXTURES/vocab"
  printf 'FAQ See [Cloud subagents](https://cursor.com/docs/subagents) for details end\n' >"$FIXTURES/old.txt"
  printf 'FAQ end\n' >"$FIXTURES/new.txt"
  assert_equals "prose" "$(label_of "$FIXTURES/vocab")"
}

function test_delta_label_reads_an_added_passage_holding_a_brace() {
  printf 'mcpServers\n' >"$FIXTURES/vocab"
  printf 'Set now\n' >"$FIXTURES/old.txt"
  printf 'Set {"mcpServers": {}} in the file now\n' >"$FIXTURES/new.txt"
  assert_equals "mentions:mcpServers" "$(label_of "$FIXTURES/vocab")"
}

# Vendor text can hold what the rendered delta uses as markers.
function test_delta_label_ignores_marker_lookalikes_in_vendor_text() {
  printf 'alwaysApply\n.cursor/BUGBOT.md\n' >"$FIXTURES/vocab"
  printf 'Use array[-1] value\n' >"$FIXTURES/old.txt"
  printf 'Use array[-1] important value\n' >"$FIXTURES/new.txt"
  assert_equals "prose" "$(label_of "$FIXTURES/vocab")"
  printf 'x changed [foo-](url) alwaysApply y\n' >"$FIXTURES/old.txt"
  printf 'x y\n' >"$FIXTURES/new.txt"
  assert_equals "mentions:alwaysApply" "$(label_of "$FIXTURES/vocab")"
  printf 'array[-1] .cursor/BUGBOT.md ordinary\n' >"$FIXTURES/old.txt"
  printf 'array[-1] .cursor/BUGBOT.md\n' >"$FIXTURES/new.txt"
  assert_equals "prose" "$(label_of "$FIXTURES/vocab")"
}

function test_delta_label_calls_an_empty_delta_whitespace_only() {
  : >"$FIXTURES/vocab"
  : >"$FIXTURES/d"
  : >"$FIXTURES/m"
  assert_equals "whitespace-only" "$(delta_label "$FIXTURES/d" "$FIXTURES/m" "$FIXTURES/vocab")"
}

# ---- delta_vocab -------------------------------------------------------------

function test_delta_vocab_keeps_paths_and_keys_but_not_plain_words() {
  local out
  out=$(delta_vocab cursor)
  assert_contains ".cursor/BUGBOT.md" "$out"
  assert_contains ".cursor/rules" "$out"
  assert_empty "$(printf '%s\n' "$out" | grep -x model)"
}

# ---- snapshots and deltas ----------------------------------------------------

# seed_run <dir> <url> <text> writes a one-row changed run with its hashed text.
# The row's hash is the real digest of its text, as fetch_one writes it.
function seed_run() {
  local dir="$1" url="$2" text="$3"
  mkdir -p "$dir/pages/cursor"
  printf '%s\n' "$text" >"$dir/pages/cursor/docs-1-x.txt"
  : >"$dir/pages/cursor/docs-1-x.body"
  printf 'cursor\tdocs\t%s\t200\thtml\t%s\t2026-09-24\tchanged\t%s\tpages/cursor/docs-1-x.body\n' \
    "$url" "$(sha256_of "$dir/pages/cursor/docs-1-x.txt")" "$url" >"$dir/docfetch.tsv"
}

# row_sha <run dir> prints the one seeded row's hash.
function row_sha() {
  cut -f6 "$1/docfetch.tsv"
}

function test_a_fetch_stores_its_text_under_its_hash() {
  seed_run "$FIXTURES/run" https://cursor.com/docs/bugbot "Bugbot reads .cursor/BUGBOT.md"
  write_deltas "$FIXTURES/run" >/dev/null
  assert_equals "Bugbot reads .cursor/BUGBOT.md" "$(cat "$(snapshot_file "$(row_sha "$FIXTURES/run")")")"
}

function test_write_deltas_diffs_a_changed_row_against_its_snapshot() {
  seed_run "$FIXTURES/old" https://cursor.com/docs/bugbot "Each rule is truncated at 30,000 characters"
  lock_merge "$FIXTURES/old/docfetch.tsv"
  seed_run "$FIXTURES/run" https://cursor.com/docs/bugbot "Each rule is truncated at 40,000 characters"
  write_deltas "$FIXTURES/run" >/dev/null
  assert_contains "[-30,000-]{+40,000+}" "$(cat "$FIXTURES/run/pages/cursor/docs-1-x.delta")"
  assert_contains "$(printf 'cursor\tdocs\thttps://cursor.com/docs/bugbot\tprose')" \
    "$(cat "$FIXTURES/run/deltas.tsv")"
}

function test_write_deltas_labels_a_row_without_a_snapshot() {
  seed_run "$FIXTURES/run" https://cursor.com/docs/bugbot "first sight"
  write_deltas "$FIXTURES/run" >/dev/null
  assert_equals "no-snapshot" "$(cut -f4 "$FIXTURES/run/deltas.tsv")"
  assert_file_not_exists "$FIXTURES/run/pages/cursor/docs-1-x.delta"
}

function test_write_deltas_prints_a_label_count() {
  seed_run "$FIXTURES/run" https://cursor.com/docs/bugbot "first sight"
  assert_equals "deltas: 1 (1 no-snapshot)" "$(write_deltas "$FIXTURES/run")"
}

function test_write_deltas_survives_the_scripts_strict_mode() {
  # docfetch.sh runs under set -euo pipefail, and diff exits 1 on any
  # difference. bashunit does not, so run it the way the script does.
  seed_run "$FIXTURES/old" https://cursor.com/docs/bugbot "old words here"
  lock_merge "$FIXTURES/old/docfetch.tsv"
  seed_run "$FIXTURES/run" https://cursor.com/docs/bugbot "new words here"
  local out
  out=$(bash -c 'set -euo pipefail; source "$1"; write_deltas "$2"' _ "$SCRIPT_DIR/docfetch.sh" "$FIXTURES/run")
  assert_equals "deltas: 1 (1 prose)" "$out"
}

function test_a_failed_row_stores_no_snapshot() {
  seed_run "$FIXTURES/shell" https://cursor.com/docs/bugbot "<div id=app></div>"
  sed -i.bak -e 's/\thtml\t[0-9a-f]*\t/\tapp-shell\t-\t/' -e 's/\tchanged\t/\tfailed\t/' "$FIXTURES/shell/docfetch.tsv"
  write_deltas "$FIXTURES/shell" >/dev/null
  assert_empty "$(ls "$DOCFETCH_SNAPSHOTS" 2>/dev/null)"
}

function test_word_delta_marks_a_capped_whitespace_diff_truncated() {
  local i
  for i in $(seq 1 300); do printf '  key%d: v\n' "$i"; done >"$FIXTURES/old.txt"
  for i in $(seq 1 300); do printf 'key%d: v\n' "$i"; done >"$FIXTURES/new.txt"
  word_delta "$FIXTURES/old.txt" "$FIXTURES/new.txt" >"$FIXTURES/d"
  assert_contains "# truncated: " "$(cat "$FIXTURES/d")"
  : >"$FIXTURES/vocab"
  assert_equals "whitespace-only:truncated" "$(label_of "$FIXTURES/vocab")"
}

function test_a_delta_follows_the_lock_not_the_last_fetch() {
  # Text A was audited, then text B (the lock moved, say by a pull). The
  # vendor reverts to A. The delta must show B -> A, not an empty diff
  # against A that some earlier local fetch left behind.
  seed_run "$FIXTURES/a" https://cursor.com/docs/bugbot "rules cap at 30,000"
  lock_merge "$FIXTURES/a/docfetch.tsv"
  seed_run "$FIXTURES/b" https://cursor.com/docs/bugbot "rules cap at 40,000"
  lock_merge "$FIXTURES/b/docfetch.tsv"
  seed_run "$FIXTURES/run" https://cursor.com/docs/bugbot "rules cap at 30,000"
  write_deltas "$FIXTURES/run" >/dev/null
  assert_contains "[-40,000-]{+30,000+}" "$(cat "$FIXTURES/run/pages/cursor/docs-1-x.delta")"
}

function test_a_lock_hash_with_no_stored_text_reads_no_snapshot() {
  # A fresh machine: the lock names a hash this checkout never fetched.
  seed_run "$FIXTURES/a" https://cursor.com/docs/bugbot "rules cap at 30,000"
  lock_merge "$FIXTURES/a/docfetch.tsv"
  rm -rf "$DOCFETCH_SNAPSHOTS"
  seed_run "$FIXTURES/run" https://cursor.com/docs/bugbot "rules cap at 40,000"
  write_deltas "$FIXTURES/run" >/dev/null
  assert_equals "no-snapshot" "$(cut -f4 "$FIXTURES/run/deltas.tsv")"
}

function test_snapshot_prune_keeps_locked_and_recent_text() {
  seed_run "$FIXTURES/a" https://cursor.com/docs/bugbot "locked text"
  lock_merge "$FIXTURES/a/docfetch.tsv"
  printf 'old unlocked\n' >"$(snapshot_file deadbeef)"
  printf 'new unlocked\n' >"$(snapshot_file cafef00d)"
  touch -t 202601010000 "$(snapshot_file deadbeef)" "$(snapshot_file "$(row_sha "$FIXTURES/a")")"
  snapshot_prune
  assert_file_not_exists "$(snapshot_file deadbeef)"
  assert_file_exists "$(snapshot_file cafef00d)"
  assert_file_exists "$(snapshot_file "$(row_sha "$FIXTURES/a")")"
}

# old_run <name> builds a run fetched on 2026-01-01 with pages and the small
# files that always stay.
function old_run() {
  local run="$DOCFETCH_RUNS/$1"
  mkdir -p "$run/pages/claude" "$run/rows"
  printf 'page\n' >"$run/pages/claude/docs-1-memory.txt"
  printf 'row\n' >"$run/rows/claude.tsv"
  printf 'row\n' >"$run/docfetch.tsv"
  printf 'Quote from local/target-audit/%s/pages/claude/docs-1-memory.txt\n' "$1" >"$run/claude-mcp.md"
  find "$run" -exec touch -t 202601010000 {} +
}

function test_pages_prune_removes_pages_of_an_old_unreferenced_run() {
  old_run 2026-01-01-run
  local out
  out=$(pages_prune "$DOCFETCH_RUNS/2026-10-28-run")
  assert_contains "pages: removed 2026-01-01-run/pages" "$out"
  assert_directory_not_exists "$DOCFETCH_RUNS/2026-01-01-run/pages"
  assert_file_exists "$DOCFETCH_RUNS/2026-01-01-run/rows/claude.tsv"
  assert_file_exists "$DOCFETCH_RUNS/2026-01-01-run/docfetch.tsv"
  assert_file_exists "$DOCFETCH_RUNS/2026-01-01-run/claude-mcp.md"
}

function test_pages_prune_keeps_a_run_an_audit_table_cites() {
  old_run 2026-01-01-run
  printf 'case\tlocal/target-audit/2026-01-01-run/pages/claude/docs-1-memory.txt\n' >"$(dirname "$LOCK")/surface-cases.tsv"
  pages_prune "$DOCFETCH_RUNS/2026-10-28-run" >/dev/null
  assert_directory_exists "$DOCFETCH_RUNS/2026-01-01-run/pages"
}

function test_pages_prune_keeps_a_run_a_report_links_into() {
  old_run 2026-01-01-run
  old_run 2026-01-01-run2
  printf 'See [mcp](2026-01-01-run/claude-mcp.md).\n' >"$DOCFETCH_RUNS/2026-01-02.md"
  pages_prune "$DOCFETCH_RUNS/2026-10-28-run" >/dev/null
  assert_directory_exists "$DOCFETCH_RUNS/2026-01-01-run/pages"
  assert_directory_not_exists "$DOCFETCH_RUNS/2026-01-01-run2/pages"
}

function test_pages_prune_keeps_the_run_being_finalized() {
  old_run 2026-01-01-run
  pages_prune "$DOCFETCH_RUNS/2026-01-01-run" >/dev/null
  assert_directory_exists "$DOCFETCH_RUNS/2026-01-01-run/pages"
}

function test_pages_prune_keeps_a_recent_run() {
  old_run 2026-10-28-run
  touch "$DOCFETCH_RUNS/2026-10-28-run/pages"
  pages_prune "$DOCFETCH_RUNS/2026-10-29-run" >/dev/null
  assert_directory_exists "$DOCFETCH_RUNS/2026-10-28-run/pages"
}

function test_pages_prune_keeps_a_cited_run_among_many_citations() {
  old_run 2026-01-01-aaa
  {
    printf 'case\tlocal/target-audit/2026-01-01-aaa/pages/claude/docs-1-memory.txt\n'
    local i
    for i in $(seq 1 2000); do
      printf 'case-%s\tlocal/target-audit/2026-09-%s-run/pages/claude/docs-1-memory.txt\n' "$i" "$i"
    done
  } >"$(dirname "$LOCK")/surface-cases.tsv"
  pages_prune "$DOCFETCH_RUNS/2026-10-28-run" >/dev/null
  assert_directory_exists "$DOCFETCH_RUNS/2026-01-01-aaa/pages"
}

function test_pages_prune_keeps_a_run_a_sub_report_of_another_run_cites() {
  old_run 2026-01-01-run
  old_run 2026-01-05-run
  printf 'Evidence: local/target-audit/2026-01-01-run/pages/claude/docs-1-memory.txt\n' >"$DOCFETCH_RUNS/2026-01-05-run/batch-4.md"
  touch -t 202601050000 "$DOCFETCH_RUNS/2026-01-05-run/batch-4.md" "$DOCFETCH_RUNS/2026-01-05-run"
  pages_prune "$DOCFETCH_RUNS/2026-10-28-run" >/dev/null
  assert_directory_exists "$DOCFETCH_RUNS/2026-01-01-run/pages"
  assert_directory_not_exists "$DOCFETCH_RUNS/2026-01-05-run/pages"
}

function test_pages_prune_keeps_a_run_a_report_names_without_a_path() {
  old_run 2026-01-01-run
  old_run 2026-01-01-run.b
  printf 'Run: `2026-01-01-run`\n' >"$DOCFETCH_RUNS/2026-01-02.md"
  pages_prune "$DOCFETCH_RUNS/2026-10-28-run" >/dev/null
  assert_directory_exists "$DOCFETCH_RUNS/2026-01-01-run/pages"
  assert_directory_not_exists "$DOCFETCH_RUNS/2026-01-01-run.b/pages"
}

function test_pages_prune_keeps_a_run_named_at_the_end_of_a_sentence() {
  old_run 2026-01-01-run
  printf 'Diffed against local/target-audit/2026-01-01-run.\n' >"$DOCFETCH_RUNS/2026-01-02.md"
  pages_prune "$DOCFETCH_RUNS/2026-10-28-run" >/dev/null
  assert_directory_exists "$DOCFETCH_RUNS/2026-01-01-run/pages"
}

function test_pages_prune_keeps_a_run_with_a_recent_write() {
  old_run 2026-01-01-run
  printf 'late report\n' >"$DOCFETCH_RUNS/2026-01-01-run/late.txt"
  pages_prune "$DOCFETCH_RUNS/2026-10-28-run" >/dev/null
  assert_directory_exists "$DOCFETCH_RUNS/2026-01-01-run/pages"
}

function test_pages_prune_keeps_a_run_whose_name_a_scan_cannot_match() {
  old_run '2026-01-01T12:00 run'
  pages_prune "$DOCFETCH_RUNS/2026-10-28-run" >/dev/null
  assert_directory_exists "$DOCFETCH_RUNS/2026-01-01T12:00 run/pages"
}

function test_out_rejects_a_folder_name_a_citation_scan_cannot_match() {
  local status=0 err
  err=$(docfetch_main --out "$FIXTURES/2026-10-28 run" claude 2>&1 >/dev/null) || status=$?
  assert_equals 2 "$status"
  assert_contains "--out folder name may use only" "$err"
  assert_directory_not_exists "$FIXTURES/2026-10-28 run"
}

function test_pages_prune_reports_a_failed_removal_and_continues() {
  old_run 2026-01-01-run
  old_run 2026-01-02-run
  chmod 555 "$DOCFETCH_RUNS/2026-01-01-run/pages/claude"
  local out err
  out=$(pages_prune "$DOCFETCH_RUNS/2026-10-28-run" 2>"$FIXTURES/err")
  err=$(cat "$FIXTURES/err")
  chmod 755 "$DOCFETCH_RUNS/2026-01-01-run/pages/claude"
  assert_contains "pages: could not remove" "$err"
  assert_contains "pages: removed 2026-01-02-run/pages" "$out"
}

function test_validated_update_prunes_old_run_pages_after_finalizing() {
  old_run 2026-01-01-run
  mkdir -p "$DOCFETCH_RUNS/2026-10-28-run"
  # shellcheck disable=SC2329
  function source_sections() { printf '%s\n' '- docs: https://code.claude.com/docs/en/memory'; }
  run_row claude https://code.claude.com/docs/en/memory new 2026-10-16 >"$DOCFETCH_RUNS/2026-10-28-run/docfetch.tsv"
  # The coverage checker has its own Go tests; here it only has to pass.
  function go() { printf '%s\n' "$*" >"$FIXTURES/go.args"; }
  local out
  out=$(docfetch_main --update "$DOCFETCH_RUNS/2026-10-28-run/docfetch.tsv" 2>/dev/null)
  unset -f go
  assert_contains "finalize $DOCFETCH_RUNS/2026-10-28-run" "$(cat "$FIXTURES/go.args")"
  assert_contains "pages: removed 2026-01-01-run/pages" "$out"
}

function test_update_prunes_old_run_pages() {
  old_run 2026-01-01-run
  run_row claude https://code.claude.com/docs/en/memory new 2026-10-16 >"$FIXTURES/docfetch.tsv"
  local out
  out=$(docfetch_main --update-unvalidated "$FIXTURES/docfetch.tsv" 2>/dev/null)
  assert_contains "pages: removed 2026-01-01-run/pages" "$out"
}

function test_lock_prune_drops_urls_no_source_lists() {
  {
    echo "# lock"
    printf 'claude\tdocs\thttps://code.claude.com/docs/en/memory\t200\thtml\taaa\t2026-09-01\n'
    printf 'junie\tdocs\thttps://junie.example/removed.html\t200\thtml\tbbb\t2026-09-01\n'
  } >"$LOCK"
  lock_prune 2>/dev/null
  assert_contains "code.claude.com/docs/en/memory" "$(cat "$LOCK")"
  assert_not_contains "junie.example/removed" "$(cat "$LOCK")"
}

function test_json_text_sorts_keys_one_value_per_line() {
  printf '{"b":1,"a":{"d":2,"c":3}}' >"$FIXTURES/j"
  json_text "$FIXTURES/j" "$FIXTURES/j.txt"
  assert_equals '"a": {' "$(sed -n 2p "$FIXTURES/j.txt" | sed 's/^ *//')"
  assert_equals "$(json_sum "$FIXTURES/j")" "$(json_sum "$FIXTURES/j.txt")"
}

function test_fetch_one_keeps_the_sorted_json_it_hashed() {
  function docfetch_curl() {
    printf '{"z":1,"a":2}' >"$2"
    printf '200\t%s\tapplication/json\n' "$1"
  }
  local row
  row=$(fetch_one openhands docs https://x.example/api "$FIXTURES/run" 1)
  assert_equals "json" "$(printf '%s' "$row" | cut -f5)"
  assert_equals '"a": 2,' "$(sed -n 2p "$FIXTURES/run/pages/openhands/docs-1-x.example-api.txt" | sed 's/^ *//')"
}

# ---- mirror measurement (#1127) ----------------------------------------------

function test_mirror_url_appends_md_to_the_page_path() {
  assert_equals "https://kiro.dev/docs/hooks.md" "$(mirror_url https://kiro.dev/docs/hooks/)"
  assert_equals "https://x.example/a/b.md" "$(mirror_url 'https://x.example/a/b?tab=ide#top')"
}

function test_mirror_one_hashes_a_served_markdown_copy() {
  local md
  md="$(printf '# Hooks\n\nHooks run on events. %.0s' 1 2 3 4 5 6 7 8 9 10 11 12)"
  stub_curl "https://kiro.dev/docs/hooks.md|200|$md"
  local row
  row=$(mirror_one kiro docs https://kiro.dev/docs/hooks/ "$FIXTURES/run" 1 reader-proxy)
  assert_equals "https://kiro.dev/docs/hooks.md" "$(printf '%s' "$row" | cut -f4)"
  assert_equals "200" "$(printf '%s' "$row" | cut -f5)"
  assert_matches '^[0-9a-f]{64}$' "$(printf '%s' "$row" | cut -f6)"
}

function test_mirror_one_rejects_an_html_answer() {
  stub_curl "https://x.example/*|200|<!DOCTYPE html><html><body>$(printf 'shell %.0s' $(seq 1 60))</body></html>"
  assert_equals "-" "$(mirror_one amp docs https://x.example/docs/tools "$FIXTURES/run" 1 html | cut -f6)"
}

function test_mirror_one_skips_rows_that_are_not_scraped_html() {
  stub_curl "https://x.example/*|200|$(printf 'text %.0s' $(seq 1 60))"
  assert_empty "$(mirror_one amp docs https://x.example/llms.txt "$FIXTURES/run" 1 text)"
}

# seed_compare <dir> <html sha> <mirror sha> writes one run with one mirrored row.
function seed_compare() {
  mkdir -p "$1"
  printf 'kiro\tdocs\thttps://kiro.dev/docs/hooks/\t200\treader-proxy\t%s\t2026-09-24\tchanged\thttps://kiro.dev/docs/hooks/\tpages/x.body\n' \
    "$2" >"$1/docfetch.tsv"
  printf 'kiro\tdocs\thttps://kiro.dev/docs/hooks/\thttps://kiro.dev/docs/hooks.md\t200\t%s\n' "$3" >"$1/mirrors.tsv"
}

function test_compare_mirrors_counts_moves_per_representation() {
  seed_compare "$FIXTURES/a" aaa mmm
  seed_compare "$FIXTURES/b" bbb mmm
  local out
  out=$(compare_mirrors "$FIXTURES/a" "$FIXTURES/b")
  assert_contains "$(printf 'kiro.dev\t1\t1\t0')" "$out"
  assert_contains "$(printf 'total\t1\t1\t0')" "$out"
}

function test_compare_mirrors_ignores_rows_without_a_mirror_in_both_runs() {
  seed_compare "$FIXTURES/a" aaa mmm
  seed_compare "$FIXTURES/b" bbb -
  assert_contains "$(printf 'total\t0\t0\t0')" "$(compare_mirrors "$FIXTURES/a" "$FIXTURES/b")"
}

function test_fetch_target_writes_mirror_rows_only_when_asked() {
  local md
  md="$(printf 'Hooks run on events. %.0s' $(seq 1 20))"
  stub_curl "https://code.claude.com/*.md|200|$md" \
    "https://*|200|<html><body><main>$(printf 'Real page text. %.0s' $(seq 1 60))</main></body></html>"
  function resolve_urls() { printf 'docs\thttps://code.claude.com/docs/en/hooks\n'; }
  fetch_target claude "$FIXTURES/plain" >/dev/null
  assert_file_not_exists "$FIXTURES/plain/rows/claude.mirrors"
  DOCFETCH_MIRRORS=1 fetch_target claude "$FIXTURES/mirr" >/dev/null
  assert_equals 1 "$(grep -c . "$FIXTURES/mirr/rows/claude.mirrors")"
}

function test_compare_mirrors_mode_requires_two_runs() {
  local status=0
  docfetch_main --compare-mirrors "$FIXTURES/a" 2>/dev/null || status=$?
  assert_equals 2 "$status"
}

function test_compare_mirrors_mode_rejects_a_run_without_mirrors() {
  mkdir -p "$FIXTURES/a" "$FIXTURES/b"
  : >"$FIXTURES/a/docfetch.tsv"
  : >"$FIXTURES/b/docfetch.tsv"
  local status=0
  docfetch_main --compare-mirrors "$FIXTURES/a" "$FIXTURES/b" 2>/dev/null || status=$?
  assert_equals 1 "$status"
}

function test_compare_mirrors_accepts_the_same_run_twice() {
  seed_compare "$FIXTURES/a" aaa mmm
  assert_contains "$(printf 'total\t1\t0\t0')" "$(compare_mirrors "$FIXTURES/a" "$FIXTURES/a")"
}

function test_fetch_target_rewrites_mirror_rows_on_a_rerun() {
  local md
  md="$(printf 'Hooks run on events. %.0s' $(seq 1 20))"
  stub_curl "https://code.claude.com/*.md|200|$md" \
    "https://*|200|<html><body><main>$(printf 'Real page text. %.0s' $(seq 1 60))</main></body></html>"
  function resolve_urls() { printf 'docs\thttps://code.claude.com/docs/en/hooks\n'; }
  DOCFETCH_MIRRORS=1 fetch_target claude "$FIXTURES/run" >/dev/null
  DOCFETCH_MIRRORS=1 fetch_target claude "$FIXTURES/run" >/dev/null
  assert_equals 1 "$(grep -c . "$FIXTURES/run/rows/claude.mirrors")"
}

function test_a_fetch_replaces_a_truncated_snapshot() {
  seed_run "$FIXTURES/run" https://cursor.com/docs/bugbot "the full page text"
  mkdir -p "$DOCFETCH_SNAPSHOTS"
  printf 'the full' >"$(snapshot_file "$(row_sha "$FIXTURES/run")")"
  write_deltas "$FIXTURES/run" >/dev/null
  assert_equals "the full page text" "$(cat "$(snapshot_file "$(row_sha "$FIXTURES/run")")")"
}

function test_a_corrupt_snapshot_at_the_lock_hash_reads_no_snapshot() {
  seed_run "$FIXTURES/a" https://cursor.com/docs/bugbot "rules cap at 30,000"
  lock_merge "$FIXTURES/a/docfetch.tsv"
  printf 'damaged by a cache restore\n' >"$(snapshot_file "$(row_sha "$FIXTURES/a")")"
  seed_run "$FIXTURES/run" https://cursor.com/docs/bugbot "rules cap at 40,000"
  write_deltas "$FIXTURES/run" >/dev/null
  assert_equals "no-snapshot" "$(cut -f4 "$FIXTURES/run/deltas.tsv")"
  assert_file_not_exists "$(snapshot_file "$(row_sha "$FIXTURES/a")")"
}


function test_source_change_label_ignores_only_junie_download_counters() {
  printf '[{"id":1,"version":"1.0","downloads":12,"notes":"Hooks"}]' >"$FIXTURES/old"
  printf '[{"id":1,"version":"1.0","downloads":13,"notes":"Hooks"}]' >"$FIXTURES/new"
  assert_equals 'counter-only:junie-downloads:v1' "$(source_change_label changelog 'https://plugins.jetbrains.com/api/plugins/26104/updates?size=20' "$FIXTURES/old" "$FIXTURES/new")"
  printf '[{"id":1,"version":"1.0","downloads":13,"notes":"New hooks"}]' >"$FIXTURES/new"
  assert_empty "$(source_change_label changelog 'https://plugins.jetbrains.com/api/plugins/26104/updates?size=20' "$FIXTURES/old" "$FIXTURES/new")"
}

function test_source_change_label_keeps_malformed_and_changed_junie_layouts_visible() {
  printf '[{"id":1,"version":"1.0","downloads":12}]' >"$FIXTURES/old"
  for response in 'bad JSON' '[{"id":1,"version":"1.0","downloads":13}][{"id":1,"version":"1.0","downloads":13}]' '[{"id":1,"version":"1.0","downloads":13},{"id":1,"version":"1.0","downloads":13}]' '[{"id":1,"version":"1.0","downloads":-1}]' '[]' '[{"id":1,"version":"1.0","downloads":"13"}]' '[{"id":1,"version":"2.0","downloads":13}]' '{"downloads":13}'; do
    printf '%s' "$response" >"$FIXTURES/new"
    assert_empty "$(source_change_label changelog 'https://plugins.jetbrains.com/api/plugins/26104/updates?size=20' "$FIXTURES/old" "$FIXTURES/new")"
  done
  assert_empty "$(source_change_label changelog 'https://plugins.jetbrains.com/api/plugins/26104/updates?size=20' "$FIXTURES/missing" "$FIXTURES/new")"
}

function test_source_change_label_ignores_only_pinned_tree_watchers() {
  printf 'Files README.md Stars 27.5k stars Watchers 8 watching Forks 5.2k forks Report repository About Stars 27.5k stars Watchers 8 watching Forks 5.2k forks Report repository\n' >"$FIXTURES/old"
  printf 'Files README.md Stars 27.5k stars Watchers 9 watching Forks 5.2k forks Report repository About Stars 27.5k stars Watchers 9 watching Forks 5.2k forks Report repository\n' >"$FIXTURES/new"
  local url='https://github.com/xai-org/grok-build/tree/2bdd1d6a6369de0e8c68132ea4539e9abd9e14a8'
  assert_equals 'counter-only:github-watchers:v1' "$(source_change_label code "$url" "$FIXTURES/old" "$FIXTURES/new")"
  assert_empty "$(source_change_label code https://github.com/xai-org/grok-build/tree/main "$FIXTURES/old" "$FIXTURES/new")"
  sed 's/README.md/AGENTS.md/g' "$FIXTURES/old" >"$FIXTURES/new"
  assert_empty "$(source_change_label code "$url" "$FIXTURES/old" "$FIXTURES/new")"
  printf 'Files README.md Watchers unknown watching Forks 2 forks\n' >"$FIXTURES/new"
  assert_empty "$(source_change_label code "$url" "$FIXTURES/old" "$FIXTURES/new")"
}

function test_source_change_label_separates_archive_additions_from_new_and_edited_entries() {
  printf '# Changelog\n\n## 2026-10-20\nNew hooks.\n\n## 2026-10-10\nOld hooks.\n' >"$FIXTURES/old"
  cp "$FIXTURES/old" "$FIXTURES/new"
  printf '\n## 2026-01-01\nRestored archive.\n' >>"$FIXTURES/new"
  assert_equals 'historical-additions:v1' "$(source_change_label changelog https://example.com/changelog "$FIXTURES/old" "$FIXTURES/new")"
  printf '\n## 2026-10-21\nNew release.\n' >>"$FIXTURES/new"
  assert_equals 'new-entries:v1' "$(source_change_label changelog https://example.com/changelog "$FIXTURES/old" "$FIXTURES/new")"
  printf '# Changelog\n\n## 2026-10-20\nNew hooks.\n\n## 2026-10-10\nEdited old hooks.\n' >"$FIXTURES/new"
  assert_equals 'edited-entries:v1' "$(source_change_label changelog https://example.com/changelog "$FIXTURES/old" "$FIXTURES/new")"
}

function test_source_change_label_falls_back_for_ambiguous_dates_and_unknown_layouts() {
  printf '## 2026-10-20\nNotes.\n' >"$FIXTURES/old"
  for heading in '## 01/02/2026' '## 2026-02-31' '## 2026-10-20 and 2026-10-21' '## 2026-01-011' '### 2026-01-01' '## Undated'; do
    { cat "$FIXTURES/old"; printf '\n%s\nArchive.\n' "$heading"; } >"$FIXTURES/new"
    assert_empty "$(source_change_label changelog https://example.com/changelog "$FIXTURES/old" "$FIXTURES/new")"
  done
}

function test_release_index_v2_notices_edited_notes_and_migrates_without_hiding_changes() {
  printf '[{"tag_name":"v1","published_at":"2026-09-01T00:00:00Z","body":"Original"}]' >"$FIXTURES/old"
  release_index "$FIXTURES/old" >"$FIXTURES/index"
  assert_contains '# docfetch-release-index-v2' "$(cat "$FIXTURES/index")"
  printf '[{"tag_name":"v1","published_at":"2026-09-01T00:00:00Z","body":"Edited"}]' >"$FIXTURES/new"
  assert_not_equals "$(cat "$FIXTURES/index")" "$(release_index "$FIXTURES/new")"
  write_lock https://github.com/example/tool/releases "$(printf '\"tag_name\":\"v1\"\n\"published_at\":\"2026-09-01T00:00:00Z\"\n' | shasum -a 256 | awk '{print $1}')"
  assert_equals changed "$(row_status https://github.com/example/tool/releases github-api "$(sha256_of "$FIXTURES/index")")"
}


function test_write_deltas_classifies_counter_replay_without_advancing_the_lock() {
  local url='https://plugins.jetbrains.com/api/plugins/26104/updates?size=20'
  seed_run "$FIXTURES/old" "$url" '[{"id":1,"version":"1.0","downloads":243,"notes":"Hooks."}]'
  lock_merge "$FIXTURES/old/docfetch.tsv"
  local before
  before=$(cat "$LOCK")
  seed_run "$FIXTURES/run" "$url" '[{"id":1,"version":"1.0","downloads":244,"notes":"Hooks."}]'
  write_deltas "$FIXTURES/run" >/dev/null
  assert_equals 'counter-only:junie-downloads:v1' "$(cut -f4 "$FIXTURES/run/deltas.tsv")"
  assert_contains '# comparison: source-aware-v1' "$(cat "$FIXTURES/run/pages/cursor/docs-1-x.delta")"
  assert_contains '244' "$(cat "$FIXTURES/run/pages/cursor/docs-1-x.delta")"
  assert_equals "$before" "$(cat "$LOCK")"
  assert_equals changed "$(cut -f8 "$FIXTURES/run/docfetch.tsv")"
  assert_file_exists "$FIXTURES/run/pages/cursor/docs-1-x.body"
}

function test_source_change_label_keeps_every_non_counter_junie_field_visible() {
  printf '[{"id":1,"version":"1.0","downloads":12,"notes":"Hooks","date":"2026-10-20","extra":{"downloads":3}}]' >"$FIXTURES/old"
  for update in '.version = "2.0"' '.notes = "New notes"' '.date = "2026-10-21"' '.extra.downloads = 4' '.new = true'; do
    jq "map($update | .downloads = 13)" "$FIXTURES/old" >"$FIXTURES/new"
    assert_empty "$(source_change_label changelog 'https://plugins.jetbrains.com/api/plugins/26104/updates?size=20' "$FIXTURES/old" "$FIXTURES/new")"
  done
}

function test_release_index_keeps_malformed_responses_visible() {
  printf '{"message":"layout changed","notes":"new"}' >"$FIXTURES/response"
  assert_equals "$(cat "$FIXTURES/response")" "$(release_index "$FIXTURES/response")"
}

function test_source_change_label_keeps_watcher_text_in_other_repositories_visible() {
  printf 'Files README.md Stars 27.5k stars Watchers 8 watching Forks 5.2k forks Report repository About Stars 27.5k stars Watchers 8 watching Forks 5.2k forks Report repository\n' >"$FIXTURES/old"
  printf 'Files README.md Stars 27.5k stars Watchers 9 watching Forks 5.2k forks Report repository About Stars 27.5k stars Watchers 9 watching Forks 5.2k forks Report repository\n' >"$FIXTURES/new"
  assert_empty "$(source_change_label code https://github.com/example/docs/tree/2bdd1d6a6369de0e8c68132ea4539e9abd9e14a8 "$FIXTURES/old" "$FIXTURES/new")"
}

function test_source_change_label_rejects_unrecognized_pinned_tree_layouts() {
  local url='https://github.com/xai-org/grok-build/tree/2bdd1d6a6369de0e8c68132ea4539e9abd9e14a8'
  printf 'Stars 27.5k stars Watchers 8 watching Forks 5.2k forks Report repository Stars 27.5k stars Watchers 8 watching Forks 5.2k forks Report repository\n' >"$FIXTURES/old"
  for current in \
    'Stars 27.5k stars Watchers 9 watching Forks 5.2k forks Report repository' \
    'Stars 27.5k stars Watchers 9 watching Forks 5.2k forks Report repository Stars 27.5k stars Watchers 10 watching Forks 5.2k forks Report repository' \
    'Stars 27.6k stars Watchers 9 watching Forks 5.2k forks Report repository Stars 27.6k stars Watchers 9 watching Forks 5.2k forks Report repository' \
    'Stars 27.5k stars Watchers 9 watching Forks 5.3k forks Report repository Stars 27.5k stars Watchers 9 watching Forks 5.3k forks Report repository'; do
    printf '%s\n' "$current" >"$FIXTURES/new"
    assert_empty "$(source_change_label code "$url" "$FIXTURES/old" "$FIXTURES/new")"
  done
  cp "$FIXTURES/old" "$FIXTURES/new"
  assert_empty "$(source_change_label docs "$url" "$FIXTURES/old" "$FIXTURES/new")"
}

function test_source_change_label_classifies_github_release_archives_and_old_note_edits() {
  local url='https://github.com/example/tool/releases'
  printf '[{"tag_name":"v2","published_at":"2026-10-20T00:00:00Z","body":"Current notes."}]' >"$FIXTURES/releases"
  release_index "$FIXTURES/releases" >"$FIXTURES/old"
  jq '. + [{tag_name:"v1", published_at:"2026-01-01T00:00:00Z", body:"Archive notes."}]' "$FIXTURES/releases" >"$FIXTURES/archive"
  release_index "$FIXTURES/archive" >"$FIXTURES/new"
  assert_equals 'historical-additions:v1' "$(source_change_label changelog "$url" "$FIXTURES/old" "$FIXTURES/new")"
  cp "$FIXTURES/new" "$FIXTURES/old"
  jq '.[1].body = "Edited archive notes."' "$FIXTURES/archive" >"$FIXTURES/edited"
  release_index "$FIXTURES/edited" >"$FIXTURES/new"
  assert_equals 'edited-entries:v1' "$(source_change_label changelog "$url" "$FIXTURES/old" "$FIXTURES/new")"
  jq '. + [{tag_name:"v3", published_at:"2026-10-21T00:00:00Z", body:"New notes."}]' "$FIXTURES/archive" >"$FIXTURES/latest"
  release_index "$FIXTURES/latest" >"$FIXTURES/new"
  assert_equals 'new-entries:v1' "$(source_change_label changelog "$url" "$FIXTURES/old" "$FIXTURES/new")"
}

function test_source_change_label_does_not_classify_removed_entries_or_header_edits() {
  printf '# Changelog\n\n## 2026-10-20\nNotes.\n\n## 2026-01-01\nOld notes.\n' >"$FIXTURES/old"
  printf '# Changelog\n\n## 2026-10-20\nNotes.\n' >"$FIXTURES/new"
  assert_empty "$(source_change_label changelog https://example.com/changelog "$FIXTURES/old" "$FIXTURES/new")"
  sed 's/# Changelog/# Updated changelog/' "$FIXTURES/old" >"$FIXTURES/new"
  assert_empty "$(source_change_label changelog https://example.com/changelog "$FIXTURES/old" "$FIXTURES/new")"
}

function test_release_index_rejects_multiple_json_documents_and_invalid_field_types() {
  for response in \
    '[{"tag_name":"v1","published_at":"2026-10-20T00:00:00Z","body":false}]' \
    '[{"tag_name":"v1","published_at":"2026-10-20T00:00:00Z","prerelease":"false"}]' \
    '[][]'; do
    printf '%s' "$response" >"$FIXTURES/response"
    assert_equals "$response" "$(release_index "$FIXTURES/response")"
  done
}

function test_fetch_one_notices_release_note_edits_without_a_new_tag() {
  stub_curl 'https://api.github.com/*|200|[{"tag_name":"v1","published_at":"2026-09-01T00:00:00Z","body":"Original notes."}]'
  local first second
  first=$(fetch_one crush changelog https://github.com/charmbracelet/crush/releases "$FIXTURES/run" 1)
  stub_curl 'https://api.github.com/*|200|[{"tag_name":"v1","published_at":"2026-09-01T00:00:00Z","body":"Edited notes."}]'
  second=$(fetch_one crush changelog https://github.com/charmbracelet/crush/releases "$FIXTURES/run" 2)
  assert_not_equals "$(printf '%s' "$first" | cut -f6)" "$(printf '%s' "$second" | cut -f6)"
  assert_contains 'Edited notes.' "$(cat "$FIXTURES/run/pages/crush/changelog-2-github.com-charmbracelet-crush-releases.body")"
}

# ---- churn-proof comparison ------------------------------------------------------

# The fixtures below are trimmed from the 2026-10-27 and 2026-10-29 audit
# runs: page bytes that moved while the documentation did not.

function openhands_page() {
  printf '<html><body><div id="content"><h1>Skills</h1><p>%s</p></div>' "$1"
  printf '<p>Was this page helpful?</p>'
  printf '<template data-mint="manifest">{"docsConfig":{"theme":"mint","name":"OpenHands Docs"},"tags":[{"tag":"Reference","count":%s}],"deployment":{"id":"%s"}}</template>' "$2" "$3"
  printf '</body></html>'
}

function test_strip_html_ignores_openhands_template_payload_churn() {
  openhands_page 'Skills load from .openhands/skills.' 95 dpl_old >"$FIXTURES/a.html"
  openhands_page 'Skills load from .openhands/skills.' 70 dpl_new >"$FIXTURES/b.html"
  openhands_page 'Skills load from .agents/skills.' 70 dpl_new >"$FIXTURES/c.html"
  assert_equals "$(strip_html "$FIXTURES/a.html")" "$(strip_html "$FIXTURES/b.html")"
  assert_not_contains 'docsConfig' "$(strip_html "$FIXTURES/a.html")"
  assert_not_equals "$(strip_html "$FIXTURES/b.html")" "$(strip_html "$FIXTURES/c.html")"
}

function vscode_page() {
  printf '<html><body><div id="main">'
  [ -z "$1" ] || printf '<header class="hf-site-header"><a class="v2-skip-link" href="#main-content">Skip to content</a><nav aria-label="Primary">Docs</nav><p class="v2-download-notice">By using VS Code, you agree to its license and privacy statement.</p></header>'
  printf '<main id="main-content"><article><header><h1>MCP configuration reference</h1></header><p>%s</p></article>' "$2"
  printf '<div class="body-footer">%s</div></main></div></body></html>' "$3"
}

function test_strip_html_ignores_the_vscode_site_header_and_footer_date() {
  vscode_page '' 'Servers live in .vscode/mcp.json.' 10/21/2026 >"$FIXTURES/a.html"
  vscode_page header 'Servers live in .vscode/mcp.json.' 10/28/2026 >"$FIXTURES/b.html"
  vscode_page header 'Servers live in .vscode/mcp.jsonc.' 10/28/2026 >"$FIXTURES/c.html"
  assert_equals "$(strip_html "$FIXTURES/a.html")" "$(strip_html "$FIXTURES/b.html")"
  assert_contains 'MCP configuration reference' "$(strip_html "$FIXTURES/b.html")"
  assert_not_equals "$(strip_html "$FIXTURES/b.html")" "$(strip_html "$FIXTURES/c.html")"
}

function test_strip_html_keeps_a_header_inside_the_page_content() {
  printf '<article><header><h1>Hooks</h1></header>Hooks run before tools.<p>Exit 2 blocks.</p></article>' >"$FIXTURES/a.html"
  assert_equals 'Hooks Hooks run before tools. Exit 2 blocks.' "$(strip_html "$FIXTURES/a.html")"
}

function test_strip_html_keeps_text_after_a_banner_and_a_template() {
  printf '<header>Search</header>Intro<main><template>{"n":1}</template>Body</main>' >"$FIXTURES/a.html"
  assert_equals 'Intro Body' "$(strip_html "$FIXTURES/a.html")"
}

function test_strip_html_keeps_a_header_on_a_page_without_main() {
  printf '<body><header><h1>Install</h1><p>Run npm i foo</p></header><div>Then start it.</div></body>' >"$FIXTURES/a.html"
  assert_equals 'Install Run npm i foo Then start it.' "$(strip_html "$FIXTURES/a.html")"
}

function test_strip_html_reads_a_tag_name_followed_by_a_newline() {
  printf '<header>Search</header><main\n  id="c"><header\n><h1>Title</h1></header><p>Body</p></main>' >"$FIXTURES/a.html"
  assert_equals 'Title Body' "$(strip_html "$FIXTURES/a.html")"
}

function test_strip_html_ignores_a_main_tag_written_in_a_script() {
  printf '<body><header><h1>Install</h1></header><div><p>Body</p></div><script>x="<main>"</script></body>' >"$FIXTURES/a.html"
  assert_equals 'Install Body' "$(strip_html "$FIXTURES/a.html")"
}

function test_strip_html_reads_tags_inside_a_script_as_script_text() {
  printf '<main><script>var t = "<template><header><nav>";</script><p>Body after script</p></main>' >"$FIXTURES/a.html"
  assert_equals 'Body after script' "$(strip_html "$FIXTURES/a.html")"
}

function test_strip_html_keeps_a_declarative_shadow_root() {
  printf '<main><div><template shadowrootmode="open">Shadow text</template></div><template>hidden</template>End</main>' >"$FIXTURES/a.html"
  assert_equals 'Shadow text End' "$(strip_html "$FIXTURES/a.html")"
}

function test_strip_html_skips_only_the_body_footer_class() {
  printf '<main><div class="card-body-footer">Use .agents/skills</div><div class="x body-footer">Updated 10/29</div></main>' >"$FIXTURES/a.html"
  assert_equals 'Use .agents/skills' "$(strip_html "$FIXTURES/a.html")"
}

function warp_page() {
  printf '# MCP Servers\n\nWarp reads %s on next launch.\n\n![MCP servers page](https://docs.warp.dev/_astro/mcp-servers-list.C9p6MT-__ZcHsrA.webp?dpl=%s)\n' "$1" "$2"
}

function test_fetch_one_ignores_warp_deployment_ids_in_markdown() {
  local first second third
  stub_curl "https://docs.warp.dev/*|200|$(warp_page .warp/mcp.json dpl_3UYmRJ5sFATkWCXNWe4b5qriTMGB)"
  first=$(fetch_one warp docs https://docs.warp.dev/agents/capabilities/mcp.md "$FIXTURES/run" 1 | cut -f6)
  stub_curl "https://docs.warp.dev/*|200|$(warp_page .warp/mcp.json dpl_EXqqRnv2bL1vHYaSh3QVnP7yZjw4)"
  second=$(fetch_one warp docs https://docs.warp.dev/agents/capabilities/mcp.md "$FIXTURES/run" 1 | cut -f6)
  stub_curl "https://docs.warp.dev/*|200|$(warp_page .warp/mcp.jsonc dpl_EXqqRnv2bL1vHYaSh3QVnP7yZjw4)"
  third=$(fetch_one warp docs https://docs.warp.dev/agents/capabilities/mcp.md "$FIXTURES/run" 1 | cut -f6)
  assert_equals "$first" "$second"
  assert_not_equals "$second" "$third"
  assert_contains 'dpl=dpl_EXqqRnv2bL1vHYaSh3QVnP7yZjw4' "$(cat "$FIXTURES/run/pages/warp/docs-1-docs.warp.dev-agents-capabilities-mcp.md.body")"
}

function test_fetch_one_hashes_raw_text_without_deployment_ids_as_fetched() {
  local row
  stub_curl "https://docs.warp.dev/*|200|$(printf '# Rules\n\nPlain page.\n')"
  row=$(fetch_one warp docs https://docs.warp.dev/agents/capabilities/rules.md "$FIXTURES/run" 1)
  assert_equals "$(sha256_of "$FIXTURES/run/pages/warp/docs-1-docs.warp.dev-agents-capabilities-rules.md.body")" "$(printf '%s' "$row" | cut -f6)"
  assert_file_not_exists "$FIXTURES/run/pages/warp/docs-1-docs.warp.dev-agents-capabilities-rules.md.txt"
}

# ---- lock completeness ------------------------------------------------------------

# augment_2026_10_29_sources is the augment registry after PR #2757 added the
# stable /cli/v0/ pages, while the 2026-10-29 run had fetched only the rest.
function augment_2026_10_29_sources() {
  local page
  printf '%s\n' '- docs: https://docs.augmentcode.com/setup-augment/guidelines.md'
  for page in subagents config rules skills hooks custom-commands integrations permissions; do
    printf '%s\n' "- docs: https://docs.augmentcode.com/cli/$page.md"
    printf '%s\n' "- docs: https://docs.augmentcode.com/cli/v0/$page.md"
  done
  printf '%s\n' '- changelog: https://www.augmentcode.com/changelog'
}

function test_update_rejects_a_run_missing_a_registered_url() {
  # shellcheck disable=SC2329
  function source_sections() { augment_2026_10_29_sources; }
  local run="$DOCFETCH_RUNS/2026-10-29-run" page err status=0
  mkdir -p "$run"
  {
    run_row augment https://docs.augmentcode.com/setup-augment/guidelines.md aaa 2026-10-29
    for page in subagents config rules skills hooks custom-commands integrations permissions; do
      run_row augment "https://docs.augmentcode.com/cli/$page.md" aaa 2026-10-29
    done
    printf 'augment\tchangelog\thttps://www.augmentcode.com/changelog\t200\thtml\taaa\t2026-10-29\tunchanged\thttps://www.augmentcode.com/changelog\tpages/x\n'
  } >"$run/docfetch.tsv"
  printf 'augment\n' >"$run/requested-targets.txt"
  write_lock https://vendor.example/old old
  local before
  before=$(cat "$LOCK")
  # shellcheck disable=SC2329
  function go() { printf 'called\n' >"$FIXTURES/go.called"; }
  err=$(docfetch_main --update "$run/docfetch.tsv" 2>&1 >/dev/null) || status=$?
  unset -f go
  assert_not_equals 0 "$status"
  assert_contains '8 registered URLs have no row' "$err"
  for page in subagents config rules skills hooks custom-commands integrations permissions; do
    assert_contains "augment	docs	https://docs.augmentcode.com/cli/v0/$page.md" "$err"
  done
  assert_not_contains 'cli/config.md' "$err"
  assert_file_not_exists "$FIXTURES/go.called"
  assert_equals "$before" "$(cat "$LOCK")"
}

function test_update_accepts_a_run_that_covers_every_registered_url() {
  # shellcheck disable=SC2329
  function source_sections() { augment_2026_10_29_sources; }
  local run="$DOCFETCH_RUNS/2026-10-29-run" kind url
  mkdir -p "$run"
  resolve_urls augment | while IFS=$'\t' read -r kind url; do
    printf 'augment\t%s\t%s\t200\thtml\taaa\t2026-10-29\tchanged\t%s\tpages/x\n' "$kind" "$url" "$url"
  done >"$run/docfetch.tsv"
  printf 'augment\n' >"$run/requested-targets.txt"
  # shellcheck disable=SC2329
  function go() { printf '%s\n' "$*" >"$FIXTURES/go.args"; }
  docfetch_main --update "$run/docfetch.tsv" >/dev/null 2>&1
  unset -f go
  assert_contains "finalize $run" "$(cat "$FIXTURES/go.args")"
}

function use_run_store() {
  export DOCFETCH_SNAPSHOTS="$DOCFETCH_RUNS/snapshots"
}

function page_kind() {
  if [ -L "$1" ]; then echo link; elif [ -f "$1" ]; then echo file; else echo missing; fi
}

function audit_run_pages() {
  local run="$DOCFETCH_RUNS/$1"
  mkdir -p "$run/pages/claude"
  printf '{}' >"$run/audit.json"
  printf '%s' "$2" >"$run/pages/claude/docs-1-hooks.body"
  printf 'delta\n' >"$run/pages/claude/docs-1-hooks.delta"
  printf '%s\n' "$run"
}

function test_pages_by_hash_stores_an_audit_run_page_once() {
  local first second
  use_run_store
  first=$(audit_run_pages run-a 'Hooks run before tools.')
  second=$(audit_run_pages run-b 'Hooks run before tools.')
  pages_by_hash "$first"
  pages_by_hash "$second"
  assert_equals link "$(page_kind "$second/pages/claude/docs-1-hooks.body")"
  assert_equals 'Hooks run before tools.' "$(cat "$second/pages/claude/docs-1-hooks.body")"
  assert_equals 1 "$(find "$DOCFETCH_SNAPSHOTS/blobs" -type f | grep -c .)"
  assert_equals "$(readlink "$first/pages/claude/docs-1-hooks.body")" "$(readlink "$second/pages/claude/docs-1-hooks.body")"
  assert_equals "$(sha256_of "$second/pages/claude/docs-1-hooks.body")" \
    "$(basename "$(readlink "$second/pages/claude/docs-1-hooks.body")")"
}

function test_pages_by_hash_keeps_deltas_in_the_run() {
  local run
  use_run_store
  run=$(audit_run_pages run-a 'Hooks run before tools.')
  pages_by_hash "$run"
  assert_equals file "$(page_kind "$run/pages/claude/docs-1-hooks.delta")"
}

function test_pages_by_hash_leaves_a_fetch_only_run_as_files() {
  local run
  use_run_store
  run=$(audit_run_pages watch 'Hooks run before tools.')
  rm "$run/audit.json"
  pages_by_hash "$run"
  assert_equals file "$(page_kind "$run/pages/claude/docs-1-hooks.body")"
  assert_directory_not_exists "$DOCFETCH_SNAPSHOTS/blobs"
}

function test_pages_by_hash_leaves_a_run_outside_the_runs_root_as_files() {
  local run="$FIXTURES/elsewhere"
  use_run_store
  mkdir -p "$run/pages/claude" "$DOCFETCH_RUNS"
  printf '{}' >"$run/audit.json"
  printf 'x' >"$run/pages/claude/docs-1-hooks.body"
  pages_by_hash "$run"
  assert_equals file "$(page_kind "$run/pages/claude/docs-1-hooks.body")"
}

function test_pages_by_hash_replaces_a_damaged_blob() {
  local run blob
  use_run_store
  run=$(audit_run_pages run-a 'Hooks run before tools.')
  mkdir -p "$DOCFETCH_SNAPSHOTS/blobs"
  blob="$DOCFETCH_SNAPSHOTS/blobs/$(sha256_of "$run/pages/claude/docs-1-hooks.body")"
  printf 'damaged' >"$blob"
  pages_by_hash "$run"
  assert_equals 'Hooks run before tools.' "$(cat "$run/pages/claude/docs-1-hooks.body")"
}

function test_blob_prune_removes_only_old_unlinked_blobs() {
  local run linked
  use_run_store
  run=$(audit_run_pages run-a 'Hooks run before tools.')
  pages_by_hash "$run"
  linked="$DOCFETCH_SNAPSHOTS/blobs/$(basename "$(readlink "$run/pages/claude/docs-1-hooks.body")")"
  printf 'old' >"$DOCFETCH_SNAPSHOTS/blobs/old"
  printf 'new' >"$DOCFETCH_SNAPSHOTS/blobs/new"
  touch -t 202001010000 "$DOCFETCH_SNAPSHOTS/blobs/old" "$linked"
  blob_prune
  assert_file_exists "$linked"
  assert_file_exists "$DOCFETCH_SNAPSHOTS/blobs/new"
  assert_file_not_exists "$DOCFETCH_SNAPSHOTS/blobs/old"
}

function test_a_refetch_into_an_audit_run_leaves_the_shared_page_alone() {
  local first second stem="pages/claude/docs-1-x.example-a.md"
  use_run_store
  stub_curl "https://x.example/a.md|200|$(printf 'first markdown %.0s' $(seq 1 40))"
  first="$DOCFETCH_RUNS/run-a"
  second="$DOCFETCH_RUNS/run-b"
  mkdir -p "$first" "$second"
  printf '{}' >"$first/audit.json"
  printf '{}' >"$second/audit.json"
  fetch_one claude docs https://x.example/a.md "$first" 1 >/dev/null
  fetch_one claude docs https://x.example/a.md "$second" 1 >/dev/null
  pages_by_hash "$first"
  pages_by_hash "$second"
  stub_curl "https://x.example/a.md|200|$(printf 'second markdown %.0s' $(seq 1 40))"
  fetch_one claude docs https://x.example/a.md "$first" 1 >/dev/null
  assert_contains 'first markdown' "$(cat "$second/$stem.body")"
  assert_contains 'second markdown' "$(cat "$first/$stem.body")"
  assert_equals "$(basename "$(readlink "$second/$stem.body")")" "$(sha256_of "$second/$stem.body")"
}

function test_pages_by_hash_links_relative_to_the_runs_root() {
  local run
  use_run_store
  run=$(audit_run_pages run-a 'Hooks run before tools.')
  pages_by_hash "$run"
  assert_equals "../../../snapshots/blobs/$(sha256_of "$run/pages/claude/docs-1-hooks.body")" \
    "$(readlink "$run/pages/claude/docs-1-hooks.body")"
  mv "$FIXTURES/runs" "$FIXTURES/moved"
  assert_equals 'Hooks run before tools.' "$(cat "$FIXTURES/moved/run-a/pages/claude/docs-1-hooks.body")"
}

function test_blob_prune_keeps_linked_blobs_under_a_symlinked_runs_root() {
  local run linked
  use_run_store
  run=$(audit_run_pages run-a 'Hooks run before tools.')
  pages_by_hash "$run"
  linked="$DOCFETCH_SNAPSHOTS/blobs/$(basename "$(readlink "$run/pages/claude/docs-1-hooks.body")")"
  touch -t 202001010000 "$linked"
  ln -s "$DOCFETCH_RUNS" "$FIXTURES/runs-link"
  DOCFETCH_RUNS="$FIXTURES/runs-link" blob_prune
  assert_file_exists "$linked"
}

function test_pages_by_hash_leaves_files_when_the_store_is_elsewhere() {
  local run
  run=$(audit_run_pages run-a 'Hooks run before tools.')
  pages_by_hash "$run"
  assert_equals file "$(page_kind "$run/pages/claude/docs-1-hooks.body")"
}
