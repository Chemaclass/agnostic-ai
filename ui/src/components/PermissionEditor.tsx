import { createContext, useContext, useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { EditError, editPermissions } from '../api'
import type { PermissionEditPreview, SectionItem, SettingField } from '../contract'
import { isPermissionKey, permissionSelection, rulesFromText } from '../model/permission-edit'
import { useApp } from './app-context'
import { useEditAudience, ViewPersonalButton } from './EditAudience'
import { Select, Code } from './ui'

type Saved = Pick<PermissionEditPreview, 'file' | 'syncCommand'>
const RULE_PLACEHOLDER = 'shell(git status)\nread(src/**)\nmcp:github'

const SavedRules = createContext<{ saved?: Saved; update: (saved?: Saved) => void }>({ update: () => {} })

export function PermissionEditProvider({ children }: { children: ReactNode }) {
  const [saved, update] = useState<Saved>()
  return <SavedRules.Provider value={{ saved, update }}>{children}</SavedRules.Provider>
}

export function PermissionSaveNotice() {
  const { saved, update } = useContext(SavedRules)
  if (!saved) return null
  return <div className="permission-saved" role="status">
    <p>Saved <Code>{saved.file}</Code>. Run <Code>{saved.syncCommand}</Code> to apply these rules to your tools.</p>
    <button type="button" className="button button-quiet" onClick={() => update()}>Dismiss</button>
  </div>
}

interface PermissionEditorProps {
  field: SettingField
  files: SectionItem[]
  sources: (shown?: string) => ReactNode
}

export function PermissionEditor(props: PermissionEditorProps) {
  const audience = useEditAudience()
  const edit = props.field.editable
  const [selected, select] = useState(() => ({ path: edit?.path ?? '', spec: edit?.sources?.find((source) => source.path === edit.path)?.spec }))
  const source = edit?.sources?.find((source) => source.path === selected.path) ?? edit?.sources?.find((source) => source.spec === selected.spec)
  const paths = edit?.paths?.length ? edit.paths : edit?.path ? [edit.path] : []
  const path = source?.path ?? (paths.includes(selected.path) ? selected.path : '')
  return <PermissionForm key={`${audience}:${path}`} {...props} path={path} select={(next) => select({ path: next, spec: edit?.sources?.find((source) => source.path === next)?.spec })} />
}

function PermissionForm({ field, files, sources, path, select }: PermissionEditorProps & { path: string; select: (path: string) => void }) {
  const app = useApp()
  const audience = useEditAudience()
  const receipt = useContext(SavedRules)
  const id = useId()
  const textarea = useRef<HTMLTextAreaElement>(null)
  const previewRegion = useRef<HTMLElement>(null)
  const editButton = useRef<HTMLButtonElement>(null)
  const restoreFocus = useRef(false)
  const active = useRef(true)
  const busy = useRef(false)
  const refreshing = useRef(false)
  const edit = field.editable
  const paths = edit?.paths?.length ? edit.paths : edit?.path ? [edit.path] : []
  const current = permissionSelection(field, files, path)
  const own = audience === 'personal' ? current.personal : current.shared
  const inherited = audience === 'personal' && !current.personalSet
  const effective = audience === 'personal' ? current.personalSet ? current.personal : current.shared : current.shared
  const [editing, setEditing] = useState(false)
  const [focusLine, setFocusLine] = useState<number>()
  const [text, setText] = useState((effective ?? []).join('\n'))
  const [before, setBefore] = useState(own)
  const [preview, setPreview] = useState<PermissionEditPreview>()
  const [pending, setPending] = useState<'preview' | 'apply'>()
  const [error, setError] = useState<EditError>()
  const [refreshed, setRefreshed] = useState(false)
  useEffect(() => {
    active.current = true
    return () => { active.current = false }
  }, [])
  useEffect(() => {
    const box = textarea.current
    if (!editing || preview || !box) return
    box.focus()
    if (focusLine === undefined) return
    const lines = box.value.split('\n')
    const start = lines.slice(0, focusLine).reduce((sum, line) => sum + line.length + 1, 0)
    box.setSelectionRange(start, start + (lines[focusLine]?.length ?? 0))
    setFocusLine(undefined)
  }, [editing, preview, focusLine])
  useEffect(() => {
    if (preview) previewRegion.current?.focus()
    else if (restoreFocus.current && !editing) editButton.current?.focus()
    if (!preview) restoreFocus.current = false
  }, [preview, editing])
  useEffect(() => {
    if (!refreshing.current) return
    refreshing.current = false
    setBefore(own)
    setError(undefined)
    setRefreshed(editing)
    setPreview(undefined)
  }, [app.overviewRevision, field])

  if (app.source.kind !== 'api' || !edit || !isPermissionKey(field.key)) return sources()
  if (edit.disabledReason) return <>{sources()}<p className="muted">{edit.disabledReason}</p></>
  const key = field.key
  const disabledReason = edit.sources?.find((source) => source.path === path)?.disabledReason
  const overridden = audience === 'shared' && current.personalSet
  const differs = JSON.stringify(current.shared ?? []) !== JSON.stringify(current.personal ?? [])
  const rules = rulesFromText(text)
  const unchanged = JSON.stringify(rules) === JSON.stringify(own ?? []) && !inherited
  const begin = (line?: number) => {
    setBefore(own)
    setFocusLine(line)
    setText((effective ?? []).join('\n'))
    setError(undefined)
    setRefreshed(false)
    setEditing(true)
  }
  const request = async (action: 'preview' | 'apply', next: string[] | null = rules) => {
    if (busy.current || !path) return
    busy.current = true
    setPending(action)
    setError(undefined)
    try {
      const result = await editPermissions({ action, audience, path, key, rules: action === 'apply' ? preview!.rules : next, before: editing ? before : own, ...(action === 'apply' ? { expected: preview!.expected } : {}) })
      if (action === 'apply') {
        receipt.update(result)
        app.reload(true)
      }
      if (!active.current) return
      if (action === 'preview') setPreview(result)
      else { setEditing(false); setPreview(undefined) }
    } catch (failure) {
      if (active.current) {
        setPreview(undefined)
        setError(failure instanceof EditError ? failure : new EditError(failure instanceof Error ? failure.message : 'Could not save the rules.'))
      }
    } finally {
      busy.current = false
      if (active.current) setPending(undefined)
    }
  }
  const locked = !!pending || !!preview || !!disabledReason
  return <>{sources()}<div className="permission-editor">
    {(paths.length > 1 || !path && paths.length > 0) && <label className="field-edit-target">Settings file
      <Select aria-label={`Settings file for ${field.label}`} value={path} disabled={!!pending} onChange={(event) => select(event.target.value)}>
        <option value="" disabled>Choose a settings file</option>
        {paths.map((candidate) => <option key={candidate} value={candidate}>{candidate}</option>)}
      </Select>
    </label>}
    {current.personalSet && <div className="permission-override">
      <p>{differs ? 'Personal overrides this file’s shared rules.' : 'Personal matches Shared and replaces this file’s list.'}</p>
      {differs && <details><summary>Compare shared ({current.shared?.length ?? 0}) and personal ({current.personal?.length ?? 0})</summary>
        <RuleComparison label="Shared" rules={current.shared} />
        <RuleComparison label="Personal" rules={current.personal} cleared={current.personal === null} />
      </details>}
      {overridden && <>{!disabledReason && <p className="muted">Reset this list in Personal to edit Shared.</p>}<ViewPersonalButton /></>}
    </div>}
    {disabledReason && <p className="muted">{disabledReason}</p>}
    {!disabledReason && !overridden && !preview && !editing && !!own?.length && <ul className="permission-rules" aria-label={`Saved ${field.label} rules`}>
      {own.map((rule, index) => <li key={`${index}:${rule}`}>
        <Code>{rule}</Code>
        <button type="button" className="button button-quiet" disabled={!path || !!pending} aria-label={`Edit rule ${rule}`} onClick={() => begin(index)}>Edit</button>
        <button type="button" className="button button-quiet" disabled={!path || !!pending} aria-label={`Remove rule ${rule}`} onClick={() => { const rest = own.filter((_, at) => at !== index); void request('preview', rest.length ? rest : null) }}>Remove</button>
      </li>)}
    </ul>}
    {!disabledReason && !overridden && !preview && !editing && <div className="field-edit-row">
      <button ref={editButton} type="button" className="button" disabled={!path || !!pending} onClick={() => begin()}>{own === null && !effective?.length ? 'Add rules' : 'Edit rules'}</button>
      {audience === 'personal' && current.personalSet && <button type="button" className="button button-quiet" disabled={!path || !!pending} onClick={() => { void request('preview', null) }}>Reset to shared rules</button>}
      {!path && <span className="muted">Choose the file whose rules you want to change.</span>}
    </div>}
    {!disabledReason && editing && !preview && <form className="permission-form" onSubmit={(event) => { event.preventDefault(); void request('preview') }}>
      <label htmlFor={id}>{field.label} rules</label>
      <p id={`${id}-hint`} className="muted">One rule per line, for example <Code>shell(git status)</Code> or <Code>read(src/**)</Code>. Blank lines are ignored.</p>
      {inherited && <p className="muted">Starts with shared rules. Saving replaces this list for you; other settings keep inheriting.</p>}
      <textarea ref={textarea} id={id} aria-describedby={`${id}-hint`} rows={4} placeholder={RULE_PLACEHOLDER} value={text} disabled={locked} spellCheck={false} onChange={(event) => { setText(event.target.value); setError(undefined) }} />
      <div className="field-edit-row">
        <button type="submit" className="button button-primary" disabled={locked || unchanged}>{pending === 'preview' ? 'Preparing preview…' : 'Preview changes'}</button>
        <button type="button" className="button button-quiet" disabled={locked} onClick={() => { setEditing(false); setError(undefined); setRefreshed(false) }}>Cancel</button>
        {audience === 'shared' && own !== null && <button type="button" className="button button-quiet" disabled={locked} onClick={() => { void request('preview', null) }}>Remove this list</button>}
      </div>
    </form>}
    {!overridden && path && !preview && <p className="muted permission-destination">{audience === 'shared' ? 'Shared file' : 'Personal file'}: {audience === 'personal' && !current.personalPath ? <>an overlay for <Code>{path}</Code></> : <Code>{audience === 'personal' ? current.personalPath : path}</Code>}</p>}
    {preview && <section ref={previewRegion} tabIndex={-1} className="permission-preview" aria-label={`${field.label} change preview`} aria-busy={!!pending}>
      <div className="permission-preview-head">
        <h3>{preview.rules === null ? audience === 'personal' ? 'Use shared rules' : 'Remove shared list' : `Save ${preview.rules.length} rule${preview.rules.length === 1 ? '' : 's'}`}</h3>
        <div className="field-edit-row">
          <button type="button" className="button button-primary" disabled={!!pending || !!disabledReason} onClick={() => { void request('apply') }}>{pending === 'apply' ? 'Saving…' : 'Save rules'}</button>
          <button type="button" className="button button-quiet" disabled={!!pending} onClick={() => { restoreFocus.current = true; setPreview(undefined); setError(undefined) }}>{editing ? 'Back to editing' : 'Cancel'}</button>
        </div>
      </div>
      <p>{audience === 'shared' ? 'Shared (committed)' : 'Personal (not committed)'} · <Code>{preview.file}</Code></p>
      <pre tabIndex={0} aria-label={`${field.label} file changes`}>{preview.diff || 'No file changes.'}</pre>
      <p className="muted">After saving, run <Code>{preview.syncCommand}</Code> to apply. Tool support varies, see Coding tool support below.</p>
    </section>}
    {error && <div className="edit-error" role="alert"><p>{error.message}</p>
      {error.status === 409 && <button type="button" className="button button-quiet" disabled={!!pending} onClick={() => { refreshing.current = true; app.reload() }}>Reload saved values</button>}
    </div>}
    {refreshed && <p className="muted" role="status">Saved rules refreshed. Your draft is kept. Preview again to review your changes against the saved list.</p>}
  </div></>
}

function RuleComparison({ label, rules, cleared = false }: { label: string; rules: string[] | null; cleared?: boolean }) {
  return <div className="permission-comparison"><strong>{label}</strong>{rules?.length ? <ul>{rules.map((rule, index) => <li key={`${index}:${rule}`}><Code>{rule}</Code></li>)}</ul> : <p className="muted">{cleared ? 'Cleared' : rules === null ? 'Not set' : 'Empty list'}</p>}</div>
}
