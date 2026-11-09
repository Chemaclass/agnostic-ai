import { Events } from '/wails/runtime.js'
import { readInbox, togglePanel } from '/api.js'

const bubble = document.getElementById('bubble')
const badge = document.getElementById('badge')
let shownCount = 0

function badgeText(count) {
  return count > 99 ? '99+' : String(count)
}

function show(view) {
  const count = view.needsYou
  const working = view.conversations.some((conversation) => conversation.state === 'working')
  bubble.classList.toggle('is-working', working)
  bubble.classList.toggle('is-troubled', Boolean(view.marksNote))
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

async function refresh() {
  show(await readInbox(''))
}

bubble.addEventListener('click', () => togglePanel())
Events.On('inbox', refresh)
refresh()
