import { useId, useState } from 'react'
import { addHookSpec, EditError } from '../api'
import type { HookSpecPreview, Stage } from '../contract'
import { audienceDescription, type EditAudience } from '../model/edit-audience'
import { emptyHookDraft, HOOK_MATCHES, hookDraftProblem, hookRequest, takesMatch, type HookDraft } from '../model/hook-form'
import { useApp } from './app-context'
import { useGlobalLevel } from './EditAudience'
import { Code, Select } from './ui'

export type HookSaved = Pick<HookSpecPreview, 'file' | 'syncCommand'>

/** The last saved hook, kept for this tab so the notice survives the reload that lists it. */
export function useHookSaved(): [HookSaved | undefined, (saved?: HookSaved) => void] {
  const app = useApp()
  const key = `agnostic-ai.hook-spec-saved:${app.overview?.snapshot.project.root ?? ''}`
  const [saved, setSaved] = useState<HookSaved | undefined>(() => {
    try {
      const value: unknown = JSON.parse(sessionStorage.getItem(key) ?? 'null')
      if (value && typeof value === 'object' && 'file' in value && typeof value.file === 'string' && 'syncCommand' in value && typeof value.syncCommand === 'string') return { file: value.file, syncCommand: value.syncCommand }
    } catch { /* A saved notice is optional. */ }
    return undefined
  })
  const save = (next?: HookSaved) => {
    setSaved(next)
    try {
      if (next) sessionStorage.setItem(key, JSON.stringify(next))
      else sessionStorage.removeItem(key)
    } catch { /* Keep the notice until this page reloads. */ }
    if (next) app.reload(true)
  }
  return [saved, save]
}

export function HookSavedNotice({ saved, dismiss }: { saved: HookSaved; dismiss: () => void }) {
  return <div className="builtin-change builtin-change-controls hook-saved" role="status">
    <span>Saved <Code>{saved.file}</Code>. Run <Code>{saved.syncCommand}</Code> to write it to your tools.</span>
    <button className="button button-quiet" type="button" onClick={dismiss}>Dismiss</button>
  </div>
}

export function AddHookForm({ stages, onClose, onSaved }: { stages: Stage[]; onClose: () => void; onSaved: (saved: HookSaved) => void }) {
  const global = useGlobalLevel()
  const id = useId()
  const [draft, setDraft] = useState<HookDraft>(() => emptyHookDraft(stages.find((stage) => stage.id === 'after-edit')?.id ?? stages[0]?.id ?? ''))
  const [audience, setAudience] = useState<EditAudience>('shared')
  const [preview, setPreview] = useState<HookSpecPreview>()
  const [pending, setPending] = useState<'preview' | 'apply'>()
  const [error, setError] = useState<{ message: string; problems: string[] }>()
  const change = (patch: Partial<HookDraft>) => {
    setDraft((current) => ({ ...current, ...patch }))
    setError(undefined)
  }

  const request = async (action: 'preview' | 'apply') => {
    if (pending) return
    const problem = hookDraftProblem(draft)
    if (problem) {
      setError({ message: problem, problems: [] })
      return
    }
    setPending(action)
    setError(undefined)
    try {
      const reply = await addHookSpec(hookRequest(draft, audience, action, action === 'apply' ? preview?.expected : undefined))
      if (action === 'apply') onSaved({ file: reply.file, syncCommand: reply.syncCommand })
      else setPreview(reply)
    } catch (failure) {
      setPreview(undefined)
      setError({ message: failure instanceof Error ? failure.message : 'Could not add the hook. Try again.', problems: failure instanceof EditError ? failure.problems : [] })
    } finally {
      setPending(undefined)
    }
  }

  const status = <span role="status" aria-live="polite">{pending === 'preview' ? 'Preparing preview…' : pending === 'apply' ? 'Saving hook…' : ''}</span>
  const failure = error && <div className="builtin-change-error" role="alert">
    <p>{error.message}</p>
    {error.problems.length > 0 && <ul>{error.problems.map((problem) => <li key={problem}>{problem}</li>)}</ul>}
  </div>

  if (preview) {
    return <section className="builtin-change builtin-change-preview hook-preview" aria-label={`${draft.name} hook preview`} aria-busy={!!pending}>
      <h3>Add {draft.name}</h3>
      <p>Create <Code>{preview.file}</Code> {audience === 'personal' ? 'for you only' : global ? 'for every project' : 'for everyone'}.</p>
      <div className="builtin-change-files"><pre tabIndex={0} aria-label={`Spec for ${draft.name}`}>{preview.spec}</pre></div>
      <p className="muted">Run <Code>{preview.syncCommand}</Code> after saving to write it to your tools.</p>
      {failure}
      <div className="builtin-change-actions">
        <button className="button button-primary" type="button" disabled={!!pending} onClick={() => { void request('apply') }}>Apply</button>
        <button className="button" type="button" disabled={!!pending} onClick={() => { setPreview(undefined); setError(undefined) }}>Edit</button>
        {status}
      </div>
    </section>
  }

  return <form className="builtin-change hook-form" aria-label="Add a hook" noValidate onSubmit={(event) => { event.preventDefault(); void request('preview') }}>
    <label>Name
      <input value={draft.name} autoFocus autoComplete="off" spellCheck={false} placeholder="format-on-save" aria-describedby={`${id}-name`} onChange={(event) => change({ name: event.target.value })} />
    </label>
    <p id={`${id}-name`} className="muted hook-form-hint">Lowercase letters, digits, and hyphens. It becomes the file name.</p>
    <label>Stage
      <Select value={draft.on} onChange={(event) => change({ on: event.target.value })}>
        {stages.map((stage) => <option key={stage.id} value={stage.id}>{stage.label}</option>)}
      </Select>
    </label>
    {takesMatch(draft.on) && <label>Tool match
      <Select value={draft.match} onChange={(event) => change({ match: event.target.value })}>
        {HOOK_MATCHES.map((match) => <option key={match.value} value={match.value}>{match.label}</option>)}
      </Select>
    </label>}
    <label>Command
      <textarea rows={3} value={draft.command} spellCheck={false} placeholder="npm run format -- $FILE" onChange={(event) => change({ command: event.target.value })} />
    </label>
    <label>Timeout in seconds (optional)
      <input type="number" min={1} step={1} inputMode="numeric" value={draft.timeout} onChange={(event) => change({ timeout: event.target.value })} />
    </label>
    <label>Description (optional)
      <input value={draft.description} autoComplete="off" onChange={(event) => change({ description: event.target.value })} />
    </label>
    <label>Save for
      <Select value={audience} onChange={(event) => { setAudience(event.target.value as EditAudience); setError(undefined) }}>
        <option value="shared">{audienceDescription('shared', global)}</option>
        <option value="personal">{audienceDescription('personal', global)}</option>
      </Select>
    </label>
    {failure}
    <div className="builtin-change-actions">
      <button className="button button-primary" type="submit" disabled={!!pending}>Preview</button>
      <button className="button" type="button" disabled={!!pending} onClick={onClose}>Cancel</button>
      {status}
    </div>
  </form>
}
