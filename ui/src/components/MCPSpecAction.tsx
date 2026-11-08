import { useEffect, useRef, useState } from 'react'
import { importMCPSpec } from '../api'
import type { MCPSpecPreview, NativeHolder, NativeServer } from '../contract'
import { useApp } from './app-context'
import { Notice } from './Banner'
import { useEditAudience, useGlobalLevel } from './EditAudience'
import { Code, ToolAvatar } from './ui'

export type MCPSpecSaved = Pick<MCPSpecPreview, 'file' | 'syncCommand'>

export interface MCPSpecImport {
  preview?: MCPSpecPreview
  pending?: 'preview' | 'apply'
  error?: string
  request: (action: 'preview' | 'apply') => Promise<void>
  cancel: () => void
}

/** Imports one native server from the chosen holder: a preview first, then apply. Undefined when the server cannot write. */
export function useMCPSpecImport(server: NativeServer, holder: NativeHolder | undefined, onSaved: (saved: MCPSpecSaved) => void): MCPSpecImport | undefined {
  const app = useApp()
  const audience = useEditAudience()
  const [preview, setPreview] = useState<MCPSpecPreview>()
  const [pending, setPending] = useState<'preview' | 'apply'>()
  const [error, setError] = useState<string>()
  const revision = useRef(0)
  const busy = useRef(false)

  useEffect(() => {
    revision.current++
    setPreview(undefined)
    setError(undefined)
  }, [audience, holder?.target, holder?.path, holder?.scope])

  if (app.source.kind !== 'api' || !holder) return undefined

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
  return { preview, pending, error, request, cancel: () => { setPreview(undefined); setError(undefined) } }
}

/** The row's one action: Import starts the preview. */
export function MCPSpecImportButton({ server, flow }: { server: NativeServer; flow: MCPSpecImport }) {
  return <>
    <span className="mcp-spec-status" role="status" aria-live="polite">{flow.pending === 'preview' ? 'Preparing preview…' : flow.pending === 'apply' ? 'Saving…' : ''}</span>
    {!flow.preview && <button className="button button-primary" type="button" aria-label={`Import ${server.name}`} disabled={!!flow.pending} onClick={() => { void flow.request('preview') }}>Import</button>}
  </>
}

/** The import's error or its preview, under the row. */
export function MCPSpecPreviewPanel({ server, flow }: { server: NativeServer; flow: MCPSpecImport }) {
  const audience = useEditAudience()
  const global = useGlobalLevel()
  const { preview, pending, error } = flow
  if (!error && !preview) return null
  return <div className="mcp-spec">
    {error && <Notice tone="error" title={error} />}
    {preview && <section className="mcp-spec-preview" aria-label={`${server.name} preview`} aria-busy={!!pending}>
      <h3>Add {server.name} to your setup</h3>
      <p>Create <Code>{preview.file}</Code> {audience === 'personal' ? 'for you only' : global ? 'for every project' : 'for everyone'}.</p>
      <div className="mcp-spec-files"><pre tabIndex={0} aria-label={`File for ${server.name}`}>{preview.spec}</pre></div>
      {preview.variables.length > 0 && <p>Set these environment variables before syncing: {preview.variables.map((variable, index) => <span key={variable}>{index > 0 && ', '}<Code>{variable}</Code></span>)}.</p>}
      <p>{preview.targets.length > 0 ? <>After sync, this server reaches <span className="tool-list">{preview.targets.map((target) => <ToolAvatar key={target} target={target} />)}</span>.</> : 'No configured tool accepts this server.'}</p>
      <p className="muted">Run <Code>{preview.syncCommand}</Code> after saving to share the server with your tools.</p>
      <div className="mcp-spec-actions">
        <button className="button button-primary" type="button" disabled={!!pending} onClick={() => { void flow.request('apply') }}>Apply</button>
        <button className="button" type="button" disabled={!!pending} onClick={flow.cancel}>Cancel</button>
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
