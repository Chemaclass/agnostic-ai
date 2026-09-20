#!/usr/bin/env node
'use strict'

const { spawnSync } = require('node:child_process')
const os = require('node:os')
const { ensureBinary } = require('../lib/download')

async function main() {
  let binary
  try {
    // Normally already on disk from postinstall; this covers an install that
    // ran with --ignore-scripts or without network.
    binary = await ensureBinary({ log: (m) => console.error(`agnostic-ai: ${m}`) })
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
