import { Events, System } from '/wails/runtime.js'
import { CallError, GivenUp, hideCompanion, hidePanel, markConversation, openConversation, openDashboard, readInbox, readPreferences, renameConversation, saveShortcut, saveStartAtLogin, showPanel } from '/api.js'
import { GROUPS, PLACES, confirmsCandidate, detailsOf, escapeTarget, folderNamesOf, grouped, keyOf, keycapsOf, neighbour, reasonOf, renameOf, rowActionOf, rowMenuOf, shortcutOf, summaryOf, viewIn, withTitle } from '/model.js'

const card = document.getElementById('card')
const queryInput = document.getElementById('query')
const clearButton = document.getElementById('clear')
const note = document.getElementById('note')
const places = document.getElementById('places')
const list = document.getElementById('list')
const summary = document.getElementById('summary')
const toast = document.getElementById('toast')

/** How long typing must pause before a search is sent. */
const SEARCH_PAUSE_MS = 300
/** How long the answer to an open stays on screen. */
const TOAST_MS = 3200
/** Only the first rows rise one after another; the rest arrive with the last of them. */
const STAGGERED_ROWS = 12

const PLACE_KEY = 'place'

/** The place chosen last time, or all. A web view that keeps no storage starts at all each run. */
function storedPlace() {
  let stored
  try {
    stored = localStorage.getItem(PLACE_KEY)
  } catch {
    return 'all'
  }
  return PLACES.some((place) => place.id === stored) ? stored : 'all'
}

const state = {
  /** Which conversations show: all, local or cloud. Kept between runs. */
  place: storedPlace(),
  /** The latest list, or the latest search result while a search is typed. */
  view: undefined,
  /** A search was typed and its answer is not here yet. */
  searching: false,
  /** Why the list could not be read; undefined when it could. */
  problem: undefined,
  /** What the companion itself has to say, such as a version mismatch. */
  note: undefined,
  /** Why a conversation did not open, said once the panel shows again. */
  reopenedToSay: undefined,
  query: '',
  /** Group ids the person folded or unfolded, over each group's start. */
  folded: new Map(GROUPS.map((group) => [group.id, group.startsFolded])),
  /** The row the arrow keys are on. */
  activeKey: undefined,
  /** What was drawn last, so an unchanged list is not drawn again. */
  drawn: '',
  /** The row whose name is being typed: {key, draft, problem?, saving?}; undefined otherwise. */
  renaming: undefined,
}

/** The list as the place switch narrows it; undefined before the first one arrived. */
function shownView() {
  return state.view === undefined ? undefined : viewIn(state.view, state.place)
}

function choosePlace(place) {
  state.place = place
  try {
    localStorage.setItem(PLACE_KEY, place)
  } catch {
    // The choice then lasts for this run only.
  }
  state.activeKey = undefined
  drawPlaces()
  draw({ force: true })
}

function drawPlaces() {
  places.replaceChildren(
    ...PLACES.map((place) => {
      const button = element('button', 'place', place.label)
      button.type = 'button'
      button.title = place.title
      button.setAttribute('aria-pressed', String(place.id === state.place))
      button.addEventListener('click', () => choosePlace(place.id))
      return button
    }),
  )
}

const ICONS = {
  chevron: 'M2.5 1.5L5.5 4L2.5 6.5',
  archive: 'M2 3.5h12v3H2zM3 6.5v6a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1v-6M6.5 9.5h3',
  hide: 'M2 2l12 12M6.5 4.2A6.3 6.3 0 0 1 8 4c3.5 0 5.5 4 5.5 4a9 9 0 0 1-1.6 2.1M9.9 11.6A5.6 5.6 0 0 1 8 12c-3.5 0-5.5-4-5.5-4a9 9 0 0 1 2-2.5',
}

function icon(name, viewBox) {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg')
  svg.setAttribute('viewBox', viewBox)
  svg.setAttribute('aria-hidden', 'true')
  svg.setAttribute('focusable', 'false')
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path')
  path.setAttribute('d', ICONS[name])
  svg.append(path)
  return svg
}

function element(tag, className, text) {
  const node = document.createElement(tag)
  node.className = className
  if (text !== undefined) node.textContent = text
  return node
}

/** A search shows every group open: the person asked for those rows. */
function isFolded(group) {
  return state.query === '' && state.folded.get(group.id)
}

function visibleKeys() {
  return grouped(shownView().conversations)
    .filter((group) => !isFolded(group))
    .flatMap((group) => group.rows.map(keyOf))
}

function actionButton(iconName, label, conversation, mark) {
  const button = element('button', 'action')
  button.type = 'button'
  button.title = label
  button.setAttribute('aria-label', label)
  button.append(icon(iconName, '0 0 16 16'))
  button.addEventListener('click', (event) => {
    event.stopPropagation()
    markRow(conversation, mark)
  })
  return button
}

/** The state's mark. Archived rows share one, however they got there. */
const MARK_OF = { completed: 'archived', dormant: 'archived' }

function stateMark(conversation) {
  return element('span', 'state is-' + (MARK_OF[conversation.state] ?? conversation.state))
}

function rowOf(conversation, order, folderNames) {
  const key = keyOf(conversation)
  const needsYou = conversation.state === 'blocked' || conversation.state === 'unread'
  const quiet = conversation.state === 'completed' || conversation.state === 'dormant'
  const row = element('div', 'row')
  row.classList.toggle('needs-you', needsYou)
  row.classList.toggle('is-quiet', quiet)
  row.classList.toggle('is-active', key === state.activeKey)
  row.dataset.key = key
  row.style.setProperty('--order', String(Math.min(order, STAGGERED_ROWS)))
  row.setAttribute('role', 'option')
  row.setAttribute('aria-selected', String(key === state.activeKey))
  row.title = conversation.folder ? conversation.open.label + '\n' + conversation.folder : conversation.open.label
  if (conversation.folder) row.setAttribute('aria-description', row.title)

  const text = element('div', 'text')
  const renaming = state.renaming?.key === key ? state.renaming : undefined
  if (renaming) {
    text.append(renameField(conversation, renaming))
  } else {
    text.append(element('div', 'title', conversation.title === '' ? 'Untitled conversation' : conversation.title))
  }
  const meta = element('div', 'meta')
  if (renaming?.problem) {
    meta.classList.add('is-problem')
    meta.title = renaming.problem
    meta.append(renaming.problem)
  } else {
    const now = Date.now()
    const reason = reasonOf(conversation, now)
    if (reason) {
      meta.append(element('span', 'why is-' + conversation.state, reason), ' · ')
    }
    meta.append(detailsOf(conversation, now, folderNames))
  }
  text.append(meta)

  const actions = element('div', 'actions')
  // Opening an archived chat, a new message in it, or its tool starting work brings it back.
  const action = rowActionOf(conversation)
  if (action === 'hide') {
    actions.append(actionButton('hide', 'Hide from agnostic-ai, until something new is written or it starts working', conversation, 'hidden'))
  } else if (action === 'archive') {
    actions.append(actionButton('archive', 'Archive, until something new is written or it starts working', conversation, 'completed'))
  }

  row.append(stateMark(conversation), text, actions)
  row.addEventListener('click', () => openRow(conversation))
  row.addEventListener('mousemove', () => setActive(key))
  row.addEventListener('contextmenu', (event) => {
    // Right-clicking only offers the menu: the chat stays closed and unread.
    event.preventDefault()
    setActive(key)
    openRowMenu(conversation, event)
  })
  return row
}

function renameField(conversation, renaming) {
  const input = element('input', 'title rename')
  input.type = 'text'
  input.value = renaming.draft
  input.spellcheck = false
  input.autocomplete = 'off'
  input.setAttribute('aria-label', 'New name')
  // The row opens on a click; typing a name must not.
  for (const kind of ['click', 'contextmenu']) {
    input.addEventListener(kind, (event) => event.stopPropagation())
  }
  input.addEventListener('input', () => { renaming.draft = input.value })
  input.addEventListener('keydown', (event) => {
    event.stopPropagation()
    if (confirmsCandidate(event)) return
    if (event.key === 'Enter') {
      event.preventDefault()
      saveRename(conversation)
    } else if (event.key === 'Escape') {
      event.preventDefault()
      escapeRename()
    }
  })
  input.addEventListener('blur', () => {
    // Chrome sends blur while a redraw is still removing the field, so wait until the redraw is over:
    // only a person leaving a field that is still on the page cancels.
    setTimeout(() => {
      if (input.isConnected) cancelRename()
    })
  })
  return input
}

function groupHead(group) {
  const folded = isFolded(group)
  const title = element('button', 'group-title')
  title.type = 'button'
  title.classList.toggle('is-urgent', group.id === 'needs-you')
  title.setAttribute('aria-expanded', String(!folded))
  title.append(icon('chevron', '0 0 8 8'), group.title, element('span', 'count', String(group.rows.length)))
  title.firstChild.classList.add('chevron')
  title.addEventListener('click', () => {
    state.folded.set(group.id, !folded)
    draw({ force: true })
  })
  const head = element('div', 'group-head')
  head.append(title)
  if (group.id === 'active') {
    const all = element('button', 'group-action', 'Archive all')
    all.type = 'button'
    all.title = 'Archive every active conversation, until something new is written in it or it starts working'
    all.addEventListener('click', () => archiveAll(group.rows))
    head.append(all)
  }
  return head
}

function emptyMessage() {
  const empty = element('div', 'empty')
  if (state.query === '') {
    empty.append(element('strong', '', 'No conversations yet'), 'Chats from Claude Code, Codex and Cursor show up here.')
  } else {
    empty.append(element('strong', '', 'Nothing matches'), 'Try a title, a folder, a branch or words from your last prompt.')
  }
  return empty
}

/** Why a cloud is missing, said only where cloud rows would show. */
function cloudAside(view) {
  return state.place === 'local' ? undefined : view.cloudNote
}

function draw({ force } = { force: false }) {
  // A new list would take the name being typed away; it waits until the rename ends.
  if (state.renaming && !force) return
  const view = shownView()
  // What could not be read comes first, then what the companion and the server have to say.
  // A cloud that is missing is not a problem with the list: it gets a quiet line under the rows.
  const said = state.problem ?? state.note ?? view?.marksNote ?? ''
  note.hidden = said === ''
  note.textContent = said
  if (state.searching) {
    // The rows on screen answer another question, so they give way until this one's answer is here.
    state.drawn = undefined
    summary.textContent = 'Searching…'
    const waiting = element('div', 'empty is-searching')
    waiting.append(element('strong', '', 'Searching…'), 'Looking through every conversation.')
    list.replaceChildren(waiting)
    return
  }
  if (!view) {
    // No list could be read. An old one would look current, so none shows.
    state.drawn = undefined
    summary.textContent = ''
    list.replaceChildren()
    return
  }
  // The reasons go in too: a silent chat's "No activity for 7m" changes with no new row data.
  const now = Date.now()
  const reasons = view.conversations.map((conversation) => reasonOf(conversation, now))
  const signature = JSON.stringify([view.conversations, reasons, view.needsYou, view.matches, said, view.cloudNote, state.query, state.activeKey, state.place])
  if (!force && signature === state.drawn) return
  state.drawn = signature

  summary.textContent = summaryOf(view, state.query)

  const groups = grouped(view.conversations)
  const scroll = list.scrollTop
  list.replaceChildren()
  if (groups.length === 0) {
    list.append(emptyMessage())
    if (cloudAside(view)) list.append(element('p', 'aside', cloudAside(view)))
    return
  }
  const folderNames = folderNamesOf(view.conversations.map((conversation) => conversation.folder))
  let order = 0
  for (const group of groups) {
    list.append(groupHead(group))
    if (isFolded(group)) continue
    for (const conversation of group.rows) {
      list.append(rowOf(conversation, order, folderNames))
      order += 1
    }
  }
  if (cloudAside(view)) list.append(element('p', 'aside', cloudAside(view)))
  list.scrollTop = scroll
}

function setActive(key) {
  if (key === state.activeKey) return
  state.activeKey = key
  for (const row of list.querySelectorAll('.row')) {
    const active = row.dataset.key === key
    row.classList.toggle('is-active', active)
    row.setAttribute('aria-selected', String(active))
  }
}

function moveActive(step) {
  const key = neighbour(visibleKeys(), state.activeKey, step)
  if (key === undefined) return
  setActive(key)
  list.querySelector('.row.is-active')?.scrollIntoView({ block: 'nearest' })
}

function activeConversation() {
  return shownView()?.conversations.find((conversation) => keyOf(conversation) === state.activeKey)
}

let toastTimer
function say(message) {
  toast.textContent = message
  toast.hidden = false
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toast.hidden = true }, TOAST_MS)
}

/** Shows what a failed call said; anything else is a bug and is thrown on. */
function sayFailure(error) {
  if (!(error instanceof CallError)) throw error
  say(error.message)
}

/** How many weeks with matches one search reads. */
const MAX_SEARCH_PAGES = 8

/** The list, or a search's matches from the newest weeks that hold any, read one after the other. */
async function readEveryPage(asked, signal) {
  let reply = await readInbox(asked, undefined, signal)
  if (asked === '') return reply
  for (let pages = 1; reply.view.next && pages < MAX_SEARCH_PAGES; pages += 1) {
    const older = await readInbox(asked, reply.view.next, signal)
    reply = { ...reply, view: { ...reply.view, next: older.view.next, conversations: [...reply.view.conversations, ...older.view.conversations] } }
  }
  return reply
}

/** The search on its way; a newer one gives it up, so the server stops working on it. */
let searchOnItsWay

async function refresh() {
  const asked = state.query
  searchOnItsWay?.abort()
  searchOnItsWay = asked === '' ? undefined : new AbortController()
  let reply
  let problem
  try {
    reply = await readEveryPage(asked, searchOnItsWay?.signal)
  } catch (error) {
    if (error instanceof GivenUp) return
    if (!(error instanceof CallError)) throw error
    problem = error.message
  }
  // A search typed meanwhile has its own request on the way.
  if (asked !== state.query) return
  state.searching = false
  state.problem = problem
  state.view = reply?.view
  state.note = reply?.note
  draw()
  keepRowMenuCurrent()
}

// The panel never activates its app, so while it shows it keeps the keys
// even after the terminal comes to the front. It hides first, and comes
// back to say why only when the conversation did not open.
async function openRow(conversation) {
  await hidePanel().catch(() => {})
  let message
  try {
    const reply = await openConversation(conversation)
    if (reply.opened) return
    message = reply.message
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    message = error.message
  }
  state.reopenedToSay = message
  await showPanel().catch((error) => {
    state.reopenedToSay = undefined
    sayFailure(error)
  })
}

async function markRow(conversation, mark) {
  try {
    await markConversation(conversation, mark)
  } catch (error) {
    sayFailure(error)
    return
  }
  await refresh()
}

// Rename

const rowMenu = document.getElementById('row-menu')
const renameItem = document.getElementById('rename')
const menuItems = { read: document.getElementById('mark-read'), rename: renameItem }
/** The conversation the row menu was opened on. */
let rowMenuFor

function openRowMenu(conversation, event) {
  rowMenuFor = conversation
  const offered = rowMenuOf(conversation)
  for (const [name, item] of Object.entries(menuItems)) item.hidden = !offered.includes(name)
  rowMenu.hidden = false
  // The menu opens at the pointer and stays inside the card.
  const bounds = card.getBoundingClientRect()
  const left = Math.min(event.clientX - bounds.left, bounds.width - rowMenu.offsetWidth - 8)
  const top = Math.min(event.clientY - bounds.top, bounds.height - rowMenu.offsetHeight - 8)
  rowMenu.style.left = Math.max(left, 8) + 'px'
  rowMenu.style.top = Math.max(top, 8) + 'px'
  menuItems[offered[0]].focus()
}

function closeRowMenu() {
  rowMenu.hidden = true
  rowMenuFor = undefined
}

/** The row as the latest list has it, while it still offers `item`; undefined otherwise. */
function rowMenuTarget(item) {
  if (!rowMenuFor) return undefined
  const key = keyOf(rowMenuFor)
  const current = shownView()?.conversations.find((conversation) => keyOf(conversation) === key)
  return current && rowMenuOf(current).includes(item) ? current : undefined
}

/** A new list can move or remove the menu's row; the menu then closes rather than act on a row that changed. */
function keepRowMenuCurrent() {
  if (rowMenu.hidden || !rowMenuFor) return
  const offered = Object.keys(menuItems).filter((item) => !menuItems[item].hidden)
  if (offered.every((item) => rowMenuTarget(item))) {
    rowMenuFor = rowMenuTarget(offered[0])
    return
  }
  closeRowMenu()
  queryInput.focus()
}

function startRename(conversation) {
  closeRowMenu()
  state.renaming = { key: keyOf(conversation), draft: conversation.title }
  draw({ force: true })
  if (!focusRename()) dropRename('That conversation is no longer in the list.')
}

/** Puts the keys in the name field; false when the row it belongs to is not drawn. */
function focusRename() {
  const input = list.querySelector('.rename')
  if (!input) return false
  input.focus()
  input.select()
  return true
}

/** Ends a rename whose row left the list, which would otherwise hold every redraw and key. */
function dropRename(message) {
  state.renaming = undefined
  draw({ force: true })
  say(message)
}

function cancelRename() {
  if (!state.renaming || state.renaming.saving) return
  state.renaming = undefined
  draw({ force: true })
}

/** Escape ends a rename; the keys go back to the search, which moves through the list. */
function escapeRename() {
  cancelRename()
  if (!state.renaming) queryInput.focus()
}

async function saveRename(conversation) {
  const renaming = state.renaming
  const { title, empty, keep } = renameOf(renaming.draft, conversation.title)
  if (keep) {
    cancelRename()
    return
  }
  if (empty) {
    renaming.problem = 'Type a name, or press Escape to keep the old one.'
    draw({ force: true })
    if (!focusRename()) dropRename(renaming.problem)
    return
  }
  renaming.saving = true
  try {
    await renameConversation(conversation, title)
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    renaming.saving = false
    renaming.problem = error.message
    draw({ force: true })
    if (!focusRename()) dropRename(error.message)
    return
  }
  state.renaming = undefined
  if (state.view) state.view = withTitle(state.view, renaming.key, title)
  draw({ force: true })
  // The shell still holds the list it polled before the rename and sends the new one as an inbox event.
  // A search asks the server itself.
  if (state.query !== '') await refresh()
}

renameItem.addEventListener('click', () => {
  if (rowMenuFor) startRename(rowMenuFor)
})
menuItems.read.addEventListener('click', () => {
  const conversation = rowMenuTarget('read')
  closeRowMenu()
  queryInput.focus()
  if (!conversation) return
  // Seen is the mark opening a chat sets, so the chat stays closed and a later write brings it back.
  markRow(conversation, 'seen')
})
list.addEventListener('scroll', () => {
  if (!rowMenu.hidden) closeRowMenu()
})

/** Archives each of the rows, one mark after the other, then reads the list once. */
async function archiveAll(rows) {
  try {
    for (const conversation of rows) {
      await markConversation(conversation, 'completed')
    }
  } catch (error) {
    sayFailure(error)
  }
  await refresh()
}

let searchTimer
function searchSoon() {
  const typed = queryInput.value.trim()
  clearButton.hidden = queryInput.value === ''
  if (typed === state.query) return
  state.query = typed
  // The server is asked once the typing pauses, not on every key.
  state.searching = typed !== ''
  draw({ force: true })
  clearTimeout(searchTimer)
  // An emptied search goes back to the list at once: it is already here.
  searchTimer = setTimeout(refresh, typed === '' ? 0 : SEARCH_PAUSE_MS)
}

queryInput.addEventListener('input', searchSoon)
clearButton.addEventListener('click', () => {
  queryInput.value = ''
  searchSoon()
  queryInput.focus()
})
// Settings

const menu = document.getElementById('menu')
const menuButton = document.getElementById('menu-button')
const loginSwitch = document.getElementById('login')
const shortcutButton = document.getElementById('shortcut')
const menuProblem = document.getElementById('menu-problem')
const isMac = System.IsMac()

const settings = {
  /** What the shell last said: {shortcut, shortcutProblem?, startAtLogin, startAtLoginProblem?}. */
  preferences: undefined,
  /** True while the next key press becomes the shortcut. */
  recording: false,
  /** Why the last change was refused; shown until the next change. */
  refusal: undefined,
}

function drawSettings() {
  const preferences = settings.preferences
  loginSwitch.setAttribute('aria-checked', String(preferences.startAtLogin))
  shortcutButton.classList.toggle('is-recording', settings.recording)
  if (settings.recording) {
    shortcutButton.replaceChildren('Press the new keys')
  } else {
    shortcutButton.replaceChildren(...keycapsOf(preferences.shortcut, isMac).map((keycap) => element('kbd', '', keycap)))
  }
  const problem = settings.refusal ?? preferences.shortcutProblem ?? preferences.startAtLoginProblem
  menuProblem.hidden = problem === undefined
  menuProblem.textContent = problem ?? ''
}

function closeMenu() {
  settings.recording = false
  menu.hidden = true
  menuButton.setAttribute('aria-expanded', 'false')
}

async function openMenu() {
  try {
    settings.preferences = await readPreferences()
  } catch (error) {
    sayFailure(error)
    return
  }
  settings.refusal = undefined
  drawSettings()
  menu.hidden = false
  menuButton.setAttribute('aria-expanded', 'true')
}

/** Saves one setting and shows what the shell now holds, or why it refused. */
async function change(save) {
  try {
    settings.preferences = await save()
    settings.refusal = undefined
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    settings.refusal = error.message
  }
  drawSettings()
}

menuButton.addEventListener('click', () => (menu.hidden ? openMenu() : closeMenu()))
loginSwitch.addEventListener('click', () => change(() => saveStartAtLogin(!settings.preferences.startAtLogin)))
shortcutButton.addEventListener('click', () => {
  settings.recording = !settings.recording
  drawSettings()
})
document.addEventListener('mousedown', (event) => {
  if (!rowMenu.hidden && !rowMenu.contains(event.target)) closeRowMenu()
  if (menu.hidden || menu.contains(event.target) || menuButton.contains(event.target)) return
  closeMenu()
})
document.getElementById('hide').addEventListener('click', () => hideCompanion())
document.getElementById('dashboard').addEventListener('click', async () => {
  try {
    await openDashboard()
  } catch (error) {
    sayFailure(error)
    return
  }
  await hidePanel()
})

const KEY_STEPS = { ArrowDown: 1, ArrowUp: -1 }
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape') {
    event.preventDefault()
    const closes = {
      'row-menu': closeRowMenu,
      rename: escapeRename,
      settings: closeMenu,
      panel: () => hidePanel().catch(sayFailure),
    }
    closes[escapeTarget({ rowMenuOpen: !rowMenu.hidden, renaming: state.renaming !== undefined, settingsOpen: !menu.hidden })]()
    return
  }
  // The open row menu takes Enter and Space on its own item.
  if (!rowMenu.hidden || state.renaming) return
  if (settings.recording) {
    // Every key belongs to the new shortcut until one is complete.
    event.preventDefault()
    const shortcut = shortcutOf(event, isMac)
    if (shortcut === undefined) return
    settings.recording = false
    change(() => saveShortcut(shortcut))
    return
  }
  const step = KEY_STEPS[event.key]
  if (step !== undefined) {
    event.preventDefault()
    moveActive(step)
    return
  }
  if (event.key === 'Enter') {
    const conversation = activeConversation()
    if (conversation) openRow(conversation)
  }
})

/** The panel opened: it grows out of the corner nearest the bubble, and the search takes the keys. */
function opened(from) {
  card.dataset.from = from
  card.classList.remove('is-opening')
  if (document.documentElement.dataset.motion !== 'off') {
    void card.offsetWidth
    card.classList.add('is-opening')
  }
  toast.hidden = true
  // Back from a conversation that did not open: the highlight stays on its row.
  const returning = state.reopenedToSay
  state.reopenedToSay = undefined
  if (returning) say(returning)
  closeMenu()
  closeRowMenu()
  state.renaming = undefined
  if (!returning) state.activeKey = undefined
  queryInput.focus()
  queryInput.select()
  draw({ force: true })
  refresh()
}

card.addEventListener('animationend', (event) => {
  if (event.target === card) card.classList.remove('is-opening')
})
// An opening left over would play again when animation is turned back on.
document.addEventListener('motion', () => {
  if (document.documentElement.dataset.motion === 'off') card.classList.remove('is-opening')
})

drawPlaces()
Events.On('inbox', refresh)
Events.On('panel-shown', (event) => opened(event.data))
refresh()
