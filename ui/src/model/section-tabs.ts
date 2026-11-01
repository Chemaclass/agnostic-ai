import type { NativeServer, Section } from '../contract'
import { fieldEntryId, findingEntryIds, itemEntryId } from './identity'

export interface SectionTab {
  id: string
  label: string
  kinds: string[]
  extra?: 'native' | 'usage' | 'targets' | 'editor'
}

const contextTabs: SectionTab[] = [
  { id: 'usage', label: 'Context size', kinds: [], extra: 'usage' },
  { id: 'guidance', label: 'Guidance', kinds: ['instructions', 'review', 'ignore'] },
  { id: 'rules', label: 'Rules', kinds: ['rule'] },
  { id: 'agents', label: 'Agents', kinds: ['agent'] },
  { id: 'skills', label: 'Skills', kinds: ['skill'] },
]
const toolsTabs: SectionTab[] = [
  { id: 'servers', label: 'Configured servers', kinds: ['mcp'] },
  { id: 'native', label: 'Found in tools', kinds: [], extra: 'native' },
  { id: 'environment', label: 'Environments', kinds: ['environment'] },
]
const settingsTabs: SectionTab[] = [
  { id: 'builtins', label: 'Built-ins', kinds: ['builtin'] },
  { id: 'targets', label: 'Coding tools', kinds: [], extra: 'targets' },
  { id: 'preferences', label: 'Preferences', kinds: ['settings'] },
  { id: 'configuration', label: 'Config files', kinds: ['config'] },
  { id: 'editor', label: 'Editor', kinds: [], extra: 'editor' },
]

export function sectionTabs(section: Section): SectionTab[] {
  const configured = section.id === 'context' ? contextTabs : section.id === 'tools' ? toolsTabs : section.id === 'settings' ? settingsTabs : []
  if (configured.length === 0) return []
  const tabs = configured.filter((tab) => tab.extra || section.kinds.some((kind) => tab.kinds.includes(kind.kind)))
  const remaining = section.kinds.filter((kind) => !tabs.some((tab) => tab.kinds.includes(kind.kind)))
  return [...tabs, ...remaining.map((kind) => ({ id: kind.kind, label: kind.label, kinds: [kind.kind] }))]
}

export function entryTab(section: Section, tabs: SectionTab[], entry?: string): string | undefined {
  if (!entry) return undefined
  const item = section.items.find((item) => itemEntryId(item) === entry || findingEntryIds(item.findings, itemEntryId(item)).includes(entry))
  if (item) return tabs.find((tab) => tab.kinds.includes(item.kind))?.id
  if (section.fields?.some((field) => fieldEntryId(field) === entry)) return tabs.find((tab) => tab.kinds.includes('settings'))?.id
  return undefined
}

export function nativeSearchText(server: NativeServer): string[] {
  return [server.name, ...server.holders.flatMap((holder) => [holder.target, holder.label, holder.path, holder.scope, holder.transport ?? ''])]
}

/** The tab a section opens on when the address names none: Context size when it can load, else the first tab with content. */
export function landingTab(section: Section, tabs: SectionTab[], usageAvailable: boolean): string | undefined {
  const usage = tabs.find((tab) => tab.extra === 'usage')
  if (usage && usageAvailable) return usage.id
  const populated = tabs.find((tab) => tab.extra === 'native'
    ? (section.native?.length ?? 0) > 0
    : section.items.some((item) => tab.kinds.includes(item.kind)) || tab.kinds.includes('settings') && (section.fields?.some((field) => field.set.length > 0) ?? false))
  return populated?.id ?? tabs[0]?.id
}
