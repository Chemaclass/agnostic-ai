// Stable identities for overview entries, built from what each entry is
// rather than where it sits in a list, so links survive reloads,
// filtering, and reordering. A finding's identity leaves out line numbers,
// in its fields or its message: an edit above it moves the line without
// changing the problem. Identical findings on one owner are told apart by
// their order among themselves.

import type { ItemFinding, Section, SectionItem, SettingField } from '../contract'

const part = (text: string) => text.replace(/%/g, '%25').replace(/:/g, '%3A')

export function itemEntryId(item: Pick<SectionItem, 'kind' | 'name' | 'path'>): string {
  return `item:${part(item.kind)}:${part(item.name)}:${part(item.path)}`
}

export function fieldEntryId(field: Pick<SettingField, 'key'>): string {
  return `field:${field.key}`
}

export function findingKey(finding: ItemFinding): string {
  const message = finding.message.replace(/\bline \d+/gi, 'line').replace(/(\S):\d+(?::\d+)?\b/g, '$1')
  return [finding.source, finding.code ?? '', finding.path ?? '', message].join('\u0000')
}

/** Link ids for the findings one owner carries, in order; the owner is an item, or none for section findings. */
export function findingEntryIds(findings: ItemFinding[], owner?: string): string[] {
  const seen = new Map<string, number>()
  return findings.map((finding) => {
    const key = findingKey(finding)
    const repeat = seen.get(key) ?? 0
    seen.set(key, repeat + 1)
    return `finding:${finding.code || finding.source}:${digest(`${owner ?? ''}\u0000${key}`)}${repeat ? `-${repeat + 1}` : ''}`
  })
}

export function sectionEntryIds(section: Section): Set<string> {
  const ids = new Set<string>()
  for (const item of section.items) {
    const owner = itemEntryId(item)
    ids.add(owner)
    for (const id of findingEntryIds(item.findings, owner)) ids.add(id)
  }
  for (const field of section.fields ?? []) ids.add(fieldEntryId(field))
  for (const id of findingEntryIds(section.findings)) ids.add(id)
  return ids
}

// FNV-1a, 32-bit: short, deterministic, and enough to tell findings apart within one overview.
function digest(text: string): string {
  let hash = 0x811c9dc5
  for (let i = 0; i < text.length; i++) {
    hash ^= text.charCodeAt(i)
    hash = Math.imul(hash, 0x01000193)
  }
  return (hash >>> 0).toString(36)
}
