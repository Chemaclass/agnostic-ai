import { Events } from '/wails/runtime.js'
import { CallError, hidePanel, markConversation, openConversation, openDashboard, quitWidget, readInbox } from '/api.js'
import { GROUPS, detailsOf, grouped, keyOf, neighbour, reasonOf, summaryOf } from '/model.js'

const card = document.getElementById('card')
const queryInput = document.getElementById('query')
const clearButton = document.getElementById('clear')
const note = document.getElementById('note')
const list = document.getElementById('list')
const summary = document.getElementById('summary')
const toast = document.getElementById('toast')

/** How long typing must pause before a search is sent. */
const SEARCH_PAUSE_MS = 300
/** How long the answer to an open stays on screen. */
const TOAST_MS = 3200
/** Only the first rows rise one after another; the rest arrive with the last of them. */
const STAGGERED_ROWS = 12

const state = {
  /** The latest list, or the latest search result while a search is typed. */
  view: undefined,
  query: '',
  /** Group ids the person folded or unfolded, over each group's start. */
  folded: new Map(GROUPS.map((group) => [group.id, group.startsFolded])),
  /** The row the arrow keys are on. */
  activeKey: undefined,
  /** What was drawn last, so an unchanged list is not drawn again. */
  drawn: '',
}

const ICONS = {
  chevron: 'M2.5 1.5L5.5 4L2.5 6.5',
  done: 'M3 8.5l3 3 7-7.5',
  read: 'M2 6l6-3.5L14 6v6.5H2zM2 6l6 4 6-4',
  undo: 'M6 3.5L2.5 7 6 10.5M3 7h6.5a3.5 3.5 0 0 1 0 7H8',
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
  return grouped(state.view.conversations)
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

function stateMark(conversation) {
  const mark = element('span', 'state is-' + conversation.state)
  if (conversation.state === 'completed') mark.append(icon('done', '0 0 16 16'))
  return mark
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
  if (conversation.state === 'unread') actions.append(actionButton('read', 'Mark as read', conversation, 'seen'))
  if (conversation.state === 'completed') {
    actions.append(actionButton('undo', 'Not done: move it back', conversation, 'open'))
  } else {
    actions.append(actionButton('done', 'Mark as done, until something new is written', conversation, 'completed'))
  }

  row.append(stateMark(conversation), text, actions)
  row.addEventListener('click', () => openRow(conversation))
  row.addEventListener('mousemove', () => setActive(key))
  return row
}

function groupTitle(group) {
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
  return title
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

function draw({ force } = { force: false }) {
  const view = state.view
  if (!view) return
  const signature = JSON.stringify([view.conversations, view.needsYou, view.matches, view.marksNote, state.query, state.activeKey])
  if (!force && signature === state.drawn) return
  state.drawn = signature

  note.hidden = !view.marksNote
  note.textContent = view.marksNote ?? ''
  summary.textContent = summaryOf(view, state.query)
  summary.classList.toggle('is-urgent', state.query === '' && view.needsYou > 0)

  const groups = grouped(view.conversations)
  const scroll = list.scrollTop
  list.replaceChildren()
  if (groups.length === 0) {
    list.append(emptyMessage())
    return
  }
  let order = 0
  for (const group of groups) {
    list.append(groupTitle(group))
    if (isFolded(group)) continue
    for (const conversation of group.rows) {
      list.append(rowOf(conversation, order))
      order += 1
    }
  }
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
  return state.view?.conversations.find((conversation) => keyOf(conversation) === state.activeKey)
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

async function refresh() {
  const asked = state.query
  let view
  try {
    view = await readInbox(asked)
  } catch (error) {
    sayFailure(error)
    return
  }
  // A search typed meanwhile has its own request on the way.
  if (asked !== state.query) return
  state.view = view
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

let searchTimer
function searchSoon() {
  state.query = queryInput.value.trim()
  clearButton.hidden = queryInput.value === ''
  clearTimeout(searchTimer)
  searchTimer = setTimeout(refresh, SEARCH_PAUSE_MS)
}

queryInput.addEventListener('input', searchSoon)
clearButton.addEventListener('click', () => {
  queryInput.value = ''
  searchSoon()
  queryInput.focus()
})
document.getElementById('quit').addEventListener('click', () => quitWidget())
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
  state.activeKey = undefined
  queryInput.focus()
  queryInput.select()
  draw({ force: true })
  refresh()
}

card.addEventListener('animationend', (event) => {
  if (event.target === card) card.classList.remove('is-opening')
})

Events.On('inbox', refresh)
Events.On('panel-shown', (event) => opened(event.data))
refresh()
