import { describe, expect, it } from 'vitest'
import type { Stage } from '../contract'
import { emptyHookDraft, hookDraftProblem, hookRequest, hookStages, takesMatch } from './hook-form'

const stage = (id: string): Stage => ({ id, label: id, description: '', targets: [] })

describe('hook form', () => {
  it('offers only the portable stages the page shows', () => {
    expect(hookStages([stage('session-start'), stage('after-edit'), stage('native:BeforeAgent')]).map((s) => s.id)).toEqual(['session-start', 'after-edit'])
  })

  it('takes a tool match only before and after a tool', () => {
    expect(takesMatch('before-tool')).toBe(true)
    expect(takesMatch('after-tool')).toBe(true)
    expect(takesMatch('after-edit')).toBe(false)
    expect(takesMatch('stop')).toBe(false)
  })

  it.each([
    ['', 'Enter a name.'],
    ['Format', 'Use lowercase letters, digits, and hyphens, such as format-on-save.'],
    ['../escape', 'Use lowercase letters, digits, and hyphens, such as format-on-save.'],
    ['-lead', 'Use lowercase letters, digits, and hyphens, such as format-on-save.'],
  ])('refuses the name %j', (name, problem) => {
    expect(hookDraftProblem({ ...emptyHookDraft('after-edit'), name, command: 'npm run format' })).toBe(problem)
  })

  it('needs a command and a whole-second timeout', () => {
    const draft = { ...emptyHookDraft('after-edit'), name: 'format-on-save' }
    expect(hookDraftProblem(draft)).toBe('Enter the command to run.')
    expect(hookDraftProblem({ ...draft, command: 'x', timeout: '0' })).toBe('Timeout must be a whole number of seconds above zero.')
    expect(hookDraftProblem({ ...draft, command: 'x', timeout: '1.5' })).toBe('Timeout must be a whole number of seconds above zero.')
    expect(hookDraftProblem({ ...draft, command: 'x', timeout: '30' })).toBeUndefined()
  })

  it('sends only the fields given, and drops a match the stage cannot take', () => {
    const draft = { ...emptyHookDraft('after-edit'), name: 'format-on-save', command: 'npm run format -- $FILE', match: 'shell' }
    expect(hookRequest(draft, 'shared', 'preview')).toEqual({ action: 'preview', audience: 'shared', name: 'format-on-save', on: 'after-edit', command: 'npm run format -- $FILE' })
    const guard = { ...draft, name: 'no-push', on: 'before-tool', timeout: '10', description: '  Block pushes.  ' }
    expect(hookRequest(guard, 'personal', 'apply', 'abc')).toEqual({ action: 'apply', audience: 'personal', name: 'no-push', on: 'before-tool', match: 'shell', command: 'npm run format -- $FILE', timeout: 10, description: 'Block pushes.', expected: 'abc' })
  })
})
