import { describe, expect, it } from 'vitest'
import { copyablePath, effectiveEditor, editorUrl, parseEditor, templateUrl, validateTemplate } from './editor'

describe('editor file links', () => {
  it.each([
    ['cursor', 'cursor:'],
    ['vscode', 'vscode:'],
    ['vscode-insiders', 'vscode-insiders:'],
    ['vscodium', 'vscodium:'],
    ['zed', 'zed:'],
  ] as const)('uses the %s scheme', (editor, protocol) => {
    expect(new URL(editorUrl(editor, '/tmp/a.md')).protocol).toBe(protocol)
  })

  it.each([
    '/tmp/rules with spaces.md',
    '/tmp/ågent#review?.yaml',
    '/tmp/100% done & more;=.md',
    '/C:/Users/Someone/project/#local?.md',
  ])('keeps the complete file path %s', (path) => {
    for (const editor of ['cursor', 'vscode', 'vscode-insiders', 'vscodium', 'zed'] as const) {
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
    expect(parseEditor('zed')).toBe('zed')
    expect(parseEditor('custom')).toBe('custom')
  })
})

describe('custom link templates', () => {
  it.each([
    ['', 'Enter a link template.'],
    [`windsurf://file{path}${'a'.repeat(512)}`, 'Keep the template to 512 characters or fewer.'],
    ['windsurf://file{path} :{line}', 'The template cannot contain spaces or control characters.'],
    ['windsurf://file{path}\u0007x','The template cannot contain spaces or control characters.'],
    ['file{path}', 'Start with a scheme, such as windsurf://file{path}.'],
    ['javascript:alert({path})', 'That scheme cannot open an editor.'],
    ['DATA:text/html,{path}', 'That scheme cannot open an editor.'],
    ['vbscript:{path}', 'That scheme cannot open an editor.'],
    ['file://{path}', 'That scheme cannot open an editor.'],
    ['blob:{path}', 'That scheme cannot open an editor.'],
    ['about:{path}', 'That scheme cannot open an editor.'],
    ['filesystem:{path}', 'That scheme cannot open an editor.'],
    ['view-source:{path}', 'That scheme cannot open an editor.'],
    ['intent:{path}', 'That scheme cannot open an editor.'],
    ['ms-msdt:{path}', 'That scheme cannot open an editor.'],
    ['search-ms:{path}', 'That scheme cannot open an editor.'],
    ['windsurf://file/tmp/a.md', 'Include {path} where the file path goes.'],
    ['windsurf://file{path}:{col}', 'Only {path} and {line} are replaced.'],
  ])('refuses %j', (template, message) => {
    expect(validateTemplate(template)).toBe(message)
  })

  it.each([
    'windsurf://file{path}:{line}',
    'kiro://file{path}',
    'http://localhost:8091/?file={path}&line={line}',
  ])('accepts %s', (template) => {
    expect(validateTemplate(template)).toBeUndefined()
  })

  it('encodes the path as the built-in editors do', () => {
    expect(templateUrl('windsurf://file{path}:{line}', '/tmp/rule #1.md', 12)).toBe('windsurf://file/tmp/rule%20%231.md:12')
  })

  it('drops {line} and the colon before it when no line is known', () => {
    expect(templateUrl('windsurf://file{path}:{line}', '/tmp/a.md')).toBe('windsurf://file/tmp/a.md')
    expect(templateUrl('http://localhost:8091/?file={path}&line={line}', '/tmp/a.md')).toBe('http://localhost:8091/?file=/tmp/a.md&line=')
    expect(templateUrl('windsurf://file{path}:{line}', '/tmp/a.md', 0)).toBe('windsurf://file/tmp/a.md')
  })

  it('accepts a template of exactly 512 characters', () => {
    expect(validateTemplate(`windsurf://file{path}${'a'.repeat(512 - 'windsurf://file{path}'.length)}`)).toBeUndefined()
  })

  it('refuses a template of 513 characters', () => {
    expect(validateTemplate(`windsurf://file{path}${'a'.repeat(513 - 'windsurf://file{path}'.length)}`)).toBe('Keep the template to 512 characters or fewer.')
  })

  it('encodes characters that would end a query value', () => {
    const template = 'http://localhost:8091/?file={path}&line={line}'
    expect(templateUrl(template, '/r/a&line=1.md', 7)).toBe('http://localhost:8091/?file=/r/a%26line%3D1.md&line=7')
    expect(templateUrl(template, '/r/c++;x.md', 7)).toBe('http://localhost:8091/?file=/r/c%2B%2B%3Bx.md&line=7')
    expect(new URL(templateUrl(template, '/r/a&line=1.md', 7)).searchParams.get('file')).toBe('/r/a&line=1.md')
  })

  it('never removes the scheme colon when {line} is dropped', () => {
    expect(templateUrl('x:{line}{path}', '/r/a.md')).toBe('x:/r/a.md')
    expect(templateUrl('x:{path}:{line}', '/r/a.md')).toBe('x:/r/a.md')
  })

  it('fills {path} every time it appears', () => {
    expect(templateUrl('x://{path}?again={path}', '/a b')).toBe('x:///a%20b?again=/a%20b')
  })

  it('is used by a custom editor only while it is valid', () => {
    expect(editorUrl('custom', '/tmp/a.md', 3, 'windsurf://file{path}:{line}')).toBe('windsurf://file/tmp/a.md:3')
    expect(editorUrl('custom', '/tmp/a.md', 3, 'javascript:{path}')).toBe('cursor://file/tmp/a.md:3')
    expect(effectiveEditor('custom', 'javascript:{path}')).toBe('cursor')
    expect(effectiveEditor('custom', 'kiro://file{path}')).toBe('custom')
    expect(effectiveEditor('zed', 'javascript:{path}')).toBe('zed')
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
