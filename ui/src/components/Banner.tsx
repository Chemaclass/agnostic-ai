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
