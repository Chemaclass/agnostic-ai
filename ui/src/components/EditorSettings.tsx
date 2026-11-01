import { useId, useState } from 'react'
import { CUSTOM_EDITOR, EDITORS, parseEditor, templateUrl, validateTemplate } from '../model/editor'
import { useApp } from './app-context'
import { chooseEditor, chooseTemplate, useEditorChoice } from './EditorLink'
import { SectionHeading } from './SectionHeading'
import { Select } from './ui'

const EXAMPLE_PATH = '/Users/you/project/AGENTS.md'
const EXAMPLE_LINE = 12

export function EditorSettings() {
  const app = useApp()
  const { editor, template, saveFailed } = useEditorChoice()
  const id = useId()
  const [touched, setTouched] = useState(() => template.trim() !== '')
  const [announced, setAnnounced] = useState('')
  if (app.source.kind !== 'api') return null
  const custom = editor === CUSTOM_EDITOR
  const problem = custom ? validateTemplate(template) : undefined
  const error = touched ? problem : undefined
  const helpId = `${id}-help`
  const errorId = `${id}-error`
  return (
    <section className="block editor-settings" aria-labelledby="settings-editor">
      <SectionHeading topic="editor" id="settings-editor" focusable>Editor</SectionHeading>
      <p className="muted">Open links use this editor. Saved in this browser only, not in your config files.</p>
      <label htmlFor={`${id}-select`}>Open files in</label>
      <Select id={`${id}-select`} value={editor} onChange={(event) => { setAnnounced(''); chooseEditor(parseEditor(event.target.value)) }}>
        {EDITORS.map((e) => <option key={e.id} value={e.id}>{e.label}</option>)}
      </Select>
      {custom && (
        <>
          <label htmlFor={`${id}-template`}>Link template</label>
          <input
            id={`${id}-template`}
            type="text"
            className="editor-settings-input"
            value={template}
            spellCheck={false}
            autoComplete="off"
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? `${helpId} ${errorId}` : helpId}
            onChange={(event) => {
              const next = event.target.value
              const nextProblem = validateTemplate(next)
              setTouched(true)
              if ((nextProblem === undefined) !== (problem === undefined)) setAnnounced(nextProblem ?? '')
              chooseTemplate(next)
            }}
            onBlur={() => { setTouched(true); setAnnounced(problem ?? '') }}
          />
          <p id={helpId} className="muted">Use {'{path}'} for the file and {'{line}'} for the line. Example: windsurf://file{'{path}'}:{'{line}'}</p>
          {error
            ? <p id={errorId} className="edit-error editor-settings-error">{error}</p>
            : problem ? null : <p className="muted editor-settings-example">Opens <code>{templateUrl(template, EXAMPLE_PATH, EXAMPLE_LINE)}</code></p>}
        </>
      )}
      <p className="sr-only" role="alert">{announced}</p>
      <p className="muted" role="status">{saveFailed ? 'Could not save in this browser. The choice lasts until you reload.' : ''}</p>
    </section>
  )
}
