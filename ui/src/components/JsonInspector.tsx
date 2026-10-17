// A drawer with the exact JSON behind the screen: the current view, the
// overview, or the latest impact result, with its endpoint, schema
// version, and revision.

import { useEffect, useMemo, useRef, type ReactNode } from 'react'
import type { AuthorityView, OverviewView } from '../contract'
import type { ImpactState } from '../App'
import type { InspectorTab } from './app-context'
import type { Remote } from './Shell'
import { CopyButton } from './ui'

const TOKEN = /("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g

function highlight(json: string): ReactNode[] {
  const out: ReactNode[] = []
  let last = 0
  let key = 0
  TOKEN.lastIndex = 0
  for (let m = TOKEN.exec(json); m; m = TOKEN.exec(json)) {
    if (m.index > last) out.push(json.slice(last, m.index))
    if (m[1] !== undefined) {
      if (m[2] !== undefined) {
        out.push(
          <span key={key++} className="j-key">
            {m[1]}
          </span>,
          m[2],
        )
      } else {
        out.push(
          <span key={key++} className="j-str">
            {m[1]}
          </span>,
        )
      }
    } else if (m[3] !== undefined) {
      out.push(
        <span key={key++} className="j-lit">
          {m[3]}
        </span>,
      )
    } else if (m[4] !== undefined) {
      out.push(
        <span key={key++} className="j-num">
          {m[4]}
        </span>,
      )
    }
    last = TOKEN.lastIndex
  }
  out.push(json.slice(last))
  return out
}

function sizeLabel(bytes: number): string {
  return bytes < 1024 ? `${bytes} bytes` : `${(bytes / 1024).toFixed(1)} KB`
}

export function JsonInspector({
  tab,
  onTab,
  onClose,
  authority,
  overview,
  impact,
}: {
  tab: InspectorTab
  onTab: (tab: InspectorTab) => void
  onClose: () => void
  authority: Remote<AuthorityView>
  overview: Remote<OverviewView>
  impact: ImpactState
}) {
  const closeRef = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    const opener = document.activeElement as HTMLElement | null
    closeRef.current?.focus()
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onClose()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
      opener?.focus?.()
    }
    // The opener is captured once, when the drawer opens.
  }, [])

  const doc = tab === 'impact' ? impact.result : tab === 'overview' ? overview.loaded : authority.loaded
  const json = useMemo(() => (doc ? JSON.stringify(doc.data, null, 2) : ''), [doc])
  const nodes = useMemo(() => highlight(json), [json])

  let schema = ''
  let revision = ''
  if (doc && tab === 'impact' && impact.result) {
    schema = impact.result.data.schemaVersion
    revision = `${impact.result.data.baseRevision} → ${impact.result.data.proposedRevision} (proposed)`
  } else if (doc) {
    const data = doc.data as AuthorityView | OverviewView
    schema = data.schemaVersion
    revision = data.snapshot.revision
  }

  return (
    <aside id="json-inspector" className="inspector" role="dialog" aria-modal="false" aria-labelledby="inspector-title">
      <div className="inspector-head">
        <h2 id="inspector-title">JSON inspector</h2>
        <button ref={closeRef} type="button" className="button button-quiet" onClick={onClose}>
          Close<span className="sr-only"> the JSON inspector</span>
        </button>
      </div>
      <div className="segmented" role="group" aria-label="Document to show">
        <button type="button" aria-pressed={tab === 'authority'} onClick={() => onTab('authority')}>
          Authority view
        </button>
        <button type="button" aria-pressed={tab === 'overview'} onClick={() => onTab('overview')}>
          Overview
        </button>
        <button type="button" aria-pressed={tab === 'impact'} onClick={() => onTab('impact')} disabled={!impact.result}>
          Impact result
        </button>
      </div>
      {doc ? (
        <>
          <dl className="inspector-meta">
            <div>
              <dt>Endpoint</dt>
              <dd>
                <code className="code">{doc.endpoint}</code>
              </dd>
            </div>
            <div>
              <dt>Schema</dt>
              <dd>
                <code className="code">{schema || 'not declared'}</code>
              </dd>
            </div>
            <div>
              <dt>Revision</dt>
              <dd>
                <code className="code">{revision || 'not declared'}</code>
              </dd>
            </div>
          </dl>
          <div className="inspector-actions">
            <CopyButton text={json} label="Copy JSON" copiedLabel="JSON copied" />
            <span className="muted">{sizeLabel(new TextEncoder().encode(json).length)}</span>
          </div>
          <pre className="json" tabIndex={0} aria-label={`${tab === 'impact' ? 'Impact result' : tab === 'overview' ? 'Overview' : 'Authority view'} JSON`}>
            {nodes}
          </pre>
        </>
      ) : (
        <p className="muted inspector-empty">{tab === 'impact' ? 'Validate a proposal to see its impact result.' : 'This document has not loaded.'}</p>
      )}
    </aside>
  )
}
