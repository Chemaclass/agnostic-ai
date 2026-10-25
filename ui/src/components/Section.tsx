import { createContext, useContext, useId, useRef, useState, type ReactNode } from 'react'
import { API, type ItemFinding, type OverviewView, type Section, type SectionItem, type SettingField, type Stage, type ToolListing, type Value } from '../contract'
import { EditError, postEdit } from '../api'
import { PageHeader, useApp } from './app-context'
import { ErrorPanel } from './Banner'
import { BuiltinChange, BuiltinChangeControls, BuiltinChangeProvider } from './BuiltinChange'
import { cursorUrl } from '../model/editor'
import { findingFixPrompt } from '../model/finding-prompt'
import type { Remote } from './Shell'
import { Badge, Chip, Code, CopyButton, Icon, Switch, Toggle, ToolAvatar } from './ui'

const ConfigurationRoot = createContext<string | undefined>(undefined)

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
      <ConfigurationRoot.Provider value={data.snapshot.project.root}>
        <SectionContents key={section.id} section={section} />
      </ConfigurationRoot.Provider>
      <SupportTable section={section} toolLabel={toolLabel} />
    </>
  )
}

export function SectionContents({ section }: { section: Section }) {
  return <BuiltinChangeProvider active={section.items.some((item) => item.kind === 'builtin')}><SectionContentsBody section={section} /></BuiltinChangeProvider>
}

function SectionContentsBody({ section }: { section: Section }) {
  const [search, setSearch] = useState('')
  const searchInput = useRef<HTMLInputElement>(null)
  const searchId = useId()
  const query = search.trim().toLowerCase()
  const matches = (parts: (string | undefined)[]) => parts.some((part) => part?.toLowerCase().includes(query))
  const matchingItems = new Set(section.items.filter((item) => matches([
    item.name, item.label, item.path, item.description, ...searchValues(item.values),
    ...(item.tools?.tools.flatMap((tool) => [tool.name, tool.title, tool.description]) ?? []),
  ])))
  const matchingFields = section.fields?.filter((field) => matches([
    field.key, field.label, field.description,
    ...field.set.flatMap((source) => [source.spec, source.path, ...searchValues(source.values)]),
  ]))
  const visibleItems = section.items.filter((item) => matchingItems.has(item) || item.findings.length > 0)
  const matchCount = matchingItems.size + (matchingFields?.length ?? 0)
  const totalCount = section.items.length + (section.fields?.length ?? 0)
  const retainedFindings = section.items.some((item) => item.findings.length > 0 && !matchingItems.has(item))
  const errors = section.findings.filter((finding) => finding.severity === 'error').length
  const warnings = section.findings.length - errors
  const findingsId = `${section.id}-findings`
  const findingCounts = [errors ? `${errors} error${errors === 1 ? '' : 's'}` : '', warnings ? `${warnings} warning${warnings === 1 ? '' : 's'}` : ''].filter(Boolean).join(', ')
  const otherFindings = section.findings.length > 0 && (
    <section className="block" aria-labelledby={findingsId}>
      <h2 id={findingsId} tabIndex={-1}>Other findings</h2>
      <Findings findings={section.findings} />
    </section>
  )
  return (
    <>
      <div className="section-search" role="search" aria-label={`Search ${section.label}`}>
        <label htmlFor={searchId}>Search {section.label.toLowerCase()}</label>
        <div className="section-search-controls">
          <input ref={searchInput} id={searchId} type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Name, path, description, or value" aria-describedby={`${searchId}-status`} />
          {search && <button type="button" className="button button-quiet" onClick={() => {
            setSearch('')
            searchInput.current?.focus()
          }}>Clear search</button>}
        </div>
        <p id={`${searchId}-status`} className="muted section-search-status" role="status">
          {query ? `${matchCount} of ${totalCount} items${section.fields ? ' and fields' : ''} match.` : `${totalCount} items${section.fields ? ' and fields' : ''}`}
          {query && retainedFindings && ' Items with findings stay visible.'}
        </p>
      </div>
      {section.findings.length > 0 && (
        <div className={`section-findings-summary${errors ? ' has-errors' : ''}`} role={errors ? 'alert' : undefined}>
          <Icon name="warning" />
          <strong>{findingCounts}</strong>
          <button type="button" className="button button-quiet" onClick={() => {
            const heading = document.getElementById(findingsId)
            const main = document.getElementById('main')
            if (heading && main) {
              main.scrollTo({ top: main.scrollTop + heading.getBoundingClientRect().top - main.getBoundingClientRect().top - 16 })
              heading.focus({ preventScroll: true })
            }
          }}>Review {section.findings.length} finding{section.findings.length === 1 ? '' : 's'}</button>
        </div>
      )}
      {errors > 0 && otherFindings}
      {section.kinds.map((k) => {
        const items = visibleItems.filter((it) => it.kind === k.kind)
        if (k.kind === 'hook' && section.stages) return <Lifecycle key={k.kind} stages={section.stages} hooks={items} count={k.count} filtering={Boolean(query)} />
        if (k.kind === 'settings' && matchingFields) return <SettingsFields key={k.kind} title={k.label} fields={matchingFields} files={items} filtering={Boolean(query)} />
        const titleId = `${section.id}-${k.kind}`
        return (
          <section className="block" aria-labelledby={titleId} key={k.kind}>
            <h2 id={titleId}>
              {k.label} <span className="muted kind-count">{k.count}{k.kind === 'builtin' ? ' enabled' : ''}</span>
            </h2>
            {k.kind === 'builtin' && <BuiltinChangeControls labels={section.items} />}
            {items.length === 0 ? <p className="muted">{query ? 'No matches.' : 'None configured.'}</p> : <GroupedItems items={items} />}
          </section>
        )
      })}
      {errors === 0 && otherFindings}
    </>
  )
}

function searchValues(values: Value[]): string[] {
  return values.flatMap((value) => [value.key, value.value ?? '', ...(value.list ?? [])])
}

function SettingsFields({ title, fields, files, filtering }: { title: string; fields: SettingField[]; files: SectionItem[]; filtering: boolean }) {
  const id = `fields-${title.toLowerCase().replace(/\W+/g, '-')}`
  const configured = fields.filter((field) => field.set.length > 0)
  const unconfigured = fields.filter((field) => field.set.length === 0)
  return (
    <section className="block" aria-labelledby={id}>
      <h2 id={id}>{title}</h2>
      {configured.length ? <FieldList fields={configured} /> : <p className="muted">{filtering ? 'No matching configured settings.' : 'No settings configured.'}</p>}
      {unconfigured.length > 0 && (
        <details className="unconfigured-fields" open={filtering || undefined}>
          <summary>Unconfigured fields ({unconfigured.length})</summary>
          <FieldList fields={unconfigured} />
        </details>
      )}
      {files.length > 0 && (
        <>
          <h3 className="item-group-title">Files</h3>
          <ItemList items={files} hideValues />
        </>
      )}
    </section>
  )
}

function FieldList({ fields }: { fields: SettingField[] }) {
  return (
    <ul className="fields">
      {fields.map((f) => (
        <li key={f.key} className={`field-row${f.set.length ? ' is-set' : ''}`}>
          <div className="field-head">
            <strong>{f.label}</strong>
            {f.key !== 'x-' && <code className="muted">{f.key}</code>}
            <p className="muted">{f.description}</p>
          </div>
          {f.set.length === 0 ? (
            <p className="muted field-unset">Not set</p>
          ) : (
            f.set.map((src) => (
              <div key={src.spec} className="field-source">
                {f.key === 'x-' ? <NativeValues values={src.values} file={src.path} /> : <Values values={src.values} file={src.path} />}
                <p className="muted field-from">
                  from <Code>{src.path}</Code>
                </p>
              </div>
            ))
          )}
        </li>
      ))}
    </ul>
  )
}

function Lifecycle({ stages, hooks, count, filtering }: { stages: Stage[]; hooks: SectionItem[]; count: number; filtering: boolean }) {
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
                <div className="stage-label">
                  <h3 className="stage-title">{st.label}</h3>
                  {!here.length && <p className="muted stage-empty">{filtering ? 'No matching hooks.' : 'No hook runs here.'}</p>}
                </div>
                {here.length ? <p className="muted stage-description">{st.description}</p> : (
                  <details className="stage-description">
                    <summary>When this runs</summary>
                    <p className="muted">{st.description}</p>
                  </details>
                )}
                {st.targets.length > 0 && (
                  <p className="stage-tools">
                    {st.targets.map((t) => (
                      <ToolAvatar key={t.target} target={t.target} label={`${t.label} - ${t.event}`} />
                    ))}
                  </p>
                )}
              </div>
              {here.length > 0 && <ItemList items={here} />}
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
      {items.map((item) => {
        const control = item.enabled !== undefined && <EditSwitch on={item.enabled} label={`${item.name} built-in`} save={(on) => postEdit(API.editBuiltin, { name: item.name, enabled: on })} />
        const heading = (toggle: ReactNode) => <ItemHeading item={item} control={toggle} />
        return (
          <li key={`${item.kind}:${item.name}:${item.path}`} className={`item-row${item.findings.length ? ' has-findings' : ''}`}>
            {item.kind === 'builtin' && item.enabled !== undefined
              ? <BuiltinChange name={item.name} label={item.label} on={item.enabled} fallback={control} renderHeading={heading} />
              : heading(control)}
            {!hideValues && item.values.length > 0 && <Values values={item.values} file={item.kind === 'settings' ? item.path : undefined} />}
            {item.tools && <Tools listing={item.tools} />}
            {item.findings.length > 0 && <Findings findings={item.findings} />}
          </li>
        )
      })}
    </ul>
  )
}

function ItemHeading({ item, control }: { item: SectionItem; control: ReactNode }) {
  return (
    <div className="item-heading">
      <div className="item-main">
        <strong>{item.label}</strong>
        {item.description && <span className="muted item-description">{item.description}</span>}
        {item.enabled === undefined && <code className="item-path" title={item.path}>{item.path}</code>}
      </div>
      <div className="item-meta">
        {control}
        {item.enabled === undefined && <span className="item-layer" title="Layer the item comes from">{item.layer}</span>}
        {item.scope && <Chip>{item.scope}</Chip>}
        {item.openPath && (
          <a className="edit-link" href={cursorUrl(item.openPath)} aria-label={`Edit ${item.label} in Cursor`} title="Edit in Cursor">
            <Icon name="edit" />
          </a>
        )}
      </div>
    </div>
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

function NativeValues({ values, file }: { values: Value[]; file: string }) {
  const groups = new Map<string, Value[]>()
  for (const value of values) {
    const prefix = value.key.split('.')[0]
    const group = groups.get(prefix) ?? []
    group.push(value)
    groups.set(prefix, group)
  }
  return (
    <div className="native-groups">
      {[...groups].map(([prefix, entries]) => (
        <div key={prefix} className="native-group">
          <h4 className="native-group-title"><ToolAvatar target={prefix.slice(2)} /><code>{prefix}</code></h4>
          <Values values={entries} file={file} prefix={prefix} />
        </div>
      ))}
    </div>
  )
}

function Values({ values, file, prefix }: { values: Value[]; file?: string; prefix?: string }) {
  return (
    <dl className="values">
      {values.map((v) => (
        <div key={v.key} className="value-row">
          <dt>
            <code title={prefix ? v.key : undefined}>
              {(prefix && v.key.startsWith(`${prefix}.`) ? v.key.slice(prefix.length + 1) : v.key).split('.').map((part, i) => (
                <span key={i}>{i > 0 && <>.<wbr /></>}{part}</span>
              ))}
            </code>
          </dt>
          <dd>
            {v.type === 'list' ? (
              <span className="value-chips">
                {(v.list ?? []).length ? (v.list ?? []).map((entry, i) => v.key === 'targets'
                  ? <ToolAvatar key={`${i}-${entry}`} target={entry} />
                  : <Chip key={`${i}-${entry}`}>{entry}</Chip>) : <span className="muted">Empty</span>}
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

function Findings({ findings }: { findings: ItemFinding[] }) {
  return (
    <ul className="findings">
      {findings.map((f, i) => (
        <Finding key={`${f.code ?? f.source}-${f.line ?? 0}-${i}`} finding={f} />
      ))}
    </ul>
  )
}

function Finding({ finding: f }: { finding: ItemFinding }) {
  const root = useContext(ConfigurationRoot)
  const prompt = findingFixPrompt(f, root)
  const promptId = useId()
  return (
    <li className={`finding finding-${f.severity === 'error' ? 'error' : 'warn'}`}>
      <div className="finding-body">
        <div className="finding-head">
          <div className="finding-meta">
            <Icon name="warning" className="finding-icon" />
            <Badge tone={f.severity === 'error' ? 'blocked' : 'approval'}>{f.severity === 'error' ? 'Error' : 'Warning'}</Badge>
            {f.code && <Code>{f.code}</Code>}
            <span className="muted">{f.source}</span>
          </div>
          <CopyButton text={prompt} label="Copy fix prompt" copiedLabel="Copied" className="finding-copy" />
        </div>
        <FindingMessage message={f.message} />
        {f.path ? (
          <p className="finding-source muted">
            <Code>{f.path}{f.line ? `:${f.line}` : ''}</Code>
          </p>
        ) : f.line ? <p className="finding-source muted">Line {f.line}</p> : null}
        <details className="finding-details">
          <summary>Details</summary>
          <div className="finding-evidence muted">
            {f.fix && <p><strong>Suggested fix</strong><span>{f.fix}</span></p>}
            {f.explanation && <p><strong>Why this appears</strong><span>{f.explanation}</span></p>}
            <div className="finding-prompt">
              <label htmlFor={promptId}>Fix prompt</label>
              <p id={`${promptId}-hint`}>Paste this prompt into your coding agent.</p>
              <textarea id={promptId} aria-describedby={`${promptId}-hint`} value={prompt} readOnly rows={12} spellCheck={false} />
            </div>
          </div>
        </details>
      </div>
    </li>
  )
}

function FindingMessage({ message }: { message: string }) {
  const [expanded, setExpanded] = useState(false)
  const messageId = useId()
  const collapsible = message.length > 240
  return (
    <div className="finding-message-wrap">
      <p id={messageId} className={`finding-message${collapsible && !expanded ? ' is-collapsed' : ''}`}>{message}</p>
      {collapsible && (
        <button type="button" className="finding-expand" aria-expanded={expanded} aria-controls={messageId} onClick={() => setExpanded(!expanded)}>
          {expanded ? 'Show less' : 'Show full message'}
        </button>
      )}
    </div>
  )
}

function SupportTable({ section, toolLabel }: { section: Section; toolLabel: (target: string) => string }) {
  if (!section.support.length) return null
  return (
    <section className="block" aria-labelledby="support-title">
      <h2 id="support-title">Coding tool support</h2>
      <div className="table-wrap">
        <table className="table support-table" role="table" aria-labelledby="support-title">
          <thead role="rowgroup">
            <tr role="row">
              <th id="support-kind" scope="col" role="columnheader">Kind</th>
              <th id="support-supported" scope="col" role="columnheader">Supported on</th>
              <th id="support-unsupported" scope="col" role="columnheader">Not supported on</th>
            </tr>
          </thead>
          <tbody role="rowgroup">
            {section.support.map((k) => (
              <tr key={k.kind} role="row">
                <th id={`${section.id}-${k.kind}-support`} scope="row" role="rowheader">{k.label}</th>
                <td role="cell" headers={`${section.id}-${k.kind}-support support-supported`}>
                  <span className="support-status-label" aria-hidden="true">Supported on</span>
                  {k.supported.length ? (
                    <span className="tool-list">
                      {k.supported.map((t) => (
                        <ToolAvatar key={t} target={t} label={toolLabel(t)} />
                      ))}
                    </span>
                  ) : (
                    <span className="muted">No configured tool</span>
                  )}
                </td>
                <td role="cell" headers={`${section.id}-${k.kind}-support support-unsupported`}>
                  <span className="support-status-label" aria-hidden="true">Not supported on</span>
                  {k.unsupported.length ? (
                    <span className="tool-list">
                      {k.unsupported.map((t) => (
                        <ToolAvatar key={t} target={t} label={toolLabel(t)} />
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
