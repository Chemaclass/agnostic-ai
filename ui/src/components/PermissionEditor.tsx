import { createContext, useCallback, useContext, useEffect, useId, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { EditError, editPermissions } from '../api'
import type { PermissionEditPreview, SectionItem, SettingField } from '../contract'
import { audienceDescription, audienceLabel } from '../model/edit-audience'
import { isPermissionKey, permissionSelection, rulesFromText } from '../model/permission-edit'
import { useApp } from './app-context'
import { Notice } from './Banner'
import { useEditAudience, useGlobalLevel, useViewAudience, ViewPersonalButton } from './EditAudience'
import { DECISIONS, ruleRows, type Decision, type RuleRow } from '../model/authority'
import { fieldEntryId } from '../model/identity'
import { useEntry } from './section-context'
import { Code, Icon, RowActions, Select } from './ui'

type Saved = Pick<PermissionEditPreview, 'file' | 'syncCommand'>
const RULE_PLACEHOLDER = 'shell(git status)\nread(src/**)\nmcp:github'

interface RuleRequest { key: string; id: number }

// The saved-file notice, and the list an Add a rule choice asks to open.
const SavedRules = createContext<{ saved?: Saved; update: (saved?: Saved) => void; request?: RuleRequest; ask: (key?: string) => void }>({ update: () => {}, ask: () => {} })

export function PermissionEditProvider({ children }: { children: ReactNode }) {
  const [saved, update] = useState<Saved>()
  const [request, setRequest] = useState<RuleRequest>()
  const ask = useCallback((key?: string) => setRequest((last) => (key ? { key, id: (last?.id ?? 0) + 1 } : undefined)), [])
  return <SavedRules.Provider value={{ saved, update, request, ask }}>{children}</SavedRules.Provider>
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
    actions={error.status === 409 && <button type="button" className="button" disabled={busy} onClick={onReload}>Reload</button>}
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
  decision: Decision
  /** Rules to show; the rest stay saved but hidden while the page filters. */
  shows: (row: RuleRow) => boolean
}

/** One permission list as rows of the Authority rules table, with its editor below them. */
export function PermissionEditor(props: PermissionEditorProps) {
  const audience = useEditAudience()
  const edit = props.field.editable
  const [selected, select] = useState(() => ({ path: edit?.path ?? '', spec: edit?.sources?.find((source) => source.path === edit.path)?.spec }))
  const source = edit?.sources?.find((source) => source.path === selected.path) ?? edit?.sources?.find((source) => source.spec === selected.spec)
  const paths = edit?.paths?.length ? edit.paths : edit?.path ? [edit.path] : []
  const path = source?.path ?? (paths.includes(selected.path) ? selected.path : '')
  return <PermissionForm key={`${audience}:${path}`} {...props} path={path} select={(next) => select({ path: next, spec: edit?.sources?.find((source) => source.path === next)?.spec })} />
}

function PermissionForm({ field, files, decision, shows, path, select }: PermissionEditorProps & { path: string; select: (path: string) => void }) {
  const app = useApp()
  const audience = useEditAudience()
  const viewAudience = useViewAudience()
  const global = useGlobalLevel()
  const Level = audienceLabel('shared', global)
  const level = Level.toLowerCase()
  const receipt = useContext(SavedRules)
  const id = useId()
  const { linked, ...entry } = useEntry(fieldEntryId(field))
  const textarea = useRef<HTMLTextAreaElement>(null)
  const previewRegion = useRef<HTMLElement>(null)
  const savedList = useRef<HTMLUListElement>(null)
  const panel = useRef<HTMLDivElement>(null)
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
    else if (restoreFocus.current && !editing && !settling) (savedList.current ?? addRuleButton())?.focus()
    if (!preview && !settling) restoreFocus.current = false
  }, [preview, editing, settling])
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

  const editable = app.source.kind === 'api' && !!edit && isPermissionKey(field.key) && !edit.disabledReason
  const disabledReason = edit?.sources?.find((source) => source.path === path)?.disabledReason
  const overridden = audience === 'shared' && current.personalSet
  const waiting = !!pending || settling
  const locked = waiting || !!preview || !!disabledReason
  const canEdit = editable && !disabledReason && !overridden && !!path
  const begin = (line?: number, append = false) => {
    const start = effective ?? []
    setBefore(own)
    setFocusLine(append && start.length ? start.length : line)
    setText(start.join('\n') + (append && start.length ? '\n' : ''))
    setError(undefined)
    setRefreshed(false)
    setEditing(true)
  }
  const asked = receipt.request?.key === field.key ? receipt.request : undefined
  // A request made while a save settles waits for the refreshed rules.
  useEffect(() => {
    if (!asked || waiting) return
    receipt.ask()
    if (preview) previewRegion.current?.focus()
    else if (editing) {
      // Keep the draft; start a new line at its end.
      const next = text && !text.endsWith('\n') ? `${text}\n` : text
      setText(next)
      setFocusLine(next.split('\n').length - 1)
      textarea.current?.focus()
    } else if (canEdit) begin(undefined, true)
    else panel.current?.focus()
  }, [asked, waiting])
  const refocus = () => (editing ? textarea.current : savedList.current ?? addRuleButton())?.focus()
  const awaitRefresh = () => {
    settleFrom.current = app.overviewRevision ?? null
    setSettling(true)
  }
  const rules = rulesFromText(text)
  const unchanged = JSON.stringify(rules) === JSON.stringify(own ?? []) && !inherited
  const request = async (action: 'preview' | 'apply', next: string[] | null = rules) => {
    if (busy.current || settleFrom.current || !path || !isPermissionKey(field.key)) return
    busy.current = true
    setPending(action)
    setError(undefined)
    try {
      const result = await editPermissions({ action, audience, path, key: field.key, rules: action === 'apply' ? preview!.rules : next, before: editing ? before : own, ...(action === 'apply' ? { expected: preview!.expected } : {}) })
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
  // A row edits the list this view saves to: the local list once it exists, else the shared one.
  const ownsRow = (row: RuleRow) => canEdit && (audience === 'personal' && current.personalSet ? row.local && row.path === current.personalPath : !row.local && row.path === path)
  const remove = (index: number) => {
    const rest = (effective ?? []).filter((_, at) => at !== index)
    void request('preview', rest.length ? rest : audience === 'personal' ? [] : null)
  }
  const rows = ruleRows(field, viewAudience).filter(shows)
  const scope = (row: RuleRow) => (row.local ? 'Local' : Level)
  const choosing = editable && (paths.length > 1 || !path && paths.length > 0)
  const resettable = editable && audience === 'personal' && current.personalSet && !disabledReason && !preview && !editing
  const differs = JSON.stringify(current.shared ?? []) !== JSON.stringify(current.personal ?? [])
  const showsPanel = linked && !rows.length || choosing || (editable && current.personalSet) || !!disabledReason || !!edit?.disabledReason && app.source.kind === 'api' || resettable || editing || !!preview || !!error || refreshed
  return <div {...entry} className={`authority-group${linked ? ' is-linked' : ''}`} role={rows.length || showsPanel ? 'group' : undefined} aria-label={rows.length || showsPanel ? field.label : undefined}>
    {rows.length > 0 && <ul ref={savedList} tabIndex={-1} className="authority-rule-list" aria-label={`Saved ${field.label} rules`}>
      {rows.map((row) => <li key={`${row.path}:${row.index}:${row.rule}`} className="authority-rule row-hover">
        <Code>{row.rule}</Code>
        <span className={`authority-decision is-${decision.tone}`}><DecisionGlyph decision={decision} />{decision.word}</span>
        <span className="authority-scope">{scope(row)}</span>
        <span className="authority-rule-actions">{ownsRow(row) && <RowActions label={row.rule}>
          <button type="button" className="button button-quiet" disabled={waiting || editing || !!preview} aria-label={`Edit rule ${row.rule}`} onClick={() => begin(row.index)}>Edit</button>
          <button type="button" className="button button-quiet" disabled={waiting || editing || !!preview} aria-label={`Remove rule ${row.rule}`} onClick={() => remove(row.index)}>Remove</button>
        </RowActions>}</span>
      </li>)}
    </ul>}
    {showsPanel && <div ref={panel} tabIndex={-1} className="permission-editor">
      {linked && !rows.length && !editing && <p className="muted">{field.label}: no rules yet.</p>}
      {edit?.disabledReason && <p className="muted">{edit.disabledReason}</p>}
      {choosing && <label className="field-edit-target">Settings file
        <Select aria-label={`Settings file for ${field.label}`} value={path} disabled={!!pending} onChange={(event) => select(event.target.value)}>
          <option value="" disabled>Choose a settings file</option>
          {paths.map((candidate) => <option key={candidate} value={candidate}>{candidate}</option>)}
        </Select>
      </label>}
      {choosing && !path && <p className="muted">Choose the file whose {decision.word.toLowerCase()} rules you want to change.</p>}
      {editable && current.personalSet && <div className="permission-override">
        <Notice tone="info" actions={overridden && <ViewPersonalButton />}>
          <p>{differs ? `${field.label}: local overrides this file’s ${level} rules.` : `${field.label}: local matches ${Level} and replaces this file’s list.`}</p>
          {differs && <details><summary>Compare {level} ({current.shared?.length ?? 0}) and local ({current.personal?.length ?? 0})</summary>
            <RuleComparison label={Level} rules={current.shared} />
            <RuleComparison label="Local" rules={current.personal} cleared={current.personal === null} />
          </details>}
          {overridden && !disabledReason && <p className="muted">Reset this list in Local to edit {Level}.</p>}
        </Notice>
      </div>}
      {disabledReason && <p className="muted">{disabledReason}</p>}
      {resettable && <div className="field-edit-row">
        <button type="button" className="button button-quiet" disabled={!path || waiting} onClick={() => { void request('preview', null) }}>Reset to the {level} rules</button>
      </div>}
      {editing && !preview && <form className="permission-form" onSubmit={(event) => { event.preventDefault(); void request('preview') }}>
        <label htmlFor={id}>{field.label} rules</label>
        <p id={`${id}-hint`} className="muted">One rule per line, for example <Code>shell(git status)</Code> or <Code>read(src/**)</Code>. Blank lines are ignored.</p>
        {inherited && <p className="muted">Starts with the {level} rules. Saving replaces this list for you; other settings keep inheriting.</p>}
        <textarea ref={textarea} id={id} aria-describedby={`${id}-hint`} rows={4} placeholder={RULE_PLACEHOLDER} value={text} disabled={locked} spellCheck={false} onChange={(event) => { setText(event.target.value); setError(undefined) }} />
        <div className="field-edit-row">
          <button type="submit" className="button button-primary" disabled={locked || unchanged}>{pending === 'preview' ? 'Preparing preview…' : 'Preview'}</button>
          <button type="button" className="button button-quiet" disabled={locked} onClick={() => { restoreFocus.current = true; setEditing(false); setError(undefined); setRefreshed(false) }}>Cancel</button>
          {audience === 'shared' && own !== null && <button type="button" className="button button-quiet" disabled={locked} onClick={() => { void request('preview', null) }}>Remove this list</button>}
        </div>
        {path && <p className="muted permission-destination">{audience === 'shared' ? `${Level} file` : 'Local file'}: {audience === 'personal' && !current.personalPath ? <>an overlay for <Code>{path}</Code></> : <Code>{audience === 'personal' ? current.personalPath : path}</Code>}</p>}
      </form>}
      {preview && <section ref={previewRegion} tabIndex={-1} className="permission-preview" aria-label={`${field.label} change preview`} aria-busy={!!pending}>
        <div className="permission-preview-head">
          <h3>{preview.rules === null ? audience === 'personal' ? `Use the ${level} rules` : `Remove the ${level} list` : preview.rules.length === 0 && audience === 'personal' ? 'Clear the list for you' : `Save ${preview.rules.length} rule${preview.rules.length === 1 ? '' : 's'}`}</h3>
          <div className="field-edit-row">
            <button type="button" className="button button-primary" disabled={!!pending || !!disabledReason} onClick={() => { void request('apply') }}>{pending === 'apply' ? 'Applying…' : 'Apply'}</button>
            <button type="button" className="button button-quiet" disabled={!!pending} onClick={() => { restoreFocus.current = true; setPreview(undefined); setError(undefined) }}>{editing ? 'Edit' : 'Cancel'}</button>
          </div>
        </div>
        <p>{audienceDescription(audience, global)} · <Code>{preview.file}</Code></p>
        <FileDiff label={`${field.label} file changes`} diff={preview.diff} />
        <p className="muted">Apply saves this file. Then run <Code>{preview.syncCommand}</Code> to update your tools. Tool support varies; see Tool support at the top of the page.</p>
      </section>}
      {error && <EditFailure error={error} action={failed} busy={waiting} onReload={() => { refreshing.current = true; awaitRefresh(); app.reload() }} onDismiss={() => { setError(undefined); refocus() }} />}
      {refreshed && <div role="status"><Notice tone="info" title="Saved rules refreshed" onDismiss={() => { setRefreshed(false); refocus() }}><p>Your draft is kept. Preview again to compare it with the saved list.</p></Notice></div>}
    </div>}
  </div>
}

function DecisionGlyph({ decision }: { decision: Decision }) {
  if (decision.tone === 'blocked') return <Icon name="close" />
  if (decision.tone === 'permitted') return <Icon name="check" />
  return <span className="authority-glyph" aria-hidden="true">?</span>
}

function addRuleButton(): HTMLElement | null {
  return document.querySelector<HTMLElement>('[data-add-rule]')
}

/** The page's primary action: it asks which list, then opens that list's editor with a new line. */
export function AddRule({ fields }: { fields: SettingField[] }) {
  const { ask } = useContext(SavedRules)
  const [open, setOpen] = useState(false)
  const root = useRef<HTMLDivElement>(null)
  const button = useRef<HTMLButtonElement>(null)
  const menuId = useId()
  const lists = DECISIONS.map((decision) => ({ decision, field: fields.find((field) => field.key === decision.key) })).filter((list): list is { decision: Decision; field: SettingField } => list.field?.editable?.kind === 'permission-list' && !list.field.editable.disabledReason)
  useEffect(() => {
    if (!open) return
    root.current?.querySelector<HTMLButtonElement>('[role="menuitem"]')?.focus()
    const outside = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false)
    }
    document.addEventListener('pointerdown', outside)
    return () => document.removeEventListener('pointerdown', outside)
  }, [open])
  if (!lists.length) return null
  const onKeyDown = (event: KeyboardEvent) => {
    if (!open) return
    if (event.key === 'Escape') {
      event.stopPropagation()
      setOpen(false)
      button.current?.focus()
      return
    }
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
    event.preventDefault()
    const items = [...(root.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [])]
    const at = items.indexOf(document.activeElement as HTMLButtonElement)
    const next = at < 0 ? (event.key === 'ArrowDown' ? 0 : items.length - 1) : (at + (event.key === 'ArrowDown' ? 1 : items.length - 1)) % items.length
    items[next]?.focus()
  }
  return <div ref={root} className="add-rule" onKeyDown={onKeyDown} onBlur={(event) => { if (event.relatedTarget && !root.current?.contains(event.relatedTarget)) setOpen(false) }}>
    <button ref={button} data-add-rule type="button" className="button button-primary" aria-haspopup="menu" aria-expanded={open} aria-controls={menuId} onClick={() => setOpen(!open)}>Add a rule</button>
    <div id={menuId} role="menu" aria-label="Add a rule to" className="add-rule-menu" hidden={!open}>
      {lists.map(({ decision, field }) => <button key={field.key} type="button" role="menuitem" className="button button-quiet" onClick={() => { setOpen(false); ask(field.key) }}>
        <span className={`authority-decision is-${decision.tone}`}><DecisionGlyph decision={decision} />{field.label}</span>
      </button>)}
    </div>
  </div>
}

function RuleComparison({ label, rules, cleared = false }: { label: string; rules: string[] | null; cleared?: boolean }) {
  return <div className="permission-comparison"><strong>{label}</strong>{rules?.length ? <ul>{rules.map((rule, index) => <li key={`${index}:${rule}`}><Code>{rule}</Code></li>)}</ul> : <p className="muted">{cleared ? 'Cleared' : rules === null ? 'Not set' : 'Empty list'}</p>}</div>
}
