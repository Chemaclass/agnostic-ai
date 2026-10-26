import { useEffect, useRef, useState } from 'react'
import { API, type OverviewView } from '../contract'
import type { Loaded } from '../api'
import { SECTION_VIEWS, type ViewId } from '../model/url'
import { AppLink, useApp } from './app-context'
import { APPLE_PLATFORM, Icon, ToolAvatar } from './ui'

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

const SECTION_DESCRIPTIONS: Record<string, string> = {
  context: 'Instructions, rules & skills',
  authority: 'Permissions & boundaries',
  tools: 'Servers & available tools',
  lifecycle: 'Hooks through a session',
  settings: 'Built-ins & preferences',
}

const PRODUCT_NAME = 'agnostic-ai'

export function Header({
  overview,
  inspectorOpen,
  onToggleInspector,
  theme,
  onToggleTheme,
  onOpenSearch,
}: {
  overview: Remote<OverviewView>
  inspectorOpen: boolean
  onToggleInspector: () => void
  theme: 'dark' | 'light'
  onToggleTheme: () => void
  onOpenSearch: () => void
}) {
  const app = useApp()
  const snapshot = overview.loaded?.data.snapshot
  const projectName = snapshot?.project.name === PRODUCT_NAME ? undefined : snapshot?.project.name
  const [served, setServed] = useState<{ root: string; global: boolean }>()
  useEffect(() => {
    if (app.source.kind !== 'api') return
    let live = true
    fetch(API.health).then((response) => response.ok ? response.json() : undefined).then((health) => {
      if (live && health && typeof health.root === 'string') setServed({ root: health.root, global: health.global === true })
    }).catch(() => undefined)
    return () => { live = false }
  }, [app.source.kind])
  const root = served?.root ?? (snapshot?.project.root && snapshot.project.root !== '.' ? snapshot.project.root : undefined)

  return (
    <header className="topbar">
      <div className="brand">
        <svg className="logo" viewBox="0 0 64 64" aria-hidden="true" focusable="false">
          <path d="M43 32H50.5M39.78 39.78L45.08 45.08M32 43V50.5M24.22 39.78L18.92 45.08M21 32H13.5M24.22 24.22L18.92 18.92M32 21V13.5M39.78 24.22L45.08 18.92" fill="none" stroke="currentColor" strokeWidth="3.5" strokeLinecap="round" />
          <circle cx="32" cy="32" r="8" fill="currentColor" />
          <g fill="currentColor">
            <circle cx="55" cy="32" r="4.5" /><circle cx="32" cy="55" r="4.5" />
            <circle cx="9" cy="32" r="4.5" /><circle cx="32" cy="9" r="4.5" />
          </g>
          <g fill="var(--text-muted)">
            <circle cx="48.26" cy="48.26" r="4.5" /><circle cx="15.74" cy="48.26" r="4.5" />
            <circle cx="15.74" cy="15.74" r="4.5" /><circle cx="48.26" cy="15.74" r="4.5" />
          </g>
        </svg>
        <span className="brand-name">{PRODUCT_NAME}</span>
      </div>
      {(projectName || app.source.kind === 'static' || app.source.kind === 'mock') && (
        <div className="topbar-context">
          {projectName && (
            <span className="project-name" title={root ? `${served?.global ? 'User configuration' : 'Project root'}: ${root}` : undefined}>
              <span className="sr-only">{served?.global ? 'User configuration ' : 'Project '}</span>
              <span className="project-title">{projectName}{served?.global && <span className="chip chip-source">Global</span>}</span>
              {root && <code className="project-root"><bdi>{root}</bdi></code>}
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
      )}
      <button type="button" className="button button-search" aria-keyshortcuts={APPLE_PLATFORM ? 'Meta+K' : 'Control+K'} onClick={onOpenSearch}>
        <Icon name="search" />
        <span>Search</span>
        <kbd aria-hidden="true">{APPLE_PLATFORM ? '⌘K' : 'Ctrl K'}</kbd>
      </button>
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
          <span className="nav-text">{label}<span className="nav-description">{SECTION_DESCRIPTIONS[id]}</span></span>
        </AppLink>
      </li>
    )
  }

  return (
    <nav className="sidenav" aria-label="Configuration sections" ref={navRef}>
      <ul className="nav-list">{ordered.map((s) => item(s.id, s.label))}</ul>
      {overview.loaded && (
        <div className="nav-project">
          <span className="nav-project-label">Configured tools</span>
          <div className="nav-targets">
            {overview.loaded.data.targets.map((target) => <ToolAvatar key={target.target} target={target.target} label={target.label} />)}
            {overview.loaded.data.targets.length === 0 && <span className="muted">None selected</span>}
          </div>
          <p>{app.source.kind === 'api' ? 'Changes are saved to your source configuration.' : 'Read-only configuration snapshot.'}</p>
        </div>
      )}
    </nav>
  )
}
