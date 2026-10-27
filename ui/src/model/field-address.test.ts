import { describe, expect, it } from 'vitest'
import type { TargetValue, Value } from '../contract'
import { addressKey, configOverride, originLabel, personalSetsWhole, personalValueAt, valueAt } from './field-address'

const text = (key: string, value: string): Value => ({ key, type: 'text', value })
const tool = (over: Partial<TargetValue>): TargetValue => ({ target: 'codex', label: 'Codex', checked: true, others: [], ...over })

describe('field address', () => {
  it('names one tool as a dotted key', () => {
    expect(addressKey('model')).toBe('model')
    expect(addressKey('model', 'codex')).toBe('model.codex')
  })

  it('reads the value for every tool from a scalar or the map default', () => {
    expect(valueAt([text('model', 'sonnet')], 'model')).toBe('sonnet')
    expect(valueAt([text('model.default', 'sonnet'), text('model.codex', 'gpt-5')], 'model')).toBe('sonnet')
    expect(valueAt([text('model.codex', 'gpt-5')], 'model')).toBeUndefined()
  })

  it('reads one tool only from its own entry', () => {
    expect(valueAt([text('model.default', 'sonnet'), text('model.codex', 'gpt-5')], 'model', 'codex')).toBe('gpt-5')
    expect(valueAt([text('model', 'sonnet')], 'model', 'codex')).toBeUndefined()
  })

  it('counts only the values a personal file sets itself', () => {
    const merged = [text('model.default', 'sonnet'), text('model.claude', 'opus'), text('model.codex', 'gpt-5')]
    expect(personalValueAt(merged, ['model.codex'], 'model', 'codex')).toBe('gpt-5')
    expect(personalValueAt(merged, ['model.codex'], 'model', 'claude')).toBeUndefined()
    expect(personalValueAt(merged, ['model.codex'], 'model')).toBeUndefined()
    expect(personalValueAt(merged, ['model.default', 'model.codex'], 'model')).toBe('sonnet')
    expect(personalValueAt([text('model', 'haiku')], ['model'], 'model')).toBe('haiku')
    expect(personalValueAt([text('model', 'haiku')], undefined, 'model')).toBe('haiku')
  })

  it('knows when a personal value for every tool hides each tool entry', () => {
    expect(personalSetsWhole(['model'], 'model')).toBe(true)
    expect(personalSetsWhole(['model.codex'], 'model')).toBe(false)
    expect(personalSetsWhole(undefined, 'model')).toBe(false)
  })

  it('labels where the value comes from, and the config key that replaces it', () => {
    expect(originLabel(tool({ value: 'gpt-5', from: { spec: 'defaults', path: 'a.yaml', local: true, perTool: true } }))).toBe('personal · codex')
    expect(originLabel(tool({ value: 'sonnet', from: { spec: 'defaults', path: 'a.yaml' } }))).toBe('shared · all tools')
    const replaced = tool({ value: 'o3', from: { path: 'outputs.codex.config.model' } })
    expect(originLabel(replaced)).toBeUndefined()
    expect(configOverride(replaced)).toBe('outputs.codex.config.model')
    expect(configOverride(tool({ from: { spec: 'defaults', path: 'a.yaml' } }))).toBeUndefined()
  })
})
