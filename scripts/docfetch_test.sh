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
  assert_equals "https://api.github.com/repos/cline/cline/releases?per_page=10	github-api" \
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
  local modes
  modes=$(fetch_target kiro "$FIXTURES/run" | cut -f5 | sort -u)
  assert_equals "reader-proxy" "$modes"
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

function test_fetch_one_hashes_only_release_tags_and_dates() {
  # The releases API answers on one line and carries download counters that
  # move without a release. Only the tags and dates may reach the hash.
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
  printf '# whitespace\n@@ -1 +1 @@\n-  b: 1\n+b: 1\n' >"$FIXTURES/d"
  assert_equals "whitespace-only" "$(delta_label "$FIXTURES/d" "$FIXTURES/vocab")"
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

function test_delta_label_names_a_path_we_write() {
  printf '.cursor/BUGBOT.md\nalwaysApply\n' >"$FIXTURES/vocab"
  printf 'Each [-rule-]{+.cursor/BUGBOT.md file+} is truncated\n' >"$FIXTURES/d"
  assert_equals "mentions:.cursor/BUGBOT.md" "$(delta_label "$FIXTURES/d" "$FIXTURES/vocab")"
}

function test_delta_label_ignores_a_vocabulary_hit_in_context_only() {
  printf '.cursor/BUGBOT.md\n' >"$FIXTURES/vocab"
  printf 'Create .cursor/BUGBOT.md files [-now-]{+today+}\n' >"$FIXTURES/d"
  assert_equals "prose" "$(delta_label "$FIXTURES/d" "$FIXTURES/vocab")"
}

function test_delta_label_calls_known_page_chrome_chrome() {
  : >"$FIXTURES/vocab"
  printf 'Search... [-Ask Assistant \xe2\x8c\x98 I-] Navigation\n' >"$FIXTURES/d"
  printf 'Dismiss {+[](https://kiro.dev/) K+} Changelog\n' >>"$FIXTURES/d"
  printf 'faster. {+Loading diagram...+} Instead\n' >>"$FIXTURES/d"
  assert_equals "chrome-only" "$(delta_label "$FIXTURES/d" "$FIXTURES/vocab")"
}

function test_delta_label_keeps_prose_that_sits_beside_chrome() {
  : >"$FIXTURES/vocab"
  printf 'x {+Loading diagram... Rules now need a trigger key+} y\n' >"$FIXTURES/d"
  assert_equals "prose" "$(delta_label "$FIXTURES/d" "$FIXTURES/vocab")"
}

function test_delta_label_calls_an_empty_delta_whitespace_only() {
  : >"$FIXTURES/vocab"
  : >"$FIXTURES/d"
  assert_equals "whitespace-only" "$(delta_label "$FIXTURES/d" "$FIXTURES/vocab")"
}

# ---- delta_vocab -------------------------------------------------------------

function test_delta_vocab_keeps_paths_and_keys_but_not_plain_words() {
  local out
  out=$(delta_vocab cursor)
  assert_contains ".cursor/BUGBOT.md" "$out"
  assert_contains ".cursor/rules" "$out"
  assert_not_contains "$(printf '\nmodel\n')" "$(printf '\n%s\n' "$out")"
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

function test_update_saves_the_hashed_text_as_the_snapshot() {
  seed_run "$FIXTURES/run" https://cursor.com/docs/bugbot "Bugbot reads .cursor/BUGBOT.md"
  lock_merge "$FIXTURES/run/docfetch.tsv"
  assert_equals "Bugbot reads .cursor/BUGBOT.md" "$(cat "$(snapshot_path https://cursor.com/docs/bugbot)")"
}

function test_update_leaves_unselected_targets_snapshots_alone() {
  seed_run "$FIXTURES/run" https://cursor.com/docs/bugbot "new text"
  lock_merge "$FIXTURES/run/docfetch.tsv" claude
  assert_file_not_exists "$(snapshot_path https://cursor.com/docs/bugbot)"
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

function test_update_keeps_the_last_good_snapshot_over_an_app_shell() {
  seed_run "$FIXTURES/good" https://cursor.com/docs/bugbot "real page text"
  lock_merge "$FIXTURES/good/docfetch.tsv"
  seed_run "$FIXTURES/shell" https://cursor.com/docs/bugbot "<div id=app></div>"
  sed -i.bak -e 's/\thtml\t[0-9a-f]*\t/\tapp-shell\t-\t/' -e 's/\tchanged\t/\tfailed\t/' "$FIXTURES/shell/docfetch.tsv"
  lock_merge "$FIXTURES/shell/docfetch.tsv"
  assert_equals "real page text" "$(cat "$(snapshot_path https://cursor.com/docs/bugbot)")"
}

function test_word_delta_marks_a_capped_whitespace_diff_truncated() {
  local i
  for i in $(seq 1 300); do printf '  key%d: v\n' "$i"; done >"$FIXTURES/old.txt"
  for i in $(seq 1 300); do printf 'key%d: v\n' "$i"; done >"$FIXTURES/new.txt"
  word_delta "$FIXTURES/old.txt" "$FIXTURES/new.txt" >"$FIXTURES/d"
  assert_contains "# truncated: " "$(cat "$FIXTURES/d")"
  : >"$FIXTURES/vocab"
  assert_equals "whitespace-only:truncated" "$(delta_label "$FIXTURES/d" "$FIXTURES/vocab")"
}

function test_write_deltas_distrusts_a_snapshot_the_lock_does_not_match() {
  # The snapshot came from an older audit; the tracked lock moved since
  # (a pull, a branch switch). The vendor then reverts to the old text.
  seed_run "$FIXTURES/old" https://cursor.com/docs/bugbot "rules cap at 30,000"
  lock_merge "$FIXTURES/old/docfetch.tsv"
  seed_run "$FIXTURES/newer" https://cursor.com/docs/bugbot "rules cap at 40,000"
  awk -F '\t' -v OFS='\t' 'NR <= 2 { print; next } { $6 = "'"$(sha256_of "$FIXTURES/newer/pages/cursor/docs-1-x.txt")"'"; print }' \
    "$LOCK" >"$LOCK.new" && mv "$LOCK.new" "$LOCK"
  seed_run "$FIXTURES/run" https://cursor.com/docs/bugbot "rules cap at 30,000"
  write_deltas "$FIXTURES/run" >/dev/null
  assert_equals "no-snapshot" "$(cut -f4 "$FIXTURES/run/deltas.tsv")"
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
