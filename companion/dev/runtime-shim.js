// Stands in for /wails/runtime.js in the browser preview. The shell's events come
// from the preview server over one event stream; a reload event reloads the page.
const handlers = new Map()
const stream = new EventSource('/dev/events')
stream.addEventListener('message', (message) => {
  const { name, data } = JSON.parse(message.data)
  if (name === 'reload') {
    stream.close()
    location.reload()
    return
  }
  for (const handler of handlers.get(name) ?? []) handler({ name, data })
})

let shown = false

export const Events = {
  On(name, handler) {
    if (!handlers.has(name)) handlers.set(name, new Set())
    handlers.get(name).add(handler)
    // The app shows the panel when asked; the preview shows it once the page is ready.
    if (name === 'panel-shown' && !shown) {
      shown = true
      setTimeout(() => handler({ name, data: 'bottom-right' }))
    }
    return () => handlers.get(name).delete(handler)
  },
}

export const System = {
  IsMac: () => true,
  // The bubble asks the window system to drag it; a browser page has no window to drag.
  invoke: () => Promise.resolve(),
}
