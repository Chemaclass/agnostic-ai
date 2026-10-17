// The permission circuit: subject → operation pills → resource nodes,
// drawn as hand-written SVG from the pure layout in model/circuit.ts.
// Edge style follows the effective effect; a proposal redraws the edges
// it touches in amber with a tag. Motion shows direction (authority
// flows from the subject to the resource) and stops under reduced motion,
// where the arrowheads remain as static markers.

import { useId, useMemo } from 'react'
import type { AuthorityView, Effect, Resource } from '../contract'
import { layoutCircuit, type CircuitInput, type EdgeStyle, type ProposedState } from '../model/circuit'
import { findProvider, primaryDomain, resourceTitle } from '../model/domains'
import { declaredWording, effectTone, effectWording, upperFirst } from '../model/effects'
import { sourceRef } from '../model/format'
import { resourceStatus } from '../model/status'
import { useApp } from './app-context'
import { effectGlyph } from './ui'

export interface ProposedMark {
  effect: Effect
  state: ProposedState
}

const STYLES: EdgeStyle[] = ['permitted', 'approval', 'blocked', 'muted', 'proposed']

export function proposedTagText(state: ProposedState): string {
  return state === 'validated' ? 'Validated, not applied' : 'Proposed'
}

export function Circuit({
  view,
  resources,
  proposed,
  selectedResourceId,
  domainId,
  caption,
}: {
  view: AuthorityView
  resources: Resource[]
  proposed: Map<string, ProposedMark>
  selectedResourceId?: string
  domainId?: string
  caption: string
}) {
  const app = useApp()
  const titleId = useId()
  const descId = useId()

  const input: CircuitInput = useMemo(() => {
    const edgesByOp = new Map(view.edges.filter((e) => e.subjectId === view.agent.id).map((e) => [e.operationId, e]))
    return {
      subject: { id: view.agent.id, label: view.agent.label, caption: view.agent.kind === 'session' ? 'Tool session' : 'Selected agent' },
      resources: resources.map((r) => {
        const provider = findProvider(view, r.providerId)
        const { title } = resourceTitle(r, provider)
        return { id: r.id, title, eyebrow: r.kind === 'undeclared-scope' ? 'Scope not declared' : (provider?.label ?? ''), status: resourceStatus(r) }
      }),
      edges: resources.flatMap((r) =>
        r.operations.map((op) => {
          const edge = edgesByOp.get(op.id)
          return {
            id: edge?.id ?? `${view.agent.id}|${op.id}`,
            resourceId: r.id,
            operationId: op.id,
            label: edge?.label || op.label,
            effect: edge?.effect ?? op.effect,
            declared: edge?.declared ?? op.declared,
            hypothetical: op.hypothetical,
            ...(proposed.has(op.id) ? { proposed: proposed.get(op.id)! } : {}),
          }
        }),
      ),
    }
  }, [view, resources, proposed])

  const layout = useMemo(() => layoutCircuit(input), [input])
  const markerId = (style: EdgeStyle) => `arrow-${style}-${titleId.replace(/[^A-Za-z0-9_-]/g, '')}`
  const operations = useMemo(() => new Map(resources.flatMap((r) => r.operations.map((op) => [op.id, op] as const))), [resources])
  const hypothetical = input.edges.some((e) => e.hypothetical)
  const counts = new Map<Effect, number>()
  for (const e of input.edges) counts.set(e.effect, (counts.get(e.effect) ?? 0) + 1)
  const summary = [...counts.entries()].map(([effect, n]) => `${n} ${effectWording(effect, hypothetical).toLowerCase()}`).join('; ')

  return (
    <figure className="circuit">
      <div className="circuit-scroll">
        <svg
          className="circuit-svg"
          viewBox={`0 0 ${layout.width} ${layout.height}`}
          width={layout.width}
          height={layout.height}
          role="group"
          aria-labelledby={titleId}
          aria-describedby={descId}
          data-testid="permission-circuit"
        >
          <title id={titleId}>{caption}</title>
          <desc id={descId}>
            {input.edges.length} operations from {view.agent.label} to {input.resources.length} resources: {summary}.
          </desc>
          <defs>
            {STYLES.map((style) => (
              <marker key={style} id={markerId(style)} className={`arrow arrow-${style}`} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
                <path d="M 0 0 L 10 5 L 0 10 z" />
              </marker>
            ))}
          </defs>

          <g className="edges">
            {layout.paths.map((p) => (
              <path
                key={p.id}
                d={p.d}
                className={`edge edge-${p.style} edge-${p.segment}`}
                data-edge={p.edgeId}
                markerEnd={p.segment === 'out' ? `url(#${markerId(p.style)})` : undefined}
              />
            ))}
          </g>

          <g className="node node-subject" transform={`translate(${layout.subject.x} ${layout.subject.y})`}>
            <rect width={layout.subject.width} height={layout.subject.height} rx="14" />
            <text x="18" y="30" className="node-eyebrow">
              SUBJECT
            </text>
            <text x="18" y="62" className="node-title">
              {layout.subject.labelText}
            </text>
            <text x="18" y="88" className="node-caption">
              {layout.subject.caption}
            </text>
          </g>

          <g className="pills">
            {layout.pills.map((p) => {
              const effective = effectWording(p.effect, p.hypothetical)
              const tag = p.proposed ? proposedTagText(p.proposed.state) : undefined
              const tagWidth = tag ? tag.length * 6.1 + 14 : 0
              const decided = operations.get(p.operationId)?.decided
              const evidence = decided ? ` · decided by ${decided.list} ${decided.rule} (${sourceRef(decided.source)})` : ' · no rule decides it'

              return (
                <g key={p.edgeId} className={`pill pill-${p.style} tone-${effectTone(p.effect)}`} transform={`translate(${p.x} ${p.y})`} data-operation={p.operationId}>
                  <title>
                    {`${p.label}: ${effective}`}
                    {p.mismatch ? ` (${declaredWording(p.declared).toLowerCase()})` : ''}
                    {evidence}
                    {p.proposed ? ` · ${tag}: ${effectWording(p.proposed.effect, p.hypothetical).toLowerCase()}` : ''}
                  </title>
                  <rect width={p.width} height={p.height} rx={p.height / 2} />
                  <text x="15" y={p.height / 2 + 4.5} className="pill-glyph" aria-hidden="true">
                    {effectGlyph(p.effect)}
                  </text>
                  <text x="33" y={p.height / 2 + 4.5} className="pill-text">
                    {p.text}
                  </text>
                  {p.mismatch && (
                    <path className={`declared-marker tone-${effectTone(p.declared)}`} d={`M 0 ${p.height / 2 - 6} L 6 ${p.height / 2} L 0 ${p.height / 2 + 6} L -6 ${p.height / 2} Z`}>
                      <title>{`${upperFirst(declaredWording(p.declared).toLowerCase())}; effective: ${effective.toLowerCase()}`}</title>
                    </path>
                  )}
                  {tag && (
                    <g className={`proposed-tag tag-${p.proposed!.state}`} transform={`translate(${p.width - tagWidth} ${-17})`}>
                      <rect width={tagWidth} height="15" rx="7.5" />
                      <text x={tagWidth / 2} y="11" textAnchor="middle">
                        {tag}
                      </text>
                    </g>
                  )}
                </g>
              )
            })}
          </g>

          <g className="resources">
            {layout.resources.map((n) => {
              const selected = n.id === selectedResourceId
              const resource = resources.find((r) => r.id === n.id)!
              const target = { resource: n.id, domain: domainId && resource.domainIds.includes(domainId) ? domainId : primaryDomain(view, resource) }
              const longest = Math.max(...n.statusLines.map((l) => l.length))
              const statusWidth = Math.min(n.width - 36, longest * 6.3 + 22)
              const statusHeight = 9 + n.statusLines.length * 15
              return (
                <a
                  key={n.id}
                  href={app.href(target)}
                  className={`node node-resource${selected ? ' is-selected' : ''}`}
                  aria-label={`${n.title}, ${n.eyebrow}: ${n.status.text}. Open the resource.`}
                  aria-current={selected ? 'true' : undefined}
                  onClick={(event) => {
                    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
                    event.preventDefault()
                    app.navigate(target)
                  }}
                >
                  <g transform={`translate(${n.x} ${n.y})`}>
                    <title>{`${n.title} · ${n.status.text}`}</title>
                    <rect width={n.width} height={n.height} rx="14" />
                    <text x="18" y="28" className="node-eyebrow">
                      {n.eyebrowText}
                    </text>
                    <text x="18" y="54" className="node-title">
                      {n.titleText}
                    </text>
                    <g className={`node-status tone-${n.status.tone}`} transform="translate(18 66)">
                      <rect width={statusWidth} height={statusHeight} rx="12" />
                      {n.statusLines.map((line, i) => (
                        <text key={i} x="11" y={16 + i * 15}>
                          {line}
                        </text>
                      ))}
                    </g>
                  </g>
                </a>
              )
            })}
          </g>
        </svg>
      </div>
      <figcaption className="legend">
        <span className="legend-title">How to read it</span>
        <ul>
          <li>
            <LegendLine style="permitted" /> {hypothetical ? 'Would be permitted once synced' : 'Permitted'} (moving dashes show the direction)
          </li>
          <li>
            <LegendLine style="approval" /> {hypothetical ? 'Would require approval once synced' : 'Permitted, approval required'}
          </li>
          <li>
            <LegendLine style="blocked" /> {hypothetical ? 'Would be blocked once synced' : 'Blocked'}
          </li>
          <li>
            <LegendLine style="muted" /> No rule, unavailable through this route, or unresolved
          </li>
          <li>
            <svg className="legend-swatch" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true">
              <path className="declared-marker tone-permitted" d="M 8 2 L 14 8 L 8 14 L 2 8 Z" />
            </svg>{' '}
            Declared differs from effective
          </li>
          <li>
            <LegendLine style="proposed" /> Proposed, then validated, not applied
          </li>
        </ul>
        <span className="muted">Select a resource node to open it. On a narrow screen, scroll the diagram sideways.</span>
      </figcaption>
    </figure>
  )
}

function LegendLine({ style }: { style: EdgeStyle }) {
  return (
    <svg className="legend-swatch" viewBox="0 0 36 12" width="36" height="12" aria-hidden="true">
      <path d="M 1 6 L 35 6" className={`edge edge-${style}`} />
    </svg>
  )
}
