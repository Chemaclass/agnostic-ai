/** Whole-number percent of the total; 0 when the total is empty. */
export function sharePercent(part: number, total: number): number {
  return total > 0 ? Math.round((part / total) * 100) : 0
}

export function signed(value: number): string {
  if (value === 0) return 'no change'
  return `${value > 0 ? '+' : '-'}${Math.abs(value).toLocaleString('en-US')}`
}

export function isBuiltin(source: string): boolean {
  return source.startsWith('builtin:')
}

/** "builtin:shared-memory-policy" reads as "Shared memory policy". */
export function builtinTitle(source: string): string {
  const name = source.slice('builtin:'.length).replace(/[-_]+/g, ' ').trim()
  return name.charAt(0).toUpperCase() + name.slice(1)
}
