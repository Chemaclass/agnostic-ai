import { Events, System } from '/wails/runtime.js'
import { CallError, readInbox, togglePanel } from '/api.js'
import { isDrag } from '/model.js'
// Waits for the person's animation choice, so a still circle never starts spinning.
import '/motion.js'

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
  const troubled = Boolean(view.marksNote || note)
  bubble.classList.toggle('is-troubled', troubled)
  bubble.classList.toggle('is-idle', count === 0 && !troubled)
  bubble.setAttribute('aria-label', count === 0 ? 'Conversations: none needs you' : `Conversations: ${count} need you`)
  badge.hidden = count === 0
  badge.textContent = badgeText(count)
  if (count > shownCount && document.documentElement.dataset.motion !== 'off') {
    // Restart the pop, so each new arrival is noticed.
    badge.classList.remove('is-new')
    void badge.offsetWidth
    badge.classList.add('is-new')
  }
  shownCount = count
}

// A pop left over would play again when animation is turned back on.
badge.addEventListener('animationend', () => badge.classList.remove('is-new'))
let wasHidden = false
document.addEventListener('motion', () => {
  if (document.documentElement.dataset.motion === 'off') badge.classList.remove('is-new')
  if (isHidden()) showHidden()
  // The news of a new list can arrive before the page hears it is shown again.
  else if (wasHidden) refresh()
  wasHidden = isHidden()
})

function isHidden() {
  return document.documentElement.dataset.hidden === 'on'
}

/** Hidden: nothing is read, so no old count waits for the return; the first list after it draws a new one. */
function showHidden() {
  // The pointer may leave while the circle is off the screen, unreported.
  bubble.classList.remove('is-working', 'is-troubled', 'is-idle', 'is-hovered')
  bubble.setAttribute('aria-label', 'Conversations: reading a new list')
  badge.hidden = true
  badge.classList.remove('is-new')
  shownCount = 0
}

/** No list could be read: an old count would be a guess, so none shows. */
function showTrouble(message) {
  bubble.classList.remove('is-working')
  bubble.classList.add('is-troubled')
  bubble.classList.remove('is-idle')
  bubble.setAttribute('aria-label', 'Conversations: ' + message)
  badge.hidden = true
  shownCount = 0
}

async function refresh() {
  // A hidden companion asks nothing.
  if (isHidden()) return
  let reply
  try {
    reply = await readInbox('')
  } catch (error) {
    if (!(error instanceof CallError)) throw error
    // Hidden while the list was on its way: the failure is the hiding.
    if (!isHidden()) showTrouble(error.message)
    return
  }
  if (isHidden()) return
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

// Ctrl+click on a Mac opens the menu; its release goes to the menu, not here.
bubble.addEventListener('mousedown', (event) => {
  if (event.button !== 0 || event.ctrlKey) return
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

window.addEventListener('contextmenu', () => {
  press.at = undefined
})

bubble.addEventListener('click', (event) => {
  if (press.dragged || event.ctrlKey) return
  togglePanel()
})
// The shell reports the pointer while another app is in front, where :hover may not fire.
Events.On('pointer', (event) => bubble.classList.toggle('is-hovered', event.data === true))
Events.On('inbox', refresh)
refresh()
