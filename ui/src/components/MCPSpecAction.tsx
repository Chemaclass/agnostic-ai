import { useEffect, useRef, useState } from 'react'
import { importMCPSpec } from '../api'
import type { MCPSpecPreview, NativeServer } from '../contract'
import { useApp } from './app-context'
import { Notice } from './Banner'
import { useEditAudience, useGlobalLevel } from './EditAudience'
import { Select, Code, ToolAvatar } from './ui'

export type MCPSpecSaved = Pick<MCPSpecPreview, 'file' | 'syncCommand'>

export function MCPSpecAction({ server, onSaved }: { server: NativeServer; onSaved: (saved: MCPSpecSaved) => void }) {
  const app = useApp()
  const audience = useEditAudience()
  const global = useGlobalLevel()
  const [selected, setSelected] = useState(0)
  const [preview, setPreview] = useState<MCPSpecPreview>()
  const [pending, setPending] = useState<'preview' | 'apply'>()
  const [error, setError] = useState<string>()
  const revision = useRef(0)
  const busy = useRef(false)
  const holder = server.holders[selected] ?? server.holders[0]

  useEffect(() => {
    revision.current++
    setPreview(undefined)
    setError(undefined)
  }, [audience, holder?.target, holder?.path, holder?.scope])

  if (app.source.kind !== 'api' || !holder) return null

  const request = async (action: 'preview' | 'apply') => {
    if (busy.current) return
    busy.current = true
    setPending(action)
    setError(undefined)
    const current = revision.current
    try {
      const value = await importMCPSpec(action, audience, server.name, holder, action === 'apply' ? preview?.expected : undefined)
      if (action === 'apply') {
        setPreview(undefined)
        onSaved(value)
      } else if (revision.current === current) setPreview(value)
    } catch (failure) {
      if (revision.current === current) {
        setError(failure instanceof Error ? failure.message : 'Could not add this server to your setup. Try again.')
        setPreview(undefined)
      }
    } finally {
      busy.current = false
      setPending(undefined)
    }
  }

  return <div className="mcp-spec">
    <div className="mcp-spec-controls">
      {server.holders.length > 1 && <label className="field-edit-target">Copy from <Select aria-label={`Source for ${server.name}`} value={selected} disabled={!!pending} onChange={(event) => setSelected(Number(event.target.value))}>
        {server.holders.map((source, index) => <option key={`${source.target}:${source.path}:${source.scope}`} value={index}>{source.label}: {source.path} ({source.scope})</option>)}
      </Select></label>}
      {!preview && <button className="button" type="button" disabled={!!pending} onClick={() => { void request('preview') }}>Add to your setup</button>}
      <span role="status" aria-live="polite">{pending === 'preview' ? 'Preparing preview…' : pending === 'apply' ? 'Saving…' : ''}</span>
    </div>
    {error && <Notice tone="error" title={error} />}
    {preview && <section className="mcp-spec-preview" aria-label={`${server.name} preview`} aria-busy={!!pending}>
      <h3>Add {server.name} to your setup</h3>
      <p>Create <Code>{preview.file}</Code> {audience === 'personal' ? 'for you only' : global ? 'for every project' : 'for everyone'}.</p>
      <div className="mcp-spec-files"><pre tabIndex={0} aria-label={`File for ${server.name}`}>{preview.spec}</pre></div>
      {preview.variables.length > 0 && <p>Set these environment variables before syncing: {preview.variables.map((variable, index) => <span key={variable}>{index > 0 && ', '}<Code>{variable}</Code></span>)}.</p>}
      <p>{preview.targets.length > 0 ? <>After sync, this server reaches <span className="tool-list">{preview.targets.map((target) => <ToolAvatar key={target} target={target} />)}</span>.</> : 'No configured tool accepts this server.'}</p>
      <p className="muted">Run <Code>{preview.syncCommand}</Code> after saving to share the server with your tools.</p>
      <div className="mcp-spec-actions">
        <button className="button button-primary" type="button" disabled={!!pending} onClick={() => { void request('apply') }}>Apply</button>
        <button className="button" type="button" disabled={!!pending} onClick={() => { setPreview(undefined); setError(undefined) }}>Cancel</button>
      </div>
    </section>}
  </div>
}

export function MCPSpecNotice({ saved, dismiss }: { saved: MCPSpecSaved; dismiss: () => void }) {
  return <div role="status">
    <Notice tone="success" title={<>Saved <Code>{saved.file}</Code>.</>} onDismiss={dismiss}>
      <p>Run <Code>{saved.syncCommand}</Code> to share it with your tools.</p>
    </Notice>
  </div>
}
