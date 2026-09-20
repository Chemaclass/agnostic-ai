'use strict'

// The one table the whole npm distribution reads: the bin shim uses it to find
// the binary at runtime, scripts/build-platform-packages.js uses it to emit the
// packages at release time, and the parent package.json pins the same six names
// in optionalDependencies.
//
// Keeping it in one file is the point. npm skips an optional dependency whose
// `os` or `cpu` does not match the machine, and it skips it silently, so a pair
// that disagrees with what the shim asks for installs nothing and leaves the CLI
// missing with no error anywhere. Two copies of this table is how that happens.

const SCOPE = '@agnostic-ai'

const BINARY = 'agnostic-ai'

// `os` and `cpu` are npm's own spellings, matched against process.platform and
// process.arch. `goos` and `goarch` are Go's, and name the release archive the
// binary is extracted from. The two vocabularies differ on win32/windows and
// x64/amd64, which is the mapping this table exists to hold.
const PLATFORMS = [
  { os: 'darwin', cpu: 'arm64', goos: 'darwin', goarch: 'arm64' },
  { os: 'darwin', cpu: 'x64', goos: 'darwin', goarch: 'amd64' },
  { os: 'linux', cpu: 'arm64', goos: 'linux', goarch: 'arm64' },
  { os: 'linux', cpu: 'x64', goos: 'linux', goarch: 'amd64' },
  { os: 'win32', cpu: 'arm64', goos: 'windows', goarch: 'arm64' },
  { os: 'win32', cpu: 'x64', goos: 'windows', goarch: 'amd64' },
]

function packageName(platform) {
  return `${SCOPE}/${platform.os}-${platform.cpu}`
}

function binaryName(platform) {
  return platform.os === 'win32' ? `${BINARY}.exe` : BINARY
}

// What the shim hands require.resolve(). A subpath of the platform package, so
// Node walks the same node_modules lookup npm just populated.
function entryPoint(platform) {
  return `${packageName(platform)}/${binaryName(platform)}`
}

function platformFor(nodeOs, nodeCpu) {
  return PLATFORMS.find((p) => p.os === nodeOs && p.cpu === nodeCpu)
}

// Exact pins, never a range. A platform package carries a binary built from one
// commit, so a caret would let npm pair a new CLI with an old binary.
function optionalDependencies(version) {
  return Object.fromEntries(PLATFORMS.map((p) => [packageName(p), version]))
}

module.exports = {
  PLATFORMS,
  binaryName,
  entryPoint,
  optionalDependencies,
  packageName,
  platformFor,
}
