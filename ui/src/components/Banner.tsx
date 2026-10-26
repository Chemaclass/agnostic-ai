// Page-level messages: load errors, source notices, schema warnings.

import type { ReactNode } from 'react'
import { LoadError } from '../api'

export function ErrorPanel({ error }: { error: Error }) {
  const hint = error instanceof LoadError ? error.hint : undefined
  return (
    <div className="banner banner-error" role="alert">
      <p className="banner-title">{error.message}</p>
      {hint && <p>{hint}</p>}
    </div>
  )
}

export function Notice({ children, tone = 'info' }: { children: ReactNode; tone?: 'info' | 'warning' }) {
  return <div className={`banner banner-${tone}`}>{children}</div>
}

export function RefreshFailure({ error, saved, retrying, onRetry }: { error: Error; saved: boolean; retrying: boolean; onRetry: () => void }) {
  const hint = error instanceof LoadError ? error.hint : undefined
  return (
    <div className="banner banner-error" role="alert">
      <p className="banner-title">{saved ? 'Your change was saved, but the overview could not be refreshed.' : 'The overview could not be refreshed.'}</p>
      <p>{error.message}</p>
      {hint && <p>{hint}</p>}
      <p>The page below shows the previous overview and may not match the files on disk. Editing is off until a refresh succeeds.</p>
      <button type="button" className="button" disabled={retrying} onClick={onRetry}>{retrying ? 'Retrying…' : 'Retry'}</button>
    </div>
  )
}
