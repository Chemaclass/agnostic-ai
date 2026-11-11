#!/usr/bin/env node
// Serves the companion's real pages in a browser with fixture data, and reloads
// them when a file under companion/frontend changes. Development only: it lives
// outside the embedded frontend folder, so no build of the companion holds it.
//
//   node companion/dev/preview.mjs [--port 4320] [--fixture mixed] [--now 2026-11-07T15:30:00Z]

import { watch } from 'node:fs'
import { readFile } from 'node:fs/promises'
import { createServer } from 'node:http'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { FIXTURES, arrival } from './fixtures.mjs'

const here = resolve(fileURLToPath(new URL('.', import.meta.url)))
const FRONTEND = resolve(here, '../frontend')
const LOOPBACK = new Set(['127.0.0.1', 'localhost', '::1'])
const DEFAULT_PORT = 4320

const TYPES = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.json': 'application/json',
}

const NEEDS_PERSON = { blocked: true, unread: true }

/** The state one preview holds: the chosen fixture's rows and settings, changed only by its own calls. */
function fixtureState(name, now) {
  const fixture = FIXTURES[name]
  if (!fixture) throw new Error(`no fixture named ${name}; choose one of ${Object.keys(FIXTURES).join(', ')}`)
  return {
    name,
    fixture,
    conversations: fixture.conversations(now),
    preferences: { shortcut: 'Ctrl+Alt+Space', startAtLogin: false },
    appearance: { animate: true, dimWhenIdle: false },
  }
}

function viewOf(state, conversations, query) {
  const view = { conversations, needsYou: conversations.filter((row) => NEEDS_PERSON[row.state]).length }
  if (query !== undefined) view.matches = conversations.length
  if (state.fixture.cloudNote) view.cloudNote = state.fixture.cloudNote
  if (state.fixture.marksNote) view.marksNote = state.fixture.marksNote
  return view
}

function matches(row, words) {
  const text = [row.title, row.folder, row.branch].join(' ').toLowerCase()
  return words.every((word) => text.includes(word))
}

function slowness(state, query) {
  const slow = state.fixture.slowQueries ?? {}
  return Object.entries(slow).find(([word]) => query.includes(word))?.[1] ?? 0
}

function sendJSON(response, status, value) {
  response.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' })
  response.end(JSON.stringify(value))
}

function noContent(response) {
  response.writeHead(204, { 'Cache-Control': 'no-store' })
  response.end()
}

async function bodyOf(request) {
  const chunks = []
  let size = 0
  for await (const chunk of request) {
    size += chunk.length
    if (size > 64 << 10) throw new Error('the request is larger than the preview reads')
    chunks.push(chunk)
  }
  const text = Buffer.concat(chunks).toString('utf8')
  return text === '' ? {} : JSON.parse(text)
}

function hostAllowed(header) {
  if (!header) return false
  const name = header.startsWith('[') ? header.slice(1, header.indexOf(']')) : header.split(':')[0]
  return LOOPBACK.has(name)
}

/**
 * Starts a preview and resolves to {url, close}. host must name this machine.
 * frontendDir is the folder of pages to serve and watch; tests pass a copy.
 * now is the time the fixtures were written against, in milliseconds; tests fix it.
 */
export async function startPreview({ host = '127.0.0.1', port = DEFAULT_PORT, fixture = 'mixed', frontendDir = FRONTEND, now } = {}) {
  if (!LOOPBACK.has(host)) throw new Error(`the preview only listens on this machine (127.0.0.1, localhost or ::1), not ${host}`)
  const pages = resolve(frontendDir)
  const timeOf = () => now ?? Date.now()
  let state = fixtureState(fixture, timeOf())
  const listeners = new Set()

  function emit(name, data) {
    const line = `data: ${JSON.stringify({ name, data })}\n\n`
    for (const listener of listeners) listener.write(line)
  }

  /** The calls the pages made since the fixture was chosen, newest first, for the preview page. */
  let calls = []
  function logCall(path, body) {
    calls = [{ path, body }, ...calls].slice(0, 50)
  }

  // Opens a fixture holds, answered when a test releases them.
  let heldOpens = []

  function find(body) {
    return state.conversations.find((row) => row.tool === body.tool && row.id === body.id)
  }

  async function inbox(request, response, url) {
    if (state.fixture.listProblem) return sendJSON(response, 502, { error: state.fixture.listProblem })
    const query = url.searchParams.get('q')
    const note = state.fixture.note
    if (!query) return sendJSON(response, 200, { view: viewOf(state, state.conversations), note })
    const words = query.toLowerCase().split(/\s+/).filter(Boolean)
    const wait = slowness(state, query.toLowerCase())
    if (wait > 0) await new Promise((done) => setTimeout(done, wait))
    // The page gave this search up; a fixture chosen meanwhile answers with its own rows.
    if (response.destroyed) return
    sendJSON(response, 200, { view: viewOf(state, state.conversations.filter((row) => matches(row, words)), query), note })
  }

  async function mark(request, response) {
    const body = await bodyOf(request)
    logCall('/api/mark', body)
    if (state.fixture.markProblem) return sendJSON(response, 502, { error: state.fixture.markProblem })
    const row = find(body)
    if (!row) return sendJSON(response, 404, { error: 'That conversation is not in this fixture.' })
    if (body.as === 'hidden') state.conversations = state.conversations.filter((other) => other !== row)
    else if (body.as === 'completed') row.state = 'completed'
    else if (body.as === 'seen' && NEEDS_PERSON[row.state]) row.state = 'active'
    else return sendJSON(response, 400, { error: `unknown mark ${body.as}` })
    noContent(response)
    emit('inbox')
  }

  async function rename(request, response) {
    const body = await bodyOf(request)
    logCall('/api/rename', body)
    if (state.fixture.renameProblem) return sendJSON(response, 409, { error: state.fixture.renameProblem })
    const row = find(body)
    if (!row) return sendJSON(response, 404, { error: 'That conversation is not in this fixture.' })
    row.title = body.title
    noContent(response)
    emit('inbox')
  }

  async function open(request, response) {
    // A held open answers from the fixture it started under, even if another is chosen meanwhile.
    const fixture = state.fixture
    const body = await bodyOf(request)
    logCall('/api/open', body)
    if (fixture.heldOpens?.includes(body.id)) {
      // A page that closes first, such as a test that failed, leaves nothing held for the next one.
      await new Promise((release) => {
        heldOpens.push(release)
        response.on('close', () => {
          heldOpens = heldOpens.filter((held) => held !== release)
          release()
        })
      })
    }
    if (response.destroyed) return
    if (fixture.openProblem) return sendJSON(response, 200, { opened: false, message: fixture.openProblem, command: fixture.openCommand })
    const row = find(body)
    if (row && NEEDS_PERSON[row.state]) row.state = 'active'
    sendJSON(response, 200, { opened: true })
    emit('inbox')
  }

  function events(request, response) {
    response.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-store', Connection: 'keep-alive' })
    response.write(': connected\n\n')
    listeners.add(response)
    request.on('close', () => listeners.delete(response))
  }

  async function chooseFixture(request, response) {
    const body = await bodyOf(request)
    state = fixtureState(body.name, timeOf())
    calls = []
    noContent(response)
    emit('inbox')
  }

  async function serveFile(response, folder, name) {
    const path = resolve(folder, '.' + sep + name)
    if (path !== folder && !path.startsWith(folder + sep)) return sendJSON(response, 404, { error: 'not found' })
    let content
    try {
      content = await readFile(path)
    } catch {
      return sendJSON(response, 404, { error: 'not found' })
    }
    response.writeHead(200, { 'Content-Type': TYPES[extname(path)] ?? 'application/octet-stream', 'Cache-Control': 'no-store' })
    response.end(content)
  }

  const routes = {
    'GET /api/inbox': inbox,
    'POST /api/open': open,
    'POST /api/mark': mark,
    'POST /api/rename': rename,
    'POST /api/panel/toggle': (request, response) => { logCall('/api/panel/toggle'); noContent(response); emit('panel-shown', 'bottom-right') },
    'POST /api/panel/hide': (request, response) => { logCall('/api/panel/hide'); noContent(response) },
    'POST /api/panel/show': (request, response) => { logCall('/api/panel/show'); noContent(response); emit('panel-shown', 'bottom-right') },
    'GET /api/preferences': (request, response) => sendJSON(response, 200, state.preferences),
    'POST /api/preferences/shortcut': async (request, response) => {
      state.preferences.shortcut = (await bodyOf(request)).shortcut
      sendJSON(response, 200, state.preferences)
    },
    'POST /api/preferences/start-at-login': async (request, response) => {
      state.preferences.startAtLogin = (await bodyOf(request)).enabled === true
      sendJSON(response, 200, state.preferences)
    },
    'GET /api/appearance': (request, response) => sendJSON(response, 200, state.appearance),
    'POST /api/dashboard': (request, response) => { logCall('/api/dashboard'); noContent(response) },
    'POST /api/hide': (request, response) => { logCall('/api/hide'); noContent(response) },
    'GET /wails/runtime.js': (request, response) => serveFile(response, here, 'runtime-shim.js'),
    'GET /dev/': (request, response) => serveFile(response, here, 'preview.html'),
    'GET /dev/events': events,
    'GET /dev/fixtures': (request, response) =>
      sendJSON(response, 200, { chosen: state.name, fixtures: Object.entries(FIXTURES).map(([name, { label }]) => ({ name, label })) }),
    'GET /dev/calls': (request, response) => sendJSON(response, 200, calls),
    'POST /dev/fixture': chooseFixture,
    'POST /dev/release-opens': (request, response) => {
      const released = heldOpens
      heldOpens = []
      for (const release of released) release()
      sendJSON(response, 200, { released: released.length })
    },
    'POST /dev/arrive': (request, response) => {
      state.conversations = [arrival(timeOf()), ...state.conversations.filter((row) => row.id !== 'arrived-1')]
      noContent(response)
      emit('inbox')
    },
  }

  const server = createServer(async (request, response) => {
    // A page on another site that names this port through its own host name gets nothing.
    if (!hostAllowed(request.headers.host)) return sendJSON(response, 421, { error: 'the preview answers only on this machine' })
    const url = new URL(request.url, 'http://preview')
    const route = routes[`${request.method} ${url.pathname}`]
    try {
      if (route) return await route(request, response, url)
      if (request.method !== 'GET') return sendJSON(response, 405, { error: 'not a call the companion makes' })
      await serveFile(response, pages, url.pathname === '/' ? 'index.html' : url.pathname.slice(1))
    } catch (error) {
      if (!response.headersSent) sendJSON(response, 400, { error: error.message })
      else response.end()
    }
  })

  await new Promise((listening, failed) => {
    server.once('error', failed)
    server.listen(port, host, listening)
  })

  let reloadTimer
  // Saving a file writes it in several steps; one reload follows the last of them.
  const watcher = watch(pages, () => {
    clearTimeout(reloadTimer)
    reloadTimer = setTimeout(() => emit('reload'), 50)
  })
  const address = server.address()
  const shownHost = address.family === 'IPv6' ? `[${address.address}]` : address.address

  let closed
  async function close() {
    closed ??= new Promise((done) => {
      clearTimeout(reloadTimer)
      watcher.close()
      for (const listener of listeners) listener.end()
      listeners.clear()
      server.close(() => done())
      server.closeAllConnections()
    })
    return closed
  }

  return { url: `http://${shownHost}:${address.port}`, close }
}

function argumentsOf(argv) {
  const options = {}
  for (let at = 0; at < argv.length; at += 1) {
    const [flag, inline] = argv[at].split('=', 2)
    const value = () => inline ?? argv[(at += 1)]
    if (flag === '--host') options.host = value()
    else if (flag === '--port') options.port = Number(value())
    else if (flag === '--fixture') options.fixture = value()
    else if (flag === '--now') options.now = Date.parse(value())
    else throw new Error(`unknown argument ${argv[at]}; use --host, --port, --fixture or --now`)
    if (Number.isNaN(options.now)) throw new Error('--now takes a time such as 2026-11-07T15:30:00Z')
  }
  return options
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const preview = await startPreview(argumentsOf(process.argv.slice(2)))
    console.log(`Companion preview at ${preview.url}/dev/`)
    console.log(`Fixtures: ${Object.keys(FIXTURES).join(', ')}`)
    console.log('Edits under companion/frontend reload the pages. Press Ctrl+C to stop.')
    for (const signal of ['SIGINT', 'SIGTERM']) {
      process.once(signal, async () => {
        await preview.close()
        process.exit(0)
      })
    }
  } catch (error) {
    console.error(error.message)
    process.exit(1)
  }
}
