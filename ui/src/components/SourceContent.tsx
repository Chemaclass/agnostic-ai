import { useEffect, useState } from 'react'
import type { SectionItem, SourceView } from '../contract'
import { LoadError, loadSource } from '../api'
import { useApp } from './app-context'
import { Chip } from './ui'

const sourceKinds = new Set(['instructions', 'rule', 'agent', 'skill', 'command', 'review', 'ignore'])

type Loaded = { view: SourceView } | { error: string }

/** Show content on a Context row: reads the file from the running server on first open and keeps it until the overview reloads. */
export function SourceContent({ item }: { item: SectionItem }) {
  const app = useApp()
  const [open, setOpen] = useState(false)
  const [loaded, setLoaded] = useState<Loaded | undefined>()
  const path = item.openPath
  const shown = app.source.kind === 'api' && path !== undefined && sourceKinds.has(item.kind)
  useEffect(() => setLoaded(undefined), [app.overviewRevision, path])
  useEffect(() => {
    if (!shown || !open || loaded || !path) return
    const controller = new AbortController()
    loadSource(path, controller.signal)
      .then((view) => setLoaded({ view }))
      .catch((err: unknown) => {
        if ((err as Error).name === 'AbortError') return
        setLoaded({ error: err instanceof LoadError && err.status === 404 ? 'the server no longer lists it. Reload the page to see the current files.' : (err as Error).message })
      })
    return () => controller.abort()
  }, [shown, open, loaded, path, app.overviewRevision])
  if (!shown) return null
  return (
    <details className="item-source" onToggle={(event) => setOpen(event.currentTarget.open)}>
      <summary aria-label={`${open ? 'Hide' : 'Show'} content of ${item.label}`}>{open ? 'Hide content' : 'Show content'}</summary>
      {!loaded && <p className="loading" aria-busy="true">Reading the file…</p>}
      {loaded && 'error' in loaded && <p className="tool-state is-warning" role="alert">Could not show the file: {loaded.error}</p>}
      {loaded && 'view' in loaded && <SourceText view={loaded.view} />}
    </details>
  )
}

function SourceText({ view }: { view: SourceView }) {
  return (
    <div className="item-source-body">
      {view.frontmatter.length > 0 && (
        <span className="value-chips" role="group" aria-label="Frontmatter">
          {view.frontmatter.map((field) => (
            <Chip key={field.key} title={`${field.key}: ${field.value}`}>
              {field.key}: {field.value}
            </Chip>
          ))}
        </span>
      )}
      {view.withheld
        ? <p className="muted">{view.truncated
          ? 'The file is too large to check for credentials, so its text is hidden. Open it in your editor to read it.'
          : 'The text is hidden because it looks like it holds a credential. Open the file in your editor to read it.'}</p>
        : view.body.trim() ? <pre className="source-text">{view.body}</pre> : <p className="muted">The file has no text after its frontmatter.</p>}
      {view.truncated && !view.withheld && <p className="muted">Showing the first 256 KB of {view.bytes.toLocaleString()} bytes. Open the file to read the rest.</p>}
    </div>
  )
}
