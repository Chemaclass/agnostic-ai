'use strict'

// Run: node npm/lib/download_test.js  (or npm test inside npm/)
//
// Covers the pure mapping logic plus the failure paths of the download: every
// server here is a throwaway listener on 127.0.0.1, so the suite needs no
// network and no dependencies. The happy path is exercised end to end by
// .github/workflows/install.yml against a real release.

const assert = require('node:assert')
const { spawnSync } = require('node:child_process')
const fs = require('node:fs')
const http = require('node:http')
const https = require('node:https')
const net = require('node:net')
const os = require('node:os')
const path = require('node:path')
const {
  assetName,
  binaryPath,
  describeError,
  downloadUrl,
  extract,
  get,
  latestTag,
  resolveVersion,
  target,
} = require('./download')

function listen(server) {
  return new Promise((resolve) => {
    server.listen(0, '127.0.0.1', () => resolve(`http://127.0.0.1:${server.address().port}`))
  })
}

function close(server, sockets = []) {
  return new Promise((resolve) => {
    // close() alone waits for every open connection: the keep-alive sockets an
    // http.Server holds, and the one the wedged server keeps open on purpose.
    // Drop both, or the suite hangs here.
    if (server.closeAllConnections) server.closeAllConnections()
    for (const socket of sockets) socket.destroy()
    server.close(resolve)
  })
}

async function rejection(promise) {
  try {
    await promise
  } catch (err) {
    return err
  }
  throw new Error('expected the promise to reject')
}

function thrown(fn) {
  try {
    fn()
  } catch (err) {
    return err
  }
  throw new Error('expected the call to throw')
}

// A real AggregateError, thrown by Node itself: the custom lookup hands the
// connect both loopback addresses so every attempt fails, which is the shape of
// github.com resolving to several IPs behind a firewall. Both refuse instantly,
// so this stays fast and needs no network.
function multiAddressFailure(port) {
  return new Promise((resolve) => {
    https
      .get(
        `https://localhost:${port}/x`,
        {
          autoSelectFamily: true,
          lookup: (host, opts, cb) =>
            cb(null, [
              { address: '127.0.0.1', family: 4 },
              { address: '::1', family: 6 },
            ]),
        },
        () => {}
      )
      .on('error', resolve)
  })
}

function closedPort() {
  return new Promise((resolve) => {
    const probe = net.createServer()
    probe.listen(0, '127.0.0.1', () => {
      const { port } = probe.address()
      probe.close(() => resolve(port))
    })
  })
}

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

  async 'a version pin resolves the same with or without the v prefix'() {
    try {
      for (const pin of ['0.61.0', 'v0.61.0', ' 0.61.0 ']) {
        process.env.AGNOSTIC_AI_VERSION = pin
        assert.strictEqual(await resolveVersion(), 'v0.61.0', `pin ${JSON.stringify(pin)}`)
      }
    } finally {
      delete process.env.AGNOSTIC_AI_VERSION
    }
  },

  async 'an error with no message is described from its code and its causes'() {
    const port = await closedPort()
    const err = await multiAddressFailure(port)

    // Guard the premise: this is what the user currently sees printed.
    assert.ok(err instanceof AggregateError, `expected an AggregateError, got ${err.name}`)
    assert.strictEqual(err.message, '')

    const described = describeError(err).message
    assert.ok(described.length > 0, 'described message is empty')
    assert.match(described, /ECONNREFUSED/)
    assert.ok(described.includes(`127.0.0.1:${port}`), `missing the address: ${described}`)
    assert.ok(described.includes('::1'), `missing the second address: ${described}`)
  },

  'an error that already has a message is left alone'() {
    const err = new Error('connect ETIMEDOUT 203.0.113.1:443')
    assert.strictEqual(describeError(err), err)
  },

  async 'a server that never answers fails with a timeout naming the url'() {
    // Accept the connection and say nothing: no OS-level timeout ever fires.
    const accepted = []
    const server = net.createServer((socket) => accepted.push(socket))
    const base = await listen(server)
    try {
      const err = await rejection(get(`${base}/wedged`, { timeoutMs: 200 }))
      assert.ok(err.message.includes(`${base}/wedged`), `missing the url: ${err.message}`)
      assert.match(err.message, /timed out after 0\.2s/)
    } finally {
      await close(server, accepted)
    }
  },

  async 'a single redirect is still followed'() {
    const server = http.createServer((req, res) => {
      if (req.url === '/asset') {
        res.writeHead(302, { location: `${base}/storage` })
        res.end()
        return
      }
      res.writeHead(200)
      res.end('payload')
    })
    const base = await listen(server)
    try {
      assert.strictEqual((await get(`${base}/asset`)).toString('utf8'), 'payload')
    } finally {
      await close(server)
    }
  },

  async 'a redirect loop stops at the cap'() {
    let hops = 0
    const server = http.createServer((req, res) => {
      hops++
      res.writeHead(302, { location: `${base}/loop` })
      res.end()
    })
    const base = await listen(server)
    try {
      const err = await rejection(get(`${base}/loop`))
      assert.match(err.message, /redirect/i)
      assert.match(err.message, /5/)
      assert.ok(hops <= 6, `followed ${hops} hops, expected at most 6`)
    } finally {
      await close(server)
    }
  },

  async 'the latest release comes from the redirect, not the rate-limited api'() {
    const seen = []
    const server = http.createServer((req, res) => {
      seen.push(req.url)
      res.writeHead(302, { location: 'https://github.com/Chemaclass/agnostic-ai/releases/tag/v0.62.0' })
      res.end()
    })
    const base = await listen(server)
    try {
      assert.strictEqual(await latestTag(`${base}/releases/latest`), 'v0.62.0')
      assert.deepStrictEqual(seen, ['/releases/latest'])
    } finally {
      await close(server)
    }
  },

  async 'a rate-limited release lookup says so, a missing release says that'() {
    let status = 403
    const server = http.createServer((req, res) => {
      res.writeHead(status)
      res.end()
    })
    const base = await listen(server)
    try {
      const limited = await rejection(latestTag(`${base}/releases/latest`))
      assert.match(limited.message, /rate-limited/)
      assert.match(limited.message, /AGNOSTIC_AI_VERSION/)

      status = 404
      const missing = await rejection(latestTag(`${base}/releases/latest`))
      assert.match(missing.message, /no published release/)
      assert.ok(!/rate-limited/.test(missing.message), missing.message)
    } finally {
      await close(server)
    }
  },

  'a failed extraction names the archive and an alternative install route'() {
    const work = fs.mkdtempSync(path.join(os.tmpdir(), 'agnostic-ai-extract-'))
    try {
      const archive = path.join(work, 'agnostic-ai_broken.tar.gz')
      fs.writeFileSync(archive, 'not an archive')
      const err = thrown(() => extract(archive, work))
      assert.ok(err.message.includes('agnostic-ai_broken.tar.gz'), `missing the archive: ${err.message}`)
      assert.match(err.message, /go install github\.com\/chemaclass\/agnostic-ai/)
    } finally {
      fs.rmSync(work, { recursive: true, force: true })
    }
  },

  'the bin shim exits with 128 plus the signal number'() {
    if (process.platform === 'win32') {
      console.log('     skipped: no POSIX signals on windows')
      return
    }
    const work = fs.mkdtempSync(path.join(os.tmpdir(), 'agnostic-ai-shim-'))
    try {
      fs.mkdirSync(path.join(work, 'bin'))
      fs.mkdirSync(path.join(work, 'lib'))
      fs.writeFileSync(path.join(work, 'package.json'), '{"version":"0.0.0-dev"}')
      fs.copyFileSync(path.join(__dirname, 'download.js'), path.join(work, 'lib', 'download.js'))
      const shim = path.join(work, 'bin', 'agnostic-ai.js')
      fs.copyFileSync(path.join(__dirname, '..', 'bin', 'agnostic-ai.js'), shim)

      // ensureBinary short-circuits on an existing binary, so the shim spawns
      // this stub instead of downloading anything. It kills itself with
      // SIGTERM (15), so the shim must report 143.
      const stub = path.join(work, 'bin', 'agnostic-ai')
      fs.writeFileSync(stub, '#!/bin/sh\nkill -TERM $$\n')
      fs.chmodSync(stub, 0o755)

      const run = spawnSync(process.execPath, [shim], { encoding: 'utf8' })
      assert.strictEqual(run.status, 128 + os.constants.signals.SIGTERM, run.stderr)
    } finally {
      fs.rmSync(work, { recursive: true, force: true })
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
  // exitCode, not exit(): process.exit() drops whatever stdout has not flushed
  // yet, which silently swallows the tail of the report when it runs in CI with
  // stdout on a pipe.
  process.exitCode = failed === 0 ? 0 : 1
}

run()
