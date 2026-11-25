import { Events, System } from '/wails/runtime.js'
import { mailboxPanel } from '/panel-mailbox.js'
import { CallError, GivenUp, askApprovals, checkForUpdates, deleteConversation, hideCompanion, hidePanel, markConversation, openConversation, openDashboard, openReleasePage, quitCompanion, readAppearance, readClipboard, readInbox, readMenu, readMailboxSettings, readPreferences, readUpdate, renameConversation, readLateOpenNotice, dismissLateOpenNotice, saveAnimate, saveCheckForUpdates, saveDimWhenIdle, saveMailboxSettings, saveSendShortcut, saveShortcut, saveSmallCompanion, saveStartAtLogin, sendClipboard, showPanel, peekPanel, startUpdate, stopSession, stopShell, readKeptShells, keepShell, forgetShells } from '/api.js'
import { currentShellOf, finishedKeepsOf, firstWaitingOf, keepOf, keptShellNoteOf, keptShellsOfRow, keptViewOf, needsReasonOf, shellCommandOf, shellLabelsOf, shellsLeadOf, shellWarningOf, stateIconOf, stopShellDetailsOf, stopShellWarningOf, stoppedShellOf, stuckShellsOf, missingToolsNoteOf } from '/model.js'
import { CLOUD_PAGE_LABEL, GROUPS, NOTHING_TO_SEND, NOTICES, lateOpenNoticeOf, PLACES, SHARED_TITLE, afterPage, companionApprovalsNeedSetup, approvalOf, approvalReasonOf, approvalsHeaderOf, approvedCallsOf, approvedToggleOf, ago, chipTextOf, cloudPageOf, confirmsCandidate, deleteConfirmOf, deletedOf, detailsOf, escapeTarget, folderNamesOf, grouped, helperDetailsOf, helperOpenOf, helperReplyOf, helperRowsOf, helperTitleOf, helperTooltipOf, helpersToggleOf, keyOf, lateNoticeReplaces, subtreeToggleOf, keycapsOf, menuRowsOf, neighbour, openFailureOf, pickKeyOf, pickTitleOf, pickViewOf, pickableClip, quietNoteOf, renameOf, renames, replyOf, rowActionOf, refusedShortcutOf, rowMenuOf, rowTooltipOf, roleOfTone, sendingTitleOf, shortcutOf, startedFromOf, startsRename, stillActive, stillWaits, straightSendOf, openNoticeOf, unreadLabelOf, unreadMarkOf, summaryOf, takesText, titleOf, togglesPick, typableRows, unreadableClipOf, unsentNoticeOf, updateBadgeOf, updateConfirmOf, updateFeed, updateMessageOf, viewIn, workingHelpersOf, stopSessionConfirmOf, stoppedSessionOf } from '/model.js'
import { decideApproval } from '/api.js'
import { PENDING_CHANGED, pendingOf } from '/model.js'
import { CLAIM_HELP, claimChipOf, helpOf, needsPersonOf } from '/model.js'
import { openDocs } from '/api.js'
import { approvalsMenu } from '/panel-approvals.js'

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
const tooltip = document.getElementById('tooltip')

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

// One jump from search leaves the header and group controls reachable on the next Tab cycle.
let tabToConversation = true

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
  /** Subagents whose own subagents the person folded shut, by chat key and subagent id; the rest show unfolded. */
  helpersShut: new Set(),
  /** Keys of the chats whose approved requests are unfolded; they last until the companion quits. */
  approvedOpen: new Set(),
  /** Pick mode: the copied text as the banner shows it, {preview, words, chars, hash}; undefined outside it. */
  pick: undefined,
  /** The stuck shells the person kept running, as the companion settings hold them. */
  keptShells: [],
  /**
   * Each shell a Stop on its line was confirmed for, by shellKeyOf: {key, stopping} while the request runs, then
   * {key, text, problem, version} until a list other than the one the stop made is shown.
   */
  shellStops: new Map(),
}

/** A help link: a "?" that names its docs page on hover and opens it in the browser. */
for (const link of document.querySelectorAll('[data-help]')) {
  const { path, title } = helpOf(link.dataset.help)
  link.title = title
  link.setAttribute('aria-label', title)
  link.addEventListener('click', () => openDocs(path).catch(sayFailure))
}
for (const line of document.querySelectorAll('[data-claim-help]')) line.textContent = CLAIM_HELP

const mailbox = mailboxPanel({
  rows: () => state.view?.conversations ?? [],
  shownRows: () => list.querySelectorAll('.row[data-key]'),
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
  const view = viewIn(keptViewOf(state.view, state.keptShells, Date.now()), shownPlace())
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
  web: 'M9.5 2.5h4v4M13.5 2.5L8 8M11.5 9.5v4h-9v-9h4',
  agents: 'M8 2.5a2 2 0 1 0 0 4a2 2 0 0 0 0-4M4 13v-1a4 4 0 0 1 8 0v1M3 4a1.5 1.5 0 1 0 0 3M13 4a1.5 1.5 0 1 1 0 3M1 12v-1a3 3 0 0 1 2-2.8M15 12v-1a3 3 0 0 0-2-2.8',
  hide: 'M2 2l12 12M6.5 4.2A6.3 6.3 0 0 1 8 4c3.5 0 5.5 4 5.5 4a9 9 0 0 1-1.6 2.1M9.9 11.6A5.6 5.6 0 0 1 8 12c-3.5 0-5.5-4-5.5-4a9 9 0 0 1 2-2.5',
  key: 'M2.5 10.8a2.7 2.7 0 1 0 5.4 0a2.7 2.7 0 1 0-5.4 0M7.2 8.8L13 3M11 5l1.6 1.6M9.6 6.4l1.2 1.2',
  lock: 'M4.6 7h6.8a1.6 1.6 0 0 1 1.6 1.6v3.8a1.6 1.6 0 0 1-1.6 1.6H4.6A1.6 1.6 0 0 1 3 12.4V8.6A1.6 1.6 0 0 1 4.6 7zM5.5 7V5a2.5 2.5 0 0 1 5 0v2',
  terminal: 'M3.6 2.5h8.8a1.6 1.6 0 0 1 1.6 1.6v7.8a1.6 1.6 0 0 1-1.6 1.6H3.6A1.6 1.6 0 0 1 2 11.9V4.1a1.6 1.6 0 0 1 1.6-1.6zM5 6l2.2 2L5 10M8.8 10.5h2.4',
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
    // A pointer click on a row's buttons while they still fade in was aimed at the row under them.
    const showing = button.closest('.actions')
    if (showing && event.detail > 0 && Number(getComputedStyle(showing).opacity) < 1) return
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
  // The circle's lock, on the chats the circle counts as waiting: apart from unread replies. A shell wait has a terminal instead.
  const waitIcon = stateIconOf(conversation)
  if (waitIcon) {
    mark.classList.add('has-lock')
    mark.append(icon(waitIcon, '0 0 16 16'))
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

/** The title of a listed session, for a conflict line; undefined for one the list does not hold. */
function sessionTitleOf(ref) {
  const listed = state.view?.conversations.find((conversation) => keyOf(conversation) === keyOf(ref))
  return listed && titleOf(listed)
}

/** A path conflict's reason line: a button that opens the session's claims, where it can release or dismiss. */
function conflictLine(conversation, text) {
  const line = element('button', 'needs-reason is-conflict', text)
  line.type = 'button'
  line.addEventListener('click', (event) => {
    event.stopPropagation()
    mailbox.openClaims(conversation)
  })
  return line
}

/** The meta line's chip for a session that claims files: it opens that session's claims. */
function claimChip(conversation, text) {
  const chip = element('button', 'claim-chip', text)
  chip.type = 'button'
  chip.setAttribute('aria-label', `${text} by ${titleOf(conversation)}`)
  chip.addEventListener('click', (event) => {
    event.stopPropagation()
    mailbox.openClaims(conversation)
  })
  return chip
}

function rowOf(conversation, order, folderNames, under) {
  const key = keyOf(conversation)
  const needsYou = needsPersonOf(conversation)
  const canRename = renames(conversation)
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
  row.tabIndex = 0
  row.setAttribute('role', 'option')
  if (canRename) row.setAttribute('aria-keyshortcuts', 'F2')
  row.setAttribute('aria-selected', String(key === state.activeKey))
  const now = Date.now()
  const tip = rowTooltipOf(conversation, now)
  row.dataset.tooltip = tip
  const described = [under && startedFromOf(under), tip !== conversation.open.label && tip, conversation.cloud && SHARED_TITLE].filter(Boolean)
  if (described.length > 0) row.setAttribute('aria-description', described.join('\n'))

  const text = element('div', 'text')
  const renaming = state.renaming?.key === key ? state.renaming : undefined
  if (renaming) {
    text.append(renameField(conversation, renaming))
  } else {
    text.append(canRename ? renamableTitle(conversation, tip) : element('div', 'title', titleOf(conversation)))
  }
  const approval = approvalOf(conversation)
  const reply = replyOf(conversation)
  // A stop that went through shows on the line before the next list drops the shell.
  const stopped = (shell) => {
    const stop = state.shellStops.get(shellKeyOf(shell))
    return Boolean(stop?.text) && !stop.problem
  }
  const needs = needsReasonOf(conversation, now, sessionTitleOf, stopped)
  if (needs?.kind === 'conflict') text.append(conflictLine(conversation, needs.text))
  else if (needs && needs.kind !== 'permission') text.append(element('div', 'needs-reason is-' + needs.kind, needs.text))
  if (approval) text.append(approvalLine(approval))
  else if (reply) text.append(element('div', 'reply', reply))
  const reason = approvalReasonOf(conversation)
  if (reason) text.append(element('div', 'approval-reason', reason))
  const held = pendingOf(conversation)
  if (held) text.append(heldRequest(held, key))
  // The meta line is the same in every group: tool, place, folder and age.
  const meta = element('div', 'meta')
  if (renaming?.problem) {
    meta.classList.add('is-problem')
    meta.dataset.tooltip = renaming.problem
    meta.append(renaming.problem)
  } else {
    const claimed = claimChipOf(conversation)
    if (claimed) {
      meta.classList.add('has-chip')
      meta.append(element('span', 'meta-text', detailsOf(conversation, now, folderNames)), claimChip(conversation, claimed))
    } else {
      meta.append(detailsOf(conversation, now, folderNames))
    }
    if (conversation.cloud) meta.dataset.tooltip = SHARED_TITLE
  }
  text.append(meta)
  const subagents = subagentsLine(conversation, key)
  if (subagents) text.append(subagents)
  const shells = stuckShellsOf(conversation)
  const labels = shellLabelsOf(shells, now)
  const lead = shellsLeadOf(conversation, now, stopped)
  if (lead) text.append(element('div', 'shells-lead', lead))
  shells.forEach((shell, index) => text.append(shellLine(conversation, shell, labels[index], now, shells.length > 1)))
  const listed = new Set(shells.map(shellKeyOf))
  for (const [shell, stop] of state.shellStops) {
    if (stop.key === key && stop.text && !listed.has(shell)) text.append(shellStoppedLine(stop))
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

  // The buttons sit on the title line, where the name field would be under them.
  if (actions.childElementCount > 0 && !renaming) text.append(actions)
  row.append(stateMark(conversation), text)
  row.addEventListener('click', () => chooseRow(conversation))
  row.addEventListener('mousemove', () => {
    if (!document.activeElement.matches('#list .row')) setActive(key)
  })
  row.addEventListener('focusin', () => setActive(key))
  row.addEventListener('contextmenu', (event) => {
    // Right-clicking only offers the menu: the chat stays closed and unread.
    event.preventDefault()
    if (mailbox.composing) return
    setActive(key)
    openRowMenu(conversation, event)
  })
  return row
}

function shellKeyOf(shell) {
  return shell.pid + ':' + shell.procStart
}

function shellButton(label, accessibleName, press) {
  const button = element('button', 'shell-action', label)
  button.type = 'button'
  button.setAttribute('aria-label', accessibleName)
  button.addEventListener('click', (event) => {
    event.stopPropagation()
    press()
  })
  return button
}

/**
 * A stuck shell's line: its run time and its command cut in the middle, with Keep running and Stop; the grey line of a
 * kept shell with Stop; or the stop on its way. The buttons wrap under the words in a row of several shells, or when
 * the panel is narrow. The hover shows the whole command, the start time, the PID and the processes.
 */
function shellLine(conversation, shell, label, now, stacked) {
  const line = element('div', 'shell-warning')
  const stop = state.shellStops.get(shellKeyOf(shell))
  if (stop?.text) return shellStoppedLine(stop)
  if (stop?.stopping) {
    line.classList.add('is-stopping')
    line.append(element('span', 'shell-text', 'Stopping the shell…'))
    return line
  }
  const command = shellCommandOf(shell)
  const words = element('span', 'shell-text')
  words.dataset.tooltip = label.full
  // The spaces are for screen readers and copied text; the gap spaces the parts on screen.
  if (shell.keptUntil) words.append(element('span', 'shell-note', keptShellNoteOf(shell, now) + ':'), ' ')
  else {
    if (label.detail) {
      // The name gives way before the command, whose end tells two shells apart.
      const owner = element('span', 'shell-owner')
      owner.append(element('span', 'shell-owner-name', label.detail), ' ·')
      words.append(owner, ' ')
    }
    words.append(element('span', 'shell-age', label.age), ' ')
  }
  words.append(element('code', 'shell-command', label.command))
  line.append(words)
  line.classList.toggle('is-kept', Boolean(shell.keptUntil))
  line.classList.toggle('is-stacked', stacked)
  const buttons = element('span', 'shell-actions')
  if (!shell.keptUntil) buttons.append(shellButton('Keep running', 'Keep running: ' + command, () => keepRunning(conversation, shell)))
  buttons.append(shellButton('Stop', 'Stop shell: ' + command, () => openStopShell(conversation, shell)))
  line.append(buttons)
  return line
}

function shellStoppedLine(stop) {
  const line = element('div', 'shell-warning is-stopped')
  line.classList.toggle('is-problem', stop.problem)
  const words = element('span', 'shell-text', stop.text)
  words.dataset.tooltip = stop.text
  line.append(words)
  return line
}

async function keepRunning(conversation, shell) {
  try {
    ({ shells: state.keptShells } = await keepShell(keepOf(conversation, shell, state.keptShells, Date.now())))
  } catch (error) {
    sayFailure(error)
    return
  }
  redrawKeepingKeys()
}

/** A redraw that removed the focused button gives the keys back to the search, as a menu item does. */
function redrawKeepingKeys() {
  draw({ force: true })
  if (!document.activeElement || document.activeElement === document.body) queryInput.focus()
}

async function forgetKept(records) {
  if (records.length === 0) return
  try {
    ({ shells: state.keptShells } = await forgetShells(records))
  } catch (error) {
    sayFailure(error)
    return
  }
  redrawKeepingKeys()
}

/** The reply line's stand-in while a tool call waits for approval: a key and "Approve edit dashboard.tsx", or a lock and "Needs permission". */
function approvalLine(approval) {
  const line = element('div', 'reply is-approval')
  const pill = element('span', 'approval is-' + approval.icon)
  // A long detail is cut on screen; the whole pill reads on hover.
  pill.dataset.tooltip = approval.text
  pill.append(icon(approval.icon, '0 0 16 16'), element('span', 'approval-label', approval.label))
  // The space is for screen readers and copied text; the pill's gap spaces the words on screen.
  if (approval.code) pill.append(' ', element('code', 'approval-code', approval.code))
  line.append(pill)
  return line
}

/** Ids of the held requests answered from this panel, whose buttons stay off until the list after the answer is read. */
const answering = new Set()

/**
 * How long the buttons stay off on the row's next request after an answer, so the second click of a double click
 * cannot answer a request the person has not read.
 */
const SETTLE_MS = 800

/** Per row key, when the buttons of the next request come on. */
const settling = new Map()

/**
 * A request the approvals hook holds for the companion: what it runs or writes, then Approve and Deny. The block is the
 * confirmation, so a click answers at once; no key does.
 */
function heldRequest(held, key) {
  const box = element('div', 'held-request')
  for (const block of held.blocks) {
    box.append(element('div', 'held-label', block.label), element('pre', 'held-text', block.text))
  }
  const line = element('div', 'held-actions')
  const off = answering.has(held.id) || (settling.get(key) ?? 0) > Date.now()
  const actions = [['Approve', 'approve'], ['Deny', 'deny']]
  if (held.inTerminal) actions.push(['Answer in terminal', 'release'])
  for (const [label, decision] of actions) {
    const button = element('button', 'held-action is-' + decision, label)
    button.type = 'button'
    button.disabled = off
    button.addEventListener('click', (event) => {
      event.stopPropagation()
      answerHeld(held, key, decision)
    })
    line.append(button)
  }
  if (held.more > 0) line.append(element('span', 'held-more', `+${held.more} more`))
  box.append(line)
  // A click in the block must not open the chat: the text is there to read and select.
  box.addEventListener('click', (event) => event.stopPropagation())
  return box
}

async function answerHeld(held, key, decision) {
  answering.add(held.id)
  // Any list read before the end draws the row's next request; its buttons must already be off.
  settling.set(key, Infinity)
  redrawKeepingKeys()
  let version
  try {
    ({ version } = await decideApproval(held, decision))
  } catch (error) {
    if (error instanceof CallError && (error.status === 404 || error.status === 409)) say(PENDING_CHANGED, 'error')
    else sayFailure(error)
  }
  await afterChange(version)
  settling.set(key, Date.now() + SETTLE_MS)
  answering.delete(held.id)
  redrawKeepingKeys()
  setTimeout(() => {
    settling.delete(key)
    redrawKeepingKeys()
  }, SETTLE_MS)
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
  document.querySelector(selector)?.focus({ preventScroll: true })
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
  for (const node of rows) box.append(helperNode(conversation, key, node, now))
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

/** A subagent's row and, under a toggle that folds them, the subagents it started, one step further in. */
function helperNode(conversation, key, node, now) {
  const item = element('div', 'helper-node')
  item.append(helperRow(conversation, node.helper, now))
  if (node.children.length === 0) return item
  const nodeKey = key + '|' + node.helper.id
  const open = !state.helpersShut.has(nodeKey)
  const toggle = element('button', 'helpers-toggle is-nested', subtreeToggleOf(node, open))
  toggle.type = 'button'
  toggle.setAttribute('aria-expanded', String(open))
  toggle.setAttribute('aria-controls', helpersId(nodeKey))
  toggle.addEventListener('click', (event) => {
    event.stopPropagation()
    const focused = document.activeElement === toggle
    if (open) state.helpersShut.add(nodeKey)
    else state.helpersShut.delete(nodeKey)
    draw({ force: true })
    if (focused) refocus(`.helpers-toggle[aria-controls="${helpersId(nodeKey)}"]`)
  })
  item.append(toggle)
  if (open) {
    const nested = element('div', 'helpers is-nested')
    nested.id = helpersId(nodeKey)
    for (const child of node.children) nested.append(helperNode(conversation, key, child, now))
    item.append(nested)
  }
  return item
}

function helperRow(conversation, helper, now) {
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
  return row
}

const RENAME_TIP = 'Click to rename, or press F2'

/** The row's name: a click on it starts the rename, except while the row is being picked as a target. */
function renamableTitle(conversation, rowTip) {
  const title = element('div', 'title', titleOf(conversation))
  title.dataset.tooltip = rowTip + '\n' + RENAME_TIP
  title.addEventListener('click', (event) => {
    if (state.pick || mailbox.composing) return
    event.stopPropagation()
    startRename(conversation)
  })
  return title
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
    if (event.repeat && (event.key === 'Enter' || event.key === ' ')) {
      event.preventDefault()
      return
    }
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
    all.title = 'Mark each chat shown here as read without opening it. A chat that waits on something else, such as a permission or a path conflict, stays here.'
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
  const place = state.pick ? 'all' : state.place
  if (state.query === '' && place !== 'all') {
    empty.append(element('strong', '', `No ${place} conversations`), 'Choose All to see every conversation.')
  } else if (state.query === '') {
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

/** The active approvals mode as a chip in the header, with its Ask me; the full sentence is its tooltip and name. */
function drawApprovals(view) {
  const header = approvalsHeaderOf(view)
  approvalsBar.hidden = header === undefined
  if (!header) return
  approvalsMode.textContent = header.label
  approvalsBar.title = header.title
  approvalsBar.setAttribute('aria-label', header.title)
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
  const focusedRow = document.activeElement.matches('#list .row') ? document.activeElement : null
  if (state.searching) {
    // The rows on screen answer another question, so they give way until this one's answer is here.
    state.drawn = undefined
    summary.textContent = 'Searching…'
    const waiting = element('div', 'empty is-searching')
    waiting.append(element('strong', '', 'Searching…'), 'Looking through every conversation.')
    list.replaceChildren(waiting)
    if (focusedRow) queryInput.focus()
    announceActive()
    return
  }
  if (!view) {
    // No list could be read. An old one would look current, so none shows.
    state.drawn = undefined
    summary.textContent = ''
    list.replaceChildren()
    if (!said) list.append(element('p', 'loading-conversations', 'Loading conversations…'))
    if (focusedRow) queryInput.focus()
    announceActive()
    return
  }
  // The quiet notes go in too: a silent chat's "No activity for 7m" tooltip changes with no new row data.
  const now = Date.now()
  const reasons = view.conversations.map((conversation) => quietNoteOf(conversation, now))
  const shellAges = view.conversations.flatMap((conversation) => {
    const shells = stuckShellsOf(conversation)
    return shellLabelsOf(shells, now).map((label, index) => [label.full, shells[index].keptUntil && keptShellNoteOf(shells[index], now)])
  })
  const stillSearching = state.query !== '' && state.readingOlder === state.query
  const signature = JSON.stringify([view.conversations, reasons, shellAges, [...state.shellStops], view.needsYou, view.matches, said, view.cloudNote, missingToolsNoteOf(state.view ?? view), state.query, state.activeKey, state.place, state.pick?.hash, stillSearching])
  if (!force && signature === state.drawn) return
  state.drawn = signature

  // The count covers weeks not read yet, so it waits for the last page.
  summary.textContent = stillSearching ? 'Searching…' : summaryOf(view, state.query)

  const groups = grouped(view.conversations)
  const scroll = list.scrollTop
  list.replaceChildren()
  if (groups.length === 0) {
    list.append(emptyMessage(), ...asidesOf(view))
    if (focusedRow) queryInput.focus()
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
  if (focusedRow) (document.getElementById(focusedRow.id) ?? queryInput).focus({ preventScroll: true })
  mailbox.markRows()
  holdWorkPhase(list)
  announceActive()
  tipAfterRedraw()
}

/*
 * One tooltip the page draws for the rows, the shell lines and the approval pills, in place of the browser's own:
 * the system draws a title under the pointer, over the next row of a tall one, and keeps it up while a redraw moves
 * another row under a still pointer. It opens after a pause, or on a row the keys select; it sits above the row, or
 * below when there is no room above, never over the row; a press, a scroll, or a redraw that takes its row away closes it.
 */
const TOOLTIP_MS = 600
const TOOLTIP_GAP = 4
const tip = { timer: undefined, target: undefined, pointer: undefined }

/** What a hover shows: the nearest element with a tooltip, unless a button, a field or a native title is nearer; those keep their own. */
function tipTargetOf(element) {
  let hit = element?.closest?.('button, input, select, textarea, [title], [data-tooltip]')
  // While a click on the name picks the row, the name says what the row says.
  if (hit?.matches('.title[data-tooltip]') && (state.pick || mailbox.composing)) hit = hit.closest('.row')
  return hit && hit.hasAttribute('data-tooltip') && list.contains(hit) ? hit : null
}

function placeTip(target) {
  const row = target.closest('.row') ?? target
  tooltip.textContent = target.dataset.tooltip
  tooltip.style.maxHeight = ''
  tooltip.hidden = false
  const box = row.getBoundingClientRect()
  const above = box.top - TOOLTIP_GAP * 2
  const below = window.innerHeight - box.bottom - TOOLTIP_GAP * 2
  const onTop = tooltip.offsetHeight <= above || above >= below
  const room = Math.max(0, onTop ? above : below)
  if (tooltip.offsetHeight > room) tooltip.style.maxHeight = `${room}px`
  const height = tooltip.offsetHeight
  tooltip.style.top = `${onTop ? box.top - TOOLTIP_GAP - height : box.bottom + TOOLTIP_GAP}px`
  const x = tip.pointer?.x ?? box.left + 24
  tooltip.style.left = `${Math.max(8, Math.min(x - 16, window.innerWidth - tooltip.offsetWidth - 8))}px`
  tip.target = target
}

function hideTip() {
  clearTimeout(tip.timer)
  tip.timer = undefined
  tip.target = undefined
  tooltip.hidden = true
}

function tipSoon(target) {
  if (target === tip.target && (tip.timer !== undefined || !tooltip.hidden)) return
  clearTimeout(tip.timer)
  tip.timer = undefined
  if (!target) {
    hideTip()
    return
  }
  // Moving from one tooltip to the next shows the next at once, as the system's do.
  if (!tooltip.hidden) {
    placeTip(target)
    return
  }
  tip.target = target
  tip.timer = setTimeout(() => {
    tip.timer = undefined
    if (target.isConnected) placeTip(target)
  }, TOOLTIP_MS)
}

/** After a redraw or a scroll, what is under a still pointer changed: the tooltip follows it, or closes. */
function tipAfterRedraw() {
  if (tip.target === undefined) return
  const waiting = tip.timer !== undefined
  const selectedRow = state.activeKey === undefined ? null : document.getElementById(rowId(state.activeKey))
  const under = tip.pointer ? tipTargetOf(document.elementFromPoint(tip.pointer.x, tip.pointer.y)) : selectedRow
  hideTip()
  if (!under) return
  if (waiting) tipSoon(under)
  else placeTip(under)
}

list.addEventListener('pointermove', (event) => {
  tip.pointer = { x: event.clientX, y: event.clientY }
  tipSoon(tipTargetOf(event.target))
})
list.addEventListener('pointerleave', () => {
  tip.pointer = undefined
  hideTip()
})
list.addEventListener('pointerdown', hideTip)
// The person scrolling closes it; a scroll the page makes, such as the browser keeping rows in place, moves it along.
list.addEventListener('wheel', hideTip, { passive: true })
list.addEventListener('scroll', tipAfterRedraw, { passive: true })
document.addEventListener('keydown', hideTip, true)
window.addEventListener('blur', hideTip)

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

function moveActive(step, focusRow) {
  const key = neighbour(visibleKeys(), state.activeKey, step)
  if (key === undefined) return
  setActive(key)
  const row = list.querySelector('.row.is-active')
  if (row) {
    if (focusRow) row.focus({ preventScroll: true })
    row.scrollIntoView({ block: 'nearest' })
    tip.pointer = undefined
    tipSoon(row)
  }
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
  // A stop's line stays for the list it made, or the first list after it when the answer named none.
  for (const [shell, stop] of state.shellStops) {
    if (stop.stopping) continue
    if (stop.version === undefined) stop.version = reply?.version ?? null
    else if (stop.version !== reply?.version) state.shellStops.delete(shell)
  }
  // Only a full list tells a shell is gone: a search leaves chats out.
  if (state.view && !state.query) forgetKept(finishedKeepsOf(state.view, state.keptShells))
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

/** The latest late open can show a failure or a hint until dismissed. */
async function showLateOpenNotice() {
  if (opensOnTheirWay > 0) return
  const thisOpen = latestOpen
  let reply
  try {
    reply = await readLateOpenNotice()
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    return
  }
  const shown = reply && lateOpenNoticeOf(reply, lastDismissedLate)
  if (!shown || !lateNoticeReplaces(state.openFailure) || thisOpen !== latestOpen || opensOnTheirWay > 0) return
  showOpenFailure(shown, shown.about ?? NOTICES.notOpened)
  if (shown.about === NOTICES.subagent && !listShown) await peekOpenNotice()
}

/** The last dismissed late notice may still come back on a read already on its way. */
let lastDismissedLate

// The panel never activates its app, so while it shows it keeps the keys
// even after the terminal comes to the front. It hides first, and comes
// back with directions to a subagent or the reason it did not open.
// A guessed tab's notice waits for the next opening of the list.
let latestOpen = 0
/** Opens sent and not yet answered: the companion drops an older late notice as each one arrives. */
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
    note = reply.pending ? undefined : openNoticeOf(reply)
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    failure = { message: error.message }
  } finally {
    opensOnTheirWay -= 1
  }
  // A late notice may have landed while this answer was on its way, when the panel skipped it.
  if (pending) showLateOpenNotice()
  // An open made meanwhile, such as one waiting on a macOS prompt, has its own answer.
  if (thisOpen !== latestOpen) return
  if (note) showOpenFailure(note, note.about)
  if (!failure) {
    if (note?.about === NOTICES.subagent) await peekOpenNotice()
    return
  }
  showOpenFailure(failure, NOTICES.notOpened)
  state.returning = true
  await showPanel().catch((error) => {
    state.returning = false
    sayFailure(error)
  })
}

async function peekOpenNotice() {
  // The panel-shown event can arrive before the peek's reply and takes returning back.
  state.returning = true
  const peeked = await peekPanel().catch(() => ({ shown: false }))
  if (!peeked?.shown) state.returning = false
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
    noticeCopy.textContent = about === NOTICES.subagent ? 'Copy attach command' : 'Copy resume command'
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
  tabToConversation = true
  queryInput.focus()
  draw({ force: true })
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
  // A late notice stays with the companion until dismissed; another notice may have covered it.
  if (dismissed?.late) {
    lastDismissedLate = dismissed.id
    dismissLateOpenNotice(dismissed.id).catch(sayFailure)
  }
  else showLateOpenNotice()
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
  'copy-cloud-resume': document.getElementById('copy-cloud-resume'),
  'send-to': document.getElementById('send-to'),
  'claim-files': document.getElementById('claim-files'),
  complete: document.getElementById('complete'),
  reopen: document.getElementById('reopen'),
  hide: document.getElementById('hide'),
  delete: document.getElementById('delete'),
  web: document.getElementById('open-web'),
  'stop-shell': document.getElementById('stop-shell'),
  'ask-shell-again': document.getElementById('ask-shell-again'),
  'stop-session': document.getElementById('stop-session'),
}
/** The conversation the row menu was opened on. */
let rowMenuFor

function openRowMenu(conversation, event) {
  rowMenuFor = conversation
  const offered = rowMenuOf(conversation)
  for (const [name, item] of Object.entries(menuItems)) item.hidden = !offered.includes(name)
  let hasGroup = false
  for (const group of rowMenu.querySelectorAll('[role="group"]')) {
    group.hidden = !group.querySelector('.menu-item:not([hidden])')
    const separator = group.previousElementSibling
    if (separator) separator.hidden = group.hidden || !hasGroup
    hasGroup ||= !group.hidden
  }
  rowMenu.hidden = false
  // The menu opens at the pointer and stays inside the card.
  const bounds = card.getBoundingClientRect()
  const left = Math.min(event.clientX - bounds.left, bounds.width - rowMenu.offsetWidth - 8)
  const top = Math.min(event.clientY - bounds.top, bounds.height - rowMenu.offsetHeight - 8)
  rowMenu.style.left = Math.max(left, 8) + 'px'
  rowMenu.style.top = Math.max(top, 8) + 'px'
  rowMenu.querySelector('.menu-item:not([hidden])')?.focus()
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
  if (!renames(conversation)) return
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
function bindRowCopy(item, field, copied, failed) {
  menuItems[item].addEventListener('click', async () => {
    const conversation = rowMenuTarget(item)
    closeRowMenu()
    queryInput.focus()
    if (!conversation) return
    try {
      await navigator.clipboard.writeText(conversation[field])
      say(copied)
    } catch {
      say(failed, 'error')
    }
  })
}
bindRowCopy('copy-session-id', 'id', 'Session ID copied.', 'Could not copy the session ID.')
bindRowCopy('copy-cloud-resume', 'cloudResumeCommand', 'Resume command copied. Paste it in a terminal.', 'Could not copy the resume command.')
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

const stopShellWorking = document.getElementById('stop-shell-working')

menuItems['stop-shell'].addEventListener('click', () => {
  const conversation = rowMenuTarget('stop-shell')
  closeRowMenu()
  queryInput.focus()
  if (conversation) openStopShell(conversation)
})

menuItems['ask-shell-again'].addEventListener('click', () => {
  const conversation = rowMenuTarget('ask-shell-again')
  closeRowMenu()
  queryInput.focus()
  if (conversation) forgetKept(keptShellsOfRow(conversation, state.keptShells))
})

/** The stop confirm, on the shell a line's Stop chose, or with a picker when the row has several and none was chosen. */
function openStopShell(conversation, chosen) {
  if (stoppingShell) return
  const shells = stuckShellsOf(conversation)
  const now = Date.now()
  stopShellFor = { conversation, shells }
  stopShellChoice.replaceChildren(new Option('Choose a shell', ''))
  shells.forEach((shell, index) => {
    stopShellChoice.add(new Option(shellWarningOf(shell, now) + ' (PID ' + shell.pid + ')', String(index)))
  })
  const selected = chosen ?? (shells.length === 1 ? shells[0] : undefined)
  stopShellChoice.hidden = selected !== undefined
  const working = stopShellWarningOf(conversation)
  stopShellWorking.hidden = !working
  stopShellWorking.textContent = working ?? ''
  showShellChoice(selected)
  stopShellDialog.hidden = false
  stopShellCancel.focus()
}

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
  const shellKey = shellKeyOf(selected)
  const key = keyOf(conversation)
  state.shellStops.set(shellKey, { key, stopping: true })
  draw({ force: true })
  let result
  try {
    result = await stopShell(conversation, selected)
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    showOpenFailure({ message: error.message }, NOTICES.shellNotStopped)
  } finally {
    stoppingShell = false
    stopShellChoice.disabled = false
    state.shellStops.delete(shellKey)
    if (stopShellFor) closeStopShell()
  }
  if (result) {
    const message = stoppedShellOf(result.body)
    const problem = Boolean(result.body.stillRunning?.length)
    state.shellStops.set(shellKey, { key, text: message, problem, version: result.version })
    // A stopped shell is done with, even where the next list cannot say it is gone.
    forgetKept(state.keptShells.filter((record) => shellKeyOf(record) === shellKey))
    if (problem) showOpenFailure({ message }, NOTICES.shellProcessesLeft)
    else say(message)
  }
  draw({ force: true })
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
  tabToConversation = true
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
queryInput.addEventListener('pointerdown', () => { tabToConversation = true })
clearButton.addEventListener('click', clearSearch)
// Settings

const menu = document.getElementById('menu')
const menuButton = document.getElementById('menu-button')
const menuProblem = document.getElementById('menu-problem')
const isMac = System.IsMac()

const settings = {
  /** menu.json, read on the first open. */
  items: undefined,
  metadata: undefined,
  sections: new Map(),
  updates: undefined,
  /** Each row's control by its id in menu.json. */
  controls: new Map(),
  /** Each row's placed element by its id, to hide a row whose value could not be read. */
  rows: new Map(),
  /** What the shell last said: {shortcut, shortcutProblem?, startAtLogin, startAtLoginProblem?, checkForUpdates}. */
  preferences: undefined,
  /** How the pages look: {animate, dimWhenIdle, smallCompanion, presence}. */
  appearance: undefined,
  /** The server's mailbox settings: {approveAgentSends}. */
  mailbox: undefined,
  /** The server's settings document, which the approvals rows read; undefined when it could not be read. */
  approvals: undefined,
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

const approvalsRows = approvalsMenu({
  changed: (doc) => {
    settings.approvals = doc
    if (!menu.hidden && settings.items) drawSettings()
  },
  listChanged: afterChange,
  row: (id) => (id === 'approvalsMode' ? menu.querySelector('.choice[aria-checked="true"]') : settings.controls.get(id)),
})

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
  approvalsMode: (event) => approvalsRows.choose(event.target.closest('[data-value]')?.dataset.value, () => menu.querySelector('.choice[aria-checked="true"]')),
  approvalsCompanion: () => approvalsRows.save('approvals.companion', flip('approvalsCompanion'), !flip('approvalsCompanion')),
  checkForUpdates: () => change('preferences', () => saveCheckForUpdates(flip('checkForUpdates'))),
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
function buildMenu(metadata) {
  for (const section of metadata.sections) {
    const group = element('div', 'menu-section')
    const heading = element('h2', 'menu-heading', section.label)
    heading.id = `menu-section-${section.id}`
    group.setAttribute('role', 'group')
    group.setAttribute('aria-labelledby', heading.id)
    group.append(heading)
    settings.sections.set(section.id, group)
    menu.insertBefore(group, menuProblem)
  }
  for (const row of metadata.items) {
    const act = MENU_ACTIONS[row.id]
    if (!act) throw new Error(`the menu item ${row.id} has no action here`)
    let control
    let placed
    if (row.kind === 'action') {
      control = element('button', 'menu-item', row.label)
      placed = control
    } else if (row.kind === 'choice') {
      const labelId = `menu-${row.id}-label`
      const label = element('span', 'menu-label', row.label)
      label.id = labelId
      control = element('div', 'choices')
      control.setAttribute('role', 'radiogroup')
      control.setAttribute('aria-labelledby', labelId)
      for (const choice of row.choices) {
        const option = element('button', 'choice', choice.label)
        option.type = 'button'
        option.setAttribute('role', 'radio')
        option.dataset.value = choice.value
        control.append(option)
      }
      control.addEventListener('keydown', moveChoice)
      placed = element('div', 'menu-row menu-choice')
      placed.append(label, control)
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
    if (row.kind !== 'choice') control.type = 'button'
    if (row.id === 'approvalsCompanion') {
      const hint = element('p', 'menu-hint', row.hint)
      hint.id = 'menu-approvals-companion-hint'
      const status = element('span', 'menu-setup-state', 'Setup required')
      status.id = 'menu-approvals-companion-state'
      status.hidden = true
      placed.classList.add('menu-row-with-hint')
      placed.append(status, hint)
      control.setAttribute('aria-describedby', hint.id)
    } else if (row.hint) control.title = row.hint
    control.addEventListener('click', act)
    settings.controls.set(row.id, control)
    settings.rows.set(row.id, placed)
    const section = settings.sections.get(row.section)
    if (row.id === 'update' || row.id === 'checkForUpdates') {
      if (!settings.updates) {
        const box = element('div', 'menu-updates')
        const toggle = element('button', 'menu-item menu-updates-toggle', metadata.updatesLabel)
        toggle.type = 'button'
        toggle.setAttribute('aria-expanded', 'false')
        const body = element('div', 'menu-updates-body')
        body.id = 'menu-updates-body'
        body.hidden = true
        toggle.setAttribute('aria-controls', body.id)
        toggle.append(icon('chevron', '0 0 8 8'))
        toggle.addEventListener('click', () => {
          body.hidden = !body.hidden
          toggle.setAttribute('aria-expanded', String(!body.hidden))
        })
        box.append(toggle, body)
        section.append(menuProblem, box)
        settings.updates = { box, toggle, body }
      }
      settings.updates.body.append(placed)
    } else {
      section.append(placed)
    }
    if (row.id === 'approvalsCompanion') section.append(approvalsRows.box)
  }
}

function drawSettings() {
  const rows = menuRowsOf(settings.items, settings, isMac)
  const visible = new Set(rows.filter((row) => !row.unavailable).map((row) => row.id))
  for (const [id, placed] of settings.rows) placed.hidden = !visible.has(id)
  for (const row of rows) {
    const control = settings.controls.get(row.id)
    if (!control) continue
    if (row.unavailable) continue
    if (row.kind === 'action') control.textContent = row.label
    control.disabled = Boolean(row.disabled)
    if (row.kind === 'switch') control.setAttribute('aria-checked', String(row.value))
    if (row.id === 'approvalsCompanion') {
      const pending = companionApprovalsNeedSetup(settings.approvals)
      const status = document.getElementById('menu-approvals-companion-state')
      status.hidden = !pending
      const hint = 'menu-approvals-companion-hint'
      control.setAttribute('aria-describedby', pending ? `${status.id} ${hint}` : hint)
    }
    if (row.kind === 'choice') {
      for (const option of control.children) {
        const chosen = option.dataset.value === row.value
        option.setAttribute('aria-checked', String(chosen))
        // One Tab stop for the group, on the chosen radio; the arrows move between them.
        option.tabIndex = chosen ? 0 : -1
      }
    }
    if (row.kind !== 'shortcut') continue
    const recording = settings.recording === row.id
    control.classList.toggle('is-recording', recording)
    if (recording) {
      control.replaceChildren('Press the new keys')
    } else {
      control.replaceChildren(...keycapsOf(row.value, isMac).map((keycap) => element('kbd', '', keycap)))
    }
  }
  for (const [id, section] of settings.sections) {
    section.hidden = !rows.some((row) => row.section === id && !row.unavailable)
  }
  if (settings.updates) settings.updates.box.hidden = !rows.some((row) => row.id === 'update')
  const preferences = settings.preferences
  const problems = settings.refusal !== undefined ? [settings.refusal] : [preferences.shortcutProblem, preferences.sendShortcutProblem, preferences.startAtLoginProblem].filter(Boolean)
  menuProblem.hidden = problems.length === 0
  menuProblem.textContent = problems.join('. ')
}

/** The arrow keys move the focus between a choice row's radios, as in any radio group; Space or Enter picks one. */
function moveChoice(event) {
  const step = { ArrowDown: 1, ArrowRight: 1, ArrowUp: -1, ArrowLeft: -1 }[event.key]
  if (!step) return
  const options = [...event.currentTarget.children]
  const next = options[(options.indexOf(document.activeElement) + step + options.length) % options.length]
  event.preventDefault()
  event.stopPropagation()
  for (const option of options) option.tabIndex = option === next ? 0 : -1
  next.focus()
}

function closeMenu() {
  approvalsRows.closed()
  settings.recording = undefined
  menu.hidden = true
  menuButton.setAttribute('aria-expanded', 'false')
}

/** Opens the menu with the values in effect; `focus` names the row to focus. */
async function openMenu(focus) {
  try {
    const showRead = updates.since()
    const [items, preferences, appearance, update, mailbox] = await Promise.all([settings.metadata ?? readMenu(), readPreferences(), readAppearance(), readUpdate(), readMailboxSettings().catch(unreadSetting), approvalsRows.read()])
    if (!settings.items) {
      settings.metadata = items
      settings.items = items.items
      buildMenu(items)
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
  approvalsRows.opened()
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

const updateGo = document.getElementById('update-go')
const updateCommand = document.getElementById('update-command')
const updateStatusCommand = document.getElementById('update-status-command')
const updateCopy = document.getElementById('update-copy')
const updateRelease = document.getElementById('update-release')
const updateStatusRelease = document.getElementById('update-status-release')

let confirmedUpdate

function openUpdateConfirm() {
  confirmedUpdate = settings.update
  const question = updateConfirmOf(confirmedUpdate)
  document.getElementById('update-title').textContent = question.title
  document.getElementById('update-body').textContent = question.body
  updateCommand.hidden = !question.command
  updateCommand.textContent = question.command ?? ''
  updateGo.textContent = question.go
  updateRelease.hidden = !question.releasePage
  updateConfirm.hidden = false
  updateCancel.focus()
}

/** Copies command for a terminal and says whether it worked. */
async function copyUpdateCommand(command) {
  try {
    await navigator.clipboard.writeText(command)
  } catch {
    say(`Could not copy it. Run ${command} in a terminal.`, 'error')
    return
  }
  say(`Copied ${command}. Paste it in a terminal.`)
}

function closeUpdateConfirm() {
  confirmedUpdate = undefined
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
  updateStatusCommand.hidden = updateCopy.hidden = !message.command
  updateStatusCommand.textContent = message.command ?? ''
  updateStatusRelease.hidden = !message.releasePage
}

updateCancel.addEventListener('click', closeUpdateConfirm)
updateGo.addEventListener('click', () => {
  const offered = confirmedUpdate
  if (!offered) return
  const { command } = updateConfirmOf(offered)
  closeUpdateConfirm()
  if (command) copyUpdateCommand(command)
  else startUpdate(offered.version).catch(sayFailure)
})
updateCopy.addEventListener('click', () => copyUpdateCommand(updateMessageOf(settings.update).command))
updateRetry.addEventListener('click', () => startUpdate(settings.update.version).catch(sayFailure))
updateRelease.addEventListener('click', () => {
  closeUpdateConfirm()
  openReleasePage().catch(sayFailure)
})
updateStatusRelease.addEventListener('click', () => openReleasePage().catch(sayFailure))
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
  if (tabToConversation && event.key === 'Tab' && event.target === queryInput && !event.shiftKey && !event.altKey && !event.metaKey && !event.ctrlKey) {
    const row = list.querySelector('.row.is-active') ?? list.querySelector('.row')
    if (!row) return
    event.preventDefault()
    tabToConversation = false
    row.focus()
    return
  }
  const step = KEY_STEPS[event.key]
  if (step !== undefined) {
    event.preventDefault()
    if (event.target === queryInput) tabToConversation = true
    moveActive(step, event.target.matches('.row'))
    return
  }
  if (!mailbox.composing && startsRename(event)) {
    // F2 on a row's own button renames that row; a button outside the rows keeps the key.
    const inRow = event.target.closest('.row')
    if (event.target.closest('button') && !inRow) return
    const conversation = inRow ? shownView()?.conversations.find((shown) => keyOf(shown) === inRow.dataset.key) : activeConversation()
    if (!conversation) return
    event.preventDefault()
    startRename(conversation)
    return
  }
  // A focused button handles its own activation keys.
  if (event.target.closest('button')) return
  const choosesWithSpace = event.key === ' ' && event.target.matches('.row') && (state.pick || mailbox.composing)
  if ((event.key === 'Enter' || choosesWithSpace) && !event.repeat && !confirmsCandidate(event)) {
    if (state.pick && state.searching) return
    const conversation = activeConversation()
    if (conversation) {
      event.preventDefault()
      chooseRow(conversation)
    }
  }
})

/** The panel opened: it grows out of the corner nearest the bubble, and the search takes the keys. */
function opened(data) {
  // A click on the circle while a row waits sends {from, jump}; every other showing sends the corner alone.
  const { from, jump } = typeof data === 'object' && data !== null ? data : { from: data, jump: false }
  listShown = true
  hideTip()
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
  if (jump) {
    // The waiting row may sit outside the search typed before: the circle asks for every row.
    queryInput.value = ''
    clearButton.hidden = true
    state.query = ''
    state.searching = false
  }
  tabToConversation = true
  queryInput.focus()
  queryInput.select()
  draw({ force: true })
  return refresh().then(() => {
    if (jump) jumpToWaiting()
    return showLateOpenNotice()
  })
}

/**
 * Selects the first row under Needs you and brings it into view: the place switches to All when none waits in the
 * chosen place, and a folded Needs you unfolds.
 */
function jumpToWaiting() {
  if (state.query !== '' || state.pick) return
  let key = firstWaitingOf(shownView())
  if (key === undefined) {
    key = firstWaitingOf(keptViewOf(state.view, state.keptShells, Date.now()))
    if (key === undefined) return
    choosePlace('all')
  }
  state.folded.set('needs-you', false)
  state.activeKey = key
  draw({ force: true })
  document.getElementById(rowId(key))?.scrollIntoView({ block: 'nearest' })
}

card.addEventListener('animationend', (event) => {
  if (event.target === card) card.classList.remove('is-opening')
})
// An opening left over would play again when animation is turned back on.
document.addEventListener('motion', () => {
  if (document.documentElement.dataset.motion === 'off') card.classList.remove('is-opening')
})

drawPlaces()
Events.On('kept-shells', (event) => {
  state.keptShells = event.data ?? []
  draw()
})
readKeptShells().then(({ shells }) => {
  state.keptShells = shells
  draw()
}, sayFailure)
// A kept shell asks again at its wake time, with no new list from the server.
setInterval(() => draw(), 30 * 1000)
Events.On('inbox', (event) => {
  // A search costs the server a read of every week it covers; a closed list asks again when it opens.
  if (!listShown && state.query !== '') {
    showLateOpenNotice()
    return
  }
  refresh({ version: event?.data }).then(showLateOpenNotice)
})
Events.On('panel-shown', (event) => opened(event.data))
Events.On('panel-hidden', () => {
  listShown = false
  approvalsRows.closed()
  hideTip()
})
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
