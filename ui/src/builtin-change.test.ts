import { describe, expect, it } from 'vitest'
import { parseBuiltinChange } from './builtin-change'

const proposal = {
  schemaVersion: 'builtin-change/v1', name: 'caveman', status: 'proposed', enabled: true, supported: true,
  id: 'proposal', config: 'agnostic-ai.yaml', targets: ['claude', 'codex'],
  changes: [{ path: 'agnostic-ai.yaml', action: 'update', diff: '+builtins: [caveman]' }], findings: [], undoAvailable: false,
}

describe('builtin change contract', () => {
  it('keeps the exact target and file preview', () => {
    expect(parseBuiltinChange(proposal)).toMatchObject({ targets: proposal.targets, changes: proposal.changes, id: proposal.id })
  })

  it.each([
    { schemaVersion: 'builtin-change/v2' }, { id: undefined }, { changes: [{}] },
    { name: '' }, { name: '  ' }, { name: undefined }, { undoName: '' }, { undoName: 'handoff' },
    { undoId: '' }, { undoId: 'orphan' }, { undoName: 1 },
    { undoAvailable: true }, { targets: [1] }, { findings: 'warning' }, { status: 'applied-anyway' },
  ])('refuses an unsafe response %j', (change) => {
    expect(() => parseBuiltinChange({ ...proposal, ...change })).toThrow('invalid change')
  })

  it('keeps the one tool a setting change targets', () => {
    const setting = { ...proposal, name: 'model', key: 'model', title: 'model for codex set to gpt-5' }
    expect(parseBuiltinChange({ ...setting, target: 'codex' })).toMatchObject({ key: 'model', target: 'codex' })
    expect(() => parseBuiltinChange({ ...setting, target: '' })).toThrow('invalid change')
    expect(() => parseBuiltinChange({ ...proposal, target: 'codex' })).toThrow('invalid change')
  })

  it('keeps the scope and a reset of the whole personal list', () => {
    expect(parseBuiltinChange({ ...proposal, name: undefined, enabled: false, reset: true, scope: 'local' }))
      .toMatchObject({ reset: true, scope: 'local' })
    expect(() => parseBuiltinChange({ ...proposal, scope: 'everyone' })).toThrow('invalid change')
  })

  it('keeps the selected built-in and the previous successful undo owner', () => {
    expect(parseBuiltinChange({ ...proposal, name: 'handoff', undoAvailable: true, undoId: 'previous', undoName: 'caveman' }))
      .toMatchObject({ name: 'handoff', undoId: 'previous', undoName: 'caveman' })
  })

  it('accepts a new server built-in without maintaining a client catalog', () => {
    expect(parseBuiltinChange({ ...proposal, name: 'new-builtin' }).name).toBe('new-builtin')
  })

  it('accepts idle state without a selected built-in', () => {
    expect(parseBuiltinChange({ ...proposal, status: 'idle', name: undefined, id: undefined }).status).toBe('idle')
  })

  it('accepts older undo metadata without an owner', () => {
    expect(parseBuiltinChange({ ...proposal, undoAvailable: true, undoId: 'previous' }).undoId).toBe('previous')
  })

  it('keeps a setting change and the title that names it for undo', () => {
    const setting = { ...proposal, name: 'effort', key: 'effort', title: 'effort set to high', undoAvailable: true, undoId: 'proposal', undoName: 'effort', undoTitle: 'effort set to high' }
    expect(parseBuiltinChange(setting)).toMatchObject({ key: 'effort', title: 'effort set to high', undoTitle: 'effort set to high' })
    for (const unsafe of [{ key: 'permissions' }, { key: 'model' }, { title: undefined }, { title: ' ' }, { undoTitle: '' }, { undoTitle: 'x', undoId: undefined, undoName: undefined, undoAvailable: false }]) {
      expect(() => parseBuiltinChange({ ...setting, ...unsafe })).toThrow('invalid change')
    }
  })
})
