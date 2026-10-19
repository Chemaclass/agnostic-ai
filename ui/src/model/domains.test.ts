import { describe, expect, it } from 'vitest'
import { domainCards, labelForId, providersInDomain, resourceTitle, UNDECLARED_SCOPE_LINE } from './domains'
import { loadAuthorityFixture, usingMockAuthority } from './testing'

describe('grouping resources by domain and provider', () => {
  const view = loadAuthorityFixture()

  it('places every resource under each of its domains exactly once', () => {
    for (const domain of view.domains) {
      const groups = providersInDomain(view, domain.id)
      const listed = groups.flatMap((g) => g.resources.map((r) => r.id))
      expect(new Set(listed).size, domain.id).toBe(listed.length)
      for (const id of domain.resourceIds) expect(listed, `${domain.id} lists ${id}`).toContain(id)
      for (const group of groups) for (const r of group.resources) expect(r.providerId).toBe(group.provider.id)
    }
  })

  it('returns the domains in their declared order with provider labels', () => {
    const cards = domainCards(view)
    const orders = cards.map((c) => c.domain.order)
    expect(orders).toEqual([...orders].sort((a, b) => a - b))
    for (const card of cards) expect(card.providerLabels.length).toBe(card.domain.providerIds.length)
  })

  it('returns nothing for an unknown domain', () => {
    expect(providersInDomain(view, 'no-such-domain')).toEqual([])
  })

  it.runIf(usingMockAuthority)('groups the mock as the demo project describes', () => {
    expect(domainCards(view).map((c) => c.domain.label)).toHaveLength(8)
    const cloud = providersInDomain(view, 'cloud-deployment')
    expect(cloud.map((g) => g.provider.label)).toEqual(['Railway'])
    expect(cloud[0].resources.map((r) => r.id)).toEqual(['mcp-railway-scope'])
    const files = providersInDomain(view, 'filesystem-execution')
    expect(files.map((g) => g.provider.id)).toEqual(['local-files', 'shell', 'mcp-workspace-files'])
    expect(files[2].resources.map((r) => r.label)).toEqual(['docs folder'])
    expect(domainCards(view).find((c) => c.domain.id === 'secrets-identity')?.secretCount).toBe(3)
  })
})

describe('resource titles and labels', () => {
  const view = loadAuthorityFixture()

  it('titles an undeclared scope with its server name and states the scope plainly', () => {
    for (const r of view.resources.filter((res) => res.kind === 'undeclared-scope')) {
      const provider = view.providers.find((p) => p.id === r.providerId)
      expect(resourceTitle(r, provider)).toEqual({ title: provider?.label ?? r.label, subtitle: UNDECLARED_SCOPE_LINE })
    }
    expect(UNDECLARED_SCOPE_LINE).toBe('Scope not declared · credential reach not inspected')
  })

  it('resolves ids to human labels and leaves unknown ids alone', () => {
    expect(labelForId(view, view.agent.id)).toBe(view.agent.label)
    expect(labelForId(view, view.harness.target)).toBe(view.harness.label)
    expect(labelForId(view, 'definitely-not-an-id')).toBeUndefined()
  })
})
