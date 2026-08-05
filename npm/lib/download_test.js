'use strict'

// Run: node npm/lib/download_test.js  (or npm test inside npm/)
//
// Covers the pure mapping logic. The download path itself is exercised
// end to end by .github/workflows/install.yml against a real release.

const assert = require('node:assert')
const path = require('node:path')
const { assetName, binaryPath, downloadUrl, resolveVersion, target } = require('./download')

const tests = {
  'asset name matches the release archive for this platform'() {
    const { goos, goarch } = target()
    const expected = goos === 'windows'
      ? `agnostic-ai_${goos}_${goarch}.zip`
      : `agnostic-ai_${goos}_${goarch}.tar.gz`
    assert.strictEqual(assetName(), expected)
  },

  'windows gets a zip, every other platform a tar.gz'() {
    assert.strictEqual(target().ext, process.platform === 'win32' ? 'zip' : 'tar.gz')
  },

  'download url points at the tagged release asset'() {
    assert.strictEqual(
      downloadUrl('v0.45.0', 'agnostic-ai_linux_amd64.tar.gz'),
      'https://github.com/Chemaclass/agnostic-ai/releases/download/v0.45.0/agnostic-ai_linux_amd64.tar.gz'
    )
  },

  'binary path lands in the package bin dir'() {
    assert.strictEqual(path.dirname(binaryPath()), path.join(__dirname, '..', 'bin'))
    assert.match(path.basename(binaryPath()), /^agnostic-ai(\.exe)?$/)
  },

  async 'env override wins over the package version'() {
    process.env.AGNOSTIC_AI_VERSION = 'v1.2.3'
    try {
      assert.strictEqual(await resolveVersion(), 'v1.2.3')
    } finally {
      delete process.env.AGNOSTIC_AI_VERSION
    }
  },
}

async function run() {
  let failed = 0
  for (const [name, fn] of Object.entries(tests)) {
    try {
      await fn()
      console.log(`ok   ${name}`)
    } catch (err) {
      failed++
      console.error(`FAIL ${name}\n     ${err.message}`)
    }
  }
  console.log(`\n${Object.keys(tests).length - failed} passed, ${failed} failed`)
  process.exit(failed === 0 ? 0 : 1)
}

run()
