import { useState, useSyncExternalStore } from 'react'
import { copyablePath, CUSTOM_EDITOR, DEFAULT_EDITOR, editorLabel, editorUrl, effectiveEditor, parseEditor, type EditorId } from '../model/editor'
import { AppLink, useApp } from './app-context'
import { CopyButton, Icon } from './ui'

const STORAGE_KEY = 'agnostic-ai.editor'
const TEMPLATE_KEY = 'agnostic-ai.editor-template'
const listeners = new Set<() => void>()

interface Choice { editor: EditorId; template: string; saveFailed: boolean }

const serverChoice: Choice = { editor: DEFAULT_EDITOR, template: '', saveFailed: false }
let snapshot: Choice | undefined

function stored(key: string): string | null {
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

function readChoice(): Choice {
  snapshot ??= { editor: parseEditor(stored(STORAGE_KEY)), template: stored(TEMPLATE_KEY) ?? '', saveFailed: false }
  return snapshot
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  const onStorage = (event: StorageEvent) => {
    if (event.key !== null && event.key !== STORAGE_KEY && event.key !== TEMPLATE_KEY) return
    snapshot = undefined
    listener()
  }
  window.addEventListener('storage', onStorage)
  return () => {
    listeners.delete(listener)
    window.removeEventListener('storage', onStorage)
  }
}

function update(next: Partial<Choice>, key: string, value: string) {
  const current = readChoice()
  let saveFailed = false
  try {
    window.localStorage.setItem(key, value)
  } catch {
    saveFailed = true
  }
  snapshot = { ...current, ...next, saveFailed }
  for (const listener of listeners) listener()
}

export function chooseEditor(editor: EditorId) {
  update({ editor }, STORAGE_KEY, editor)
}

export function chooseTemplate(template: string) {
  update({ template }, TEMPLATE_KEY, template)
}

export function useEditorChoice(): Choice {
  return useSyncExternalStore(subscribe, readChoice, () => serverChoice)
}

export function OpenInEditor({ path, line, label, compact, shownPath }: { path?: string; line?: number; label: string; compact?: boolean; shownPath?: string }) {
  const app = useApp()
  const { editor: chosen, template } = useEditorChoice()
  const editor = effectiveEditor(chosen, template)
  const [attempted, setAttempted] = useState(false)
  const copy = (text: string) => <CopyButton text={copyablePath(text)} label="Copy path" ariaLabel={`Copy path of ${label}`} className="editor-copy" />
  if (app.source.kind !== 'api' || !path) {
    if (!shownPath) return null
    return (
      <span className="editor-actions">
        <span className="muted editor-unavailable">
          {app.source.kind !== 'api'
            ? 'Files open in an editor only from a running agnostic-ai serve.'
            : 'This path is not a file on disk, so it cannot be opened in an editor.'}
        </span>
        {copy(shownPath)}
      </span>
    )
  }
  const name = editor === CUSTOM_EDITOR ? 'your editor' : editorLabel(editor)
  const href = editorUrl(editor, path, line, template)
  const newTab = /^https?:/i.test(href)
  const where = line ? ` at line ${line}` : ''
  return (
    <span className="editor-actions">
      <a
        className="button button-quiet editor-open"
        href={href}
        target={newTab ? '_blank' : undefined}
        rel={newTab ? 'noopener noreferrer' : undefined}
        aria-label={`Open ${label} in ${name}${where}`}
        title={`Open in ${name}${where}`}
        onClick={() => setAttempted(true)}
      >
        <Icon name="arrow" />
        <span>{compact ? 'Open' : `Open in ${name}`}</span>
      </a>
      {copy(path)}
      <span className="muted editor-hint" role="status">
        {attempted && <>Allow your browser to open {name}. If nothing opens, change the editor in <AppLink to={{ view: 'settings', tab: 'editor' }}>Settings</AppLink> or copy the path.</>}
      </span>
    </span>
  )
}
