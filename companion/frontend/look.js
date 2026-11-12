// How the pages look, from the appearance the shell answers. No DOM here.

/** `off` only when the person turned animation off; an unread appearance moves. */
export function motionOf(appearance) {
  return appearance?.animate === false ? 'off' : 'on'
}

/** `on` only when the person turned dimming on; an unread appearance does not dim. */
export function dimOf(appearance) {
  return appearance?.dimWhenIdle === true ? 'on' : 'off'
}

const PRESENCES = new Set(['shown', 'hidden', 'faded'])

/** How the companion sits on the screen: `shown`, `hidden`, or `faded` when it came back on its own. An unread appearance is shown. */
export function presenceOf(appearance) {
  return PRESENCES.has(appearance?.presence) ? appearance.presence : 'shown'
}
