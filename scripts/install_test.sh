#!/usr/bin/env bash
#
# bashunit tests for scripts/install.sh
#
# Run:
#   bashunit scripts/install_test.sh
#
# Pure helpers (os/arch detection, asset naming, URL building, install-dir
# resolution) are tested by sourcing the script. The download path is not
# exercised here: it needs the network and a published release.

SCRIPT_DIR="$(cd "$(dirname "$BASH_SOURCE")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/install.sh"

# ---- asset_name / download_url -----------------------------------------------

function test_asset_name_uses_targz_per_os_arch() {
  assert_same "agnostic-ai_darwin_arm64.tar.gz" "$(asset_name darwin arm64)"
  assert_same "agnostic-ai_linux_amd64.tar.gz" "$(asset_name linux amd64)"
}

function test_download_url_points_at_the_tagged_release() {
  assert_same \
    "https://github.com/Chemaclass/agnostic-ai/releases/download/v0.45.0/agnostic-ai_linux_amd64.tar.gz" \
    "$(download_url v0.45.0 agnostic-ai_linux_amd64.tar.gz)"
}

# ---- detect_arch -------------------------------------------------------------

function test_detect_arch_maps_x86_64_to_amd64() {
  function uname() { printf 'x86_64\n'; }
  assert_same "amd64" "$(detect_arch)"
  unset -f uname
}

function test_detect_arch_maps_aarch64_to_arm64() {
  function uname() { printf 'aarch64\n'; }
  assert_same "arm64" "$(detect_arch)"
  unset -f uname
}

function test_detect_arch_rejects_32_bit() {
  function uname() { printf 'i686\n'; }
  assert_general_error "$(detect_arch 2>/dev/null)"
  unset -f uname
}

# ---- detect_os ---------------------------------------------------------------

function test_detect_os_maps_darwin() {
  function uname() { printf 'Darwin\n'; }
  assert_same "darwin" "$(detect_os)"
  unset -f uname
}

function test_detect_os_points_windows_at_the_powershell_script() {
  function uname() { printf 'MINGW64_NT-10.0\n'; }
  assert_contains "install.ps1" "$(detect_os 2>&1)"
  unset -f uname
}

# ---- resolve_install_dir -----------------------------------------------------

function test_resolve_install_dir_prefers_the_env_override() {
  local dir
  dir="$(AGNOSTIC_AI_INSTALL_DIR=/opt/tools resolve_install_dir)"
  assert_same "/opt/tools" "$dir"
}

function test_resolve_install_dir_falls_back_to_local_bin() {
  # Only meaningful where /usr/local/bin is not writable; skip otherwise so the
  # test does not silently assert nothing on a permissive machine.
  if [[ -w /usr/local/bin ]]; then
    assert_same "/usr/local/bin" "$(resolve_install_dir)"
  else
    assert_same "$HOME/.local/bin" "$(resolve_install_dir)"
  fi
}

# ---- verify_checksum ---------------------------------------------------------

function test_verify_checksum_fails_on_mismatch() {
  local tmp asset
  tmp="$(mktemp -d)"
  asset="agnostic-ai_linux_amd64.tar.gz"
  printf 'payload\n' > "$tmp/$asset"
  printf '%s  %s\n' "0000000000000000000000000000000000000000000000000000000000000000" "$asset" \
    > "$tmp/checksums.txt"

  # Stub the fetch so the local checksums.txt above is what gets compared.
  function curl() { return 0; }
  assert_contains "checksum mismatch" "$(verify_checksum "$tmp/$asset" "$asset" v0.45.0 2>&1)"
  unset -f curl

  rm -rf "$tmp"
}

function test_verify_checksum_accepts_a_matching_digest() {
  local tmp asset sum
  tmp="$(mktemp -d)"
  asset="agnostic-ai_linux_amd64.tar.gz"
  printf 'payload\n' > "$tmp/$asset"

  if command -v sha256sum >/dev/null 2>&1; then
    sum="$(sha256sum "$tmp/$asset" | awk '{print $1}')"
  else
    sum="$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')"
  fi
  printf '%s  %s\n' "$sum" "$asset" > "$tmp/checksums.txt"

  function curl() { return 0; }
  assert_contains "checksum verified" "$(verify_checksum "$tmp/$asset" "$asset" v0.45.0 2>&1)"
  unset -f curl

  rm -rf "$tmp"
}
