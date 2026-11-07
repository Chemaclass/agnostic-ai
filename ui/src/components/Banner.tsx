// Page-level messages: load errors, refresh failures, saved states.

import { useEffect, useState, type ReactNode } from 'react'
import { LoadError } from '../api'
import { useApp } from './app-context'
import { CopyButton } from './ui'

export type NoticeTone = 'info' | 'success' | 'warning' | 'error'

export function Notice({ tone = 'info', title, children, details, actions, onDismiss }: {
  tone?: NoticeTone
  title?: ReactNode
  children?: ReactNode
  /** Raw text for a reader who needs it, such as a server message; folded behind Details with a copy button. */
  details?: string
  actions?: ReactNode
  onDismiss?: () => void
}) {
  return (
    <div className={`banner banner-${tone}`} role={tone === 'error' ? 'alert' : undefined}>
      {title && <p className="banner-title">{title}</p>}
      {children}
      {details && (
        <details className="banner-details">
          <summary>Details</summary>
          <pre>{details}</pre>
          <CopyButton text={details} label="Copy details" />
        </details>
      )}
      {(actions || onDismiss) && (
        <div className="banner-actions">
          {actions}
          {onDismiss && <button type="button" className="button button-quiet" onClick={onDismiss}>Dismiss</button>}
        </div>
      )}
    </div>
  )
}

function errorDetails(error: Error): string {
  const hint = error instanceof LoadError ? error.hint : undefined
  return hint ? `${error.message}\n${hint}` : error.message
}

/** A failed load. Without a title or retry it describes the overview, which is what most pages load. */
export function ErrorPanel({ error, title, onRetry }: { error: Error; title?: string; onRetry?: () => void }) {
  const app = useApp()
  const [retrying, setRetrying] = useState(false)
  useEffect(() => setRetrying(false), [error])
  // A source the client refuses fails the same way on every retry, and its reason is the fix.
  if (!title && app.source.kind === 'invalid') return <Notice tone="error" title={error.message} details={errorDetails(error)} />
  const retry = onRetry ?? (() => app.reload())
  const heading = title ?? (app.source.kind === 'api' ? 'Could not load the overview' : 'Could not load the snapshot')
  const action = (
    <button type="button" className="button" disabled={retrying} onClick={() => { setRetrying(true); retry() }}>{retrying ? 'Retrying…' : 'Retry'}</button>
  )
  return <Notice tone="error" title={heading} details={errorDetails(error)} actions={action} />
}

export function RefreshFailure({ error, saved, retrying, onRetry }: { error: Error; saved: boolean; retrying: boolean; onRetry: () => void }) {
  return (
    <Notice
      tone="error"
      title={saved ? 'Your change was saved, but the overview could not be refreshed.' : 'The overview could not be refreshed.'}
      details={errorDetails(error)}
      actions={<button type="button" className="button" disabled={retrying} onClick={onRetry}>{retrying ? 'Retrying…' : 'Retry'}</button>}
    >
      <p>The page below shows the previous overview and may not match the files on disk. Editing is off until a refresh succeeds.</p>
    </Notice>
  )
}
