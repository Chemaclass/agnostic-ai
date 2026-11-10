import { Events, System } from '/wails/runtime.js'
import { CallError, readInbox, togglePanel } from '/api.js'
import { isDrag } from '/model.js'

const bubble = document.getElementById('bubble')
const badge = document.getElementById('badge')
let shownCount = 0

function badgeText(count) {
  return count > 99 ? '99+' : String(count)
}

function show({ view, note }) {
  const count = view.needsYou
  const working = view.conversations.some((conversation) => conversation.state === 'working')
  bubble.classList.toggle('is-working', working)
  bubble.classList.toggle('is-troubled', Boolean(view.marksNote || note))
  bubble.setAttribute('aria-label', count === 0 ? 'Conversations: none needs you' : `Conversations: ${count} need you`)
  badge.hidden = count === 0
  badge.textContent = badgeText(count)
  if (count > shownCount) {
    // Restart the pop, so each new arrival is noticed.
    badge.classList.remove('is-new')
    void badge.offsetWidth
    badge.classList.add('is-new')
  }
  shownCount = count
}

/** No list could be read: an old count would be a guess, so none shows. */
function showTrouble(message) {
  bubble.classList.remove('is-working')
  bubble.classList.add('is-troubled')
  bubble.setAttribute('aria-label', 'Conversations: ' + message)
  badge.hidden = true
  shownCount = 0
}

async function refresh() {
  let reply
  try {
    reply = await readInbox('')
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    showTrouble(error.message)
    return
  }
  show(reply)
}

// A press becomes a drag once the pointer has travelled; until then it is a
// click. While the window system drags the window it keeps the mouse, so the
// release may never reach this page: the next press starts over.
const press = {
  /** Where the primary button went down; undefined when it is up or the drag began. */
  at: undefined,
  dragged: false,
}

bubble.addEventListener('mousedown', (event) => {
  if (event.button !== 0) return
  press.at = { x: event.screenX, y: event.screenY }
  press.dragged = false
})

window.addEventListener('mousemove', (event) => {
  if (!press.at) return
  if (!isDrag(press.at, { x: event.screenX, y: event.screenY })) return
  press.at = undefined
  press.dragged = true
  System.invoke('wails:drag')
})

window.addEventListener('mouseup', () => {
  press.at = undefined
})

bubble.addEventListener('click', () => {
  if (press.dragged) return
  togglePanel()
})
Events.On('inbox', refresh)
refresh()
