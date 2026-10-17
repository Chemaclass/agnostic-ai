#!/usr/bin/env bash

NPM_TOOLCHAIN_PIN="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/npm-toolchain-pin.json"

function npm_toolchain_verify() {
  node - "$NPM_TOOLCHAIN_PIN" "$1" "$2" "$3" <<'JS'
const fs = require('node:fs')
const crypto = require('node:crypto')
const [pinPath, archive, version, integrity] = process.argv.slice(2)
try {
  let pin
  try { pin = JSON.parse(fs.readFileSync(pinPath, 'utf8')) } catch { throw new Error('npm toolchain pin is invalid') }
  if (!/^\d+\.\d+\.\d+$/.test(pin.version) || pin.url !== `https://registry.npmjs.org/npm/-/npm-${pin.version}.tgz` || !validIntegrity(pin.integrity)) throw new Error('npm toolchain pin is invalid')
  if (version !== pin.version) throw new Error('npm toolchain version does not match the committed pin')
  if (!validIntegrity(integrity)) throw new Error('npm toolchain integrity is malformed')
  if (integrity !== pin.integrity) throw new Error('npm toolchain integrity does not match the committed pin')
  let bytes
  try { bytes = fs.readFileSync(archive) } catch { throw new Error('npm toolchain cannot read archive') }
  const expected = Buffer.from(integrity.slice(7), 'base64')
  const actual = crypto.createHash('sha512').update(bytes).digest()
  if (!crypto.timingSafeEqual(actual, expected)) throw new Error('npm toolchain checksum mismatch')
} catch (error) {
  console.error(error.message)
  process.exitCode = 1
}
function validIntegrity(value) {
  return typeof value === 'string' && /^sha512-[A-Za-z0-9+/]{86}==$/.test(value) && Buffer.from(value.slice(7), 'base64').length === 64 && Buffer.from(value.slice(7), 'base64').toString('base64') === value.slice(7)
}
JS
}

function npm_toolchain_install_verified() (
  set -euo pipefail
  if [ "$#" -ne 3 ]; then printf 'npm toolchain needs archive, version and integrity\n' >&2; return 2; fi
  npm_toolchain_verify "$1" "$2" "$3"
  local temporary
  temporary="$(mktemp -d "${TMPDIR:-/tmp}/agnostic-npm-toolchain.XXXXXX")"
  trap 'case "$temporary" in "${TMPDIR:-/tmp}"/agnostic-npm-toolchain.*) printf "Removing npm toolchain directory: %s\n" "$temporary" >&2; rm -rf "$temporary" ;; esac' EXIT
  mkdir "$temporary/cache"
  : > "$temporary/user.npmrc"
  : > "$temporary/global.npmrc"
  unset NODE_AUTH_TOKEN NPM_TOKEN
  local variable
  while IFS= read -r variable; do
    case "$variable" in
      npm_config_*|NPM_CONFIG_*)
        case "$variable" in npm_config_prefix|NPM_CONFIG_PREFIX) ;; *) unset "$variable" ;; esac
        ;;
    esac
  done < <(compgen -e)
  export NPM_CONFIG_USERCONFIG="$temporary/user.npmrc"
  export NPM_CONFIG_GLOBALCONFIG="$temporary/global.npmrc"
  export NPM_CONFIG_CACHE="$temporary/cache"
  local global_modules
  global_modules="$(npm root -g --cache "$temporary/probe-cache")"
  if [ -z "$global_modules" ]; then printf 'npm toolchain cannot resolve global package directory\n' >&2; return 1; fi
  npm install --global --offline --ignore-scripts --no-audit --no-fund "$1"
  hash -r
  if [ "$(node "$global_modules/npm/bin/npm-cli.js" --version)" != "$2" ]; then printf 'npm toolchain installed version does not match the committed pin\n' >&2; return 1; fi
  if [ "$(npm --version)" != "$2" ]; then printf 'npm toolchain PATH client does not match installed version\n' >&2; return 1; fi
)

function npm_toolchain_main() (
  set -euo pipefail
  if [ "$#" -ne 0 ]; then printf 'npm toolchain takes no arguments\n' >&2; return 2; fi
  unset NODE_AUTH_TOKEN NPM_TOKEN
  local temporary
  temporary="$(mktemp -d "${TMPDIR:-/tmp}/agnostic-npm-download.XXXXXX")"
  trap 'case "$temporary" in "${TMPDIR:-/tmp}"/agnostic-npm-download.*) printf "Removing npm toolchain directory: %s\n" "$temporary" >&2; rm -rf "$temporary" ;; esac' EXIT
  node - "$NPM_TOOLCHAIN_PIN" "$temporary/npm.tgz" <<'JS'
const fs = require('node:fs')
const https = require('node:https')
const crypto = require('node:crypto')
const [pinPath, destination] = process.argv.slice(2)
let pin
try {
  pin = JSON.parse(fs.readFileSync(pinPath, 'utf8'))
  if (!/^\d+\.\d+\.\d+$/.test(pin.version) || pin.url !== `https://registry.npmjs.org/npm/-/npm-${pin.version}.tgz` || !/^sha512-[A-Za-z0-9+/]{86}==$/.test(pin.integrity)) throw new Error()
} catch { console.error('npm toolchain pin is invalid'); process.exit(1) }
let request
const deadline = setTimeout(() => { request.destroy(new Error('timeout')) }, 120000)
request = https.get(pin.url, response => {
  if (response.statusCode !== 200) { response.resume(); request.destroy(new Error('status')); return }
  const chunks = []
  let size = 0
  response.on('data', chunk => {
    size += chunk.length
    if (size > 32 * 1024 * 1024) { request.destroy(new Error('size')); return }
    chunks.push(chunk)
  })
  response.on('error', () => fail())
  response.on('end', () => {
    clearTimeout(deadline)
    if (!response.complete) { fail(); return }
    const bytes = Buffer.concat(chunks)
    const actual = crypto.createHash('sha512').update(bytes).digest('base64')
    if (`sha512-${actual}` !== pin.integrity) { console.error('npm toolchain checksum mismatch'); process.exitCode = 1; return }
    try { fs.writeFileSync(destination, bytes, {flag:'wx', mode:0o600}) } catch { console.error('npm toolchain cannot save verified archive'); process.exitCode = 1 }
  })
})
request.on('error', fail)
function fail() { clearTimeout(deadline); console.error('npm toolchain download failed'); process.exitCode = 1 }
JS
  local version integrity
  version="$(node -p 'require(process.argv[1]).version' "$NPM_TOOLCHAIN_PIN")"
  integrity="$(node -p 'require(process.argv[1]).integrity' "$NPM_TOOLCHAIN_PIN")"
  npm_toolchain_install_verified "$temporary/npm.tgz" "$version" "$integrity"
)

if [ "${BASH_SOURCE[0]}" = "$0" ]; then npm_toolchain_main "$@"; fi
