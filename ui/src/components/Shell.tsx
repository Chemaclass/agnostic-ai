import { useEffect, useRef, useState, type RefObject } from 'react'
import { API, type OverviewView } from '../contract'
import type { Loaded } from '../api'
import { baseName, parseServedConfig, type ServedConfig } from '../model/config-switch'
import { ConfigSwitch } from './ConfigSwitch'
import { Notice } from './Banner'
import { SECTION_VIEWS, type ViewId } from '../model/url'
import { AppLink, useApp } from './app-context'
import { APPLE_PLATFORM, Icon } from './ui'
import { requiredRelease, servingRelease } from '../model/release'
import { countFindings, healthSummary, type FindingCount } from '../model/health'
import { HEALTH_STATUS, healthTone, sectionSummary, type HealthTone } from '../model/rail'

export interface Remote<T> {
  status: 'loading' | 'ready' | 'error'
  loaded?: Loaded<T>
  error?: Error
}

const DEFAULT_SECTIONS: Record<string, { label: string; question: string }> = {
  context: { label: 'Context', question: 'What is the agent told?' },
  authority: { label: 'Authority', question: 'What may the agent do?' },
  tools: { label: 'Tools', question: 'What can the agent use?' },
  lifecycle: { label: 'Lifecycle', question: 'What runs around the agent, and when?' },
  settings: { label: 'Settings', question: 'How are your tools set up?' },
}

const PRODUCT_NAME = 'agnostic-ai'
const PRODUCT_REPOSITORY = 'https://github.com/Chemaclass/agnostic-ai'
// At this width and below the rail becomes a top bar and a row of page links.
const COMPACT_SHELL = '(max-width: 900px)'
const SEARCH_SHORTCUT = APPLE_PLATFORM ? '⌘K' : 'Ctrl K'

function compactShell(): boolean {
  return window.matchMedia?.(COMPACT_SHELL).matches ?? false
}

// The phone top bar and page-link row stick over the scrolling document and
// their heights wrap, so sticky page parts read them from these variables.
// On wide screens they sit in the rail and offset nothing.
function useHeightVariable(ref: RefObject<HTMLElement | null>, name: string) {
  useEffect(() => {
    const element = ref.current
    if (!element || typeof ResizeObserver === 'undefined') return
    const root = document.documentElement.style
    const measure = () => root.setProperty(name, compactShell() ? `${element.getBoundingClientRect().height}px` : '0px')
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    const query = window.matchMedia?.(COMPACT_SHELL)
    query?.addEventListener('change', measure)
    return () => {
      observer.disconnect()
      query?.removeEventListener('change', measure)
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

// Counts by severity for the phone top bar, linking to Setup health; nothing shows until the overview loads.
function HealthBadge({ overview }: { overview: OverviewView | undefined }) {
  const app = useApp()
  if (!overview) return null
  const count = countFindings(overview)
  const summary = healthSummary(count)
  const tone = healthTone(count)
  return (
    <AppLink to={{ view: 'health' }} className={`health-badge is-${tone}`} aria-label={`Setup health: ${summary}`} title={`Setup health: ${summary}`} aria-current={app.state.view === 'health' ? 'page' : undefined}>
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

function ProjectName({ overview, served }: { overview: Remote<OverviewView>; served: ServedConfig | undefined }) {
  const app = useApp()
  const snapshot = overview.loaded?.data.snapshot
  const root = served?.root ?? (snapshot?.project.root && snapshot.project.root !== '.' ? snapshot.project.root : undefined)
  const projectName = app.source.kind === 'api' ? apiProjectName(snapshot, served) : snapshot?.project.name === PRODUCT_NAME ? undefined : snapshot?.project.name
  if (!projectName) return null
  return (
    <span className="project-name" title={root ? `${served?.global ? 'User configuration' : 'Project root'}: ${root}` : undefined}>
      <span className="sr-only">{served?.global ? 'User configuration ' : 'Project '}</span>
      <span className="project-title"><span className="project-label">{projectName}</span>{served && <span className="chip chip-source" aria-hidden="true">{served.global ? 'Global' : 'Project'}</span>}</span>
      {root && <code className="project-root"><bdi>{root}</bdi></code>}
    </span>
  )
}

/** Wordmark and project on the rail, or the top bar with the finding chip, Search and Menu on phones. */
export function Header({
  overview,
  served,
  menuButton,
  menuOpen,
  onOpenMenu,
  onOpenSearch,
}: {
  overview: Remote<OverviewView>
  served: ServedConfig | undefined
  menuButton: RefObject<HTMLButtonElement | null>
  menuOpen: boolean
  onOpenMenu: () => void
  onOpenSearch: () => void
}) {
  const headerRef = useRef<HTMLElement>(null)
  useHeightVariable(headerRef, '--topbar-offset')
  return (
    <header className="topbar" ref={headerRef} data-busy={overview.status === 'loading' || undefined}>
      <div className="brand">
        <span className="brand-name">{PRODUCT_NAME}</span>
        <ProjectName overview={overview} served={served} />
      </div>
      <div className="topbar-actions">
        <HealthBadge overview={overview.loaded?.data} />
        <button type="button" className="button button-icon" aria-label="Search" aria-keyshortcuts={APPLE_PLATFORM ? 'Meta+K' : 'Control+K'} title={`Search (${SEARCH_SHORTCUT})`} onClick={onOpenSearch}>
          <Icon name="search" />
        </button>
        <button ref={menuButton} type="button" className="button button-icon" aria-label="Menu" aria-haspopup="dialog" aria-expanded={menuOpen} onClick={onOpenMenu}>
          <Icon name="menu" />
        </button>
      </div>
    </header>
  )
}

// A published release links its notes; a checkout build shows its label as it is.
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
      <span className="release-caption">
        {release.kind === 'release' ? 'Release notes' : 'Development build'}
        {release.kind === 'release' && <Icon name="arrow" className="release-arrow" />}
      </span>
    </span>
  )
  if (release.kind === 'build') {
    return <div className="release-badge release-badge-build" title={`agnostic-ai ${release.label}`}>{text}</div>
  }
  return (
    <a className="release-badge" href={release.notesUrl} target="_blank" rel="noopener noreferrer" title={`Read the agnostic-ai ${release.label} release notes (opens in a new tab)`}>
      {text}
      <span className="sr-only"> (opens in a new tab)</span>
    </a>
  )
}

interface SectionLink {
  id: ViewId
  label: string
  question: string
  summary?: string
}

function sectionLinks(overview: OverviewView | undefined): SectionLink[] {
  const sections = overview?.sections
  if (!sections?.length) return SECTION_VIEWS.map((id) => ({ id, ...DEFAULT_SECTIONS[id] }))
  return [...sections].sort((a, b) => a.order - b.order).map((section) => ({
    id: section.id as ViewId,
    label: section.label,
    question: section.question || DEFAULT_SECTIONS[section.id]?.question || section.label,
    summary: sectionSummary(section),
  }))
}

/** Conversations, Setup health as a status row, then each section: its name over its question. A section's live summary, such as "1 rule, 3 agents", is its tooltip; Setup health shows its count in its question. */
function PageLinks({ overview, onNavigate }: { overview: OverviewView | undefined; onNavigate?: () => void }) {
  const app = useApp()
  const count = overview ? countFindings(overview) : undefined
  const tone = count ? healthTone(count) : undefined
  const link = (id: ViewId, className: string | undefined, label: string, question: string, summary: string | undefined, dot?: string) => (
    <li key={id} className={className}>
      <AppLink to={{ view: id }} className="nav-link" aria-current={app.state.view === id ? 'page' : undefined} onNavigate={onNavigate} title={summary}>
        {dot && <span className={`nav-dot is-${dot}`} aria-hidden="true" />}
        <Icon name={id} />
        <span className="nav-text">
          <span className="nav-label">{label}</span>
          <span className="nav-question">{question}</span>
        </span>
      </AppLink>
    </li>
  )
  return (
    <ul className="nav-list">
      {link('conversations', 'nav-item-conversations', 'Conversations', 'Where did you leave off?', undefined)}
      {link('health', 'nav-item-health', 'Setup health', healthQuestion(tone, count), undefined, tone ?? 'loading')}
      {sectionLinks(overview).map((section) => link(section.id, undefined, section.label, section.question, section.summary))}
    </ul>
  )
}

/** Setup health's line under its name: the status, and what it counts unless all is clear. */
function healthQuestion(tone: HealthTone | undefined, count: FindingCount | undefined): string {
  if (!tone || !count) return 'Checking your setup'
  if (tone === 'clean') return HEALTH_STATUS.clean
  return `${HEALTH_STATUS[tone]}: ${healthSummary(count)}`
}

function RailFoot({
  overview,
  served,
  theme,
  onToggleTheme,
  onOpenSearch,
  inspectorOpen,
  onToggleInspector,
}: {
  overview: OverviewView | undefined
  served: ServedConfig | undefined
  theme: 'dark' | 'light'
  onToggleTheme: () => void
  onOpenSearch?: () => void
  inspectorOpen: boolean
  onToggleInspector: () => void
}) {
  const repository = overview?.repository
  const themeLabel = theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'
  return (
    <footer className="sidenav-footer">
      {onOpenSearch && (
        <button type="button" className="button button-search" aria-keyshortcuts={APPLE_PLATFORM ? 'Meta+K' : 'Control+K'} onClick={onOpenSearch}>
          <Icon name="search" />
          <span className="button-search-label">Search</span>
          <kbd aria-hidden="true">{SEARCH_SHORTCUT}</kbd>
        </button>
      )}
      {served && <ConfigSwitch served={served} />}
      <div className="sidenav-row">
        <button type="button" className="button button-quiet sidenav-theme" aria-label={themeLabel} title={themeLabel} onClick={onToggleTheme}>
          <Icon name={theme === 'dark' ? 'sun' : 'moon'} />
          <span aria-hidden="true">{theme === 'dark' ? 'Light theme' : 'Dark theme'}</span>
        </button>
        <span className="sidenav-links">
          {repository && (
            <a className="button button-quiet button-icon" href={repository.url} target="_blank" rel="noopener noreferrer" title={`${repository.name} on GitHub`}>
              <Icon name="github" />
              <span className="sr-only">{repository.name} on GitHub (opens in a new tab)</span>
            </a>
          )}
          <a className="sidenav-github" href={PRODUCT_REPOSITORY} target="_blank" rel="noopener noreferrer" title={`${PRODUCT_NAME} on GitHub`}>
            <Icon name="github" />
            <span className="sidenav-github-label">{PRODUCT_NAME} on GitHub</span>
            <span className="sr-only"> (opens in a new tab)</span>
          </a>
        </span>
      </div>
      <div className="sidenav-row">
        {served?.version && <ReleaseBadge served={served} version={served.version} />}
        <button type="button" className="sidenav-json" aria-pressed={inspectorOpen} onClick={onToggleInspector}>
          Overview JSON
        </button>
      </div>
    </footer>
  )
}

export interface RailProps {
  overview: Remote<OverviewView>
  served: ServedConfig | undefined
  theme: 'dark' | 'light'
  onToggleTheme: () => void
  onOpenSearch: () => void
  inspectorOpen: boolean
  onToggleInspector: () => void
}

/** The page links and the rail foot; at 900px and below only the page links show, as one row. */
export function Nav({ overview, ...foot }: RailProps) {
  const asideRef = useRef<HTMLElement>(null)
  useHeightVariable(asideRef, '--nav-offset')
  return (
    <aside className="sidenav" ref={asideRef}>
      <nav className="section-nav" aria-label="Configuration sections">
        <PageLinks overview={overview.loaded?.data} />
      </nav>
      <RailFoot overview={overview.loaded?.data} {...foot} />
    </aside>
  )
}

/** The phone Menu: the rail's questions, summaries and foot in a sheet from the left. */
export function MenuSheet({ open, onClose, overview, onOpenSearch: _search, ...foot }: RailProps & { open: boolean; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const element = dialog.current
    if (!element) return
    if (open && !element.open) element.showModal()
    if (!open && element.open) element.close()
  }, [open])
  useEffect(() => {
    if (!open) return
    const query = window.matchMedia?.(COMPACT_SHELL)
    const onChange = () => { if (!query?.matches) onClose() }
    query?.addEventListener('change', onChange)
    return () => query?.removeEventListener('change', onChange)
  }, [open, onClose])
  return (
    <dialog ref={dialog} className="menu-sheet" aria-label="Menu" onClose={onClose} onClick={(event) => { if (event.target === event.currentTarget) onClose() }}>
      {open && (
        <div className="menu-sheet-body">
          <div className="menu-sheet-head">
            <div className="brand">
              <span className="brand-name">{PRODUCT_NAME}</span>
              <ProjectName overview={overview} served={foot.served} />
            </div>
            <button type="button" className="button button-icon menu-sheet-close" aria-label="Close menu" onClick={onClose}>
              <Icon name="close" />
            </button>
          </div>
          <nav aria-label="Sections">
            <PageLinks overview={overview.loaded?.data} onNavigate={onClose} />
          </nav>
          <RailFoot overview={overview.loaded?.data} {...foot} onToggleInspector={() => { onClose(); foot.onToggleInspector() }} />
        </div>
      )}
    </dialog>
  )
}

/** Says once, at the top of the page, that the data is not a live scan. */
export function SourceNotice({ overview }: { overview: Remote<OverviewView> }) {
  const app = useApp()
  if (app.source.kind === 'static') {
    const revision = overview.loaded?.data.snapshot.revision
    return (
      <Notice tone="info">
        <p><strong>Static snapshot{revision ? ` · revision ${revision}` : ''}.</strong> Read from {app.source.base} with no server. Opening files, rechecks and switches need <code>agnostic-ai view</code>.</p>
      </Notice>
    )
  }
  if (app.source.kind === 'mock') {
    return (
      <Notice tone="info">
        <p><strong>Mock data.</strong> Handwritten data bundled with the page, not a live scan of a configuration. Opening files, rechecks and switches need <code>agnostic-ai view</code>.</p>
      </Notice>
    )
  }
  return null
}
