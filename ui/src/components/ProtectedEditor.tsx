import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { EditError, editProtected } from '../api'
import type { ProtectedBlock, ProtectedEditPreview, SectionItem, SettingField } from '../contract'
import { rulesFromText } from '../model/permission-edit'
import { hidesValue, PROTECT_DECISIONS, protectedSelection } from '../model/protected-edit'
import { useApp } from './app-context'
import { useEditAudience, ViewPersonalButton } from './EditAudience'
import { useSavedEditNotice } from './PermissionEditor'
import { Code, Select } from './ui'

const PATH_PLACEHOLDER = '.github/**\ncomposer.lock\nmigrations/'

interface ProtectedEditorProps {
  field: SettingField
  files: SectionItem[]
  sources: (shown?: string) => ReactNode
}

export function ProtectedEditor(props: ProtectedEditorProps) {
  const audience = useEditAudience()
  const edit = props.field.editable
  const [path, select] = useState(edit?.path ?? '')
  return <ProtectedForm key={`${audience}:${path}`} {...props} path={path} select={select} />
}

function ProtectedForm({ field, files, sources, path, select }: ProtectedEditorProps & { path: string; select: (path: string) => void }) {
  const app = useApp()
  const audience = useEditAudience()
  const saved = useSavedEditNotice()
  const id = useId()
  const active = useRef(true)
  const busy = useRef(false)
  const edit = field.editable
  const paths = edit?.paths?.length ? edit.paths : edit?.path ? [edit.path] : []
  const current = protectedSelection(field, files, path)
  const local = audience === 'personal'
  const own = local ? current.personal : current.shared
  const effective = local ? current.merged : current.shared
  const [editing, setEditing] = useState(false)
  const [text, setText] = useState('')
  const [decision, setDecision] = useState('ask')
  const [reason, setReason] = useState('')
  const [preview, setPreview] = useState<ProtectedEditPreview>()
  const [pending, setPending] = useState<'preview' | 'apply'>()
  const [error, setError] = useState<EditError>()
  useEffect(() => {
    active.current = true
    return () => { active.current = false }
  }, [])

  if (app.source.kind !== 'api' || !edit) return sources()
  if (edit.disabledReason) return <>{sources()}<p className="muted">{edit.disabledReason}</p></>
  if (hidesValue(effective) || hidesValue(own)) return <>{sources()}<p className="muted">These protected paths include a value hidden because it looks like a credential. Edit the file directly.</p></>
  const overridden = !local && current.personalSet
  const draft: ProtectedBlock = { paths: rulesFromText(text), decision, reason: reason.trim() }
  const begin = () => {
    setText((effective?.paths ?? []).join('\n'))
    setDecision(effective?.decision || 'ask')
    setReason(effective?.reason ?? '')
    setError(undefined)
    setEditing(true)
  }
  const request = async (action: 'preview' | 'apply', next: ProtectedBlock | null = draft) => {
    if (busy.current || !path) return
    busy.current = true
    setPending(action)
    setError(undefined)
    try {
      const result = await editProtected({ action, audience, path, protected: action === 'apply' ? preview!.protected : next, before: own, ...(action === 'apply' ? { expected: preview!.expected } : {}) })
      if (action === 'apply') {
        saved(result)
        app.reload(true)
      }
      if (!active.current) return
      if (action === 'preview') setPreview(result)
      else { setEditing(false); setPreview(undefined) }
    } catch (failure) {
      if (active.current) {
        setPreview(undefined)
        setError(failure instanceof EditError ? failure : new EditError(failure instanceof Error ? failure.message : 'Could not save the protected paths.'))
      }
    } finally {
      busy.current = false
      if (active.current) setPending(undefined)
    }
  }
  const locked = !!pending || !!preview
  const where = local ? 'Local' : 'Project'
  return <>{sources()}<div className="permission-editor">
    {paths.length > 1 && <label className="field-edit-target">Settings file
      <Select aria-label={`Settings file for ${field.label}`} value={path} disabled={!!pending} onChange={(event) => select(event.target.value)}>
        <option value="" disabled>Choose a settings file</option>
        {paths.map((candidate) => <option key={candidate} value={candidate}>{candidate}</option>)}
      </Select>
    </label>}
    {overridden && <div className="permission-override">
      <p>Your local file sets protected paths for this spec. Reset them in Local to edit the project paths.</p>
      <ViewPersonalButton />
    </div>}
    {!overridden && !preview && !editing && <div className="field-edit-row">
      <button type="button" className="button" disabled={!path || !!pending} onClick={begin}>{effective ? 'Edit paths' : 'Add paths'}</button>
      {local && current.personal && <button type="button" className="button button-quiet" disabled={!path || !!pending} onClick={() => { void request('preview', null) }}>Reset to project paths</button>}
      {!path && <span className="muted">Choose the file whose protected paths you want to change.</span>}
    </div>}
    {editing && !preview && <form className="permission-form" onSubmit={(event) => { event.preventDefault(); void request('preview') }}>
      <label htmlFor={id}>Paths to protect</label>
      <p id={`${id}-hint`} className="muted">One path per line from the project root, for example <Code>.github/**</Code> or <Code>composer.lock</Code>. Blank lines are ignored.</p>
      {local && !current.personalSet && current.shared && <p className="muted">Starts with the project paths. Saving replaces them for you only.</p>}
      <textarea id={id} aria-describedby={`${id}-hint`} rows={4} placeholder={PATH_PLACEHOLDER} value={text} disabled={locked} spellCheck={false} onChange={(event) => { setText(event.target.value); setError(undefined) }} />
      <label className="field-edit-target">Decision
        <Select aria-label="Protected paths decision" value={decision} disabled={locked} onChange={(event) => setDecision(event.target.value)}>
          {PROTECT_DECISIONS.map((option) => <option key={option} value={option}>{option === 'ask' ? 'ask: the agent asks first' : 'deny: the edit is blocked'}</option>)}
        </Select>
      </label>
      <label className="field-edit-target">Reason
        <input aria-label="Protected paths reason" value={reason} disabled={locked} placeholder="Shown to the agent when it is blocked" onChange={(event) => setReason(event.target.value)} />
      </label>
      <div className="field-edit-row">
        <button type="submit" className="button button-primary" disabled={locked || draft.paths.length === 0}>{pending === 'preview' ? 'Preparing preview…' : 'Preview changes'}</button>
        <button type="button" className="button button-quiet" disabled={locked} onClick={() => { setEditing(false); setError(undefined) }}>Cancel</button>
        {!local && own && <button type="button" className="button button-quiet" disabled={locked} onClick={() => { void request('preview', null) }}>Remove protected paths</button>}
      </div>
    </form>}
    {!overridden && path && !preview && <p className="muted permission-destination">{where} file: {local && !current.personalPath ? <>an overlay for <Code>{path}</Code></> : <Code>{local ? current.personalPath : path}</Code>}</p>}
    {preview && <section tabIndex={-1} className="permission-preview" aria-label={`${field.label} change preview`} aria-busy={!!pending}>
      <div className="permission-preview-head">
        <h3>{preview.protected === null ? local ? 'Use project paths' : 'Remove protected paths' : `Protect ${preview.protected.paths.length} path${preview.protected.paths.length === 1 ? '' : 's'}`}</h3>
        <div className="field-edit-row">
          <button type="button" className="button button-primary" disabled={!!pending} onClick={() => { void request('apply') }}>{pending === 'apply' ? 'Saving…' : 'Save paths'}</button>
          <button type="button" className="button button-quiet" disabled={!!pending} onClick={() => { setPreview(undefined); setError(undefined) }}>{editing ? 'Back to editing' : 'Cancel'}</button>
        </div>
      </div>
      <p>{local ? 'Local (not committed)' : 'Project (committed)'} · <Code>{preview.file}</Code></p>
      <pre tabIndex={0} aria-label={`${field.label} file changes`}>{preview.diff || 'No file changes.'}</pre>
      <p className="muted">After saving, run <Code>{preview.syncCommand}</Code> to apply.</p>
    </section>}
    {error && <div className="edit-error" role="alert"><p>{error.message}</p>
      {error.status === 409 && <button type="button" className="button button-quiet" disabled={!!pending} onClick={() => app.reload()}>Reload saved values</button>}
    </div>}
  </div></>
}
