#!/usr/bin/env bash
#
# bashunit tests for scripts/npm-binaries.sh
#
# Run:
#   bashunit scripts/npm-binaries_test.sh
#
# The script runs once per tag and nothing downstream rechecks it, so a defect
# here first shows up as a broken npm install. `gh` is stubbed as a shell
# function and the archives are built locally, which covers everything except
# the download itself: naming, checksum verification, unpacking, and the
# refusal to hand a partial set to the generator.

SCRIPT_DIR="$(cd "$(dirname "$BASH_SOURCE")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/npm-binaries.sh"

# ---- naming ------------------------------------------------------------------

function test_archive_name_matches_the_release_assets() {
  assert_same "agnostic-ai_darwin_arm64.tar.gz" "$(archive_name darwin_arm64)"
  assert_same "agnostic-ai_linux_amd64.tar.gz" "$(archive_name linux_amd64)"
  assert_same "agnostic-ai_windows_amd64.zip" "$(archive_name windows_amd64)"
}

function test_only_windows_binaries_carry_the_exe_suffix() {
  assert_same "agnostic-ai" "$(binary_name linux_arm64)"
  assert_same "agnostic-ai" "$(binary_name darwin_amd64)"
  assert_same "agnostic-ai.exe" "$(binary_name windows_arm64)"
}

function test_it_covers_the_six_targets_the_npm_packages_need() {
  assert_same "6" "${#TARGETS[@]}"
  assert_contains "darwin_arm64" "${TARGETS[*]}"
  assert_contains "windows_arm64" "${TARGETS[*]}"
}

# ---- fixtures ----------------------------------------------------------------

# Builds real archives with real digests in $1, the way a release looks to
# `gh release download`. Pass a target as $2 to leave that archive out.
function fake_release() {
  local dir="$1" skip="${2:-}" target stage
  mkdir -p "$dir"
  stage="$dir/stage"
  for target in "${TARGETS[@]}"; do
    [[ "$target" == "$skip" ]] && continue
    rm -rf "$stage"
    mkdir -p "$stage"
    printf 'binary for %s\n' "$target" > "$stage/$(binary_name "$target")"
    if [[ "$target" == windows_* ]]; then
      (cd "$stage" && zip -q "$dir/$(archive_name "$target")" "$(binary_name "$target")")
    else
      tar -czf "$dir/$(archive_name "$target")" -C "$stage" "$(binary_name "$target")"
    fi
  done
  rm -rf "$stage"
  (cd "$dir" && shasum -a 256 ./*.tar.gz ./*.zip | sed 's| \./| |' > checksums.txt)
}

# Stubs `gh release download` with a copy out of the fixture directory. The
# directory goes in a global: the stub body is expanded when gh is called, by
# which time a local in this function is long gone.
function stub_gh() {
  STUB_RELEASE_DIR="$1"
  # shellcheck disable=SC2317
  function gh() {
    local dir=""
    while [[ $# -gt 0 ]]; do
      [[ "$1" == "--dir" ]] && dir="$2"
      shift
    done
    cp "$STUB_RELEASE_DIR"/* "$dir/"
  }
}

# ---- verify_checksums --------------------------------------------------------

function test_verify_checksums_accepts_the_published_digests() {
  local tmp code
  tmp="$(mktemp -d)"
  fake_release "$tmp"
  code="$(verify_checksums "$tmp" >/dev/null 2>&1; echo $?)"
  rm -rf "$tmp"

  assert_same "0" "$code"
}

function test_verify_checksums_rejects_a_tampered_archive() {
  local tmp code
  tmp="$(mktemp -d)"
  fake_release "$tmp"
  printf 'tampered' >> "$tmp/agnostic-ai_linux_amd64.tar.gz"
  code="$(verify_checksums "$tmp" >/dev/null 2>&1; echo $?)"
  rm -rf "$tmp"

  assert_not_same "0" "$code"
}

function test_verify_checksums_fails_when_an_archive_is_not_listed() {
  local tmp out
  tmp="$(mktemp -d)"
  fake_release "$tmp"
  grep -v 'linux_amd64' "$tmp/checksums.txt" > "$tmp/trimmed"
  mv "$tmp/trimmed" "$tmp/checksums.txt"
  out="$(verify_checksums "$tmp" 2>&1 || true)"
  rm -rf "$tmp"

  assert_contains "no entry for agnostic-ai_linux_amd64.tar.gz" "$out"
}

# ---- unpack ------------------------------------------------------------------

function test_unpack_lays_out_one_directory_per_target() {
  local tmp
  tmp="$(mktemp -d)"
  fake_release "$tmp/release"
  unpack "$tmp/release" "$tmp/out"

  assert_file_exists "$tmp/out/darwin_arm64/agnostic-ai"
  assert_file_exists "$tmp/out/linux_amd64/agnostic-ai"
  assert_file_exists "$tmp/out/windows_amd64/agnostic-ai.exe"
  rm -rf "$tmp"
}

function test_unpack_puts_each_binary_under_its_own_target() {
  local tmp linux windows
  tmp="$(mktemp -d)"
  fake_release "$tmp/release"
  unpack "$tmp/release" "$tmp/out"
  linux="$(cat "$tmp/out/linux_arm64/agnostic-ai")"
  windows="$(cat "$tmp/out/windows_arm64/agnostic-ai.exe")"
  rm -rf "$tmp"

  assert_same "binary for linux_arm64" "$linux"
  assert_same "binary for windows_arm64" "$windows"
}

function test_unpack_leaves_every_binary_executable() {
  local tmp code
  tmp="$(mktemp -d)"
  fake_release "$tmp/release"
  unpack "$tmp/release" "$tmp/out"
  code="$([[ -x "$tmp/out/darwin_amd64/agnostic-ai" ]]; echo $?)"
  rm -rf "$tmp"

  assert_same "0" "$code"
}

# ---- require_every_binary ----------------------------------------------------

function test_require_every_binary_passes_on_a_complete_set() {
  local tmp code
  tmp="$(mktemp -d)"
  fake_release "$tmp/release"
  unpack "$tmp/release" "$tmp/out"
  code="$(require_every_binary "$tmp/out" 2>/dev/null; echo $?)"
  rm -rf "$tmp"

  assert_same "0" "$code"
}

function test_require_every_binary_rejects_a_missing_target() {
  local tmp out
  tmp="$(mktemp -d)"
  fake_release "$tmp/release"
  unpack "$tmp/release" "$tmp/out"
  rm -rf "${tmp:?}/out/windows_arm64"
  out="$(require_every_binary "$tmp/out" 2>&1 || true)"
  rm -rf "$tmp"

  assert_contains "no binary for windows_arm64" "$out"
}

function test_require_every_binary_rejects_an_empty_binary() {
  local tmp code
  tmp="$(mktemp -d)"
  fake_release "$tmp/release"
  unpack "$tmp/release" "$tmp/out"
  : > "$tmp/out/linux_amd64/agnostic-ai"
  code="$(require_every_binary "$tmp/out" 2>/dev/null; echo $?)"
  rm -rf "$tmp"

  assert_not_same "0" "$code"
}

# ---- main --------------------------------------------------------------------

function test_main_downloads_verifies_and_unpacks_every_target() {
  local tmp content
  tmp="$(mktemp -d)"
  fake_release "$tmp/release"
  stub_gh "$tmp/release"
  main v9.9.9 "$tmp/out" >/dev/null
  unset -f gh
  content="$(cat "$tmp/out/darwin_arm64/agnostic-ai")"

  assert_file_exists "$tmp/out/windows_amd64/agnostic-ai.exe"
  assert_same "binary for darwin_arm64" "$content"
  rm -rf "$tmp"
}

function test_main_fails_when_the_release_is_missing_an_archive() {
  local tmp code
  tmp="$(mktemp -d)"
  fake_release "$tmp/release" windows_arm64
  stub_gh "$tmp/release"
  code="$(main v9.9.9 "$tmp/out" >/dev/null 2>&1; echo $?)"
  unset -f gh
  rm -rf "$tmp"

  assert_not_same "0" "$code"
}

function test_main_needs_a_tag_and_a_destination() {
  assert_same "2" "$(main >/dev/null 2>&1; echo $?)"
  assert_same "2" "$(main v9.9.9 >/dev/null 2>&1; echo $?)"
}
