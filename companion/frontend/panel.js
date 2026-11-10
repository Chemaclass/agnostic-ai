import { Events, System } from '/wails/runtime.js'
import { CallError, GivenUp, hideCompanion, hidePanel, markConversation, openConversation, openDashboard, readInbox, readPreferences, saveShortcut, saveStartAtLogin } from '/api.js'
import { GROUPS, PLACES, detailsOf, grouped, keyOf, keycapsOf, neighbour, reasonOf, shortcutOf, summaryOf, viewIn } from '/model.js'

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
  query: '',
  /** Group ids the person folded or unfolded, over each group's start. */
  folded: new Map(GROUPS.map((group) => [group.id, group.startsFolded])),
  /** The row the arrow keys are on. */
  activeKey: undefined,
  /** What was drawn last, so an unchanged list is not drawn again. */
  drawn: '',
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
  archive: 'M3 8.5l3 3 7-7.5',
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

function rowOf(conversation, order) {
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
  row.title = conversation.open.label

  const text = element('div', 'text')
  text.append(element('div', 'title', conversation.title === '' ? 'Untitled conversation' : conversation.title))
  const meta = element('div', 'meta')
  const reason = reasonOf(conversation)
  if (reason) {
    meta.append(element('span', 'why is-' + conversation.state, reason), ' · ')
  }
  meta.append(detailsOf(conversation, Date.now()))
  text.append(meta)

  const actions = element('div', 'actions')
  // One button a row: archive a chat that is not archived yet, hide one that is.
  // Opening an archived chat, or a new message in it, brings it back.
  const archived = conversation.state === 'completed' || conversation.state === 'dormant'
  if (archived) {
    actions.append(actionButton('hide', 'Hide from agnostic-ai, until something new is written', conversation, 'hidden'))
  } else {
    actions.append(actionButton('archive', 'Archive, until something new is written', conversation, 'completed'))
  }

  row.append(stateMark(conversation), text, actions)
  row.addEventListener('click', () => openRow(conversation))
  row.addEventListener('mousemove', () => setActive(key))
  return row
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
    all.title = 'Archive every active conversation, until something new is written in it'
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
  const signature = JSON.stringify([view.conversations, view.needsYou, view.matches, said, view.cloudNote, state.query, state.activeKey, state.place])
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
  let order = 0
  for (const group of groups) {
    list.append(groupHead(group))
    if (isFolded(group)) continue
    for (const conversation of group.rows) {
      list.append(rowOf(conversation, order))
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
}

async function openRow(conversation) {
  try {
    const reply = await openConversation(conversation)
    say(reply.message)
    if (reply.opened) await hidePanel()
  } catch (error) {
    sayFailure(error)
  }
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
  void card.offsetWidth
  card.classList.add('is-opening')
  toast.hidden = true
  closeMenu()
  state.activeKey = undefined
  queryInput.focus()
  queryInput.select()
  draw({ force: true })
  refresh()
}

card.addEventListener('animationend', (event) => {
  if (event.target === card) card.classList.remove('is-opening')
})

drawPlaces()
Events.On('inbox', refresh)
Events.On('panel-shown', (event) => opened(event.data))
refresh()
