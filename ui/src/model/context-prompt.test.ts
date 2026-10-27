import { describe, expect, it } from 'vitest'
import type { ContextOpportunity } from '../contract'
import fixture from '../fixtures/context.mock.json'
import { contextReviewPrompt } from './context-prompt'
import { normalizeContextView } from './normalize'

const opportunity: ContextOpportunity = {
  kind: 'duplicate-passage',
  target: 'codex',
  load: 'startup',
  passage: 'Run the tests before you push.',
  copies: 3,
  size: { words: 6, bytes: 30 },
  reduction: { words: 12, bytes: 60 },
  sources: [
    { source: '.agnostic-ai/AGNOSTIC_AI.md', category: 'entry-point layer' },
    { source: '.agnostic-ai/rules/testing.md', category: 'rules section' },
    { source: '.agnostic-ai/rules/style.md', category: '' },
  ],
  suggestion: 'Review the copies.',
}

describe('context review prompts', () => {
  it('names the target, load, passage, sources, size, and reduction', () => {
    const prompt = contextReviewPrompt(opportunity, '/tmp/project')
    expect(prompt).toContain('Target: codex')
    expect(prompt).toContain('Loads: at the start of every session (startup)')
    expect(prompt).toContain('Copies: 3')
    expect(prompt).toContain('Size of one copy: 6 words, 30 bytes')
    expect(prompt).toContain('Possible reduction: 12 words, 60 bytes from startup content')
    expect(prompt).toContain('Configuration root: /tmp/project')
    expect(prompt).toContain('- .agnostic-ai/AGNOSTIC_AI.md (entry-point layer)')
    expect(prompt).toContain('- .agnostic-ai/rules/testing.md (rules section)')
    expect(prompt).toContain('- .agnostic-ai/rules/style.md\n')
    expect(prompt).toContain('"""\nRun the tests before you push.\n"""')
  })

  it('asks for a proposal with tradeoffs and no edits before agreement', () => {
    const prompt = contextReviewPrompt(opportunity)
    expect(prompt).toContain('state the tradeoffs')
    expect(prompt).toContain('canonical specs under .agnostic-ai/, not generated files')
    expect(prompt).toContain('Do not apply any change until I agree.')
    expect(prompt).toContain('agnostic-ai sync, then agnostic-ai explain --context --target codex')
    expect(prompt).not.toContain('Configuration root')
    expect(prompt).not.toContain('undefined')
  })
})

describe('context fixture', () => {
  it('normalizes with one duplicate startup opportunity', () => {
    const view = normalizeContextView(fixture)
    expect(view.supported).toBe(true)
    expect(view.targets).toContain(view.target)
    expect(view.report?.opportunities).toHaveLength(1)
    expect(view.report?.opportunities[0]).toMatchObject({ kind: 'duplicate-passage', load: 'startup', copies: 2 })
    expect(view.report?.file).toBeUndefined()
    expect(view.report?.file_scope).toEqual({ words: 0, bytes: 0 })
    expect(view.report?.contributions.some((c) => c.load === 'file-scope')).toBe(false)
  })

  it('turns null lists and missing sizes into empty values', () => {
    const view = normalizeContextView({ schemaVersion: '1', supported: true, targets: null, target: 'claude', report: { contributions: null, opportunities: null } })
    expect(view.targets).toEqual([])
    expect(view.report?.startup).toEqual({ words: 0, bytes: 0 })
    expect(view.report?.contributions).toEqual([])
    expect(view.report?.opportunities).toEqual([])
  })

  it('keeps the reason when the scope is not supported', () => {
    const view = normalizeContextView({ schemaVersion: '1', supported: false, reason: 'Open a project to see it.', targets: [], target: '' })
    expect(view.supported).toBe(false)
    expect(view.reason).toBe('Open a project to see it.')
    expect(view.report).toBeUndefined()
  })
})
