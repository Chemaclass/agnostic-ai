import { CallError, GivenUp, changeThread, readThreads, sendMessage, stageMessage } from '/api.js'
import { keyOf, mailboxSessionOf, threadActionsOf, threadLabelOf, threadsOf, titleOf } from '/model.js'

const ACTIONS = {
  reply: 'Reply',
  pause: 'Pause',
  resume: 'Resume',
  close: 'Close thread',
  'raise-cap': 'Allow 8 more',
  stop: 'Stop',
}
const TYPES = { question: 'Question', finding: 'Finding', done: 'Done' }
const STATES = { open: 'Open', paused: 'Paused', closing: 'Closing', closed: 'Closed', waiting: 'Waiting', staged: 'Staged', cancelled: 'Cancelled' }
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

function threadCount({ cap, messages }) {
  const count = messages.filter(({ type, state }) => type !== 'done' && state !== 'rejected').length
  const waiting = messages.filter(({ state }) => state === 'waiting').length
  return { cap, count, waiting }
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

export function mailboxPanel({ rows, focusRows, afterChange, say, hideForTyping, showAfterRefusal }) {
  const byId = (id) => document.getElementById(id)
  const warning = byId('mailbox-warning')
  const composer = byId('mailbox-compose')
  const fromLabel = byId('mailbox-from')
  const target = byId('mailbox-target')
  const hint = byId('mailbox-target-hint')
  const type = byId('mailbox-type')
  const body = byId('mailbox-body')
  const send = byId('mailbox-send')
  const details = byId('mailbox-details')
  const title = byId('mailbox-details-title')
  const meta = byId('mailbox-details-meta')
  const messages = byId('mailbox-messages')
  const result = byId('mailbox-result')
  const actions = byId('mailbox-thread-actions')
  const dismiss = byId('mailbox-dismiss')
  let draft
  let opened
  let nativeCopy = false

  function nameOf(ref, fallback) {
    const row = rows().find((conversation) => keyOf(conversation) === keyOf(ref))
    return row ? titleOf(row) : fallback || ref.id
  }

  function start(from, to, thread) {
    if (draft?.pending || opened?.pending) return
    if (!thread) closeDetails(false)
    composer.reset()
    draft = { from: refOf(from), to, thread }
    fromLabel.textContent = `From ${nameOf(from)}`
    target.value = to ? to.id : ''
    target.readOnly = Boolean(thread)
    hint.hidden = Boolean(thread)
    send.disabled = false
    details.hidden = true
    composer.hidden = false
    document.getElementById('card').classList.add('is-mailbox-composing')
    body.focus()
  }

  function closeComposer(focus = true) {
    if (draft?.pending) return
    draft = undefined
    composer.hidden = true
    document.getElementById('card').classList.remove('is-mailbox-composing')
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
    draft.to = refOf(conversation)
    target.value = conversation.id
    body.focus()
    return true
  }

  target.addEventListener('input', () => {
    if (draft) draft.to = undefined
  })
  byId('mailbox-cancel').addEventListener('click', () => closeComposer())
  composer.addEventListener('submit', async (event) => {
    event.preventDefault()
    const current = draft
    if (!current || current.pending) return
    current.pending = true
    send.disabled = true
    const to = current.to ?? { id: target.value.trim() }
    const request = { from: current.from, to, type: type.value, body: body.value }
    if (current.thread) request.thread = current.thread
    try {
      const { version } = await sendMessage(request)
      current.pending = false
      closeComposer()
      say('Message waiting in the thread.')
      await afterChange(version)
      if (opened) await load(opened)
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      say(refusalText(error), 'error')
      if (opened) await load(opened)
    } finally {
      current.pending = false
      send.disabled = false
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

  function rowsOf(conversation) {
    const summaries = threadsOf(conversation)
    if (!summaries.length) return undefined
    const box = element('div', 'mailbox-threads')
    for (const thread of summaries) {
      const other = nameOf(thread.with, thread.withTitle)
      const control = button('', 'open', () => {
        if (draft || opened?.pending) return
        opened?.reading?.abort()
        opened = { from: refOf(conversation), to: thread.with, summary: thread }
        title.textContent = `Thread with ${other}`
        details.hidden = false
        render(opened)
        dismiss.focus()
        load(opened)
      }, 'mailbox-summary')
      control.append(element('strong', '', `With ${other}`), element('span', '', [TYPES[thread.lastType], STATES[thread.status], threadLabelOf(thread)].filter(Boolean).join(' · ')))
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
    for (const message of thread.messages) {
      const entry = element('article', 'mailbox-message')
      entry.append(element('p', 'mailbox-muted', [nameOf(message.from), TYPES[message.type] ?? message.type, STATES[message.state], sentAt(message.at)].filter(Boolean).join(' · ')))
      if (message.body) entry.append(element('p', 'mailbox-body', message.body))
      if (message.state === 'waiting' && thread.status !== 'paused') {
        const controls = element('div', 'mailbox-actions')
        controls.append(
          button('Type into session', `type-${message.n}`, () => stage(current, message.n, 'type')),
          button('Copy message', `copy-${message.n}`, () => stage(current, message.n, 'copy')),
        )
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
        start(current.from, current.to, current.summary.id)
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
    warning.textContent = view?.mailboxWarning ?? ''
    warning.hidden = !warning.textContent
    if (!opened || opened.pending || opened.version === version) return
    opened.version = version
    load(opened)
  }

  function escape() {
    if (draft) {
      closeComposer()
    } else if (opened?.confirmStop) {
      cancelStop(opened)
    } else {
      closeDetails()
    }
  }

  return {
    start, choose, rowsOf, viewChanged, escape, closeDetails,
    get composing() { return Boolean(draft) },
    get open() { return Boolean(draft || opened) },
  }
}
