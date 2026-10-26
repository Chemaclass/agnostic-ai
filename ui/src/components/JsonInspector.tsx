// A drawer with the exact JSON behind the screen: the overview, with its
// endpoint, schema version, and revision.

import { useEffect, useMemo, useRef, type ReactNode } from 'react'
import type { OverviewView } from '../contract'
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

export function JsonInspector({ onClose, overview }: { onClose: () => void; overview: Remote<OverviewView> }) {
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

  const doc = overview.loaded
  const json = useMemo(() => (doc ? JSON.stringify(doc.data, null, 2) : ''), [doc])
  const nodes = useMemo(() => highlight(json), [json])

  return (
    <aside id="json-inspector" className="inspector" role="dialog" aria-modal="false" aria-labelledby="inspector-title">
      <div className="inspector-head">
        <h2 id="inspector-title">JSON inspector</h2>
        <button ref={closeRef} type="button" className="button button-quiet" onClick={onClose}>
          Close<span className="sr-only"> the JSON inspector</span>
        </button>
      </div>
      {doc ? (
        <>
          {overview.error && <p className="muted" role="status">Not refreshed: this is the previous overview, and the latest refresh failed.</p>}
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
                <code className="code">{doc.data.schemaVersion || 'not declared'}</code>
              </dd>
            </div>
            <div>
              <dt>Revision</dt>
              <dd>
                <code className="code">{doc.data.snapshot.revision || 'not declared'}</code>
              </dd>
            </div>
          </dl>
          <div className="inspector-actions">
            <CopyButton text={json} label="Copy JSON" copiedLabel="JSON copied" />
            <span className="muted">{sizeLabel(new TextEncoder().encode(json).length)}</span>
          </div>
          <pre className="json" tabIndex={0} aria-label="Overview JSON">
            {nodes}
          </pre>
        </>
      ) : (
        <p className="muted inspector-empty">The overview has not loaded.</p>
      )}
    </aside>
  )
}
