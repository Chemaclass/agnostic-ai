'use strict'

// Downloads the binary at install time. A failure here is a warning, never an
// install error: the bin shim retries on first run, so an offline install or a
// blocked proxy does not break `npm install` for the whole project.

const { ensureBinary } = require('../lib/download')

ensureBinary({ log: (m) => console.log(`agnostic-ai: ${m}`) }).catch((err) => {
  console.warn(`agnostic-ai: ${err.message}`)
  console.warn('agnostic-ai: the binary will be fetched on first run instead')
})
