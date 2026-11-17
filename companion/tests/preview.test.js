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

test('sends the copied text only while it is the text the page showed', async (t) => {
  const preview = await startPreview({ port: 0 })
  t.after(preview.close)
  const copied = await (await fetch(preview.url + '/api/clipboard')).json()
  assert.equal(copied.words, 14)
  assert.match(copied.hash, /^[0-9a-f]{64}$/)

  const sent = await postJSON(preview.url + '/api/send', { tool: 'codex', id: 'reply-1', hash: copied.hash })
  assert.deepEqual(await sent.json(), { sent: true, message: 'Typed into the Ghostty tab that runs it. Press Enter there to send it.' })
  const switched = await postJSON(preview.url + '/api/send', { tool: 'cursor', id: 'active-1', hash: copied.hash })
  assert.deepEqual(await switched.json(), { sent: false, message: 'Switched to it. Paste with Cmd+V.' })

  await postJSON(preview.url + '/dev/clipboard', { text: 'something else' })
  const changed = await postJSON(preview.url + '/api/send', { tool: 'claude', id: 'waits-1', hash: copied.hash })
  assert.equal(changed.status, 409)
})

test('a copied web page goes as Markdown, which only the send reply names', async (t) => {
  const preview = await startPreview({ port: 0, fixture: 'markdown-clipboard' })
  t.after(preview.close)
  const copied = await (await fetch(preview.url + '/api/clipboard')).json()
  assert.equal(copied.format, undefined)

  const sent = await postJSON(preview.url + '/api/send', { tool: 'codex', id: 'reply-1', hash: copied.hash })
  assert.deepEqual(await sent.json(), { sent: true, message: 'Typed the Markdown into the Ghostty tab that runs it. Press Enter there to send it.' })
})

test('a fixture with nothing copied describes an empty clipboard', async (t) => {
  const preview = await startPreview({ port: 0, fixture: 'empty-clipboard' })
  t.after(preview.close)
  assert.deepEqual(await (await fetch(preview.url + '/api/clipboard')).json(), { preview: '', words: 0, chars: 0, hash: '' })
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

test('a new message brings a chat marked read back to Needs you', async (t) => {
  const now = Date.parse('2026-11-07T15:30:00Z')
  const preview = await startPreview({ port: 0, fixture: 'needs-you', now })
  t.after(preview.close)
  assert.equal((await inbox(preview.url)).body.view.needsYou, 4)
  assert.equal((await postJSON(preview.url + '/api/mark', { tool: 'codex', id: 'reply-2', as: 'seen' })).status, 204)
  assert.equal((await inbox(preview.url)).body.view.needsYou, 3)

  assert.equal((await postJSON(preview.url + '/dev/write', { tool: 'codex', id: 'reply-2' })).status, 204)
  const after = await inbox(preview.url)
  const written = after.body.view.conversations.find((row) => row.id === 'reply-2')
  assert.equal(written.state, 'unread')
  assert.equal(written.updatedAt, new Date(now).toISOString())
  assert.equal(after.body.view.needsYou, 4)
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
  // The watcher can miss a write that lands before the system starts reporting changes.
  let seenReload = false
  reloaded.then(() => { seenReload = true }, () => {})
  while (!seenReload) {
    await writeFile(join(copy, 'panel.css'), '/* edited */\n', { flag: 'a' })
    await Promise.race([reloaded, new Promise((r) => setTimeout(r, 250))])
  }
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

test('hide sets the presence to hidden, and the shortcut shows it again', async (t) => {
  const preview = await startPreview({ port: 0 })
  t.after(preview.close)
  const appearance = async () => (await fetch(preview.url + '/api/appearance')).json()
  assert.equal((await appearance()).presence, 'shown')

  const hidden = await fetch(preview.url + '/api/hide', { method: 'POST' })
  assert.equal(hidden.status, 204)
  assert.equal((await appearance()).presence, 'hidden')
  const calls = await (await fetch(preview.url + '/dev/calls')).json()
  assert.equal(calls[0].path, '/api/hide')

  await fetch(preview.url + '/api/panel/toggle', { method: 'POST' })
  assert.equal((await appearance()).presence, 'shown')
})

test('a chat that starts waiting brings a hidden companion back faded; one already waiting does not', async (t) => {
  const preview = await startPreview({ port: 0 })
  t.after(preview.close)
  const appearance = async () => (await fetch(preview.url + '/api/appearance')).json()
  await fetch(preview.url + '/api/hide', { method: 'POST' })

  await fetch(preview.url + '/dev/arrive', { method: 'POST' })
  assert.equal((await appearance()).presence, 'faded')

  await fetch(preview.url + '/api/hide', { method: 'POST' })
  await fetch(preview.url + '/dev/arrive', { method: 'POST' })
  assert.equal((await appearance()).presence, 'hidden')
})

test('a list that asks to show itself while hidden stays hidden', async (t) => {
  const preview = await startPreview({ port: 0 })
  t.after(preview.close)
  await fetch(preview.url + '/api/hide', { method: 'POST' })

  const shown = await fetch(preview.url + '/api/panel/show', { method: 'POST' })

  assert.equal(shown.status, 204)
  assert.equal((await (await fetch(preview.url + '/api/appearance')).json()).presence, 'hidden')
})

test('the menu list is served with the pages', async (t) => {
  const preview = await startPreview({ port: 0 })
  t.after(preview.close)
  const menu = await fetch(preview.url + '/menu.json')
  assert.equal(menu.status, 200)
  assert.deepEqual((await menu.json()).map((item) => item.id), ['animate', 'dimWhenIdle', 'smallCompanion', 'startAtLogin', 'shortcut', 'sendShortcut', 'hide', 'quit'])
})

test('the list menu turns animation, dimming and the small circle on and off and answers the appearance', async (t) => {
  const preview = await startPreview({ port: 0 })
  t.after(preview.close)

  const animate = await postJSON(preview.url + '/api/appearance/animate', { enabled: false })
  assert.equal(animate.status, 200)
  assert.equal((await animate.json()).animate, false)
  const dim = await postJSON(preview.url + '/api/appearance/dim-when-idle', { enabled: true })
  assert.deepEqual(await dim.json(), { animate: false, dimWhenIdle: true, smallCompanion: false, presence: 'shown' })
  const small = await postJSON(preview.url + '/api/appearance/small-companion', { enabled: true })
  assert.deepEqual(await small.json(), { animate: false, dimWhenIdle: true, smallCompanion: true, presence: 'shown' })
  assert.deepEqual(await (await fetch(preview.url + '/api/appearance')).json(), { animate: false, dimWhenIdle: true, smallCompanion: true, presence: 'shown' })
})
