import { describe, expect, it } from 'vitest'
import type { ItemFinding, Section } from '../contract'
import { fieldEntryId, findingEntryIds, findingKey, itemEntryId, sectionEntryIds } from './identity'

const finding: ItemFinding = { source: 'lint', code: 'LINT038', severity: 'warning', message: 'Bare read covers every file.', path: '.agnostic-ai/settings/permissions.yaml', line: 3 }

describe('entry identity', () => {
  it('names an item by kind, name, and path, never by position', () => {
    expect(itemEntryId({ kind: 'mcp', name: 'github', path: '.agnostic-ai/mcps/github.yaml' })).toBe('item:mcp:github:.agnostic-ai/mcps/github.yaml')
    expect(itemEntryId({ kind: 'mcp', name: 'github', path: '~/.agnostic-ai/mcps/github.yaml' })).not.toBe(itemEntryId({ kind: 'mcp', name: 'github', path: '.agnostic-ai/mcps/github.yaml' }))
    expect(itemEntryId({ kind: 'rule', name: 'a:b', path: 'c' })).not.toBe(itemEntryId({ kind: 'rule', name: 'a', path: 'b:c' }))
    expect(fieldEntryId({ key: 'default-model' })).toBe('field:default-model')
  })

  it('keeps a finding when only its line moves, and tells apart different problems', () => {
    const id = (f: ItemFinding, owner?: string) => findingEntryIds([f], owner)[0]
    expect(findingKey({ ...finding, line: 9 })).toBe(findingKey(finding))
    expect(id({ ...finding, line: 9 }, 'item:x')).toBe(id(finding, 'item:x'))
    expect(id({ ...finding, message: 'Another problem.' }, 'item:x')).not.toBe(id(finding, 'item:x'))
    expect(id(finding, 'item:y')).not.toBe(id(finding, 'item:x'))
    expect(id(finding)).toMatch(/^finding:LINT038:[0-9a-z]+$/)
  })

  it('ignores line numbers written in the message', () => {
    const at = (message: string) => findingKey({ ...finding, message })
    expect(at('line 4 links notes.md, which does not exist')).toBe(at('line 12 links notes.md, which does not exist'))
    expect(at('rules/a.md:3:7: bad glob')).toBe(at('rules/a.md:40:1: bad glob'))
    expect(at('line 4 links notes.md')).not.toBe(at('line 4 links other.md'))
  })

  it('tells apart identical findings on one owner by their order', () => {
    const ids = findingEntryIds([finding, { ...finding, line: 8 }, { ...finding, message: 'Other.' }], 'item:x')
    expect(new Set(ids).size).toBe(3)
    expect(ids[1]).toBe(`${ids[0]}-2`)
  })

  it('lists every entry a section can link to, whatever the order', () => {
    const section = {
      items: [{ kind: 'settings', name: 'permissions', path: 'p.yaml', findings: [finding] }],
      fields: [{ key: 'default-model' }],
      findings: [{ source: 'doctor', severity: 'error', message: 'Not checked.' }],
    } as unknown as Section
    const ids = sectionEntryIds(section)
    expect([...ids]).toEqual([
      'item:settings:permissions:p.yaml',
      ...findingEntryIds([finding], 'item:settings:permissions:p.yaml'),
      'field:default-model',
      ...findingEntryIds(section.findings),
    ])
    expect(sectionEntryIds({ ...section, items: [...section.items].reverse() })).toEqual(ids)
  })
})
