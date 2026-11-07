import { useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { API, type KindSupport, type SectionItem, type SettingField, type TargetValue } from '../contract'
import { EditError, postEdit } from '../api'
import { useApp } from './app-context'
import { Notice } from './Banner'
import { ChangeError, SettingChangePreview, useSettingChange } from './BuiltinChange'
import { ViewPersonalButton, useEditAudience, useLevelName, useViewAudience, type EditAudience } from './EditAudience'
import { PermissionEditor, useSavedEditNotice } from './PermissionEditor'
import { ProtectedEditor } from './ProtectedEditor'
import { ConfigSwitch } from './ConfigSwitch'
import { useServedConfig } from './Shell'
import { SectionHeading } from './SectionHeading'
import { documentationUrl } from '../model/documentation'
import { generatedAnswer, installedAnswer, valueGroups } from '../model/field-resolution'
import { addressKey, configOverride, originLabel, personalSetsWhole, personalValueAt, perToolEdit, valueAt } from '../model/field-address'
import { sourcesForAudience } from '../model/audience-view'
import { fieldEntryId } from '../model/identity'
import { Select, Badge, Code, Toggle } from './ui'
import { ConfiguredTools, useEntry } from './section-context'
import { ItemList, NativeValues, OverrideComparison, Values } from './Items'
import { EmptyState } from './EmptyState'

function fieldsHeadingId(title: string) {
  return `fields-${title.toLowerCase().replace(/\W+/g, '-')}`
}

export function SettingsFields({ title, headingEntry, fields, files, filtering, status, support }: { title: string; headingEntry?: string; fields: SettingField[]; files: SectionItem[]; filtering: boolean; status?: ReactNode; support?: KindSupport }) {
  const entry = useApp().state.entry
  const audience = useViewAudience()
  const id = fieldsHeadingId(title)
  const configured = fields.filter((field) => sourcesForAudience(field.set, audience).length > 0)
  const unconfigured = fields.filter((field) => sourcesForAudience(field.set, audience).length === 0)
  const editablePermissions = unconfigured.filter((field) => field.editable?.kind === 'permission-list' || field.editable?.kind === 'protected-list' || field.key === 'permissions.default-mode' && field.editable)
  const remaining = unconfigured.filter((field) => !editablePermissions.includes(field))
  const visibleFields = fields.filter((field) => configured.includes(field) || editablePermissions.includes(field))
  return (
    <section className="block" aria-labelledby={id}>
      <SectionHeading id={id} focusable entry={headingEntry}>{title}</SectionHeading>
      {status}
      {configured.length === 0 && (filtering || editablePermissions.length
        ? <p className="muted">{filtering ? 'No matching configured settings.' : 'No permission rules configured. Add rules below.'}</p>
        : <EmptyState kind={{ kind: 'settings', label: title }} support={support} />)}
      {visibleFields.length > 0 && <FieldList fields={visibleFields} files={files} />}
      {remaining.length > 0 && (
        <details className="unconfigured-fields" open={filtering || remaining.some((field) => fieldEntryId(field) === entry) || undefined}>
          <summary>Unconfigured fields <span className="kind-count">{remaining.length}</span></summary>
          <FieldList fields={remaining} files={files} />
        </details>
      )}
      {files.length > 0 && (
        <>
          <h3 className="item-group-title">Files</h3>
          <ItemList items={files} hideValues />
        </>
      )}
    </section>
  )
}

function FieldList({ fields, files }: { fields: SettingField[]; files: SectionItem[] }) {
  const viewAudience = useViewAudience()
  const level = useLevelName()
  return (
    <ul className="fields">
      {fields.map((f) => (
        <FieldRow key={f.key} field={f}>
          <div className="field-head">
            <strong>{f.label}</strong>
            {f.key !== 'x-' && <code className="muted">{f.key}</code>}
            {viewAudience === 'personal' && f.set.some((src) => src.local) && <Badge tone="accent" title="Set in a local file Git ignores">local</Badge>}
            {viewAudience === 'personal' && f.set.length > 0 && !f.set.some((src) => src.local) && <span className="item-layer">Inherited from {level}</span>}
            <p className="muted">{f.description}</p>
          </div>
          {sourcesForAudience(f.set, viewAudience).length === 0 && <p className="muted field-unset">Not set</p>}
          <FieldEditor field={f} files={files} sources={(shown) => sourcesForAudience(f.set, viewAudience).filter((src) => src.path !== shown).map((src) => (
            <div key={`${src.spec}:${src.path}`} className="field-source">
              {f.key === 'x-' ? <NativeValues
                values={src.values}
                file={src.path}
                localKeys={src.local ? files.find((item) => item.name === src.spec && item.path === src.path)?.localKeys : undefined}
                overrides={viewAudience === 'shared' ? f.set.filter((other) => other.local && other.spec === src.spec).flatMap((other) => other.values) : undefined}
                baseline={viewAudience === 'personal' && src.local ? f.set.filter((other) => !other.local && other.spec === src.spec).flatMap((other) => other.values) : undefined}
              /> : <Values values={src.values} file={f.key.startsWith('voice.') ? undefined : src.path} prefix={f.key} fieldKey={f.key} />}
              <p className="muted field-from">
                from <Code>{src.path}</Code>
              </p>
            </div>
          ))} />
          {f.resolved && f.resolved.length > 0 && <FieldResolution field={f} files={files} resolved={f.resolved} />}
        </FieldRow>
      ))}
    </ul>
  )
}

function FieldResolution({ field, files, resolved }: { field: SettingField; files: SectionItem[]; resolved: TargetValue[] }) {
  const app = useApp()
  const label = field.label
  const level = useLevelName()
  const tools = useContext(ConfiguredTools)
  const [selected, setSelected] = useState(resolved[0].target)
  const current = resolved.find((r) => r.target === selected) ?? resolved[0]
  const groups = valueGroups(resolved)
  const origin = originLabel(current, level)
  const override = configOverride(current)
  const toolEdit = app.source.kind === 'api' && perToolEdit(field, current.target)
  return (
    <details className="field-resolution">
      <summary>What each tool gets</summary>
      <ul className="field-resolution-groups" aria-label={`${label} by tool`}>
        {groups.map((g) => <li key={g.unchecked ? 'unchecked' : g.value ?? ''}>{g.labels.join(', ')}: {g.unchecked ? <span className="muted">not checked</span> : g.value ? <Code>{g.value}</Code> : <span className="muted">no value written</span>}</li>)}
      </ul>
      {resolved.length > 1 && <label className="field-resolution-tool">Tool <Select value={current.target} onChange={(event) => setSelected(event.target.value)}>
        {resolved.map((r) => <option key={r.target} value={r.target}>{r.label}</option>)}
      </Select></label>}
      <div className="field-resolution-detail" aria-live="polite">
        <p>{generatedAnswer(current)}</p>
        {current.from && <p className="muted">From <Code>{current.from.path}</Code>{origin ? <>, {origin}</> : ' in the config'}</p>}
        {override && <p className="muted"><Code>{override}</Code> in the configuration replaces every value from your files for {current.label}. An edit below changes the file only; remove that key for {current.label} to get it.</p>}
        {current.others.length > 0 && <>
          <p className="muted">Values that do not win:</p>
          <ul className="field-resolution-others">
            {current.others.map((o, i) => <li key={`${o.path}:${i}`}>{o.value ? <Code>{o.value}</Code> : 'No value'} in <Code>{o.path}</Code>: {o.reason}</li>)}
          </ul>
        </>}
        {current.native?.map((n) => <p key={n.path} className="muted"><Code>{n.value}</Code> is also set under <Code>{`x-${current.target}.model`}</Code> in <Code>{n.path}</Code>. {current.checked ? `Sync writes that native key as is, so check the generated file for the value ${current.label} reads.` : 'Sync was not rendered here, so this native key was not checked.'}</p>)}
        <p className="muted">{installedAnswer(current.label, tools.find((t) => t.target === current.target)?.sync.state)} agnostic-ai does not see what a running tool uses. Other <Code>{`x-${current.target}`}</Code> keys and imported native settings files are not covered here.</p>
        {typeof toolEdit === 'string' && <p className="muted field-tool-edit">{toolEdit}</p>}
        {toolEdit === true && <div className="field-tool-edit">
          <p className="muted">Set a value for {current.label} only. It is saved as <Code>{addressKey(field.key, current.target)}</Code> and wins over the value for every tool.</p>
          <FieldEditControl key={current.target} field={field} files={files} target={current.target} toolLabel={current.label} sources={() => null} />
        </div>}
      </div>
    </details>
  )
}

function FieldRow({ field, children }: { field: SettingField; children: ReactNode }) {
  const audience = useViewAudience()
  const { linked, ...entry } = useEntry(fieldEntryId(field))
  return <li {...entry} className={`field-row${sourcesForAudience(field.set, audience).length ? ' is-set' : ''}${linked ? ' is-linked' : ''}`}>{children}</li>
}

interface FieldEditControlProps {
  field: SettingField
  files: SectionItem[]
  sources: (shown?: string) => ReactNode
  target?: string
  toolLabel?: string
}

function FieldEditor(props: FieldEditControlProps) {
  if (props.field.editable?.kind === 'protected-list') return <ProtectedEditor {...props} />
  return props.field.editable?.kind === 'permission-list' ? <PermissionEditor {...props} /> : <FieldEditControl {...props} />
}

function GlobalSettingsAction() {
  const { served } = useServedConfig(useApp().source.kind, 0)
  return served && !served.global ? <div className="field-edit-row"><ConfigSwitch served={served} /></div> : null
}

function FieldEditControl(props: FieldEditControlProps) {
  const audience = useEditAudience()
  const edit = props.field.editable
  const [selected, select] = useState(() => {
    const path = edit?.path ?? edit?.paths?.[0] ?? ''
    return { path, spec: edit?.sources?.find((source) => source.path === path)?.spec }
  })
  const paths = edit?.paths?.length ? edit.paths : edit?.path ? [edit.path] : []
  // A save can move the selected spec, such as a first local file; follow it there.
  const moved = !paths.includes(selected.path) && !edit?.sources?.some((source) => source.path === selected.path) ? edit?.sources?.find((source) => source.spec === selected.spec)?.path : undefined
  const path = moved ?? selected.path
  return <FieldEditForm key={`${audience}:${path}`} {...props} path={path} setPath={(next) => select({ path: next, spec: edit?.sources?.find((source) => source.path === next)?.spec })} />
}

function FieldEditForm({ field, files, sources, target, toolLabel, path, setPath }: FieldEditControlProps & { path: string; setPath: (path: string) => void }) {
  const app = useApp()
  const audience = useEditAudience()
  const level = useLevelName()
  const transaction = useSettingChange(field.key, target)
  const savedNotice = useSavedEditNotice()
  const edit = field.editable
  const paths = edit?.paths?.length ? edit.paths : edit?.path ? [edit.path] : []
  const valueOf = (source?: SettingField['set'][number]) => source && valueAt(source.values, field.key, target)
  const specPath = path
  const selectedSource = edit?.sources?.find((source) => source.path === specPath)
  const specName = selectedSource?.spec ?? field.set.find((source) => source.path === specPath)?.spec
  const localPath = selectedSource?.localPath ?? (specPath === edit?.path ? edit?.localPath : undefined)
  const localSource = field.set.find((source) => source.local && (specName ? source.spec === specName : source.path === localPath))
  const localKeys = localSource && files.find((item) => item.name === localSource.spec && item.path === localSource.path)?.localKeys
  const teamSource = field.set.find((source) => source.path === path && !source.local)
  const localCurrent = localSource && personalValueAt(localSource.values, localKeys, field.key, target)
  const wholePersonal = target !== undefined && localSource !== undefined && personalSetsWhole(localKeys, field.key)
  const teamCurrent = valueOf(teamSource)
  const selectedCurrent = audience === 'personal' ? localCurrent ?? teamCurrent : teamCurrent
  const current = selectedCurrent === 'null' ? '' : selectedCurrent ?? ''
  const [value, setValue] = useState(current)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<EditError>()
  const [reloaded, setReloaded] = useState(false)
  // A draft keeps the values it started from, so a save after another reload still detects the change.
  const basis = useRef({ current, local: localCurrent, team: teamCurrent })
  const draft = useRef(value)
  draft.current = value
  const latest = useRef(basis.current)
  latest.current = { current, local: localCurrent, team: teamCurrent }
  const conflict = useRef(false)
  const settleFrom = useRef<object | null>(null)
  const [settling, setSettling] = useState(false)
  useEffect(() => {
    if (settleFrom.current && app.overviewRevision !== settleFrom.current) {
      settleFrom.current = null
      setSettling(false)
    }
  }, [app.overviewRevision])
  const reloadOverview = (saved = false) => {
    settleFrom.current = app.overviewRevision ?? null
    setSettling(true)
    app.reload(saved)
  }
  useEffect(() => {
    const clean = draft.current === basis.current.current || draft.current === current
    if (conflict.current) {
      conflict.current = false
      setError(undefined)
      setReloaded(true)
    } else if (!clean) {
      const was = basis.current
      if (current !== was.current || localCurrent !== was.local || teamCurrent !== was.team) setReloaded(true)
      return
    }
    basis.current = { current, local: localCurrent, team: teamCurrent }
    if (clean) setValue(current)
  }, [current, localCurrent, teamCurrent])
  if (!edit) return sources()
  if (edit.disabledReason) return <>{sources()}<p className="muted">{edit.disabledReason}{field.key === 'session-retention-days' && !target && <> <a href={documentationUrl('/docs/cli-reference/inspect/#serve')}>Global view instructions</a></>}</p>{field.key === 'permissions.default-mode' && <GlobalSettingsAction />}</>
  const sourceDisabledReason = edit.sources?.find((source) => source.path === path)?.disabledReason
  if (sourceDisabledReason) return <>{sources()}<p className="muted">{sourceDisabledReason}</p>{paths.length > 1 && <label className="field-edit-target">{audience === 'shared' ? 'Save to' : 'Override file'} <Select value={path} onChange={(event) => setPath(event.target.value)}>{paths.map((candidate) => <option key={candidate} value={candidate}>{candidate}</option>)}</Select></label>}</>
  if (app.source.kind !== 'api') return sources()
  const specFile = specPath
  const post = async (body: Record<string, unknown>, expected: string | undefined) => {
    setPending(true)
    setError(undefined)
    setReloaded(false)
    conflict.current = false
    try {
      const reply = await postEdit(API.editField, { path: specFile || undefined, key: field.key, target, ...body, expected: expected ?? null })
      if (field.key === 'permissions.default-mode' && reply.file) savedNotice({ file: reply.file, syncCommand })
      const saved = body.value === null ? undefined : String(body.value)
      const next = body.audience === 'personal' ? { ...basis.current, local: saved } : { ...basis.current, team: saved }
      basis.current = { ...next, current: (audience === 'personal' ? next.local ?? next.team : next.team) ?? '' }
      setValue(basis.current.current)
      reloadOverview(true)
    } catch (err) {
      const failure = err instanceof EditError ? err : new EditError(String(err))
      const now = latest.current
      const shown = basis.current
      if (failure.status === 409 && (now.current !== shown.current || now.local !== shown.local || now.team !== shown.team)) {
        basis.current = now
        setReloaded(true)
        conflict.current = true
        reloadOverview()
        return
      }
      conflict.current = failure.status === 409
      setError(failure)
    } finally { setPending(false) }
  }
  const save = (clear: boolean) => post({ audience, value: clear ? null : edit.kind === 'number' ? Number(value) : edit.kind === 'boolean' ? value === 'true' : value }, audience === 'personal' ? basis.current.local : basis.current.team)
  const reset = () => post({ audience: 'personal', value: null }, basis.current.local)
  const overridden = audience === 'shared' && (localCurrent !== undefined || wholePersonal)
  const resetLabel = target ? 'Use the value for every tool' : `Reset to the ${level} value`
  const valueLabel = `${field.label}${toolLabel ? ` for ${toolLabel}` : ''} value`
  const allowed = target ? edit.toolAllowed?.[target] : edit.allowed
  const configured = audience === 'personal' ? localCurrent !== undefined : teamCurrent !== undefined
  const syncCommand = edit.syncCommand ?? `agnostic-ai sync${field.key === 'session-retention-days' ? ' --global' : ''}`
  // A shown preview keeps its draft, so the applied value becomes the new basis.
  const locked = pending || settling || !!transaction.proposal
  const resetButton = audience === 'personal' && !transaction.supported && localCurrent !== undefined && <button type="button" className="button button-quiet" disabled={locked} onClick={reset}>{resetLabel}</button>
  const previewing = pending || settling || !transaction.ready
  const preview = async (next: string | null, where: EditAudience) => {
    setError(undefined)
    setReloaded(false)
    conflict.current = false
    const stale = await transaction.previewSetting({ path: specFile || undefined, key: field.key as 'model' | 'effort', target, value: next, expected: (where === 'personal' ? basis.current.local : basis.current.team) ?? null }, where)
    if (stale) {
      const now = latest.current
      const shown = basis.current
      if (now.current !== shown.current || now.local !== shown.local || now.team !== shown.team) {
        basis.current = now
        setReloaded(true)
      } else conflict.current = true
    }
  }
  const shown = overridden ? undefined : audience === 'personal' ? (localCurrent !== undefined ? localSource?.path : undefined) : teamCurrent !== undefined ? teamSource?.path : undefined
  return <>{sources(shown)}<div className="field-edit">
    {(localCurrent !== undefined || wholePersonal) && <OverrideComparison shared={teamCurrent ?? (target ? valueAt(teamSource?.values ?? [], field.key) : undefined)} personal={localCurrent ?? valueAt(localSource?.values ?? [], field.key)} />}
    {overridden
      ? <div className="field-edit-row">
        <input aria-label={valueLabel} value={teamCurrent ?? ''} placeholder="Not set" disabled />
        <p className="muted">{localCurrent === undefined ? `Your local value for every tool replaces the ${level} per-tool values.` : `Your local value overrides the ${level} value.`} Reset it in Local to change this {level} value.</p>
        <ViewPersonalButton />
      </div>
      : <div className="field-edit-row">
        {edit.kind === 'boolean'
          ? <Toggle on={value === 'true'} label={valueLabel} disabled={locked} onToggle={() => setValue(value === 'true' ? 'false' : 'true')} />
          : allowed && allowed.length > 0
          ? <Select aria-label={valueLabel} value={value} onChange={(event) => setValue(event.target.value)} disabled={locked}>
            {!allowed.includes(value) && <option value={value} disabled>{value || 'Choose a value'}</option>}
            {allowed.map((option) => <option key={option} value={option}>{option}</option>)}
          </Select>
          : <input aria-label={valueLabel} type={edit.kind === 'number' ? 'number' : 'text'} min={edit.kind === 'number' ? 1 : undefined} step={edit.kind === 'number' ? 1 : undefined} value={value} placeholder={selectedCurrent === 'null' ? 'Cleared for you' : field.set.length > 0 && !current ? 'Not set in this file' : 'Not set'} onChange={(event) => setValue(event.target.value)} disabled={locked} />}
        {!transaction.supported && <button type="button" className="button button-primary" disabled={locked || !value.trim() || value === current} onClick={() => save(false)}>Apply</button>}
        {!transaction.supported && audience === 'shared' && configured && <button type="button" className="button button-quiet" disabled={locked} onClick={() => save(true)}>{target ? resetLabel : 'Clear'}</button>}
        {resetButton}
      </div>}
    {paths.length > 1 && <label className="field-edit-target">{audience === 'shared' ? 'Save to' : 'Override file'} <Select value={path} onChange={(event) => setPath(event.target.value)}>{paths.map((candidate) => <option key={candidate} value={candidate}>{candidate}</option>)}</Select></label>}
    {error && <Notice tone="error" title="Could not apply the change" actions={error.status === 409 && <button type="button" className="button" disabled={settling} onClick={() => reloadOverview()}>Reload</button>} onDismiss={() => setError(undefined)}>
      <p>{error.message}</p>
      {error.problems.length > 0 && <ul>{error.problems.map((problem) => <li key={problem}>{problem}</li>)}</ul>}
    </Notice>}
    {transaction.supported && <div className="field-edit-row field-edit-apply">
      <span className="muted">Apply to your tools now:</span>
      {!overridden && <button type="button" className="button" disabled={previewing || !value.trim() || value === current} onClick={() => { void preview(value, audience) }}>Preview</button>}
      {!overridden && audience === 'shared' && configured && <button type="button" className="button button-quiet" disabled={previewing} onClick={() => { void preview(null, 'shared') }}>{target ? 'Preview using the value for every tool' : 'Preview removal'}</button>}
      {audience === 'personal' && localCurrent !== undefined && <button type="button" className="button button-quiet" disabled={previewing} onClick={() => { void preview(null, 'personal') }}>Preview local reset</button>}
      {transaction.preparing && <span className="muted">Preparing preview…</span>}
    </div>}
    {transaction.error && <ChangeError />}
    {transaction.proposal && <SettingChangePreview change={transaction.proposal} label={toolLabel ? `${field.label} for ${toolLabel}` : field.label} />}
    {reloaded && <p className="muted" role="status">The saved value changed after you started editing and is now {current ? <Code>{current}</Code> : 'not set'}. Your draft is kept; save it to replace the saved value.</p>}
    <p className="muted field-edit-hint">
      {audience === 'personal' && (localSource?.path || localPath) && <>Saves to <Code>{localSource?.path ?? localPath}</Code> · </>}
      {audience === 'personal' && !localSource?.path && !localPath && <>Saves to your local override of <Code>{path}</Code> · </>}
      {audience === 'shared' && configured && paths.length <= 1 && <>from <Code>{path}</Code> · </>}
      {audience === 'shared' && !configured && field.set.length > 0 && paths.length <= 1 && path && <>Saves to <Code>{path}</Code> · </>}
      {!transaction.supported && <>Run <Code>{syncCommand}</Code> to apply.</>}
    </p>
  </div></>
}
