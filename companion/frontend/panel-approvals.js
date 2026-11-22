import { CallError, applySync, planSync, readSettings, saveSetting } from '/api.js'
import { approvalsConfirmOf, approvalsStatusOf, syncConfirmOf, syncFailureOf, syncResultOf } from '/model.js'

/** How often the open menu reads the settings again, so a change made in the web page shows. */
const READ_EVERY_MS = 2000

function element(tag, className, text) {
  const node = document.createElement(tag)
  if (className) node.className = className
  if (text !== undefined) node.textContent = text
  return node
}

function button(className, text) {
  const node = element('button', className, text)
  node.type = 'button'
  return node
}

/**
 * The approvals part of the settings menu: the status line under its rows, the question before a mode that
 * answers prompts, and Apply to my tools. `changed(doc)` hands each new settings document to the menu, which
 * draws the rows; `listChanged(version)` reads the list a change made, so the header chip follows. `row(id)` is the
 * control the focus goes to once a question is answered: the checked mode, or the companion switch after a sync.
 */
export function approvalsMenu({ changed, listChanged, row }) {
  const state = { doc: undefined, tag: undefined, timer: undefined, busy: false, said: undefined, pending: undefined, back: undefined, shown: 0, taken: 0 }

  const box = element('div', 'menu-approvals')
  const lines = element('div', 'menu-approvals-lines')
  const apply = button('menu-approvals-apply', 'Apply to my tools')
  apply.title = 'Runs agnostic-ai sync --scope user, after showing what it changes'
  const said = element('p', 'menu-approvals-said')
  said.setAttribute('role', 'status')
  const confirm = element('div', 'menu-confirm')
  confirm.setAttribute('role', 'group')
  const confirmTitle = element('p', 'menu-confirm-title')
  const confirmBody = element('p', 'menu-confirm-body')
  const confirmFiles = element('ul', 'menu-confirm-files')
  const confirmCommandsLabel = element('p', 'menu-confirm-body', 'Your tools will then run these commands:')
  const confirmCommands = element('ul', 'menu-confirm-files menu-confirm-commands')
  const confirmNotes = element('div', 'menu-confirm-notes')
  const confirmGo = button('menu-confirm-go', '')
  const confirmCancel = button('menu-confirm-cancel', 'Cancel')
  const confirmActions = element('div', 'menu-confirm-actions')
  confirmActions.append(confirmCancel, confirmGo)
  confirm.append(confirmTitle, confirmBody, confirmFiles, confirmCommandsLabel, confirmCommands, confirmNotes, confirmActions)
  confirm.setAttribute('aria-labelledby', 'menu-confirm-title')
  confirmTitle.id = 'menu-confirm-title'
  box.append(lines, apply, said, confirm)

  function draw() {
    const status = approvalsStatusOf(state.doc)
    box.hidden = state.doc === undefined
    lines.replaceChildren(...status.lines.map((line) => element('p', '', line)))
    apply.hidden = !status.apply || state.pending !== undefined
    apply.disabled = state.busy
    said.hidden = state.said === undefined
    said.textContent = state.said ?? ''
    confirm.hidden = state.pending === undefined
  }

  /** Shows doc. A document from a save or a sync comes without a tag, so the next read answers in full. */
  function take(doc, tag) {
    state.taken += 1
    state.doc = doc
    state.tag = tag
    changed(doc)
    draw()
  }

  /** Reads the settings; a document the tag still names changes nothing. One that cannot be read hides the rows. */
  async function read() {
    // A save or read that finished meanwhile holds a newer document than this answer.
    const taken = state.taken
    try {
      const answer = await readSettings(state.tag)
      if (answer && taken === state.taken) take(answer.settings, answer.tag)
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      if (taken !== state.taken) return
      take(undefined)
    }
  }

  /** Shows a question; Cancel takes the focus, so a repeated key does not answer it. */
  function ask(question, go, back) {
    state.pending = go
    state.back = back
    confirmTitle.textContent = question.title
    confirmBody.textContent = question.body
    confirmFiles.replaceChildren(...(question.files ?? []).map((file) => element('li', '', file)))
    confirmFiles.hidden = !question.files?.length
    confirmCommands.replaceChildren(...(question.commands ?? []).map((command) => element('li', '', command)))
    confirmCommands.hidden = confirmCommandsLabel.hidden = !question.commands?.length
    confirmNotes.replaceChildren(...(question.notes ?? []).map((note) => element('p', '', note)))
    confirmNotes.hidden = !question.notes?.length
    confirmGo.hidden = !question.go
    confirmGo.textContent = question.go ?? ''
    draw()
    confirmCancel.focus()
  }

  /** Closes the question; Cancel puts the focus back where it was asked from. */
  function answered() {
    const back = state.back
    state.pending = undefined
    state.back = undefined
    draw()
    return back
  }

  confirmCancel.addEventListener('click', () => answered()?.()?.focus())
  confirmGo.addEventListener('click', async () => {
    const go = state.pending
    answered()
    // Go hides the question, and the button it was asked from may hide too; the focus waits on the box until the
    // answer names where it belongs.
    box.tabIndex = -1
    box.focus()
    await go?.()
  })

  /** Saves one setting with the value the menu showed; a refusal shows why, and the server's current value. */
  async function save(key, value, expected) {
    if (state.busy) return
    state.busy = true
    state.said = undefined
    draw()
    let version
    try {
      const answer = await saveSetting(key, value, expected)
      version = answer.version
      take(answer.body.settings)
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      state.said = error.message
      if (error.settings) take(error.settings)
    } finally {
      state.busy = false
      draw()
    }
    if (version) await listChanged(version)
  }

  /** Ask me saves at once; a mode that answers prompts asks first. */
  function choose(mode, back) {
    const current = state.doc?.approvals?.mode
    if (!mode || mode === current || state.busy) return
    const question = approvalsConfirmOf(mode)
    if (!question) {
      save('approvals.mode', mode, current)
      return
    }
    ask(question, async () => {
      await save('approvals.mode', mode, current)
      row('approvalsMode')?.focus()
    }, back)
  }

  /** Apply to my tools: shows what a sync would change, then runs it. */
  async function sync() {
    if (state.busy) return
    const shown = state.shown
    state.busy = true
    state.said = 'Checking what a sync would change…'
    draw()
    let plan
    try {
      plan = await planSync()
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      state.said = syncFailureOf(error)
      return
    } finally {
      state.busy = false
      draw()
    }
    // The menu closed while the plan was read: nobody asked any more.
    if (shown !== state.shown) return
    state.said = undefined
    ask(syncConfirmOf(plan), () => runThenFocus(plan.id), () => apply)
  }

  async function runThenFocus(id) {
    await run(id)
    if (state.pending === undefined) row('approvalsCompanion')?.focus()
  }

  async function run(id) {
    const shown = state.shown
    state.busy = true
    state.said = 'Applying to your tools…'
    draw()
    try {
      const reply = await applySync(id)
      state.said = syncResultOf(reply)
      take(reply.settings)
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      if (error.plan) {
        state.busy = false
        if (shown !== state.shown) return
        state.said = error.message
        ask(syncConfirmOf(error.plan), () => runThenFocus(error.plan.id), () => apply)
        return
      }
      state.said = syncFailureOf(error)
    } finally {
      state.busy = false
      draw()
    }
  }

  apply.addEventListener('click', sync)

  return {
    box,
    read,
    choose,
    save,
    get doc() {
      return state.doc
    },
    /** Starts the reads while the menu shows. */
    opened() {
      clearInterval(state.timer)
      state.timer = setInterval(read, READ_EVERY_MS)
    },
    closed() {
      clearInterval(state.timer)
      state.timer = undefined
      state.shown += 1
      state.pending = undefined
      state.back = undefined
      state.said = undefined
      draw()
    },
  }
}
