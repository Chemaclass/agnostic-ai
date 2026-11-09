// The widget's own calls, answered by the Go shell in this process.

/** A failed call. `message` is what the person reads. */
export class CallError extends Error {}

async function call(path, options) {
  let response
  try {
    response = await fetch(path, options)
  } catch (cause) {
    throw new CallError('The widget could not reach its own shell.', { cause })
  }
  if (response.status === 204) return undefined
  const body = await response.json()
  if (!response.ok) throw new CallError(body.error)
  return body
}

function post(path, body) {
  return call(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body ?? {}) })
}

/** The list: every open conversation and the past week, or every match of `query`. */
export function readInbox(query) {
  return call(query ? '/api/inbox?q=' + encodeURIComponent(query) : '/api/inbox')
}

export function openConversation(conversation) {
  return post('/api/open', { tool: conversation.tool, id: conversation.id })
}

/** mark is `seen`, `completed` or `open`. */
export function markConversation(conversation, mark) {
  return post('/api/mark', { tool: conversation.tool, id: conversation.id, as: mark })
}

export const togglePanel = () => post('/api/panel/toggle')
export const hidePanel = () => post('/api/panel/hide')
/** Counts everything written up to now as read, in every conversation. */
export const markAllSeen = () => post('/api/mark-all-seen')

/** The settings menu's values: {shortcut, shortcutProblem?, startAtLogin, startAtLoginProblem?}. */
export const readPreferences = () => call('/api/preferences')
export const saveShortcut = (shortcut) => post('/api/preferences/shortcut', { shortcut })
export const saveStartAtLogin = (enabled) => post('/api/preferences/start-at-login', { enabled })

/** Opens the full web interface in the browser. */
export const openDashboard = () => post('/api/dashboard')
export const quitWidget = () => post('/api/quit')
