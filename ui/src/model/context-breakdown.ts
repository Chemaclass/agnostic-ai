import type { ContextContribution, ContextLoad, ContextSize } from '../contract'

export interface KindTotal extends ContextSize {
  category: string
  sources: number
}

/** Whole-number percent of the total; 0 when the total is empty. */
export function sharePercent(part: number, total: number): number {
  return total > 0 ? Math.round((part / total) * 100) : 0
}

/** Totals per kind of source for one load, largest first. */
export function kindTotals(contributions: ContextContribution[], load: ContextLoad): KindTotal[] {
  const byKind = new Map<string, KindTotal>()
  for (const c of contributions) {
    if (c.load !== load) continue
    const total = byKind.get(c.category) ?? { category: c.category, sources: 0, words: 0, bytes: 0 }
    total.sources += 1
    total.words += c.words
    total.bytes += c.bytes
    byKind.set(c.category, total)
  }
  return [...byKind.values()].sort((a, b) => b.words - a.words || a.category.localeCompare(b.category))
}

export function signed(value: number): string {
  if (value === 0) return 'no change'
  return `${value > 0 ? '+' : '-'}${Math.abs(value).toLocaleString('en-US')}`
}

export function isBuiltin(source: string): boolean {
  return source.startsWith('builtin:')
}
