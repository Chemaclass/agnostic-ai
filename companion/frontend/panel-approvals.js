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
 * draws the rows; `listChanged(version)` reads the list a change made, so the header chip follows.
 */
export function approvalsMenu({ changed, listChanged }) {
  const state = { doc: undefined, tag: undefined, timer: undefined, busy: false, said: undefined, pending: undefined }

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
  const confirmGo = button('menu-confirm-go', '')
  const confirmCancel = button('menu-confirm-cancel', 'Cancel')
  const confirmActions = element('div', 'menu-confirm-actions')
  confirmActions.append(confirmCancel, confirmGo)
  confirm.append(confirmTitle, confirmBody, confirmFiles, confirmActions)
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

  function take(doc, tag) {
    state.doc = doc
    if (tag !== undefined) state.tag = tag
    changed(doc)
    draw()
  }

  /** Reads the settings; a document the tag still names changes nothing. One that cannot be read hides the rows. */
  async function read() {
    try {
      const answer = await readSettings(state.tag)
      if (answer) take(answer.settings, answer.tag)
    } catch (error) {
      if (!(error instanceof CallError)) throw error
      state.tag = undefined
      take(undefined)
    }
  }

  function ask(question, go) {
    state.pending = go
    confirmTitle.textContent = question.title
    confirmBody.textContent = question.body
    confirmFiles.replaceChildren(...(question.files ?? []).map((file) => element('li', '', file)))
    confirmFiles.hidden = !question.files?.length
    confirmGo.hidden = !question.go
    confirmGo.textContent = question.go ?? ''
    draw()
    ;(question.go ? confirmGo : confirmCancel).focus()
  }

  function dismiss() {
    state.pending = undefined
    draw()
  }

  confirmCancel.addEventListener('click', dismiss)
  confirmGo.addEventListener('click', () => {
    const go = state.pending
    state.pending = undefined
    draw()
    go?.()
  })

  /** Saves one setting with the value the menu showed; a refusal shows why, and the server's current value. */
  async function save(key, value, expected) {
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
  function choose(mode) {
    const current = state.doc?.approvals?.mode
    if (!mode || mode === current || state.busy) return
    const question = approvalsConfirmOf(mode)
    if (!question) {
      save('approvals.mode', mode, current)
      return
    }
    ask(question, () => save('approvals.mode', mode, current))
  }

  /** Apply to my tools: shows what a sync would change, then runs it. */
  async function sync() {
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
    state.said = undefined
    ask(syncConfirmOf(plan), () => run(plan.id))
  }

  async function run(id) {
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
        state.said = error.message
        ask(syncConfirmOf(error.plan), () => run(error.plan.id))
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
      state.pending = undefined
      state.said = undefined
      draw()
    },
  }
}
