'use strict'

// Run: node npm/lib/platforms_test.js  (or npm test inside npm/)
//
// Covers the platform table and the bin shim's resolution of it. Both are the
// whole install mechanism now: npm matches `os` and `cpu` during resolution and
// drops an optional dependency that does not fit, so a wrong tuple here ships a
// package nobody can install and says nothing about it.
//
// No network, no dependencies. The shim tests build a fake node_modules tree in
// a temp directory and run the real shim against it.

const assert = require('node:assert')
const { spawnSync } = require('node:child_process')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const {
  PLATFORMS,
  binaryName,
  entryPoint,
  optionalDependencies,
  packageName,
  platformFor,
} = require('./platforms')

const SHIM = path.join(__dirname, '..', 'bin', 'agnostic-ai.js')
const PARENT = require('../package.json')

function tempDir(prefix) {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix))
}

// A node_modules tree shaped like the one npm builds: the shim in a package
// that has the platform package as a sibling, which is what require.resolve
// walks. `stub` is the shell script the "binary" runs.
//
// `opts.global` builds the other shape npm produces, `<prefix>/lib/node_modules`,
// the one `npm install` without `-g` cannot repair. The shim reads its own path
// to tell the two apart, so the tree layout is the thing under test.
function installTree(platform, stub, opts = {}) {
  const root = tempDir('agnostic-ai-shim-')
  const modules = opts.global
    ? path.join(root, 'lib', 'node_modules')
    : path.join(root, 'node_modules')
  const pkg = path.join(modules, 'agnostic-ai')
  fs.mkdirSync(path.join(pkg, 'bin'), { recursive: true })
  fs.mkdirSync(path.join(pkg, 'lib'), { recursive: true })
  fs.copyFileSync(SHIM, path.join(pkg, 'bin', 'agnostic-ai.js'))
  fs.copyFileSync(path.join(__dirname, 'platforms.js'), path.join(pkg, 'lib', 'platforms.js'))

  if (platform) {
    const dir = path.join(modules, packageName(platform))
    fs.mkdirSync(dir, { recursive: true })
    fs.writeFileSync(
      path.join(dir, 'package.json'),
      JSON.stringify({ name: packageName(platform), version: '0.0.0-dev' })
    )
    const binary = path.join(dir, binaryName(platform))
    fs.writeFileSync(binary, stub)
    fs.chmodSync(binary, 0o755)
  }

  return { root, shim: path.join(pkg, 'bin', 'agnostic-ai.js') }
}

function runShim(shim, args = [], env = {}) {
  return spawnSync(process.execPath, [shim, ...args], {
    encoding: 'utf8',
    env: { ...process.env, AGNOSTIC_AI_BINARY: '', ...env },
  })
}

const tests = {
  'every supported target has one entry'() {
    assert.strictEqual(PLATFORMS.length, 6)
    const names = PLATFORMS.map(packageName)
    assert.deepStrictEqual([...new Set(names)].sort(), names.slice().sort())
  },

  'os and cpu use npm spellings, goos and goarch use go spellings'() {
    for (const p of PLATFORMS) {
      assert.ok(['darwin', 'linux', 'win32'].includes(p.os), `bad os ${p.os}`)
      assert.ok(['x64', 'arm64'].includes(p.cpu), `bad cpu ${p.cpu}`)
      assert.ok(['darwin', 'linux', 'windows'].includes(p.goos), `bad goos ${p.goos}`)
      assert.ok(['amd64', 'arm64'].includes(p.goarch), `bad goarch ${p.goarch}`)
    }
  },

  // The pair npm matches on is the one the release archive was built for. Cross
  // them and npm installs a linux binary on macOS, or nothing at all.
  'each npm pair maps to the matching go pair'() {
    const wantOs = { darwin: 'darwin', linux: 'linux', win32: 'windows' }
    const wantCpu = { x64: 'amd64', arm64: 'arm64' }
    for (const p of PLATFORMS) {
      assert.strictEqual(p.goos, wantOs[p.os], `${packageName(p)} points at ${p.goos}`)
      assert.strictEqual(p.goarch, wantCpu[p.cpu], `${packageName(p)} points at ${p.goarch}`)
    }
  },

  'only windows gets the .exe suffix'() {
    for (const p of PLATFORMS) {
      assert.strictEqual(binaryName(p), p.os === 'win32' ? 'agnostic-ai.exe' : 'agnostic-ai')
    }
  },

  'the entry point is a subpath of the platform package'() {
    const p = platformFor('win32', 'x64')
    assert.strictEqual(entryPoint(p), '@agnostic-ai/win32-x64/agnostic-ai.exe')
    assert.strictEqual(entryPoint(platformFor('linux', 'arm64')), '@agnostic-ai/linux-arm64/agnostic-ai')
  },

  'an unsupported pair resolves to nothing'() {
    assert.strictEqual(platformFor('freebsd', 'x64'), undefined)
    assert.strictEqual(platformFor('darwin', 'ia32'), undefined)
  },

  // A caret would let npm pair a new CLI with an older binary, which is exactly
  // the drift the platform packages exist to remove.
  'optional dependencies pin exact versions'() {
    const deps = optionalDependencies('1.2.3')
    assert.strictEqual(Object.keys(deps).length, PLATFORMS.length)
    for (const [name, version] of Object.entries(deps)) {
      assert.strictEqual(version, '1.2.3', `${name} is not pinned`)
      assert.match(name, /^@agnostic-ai\//)
    }
  },

  // The committed manifest is what a contributor packs locally, and what the
  // release script edits in place. If it lists a name the table does not know,
  // the release publishes six packages and pins a seventh that never exists.
  'the committed parent manifest pins exactly the table'() {
    assert.deepStrictEqual(
      Object.keys(PARENT.optionalDependencies || {}).sort(),
      PLATFORMS.map(packageName).sort()
    )
    for (const version of Object.values(PARENT.optionalDependencies)) {
      assert.strictEqual(version, PARENT.version)
    }
  },

  // Following biome: the parent installs everywhere and the shim explains the
  // gap. `os`/`cpu` on the parent would fail `npm install` for a whole project
  // because one machine in the team runs something we do not build for.
  'the parent declares no os or cpu of its own'() {
    assert.strictEqual(PARENT.os, undefined)
    assert.strictEqual(PARENT.cpu, undefined)
  },

  // The reason this change exists. An install script is the failure surface.
  'the parent runs no install script'() {
    for (const hook of ['preinstall', 'install', 'postinstall', 'prepare']) {
      assert.strictEqual(PARENT.scripts[hook], undefined, `${hook} is back`)
    }
  },

  // Named one by one, not by directory: `lib/` would also ship this test
  // file, and `scripts/` would ship the generator, to every user.
  'the published tarball carries the shim and the table, nothing else'() {
    assert.deepStrictEqual(PARENT.files, ['bin/agnostic-ai.js', 'lib/platforms.js', 'README.md'])
  },

  'the shim runs the binary out of the platform package'() {
    if (process.platform === 'win32') {
      console.log('     skipped: the stub is a shell script')
      return
    }
    const platform = platformFor(process.platform, process.arch)
    assert.ok(platform, `no table entry for ${process.platform}/${process.arch}`)
    const { root, shim } = installTree(platform, '#!/bin/sh\necho "ran $*"\n')
    try {
      const run = runShim(shim, ['sync', '--check'])
      assert.strictEqual(run.status, 0, run.stderr)
      assert.strictEqual(run.stdout.trim(), 'ran sync --check')
    } finally {
      fs.rmSync(root, { recursive: true, force: true })
    }
  },

  // npm reuses a lockfile written on another platform without re-resolving
  // optional dependencies (npm/cli#4828), and --omit=optional does it on
  // purpose. Both look like a clean install, so the message has to say what is
  // missing and how to get it.
  //
  // --include=optional is half the repair: --omit=optional can be a persistent
  // entry in the user's npm config, and a reinstall without it obeys that entry
  // and drops the package again.
  'a missing platform package fails with a message that names it'() {
    const { root, shim } = installTree(null)
    try {
      const run = runShim(shim, ['--version'])
      assert.strictEqual(run.status, 1)
      assert.match(run.stderr, /@agnostic-ai\//)
      assert.match(run.stderr, /npm install agnostic-ai --force --include=optional/)
      assert.ok(!/npm install -g/.test(run.stderr), `project install told to reinstall globally:\n${run.stderr}`)
      assert.match(run.stderr, /AGNOSTIC_AI_BINARY/)
    } finally {
      fs.rmSync(root, { recursive: true, force: true })
    }
  },

  // A global wrapper resolves its dependencies from the global tree, so
  // `npm install agnostic-ai --force` installs into the current directory and
  // leaves the CLI as broken as it was. The hint has to carry -g.
  'a global install is told to repair the global tree'() {
    const { root, shim } = installTree(null, undefined, { global: true })
    try {
      const run = runShim(shim, ['--version'])
      assert.strictEqual(run.status, 1)
      assert.match(run.stderr, /@agnostic-ai\//)
      assert.match(run.stderr, /npm install -g agnostic-ai --force --include=optional/)
      assert.match(run.stderr, /AGNOSTIC_AI_BINARY/)
    } finally {
      fs.rmSync(root, { recursive: true, force: true })
    }
  },

  // The global tree is not a special case for anything but the message: the
  // binary still has to run out of it.
  'the shim runs the binary out of a global platform package'() {
    if (process.platform === 'win32') {
      console.log('     skipped: the stub is a shell script')
      return
    }
    const platform = platformFor(process.platform, process.arch)
    const { root, shim } = installTree(platform, '#!/bin/sh\necho "ran $*"\n', { global: true })
    try {
      const run = runShim(shim, ['sync'])
      assert.strictEqual(run.status, 0, run.stderr)
      assert.strictEqual(run.stdout.trim(), 'ran sync')
    } finally {
      fs.rmSync(root, { recursive: true, force: true })
    }
  },

  'AGNOSTIC_AI_BINARY runs a binary this package does not ship'() {
    if (process.platform === 'win32') {
      console.log('     skipped: the stub is a shell script')
      return
    }
    const { root, shim } = installTree(null)
    try {
      const own = path.join(root, 'elsewhere')
      fs.writeFileSync(own, '#!/bin/sh\necho override\n')
      fs.chmodSync(own, 0o755)
      const run = runShim(shim, [], { AGNOSTIC_AI_BINARY: own })
      assert.strictEqual(run.status, 0, run.stderr)
      assert.strictEqual(run.stdout.trim(), 'override')
    } finally {
      fs.rmSync(root, { recursive: true, force: true })
    }
  },

  'the shim reports a signalled child as 128 plus the signal'() {
    if (process.platform === 'win32') {
      console.log('     skipped: no POSIX signals on windows')
      return
    }
    const platform = platformFor(process.platform, process.arch)
    const { root, shim } = installTree(platform, '#!/bin/sh\nkill -TERM $$\n')
    try {
      const run = runShim(shim)
      assert.strictEqual(run.status, 128 + os.constants.signals.SIGTERM, run.stderr)
    } finally {
      fs.rmSync(root, { recursive: true, force: true })
    }
  },
}

function run() {
  let failed = 0
  for (const [name, fn] of Object.entries(tests)) {
    try {
      fn()
      console.log(`ok   ${name}`)
    } catch (err) {
      failed++
      console.error(`FAIL ${name}\n     ${err.message}`)
    }
  }
  console.log(`\n${Object.keys(tests).length - failed} passed, ${failed} failed`)
  // exitCode, not exit(): process.exit() drops whatever stdout has not flushed
  // yet, which silently swallows the tail of the report when it runs in CI with
  // stdout on a pipe.
  process.exitCode = failed === 0 ? 0 : 1
}

run()
