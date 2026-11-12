// The companion's own calls, answered by the Go shell in this process,
// which asks the agnostic-ai server.

/** A failed call. `message` is what the person reads. */
export class CallError extends Error {}

/** A call its caller gave up on, through the `signal` it passed. Nothing failed. */
export class GivenUp extends Error {}

async function call(path, options) {
  let response
  try {
    response = await fetch(path, options)
  } catch (cause) {
    if (options?.signal?.aborted) throw new GivenUp()
    throw new CallError('The companion could not reach its own shell.', { cause })
  }
  if (response.status === 204) return undefined
  const body = await response.json()
  if (!response.ok) throw new CallError(body.error)
  return body
}

function post(path, body) {
  return call(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body ?? {}) })
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

export function openConversation(conversation) {
  return post('/api/open', { tool: conversation.tool, id: conversation.id })
}

/** mark is `seen`, `completed` or `hidden`. */
export function markConversation(conversation, mark) {
  return post('/api/mark', { tool: conversation.tool, id: conversation.id, as: mark })
}

export function renameConversation(conversation, title) {
  return post('/api/rename', { tool: conversation.tool, id: conversation.id, title })
}

/** Deletes an archived chat from this computer for good: `{deleted, freedBytes?, note?}`. */
export function deleteConversation(conversation) {
  return post('/api/delete', { tool: conversation.tool, id: conversation.id })
}

export const togglePanel = () => post('/api/panel/toggle')
export const hidePanel = () => post('/api/panel/hide')
export const showPanel = () => post('/api/panel/show')

/** The settings menu's values: {shortcut, shortcutProblem?, startAtLogin, startAtLoginProblem?}. */
export const readPreferences = () => call('/api/preferences')
export const saveShortcut = (shortcut) => post('/api/preferences/shortcut', { shortcut })
export const saveStartAtLogin = (enabled) => post('/api/preferences/start-at-login', { enabled })

/** How the pages look: {animate, dimWhenIdle, hidden}. */
export const readAppearance = () => call('/api/appearance')
export const saveAnimate = (enabled) => post('/api/appearance/animate', { enabled })
export const saveDimWhenIdle = (enabled) => post('/api/appearance/dim-when-idle', { enabled })
/** The items both settings menus show, in order: [{id, label, kind, hint?}]. */
export const readMenu = () => call('/menu.json')

/** Opens the full web interface in the browser. */
export const openDashboard = () => post('/api/dashboard')
/** Takes the circle and the list off the screen and stops reading; the shortcut brings them back. */
export const hideCompanion = () => post('/api/hide')
/** Ends the companion and its server. */
export const quitCompanion = () => post('/api/quit')
