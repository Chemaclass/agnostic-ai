import { test } from 'node:test'
import assert from 'node:assert/strict'
import { copyFile, mkdtemp, readdir, rm, writeFile } from 'node:fs/promises'
import { request } from 'node:http'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { FIXTURES } from '../dev/fixtures.mjs'
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

test('undoing a completion uses the fixture clock and keeps closed or app-archived chats in Archived', async (t) => {
  const now = Date.parse('2026-11-07T15:30:00Z')
  const preview = await startPreview({ port: 0, fixture: 'completions', now })
  t.after(preview.close)

  const states = { 'recent-complete': 'active', 'closed-complete': 'dormant', 'cloud-complete': 'active', 'app-archive': 'dormant' }
  for (const [id, state] of Object.entries(states)) {
    const reply = await postJSON(preview.url + '/api/mark', { tool: 'claude', id, as: 'open' })
    assert.equal(reply.status, 204)
    const { body } = await inbox(preview.url)
    const row = body.view.conversations.find((row) => row.id === id)
    assert.equal(row.state, state, id)
    assert.equal(row.completedAt, undefined, id)
  }
})

test('completing a chat that only waits on a shell saves the mark and leaves it blocked', async (t) => {
  const now = Date.parse('2026-11-07T15:30:00Z')
  const preview = await startPreview({ port: 0, fixture: 'stuck-shells', now })
  t.after(preview.close)
  const chat = { tool: 'claude', id: 'shell-blocked' }

  assert.equal((await postJSON(preview.url + '/api/mark', { ...chat, as: 'completed' })).status, 204)
  const completed = (await inbox(preview.url)).body.view.conversations.find((row) => row.id === chat.id)
  assert.equal(completed.state, 'blocked')
  assert.equal(completed.completedAt, new Date(now).toISOString())

  assert.equal((await postJSON(preview.url + '/api/mark', { ...chat, as: 'open' })).status, 204)
  const afterUndo = (await inbox(preview.url)).body.view.conversations.find((row) => row.id === chat.id)
  assert.equal(afterUndo.state, 'blocked')
  assert.equal(afterUndo.completedAt, undefined)
})

test('stopping a chat archives the transcript and keeps it archived after Undo complete', async (t) => {
  const now = Date.parse('2026-11-07T15:30:00Z')
  const preview = await startPreview({ port: 0, fixture: 'stuck-shells', now })
  t.after(preview.close)
  const chat = { tool: 'claude', id: 'shell-blocked' }
  const readRow = async () => (await inbox(preview.url)).body.view.conversations.find((row) => row.id === chat.id)
  const { stoppable } = await readRow()
  assert.equal((await postJSON(preview.url + '/api/stop-session', chat)).status, 400)
  const stale = await postJSON(preview.url + '/api/stop-session', { ...chat, ...stoppable, procStart: 'another start' })
  assert.equal(stale.status, 409)
  assert.equal((await readRow()).live, true)
  assert.equal((await postJSON(preview.url + '/api/mark', { ...chat, as: 'completed' })).status, 204)

  assert.equal((await postJSON(preview.url + '/api/stop-session', { ...chat, ...stoppable })).status, 200)
  const stopped = await readRow()
  assert.equal(stopped.state, 'completed')
  assert.equal(stopped.stoppable, undefined)
  assert.equal(stopped.stuckShells, undefined)
  assert.equal(stopped.open.kind, 'terminal')
  assert.equal(stopped.open.label, 'Resume in Terminal')

  assert.equal((await postJSON(preview.url + '/api/mark', { ...chat, as: 'open' })).status, 204)
  assert.equal((await readRow()).state, 'dormant')
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

test('every fixture row opens the way the server reopens it', () => {
  const now = Date.parse('2026-11-07T15:30:00Z')
  for (const [name, fixture] of Object.entries(FIXTURES)) {
    for (const row of fixture.conversations(now)) {
      const where = `${name} ${row.id}`
      if (row.app === 'cloud') {
        assert.equal(row.open.kind, 'link', where)
        assert.equal(row.open.label, row.tool === 'codex' ? 'Open in Codex app' : 'Open on claude.ai', where)
        assert.ok(row.open.link, where)
        continue
      }
      assert.equal(row.open.kind, row.live ? 'focus' : 'terminal', where)
      if (!row.live) assert.equal(row.open.label, 'Resume in Terminal', where)
      assert.match(row.open.command, new RegExp(`^cd '/Users/you/[^']+' && \\S.* ${row.id}$`), where)
    }
  }
})

test('a search counts every chat that needs the person, not only its matches', async (t) => {
  const preview = await startPreview({ port: 0 })
  t.after(preview.close)
  const all = await inbox(preview.url)
  const found = await inbox(preview.url, 'login')
  assert.ok(all.body.view.needsYou > 0)
  assert.equal(found.body.view.needsYou, all.body.view.needsYou)
})

test('pages a search by time, newest first, and refuses a before that is not one, as serve does', async (t) => {
  const preview = await startPreview({ port: 0, fixture: 'paged-search' })
  t.after(preview.close)
  const ids = []
  let next
  do {
    const response = await fetch(preview.url + '/api/inbox?q=the' + (next ? '&before=' + encodeURIComponent(next) : ''))
    const { view } = await response.json()
    ids.push(...view.conversations.map((row) => row.id))
    next = view.next
    if (next) assert.match(next, /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/)
  } while (next)
  const found = await inbox(preview.url, 'the')
  assert.equal(ids.length, found.body.view.matches)
  assert.equal(new Set(ids).size, ids.length)
  for (const before of ['2', '2026-13-45T99:99:99Z', '2026-02-30T00:00:00Z', '2026-11-31T00:00:00Z', '2026-11-01T24:00:00Z', '2026-11-01T12:00:00+24:00']) {
    const refused = await fetch(preview.url + '/api/inbox?q=the&before=' + before)
    assert.equal(refused.status, 400, before)
    assert.match((await refused.json()).error, /^before must be a time/)
  }
  assert.equal((await fetch(preview.url + '/api/inbox?q=the&before=')).status, 200)
  for (const before of ['2028-02-29T00:00:00Z', '2026-11-01T12:00:00.5+02:00', '2026-11-30T23:59:59-05:30']) {
    assert.equal((await fetch(preview.url + '/api/inbox?q=the&before=' + encodeURIComponent(before))).status, 200, before)
  }
})

test('refuses a search longer than serve takes, as serve does', async (t) => {
  const preview = await startPreview({ port: 0 })
  t.after(preview.close)
  assert.equal((await inbox(preview.url, 'é'.repeat(200))).status, 200)
  assert.deepEqual(await inbox(preview.url, 'a'.repeat(201)), { status: 400, body: { error: 'the search is longer than 200 characters' } })
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
  assert.deepEqual((await menu.json()).map((item) => item.id), ['animate', 'dimWhenIdle', 'smallCompanion', 'startAtLogin', 'approvalsMode', 'approvalsCompanion', 'approveAgentSends', 'shortcut', 'sendShortcut', 'checkForUpdates', 'update', 'hide', 'quit'])
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

test('choosing a fixture drops a late open failure from the one before', async (t) => {
  const preview = await startPreview({ port: 0 })
  t.after(preview.close)
  assert.equal((await postJSON(preview.url + '/dev/fixture', { name: 'late-open-failure' })).status, 204)
  const opened = await postJSON(preview.url + '/api/open', { tool: 'claude', id: 'done-2', title: 'Tidy the old branches' })
  assert.equal(opened.status, 202)
  assert.deepEqual(await (await postJSON(preview.url + '/dev/settle-late-opens', {})).json(), { settled: 1 })
  assert.notEqual((await fetch(preview.url + '/api/open/late')).status, 204)

  assert.equal((await postJSON(preview.url + '/dev/fixture', { name: 'mixed' })).status, 204)
  assert.equal((await fetch(preview.url + '/api/open/late')).status, 204)
})

test('the forgotten shell fixture refuses stale identities and changes only the chosen shell', async (t) => {
  const preview = await startPreview({ port: 0, fixture: 'stuck-shells' })
  t.after(preview.close)
  const before = await inbox(preview.url)
  const row = before.body.view.conversations.find((row) => row.id === 'shells-working')
  const [first, other] = row.stuckShells
  const body = { tool: row.tool, id: row.id, pid: first.pid, procStart: first.procStart }
  const refused = await postJSON(preview.url + '/api/stop-shell', { ...body, procStart: 'stale' })
  assert.equal(refused.status, 409)

  const stopped = await postJSON(preview.url + '/api/stop-shell', body)
  assert.equal(stopped.status, 200)
  assert.ok(stopped.headers.get('List-Version'))
  assert.deepEqual(await stopped.json(), { stillRunning: [{ pid: first.pid + 1, procStart: first.procStart }] })
  const after = await inbox(preview.url)
  assert.deepEqual(after.body.view.conversations.find((row) => row.id === body.id).stuckShells, [other])
})

test('the preview approves, rejects and queues mailbox messages and keeps the approval switch', async (t) => {
  const preview = await startPreview({ port: 0, fixture: 'mailbox-delivery' })
  t.after(preview.close)
  const post = (path, body) => fetch(preview.url + path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
  const thread = 'th_20261107_00000005'
  assert.equal((await post('/api/mailbox/approve', { thread, n: 4, action: 'approve' })).status, 200)
  assert.equal((await post('/api/mailbox/approve', { thread, n: 5, action: 'reject' })).status, 200)
  assert.equal((await post('/api/mailbox/approve', { thread, n: 5, action: 'approve' })).status, 422)
  assert.equal((await post('/api/mailbox/queue', { thread, n: 2 })).status, 200)
  const reply = await (await fetch(preview.url + '/api/mailbox/threads?tool=codex&id=0199a2b4-5c1e-7d30-9f2a-3b6c8d1e4f50')).json()
  assert.deepEqual(reply.threads[0].messages.map(({ state }) => state), ['delivered', 'waiting', 'rejected', 'waiting', 'rejected'])
  assert.deepEqual(await (await fetch(preview.url + '/api/mailbox/settings')).json(), { approveAgentSends: true })
  assert.equal((await post('/api/mailbox/settings', { approveAgentSends: false })).status, 204)
  assert.equal((await post('/api/mailbox/settings', {})).status, 422)
  assert.deepEqual(await (await fetch(preview.url + '/api/mailbox/settings')).json(), { approveAgentSends: false })
})

test('the preview answers the update entry: none by default, a newer release in its fixture', async (t) => {
  const plain = await startPreview({ port: 0 })
  t.after(plain.close)
  assert.deepEqual(await (await fetch(plain.url + '/api/update')).json(), { state: 'unavailable' })

  const newer = await startPreview({ port: 0, fixture: 'update-available' })
  t.after(newer.close)
  assert.deepEqual(await (await fetch(newer.url + '/api/update')).json(), { state: 'available', version: '0.97.0', current: '0.96.0' })
  assert.equal((await fetch(newer.url + '/api/update', { method: 'POST' })).status, 204)
  assert.equal((await fetch(plain.url + '/api/update/start', { method: 'POST' })).status, 409)
  assert.equal((await fetch(newer.url + '/api/update/start', { method: 'POST' })).status, 204)
  assert.deepEqual(await (await fetch(newer.url + '/api/update')).json(), { state: 'updating', version: '0.97.0', current: '0.96.0', step: 'downloading' })
})

test('an answer to a held request needs its digest, then the request leaves the row', async (t) => {
  const preview = await startPreview({ port: 0, fixture: 'companion-approve' })
  t.after(preview.close)

  assert.equal((await postJSON(preview.url + '/api/approvals/decide', { id: 'a1b2', digest: 'other', decision: 'approve' })).status, 409)
  assert.equal((await postJSON(preview.url + '/api/approvals/decide', { id: 'none', digest: 'digest-none', decision: 'approve' })).status, 404)
  assert.equal((await postJSON(preview.url + '/api/approvals/decide', { id: 'a1b2', digest: 'digest-a1b2', decision: 'always' })).status, 400)

  const answered = await postJSON(preview.url + '/api/approvals/decide', { id: 'c3d4', digest: 'digest-c3d4', decision: 'deny' })
  assert.equal(answered.status, 200)
  const { body } = await inbox(preview.url)
  const edit = body.view.conversations.find((row) => row.id === 'held-edit')
  assert.deepEqual(edit.approvals.pending.map((p) => p.id), ['e5f6'])
  assert.equal(body.view.conversations.find((row) => row.id === 'held-bash').approvals.pending.length, 1)
})

test('the claim fixtures send row path counts and the footer totals the server sends', async (t) => {
  const preview = await startPreview({ port: 0, fixture: 'mailbox-claims' })
  t.after(preview.close)
  const { view } = (await inbox(preview.url)).body
  assert.deepEqual(view.mailboxClaims, { paths: 3, stale: 0, conflicts: 0 })
  assert.deepEqual(view.conversations.map((row) => row.claimedPaths), [2, 1])

  await postJSON(preview.url + '/dev/fixture', { name: 'mailbox-stale-claims' })
  assert.deepEqual((await inbox(preview.url)).body.view.mailboxClaims, { paths: 2, stale: 2, conflicts: 0 })

  await postJSON(preview.url + '/dev/fixture', { name: 'mailbox' })
  assert.equal((await inbox(preview.url)).body.view.mailboxClaims, undefined)
})

test('the conflict fixture puts the conflict on the holder and the requester rows until it is dismissed', async (t) => {
  const preview = await startPreview({ port: 0, fixture: 'mailbox-claim-conflict' })
  t.after(preview.close)
  const conflicted = async () => (await inbox(preview.url)).body.view.conversations.filter((row) => row.claimConflicts?.length).map((row) => row.title)
  assert.deepEqual(await conflicted(), ['Review the parser', 'Draft the tests'])
  assert.deepEqual((await inbox(preview.url)).body.view.mailboxClaims, { paths: 1, stale: 0, conflicts: 1 })
  const ask = { tool: 'claude', id: '84fc6d21-820c-4c88-9729-ff8ee40de1f3', worktree: '/Users/you/Code/agnostic-ai', path: 'internal/mailbox/handoff.go' }
  assert.equal((await postJSON(preview.url + '/api/mailbox/conflict/dismiss', ask)).status, 200)
  assert.deepEqual(await conflicted(), [])
})
