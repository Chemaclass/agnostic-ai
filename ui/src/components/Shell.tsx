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
import { healthRow, healthTone, sectionSummary } from '../model/rail'

export interface Remote<T> {
  status: 'loading' | 'ready' | 'error'
  loaded?: Loaded<T>
  error?: Error
}

// Each page's name, and the question it answers, which the rail shows under the name.
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

/** Reads `/api/health` for an API source with each overview load, so an edited `requires` shows; other sources have no server to ask.
    `pending` holds until the first answer, so the rail can keep room for what it brings. */
export function useServedConfig(sourceKind: string, loads: number): { served?: ServedConfig; pending: boolean } {
  const [served, setServed] = useState<ServedConfig>()
  const [answered, setAnswered] = useState(false)
  useEffect(() => {
    if (sourceKind !== 'api') return
    let live = true
    fetch(API.health).then((response) => response.ok ? response.json() : undefined).then((health) => {
      const config = parseServedConfig(health)
      if (live && config) setServed(config)
    }).catch(() => undefined).finally(() => {
      if (live) setAnswered(true)
    })
    return () => { live = false }
  }, [sourceKind, loads])
  return { served, pending: sourceKind === 'api' && !answered }
}

function overviewPending(overview: Remote<OverviewView>): boolean {
  return !overview.loaded && overview.status === 'loading'
}

// A placeholder bar that holds the size of text still loading.
function Placeholder() {
  return <span className="skeleton" aria-hidden="true" />
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

/** The project's name, source chip, GitHub link and root, or placeholders of the same size while they load. */
function ProjectName({ overview, served, servedPending }: { overview: Remote<OverviewView>; served: ServedConfig | undefined; servedPending: boolean }) {
  const app = useApp()
  if (overviewPending(overview) || servedPending) {
    return (
      <span className="project-name is-pending" aria-hidden="true">
        <span className="project-title"><Placeholder /></span>
        {app.source.kind === 'api' && <span className="project-root"><Placeholder /></span>}
      </span>
    )
  }
  const snapshot = overview.loaded?.data.snapshot
  const repository = overview.loaded?.data.repository
  const root = served?.root ?? (snapshot?.project.root && snapshot.project.root !== '.' ? snapshot.project.root : undefined)
  const projectName = app.source.kind === 'api' ? apiProjectName(snapshot, served) : snapshot?.project.name === PRODUCT_NAME ? undefined : snapshot?.project.name
  if (!projectName) return null
  return (
    <span className="project-name" title={root ? `${served?.global ? 'User configuration' : 'Project root'}: ${root}` : undefined}>
      <span className="sr-only">{served?.global ? 'User configuration ' : 'Project '}</span>
      <span className="project-title">
        <span className="project-label">{projectName}</span>
        {served && <span className="chip chip-source" aria-hidden="true">{served.global ? 'Global' : 'Project'}</span>}
        {repository && (
          <a className="project-repo" href={repository.url} target="_blank" rel="noopener noreferrer" title={`${repository.name} on GitHub`}>
            <Icon name="github" />
            <span className="sr-only">{repository.name} on GitHub (opens in a new tab)</span>
          </a>
        )}
      </span>
      {root && <code className="project-root"><bdi>{root}</bdi></code>}
    </span>
  )
}

/** Wordmark and project on the rail, or the top bar with the finding chip, Search and Menu on phones. */
export function Header({
  overview,
  served,
  servedPending,
  menuButton,
  menuOpen,
  onOpenMenu,
  onOpenSearch,
}: {
  overview: Remote<OverviewView>
  served: ServedConfig | undefined
  servedPending: boolean
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
        <ProjectName overview={overview} served={served} servedPending={servedPending} />
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
      <span className="release-line">
        <span className="release-version"><span className="sr-only">agnostic-ai </span>{release.label}</span>
        {release.kind === 'release' && <Icon name="arrow" className="release-arrow" />}
      </span>
      {required?.state === 'unmet' && (
        <>
          {' '}
          <span className="release-requires is-unmet" title={required.title}>
            Update needed
            <span className="sr-only">: {required.label}, not met by this version</span>
          </span>
        </>
      )}{' '}
      <span className="sr-only">{release.kind === 'release' ? 'Release notes' : 'Development build'}</span>
    </span>
  )
  if (release.kind === 'build') {
    return <div className="release-badge release-badge-build" title={`${release.label} development build · by Chemaclass`}>{text}</div>
  }
  return (
    <a className="release-badge" href={release.notesUrl} target="_blank" rel="noopener noreferrer" title={`${release.label} release notes · by Chemaclass`}>
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
    question: section.question || DEFAULT_SECTIONS[section.id]?.question || '',
    summary: sectionSummary(section),
  }))
}

/** Setup health, Conversations, then each section: the page name over the question it answers.
    The live counts are each link's tooltip, and its name reads as one phrase: "Context: 1 rule, 3 agents". */
function PageLinks({ overview, pending, onNavigate }: { overview: OverviewView | undefined; pending: boolean; onNavigate?: () => void }) {
  const app = useApp()
  const count = overview ? countFindings(overview) : undefined
  const tone = count ? healthTone(count) : undefined
  const health = count && overview ? healthRow(count, overview.health.gaps.length) : undefined
  const link = (id: ViewId, className: string | undefined, label: string, question: string, summary: string | undefined, status?: string, dot?: string) => {
    const counted = summary && !pending ? summary : undefined
    return (
      <li key={id} className={className}>
        <AppLink to={{ view: id }} className="nav-link" aria-label={counted ? `${label}: ${counted}${status ? ` ${status}` : ''}` : undefined} title={counted} aria-current={app.state.view === id ? 'page' : undefined} onNavigate={onNavigate}>
          {dot && <span className={`nav-dot is-${dot}`} aria-hidden="true" />}
          <Icon name={id} />
          <span className="nav-text">
            <span className="nav-label">{label}</span>
            <span className="nav-question">{question}</span>
          </span>
        </AppLink>
      </li>
    )
  }
  return (
    <ul className="nav-list">
      {link('health', 'nav-item-health', 'Setup health', healthQuestion(count, health?.status), health?.summary, health?.status, tone ?? 'loading')}
      {link('conversations', 'nav-item-conversations', 'Conversations', 'Where did you leave off?', undefined)}
      {sectionLinks(overview).map((section) => link(section.id, undefined, section.label, section.question, section.summary))}
    </ul>
  )
}

/** Setup health's line under its name: its status, with how many findings to fix. */
function healthQuestion(count: FindingCount | undefined, status: string | undefined): string {
  if (!count || !status) return 'Checking your setup'
  const findings = count.errors + count.warnings
  return findings ? `${status}: ${findings} to fix` : status
}

function RailFoot({
  served,
  servedPending,
  theme,
  onToggleTheme,
  onOpenSearch,
  inspectorOpen,
  onToggleInspector,
}: {
  served: ServedConfig | undefined
  servedPending: boolean
  theme: 'dark' | 'light'
  onToggleTheme: () => void
  onOpenSearch?: () => void
  inspectorOpen: boolean
  onToggleInspector: () => void
}) {
  const themeLabel = theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'
  return (
    <footer className="sidenav-footer">
      <div className="sidenav-actions">
        {onOpenSearch && (
          <button type="button" className="sidenav-action button-search" aria-keyshortcuts={APPLE_PLATFORM ? 'Meta+K' : 'Control+K'} onClick={onOpenSearch}>
            <Icon name="search" />
            <span className="sidenav-action-label">Search</span>
            <kbd aria-hidden="true">{SEARCH_SHORTCUT}</kbd>
          </button>
        )}
        {served ? <ConfigSwitch served={served} inRail /> : servedPending && <span className="config-switch is-pending"><Placeholder /></span>}
      </div>
      <div className="sidenav-bar">
        {served?.version
          ? <ReleaseBadge served={served} version={served.version} />
          : servedPending && <span className="release-badge is-pending"><span className="release-text"><Placeholder /></span></span>}
        <div className="sidenav-icons">
          <button type="button" className="sidenav-icon" aria-label={themeLabel} title={themeLabel} onClick={onToggleTheme}>
            <Icon name={theme === 'dark' ? 'sun' : 'moon'} />
          </button>
          <button type="button" className="sidenav-icon sidenav-json" aria-label="Overview JSON" title="Overview JSON: the data behind this page" aria-pressed={inspectorOpen} onClick={onToggleInspector}>
            <Icon name="code" />
          </button>
          <a className="sidenav-icon sidenav-github" href={PRODUCT_REPOSITORY} target="_blank" rel="noopener noreferrer" title={`${PRODUCT_NAME} on GitHub`}>
            <Icon name="github" />
            <span className="sr-only">{PRODUCT_NAME} on GitHub (opens in a new tab)</span>
          </a>
        </div>
      </div>
    </footer>
  )
}

export interface RailProps {
  overview: Remote<OverviewView>
  served: ServedConfig | undefined
  servedPending: boolean
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
        <PageLinks overview={overview.loaded?.data} pending={overviewPending(overview)} />
      </nav>
      <RailFoot {...foot} />
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
              <ProjectName overview={overview} served={foot.served} servedPending={foot.servedPending} />
            </div>
            <button type="button" className="button button-icon menu-sheet-close" aria-label="Close menu" onClick={onClose}>
              <Icon name="close" />
            </button>
          </div>
          <nav aria-label="Sections">
            <PageLinks overview={overview.loaded?.data} pending={overviewPending(overview)} onNavigate={onClose} />
          </nav>
          <RailFoot {...foot} onToggleInspector={() => { onClose(); foot.onToggleInspector() }} />
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
        <p><strong>Static snapshot{revision ? ` · revision ${revision}` : ''}.</strong> <span className="source-notice-detail">Read from {app.source.base} with no server.</span> Opening files, rechecks and switches need <code>agnostic-ai view</code>.</p>
      </Notice>
    )
  }
  if (app.source.kind === 'mock') {
    return (
      <Notice tone="info">
        <p><strong>Mock data.</strong> <span className="source-notice-detail">Handwritten data bundled with the page, not a live scan of a configuration.</span> Opening files, rechecks and switches need <code>agnostic-ai view</code>.</p>
      </Notice>
    )
  }
  return null
}
