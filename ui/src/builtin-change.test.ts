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
    { undoAvailable: true }, { targets: [1] }, { findings: 'warning' }, { status: 'applied-anyway' },
  ])('refuses an unsafe response %j', (change) => {
    expect(() => parseBuiltinChange({ ...proposal, ...change })).toThrow('invalid change')
  })

  it('keeps the previous successful undo while a new change is proposed', () => {
    expect(parseBuiltinChange({ ...proposal, undoAvailable: true, undoId: 'previous' }).undoId).toBe('previous')
  })
})
