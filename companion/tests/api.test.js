import { test } from 'node:test'
import assert from 'node:assert/strict'
import { CallError, GivenUp, readInbox } from '../frontend/api.js'

function answering(json) {
  globalThis.fetch = async () => ({ status: 200, ok: true, json })
}

test('a search given up while its answer is read is given up, not a failure', async () => {
  const asked = new AbortController()
  answering(async () => {
    asked.abort()
    throw new DOMException('The operation was aborted.', 'AbortError')
  })
  await assert.rejects(readInbox('release', undefined, asked.signal), GivenUp)
})

test('an answer that breaks off while it is read is a failed call', async () => {
  answering(async () => {
    throw new TypeError('network error')
  })
  await assert.rejects(readInbox('release'), CallError)
})
