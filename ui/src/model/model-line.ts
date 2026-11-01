import type { OverviewView, SettingField, TargetValue } from '../contract'
import { generatedAnswer } from './field-resolution'

export interface ModelLine {
  text: string
  /** True when no spec sets a model or effort for the tool. */
  unset: boolean
  field: SettingField
}

function settingField(overview: OverviewView, key: string): SettingField | undefined {
  return overview.sections.find((s) => s.id === 'settings')?.fields?.find((f) => f.key === key)
}

function lowerFirst(s: string): string {
  return s.charAt(0).toLowerCase() + s.slice(1)
}

/** What the specs and sync say for one key, or nothing when no spec sets it. */
function describe(r: TargetValue | undefined, key: string): string | undefined {
  if (!r) return undefined
  if (r.value && r.native?.length) {
    const native = r.native[0].value
    return `a spec sets ${r.value}, and x-${r.target}.${key}${native ? ` is also set to ${native}` : ' is also set'}. Sync writes that native key as is and it wins, so check the generated file for the value ${r.label} reads.`
  }
  if (r.value || r.dropped) return lowerFirst(generatedAnswer(r))
  if (r.native?.length) return `a spec sets x-${r.target}.${key}, which sync writes as is.`
  return undefined
}

/** What sync writes as the default model and effort for one tool, from the Settings resolution. */
export function modelLine(overview: OverviewView | undefined, target: string): ModelLine | undefined {
  if (!overview) return undefined
  const model = settingField(overview, 'model')
  const effort = settingField(overview, 'effort')
  const m = model?.resolved?.find((r) => r.target === target)
  if (!model || !m) return undefined
  const modelText = describe(m, 'model')
  const effortText = describe(effort?.resolved?.find((r) => r.target === target), 'effort')
  if (!modelText && !effortText) return { text: `No default model or reasoning effort is set for ${m.label}, so it keeps its own default.`, unset: true, field: model }
  const parts = [modelText ? `Model for ${m.label}: ${modelText}` : `No default model is set for ${m.label}.`]
  if (effortText) parts.push(`Reasoning effort: ${effortText}`)
  return { text: parts.join(' '), unset: false, field: model }
}
