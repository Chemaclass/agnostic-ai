import { useContext, type ReactNode } from 'react'
import { API, type SectionItem, type ToolListing, type Value } from '../contract'
import { postEdit } from '../api'
import { BuiltinChange } from './BuiltinChange'
import { ViewPersonalButton, useEditAudience, useLevelName, usePageAudience, useViewAudience } from './EditAudience'
import { OpenInEditor } from './EditorLink'
import { useApp } from './app-context'
import { SpecMoveAction } from './SpecMoveAction'
import { SourceContent } from './SourceContent'
import { HookCommand } from './HookDetails'
import { instructionsRefinePrompt } from '../model/refine-prompt'
import { builtinEnabled, comparisonValue } from '../model/audience-view'
import { audienceLabel, layerLabel } from '../model/edit-audience'
import { itemEntryId } from '../model/identity'
import { isTurnedOff } from '../model/counts'
import { Badge, Chip, Code, CopyButton, RowActions, Switch, Toggle, ToolAvatar } from './ui'
import { ConfigurationRoot, PersonalBuiltins, useEntry } from './section-context'
import { Findings } from './Findings'
import { EditSwitch, ResetAction } from './EditSwitch'

// When no server has a tool list yet, one notice under the heading replaces
// the same line on every server.
export function sharedUnlistedCommand(items: SectionItem[]): string | undefined {
  const listings = items.flatMap((item) => (item.tools ? [item.tools] : []))
  const command = listings[0]?.command
  return listings.length > 0 && listings.every((listing) => listing.state === 'not-listed' && listing.command === command) ? command : undefined
}

export function ConfigurationFiles({ items }: { items: SectionItem[] }) {
  return (
    <>
      <ItemList items={items} hideValues />
      <div className="effective-config">
        <h3>Effective configuration</h3>
        <p className="muted">Combined configuration used for sync, including local overrides.</p>
        <Values values={items[0].values} />
      </div>
    </>
  )
}

export function GroupedItems({ items, sharedUnlisted }: { items: SectionItem[]; sharedUnlisted?: boolean }) {
  if (!items.some((it) => it.group)) return <ItemList items={items} sharedUnlisted={sharedUnlisted} />
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
          <ItemList items={g.items} sharedUnlisted={sharedUnlisted} />
        </div>
      ))}
    </div>
  )
}

export function ItemList({ items, hideValues, sharedUnlisted }: { items: SectionItem[]; hideValues?: boolean; sharedUnlisted?: boolean }) {
  const audience = useEditAudience()
  const viewAudience = useViewAudience()
  const personalBuiltins = useContext(PersonalBuiltins)
  const level = useLevelName()
  const sharedBlocked = viewAudience === 'shared' && personalBuiltins
  return (
    <ul className="item-list">
      {items.map((item) => {
        const on = builtinEnabled(item, viewAudience)
        const control = item.enabled !== undefined && <>
          {sharedBlocked
            ? <Toggle on={on} label={`${item.name} built-in`} disabled onToggle={() => {}} />
            : <EditSwitch key={audience} on={on} label={`${item.name} built-in`} save={(enabled) => postEdit(API.editBuiltin, { audience, name: item.name, enabled, expected: on })} />}
          {viewAudience === 'personal' && item.local && <ResetAction label={`Reset ${item.name} to the ${level} value`} reset={() => postEdit(API.editBuiltin, { audience: 'personal', name: item.name, enabled: !on, expected: on })} />}
        </>
        const heading = (toggle: ReactNode) => <ItemHeading item={item} control={toggle} />
        return (
          <ItemRow key={itemEntryId(item)} item={item}>
            {item.kind === 'builtin' && item.enabled !== undefined && !sharedBlocked
              ? <BuiltinChange name={item.name} label={item.label} on={on} local={viewAudience === 'personal' && (item.local ?? false)} fallback={control} renderHeading={heading} />
              : heading(control)}
            <SpecMoveAction item={item} />
            {!hideValues && item.values.length > 0 && <Values values={item.values} file={item.kind === 'settings' ? item.path : undefined} personal={item.layer === 'project-user' || item.layer === 'global-local'} localKeys={item.localKeys} />}
            {item.tools && !(sharedUnlisted && item.tools.state === 'not-listed') && <Tools listing={item.tools} />}
            {item.kind === 'hook' ? <HookCommand item={item} /> : <SourceContent item={item} />}
            <Findings findings={item.findings} owner={itemEntryId(item)} place={itemEntryId(item)} />
          </ItemRow>
        )
      })}
    </ul>
  )
}

function ItemRow({ item, children }: { item: SectionItem; children: ReactNode }) {
  const { linked, ...entry } = useEntry(itemEntryId(item))
  return <li {...entry} className={`item-row row-hover${isTurnedOff(item) ? ' is-off' : ''}${item.findings.length ? ' has-findings' : ''}${linked ? ' is-linked' : ''}`}>{children}</li>
}

function ItemHeading({ item, control }: { item: SectionItem; control: ReactNode }) {
  const root = useContext(ConfigurationRoot)
  const audience = useViewAudience()
  const app = useApp()
  const refinable = item.kind === 'instructions' && item.layer === 'global'
  const openable = app.source.kind === 'api' && item.openPath !== undefined
  return (
    <div className="item-heading">
      <div className="item-main">
        <strong>{item.label}</strong>
        {item.kind === 'builtin' && item.name !== item.label && <code className="muted item-key">{item.name}</code>}
        {item.description && <span className="muted item-description">{item.description}</span>}
        {item.kind === 'builtin' && item.local && audience && <OverrideComparison shared={builtinEnabled(item, 'shared') ? 'On' : 'Off'} personal={item.enabled ? 'On' : 'Off'} />}
        {item.enabled === undefined && <code className="item-path" title={item.path}>{item.path}</code>}
      </div>
      <div className="item-meta">
        {item.local && audience !== 'shared' && <Badge tone="accent" title="Set in a local file Git ignores">local</Badge>}
        {isTurnedOff(item) && <Chip>Turned off</Chip>}
        {control}
        {item.enabled === undefined && <span className="item-layer" title="Layer the item comes from">{layerLabel(item.layer)}</span>}
        {item.scope && <Chip>{item.scope}</Chip>}
        {(refinable || openable) && (
          <RowActions label={item.label}>
            {refinable && <CopyButton text={instructionsRefinePrompt(item.openPath ?? (root ? `${root}/${item.path}` : item.path))} label="Copy refine prompt" ariaLabel={`Copy refine prompt for ${item.label}`} className="refine-copy" />}
            <OpenInEditor path={item.openPath} label={item.label} compact />
          </RowActions>
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
          The file changed after these were listed ({when}). Run <Code>{listing.command}</Code> to list them again.
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

export function OverrideComparison({ shared, personal }: { shared?: string; personal?: string }) {
  const level = useLevelName()
  if (shared === personal) return null
  const label = (value?: string) => value === 'null' ? 'Cleared' : value ?? 'Not set'
  return <span className="override-comparison" role="note" aria-label={`Local overrides ${level}`}>
    <span>{audienceLabel('shared', level === 'global')}: <strong>{label(shared)}</strong></span>
    <span aria-hidden="true">·</span>
    <span>Local: <strong>{label(personal)}</strong></span>
  </span>
}

function NativeComparison({ value, overrides, baseline }: { value: Value; overrides?: Value[]; baseline?: Value[] }) {
  const key = value.key
  const own = comparisonValue([value], key)
  if (baseline) return <OverrideComparison shared={comparisonValue(baseline, key)} personal={own} />
  if (overrides?.some((override) => override.key === key || key.startsWith(`${override.key}.`))) return <OverrideComparison shared={own} personal={comparisonValue(overrides, key)} />
  return null
}

export function NativeValues({ values, file, localKeys, overrides, baseline }: { values: Value[]; file: string; localKeys?: string[]; overrides?: Value[]; baseline?: Value[] }) {
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
          <Values values={entries} file={file} prefix={prefix} localKeys={localKeys} overrides={overrides} baseline={baseline} />
        </div>
      ))}
    </div>
  )
}

export function Values({ values, file, prefix, personal, localKeys, fieldKey, overrides, baseline }: { values: Value[]; file?: string; prefix?: string; personal?: boolean; localKeys?: string[]; fieldKey?: string; overrides?: Value[]; baseline?: Value[] }) {
  const audience = usePageAudience() ?? (personal ? 'personal' : 'shared')
  const level = useLevelName()
  return (
    <dl className="values">
      {values.map((v) => (
        <div key={v.key} className={`value-row${v.key === fieldKey ? ' repeats-field-key' : ''}`}>
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
                <>
                  {overrides?.some((override) => override.key === v.key || v.key.startsWith(`${override.key}.`))
                    ? <span className="value-override"><Toggle on={v.value === 'true'} label={v.key} disabled onToggle={() => {}} /><span className="muted">Overridden in Local</span><ViewPersonalButton /></span>
                    : <EditSwitch key={audience} on={v.value === 'true'} label={v.key} save={(on) => postEdit(API.editValue, { audience, path: file, key: v.key, value: on, expected: v.value === 'true' })} />}
                  {audience === 'personal' && localKeys?.includes(v.key) && <ResetAction label={`Reset ${v.key} to the ${level} value`} reset={() => postEdit(API.editValue, { audience: 'personal', path: file, key: v.key, value: null, expected: v.value === 'true' })} />}
                </>
              ) : (
                <Switch on={v.value === 'true'} />
              )
            ) : (
              <span className="value-text">{v.value}</span>
            )}
            <NativeComparison value={v} overrides={overrides} baseline={baseline} />
          </dd>
        </div>
      ))}
    </dl>
  )
}
