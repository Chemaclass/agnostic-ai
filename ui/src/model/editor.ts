export type EditorId = 'cursor' | 'vscode' | 'vscode-insiders' | 'vscodium' | 'zed' | 'custom'

export const CUSTOM_EDITOR = 'custom'

export const EDITORS: { id: EditorId; label: string; scheme: string }[] = [
  { id: 'cursor', label: 'Cursor', scheme: 'cursor' },
  { id: 'vscode', label: 'VS Code', scheme: 'vscode' },
  { id: 'vscode-insiders', label: 'VS Code Insiders', scheme: 'vscode-insiders' },
  { id: 'vscodium', label: 'VSCodium', scheme: 'vscodium' },
  { id: 'zed', label: 'Zed', scheme: 'zed' },
  { id: CUSTOM_EDITOR, label: 'Custom link', scheme: '' },
]

export const DEFAULT_EDITOR: EditorId = 'cursor'

export const TEMPLATE_MAX = 512

/** Schemes that run code or read local content instead of opening an editor. */
export const BLOCKED_SCHEMES = ['javascript', 'data', 'vbscript', 'file', 'blob', 'about', 'filesystem', 'view-source', 'intent', 'ms-msdt', 'search-ms']

export function parseEditor(value: unknown): EditorId {
  return EDITORS.some((e) => e.id === value) ? (value as EditorId) : DEFAULT_EDITOR
}

export function editorLabel(editor: EditorId): string {
  return EDITORS.find((e) => e.id === editor)?.label ?? editor
}

/** The message for the first rule the template breaks, or undefined when it can open an editor. */
export function validateTemplate(template: string): string | undefined {
  const value = template.trim()
  if (value.length < 1) return 'Enter a link template.'
  if (value.length > TEMPLATE_MAX) return `Keep the template to ${TEMPLATE_MAX} characters or fewer.`
  if (/[\s\u0000-\u001f\u007f]/.test(value)) return 'The template cannot contain spaces or control characters.'
  const scheme = /^([a-z][a-z0-9+.-]*):/i.exec(value)?.[1]
  if (!scheme) return 'Start with a scheme, such as windsurf://file{path}.'
  if (BLOCKED_SCHEMES.includes(scheme.toLowerCase())) return 'That scheme cannot open an editor.'
  if (!value.includes('{path}')) return 'Include {path} where the file path goes.'
  if (/\{(?!path\}|line\})[^{}]*\}/.test(value)) return 'Only {path} and {line} are replaced.'
  return undefined
}

function encodedFilePath(path: string): string {
  const slashed = path.replace(/\\/g, '/')
  const filePath = slashed.startsWith('/') ? slashed : `/${slashed}`
  return encodeURI(filePath).replace(/[?#]/g, encodeURIComponent)
}

function lineNumber(line?: number): number | undefined {
  return line && line > 0 ? Math.floor(line) : undefined
}

/** Fills a valid template. Without a line, {line} and one ":" before it are removed. */
export function templateUrl(template: string, path: string, line?: number): string {
  const known = lineNumber(line)
  const queryPath = encodedFilePath(path).replace(/[&=+;]/g, encodeURIComponent)
  const filled = template.trim().replace(/\{path\}/g, () => queryPath)
  if (known !== undefined) return filled.replace(/\{line\}/g, String(known))
  const scheme = /^[a-z][a-z0-9+.-]*:/i.exec(filled)?.[0] ?? ''
  return scheme + filled.slice(scheme.length).replace(/:?\{line\}/g, '')
}

/** The editor Open links use: a custom choice with a broken template falls back to the default. */
export function effectiveEditor(editor: EditorId, template: string): EditorId {
  return editor === CUSTOM_EDITOR && validateTemplate(template) !== undefined ? DEFAULT_EDITOR : editor
}

/** A link that asks the editor to open path, at line when it is known. */
export function editorUrl(editor: EditorId, path: string, line?: number, template = ''): string {
  if (editor === CUSTOM_EDITOR && validateTemplate(template) === undefined) return templateUrl(template, path, line)
  const known = lineNumber(line)
  const scheme = EDITORS.find((e) => e.id === editor)?.scheme || EDITORS[0].scheme
  return `${scheme}://file${encodedFilePath(path)}${known === undefined ? '' : `:${known}`}`
}

/** The path as the person's system writes it: backslashes for a Windows drive or share. */
export function copyablePath(path: string): string {
  return /^[A-Za-z]:[\\/]/.test(path) || /^(\/\/|\\\\)[^/\\]/.test(path) ? path.replace(/\//g, '\\') : path
}
