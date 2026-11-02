// The agnostic-ai version that serves the page, for the sidebar badge.

/** Each published release has its notes on its GitHub release page. */
const RELEASE_NOTES_URL = 'https://github.com/Chemaclass/agnostic-ai/releases/tag/'

/** A published release as `--version` prints it, such as 0.91.0 or 0.92.0-rc.1. A checkout build prints `<branch>@<sha>` instead. */
const RELEASE_VERSION = /^v?(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$/

export function servingRelease(version: string): { kind: 'release'; label: string; notesUrl: string } | { kind: 'build'; label: string } {
  const release = RELEASE_VERSION.exec(version)
  if (!release) return { kind: 'build', label: version }
  const tag = `v${release[1]}`
  return { kind: 'release', label: tag, notesUrl: RELEASE_NOTES_URL + tag }
}
