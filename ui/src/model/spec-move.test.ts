import { describe, expect, it } from 'vitest'
import type { SectionItem } from '../contract'
import { canMoveSpec, destinationNote, parseSpecMoved, specMoveLabel, specMovedText, specMoveRequest } from './spec-move'

const item = (kind: string, layer: string): SectionItem => ({ kind, name: 'review-pr', label: 'review-pr', description: '', path: '', layer, values: [], findings: [] })

describe('spec move', () => {
  it.each(['rule', 'agent', 'skill', 'hook', 'mcp'])('offers a shared %s of the served level', (kind) => {
    expect(canMoveSpec(item(kind, 'project'), 'project')).toBe(true)
    expect(canMoveSpec(item(kind, 'global'), 'global')).toBe(true)
  })

  it.each([
    ['command', 'project'],
    ['settings', 'project'],
    ['instructions', 'project'],
    ['skill', 'builtin'],
    ['skill', 'pack:team'],
    ['skill', 'project-user'],
    ['skill', 'global-local'],
  ])('leaves a %s from layer %s alone', (kind, layer) => {
    expect(canMoveSpec(item(kind, layer), 'project')).toBe(false)
    expect(canMoveSpec(item(kind, layer), 'global')).toBe(false)
  })

  it('names the other level', () => {
    expect(specMoveLabel('project')).toBe('Copy to global')
    expect(specMoveLabel('global')).toBe('Copy to project')
  })

  it('builds the request from the served level to the other one', () => {
    expect(specMoveRequest(item('skill', 'project'), 'project', 'preview')).toEqual({ action: 'preview', kind: 'skill', name: 'review-pr', from: 'project', to: 'global', mode: 'copy', overwrite: false })
    expect(specMoveRequest(item('hook', 'global'), 'global', 'apply', 'move', true, 'abc')).toEqual({ action: 'apply', kind: 'hook', name: 'review-pr', from: 'global', to: 'project', mode: 'move', overwrite: true, expected: 'abc' })
  })

  it('says whether the destination holds a spec', () => {
    const preview = { source: 's', destination: 'd', files: [], syncCommands: [], expected: 'e' }
    expect(destinationNote({ ...preview, exists: false })).toBe('Nothing is there yet.')
    expect(destinationNote({ ...preview, exists: true })).toContain('Replace it')
  })

  it('keeps a saved notice only when it has every field', () => {
    const moved = { name: 'review-pr', mode: 'move', destination: '~/.agnostic-ai/skills/review-pr/SKILL.md', syncCommands: ['agnostic-ai sync', 'agnostic-ai sync --global'] }
    expect(parseSpecMoved(moved)).toEqual(moved)
    expect(specMovedText(parseSpecMoved(moved)!)).toBe('Moved review-pr to ~/.agnostic-ai/skills/review-pr/SKILL.md.')
    expect(parseSpecMoved({ ...moved, mode: 'link' })).toBeUndefined()
    expect(parseSpecMoved({ ...moved, syncCommands: [1] })).toBeUndefined()
    expect(parseSpecMoved(null)).toBeUndefined()
  })
})
