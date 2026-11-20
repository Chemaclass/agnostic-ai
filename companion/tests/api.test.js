import { test } from 'node:test'
import assert from 'node:assert/strict'
import { CallError, GivenUp, readInbox, stopShell } from '../frontend/api.js'

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

test('stops the selected shell by process identity and returns any remaining processes', async () => {
  const body = { stillRunning: [{ pid: 91, procStart: 'child-start' }] }
  globalThis.fetch = async (path, options) => {
    assert.equal(path, '/api/stop-shell')
    assert.equal(options.method, 'POST')
    assert.deepEqual(JSON.parse(options.body), { tool: 'claude', id: 'session', pid: 90, procStart: 'shell-start' })
    return new Response(JSON.stringify(body), { headers: { 'List-Version': '"v2"' } })
  }
  assert.deepEqual(await stopShell({ tool: 'claude', id: 'session' }, { pid: 90, procStart: 'shell-start', command: 'npm run dev' }), { body, version: '"v2"' })
})
