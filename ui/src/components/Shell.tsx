import { useEffect, useRef, useState, type RefObject } from 'react'
import { API, type OverviewView } from '../contract'
import type { Loaded } from '../api'
import { baseName, parseServedConfig, type ServedConfig } from '../model/config-switch'
import { ConfigSwitch } from './ConfigSwitch'
import { SECTION_VIEWS, type ViewId } from '../model/url'
import { AppLink, useApp } from './app-context'
import { APPLE_PLATFORM, Icon } from './ui'
import { requiredRelease, servingRelease } from '../model/release'
import { countFindings, healthSummary } from '../model/health'

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
  health: 'Findings to fix, errors first',
}

const PRODUCT_NAME = 'agnostic-ai'

// The header and the navigation stick over the scrolling document; their
// heights wrap with the window, so the layout reads them from these variables.
function useHeightVariable(ref: RefObject<HTMLElement | null>, name: string) {
  useEffect(() => {
    const element = ref.current
    if (!element || typeof ResizeObserver === 'undefined') return
    const root = document.documentElement.style
    const observer = new ResizeObserver(() => root.setProperty(name, `${element.getBoundingClientRect().height}px`))
    observer.observe(element)
    return () => {
      observer.disconnect()
      root.removeProperty(name)
    }
  }, [ref, name])
}

/** Reads `/api/health` for an API source with each overview load, so an edited `requires` shows; other sources have no server to ask. */
export function useServedConfig(sourceKind: string, loads: number): ServedConfig | undefined {
  const [served, setServed] = useState<ServedConfig>()
  useEffect(() => {
    if (sourceKind !== 'api') return
    let live = true
    fetch(API.health).then((response) => response.ok ? response.json() : undefined).then((health) => {
      const config = parseServedConfig(health)
      if (live && config) setServed(config)
    }).catch(() => undefined)
    return () => { live = false }
  }, [sourceKind, loads])
  return served
}

function LogoMark({ className }: { className: string }) {
  return (
    <svg className={className} viewBox="0 0 64 64" aria-hidden="true" focusable="false">
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
  )
}

// Counts by severity, linking to the Setup health view; nothing shows until the overview loads.
function HealthBadge({ overview }: { overview: OverviewView | undefined }) {
  const app = useApp()
  if (!overview) return null
  const count = countFindings(overview)
  const summary = healthSummary(count)
  const tone = count.errors ? 'is-error' : count.warnings ? 'is-warning' : 'is-clean'
  return (
    <AppLink to={{ view: 'health' }} className={`health-badge ${tone}`} aria-label={`Setup health: ${summary}`} title={`Setup health: ${summary}`} aria-current={app.state.view === 'health' ? 'page' : undefined}>
      <Icon name={count.errors || count.warnings ? 'warning' : 'health'} />
      {count.errors > 0 && <span className="health-count is-error">{count.errors}<span className="health-count-letter" aria-hidden="true">E</span><span className="health-count-word"> {count.errors === 1 ? 'error' : 'errors'}{count.warnings ? ',' : ''}</span></span>}
      {count.errors > 0 && count.warnings > 0 && ' '}
      {count.warnings > 0 && <span className="health-count is-warning">{count.warnings}<span className="health-count-letter" aria-hidden="true">W</span><span className="health-count-word"> {count.warnings === 1 ? 'warning' : 'warnings'}</span></span>}
      {!count.errors && !count.warnings && <span className="health-count-word">{summary}</span>}
    </AppLink>
  )
}

function apiProjectName(snapshot: { project: { name?: string } } | undefined, served: ServedConfig | undefined): string | undefined {
  if (!snapshot) return undefined
  if (snapshot.project.name) return snapshot.project.name
  if (!served || served.global) return undefined
  const folder = baseName(served.root)
  return folder === '' || /^[\\/]+$/.test(folder) ? undefined : folder
}

export function Header({
  overview,
  served,
  theme,
  onToggleTheme,
  onOpenSearch,
}: {
  overview: Remote<OverviewView>
  served: ServedConfig | undefined
  theme: 'dark' | 'light'
  onToggleTheme: () => void
  onOpenSearch: () => void
}) {
  const app = useApp()
  const headerRef = useRef<HTMLElement>(null)
  useHeightVariable(headerRef, '--topbar-offset')
  const snapshot = overview.loaded?.data.snapshot
  const root = served?.root ?? (snapshot?.project.root && snapshot.project.root !== '.' ? snapshot.project.root : undefined)
  const projectName = app.source.kind === 'api' ? apiProjectName(snapshot, served) : snapshot?.project.name === PRODUCT_NAME ? undefined : snapshot?.project.name

  return (
    <header className="topbar" ref={headerRef}>
      <div className="brand">
        <LogoMark className="logo" />
        <span className="brand-name">{PRODUCT_NAME}</span>
      </div>
      {(projectName || app.source.kind === 'static' || app.source.kind === 'mock') && (
        <div className="topbar-context">
          {projectName && (
            <span className="project-name" title={root ? `${served?.global ? 'User configuration' : 'Project root'}: ${root}` : undefined}>
              <span className="sr-only">{served?.global ? 'User configuration ' : 'Project '}</span>
              <span className="project-title"><span className="project-label">{projectName}</span>{served && <span className="chip chip-source" aria-hidden="true">{served.global ? 'Global' : 'Project'}</span>}</span>
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
      <div className="topbar-actions">
        <HealthBadge overview={overview.loaded?.data} />
        <button type="button" className="button button-search" aria-keyshortcuts={APPLE_PLATFORM ? 'Meta+K' : 'Control+K'} onClick={onOpenSearch}>
          <Icon name="search" />
          <span className="button-search-label">Search</span>
          <kbd aria-hidden="true">{APPLE_PLATFORM ? '⌘K' : 'Ctrl K'}</kbd>
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
      </div>
    </header>
  )
}

// The release badge sits at the bottom of the sidebar. A published
// release links its notes; a checkout build shows its label as it is.
function ReleaseBadge({ served, version }: { served: ServedConfig; version: string }) {
  const release = servingRelease(version)
  const required = requiredRelease(served)
  const text = (
    <span className="release-text">
      <span className="release-version"><span className="sr-only">agnostic-ai </span>{release.label}</span>{' '}
      {required && (
        <>
          <span className={`release-requires is-${required.state}`} title={required.title}>
            {required.label}
            {required.state === 'unmet' && <span className="sr-only">, not met by this version</span>}
          </span>{' '}
        </>
      )}
      <span className="release-caption">{release.kind === 'release' ? 'Release notes' : 'Development build'}</span>
    </span>
  )
  if (release.kind === 'build') {
    return (
      <div className="release-badge release-badge-build" title={`agnostic-ai ${release.label}`}>
        <LogoMark className="release-mark" />
        {text}
      </div>
    )
  }
  return (
    <a className="release-badge" href={release.notesUrl} target="_blank" rel="noopener noreferrer" title={`Read the agnostic-ai ${release.label} release notes (opens in a new tab)`}>
      <LogoMark className="release-mark" />
      {text}
      <Icon name="arrow" className="release-arrow" />
      <span className="sr-only"> (opens in a new tab)</span>
    </a>
  )
}

export function Nav({
  overview,
  served,
  inspectorOpen,
  onToggleInspector,
}: {
  overview: Remote<OverviewView>
  served: ServedConfig | undefined
  inspectorOpen: boolean
  onToggleInspector: () => void
}) {
  const app = useApp()
  const asideRef = useRef<HTMLElement>(null)
  useHeightVariable(asideRef, '--nav-offset')
  const sections = overview.loaded?.data.sections
  const ordered = sections?.length
    ? [...sections].sort((a, b) => a.order - b.order).map((s) => ({ id: s.id as ViewId, label: s.label }))
    : SECTION_VIEWS.map((id) => ({ id: id as ViewId, label: DEFAULT_SECTION_LABELS[id] }))

  const item = (id: ViewId, label: string) => {
    return (
      <li key={id} className={id === 'health' ? 'nav-item-health' : undefined}>
        <AppLink to={{ view: id }} className="nav-link" aria-current={app.state.view === id ? 'page' : undefined}>
          <Icon name={id} />
          <span className="nav-text">{label}<span className="nav-description">{SECTION_DESCRIPTIONS[id]}</span></span>
        </AppLink>
      </li>
    )
  }

  return (
    <aside className="sidenav" ref={asideRef}>
      <nav className="section-nav" aria-label="Configuration sections">
        <ul className="nav-list">{ordered.map((s) => item(s.id, s.label))}{item('health', 'Setup health')}</ul>
      </nav>
      <footer className="sidenav-footer">
        {served && <ConfigSwitch served={served} />}
        {served?.version && <ReleaseBadge served={served} version={served.version} />}
        <button type="button" className="sidenav-json" aria-pressed={inspectorOpen} onClick={onToggleInspector}>
          Overview JSON
        </button>
      </footer>
    </aside>
  )
}
