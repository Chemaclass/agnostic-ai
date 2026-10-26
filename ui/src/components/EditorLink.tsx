import { useId, useState, useSyncExternalStore } from 'react'
import { copyablePath, DEFAULT_EDITOR, EDITORS, editorLabel, editorUrl, parseEditor, type EditorId } from '../model/editor'
import { useApp } from './app-context'
import { CopyButton, Icon } from './ui'

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

/** Picks the editor every file link opens; only a local server has files to open. */
export function EditorChoice() {
  const app = useApp()
  const editor = useEditor()
  const labelId = useId()
  if (app.source.kind !== 'api') return null
  return (
    <div className="editor-choice">
      <span id={labelId} className="editor-choice-label">Open files in</span>
      <div className="segmented" role="group" aria-labelledby={labelId}>
        {EDITORS.map((e) => (
          <button key={e.id} type="button" aria-pressed={editor === e.id} onClick={() => chooseEditor(e.id)}>{e.label}</button>
        ))}
      </div>
    </div>
  )
}

/**
 * Opens a local file in the chosen editor, at its line when known, with
 * Copy path beside it for when no editor answers the link. A browser
 * cannot tell whether the editor opened, so the link never claims it.
 */
export function OpenInEditor({ path, line, label, compact, shownPath }: { path?: string; line?: number; label: string; compact?: boolean; shownPath?: string }) {
  const app = useApp()
  const editor = useEditor()
  const [sentTo, setSentTo] = useState<string>()
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
        className={compact ? 'edit-link' : 'button button-quiet editor-open'}
        href={editorUrl(editor, path, line)}
        aria-label={`Open ${label} in ${name}${where}`}
        title={`Open in ${name}${where}`}
        onClick={() => setSentTo(name)}
      >
        <Icon name="edit" />
        {!compact && <span>Open in {name}</span>}
      </a>
      {copy(path)}
      <span className="muted editor-hint" role="status">
        {sentTo && `Sent to ${sentTo}. If it did not open, copy the path.`}
      </span>
    </span>
  )
}
