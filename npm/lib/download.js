'use strict'

// Fetches the agnostic-ai release binary for the current platform. Shared by
// the postinstall hook and the bin shim, so a failed install still recovers on
// first run instead of leaving a broken command.

const { execFileSync } = require('node:child_process')
const crypto = require('node:crypto')
const fs = require('node:fs')
const http = require('node:http')
const https = require('node:https')
const os = require('node:os')
const path = require('node:path')

const REPO = 'Chemaclass/agnostic-ai'
const BINARY = process.platform === 'win32' ? 'agnostic-ai.exe' : 'agnostic-ai'

const PLATFORMS = { darwin: 'darwin', linux: 'linux', win32: 'windows' }
const ARCHS = { x64: 'amd64', arm64: 'arm64' }

// GitHub redirects a release asset to object storage exactly once, so five hops
// is slack for a proxy in the way while still ending a redirect loop.
const MAX_REDIRECTS = 5

// Silence, not slowness, is what this catches: the archives are a few MB, so 30
// seconds without a single byte means the connection is wedged. Left unbounded,
// the OS connect timeout alone runs to 75 seconds, and a server that accepts and
// never answers never times out at all. This runs inside postinstall, where a
// stall blocks `npm install` for the whole project.
const TIMEOUT_MS = 30_000

function target() {
  const goos = PLATFORMS[process.platform]
  const goarch = ARCHS[process.arch]
  if (!goos || !goarch) {
    throw new Error(
      `agnostic-ai has no prebuilt binary for ${process.platform}/${process.arch}. ` +
        'Build from source: go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest'
    )
  }
  return { goos, goarch, ext: goos === 'windows' ? 'zip' : 'tar.gz' }
}

function assetName() {
  const { goos, goarch, ext } = target()
  return `agnostic-ai_${goos}_${goarch}.${ext}`
}

function binaryPath() {
  return path.join(__dirname, '..', 'bin', BINARY)
}

// Node throws an AggregateError with an empty `message` when a host resolves to
// several addresses and every connection fails, which is the ordinary shape of
// github.com behind a firewall. Printing `err.message` then prints nothing, so
// rebuild a message out of what the error does carry.
function describeError(err) {
  if (!err || err.message) return err

  const codes = new Set()
  const causes = new Set()
  if (err.code) codes.add(err.code)
  for (const cause of Array.isArray(err.errors) ? err.errors : []) {
    if (!cause) continue
    if (cause.code) codes.add(cause.code)
    if (cause.message) causes.add(cause.message)
  }

  const summary = codes.size ? [...codes].join(', ') : `${err.name || 'Error'} with no message`
  const described = new Error(causes.size ? `${summary}: ${[...causes].join('; ')}` : summary)
  described.cause = err
  return described
}

function client(url) {
  if (url.startsWith('https:')) return https
  // Only reachable through a redirect; kept so a plain http hop reports
  // something readable instead of ERR_INVALID_PROTOCOL.
  if (url.startsWith('http:')) return http
  throw new Error(`GET ${url} uses an unsupported protocol`)
}

// Resolves once the response headers are in, with a `body()` that reads the
// rest. Splitting it that way lets a caller act on the headers alone, and keeps
// the one timeout covering both halves: the socket timeout fires on silence
// whether or not the body has started, and a request torn down by it must
// report the timeout rather than the `aborted` the stream raises in its wake.
function request(url, timeoutMs) {
  return new Promise((resolve, reject) => {
    let timedOut = null
    const fail = (err) => reject(timedOut || describeError(err))

    const req = client(url)
      .get(url, { headers: { 'user-agent': 'agnostic-ai-npm' }, timeout: timeoutMs }, (res) => {
        resolve({
          res,
          body: () =>
            new Promise((done, failBody) => {
              const chunks = []
              res.on('data', (c) => chunks.push(c))
              res.on('end', () => done(Buffer.concat(chunks)))
              res.on('error', (err) => failBody(timedOut || describeError(err)))
            }),
        })
      })
      .on('timeout', () => {
        // The socket timeout only raises the event; the request has to be torn
        // down by hand, and destroy(err) is what surfaces as a rejection.
        timedOut = new Error(`GET ${url} timed out after ${timeoutMs / 1000}s of silence`)
        req.destroy(timedOut)
      })
      .on('error', fail)
  })
}

// Drain a response nobody will read. The listener is not optional: a response
// that errors with nothing attached takes the whole process down.
function discard(res) {
  res.resume()
  res.on('error', () => {})
}

function redirectTo(res) {
  const { statusCode, headers } = res
  return statusCode >= 300 && statusCode < 400 && headers.location ? headers.location : ''
}

async function get(url, { redirects = 0, timeoutMs = TIMEOUT_MS } = {}) {
  const { res, body } = await request(url, timeoutMs)

  // GitHub redirects release assets to a signed object-store URL.
  const next = redirectTo(res)
  if (next) {
    discard(res)
    if (redirects >= MAX_REDIRECTS) {
      throw new Error(`GET ${url} still redirecting after ${MAX_REDIRECTS} hops`)
    }
    if (url.startsWith('https:') && next.startsWith('http:')) {
      throw new Error(`GET ${url} redirects to plain http (${next}); refusing`)
    }
    return get(next, { redirects: redirects + 1, timeoutMs })
  }

  if (res.statusCode !== 200) {
    discard(res)
    throw new Error(`GET ${url} failed with HTTP ${res.statusCode}`)
  }
  return body()
}

// Release tags carry the `v`, but `npm view` and package.json print the bare
// number, so both spellings of AGNOSTIC_AI_VERSION have to reach the same tag.
// Anything that does not start with a digit is passed through untouched.
function releaseTag(version) {
  const trimmed = version.trim()
  return /^\d/.test(trimmed) ? `v${trimmed}` : trimmed
}

// `releases/latest` on github.com answers 302 to the tag page, so the tag is in
// the Location header. That keeps release resolution off api.github.com, which
// allows 60 unauthenticated requests an hour per IP: a shared CI address burns
// through that and every install then fails with HTTP 403 (#940). The redirect
// target is what this wants, so it reads the response itself instead of going
// through get(), which exists to follow the redirect and hand back a body.
async function latestTag(url = `https://github.com/${REPO}/releases/latest`) {
  const { res } = await request(url, TIMEOUT_MS)
  discard(res)

  const location = redirectTo(res)
  if (location) {
    const tag = decodeURIComponent(location.split('/').pop() || '')
    if (/^v?\d/.test(tag)) return tag
    throw new Error(`could not read a release tag out of ${location}`)
  }
  if (res.statusCode === 403 || res.statusCode === 429) {
    throw new Error(
      `GitHub rate-limited the release lookup (HTTP ${res.statusCode}). Retry in a few ` +
        'minutes, or pin the binary with AGNOSTIC_AI_VERSION=vX.Y.Z'
    )
  }
  if (res.statusCode === 404) {
    throw new Error(
      `${REPO} reports no published release (HTTP 404). Pin one with AGNOSTIC_AI_VERSION=vX.Y.Z`
    )
  }
  throw new Error(`could not resolve the latest agnostic-ai release: HTTP ${res.statusCode}`)
}

// The published package carries the release version; a checkout carries the
// 0.0.0-dev placeholder, which has no matching release, so fall back to latest.
async function resolveVersion() {
  if (process.env.AGNOSTIC_AI_VERSION) return releaseTag(process.env.AGNOSTIC_AI_VERSION)

  const { version } = require('../package.json')
  if (version && !version.startsWith('0.0.0')) return releaseTag(version)

  return latestTag()
}

function downloadUrl(version, asset) {
  return `https://github.com/${REPO}/releases/download/${version}/${asset}`
}

async function verifyChecksum(archive, asset, version) {
  const sums = (await get(downloadUrl(version, 'checksums.txt'))).toString('utf8')
  const line = sums.split('\n').find((l) => l.trim().endsWith(asset))
  if (!line) throw new Error(`${asset} missing from checksums.txt`)

  const expected = line.trim().split(/\s+/)[0]
  const actual = crypto.createHash('sha256').update(fs.readFileSync(archive)).digest('hex')
  if (actual !== expected) throw new Error(`checksum mismatch for ${asset}`)
}

function extract(archive, dir) {
  // bsdtar reads both tar.gz and zip, and ships with macOS and Windows 10
  // 1803+; Linux only ever gets the tar.gz here, so GNU tar is fine too.
  try {
    execFileSync('tar', ['-xf', archive, '-C', dir, BINARY], { stdio: 'ignore' })
  } catch (err) {
    const reason = err.code === 'ENOENT' ? 'tar is not installed' : describeError(err).message
    throw new Error(
      `could not extract ${path.basename(archive)}: ${reason}. Install agnostic-ai another ` +
        'way instead: go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest, ' +
        'or the routes at https://agnostic-ai.org/docs/installation/'
    )
  }
}

async function ensureBinary({ log = () => {} } = {}) {
  const dest = binaryPath()
  if (fs.existsSync(dest)) return dest

  const version = await resolveVersion()
  const asset = assetName()
  log(`downloading agnostic-ai ${version} (${asset})`)

  const work = fs.mkdtempSync(path.join(os.tmpdir(), 'agnostic-ai-'))
  try {
    const archive = path.join(work, asset)
    fs.writeFileSync(archive, await get(downloadUrl(version, asset)))
    await verifyChecksum(archive, asset, version)
    extract(archive, work)

    fs.mkdirSync(path.dirname(dest), { recursive: true })
    fs.copyFileSync(path.join(work, BINARY), dest)
    fs.chmodSync(dest, 0o755)
  } finally {
    fs.rmSync(work, { recursive: true, force: true })
  }

  log(`installed ${dest}`)
  return dest
}

module.exports = {
  assetName,
  binaryPath,
  describeError,
  downloadUrl,
  ensureBinary,
  extract,
  get,
  latestTag,
  resolveVersion,
  target,
}
