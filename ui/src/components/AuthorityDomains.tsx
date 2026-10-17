// The Authority domain level: who is selected, the eight capability
// domains, and a domain's providers with their concrete resources.

import type { AuthorityView, Domain, Provider, Resource } from '../contract'
import { domainCards, providersInDomain, resourceTitle, SECRETS_DOMAIN } from '../model/domains'
import { installedDisplay, permissionSupportLabel, rightDisplay, RIGHT_NAMES } from '../model/effects'
import { plural, sourceRef } from '../model/format'
import { resourceStatus, type ResourceStatus } from '../model/status'
import { AppLink } from './app-context'
import { Badge, Chip, Code, EffectCounts, Icon, OperationList } from './ui'

export function StatusPill({ status, resource }: { status: ResourceStatus; resource?: Resource }) {
  const op = resource?.operations.find((o) => o.id === status.operationId)
  const decided = op?.decided
  const title = op ? (decided ? `${op.label}: decided by ${decided.list} ${decided.rule} (${sourceRef(decided.source)})` : `${op.label}: no rule decides it`) : undefined
  return (
    <Badge tone={status.tone} className="status-pill" title={title}>
      {status.text}
    </Badge>
  )
}

export function SubjectStrip({ view, compact }: { view: AuthorityView; compact?: boolean }) {
  const { agent, harness } = view
  const installed = installedDisplay(harness.installed.state, view.explanations)
  const hypothetical = harness.installed.state !== 'in-sync'
  return (
    <section className={`subject-strip${compact ? ' is-compact' : ''}`} aria-label="Selected subject and coding tool">
      <div className="subject-card">
        <p className="eyebrow">{agent.kind === 'session' ? 'Subject' : 'Selected agent'}</p>
        <p className="subject-name">{agent.label}</p>
        <p className="muted">{agent.description}</p>
        {!compact && (
          <dl className="facts facts-inline">
            <div>
              <dt>Tool scope</dt>
              <dd>
                {agent.toolScope.mode === 'unrestricted' ? (
                  'Unrestricted: inherits every tool the session permits'
                ) : (
                  <span className="code-list">
                    <span className="muted">{agent.toolScope.field || 'can'}: </span>
                    {agent.toolScope.declared.map((entry) => (
                      <Code key={entry}>{entry}</Code>
                    ))}
                  </span>
                )}
              </dd>
            </div>
            <div>
              <dt>MCP servers</dt>
              <dd>
                {agent.mcpServers.mode === 'all' ? (
                  'Every configured server'
                ) : agent.mcpServers.names.length ? (
                  <span className="code-list">
                    {agent.mcpServers.names.map((n) => (
                      <Code key={n}>{n}</Code>
                    ))}
                  </span>
                ) : (
                  'The list names no server'
                )}
              </dd>
            </div>
            {agent.readonly && (
              <div>
                <dt>Read-only</dt>
                <dd>The spec sets readonly: true</dd>
              </div>
            )}
            {agent.source && (
              <div>
                <dt>Declared in</dt>
                <dd>
                  <Code>{sourceRef(agent.source)}</Code>
                </dd>
              </div>
            )}
          </dl>
        )}
      </div>
      <div className="subject-card harness-card">
        <p className="eyebrow">Coding tool</p>
        <p className="subject-name">{harness.label}</p>
        <p className="muted">{permissionSupportLabel(harness.permissionSupport)}</p>
        <p className="harness-installed">
          <Badge tone={installed.tone}>Installed: {installed.text.toLowerCase()}</Badge>{' '}
          <span className="muted">
            {installed.note}
            {hypothetical ? ` Effects are what ${harness.label} would do once synced.` : ''}
          </span>
        </p>
        {!compact && harness.identity.description && <p className="muted">{harness.identity.description}</p>}
        {!compact && harness.note && <p className="muted">{harness.note}</p>}
      </div>
    </section>
  )
}

export function DomainGrid({ view }: { view: AuthorityView }) {
  const cards = domainCards(view)
  return (
    <section className="block" aria-labelledby="domains-title">
      <h2 id="domains-title" className="section-label">
        Capability domains
      </h2>
      <p className="muted section-note">Domains organize navigation; policies decide the actual operations.</p>
      <ul className="domain-grid">
        {cards.map((card) => (
          <li key={card.domain.id}>
            <AppLink to={{ domain: card.domain.id, tab: 'resources' }} className="domain-card">
              <span className="domain-card-head">
                <span className="domain-card-title">{card.domain.label}</span>
                <Icon name="arrow" className="domain-arrow" />
              </span>
              <span className="domain-card-desc">{card.domain.description}</span>
              {card.domain.id === SECRETS_DOMAIN ? (
                <span className="domain-card-secrets">
                  {plural(card.secretCount, 'credential')} referenced by name · values never shown
                </span>
              ) : (
                <EffectCounts counts={card.domain.summary} hypothetical={card.hypothetical} compact />
              )}
              <span className="domain-card-providers">
                {card.providerLabels.length ? card.providerLabels.join(' · ') : `No provider configured · e.g. ${card.domain.examples.slice(0, 3).join(', ')}`}
              </span>
            </AppLink>
          </li>
        ))}
      </ul>
    </section>
  )
}

export function ResourceCard({ resource, provider, domainId }: { resource: Resource; provider: Provider; domainId: string }) {
  const { title, subtitle } = resourceTitle(resource, provider)
  const status = resourceStatus(resource)
  return (
    <AppLink to={{ domain: domainId, resource: resource.id }} className="resource-card">
      <span className="resource-card-head">
        <span className="resource-card-title">{title}</span>
        <StatusPill status={status} resource={resource} />
      </span>
      {subtitle && <span className="resource-card-subtitle">{subtitle}</span>}
      <OperationList operations={resource.operations} />
      <span className="resource-card-hint">Select to inspect conditions, approval rules, and provenance</span>
    </AppLink>
  )
}

function ProviderGroup({ view, provider, resources, domainId }: { view: AuthorityView; provider: Provider; resources: Resource[]; domainId: string }) {
  const unavailable = provider.status === 'unavailable-to-agent'
  return (
    <section className="provider-group" aria-label={`${provider.label} resources`}>
      <header className="provider-head">
        <span className="provider-chip">{provider.label}</span>
        <span className="provider-desc">
          {provider.description}
          {provider.transport?.type && <span className="muted"> · {provider.kind === 'mcp' ? `MCP server over ${provider.transport.type}` : 'Built into the coding tool'}</span>}
          {provider.kind === 'builtin' && !provider.transport && <span className="muted"> · Built into the coding tool</span>}
        </span>
      </header>
      {unavailable && (
        <p className="provider-note">
          <Badge tone="neutral">Unavailable to {view.agent.label}</Badge> Its spec leaves this server out; other routes may still reach the same system.
        </p>
      )}
      {provider.status === 'disabled' && (
        <p className="provider-note">
          <Badge tone="neutral">Disabled</Badge> The spec switches this server off.
        </p>
      )}
      {provider.domainResolution === 'default' && <p className="provider-note muted">No curated domain matches this provider, so it is listed here by default.</p>}
      <ul className="resource-list">
        {resources.map((r) => (
          <li key={r.id}>
            <ResourceCard resource={r} provider={provider} domainId={domainId} />
          </li>
        ))}
      </ul>
      {provider.coverageNote && <p className="provider-coverage muted">{provider.coverageNote}</p>}
    </section>
  )
}

export function DomainResources({ view, domain }: { view: AuthorityView; domain: Domain }) {
  const groups = providersInDomain(view, domain.id)
  if (groups.length === 0) {
    return (
      <div className="empty">
        <p className="empty-title">No configured provider falls in this domain.</p>
        <p className="muted">Examples of what would: {domain.examples.join(', ')}.</p>
      </div>
    )
  }
  return (
    <div className="provider-groups">
      {groups.map((g) => (
        <ProviderGroup key={g.provider.id} view={view} provider={g.provider} resources={g.resources} domainId={domain.id} />
      ))}
    </div>
  )
}

export function SecretsPanel({ view }: { view: AuthorityView }) {
  const providerLabel = (id: string) => view.providers.find((p) => p.id === id)?.label ?? id
  return (
    <div className="secrets">
      <section className="panel identity-card" aria-labelledby="identity-title">
        <h3 id="identity-title">Who the tool acts as</h3>
        <p>{view.harness.identity.description || 'The tool runs as a local process.'}</p>
        <p className="muted">Kind: {view.harness.identity.kind === 'local-process' ? 'local process' : view.harness.identity.kind}. No user name is resolved.</p>
      </section>
      {view.secrets.length === 0 ? (
        <p className="muted">The configuration references no credential by name.</p>
      ) : (
        <ul className="secret-list">
          {view.secrets.map((s) => (
            <li key={s.id} className="panel secret-card">
              <div className="secret-head">
                <h3>{s.label}</h3>
                <Code>{s.name}</Code>
              </div>
              <p className="muted">
                Used by {s.providerIds.map(providerLabel).join(', ') || 'no provider'}
                {s.referencedBy.length > 0 && (
                  <>
                    {' '}
                    · referenced in{' '}
                    {s.referencedBy.map((ref, i) => (
                      <span key={i}>
                        {i > 0 && ', '}
                        <Code>{sourceRef(ref.source)}</Code> ({ref.field})
                      </span>
                    ))}
                  </>
                )}
              </p>
              <ul className="rights" aria-label={`Rights over ${s.label}`}>
                {(Object.keys(RIGHT_NAMES) as Array<keyof typeof RIGHT_NAMES>).map((key) => {
                  const right = rightDisplay(s.rights[key])
                  return (
                    <li key={key} className={`right right-${right.tone}`} title={view.explanations.rights[s.rights[key]]}>
                      <span className="right-name">{RIGHT_NAMES[key]}</span>
                      <Badge tone={right.tone}>{right.text}</Badge>
                    </li>
                  )
                })}
              </ul>
              <p>{s.explanation}</p>
            </li>
          ))}
        </ul>
      )}
      <p className="muted">Discover, read value, use, and manage are distinct rights. Secret values are never read, shown, or sent.</p>
    </div>
  )
}

export function DomainBanner({ view, domain }: { view: AuthorityView; domain: Domain }) {
  const providers = providersInDomain(view, domain.id).length
  const hypothetical = view.resources.some((r) => domain.resourceIds.includes(r.id) && r.operations.some((o) => o.hypothetical))
  return (
    <section className="domain-banner" aria-label={`${domain.label} summary`}>
      <p className="domain-banner-line">
        {domain.id === SECRETS_DOMAIN ? plural(view.secrets.length, 'credential') : plural(providers, 'provider')} · {domain.description}
      </p>
      {domain.id !== SECRETS_DOMAIN && <EffectCounts counts={domain.summary} hypothetical={hypothetical} />}
      <p className="domain-banner-examples muted">
        Examples of this domain: {domain.examples.join(', ')}
        {domain.providerIds.length > 0 && (
          <>
            {' '}
            · configured here:{' '}
            {domain.providerIds.map((id) => (
              <Chip key={id}>{view.providers.find((p) => p.id === id)?.label ?? id}</Chip>
            ))}
          </>
        )}
      </p>
    </section>
  )
}
