// Layout for the permission circuit: subject → operation pills →
// resource nodes, in three columns. Pure and deterministic: the same
// input always yields the same coordinates, so the SVG never jumps
// between renders and the layout is testable without a browser.

import type { Effect } from '../contract'
import type { Tone } from './effects'

export type EdgeStyle = 'permitted' | 'approval' | 'blocked' | 'muted' | 'proposed'
export type ProposedState = 'proposed' | 'validated'

export interface CircuitInput {
  subject: { id: string; label: string; caption: string }
  resources: Array<{ id: string; title: string; eyebrow: string; status: { text: string; tone: Tone } }>
  edges: Array<{
    id: string
    resourceId: string
    operationId: string
    label: string
    effect: Effect
    declared: Effect
    hypothetical: boolean
    proposed?: { effect: Effect; state: ProposedState }
  }>
}

export interface Box {
  x: number
  y: number
  width: number
  height: number
}

export interface CircuitNode extends Box {
  id: string
  title: string
  titleText: string
  eyebrow: string
  eyebrowText: string
  status: { text: string; tone: Tone }
  /** The status, wrapped to at most two lines for the node. */
  statusLines: string[]
}

export interface CircuitPill extends Box {
  edgeId: string
  operationId: string
  resourceId: string
  label: string
  text: string
  effect: Effect
  declared: Effect
  hypothetical: boolean
  style: EdgeStyle
  mismatch: boolean
  proposed?: { effect: Effect; state: ProposedState }
}

export interface CircuitPath {
  id: string
  edgeId: string
  segment: 'in' | 'out'
  d: string
  style: EdgeStyle
}

export interface CircuitLayout {
  width: number
  height: number
  subject: Box & { id: string; label: string; labelText: string; caption: string }
  resources: CircuitNode[]
  pills: CircuitPill[]
  paths: CircuitPath[]
}

export const CIRCUIT = {
  width: 960,
  padding: 24,
  subjectWidth: 220,
  subjectHeight: 112,
  pillX: 352,
  pillWidth: 210,
  pillHeight: 32,
  // Room above each pill for a "Proposed" tag, so adding a proposal never moves the layout.
  pillGap: 20,
  resourceWidth: 300,
  resourceHeight: 118,
  groupGap: 28,
} as const

export function edgeStyle(effect: Effect): EdgeStyle {
  switch (effect) {
    case 'allowed':
      return 'permitted'
    case 'ask':
      return 'approval'
    case 'denied':
      return 'blocked'
    default:
      return 'muted'
  }
}

export function truncate(text: string, max: number): string {
  return text.length > max ? `${text.slice(0, max - 1).trimEnd()}…` : text
}

/** Greedy word wrap; the last allowed line is truncated when text remains. */
export function wrapText(text: string, maxChars: number, maxLines: number): string[] {
  const words = text.split(/\s+/).filter(Boolean)
  const lines: string[] = []
  let line = ''
  for (let i = 0; i < words.length; i++) {
    const next = line ? `${line} ${words[i]}` : words[i]
    if (next.length <= maxChars || !line) {
      line = next
      continue
    }
    lines.push(line)
    line = words[i]
    if (lines.length === maxLines - 1) {
      const rest = words.slice(i).join(' ')
      lines.push(truncate(rest, maxChars))
      return lines
    }
  }
  if (line) lines.push(truncate(line, maxChars))
  return lines
}

const round = (n: number) => Math.round(n * 10) / 10

function curve(x1: number, y1: number, x2: number, y2: number): string {
  const dx = (x2 - x1) / 2
  return `M ${round(x1)} ${round(y1)} C ${round(x1 + dx)} ${round(y1)}, ${round(x2 - dx)} ${round(y2)}, ${round(x2)} ${round(y2)}`
}

export function layoutCircuit(input: CircuitInput): CircuitLayout {
  const c = CIRCUIT
  const resourceX = c.width - c.padding - c.resourceWidth
  const resources: CircuitNode[] = []
  const pills: CircuitPill[] = []
  let y = c.padding

  for (const resource of input.resources) {
    const edges = input.edges.filter((e) => e.resourceId === resource.id)
    const count = Math.max(edges.length, 1)
    const groupHeight = count * c.pillHeight + (count - 1) * c.pillGap
    const block = Math.max(groupHeight, c.resourceHeight)
    resources.push({
      id: resource.id,
      title: resource.title,
      titleText: truncate(resource.title, 24),
      eyebrow: resource.eyebrow,
      eyebrowText: truncate(resource.eyebrow.toUpperCase(), 30),
      status: { text: resource.status.text, tone: resource.status.tone },
      statusLines: wrapText(resource.status.text, 38, 2),
      x: resourceX,
      y: round(y + (block - c.resourceHeight) / 2),
      width: c.resourceWidth,
      height: c.resourceHeight,
    })
    const top = y + (block - groupHeight) / 2
    edges.forEach((edge, i) => {
      pills.push({
        edgeId: edge.id,
        operationId: edge.operationId,
        resourceId: edge.resourceId,
        label: edge.label,
        text: truncate(edge.label, 26),
        effect: edge.effect,
        declared: edge.declared,
        hypothetical: edge.hypothetical,
        style: edge.proposed ? 'proposed' : edgeStyle(edge.effect),
        mismatch: edge.declared !== edge.effect,
        ...(edge.proposed ? { proposed: edge.proposed } : {}),
        x: c.pillX,
        y: round(top + i * (c.pillHeight + c.pillGap)),
        width: c.pillWidth,
        height: c.pillHeight,
      })
    })
    y += block + c.groupGap
  }

  const contentBottom = resources.length ? y - c.groupGap : c.padding
  const height = round(Math.max(contentBottom + c.padding, c.subjectHeight + 2 * c.padding))
  const subject = {
    id: input.subject.id,
    label: input.subject.label,
    labelText: truncate(input.subject.label, 20),
    caption: input.subject.caption,
    x: c.padding,
    y: round((height - c.subjectHeight) / 2),
    width: c.subjectWidth,
    height: c.subjectHeight,
  }

  const paths: CircuitPath[] = []
  const sx = subject.x + subject.width
  const sy = subject.y + subject.height / 2
  for (const pill of pills) {
    const py = pill.y + pill.height / 2
    const node = resources.find((r) => r.id === pill.resourceId)!
    paths.push({ id: `${pill.edgeId}#in`, edgeId: pill.edgeId, segment: 'in', d: curve(sx, sy, pill.x, py), style: pill.style })
    paths.push({
      id: `${pill.edgeId}#out`,
      edgeId: pill.edgeId,
      segment: 'out',
      d: curve(pill.x + pill.width, py, node.x, node.y + node.height / 2),
      style: pill.style,
    })
  }

  return { width: c.width, height, subject, resources, pills, paths }
}
