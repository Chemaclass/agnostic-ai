'use strict'

const fs = require('node:fs')
const path = require('node:path')
const { execFileSync } = require('node:child_process')
const { PLATFORMS, platformFor, packageName, binaryName } = require('../npm/lib/platforms')

function verifyTarballs(work, version) {
  const names = ['agnostic-ai', ...PLATFORMS.map(packageName)]
  for (const name of names) {
    const filename = `${name.replace(/^@/, '').replace('/', '-')}-${version}.tgz`
    const file = path.join(work, filename)
    if (!fs.existsSync(file)) throw new Error(`missing local tarball: ${filename}`)
    const manifest = JSON.parse(execFileSync('tar', ['-xOf', file, 'package/package.json'], { encoding: 'utf8' }))
    if (manifest.name !== name || manifest.version !== version) throw new Error(`${filename}: package identity mismatch`)
    if (name === 'agnostic-ai') {
      const pins = manifest.optionalDependencies || {}
      if (Object.keys(pins).length !== PLATFORMS.length || PLATFORMS.some(p => pins[packageName(p)] !== version)) {
        throw new Error(`${filename}: platform pins do not match local tarballs`)
      }
    }
  }
}

function verifyInstalled(work, version) {
  const platform = platformFor(process.platform, process.arch)
  if (!platform) throw new Error('unsupported smoke-test platform')
  const modules = path.join(work, 'project', 'node_modules')
  for (const name of ['agnostic-ai', packageName(platform)]) {
    const manifest = JSON.parse(fs.readFileSync(path.join(modules, name, 'package.json'), 'utf8'))
    if (manifest.name !== name || manifest.version !== version) throw new Error('installed local package identity mismatch')
  }
  const installed = fs.readFileSync(path.join(modules, packageName(platform), binaryName(platform)))
  const built = fs.readFileSync(path.join(work, 'binaries', `${platform.goos}_${platform.goarch}`, binaryName(platform)))
  if (!installed.equals(built)) throw new Error('installed binary differs from the local build')
}

module.exports = { verifyTarballs, verifyInstalled }

if (require.main === module) {
  try {
    const [mode, work, version] = process.argv.slice(2)
    if (mode === 'tarballs') verifyTarballs(work, version)
    else if (mode === 'installed') verifyInstalled(work, version)
    else throw new Error('expected tarballs or installed verification')
  } catch (error) {
    console.error(`npm-smoke: ${error.message}`)
    process.exitCode = 1
  }
}
