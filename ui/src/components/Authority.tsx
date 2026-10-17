// The Authority deep view: domains → providers → resources → operations,
// the permission circuit, the proposal panel, and the uncertainties.

import { useEffect, useMemo, useRef, useState } from 'react'
import type { AuthorityView, Change, Resource } from '../contract'
import type { ImpactState } from '../App'
import { validationAvailability } from '../api'
import { findDomain, findProvider, findResource, primaryDomain, providersInDomain, resourceTitle, SECRETS_DOMAIN } from '../model/domains'
import { buildProposal, previewProposal, sameChange } from '../model/proposal'
import { AppLink, PageHeader, useApp } from './app-context'
import { DomainBanner, DomainGrid, DomainResources, SecretsPanel, SubjectStrip } from './AuthorityDomains'
import { ErrorPanel, Notice } from './Banner'
import { Circuit, type ProposedMark } from './Circuit'
import { ProposalPanel } from './Proposal'
import { ResourceDetail } from './ResourceDetail'
import type { Remote } from './Shell'
import { Uncertainties } from './Uncertainties'

export function AuthorityPage({ authority, impact, onValidate }: { authority: Remote<AuthorityView>; impact: ImpactState; onValidate: () => void }) {
  const app = useApp()
  const view = authority.loaded?.data
  const [draftOpen, setDraftOpen] = useState(false)
  // A proposal carried in the URL on arrival, from a feed card or a shared
  // link, takes focus once the view is ready, so its preview is in sight.
  const arrivedWithProposal = useRef(!!app.state.proposal?.changes.length)
  useEffect(() => {
    if (!arrivedWithProposal.current || !view) return
    arrivedWithProposal.current = false
    window.requestAnimationFrame(() => {
      const title = document.getElementById('proposal-title')
      title?.scrollIntoView({ block: 'start' })
      title?.focus({ preventScroll: true })
    })
  }, [view])
  const changes = useMemo(() => app.state.proposal?.changes ?? [], [app.state.proposal])
  const preview = useMemo(() => (view && changes.length ? previewProposal(view, changes) : undefined), [view, changes])

  // Which edges the proposal touches: the server's validated changes
  // once it has answered, the client preview until then.
  const proposed = useMemo(() => {
    const marks = new Map<string, ProposedMark>()
    const result = impact.status === 'done' ? impact.result?.data : undefined
    if (view && result?.valid) {
      for (const c of result.changes) if (c.subjectId === view.agent.id) marks.set(c.operationId, { effect: c.effectiveAfter, state: 'validated' })
    } else if (preview) {
      for (const c of preview.changes) marks.set(c.operationId, { effect: c.effectiveAfter, state: 'proposed' })
    }
    return marks
  }, [view, preview, impact])

  if (!view) {
    return (
      <>
        <PageHeader title="Authority" subtitle="Which exact resources can the agent reach, and why?" />
        {authority.error ? (
          <>
            <ErrorPanel error={authority.error} />
            {app.source.kind === 'api' && (
              <p>
                <AppLink to={{ mock: true }}>Open the bundled mock data instead</AppLink>
              </p>
            )}
          </>
        ) : (
          <p className="loading" aria-busy="true">
            Loading the Authority view…
          </p>
        )}
      </>
    )
  }

  const resource = findResource(view, app.state.resource)
  const requestedDomain = findDomain(view, app.state.domain)
  const domain = resource
    ? requestedDomain && (resource.domainIds.includes(requestedDomain.id) || requestedDomain.resourceIds.includes(resource.id))
      ? requestedDomain
      : findDomain(view, primaryDomain(view, resource))
    : requestedDomain

  const setChanges = (next: Change[]) => {
    app.navigate({ proposal: next.length ? buildProposal(view, next) : undefined }, { replace: true })
  }
  const focusPanel = () => window.requestAnimationFrame(() => document.getElementById('proposal-title')?.focus())
  const addChanges = (added: Change[]) => {
    const merged = [...changes, ...added.filter((c) => !changes.some((e) => sameChange(e, c)))]
    setChanges(merged)
    setDraftOpen(true)
    focusPanel()
  }
  const openDraft = () => {
    setDraftOpen(true)
    window.requestAnimationFrame(() => {
      const input = document.querySelector<HTMLInputElement>('.rule-form input')
      input?.focus()
      input?.scrollIntoView({ block: 'center' })
    })
  }

  const showProposal = changes.length > 0 || draftOpen
  const proposalPanel = showProposal ? (
    <ProposalPanel
      view={view}
      changes={changes}
      preview={preview}
      impact={impact}
      validation={validationAvailability(app.source)}
      onChange={setChanges}
      onValidate={onValidate}
      onDiscard={() => {
        setDraftOpen(false)
        setChanges([])
      }}
    />
  ) : null

  const notices = (
    <>
      {authority.loaded?.notice && <Notice>{authority.loaded.notice}</Notice>}
      {authority.loaded?.schemaWarning && <Notice tone="warning">{authority.loaded.schemaWarning}</Notice>}
      {authority.status === 'loading' && <p className="loading-inline" aria-live="polite">Updating…</p>}
      {authority.status === 'error' && authority.error && <ErrorPanel error={authority.error} />}
    </>
  )

  const subtitle = `${view.agent.label} on ${view.harness.label}`

  if (resource && domain) {
    const provider = findProvider(view, resource.providerId)
    const { title, subtitle: resourceSubtitle } = resourceTitle(resource, provider)
    return (
      <>
        <PageHeader eyebrow={<Breadcrumbs view={view} domainId={domain.id} resource={resource} />} title={title} subtitle={resourceSubtitle ?? subtitle} />
        {notices}
        <SubjectStrip view={view} compact />
        <ResourceDetail
          view={view}
          resource={resource}
          onPropose={addChanges}
          onOpenDraft={openDraft}
          proposalSlot={proposalPanel}
          circuitSlot={
            <section className="block" aria-labelledby="circuit-title">
              <h2 id="circuit-title">Permission circuit</h2>
              <Circuit view={view} resources={[resource]} proposed={proposed} selectedResourceId={resource.id} domainId={domain.id} caption={`Permission circuit for ${title}: ${subtitle}`} />
            </section>
          }
        />
        <Uncertainties view={view} />
      </>
    )
  }

  if (domain) {
    const resources: Resource[] = providersInDomain(view, domain.id).flatMap((g) => g.resources)
    const isSecrets = domain.id === SECRETS_DOMAIN
    const tab = isSecrets ? 'resources' : app.state.tab
    return (
      <>
        <PageHeader eyebrow={<Breadcrumbs view={view} domainId={domain.id} />} title={domain.label} subtitle={`Capability domain → providers → concrete resources · ${subtitle}`} />
        {notices}
        <SubjectStrip view={view} compact />
        <DomainBanner view={view} domain={domain} />
        {!isSecrets && resources.length > 0 && (
          <div className="segmented view-switch" role="group" aria-label="Show the domain as">
            <button type="button" aria-pressed={tab === 'resources'} onClick={() => app.navigate({ tab: 'resources' }, { replace: true })}>
              Resources
            </button>
            <button type="button" aria-pressed={tab === 'circuit'} onClick={() => app.navigate({ tab: 'circuit' }, { replace: true })}>
              Circuit
            </button>
          </div>
        )}
        {isSecrets ? (
          <SecretsPanel view={view} />
        ) : tab === 'circuit' && resources.length > 0 ? (
          <section className="block" aria-labelledby="circuit-title">
            <h2 id="circuit-title" className="sr-only">
              Permission circuit
            </h2>
            <Circuit view={view} resources={resources} proposed={proposed} domainId={domain.id} caption={`Permission circuit for ${domain.label}: ${subtitle}`} />
          </section>
        ) : (
          <DomainResources view={view} domain={domain} />
        )}
        <p className="domain-note">Provider is not capability: domains organize navigation, and policies decide the actual operations.</p>
        {proposalPanel}
        <Uncertainties view={view} />
      </>
    )
  }

  return (
    <>
      <PageHeader title="Authority" subtitle={`${subtitle} · Which exact resources can it reach, and why?`} />
      {notices}
      <SubjectStrip view={view} />
      <DomainGrid view={view} />
      {proposalPanel}
      <Uncertainties view={view} />
    </>
  )
}

function Breadcrumbs({ view, domainId, resource }: { view: AuthorityView; domainId: string; resource?: Resource }) {
  const domain = findDomain(view, domainId)
  return (
    <nav aria-label="Breadcrumb" className="breadcrumbs">
      <ol>
        <li>
          <AppLink to={{ domain: undefined, resource: undefined, tab: 'resources' }}>Authority</AppLink>
        </li>
        <li>
          {resource ? (
            <AppLink to={{ domain: domainId, resource: undefined }}>{domain?.label ?? domainId}</AppLink>
          ) : (
            <span aria-current="page">{domain?.label ?? domainId}</span>
          )}
        </li>
        {resource && (
          <li>
            <span aria-current="page">{resourceTitle(resource, findProvider(view, resource.providerId)).title}</span>
          </li>
        )}
      </ol>
    </nav>
  )
}
