// The companion's own calls, answered by the Go shell in this process,
// which asks the agnostic-ai server.

/** A failed call. `message` is what the person reads. */
export class CallError extends Error {}

/** A call its caller gave up on, through the `signal` it passed. Nothing failed. */
export class GivenUp extends Error {}

async function call(path, options) {
  return (await answered(path, options)).body
}

/** A call's answer as {body, version}: version names the list the companion read after a change; undefined when it did not. */
async function answered(path, options) {
  let response
  let body
  try {
    response = await fetch(path, options)
    if (response.status !== 204) body = await response.json()
  } catch (cause) {
    if (options?.signal?.aborted) throw new GivenUp()
    throw new CallError('The companion could not reach its own shell.', { cause })
  }
  if (!response.ok) {
    const error = new CallError(body.error)
    if (body.guard) error.guard = body.guard
    throw error
  }
  return { body, version: response.headers.get('List-Version') || undefined }
}

function postOptions(body) {
  return { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body ?? {}) }
}

function post(path, body) {
  return call(path, postOptions(body))
}

/** A change: the companion answers once it holds the list the change made, as {body, version}. */
function change(path, body) {
  return answered(path, postOptions(body))
}

/**
 * The list, as `{view, note?}`. `view` is the server's answer: every open,
 * blocked or unread conversation and the past week, or every match of `query`. `note` is
 * something the companion itself has to say, such as a version mismatch.
 */
export function readInbox(query, before, signal) {
  if (!query) return call('/api/inbox')
  const asked = new URLSearchParams({ q: query })
  // A search comes one week at a time; `before` is the `next` of the page already read.
  if (before) asked.set('before', before)
  // `signal` lets the caller give up a search that newer typing replaced.
  return call('/api/inbox?' + asked, { signal })
}

/**
 * Opens a row, or with `helper` the chat that Claude Code subagent runs in. Its title goes along, so a failure that
 * comes after the companion stopped waiting can name it.
 */
export function openConversation(conversation) {
  return post('/api/open', {
    tool: conversation.tool,
    id: conversation.id,
    ...(conversation.title ? { title: conversation.title } : {}),
    ...(conversation.helper ? { helper: conversation.helper } : {}),
  })
}

/**
 * Why an open the companion stopped waiting for did not open, as `{id, title?, message, command?}`; undefined
 * when there is none. The companion keeps it until `dismissLateOpenFailure(id)` or a newer open.
 */
export const readLateOpenFailure = () => call('/api/open/late')
export const dismissLateOpenFailure = (id) => post('/api/open/late/dismiss', { id })

/**
 * mark is `seen`, `completed`, `open` or `hidden`; answers {version}. With `later`, such as each mark of a run but the
 * last, the companion answers at once, with no version, and reads the list soon after.
 */
export function markConversation(conversation, mark, { later = false } = {}) {
  return change(later ? '/api/mark?read=later' : '/api/mark', { tool: conversation.tool, id: conversation.id, as: mark })
}

/** Answers {version}. */
export function renameConversation(conversation, title) {
  return change('/api/rename', { tool: conversation.tool, id: conversation.id, title })
}

/** Deletes an archived chat from this computer for good. Answers {body, version}; body is `{deleted, freedBytes?, note?}`. */
export function deleteConversation(conversation) {
  return change('/api/delete', { tool: conversation.tool, id: conversation.id })
}

/** Ends the process `stoppable` named on a live Claude Code chat in a terminal; answers {body: {forced?, stillRunning?}, version}. */
export function stopSession(conversation, process) {
  return change('/api/stop-session', { tool: conversation.tool, id: conversation.id, pid: process.pid, procStart: process.procStart })
}

export function stopShell(conversation, shell) {
  return change('/api/stop-shell', { tool: conversation.tool, id: conversation.id, pid: shell.pid, procStart: shell.procStart })
}

/** Turns automatic approvals off for every tool and project, from the next request on. Answers {body, version}. */
export function askApprovals() {
  return change('/api/approvals/ask', { mode: 'ask' })
}

/** What was copied, without all of it: {preview, words, chars, hash}; words is 0 when no text was copied. */
export const readClipboard = () => call('/api/clipboard')

/**
 * Types the copied text into a conversation's input, without sending it: {sent, message}. The shell reads
 * the clipboard itself and refuses when it no longer matches `hash`, the one the page showed.
 */
export function sendClipboard(conversation, hash) {
  return post('/api/send', { tool: conversation.tool, id: conversation.id, hash })
}

export function readRefs({ tool, id }, base, signal) {
  return call('/api/mailbox/refs', { ...postOptions({ tool, id, ...(base ? { base } : {}) }), signal })
}

export const sendMessage = (message) => change('/api/mailbox/send', message)
export function readThreads({ tool, id }, signal) {
  return call('/api/mailbox/threads?' + new URLSearchParams({ tool, id }), { signal })
}
export const changeThread = (thread, action) => change('/api/mailbox/thread', { thread, action })
export function stageMessage(thread, n, how, { confirm } = {}) {
  return change('/api/mailbox/stage', { thread, n, how, ...(confirm ? { confirm } : {}) })
}

export function readClaims(owner, signal) {
  const query = owner ? '?' + new URLSearchParams({ tool: owner.tool, id: owner.id }) : ''
  return call('/api/mailbox/claims' + query, { signal })
}
export const claimPaths = (claim) => change('/api/mailbox/claim', claim)
export const releasePaths = (claim) => change('/api/mailbox/release', claim)
export const answerHandoff = (thread, n, action) => change('/api/mailbox/handoff', { thread, n, action })

export const togglePanel = () => post('/api/panel/toggle')
export const hidePanel = () => post('/api/panel/hide')
export const showPanel = () => post('/api/panel/show')

/** The settings menu's values: {shortcut, shortcutProblem?, sendShortcut, sendShortcutProblem?, startAtLogin, startAtLoginProblem?}. */
export const readPreferences = () => call('/api/preferences')
export const saveShortcut = (shortcut) => post('/api/preferences/shortcut', { shortcut })
export const saveSendShortcut = (shortcut) => post('/api/preferences/send-shortcut', { shortcut })
export const saveStartAtLogin = (enabled) => post('/api/preferences/start-at-login', { enabled })

/** How the pages look: {animate, dimWhenIdle, smallCompanion, presence}: presence is shown, hidden or faded. */
export const readAppearance = () => call('/api/appearance')
export const saveAnimate = (enabled) => post('/api/appearance/animate', { enabled })
export const saveDimWhenIdle = (enabled) => post('/api/appearance/dim-when-idle', { enabled })
export const saveSmallCompanion = (enabled) => post('/api/appearance/small-companion', { enabled })
/** The update entry: {state, version?}; state is unavailable, idle, checking, available, updating or failed. */
export const readUpdate = () => call('/api/update')
/** Checks for a newer release now; the update event says what it found. */
export const checkForUpdates = () => post('/api/update')
/** Updates to the known release and restarts; the update event follows the steps. */
export const startUpdate = () => post('/api/update/start')
/** The items both settings menus show, in order: [{id, label, kind, hint?}]. */
export const readMenu = () => call('/menu.json')

/** Opens the full web interface in the browser. */
export const openDashboard = () => post('/api/dashboard')
/** Takes the circle and the list off the screen; the shortcut brings them back, and something new waiting brings the circle back faded. */
export const hideCompanion = () => post('/api/hide')
/** Ends the companion and its server, without asking: the page asks first. */
export const quitCompanion = () => post('/api/quit')
