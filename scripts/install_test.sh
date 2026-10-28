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
#
# One gap worth naming: `curl` is stubbed as a shell function, so no test
# here can reproduce curl taking EPIPE from a `grep -m1` further down the
# pipe. That race broke an earlier `latest_version` that piped curl into
# grep, and it was found by running the real script, not by this suite.
# `latest_version` no longer pipes curl into anything, but the gap stays
# real for any future code that does: fetch first, then match.

SCRIPT_DIR="$(cd "$(dirname "$BASH_SOURCE")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/install.sh"

# A value exported in the developer's shell would change what these check.
function set_up() {
  unset AGNOSTIC_AI_VERIFY_ATTESTATION AGNOSTIC_AI_VERSION
}

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

function test_verify_checksum_fails_when_checksums_cannot_be_fetched() {
  local tmp asset
  tmp="$(mktemp -d)"
  asset="agnostic-ai_linux_amd64.tar.gz"
  printf 'payload\n' > "$tmp/$asset"

  function curl() { return 22; }
  local out
  out="$(verify_checksum "$tmp/$asset" "$asset" v0.45.0 2>&1)" && fail "an unverifiable archive installed"
  assert_contains "cannot be verified" "$out"
  unset -f curl

  rm -rf "$tmp"
}

# ---- verify_attestation ------------------------------------------------------

ATTESTED_ARGS="attestation verify /tmp/archive --hostname github.com --repo Chemaclass/agnostic-ai --signer-workflow Chemaclass/agnostic-ai/.github/workflows/release.yml --source-ref refs/tags/v0.90.0 --deny-self-hosted-runners"

function test_verify_attestation_checks_by_default_when_gh_is_present() {
  local out code
  # shellcheck disable=SC2317
  function gh() { [[ "$*" == "$ATTESTED_ARGS" ]]; }
  code=0
  out="$(AGNOSTIC_AI_VERIFY_ATTESTATION='' verify_attestation /tmp/archive v0.90.0 2>&1)" || code=$?
  assert_same "0" "$code"
  unset -f gh
  assert_contains "build provenance verified" "$out"
}

function test_verify_attestation_fails_when_gh_rejects_it() {
  local out code
  # shellcheck disable=SC2317
  function gh() { return 1; }
  code=0
  out="$(verify_attestation /tmp/archive v0.90.0 2>&1)" || code=$?
  assert_not_same "0" "$code"
  unset -f gh
  assert_contains "did not verify for archive" "$out"
}

function test_verify_attestation_notes_and_continues_without_gh() {
  local out code
  code=0
  out="$(PATH=/nonexistent verify_attestation /tmp/archive v0.90.0 2>&1)" || code=$?
  assert_same "0" "$code"
  assert_contains "build provenance not checked: install the GitHub CLI (gh)" "$out"
}

function test_verify_attestation_notes_and_continues_when_gh_is_logged_out() {
  local out code
  # shellcheck disable=SC2317
  function gh() { return 4; }
  code=0
  out="$(verify_attestation /tmp/archive v0.90.0 2>&1)" || code=$?
  assert_same "0" "$code"
  unset -f gh
  assert_contains "gh is not logged in" "$out"
}

function test_verify_attestation_forced_fails_without_gh() {
  local out code
  code=0
  out="$(PATH=/nonexistent AGNOSTIC_AI_VERIFY_ATTESTATION=1 verify_attestation /tmp/archive v0.90.0 2>&1)" || code=$?
  assert_not_same "0" "$code"
  assert_contains "needs the GitHub CLI" "$out"
}

function test_verify_attestation_forced_fails_when_gh_is_logged_out() {
  local out code
  # shellcheck disable=SC2317
  function gh() { return 4; }
  code=0
  out="$(AGNOSTIC_AI_VERIFY_ATTESTATION=1 verify_attestation /tmp/archive v0.90.0 2>&1)" || code=$?
  assert_not_same "0" "$code"
  unset -f gh
  assert_contains "did not verify" "$out"
}

function test_verify_attestation_skips_releases_before_provenance_existed() {
  local out code
  # shellcheck disable=SC2317
  function gh() { echo "gh called" >&2; return 1; }
  code=0
  out="$(verify_attestation /tmp/archive v0.74.0 pinned 2>&1)" || code=$?
  assert_same "0" "$code"
  unset -f gh
  assert_not_contains "gh called" "$out"
  assert_contains "releases before v0.75.0" "$out"
}

function test_verify_attestation_refuses_a_latest_release_that_predates_provenance() {
  local out code
  code=0
  out="$(verify_attestation /tmp/archive v0.74.9 2>&1)" || code=$?
  assert_not_same "0" "$code"
  assert_contains "the latest release v0.74.9 predates build provenance" "$out"
  assert_contains "AGNOSTIC_AI_VERSION" "$out"
}

function test_verify_attestation_is_skipped_when_turned_off() {
  local out code
  # shellcheck disable=SC2317
  function gh() { echo "gh called" >&2; return 1; }
  code=0
  out="$(AGNOSTIC_AI_VERIFY_ATTESTATION=0 verify_attestation /tmp/archive v0.74.9 2>&1)" || code=$?
  unset -f gh
  assert_same "0" "$code"
  assert_not_contains "gh called" "$out"
  assert_contains "AGNOSTIC_AI_VERIFY_ATTESTATION=0" "$out"
}

function test_verify_attestation_notes_a_gh_too_old_to_verify() {
  local out code
  # shellcheck disable=SC2317
  function gh() { echo "unknown flag: --source-ref" >&2; return 1; }
  code=0
  out="$(verify_attestation /tmp/archive v0.90.0 2>&1)" || code=$?
  unset -f gh
  assert_same "0" "$code"
  assert_contains "update gh" "$out"
}

# ---- release_tag -------------------------------------------------------------

function test_release_tag_accepts_a_release_with_or_without_v() {
  assert_same "v0.90.0" "$(release_tag 0.90.0)"
  assert_same "v0.90.0" "$(release_tag v0.90.0)"
  assert_same "v1.2.3-rc.1" "$(release_tag v1.2.3-rc.1)"
}

function test_release_tag_rejects_anything_else() {
  local input out code
  for input in 'v1.0.0/../../evil' 'latest' '1.0' 'v1.0.0 ' 'v1.0.0?x=1' ''; do
    code=0
    out="$(release_tag "$input" 2>&1)" || code=$?
    assert_not_same "0" "$code"
    assert_contains "is not a release version" "$out"
  done
}

function test_main_rejects_an_invalid_version_before_any_download() {
  local tmp out code
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2317
  function curl() { touch "$tmp/curl-called"; return 1; }
  code=0
  out="$(AGNOSTIC_AI_INSTALL_DIR="$tmp/bin" AGNOSTIC_AI_VERSION='v1.0.0/../../evil' main 2>&1)" || code=$?
  assert_not_same "0" "$code"
  unset -f curl
  assert_contains "is not a release version" "$out"
  assert_file_not_exists "$tmp/curl-called"
  rm -rf "$tmp"
}

# ---- latest_version ----------------------------------------------------------
#
# latest_version reads the tag out of a 302 Location, so every stub below
# prints what `curl -w '%{http_code} %{redirect_url}'` writes: the status,
# a space, then the redirect target (empty when there is none).

function test_latest_version_reads_the_tag_from_the_redirect() {
  function curl() { printf '302 https://github.com/Chemaclass/agnostic-ai/releases/tag/v0.62.0'; }
  assert_equals "v0.62.0" "$(latest_version)"
  unset -f curl
}

function test_latest_version_asks_github_com_not_the_rate_limited_api() {
  local seen
  # stdout carries the stubbed response, so the URL under test goes to stderr.
  function curl() {
    printf '%s\n' "${@: -1}" >&2
    printf '302 https://github.com/Chemaclass/agnostic-ai/releases/tag/v1.2.3'
  }
  seen="$(latest_version 2>&1 >/dev/null)"
  unset -f curl

  assert_same "https://github.com/Chemaclass/agnostic-ai/releases/latest" "$seen"
}

function test_latest_version_names_rate_limiting_instead_of_printing_403() {
  function curl() { printf '403 '; }
  local out
  out="$(latest_version 2>&1)"
  unset -f curl

  assert_contains "rate limit" "$out"
  assert_not_contains "no published release" "$out"
}

function test_latest_version_names_rate_limiting_on_429() {
  function curl() { printf '429 '; }
  assert_contains "rate limit" "$(latest_version 2>&1)"
  unset -f curl
}

function test_latest_version_names_an_absent_release_on_404() {
  function curl() { printf '404 '; }
  local out
  out="$(latest_version 2>&1)"
  unset -f curl

  assert_contains "no published release" "$out"
  assert_not_contains "rate limit" "$out"
}

function test_latest_version_rejects_a_redirect_that_is_not_a_tag_page() {
  function curl() { printf '302 https://github.com/Chemaclass/agnostic-ai/releases'; }
  assert_contains "no published release" "$(latest_version 2>&1)"
  unset -f curl
}

function test_latest_version_reports_an_unexpected_status_with_its_code() {
  function curl() { printf '500 '; }
  assert_contains "HTTP 500" "$(latest_version 2>&1)"
  unset -f curl
}

function test_latest_version_reports_a_transport_failure() {
  function curl() { return 6; }
  assert_contains "could not reach" "$(latest_version 2>&1)"
  unset -f curl
}

# The API endpoint is the bug: 60 unauthenticated requests per hour per IP.
function test_latest_version_never_touches_the_github_api() {
  assert_not_contains "api.github.com" "$(declare -f latest_version)"
}

# ---- install.ps1 -------------------------------------------------------------
#
# The Windows installer is exercised for real by .github/workflows/install.yml
# on windows-latest under `powershell` (5.1). These are text assertions, not a
# second harness: they run on Linux alongside the rest and stop the API
# endpoint from creeping back in unnoticed between Windows runs.

# Comments are stripped: they discuss the endpoint that was removed and the
# cmdlet that was replaced, so matching them would defeat the guard.
function ps1_code() {
  grep -v '^[[:space:]]*#' "$SCRIPT_DIR/install.ps1"
}

function test_ps1_resolves_the_latest_release_without_the_github_api() {
  assert_not_contains "api.github.com" "$(ps1_code)"
  assert_contains 'https://github.com/$repo/releases/latest' "$(ps1_code)"
}

function test_ps1_disables_redirect_following_so_the_location_survives() {
  # Invoke-RestMethod follows redirects and drops the header we came for.
  assert_contains 'AllowAutoRedirect = $false' "$(ps1_code)"
  assert_not_contains "Invoke-RestMethod" "$(ps1_code)"
}

function test_ps1_names_rate_limiting_rather_than_printing_a_status() {
  assert_contains "rate limited" "$(ps1_code)"
  assert_contains "no published release" "$(ps1_code)"
}

function test_ps1_validates_the_version_before_any_download() {
  local code
  code="$(ps1_code)"
  assert_contains "'^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?\\z'" "$code"
  assert_contains '$version -cnotmatch $versionPattern' "$code"
  local check download
  check="$(grep -n '^\$tag = Get-ReleaseTag' <<<"$code" | head -1 | cut -d: -f1)"
  download="$(grep -n 'Invoke-WebRequest -Uri (Get-DownloadUrl \$tag \$asset)' <<<"$code" | head -1 | cut -d: -f1)"
  assert_not_empty "$download"
  assert_not_empty "$check"
  assert_less_than "$download" "$check"
  assert_contains 'throw "'"'"'$version'"'"' is not a release version' "$code"
}

function test_ps1_verifies_provenance_by_default_pinned_to_the_release_workflow() {
  local code
  code="$(ps1_code)"
  assert_not_contains 'if (-not $VerifyAttestation) { return }' "$code"
  assert_contains '--signer-workflow "$repo/.github/workflows/release.yml"' "$code"
  assert_contains '--source-ref "refs/tags/$tag"' "$code"
  assert_contains '--hostname github.com' "$code"
  assert_contains '$code -eq 4' "$code"
  assert_contains "unknown (flag|command)" "$code"
  assert_contains "AGNOSTIC_AI_VERIFY_ATTESTATION -eq '0'" "$code"
  assert_contains 'Test-Attestation $archive $tag ($Version -ne '"'latest'"')' "$code"
  assert_contains 'the latest release $tag predates build provenance' "$code"
}

function test_ps1_matches_the_archive_line_exactly() {
  assert_contains "-Pattern ('\\s' + [regex]::Escape(\$asset) + '\$')" "$(ps1_code)"
}

function test_released_install_checks_the_exact_binary_version() {
  local fixture check code report
  fixture=$(mktemp -d)
  # shellcheck disable=SC2016
  printf '#!/bin/sh\nprintf "%%s\\n" "$BINARY_REPORT"\n' > "$fixture/agnostic-ai"
  chmod +x "$fixture/agnostic-ai"
  check=$(awk '
    /- name: Binary reports the published version/ { found = 1; next }
    found && /run: \|/ { body = 1; next }
    body && /^          / { sub(/^          /, ""); print; next }
    body { exit }
  ' "$SCRIPT_DIR/../.github/workflows/install.yml")
  assert_not_empty "$check"
  for report in 'agnostic-ai version 1.2.30' 'agnostic-ai version 11.2.3' 'agnostic-ai version 1.2.3-extra' 'unexpected 1.2.3'; do
    code=0
    PATH="$fixture:$PATH" BINARY_REPORT="$report" NPM_RELEASE_VERSION=1.2.3 bash -c "$check" >/dev/null 2>&1 || code=$?
    assert_not_equals 0 "$code"
  done
  code=0
  PATH="$fixture:$PATH" BINARY_REPORT='agnostic-ai version 1.2.3' NPM_RELEASE_VERSION=1.2.3 bash -c "$check" >/dev/null 2>&1 || code=$?
  assert_equals 0 "$code"
  code=0
  PATH="$fixture:$PATH" BINARY_REPORT='agnostic-ai version 1.2.3-rc.1' NPM_RELEASE_VERSION=1.2.3-rc.1 bash -c "$check" >/dev/null 2>&1 || code=$?
  assert_equals 0 "$code"
  rm -rf "$fixture"
}
