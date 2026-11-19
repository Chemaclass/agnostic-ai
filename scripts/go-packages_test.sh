#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

function set_up() {
  dir="$(mktemp -d)"
  mkdir -p "$dir/bin"
  cat > "$dir/bin/go" <<'STUB'
#!/usr/bin/env bash
if [[ "${FAIL_LIST:-}" == 1 ]]; then exit 1; fi
if [[ "$2" == -m ]]; then
  printf '%s\n' example
  exit 0
fi
printf '%s\n' example/cmd example/internal/cli example/internal/clip \
  example/tests/integration example/tests/integration/fixture example/tests/integrationx
STUB
  chmod +x "$dir/bin/go"
}

function tear_down() {
  rm -rf "$dir"
}

function test_prints_every_package_without_arguments() {
  local out
  out="$(PATH="$dir/bin:$PATH" "$SCRIPT_DIR/go-packages.sh")"
  assert_successful_code "$?"
  assert_equals $'example/cmd\nexample/internal/cli\nexample/internal/clip\nexample/tests/integration\nexample/tests/integration/fixture\nexample/tests/integrationx' "$out"
}

function test_drops_exact_packages_and_keeps_their_sub_packages() {
  local out
  out="$(PATH="$dir/bin:$PATH" "$SCRIPT_DIR/go-packages.sh" ./internal/cli ./tests/integration)"
  assert_successful_code "$?"
  assert_equals $'example/cmd\nexample/internal/clip\nexample/tests/integration/fixture\nexample/tests/integrationx' "$out"
}

function test_drops_sub_packages_for_a_trailing_ellipsis() {
  local out
  out="$(PATH="$dir/bin:$PATH" "$SCRIPT_DIR/go-packages.sh" ./internal/cli ./tests/integration/...)"
  assert_successful_code "$?"
  assert_equals $'example/cmd\nexample/internal/clip\nexample/tests/integrationx' "$out"
}

function test_accepts_full_import_paths() {
  local out
  out="$(PATH="$dir/bin:$PATH" "$SCRIPT_DIR/go-packages.sh" example/internal/cli)"
  assert_successful_code "$?"
  assert_not_contains $'example/internal/cli\n' "$out"$'\n'
  assert_contains "example/internal/clip" "$out"
}

function test_fails_when_an_argument_matches_no_package() {
  local out result=0
  out="$(PATH="$dir/bin:$PATH" "$SCRIPT_DIR/go-packages.sh" ./internal/cli ./internal/missing 2>&1)" || result=$?
  assert_equals 1 "$result"
  assert_contains "example/internal/missing matches no package" "$out"
}

function test_fails_when_go_list_fails() {
  local result=0
  PATH="$dir/bin:$PATH" FAIL_LIST=1 "$SCRIPT_DIR/go-packages.sh" >/dev/null 2>&1 || result=$?
  assert_equals 1 "$result"
}
