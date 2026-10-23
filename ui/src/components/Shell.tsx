import { useEffect, useRef } from 'react'
import type { OverviewView } from '../contract'
import type { Loaded } from '../api'
import { SECTION_VIEWS, type ViewId } from '../model/url'
import { AppLink, useApp } from './app-context'
import { Icon } from './ui'

export interface Remote<T> {
  status: 'loading' | 'ready' | 'error'
  loaded?: Loaded<T>
  error?: Error
}

const DEFAULT_SECTION_LABELS: Record<string, string> = {
  context: 'Context',
  authority: 'Authority',
  tools: 'Tools',
  lifecycle: 'Lifecycle',
  settings: 'Settings',
}

export function Header({
  overview,
  inspectorOpen,
  onToggleInspector,
  theme,
  onToggleTheme,
}: {
  overview: Remote<OverviewView>
  inspectorOpen: boolean
  onToggleInspector: () => void
  theme: 'dark' | 'light'
  onToggleTheme: () => void
}) {
  const app = useApp()
  const snapshot = overview.loaded?.data.snapshot

  return (
    <header className="topbar">
      <div className="brand">
        <span className="logo" aria-hidden="true">
          A
        </span>
        <span className="brand-name">agnostic-ai</span>
      </div>
      <div className="topbar-chips">
        {snapshot?.project.name && (
          <span className="chip chip-project" title={snapshot.project.root ? `Project root: ${snapshot.project.root}` : undefined}>
            <span className="sr-only">Project </span>
            {snapshot.project.name}
          </span>
        )}
        {app.source.kind === 'static' && (
          <span className="chip chip-source" title={`Read from ${app.source.base} with no server`}>
            Static snapshot{snapshot?.revision ? ` · revision ${snapshot.revision}` : ''}
          </span>
        )}
        {app.source.kind === 'mock' && (
          <span className="chip chip-source" title="Handwritten data bundled with the client; not a live configuration scan">
            Mock fixtures
          </span>
        )}
      </div>
      <button type="button" className="button button-json" aria-pressed={inspectorOpen} onClick={onToggleInspector}>
        JSON
      </button>
      <button
        type="button"
        className="button button-quiet button-icon"
        aria-label={theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}
        title={theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}
        onClick={onToggleTheme}
      >
        <Icon name={theme === 'dark' ? 'sun' : 'moon'} />
      </button>
    </header>
  )
}

export function Nav({ overview }: { overview: Remote<OverviewView> }) {
  const app = useApp()
  const navRef = useRef<HTMLElement>(null)
  // On narrow screens the navigation is one scrolling row; keep the
  // current page in view.
  useEffect(() => {
    const current = navRef.current?.querySelector<HTMLElement>('[aria-current="page"]')
    const nav = navRef.current
    if (current && nav && nav.scrollWidth > nav.clientWidth) {
      nav.scrollLeft = Math.max(0, current.offsetLeft - nav.clientWidth / 2 + current.offsetWidth / 2)
    }
  }, [app.state.view])
  const sections = overview.loaded?.data.sections
  const ordered = sections?.length
    ? [...sections].sort((a, b) => a.order - b.order).map((s) => ({ id: s.id as ViewId, label: s.label }))
    : SECTION_VIEWS.map((id) => ({ id: id as ViewId, label: DEFAULT_SECTION_LABELS[id] }))

  const item = (id: ViewId, label: string) => {
    return (
      <li key={id}>
        <AppLink to={{ view: id }} className="nav-link" aria-current={app.state.view === id ? 'page' : undefined}>
          <Icon name={id} />
          <span className="nav-text">{label}</span>
        </AppLink>
      </li>
    )
  }

  return (
    <nav className="sidenav" aria-label="Configuration sections" ref={navRef}>
      <ul className="nav-list">{ordered.map((s) => item(s.id, s.label))}</ul>
    </nav>
  )
}
