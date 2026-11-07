import type { ReactNode } from 'react'
import { Icon } from './ui'
import { DOC_FALLBACK, DOC_TOPICS, documentationUrl } from '../model/documentation'

export function SectionHeading({ topic, id, focusable, entry, children }: { topic: string; id?: string; focusable?: boolean; entry?: string; children: ReactNode }) {
  return (
    <div className="section-head section-help-heading">
      <h2 id={id} tabIndex={focusable || entry ? -1 : undefined} data-entry={entry}>{children}</h2>
      <DocLink topic={topic} />
    </div>
  )
}

export function DocLink({ topic }: { topic: string }) {
  const doc = DOC_TOPICS[topic] ?? DOC_FALLBACK
  return (
    <a className="section-doc-link" href={documentationUrl(doc.path)} target="_blank" rel="noopener noreferrer" aria-label={`Documentation for ${doc.label} (opens in a new tab)`} title={`Read about ${doc.label} (opens in a new tab)`}>
      <span>Docs</span><Icon name="arrow" />
    </a>
  )
}
