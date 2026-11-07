import { createContext, useContext, useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { EditError, editPermissions } from '../api'
import type { PermissionEditPreview, SectionItem, SettingField } from '../contract'
import { audienceDescription, audienceLabel } from '../model/edit-audience'
import { isPermissionKey, permissionSelection, rulesFromText } from '../model/permission-edit'
import { useApp } from './app-context'
import { Notice } from './Banner'
import { useEditAudience, useGlobalLevel, ViewPersonalButton } from './EditAudience'
import { Select, Code } from './ui'

type Saved = Pick<PermissionEditPreview, 'file' | 'syncCommand'>
const RULE_PLACEHOLDER = 'shell(git status)\nread(src/**)\nmcp:github'

const SavedRules = createContext<{ saved?: Saved; update: (saved?: Saved) => void }>({ update: () => {} })

export function PermissionEditProvider({ children }: { children: ReactNode }) {
  const [saved, update] = useState<Saved>()
  return <SavedRules.Provider value={{ saved, update }}>{children}</SavedRules.Provider>
}

export function useSavedEditNotice() {
  return useContext(SavedRules).update
}

export function PermissionSaveNotice() {
  const { saved, update } = useContext(SavedRules)
  if (!saved) return null
  return <div className="permission-saved" role="status">
    <Notice tone="success" title={<>Saved <Code>{saved.file}</Code></>} onDismiss={() => update()}>
      <p>Run <Code>{saved.syncCommand}</Code> to update your tools.</p>
    </Notice>
  </div>
}

export function EditFailure({ error, action, busy, onReload, onDismiss }: { error: EditError; action: 'preview' | 'apply'; busy: boolean; onReload: () => void; onDismiss: () => void }) {
  return <Notice
    tone="error"
    title={action === 'apply' ? 'Could not apply the change' : 'Could not preview the change'}
    actions={error.status === 409 && <button type="button" className="button" disabled={busy} onClick={onReload}>Reload saved values</button>}
    onDismiss={onDismiss}
  >
    <p>{error.message}</p>
    {error.problems.length > 0 && <ul>{error.problems.map((problem) => <li key={problem}>{problem}</li>)}</ul>}
  </Notice>
}

export function FileDiff({ label, diff }: { label: string; diff: string }) {
  return <pre className="file-diff" tabIndex={0} aria-label={label}>
    {diff ? diff.split('\n').map((line, index) => <span key={index} className={diffLineClass(line)}>{line || ' '}</span>) : 'No file changes.'}
  </pre>
}

function diffLineClass(line: string) {
  if (line.startsWith('+++') || line.startsWith('---') || line.startsWith('@@')) return 'diff-line diff-meta'
  if (line.startsWith('+')) return 'diff-line diff-added'
  if (line.startsWith('-')) return 'diff-line diff-removed'
  return 'diff-line'
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
  const global = useGlobalLevel()
  const Level = audienceLabel('shared', global)
  const level = Level.toLowerCase()
  const receipt = useContext(SavedRules)
  const id = useId()
  const textarea = useRef<HTMLTextAreaElement>(null)
  const previewRegion = useRef<HTMLElement>(null)
  const editButton = useRef<HTMLButtonElement>(null)
  const restoreFocus = useRef(false)
  const active = useRef(true)
  const busy = useRef(false)
  const refreshing = useRef(false)
  const settleFrom = useRef<object | null>(null)
  const [settling, setSettling] = useState(false)
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
  const [failed, setFailed] = useState<'preview' | 'apply'>('preview')
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
    if (settleFrom.current && app.overviewRevision !== settleFrom.current) {
      settleFrom.current = null
      setSettling(false)
    }
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
  const awaitRefresh = () => {
    settleFrom.current = app.overviewRevision ?? null
    setSettling(true)
  }
  const request = async (action: 'preview' | 'apply', next: string[] | null = rules) => {
    if (busy.current || settleFrom.current || !path) return
    busy.current = true
    setPending(action)
    setError(undefined)
    try {
      const result = await editPermissions({ action, audience, path, key, rules: action === 'apply' ? preview!.rules : next, before: editing ? before : own, ...(action === 'apply' ? { expected: preview!.expected } : {}) })
      if (action === 'apply') {
        receipt.update(result)
        awaitRefresh()
        app.reload(true)
      }
      if (!active.current) return
      if (action === 'preview') setPreview(result)
      else { restoreFocus.current = true; setEditing(false); setPreview(undefined) }
    } catch (failure) {
      if (active.current) {
        setPreview(undefined)
        setFailed(action)
        setError(failure instanceof EditError ? failure : new EditError(failure instanceof Error ? failure.message : 'Could not save the rules.'))
      }
    } finally {
      busy.current = false
      if (active.current) setPending(undefined)
    }
  }
  const waiting = !!pending || settling
  const locked = waiting || !!preview || !!disabledReason
  return <>{sources()}<div className="permission-editor">
    {(paths.length > 1 || !path && paths.length > 0) && <label className="field-edit-target">Settings file
      <Select aria-label={`Settings file for ${field.label}`} value={path} disabled={!!pending} onChange={(event) => select(event.target.value)}>
        <option value="" disabled>Choose a settings file</option>
        {paths.map((candidate) => <option key={candidate} value={candidate}>{candidate}</option>)}
      </Select>
    </label>}
    {current.personalSet && <div className="permission-override">
      <Notice tone="info" actions={overridden && <ViewPersonalButton />}>
        <p>{differs ? `Local overrides this file’s ${level} rules.` : `Local matches ${Level} and replaces this file’s list.`}</p>
        {differs && <details><summary>Compare {level} ({current.shared?.length ?? 0}) and local ({current.personal?.length ?? 0})</summary>
          <RuleComparison label={Level} rules={current.shared} />
          <RuleComparison label="Local" rules={current.personal} cleared={current.personal === null} />
        </details>}
        {overridden && !disabledReason && <p className="muted">Reset this list in Local to edit {Level}.</p>}
      </Notice>
    </div>}
    {disabledReason && <p className="muted">{disabledReason}</p>}
    {!disabledReason && !overridden && !preview && !editing && !!own?.length && <ul className="permission-rules" aria-label={`Saved ${field.label} rules`}>
      {own.map((rule, index) => <li key={`${index}:${rule}`}>
        <Code>{rule}</Code>
        <button type="button" className="button button-quiet" disabled={!path || waiting} aria-label={`Edit rule ${rule}`} onClick={() => begin(index)}>Edit</button>
        <button type="button" className="button button-quiet" disabled={!path || waiting} aria-label={`Remove rule ${rule}`} onClick={() => { const rest = own.filter((_, at) => at !== index); void request('preview', rest.length ? rest : audience === 'personal' ? [] : null) }}>Remove</button>
      </li>)}
    </ul>}
    {!disabledReason && !overridden && !preview && !editing && <div className="field-edit-row">
      <button ref={editButton} type="button" className="button" disabled={!path || waiting} onClick={() => begin()}>{own === null && !effective?.length ? 'Add rules' : 'Edit rules'}</button>
      {audience === 'personal' && current.personalSet && <button type="button" className="button button-quiet" disabled={!path || waiting} onClick={() => { void request('preview', null) }}>Reset to the {level} rules</button>}
      {!path && <span className="muted">Choose the file whose rules you want to change.</span>}
    </div>}
    {!disabledReason && editing && !preview && <form className="permission-form" onSubmit={(event) => { event.preventDefault(); void request('preview') }}>
      <label htmlFor={id}>{field.label} rules</label>
      <p id={`${id}-hint`} className="muted">One rule per line, for example <Code>shell(git status)</Code> or <Code>read(src/**)</Code>. Blank lines are ignored.</p>
      {inherited && <p className="muted">Starts with the {level} rules. Saving replaces this list for you; other settings keep inheriting.</p>}
      <textarea ref={textarea} id={id} aria-describedby={`${id}-hint`} rows={4} placeholder={RULE_PLACEHOLDER} value={text} disabled={locked} spellCheck={false} onChange={(event) => { setText(event.target.value); setError(undefined) }} />
      <div className="field-edit-row">
        <button type="submit" className="button button-primary" disabled={locked || unchanged}>{pending === 'preview' ? 'Preparing preview…' : 'Preview'}</button>
        <button type="button" className="button button-quiet" disabled={locked} onClick={() => { setEditing(false); setError(undefined); setRefreshed(false) }}>Cancel</button>
        {audience === 'shared' && own !== null && <button type="button" className="button button-quiet" disabled={locked} onClick={() => { void request('preview', null) }}>Remove this list</button>}
      </div>
    </form>}
    {!overridden && path && !preview && <p className="muted permission-destination">{audience === 'shared' ? `${Level} file` : 'Local file'}: {audience === 'personal' && !current.personalPath ? <>an overlay for <Code>{path}</Code></> : <Code>{audience === 'personal' ? current.personalPath : path}</Code>}</p>}
    {preview && <section ref={previewRegion} tabIndex={-1} className="permission-preview" aria-label={`${field.label} change preview`} aria-busy={!!pending}>
      <div className="permission-preview-head">
        <h3>{preview.rules === null ? audience === 'personal' ? `Use the ${level} rules` : `Remove the ${level} list` : preview.rules.length === 0 && audience === 'personal' ? 'Clear the list for you' : `Save ${preview.rules.length} rule${preview.rules.length === 1 ? '' : 's'}`}</h3>
        <div className="field-edit-row">
          <button type="button" className="button button-primary" disabled={!!pending || !!disabledReason} onClick={() => { void request('apply') }}>{pending === 'apply' ? 'Applying…' : 'Apply'}</button>
          <button type="button" className="button button-quiet" disabled={!!pending} onClick={() => { restoreFocus.current = true; setPreview(undefined); setError(undefined) }}>Cancel</button>
        </div>
      </div>
      <p>{audienceDescription(audience, global)} · <Code>{preview.file}</Code></p>
      <FileDiff label={`${field.label} file changes`} diff={preview.diff} />
      <p className="muted">Apply saves this file. Then run <Code>{preview.syncCommand}</Code> to update your tools. Tool support varies; see Coding tool support below.</p>
    </section>}
    {error && <EditFailure error={error} action={failed} busy={waiting} onReload={() => { refreshing.current = true; awaitRefresh(); app.reload() }} onDismiss={() => setError(undefined)} />}
    {refreshed && <div role="status"><Notice tone="info" title="Saved rules refreshed" onDismiss={() => setRefreshed(false)}><p>Your draft is kept. Preview again to compare it with the saved list.</p></Notice></div>}
  </div></>
}

function RuleComparison({ label, rules, cleared = false }: { label: string; rules: string[] | null; cleared?: boolean }) {
  return <div className="permission-comparison"><strong>{label}</strong>{rules?.length ? <ul>{rules.map((rule, index) => <li key={`${index}:${rule}`}><Code>{rule}</Code></li>)}</ul> : <p className="muted">{cleared ? 'Cleared' : rules === null ? 'Not set' : 'Empty list'}</p>}</div>
}
