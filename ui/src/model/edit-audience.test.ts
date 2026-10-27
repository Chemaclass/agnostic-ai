import { describe, expect, it } from 'vitest'
import { readAudience } from './edit-audience'

describe('saved edit audience', () => {
  it.each([
    ['personal', 'personal'],
    ['local', 'personal'],
    ['shared', 'shared'],
    ['team', 'shared'],
    [null, 'shared'],
    ['everyone', 'shared'],
  ] as const)('reads %s as %s', (saved, audience) => {
    expect(readAudience(saved)).toBe(audience)
  })
})
