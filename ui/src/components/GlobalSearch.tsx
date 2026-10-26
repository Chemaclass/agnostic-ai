import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import type { OverviewView } from '../contract'
import { buildSearchIndex, searchOverview, type SearchResult } from '../model/search'
import { useApp } from './app-context'

const SHOWN = 50

const KIND_LABELS: Record<SearchResult['kind'], string> = { item: 'Item', field: 'Setting', finding: 'Finding' }

export function GlobalSearch({ overview, onClose }: { overview?: OverviewView; onClose: (navigated: boolean) => void }) {
  const app = useApp()
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const dialog = useRef<HTMLDivElement>(null)
  const input = useRef<HTMLInputElement>(null)
  const listId = useId()
  const index = useMemo(() => (overview ? buildSearchIndex(overview) : []), [overview])
  const results = useMemo(() => searchOverview(index, query), [index, query])
  const shown = results.slice(0, SHOWN)

  useEffect(() => {
    input.current?.focus()
  }, [])
  useEffect(() => setActive(0), [results])
  useEffect(() => {
    document.getElementById(`${listId}-${active}`)?.scrollIntoView({ block: 'nearest' })
  }, [active, listId])

  const open = (result: SearchResult) => {
    onClose(true)
    if (app.state.view === result.view && app.state.entry === result.entry) {
      const target = document.querySelector<HTMLElement>(`[data-entry="${CSS.escape(result.entry)}"]`)
      target?.scrollIntoView({ block: 'center' })
      target?.focus({ preventScroll: true })
      return
    }
    app.navigate({ view: result.view, q: undefined, entry: result.entry })
  }

  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      onClose(false)
    } else if (event.key === 'ArrowDown' && shown.length) {
      event.preventDefault()
      setActive((i) => (i + 1) % shown.length)
    } else if (event.key === 'ArrowUp' && shown.length) {
      event.preventDefault()
      setActive((i) => (i - 1 + shown.length) % shown.length)
    } else if (event.key === 'Enter' && event.target === input.current && shown[active]) {
      event.preventDefault()
      open(shown[active])
    } else if (event.key === 'Tab') {
      const focusable = [...(dialog.current?.querySelectorAll<HTMLElement>('input, button') ?? [])]
      const at = focusable.indexOf(document.activeElement as HTMLElement)
      event.preventDefault()
      focusable[(at + (event.shiftKey ? -1 : 1) + focusable.length) % focusable.length]?.focus()
    }
  }

  const trimmed = query.trim()
  return (
    <div className="search-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(false) }}>
      <div ref={dialog} className="search-dialog" role="dialog" aria-modal="true" aria-labelledby={`${listId}-title`} tabIndex={-1} onKeyDown={onKeyDown}
        onMouseDown={(event) => { if (!(event.target as HTMLElement).closest('input, button')) event.preventDefault() }}>
        <div className="search-dialog-head">
          <h2 id={`${listId}-title`} className="sr-only">Search all configuration</h2>
          <input
            ref={input}
            type="search"
            role="combobox"
            aria-label="Search all configuration"
            aria-expanded={shown.length > 0}
            aria-controls={listId}
            aria-activedescendant={shown.length ? `${listId}-${active}` : undefined}
            aria-autocomplete="list"
            aria-describedby={`${listId}-status`}
            placeholder="Search names, paths, values, and findings"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          <button type="button" className="button button-quiet" onClick={() => onClose(false)}>Close</button>
        </div>
        <p id={`${listId}-status`} className="muted search-dialog-status" role="status">
          {!overview ? 'The overview has not loaded yet.'
            : !trimmed ? 'Type to search every section. Use the arrow keys and Enter to open a result.'
            : results.length === 0 ? `No results for “${trimmed}”.`
            : results.length > SHOWN ? `Showing the first ${SHOWN} of ${results.length} results. Add words to narrow them.`
            : `${results.length} result${results.length === 1 ? '' : 's'}.`}
        </p>
        <ul id={listId} role="listbox" aria-label="Results" className="search-results">
          {shown.map((result, i) => (
            <li
              key={`${result.view}:${result.entry}:${i}`}
              id={`${listId}-${i}`}
              role="option"
              aria-selected={i === active}
              className="search-result"
              onMouseMove={() => setActive(i)}
              onClick={() => open(result)}
            >
              <span className="search-result-title">{result.title}</span>
              <span className="search-result-context">
                <span className="search-result-kind">{KIND_LABELS[result.kind]}</span> {result.context}
                {result.detail && <code>{result.detail}</code>}
              </span>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}
