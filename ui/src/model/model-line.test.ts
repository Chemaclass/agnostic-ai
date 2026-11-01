import { describe, expect, it } from 'vitest'
import type { OverviewView, TargetValue } from '../contract'
import { modelLine } from './model-line'

const resolved = (over: Partial<TargetValue>): TargetValue => ({ target: 'claude', label: 'Claude Code', checked: true, others: [], ...over })
const overview = (model: TargetValue[], effort: TargetValue[]): OverviewView => ({
  schemaVersion: '1', snapshot: {} as OverviewView['snapshot'], targets: [], tools: { source: 'default', items: [] },
  sections: [{ id: 'settings', label: 'Settings', question: '', order: 5, kinds: [], items: [], findings: [], support: [], fields: [
    { key: 'model', label: 'Default model', description: '', set: [], resolved: model },
    { key: 'effort', label: 'Reasoning effort', description: '', set: [], resolved: effort },
  ] }],
})
const origin = { spec: 'defaults', path: '.agnostic-ai/settings/defaults.yaml' }

describe('model line', () => {
  it('says what sync writes for the tool, or that nothing is set', () => {
    expect(modelLine(overview([resolved({})], [resolved({})]), 'claude')).toMatchObject({ unset: true, text: 'No default model or reasoning effort is set for Claude Code, so it keeps its own default.' })
    expect(modelLine(overview([resolved({ value: 'sonnet', alias: 'claude-sonnet-4-5' })], [resolved({ value: 'high' })]), 'claude')?.text).toBe('Model for Claude Code: sync writes sonnet (resolved from claude-sonnet-4-5). Reasoning effort: sync writes high.')
    expect(modelLine(overview([resolved({ value: 'opus', tier: 'strong' })], [resolved({})]), 'claude')?.text).toBe('Model for Claude Code: sync writes opus (from model tier strong).')
    expect(modelLine(overview([resolved({})], [resolved({ value: 'high' })]), 'claude')?.text).toBe('No default model is set for Claude Code. Reasoning effort: sync writes high.')
    expect(modelLine(overview([resolved({})], [resolved({})]), 'codex')).toBeUndefined()
    expect(modelLine(undefined, 'claude')).toBeUndefined()
  })

  it('never says nothing is set when a spec sets a value that is unverified, dropped or native', () => {
    const unverified = modelLine(overview([resolved({ value: 'sonnet', checked: false })], [resolved({})]), 'claude')
    expect(unverified?.unset).toBe(false)
    expect(unverified?.text).toBe('Model for Claude Code: the configured value for Claude Code is sonnet, but sync was not rendered here, so whether its files take this value is unknown.')
    const dropped = modelLine(overview([resolved({ dropped: 'no place for it' })], [resolved({})]), 'claude')
    expect(dropped?.unset).toBe(false)
    expect(dropped?.text).toContain('sync writes no value: no place for it')
    const both = modelLine(overview([resolved({ value: 'sonnet', native: [{ ...origin, value: 'opus' }] })], [resolved({})]), 'claude')
    expect(both?.unset).toBe(false)
    expect(both?.text).toBe('Model for Claude Code: a spec sets sonnet, and x-claude.model is also set to opus. Sync writes that native key as is and it wins, so check the generated file for the value Claude Code reads.')
    expect(both?.text).not.toContain('sync writes sonnet')
    const native = modelLine(overview([resolved({ native: [origin] })], [resolved({})]), 'claude')
    expect(native?.unset).toBe(false)
    expect(native?.text).toBe('Model for Claude Code: a spec sets x-claude.model, which sync writes as is.')
  })
})
