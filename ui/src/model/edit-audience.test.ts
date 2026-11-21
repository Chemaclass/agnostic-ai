import { describe, expect, it } from 'vitest'
import { audienceDescription, audienceLabel, layerLabel, levelName, specLevel, readAudience } from './edit-audience'

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

describe('audience labels', () => {
  it('names the level of the served setup', () => {
    expect(levelName(false)).toBe('project')
    expect(levelName(true)).toBe('user')
  })

  it('keeps the API name of the user level for requests', () => {
    expect(specLevel(false)).toBe('project')
    expect(specLevel(true)).toBe('global')
  })

  it.each([
    ['shared', false, 'Project'],
    ['shared', true, 'User'],
    ['personal', false, 'Local'],
    ['personal', true, 'Local'],
  ] as const)('shows %s as %s in global=%s', (audience, userLevel, label) => {
    expect(audienceLabel(audience, userLevel)).toBe(label)
  })

  it('says whether Git tracks the files', () => {
    expect(audienceDescription('shared', false)).toBe('Project (committed)')
    expect(audienceDescription('shared', true)).toBe('User (every project)')
    expect(audienceDescription('personal', false)).toBe('Local (not committed)')
    expect(audienceDescription('personal', true)).toBe('Local (not committed)')
  })

  it.each([
    ['local', 'local'],
    ['user-local', 'local'],
    ['project', 'project'],
    ['user', 'user'],
    ['builtin', 'builtin'],
  ])('shows layer %s as %s', (layer, label) => {
    expect(layerLabel(layer)).toBe(label)
  })
})
