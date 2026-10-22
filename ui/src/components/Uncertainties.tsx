// Everything the view cannot determine, as a collapsible panel. An
// uncertainty is never shown as granted or denied.

import { useId, useState } from 'react'
import type { AuthorityView, Uncertainty } from '../contract'
import { labelForId } from '../model/domains'
import { plural } from '../model/format'
import { Badge, Chip } from './ui'

export function UncertaintyItem({ view, item }: { view: AuthorityView; item: Uncertainty }) {
  const related = item.relatedIds.map((id) => ({ id, label: labelForId(view, id) }))
  return (
    <li className="uncertainty">
      <Badge tone={item.severity === 'warning' ? 'approval' : 'info'}>{item.severity === 'warning' ? 'Warning' : 'Info'}</Badge>
      <span className="uncertainty-text">{item.message}</span>
      {related.length > 0 && (
        <span className="uncertainty-related">
          {related.map((r) => (
            <Chip key={r.id} title={r.label ? undefined : 'Not an item in this view'}>
              {r.label ?? r.id}
            </Chip>
          ))}
        </span>
      )}
    </li>
  )
}

export function Uncertainties({ view, defaultOpen, title = 'Uncertainties' }: { view: AuthorityView; defaultOpen?: boolean; title?: string }) {
  const [open, setOpen] = useState(!!defaultOpen)
  const id = useId()
  const items = view.uncertainties
  const warnings = items.filter((u) => u.severity === 'warning').length
  return (
    <section className="panel uncertainties" aria-labelledby={`${id}-title`}>
      <h2 id={`${id}-title`} className="panel-title">
        <button type="button" className="disclosure" aria-expanded={open} aria-controls={`${id}-list`} onClick={() => setOpen((o) => !o)}>
          <span className="disclosure-mark" aria-hidden="true">
            <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="2.25" strokeLinecap="round">
              <path d={open ? 'M6 12h12' : 'M6 12h12M12 6v12'} />
            </svg>
          </span>
          {title}
          <span className="disclosure-count">
            {items.length}
            {warnings > 0 && <span className="muted"> · {plural(warnings, 'warning')}</span>}
          </span>
        </button>
      </h2>
      <p className="muted">What this view cannot determine. Runtime enforcement is never observed here.</p>
      {open && (
        <ul id={`${id}-list`} className="uncertainty-list">
          {items.map((u) => (
            <UncertaintyItem key={u.id} view={view} item={u} />
          ))}
        </ul>
      )}
    </section>
  )
}
