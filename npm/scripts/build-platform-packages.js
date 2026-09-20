'use strict'

// Emits the six platform packages the parent depends on, and pins the parent to
// them. Run at release time, after the binaries exist:
//
//   node npm/scripts/build-platform-packages.js --binaries <dir> --version 1.2.3
//
// <dir> holds one subdirectory per target, named <goos>_<goarch>, each with the
// extracted binary in it. That layout comes straight from unpacking the six
// release archives, which all contain a file called `agnostic-ai`.
//
// This script is release-critical. Everything it writes has to agree with
// lib/platforms.js, because npm resolves an optional dependency by matching its
// `os` and `cpu` against the running machine and skips the ones that do not
// match without saying so. A wrong pair here publishes fine and installs
// nothing.

const fs = require('node:fs')
const path = require('node:path')
const {
  PLATFORMS,
  binaryName,
  optionalDependencies,
  packageName,
} = require('../lib/platforms')

const PACKAGE_ROOT = path.join(__dirname, '..')
const PARENT_MANIFEST = path.join(PACKAGE_ROOT, 'package.json')
const DEFAULT_OUT = path.join(PACKAGE_ROOT, 'platforms')

// --manifest exists so the test suite can run the real build against a copy of
// the parent package.json instead of rewriting the one in the repo.
const FLAGS = ['binaries', 'manifest', 'out', 'version']

function parseArgs(argv) {
  const args = {}
  for (let i = 0; i < argv.length; i++) {
    const flag = argv[i]
    if (!flag.startsWith('--')) throw new Error(`unexpected argument ${flag}`)
    const name = flag.slice(2)
    if (!FLAGS.includes(name)) throw new Error(`unknown flag ${flag}, expected --${FLAGS.join(', --')}`)
    const value = argv[++i]
    if (value === undefined) throw new Error(`${flag} needs a value`)
    args[name] = value
  }
  return args
}

function readJson(file) {
  return JSON.parse(fs.readFileSync(file, 'utf8'))
}

// npm rewrites package.json with two-space indent and a trailing newline, so
// match it: a release that reformats the file makes every diff unreadable.
function writeJson(file, value) {
  fs.writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`)
}

function sourceBinary(binaries, platform) {
  return path.join(binaries, `${platform.goos}_${platform.goarch}`, binaryName(platform))
}

// Check all six before writing any, so a missing binary cannot leave half the
// packages on disk for the publish loop to ship.
function requireEveryBinary(binaries) {
  const missing = PLATFORMS.filter((p) => !fs.existsSync(sourceBinary(binaries, p)))
  if (missing.length === 0) return
  const names = missing.map((p) => sourceBinary(binaries, p)).join('\n  ')
  throw new Error(`no binary for ${missing.length} of ${PLATFORMS.length} targets:\n  ${names}`)
}

function manifest(parent, platform, version) {
  return {
    name: packageName(platform),
    version,
    description: `The ${platform.os}-${platform.cpu} binary for agnostic-ai.`,
    homepage: parent.homepage,
    bugs: parent.bugs,
    repository: parent.repository,
    license: parent.license,
    author: parent.author,
    engines: parent.engines,
    // npm reads these two during resolution. They are the whole mechanism.
    os: [platform.os],
    cpu: [platform.cpu],
    files: [binaryName(platform), 'README.md'],
  }
}

function readme(platform) {
  return [
    `# ${packageName(platform)}`,
    '',
    `The agnostic-ai binary for ${platform.os} on ${platform.cpu}.`,
    '',
    'Install [agnostic-ai](https://www.npmjs.com/package/agnostic-ai) instead. It depends on',
    'this package optionally, and npm picks the one that matches your machine.',
    '',
    'Docs: [agnostic-ai.org](https://agnostic-ai.org).',
    '',
  ].join('\n')
}

// A rerun has to replace the previous output, and replacing means a recursive
// delete. Keep it to a path that can only be one platform directory: named
// <os>-<cpu>, below a non-empty --out, and never a filesystem root.
function platformDir(out, platform) {
  const resolved = path.resolve(out)
  if (resolved === path.parse(resolved).root) {
    throw new Error(`--out resolves to ${resolved}; refusing to write platform packages there`)
  }
  return path.join(resolved, `${platform.os}-${platform.cpu}`)
}

function emit(parent, platform, { binaries, out, version }) {
  const dir = platformDir(out, platform)
  fs.rmSync(dir, { recursive: true, force: true })
  fs.mkdirSync(dir, { recursive: true })

  writeJson(path.join(dir, 'package.json'), manifest(parent, platform, version))
  fs.writeFileSync(path.join(dir, 'README.md'), readme(platform))

  const dest = path.join(dir, binaryName(platform))
  fs.copyFileSync(sourceBinary(binaries, platform), dest)
  // npm preserves the executable bit in a tarball, and the archives we unpack
  // already carry it. Set it anyway: a binary copied out of a zip on Windows,
  // or through a checkout that dropped the mode, spawns with EACCES.
  fs.chmodSync(dest, 0o755)

  return dir
}

function build({ binaries, manifest: manifestPath = PARENT_MANIFEST, out = DEFAULT_OUT, version }) {
  if (!binaries) throw new Error('--binaries is required: the directory holding the built binaries')
  if (!out) throw new Error('--out cannot be empty')
  const parent = readJson(manifestPath)
  const resolved = version || parent.version

  requireEveryBinary(binaries)

  const dirs = PLATFORMS.map((p) => emit(parent, p, { binaries, out, version: resolved }))

  // The parent moves in lockstep with the six. Writing both here is what keeps
  // them at one version: nothing else in the release sets these pins.
  parent.version = resolved
  parent.optionalDependencies = optionalDependencies(resolved)
  writeJson(manifestPath, parent)

  return { dirs, version: resolved }
}

if (require.main === module) {
  try {
    const { dirs, version } = build(parseArgs(process.argv.slice(2)))
    for (const dir of dirs) console.log(`built ${path.relative(process.cwd(), dir)}`)
    console.log(`pinned agnostic-ai@${version} to ${dirs.length} platform packages`)
  } catch (err) {
    console.error(`build-platform-packages: ${err.message}`)
    process.exit(1)
  }
}

module.exports = { build, parseArgs, sourceBinary }
