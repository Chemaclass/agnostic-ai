'use strict'

// Fetches the agnostic-ai release binary for the current platform. Shared by
// the postinstall hook and the bin shim, so a failed install still recovers on
// first run instead of leaving a broken command.

const { execFileSync } = require('node:child_process')
const crypto = require('node:crypto')
const fs = require('node:fs')
const https = require('node:https')
const os = require('node:os')
const path = require('node:path')

const REPO = 'Chemaclass/agnostic-ai'
const BINARY = process.platform === 'win32' ? 'agnostic-ai.exe' : 'agnostic-ai'

const PLATFORMS = { darwin: 'darwin', linux: 'linux', win32: 'windows' }
const ARCHS = { x64: 'amd64', arm64: 'arm64' }

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

function get(url) {
  return new Promise((resolve, reject) => {
    https
      .get(url, { headers: { 'user-agent': 'agnostic-ai-npm' } }, (res) => {
        // GitHub redirects release assets to a signed object-store URL.
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          res.resume()
          resolve(get(res.headers.location))
          return
        }
        if (res.statusCode !== 200) {
          res.resume()
          reject(new Error(`GET ${url} failed with HTTP ${res.statusCode}`))
          return
        }
        const chunks = []
        res.on('data', (c) => chunks.push(c))
        res.on('end', () => resolve(Buffer.concat(chunks)))
        res.on('error', reject)
      })
      .on('error', reject)
  })
}

// The published package carries the release version; a checkout carries the
// 0.0.0-dev placeholder, which has no matching release, so fall back to latest.
async function resolveVersion() {
  if (process.env.AGNOSTIC_AI_VERSION) return process.env.AGNOSTIC_AI_VERSION

  const { version } = require('../package.json')
  if (version && !version.startsWith('0.0.0')) return `v${version}`

  const body = await get(`https://api.github.com/repos/${REPO}/releases/latest`)
  const tag = JSON.parse(body.toString('utf8')).tag_name
  if (!tag) throw new Error('could not resolve the latest agnostic-ai release')
  return tag
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

    // bsdtar reads both tar.gz and zip, and ships with macOS and Windows 10
    // 1803+; Linux only ever gets the tar.gz here, so GNU tar is fine too.
    execFileSync('tar', ['-xf', archive, '-C', work, BINARY], { stdio: 'ignore' })

    fs.mkdirSync(path.dirname(dest), { recursive: true })
    fs.copyFileSync(path.join(work, BINARY), dest)
    fs.chmodSync(dest, 0o755)
  } finally {
    fs.rmSync(work, { recursive: true, force: true })
  }

  log(`installed ${dest}`)
  return dest
}

module.exports = { assetName, binaryPath, downloadUrl, ensureBinary, resolveVersion, target }
