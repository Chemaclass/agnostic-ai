#!/usr/bin/env bash
#
# bashunit tests for scripts/release-verify.sh. Fake gh and curl commands on
# PATH stand in for GitHub and the site.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERIFY="$SCRIPT_DIR/release-verify.sh"

function set_up() {
  dir="$(mktemp -d)"
  mkdir -p "$dir/bin" "$dir/root/docs/site/content/updates" "$dir/assets/pkg"
  : > "$dir/root/docs/site/content/updates/2026-12-01-v1.11.0.md"

  case "$(uname -s)" in Darwin) os=darwin ;; *) os=linux ;; esac
  case "$(uname -m)" in x86_64 | amd64) arch=amd64 ;; *) arch=arm64 ;; esac
  archive="agnostic-ai_${os}_${arch}.tar.gz"
  write_binary "1.11.0"
  printf 'Notes.\n' > "$dir/assets/body"
  printf '200' > "$dir/assets/http_code"

  cat > "$dir/bin/gh" <<EOF
#!/usr/bin/env bash
assets="$dir/assets"
if [ "\$1 \$2" = "release download" ]; then
  out=""
  while [ \$# -gt 0 ]; do
    [ "\$1" = --dir ] && out="\$2"
    shift
  done
  cp "\$assets/$archive" "\$assets/checksums.txt" "\$out/"
  exit 0
fi
if [ "\$1 \$2" = "release view" ]; then
  cat "\$assets/body"
  exit 0
fi
exit 1
EOF
  cat > "$dir/bin/curl" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" > "$dir/assets/curl_args"
cat "$dir/assets/http_code"
EOF
  chmod +x "$dir/bin/gh" "$dir/bin/curl"
}

function tear_down() {
  rm -rf "$dir"
}

# write_binary packs a fake binary that prints the given version.
function write_binary() {
  printf '#!/bin/sh\necho "agnostic-ai version %s"\n' "$1" > "$dir/assets/pkg/agnostic-ai"
  chmod +x "$dir/assets/pkg/agnostic-ai"
  tar -czf "$dir/assets/$archive" -C "$dir/assets/pkg" agnostic-ai
  local sum
  if command -v sha256sum >/dev/null 2>&1; then
    sum="$(sha256sum "$dir/assets/$archive" | awk '{print $1}')"
  else
    sum="$(shasum -a 256 "$dir/assets/$archive" | awk '{print $1}')"
  fi
  printf '%s  %s\n' "$sum" "$archive" > "$dir/assets/checksums.txt"
}

function verify() {
  PATH="$dir/bin:$PATH" RELEASE_ROOT="$dir/root" RELEASE_SITE_URL="https://site.test" "$VERIFY" "$@"
}

function test_passes_a_good_release() {
  local out
  out="$(verify v1.11.0 2>&1)"
  assert_successful_code "$?"
  assert_contains "ok   checksum $archive" "$out"
  assert_contains "ok   version: agnostic-ai version 1.11.0" "$out"
  assert_contains "ok   release notes are not empty" "$out"
  assert_contains "ok   briefing https://site.test/updates/2026-12-01-v1.11.0/" "$out"
  assert_contains "https://site.test/updates/2026-12-01-v1.11.0/" "$(cat "$dir/assets/curl_args")"
}

function test_fails_on_a_checksum_mismatch() {
  printf '%s  %s\n' "0000" "$archive" > "$dir/assets/checksums.txt"
  local out
  out="$(verify v1.11.0 2>&1)"
  assert_general_error "$?"
  assert_contains "FAIL checksum $archive" "$out"
}

function test_fails_on_another_version() {
  write_binary "1.10.0"
  local out
  out="$(verify v1.11.0 2>&1)"
  assert_general_error "$?"
  assert_contains "FAIL version: got 'agnostic-ai version 1.10.0', want 1.11.0" "$out"
}

function test_fails_on_empty_release_notes() {
  printf '  \n' > "$dir/assets/body"
  local out
  out="$(verify v1.11.0 2>&1)"
  assert_general_error "$?"
  assert_contains "FAIL release v1.11.0 has no notes" "$out"
}

function test_fails_when_the_briefing_is_not_published() {
  printf '404' > "$dir/assets/http_code"
  local out
  out="$(verify v1.11.0 2>&1)"
  assert_general_error "$?"
  assert_contains "answered 404" "$out"
}

function test_fails_without_a_briefing_file() {
  rm "$dir/root/docs/site/content/updates/2026-12-01-v1.11.0.md"
  local out
  out="$(verify v1.11.0 2>&1)"
  assert_general_error "$?"
  assert_contains "FAIL no briefing" "$out"
}

function test_rejects_a_bad_version() {
  verify 1.11.0 >/dev/null 2>&1
  assert_exit_code 2
}
