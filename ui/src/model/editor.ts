export type EditorId = 'cursor' | 'vscode'

export const EDITORS: { id: EditorId; label: string; scheme: string }[] = [
  { id: 'cursor', label: 'Cursor', scheme: 'cursor' },
  { id: 'vscode', label: 'VS Code', scheme: 'vscode' },
]

export const DEFAULT_EDITOR: EditorId = 'cursor'

export function parseEditor(value: unknown): EditorId {
  return EDITORS.some((e) => e.id === value) ? (value as EditorId) : DEFAULT_EDITOR
}

export function editorLabel(editor: EditorId): string {
  return EDITORS.find((e) => e.id === editor)?.label ?? editor
}

/** A link that asks the editor to open path, at line when it is known. */
export function editorUrl(editor: EditorId, path: string, line?: number): string {
  const scheme = EDITORS.find((e) => e.id === editor)?.scheme ?? DEFAULT_EDITOR
  const slashed = path.replace(/\\/g, '/')
  const filePath = slashed.startsWith('/') ? slashed : `/${slashed}`
  const at = line && line > 0 ? `:${Math.floor(line)}` : ''
  return `${scheme}://file${encodeURI(filePath).replace(/[?#]/g, encodeURIComponent)}${at}`
}

/** The path as the person's system writes it: backslashes for a Windows drive or share. */
export function copyablePath(path: string): string {
  return /^[A-Za-z]:[\\/]/.test(path) || /^(\/\/|\\\\)[^/\\]/.test(path) ? path.replace(/\//g, '\\') : path
}
