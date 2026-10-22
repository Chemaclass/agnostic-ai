// Overview page for a harness section: what it holds, which coding
// tools support each kind, and its cards.

import type { AuthorityView, OverviewView } from '../contract'
import type { FeedDecisions } from '../model/feed'
import { cardKindLabel } from '../model/feed'
import { plural } from '../model/format'
import { AppLink, PageHeader } from './app-context'
import { ErrorPanel } from './Banner'
import { priorityTone } from './Feed'
import type { Remote } from './Shell'
import { Badge, Chip, Code } from './ui'

export function SectionPage({
  sectionId,
  overview,
  authority,
  decisions,
}: {
  sectionId: string
  overview: Remote<OverviewView>
  authority: Remote<AuthorityView>
  decisions: FeedDecisions
}) {
  const data = overview.loaded?.data
  if (!data) {
    return (
      <>
        <PageHeader title={sectionId.charAt(0).toUpperCase() + sectionId.slice(1)} />
        {overview.error ? <ErrorPanel error={overview.error} /> : <p className="loading" aria-busy="true">Loading the overview…</p>}
      </>
    )
  }
  const section = data.sections.find((s) => s.id === sectionId)
  if (!section) {
    return (
      <>
        <PageHeader title="Section not found" />
        <p>The overview has no section named {sectionId}.</p>
      </>
    )
  }

  const toolLabel = (target: string) =>
    data.targets.find((t) => t.target === target)?.label ?? authority.loaded?.data.harness.available.find((h) => h.target === target)?.label ?? target
  const cards = section.cardIds.map((id) => data.feed.find((c) => c.id === id)).filter((c): c is NonNullable<typeof c> => !!c)
  const kindLabel = (kind: string) => section.kinds.find((k) => k.kind === kind)?.label ?? kind

  return (
    <>
      <PageHeader title={section.label} subtitle={section.question} />

      <section className="block" aria-labelledby="kinds-title">
        <h2 id="kinds-title">What it holds</h2>
        {section.kinds.length ? (
          <ul className="kind-counts">
            {section.kinds.map((k) => (
              <li key={k.kind}>
                <span className="kind-count">{k.count}</span>
                <span>{k.label}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="muted">This section declares no spec kinds.</p>
        )}
      </section>

      <section className="block" aria-labelledby="items-title">
        <h2 id="items-title">Items</h2>
        {section.items.length ? (
          <ul className="item-list">
            {section.items.map((item) => (
              <li key={`${item.kind}:${item.name}`} className="item-row">
                <div className="item-main">
                  <strong>{item.label}</strong>
                  {item.description && <span className="muted"> · {item.description}</span>}
                </div>
                <div className="item-meta">
                  <span className="muted">{kindLabel(item.kind)}</span>
                  <Code>{item.path}</Code>
                  <Chip title="Layer the spec comes from">{item.layer}</Chip>
                  {item.scope && <Chip>{item.scope}</Chip>}
                </div>
              </li>
            ))}
          </ul>
        ) : (
          <p className="muted">Nothing is configured in this section yet.</p>
        )}
      </section>

      <section className="block" aria-labelledby="support-title">
        <h2 id="support-title">Coding tool support</h2>
        {section.support.length ? (
          <div className="table-wrap">
            <table className="table support-table">
              <thead>
                <tr>
                  <th scope="col">Kind</th>
                  <th scope="col">Supported on</th>
                  <th scope="col">Not supported on</th>
                </tr>
              </thead>
              <tbody>
                {section.support.map((k) => (
                  <tr key={k.kind}>
                    <th scope="row">{k.label}</th>
                    <td>
                      {k.supported.length ? (
                        <span className="tool-list">
                          {k.supported.map((t) => (
                            <Chip key={t} tone="permitted">
                              {toolLabel(t)}
                            </Chip>
                          ))}
                        </span>
                      ) : (
                        <span className="muted">No configured tool</span>
                      )}
                    </td>
                    <td>
                      {k.unsupported.length ? (
                        <span className="tool-list">
                          {k.unsupported.map((t) => (
                            <Chip key={t}>{toolLabel(t)}</Chip>
                          ))}
                        </span>
                      ) : (
                        <span className="muted">Every configured tool supports it</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="muted">The adapters declare no support for this section's kinds.</p>
        )}
      </section>

      <section className="block" aria-labelledby="cards-title">
        <h2 id="cards-title">Feed cards for {section.label}</h2>
        {cards.length ? (
          <ul className="mini-cards">
            {cards.map((card) => (
              <li key={card.id} className="mini-card">
                <div className="card-meta">
                  <Badge tone={priorityTone(card.priority)}>{card.priorityLabel}</Badge>
                  <span className="card-kind">{cardKindLabel(card.kind)}</span>
                  {decisions.dismissed[card.fingerprint] && <span className="card-acted">Dismissed in this browser</span>}
                </div>
                <AppLink to={{ view: 'feed', card: card.id }} className="mini-card-title">
                  {card.title}
                </AppLink>
                <p className="muted">{card.consequence}</p>
              </li>
            ))}
          </ul>
        ) : (
          <p className="muted">No card concerns this section. {plural(data.feed.length, 'card is', 'cards are')} in the feed overall.</p>
        )}
      </section>
    </>
  )
}
