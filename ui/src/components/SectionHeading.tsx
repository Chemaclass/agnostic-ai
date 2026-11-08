import type { ReactNode } from 'react'
import { Icon } from './ui'
import { DOC_FALLBACK, DOC_TOPICS, documentationUrl } from '../model/documentation'

// A quiet heading is for screen readers until a link focuses it, since the tab already names the list.
// A page shows one Docs link: section pages put it in the filter row, so their headings pass no `docs`.
export function SectionHeading({ docs, id, focusable, entry, actions, hidden, children }: { docs?: string; id?: string; focusable?: boolean; entry?: string; actions?: ReactNode; hidden?: boolean; children: ReactNode }) {
  return (
    <div className={`section-head section-help-heading${hidden ? ' is-quiet' : ''}`}>
      <h2 id={id} tabIndex={focusable || entry ? -1 : undefined} data-entry={entry}>{children}</h2>
      {actions}
      {docs && <DocLink topic={docs} />}
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
