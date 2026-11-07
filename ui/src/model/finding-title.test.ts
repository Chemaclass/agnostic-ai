import { describe, expect, it } from 'vitest'
import { findingHasRawMessage, findingTitle } from './finding-title'

describe('findingTitle', () => {
  it('leads with a plain sentence for a known code', () => {
    const finding = { code: 'LINT038', message: 'permissions.allow: bare read now covers every file read (claude: Read)' }
    expect(findingTitle(finding)).toBe('A permission allows every use of a tool, not only the paths or commands you meant.')
    expect(findingHasRawMessage(finding)).toBe(true)
  })

  it('falls back to the raw message for an unknown or missing code', () => {
    expect(findingTitle({ code: 'LINT999', message: 'Raw text.' })).toBe('Raw text.')
    expect(findingTitle({ message: 'Raw text.' })).toBe('Raw text.')
    expect(findingHasRawMessage({ message: 'Raw text.' })).toBe(false)
  })

  it('keeps the raw message when the check could not run', () => {
    const message = 'cannot measure the always-loaded instructions: read error'
    expect(findingTitle({ code: 'LINT011', message })).toBe(message)
  })
})
