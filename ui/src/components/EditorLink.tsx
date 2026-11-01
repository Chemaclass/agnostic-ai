import { useId, useState, useSyncExternalStore } from 'react'
import { copyablePath, DEFAULT_EDITOR, EDITORS, editorLabel, editorUrl, parseEditor, type EditorId } from '../model/editor'
import { useApp } from './app-context'
import { Select, CopyButton, Icon } from './ui'

const STORAGE_KEY = 'agnostic-ai.editor'
const listeners = new Set<() => void>()
let chosen: EditorId | undefined

function readEditor(): EditorId {
  if (chosen) return chosen
  try {
    return parseEditor(window.localStorage.getItem(STORAGE_KEY))
  } catch {
    return DEFAULT_EDITOR
  }
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  const onStorage = (event: StorageEvent) => {
    if (event.key !== STORAGE_KEY) return
    chosen = undefined
    listener()
  }
  window.addEventListener('storage', onStorage)
  return () => {
    listeners.delete(listener)
    window.removeEventListener('storage', onStorage)
  }
}

function chooseEditor(editor: EditorId) {
  chosen = editor
  try {
    window.localStorage.setItem(STORAGE_KEY, editor)
  } catch {
    // The choice still holds until the page reloads.
  }
  for (const listener of listeners) listener()
}

export function useEditor(): EditorId {
  return useSyncExternalStore(subscribe, readEditor, () => DEFAULT_EDITOR)
}

export function EditorChoice() {
  const app = useApp()
  const editor = useEditor()
  const id = useId()
  if (app.source.kind !== 'api') return null
  return (
    <div className="editor-choice" role="group" aria-label="Open files in">
      <div className="editor-switch" role="radiogroup" aria-label="Editor" onKeyDown={(event) => {
        const index = EDITORS.findIndex((e) => e.id === editor)
        const step = event.key === 'ArrowRight' || event.key === 'ArrowDown' ? 1 : event.key === 'ArrowLeft' || event.key === 'ArrowUp' ? -1 : 0
        if (step === 0) return
        event.preventDefault()
        const next = EDITORS[(index + step + EDITORS.length) % EDITORS.length]
        chooseEditor(next.id)
        ;(event.currentTarget.children[EDITORS.indexOf(next)] as HTMLButtonElement | undefined)?.focus()
      }}>
        {EDITORS.map((e) => (
          <button key={e.id} type="button" role="radio" className="editor-switch-option" aria-checked={editor === e.id} tabIndex={editor === e.id ? 0 : -1} onClick={() => chooseEditor(e.id)}>
            {e.label}
          </button>
        ))}
      </div>
      <div className="editor-choice-mobile">
        <label htmlFor={`${id}-select`}>Editor</label>
        <Select id={`${id}-select`} value={editor} onChange={(event) => chooseEditor(parseEditor(event.target.value))}>
          {EDITORS.map((e) => <option key={e.id} value={e.id}>{e.label}</option>)}
        </Select>
      </div>
    </div>
  )
}

export function OpenInEditor({ path, line, label, compact, shownPath }: { path?: string; line?: number; label: string; compact?: boolean; shownPath?: string }) {
  const app = useApp()
  const editor = useEditor()
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
  const name = editorLabel(editor)
  const where = line ? ` at line ${line}` : ''
  return (
    <span className="editor-actions">
      <a
        className="button button-quiet editor-open"
        href={editorUrl(editor, path, line)}
        aria-label={`Open ${label} in ${name}${where}`}
        title={`Open in ${name}${where}`}
        onClick={() => setAttempted(true)}
      >
        <Icon name="arrow" />
        <span>{compact ? 'Open' : `Open in ${name}`}</span>
      </a>
      {copy(path)}
      <span className="muted editor-hint" role="status">
        {attempted && `Allow your browser to open ${name}. If nothing opens, choose an installed editor above or copy the path.`}
      </span>
    </span>
  )
}
