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

export interface RequiredRelease {
  label: string
  state: 'met' | 'unmet' | 'unchecked'
  title: string
}

/** The config's `requires` beside the serving version, and whether that version meets it. */
export function requiredRelease(served: { global: boolean; version?: string; requires?: string; requiresMet?: boolean | null }): RequiredRelease | undefined {
  if (!served.requires) return undefined
  const needs = `${served.global ? 'The global setup' : 'This project'} needs agnostic-ai ${served.requires}`
  const running = served.version ? servingRelease(served.version).label : 'the running build'
  const label = `requires ${served.requires}`
  if (served.requiresMet === true) return { label, state: 'met', title: `${needs}; ${running} meets it.` }
  if (served.requiresMet === false) return { label, state: 'unmet', title: `${needs}, and ${running} is outside it. Run ${served.global ? 'agnostic-ai sync --global' : 'agnostic-ai sync'} to see the fix.` }
  return { label, state: 'unchecked', title: `${needs}. This build is not a release, so it is not checked.` }
}
