import { describe, expect, it } from 'vitest'
import { CIRCUIT, edgeStyle, layoutCircuit, wrapText, type CircuitInput } from './circuit'
import { loadAuthorityFixture } from './testing'

function inputFromFixture(): CircuitInput {
  const view = loadAuthorityFixture()
  const resources = view.resources.slice(0, 5)
  const ids = new Set(resources.map((r) => r.id))
  return {
    subject: { id: view.agent.id, label: view.agent.label, caption: 'Selected agent' },
    resources: resources.map((r) => ({ id: r.id, title: r.label, eyebrow: r.providerId, status: { text: 'x', tone: 'neutral' } })),
    edges: view.edges
      .filter((e) => ids.has(e.resourceId))
      .map((e) => ({ id: e.id, resourceId: e.resourceId, operationId: e.operationId, label: e.label, effect: e.effect, declared: e.declared, hypothetical: true })),
  }
}

const overlaps = (a: { y: number; height: number }, b: { y: number; height: number }) => a.y < b.y + b.height && b.y < a.y + a.height

describe('circuit layout', () => {
  it('is deterministic: the same input yields the same layout', () => {
    const a = layoutCircuit(inputFromFixture())
    const b = layoutCircuit(inputFromFixture())
    expect(JSON.stringify(a)).toBe(JSON.stringify(b))
  })

  it('places one pill per edge without overlap, each with an in and an out path', () => {
    const input = inputFromFixture()
    const layout = layoutCircuit(input)
    expect(layout.pills).toHaveLength(input.edges.length)
    for (let i = 0; i < layout.pills.length; i++) {
      for (let j = i + 1; j < layout.pills.length; j++) expect(overlaps(layout.pills[i], layout.pills[j])).toBe(false)
    }
    for (let i = 0; i < layout.resources.length; i++) {
      for (let j = i + 1; j < layout.resources.length; j++) expect(overlaps(layout.resources[i], layout.resources[j])).toBe(false)
    }
    for (const pill of layout.pills) {
      expect(layout.paths.filter((p) => p.edgeId === pill.edgeId).map((p) => p.segment)).toEqual(['in', 'out'])
    }
  })

  it('keeps three columns: subject, operations, resources', () => {
    const layout = layoutCircuit(inputFromFixture())
    expect(layout.subject.x + layout.subject.width).toBeLessThan(CIRCUIT.pillX)
    for (const pill of layout.pills) expect(pill.x + pill.width).toBeLessThan(layout.resources[0].x)
    for (const node of layout.resources) expect(node.x + node.width).toBeLessThanOrEqual(layout.width)
    expect(layout.subject.y).toBeGreaterThanOrEqual(0)
    expect(layout.subject.y + layout.subject.height).toBeLessThanOrEqual(layout.height)
  })

  it('styles edges by effective effect and marks declared ≠ effective', () => {
    expect(edgeStyle('allowed')).toBe('permitted')
    expect(edgeStyle('ask')).toBe('approval')
    expect(edgeStyle('denied')).toBe('blocked')
    for (const effect of ['default', 'unavailable', 'unresolved'] as const) expect(edgeStyle(effect)).toBe('muted')
    const layout = layoutCircuit({
      subject: { id: 's', label: 'S', caption: '' },
      resources: [{ id: 'r', title: 'R', eyebrow: 'P', status: { text: '', tone: 'neutral' } }],
      edges: [
        { id: 'e1', resourceId: 'r', operationId: 'o1', label: 'Read', effect: 'unresolved', declared: 'allowed', hypothetical: false },
        { id: 'e2', resourceId: 'r', operationId: 'o2', label: 'Edit', effect: 'allowed', declared: 'allowed', hypothetical: false, proposed: { effect: 'ask', state: 'proposed' } },
      ],
    })
    expect(layout.pills[0]).toMatchObject({ style: 'muted', mismatch: true })
    expect(layout.pills[1]).toMatchObject({ style: 'proposed', mismatch: false, proposed: { effect: 'ask', state: 'proposed' } })
  })

  it('grows with the resources it holds and handles an empty circuit', () => {
    const one = layoutCircuit({ ...inputFromFixture(), resources: inputFromFixture().resources.slice(0, 1) })
    const all = layoutCircuit(inputFromFixture())
    expect(all.height).toBeGreaterThan(one.height)
    const empty = layoutCircuit({ subject: { id: 's', label: 'S', caption: '' }, resources: [], edges: [] })
    expect(empty.pills).toEqual([])
    expect(empty.height).toBe(CIRCUIT.subjectHeight + 2 * CIRCUIT.padding)
  })

  it('truncates long labels for display and keeps the full label', () => {
    const layout = layoutCircuit({
      subject: { id: 's', label: 'A subject with a very long label indeed', caption: '' },
      resources: [{ id: 'r', title: 'A resource title that keeps going and going', eyebrow: 'provider', status: { text: '', tone: 'neutral' } }],
      edges: [{ id: 'e', resourceId: 'r', operationId: 'o', label: 'An operation label that is far too long to fit', effect: 'allowed', declared: 'allowed', hypothetical: false }],
    })
    expect(layout.pills[0].text.endsWith('…')).toBe(true)
    expect(layout.pills[0].label).toBe('An operation label that is far too long to fit')
    expect(layout.resources[0].titleText.length).toBeLessThanOrEqual(26)
  })

  it('wraps a long status onto two lines and truncates what remains', () => {
    expect(wrapText('Deploy would be permitted once synced', 38, 2)).toEqual(['Deploy would be permitted once synced'])
    expect(wrapText('Merge pull request would require approval once synced', 38, 2)).toEqual(['Merge pull request would require', 'approval once synced'])
    const lines = wrapText('one two three four five six seven eight nine ten eleven twelve', 10, 2)
    expect(lines).toHaveLength(2)
    expect(lines[1].endsWith('…')).toBe(true)
  })
})
