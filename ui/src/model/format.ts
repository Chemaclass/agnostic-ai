// Small formatting helpers shared by the components.

import type { Source } from '../contract'

/** "path:line" or "path". */
export function sourceRef(source: Pick<Source, 'path' | 'line'> | undefined): string {
  if (!source) return ''
  return source.line ? `${source.path}:${source.line}` : source.path
}

export function plural(count: number, one: string, many = `${one}s`): string {
  return `${count} ${count === 1 ? one : many}`
}

/** A readable local date and time; falls back to the raw value. */
export function formatTime(iso: string | undefined): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  return date.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

export function joinList(items: string[], conjunction = 'and'): string {
  if (items.length <= 1) return items.join('')
  if (items.length === 2) return `${items[0]} ${conjunction} ${items[1]}`
  return `${items.slice(0, -1).join(', ')}, ${conjunction} ${items[items.length - 1]}`
}
