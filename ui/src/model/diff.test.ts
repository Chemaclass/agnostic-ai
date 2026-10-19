import { describe, expect, it } from 'vitest'
import { diffLines, unifiedDiff } from './diff'

const before = ['permissions:', '  allow:', '    - read', '    - mcp:railway/deploy', '  ask:', '    - shell(git push:*)', ''].join('\n')
const after = ['permissions:', '  allow:', '    - read', '  ask:', '    - shell(git push:*)', '    - mcp:railway/deploy', ''].join('\n')

describe('file diff', () => {
  it('marks removed and added lines with their numbers', () => {
    const lines = diffLines(before, after)
    expect(lines.filter((l) => l.kind === 'del')).toEqual([{ kind: 'del', text: '    - mcp:railway/deploy', oldNo: 4 }])
    expect(lines.filter((l) => l.kind === 'add')).toEqual([{ kind: 'add', text: '    - mcp:railway/deploy', newNo: 6 }])
    expect(lines.filter((l) => l.kind === 'same')).toHaveLength(5)
  })

  it('writes a unified diff that git apply reads', () => {
    expect(unifiedDiff('.agnostic-ai/settings/permissions.yaml', before, after)).toBe(
      [
        '--- a/.agnostic-ai/settings/permissions.yaml',
        '+++ b/.agnostic-ai/settings/permissions.yaml',
        '@@ -1,6 +1,6 @@',
        ' permissions:',
        '   allow:',
        '     - read',
        '-    - mcp:railway/deploy',
        '   ask:',
        '     - shell(git push:*)',
        '+    - mcp:railway/deploy',
        '',
      ].join('\n'),
    )
  })

  it('handles a new file and an unchanged file', () => {
    expect(unifiedDiff('new.yaml', '', 'a\nb\n')).toBe('--- /dev/null\n+++ b/new.yaml\n@@ -0,0 +1,2 @@\n+a\n+b\n')
    expect(unifiedDiff('same.yaml', 'a\n', 'a\n')).toBe('--- a/same.yaml\n+++ b/same.yaml\n')
  })

  it('splits distant changes into separate hunks', () => {
    const many = Array.from({ length: 20 }, (_, i) => `line ${i + 1}`)
    const changed = [...many]
    changed[1] = 'changed 2'
    changed[18] = 'changed 19'
    const diff = unifiedDiff('f', many.join('\n') + '\n', changed.join('\n') + '\n')
    expect(diff.match(/^@@/gm)).toHaveLength(2)
    expect(diff).toContain('@@ -1,5 +1,5 @@')
    expect(diff).toContain('@@ -16,5 +16,5 @@')
  })
})
