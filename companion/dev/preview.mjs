#!/usr/bin/env node
// Serves the companion's real pages in a browser with fixture data, and reloads
// them when a file under companion/frontend changes. Development only: it lives
// outside the embedded frontend folder, so no build of the companion holds it.
//
//   node companion/dev/preview.mjs [--port 4320] [--fixture mixed] [--now 2026-11-07T15:30:00Z]

import { createHash } from 'node:crypto'
import { watch } from 'node:fs'
import { readFile } from 'node:fs/promises'
import { createServer } from 'node:http'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { FIXTURES, arrival } from './fixtures.mjs'
import { mailboxAnswer, mailboxClaimTotals, mailboxDismiss, mailboxApprove, mailboxChange, mailboxClaim, mailboxClaims, mailboxQueue, mailboxRefs, mailboxRelease, mailboxRows, mailboxSaveSettings, mailboxSend, mailboxStage, mailboxThreads } from './mailbox.mjs'

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
const DAY_MS = 24 * 60 * 60 * 1000
const OPEN_UNTIL_ARCHIVED = new Set(['desktop', 'chat', 'cowork', 'cloud'])

/** The state one preview holds: the chosen fixture's rows and settings, changed only by its own calls. */
function fixtureState(name, now) {
  const fixture = FIXTURES[name]
  if (!fixture) throw new Error(`no fixture named ${name}; choose one of ${Object.keys(FIXTURES).join(', ')}`)
  const conversations = fixture.conversations(now)
  return {
    name,
    fixture,
    conversations,
    mailbox: fixture.mailbox?.(now) ?? [],
    claims: fixture.mailboxClaims?.(now) ?? [],
    claimConflicts: fixture.claimConflicts?.() ?? [],
    conflictSeq: 0,
    mailboxSettings: { approveAgentSends: fixture.approveAgentSends ?? true },
    knownClosed: new Set(fixture.knownClosed),
    /** The list as the shell last polled it, for a fixture whose shell reads it again only after rereadMs. */
    polled: structuredClone(conversations),
    preferences: { shortcut: 'Ctrl+Alt+Space', sendShortcut: 'Ctrl+Alt+V', startAtLogin: false, checkForUpdates: true },
    appearance: { animate: true, dimWhenIdle: false, smallCompanion: false, presence: 'shown' },
    /** The stuck shells kept running, as the shell keeps them in its settings file. */
    keptShells: [],
    /** The update entry; a terminal install has none yet. */
    update: fixture.update ?? { state: 'unavailable' },
    /** What waited in the last list, as the shell remembers it. */
    waitingSeen: waitingIn(conversations),
    /** What the preview's clipboard holds, as the shell would read it. */
    clipboard: fixture.clipboard ?? COPIED,
    /** Whether the copied text came as HTML the server turns into Markdown. */
    copiedMarkdown: Boolean(fixture.clipboardMarkdown),
    /** The approvals mode the header names; Ask me clears it. */
    approvalsMode: fixture.approvalsMode,
    /** The home approvals settings the menu shows; the hook counts as installed in a fixture that starts loose. */
    approvals: { mode: fixture.approvalsMode ?? 'ask', companion: Boolean(fixture.approvalsCompanion), hookInstalled: Boolean(fixture.approvalsMode) },
  }
}

const SETTINGS_ROOT = '/Users/you/.agnostic-ai'
const SYNC_FILES = [
  { path: '/Users/you/.claude/settings.json', change: 'update', target: 'claude' },
  { path: '/Users/you/.codex/hooks.json', change: 'create', target: 'codex' },
]
const CODEX_TRUST = 'Codex will show Hooks need review once. Open /hooks in Codex and trust it.'

/** The settings document as serve answers GET /api/views/settings, for the preview's own state. */
function settingsDocOf(state) {
  const { mode, companion, hookInstalled } = state.approvals
  const reason = 'the hook is not installed; add approvals to builtins in the home config and run sync --scope user'
  const loose = mode !== 'ask' || companion
  const tools = ['claude', 'codex'].map((tool) => ({
    tool,
    mode,
    effective: mode === 'ask' || hookInstalled ? mode : 'ask',
    from: mode === 'ask' ? 'default' : 'approvals.mode',
    ...(mode !== 'ask' && !hookInstalled ? { reason } : {}),
  }))
  // Like serve: a looser mode, or the companion buttons, need the hook a sync installs.
  const reasons = loose && !hookInstalled ? tools.map((tool) => `${tool.tool}: ${reason}`) : []
  return {
    schemaVersion: 'settings/v1',
    configRoot: SETTINGS_ROOT,
    approvals: { mode, modeFile: mode === 'ask' && !companion ? '' : 'local/agnostic-ai.yaml', companion, overrides: { tools: 0, projects: 0, alwaysAsk: 0 }, shadowed: [], tools },
    mailbox: state.mailboxSettings,
    sync: { needed: reasons.length > 0, reasons },
    canLoosen: true,
  }
}

function settingsTagOf(doc) {
  return `"${createHash('sha256').update(JSON.stringify(doc)).digest('hex').slice(0, 32)}"`
}

/** Three copied lines, what most fixtures hold on the clipboard. */
const COPIED = 'Fix the flaky login test.\nThen run the whole suite.\nReport what still fails.'

const TOOL_LABELS = { claude: 'Claude Code', codex: 'Codex', cursor: 'Cursor', chatgpt: 'ChatGPT' }

/** Like the shell: the copied text without all of it. */
function clipboardOf(text) {
  if (text.trim() === '') return { preview: '', words: 0, chars: 0, hash: '' }
  const chars = [...text]
  return {
    preview: chars.slice(0, 200).join(''),
    words: text.split(/\s+/).filter(Boolean).length,
    chars: chars.length,
    hash: createHash('sha256').update(text).digest('hex'),
  }
}

/** Like the server: typed into a live terminal session, else switched to with a paste to make. */
function sendReplyOf(row, markdown = false) {
  if (row.app === 'cloud' || row.tool === 'cursor') return { sent: false, message: 'Switched to it. Paste with Cmd+V.' }
  if (row.state === 'blocked') {
    return { sent: false, message: 'It waits for your answer, and typed text could answer its question, so nothing was typed. Switched to it. Paste with Cmd+V.' }
  }
  if (row.state === 'working' && row.tool !== 'claude') {
    return { sent: false, message: `${TOOL_LABELS[row.tool]} is working and may drop typed text, so nothing was typed. Switched to it. Paste with Cmd+V.` }
  }
  return { sent: true, message: `Typed${markdown ? ' the Markdown' : ''} into the Ghostty tab that runs it. Press Enter there to send it.` }
}

function waitingIn(conversations) {
  return new Set(conversations.filter((row) => NEEDS_PERSON[row.state]).map((row) => `${row.tool}:${row.id}`))
}

/** A page of the list; like the server, needsYou counts every chat, not only the search's matches. */
function viewOf(state, conversations, query, all = conversations) {
  const conflicted = (row) => state.claimConflicts.some(({ requester, holder }) =>
    [requester, holder].some((ref) => ref?.tool === row.tool && ref?.id === row.id))
  const view = { conversations: mailboxRows(state, conversations), needsYou: all.filter((row) => NEEDS_PERSON[row.state] || conflicted(row)).length }
  if (query !== undefined) view.matches = conversations.length
  if (state.fixture.cloudNote) view.cloudNote = state.fixture.cloudNote
  if (state.fixture.marksNote) view.marksNote = state.fixture.marksNote
  if (state.fixture.mailboxWarning) view.mailboxWarning = state.fixture.mailboxWarning
  const claims = mailboxClaimTotals(state)
  if (claims) view.mailboxClaims = claims
  if (state.approvalsMode) view.approvalsMode = state.approvalsMode
  return view
}

function weekViewOf(state, rows, before, now) {
  const first = before === undefined
  const onFirst = (row) => row.live || NEEDS_PERSON[row.state] || Boolean(row.pinnedAt)
  const to = first ? now : Date.parse(before)
  const week = 7 * DAY_MS
  const older = rows.filter((row) => !onFirst(row) && (first || Date.parse(row.updatedAt) < to))
  const newest = Math.max(...older.map((row) => Date.parse(row.updatedAt)))
  const from = (newest < to - week && older.length ? newest : to) - week
  const page = [...(first ? rows.filter(onFirst) : []), ...older.filter((row) => Date.parse(row.updatedAt) >= from)]
  page.sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt))
  const view = { ...viewOf(state, page, undefined, rows), from: new Date(from).toISOString(), to: new Date(to).toISOString() }
  if (older.some((row) => Date.parse(row.updatedAt) < from)) view.next = view.from
  return view
}

/** The longest search serve takes, in characters. */
const MAX_QUERY = 200

/** A time as the server writes and reads it: RFC 3339. */
const RFC3339 = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(\.\d+)?(?:Z|[+-](\d{2}):(\d{2}))$/

/** Whether text is a time Go's time.Parse(time.RFC3339) takes: Date.parse rolls 2026-02-30 over to March instead of refusing it. */
function isRFC3339(text) {
  const parts = RFC3339.exec(text)
  if (!parts) return false
  const [year, month, day, hour, minute, second, , offsetHour, offsetMinute] = parts.slice(1).map((part) => Number(part ?? 0))
  const days = new Date(Date.UTC(year, month, 0)).getUTCDate()
  return month >= 1 && month <= 12 && day >= 1 && day <= days && hour < 24 && minute < 60 && second < 60 && offsetHour < 24 && offsetMinute < 60
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
  /** Lists made so far, so each one has its own version even across fixtures. */
  let versions = 0
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

  /** The shortcut or a click brings a hidden or faded companion back at full color. */
  function comeBack() {
    if (state.appearance.presence === 'shown') return
    state.appearance.presence = 'shown'
    emit('appearance', state.appearance)
  }

  /** Like the shell: the pages read the new list, and a chat that starts waiting brings a hidden circle back faded. */
  function listChanged(polled = state.conversations) {
    state.polled = structuredClone(polled)
    versions += 1
    state.version = String(versions)
    emit('inbox', state.version)
    const now = waitingIn(state.conversations)
    const gained = [...now].some((key) => !state.waitingSeen.has(key))
    state.waitingSeen = now
    if (!gained || state.appearance.presence !== 'hidden') return
    state.appearance.presence = 'faded'
    emit('appearance', state.appearance)
  }

  // Opens a fixture holds, answered when a test releases them.
  let heldOpens = []
  // Like the shell: the failure of an open answered as pending, kept once it settles until it is dismissed.
  // A newer open drops it, and so does choosing a fixture, so one test's failure does not reach the next.
  let lateOpen = { waiting: undefined, settled: undefined }
  let opensMade = 0

  function find(body) {
    return state.conversations.find((row) => row.tool === body.tool && row.id === body.id)
  }

  async function inbox(request, response, url) {
    if (state.fixture.listProblem) return sendJSON(response, 502, { error: state.fixture.listProblem })
    // Like serve, an empty before asks for the first page.
    const before = url.searchParams.get('before') || undefined
    if (before !== undefined && !isRFC3339(before)) return sendJSON(response, 400, { error: 'before must be a time such as 2026-11-01T12:00:00Z, the next value of the previous page' })
    const query = url.searchParams.get('q')
    if (query && [...query].length > MAX_QUERY) return sendJSON(response, 400, { error: `the search is longer than ${MAX_QUERY} characters` })
    const note = state.fixture.note
    const version = state.version
    if (!query) {
      const rows = state.fixture.rereadMs ? state.polled : state.conversations
      const view = state.fixture.weekPages ? weekViewOf(state, rows, before, timeOf()) : viewOf(state, rows)
      return sendJSON(response, 200, { view, note, version })
    }
    const words = query.toLowerCase().split(/\s+/).filter(Boolean)
    // Like the server: a slow search answers with the rows as they were when it was asked.
    const asked = state
    const read = structuredClone(state.conversations.filter((row) => matches(row, words)))
    const paged = state.fixture.searchPages
    const wait = paged?.ms ?? slowness(state, query.toLowerCase())
    if (wait > 0) await new Promise((done) => setTimeout(done, wait))
    // The page gave this search up; a fixture chosen meanwhile answers with its own rows.
    if (response.destroyed) return
    const rows = state === asked ? read : state.conversations.filter((row) => matches(row, words))
    if (!paged) return sendJSON(response, 200, { view: viewOf(state, rows, query, state.conversations), note, version })
    // Like the server's weeks: before is the time the previous page stopped at, newest first, and matches counts every page.
    const older = rows.filter((row) => before === undefined || Date.parse(row.updatedAt) < Date.parse(before)).sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt))
    // A page ends between two times, so rows written at its last time all stay on it.
    let end = Math.min(paged.size, older.length)
    while (end < older.length && older[end].updatedAt === older[end - 1].updatedAt) end += 1
    const page = older.slice(0, end)
    const view = { ...viewOf(state, page, query, state.conversations), matches: rows.length }
    if (older.length > page.length) view.next = page.at(-1).updatedAt.replace(/\.\d+Z$/, 'Z')
    sendJSON(response, 200, { view, note, version })
  }

  async function mark(request, response, url) {
    const body = await bodyOf(request)
    logCall('/api/mark', body)
    if (state.fixture.markProblem) return sendJSON(response, 502, { error: state.fixture.markProblem })
    const row = find(body)
    if (!row) return sendJSON(response, 404, { error: 'That conversation is not in this fixture.' })
    // With marksIgnored the server keeps the mark, but the list it reads next shows the chat where it was.
    const marking = state.fixture.marksIgnored ? structuredClone(row) : row
    if (body.as === 'hidden') state.conversations = state.conversations.filter((other) => other !== marking)
    else if (body.as === 'pinned' && !row.parent) marking.pinnedAt ??= new Date(timeOf()).toISOString()
    else if (body.as === 'unpinned' && !row.parent) delete marking.pinnedAt
    else if (body.as === 'completed') {
      marking.completedAt = new Date(timeOf()).toISOString()
      delete marking.unread
      if (marking.waiting?.for !== 'shell' && !state.fixture.appArchived?.includes(marking.id)) marking.state = 'completed'
    } else if (body.as === 'open') {
      delete marking.completedAt
      if (marking.state === 'completed') {
        const knownClosed = state.knownClosed.has(marking.id)
        const active = marking.live || OPEN_UNTIL_ARCHIVED.has(marking.app) || timeOf() - Date.parse(marking.updatedAt) < DAY_MS
        marking.state = active && !knownClosed ? 'active' : 'dormant'
      }
    } else if (body.as === 'seen' && (NEEDS_PERSON[marking.state] || marking.unread)) {
      if (NEEDS_PERSON[marking.state]) marking.state = 'active'
      delete marking.unread
    }
    else return sendJSON(response, 400, { error: `unknown mark ${body.as}` })
    // Like the shell: a mark answers once the new list is read, after the event for it goes out.
    // A mark with read=later answers first and leaves the list to the next poll.
    const marked = state
    const read = structuredClone(state.conversations)
    if (url.searchParams.get('read') === 'later') {
      noContent(response)
      setTimeout(() => state === marked && listChanged(read))
      return
    }
    if (marked.fixture.rereadMs) await new Promise((done) => setTimeout(done, marked.fixture.rereadMs))
    if (state === marked) listChanged(read)
    response.setHeader('List-Version', state.version)
    noContent(response)
  }

  async function rename(request, response) {
    const body = await bodyOf(request)
    logCall('/api/rename', body)
    if (state.fixture.renameProblem) return sendJSON(response, 409, { error: state.fixture.renameProblem })
    const row = find(body)
    if (!row) return sendJSON(response, 404, { error: 'That conversation is not in this fixture.' })
    row.title = body.title
    listChanged()
    response.setHeader('List-Version', state.version)
    noContent(response)
  }

  async function remove(request, response) {
    const body = await bodyOf(request)
    logCall('/api/delete', body)
    if (state.fixture.deleteProblem) return sendJSON(response, 500, { error: state.fixture.deleteProblem, reason: 'failed' })
    const row = find(body)
    if (!row) return sendJSON(response, 404, { error: 'That conversation is not in this list any more. Refresh the page.' })
    if (row.state !== 'completed' && row.state !== 'dormant') {
      return sendJSON(response, 409, { error: 'Only a conversation under Archived can be deleted. This one is not there any more: something was written in it, or it was opened. Refresh the list.', reason: 'not-archived' })
    }
    if (row.deleteUnsupported) return sendJSON(response, 422, { error: row.deleteUnsupported, reason: 'unsupported' })
    state.conversations = state.conversations.filter((other) => other !== row)
    listChanged()
    response.setHeader('List-Version', state.version)
    sendJSON(response, 200, { deleted: true, freedBytes: 12_400_000, ...(state.fixture.deleteNote ? { note: state.fixture.deleteNote } : {}) })
  }

  async function stopSession(request, response) {
    const body = await bodyOf(request)
    logCall('/api/stop-session', body)
    if (typeof body?.id !== 'string' || !body.id.trim() ||
        !Number.isInteger(body.pid) || body.pid <= 1 || typeof body.procStart !== 'string' || !body.procStart.trim()) {
      return sendJSON(response, 400, { error: 'send {tool, id, pid, procStart}' })
    }
    const row = find(body)
    if (!row?.live || row.stoppable?.pid !== body.pid || row.stoppable?.procStart !== body.procStart) return sendJSON(response, 409, { error: 'this chat no longer runs in a terminal here' })
    state.knownClosed.add(row.id)
    for (const key of ['live', 'stoppable', 'activity', 'waiting', 'stuckShells', 'helpers', 'workingSubagents', 'unread', 'quietSince']) delete row[key]
    row.state = row.completedAt ? 'completed' : 'dormant'
    row.open = { ...row.open, kind: 'terminal', label: 'Resume in Terminal' }
    listChanged()
    response.setHeader('List-Version', state.version)
    sendJSON(response, 200, {})
  }

  async function stopShell(request, response) {
    const body = await bodyOf(request)
    logCall('/api/stop-shell', body)
    const row = find(body)
    const shell = row?.live && row.stuckShells?.find((shell) => shell.pid === body.pid && shell.procStart === body.procStart)
    if (!shell) return sendJSON(response, 409, { error: 'The shell is no longer in the list.' })
    row.stuckShells = row.stuckShells.filter((other) => other !== shell)
    if (row.stuckShells.length === 0) {
      delete row.stuckShells
      if (row.waiting?.for === 'shell') {
        delete row.waiting
        row.state = row.completedAt ? 'completed' : 'active'
      }
    }
    listChanged()
    response.setHeader('List-Version', state.version)
    sendJSON(response, 200, shell.pid === 840 ? { stillRunning: [{ pid: shell.pid + 1, procStart: shell.procStart }] } : {})
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
    lateOpen = { waiting: undefined, settled: undefined }
    opensMade += 1
    if (fixture.openPending) {
      if (fixture.lateOpenProblem) lateOpen.waiting = { id: opensMade, tool: body.tool, chat: body.id, sentTitle: body.title, message: fixture.lateOpenProblem, command: fixture.openCommand }
      return sendJSON(response, 202, { pending: true, message: 'Still opening. This takes longer than usual.' })
    }
    if (fixture.openProblem) return sendJSON(response, 200, { opened: false, message: fixture.openProblem, command: fixture.openCommand })
    const row = find(body)
    if (row && NEEDS_PERSON[row.state]) row.state = 'active'
    const picked = body.helper && row?.helpers?.find((helper) => helper.tool === 'claude' && helper.id === body.helper)
    sendJSON(response, 200, { opened: true, ...helperOpenReplyOf(body, picked) })
    listChanged()
  }

  /** What the server says when an open named a subagent: its pane, or the chat it runs in with a note. */
  function helperOpenReplyOf(body, picked) {
    if (!body.helper) return {}
    if (!picked) return { message: 'That subagent has finished. Its chat is open.', reached: 'parent' }
    if (picked.reach === 'pane') return { message: 'Switched to the Terminal tab that runs it.', helper: picked, reached: 'subagent' }
    const note = picked.reach === 'nested'
      ? `It runs inside '${picked.parentName}'. Open that row first.`
      : `Press the down arrow to pick '${picked.title}' below the prompt, then Enter.`
    return { message: `Switched to the Terminal tab that runs it. ${note}`, helper: picked, reached: 'parent' }
  }

  async function send(request, response) {
    const body = await bodyOf(request)
    logCall('/api/send', body)
    if (state.fixture.sendProblem) return sendJSON(response, 502, { error: state.fixture.sendProblem })
    const copied = clipboardOf(state.clipboard)
    if (copied.words === 0) return sendJSON(response, 409, { error: 'Nothing to send: copy some text first.' })
    if (body.hash !== copied.hash) return sendJSON(response, 409, { error: 'The copied text changed. Check the new text, then send again.' })
    const row = find(body)
    if (!row) return sendJSON(response, 404, { error: 'That conversation is not in this list any more. Refresh the page.' })
    if (state.fixture.unsent) return sendJSON(response, 200, { sent: false, message: state.fixture.unsent })
    sendJSON(response, 200, sendReplyOf(row, state.copiedMarkdown))
  }

  /** Like the shell: one press cannot do both, so the other shortcut's keys are refused. */
  async function saveShortcut(request, response, id, other, otherDoes) {
    const keys = (await bodyOf(request)).shortcut
    if (keys === state.preferences[other]) return sendJSON(response, 409, { error: `${keys} already ${otherDoes}; choose other keys` })
    state.preferences[id] = keys
    sendJSON(response, 200, state.preferences)
    emit('preferences', state.preferences)
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
    lateOpen = { waiting: undefined, settled: undefined }
    noContent(response)
    listChanged()
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

  function mailboxRoute(path, operation) {
    return async (request, response) => {
      logCall(path)
      const reply = operation(state, await bodyOf(request), new Date(timeOf()))
      if (reply.changed) {
        listChanged()
        response.setHeader('List-Version', state.version)
      }
      sendJSON(response, reply.status, reply.body)
    }
  }

  const routes = {
    'GET /api/inbox': inbox,
    'GET /api/mailbox/threads': (_request, response, url) => {
      const reply = mailboxThreads(state, url)
      sendJSON(response, reply.status, reply.body)
    },
    'GET /api/mailbox/claims': (_request, response, url) => {
      const reply = mailboxClaims(state, url)
      sendJSON(response, reply.status, reply.body)
    },
    'POST /api/mailbox/claim': mailboxRoute('/api/mailbox/claim', mailboxClaim),
    'POST /api/mailbox/release': mailboxRoute('/api/mailbox/release', mailboxRelease),
    'POST /api/mailbox/conflict/dismiss': mailboxRoute('/api/mailbox/conflict/dismiss', mailboxDismiss),
    'POST /api/mailbox/handoff': mailboxRoute('/api/mailbox/handoff', mailboxAnswer),
    'POST /api/mailbox/refs': mailboxRoute('/api/mailbox/refs', mailboxRefs),
    'POST /api/mailbox/send': mailboxRoute('/api/mailbox/send', mailboxSend),
    'POST /api/mailbox/thread': mailboxRoute('/api/mailbox/thread', mailboxChange),
    'POST /api/mailbox/stage': mailboxRoute('/api/mailbox/stage', mailboxStage),
    'POST /api/mailbox/approve': mailboxRoute('/api/mailbox/approve', mailboxApprove),
    'POST /api/mailbox/queue': mailboxRoute('/api/mailbox/queue', mailboxQueue),
    'GET /api/mailbox/settings': (_request, response) => sendJSON(response, 200, state.mailboxSettings),
    'POST /api/mailbox/settings': async (request, response) => {
      const body = await bodyOf(request)
      logCall('/api/mailbox/settings', body)
      const reply = mailboxSaveSettings(state, body)
      if (reply.status === 204) return noContent(response)
      sendJSON(response, reply.status, reply.body)
    },
    'POST /api/open': open,
    'POST /api/docs': async (request, response) => {
      logCall('/api/docs', await bodyOf(request))
      noContent(response)
    },
    'GET /api/open/late': (request, response) => {
      logCall('/api/open/late')
      const failure = lateOpen.settled
      if (!failure) return noContent(response)
      const title = state.conversations.find((row) => row.tool === failure.tool && row.id === failure.chat)?.title || failure.sentTitle
      sendJSON(response, 200, { id: failure.id, ...(title ? { title } : {}), message: failure.message, command: failure.command })
    },
    'POST /api/open/late/dismiss': async (request, response) => {
      const body = await bodyOf(request)
      logCall('/api/open/late/dismiss', body)
      if (lateOpen.settled?.id === body.id) lateOpen.settled = undefined
      noContent(response)
    },
    // Like serve: one document of the user settings, with a tag a poll sends back.
    'GET /api/settings': (request, response) => {
      const doc = settingsDocOf(state)
      const tag = settingsTagOf(doc)
      if (request.headers['if-none-match'] === tag) {
        response.writeHead(304, { ETag: tag, 'Cache-Control': 'no-store' })
        return response.end()
      }
      response.setHeader('ETag', tag)
      sendJSON(response, 200, doc)
    },
    // Like serve: one change, refused when the menu showed another value than the one in effect.
    'POST /api/settings': async (request, response) => {
      const body = await bodyOf(request)
      logCall('/api/settings', body)
      const doc = settingsDocOf(state)
      const current = { 'approvals.mode': doc.approvals.mode, 'approvals.companion': doc.approvals.companion, 'mailbox.approveAgentSends': doc.mailbox.approveAgentSends }
      if (!(body.key in current)) return sendJSON(response, 400, { error: 'key must be approvals.mode, approvals.companion or mailbox.approveAgentSends' })
      if ('expected' in body && body.expected !== current[body.key]) return sendJSON(response, 409, { error: 'This setting changed since the page read it. Check the current value.', settings: doc })
      if (body.key === 'approvals.mode') {
        if (!['ask', 'review', 'yolo'].includes(body.value)) return sendJSON(response, 400, { error: 'approvals.mode takes ask, review or yolo' })
        state.approvals.mode = body.value
        state.approvalsMode = body.value === 'ask' ? undefined : body.value
      } else if (body.key === 'approvals.companion') {
        state.approvals.companion = Boolean(body.value)
      } else {
        state.mailboxSettings = { approveAgentSends: Boolean(body.value) }
      }
      listChanged()
      response.setHeader('List-Version', state.version)
      sendJSON(response, 200, { settings: settingsDocOf(state), changed: [body.key.startsWith('mailbox') ? 'mailbox/settings.json' : 'local/agnostic-ai.yaml'] })
    },
    // Like serve: a plan names the files, and apply runs only the plan the menu showed.
    'POST /api/settings/sync': async (request, response) => {
      const body = await bodyOf(request)
      logCall('/api/settings/sync', body)
      const files = state.approvals.hookInstalled ? [] : SYNC_FILES
      const plan = { id: files.length ? 'plan-hooks' : 'plan-empty', files, notes: [] }
      if (body.action === 'plan') return sendJSON(response, 200, plan)
      if (body.action !== 'apply' || !body.id) return sendJSON(response, 400, { error: 'send {"action": "plan"} or {"action": "apply", "id": "<plan id>"}' })
      if (body.id !== plan.id) return sendJSON(response, 409, { error: 'The pending changes changed since you checked them. Check them again.', plan })
      state.approvals.hookInstalled = true
      listChanged()
      sendJSON(response, 200, { written: files.map((file) => file.path), notes: files.length ? [CODEX_TRUST] : [], settings: settingsDocOf(state) })
    },
    // Like serve: only ask can be set here, and the list read after it no longer names a mode.
    'POST /api/approvals/ask': async (request, response) => {
      const body = await bodyOf(request)
      logCall('/api/approvals/ask', body)
      if (body.mode !== 'ask' || Object.keys(body).length !== 1) return sendJSON(response, 400, { error: 'only ask can be set here; review and yolo are set in $AGNOSTIC_AI_HOME/agnostic-ai.yaml' })
      state.approvalsMode = undefined
      state.approvals.mode = 'ask'
      listChanged()
      response.setHeader('List-Version', state.version)
      sendJSON(response, 200, { mode: 'ask', path: '/Users/you/.agnostic-ai/local/agnostic-ai.yaml' })
    },
    // Like serve: an answer needs a held id and the digest the panel showed; the request then leaves the row.
    'POST /api/approvals/decide': async (request, response) => {
      const body = await bodyOf(request)
      logCall('/api/approvals/decide', body)
      if (!['approve', 'deny', 'release'].includes(body.decision)) return sendJSON(response, 400, { error: 'send {"id", "digest", "decision"} with decision approve, deny or release' })
      const row = state.conversations.find((c) => c.approvals?.pending?.some((p) => p.id === body.id))
      const held = row?.approvals.pending.find((p) => p.id === body.id)
      if (!held) return sendJSON(response, 404, { error: 'This request is no longer waiting.' })
      if (held.digest !== body.digest) return sendJSON(response, 409, { error: 'This request changed. Check it again.' })
      row.approvals = { ...row.approvals, pending: row.approvals.pending.filter((p) => p.id !== body.id) }
      if (row.approvals.pending.length === 0) {
        row.waiting = undefined
        row.activity = 'working'
        row.state = 'working'
      }
      listChanged()
      response.setHeader('List-Version', state.version)
      sendJSON(response, 200, { decision: body.decision })
    },
    'POST /api/mark': mark,
    'POST /api/rename': rename,
    'POST /api/delete': remove,
    'POST /api/stop-shell': stopShell,
    'POST /api/stop-session': stopSession,
    'GET /api/clipboard': (request, response) => sendJSON(response, 200, clipboardOf(state.clipboard)),
    'POST /api/send': send,
    'POST /api/panel/toggle': async (request, response) => {
      const { jump } = await bodyOf(request)
      logCall('/api/panel/toggle')
      comeBack()
      noContent(response)
      emit('panel-shown', jump === true ? { from: 'bottom-right', jump: true } : 'bottom-right')
    },
    'POST /api/panel/hide': (request, response) => { logCall('/api/panel/hide'); noContent(response); emit('panel-hidden') },
    // Like the shell: a hidden or faded companion does not show the list for the page.
    'POST /api/panel/show': (request, response) => {
      logCall('/api/panel/show')
      noContent(response)
      if (state.appearance.presence === 'shown') emit('panel-shown', 'bottom-right')
    },
    'POST /api/panel/peek': (request, response) => {
      logCall('/api/panel/peek')
      const shown = state.appearance.presence === 'shown'
      sendJSON(response, 200, { shown })
      if (shown) emit('panel-shown', 'bottom-right')
    },
    'GET /api/preferences': (request, response) => sendJSON(response, 200, state.preferences),
    'POST /api/preferences/shortcut': (request, response) => saveShortcut(request, response, 'shortcut', 'sendShortcut', 'sends the copied text'),
    'POST /api/preferences/send-shortcut': (request, response) => saveShortcut(request, response, 'sendShortcut', 'shortcut', 'opens the list'),
    'POST /api/preferences/start-at-login': async (request, response) => {
      state.preferences.startAtLogin = (await bodyOf(request)).enabled === true
      sendJSON(response, 200, state.preferences)
      emit('preferences', state.preferences)
    },
    'POST /api/preferences/check-for-updates': async (request, response) => {
      state.preferences.checkForUpdates = (await bodyOf(request)).enabled === true
      sendJSON(response, 200, state.preferences)
      emit('preferences', state.preferences)
    },
    'GET /api/update': (request, response) => sendJSON(response, 200, state.update),
    'POST /api/update': (request, response) => {
      logCall('/api/update')
      noContent(response)
    },
    'POST /api/update/release-page': (request, response) => {
      logCall('/api/update/release-page')
      if (state.update.state === 'unavailable') return sendJSON(response, 404, { error: 'this companion has no update entry' })
      noContent(response)
    },
    // Like the shell: only a known release can start, and the panel follows the first step.
    'POST /api/update/start': (request, response, url) => {
      logCall('/api/update/start')
      const expected = url.searchParams.get('version')
      if (expected && expected !== state.update.version) return sendJSON(response, 409, { error: 'the offered release changed; check for updates again' })
      if (state.update.state !== 'available' && state.update.state !== 'failed') return sendJSON(response, 409, { error: 'no update is known; check for updates first' })
      if (state.update.state === 'available' && state.update.command) return sendJSON(response, 409, { error: `the companion cannot update this install itself; run ${state.update.command} in a terminal` })
      state.update = { state: 'updating', version: state.update.version, current: state.update.current, step: 'downloading' }
      noContent(response)
      emit('update', state.update)
    },
    'GET /api/kept-shells': (request, response) => sendJSON(response, 200, { shells: state.keptShells }),
    // Like the shell: a record replaces the one for the same shell, and every change tells the pages.
    'POST /api/kept-shells/keep': async (request, response) => {
      const record = await bodyOf(request)
      logCall('/api/kept-shells/keep', record)
      const same = (kept) => kept.pid === record.pid && kept.procStart === record.procStart
      state.keptShells = [...state.keptShells.filter((kept) => !same(kept)), record]
      sendJSON(response, 200, { shells: state.keptShells })
      emit('kept-shells', state.keptShells)
    },
    'POST /api/kept-shells/forget': async (request, response) => {
      const body = await bodyOf(request)
      logCall('/api/kept-shells/forget', body)
      const named = (kept) => body.shells.some((shell) => shell.pid === kept.pid && shell.procStart === kept.procStart)
      state.keptShells = state.keptShells.filter((kept) => !named(kept))
      sendJSON(response, 200, { shells: state.keptShells })
      emit('kept-shells', state.keptShells)
    },
    'GET /api/appearance': (request, response) => sendJSON(response, 200, state.appearance),
    'POST /api/appearance/animate': async (request, response) => {
      state.appearance.animate = (await bodyOf(request)).enabled === true
      sendJSON(response, 200, state.appearance)
      emit('appearance', state.appearance)
    },
    'POST /api/appearance/dim-when-idle': async (request, response) => {
      state.appearance.dimWhenIdle = (await bodyOf(request)).enabled === true
      sendJSON(response, 200, state.appearance)
      emit('appearance', state.appearance)
    },
    'POST /api/appearance/small-companion': async (request, response) => {
      state.appearance.smallCompanion = (await bodyOf(request)).enabled === true
      sendJSON(response, 200, state.appearance)
      emit('appearance', state.appearance)
    },
    'POST /api/dashboard': (request, response) => { logCall('/api/dashboard'); noContent(response) },
    'POST /api/hide': (request, response) => {
      logCall('/api/hide')
      state.appearance.presence = 'hidden'
      noContent(response)
      emit('appearance', state.appearance)
    },
    'POST /api/quit': (request, response) => { logCall('/api/quit'); noContent(response) },
    'GET /wails/runtime.js': (request, response) => serveFile(response, here, 'runtime-shim.js'),
    'GET /dev/': (request, response) => serveFile(response, here, 'preview.html'),
    'GET /dev/events': events,
    'GET /dev/fixtures': (request, response) =>
      sendJSON(response, 200, { chosen: state.name, fixtures: Object.entries(FIXTURES).map(([name, { label }]) => ({ name, label })) }),
    'GET /dev/calls': (request, response) => sendJSON(response, 200, calls),
    'POST /dev/fixture': chooseFixture,
    // The open answered as pending ends now, and fails with the fixture's late reason.
    'POST /dev/settle-late-opens': (request, response) => {
      const settled = lateOpen.waiting ? 1 : 0
      lateOpen = { waiting: undefined, settled: lateOpen.waiting }
      sendJSON(response, 200, { settled })
      if (settled) emit('inbox', state.version)
    },
    'POST /dev/release-opens': (request, response) => {
      const released = heldOpens
      heldOpens = []
      for (const release of released) release()
      sendJSON(response, 200, { released: released.length })
    },
    // Stands in for Quit in the circle's right-click menu.
    'POST /dev/ask-quit': (request, response) => {
      noContent(response)
      comeBack()
      emit('quit-asked', 'bottom-right')
    },
    // Stands in for the send shortcut, pressed in another app.
    'POST /dev/send-shortcut': (request, response) => {
      logCall('/dev/send-shortcut')
      noContent(response)
      comeBack()
      emit('send-shown', 'bottom-right')
    },
    // Stands in for Open from anywhere in the circle's right-click menu.
    'POST /dev/open-settings': (request, response) => {
      noContent(response)
      emit('settings-shown', 'bottom-right')
    },
    // Stands in for Send copied text in the circle's right-click menu.
    'POST /dev/open-send-settings': (request, response) => {
      noContent(response)
      emit('send-settings-shown', 'bottom-right')
    },
    // Stands in for the circle's right-click menu, which the preview has not.
    'POST /dev/appearance': async (request, response) => {
      const { animate, dimWhenIdle, smallCompanion } = await bodyOf(request)
      if (typeof animate === 'boolean') state.appearance.animate = animate
      if (typeof dimWhenIdle === 'boolean') state.appearance.dimWhenIdle = dimWhenIdle
      if (typeof smallCompanion === 'boolean') state.appearance.smallCompanion = smallCompanion
      noContent(response)
      emit('appearance', state.appearance)
    },
    // A new message in a listed chat: it is written now and needs the person again.
    'POST /dev/write': async (request, response) => {
      const row = find(await bodyOf(request))
      if (!row) return sendJSON(response, 404, { error: 'That conversation is not in this fixture.' })
      row.updatedAt = new Date(timeOf()).toISOString()
      delete row.completedAt
      row.state = 'unread'
      noContent(response)
      listChanged()
    },
    // Something else copied while the list shows.
    'POST /dev/clipboard': async (request, response) => {
      const copied = await bodyOf(request)
      state.clipboard = String(copied.text ?? '')
      state.copiedMarkdown = Boolean(copied.markdown)
      noContent(response)
    },
    'POST /dev/arrive': (request, response) => {
      state.conversations = [arrival(timeOf()), ...state.conversations.filter((row) => row.id !== 'arrived-1')]
      noContent(response)
      listChanged()
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
