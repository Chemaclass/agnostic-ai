// One resource: its identity and caveats, then every operation with the
// four states kept apart (declared, effective on the selected tool,
// installed, runtime) and the evidence beside each badge.

import { useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import type { AuthorityView, Change, Operation, Resource } from '../contract'
import { findProvider, resourceTitle } from '../model/domains'
import {
  declaredWording,
  effectiveDisplay,
  enforcementLabel,
  installedDisplay,
  listName,
  resolutionLabel,
  RUNTIME_TEXT,
} from '../model/effects'
import { sourceRef } from '../model/format'
import { proposalAction, type ProposalActionKind } from '../model/proposal'
import { resourceStatus } from '../model/status'
import { StatusPill } from './AuthorityDomains'
import { UncertaintyItem } from './Uncertainties'
import { Badge, Code, EffectBadge } from './ui'

const BASIS: Record<string, string> = {
  'rule-pattern': 'A permission rule names it',
  'declared-root': 'The server spec declares it as a root',
  undeclared: 'Nothing declares it: the server spec names no scope',
}

const KIND: Record<string, string> = {
  'undeclared-scope': 'Everything the server reaches',
  'declared-root': 'Declared root',
  'file-pattern': 'Files matching a pattern',
  'command-pattern': 'Shell commands matching a pattern',
  web: 'The web',
}

export function ResourceDetail({
  view,
  resource,
  onPropose,
  onOpenDraft,
  proposalSlot,
  circuitSlot,
}: {
  view: AuthorityView
  resource: Resource
  onPropose: (changes: Change[]) => void
  onOpenDraft: () => void
  proposalSlot: ReactNode
  circuitSlot: ReactNode
}) {
  const provider = findProvider(view, resource.providerId)
  const { title } = resourceTitle(resource, provider)
  const status = resourceStatus(resource)
  const related = view.uncertainties.filter((u) => u.relatedIds.includes(resource.id) || (provider && u.relatedIds.includes(provider.id)))
  const identity = resource.identity

  return (
    <>
      <section className="panel resource-panel" aria-label={`${title} details`}>
        <div className="resource-panel-head">
          <p className="eyebrow">
            {provider?.label ?? resource.providerId} · {KIND[resource.kind] ?? resource.kind}
          </p>
          <StatusPill status={status} resource={resource} />
        </div>
        <dl className="facts">
          {resource.kind !== 'undeclared-scope' && resource.label !== title && (
            <div>
              <dt>Label</dt>
              <dd>{resource.label}</dd>
            </div>
          )}
          {(identity.uri || identity.pattern || identity.server) && (
            <div>
              <dt>Machine identity</dt>
              <dd className="code-list">
                {identity.uri && <Code>{identity.uri}</Code>}
                {identity.pattern && <Code>{identity.pattern}</Code>}
                {identity.server && !identity.uri && <Code>server: {identity.server}</Code>}
              </dd>
            </div>
          )}
          <div>
            <dt>Established by</dt>
            <dd>{BASIS[resource.basis] ?? resource.basis}</dd>
          </div>
          {provider?.transport && (
            <div>
              <dt>Reached through</dt>
              <dd>
                {provider.transport.type}
                {provider.transport.endpoint && (
                  <>
                    {' '}
                    <Code>{provider.transport.endpoint}</Code>
                  </>
                )}
              </dd>
            </div>
          )}
          {provider?.source && (
            <div>
              <dt>Server declared in</dt>
              <dd>
                <Code>{sourceRef(provider.source)}</Code>
              </dd>
            </div>
          )}
          {resource.matches && (
            <div>
              <dt>Matching files</dt>
              <dd>
                {resource.matches.count === 0 ? (
                  'No file matches today'
                ) : (
                  <span className="code-list">
                    <span>{resource.matches.count}:</span>
                    {resource.matches.samples.map((s) => (
                      <Code key={s}>{s}</Code>
                    ))}
                    {resource.matches.truncated && <span className="muted">and more</span>}
                  </span>
                )}
              </dd>
            </div>
          )}
        </dl>
        {resource.caveat && <p className="caveat">{resource.caveat}</p>}
        {resource.unresolvedReason && <p className="muted">{resource.unresolvedReason}</p>}
        {related.length > 0 && (
          <div className="related-uncertainties">
            <p className="muted">What the view cannot determine here:</p>
            <ul className="uncertainty-list">
              {related.map((u) => (
                <UncertaintyItem key={u.id} view={view} item={u} />
              ))}
            </ul>
          </div>
        )}
      </section>

      <section className="block" aria-labelledby="ops-title">
        <div className="section-head">
          <h2 id="ops-title">Operations</h2>
          <button type="button" className="button button-quiet" onClick={onOpenDraft}>
            Propose a change
          </button>
        </div>
        <OperationsTable view={view} resource={resource} onPropose={onPropose} onOpenDraft={onOpenDraft} />
      </section>

      {proposalSlot}
      {circuitSlot}
    </>
  )
}

export function OperationsTable({ view, resource, onPropose, onOpenDraft }: { view: AuthorityView; resource: Resource; onPropose: (changes: Change[]) => void; onOpenDraft: () => void }) {
  return (
    <div className="table-wrap ops-wrap">
      <table className="table ops-table">
        <caption className="sr-only">
          Operations on {resource.label} for {view.agent.label}, effective on {view.harness.label}
        </caption>
        <thead>
          <tr>
            <th scope="col" className="col-op">Operation</th>
            <th scope="col" className="col-declared">Declared</th>
            <th scope="col" className="col-effective">Effective on {view.harness.label}</th>
            <th scope="col" className="col-granted">Granted by</th>
            <th scope="col" className="col-installed">Installed</th>
            <th scope="col" className="col-runtime">Runtime</th>
          </tr>
        </thead>
        {resource.operations.map((op) => (
          <OperationRows key={op.id} view={view} resource={resource} op={op} onPropose={onPropose} onOpenDraft={onOpenDraft} />
        ))}
      </table>
    </div>
  )
}

function OperationRows({ view, resource, op, onPropose, onOpenDraft }: { view: AuthorityView; resource: Resource; op: Operation; onPropose: (changes: Change[]) => void; onOpenDraft: () => void }) {
  const [why, setWhy] = useState(false)
  const id = useId()
  const display = effectiveDisplay(op, view.explanations)
  const installed = installedDisplay(op.installed, view.explanations)
  const decided = op.decided ?? op.grants.find((g) => g.wins)

  return (
    <tbody className="op-group" data-operation={op.id} data-action={op.action}>
      <tr className="op-row">
        <th scope="row" data-label="Operation">
          <span className="op-label">{op.label}</span>
          <span className="op-action">{op.action}</span>
          <span className="op-actions">
            <button type="button" className="button button-quiet" aria-expanded={why} aria-controls={`${id}-why`} onClick={() => setWhy((w) => !w)}>
              Why?<span className="sr-only"> {op.label}</span>
            </button>
            <ProposeMenu resource={resource} op={op} onPropose={onPropose} onOpenDraft={onOpenDraft} />
          </span>
        </th>
        <td data-label="Declared">
          <EffectBadge effect={op.declared} text={declaredWording(op.declared)} muted title={view.explanations.effects[op.declared]} />
        </td>
        <td data-label={`Effective on ${view.harness.label}`} className="effective-cell">
          <EffectBadge effect={op.effect} text={display.text} title={display.explanation} />
          {display.secondary && <EffectBadge effect={op.declared} text={display.secondary.text} muted />}
          {display.excludedBy && <p className="cell-note">{display.excludedBy}.</p>}
          {op.effect === 'unresolved' && display.resolutionNote && <p className="cell-note">{display.resolutionNote}</p>}
          {op.effect === 'unavailable' && <RoutesSummary op={op} />}
        </td>
        <td data-label="Granted by">
          {decided ? (
            <span className="granted">
              <Code>{decided.rule}</Code>
              <span className="cell-note" title={view.explanations.lists[decided.list]}>
                {listName(decided.list)} · {decided.scope === 'agent' ? 'agent spec' : 'project settings'}
              </span>
              <Code>{sourceRef(decided.source)}</Code>
            </span>
          ) : (
            <span className="cell-note">No rule decides it</span>
          )}
        </td>
        <td data-label="Installed">
          <Badge tone={installed.tone}>{installed.text}</Badge>
          {installed.note && <p className="cell-note">{installed.note}</p>}
        </td>
        <td data-label="Runtime">
          <Badge tone="neutral" title="This view reads configuration only.">
            {RUNTIME_TEXT}
          </Badge>
        </td>
      </tr>
      <tr className="op-notes">
        <td colSpan={6}>
          <div className="notes-grid">
            <div>
              <p className="notes-title">Explanation</p>
              <p>{op.explanation || display.explanation}</p>
            </div>
            <div>
              <p className="notes-title">Conditions</p>
              {op.conditions.length ? (
                <ul className="plain-list">
                  {op.conditions.map((c) => (
                    <li key={c}>{c}</li>
                  ))}
                </ul>
              ) : (
                <p className="muted">No condition is declared.</p>
              )}
            </div>
            <div>
              <p className="notes-title">Alternate routes</p>
              <RouteCoverageView op={op} />
            </div>
          </div>
        </td>
      </tr>
      {why && (
        <tr className="op-why" id={`${id}-why`}>
          <td colSpan={6}>
            <WhyPanel view={view} op={op} />
          </td>
        </tr>
      )}
    </tbody>
  )
}

/** Under an Unavailable badge: the open routes, never a claim that none exist. */
function RoutesSummary({ op }: { op: Operation }) {
  if (op.alternateRoutes.length === 0) return <p className="cell-note">No checked route is open; see the routes below.</p>
  return (
    <div className="cell-note">
      <p>Other routes stay open:</p>
      <ul className="plain-list">
        {op.alternateRoutes.map((r) => (
          <li key={r}>{r}</li>
        ))}
      </ul>
    </div>
  )
}

export function RouteCoverageView({ op }: { op: Operation }) {
  return (
    <div className="routes">
      {op.alternateRoutes.length ? (
        <ul className="plain-list routes-open">
          {op.alternateRoutes.map((r) => (
            <li key={r}>{r}</li>
          ))}
        </ul>
      ) : (
        <p>No checked route is open</p>
      )}
      <p className="routes-label">Checked</p>
      {op.routeCoverage.checked.length ? (
        <ul className="route-list">
          {op.routeCoverage.checked.map((route) => (
            <li key={route.route}>
              <span className="route-name">{route.route}</span>
              {/* No rule waits for a sync, so a default is never worded conditionally. */}
              <EffectBadge effect={route.effect} hypothetical={op.hypothetical && route.effect !== 'default'} />
              {route.note && <span className="cell-note">{route.note}</span>}
            </li>
          ))}
        </ul>
      ) : (
        <p className="muted">No route was checked for this operation.</p>
      )}
      <p className="routes-label">Not modeled</p>
      {op.routeCoverage.notModeled.length ? (
        <ul className="plain-list route-unmodeled">
          {op.routeCoverage.notModeled.map((sentence) => (
            <li key={sentence}>{sentence}</li>
          ))}
        </ul>
      ) : (
        <p className="muted">The view lists no unmodeled route here.</p>
      )}
    </div>
  )
}

function WhyPanel({ view, op }: { view: AuthorityView; op: Operation }) {
  return (
    <div className="why">
      <div className="why-section">
        <p className="notes-title">How {view.harness.label} reaches this effect</p>
        <p>
          <strong>{resolutionLabel(op.resolution)}.</strong> {view.explanations.resolution[op.resolution]}
        </p>
        {op.native && op.native.length > 0 && (
          <p className="code-list">
            <span className="muted">Native form:</span>
            {op.native.map((n) => (
              <Code key={n}>{n}</Code>
            ))}
          </p>
        )}
      </div>
      <div className="why-section">
        <p className="notes-title">Every declaration that applies, winner first</p>
        {op.grants.length ? (
          <ol className="grant-list">
            {op.grants.map((g, i) => (
              <li key={`${g.list}:${g.rule}:${i}`} className={g.wins ? 'is-winner' : undefined}>
                {g.wins ? <Badge tone="accent">Wins</Badge> : <Badge tone="neutral" muted>Applies</Badge>}
                <Code>{g.rule}</Code>
                <span className="grant-meta">
                  {listName(g.list)} · {g.scope === 'agent' ? 'agent spec' : 'project settings'} · <Code>{sourceRef(g.source)}</Code>
                </span>
                {g.note && <span className="cell-note">{g.note}</span>}
                {view.explanations.lists[g.list] && <span className="cell-note muted">{view.explanations.lists[g.list]}</span>}
              </li>
            ))}
          </ol>
        ) : (
          <p className="muted">No declaration applies: the tool's own default decides.</p>
        )}
      </div>
      <div className="why-section">
        <p className="notes-title">On every configured coding tool</p>
        <div className="table-wrap">
          <table className="table enforcement-table">
            <thead>
              <tr>
                <th scope="col">Coding tool</th>
                <th scope="col">How it is carried</th>
                <th scope="col">Effect there</th>
                <th scope="col">Native form</th>
              </tr>
            </thead>
            <tbody>
              {op.enforcement.map((e) => (
                <tr key={e.target} className={e.target === view.harness.target ? 'is-selected' : undefined} aria-current={e.target === view.harness.target ? 'true' : undefined}>
                  <th scope="row">
                    {e.label}
                    {e.target === view.harness.target && <span className="muted"> (selected)</span>}
                  </th>
                  <td>
                    <span title={view.explanations.enforcement[e.status]}>{enforcementLabel(e.status)}</span>
                    {e.note && <span className="cell-note">{e.note}</span>}
                  </td>
                  <td>
                    <EffectBadge effect={e.effect} hypothetical={op.hypothetical && e.target === view.harness.target} />
                  </td>
                  <td className="code-list">{e.native?.length ? e.native.map((n) => <Code key={n}>{n}</Code>) : <span className="muted">Nothing written</span>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

const ACTIONS: Array<{ kind: ProposalActionKind; label: string; hint: string }> = [
  { kind: 'revoke', label: 'Revoke', hint: 'Remove the winning allow rule' },
  { kind: 'require-approval', label: 'Require approval', hint: 'Add an ask rule' },
  { kind: 'block', label: 'Block', hint: 'Add a deny rule' },
]

function ProposeMenu({ resource, op, onPropose, onOpenDraft }: { resource: Resource; op: Operation; onPropose: (changes: Change[]) => void; onOpenDraft: () => void }) {
  const [open, setOpen] = useState(false)
  const [position, setPosition] = useState<{ top: number; left: number } | null>(null)
  const id = useId()
  const root = useRef<HTMLDivElement>(null)
  const button = useRef<HTMLButtonElement>(null)
  const popover = useRef<HTMLDivElement>(null)

  // The popover is fixed to the viewport, so a scrolling table or the
  // side navigation can never clip or cover it.
  const place = () => {
    const anchor = button.current?.getBoundingClientRect()
    if (!anchor) return
    const width = Math.min(320, window.innerWidth - 16)
    const height = popover.current?.offsetHeight ?? 260
    const left = Math.min(Math.max(8, anchor.left), window.innerWidth - width - 8)
    const below = anchor.bottom + 6
    const top = below + height > window.innerHeight - 8 && anchor.top - height - 6 > 8 ? anchor.top - height - 6 : below
    setPosition({ top, left })
  }

  useLayoutEffect(() => {
    if (open) place()
  }, [open])

  useEffect(() => {
    if (!open) return
    const onDown = (event: MouseEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false)
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpen(false)
        button.current?.focus()
      }
    }
    const onMove = () => place()
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    window.addEventListener('scroll', onMove, true)
    window.addEventListener('resize', onMove)
    popover.current?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus()
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
      window.removeEventListener('scroll', onMove, true)
      window.removeEventListener('resize', onMove)
    }
  }, [open])

  return (
    <div className="propose-menu" ref={root}>
      <button ref={button} type="button" className="button button-quiet" aria-expanded={open} aria-controls={`${id}-menu`} onClick={() => setOpen((o) => !o)}>
        Propose<span className="sr-only"> a change to {op.label}</span>
      </button>
      {open && (
        <div className="propose-popover" id={`${id}-menu`} ref={popover} style={position ? { top: position.top, left: position.left } : { visibility: 'hidden' }}>
          <p className="popover-title">Propose a change to {op.label}</p>
          {ACTIONS.map((a) => {
            const availability = proposalAction(a.kind, resource, op)
            return (
              <button
                key={a.kind}
                type="button"
                className="popover-item"
                data-action={a.kind}
                disabled={!availability.enabled}
                onClick={() => {
                  setOpen(false)
                  onPropose(availability.changes)
                }}
              >
                <span className="popover-item-label">{a.label}</span>
                <span className="popover-item-hint">
                  {availability.enabled ? `${a.hint}: ${availability.changes.map((c) => c.rule).join(', ')}` : availability.reason}
                </span>
              </button>
            )
          })}
          <button
            type="button"
            className="popover-item"
            data-action="type-rule"
            onClick={() => {
              setOpen(false)
              onOpenDraft()
            }}
          >
            <span className="popover-item-label">Type a rule…</span>
            <span className="popover-item-hint">Write any allow, ask, or deny change</span>
          </button>
        </div>
      )}
    </div>
  )
}
