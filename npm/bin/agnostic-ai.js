#!/usr/bin/env node
'use strict'

const { spawnSync } = require('node:child_process')
const os = require('node:os')
const { entryPoint, packageName, platformFor } = require('../lib/platforms')

const DOCS = 'https://agnostic-ai.org/docs/installation/'

// The binary ships inside a platform package that npm installed as an optional
// dependency, so finding it is a module lookup, not a download. Nothing here
// touches the network.
function resolveBinary() {
  // Escape hatch for a binary this package does not ship: a local build, an
  // unsupported CPU, an air-gapped machine that already has one on disk.
  const override = process.env.AGNOSTIC_AI_BINARY
  if (override) return override

  const platform = platformFor(process.platform, process.arch)
  if (!platform) {
    throw new Error(
      `no prebuilt binary for ${process.platform}/${process.arch}. Build one with ` +
        '`go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest` and point ' +
        `AGNOSTIC_AI_BINARY at it, or pick another route: ${DOCS}`
    )
  }

  try {
    return require.resolve(entryPoint(platform))
  } catch {
    // npm reuses a lockfile written on another platform without re-resolving
    // optional dependencies, which leaves the matching package absent
    // (npm/cli#4828). `--no-optional` and `--omit=optional` do the same on
    // purpose. Both look like a successful install until the CLI runs.
    throw new Error(
      `${packageName(platform)} is not installed, so there is no binary to run. npm installs ` +
        'it as an optional dependency; a lockfile copied from another platform or an install ' +
        'run with --omit=optional skips it. Reinstall with `npm install agnostic-ai --force`, ' +
        `or set AGNOSTIC_AI_BINARY to a binary you already have. Other routes: ${DOCS}`
    )
  }
}

function main() {
  let binary
  try {
    binary = resolveBinary()
  } catch (err) {
    console.error(`agnostic-ai: ${err.message}`)
    process.exit(1)
  }

  const result = spawnSync(binary, process.argv.slice(2), { stdio: 'inherit' })
  if (result.error) {
    console.error(`agnostic-ai: ${result.error.message}`)
    process.exit(1)
  }
  // A signalled child reports a null status; 128+signal is the shell
  // convention, so a caller can tell a SIGINT (130) from a SIGKILL (137).
  if (result.status === null) {
    process.exit(128 + (os.constants.signals[result.signal] || 0))
  }
  process.exit(result.status)
}

main()
