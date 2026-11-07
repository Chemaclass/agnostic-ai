import type { KindCount, KindSupport } from '../contract'
import { emptyState } from '../model/verdict'
import { useApp } from './app-context'
import { DocLink } from './SectionHeading'
import { CopyButton, toolName } from './ui'

/** An empty type: what it is, which tools use it, and the next step; "No matches." while filtering. */
export function EmptyState({ kind, support, filtering }: { kind: Pick<KindCount, 'kind' | 'label'>; support?: KindSupport; filtering?: boolean }) {
  const app = useApp()
  const state = emptyState(kind, support, toolName, filtering)
  if (filtering) return <p className="muted empty-matches">{state.title}</p>
  const gap = app.overview?.health.gaps.find((g) => g.id === `kind:${kind.kind}`)
  return (
    <div className="empty-state">
      <p className="empty-state-title">{state.title}</p>
      {(state.purpose || state.tools) && <p className="empty-state-text">{[state.purpose, state.tools].filter(Boolean).join(' ')}</p>}
      <div className="empty-state-actions">
        {gap && <CopyButton text={gap.fix} label="Copy prompt" className="button-primary" ariaLabel={`Copy prompt to add ${kind.label.toLowerCase()}`} />}
        <DocLink topic={kind.kind} />
      </div>
    </div>
  )
}
