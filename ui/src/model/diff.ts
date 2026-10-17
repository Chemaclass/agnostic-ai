// Line diff for the proposal's FileDiff: the exact before and after of a
// spec file, shown as a unified diff the reader can copy and apply.

export interface DiffLine {
  kind: 'same' | 'add' | 'del'
  text: string
  oldNo?: number
  newNo?: number
}

export function splitLines(text: string): string[] {
  if (text === '') return []
  const lines = text.split('\n')
  if (lines[lines.length - 1] === '') lines.pop()
  return lines
}

/** Longest-common-subsequence diff; spec files are small, so O(n·m) is fine. */
export function diffLines(before: string, after: string): DiffLine[] {
  const a = splitLines(before)
  const b = splitLines(after)
  const n = a.length
  const m = b.length
  const lcs: Uint32Array[] = Array.from({ length: n + 1 }, () => new Uint32Array(m + 1))
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      lcs[i][j] = a[i] === b[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1])
    }
  }
  const out: DiffLine[] = []
  let i = 0
  let j = 0
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      out.push({ kind: 'same', text: a[i], oldNo: i + 1, newNo: j + 1 })
      i++
      j++
    } else if (lcs[i + 1][j] >= lcs[i][j + 1]) {
      out.push({ kind: 'del', text: a[i], oldNo: i + 1 })
      i++
    } else {
      out.push({ kind: 'add', text: b[j], newNo: j + 1 })
      j++
    }
  }
  while (i < n) out.push({ kind: 'del', text: a[i], oldNo: ++i })
  while (j < m) out.push({ kind: 'add', text: b[j], newNo: ++j })
  return out
}

/** A unified diff with `context` lines around each change, as `git apply` reads it. */
export function unifiedDiff(path: string, before: string, after: string, context = 3): string {
  const lines = diffLines(before, after)
  const changed = lines.map((line, index) => (line.kind === 'same' ? -1 : index)).filter((index) => index >= 0)
  const header = [`--- ${before === '' ? '/dev/null' : `a/${path}`}`, `+++ b/${path}`]
  if (changed.length === 0) return header.join('\n') + '\n'

  const ranges: Array<[number, number]> = []
  for (const index of changed) {
    const start = Math.max(0, index - context)
    const end = Math.min(lines.length - 1, index + context)
    const last = ranges[ranges.length - 1]
    if (last && start <= last[1] + 1) last[1] = Math.max(last[1], end)
    else ranges.push([start, end])
  }

  const out = [...header]
  for (const [start, end] of ranges) {
    const slice = lines.slice(start, end + 1)
    const oldCount = slice.filter((l) => l.kind !== 'add').length
    const newCount = slice.filter((l) => l.kind !== 'del').length
    out.push(`@@ -${hunkStart(lines, start, end, 'oldNo')},${oldCount} +${hunkStart(lines, start, end, 'newNo')},${newCount} @@`)
    for (const line of slice) out.push(`${line.kind === 'add' ? '+' : line.kind === 'del' ? '-' : ' '}${line.text}`)
  }
  return out.join('\n') + '\n'
}

// A hunk starts at its first line on each side. A side with no lines in
// the hunk reports the line before it (0 at the top), as diff does.
function hunkStart(lines: DiffLine[], start: number, end: number, key: 'oldNo' | 'newNo'): number {
  for (let k = start; k <= end; k++) if (lines[k][key] !== undefined) return lines[k][key]!
  for (let k = start - 1; k >= 0; k--) if (lines[k][key] !== undefined) return lines[k][key]!
  return 0
}
