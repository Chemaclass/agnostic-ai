// Navigation and app-wide services, shared through React context so
// pages do not thread them through every prop list.

import { createContext, useContext, type AnchorHTMLAttributes, type MouseEvent, type ReactNode } from 'react'
import type { DataSource, UrlState } from '../model/url'

export interface AppApi {
  state: UrlState
  source: DataSource
  navigate(patch: Partial<UrlState>, options?: { replace?: boolean }): void
  href(patch: Partial<UrlState>): string
  /** Loads the overview again, after an edit. */
  reload(): void
}

export const AppContext = createContext<AppApi | null>(null)

export function useApp(): AppApi {
  const app = useContext(AppContext)
  if (!app) throw new Error('useApp needs an AppContext provider')
  return app
}

function isPlainClick(event: MouseEvent): boolean {
  return !event.defaultPrevented && event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey
}

/** An in-app link: a real href for new tabs, client navigation for plain clicks. */
export function AppLink({ to, children, onNavigate, ...rest }: { to: Partial<UrlState>; children: ReactNode; onNavigate?: () => void } & Omit<AnchorHTMLAttributes<HTMLAnchorElement>, 'href'>) {
  const app = useApp()
  return (
    <a
      {...rest}
      href={app.href(to)}
      onClick={(event) => {
        rest.onClick?.(event)
        if (!isPlainClick(event)) return
        event.preventDefault()
        onNavigate?.()
        app.navigate(to)
      }}
    >
      {children}
    </a>
  )
}

export function PageHeader({ eyebrow, title, subtitle, children }: { eyebrow?: ReactNode; title: ReactNode; subtitle?: ReactNode; children?: ReactNode }) {
  return (
    <header className="page-header">
      {eyebrow && <div className="eyebrow">{eyebrow}</div>}
      <h1 id="page-title" tabIndex={-1}>
        {title}
      </h1>
      {subtitle && <p className="page-subtitle">{subtitle}</p>}
      {children}
    </header>
  )
}
