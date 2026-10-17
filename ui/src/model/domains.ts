// Grouping for the Authority domain pages: domain → providers → resources,
// and the lookups that turn ids into human labels.

import type { AuthorityView, Domain, EffectCounts, Provider, Resource } from '../contract'

export interface ProviderGroup {
  provider: Provider
  resources: Resource[]
}

export function findDomain(view: AuthorityView, id: string | undefined): Domain | undefined {
  return id ? view.domains.find((d) => d.id === id) : undefined
}

export function findResource(view: AuthorityView, id: string | undefined): Resource | undefined {
  return id ? view.resources.find((r) => r.id === id) : undefined
}

export function findProvider(view: AuthorityView, id: string | undefined): Provider | undefined {
  return id ? view.providers.find((p) => p.id === id) : undefined
}

export function sortedDomains(view: AuthorityView): Domain[] {
  return [...view.domains].sort((a, b) => a.order - b.order || a.label.localeCompare(b.label))
}

/**
 * The providers of one domain, each with its resources in that domain.
 * Order follows the domain's own lists; a resource whose provider the
 * domain does not list still appears, under its provider, at the end.
 */
export function providersInDomain(view: AuthorityView, domainId: string): ProviderGroup[] {
  const domain = findDomain(view, domainId)
  if (!domain) return []
  const inDomain = (r: Resource) => r.domainIds.includes(domainId) || domain.resourceIds.includes(r.id)
  const order = new Map(domain.resourceIds.map((id, i) => [id, i]))
  const resources = view.resources
    .filter(inDomain)
    .sort((a, b) => (order.get(a.id) ?? Number.MAX_SAFE_INTEGER) - (order.get(b.id) ?? Number.MAX_SAFE_INTEGER))

  const providerIds = [...domain.providerIds]
  for (const r of resources) if (!providerIds.includes(r.providerId)) providerIds.push(r.providerId)

  const groups: ProviderGroup[] = []
  for (const id of providerIds) {
    const provider = findProvider(view, id)
    if (!provider) continue
    groups.push({ provider, resources: resources.filter((r) => r.providerId === id) })
  }
  return groups
}

export function totalOperations(counts: EffectCounts): number {
  return counts.allowed + counts.ask + counts.denied + counts.unavailable + counts.default + counts.unresolved
}

export interface DomainCard {
  domain: Domain
  providerLabels: string[]
  total: number
  /** True when any operation in the domain is hypothetical (not installed). */
  hypothetical: boolean
  /** Secrets referenced by the configuration, shown on Secrets & Identity. */
  secretCount: number
}

export const SECRETS_DOMAIN = 'secrets-identity'

export function domainCards(view: AuthorityView): DomainCard[] {
  return sortedDomains(view).map((domain) => {
    const resources = view.resources.filter((r) => r.domainIds.includes(domain.id) || domain.resourceIds.includes(r.id))
    return {
      domain,
      providerLabels: domain.providerIds.map((id) => findProvider(view, id)?.label ?? id),
      total: totalOperations(domain.summary),
      hypothetical: resources.some((r) => r.operations.some((op) => op.hypothetical)),
      secretCount: domain.id === SECRETS_DOMAIN ? view.secrets.length : 0,
    }
  })
}

export const UNDECLARED_SCOPE_LINE = 'Scope not declared · credential reach not inspected'

/**
 * The title a resource card shows. A resource without a declared scope
 * stands for everything its server reaches, so the card is titled with
 * the server's own name and says plainly that the scope is not declared;
 * it never borrows a category name in its place.
 */
export function resourceTitle(resource: Resource, provider: Provider | undefined): { title: string; subtitle?: string } {
  if (resource.kind === 'undeclared-scope') {
    return { title: provider?.label ?? resource.label, subtitle: UNDECLARED_SCOPE_LINE }
  }
  return { title: resource.label, subtitle: resource.description }
}

/** The domain to show a resource under when only the resource is known. */
export function primaryDomain(view: AuthorityView, resource: Resource): string | undefined {
  return resource.domainIds[0] ?? view.domains.find((d) => d.resourceIds.includes(resource.id))?.id
}

/**
 * Resolves any id the view uses (resource, provider, secret, subject,
 * domain, operation, coding tool) to the label a human reads. Returns
 * undefined when the id names nothing in the view.
 */
export function labelForId(view: AuthorityView, id: string): string | undefined {
  const resource = findResource(view, id)
  if (resource) {
    const provider = findProvider(view, resource.providerId)
    return resource.kind === 'undeclared-scope' ? `${provider?.label ?? resource.label} (scope not declared)` : resource.label
  }
  const provider = findProvider(view, id)
  if (provider) return provider.label
  const secret = view.secrets.find((s) => s.id === id)
  if (secret) return secret.label
  const subject = view.agents.find((a) => a.id === id)
  if (subject) return subject.label
  const domain = findDomain(view, id)
  if (domain) return domain.label
  const harness = view.harness.available.find((h) => h.target === id)
  if (harness) return harness.label
  for (const r of view.resources) {
    const op = r.operations.find((o) => o.id === id)
    if (op) return `${op.label} · ${labelForId(view, r.id) ?? r.label}`
  }
  return undefined
}
