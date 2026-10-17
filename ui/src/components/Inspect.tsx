// Inspect: what the views were built from, coverage per coding tool,
// the supported actions, and every uncertainty.

import type { AuthorityView, OverviewView } from '../contract'
import { installedDisplay } from '../model/effects'
import { formatTime } from '../model/format'
import { PageHeader, useApp } from './app-context'
import { ErrorPanel } from './Banner'
import type { Remote } from './Shell'
import { Uncertainties } from './Uncertainties'
import { Badge, Chip, Code } from './ui'

export function InspectPage({ authority, overview }: { authority: Remote<AuthorityView>; overview: Remote<OverviewView> }) {
  const app = useApp()
  const view = authority.loaded?.data
  const data = overview.loaded?.data
  const snapshot = view?.snapshot ?? data?.snapshot
  if (!snapshot) {
    return (
      <>
        <PageHeader title="Inspect" subtitle="What these views were built from" />
        {authority.error || overview.error ? <ErrorPanel error={(authority.error ?? overview.error)!} /> : <p className="loading" aria-busy="true">Loading…</p>}
      </>
    )
  }
  const kinds = data ? [...new Set(data.targets.flatMap((t) => t.kinds))].sort() : []

  return (
    <>
      <PageHeader title="Inspect" subtitle="What these views were built from, and what they cannot tell">
        <button type="button" className="button" onClick={() => app.openInspector()}>
          Open the JSON inspector
        </button>
      </PageHeader>

      <section className="block" aria-labelledby="evidence-title">
        <h2 id="evidence-title">Evidence</h2>
        <dl className="facts">
          <div>
            <dt>Declared</dt>
            <dd>
              <Badge tone="info">{snapshot.evidence.declared === 'loaded' ? 'Loaded from the specs' : snapshot.evidence.declared}</Badge> {formatTime(snapshot.generatedAt)}
            </dd>
          </div>
          <div>
            <dt>Installed</dt>
            <dd>
              <Badge tone={snapshot.evidence.installed === 'checked' ? 'info' : 'neutral'}>{snapshot.evidence.installed === 'checked' ? 'Compared with disk' : 'Not compared with disk'}</Badge>
              {view && (
                <span className="muted">
                  {' '}
                  On {view.harness.label}: {installedDisplay(view.harness.installed.state, view.explanations).text.toLowerCase()}
                </span>
              )}
            </dd>
          </div>
          <div>
            <dt>Runtime</dt>
            <dd>
              <Badge tone="neutral">Not observed</Badge>
            </dd>
          </div>
        </dl>
        {snapshot.evidence.note && <p className="muted">{snapshot.evidence.note}</p>}
      </section>

      {view && (
        <section className="block" aria-labelledby="files-title">
          <h2 id="files-title">Generated files for {view.harness.label}</h2>
          {view.harness.installed.files.length ? (
            <ul className="file-list">
              {view.harness.installed.files.map((f) => {
                const display = installedDisplay(f.state, view.explanations)
                return (
                  <li key={f.path}>
                    <Code>{f.path}</Code> <Badge tone={display.tone}>{display.text}</Badge> <span className="muted">{display.note}</span>
                  </li>
                )
              })}
            </ul>
          ) : (
            <p className="muted">Sync writes no permission file for this tool.</p>
          )}
        </section>
      )}

      <section className="block" aria-labelledby="specs-title">
        <h2 id="specs-title">Loaded specs</h2>
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th scope="col">Name</th>
                <th scope="col">Kind</th>
                <th scope="col">Source</th>
                <th scope="col">Layer</th>
              </tr>
            </thead>
            <tbody>
              {snapshot.specs.map((s) => (
                <tr key={`${s.kind}:${s.name}:${s.path}`}>
                  <th scope="row">{s.name}</th>
                  <td>{s.kind}</td>
                  <td>
                    <Code>{s.path}</Code>
                  </td>
                  <td>{s.layer}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {snapshot.configFiles.length > 0 && (
          <p className="muted">
            Configuration read from{' '}
            {snapshot.configFiles.map((f, i) => (
              <span key={f}>
                {i > 0 && ', '}
                <Code>{f}</Code>
              </span>
            ))}
            .
          </p>
        )}
      </section>

      {data && data.targets.length > 0 && (
        <section className="block" aria-labelledby="coverage-title">
          <h2 id="coverage-title">Tool coverage</h2>
          <p className="muted">Which spec kinds each configured coding tool emits natively, from the adapters' own declarations.</p>
          <div className="table-wrap">
            <table className="table coverage-table">
              <thead>
                <tr>
                  <th scope="col">Kind</th>
                  {data.targets.map((t) => (
                    <th key={t.target} scope="col">
                      {t.label}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {kinds.map((kind) => (
                  <tr key={kind}>
                    <th scope="row">{kind}</th>
                    {data.targets.map((t) => (
                      <td key={t.target}>{t.kinds.includes(kind) ? <Chip tone="permitted">Emitted</Chip> : <span className="muted">Not emitted</span>}</td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {view && (
        <section className="block" aria-labelledby="actions-title">
          <h2 id="actions-title">What a client may do</h2>
          <ul className="actions-list">
            {view.supportedActions.map((a) => (
              <li key={a.id}>
                <Badge tone={a.status === 'available' ? 'info' : 'neutral'}>{a.status === 'available' ? 'Available' : 'Unsupported'}</Badge>{' '}
                {a.method && a.path && (
                  <Code>
                    {a.method} {a.path}
                  </Code>
                )}{' '}
                <span>{a.description}</span>
              </li>
            ))}
          </ul>
        </section>
      )}

      {view && <Uncertainties view={view} defaultOpen />}
    </>
  )
}
