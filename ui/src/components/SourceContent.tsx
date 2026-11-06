import { useEffect, useState, type ReactNode } from 'react'
import type { SectionItem, SourceView } from '../contract'
import { LoadError, loadSource } from '../api'
import { useApp } from './app-context'
import { Chip } from './ui'

const sourceKinds = new Set(['instructions', 'rule', 'agent', 'skill', 'command', 'review', 'ignore', 'hook'])

export type LoadedSource = { view: SourceView } | { error: string }

/** Whether the running server can show this row's file. */
export function useSourceShown(item: SectionItem): boolean {
  const app = useApp()
  return app.source.kind === 'api' && item.openPath !== undefined && sourceKinds.has(item.kind)
}

/** Reads the row's file from the running server once it is wanted, and keeps it until the overview reloads. */
export function useSource(item: SectionItem, wanted: boolean): LoadedSource | undefined {
  const app = useApp()
  const [loaded, setLoaded] = useState<LoadedSource | undefined>()
  const path = item.openPath
  const shown = useSourceShown(item)
  useEffect(() => setLoaded(undefined), [app.overviewRevision, path])
  useEffect(() => {
    if (!shown || !wanted || loaded || !path) return
    const controller = new AbortController()
    loadSource(path, controller.signal)
      .then((view) => setLoaded({ view }))
      .catch((err: unknown) => {
        if ((err as Error).name === 'AbortError') return
        setLoaded({ error: err instanceof LoadError && err.status === 404 ? 'the server no longer lists it. Reload the page to see the current files.' : (err as Error).message })
      })
    return () => controller.abort()
  }, [shown, wanted, loaded, path, app.overviewRevision])
  return loaded
}

/** Show content on a Context row. */
export function SourceContent({ item }: { item: SectionItem }) {
  const [open, setOpen] = useState(false)
  const loaded = useSource(item, open)
  if (!useSourceShown(item)) return null
  return (
    <details className="item-source" onToggle={(event) => setOpen(event.currentTarget.open)}>
      <summary aria-label={`${open ? 'Hide' : 'Show'} content of ${item.label}`}>{open ? 'Hide content' : 'Show content'}</summary>
      <SourceState loaded={loaded}>{(view) => <SourceText view={view} />}</SourceState>
    </details>
  )
}

export function SourceState({ loaded, children }: { loaded: LoadedSource | undefined; children: (view: SourceView) => ReactNode }) {
  if (!loaded) return <p className="loading" aria-busy="true">Reading the file…</p>
  if ('error' in loaded) return <p className="tool-state is-warning" role="alert">Could not show the file: {loaded.error}</p>
  return <>{children(loaded.view)}</>
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
      {view.body.trim() ? <pre className="source-text">{view.body}</pre> : <p className="muted">The file has no text after its frontmatter.</p>}
      {view.truncated && <p className="muted">Showing the first 256 KB of {view.bytes.toLocaleString()} bytes. Open the file to read the rest.</p>}
    </div>
  )
}
