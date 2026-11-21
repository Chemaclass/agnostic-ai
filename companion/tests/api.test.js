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

test('mailbox requests keep the conversation, action and stage confirmation fields', async () => {
  const { sendMessage, readThreads, changeThread, stageMessage } = await import('../frontend/api.js')
  const from = { tool: 'claude', id: 'a' }
  const to = { id: 'b' }
  const calls = []
  globalThis.fetch = async (path, options) => {
    calls.push([path, options?.method ?? 'GET', options?.body && JSON.parse(options.body)])
    return new Response('{}', { headers: { 'List-Version': '"v2"' } })
  }
  await readThreads(from)
  await sendMessage({ from, to, type: 'question', body: 'Is it ready?', thread: 'th_1' })
  await changeThread('th_1', 'pause')
  await stageMessage('th_1', 2, 'copy')
  assert.deepEqual(await stageMessage('th_1', 2, 'copy', { confirm: true }), { body: {}, version: '"v2"' })
  assert.deepEqual(calls, [
    ['/api/mailbox/threads?tool=claude&id=a', 'GET', undefined],
    ['/api/mailbox/send', 'POST', { from, to, type: 'question', body: 'Is it ready?', thread: 'th_1' }],
    ['/api/mailbox/thread', 'POST', { thread: 'th_1', action: 'pause' }],
    ['/api/mailbox/stage', 'POST', { thread: 'th_1', n: 2, how: 'copy' }],
    ['/api/mailbox/stage', 'POST', { thread: 'th_1', n: 2, how: 'copy', confirm: true }],
  ])
})

test('keeps the server’s mailbox guard alongside its explanation', async () => {
  const { sendMessage } = await import('../frontend/api.js')
  globalThis.fetch = async () => new Response(JSON.stringify({ error: 'Resume this thread first.', guard: 'paused' }), { status: 422 })
  await assert.rejects(sendMessage({}), { message: 'Resume this thread first.', guard: 'paused' })
})
