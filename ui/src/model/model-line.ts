import type { OverviewView, SettingField } from '../contract'
import { generatedAnswer } from './field-resolution'

export interface ModelLine {
  text: string
  /** True when no model and no effort is set for the tool. */
  unset: boolean
  field: SettingField
}

function settingField(overview: OverviewView, key: string): SettingField | undefined {
  return overview.sections.find((s) => s.id === 'settings')?.fields?.find((f) => f.key === key)
}

function lowerFirst(s: string): string {
  return s.charAt(0).toLowerCase() + s.slice(1)
}

/** What sync writes as the default model and effort for one tool, from the Settings resolution. */
export function modelLine(overview: OverviewView | undefined, target: string): ModelLine | undefined {
  if (!overview) return undefined
  const model = settingField(overview, 'model')
  const effort = settingField(overview, 'effort')
  const m = model?.resolved?.find((r) => r.target === target)
  if (!model || !m) return undefined
  const e = effort?.resolved?.find((r) => r.target === target)
  const modelSet = !!m.value && m.checked && !m.dropped
  const effortSet = !!e?.value && e.checked && !e.dropped
  if (!modelSet && !effortSet) return { text: `No default model or reasoning effort is set for ${m.label}, so it uses its own defaults.`, unset: true, field: model }
  const parts = [modelSet ? `Model for ${m.label}: ${lowerFirst(generatedAnswer(m))}` : `No default model is set for ${m.label}.`]
  if (effortSet && e) parts.push(`Reasoning effort: ${lowerFirst(generatedAnswer(e))}`)
  return { text: parts.join(' '), unset: false, field: model }
}
