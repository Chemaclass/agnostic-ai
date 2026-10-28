import { describe, expect, it } from 'vitest'
import type { TargetValue } from '../contract'
import { generatedAnswer, installedAnswer, valueGroups } from './field-resolution'

const tool = (over: Partial<TargetValue>): TargetValue => ({ target: 'claude', label: 'Claude Code', checked: true, others: [], ...over })

describe('field resolution answers', () => {
  it('groups tools by the value sync writes, counting a dropped value as none', () => {
    const groups = valueGroups([
      tool({ value: 'sonnet' }),
      tool({ target: 'codex', label: 'Codex', value: 'gpt-5.5' }),
      tool({ target: 'cursor', label: 'Cursor', value: 'sonnet', dropped: 'no model key' }),
      tool({ target: 'gemini', label: 'Gemini CLI' }),
    ])
    expect(groups).toEqual([
      { value: 'sonnet', labels: ['Claude Code'] },
      { value: 'gpt-5.5', labels: ['Codex'] },
      { value: undefined, labels: ['Cursor', 'Gemini CLI'] },
    ])
  })

  it('names the tier and alias, and never states a runtime default', () => {
    expect(generatedAnswer(tool({ value: 'gpt-6.1-sol', alias: 'sol', tier: 'deep' }))).toBe('Sync writes gpt-6.1-sol (resolved from sol and from model tier deep).')
    expect(generatedAnswer(tool({}))).toBe('No settings spec sets a value for Claude Code, so sync writes none and Claude Code keeps its own default.')
    expect(generatedAnswer(tool({ value: 'high', dropped: 'the settings file has no repository effort key' }))).toBe('Sync writes no value: the settings file has no repository effort key. Claude Code keeps its own default.')
    expect(generatedAnswer(tool({ value: 'opus', checked: false }))).toContain('is unknown')
  })

  it('reports installed files from the generated-file status only', () => {
    expect(installedAnswer('Codex', 'in-sync')).toBe('Installed Codex files match what sync would write.')
    expect(installedAnswer('Codex', 'drifted')).toContain('run sync')
    expect(installedAnswer('Codex', undefined)).toBe('Installed Codex files were not checked.')
  })

  it('does not claim an unchecked value is written or absent', () => {
    const unchecked = tool({ target: 'cursor', label: 'Cursor', value: 'high', checked: false })
    expect(valueGroups([unchecked, tool({})])).toEqual([
      { unchecked: true, labels: ['Cursor'] },
      { value: undefined, labels: ['Claude Code'] },
    ])
    expect(generatedAnswer(unchecked)).toBe('The configured value for Cursor is high, but sync was not rendered here, so whether its files take this value is unknown.')
    expect(generatedAnswer(tool({ checked: false }))).toBe('No settings spec sets a value for Claude Code. Sync was not rendered here, so what its files take is unknown.')
  })
})
