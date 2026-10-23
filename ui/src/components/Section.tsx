// Overview page for a harness section: what it holds, item by item,
// with each check finding on the item it names, and which coding tools
// support each kind.

import type { ItemFinding, OverviewView, Section, SectionItem, SettingField, Stage, Value } from '../contract'
import { PageHeader } from './app-context'
import { ErrorPanel } from './Banner'
import type { Remote } from './Shell'
import { Badge, Chip, Code, Icon, Switch } from './ui'

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
              {k.label} <span className="muted kind-count">{k.count}</span>
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
                  <Values values={src.values} />
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
                      <Chip key={t.target} title={`${t.label} calls this ${t.event}`}>
                        <span>{t.label}</span>
                        <code>{t.event}</code>
                      </Chip>
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
            {item.enabled !== undefined && <Switch on={item.enabled} />}
            {item.enabled === undefined && <Code>{item.path}</Code>}
            {item.enabled === undefined && <Chip title="Layer the item comes from">{item.layer}</Chip>}
            {item.scope && <Chip>{item.scope}</Chip>}
            {item.openPath && (
              <a className="button button-quiet open-link" href={cursorUrl(item.openPath)}>
                Open in Cursor<span className="sr-only"> {item.label}</span>
              </a>
            )}
          </div>
          {!hideValues && item.values.length > 0 && <Values values={item.values} />}
          {item.findings.length > 0 && <Findings findings={item.findings} />}
        </li>
      ))}
    </ul>
  )
}

/** A link Cursor opens: cursor://file/<absolute path>. */
function cursorUrl(path: string): string {
  return `cursor://file${encodeURI(path.startsWith('/') ? path : `/${path}`)}`
}

/** Each setting as its kind reads best: chips for a list, a switch for a boolean, text otherwise. */
function Values({ values }: { values: Value[] }) {
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
              <Switch on={v.value === 'true'} />
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
