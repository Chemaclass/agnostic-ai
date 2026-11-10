// How the pages look, from the appearance the shell answers. No DOM here.

/** `off` only when the person turned animation off; an unread appearance moves. */
export function motionOf(appearance) {
  return appearance?.animate === false ? 'off' : 'on'
}
