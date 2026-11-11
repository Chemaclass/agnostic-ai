// Sets data-motion and data-dim on the page, from the person's choices, and keeps
// them current. Each change is announced as a `motion` event on the document.
import { Events } from '/wails/runtime.js'
import { CallError, readAppearance } from '/api.js'
import { dimOf, motionOf } from '/look.js'

let told = false

function apply(appearance) {
  document.documentElement.dataset.motion = motionOf(appearance)
  document.documentElement.dataset.dim = dimOf(appearance)
  document.dispatchEvent(new Event('motion'))
}

Events.On('appearance', (event) => {
  told = true
  apply(event.data)
})
try {
  const saved = await readAppearance()
  // An event that came first is newer than this reply.
  if (!told) apply(saved)
} catch (error) {
  if (!(error instanceof CallError)) throw error
  // The page starts still until the choice is read; an unreadable choice moves.
  if (!told) apply(undefined)
}
