import { CallError, GivenUp, answerHandoff, approveMessage, changeThread, claimPaths, dismissConflict, openDocs, queueMessage, readClaims, readRefs, readThreads, releasePaths, sendMessage, stageMessage } from '/api.js'
import { arrivalOf, claimMetaOf, claimsButtonOf, claimsCardOf, partialReleaseOf, releaseBatchesOf, releaseInBatches, conflictLabelOf, handoffAnswerOf, handoffSentOf, helpOf, keyOf, mailboxSessionOf, messageActionsOf, messageLabelOf, messageStateLabelOf, modelLabelOf, receiversOf, reservationRetryAt, reviewModelsOf, senderKindOf, sessionLabelOf, statedModelOf, threadActionsOf, threadNextOf, threadsOf, titleOf, typeHelpOf, typedReceiverOf, waitingDeliveryOf } from '/model.js'

const ACTION_LABELS = {
  approve: 'Approve message',
  reject: 'Reject message',
  queue: 'Queue for delivery',
  type: 'Paste into chat',
  copy: 'Copy message',
  accept: 'Accept files',
  decline: 'Decline files',
  'copy-commit': 'Copy full commit hash',
  reply: 'Reply',
  pause: 'Pause thread',
  resume: 'Resume thread',
  close: 'Finish thread',
  'raise-cap': 'Allow 8 more',
  stop: 'Cancel thread…',
}
const ACTION_TIPS = {
  type: 'Pastes the message into the receiving chat’s terminal, where you press Enter to send it.',
  close: 'Sends a final done message and ends the thread after its queued messages are handled.',
  stop: 'Ends the thread immediately and cancels waiting messages while both chats keep running.',
}
const REVIEW = 'review_request'
const HANDOFF = 'handoff'
const REF_PREVIEW_DELAY = 220
const TYPES = { question: 'Question', finding: 'Finding', done: 'Done', [REVIEW]: 'Review request', [HANDOFF]: 'Handoff' }
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

function actionButton(action, run, n) {
  const control = button(ACTION_LABELS[action], n === undefined ? action : `${action}-${n}`, run)
  if (ACTION_TIPS[action]) control.title = ACTION_TIPS[action]
  return control
}

function referenceDetails(fields) {
  const details = element('dl', 'mailbox-ref-details')
  for (const [label, value, full] of fields) {
    const data = element('dd')
    const code = element('code', full ? 'mailbox-short-hash' : '', value)
    if (full) code.title = full
    data.append(code)
    details.append(element('dt', '', label), data)
  }
  return details
}

function refDetails(refs) {
  const details = referenceDetails([['Worktree', refs.worktree], ['Commit', refs.commit], ['Base', refs.base]])
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

function threadCount({ cap, messages }, now, listed) {
  let count = 0
  let waiting = 0
  let waitingMessage
  let listedWaiting
  let delivering = 0
  let held = 0
  let retryAt = Infinity
  for (const message of messages) {
    if (message.type !== 'done' && message.state !== 'rejected') count++
    if (message.state === 'held') held++
    const readyAt = reservationRetryAt(message)
    if (message.state === 'waiting' || readyAt <= now) {
      waiting++
      waitingMessage ??= message
      if (message.state === 'waiting' && listed?.n === message.n && keyOf(listed.from) === keyOf(message.from) && keyOf(listed.to) === keyOf(message.to)) {
        listedWaiting = message
      }
    }
    if (readyAt > now) {
      delivering++
      retryAt = Math.min(retryAt, readyAt)
    }
  }
  return { cap, count, waiting, waitingMessage: listedWaiting ?? waitingMessage, setup: listedWaiting && listed.setup, delivering, held, retryAt }
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
  const bodyLabel = byId('mailbox-body-label')
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
  const claimsTitle = byId('mailbox-claims-title')
  const claimsButton = byId('mailbox-open-claims')
  const claimsCount = byId('mailbox-claims-count')
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
  let deliveryRetry
  let claimsFor
  let nativeCopy = false
  let previewTimer
  let dragged
  let rowDeliveryInFlight = false

  function nameOf(ref, fallback) {
    const row = rows().find((conversation) => keyOf(conversation) === keyOf(ref))
    return row ? titleOf(row) : fallback || ref.id
  }

  function threadNameOf(ref, current) {
    return nameOf(ref, keyOf(ref) === keyOf(current.from) ? current.fromTitle : current.summary.withTitle)
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
    const marking = Boolean(dragged) || Boolean(draft && !draft.thread)
    const fromKey = dragged ? dragged.from : draft && keyOf(draft.from)
    const toKey = dragged ? dragged.to : marking && receiverOf().to?.tool ? keyOf(receiverOf().to) : undefined
    const byKey = new Map(rows().map((conversation) => [keyOf(conversation), conversation]))
    for (const row of shownRows()) {
      const key = row.dataset.key
      const conversation = byKey.get(key)
      const end = marking && key === fromKey ? 'From' : marking && key === toKey ? 'To' : ''
      // A tag is a line of its own: during a drag it would move the rows under the pointer.
      const tag = dragged ? '' : end
      row.classList.toggle('is-mailbox-receivable', Boolean(marking && conversation && key !== fromKey && mailboxSessionOf(conversation)))
      row.classList.toggle('is-mailbox-from', end === 'From')
      row.classList.toggle('is-mailbox-to', end === 'To')
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
    const missingWorktree = !handoffWorktree.value.trim()
    claimOffer.disabled = pending || missingWorktree
    const { to, name, group } = receiverOf()
    const pathsNeedWorktree = type.value === HANDOFF && handoff.open && handoffPaths.value.trim() && missingWorktree
    send.disabled = pending || !to || pathsNeedWorktree || (type.value === REVIEW && !draft?.refs)
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
    if (!current || current.pending || current.worktreeEdited || handoffWorktree.value) return
    const reading = new AbortController()
    current.reading = reading
    handoffStatus.textContent = 'Reading Git worktree…'
    try {
      const { worktree } = await readClaims(current.from, reading.signal)
      if (draft !== current || current.reading !== reading) return
      handoffWorktree.value = worktree ?? ''
      // Typing in the focused field replaces a suggestion that just arrived.
      if (document.activeElement === handoffWorktree) handoffWorktree.select()
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
    if (handoff.hidden) handoff.open = false
    bodyField.hidden = reviewing
    bodyLabel.textContent = messageLabelOf(type.value)
    if (reviewing) {
      compareModels()
      previewRefs(0)
    } else {
      stopPreview()
      renderReview()
    }
  }

  function start(from, { to, thread, toTitle } = {}) {
    if (draft?.pending || opened?.pending || claimsFor?.pending) return
    if (!thread) closeDetails(false)
    closeClaims(false)
    stopPreview()
    composer.reset()
    handoff.open = false
    type.value = thread ? 'question' : HANDOFF
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
    sendingModel.value = modelLabelOf(models, from)
    reviewingModel.value = modelLabelOf(models, to)
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
    const blocked = handoff.open && send.disabled
    const field = type.value === REVIEW ? base : type.value !== HANDOFF ? body : blocked ? handoffWorktree : send
    field.focus()
    return true
  }

  /** Marks the rows of a handoff being dragged, by key: `from` and the row under the pointer. Without `from` it clears them. */
  function markDrag(from, to) {
    dragged = from ? { from, to } : undefined
    markRows()
  }

  /** A drop of `from` on `to`: the new draft the menu opens, with the receiver chosen. */
  function handOff(from, to) {
    start(from)
    if (draft && !draft.thread && keyOf(draft.from) === keyOf(from)) choose(to)
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
  handoff.addEventListener('toggle', () => {
    if (!draft) return
    if (handoff.open) {
      if (!handoff.hidden) previewWorktree()
    } else {
      // A closed section gives no files, so it keeps no paths that could look offered.
      stopPreview()
      handoffPaths.value = ''
      handoffStatus.textContent = ''
    }
    updateComposerControls()
  })
  handoffPaths.addEventListener('input', updateComposerControls)
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
    const handingOff = type.value === HANDOFF
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
    } else if (handingOff) {
      if (body.value.trim()) request.body = body.value
      if (handoff.open && handoffPaths.value.trim()) request.gives = pathsOf(handoffWorktree.value, handoffPaths.value)
    } else {
      request.body = body.value
    }
    if (current.thread) request.thread = current.thread
    try {
      const { body: receipt, version } = await sendMessage(request)
      current.pending = false
      closeComposer()
      say(handingOff
        ? handoffSentOf(receipt, receiver.name, nameOf(current.from))
        : receiver.name ? `Message waiting in the thread with ${receiver.name}.` : 'Message waiting in the thread.')
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
    clearTimeout(deliveryRetry)
    opened = undefined
    messages.replaceChildren()
    actions.replaceChildren()
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
    claimsTitle.textContent = claimsCardOf(undefined, owner && nameOf(owner)).title
    claimsCard.hidden = false
    renderClaims(claimsFor)
    if (owner) claimWorktree.focus()
    else claimsDismiss.focus()
    loadClaims(claimsFor)
  }

  function renderClaims(current) {
    const restore = claimsList.contains(document.activeElement) ? claimsDismiss : current.restoreFocus
    claimsList.replaceChildren()
    for (const control of claimsCard.querySelectorAll('button, input, textarea')) {
      control.disabled = Boolean(current.pending)
    }
    if (!current.pending) {
      current.restoreFocus = undefined
      if (restore && document.activeElement === document.body) restore.focus()
    }
    if (!current.reply || current.problem) {
      claimsList.append(element('p', 'mailbox-muted', current.problem || 'Reading file claims…'))
      return
    }
    const { conflicts, warning } = current.reply
    if (warning) claimsList.append(element('p', 'mailbox-refusal', warning))
    const card = claimsCardOf(current.reply, current.owner && nameOf(current.owner))
    if (card.empty) claimsList.append(element('p', 'mailbox-muted', card.empty))
    // A gone session's claims go with `gone`, so the server checks again that the session is gone and the claims unchanged.
    const release = (claims, gone = false) => mutateClaim(current, async () => {
      const result = await releaseInBatches(releaseBatchesOf(claims, gone ? { gone: true } : {}), releasePaths)
      if (!result.error) return { version: result.version }
      if (!(result.error instanceof CallError) || result.released === 0) throw result.error
      return { version: result.version, problem: partialReleaseOf(result, refusalText(result.error)) }
    }, 'Paths released.')
    if (card.stale.length) {
      const group = element('section', 'mailbox-claims-gone')
      const heading = element('h3', 'mailbox-claims-heading', 'Session gone')
      heading.id = 'mailbox-claims-gone'
      group.setAttribute('aria-labelledby', heading.id)
      group.append(heading)
      if (card.stale.length > 1) group.append(button('Release all', 'release-all', () => release(card.stale, true)))
      for (const claim of card.stale) group.append(claimEntry(claim, (claims) => release(claims, true), current.owner))
      claimsList.append(group)
    }
    for (const claim of card.active) claimsList.append(claimEntry(claim, release, current.owner))
    for (const conflict of conflicts ?? []) {
      const line = element('p', 'mailbox-refusal', conflictText(conflict))
      // A conflict with no thread is the requester's to drop, from its own card or the footer's, where a requester
      // with no row can still be cleared; a thread's conflict clears with its thread.
      if (!conflict.thread && (!current.owner || keyOf(conflict.requester) === keyOf(current.owner))) {
        claimsList.append(line, button('Dismiss', 'dismiss-conflict', () => mutateClaim(current, () => dismissConflict({
          ...refOf(conflict.requester), conflict: conflict.id,
        }), 'Conflict dismissed.')))
      } else if (conflict.thread && !current.owner) {
        // The footer's card reaches an offer whose chats left the list, so it can still end it.
        claimsList.append(line, button('Decline offer', 'decline-offer', () => mutateClaim(current,
          () => answerHandoff(conflict.thread, conflict.n, 'decline'), 'Handoff declined.')))
      } else {
        claimsList.append(line)
      }
    }
    if (current.pending) for (const control of claimsList.querySelectorAll('button')) control.disabled = true
  }

  function claimEntry(claim, release, owner) {
    const entry = element('article', 'mailbox-message')
    const meta = claimMetaOf(claim)
    entry.append(
      element('p', 'mailbox-muted', owner ? meta : `${nameOf(claim.owner, keyOf(claim.owner))} · ${meta}`),
      ...pathDetails(claim),
      button('Release', 'release', () => release([claim])),
    )
    return entry
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
    if (claimsCard.contains(focused)) current.restoreFocus = claimsList.contains(focused) ? claimsDismiss : focused
    current.pending = true
    renderClaims(current)
    try {
      const { version, problem } = await change()
      say(problem ?? message, problem ? 'error' : undefined)
      await afterChange(version)
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      say(refusalText(error), 'error')
      if (error.guard === 'ownership') await afterChange(undefined)
    } finally {
      current.pending = false
      await loadClaims(current)
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
    if (!summaries.length) return undefined
    const box = element('div', 'mailbox-threads')
    for (const thread of summaries) {
      const other = nameOf(thread.with, thread.withTitle)
      const waiting = thread.waiting ? thread.waitingMessage : undefined
      const recipient = waiting && (keyOf(waiting.to) === keyOf(conversation) ? titleOf(conversation) : other)
      const heading = waiting
        ? `${TYPES[waiting.type]} ${keyOf(waiting.from) === keyOf(conversation) ? `sent to ${other}` : `from ${other}`}`
        : `With ${other}`
      const next = threadNextOf(thread, {
        recipient, recipientTool: waiting?.to.tool, recipientLive: waiting?.recipientLive, setup: waiting?.setup, summary: true,
      })
      const line = element('div', 'mailbox-summary-line')
      const control = button('', 'open', () => {
        if (draft || opened?.pending || claimsFor?.pending || rowDeliveryInFlight) return
        closeClaims(false)
        opened?.reading?.abort()
        opened = { from: refOf(conversation), fromTitle: titleOf(conversation), to: thread.with, summary: thread }
        title.textContent = `Thread with ${other}`
        details.hidden = false
        render(opened)
        dismiss.focus()
        load(opened)
      }, 'mailbox-summary')
      control.setAttribute('aria-label', `Open thread with ${other}`)
      const summary = element('span', 'mailbox-summary-heading')
      const arrow = element('span', 'mailbox-summary-arrow', '›')
      arrow.setAttribute('aria-hidden', 'true')
      summary.append(element('strong', '', heading), arrow)
      const reason = element('span', '', [waiting ? undefined : TYPES[thread.lastType], next].filter(Boolean).join(' · '))
      reason.id = `mailbox-summary-${keyOf(conversation)}-${thread.id}`
      summary.id = `${reason.id}-heading`
      control.id = `${reason.id}-control`
      control.setAttribute('aria-describedby', `${summary.id} ${reason.id}`)
      control.append(summary, reason)
      for (const conflict of thread.conflicts ?? []) control.append(element('span', 'mailbox-refusal', conflictText(conflict)))
      line.append(control)
      const delivery = waitingDeliveryOf(thread)
      if (delivery) {
        const action = actionButton(delivery.how, () => deliverFromRow(conversation, thread.id, delivery, action, control.id), delivery.n)
        action.id = `${control.id}-${delivery.how}`
        line.append(action)
      }
      box.append(line)
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
    clearTimeout(deliveryRetry)
    const focused = document.activeElement?.dataset.mailboxControl
    if (focused && details.contains(document.activeElement)) current.focus = focused
    const expanded = new Set([...details.querySelectorAll('details[open]')].map((node) => node.dataset.disclosure))
    const thread = current.thread
    meta.textContent = ''
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
    const now = Date.now()
    const counts = threadCount(thread, now, current.summary.waitingMessage)
    if (!current.pending && ['open', 'closing'].includes(thread.status) && Number.isFinite(counts.retryAt)) {
      deliveryRetry = setTimeout(() => {
        if (opened === current && !current.pending) load(current)
      }, Math.min(counts.retryAt - now, 2_147_483_647))
    }
    const threadControls = threadActionsOf({ status: thread.status, ...counts })
      .map((action) => actionButton(action, () => threadAction(current, action)))
    const messageGroups = thread.messages.map((message) => renderMessage(current, message, expanded, now))
    const { waitingMessage: waiting, setup } = counts
    const primary = choosePrimary(threadControls, messageGroups, thread.status, waiting)
    const pendingMessage = messageGroups.find((group) => group.controls.includes(primary))?.message
    const recipient = waiting && threadNameOf(waiting.to, current)
    const showDelivery = waiting && !counts.held && ['open', 'closing'].includes(thread.status)
    const action = primary?.dataset.mailboxControl
    meta.textContent = threadNextOf({ status: thread.status, ...counts }, {
      recipient, recipientTool: waiting?.to.tool, recipientLive: recipientLiveOf(current, waiting?.to), setup,
    })
    if (showDelivery) {
      const sender = threadNameOf(waiting.from, current)
      meta.textContent = `${TYPES[waiting.type]} from ${sender} to ${recipient}. ${meta.textContent}`
    }
    if (action?.startsWith('accept-')) {
      const accept = 'Accept the file offer to move the file notes to the receiving chat.'
      meta.textContent = showDelivery ? `${meta.textContent} ${accept}` : accept
    }
    const describedMessage = showDelivery ? waiting : pendingMessage
    if (describedMessage && thread.messages.length > 1) {
      meta.textContent = `Message ${describedMessage.n}. ${meta.textContent}`
    }
    if (thread.status === 'open' && counts.count >= counts.cap && (counts.waiting || counts.held || counts.delivering || action?.startsWith('accept-'))) {
      meta.textContent += ` This thread reached its ${counts.cap}-message limit.`
    }
    if (showDelivery && setup === 'off') {
      const { path, title: hover } = helpOf('mailbox-delivery')
      const guide = button('Turn on automatic delivery', 'delivery-guide', () => openDocs(path).catch((error) => say(error.message, 'error')), 'mailbox-delivery-guide')
      guide.title = hover
      meta.append(' Add ', element('code', '', 'builtins: [mailbox]'), ' to the receiving project’s ', element('code', '', 'agnostic-ai.yaml'), ', then sync. ', guide)
    }
    for (const refusal of thread.refusals ?? []) {
      messages.append(element('p', 'mailbox-refusal', `Refused (guard: ${refusal.guard})${refusal.reason ? `. ${refusal.reason}` : '.'}`))
    }
    if (current.confirmStop) {
      actions.append(
        element('p', 'mailbox-muted', 'Cancel this thread and its waiting messages? Both chats will keep running.'),
        button('Keep thread', 'keep', () => cancelStop(current)),
        button('Cancel thread now', 'confirm-stop', () => update(current, 'stop'), 'confirm-delete'),
      )
    } else {
      renderActions(current, primary, threadControls, messageGroups, expanded.has('more'))
    }
    for (const control of details.querySelectorAll('button[data-mailbox-control]')) control.disabled = Boolean(current.pending)
    restoreDetailFocus(current)
  }

  function recipientLiveOf(current, recipient) {
    if (!recipient) return undefined
    const key = keyOf(recipient)
    const waiting = current.summary.waitingMessage
    if (waiting && keyOf(waiting.to) === key) return waiting.recipientLive
    const row = rows().find((listed) => keyOf(listed) === key)
    return row && row.live === true
  }

  function renderMessage(current, message, expanded, now) {
    const thread = current.thread
    const entry = element('article', 'mailbox-message')
    const sender = nameOf(message.from)
    const model = message.refs ? undefined : statedModelOf(thread, message)
    const type = TYPES[message.type] ?? message.type
    const how = message.state === 'staged' && thread.events?.findLast(({ n, action }) => n === message.n && ['type', 'copy'].includes(action))?.action
    const state = reservationRetryAt(message) <= now ? 'waiting' : message.state
    const number = thread.messages.length > 1 ? `Message ${message.n}` : undefined
    entry.append(element('p', 'mailbox-muted', [number, sender, senderKindOf(message), model, type, messageStateLabelOf(state, how), handoffAnswerOf(thread, message), sentAt(message.at)].filter(Boolean).join(' · ')))
    if (message.body) entry.append(element('p', 'mailbox-body', message.body))
    if (message.refs) {
      const models = thread.models ?? {}
      const refs = element('div', 'mailbox-review-refs')
      refs.append(...refDetails(message.refs), element('p', 'mailbox-muted', modelComparison(modelLabelOf(models, message.from), modelLabelOf(models, message.to))))
      entry.append(refs)
    }
    if (message.resume) renderHandoffDetails(entry, message, expanded)
    const controls = []
    if (message.gives) {
      entry.append(...pathDetails(message.gives))
      if (message.handoff === 'offered' && !['held', 'rejected', 'cancelled'].includes(message.state) && thread.status !== 'paused' && keyOf(current.from) === keyOf(message.to)) {
        if (thread.status !== 'closed') controls.push(actionButton('accept', () => respondHandoff(current, message.n, 'accept'), message.n))
        controls.push(actionButton('decline', () => respondHandoff(current, message.n, 'decline'), message.n))
      }
    }
    for (const action of messageActionsOf(message, { status: thread.status, recipientLive: recipientLiveOf(current, message.to) }, now)) {
      controls.push(actionButton(action, () => messageAction(current, message.n, action), message.n))
    }
    if (message.resume?.head) controls.push(actionButton('copy-commit', () => copyHandoffCommit(current, message.resume.head), message.n))
    messages.append(entry)
    return { message, label: `Message ${message.n} · ${type} from ${sender}`, controls }
  }

  function renderHandoffDetails(entry, message, expanded) {
    const { worktree, project, branch, head, note } = message.resume
    const fields = [['Worktree', worktree]]
    if (project) fields.push(['Project', project])
    fields.push(['Branch', branch || 'Detached HEAD'])
    if (head) fields.push(['Commit', head.slice(0, 7), head])
    if (note) fields.push(['Note', note])
    const disclosure = element('details', 'mailbox-handoff-details')
    disclosure.dataset.disclosure = `details-${message.n}`
    disclosure.open = expanded.has(disclosure.dataset.disclosure)
    const summary = element('summary', '', 'Handoff details')
    summary.dataset.mailboxControl = disclosure.dataset.disclosure
    disclosure.append(summary, referenceDetails(fields))
    entry.append(disclosure)
  }

  async function copyHandoffCommit(current, head) {
    try {
      await copyBlock(Promise.resolve(head))
      current.result = 'Full commit hash copied.'
    } catch {
      current.result = 'Could not copy the commit hash.'
    }
    if (opened !== current) return
    result.textContent = current.result
    result.hidden = false
  }

  function choosePrimary(threadControls, messageGroups, status, waiting) {
    if (status === 'paused') return threadControls.find((control) => control.dataset.mailboxControl === 'resume')
    const controls = messageGroups.flatMap((group) => group.controls)
    for (const prefix of ['approve-', 'accept-']) {
      const control = controls.find((item) => item.dataset.mailboxControl.startsWith(prefix))
      if (control) return control
    }
    const delivery = waiting && controls.find((control) => [`type-${waiting.n}`, `copy-${waiting.n}`].includes(control.dataset.mailboxControl))
    return delivery ?? threadControls.find((control) => control.dataset.mailboxControl === 'raise-cap')
      ?? threadControls.find((control) => control.dataset.mailboxControl === 'reply')
  }

  function renderActions(current, primary, threadControls, messageGroups, expanded) {
    if (primary) {
      primary.classList.replace('confirm-cancel', 'mailbox-send')
      actions.append(primary)
    }
    const menu = element('details', 'mailbox-more')
    menu.dataset.disclosure = 'more'
    menu.open = expanded && !current.pending
    const summary = element('summary', '', 'More')
    summary.dataset.mailboxControl = 'more'
    menu.append(summary)
    const options = element('div', 'mailbox-more-options')
    const secondaryThread = threadControls.filter((control) => control !== primary)
    if (secondaryThread.length) options.append(actionGroup('Thread actions', secondaryThread))
    for (const { label, controls } of messageGroups) {
      const secondary = controls.filter((control) => control !== primary)
      if (secondary.length) options.append(actionGroup(label, secondary))
    }
    if (options.childElementCount) {
      menu.append(options)
      actions.append(menu)
    }
  }

  function actionGroup(label, controls) {
    const group = element('div', 'mailbox-more-group')
    group.setAttribute('role', 'group')
    group.setAttribute('aria-label', label)
    group.append(element('p', 'mailbox-muted', label), ...controls)
    return group
  }

  function restoreDetailFocus(current) {
    if (!current.focus || current.pending || details.hidden) return
    if (document.activeElement === document.body || details.contains(document.activeElement)) {
      const focusedControl = details.querySelector(`[data-mailbox-control="${CSS.escape(current.focus)}"]`)
      const more = actions.querySelector('.mailbox-more')
      if (focusedControl?.closest('.mailbox-more-options')) more.open = true
      const control = focusedControl ?? more?.querySelector('summary') ?? dismiss
      control.focus()
    }
    current.focus = undefined
  }

  function cancelStop(current) {
    current.confirmStop = false
    render(current)
    actions.querySelector('[data-mailbox-control="more"]')?.focus()
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

  async function deliverFromRow(conversation, id, { n, how }, control, summaryID) {
    if (rowDeliveryInFlight || draft || opened?.pending || claimsFor?.pending) return
    const row = rows().find((item) => keyOf(item) === keyOf(conversation))
    const summary = row && threadsOf(row).find((thread) => thread.id === id)
    const delivery = summary && waitingDeliveryOf(summary)
    if (delivery?.n !== n || delivery.how !== how) return
    const restoreFocus = how === 'copy' && document.activeElement === control
    let focused = control
    const trackFocus = (event) => { focused = event.target }
    if (restoreFocus) document.addEventListener('focusin', trackFocus)
    rowDeliveryInFlight = true
    control.disabled = true
    try {
      await stage({ summary }, n, how)
    } finally {
      rowDeliveryInFlight = false
      control.disabled = false
      if (restoreFocus) {
        document.removeEventListener('focusin', trackFocus)
        if (document.activeElement === document.body || document.activeElement === control) {
          const id = focused === control && !control.isConnected ? summaryID : focused.id
          document.getElementById(id)?.focus({ preventScroll: true })
        }
      }
    }
  }

  async function stage(current, n, how) {
    const inCard = opened === current
    if (current.pending || (inCard && rowDeliveryInFlight)) return
    current.pending = true
    current.result = undefined
    if (inCard) render(current)
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
      current.result = body.message || (body.staged ? (how === 'copy' ? 'Message copied.' : 'Message pasted into the chat.') : 'The message is still waiting.')
      if (body.staged) {
        await afterChange(version)
        if (how === 'copy' || !inCard) say(current.result)
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
      if (inCard) await load(current)
    }
  }

  function viewChanged(view, version, mailboxNativeCopy) {
    if (view) nativeCopy = mailboxNativeCopy === true
    if (draft && !draft.pending) refreshReceivers()
    warning.textContent = view?.mailboxWarning ?? ''
    warning.hidden = !warning.textContent
    const claims = claimsButtonOf(view)
    claimsButton.hidden = !claims.shown
    claimsButton.classList.toggle('is-warning', Boolean(claims.warns))
    claimsCount.textContent = claims.text ?? ''
    if (claims.shown) claimsButton.setAttribute('aria-label', claims.label)
    claimsButton.title = claims.label ?? ''
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
      const more = actions.querySelector('.mailbox-more[open]')
      if (more) {
        more.open = false
        more.querySelector('summary').focus()
      } else {
        closeDetails()
      }
    }
  }

  return {
    start, choose, handOff, markDrag, rowsOf, viewChanged, markRows, escape, closeDetails, openClaims, closeClaims,
    get composing() { return Boolean(draft) },
    get open() { return Boolean(draft || opened || claimsFor) },
  }
}
