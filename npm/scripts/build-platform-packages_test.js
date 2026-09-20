'use strict'

// Run: node npm/scripts/build-platform-packages_test.js  (or npm test inside npm/)
//
// The generator runs once per release and nothing downstream checks its output,
// so every assertion here stands in for a broken install nobody would see until
// a user reported it. Each case builds against a fake set of binaries in a temp
// directory and a copy of the parent manifest, so the repo is never touched.

const assert = require('node:assert')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { PLATFORMS, binaryName, packageName } = require('../lib/platforms')
const { build, manifest, parseArgs, sourceBinary } = require('./build-platform-packages')

const PARENT_MANIFEST = path.join(__dirname, '..', 'package.json')

function workspace() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'agnostic-ai-build-'))
  return {
    root,
    binaries: path.join(root, 'binaries'),
    out: path.join(root, 'platforms'),
    manifest: path.join(root, 'package.json'),
  }
}

// One fake binary per target, laid out the way the release job unpacks the six
// archives: a directory per target, each holding a file called agnostic-ai.
function plantBinaries(dir, targets = PLATFORMS) {
  for (const p of targets) {
    const file = sourceBinary(dir, p)
    fs.mkdirSync(path.dirname(file), { recursive: true })
    fs.writeFileSync(file, `binary for ${p.goos}/${p.goarch}\n`)
  }
}

function setUp(targets) {
  const ws = workspace()
  fs.copyFileSync(PARENT_MANIFEST, ws.manifest)
  plantBinaries(ws.binaries, targets)
  return ws
}

function readJson(file) {
  return JSON.parse(fs.readFileSync(file, 'utf8'))
}

function emitted(ws, platform) {
  return readJson(path.join(ws.out, `${platform.os}-${platform.cpu}`, 'package.json'))
}

function thrown(fn) {
  try {
    fn()
  } catch (err) {
    return err
  }
  throw new Error('expected the call to throw')
}

function withWorkspace(targets, fn) {
  const ws = setUp(targets)
  try {
    fn(ws)
  } finally {
    fs.rmSync(ws.root, { recursive: true, force: true })
  }
}

const tests = {
  'it emits one package per target'() {
    withWorkspace(undefined, (ws) => {
      const { dirs } = build({ binaries: ws.binaries, manifest: ws.manifest, out: ws.out, version: '1.2.3' })
      assert.strictEqual(dirs.length, PLATFORMS.length)
      assert.deepStrictEqual(
        fs.readdirSync(ws.out).sort(),
        PLATFORMS.map((p) => `${p.os}-${p.cpu}`).sort()
      )
    })
  },

  // The pair npm resolves on. Wrong and the package installs on no machine, or
  // on the wrong one, and npm reports neither.
  'each package declares the os and cpu npm matches it by'() {
    withWorkspace(undefined, (ws) => {
      build({ binaries: ws.binaries, manifest: ws.manifest, out: ws.out, version: '1.2.3' })
      for (const p of PLATFORMS) {
        const pkg = emitted(ws, p)
        assert.strictEqual(pkg.name, packageName(p))
        assert.deepStrictEqual(pkg.os, [p.os], `${pkg.name} os`)
        assert.deepStrictEqual(pkg.cpu, [p.cpu], `${pkg.name} cpu`)
      }
    })
  },

  'the windows packages carry agnostic-ai.exe and the rest carry agnostic-ai'() {
    withWorkspace(undefined, (ws) => {
      build({ binaries: ws.binaries, manifest: ws.manifest, out: ws.out, version: '1.2.3' })
      for (const p of PLATFORMS) {
        const dir = path.join(ws.out, `${p.os}-${p.cpu}`)
        const want = p.os === 'win32' ? 'agnostic-ai.exe' : 'agnostic-ai'
        assert.strictEqual(binaryName(p), want)
        assert.ok(fs.existsSync(path.join(dir, want)), `${want} missing from ${dir}`)
        assert.deepStrictEqual(emitted(ws, p).files, [want, 'README.md'])
      }
    })
  },

  'the binary it copies is the one built for that target'() {
    withWorkspace(undefined, (ws) => {
      build({ binaries: ws.binaries, manifest: ws.manifest, out: ws.out, version: '1.2.3' })
      for (const p of PLATFORMS) {
        const copied = fs.readFileSync(
          path.join(ws.out, `${p.os}-${p.cpu}`, binaryName(p)),
          'utf8'
        )
        assert.strictEqual(copied.trim(), `binary for ${p.goos}/${p.goarch}`)
      }
    })
  },

  'the copied binary is executable'() {
    if (process.platform === 'win32') {
      console.log('     skipped: no POSIX mode bits')
      return
    }
    withWorkspace(undefined, (ws) => {
      build({ binaries: ws.binaries, manifest: ws.manifest, out: ws.out, version: '1.2.3' })
      for (const p of PLATFORMS) {
        const mode = fs.statSync(path.join(ws.out, `${p.os}-${p.cpu}`, binaryName(p))).mode
        assert.ok(mode & 0o111, `${packageName(p)} binary is not executable`)
      }
    })
  },

  // Seven packages at one version or the resolution picks nothing. This is the
  // single assertion that keeps them in lockstep.
  'the parent and all six packages land on the same version'() {
    withWorkspace(undefined, (ws) => {
      build({ binaries: ws.binaries, manifest: ws.manifest, out: ws.out, version: '9.8.7' })
      const parent = readJson(ws.manifest)
      assert.strictEqual(parent.version, '9.8.7')
      assert.deepStrictEqual(
        Object.keys(parent.optionalDependencies).sort(),
        PLATFORMS.map(packageName).sort()
      )
      for (const p of PLATFORMS) {
        assert.strictEqual(parent.optionalDependencies[packageName(p)], '9.8.7')
        assert.strictEqual(emitted(ws, p).version, '9.8.7')
      }
    })
  },

  'without --version it keeps the version already in the manifest'() {
    withWorkspace(undefined, (ws) => {
      const before = readJson(ws.manifest).version
      const { version } = build({ binaries: ws.binaries, manifest: ws.manifest, out: ws.out })
      assert.strictEqual(version, before)
      assert.strictEqual(emitted(ws, PLATFORMS[0]).version, before)
    })
  },

  // A partial build is the failure this whole change introduces: publish five
  // packages and a parent that pins six, and one platform stops installing.
  // Refuse before anything reaches disk.
  'a missing binary aborts before it writes anything'() {
    withWorkspace(PLATFORMS.slice(1), (ws) => {
      const err = thrown(() =>
        build({ binaries: ws.binaries, manifest: ws.manifest, out: ws.out, version: '1.2.3' })
      )
      assert.match(err.message, /no binary for 1 of 6 targets/)
      assert.match(err.message, new RegExp(`${PLATFORMS[0].goos}_${PLATFORMS[0].goarch}`))
      assert.ok(!fs.existsSync(ws.out), 'it wrote packages anyway')
      assert.strictEqual(readJson(ws.manifest).version, '0.0.0-dev', 'it pinned the parent anyway')
    })
  },

  'it inherits the parent licence, repository and engines'() {
    withWorkspace(undefined, (ws) => {
      const parent = readJson(ws.manifest)
      build({ binaries: ws.binaries, manifest: ws.manifest, out: ws.out, version: '1.2.3' })
      const pkg = emitted(ws, PLATFORMS[0])
      assert.strictEqual(pkg.license, parent.license)
      assert.deepStrictEqual(pkg.repository, parent.repository)
      assert.deepStrictEqual(pkg.engines, parent.engines)
      assert.strictEqual(pkg.homepage, parent.homepage)
    })
  },

  // The point of the change. A platform package that runs code at install time
  // puts back everything the postinstall download cost us.
  'no emitted package declares a script'() {
    withWorkspace(undefined, (ws) => {
      build({ binaries: ws.binaries, manifest: ws.manifest, out: ws.out, version: '1.2.3' })
      for (const p of PLATFORMS) {
        assert.strictEqual(emitted(ws, p).scripts, undefined, `${packageName(p)} has scripts`)
      }
    })
  },

  'a rerun replaces the previous output instead of layering on it'() {
    withWorkspace(undefined, (ws) => {
      build({ binaries: ws.binaries, manifest: ws.manifest, out: ws.out, version: '1.2.3' })
      const dir = path.join(ws.out, `${PLATFORMS[0].os}-${PLATFORMS[0].cpu}`)
      fs.writeFileSync(path.join(dir, 'stale.txt'), 'left over')
      build({ binaries: ws.binaries, manifest: ws.manifest, out: ws.out, version: '1.2.4' })
      assert.ok(!fs.existsSync(path.join(dir, 'stale.txt')))
      assert.strictEqual(emitted(ws, PLATFORMS[0]).version, '1.2.4')
    })
  },

  'it writes the manifest the way npm does, so a release is a readable diff'() {
    withWorkspace(undefined, (ws) => {
      build({ binaries: ws.binaries, manifest: ws.manifest, out: ws.out, version: '1.2.3' })
      const text = fs.readFileSync(ws.manifest, 'utf8')
      assert.ok(text.endsWith('}\n'), 'no trailing newline')
      assert.match(text, /\n {2}"version": "1\.2\.3",/)
    })
  },

  'an unknown flag fails instead of being ignored'() {
    assert.deepStrictEqual(parseArgs(['--binaries', 'x', '--version', '1.0.0']), {
      binaries: 'x',
      version: '1.0.0',
    })
    assert.match(thrown(() => parseArgs(['--binary', 'x'])).message, /unknown flag --binary/)
    assert.match(thrown(() => parseArgs(['--version'])).message, /needs a value/)
    assert.match(thrown(() => parseArgs(['1.0.0'])).message, /unexpected argument/)
  },

  'it refuses to run without a binaries directory'() {
    assert.match(thrown(() => build({})).message, /--binaries is required/)
  },

  'the emitted manifest describes the platform in its description'() {
    const pkg = manifest({ license: 'MIT' }, PLATFORMS[0], '1.2.3')
    assert.match(pkg.description, /darwin-arm64/)
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
  process.exitCode = failed === 0 ? 0 : 1
}

run()
