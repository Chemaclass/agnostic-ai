import { describe, expect, it } from 'vitest'
import { copyablePath, editorUrl, parseEditor } from './editor'

describe('editor file links', () => {
  it.each([
    ['cursor', 'cursor:'],
    ['vscode', 'vscode:'],
  ] as const)('uses the %s scheme', (editor, protocol) => {
    expect(new URL(editorUrl(editor, '/tmp/a.md')).protocol).toBe(protocol)
  })

  it.each([
    '/tmp/rules with spaces.md',
    '/tmp/ågent#review?.yaml',
    '/tmp/100% done & more;=.md',
    '/C:/Users/Someone/project/#local?.md',
  ])('keeps the complete file path %s', (path) => {
    for (const editor of ['cursor', 'vscode'] as const) {
      const url = new URL(editorUrl(editor, path))
      expect(url.host).toBe('file')
      expect(decodeURIComponent(url.pathname)).toBe(path)
      expect(url.search).toBe('')
      expect(url.hash).toBe('')
    }
  })

  it.each([
    ['C:/Users/Some One/project/AGENTS.md', '/C:/Users/Some%20One/project/AGENTS.md'],
    ['C:\\Users\\Some One\\project\\AGENTS.md', '/C:/Users/Some%20One/project/AGENTS.md'],
  ])('opens the Windows path %s from its drive', (path, pathname) => {
    expect(new URL(editorUrl('vscode', path)).pathname).toBe(pathname)
  })

  it('opens at the line when one is known, and the file when it is not', () => {
    expect(editorUrl('vscode', '/tmp/rule #1.md', 12)).toBe('vscode://file/tmp/rule%20%231.md:12')
    expect(editorUrl('cursor', '/tmp/rule.md', 3)).toBe('cursor://file/tmp/rule.md:3')
    expect(editorUrl('vscode', '/tmp/rule.md')).toBe('vscode://file/tmp/rule.md')
    expect(editorUrl('vscode', '/tmp/rule.md', 0)).toBe('vscode://file/tmp/rule.md')
  })

  it('keeps Cursor until another known editor is chosen', () => {
    expect(parseEditor(null)).toBe('cursor')
    expect(parseEditor('emacs')).toBe('cursor')
    expect(parseEditor('vscode')).toBe('vscode')
  })
})

describe('copied paths', () => {
  it.each([
    ['/home/someone/project/rules with spaces.md', '/home/someone/project/rules with spaces.md'],
    ['/tmp/ågent#review?.yaml', '/tmp/ågent#review?.yaml'],
    ['C:/Users/Someone/project/AGENTS.md', 'C:\\Users\\Someone\\project\\AGENTS.md'],
    ['C:\\Users\\Someone\\AGENTS.md', 'C:\\Users\\Someone\\AGENTS.md'],
    ['//server/share/AGENTS.md', '\\\\server\\share\\AGENTS.md'],
  ])('copies %s as %s', (path, copied) => {
    expect(copyablePath(path)).toBe(copied)
  })
})
