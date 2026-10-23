import { useState } from 'react'
import { API, type ItemFinding, type OverviewView, type Section, type SectionItem, type SettingField, type Stage, type ToolListing, type Value } from '../contract'
import { EditError, postEdit } from '../api'
import { PageHeader, useApp } from './app-context'
import { ErrorPanel } from './Banner'
import { cursorUrl } from '../model/editor'
import type { Remote } from './Shell'
import { Badge, Chip, Code, Icon, Switch, Toggle, ToolAvatar } from './ui'

export function SectionPage({ sectionId, overview }: { sectionId: string; overview: Remote<OverviewView> }) {
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
  const toolLabel = (target: string) => data.targets.find((t) => t.target === target)?.label ?? target

  return (
    <>
      <PageHeader title={section.label} subtitle={section.question} />
      <SectionContents section={section} />
      <SupportTable section={section} toolLabel={toolLabel} />
    </>
  )
}

/** The section's items, kind by kind, with their findings. */
export function SectionContents({ section }: { section: Section }) {
  return (
    <>
      {section.findings.length > 0 && (
        <section className="block" aria-labelledby={`${section.id}-unplaced`}>
          <h2 id={`${section.id}-unplaced`}>Findings outside the configured items</h2>
          <Findings findings={section.findings} showPath />
        </section>
      )}
      {section.kinds.map((k) => {
        const items = section.items.filter((it) => it.kind === k.kind)
        if (k.kind === 'hook' && section.stages) return <Lifecycle key={k.kind} stages={section.stages} hooks={items} count={k.count} />
        if (k.kind === 'settings' && section.fields) return <SettingsFields key={k.kind} title={k.label} fields={section.fields} files={items} />
        const titleId = `${section.id}-${k.kind}`
        return (
          <section className="block" aria-labelledby={titleId} key={k.kind}>
            <h2 id={titleId}>
              {k.label} <span className="muted kind-count">{k.count}{k.kind === 'builtin' ? ' enabled' : ''}</span>
            </h2>
            {items.length === 0 ? <p className="muted">None configured.</p> : <GroupedItems items={items} />}
          </section>
        )
      })}
    </>
  )
}

/** Every field the section's settings can set, each with its value or "Not set", then the files that set them. */
function SettingsFields({ title, fields, files }: { title: string; fields: SettingField[]; files: SectionItem[] }) {
  const id = `fields-${title.toLowerCase().replace(/\W+/g, '-')}`
  return (
    <section className="block" aria-labelledby={id}>
      <h2 id={id}>{title}</h2>
      <ul className="fields">
        {fields.map((f) => (
          <li key={f.key} className={`field-row${f.set.length ? ' is-set' : ''}`}>
            <div className="field-head">
              <strong>{f.label}</strong> <code className="muted">{f.key === 'x-' ? 'x-<tool>' : f.key}</code>
              <p className="muted">{f.description}</p>
            </div>
            {f.set.length === 0 ? (
              <p className="muted field-unset">Not set</p>
            ) : (
              f.set.map((src) => (
                <div key={src.spec} className="field-source">
                  <Values values={src.values} file={src.path} />
                  <p className="muted field-from">
                    from <Code>{src.path}</Code>
                  </p>
                </div>
              ))
            )}
          </li>
        ))}
      </ul>
      {files.length > 0 && (
        <>
          <h3 className="item-group-title">Files</h3>
          <ItemList items={files} hideValues />
        </>
      )}
    </section>
  )
}

/** Every stage of a session in order, each with its hooks or none. */
function Lifecycle({ stages, hooks, count }: { stages: Stage[]; hooks: SectionItem[]; count: number }) {
  return (
    <section className="block" aria-labelledby="lifecycle-hooks">
      <h2 id="lifecycle-hooks">
        Hooks <span className="muted kind-count">{count}</span>
      </h2>
      <ol className="stages">
        {stages.map((st) => {
          const here = hooks.filter((h) => h.group === st.id)
          return (
            <li key={st.id} className={`stage${here.length ? ' has-hooks' : ''}`}>
              <div className="stage-head">
                <h3 className="stage-title">{st.label}</h3>
                <p className="muted">{st.description}</p>
                {st.targets.length > 0 && (
                  <p className="stage-tools">
                    {st.targets.map((t) => (
                      <ToolAvatar key={t.target} target={t.target} label={`${t.label} - ${t.event}`} />
                    ))}
                  </p>
                )}
              </div>
              {here.length ? <ItemList items={here} /> : <p className="muted stage-empty">No hook runs here.</p>}
            </li>
          )
        })}
      </ol>
    </section>
  )
}

function GroupedItems({ items }: { items: SectionItem[] }) {
  if (!items.some((it) => it.group)) return <ItemList items={items} />
  const groups: { name: string; items: SectionItem[] }[] = []
  for (const it of items) {
    const name = it.group ?? ''
    const last = groups[groups.length - 1]
    if (last && last.name === name) last.items.push(it)
    else groups.push({ name, items: [it] })
  }
  return (
    <div className="item-groups">
      {groups.map((g) => (
        <div key={g.name} className="item-group">
          <h3 className="item-group-title">{g.name}</h3>
          <ItemList items={g.items} />
        </div>
      ))}
    </div>
  )
}

function ItemList({ items, hideValues }: { items: SectionItem[]; hideValues?: boolean }) {
  return (
    <ul className="item-list">
      {items.map((item) => (
        <li key={`${item.kind}:${item.name}:${item.path}`} className={`item-row${item.findings.length ? ' has-findings' : ''}`}>
          <div className="item-main">
            <strong>{item.label}</strong>
            {item.description && <span className="muted"> · {item.description}</span>}
          </div>
          <div className="item-meta">
            {item.enabled !== undefined && (
              <EditSwitch on={item.enabled} label={`${item.name} built-in`} save={(on) => postEdit(API.editBuiltin, { name: item.name, enabled: on })} />
            )}
            {item.enabled === undefined && <Code>{item.path}</Code>}
            {item.enabled === undefined && <Chip title="Layer the item comes from">{item.layer}</Chip>}
            {item.scope && <Chip>{item.scope}</Chip>}
            {item.openPath && (
              <a className="edit-link" href={cursorUrl(item.openPath)} aria-label={`Edit ${item.label} in Cursor`} title="Edit in Cursor">
                <Icon name="edit" />
              </a>
            )}
          </div>
          {!hideValues && item.values.length > 0 && <Values values={item.values} file={item.kind === 'settings' ? item.path : undefined} />}
          {item.tools && <Tools listing={item.tools} />}
          {item.findings.length > 0 && <Findings findings={item.findings} />}
        </li>
      ))}
    </ul>
  )
}

/** An MCP server's tools, from the cache `agnostic-ai mcp tools` fills. */
function Tools({ listing }: { listing: ToolListing }) {
  const when = listing.listedAt ? new Date(listing.listedAt).toLocaleString() : ''
  return (
    <div className="tool-listing">
      {listing.tools.length > 0 && (
        <span className="value-chips">
          {listing.tools.map((t) => (
            <Chip key={t.name} title={t.description || t.title}>
              {t.name}
            </Chip>
          ))}
        </span>
      )}
      {listing.state === 'stale' && (
        <p className="tool-state is-warning">
          The spec changed after these were listed ({when}). Run <Code>{listing.command}</Code> to list them again.
        </p>
      )}
      {listing.state === 'failed' && (
        <p className="tool-state is-warning">
          Could not list the tools: {listing.error}. Run <Code>{listing.command}</Code> to try again.
        </p>
      )}
      {listing.state === 'not-listed' && (
        <p className="muted tool-state">
          Tools not listed yet. Run <Code>{listing.command}</Code> to list them.
        </p>
      )}
    </div>
  )
}

/**
 * A switch that saves its setting through the server. The server
 * validates the edit and restores the file when it fails; the reason
 * shows under the switch. Snapshots and the mock have no server, so the
 * switch only shows the value there.
 */
function EditSwitch({ on, label, save }: { on: boolean; label: string; save: (on: boolean) => Promise<void> }) {
  const app = useApp()
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<EditError | undefined>()
  if (app.source.kind !== 'api') return <Switch on={on} />
  const toggle = async () => {
    setPending(true)
    setError(undefined)
    try {
      await save(!on)
      app.reload()
    } catch (err) {
      setError(err instanceof EditError ? err : new EditError(String(err)))
    } finally {
      setPending(false)
    }
  }
  return (
    <span className="edit-switch">
      <Toggle on={on} label={label} pending={pending} onToggle={toggle} />
      {error && (
        <span className="edit-error" role="alert">
          {error.message}
          {error.problems.length > 0 && (
            <ul>
              {error.problems.map((p) => (
                <li key={p}>{p}</li>
              ))}
            </ul>
          )}
        </span>
      )}
    </span>
  )
}

/** Each setting as its kind reads best: chips for a list, a switch for a boolean, text otherwise. */
function Values({ values, file }: { values: Value[]; file?: string }) {
  return (
    <dl className="values">
      {values.map((v) => (
        <div key={v.key} className="value-row">
          <dt>
            <code>{v.key}</code>
          </dt>
          <dd>
            {v.type === 'list' ? (
              <span className="value-chips">
                {(v.list ?? []).length ? (v.list ?? []).map((entry, i) => <Chip key={`${i}-${entry}`}>{entry}</Chip>) : <span className="muted">Empty</span>}
              </span>
            ) : v.type === 'bool' ? (
              file ? (
                <EditSwitch on={v.value === 'true'} label={v.key} save={(on) => postEdit(API.editValue, { path: file, key: v.key, value: on })} />
              ) : (
                <Switch on={v.value === 'true'} />
              )
            ) : (
              <span className="value-text">{v.value}</span>
            )}
          </dd>
        </div>
      ))}
    </dl>
  )
}

function Findings({ findings, showPath }: { findings: ItemFinding[]; showPath?: boolean }) {
  return (
    <ul className="findings">
      {findings.map((f, i) => (
        <li key={`${f.code ?? f.source}-${f.line ?? 0}-${i}`} className={`finding finding-${f.severity === 'error' ? 'error' : 'warn'}`}>
          <Icon name="warning" className="finding-icon" />
          <div className="finding-body">
            <p>
              <Badge tone={f.severity === 'error' ? 'blocked' : 'approval'}>{f.severity === 'error' ? 'Error' : 'Warning'}</Badge>{' '}
              {f.code && <Code>{f.code}</Code>} {f.line ? <span className="muted">Line {f.line}: </span> : null}
              {f.message}
            </p>
            {showPath && f.path && (
              <p className="muted">
                <Code>{f.path}</Code>
              </p>
            )}
            {f.explanation && <p className="muted">{f.explanation}</p>}
            {f.fix && (
              <p className="muted">
                Fix: <Code>{f.fix}</Code>
              </p>
            )}
          </div>
        </li>
      ))}
    </ul>
  )
}

function SupportTable({ section, toolLabel }: { section: Section; toolLabel: (target: string) => string }) {
  if (!section.support.length) return null
  return (
    <section className="block" aria-labelledby="support-title">
      <h2 id="support-title">Coding tool support</h2>
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
    </section>
  )
}
