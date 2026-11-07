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

  it('writes the full prompt in a fixed order', () => {
    const prompt = findingFixPrompt({
      source: 'lint', code: 'LINT038', severity: 'warn',
      path: '.agnostic-ai/settings/permissions.yaml', line: 3,
      message: 'Bare read covers every file.',
      explanation: 'The selected tool accepts this permission.',
      fix: 'Scope read to the intended paths.',
    }, '/tmp/example')
    expect(prompt).toBe([
      'Fix this agnostic-ai finding in the selected project or user configuration:',
      '',
      'Check: lint (LINT038)',
      'Severity: warn',
      'Configuration root: /tmp/example',
      'Source: .agnostic-ai/settings/permissions.yaml:3',
      'Finding: Bare read covers every file.',
      'Explanation: The selected tool accepts this permission.',
      'Suggested fix: Scope read to the intended paths.',
      '',
      'Inspect the source and confirm the cause before editing. Change the canonical specs, not generated files. Preserve unrelated content and tool behavior. Keep the same project or global scope. Do not disable the check or raise its budget to hide the finding.',
      '',
      'Run the relevant agnostic-ai checks after the change. If generated files need updating, review sync --plan, then run sync and sync --check. Report what changed and any remaining findings.',
    ].join('\n'))
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

  it('points a global finding at the global setup, not the project', () => {
    const prompt = findingFixPrompt({
      source: 'lint', code: 'LINT007', severity: 'warn',
      path: 'agents/helper.md', line: 4, openPath: '/home/u/.agnostic-ai/agents/helper.md',
      message: 'Unknown key modl.', layer: 'global',
    }, '/tmp/project')
    expect(prompt).toContain('Layer: global setup, which reaches every project; edit it there, not in this project')
    expect(prompt).toContain('Source: /home/u/.agnostic-ai/agents/helper.md:4')
    expect(prompt).not.toContain('Source: agents/helper.md')
  })
})
