import type { ItemFinding, OverviewView, SectionItem, SettingField, Value } from '../contract'
import { fieldEntryId, findingEntryIds, itemEntryId } from './identity'
import type { ViewId } from './url'

export interface SearchResult {
  view: ViewId
  entry: string
  kind: 'item' | 'field' | 'finding'
  title: string
  context: string
  detail?: string
}

interface IndexedResult extends SearchResult {
  text: string
}

export type SearchIndex = IndexedResult[]

function valueText(values: Value[]): string[] {
  return values.flatMap((value) => [value.key, value.value ?? '', ...(value.list ?? [])])
}

export function itemSearchText(item: SectionItem): (string | undefined)[] {
  return [
    item.name, item.label, item.path, item.description, ...valueText(item.values),
    ...(item.tools?.tools.flatMap((tool) => [tool.name, tool.title, tool.description]) ?? []),
  ]
}

export function fieldSearchText(field: SettingField): (string | undefined)[] {
  return [
    field.key, field.label, field.description,
    ...field.set.flatMap((source) => [source.spec, source.path, ...valueText(source.values)]),
  ]
}

function findingSearchText(finding: ItemFinding): (string | undefined)[] {
  return [finding.message, finding.code, finding.source, finding.path, finding.fix]
}

function severityLabel(finding: ItemFinding): string {
  return finding.severity === 'error' ? 'Error' : 'Warning'
}

function indexed(result: SearchResult, parts: (string | undefined)[]): IndexedResult {
  return { ...result, text: parts.filter(Boolean).join('\n').toLowerCase() }
}

export function buildSearchIndex(overview: OverviewView): SearchIndex {
  const index: SearchIndex = []
  const sections = [...overview.sections].sort((a, b) => a.order - b.order)
  for (const section of sections) {
    const view = section.id as ViewId
    const kindLabel = (kind: string) => section.kinds.find((entry) => entry.kind === kind)?.label ?? kind
    const finding = (f: ItemFinding, entry: string, where: string) => indexed({
      view, kind: 'finding', entry, title: f.message,
      context: `${section.label} · ${severityLabel(f)} ${where}`,
      detail: [f.code, f.path].filter(Boolean).join(' · ') || undefined,
    }, findingSearchText(f))
    for (const item of section.items) {
      const owner = itemEntryId(item)
      index.push(indexed({ view, kind: 'item', entry: owner, title: item.label || item.name, context: `${section.label} · ${kindLabel(item.kind)}`, detail: item.path }, itemSearchText(item)))
      const ids = findingEntryIds(item.findings, owner)
      item.findings.forEach((f, i) => index.push(finding(f, ids[i], `on ${item.label || item.name}`)))
    }
    for (const field of section.fields ?? []) {
      index.push(indexed({ view, kind: 'field', entry: fieldEntryId(field), title: field.label, context: `${section.label} · Setting${field.set.length ? '' : ', not set'}`, detail: field.key }, fieldSearchText(field)))
    }
    const ids = findingEntryIds(section.findings)
    section.findings.forEach((f, i) => index.push(finding(f, ids[i], `from ${f.source}`)))
  }
  return index
}

/** Every word must appear; results whose title matches come first, in overview order otherwise. */
export function searchOverview(index: SearchIndex, query: string): SearchResult[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (!words.length) return []
  const rank = (result: IndexedResult) => {
    const title = result.title.toLowerCase()
    const phrase = words.join(' ')
    return title === phrase ? 0 : title.startsWith(phrase) ? 1 : title.includes(phrase) ? 2 : 3
  }
  return index
    .filter((result) => words.every((word) => result.text.includes(word)))
    .map((result, order) => ({ result, order, rank: rank(result) }))
    .sort((a, b) => a.rank - b.rank || a.order - b.order)
    .map(({ result: { text: _text, ...result } }) => result)
}
