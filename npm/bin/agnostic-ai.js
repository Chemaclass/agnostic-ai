#!/usr/bin/env node
'use strict'

const { spawnSync } = require('node:child_process')
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
  // A signalled child reports null status; 128+signal is the shell convention.
  process.exit(result.status === null ? 128 : result.status)
}

main()
