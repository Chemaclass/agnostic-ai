import { Events, System } from '/wails/runtime.js'
import { mailboxPanel } from '/panel-mailbox.js'
import { CallError, GivenUp, askApprovals, checkForUpdates, deleteConversation, hideCompanion, hidePanel, markConversation, openConversation, openDashboard, quitCompanion, readAppearance, readClipboard, readInbox, readMenu, readMailboxSettings, readPreferences, readUpdate, renameConversation, readLateOpenFailure, dismissLateOpenFailure, saveAnimate, saveDimWhenIdle, saveMailboxSettings, saveSendShortcut, saveShortcut, saveSmallCompanion, saveStartAtLogin, sendClipboard, showPanel, startUpdate, stopSession, stopShell } from '/api.js'
import { currentShellOf, missingToolsNoteOf, shellCommandOf, shellWarningOf, stopShellDetailsOf, stoppedShellOf, stuckShellsOf } from '/model.js'
import { CLOUD_PAGE_LABEL, GROUPS, NOTHING_TO_SEND, NOTICES, lateOpenFailureOf, PLACES, SHARED_TITLE, afterPage, approvalOf, approvalReasonOf, approvalsHeaderOf, approvedCallsOf, approvedToggleOf, ago, chipTextOf, cloudPageOf, confirmsCandidate, deleteConfirmOf, deletedOf, detailsOf, escapeTarget, folderNamesOf, grouped, helperDetailsOf, helperOpenOf, helperReplyOf, helperRowsOf, helperTitleOf, helperTooltipOf, helpersToggleOf, keyOf, lateFailureReplaces, keycapsOf, menuRowsOf, neighbour, openFailureOf, pickKeyOf, pickTitleOf, pickViewOf, pickableClip, quietNoteOf, renameOf, replyOf, rowActionOf, refusedShortcutOf, rowMenuOf, rowTooltipOf, roleOfTone, sendingTitleOf, shortcutOf, startedFromOf, startsRename, stillActive, stillWaits, straightSendOf, subagentNoticeOf, unreadLabelOf, unreadMarkOf, summaryOf, takesText, titleOf, togglesPick, typableRows, unreadableClipOf, unsentNoticeOf, updateBadgeOf, updateFeed, updateMessageOf, viewIn, workingHelpersOf, stopSessionConfirmOf, stoppedSessionOf } from '/model.js'

const card = document.getElementById('card')
const queryInput = document.getElementById('query')
const clearButton = document.getElementById('clear')
const note = document.getElementById('note')
const notice = document.getElementById('notice')
const noticeMessage = document.getElementById('notice-message')
const noticeAbout = document.getElementById('notice-about')
const noticeText = document.getElementById('notice-text')
const noticeCommand = document.getElementById('notice-command')
const noticeResult = document.getElementById('notice-result')
const noticeCopy = document.getElementById('notice-copy')
const places = document.getElementById('places')
const list = document.getElementById('list')
const summary = document.getElementById('summary')
const toast = document.getElementById('toast')
const toastText = document.getElementById('toast-text')
const approvalsBar = document.getElementById('approvals-bar')
const approvalsMode = document.getElementById('approvals-mode')
const approvalsAsk = document.getElementById('approvals-ask')

/** How long typing must pause before a search is sent. */
const SEARCH_PAUSE_MS = 300
/** How long a short answer, such as a failed mark, stays on screen. */
const TOAST_MS = 3200
/** Only the first rows rise one after another; the rest arrive with the last of them. */
const STAGGERED_ROWS = 12
/** How long a row marked complete keeps its green tick before it moves to Archived. */
const DONE_MS = 520

const PLACE_KEY = 'place'
const KEY_STEPS = { ArrowDown: 1, ArrowUp: -1 }

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
  /** The search whose matches from the newest weeks show while older weeks are still read; undefined otherwise. */
  readingOlder: undefined,
  /** Why the list could not be read; undefined when it could. */
  problem: undefined,
  /** What the companion itself has to say, such as a version mismatch. */
  note: undefined,
  /** Why the last open failed, as {message, command?}; shown until dismissed or the next open. */
  openFailure: undefined,
  /** The panel comes back from a failed open: the highlight stays on its row. */
  returning: false,
  query: '',
  /** Group ids the person folded or unfolded, over each group's start. */
  folded: new Map(GROUPS.map((group) => [group.id, group.startsFolded])),
  /** The row the arrow keys are on. */
  activeKey: undefined,
  /** What was drawn last, so an unchanged list is not drawn again. */
  drawn: '',
  /** The row whose name is being typed: {key, draft, problem?, saving?}; undefined otherwise. */
  renaming: undefined,
  /** Keys of the rows showing the green tick of a chat just marked complete. */
  completing: new Set(),
  /**
   * Keys of the chats whose subagents are unfolded, and of those showing every one past "+N more".
   * The companion keeps no per-chat state on disk, so both last until it quits.
   */
  helpersOpen: new Set(),
  helpersAll: new Set(),
  /** Keys of the chats whose approved requests are unfolded; they last until the companion quits. */
  approvedOpen: new Set(),
  /** Pick mode: the copied text as the banner shows it, {preview, words, chars, hash}; undefined outside it. */
  pick: undefined,
}

const mailbox = mailboxPanel({
  rows: () => state.view?.conversations ?? [],
  focusRows: () => queryInput.focus(),
  afterChange,
  say,
  hideForTyping: () => hidePanel(),
  showAfterRefusal: () => showPanel().catch(sayFailure),
})

/** Pick mode shows only this machine's conversations: those are the ones text can be typed into. */
function shownPlace() {
  return state.pick ? 'local' : state.place
}

/** The list as the place switch narrows it, and pick mode to the rows it offers; undefined before the first one arrived. */
function shownView() {
  if (state.view === undefined) return undefined
  const view = viewIn(state.view, shownPlace())
  return state.pick ? pickViewOf(view) : view
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
      button.setAttribute('aria-pressed', String(place.id === shownPlace()))
      button.disabled = state.pick !== undefined
      button.addEventListener('click', () => choosePlace(place.id))
      return button
    }),
  )
}

const ICONS = {
  chevron: 'M2.5 1.5L5.5 4L2.5 6.5',
  complete: 'M3 8.5l3 3 7-7.5',
  rename: 'M10.5 2.5l3 3-8 8h-3v-3zM8.5 4.5l3 3',
  web: 'M9.5 2.5h4v4M13.5 2.5L8 8M11.5 9.5v4h-9v-9h4',
  agents: 'M8 2.5a2 2 0 1 0 0 4a2 2 0 0 0 0-4M4 13v-1a4 4 0 0 1 8 0v1M3 4a1.5 1.5 0 1 0 0 3M13 4a1.5 1.5 0 1 1 0 3M1 12v-1a3 3 0 0 1 2-2.8M15 12v-1a3 3 0 0 0-2-2.8',
  hide: 'M2 2l12 12M6.5 4.2A6.3 6.3 0 0 1 8 4c3.5 0 5.5 4 5.5 4a9 9 0 0 1-1.6 2.1M9.9 11.6A5.6 5.6 0 0 1 8 12c-3.5 0-5.5-4-5.5-4a9 9 0 0 1 2-2.5',
  key: 'M2.5 10.8a2.7 2.7 0 1 0 5.4 0a2.7 2.7 0 1 0-5.4 0M7.2 8.8L13 3M11 5l1.6 1.6M9.6 6.4l1.2 1.2',
  lock: 'M4.6 7h6.8a1.6 1.6 0 0 1 1.6 1.6v3.8a1.6 1.6 0 0 1-1.6 1.6H4.6A1.6 1.6 0 0 1 3 12.4V8.6A1.6 1.6 0 0 1 4.6 7zM5.5 7V5a2.5 2.5 0 0 1 5 0v2',
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

/** The rows the arrow keys move through. */
function visibleKeys() {
  return grouped(shownView().conversations)
    .filter((group) => !isFolded(group))
    .flatMap((group) => group.rows.map(keyOf))
}

function actionButton(iconName, label, press) {
  const button = element('button', 'action')
  button.type = 'button'
  button.title = label
  button.setAttribute('aria-label', label)
  button.append(icon(iconName, '0 0 16 16'))
  button.addEventListener('click', (event) => {
    event.stopPropagation()
    press(button)
  })
  return button
}

/** The state's mark. Archived rows share one, however they got there. */
const MARK_OF = { completed: 'archived', dormant: 'archived' }

function workSystem() {
  const system = element('span', 'work-system')
  const cage = element('span', 'work-cage')
  for (let orbit = 0; orbit < 5; orbit++) {
    const track = element('span', 'work-orbit')
    track.append(element('span', 'work-traveller'))
    cage.append(track)
  }
  system.append(cage)
  return system
}

function stateMark(conversation) {
  // The row can be under Working because its children are busy. The parent's
  // own dot still describes the parent, independently of those satellites.
  let parentState = MARK_OF[conversation.state] ?? conversation.state
  const childrenWork = conversation.state === 'working' && conversation.workingSubagents > 0
  if (conversation.state === 'working' && conversation.activity === 'waiting') parentState = 'blocked'
  // Codex leaves the activity out once the parent's turn ends, so only the
  // parent's own working activity keeps its mark moving.
  else if (childrenWork && conversation.activity !== 'working') parentState = 'active'
  const mark = element('span', 'state is-' + parentState)
  mark.classList.toggle('has-unread', unreadMarkOf(conversation))
  mark.setAttribute('aria-hidden', 'true')
  if (parentState === 'working' || conversation.workingSubagents > 0) {
    // Keep the mechanism's phase when a new reply or selection redraws the list.
    mark.style.setProperty('--work-phase', `-${performance.now()}ms`)
  }
  if (parentState === 'working') {
    mark.append(workSystem())
  }
  // The circle's lock, on the chats the circle counts as waiting: apart from unread replies.
  if (conversation.state === 'blocked') {
    mark.classList.add('has-lock')
    mark.append(icon('lock', '0 0 16 16'))
  }
  if (conversation.workingSubagents > 0) {
    const workers = element('span', 'work-workers')
    for (let worker = 0; worker < conversation.workingSubagents; worker++) {
      const dot = element('span', 'work-worker')
      dot.style.setProperty('--worker-angle', `${worker * 360 / conversation.workingSubagents}deg`)
      dot.style.setProperty('--worker-radius', `${19 + (worker % 3) * 3}px`)
      dot.append(workSystem())
      workers.append(dot)
    }
    mark.append(workers)
  }
  return mark
}

function rowOf(conversation, order, folderNames, under) {
  const key = keyOf(conversation)
  const needsYou = conversation.state === 'blocked' || conversation.state === 'unread'
  const quiet = conversation.state === 'completed' || conversation.state === 'dormant'
  const row = element('div', 'row')
  row.classList.toggle('needs-you', needsYou)
  row.classList.toggle('is-quiet', quiet)
  row.classList.toggle('is-active', key === state.activeKey)
  row.classList.toggle('is-nested', under !== undefined)
  row.classList.toggle('has-workers', conversation.workingSubagents > 0)
  row.dataset.key = key
  row.id = rowId(key)
  row.style.setProperty('--order', String(Math.min(order, STAGGERED_ROWS)))
  row.setAttribute('role', 'option')
  row.setAttribute('aria-selected', String(key === state.activeKey))
  const now = Date.now()
  row.title = rowTooltipOf(conversation, now)
  const described = [under && startedFromOf(under), row.title !== conversation.open.label && row.title, conversation.cloud && SHARED_TITLE].filter(Boolean)
  if (described.length > 0) row.setAttribute('aria-description', described.join('\n'))

  const text = element('div', 'text')
  const renaming = state.renaming?.key === key ? state.renaming : undefined
  if (renaming) {
    text.append(renameField(conversation, renaming))
  } else {
    text.append(element('div', 'title', titleOf(conversation)))
  }
  const approval = approvalOf(conversation)
  const reply = replyOf(conversation)
  if (approval) text.append(approvalLine(approval))
  else if (reply) text.append(element('div', 'reply', reply))
  const reason = approvalReasonOf(conversation)
  if (reason) text.append(element('div', 'approval-reason', reason))
  // The meta line is the same in every group: tool, place, folder and age.
  const meta = element('div', 'meta')
  if (renaming?.problem) {
    meta.classList.add('is-problem')
    meta.title = renaming.problem
    meta.append(renaming.problem)
  } else {
    meta.append(detailsOf(conversation, now, folderNames))
    if (conversation.cloud) meta.title = SHARED_TITLE
  }
  text.append(meta)
  const subagents = subagentsLine(conversation, key)
  if (subagents) text.append(subagents)
  for (const shell of stuckShellsOf(conversation)) {
    const warning = shellWarningOf(shell, now)
    const line = element('div', 'shell-warning', warning)
    line.title = warning
    text.append(line)
  }
  const approved = approvedLine(conversation, key, now)
  if (approved) text.append(approved)

  const actions = element('div', 'actions')
  // Opening an archived chat, a new message in it, or its tool starting work brings it back.
  const action = rowActionOf(conversation)
  if (action === 'hide') {
    actions.append(actionButton('hide', 'Hide from agnostic-ai, until something new is written or it starts working', () => markRow(conversation, 'hidden')))
  } else if (action === 'archive') {
    actions.append(actionButton('complete', 'Mark complete, until something new is written or it starts working', (button) => completeRow(conversation, row, button)))
  }
  const page = cloudPageOf(conversation)
  if (page) actions.prepend(actionButton('web', CLOUD_PAGE_LABEL, () => openRow(page)))
  actions.prepend(actionButton('rename', 'Rename', () => startRename(conversation)))

  row.append(stateMark(conversation), text, actions)
  row.addEventListener('click', () => chooseRow(conversation))
  row.addEventListener('mousemove', () => setActive(key))
  row.addEventListener('contextmenu', (event) => {
    // Right-clicking only offers the menu: the chat stays closed and unread.
    event.preventDefault()
    if (mailbox.composing) return
    setActive(key)
    openRowMenu(conversation, event)
  })
  return row
}

/** The reply line's stand-in while a tool call waits for approval: a key and "Approve edit dashboard.tsx", or a lock and "Needs permission". */
function approvalLine(approval) {
  const line = element('div', 'reply is-approval')
  const pill = element('span', 'approval is-' + approval.icon)
  // A long detail is cut on screen; the whole pill reads on hover.
  pill.title = approval.text
  pill.append(icon(approval.icon, '0 0 16 16'), element('span', 'approval-label', approval.label))
  // The space is for screen readers and copied text; the pill's gap spaces the words on screen.
  if (approval.code) pill.append(' ', element('code', 'approval-code', approval.code))
  line.append(pill)
  return line
}

/** The line under the meta line while subagents work: "▸ 3 subagents", or their count when the server lists none of them. */
function subagentsLine(conversation, key) {
  if (workingHelpersOf(conversation).length > 0) {
    const line = element('div', 'subagents')
    line.append(helpersToggle(conversation, key))
    return line
  }
  if (conversation.workingSubagents > 0) {
    const count = conversation.workingSubagents
    const workers = element('span', 'worker-count', String(count))
    const description = `${count} working sub-agent${count === 1 ? '' : 's'}`
    workers.title = description
    workers.setAttribute('role', 'img')
    workers.setAttribute('aria-label', description)
    workers.append(icon('agents', '0 0 16 16'))
    const line = element('div', 'subagents')
    line.append(workers)
    return line
  }
  return undefined
}

/** The "▸ 3 subagents" under a chat's meta line: it folds the chat's subagents out under it, and back. */
function helpersToggle(conversation, key) {
  const open = state.helpersOpen.has(key)
  const toggle = element('button', 'helpers-toggle', helpersToggleOf(conversation, open))
  toggle.type = 'button'
  toggle.setAttribute('aria-expanded', String(open))
  toggle.setAttribute('aria-controls', helpersId(key))
  toggle.addEventListener('click', (event) => {
    // The toggle sits inside the row, which opens on a click.
    event.stopPropagation()
    const focused = document.activeElement === toggle
    if (open) state.helpersOpen.delete(key)
    else state.helpersOpen.add(key)
    draw({ force: true })
    if (focused) refocus(`.helpers-toggle[aria-controls="${helpersId(key)}"]`)
  })
  return toggle
}

/** The "▸ 3 approved for you" line under a chat's meta line: it folds out the requests approvals approved, and back. */
function approvedLine(conversation, key, now) {
  const open = state.approvedOpen.has(key)
  const label = approvedToggleOf(conversation, open)
  if (label === undefined) return undefined
  const toggle = element('button', 'approved-toggle', label)
  toggle.type = 'button'
  toggle.setAttribute('aria-expanded', String(open))
  toggle.setAttribute('aria-controls', approvedId(key))
  toggle.addEventListener('click', (event) => {
    // The toggle sits inside the row, which opens on a click.
    event.stopPropagation()
    const focused = document.activeElement === toggle
    if (open) state.approvedOpen.delete(key)
    else state.approvedOpen.add(key)
    draw({ force: true })
    if (focused) refocus(`.approved-toggle[aria-controls="${approvedId(key)}"]`)
  })
  const line = element('div', 'approved')
  line.append(toggle)
  // The list folds out inside the row, so it reads as part of it.
  if (open) line.append(approvedList(conversation, key, now))
  return line
}

/** Gives the keys back to the control a redraw replaced, as the person left them on it. */
function refocus(selector) {
  document.querySelector(selector)?.focus()
}

/** The requests approvals approved for a chat, newest first, folded out under it. */
function approvedList(conversation, key, now) {
  const box = element('ul', 'approved-list')
  box.id = approvedId(key)
  box.setAttribute('aria-label', 'Approved for you')
  // The list only tells; a click on it does not open the chat.
  box.addEventListener('click', (event) => event.stopPropagation())
  for (const call of approvedCallsOf(conversation)) {
    const item = element('li', 'approved-call', call.text)
    item.title = `${call.text}, ${ago(call.time, now)}`
    box.append(item)
  }
  return box
}

function approvedId(key) {
  return 'approved-' + key.replace(/[^A-Za-z0-9_-]/g, '-')
}

function helpersId(key) {
  return 'helpers-' + key.replace(/[^A-Za-z0-9_-]/g, '-')
}

function rowId(key) {
  return 'row-' + key.replace(/[^A-Za-z0-9_-]/g, '-')
}

/** A subagent's mark: the working gyroscope, since only working subagents are listed. */
function helperMark() {
  const mark = element('span', 'state is-working')
  mark.setAttribute('aria-hidden', 'true')
  mark.style.setProperty('--work-phase', `-${performance.now()}ms`)
  mark.append(workSystem())
  return mark
}

/** A chat's subagents, folded out under it on a tree line: a smaller copy of a row each, then "+N more". */
function helperList(conversation, key, now) {
  const box = element('div', 'helpers')
  box.id = helpersId(key)
  const { rows, more } = helperRowsOf(conversation, state.helpersAll.has(key))
  for (const helper of rows) {
    const row = element('button', 'helper-row')
    row.type = 'button'
    const text = element('span', 'text')
    text.append(element('span', 'title', helperTitleOf(helper)))
    const reply = helperReplyOf(helper)
    if (reply) text.append(element('span', 'reply', reply))
    text.append(element('span', 'meta', helperDetailsOf(helper, now)))
    if (state.pick) row.title = 'Paste the copied text into the chat it runs in'
    else row.title = helperTooltipOf(helper)
    row.append(helperMark(), text)
    row.addEventListener('click', () => {
      if (!mailbox.composing) {
        if (state.pick) chooseRow(conversation)
        else openRow(helperOpenOf(conversation, helper))
      }
    })
    box.append(row)
  }
  if (more > 0) {
    const button = element('button', 'helpers-more', `+${more} more`)
    button.type = 'button'
    button.addEventListener('click', () => {
      state.helpersAll.add(key)
      draw({ force: true })
    })
    box.append(button)
  }
  return box
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
  title.classList.toggle('is-working', group.id === 'working')
  title.setAttribute('aria-expanded', String(!folded))
  title.append(icon('chevron', '0 0 8 8'), group.title, element('span', 'count', String(group.rows.length)))
  const unread = unreadLabelOf(group.rows)
  if (unread) title.append(element('span', 'unread-count', unread))
  title.firstChild.classList.add('chevron')
  title.dataset.group = group.id
  title.addEventListener('click', () => {
    const focused = document.activeElement === title
    state.folded.set(group.id, !folded)
    draw({ force: true })
    if (focused) refocus(`.group-title[data-group="${group.id}"]`)
  })
  const head = element('div', 'group-head')
  head.append(title)
  if (group.id === 'needs-you' && !folded) {
    const all = element('button', 'group-action', 'Mark all as read')
    all.type = 'button'
    all.title = 'Take every conversation shown here out of Needs you without opening it, until something new is written in it'
    all.addEventListener('click', () => markAll(group.rows, 'seen', (conversation) => stillWaits(state.view, conversation)))
    head.append(all)
  }
  if (group.id === 'active') {
    const all = element('button', 'group-action', 'Mark all complete')
    all.type = 'button'
    all.title = 'Mark every active conversation complete, until something new is written in it or it starts working'
    all.addEventListener('click', () => markAll(group.rows, 'completed', (conversation) => stillActive(state.view, conversation)))
    head.append(all)
  }
  return head
}

function emptyMessage() {
  const empty = element('div', 'empty')
  if (state.query === '') {
    empty.append(element('strong', '', 'No conversations yet'), 'Chats from Claude Code, Codex and Cursor show up here.')
  } else {
    empty.append(element('strong', '', 'Nothing matches'), 'Try a title, a folder, a branch or words from your last prompt or the latest reply.')
  }
  return empty
}

/** Why a cloud is missing, said only where cloud rows would show. */
function cloudAside(view) {
  return shownPlace() === 'local' ? undefined : view.cloudNote
}

/** The quiet lines under the rows: tools a missing command keeps out, then a missing cloud. */
function asidesOf(view) {
  const local = shownPlace() === 'cloud' ? undefined : missingToolsNoteOf(state.view ?? view)
  return [local, cloudAside(view)].filter(Boolean).map((text) => element('p', 'aside', text))
}

/** The active approvals mode and the Ask me button above the list. */
function drawApprovals(view) {
  const header = approvalsHeaderOf(view)
  approvalsBar.hidden = header === undefined
  if (header) approvalsMode.textContent = header.label
}

/** Ask me sets the home approvals policy to ask. */
async function askAgain() {
  approvalsAsk.disabled = true
  let version
  try {
    ({ version } = await askApprovals())
  } catch (error) {
    sayFailure(error)
    return
  } finally {
    approvalsAsk.disabled = false
  }
  say('Approvals set to ask. Your tool may still use its own reviewer.')
  await afterChange(version)
}

function draw({ force } = { force: false }) {
  // A new list would take the name being typed away; it waits until the rename ends.
  if (state.renaming && !force) return
  // A new list would redraw a row in the middle of its green tick; it waits until the tick is over.
  if (state.completing.size > 0 && !force) return
  const view = shownView()
  // What could not be read comes first, then what the companion and the server have to say.
  // A cloud that is missing is not a problem with the list: it gets a quiet line under the rows.
  const said = state.problem ?? state.note ?? view?.marksNote ?? ''
  note.hidden = said === ''
  note.textContent = said
  drawApprovals(state.view)
  if (state.searching) {
    // The rows on screen answer another question, so they give way until this one's answer is here.
    state.drawn = undefined
    summary.textContent = 'Searching…'
    const waiting = element('div', 'empty is-searching')
    waiting.append(element('strong', '', 'Searching…'), 'Looking through every conversation.')
    list.replaceChildren(waiting)
    announceActive()
    return
  }
  if (!view) {
    // No list could be read. An old one would look current, so none shows.
    state.drawn = undefined
    summary.textContent = ''
    list.replaceChildren()
    if (!said) list.append(element('p', 'loading-conversations', 'Loading conversations…'))
    announceActive()
    return
  }
  // The quiet notes go in too: a silent chat's "No activity for 7m" tooltip changes with no new row data.
  const now = Date.now()
  const reasons = view.conversations.map((conversation) => quietNoteOf(conversation, now))
  const shellAges = view.conversations.flatMap((conversation) => stuckShellsOf(conversation).map((shell) => ago(shell.since, now)))
  const stillSearching = state.query !== '' && state.readingOlder === state.query
  const signature = JSON.stringify([view.conversations, reasons, shellAges, view.needsYou, view.matches, said, view.cloudNote, missingToolsNoteOf(state.view ?? view), state.query, state.activeKey, state.place, state.pick?.hash, stillSearching])
  if (!force && signature === state.drawn) return
  state.drawn = signature

  // The count covers weeks not read yet, so it waits for the last page.
  summary.textContent = stillSearching ? 'Searching…' : summaryOf(view, state.query)

  const groups = grouped(view.conversations)
  const scroll = list.scrollTop
  list.replaceChildren()
  if (groups.length === 0) {
    list.append(emptyMessage(), ...asidesOf(view))
    announceActive()
    return
  }
  const folderNames = folderNamesOf(view.conversations.map((conversation) => conversation.folder))
  let order = 0
  for (const group of groups) {
    list.append(groupHead(group))
    if (isFolded(group)) continue
    for (const conversation of group.rows) {
      list.append(rowOf(conversation, order, folderNames, group.under.get(keyOf(conversation))))
      order += 1
      const threads = !state.pick && mailbox.rowsOf(conversation)
      if (threads) list.append(threads)
      if (workingHelpersOf(conversation).length > 0 && state.helpersOpen.has(keyOf(conversation))) {
        list.append(helperList(conversation, keyOf(conversation), now))
      }
    }
  }
  list.append(...asidesOf(view))
  list.scrollTop = scroll
  holdWorkPhase(list)
  announceActive()
}

/**
 * The phase delay counts from the frame the orbit starts, not from when its mark was made,
 * so a slow draw would set every orbit back. Starting each orbit at the moment its mark was
 * made keeps the phase on the page clock however long the draw takes.
 */
function holdWorkPhase(root) {
  for (const mark of root.querySelectorAll('.state')) {
    const phase = mark.style.getPropertyValue('--work-phase')
    if (phase === '') continue
    const made = -parseFloat(phase)
    for (const animation of mark.getAnimations({ subtree: true })) {
      if (animation.animationName === 'work-orbit') animation.startTime = made
    }
  }
}

function setActive(key) {
  if (key === state.activeKey) return
  state.activeKey = key
  for (const row of list.querySelectorAll('.row')) {
    const active = row.dataset.key === key
    row.classList.toggle('is-active', active)
    row.setAttribute('aria-selected', String(active))
  }
  announceActive()
}

/** The keys stay in the search field, so it names the selected row for a screen reader. */
function announceActive() {
  const row = state.activeKey === undefined ? null : document.getElementById(rowId(state.activeKey))
  if (row) queryInput.setAttribute('aria-activedescendant', row.id)
  else queryInput.removeAttribute('aria-activedescendant')
}

function moveActive(step) {
  const key = neighbour(visibleKeys(), state.activeKey, step)
  if (key === undefined) return
  setActive(key)
  list.querySelector('.row.is-active')?.scrollIntoView({ block: 'nearest' })
}

/** The selected row while it is drawn: a key never acts on a row folded away or moved out of sight. */
function activeConversation() {
  if (state.activeKey === undefined || !document.getElementById(rowId(state.activeKey))) return undefined
  return shownView()?.conversations.find((conversation) => keyOf(conversation) === state.activeKey)
}

let toastTimer
/** A short answer over the foot for a moment; `tone` is `error` when what was asked did not happen. */
function say(message, tone = 'info') {
  toast.dataset.tone = tone
  toast.setAttribute('role', roleOfTone(tone))
  toastText.textContent = message
  toast.hidden = false
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toast.hidden = true }, TOAST_MS)
}

/** Shows what a failed call said; anything else is a bug and is thrown on. */
function sayFailure(error) {
  if (!(error instanceof CallError)) throw error
  say(error.message, 'error')
}

/** How many weeks with matches one search reads. */
const MAX_SEARCH_PAGES = 8

/**
 * The list, or a search's matches from the newest weeks that hold any, read one after the other.
 * `readSoFar` gets the matches read so far each time an older week is still to come.
 */
async function readEveryPage(asked, signal, readSoFar) {
  let reply = await readInbox(asked, undefined, signal)
  if (asked === '') return reply
  for (let pages = 1; reply.view.next && pages < MAX_SEARCH_PAGES; pages += 1) {
    readSoFar(reply)
    const older = await readInbox(asked, reply.view.next, signal)
    reply = { ...reply, view: afterPage(reply.view, older.view) }
  }
  return reply
}

/**
 * The search on its way, as {query, given, stale, staleFor, settled}. A newer search gives it up, so the server stops
 * working on it. A new list while it reads only marks it stale: it runs once more when it settles, unless its answer already
 * follows that list (staleFor), so a search slower than the lists that keep arriving still finishes. A change the
 * person made restarts it, since its answer was read before.
 */
let searchOnItsWay

/** Whether the list is on screen. A closed list keeps its search and asks for it again when it opens. */
let listShown = false

/** The last list read that landed, as {query, version}: a list event of that version brings nothing new. */
let landed

/** Reads the list again. `version` names the list an event or a change announced; one already read is not read again. */
async function refresh({ restart = false, version } = {}) {
  const asked = state.query
  if (!restart && version && landed?.version === version && landed.query === asked) return
  if (!restart && asked !== '' && searchOnItsWay?.query === asked) {
    // Two different lists announced meanwhile could arrive out of order, so then the search runs again.
    searchOnItsWay.staleFor = searchOnItsWay.stale && searchOnItsWay.staleFor !== version ? undefined : version
    searchOnItsWay.stale = true
    return
  }
  searchOnItsWay?.given.abort()
  let settle
  const search = asked === '' ? undefined : { query: asked, given: new AbortController(), stale: false, settled: new Promise((resolve) => { settle = resolve }) }
  searchOnItsWay = search
  try {
    await readList(asked, search)
  } finally {
    settle?.()
  }
}

async function readList(asked, search) {
  let reply
  let problem
  const readSoFar = (soFar) => {
    if (asked !== state.query || searchOnItsWay !== search) return
    // A search read again keeps the rows on screen: until the new answer is whole, or while older weeks were
    // still coming, until it holds as many.
    const shown = state.view?.conversations.length ?? 0
    if (!state.searching && (state.readingOlder !== asked || soFar.view.conversations.length < shown)) return
    state.readingOlder = asked
    show(soFar)
  }
  try {
    reply = await readEveryPage(asked, search?.given.signal, readSoFar)
  } catch (error) {
    if (error instanceof GivenUp) return
    if (!(error instanceof CallError)) throw error
    problem = error.message
  } finally {
    if (search && searchOnItsWay === search) searchOnItsWay = undefined
  }
  // A search typed meanwhile has its own request on the way.
  if (asked !== state.query) return
  state.readingOlder = undefined
  landed = { query: asked, version: reply?.version }
  show(reply, problem)
  if (search?.stale && listShown && !(search.staleFor && search.staleFor === reply?.version)) await refresh()
}

function show(reply, problem) {
  state.searching = false
  state.problem = problem
  state.view = reply?.view
  state.note = reply?.note
  mailbox.viewChanged(state.view, reply?.version, reply?.mailboxNativeCopy)
  if (reply?.configRoot) {
    const location = document.getElementById('config-location')
    location.textContent = 'Configuration folder: ' + reply.configRoot
    location.hidden = false
  }
  // A search's answer may leave out the selected row; the picker then selects one it offers, so Enter sends.
  if (state.pick && state.view) state.activeKey = pickKey(state.activeKey)
  draw()
  keepRowMenuCurrent()
  keepDeleteConfirmCurrent()
  keepStopSessionCurrent()
  keepStopShellCurrent()
}

/**
 * An open the companion stopped waiting for may fail later. The companion keeps that failure until the person
 * dismisses it or opens a chat, and the panel shows it each time the list shows or changes, the way a failed
 * open's shows. A notice on screen wins while it shows, and an open on its way wins over an older failure.
 */
async function showLateOpenFailure() {
  if (opensOnTheirWay > 0) return
  let failure
  try {
    failure = await readLateOpenFailure()
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    return
  }
  const shown = failure && lateOpenFailureOf(failure, lastDismissedLate)
  if (!shown || !lateFailureReplaces(state.openFailure) || opensOnTheirWay > 0) return
  showOpenFailure(shown, NOTICES.notOpened)
}

/** The late failure dismissed last; a read already past the companion when it was dismissed may return it. */
let lastDismissedLate

// The panel never activates its app, so while it shows it keeps the keys
// even after the terminal comes to the front. It hides first, and comes
// back to say why only when the conversation did not open. An open the
// companion gave up waiting for (`pending`) may still happen, so it is
// no failure and the panel stays hidden.
let latestOpen = 0
/** Opens sent and not yet answered: the companion drops an older late failure as each one arrives. */
let opensOnTheirWay = 0

async function openRow(conversation) {
  const thisOpen = ++latestOpen
  opensOnTheirWay += 1
  showOpenFailure(undefined)
  await hidePanel().catch(() => {})
  let failure
  let note
  let pending = false
  try {
    const reply = await openConversation(conversation)
    pending = reply.pending === true
    failure = reply.pending ? undefined : openFailureOf(reply)
    note = reply.pending ? undefined : subagentNoticeOf(reply)
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    failure = { message: error.message }
  } finally {
    opensOnTheirWay -= 1
  }
  // A late failure may have landed while this answer was on its way, when the panel skipped it.
  if (pending) showLateOpenFailure()
  // An open made meanwhile, such as one waiting on a macOS prompt, has its own answer.
  if (thisOpen !== latestOpen) return
  // The terminal keeps the front; the note waits in the panel for its next showing, unless a failure replaces it.
  if (note) showOpenFailure(note, NOTICES.subagent)
  if (!failure) return
  showOpenFailure(failure, NOTICES.notOpened)
  state.returning = true
  await showPanel().catch((error) => {
    state.returning = false
    sayFailure(error)
  })
}

/**
 * The notice stays until dismissed. `about`, one of NOTICES, names what it is about and sets its tone:
 * an error has a red border and a warning icon, and is read out at once. No failure hides the notice.
 */
function showOpenFailure(failure, about) {
  state.openFailure = failure
  notice.hidden = failure === undefined
  if (about) {
    notice.setAttribute('aria-label', about.label)
    notice.dataset.tone = about.tone
    // The role is set before the text, so a screen reader takes the new text with the new urgency.
    noticeMessage.setAttribute('role', roleOfTone(about.tone))
    noticeAbout.textContent = about.label + ': '
  }
  noticeText.textContent = failure?.message ?? ''
  noticeCommand.textContent = failure?.command ?? ''
  noticeCommand.hidden = noticeCopy.hidden = !failure?.command
  noticeResult.hidden = true
}

// Pick mode: the copied text goes to the row picked next, instead of the row opening.

const pickBanner = document.getElementById('pick')
const pickTitle = document.getElementById('pick-title')
const pickPreview = document.getElementById('pick-preview')
/** How long the chip takes to fly from the banner into the picked row. */
const CHIP_MS = 220
/** How long the picked row's state mark pulses before the banner closes. */
const PULSE_MS = 200
/** A send is on its way: a second click, or a double click, does nothing until it answered. */
let sending = false

function moves() {
  return document.documentElement.dataset.motion !== 'off' && !matchMedia('(prefers-reduced-motion: reduce)').matches
}

/** A click or Enter on a row: in pick mode it sends the copied text there, otherwise it opens the row. */
function chooseRow(conversation) {
  if (mailbox.choose(conversation)) return
  if (!state.pick) {
    openRow(conversation)
    return
  }
  if (!sending && takesText(conversation)) sendRow(conversation)
}

function showPick(clip) {
  const entering = !state.pick
  state.pick = clip
  pickTitle.textContent = pickTitleOf(clip)
  pickPreview.textContent = clip.preview
  pickBanner.hidden = false
  card.classList.add('is-picking')
  if (entering && moves()) {
    pickBanner.classList.remove('is-entering')
    void pickBanner.offsetWidth
    pickBanner.classList.add('is-entering')
  }
}

/** Pick mode keeps its row while offered, else selects the first row the text can be typed into, else the first. Only Enter or a click sends. */
function pickKey(active) {
  return pickKeyOf(visibleKeys(), typableRows(shownView().conversations).map(keyOf), active)
}

/** Enters pick mode on the copied text, and reports whether it did. */
async function enterPick() {
  if (mailbox.composing) {
    say('Finish or cancel the mailbox message first.', 'error')
    return false
  }
  let clip
  try {
    clip = pickableClip(await readClipboard())
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    say(unreadableClipOf(error), 'error')
    return false
  }
  if (!clip) {
    say(NOTHING_TO_SEND, 'error')
    return false
  }
  if (!state.openFailure?.late) showOpenFailure(undefined)
  mailbox.closeDetails(false)
  mailbox.closeClaims(false)
  showPick(clip)
  state.activeKey = shownView() ? pickKey(undefined) : undefined
  drawPlaces()
  draw({ force: true })
  queryInput.focus()
  return true
}

/** How long a straight send shows the row's name before the chip leaves. */
const STRAIGHT_HOLD_MS = 400

/**
 * The send shortcut opens the list in pick mode on the whole list. When exactly one row can take the text,
 * it goes there through the same send as a pick: the banner names the row, the chip flies, the list hides.
 */
async function sendFromAnywhere(from) {
  if (sending) {
    // A send already left the list: the shell showed it again, so it hides to keep the keys with the terminal.
    if (!state.pick) await hidePanel().catch(() => {})
    return
  }
  // The keys being recorded reached the shell as the send shortcut: they belong to the new shortcut, not a send.
  if (settings.recording !== undefined) return
  clearTimeout(searchTimer)
  queryInput.value = ''
  clearButton.hidden = true
  state.query = ''
  state.searching = false
  await opened(from)
  if (sending || !(await enterPick())) return
  const shown = state.pick
  const onlyRow = () => (state.query === '' && shownView() ? straightSendOf(shownView().conversations, visibleKeys()) : undefined)
  const target = onlyRow()
  if (!target) return
  state.activeKey = keyOf(target)
  pickTitle.textContent = sendingTitleOf(target)
  draw({ force: true })
  if (moves()) await new Promise((resolve) => setTimeout(resolve, STRAIGHT_HOLD_MS))
  const still = onlyRow()
  if (state.pick === shown && still && keyOf(still) === keyOf(target)) await sendRow(still)
  if (state.pick) pickTitle.textContent = pickTitleOf(state.pick)
}

function leavePick() {
  if (!state.pick) return
  state.pick = undefined
  pickBanner.hidden = true
  pickBanner.classList.remove('is-entering')
  card.classList.remove('is-picking')
  drawPlaces()
  draw({ force: true })
}

function togglePick() {
  if (sending) return
  if (state.pick) {
    leavePick()
    queryInput.focus()
  } else {
    enterPick()
  }
}

/** The chip with the first words leaves the banner and shrinks into the row's state mark, which pulses once. */
async function flyChip(conversation) {
  const row = list.querySelector(`.row[data-key="${CSS.escape(keyOf(conversation))}"]`)
  if (!row || !moves()) return
  const from = pickPreview.getBoundingClientRect()
  const to = row.querySelector('.state').getBoundingClientRect()
  const bounds = card.getBoundingClientRect()
  const chip = element('span', 'pick-chip', chipTextOf(state.pick))
  chip.setAttribute('aria-hidden', 'true')
  chip.style.left = from.left - bounds.left + 'px'
  chip.style.top = from.top - bounds.top + 'px'
  card.append(chip)
  const dx = to.left + to.width / 2 - from.left
  const dy = to.top + to.height / 2 - from.top
  const flight = chip.animate(
    [
      { transform: 'translate(0, 0) scale(1)', opacity: 1 },
      { transform: `translate(${dx}px, ${dy}px) scale(0.2)`, opacity: 0.2 },
    ],
    { duration: CHIP_MS, easing: 'cubic-bezier(0.2, 0.8, 0.2, 1)' },
  )
  try {
    await flight.finished
  } catch {
    // A cancelled flight lands at once.
  }
  chip.remove()
  row.classList.add('is-receiving')
  await new Promise((resolve) => setTimeout(resolve, PULSE_MS))
}

/**
 * Sends the copied text to a row. A clipboard that changed since the banner showed it refreshes the banner
 * and sends nothing. The list hides before the send, so a paste reaches the terminal and not the search field,
 * and comes back to say what to do when the text was not typed.
 */
async function sendRow(conversation) {
  if (sending) return
  sending = true
  try {
    const shown = state.pick
    let current
    try {
      current = pickableClip(await readClipboard())
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      say(unreadableClipOf(error), 'error')
      return
    }
    if (state.pick !== shown) return
    if (!current) {
      leavePick()
      say(NOTHING_TO_SEND, 'error')
      return
    }
    if (current.hash !== shown.hash) {
      showPick(current)
      draw({ force: true })
      showOpenFailure({ message: 'The copied text changed. Check the new text, then send again.' }, NOTICES.textChanged)
      return
    }
    if (!state.openFailure?.late) showOpenFailure(undefined)
    await flyChip(conversation)
    // Escape or the banner's close button while the chip flew takes the send back.
    if (state.pick !== shown) return
    leavePick()
    const thisSend = ++latestOpen
    await hidePanel().catch(() => {})
    let reply
    try {
      reply = await sendClipboard(conversation, shown.hash)
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      reply = { sent: false, message: error.message }
    }
    if (reply.sent || thisSend !== latestOpen) return
    showOpenFailure({ message: reply.message }, unsentNoticeOf(reply.message))
    state.returning = true
    await showPanel().catch((error) => {
      state.returning = false
      sayFailure(error)
    })
  } finally {
    sending = false
  }
}

document.getElementById('pick-close').addEventListener('click', () => {
  leavePick()
  queryInput.focus()
})

function sayCopied(message, problem) {
  noticeResult.textContent = message
  noticeResult.classList.toggle('is-problem', problem)
  noticeResult.hidden = false
}

noticeCopy.addEventListener('click', async () => {
  const failure = state.openFailure
  if (!failure?.command) return
  let copied = true
  try {
    await navigator.clipboard.writeText(failure.command)
  } catch {
    copied = false
  }
  // A newer open may have replaced the notice while the clipboard answered.
  if (state.openFailure !== failure) return
  if (copied) sayCopied('Copied. Paste it in a terminal.', false)
  else sayCopied('Could not copy it. Select the command above and copy it by hand.', true)
})

document.getElementById('notice-dismiss').addEventListener('click', () => {
  const dismissed = state.openFailure
  showOpenFailure(undefined)
  queryInput.focus()
  // A late failure stays with the companion until dismissed; any other notice may have covered one.
  if (dismissed?.late) {
    lastDismissedLate = dismissed.id
    dismissLateOpenFailure(dismissed.id).catch(sayFailure)
  }
  else showLateOpenFailure()
})

/**
 * The companion answers a change once it holds the new list, naming its version: a list of that version already
 * read, or a search on its way that will bring it, is not read again. Without a version, the list is read now.
 */
async function afterChange(version) {
  const onItsWay = searchOnItsWay
  await refresh({ restart: !version, version })
  // The search on its way was only marked to run again if it does not bring this version.
  if (searchOnItsWay === onItsWay) await onItsWay?.settled
  // A read's draw waited while a tick or a rename showed.
  draw()
}

async function markRow(conversation, mark) {
  let version
  try {
    ({ version } = await markConversation(conversation, mark))
  } catch (error) {
    sayFailure(error)
    return
  }
  await afterChange(version)
}

/**
 * Marks one row complete. Its tick turns green and stays for DONE_MS, then
 * the list read after the mark moves the row to Archived. A refused mark gives the tick back.
 */
async function completeRow(conversation, row, button) {
  const key = keyOf(conversation)
  // A second press while the tick shows would mark the same chat twice.
  if (state.completing.has(key)) return
  state.completing.add(key)
  row.classList.add('is-completing')
  button.classList.add('is-done')
  const tickShown = new Promise((resolve) => setTimeout(resolve, DONE_MS))
  let version
  try {
    ({ version } = await markConversation(conversation, 'completed'))
  } catch (error) {
    state.completing.delete(key)
    row.classList.remove('is-completing')
    button.classList.remove('is-done')
    sayFailure(error)
    // A list that arrived while the tick showed was held back; it shows now.
    draw()
    return
  }
  await tickShown
  state.completing.delete(key)
  // The row still shows its tick, so a list that left the row as it was must draw it again.
  state.drawn = undefined
  await afterChange(version)
}

// Rename

const rowMenu = document.getElementById('row-menu')
const renameItem = document.getElementById('rename')
const menuItems = {
  read: document.getElementById('mark-read'),
  rename: renameItem,
  'copy-session-id': document.getElementById('copy-session-id'),
  'send-to': document.getElementById('send-to'),
  'claim-files': document.getElementById('claim-files'),
  complete: document.getElementById('complete'),
  reopen: document.getElementById('reopen'),
  hide: document.getElementById('hide'),
  delete: document.getElementById('delete'),
  web: document.getElementById('open-web'),
  'stop-shell': document.getElementById('stop-shell'),
  'stop-session': document.getElementById('stop-session'),
}
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
  say(message, 'error')
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
  if (renaming.saving) return
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
  let version
  try {
    ({ version } = await renameConversation(conversation, title))
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    renaming.saving = false
    renaming.problem = error.message
    draw({ force: true })
    if (!focusRename()) dropRename(error.message)
    return
  }
  // The field stays, still saving, until the list with the new name is read.
  await afterChange(version)
  // A rename started on another row meanwhile keeps its field.
  if (state.renaming !== renaming) return
  state.renaming = undefined
  draw({ force: true })
}

renameItem.addEventListener('click', () => {
  if (rowMenuFor) startRename(rowMenuFor)
})
menuItems['copy-session-id'].addEventListener('click', async () => {
  const conversation = rowMenuTarget('copy-session-id')
  closeRowMenu()
  queryInput.focus()
  if (!conversation) return
  try {
    await navigator.clipboard.writeText(conversation.id)
    say('Session ID copied.')
  } catch {
    say('Could not copy the session ID.', 'error')
  }
})
menuItems['send-to'].addEventListener('click', () => {
  const conversation = rowMenuTarget('send-to')
  closeRowMenu()
  if (!conversation) return
  leavePick()
  mailbox.start(conversation)
})
menuItems['claim-files'].addEventListener('click', () => {
  const conversation = rowMenuTarget('claim-files')
  closeRowMenu()
  if (!conversation) return
  leavePick()
  mailbox.openClaims(conversation)
})
document.getElementById('mailbox-open-claims').addEventListener('click', () => {
  leavePick()
  mailbox.openClaims()
})
menuItems.read.addEventListener('click', () => {
  const conversation = rowMenuTarget('read')
  closeRowMenu()
  queryInput.focus()
  if (!conversation) return
  // Seen is the mark opening a chat sets, so the chat stays closed and a later write brings it back.
  markRow(conversation, 'seen')
})
menuItems.complete.addEventListener('click', () => {
  const conversation = rowMenuTarget('complete')
  closeRowMenu()
  queryInput.focus()
  if (!conversation) return
  // The row's own tick shows the mark, as if it was pressed.
  const row = list.querySelector(`.row[data-key="${CSS.escape(keyOf(conversation))}"]`)
  const tick = row?.querySelector('.action[aria-label^="Mark complete"]')
  if (row && tick) completeRow(conversation, row, tick)
  else markRow(conversation, 'completed')
})
for (const [item, mark] of [['reopen', 'open'], ['hide', 'hidden']]) {
  menuItems[item].addEventListener('click', () => {
    const conversation = rowMenuTarget(item)
    closeRowMenu()
    queryInput.focus()
    if (conversation) markRow(conversation, mark)
  })
}
menuItems.web.addEventListener('click', () => {
  const conversation = rowMenuTarget('web')
  closeRowMenu()
  queryInput.focus()
  if (conversation) openRow(cloudPageOf(conversation))
})
list.addEventListener('scroll', () => {
  if (!rowMenu.hidden) closeRowMenu()
})
rowMenu.addEventListener('keydown', (event) => {
  const step = KEY_STEPS[event.key]
  if (step === undefined) return
  event.preventDefault()
  const items = [...rowMenu.querySelectorAll('.menu-item')].filter((item) => !item.hidden)
  const at = items.indexOf(document.activeElement)
  items[(at + step + items.length) % items.length].focus()
})

/** A question keeps the keys: Tab goes round its controls. */
function holdTab(dialog) {
  dialog.addEventListener('keydown', (event) => {
    if (event.key !== 'Tab') return
    event.preventDefault()
    const controls = [...dialog.querySelectorAll(':is(button, select):not(:disabled, [hidden])')]
    const at = controls.indexOf(document.activeElement)
    const next = event.shiftKey ? (at <= 0 ? controls.length - 1 : at - 1) : (at + 1) % controls.length
    controls[next]?.focus()
  })
}

// Quit asks first: it ends the program, and the shortcut with it.

const quitConfirm = document.getElementById('quit-confirm')
const quitCancel = document.getElementById('quit-cancel')

function openQuitConfirm() {
  quitConfirm.hidden = false
  quitCancel.focus()
}

function closeQuitConfirm() {
  quitConfirm.hidden = true
  queryInput.focus()
}

quitCancel.addEventListener('click', closeQuitConfirm)
document.getElementById('quit-hide').addEventListener('click', () => {
  closeQuitConfirm()
  hideCompanion().catch(sayFailure)
})
// The app can end before its answer arrives.
document.getElementById('quit-go').addEventListener('click', () => quitCompanion().catch(() => {}))
document.addEventListener('mousedown', (event) => {
  if (!quitConfirm.hidden && !quitConfirm.contains(event.target)) closeQuitConfirm()
})
holdTab(quitConfirm)

// Delete permanently

const deleteConfirm = document.getElementById('delete-confirm')
const deleteTitle = document.getElementById('delete-title')
const deleteDetails = document.getElementById('delete-details')
const deleteBody = document.getElementById('delete-body')
const deleteCancel = document.getElementById('delete-cancel')
const deleteGo = document.getElementById('delete-go')
/** The conversation the confirmation asks about; undefined while none shows. */
let deleteFor

menuItems.delete.addEventListener('click', () => {
  const conversation = rowMenuTarget('delete')
  closeRowMenu()
  if (!conversation) {
    queryInput.focus()
    return
  }
  const confirm = deleteConfirmOf(conversation, Date.now(), folderNamesOf(shownView().conversations.map((row) => row.folder)))
  if (confirm.unsupported) {
    // Nothing is sent: the server already said this chat cannot be deleted from here.
    showOpenFailure({ message: confirm.unsupported }, NOTICES.cannotDelete)
    queryInput.focus()
    return
  }
  deleteFor = conversation
  deleteTitle.textContent = confirm.title
  deleteDetails.textContent = confirm.details
  deleteBody.textContent = confirm.body
  deleteGo.disabled = false
  deleteConfirm.hidden = false
  deleteCancel.focus()
})

function closeDeleteConfirm() {
  deleteConfirm.hidden = true
  deleteFor = undefined
  queryInput.focus()
}

/** A new list that no longer offers Delete for the row, such as one where it was written in again, closes the confirmation. */
function keepDeleteConfirmCurrent() {
  if (deleteConfirm.hidden || !deleteFor || deleteGo.disabled) return
  const key = keyOf(deleteFor)
  const current = shownView()?.conversations.find((conversation) => keyOf(conversation) === key)
  if (current && rowMenuOf(current).includes('delete') && !current.deleteUnsupported) return
  closeDeleteConfirm()
}

deleteCancel.addEventListener('click', closeDeleteConfirm)
document.addEventListener('mousedown', (event) => {
  if (!deleteConfirm.hidden && !deleteConfirm.contains(event.target)) closeDeleteConfirm()
})
holdTab(deleteConfirm)
deleteGo.addEventListener('click', async () => {
  const conversation = deleteFor
  if (!conversation) return
  deleteGo.disabled = true
  let reply
  let version
  try {
    ({ body: reply, version } = await deleteConversation(conversation))
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    closeDeleteConfirm()
    // A refusal can list what is still on disk: it stays until dismissed, unlike the toast.
    showOpenFailure({ message: error.message }, NOTICES.notDeleted)
    await afterChange()
    return
  }
  closeDeleteConfirm()
  const { message, note: problem } = deletedOf(reply)
  if (problem) showOpenFailure({ message: message + ' ' + problem }, NOTICES.notTidied)
  else say(message)
  await afterChange(version)
})

const stopShellDialog = document.getElementById('stop-shell-dialog')
const stopShellChoice = document.getElementById('stop-shell-choice')
const stopShellCommand = document.getElementById('stop-shell-command')
const stopShellDetails = document.getElementById('stop-shell-details')
const stopShellCancel = document.getElementById('stop-shell-cancel')
const stopShellGo = document.getElementById('stop-shell-go')
let stopShellFor
let stoppingShell = false

menuItems['stop-shell'].addEventListener('click', () => {
  const conversation = rowMenuTarget('stop-shell')
  closeRowMenu()
  queryInput.focus()
  if (!conversation || stoppingShell) return
  const shells = stuckShellsOf(conversation)
  const now = Date.now()
  stopShellFor = { conversation, shells }
  stopShellChoice.replaceChildren(new Option('Choose a shell', ''))
  shells.forEach((shell, index) => {
    stopShellChoice.add(new Option(shellWarningOf(shell, now) + ' (PID ' + shell.pid + ')', String(index)))
  })
  stopShellChoice.hidden = shells.length === 1
  showShellChoice(shells.length === 1 ? shells[0] : undefined)
  stopShellDialog.hidden = false
  stopShellCancel.focus()
})

function showShellChoice(shell) {
  stopShellFor.selected = shell
  stopShellCommand.hidden = !shell
  stopShellCommand.textContent = shell ? shellCommandOf(shell) : ''
  stopShellDetails.hidden = !shell
  stopShellDetails.textContent = shell ? stopShellDetailsOf(shell, Date.now()) : ''
  stopShellGo.disabled = !shell
}

function closeStopShell() {
  stopShellDialog.hidden = true
  stopShellFor = undefined
  queryInput.focus()
}

function stopShellRow() {
  const key = keyOf(stopShellFor.conversation)
  return shownView()?.conversations.find((conversation) => keyOf(conversation) === key)
}

function shellChanged() {
  closeStopShell()
  showOpenFailure({ message: 'The shell changed. Check the list and choose it again.' }, NOTICES.shellNotStopped)
}

function keepStopShellCurrent() {
  if (!stopShellFor || stoppingShell) return
  const row = stopShellRow()
  const { selected, shells } = stopShellFor
  if (selected ? currentShellOf(row, selected) : shells.some((shell) => currentShellOf(row, shell))) return
  shellChanged()
}

stopShellChoice.addEventListener('change', () => {
  if (!stopShellFor) return
  const choice = stopShellChoice.value
  const shell = choice === '' ? undefined : stopShellFor.shells[Number(choice)]
  if (shell && !currentShellOf(stopShellRow(), shell)) {
    shellChanged()
    return
  }
  showShellChoice(shell)
})
stopShellCancel.addEventListener('click', closeStopShell)
document.addEventListener('mousedown', (event) => {
  if (!stopShellDialog.hidden && !stopShellDialog.contains(event.target)) closeStopShell()
})
holdTab(stopShellDialog)
stopShellGo.addEventListener('click', async () => {
  if (!stopShellFor?.selected || stoppingShell) return
  const { conversation, selected } = stopShellFor
  if (!currentShellOf(stopShellRow(), selected)) {
    shellChanged()
    return
  }
  stoppingShell = true
  stopShellGo.disabled = true
  stopShellChoice.disabled = true
  let result
  try {
    result = await stopShell(conversation, selected)
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    showOpenFailure({ message: error.message }, NOTICES.shellNotStopped)
  } finally {
    stoppingShell = false
    stopShellChoice.disabled = false
    if (stopShellFor) closeStopShell()
  }
  if (result) {
    const message = stoppedShellOf(result.body)
    if (result.body.stillRunning?.length) showOpenFailure({ message }, NOTICES.shellProcessesLeft)
    else say(message)
  }
  await afterChange(result?.version)
})

// Stop chat ends a live local Claude Code chat's process, after a question.

const stopSessionConfirm = document.getElementById('stop-session-confirm')
const stopSessionCancel = document.getElementById('stop-session-cancel')
const stopSessionGo = document.getElementById('stop-session-go')
/** Each question has its own identity while a stop reply is on its way. */
let stopSessionFor

menuItems['stop-session'].addEventListener('click', () => {
  const conversation = rowMenuTarget('stop-session')
  closeRowMenu()
  if (!conversation) {
    queryInput.focus()
    return
  }
  const { title, body } = stopSessionConfirmOf(conversation)
  stopSessionFor = { conversation }
  document.getElementById('stop-session-title').textContent = title
  document.getElementById('stop-session-body').textContent = body
  stopSessionGo.disabled = false
  stopSessionConfirm.hidden = false
  stopSessionCancel.focus()
})

function closeStopSession() {
  stopSessionConfirm.hidden = true
  stopSessionFor = undefined
  queryInput.focus()
}

const sameProcess = (a, b) => a?.pid === b?.pid && a?.procStart === b?.procStart

/** A new list where the chat no longer runs here, or runs in another process, closes the question. */
function keepStopSessionCurrent() {
  if (stopSessionConfirm.hidden || !stopSessionFor || stopSessionGo.disabled) return
  const { conversation } = stopSessionFor
  const key = keyOf(conversation)
  const current = shownView()?.conversations.find((conversation) => keyOf(conversation) === key)
  if (current?.live && current.stoppable && sameProcess(current.stoppable, conversation.stoppable)) return
  closeStopSession()
}

stopSessionCancel.addEventListener('click', closeStopSession)
document.addEventListener('mousedown', (event) => {
  if (!stopSessionConfirm.hidden && !stopSessionConfirm.contains(event.target)) closeStopSession()
})
holdTab(stopSessionConfirm)
stopSessionGo.addEventListener('click', async () => {
  keepStopSessionCurrent()
  const confirmation = stopSessionFor
  if (!confirmation || stopSessionGo.disabled) return
  const { conversation } = confirmation
  stopSessionGo.disabled = true
  let result
  let version
  try {
    const response = await stopSession(conversation, conversation.stoppable)
    result = stoppedSessionOf(response.body ?? {})
    version = response.version
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    result = { message: error.message, problem: true }
  } finally {
    if (stopSessionFor === confirmation) closeStopSession()
  }
  const { message, problem } = result
  if (problem) showOpenFailure({ message }, NOTICES.sessionNotStopped)
  else say(message)
  await afterChange(version)
})

/** A Mark all run is on its way, so another press does nothing. Each mark redraws the list, so the button cannot hold this. */
let markingAll = false

/**
 * Gives each of the rows `mark`, one after the other. A row `keep` turns down is left as it is. Only the last
 * mark waits for the list read after it; the companion reads the list soon after each other one. The first
 * refused mark stops the run and says why; the rows after it keep their place. No row shows a tick: this is
 * tidying up.
 */
async function markAll(rows, mark, keep = () => true) {
  if (markingAll) return
  markingAll = true
  let version
  try {
    for (const [index, conversation] of rows.entries()) {
      if (!keep(conversation)) continue
      // Decided now: a row that stops being kept later leaves the last mark without a version, so the list
      // is read at once and the poll's event brings the rest.
      ({ version } = await markConversation(conversation, mark, { later: rows.slice(index + 1).some(keep) }))
    }
  } catch (error) {
    sayFailure(error)
  } finally {
    markingAll = false
  }
  await afterChange(version)
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

function clearSearch() {
  queryInput.value = ''
  searchSoon()
  queryInput.focus()
}

queryInput.addEventListener('input', searchSoon)
clearButton.addEventListener('click', clearSearch)
// Settings

const menu = document.getElementById('menu')
const menuButton = document.getElementById('menu-button')
const menuProblem = document.getElementById('menu-problem')
const isMac = System.IsMac()

const settings = {
  /** menu.json, read on the first open. */
  items: undefined,
  /** Each row's control by its id in menu.json. */
  controls: new Map(),
  /** Each row's placed element by its id, to hide a row whose value could not be read. */
  rows: new Map(),
  /** What the shell last said: {shortcut, shortcutProblem?, startAtLogin, startAtLoginProblem?}. */
  preferences: undefined,
  /** How the pages look: {animate, dimWhenIdle, smallCompanion, presence}. */
  appearance: undefined,
  /** The server's mailbox settings: {approveAgentSends}. */
  mailbox: undefined,
  /** The update entry: {state, version?}. */
  update: { state: 'unavailable' },
  /** The id of the shortcut the next key press becomes; undefined otherwise. */
  recording: undefined,
  /** Why the last change was refused; shown until the next change. */
  refusal: undefined,
}

/** A setting the server keeps, such as mailbox approval, hides its row when it cannot be read. */
function unreadSetting(error) {
  if (!(error instanceof CallError)) throw error
  return undefined
}

/** Saves one setting and shows what the shell now holds, or why it refused. */
async function change(key, save) {
  try {
    settings[key] = await save()
    settings.refusal = undefined
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    settings.refusal = error.message
  }
  drawSettings()
}

const flip = (id) => !menuRowsOf(settings.items, settings).find((row) => row.id === id).value

const SAVE_SHORTCUT = { shortcut: saveShortcut, sendShortcut: saveSendShortcut }

/** A click on a shortcut starts taking its new keys, or stops when it was already. */
function record(id) {
  settings.recording = settings.recording === id ? undefined : id
  drawSettings()
}

const MENU_ACTIONS = {
  animate: () => change('appearance', () => saveAnimate(flip('animate'))),
  dimWhenIdle: () => change('appearance', () => saveDimWhenIdle(flip('dimWhenIdle'))),
  smallCompanion: () => change('appearance', () => saveSmallCompanion(flip('smallCompanion'))),
  startAtLogin: () => change('preferences', () => saveStartAtLogin(flip('startAtLogin'))),
  approveAgentSends: () => change('mailbox', () => saveMailboxSettings({ approveAgentSends: flip('approveAgentSends') })),
  shortcut: () => record('shortcut'),
  sendShortcut: () => record('sendShortcut'),
  update: () => {
    closeMenu()
    if (settings.update.state === 'available' || settings.update.state === 'failed') openUpdateConfirm()
    else checkForUpdates().catch(sayFailure)
  },
  hide: () => hideCompanion().catch(sayFailure),
  quit: () => {
    closeMenu()
    openQuitConfirm()
  },
}

/** Builds the rows once, in menu.json order; drawSettings fills in their values. */
function buildMenu(rows) {
  for (const row of rows) {
    const act = MENU_ACTIONS[row.id]
    if (!act) throw new Error(`the menu item ${row.id} has no action here`)
    let control
    let placed
    if (row.kind === 'action') {
      control = element('button', 'menu-item', row.label)
      placed = control
    } else {
      const labelId = `menu-${row.id}-label`
      const label = element('span', 'menu-label', row.label)
      label.id = labelId
      control = element('button', row.kind === 'switch' ? 'switch' : 'shortcut')
      control.setAttribute('aria-labelledby', labelId)
      if (row.kind === 'switch') {
        control.setAttribute('role', 'switch')
        control.append(element('span', ''))
      }
      placed = element('div', 'menu-row')
      placed.append(label, control)
    }
    control.type = 'button'
    if (row.hint) control.title = row.hint
    control.addEventListener('click', act)
    settings.controls.set(row.id, control)
    settings.rows.set(row.id, placed)
    // The problem line sits between the settings and the actions.
    if (row.kind === 'action') menu.append(placed)
    else menu.insertBefore(placed, menuProblem)
  }
}

function drawSettings() {
  for (const row of menuRowsOf(settings.items, settings, isMac)) {
    const control = settings.controls.get(row.id)
    if (!control) continue
    settings.rows.get(row.id).hidden = Boolean(row.unavailable)
    if (row.unavailable) continue
    if (row.kind === 'action') control.textContent = row.label
    control.disabled = Boolean(row.disabled)
    if (row.kind === 'switch') control.setAttribute('aria-checked', String(row.value))
    if (row.kind !== 'shortcut') continue
    const recording = settings.recording === row.id
    control.classList.toggle('is-recording', recording)
    if (recording) {
      control.replaceChildren('Press the new keys')
    } else {
      control.replaceChildren(...keycapsOf(row.value, isMac).map((keycap) => element('kbd', '', keycap)))
    }
  }
  const preferences = settings.preferences
  const problems = settings.refusal !== undefined ? [settings.refusal] : [preferences.shortcutProblem, preferences.sendShortcutProblem, preferences.startAtLoginProblem].filter(Boolean)
  menuProblem.hidden = problems.length === 0
  menuProblem.textContent = problems.join('. ')
}

function closeMenu() {
  settings.recording = undefined
  menu.hidden = true
  menuButton.setAttribute('aria-expanded', 'false')
}

/** Opens the menu with the values in effect; `focus` names the row to focus. */
async function openMenu(focus) {
  try {
    const showRead = updates.since()
    const [items, preferences, appearance, update, mailbox] = await Promise.all([settings.items ?? readMenu(), readPreferences(), readAppearance(), readUpdate(), readMailboxSettings().catch(unreadSetting)])
    if (!settings.items) {
      settings.items = items
      buildMenu(menuRowsOf(items, { preferences, appearance, mailbox, update }, isMac))
    }
    settings.preferences = preferences
    settings.appearance = appearance
    settings.mailbox = mailbox
    showRead(update)
  } catch (error) {
    sayFailure(error)
    return
  }
  settings.refusal = undefined
  drawSettings()
  menu.hidden = false
  menuButton.setAttribute('aria-expanded', 'true')
  if (focus) settings.controls.get(focus)?.focus()
}

// Update: the question before it restarts, and each step after.

const updateConfirm = document.getElementById('update-confirm')
const updateCancel = document.getElementById('update-cancel')
const updateStatus = document.getElementById('update-status')
const updateText = document.getElementById('update-text')
const updateLog = document.getElementById('update-log')
const updateRetry = document.getElementById('update-retry')
/** The status whose message was dismissed; a new status shows again. */
let updateDismissed

function openUpdateConfirm() {
  document.getElementById('update-title').textContent = `Update to v${settings.update.version} and restart?`
  updateConfirm.hidden = false
  updateCancel.focus()
}

function closeUpdateConfirm() {
  updateConfirm.hidden = true
  queryInput.focus()
}

function drawUpdateStatus() {
  const message = updateMessageOf(settings.update)
  updateStatus.hidden = !message || JSON.stringify(settings.update) === updateDismissed
  if (!message) return
  updateText.textContent = message.text
  updateLog.hidden = !message.log
  updateLog.textContent = message.log ? `Details: ${message.log}` : ''
  updateRetry.hidden = !message.retry
}

updateCancel.addEventListener('click', closeUpdateConfirm)
document.getElementById('update-go').addEventListener('click', () => {
  closeUpdateConfirm()
  startUpdate().catch(sayFailure)
})
updateRetry.addEventListener('click', () => startUpdate().catch(sayFailure))
document.getElementById('update-dismiss').addEventListener('click', () => {
  updateDismissed = JSON.stringify(settings.update)
  drawUpdateStatus()
})
document.addEventListener('mousedown', (event) => {
  if (!updateConfirm.hidden && !updateConfirm.contains(event.target)) closeUpdateConfirm()
})
holdTab(updateConfirm)

/** Names the settings button after a known update, with a dot on it, and says the step. */
function showUpdate(update) {
  settings.update = update
  if (JSON.stringify(update) !== updateDismissed) updateDismissed = undefined
  drawUpdateStatus()
  const badge = updateBadgeOf(update)
  menuButton.setAttribute('aria-label', badge ?? 'Settings')
  menuButton.title = badge ?? 'Settings'
  if (badge) menuButton.dataset.update = update.version
  else delete menuButton.dataset.update
}

const updates = updateFeed(showUpdate)

/** A change made in the right-click menu shows in an open menu at once. */
function heard(key, value) {
  settings[key] = value
  if (!menu.hidden) drawSettings()
}

menuButton.addEventListener('click', () => (menu.hidden ? openMenu() : closeMenu()))
document.addEventListener('mousedown', (event) => {
  if (!rowMenu.hidden && !rowMenu.contains(event.target)) closeRowMenu()
  if (menu.hidden || menu.contains(event.target) || menuButton.contains(event.target)) return
  closeMenu()
})
Events.On('appearance', (event) => heard('appearance', event.data))
Events.On('preferences', (event) => heard('preferences', event.data))
Events.On('update', (event) => {
  updates.event(event.data)
  if (!menu.hidden && settings.items) drawSettings()
})
readUpdate().then(updates.since(), sayFailure)
approvalsAsk.addEventListener('click', askAgain)
const dashboardButton = document.getElementById('dashboard')
dashboardButton.addEventListener('click', async () => {
  dashboardButton.disabled = true
  try {
    await openDashboard()
  } catch (error) {
    sayFailure(error)
    return
  } finally {
    dashboardButton.disabled = false
  }
  await hidePanel()
})

document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape') {
    event.preventDefault()
    const closes = {
      'quit-confirm': closeQuitConfirm,
      'update-confirm': closeUpdateConfirm,
      'delete-confirm': closeDeleteConfirm,
      'stop-shell': closeStopShell,
      'stop-session': closeStopSession,
      'row-menu': closeRowMenu,
      rename: escapeRename,
      settings: () => {
        closeMenu()
        queryInput.focus()
      },
      pick: () => {
        leavePick()
        queryInput.focus()
      },
      mailbox: mailbox.escape,
      search: clearSearch,
      panel: () => hidePanel().catch(sayFailure),
    }
    closes[escapeTarget({ quitConfirmOpen: !quitConfirm.hidden, updateConfirmOpen: !updateConfirm.hidden, deleteConfirmOpen: !deleteConfirm.hidden, stopShellOpen: !stopShellDialog.hidden, stopSessionOpen: !stopSessionConfirm.hidden, rowMenuOpen: !rowMenu.hidden, renaming: state.renaming !== undefined, settingsOpen: !menu.hidden, picking: state.pick !== undefined, mailboxOpen: mailbox.open, searching: queryInput.value !== '' })]()
    return
  }
  if (event.target.closest('.mailbox-card') || event.target.closest('.mailbox-threads')) return
  // Menus and confirmations handle Enter and Space on their own controls.
  if (!rowMenu.hidden || !deleteConfirm.hidden || !stopShellDialog.hidden || !stopSessionConfirm.hidden || !quitConfirm.hidden || !updateConfirm.hidden || state.renaming) return
  if (togglesPick(event, isMac) && settings.recording === undefined) {
    event.preventDefault()
    togglePick()
    return
  }
  if (settings.recording !== undefined) {
    // Every key belongs to the new shortcut until one is complete.
    event.preventDefault()
    const shortcut = shortcutOf(event, isMac)
    if (shortcut === undefined) return
    const save = SAVE_SHORTCUT[settings.recording]
    settings.recording = undefined
    const refused = refusedShortcutOf(shortcut, isMac)
    if (refused) {
      settings.refusal = refused
      drawSettings()
      return
    }
    change('preferences', () => save(shortcut))
    return
  }
  // The list hides behind the open settings menu, so it takes no keys.
  if (!menu.hidden) return
  if (!mailbox.composing && (event.key === 'ContextMenu' || (event.key === 'F10' && event.shiftKey))) {
    const conversation = activeConversation()
    if (!conversation) return
    const bounds = document.getElementById(rowId(keyOf(conversation))).getBoundingClientRect()
    event.preventDefault()
    openRowMenu(conversation, { clientX: bounds.left, clientY: bounds.bottom })
    return
  }
  const step = KEY_STEPS[event.key]
  if (step !== undefined) {
    event.preventDefault()
    moveActive(step)
    return
  }
  // A focused button takes Enter as its own click.
  if (event.target.closest('button')) return
  if (event.key === 'Enter') {
    // The selected row answers the last search, not the one on its way.
    if (state.pick && state.searching) return
    const conversation = activeConversation()
    if (conversation) chooseRow(conversation)
  } else if (!mailbox.composing && startsRename(event)) {
    const conversation = activeConversation()
    if (!conversation) return
    event.preventDefault()
    startRename(conversation)
  }
})

/** The panel opened: it grows out of the corner nearest the bubble, and the search takes the keys. */
function opened(from) {
  listShown = true
  card.dataset.from = from
  card.classList.remove('is-opening')
  if (document.documentElement.dataset.motion !== 'off') {
    void card.offsetWidth
    card.classList.add('is-opening')
  }
  toast.hidden = true
  closeMenu()
  closeRowMenu()
  deleteConfirm.hidden = true
  deleteFor = undefined
  stopShellDialog.hidden = true
  stopShellFor = undefined
  stopSessionConfirm.hidden = true
  stopSessionFor = undefined
  quitConfirm.hidden = true
  updateConfirm.hidden = true
  state.renaming = undefined
  if (!state.returning) {
    state.activeKey = undefined
    leavePick()
  }
  state.returning = false
  queryInput.focus()
  queryInput.select()
  draw({ force: true })
  return refresh().then(showLateOpenFailure)
}

card.addEventListener('animationend', (event) => {
  if (event.target === card) card.classList.remove('is-opening')
})
// An opening left over would play again when animation is turned back on.
document.addEventListener('motion', () => {
  if (document.documentElement.dataset.motion === 'off') card.classList.remove('is-opening')
})

drawPlaces()
Events.On('inbox', (event) => {
  // A search costs the server a read of every week it covers; a closed list asks again when it opens.
  if (!listShown && state.query !== '') {
    showLateOpenFailure()
    return
  }
  refresh({ version: event?.data }).then(showLateOpenFailure)
})
Events.On('panel-shown', (event) => opened(event.data))
Events.On('panel-hidden', () => { listShown = false })
Events.On('send-shown', (event) => sendFromAnywhere(event.data))
// Quit in the right-click menu: the list opens on the question.
Events.On('quit-asked', (event) => {
  opened(event.data)
  openQuitConfirm()
})
// Update to vX.Y.Z in the right-click menu, or a check that found one: the list opens on the question.
Events.On('update-asked', async (event) => {
  opened(event.data)
  const showRead = updates.since()
  try {
    showRead(await readUpdate())
  } catch (error) {
    sayFailure(error)
    return
  }
  if (settings.update.state === 'available' || settings.update.state === 'failed') openUpdateConfirm()
})
// Open from anywhere in the right-click menu: the list opens on its menu, ready to change the keys.
Events.On('settings-shown', (event) => {
  opened(event.data)
  openMenu('shortcut')
})
// Send copied text in the right-click menu: the same, on the send shortcut.
Events.On('send-settings-shown', (event) => {
  opened(event.data)
  openMenu('sendShortcut')
})
async function showFirstAppPanel() {
  if (!new URLSearchParams(location.search).has('packaged')) return
  const key = 'packaged-panel-shown'
  try {
    if (localStorage.getItem(key)) return
  } catch {
    // A webview without storage still shows the panel.
  }
  await showPanel()
  try {
    localStorage.setItem(key, '1')
  } catch {
    // The next launch will show it again.
  }
}

showFirstAppPanel().catch(sayFailure)
refresh()
