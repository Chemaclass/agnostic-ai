import { describe, expect, it } from 'vitest'
import { findingFixPrompt } from './finding-prompt'

describe('finding fix prompts', () => {
  it('includes the source, evidence, and suggested fix', () => {
    const prompt = findingFixPrompt({
      source: 'lint', code: 'LINT038', severity: 'warn',
      path: '.agnostic-ai/settings/permissions.yaml', line: 3,
      message: 'Bare read covers every file.',
      explanation: 'The selected tool accepts this permission.',
      fix: 'Scope read to the intended paths.',
    }, '/tmp/example')
    expect(prompt).toContain('Configuration root: /tmp/example')
    expect(prompt).toContain('lint (LINT038)')
    expect(prompt).toContain('.agnostic-ai/settings/permissions.yaml:3')
    expect(prompt).toContain('Bare read covers every file.')
    expect(prompt).toContain('The selected tool accepts this permission.')
    expect(prompt).toContain('Scope read to the intended paths.')
    expect(prompt).toContain('canonical specs, not generated files')
    expect(prompt).toContain('sync --check')
  })

  it('handles doctor findings with no path or suggested fix', () => {
    const prompt = findingFixPrompt({ source: 'doctor', severity: 'error', message: 'The selected tool is unavailable.' })
    expect(prompt).toContain('Check: doctor\n')
    expect(prompt).toContain('Severity: error')
    expect(prompt).toContain('The selected tool is unavailable.')
    expect(prompt).not.toContain('undefined')
    expect(prompt).not.toContain('Source:')
    expect(prompt).not.toContain('Suggested fix:')
  })
})
