import type { ReactNode } from 'react'
import { Icon } from './ui'
import { DOC_FALLBACK, DOC_TOPICS, documentationUrl } from '../model/documentation'

export function SectionHeading({ topic, id, focusable, children }: { topic: string; id?: string; focusable?: boolean; children: ReactNode }) {
  const doc = DOC_TOPICS[topic] ?? DOC_FALLBACK
  return (
    <div className="section-head section-help-heading">
      <h2 id={id} tabIndex={focusable ? -1 : undefined}>{children}</h2>
      <a className="section-doc-link" href={documentationUrl(doc.path)} target="_blank" rel="noopener noreferrer" aria-label={`Documentation for ${doc.label} (opens in a new tab)`} title={`Read about ${doc.label} (opens in a new tab)`}>
        <span>Docs</span><Icon name="arrow" />
      </a>
    </div>
  )
}
