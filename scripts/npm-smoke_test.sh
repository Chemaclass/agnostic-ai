#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/npm-smoke.sh"

function test_local_tarball_install_is_offline_with_an_empty_cache() {
  local fixture log
  fixture=$(mktemp -d)
  log="$fixture/npm.log"
  # shellcheck disable=SC2329
  function build_binaries() { :; }
  # shellcheck disable=SC2329
  function host_package() { printf '@agnostic-ai/linux-x64\n'; }
  # shellcheck disable=SC2329
  function node() { [[ "${1:-}" == -p ]] && printf '1.2.3\n'; return 0; }
  # shellcheck disable=SC2329
  function npm() {
    printf '%s\n' "$*" >> "$log"
    if [[ "$1" == install ]]; then
      mkdir -p node_modules/.bin
      printf '#!/bin/sh\necho version 1.2.3\n' > node_modules/.bin/agnostic-ai
      chmod +x node_modules/.bin/agnostic-ai
    fi
  }
  main >/dev/null
  assert_contains '--offline' "$(cat "$log")"
  assert_contains '--cache' "$(cat "$log")"
  assert_contains '--ignore-scripts' "$(cat "$log")"
  unset -f npm node build_binaries host_package
  rm -rf "$fixture"
}

function test_local_tarball_identity_and_installed_bytes_are_verified() {
  local output
  output=$(node - "$SCRIPT_DIR" <<'NODE'
const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { execFileSync } = require('node:child_process')
const root = process.argv[2]
const { verifyTarballs, verifyInstalled } = require(path.join(root, 'npm-smoke-verify'))
const { PLATFORMS, packageName, binaryName, platformFor, optionalDependencies } = require(path.join(root, '../npm/lib/platforms'))
const work = fs.mkdtempSync(path.join(os.tmpdir(), 'npm-smoke-fixture-'))
const version = '1.2.3'
function pack(name, changes = {}) {
  const directory = path.join(work, 'packed', 'package')
  fs.mkdirSync(directory, { recursive: true })
  const manifest = { name, version, ...changes }
  if (name === 'agnostic-ai' && !manifest.optionalDependencies) manifest.optionalDependencies = optionalDependencies(version)
  fs.writeFileSync(path.join(directory, 'package.json'), JSON.stringify(manifest))
  const filename = path.join(work, `${name.replace(/^@/, '').replace('/', '-')}-${version}.tgz`)
  execFileSync('tar', ['-czf', filename, '-C', path.dirname(directory), 'package'])
  return filename
}
try {
  for (const name of ['agnostic-ai', ...PLATFORMS.map(packageName)]) pack(name)
  verifyTarballs(work, version)
  const wrong = packageName(PLATFORMS[0])
  const file = pack(wrong, { name: 'substitute' })
  assert.throws(() => verifyTarballs(work, version), /identity mismatch/)
  pack(wrong, { version: '1.2.4' })
  assert.throws(() => verifyTarballs(work, version), /identity mismatch/)
  fs.unlinkSync(file)
  assert.throws(() => verifyTarballs(work, version), /missing local tarball/)
  pack(wrong)
  pack('agnostic-ai', { optionalDependencies: { ...optionalDependencies(version), [wrong]: '1.2.4' } })
  assert.throws(() => verifyTarballs(work, version), /platform pins/)
  const platform = platformFor(process.platform, process.arch)
  const modules = path.join(work, 'project', 'node_modules')
  for (const name of ['agnostic-ai', packageName(platform)]) {
    fs.mkdirSync(path.join(modules, name), { recursive: true })
    fs.writeFileSync(path.join(modules, name, 'package.json'), JSON.stringify({ name, version }))
  }
  const installed = path.join(modules, packageName(platform), binaryName(platform))
  const built = path.join(work, 'binaries', `${platform.goos}_${platform.goarch}`, binaryName(platform))
  fs.mkdirSync(path.dirname(built), { recursive: true })
  fs.writeFileSync(built, 'local build bytes')
  fs.writeFileSync(installed, 'local build bytes')
  verifyInstalled(work, version)
  fs.writeFileSync(installed, 'registry substitute')
  assert.throws(() => verifyInstalled(work, version), /differs from the local build/)
  fs.writeFileSync(path.join(modules, packageName(platform), 'package.json'), JSON.stringify({ name: packageName(platform), version: '1.2.4' }))
  assert.throws(() => verifyInstalled(work, version), /identity mismatch/)
  console.log('verified valid bytes and six rejection cases')
} finally {
  fs.rmSync(work, { recursive: true, force: true })
}
NODE
)
  assert_equals 'verified valid bytes and six rejection cases' "$output"
}
