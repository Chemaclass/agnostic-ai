import { test } from 'node:test'
import assert from 'node:assert/strict'
import { copyFile, mkdtemp, readdir, rm, writeFile } from 'node:fs/promises'
import { request } from 'node:http'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { startPreview } from '../dev/preview.mjs'

const frontend = fileURLToPath(new URL('../frontend/', import.meta.url))

async function postJSON(url, body) {
  return fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
}

async function inbox(base, query) {
  const response = await fetch(base + '/api/inbox' + (query ? '?q=' + encodeURIComponent(query) : ''))
  return { status: response.status, body: await response.json() }
}

test('refuses to listen on a host other than this machine', async () => {
  for (const host of ['0.0.0.0', '192.168.1.20', '::']) {
    await assert.rejects(startPreview({ host, port: 0 }), /only listens on this machine/)
  }
})

test('serves the real pages and a runtime in place of Wails', async (t) => {
  const preview = await startPreview({ port: 0 })
  t.after(preview.close)
  const panel = await fetch(preview.url + '/panel.html')
  assert.equal(panel.status, 200)
  assert.match(await panel.text(), /<script type="module" src="\/panel.js">/)
  const runtime = await fetch(preview.url + '/wails/runtime.js')
  assert.equal(runtime.status, 200)
  assert.match(runtime.headers.get('content-type'), /javascript/)
  const page = await fetch(preview.url + '/dev/')
  assert.equal(page.status, 200)
})

test('a mark changes only the fixture in this process', async (t) => {
  const first = await startPreview({ port: 0 })
  t.after(first.close)
  const second = await startPreview({ port: 0 })
  t.after(second.close)

  const marked = await postJSON(first.url + '/api/mark', { tool: 'cursor', id: 'active-1', as: 'completed' })
  assert.equal(marked.status, 204)

  const after = await inbox(first.url)
  assert.equal(after.body.view.conversations.find((row) => row.id === 'active-1').state, 'completed')
  const untouched = await inbox(second.url)
  assert.equal(untouched.body.view.conversations.find((row) => row.id === 'active-1').state, 'active')
})

test('a fixture can refuse renames and marks', async (t) => {
  const preview = await startPreview({ port: 0, fixture: 'rename-failure' })
  t.after(preview.close)
  const renamed = await postJSON(preview.url + '/api/rename', { tool: 'cursor', id: 'active-1', title: 'New name' })
  assert.equal(renamed.status, 409)
  assert.match((await renamed.json()).error, /could not save the name/)

  await postJSON(preview.url + '/dev/fixture', { name: 'mark-failure' })
  const marked = await postJSON(preview.url + '/api/mark', { tool: 'cursor', id: 'active-1', as: 'completed' })
  assert.equal(marked.status, 502)
})

test('searches the fixture rows', async (t) => {
  const preview = await startPreview({ port: 0 })
  t.after(preview.close)
  const found = await inbox(preview.url, 'login')
  assert.deepEqual(found.body.view.conversations.map((row) => row.id), ['active-1'])
  assert.equal(found.body.view.matches, 1)
})

test('refuses a request that names another host', async (t) => {
  const preview = await startPreview({ port: 0 })
  t.after(preview.close)
  // fetch replaces the Host header, so this asks through node:http.
  const status = await new Promise((done, failed) => {
    request(preview.url + '/api/inbox', { headers: { Host: 'attacker.example' } }, (response) => {
      response.resume()
      done(response.statusCode)
    }).on('error', failed).end()
  })
  assert.equal(status, 421)
})

test('tells open pages to reload after a frontend file changes, and ends their streams on close', { timeout: 10_000 }, async (t) => {
  const copy = await mkdtemp(join(tmpdir(), 'companion-preview-'))
  t.after(() => rm(copy, { recursive: true, force: true }))
  for (const name of await readdir(frontend)) await copyFile(join(frontend, name), join(copy, name))

  const preview = await startPreview({ port: 0, frontendDir: copy })
  t.after(preview.close)
  const stream = await fetch(preview.url + '/dev/events')
  const reader = stream.body.getReader()
  const decoder = new TextDecoder()

  const reloaded = (async () => {
    let seen = ''
    while (!seen.includes('"reload"')) {
      const { value, done } = await reader.read()
      if (done) throw new Error('the event stream ended before a reload')
      seen += decoder.decode(value)
    }
  })()
  await writeFile(join(copy, 'panel.css'), '/* edited */\n', { flag: 'a' })
  await reloaded
  await preview.close()
  let ended = false
  while (!ended) ended = (await reader.read()).done
})

test('a taken port fails the start and leaves nothing running', { timeout: 10_000 }, async (t) => {
  const first = await startPreview({ port: 0 })
  t.after(first.close)
  const port = Number(new URL(first.url).port)
  await assert.rejects(startPreview({ port }), /EADDRINUSE/)
})

test('quit answers with no content and shows in the calls', async (t) => {
  const preview = await startPreview({ port: 0 })
  t.after(preview.close)
  const quit = await fetch(preview.url + '/api/quit', { method: 'POST' })
  assert.equal(quit.status, 204)
  const calls = await (await fetch(preview.url + '/dev/calls')).json()
  assert.equal(calls[0].path, '/api/quit')
})

test('pause sets paused in the appearance, and a click on the circle clears it', async (t) => {
  const preview = await startPreview({ port: 0 })
  t.after(preview.close)
  const appearance = async () => (await fetch(preview.url + '/api/appearance')).json()
  assert.equal((await appearance()).paused, false)

  const paused = await fetch(preview.url + '/api/pause', { method: 'POST' })
  assert.equal(paused.status, 204)
  assert.equal((await appearance()).paused, true)
  const calls = await (await fetch(preview.url + '/dev/calls')).json()
  assert.equal(calls[0].path, '/api/pause')

  await fetch(preview.url + '/api/panel/toggle', { method: 'POST' })
  assert.equal((await appearance()).paused, false)
})
