import { CallError, GivenUp, answerHandoff, approveMessage, changeThread, claimPaths, queueMessage, readClaims, readRefs, readThreads, releasePaths, sendMessage, stageMessage } from '/api.js'
import { arrivalOf, claimCountOf, claimStatusOf, conflictLabelOf, handoffAnswerOf, keyOf, mailboxSessionOf, messageActionsOf, messageStateLabelOf, receiversOf, reviewModelsOf, senderKindOf, sessionLabelOf, statedModelOf, threadActionsOf, threadLabelOf, threadsOf, titleOf, typeHelpOf, typedReceiverOf } from '/model.js'

const MESSAGE_ACTIONS = {
  approve: 'Approve',
  reject: 'Reject',
  queue: 'Queue for delivery',
  type: 'Type into session',
  copy: 'Copy message',
}
const ACTIONS = {
  reply: 'Reply',
  pause: 'Pause',
  resume: 'Resume',
  close: 'Close thread',
  'raise-cap': 'Allow 8 more',
  stop: 'Stop',
}
const REVIEW = 'review_request'
const HANDOFF = 'handoff'
const REF_PREVIEW_DELAY = 220
const TYPES = { question: 'Question', finding: 'Finding', done: 'Done', [REVIEW]: 'Review request', [HANDOFF]: 'Handoff' }
const STATES = { open: 'Open', paused: 'Paused', closing: 'Closing', closed: 'Closed' }
const refOf = ({ tool, id }) => ({ tool, id })

function element(tag, className, text) {
  const node = document.createElement(tag)
  if (className) node.className = className
  if (text !== undefined) node.textContent = text
  return node
}

function button(label, action, clicked, className = 'confirm-cancel') {
  const control = element('button', className, label)
  control.type = 'button'
  control.dataset.mailboxControl = action
  control.addEventListener('click', clicked)
  return control
}

function refDetails(refs) {
  const details = element('dl', 'mailbox-ref-details')
  for (const [label, value] of [['Worktree', refs.worktree], ['Commit', refs.commit], ['Base', refs.base]]) {
    const data = element('dd')
    data.append(element('code', '', value))
    details.append(element('dt', '', label), data)
  }
  const count = refs.paths?.length ?? 0
  const changes = element('p', 'mailbox-muted', `${count} changed ${count === 1 ? 'path' : 'paths'}`)
  if (!refs.uncommitted) return [details, changes]
  const dirty = refs.uncommitted
  return [
    details,
    changes,
    element('p', '', `${dirty} uncommitted ${dirty === 1 ? 'file is' : 'files are'} outside this review.`),
  ]
}

function modelComparison(fromName, toName) {
  const { from, to, same } = reviewModelsOf(fromName, toName)
  return `Sending: ${from} · Reviewing: ${to}${same ? '. Both sessions state the same model.' : ''}`
}

function pathsOf(worktree, text) {
  return { worktree, paths: text.split(/\r?\n/).map((line) => line.trim()).filter(Boolean) }
}

function pathDetails({ worktree, paths, except }) {
  const directory = element('p', 'mailbox-muted')
  directory.append(element('code', '', worktree))
  const files = element('ul', 'mailbox-claim-paths')
  for (const path of paths) {
    const item = element('li')
    item.append(element('code', '', path))
    files.append(item)
  }
  if (!except?.length) return [directory, files]
  return [directory, files, element('p', 'mailbox-muted', 'Except ' + except.join(', '))]
}

function threadCount({ cap, messages }) {
  const count = messages.filter(({ type, state }) => type !== 'done' && state !== 'rejected').length
  const waiting = messages.filter(({ state }) => state === 'waiting').length
  const held = messages.filter(({ state }) => state === 'held').length
  return { cap, count, waiting, held }
}

function sentAt(when) {
  const time = new Date(when)
  return Number.isNaN(time.valueOf()) ? undefined : time.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

function refusalText(error) {
  return error.guard && !error.message.includes(`guard: ${error.guard}`) ? `Not sent (guard: ${error.guard}). ${error.message}` : error.message
}

async function copyBlock(prepared) {
  // WebKit requires clipboard.write to start during the click; the server's text may arrive later.
  if (typeof ClipboardItem === 'function' && typeof navigator.clipboard?.write === 'function') {
    const data = prepared.then((text) => new Blob([text], { type: 'text/plain' }))
    const write = async () => navigator.clipboard.write([new ClipboardItem({ 'text/plain': data })])
    await Promise.all([data, write()])
  } else {
    await navigator.clipboard.writeText(await prepared)
  }
}

export function mailboxPanel({ rows, shownRows, focusRows, afterChange, say, hideForTyping, showAfterRefusal }) {
  const byId = (id) => document.getElementById(id)
  const warning = byId('mailbox-warning')
  const composer = byId('mailbox-compose')
  const composerTitle = byId('mailbox-compose-title')
  const fromLabel = byId('mailbox-from')
  const toFixed = byId('mailbox-to-fixed')
  const toName = byId('mailbox-to-name')
  const toList = byId('mailbox-to-list')
  const toSelect = byId('mailbox-to')
  const toHint = byId('mailbox-to-hint-text')
  const enterId = byId('mailbox-enter-id')
  const toId = byId('mailbox-to-id')
  const target = byId('mailbox-target')
  const targetStatus = byId('mailbox-target-status')
  const useList = byId('mailbox-use-list')
  const type = byId('mailbox-type')
  const typeHelp = byId('mailbox-type-help')
  const arrival = byId('mailbox-arrival')
  const body = byId('mailbox-body')
  const bodyField = byId('mailbox-body-field')
  const handoff = byId('mailbox-handoff')
  const handoffWorktree = byId('mailbox-handoff-worktree')
  const handoffStatus = byId('mailbox-handoff-status')
  const handoffPaths = byId('mailbox-handoff-paths')
  const claimOffer = byId('mailbox-claim-offer')
  const review = byId('mailbox-review')
  const base = byId('mailbox-review-base')
  const reviewStatus = byId('mailbox-review-status')
  const resolved = byId('mailbox-review-refs')
  const sendingModel = byId('mailbox-sending-model')
  const reviewingModel = byId('mailbox-reviewing-model')
  const comparison = byId('mailbox-review-comparison')
  const send = byId('mailbox-send')
  const cancel = byId('mailbox-cancel')
  const fields = [toSelect, enterId, target, useList, type, body, base, sendingModel, reviewingModel, handoffWorktree, handoffPaths, claimOffer, cancel]
  const claimsCard = byId('mailbox-claims')
  const claimsOwner = byId('mailbox-claims-owner')
  const claimsList = byId('mailbox-claims-list')
  const claimsDismiss = byId('mailbox-claims-dismiss')
  const claimForm = byId('mailbox-claim-form')
  const claimWorktree = byId('mailbox-claim-worktree')
  const claimPathInput = byId('mailbox-claim-paths')
  const details = byId('mailbox-details')
  const title = byId('mailbox-details-title')
  const meta = byId('mailbox-details-meta')
  const messages = byId('mailbox-messages')
  const result = byId('mailbox-result')
  const actions = byId('mailbox-thread-actions')
  const dismiss = byId('mailbox-dismiss')
  let draft
  let opened
  let claimsFor
  let nativeCopy = false
  let previewTimer

  function nameOf(ref, fallback) {
    const row = rows().find((conversation) => keyOf(conversation) === keyOf(ref))
    return row ? titleOf(row) : fallback || ref.id
  }

  function stopPreview() {
    clearTimeout(previewTimer)
    draft?.reading?.abort()
    if (draft) draft.reading = undefined
  }

  function receiverOptions() {
    return draft ? receiversOf(rows(), draft.from, Date.now()) : []
  }

  function listedReceiver(key) {
    return receiverOptions().flatMap((group) => group.options).find((option) => option.value === key)
  }

  /** A listed receiver, or the chosen one that left the list since. */
  function receiverByKey(key) {
    return listedReceiver(key) ?? (draft?.kept?.value === key ? draft.kept : undefined)
  }

  /** Who the draft goes to: `to` for the request, and the name and group when the list knows the session. */
  function receiverOf(current = draft) {
    if (!current) return {}
    if (current.thread) {
      return { to: current.to, name: nameOf(current.to, current.toTitle), group: listedReceiver(keyOf(current.to))?.group }
    }
    if (current.typing) {
      const typed = current.typed
      if (typed?.status === 'found') return { to: typed.receiver.to, name: typed.receiver.title, group: typed.receiver.group }
      if (typed?.status === 'unknown') return { to: { id: target.value.trim() } }
      return {}
    }
    const chosen = current.receiver
    return chosen ? { to: chosen.to, name: chosen.title, group: chosen.group } : {}
  }

  function fillReceivers() {
    const groups = receiverOptions()
    const options = groups.flatMap((group) => group.options)
    const chosen = draft.receiver && (options.find((option) => option.value === draft.receiver.value) ?? draft.receiver)
    draft.receiver = chosen
    // A chosen session that left the list stays chosen and still sends.
    if (chosen && !options.includes(chosen)) draft.kept = chosen
    const kept = draft.kept && !options.some((option) => option.value === draft.kept.value) ? draft.kept : undefined
    const shape = JSON.stringify([groups.map((group) => [group.label, group.options.map((option) => [option.value, option.label])]), kept?.value])
    // Rebuilding the options while the popup is open could move the pick, so an unchanged list stays as it is.
    if (shape !== toSelect.dataset.shape) {
      toSelect.dataset.shape = shape
      const children = [new Option(groups.length ? 'Choose a session' : 'No other local Claude Code or Codex sessions', '')]
      for (const group of groups) {
        const box = document.createElement('optgroup')
        box.label = group.label
        box.append(...group.options.map((option) => new Option(option.label, option.value)))
        children.push(box)
      }
      if (kept) children.push(new Option(kept.label, kept.value))
      toSelect.replaceChildren(...children)
    }
    toSelect.value = chosen ? chosen.value : ''
    const empty = groups.length === 0 && !kept
    toSelect.dataset.empty = String(empty)
    toHint.textContent = empty ? 'Start another session, or enter an ID.' : 'Or click a session in the list.'
  }

  function refreshReceivers() {
    if (draft.thread) {
      const name = nameOf(draft.to, draft.toTitle)
      composerTitle.textContent = `Reply to ${name}`
      toName.textContent = replyLabel(draft.to, draft.toTitle)
    } else if (draft.typing) {
      readTyped()
    } else {
      fillReceivers()
    }
    updateComposerControls()
  }

  function readTyped() {
    if (!draft?.typing) return
    const flat = receiverOptions().flatMap((group) => group.options)
    draft.typed = typedReceiverOf(flat, draft.from, target.value)
    targetStatus.textContent = draft.typed.note ?? ''
  }

  function markRows() {
    const marking = draft && !draft.thread
    const fromKey = draft && keyOf(draft.from)
    const toKey = marking && receiverOf().to?.tool ? keyOf(receiverOf().to) : undefined
    const byKey = new Map(rows().map((conversation) => [keyOf(conversation), conversation]))
    for (const row of shownRows()) {
      const key = row.dataset.key
      const conversation = byKey.get(key)
      const tag = marking && key === fromKey ? 'From' : marking && key === toKey ? 'To' : ''
      row.classList.toggle('is-mailbox-receivable', Boolean(marking && conversation && key !== fromKey && mailboxSessionOf(conversation)))
      row.classList.toggle('is-mailbox-from', tag === 'From')
      row.classList.toggle('is-mailbox-to', tag === 'To')
      const shown = row.querySelector(':scope > .text > .mailbox-tag')
      if (!tag) {
        shown?.remove()
      } else if (shown) {
        shown.textContent = tag
      } else {
        row.querySelector(':scope > .text')?.prepend(element('span', 'mailbox-tag', tag))
      }
    }
  }

  function updateComposerControls() {
    const pending = Boolean(draft?.pending)
    for (const field of fields) field.disabled = pending
    if (draft && !draft.thread) toSelect.disabled = pending || toSelect.dataset.empty === 'true'
    const missingWorktree = type.value === HANDOFF && !handoffWorktree.value.trim()
    claimOffer.disabled = pending || missingWorktree
    const { to, name, group } = receiverOf()
    send.disabled = pending || !to || missingWorktree || (type.value === REVIEW && !draft?.refs)
    send.textContent = name ? `Send to ${name}` : 'Send'
    typeHelp.textContent = typeHelpOf(type.value, name)
    arrival.textContent = arrivalOf(group, name)
    arrival.hidden = !arrival.textContent
    markRows()
  }

  function showTyping(typing) {
    draft.typing = typing
    toList.hidden = typing
    toId.hidden = !typing
    if (typing) {
      readTyped()
    } else {
      if (draft.typed?.status === 'found') draft.receiver = draft.typed.receiver
      draft.typed = undefined
      targetStatus.textContent = ''
      fillReceivers()
    }
    updateComposerControls()
  }

  function renderReview(refs, message = '') {
    reviewStatus.textContent = message
    resolved.hidden = !refs
    resolved.replaceChildren(...(refs ? refDetails(refs) : []))
    updateComposerControls()
  }

  function compareModels() {
    comparison.textContent = modelComparison(sendingModel.value, reviewingModel.value)
  }

  function previewRefs(delay) {
    stopPreview()
    const current = draft
    if (!current || current.pending) return
    const asked = base.value.trim()
    const reading = new AbortController()
    current.reading = reading
    current.refs = undefined
    renderReview(undefined, 'Reading Git references…')
    previewTimer = setTimeout(async () => {
      try {
        const refs = await readRefs(current.from, asked, reading.signal)
        if (draft !== current || current.reading !== reading) return
        if (!asked) base.value = refs.base
        current.refs = refs
        base.removeAttribute('aria-invalid')
        renderReview(refs)
      } catch (error) {
        if (error instanceof GivenUp || draft !== current || current.reading !== reading) return
        if (!(error instanceof CallError)) throw error
        base.setAttribute('aria-invalid', 'true')
        renderReview(undefined, refusalText(error))
      }
    }, delay)
  }

  async function previewWorktree() {
    const current = draft
    if (!current || current.worktreeEdited || handoffWorktree.value) return
    const reading = new AbortController()
    current.reading = reading
    handoffStatus.textContent = 'Reading Git worktree…'
    try {
      const { worktree } = await readClaims(current.from, reading.signal)
      if (draft !== current || current.reading !== reading) return
      handoffWorktree.value = worktree ?? ''
      handoffStatus.textContent = worktree ? '' : 'Enter the Git worktree root.'
    } catch (error) {
      if (error instanceof GivenUp || draft !== current || current.reading !== reading) return
      if (!(error instanceof CallError)) throw error
      handoffStatus.textContent = 'Could not read the Git worktree. Enter its root.'
    } finally {
      if (draft === current && current.reading === reading) {
        current.reading = undefined
        updateComposerControls()
      }
    }
  }

  function changeType() {
    const reviewing = type.value === REVIEW
    review.hidden = !reviewing
    handoff.hidden = type.value !== HANDOFF
    bodyField.hidden = reviewing
    if (reviewing) {
      compareModels()
      previewRefs(0)
    } else {
      stopPreview()
      renderReview()
      if (!handoff.hidden) previewWorktree()
    }
  }

  function start(from, { to, thread, toTitle } = {}) {
    if (draft?.pending || opened?.pending || claimsFor?.pending) return
    if (!thread) closeDetails(false)
    closeClaims(false)
    stopPreview()
    composer.reset()
    draft = { from: refOf(from), to, thread, toTitle }
    handoffStatus.textContent = ''
    targetStatus.textContent = ''
    const sender = rows().find((conversation) => keyOf(conversation) === keyOf(from))
    fromLabel.textContent = sender ? sessionLabelOf(sender) : nameOf(from)
    toFixed.hidden = !thread
    toList.hidden = Boolean(thread)
    toId.hidden = true
    toSelect.dataset.shape = ''
    composerTitle.textContent = 'Message another session'
    refreshReceivers()
    const models = thread ? opened?.thread?.models : undefined
    sendingModel.value = models?.[keyOf(from)]?.name ?? ''
    reviewingModel.value = to ? models?.[keyOf(to)]?.name ?? '' : ''
    base.removeAttribute('aria-invalid')
    changeType()
    details.hidden = true
    composer.hidden = false
    document.getElementById('card').classList.add('is-mailbox-composing')
    updateComposerControls()
    if (thread || toSelect.disabled) body.focus()
    else toSelect.focus()
  }

  /** A reply's To: the receiver as the list labels it, or the thread's stored title and the ID when the list does not hold it. */
  function replyLabel(to, toTitle) {
    const option = listedReceiver(keyOf(to))
    if (option) return option.label
    return toTitle ? `${toTitle} · ${to.id}` : to.id
  }

  function closeComposer(focus = true) {
    if (draft?.pending) return
    stopPreview()
    draft = undefined
    composer.hidden = true
    document.getElementById('card').classList.remove('is-mailbox-composing')
    markRows()
    if (opened) {
      details.hidden = false
      if (focus) dismiss.focus()
    } else if (focus) {
      focusRows()
    }
  }

  function choose(conversation) {
    if (!draft) return false
    if (draft.pending || draft.thread) return true
    if (!mailboxSessionOf(conversation) || keyOf(conversation) === keyOf(draft.from)) {
      say('Choose a different local Claude Code or Codex session.', 'error')
      return true
    }
    draft.receiver = listedReceiver(keyOf(conversation))
    draft.typed = undefined
    if (draft.typing) showTyping(false)
    else fillReceivers()
    updateComposerControls()
    const field = type.value === REVIEW ? base : type.value === HANDOFF ? handoffPaths : body
    field.focus()
    return true
  }

  toSelect.addEventListener('change', () => {
    if (!draft) return
    draft.receiver = receiverByKey(toSelect.value)
    updateComposerControls()
  })
  enterId.addEventListener('click', () => {
    if (!draft) return
    target.value = ''
    showTyping(true)
    target.focus()
  })
  useList.addEventListener('click', () => {
    if (!draft) return
    showTyping(false)
    toSelect.focus()
  })
  target.addEventListener('input', () => {
    readTyped()
    updateComposerControls()
  })
  type.addEventListener('change', () => {
    changeType()
    updateComposerControls()
  })
  handoffWorktree.addEventListener('input', () => {
    if (!draft) return
    draft.worktreeEdited = true
    stopPreview()
    handoffStatus.textContent = ''
    updateComposerControls()
  })
  base.addEventListener('input', () => {
    base.removeAttribute('aria-invalid')
    previewRefs(REF_PREVIEW_DELAY)
  })
  sendingModel.addEventListener('input', compareModels)
  reviewingModel.addEventListener('input', compareModels)
  cancel.addEventListener('click', () => closeComposer())
  claimOffer.addEventListener('click', async () => {
    const current = draft
    if (!current || type.value !== HANDOFF || claimOffer.disabled) return
    current.pending = true
    updateComposerControls()
    try {
      const { version } = await claimPaths({ ...current.from, ...pathsOf(handoffWorktree.value, handoffPaths.value) })
      say('Paths claimed.')
      await afterChange(version)
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      say(refusalText(error), 'error')
      if (error.guard === 'ownership') await afterChange(undefined)
    } finally {
      current.pending = false
      if (draft === current) refreshReceivers()
    }
  })
  composer.addEventListener('submit', async (event) => {
    event.preventDefault()
    const current = draft
    const reviewing = type.value === REVIEW
    if (!current || send.disabled) return
    current.pending = true
    updateComposerControls()
    const receiver = receiverOf(current)
    const to = receiver.to
    const request = { from: current.from, to, type: type.value }
    if (reviewing) {
      const { worktree, commit, base } = current.refs
      request.refs = { worktree, commit, base }
      const from = sendingModel.value.trim()
      const to = reviewingModel.value.trim()
      if (from || to) request.models = { ...(from && { from }), ...(to && { to }) }
    } else {
      request.body = body.value
      if (type.value === HANDOFF) request.gives = pathsOf(handoffWorktree.value, handoffPaths.value)
    }
    if (current.thread) request.thread = current.thread
    try {
      const { version } = await sendMessage(request)
      current.pending = false
      closeComposer()
      say(receiver.name ? `Message waiting in the thread with ${receiver.name}.` : 'Message waiting in the thread.')
      await afterChange(version)
      if (opened) await load(opened)
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      say(refusalText(error), 'error')
      if (error.guard === 'ownership') await afterChange(undefined)
      if (opened) await load(opened)
    } finally {
      current.pending = false
      if (draft === current) refreshReceivers()
    }
  })

  function closeDetails(focus = true) {
    if (opened?.pending) return
    opened?.reading?.abort()
    opened = undefined
    details.hidden = true
    if (focus) focusRows()
  }

  dismiss.addEventListener('click', () => closeDetails())

  function conflictText(conflict) {
    return conflictLabelOf(conflict, conflict.holder && nameOf(conflict.holder, keyOf(conflict.holder)))
  }

  function closeClaims(focus = true) {
    if (claimsFor?.pending) return
    claimsFor?.reading?.abort()
    claimsFor = undefined
    claimsCard.hidden = true
    if (focus) focusRows()
  }

  function openClaims(conversation) {
    if (draft || opened?.pending || claimsFor?.pending) return
    closeDetails(false)
    closeClaims(false)
    const owner = conversation && refOf(conversation)
    claimsFor = { owner, folder: conversation?.folder ?? '' }
    claimForm.reset()
    claimForm.hidden = !owner
    claimWorktree.value = claimsFor.folder
    claimsOwner.hidden = !owner
    claimsOwner.textContent = owner ? nameOf(owner) : ''
    claimsCard.hidden = false
    renderClaims(claimsFor)
    if (owner) claimWorktree.focus()
    else claimsDismiss.focus()
    loadClaims(claimsFor)
  }

  function renderClaims(current) {
    const focusedRelease = claimsList.contains(document.activeElement)
    claimsList.replaceChildren()
    for (const control of claimsCard.querySelectorAll('button, input, textarea')) {
      control.disabled = Boolean(current.pending)
    }
    if (focusedRelease && !current.pending) claimsDismiss.focus()
    if (!current.reply || current.problem) {
      claimsList.append(element('p', 'mailbox-muted', current.problem || 'Reading file claims…'))
      return
    }
    const { claims, conflicts, warning } = current.reply
    if (warning) claimsList.append(element('p', 'mailbox-refusal', warning))
    if (!claims?.length) claimsList.append(element('p', 'mailbox-muted', 'No file claims.'))
    for (const claim of claims ?? []) {
      const entry = element('article', 'mailbox-message')
      const release = button('Release', 'release', () => mutateClaim(current, () => releasePaths({
        ...refOf(claim.owner), worktree: claim.worktree, paths: claim.paths,
      }), 'Paths released.'))
      release.disabled = Boolean(current.pending)
      entry.append(
        element('p', 'mailbox-muted', `${claimStatusOf(claim)} · ${nameOf(claim.owner, keyOf(claim.owner))}`),
        ...pathDetails(claim),
        release,
      )
      claimsList.append(entry)
    }
    for (const conflict of conflicts ?? []) {
      claimsList.append(element('p', 'mailbox-refusal', conflictText(conflict)))
    }
  }

  async function loadClaims(current) {
    current.reading?.abort()
    const reading = new AbortController()
    current.reading = reading
    try {
      const reply = await readClaims(current.owner, reading.signal)
      if (claimsFor !== current || current.reading !== reading) return
      current.reply = reply
      current.problem = undefined
      if (reply.worktree && claimWorktree.value === current.folder) {
        claimWorktree.value = reply.worktree
        current.folder = reply.worktree
      }
    } catch (error) {
      if (error instanceof GivenUp || claimsFor !== current || current.reading !== reading) return
      if (!(error instanceof CallError)) throw error
      current.problem = error.message
    } finally {
      if (claimsFor === current && current.reading === reading) {
        current.reading = undefined
        renderClaims(current)
      }
    }
  }

  async function mutateClaim(current, change, message) {
    if (claimsFor !== current || current.pending) return
    const focused = document.activeElement
    const restore = claimsList.contains(focused) ? claimsDismiss : focused
    current.pending = true
    renderClaims(current)
    try {
      const { version } = await change()
      say(message)
      await afterChange(version)
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      say(refusalText(error), 'error')
      if (error.guard === 'ownership') await afterChange(undefined)
    } finally {
      current.pending = false
      await loadClaims(current)
      if (claimsFor === current && document.activeElement === document.body && claimsCard.contains(restore)) restore.focus()
    }
  }

  claimsDismiss.addEventListener('click', () => closeClaims())
  claimForm.addEventListener('submit', (event) => {
    event.preventDefault()
    const current = claimsFor
    if (!current?.owner) return
    const paths = pathsOf(claimWorktree.value, claimPathInput.value)
    mutateClaim(current, () => claimPaths({ ...current.owner, ...paths }), 'Paths claimed.')
  })

  function rowsOf(conversation) {
    const summaries = threadsOf(conversation)
    if (!summaries.length && !conversation.claimCount) return undefined
    const box = element('div', 'mailbox-threads')
    if (conversation.claimCount) {
      const label = claimCountOf(conversation.claimCount)
      const control = button(label, 'claims', () => openClaims(conversation), 'mailbox-summary')
      control.setAttribute('aria-label', `${label} for ${nameOf(conversation)}`)
      box.append(control)
    }
    for (const thread of summaries) {
      const other = nameOf(thread.with, thread.withTitle)
      const control = button('', 'open', () => {
        if (draft || opened?.pending || claimsFor?.pending) return
        closeClaims(false)
        opened?.reading?.abort()
        opened = { from: refOf(conversation), to: thread.with, summary: thread }
        title.textContent = `Thread with ${other}`
        details.hidden = false
        render(opened)
        dismiss.focus()
        load(opened)
      }, 'mailbox-summary')
      control.append(element('strong', '', `With ${other}`), element('span', '', [TYPES[thread.lastType], STATES[thread.status], threadLabelOf(thread)].filter(Boolean).join(' · ')))
      for (const conflict of thread.conflicts ?? []) control.append(element('span', 'mailbox-refusal', conflictText(conflict)))
      box.append(control)
    }
    return box
  }

  async function load(current) {
    current.reading?.abort()
    const reading = new AbortController()
    current.reading = reading
    try {
      const reply = await readThreads(current.from, reading.signal)
      if (opened !== current || current.reading !== reading) return
      current.thread = reply.threads.find(({ id }) => id === current.summary.id)
      current.problem = current.thread ? '' : reply.warning || 'This thread could not be read.'
    } catch (error) {
      if (error instanceof GivenUp || opened !== current || current.reading !== reading) return
      if (!(error instanceof CallError)) throw error
      current.problem = error.message
    } finally {
      if (opened === current && current.reading === reading) {
        current.reading = undefined
        render(current)
      }
    }
  }

  function render(current) {
    const focused = document.activeElement?.dataset.mailboxControl
    if (focused && details.contains(document.activeElement)) current.focus = focused
    const thread = current.thread
    meta.textContent = thread ? [STATES[thread.status], threadLabelOf(threadCount(thread))].join(' · ') : ''
    messages.replaceChildren()
    actions.replaceChildren()
    result.textContent = current.result ?? ''
    result.hidden = !current.result
    dismiss.disabled = Boolean(current.pending)
    if (current.problem || !thread) {
      messages.append(element('p', 'mailbox-muted', current.problem || 'Loading thread…'))
      restoreDetailFocus(current)
      return
    }
    for (const conflict of current.summary.conflicts ?? []) {
      messages.append(element('p', 'mailbox-refusal', conflictText(conflict)))
    }
    for (const message of thread.messages) {
      const entry = element('article', 'mailbox-message')
      const model = message.refs ? undefined : statedModelOf(thread, message)
      entry.append(element('p', 'mailbox-muted', [nameOf(message.from), senderKindOf(message), model, TYPES[message.type] ?? message.type, messageStateLabelOf(message.state), handoffAnswerOf(thread, message), sentAt(message.at)].filter(Boolean).join(' · ')))
      if (message.body) entry.append(element('p', 'mailbox-body', message.body))
      if (message.refs) {
        const models = thread.models ?? {}
        const refs = element('div', 'mailbox-review-refs')
        refs.append(...refDetails(message.refs), element('p', 'mailbox-muted', modelComparison(models[keyOf(message.from)]?.name, models[keyOf(message.to)]?.name)))
        entry.append(refs)
      }
      if (message.gives) {
        entry.append(...pathDetails(message.gives))
        if (message.handoff === 'offered' && !['held', 'rejected', 'cancelled'].includes(message.state) && thread.status !== 'paused' && keyOf(current.from) === keyOf(message.to)) {
          const controls = element('div', 'mailbox-actions')
          if (thread.status !== 'closed') controls.append(button('Accept', `accept-${message.n}`, () => respondHandoff(current, message.n, 'accept')))
          controls.append(button('Decline', `decline-${message.n}`, () => respondHandoff(current, message.n, 'decline')))
          entry.append(controls)
        }
      }
      const offered = messageActionsOf(message, thread)
      if (offered.length) {
        const controls = element('div', 'mailbox-actions')
        for (const action of offered) {
          controls.append(button(MESSAGE_ACTIONS[action], `${action}-${message.n}`, () => messageAction(current, message.n, action)))
        }
        entry.append(controls)
      }
      messages.append(entry)
    }
    for (const refusal of thread.refusals ?? []) {
      messages.append(element('p', 'mailbox-refusal', `Refused (guard: ${refusal.guard})${refusal.reason ? `. ${refusal.reason}` : '.'}`))
    }
    if (current.confirmStop) {
      actions.append(
        element('p', 'mailbox-muted', 'Stop this thread and cancel its waiting messages?'),
        button('Keep thread', 'keep', () => cancelStop(current)),
        button('Stop thread', 'confirm-stop', () => update(current, 'stop'), 'confirm-delete'),
      )
    } else {
      for (const action of threadActionsOf({ status: thread.status, ...threadCount(thread) })) {
        actions.append(button(ACTIONS[action], action, () => threadAction(current, action)))
      }
    }
    for (const control of details.querySelectorAll('[data-mailbox-control]')) control.disabled = Boolean(current.pending)
    restoreDetailFocus(current)
  }

  function restoreDetailFocus(current) {
    if (!current.focus || current.pending || details.hidden) return
    if (document.activeElement === document.body || details.contains(document.activeElement)) {
      const focusedControl = details.querySelector(`[data-mailbox-control="${CSS.escape(current.focus)}"]`)
      const control = focusedControl ?? dismiss
      control.focus()
    }
    current.focus = undefined
  }

  function cancelStop(current) {
    current.confirmStop = false
    render(current)
    actions.querySelector('[data-mailbox-control="stop"]')?.focus()
  }

  function threadAction(current, action) {
    switch (action) {
      case 'reply':
        start(current.from, { to: current.to, thread: current.summary.id, toTitle: current.summary.withTitle })
        break
      case 'stop':
        current.confirmStop = true
        render(current)
        actions.querySelector('[data-mailbox-control="keep"]')?.focus()
        break
      default:
        update(current, action)
    }
  }

  async function update(current, action) {
    if (current.pending) return
    current.pending = true
    current.confirmStop = false
    current.result = undefined
    render(current)
    try {
      const id = current.summary.id
      const { version } = action === 'close'
        ? await sendMessage({ from: current.from, to: current.to, type: 'done', thread: id })
        : await changeThread(id, action)
      await afterChange(version)
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      current.result = refusalText(error)
      say(current.result, 'error')
    } finally {
      current.pending = false
      await load(current)
    }
  }

  function messageAction(current, n, action) {
    switch (action) {
      case 'type':
      case 'copy':
        return stage(current, n, action)
      case 'queue':
        return answer(current, () => queueMessage(current.summary.id, n), 'Message queued for delivery.')
      default:
        return answer(current, () => approveMessage(current.summary.id, n, action), action === 'approve' ? 'Message approved.' : 'Message rejected.')
    }
  }

  async function answer(current, change, done) {
    if (current.pending) return
    current.pending = true
    current.result = undefined
    render(current)
    try {
      const { version } = await change()
      current.result = done
      await afterChange(version)
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      current.result = refusalText(error)
      say(current.result, 'error')
    } finally {
      current.pending = false
      await load(current)
    }
  }

  async function respondHandoff(current, n, action) {
    if (current.pending) return
    current.pending = true
    current.result = undefined
    render(current)
    try {
      const { version } = await answerHandoff(current.summary.id, n, action)
      current.result = action === 'accept' ? 'Handoff accepted.' : 'Handoff declined.'
      await afterChange(version)
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      current.result = refusalText(error)
      say(current.result, 'error')
      if (error.guard === 'ownership') await afterChange(undefined)
    } finally {
      current.pending = false
      await load(current)
    }
  }

  async function stage(current, n, how) {
    if (current.pending) return
    current.pending = true
    current.result = undefined
    render(current)
    let browserCopied = false
    try {
      const id = current.summary.id
      if (how === 'copy' && !nativeCopy) {
        const prepared = stageMessage(id, n, 'copy').then(({ body }) => {
          if (typeof body.text !== 'string') throw new CallError(body.message || 'The message could not be prepared.')
          return body.text
        })
        try {
          await Promise.all([prepared, copyBlock(prepared)])
        } catch (error) {
          if (error instanceof CallError) throw error
          throw new CallError('Could not copy the message. It is still waiting.', { cause: error })
        }
        browserCopied = true
      } else if (how === 'type') {
        await hideForTyping()
      }
      const { body, version } = await stageMessage(id, n, how, { confirm: browserCopied })
      current.result = body.message || (body.staged ? 'Message staged.' : 'The message is still waiting.')
      if (body.staged) {
        await afterChange(version)
        if (how === 'copy') say(current.result)
      } else {
        if (how === 'type') await showAfterRefusal()
        say(current.result, 'error')
      }
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      current.result = browserCopied ? `Copied to the clipboard. ${refusalText(error)}` : refusalText(error)
      if (how === 'type') await showAfterRefusal()
      say(current.result, 'error')
    } finally {
      current.pending = false
      await load(current)
    }
  }

  function viewChanged(view, version, mailboxNativeCopy) {
    if (view) nativeCopy = mailboxNativeCopy === true
    if (draft && !draft.pending) refreshReceivers()
    warning.textContent = view?.mailboxWarning ?? ''
    warning.hidden = !warning.textContent
    if (claimsFor && !claimsFor.pending && claimsFor.version !== version) {
      claimsFor.version = version
      loadClaims(claimsFor)
    }
    if (!opened || opened.version === version) return
    const row = rows().find((conversation) => keyOf(conversation) === keyOf(opened.from))
    const summary = row && threadsOf(row).find((thread) => thread.id === opened.summary.id)
    if (summary) opened.summary = summary
    opened.version = version
    if (!opened.pending) load(opened)
  }

  function escape() {
    if (draft) {
      closeComposer()
    } else if (claimsFor) {
      closeClaims()
    } else if (opened?.confirmStop) {
      cancelStop(opened)
    } else {
      closeDetails()
    }
  }

  return {
    start, choose, rowsOf, viewChanged, markRows, escape, closeDetails, openClaims, closeClaims,
    get composing() { return Boolean(draft) },
    get open() { return Boolean(draft || opened || claimsFor) },
  }
}
