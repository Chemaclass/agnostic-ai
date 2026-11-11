// How the pages look, from the appearance the shell answers. No DOM here.

/** `off` only when the person turned animation off; an unread appearance moves. */
export function motionOf(appearance) {
  return appearance?.animate === false ? 'off' : 'on'
}

/** `on` only when the person turned dimming on; an unread appearance does not dim. */
export function dimOf(appearance) {
  return appearance?.dimWhenIdle === true ? 'on' : 'off'
}

/** `on` only while the shell says the companion is paused; an unread appearance is awake. */
export function pausedOf(appearance) {
  return appearance?.paused === true ? 'on' : 'off'
}
