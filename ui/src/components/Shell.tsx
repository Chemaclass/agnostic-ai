// The frame around every page: the header with the project chip and
// each coding tool's sync state, the harness navigation with each
// section's finding count, and the footer with the snapshot revision.

import { useEffect, useRef } from 'react'
import type { OverviewView, SyncState, TargetSupport } from '../contract'
import type { Loaded } from '../api'
import { formatTime } from '../model/format'
import { SECTION_VIEWS, type ViewId } from '../model/url'
import { AppLink, useApp } from './app-context'
import { Chip, Icon, type Tone } from './ui'

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

const SYNC_WORDING: Record<SyncState, string> = {
  'in-sync': 'in sync',
  drifted: 'out of sync',
  'not-checked': 'not checked',
}

const SYNC_TONE: Record<SyncState, Tone> = {
  'in-sync': 'permitted',
  drifted: 'approval',
  'not-checked': 'neutral',
}

// SyncChips states, per coding tool, whether its generated files match
// what sync would write. A drifted tool names the differing files.
function SyncChips({ targets }: { targets: TargetSupport[] }) {
  return (
    <>
      {targets.map((t) => {
        const files = t.sync.files.map((f) => `${f.path} (${f.state})`).join('\n')
        return (
          <Chip key={t.target} tone={SYNC_TONE[t.sync.state]} title={files ? `Differs from what sync would write:\n${files}` : undefined}>
            {t.label} · {SYNC_WORDING[t.sync.state]}
          </Chip>
        )
      })}
    </>
  )
}

export function Header({ overview, inspectorOpen, onToggleInspector }: { overview: Remote<OverviewView>; inspectorOpen: boolean; onToggleInspector: () => void }) {
  const app = useApp()
  const snapshot = overview.loaded?.data.snapshot

  return (
    <header className="topbar">
      <div className="brand">
        <span className="logo" aria-hidden="true">
          A
        </span>
        <span className="brand-name">Agnostic AI</span>
      </div>
      <div className="topbar-chips">
        {snapshot?.project.name && (
          <span className="chip chip-project" title={snapshot.project.root ? `Project root: ${snapshot.project.root}` : undefined}>
            <span className="sr-only">Project </span>
            {snapshot.project.name}
          </span>
        )}
        {overview.loaded && <SyncChips targets={overview.loaded.data.targets} />}
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

  // Each section shows how many check findings it holds.
  const findingCount = (id: string) => {
    const section = sections?.find((s) => s.id === id)
    if (!section) return 0
    return section.findings.length + section.items.reduce((n, it) => n + it.findings.length, 0)
  }

  const item = (id: ViewId, label: string) => {
    const findings = findingCount(id)
    return (
      <li key={id}>
        <AppLink to={{ view: id }} className="nav-link" aria-current={app.state.view === id ? 'page' : undefined}>
          <Icon name={id} />
          <span className="nav-text">{label}</span>
          {findings > 0 && (
            <span className="nav-count">
              {findings}
              <span className="sr-only"> {findings === 1 ? 'finding' : 'findings'}</span>
            </span>
          )}
        </AppLink>
      </li>
    )
  }

  return (
    <nav className="sidenav" aria-label="Harness" ref={navRef}>
      <ul className="nav-list">{ordered.map((s) => item(s.id, s.label))}</ul>
    </nav>
  )
}

export function Footer({ overview, theme, onToggleTheme }: { overview: Remote<OverviewView>; theme: 'dark' | 'light'; onToggleTheme: () => void }) {
  const snapshot = overview.loaded?.data.snapshot
  return (
    <footer className="footer">
      <p className="footer-facts">
        {snapshot?.revision && (
          <span>
            Revision <code className="code">{snapshot.revision}</code>
          </span>
        )}
        {snapshot?.generatedAt && <span>Built {formatTime(snapshot.generatedAt)}</span>}
      </p>
      <button type="button" className="button button-quiet" aria-pressed={theme === 'light'} onClick={onToggleTheme}>
        Light theme
      </button>
    </footer>
  )
}
