// The frame around every page: the header with the project chip and
// each coding tool's sync state, the harness navigation with each
// section's finding count, and the footer that always states the snapshot
// revision and that runtime enforcement is not observed. The coding tool
// and agent selectors belong to the Authority page, the only view that
// resolves anything per tool and per agent.

import { useEffect, useRef } from 'react'
import type { AuthorityView, OverviewView, SyncState, TargetSupport } from '../contract'
import type { Loaded } from '../api'
import { installedDisplay, type Tone } from '../model/effects'
import { formatTime } from '../model/format'
import { SECTION_VIEWS, type ViewId } from '../model/url'
import { AppLink, useApp } from './app-context'
import { Chip, Icon } from './ui'

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

export function Header({
  authority,
  overview,
  inspectorOpen,
  onToggleInspector,
}: {
  authority: Remote<AuthorityView>
  overview: Remote<OverviewView>
  inspectorOpen: boolean
  onToggleInspector: () => void
}) {
  const app = useApp()
  const view = authority.loaded?.data
  const snapshot = view?.snapshot ?? overview.loaded?.data.snapshot

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

// The coding tool and agent the Authority view resolves effects for.
// Rendered on the Authority page only; every other page is independent
// of both.
export function SubjectControls({ authority, overview }: { authority: Remote<AuthorityView>; overview: Remote<OverviewView> }) {
  const app = useApp()
  const view = authority.loaded?.data
  const tools = view?.harness.available.length
    ? view.harness.available
    : (overview.loaded?.data.targets.map((t) => ({ target: t.target, label: t.label })) ?? [])
  const agents = view?.agents ?? []
  const target = view?.harness.target ?? app.state.target ?? ''
  const agentId = view?.agent.id ?? app.state.agentId ?? ''

  return (
    <div className="page-controls" role="group" aria-label="Resolve for">
      <label className="field">
        <span className="field-label">Coding tool</span>
        <select value={target} onChange={(e) => app.navigate({ target: e.target.value })} disabled={tools.length === 0}>
          {tools.length === 0 && <option value="">{authority.error && overview.error ? 'Unavailable' : 'Loading…'}</option>}
          {tools.map((t) => (
            <option key={t.target} value={t.target}>
              {t.label}
            </option>
          ))}
        </select>
      </label>
      <label className="field">
        <span className="field-label">Agent</span>
        <select value={agentId} onChange={(e) => app.navigate({ agentId: e.target.value })} disabled={agents.length === 0}>
          {agents.length === 0 && <option value="">{authority.error ? 'Unavailable' : 'Loading…'}</option>}
          {agents.map((a) => (
            <option key={a.id} value={a.id}>
              {a.label}
            </option>
          ))}
        </select>
      </label>
    </div>
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

export function Footer({ authority, overview, theme, onToggleTheme }: { authority: Remote<AuthorityView>; overview: Remote<OverviewView>; theme: 'dark' | 'light'; onToggleTheme: () => void }) {
  const view = authority.loaded?.data
  const snapshot = view?.snapshot ?? overview.loaded?.data.snapshot
  const installed = view ? installedDisplay(view.harness.installed.state, view.explanations) : undefined
  return (
    <footer className="footer">
      <p className="footer-facts">
        {snapshot?.revision && (
          <span>
            Revision <code className="code">{snapshot.revision}</code>
          </span>
        )}
        {snapshot?.generatedAt && <span>Built {formatTime(snapshot.generatedAt)}</span>}
        <span>Declared: {snapshot?.evidence.declared === 'loaded' ? 'loaded from the specs' : 'loading'}</span>
        {view && installed && (
          <span title={installed.note}>
            Installed on {view.harness.label}: {installed.text.toLowerCase()}
          </span>
        )}
        <strong className="runtime-note">Runtime enforcement: not observed</strong>
      </p>
      <button type="button" className="button button-quiet" aria-pressed={theme === 'light'} onClick={onToggleTheme}>
        Light theme
      </button>
    </footer>
  )
}
