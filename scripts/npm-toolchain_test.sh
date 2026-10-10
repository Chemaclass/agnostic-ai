#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

function set_up() {
  unset TOOLCHAIN_TEST_STALE_PATH
  TOOLCHAIN_TEST_DIR="$(mktemp -d "${TMPDIR:-/tmp}/agnostic-npm-toolchain-tests.XXXXXX")"
  export TOOLCHAIN_TEST_DIR
  mkdir -p "$TOOLCHAIN_TEST_DIR/package" "$TOOLCHAIN_TEST_DIR/bin"
  printf '{"name":"npm","version":"11.5.1"}\n' > "$TOOLCHAIN_TEST_DIR/package/package.json"
  tar -czf "$TOOLCHAIN_TEST_DIR/npm fixture.tgz" -C "$TOOLCHAIN_TEST_DIR" package
  TOOLCHAIN_TEST_SRI="$(node -e 'const fs=require("node:fs"),crypto=require("node:crypto");process.stdout.write("sha512-"+crypto.createHash("sha512").update(fs.readFileSync(process.argv[1])).digest("base64"))' "$TOOLCHAIN_TEST_DIR/npm fixture.tgz")"
  export TOOLCHAIN_TEST_SRI
  TOOLCHAIN_TEST_BOOTSTRAP_NPM="$(command -v npm)"
  export TOOLCHAIN_TEST_BOOTSTRAP_NPM
  mkdir "$TOOLCHAIN_TEST_DIR/helper"
  cp "$SCRIPT_DIR/npm-toolchain.sh" "$TOOLCHAIN_TEST_DIR/helper/npm-toolchain.sh"
  node - "$TOOLCHAIN_TEST_DIR/helper/npm-toolchain-pin.json" "$TOOLCHAIN_TEST_SRI" <<'JS'
const fs = require('node:fs')
fs.writeFileSync(process.argv[2], JSON.stringify({version:'11.5.1',url:'https://registry.npmjs.org/npm/-/npm-11.5.1.tgz',integrity:process.argv[3]}))
JS
  cat > "$TOOLCHAIN_TEST_DIR/bin/npm" <<'NPM'
#!/usr/bin/env bash
if [ "$1" = '--version' ]; then
  if [ "${TOOLCHAIN_TEST_STALE_PATH:-}" != '1' ] && [ -f "$TOOLCHAIN_TEST_DIR/global/lib/node_modules/npm/bin/npm-cli.js" ]; then
    printf '11.5.1\n'
  else
    printf '10.9.9\n'
  fi
  exit 0
fi
if [ "$1" = 'root' ] && [ "$2" = '-g' ]; then
  "$TOOLCHAIN_TEST_BOOTSTRAP_NPM" config get prefix --cache "$4" > "$TOOLCHAIN_TEST_DIR/real-config-prefix.log" || exit "$?"
  printf '%s/global/lib/node_modules\n' "$TOOLCHAIN_TEST_DIR"
  exit 0
fi
node - "$@" <<'JS'
const fs = require('node:fs')
const record = {args:process.argv.slice(2), tokenPresent:!!process.env.NODE_AUTH_TOKEN, config:process.env.NPM_CONFIG_USERCONFIG || process.env.npm_config_userconfig, globalConfig:process.env.NPM_CONFIG_GLOBALCONFIG || process.env.npm_config_globalconfig, cache:process.env.NPM_CONFIG_CACHE || process.env.npm_config_cache}
record.configContents = record.config ? fs.readFileSync(record.config, 'utf8') : null
record.globalConfigContents = record.globalConfig ? fs.readFileSync(record.globalConfig, 'utf8') : null
record.cacheEntries = record.cache ? fs.readdirSync(record.cache) : null
fs.writeFileSync(process.env.TOOLCHAIN_TEST_DIR+'/installer.json', JSON.stringify(record))
const installed = process.env.TOOLCHAIN_TEST_DIR+'/global/lib/node_modules/npm/bin'
fs.mkdirSync(installed, {recursive:true})
fs.writeFileSync(installed+'/npm-cli.js', "console.log('11.5.1')\n")
JS
NPM
  chmod +x "$TOOLCHAIN_TEST_DIR/bin/npm"
}

function tear_down() {
  case "$TOOLCHAIN_TEST_DIR" in
    "${TMPDIR:-/tmp}"/agnostic-npm-toolchain-tests.*)
      printf 'Removing fixture directory: %s\n' "$TOOLCHAIN_TEST_DIR"
      rm -rf "$TOOLCHAIN_TEST_DIR"
      ;;
    *) return 1 ;;
  esac
}

function run_verified_fixture() {
  local helper_dir="${4:-$TOOLCHAIN_TEST_DIR/helper}"
  PATH="$TOOLCHAIN_TEST_DIR/bin:$PATH" NODE_AUTH_TOKEN='fixture-token-must-not-reach-installer' bash -c '
    set -euo pipefail
    source "$1/npm-toolchain.sh"
    npm_toolchain_install_verified "$2" "$3" "$4"
  ' bash "$helper_dir" "$1" "$2" "$3" > "$TOOLCHAIN_TEST_DIR/result.log" 2>&1
}

function test_verified_local_bundle_installs_offline_without_scripts_or_token() {
  run_verified_fixture "$TOOLCHAIN_TEST_DIR/npm fixture.tgz" '11.5.1' "$TOOLCHAIN_TEST_SRI"
  assert_successful_code "$?"
  node - "$TOOLCHAIN_TEST_DIR" <<'JS'
const assert = require('node:assert/strict'), fs = require('node:fs'), path = require('node:path')
const root = process.argv[2]
const record = JSON.parse(fs.readFileSync(root+'/installer.json'))
for (const flag of ['--global','--offline','--ignore-scripts','--no-audit','--no-fund']) assert.ok(record.args.includes(flag), flag)
assert.ok(record.args.some(arg => !arg.startsWith('-') && fs.existsSync(arg) && fs.realpathSync(arg) === fs.realpathSync(path.join(root,'npm fixture.tgz'))), 'verified local artifact')
assert.ok(!record.args.some(arg => /^npm@/.test(arg)), 'no registry install')
assert.ok(!record.args.some(arg => arg === '--prefix' || arg.startsWith('--prefix=')), 'preserve setup-node global prefix')
assert.equal(record.tokenPresent, false)
assert.ok(record.config && record.cache, 'isolated config and cache')
assert.equal(record.configContents, '')
assert.equal(record.globalConfigContents, '')
assert.notEqual(record.config, record.globalConfig)
assert.deepEqual(record.cacheEntries, [])
JS
  assert_successful_code "$?"
}

function test_wrong_checksum_stops_before_installer() {
  local invalid
  invalid="$(node -e 'process.stdout.write("sha512-"+Buffer.alloc(64).toString("base64"))')"
  node - "$TOOLCHAIN_TEST_DIR/helper/npm-toolchain-pin.json" "$invalid" <<'JS'
const fs = require('node:fs')
const pin = JSON.parse(fs.readFileSync(process.argv[2]))
pin.integrity = process.argv[3]
fs.writeFileSync(process.argv[2], JSON.stringify(pin))
JS
  run_verified_fixture "$TOOLCHAIN_TEST_DIR/npm fixture.tgz" '11.5.1' "$invalid"
  assert_not_same '0' "$?"
  assert_contains "npm toolchain checksum mismatch" "$(cat "$TOOLCHAIN_TEST_DIR/result.log")"
  assert_file_not_exists "$TOOLCHAIN_TEST_DIR/installer.json"
}

function test_modified_archive_stops_before_installer() {
  printf 'changed bytes' >> "$TOOLCHAIN_TEST_DIR/npm fixture.tgz"
  run_verified_fixture "$TOOLCHAIN_TEST_DIR/npm fixture.tgz" '11.5.1' "$TOOLCHAIN_TEST_SRI"
  assert_not_same '0' "$?"
  assert_contains "npm toolchain checksum mismatch" "$(cat "$TOOLCHAIN_TEST_DIR/result.log")"
  assert_file_not_exists "$TOOLCHAIN_TEST_DIR/installer.json"
}

function test_altered_version_is_rejected_before_installer() {
  run_verified_fixture "$TOOLCHAIN_TEST_DIR/npm fixture.tgz" '11.5.2' "$TOOLCHAIN_TEST_SRI"
  assert_not_same '0' "$?"
  assert_contains "npm toolchain version does not match the committed pin" "$(cat "$TOOLCHAIN_TEST_DIR/result.log")"
  assert_file_not_exists "$TOOLCHAIN_TEST_DIR/installer.json"
}

function test_missing_input_is_rejected_before_installer() {
  run_verified_fixture "$TOOLCHAIN_TEST_DIR/missing.tgz" '11.5.1' "$TOOLCHAIN_TEST_SRI"
  assert_not_same '0' "$?"
  assert_contains "npm toolchain cannot read archive" "$(cat "$TOOLCHAIN_TEST_DIR/result.log")"
  assert_file_not_exists "$TOOLCHAIN_TEST_DIR/installer.json"
}

function test_unparseable_integrity_is_rejected_before_installer() {
  run_verified_fixture "$TOOLCHAIN_TEST_DIR/npm fixture.tgz" '11.5.1' 'sha512-not-a-digest'
  assert_not_same '0' "$?"
  assert_contains "npm toolchain integrity is malformed" "$(cat "$TOOLCHAIN_TEST_DIR/result.log")"
  assert_file_not_exists "$TOOLCHAIN_TEST_DIR/installer.json"
}

function test_release_jobs_share_verified_acquisition_and_publish_only_token() {
  node - "$SCRIPT_DIR/../.github/workflows/release.yml" <<'JS'
const assert = require('node:assert/strict'), fs = require('node:fs')
const workflow = fs.readFileSync(process.argv[2], 'utf8')
assert.equal((workflow.match(/scripts\/npm-toolchain\.sh/g) || []).length, 2, 'both npm release jobs use shared verified acquisition')
assert.ok(!/npm install -g .*npm@/.test(workflow), 'no unverified registry install')
const lines = workflow.split('\n')
const tokenLines = lines.map((line,index) => ({line,index})).filter(({line}) => /^\s*NODE_AUTH_TOKEN:/.test(line))
assert.equal(tokenLines.length, 1)
assert.equal(tokenLines[0].line.match(/^\s*/)[0].length, 10, 'token only in step env, not job env')
const before = lines.slice(0,tokenLines[0].index).join('\n')
assert.equal([...before.matchAll(/^      - name: (.+)$/gm)].pop()[1], 'Publish')
JS
  assert_successful_code "$?"
}

function test_unparseable_committed_pin_stops_before_installer() {
  mkdir "$TOOLCHAIN_TEST_DIR/invalid-helper"
  cp "$SCRIPT_DIR/npm-toolchain.sh" "$TOOLCHAIN_TEST_DIR/invalid-helper/npm-toolchain.sh"
  printf '{ invalid json' > "$TOOLCHAIN_TEST_DIR/invalid-helper/npm-toolchain-pin.json"
  run_verified_fixture "$TOOLCHAIN_TEST_DIR/npm fixture.tgz" '11.5.1' "$TOOLCHAIN_TEST_SRI" "$TOOLCHAIN_TEST_DIR/invalid-helper"
  assert_not_same '0' "$?"
  assert_contains 'npm toolchain pin is invalid' "$(cat "$TOOLCHAIN_TEST_DIR/result.log")"
  assert_file_not_exists "$TOOLCHAIN_TEST_DIR/installer.json"
}

function test_external_integrity_cannot_replace_the_committed_pin() {
  local invalid
  invalid="$(node -e 'process.stdout.write("sha512-"+Buffer.alloc(64).toString("base64"))')"
  run_verified_fixture "$TOOLCHAIN_TEST_DIR/npm fixture.tgz" '11.5.1' "$invalid"
  assert_not_same '0' "$?"
  assert_contains 'npm toolchain integrity does not match the committed pin' "$(cat "$TOOLCHAIN_TEST_DIR/result.log")"
  assert_file_not_exists "$TOOLCHAIN_TEST_DIR/installer.json"
}

function test_real_npm_config_loader_accepts_distinct_empty_config_files() {
  run_verified_fixture "$TOOLCHAIN_TEST_DIR/npm fixture.tgz" '11.5.1' "$TOOLCHAIN_TEST_SRI"
  assert_successful_code "$?"
  assert_file_exists "$TOOLCHAIN_TEST_DIR/real-config-prefix.log"
  assert_not_same '' "$(cat "$TOOLCHAIN_TEST_DIR/real-config-prefix.log")"
  node - "$TOOLCHAIN_TEST_DIR/installer.json" <<'JS'
const assert = require('node:assert/strict'), fs = require('node:fs')
const record = JSON.parse(fs.readFileSync(process.argv[2]))
assert.notEqual(record.config, record.globalConfig)
assert.equal(record.configContents, '')
assert.equal(record.globalConfigContents, '')
JS
  assert_successful_code "$?"
}

function test_stale_path_client_cannot_claim_publication_readiness() {
  export TOOLCHAIN_TEST_STALE_PATH=1
  run_verified_fixture "$TOOLCHAIN_TEST_DIR/npm fixture.tgz" '11.5.1' "$TOOLCHAIN_TEST_SRI"
  assert_not_same '0' "$?"
  assert_contains 'npm toolchain PATH client does not match installed version' "$(cat "$TOOLCHAIN_TEST_DIR/result.log")"
  assert_file_exists "$TOOLCHAIN_TEST_DIR/installer.json"
  assert_same '11.5.1' "$(node "$TOOLCHAIN_TEST_DIR/global/lib/node_modules/npm/bin/npm-cli.js" --version)"
  assert_same '10.9.9' "$(PATH="$TOOLCHAIN_TEST_DIR/bin:$PATH" npm --version)"
  unset TOOLCHAIN_TEST_STALE_PATH
}
