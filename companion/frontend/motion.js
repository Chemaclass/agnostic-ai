// Sets data-motion on the page, from the person's choice, and keeps it current.
import { Events } from '/wails/runtime.js'
import { CallError, readAppearance } from '/api.js'
import { motionOf } from '/look.js'

function apply(appearance) {
  document.documentElement.dataset.motion = motionOf(appearance)
}

Events.On('appearance', (event) => apply(event.data))
try {
  apply(await readAppearance())
} catch (error) {
  if (!(error instanceof CallError)) throw error
}
