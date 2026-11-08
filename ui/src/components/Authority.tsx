import type { ReactNode } from 'react'
import type { SectionItem, SettingField } from '../contract'
import { DECISIONS, hasRules, modeSummary, protectedPaths, protectedSummary, ruleRows, type Decision, type RuleRow } from '../model/authority'
import { sourcesForAudience } from '../model/audience-view'
import { fieldEntryId } from '../model/identity'
import { fieldSearchText, matchesSearch } from '../model/search'
import { useApp } from './app-context'
import { useEditAudience, useGlobalLevel, useViewAudience } from './EditAudience'
import { FieldEditor, FieldResolution, SettingsFields } from './Fields'
import { ItemList } from './Items'
import { AddRule, PermissionEditor } from './PermissionEditor'
import { useEntry } from './section-context'
import { Code } from './ui'

const ALSO: Record<string, { label: string; text: string; value: (field: SettingField, audience: ReturnType<typeof useViewAudience>) => string }> = {
  protected: { label: 'Protected paths', text: 'Files agents must not edit without asking.', value: protectedSummary },
  'permissions.default-mode': { label: 'Default permission mode', text: 'The mode a session starts in. Claude Code only.', value: modeSummary },
}

/** The Authority page body: one rules table, then the two settings that sit beside it. */
export function AuthorityFields({ title, fields, files, query, headingEntry }: { title: string; fields: SettingField[]; files: SectionItem[]; query: string; headingEntry: string }) {
  const audience = useViewAudience()
  const lists = DECISIONS.flatMap((decision) => {
    const field = fields.find((candidate) => candidate.key === decision.key)
    return field ? [{ decision, field }] : []
  })
  const also = Object.keys(ALSO).flatMap((key) => fields.filter((field) => field.key === key))
  const others = fields.filter((field) => !lists.some((list) => list.field === field) && !also.includes(field))
  const fieldMatches = (field: SettingField) => matchesSearch(fieldSearchText(field), query)
  const shows = (field: SettingField, decision: Decision) => (row: RuleRow) => !query || matchesSearch([row.rule, row.path, decision.word, field.key, field.label, field.description], query)
  const any = hasRules(fields, audience)
  const filteredOut = any && lists.every(({ decision, field }) => ruleRows(field, audience).filter(shows(field, decision)).length === 0)
  const alsoShown = also.filter((field) => !query || fieldMatches(field))
  return <>
    <section className="block authority-rules-block" aria-labelledby="authority-rules-title">
      {any
        ? <h2 id="authority-rules-title" className="sr-only" tabIndex={-1} data-entry={headingEntry}>{title}</h2>
        : <EmptyRules entry={headingEntry} filtering={!!query} fields={fields} />}
      {filteredOut && <p className="muted">No matching rules.</p>}
      <div className="authority-rules">
        {any && !filteredOut && <div className="authority-rules-head" aria-hidden="true"><span>Rule</span><span>Decision</span><span>Scope</span></div>}
        {lists.map(({ decision, field }) => <PermissionEditor key={field.key} field={field} files={files} decision={decision} shows={shows(field, decision)} />)}
      </div>
    </section>
    {alsoShown.length > 0 && <section className="block authority-also" aria-labelledby="authority-also-title">
      <h2 id="authority-also-title" className="authority-also-title">Also on this page</h2>
      <ul className="authority-also-list">
        {alsoShown.map((field) => <AlsoRow key={field.key} field={field} files={files} />)}
      </ul>
    </section>}
    {others.length > 0 && <SettingsFields title="Other settings" fields={others} files={[]} filtering={!!query} />}
    {files.length > 0 && <section className="block" aria-labelledby="authority-files-title">
      <h2 id="authority-files-title" className="item-group-title">Files</h2>
      <ItemList items={files} hideValues />
    </section>}
  </>
}

function EmptyRules({ entry, filtering, fields }: { entry: string; filtering: boolean; fields: SettingField[] }) {
  const audience = useEditAudience()
  const global = useGlobalLevel()
  const where = audience === 'personal' ? 'Saved in your local files, ignored by Git' : global ? 'Saved in your global files' : 'Saved in the project, shared through Git'
  return <div className="authority-empty">
    <h2 id="authority-rules-title" className="empty-state-title" tabIndex={-1} data-entry={entry}>{filtering ? 'No matching rules.' : 'No rules yet.'}</h2>
    {!filtering && <p className="empty-state-text">A rule lets a command run without asking, asks before a risky one, or blocks what must never run. Sync writes the same rules to each tool that supports them.</p>}
    {!filtering && <AddRuleAction fields={fields}><span className="muted">{where}</span></AddRuleAction>}
  </div>
}

function AddRuleAction({ fields, children }: { fields: SettingField[]; children?: ReactNode }) {
  if (useApp().source.kind !== 'api') return null
  return <div className="empty-state-actions"><AddRule fields={fields} />{children}</div>
}

/** The toolbar's primary action, once the page has rules; the empty state offers it until then. */
export function AuthorityToolbarAction({ fields }: { fields: SettingField[] }) {
  const app = useApp()
  const audience = useViewAudience()
  if (app.source.kind !== 'api' || !hasRules(fields, audience)) return null
  return <div className="section-search-primary"><AddRule fields={fields} /></div>
}

function AlsoRow({ field, files }: { field: SettingField; files: SectionItem[] }) {
  const audience = useViewAudience()
  const { linked, ...entry } = useEntry(fieldEntryId(field))
  const copy = ALSO[field.key]
  const set = sourcesForAudience(field.set, audience).length > 0
  return <li {...entry} className={`authority-also-row${set ? ' is-set' : ''}${linked ? ' is-linked' : ''}`}>
    <strong className="authority-also-name">{copy.label}</strong>
    <span className="authority-also-text">{field.key === 'protected' ? <ProtectedText field={field} fallback={copy.text} /> : copy.text}</span>
    <span className="authority-also-value">{copy.value(field, audience)}</span>
    <div className="authority-also-edit">
      <FieldEditor field={field} files={files} sources={() => null} />
    </div>
    {field.resolved && field.resolved.length > 0 && <div className="authority-also-more"><FieldResolution field={field} files={files} resolved={field.resolved} /></div>}
  </li>
}

function ProtectedText({ field, fallback }: { field: SettingField; fallback: string }) {
  const { paths, reason } = protectedPaths(field, useViewAudience())
  if (!paths.length) return fallback
  return <>
    <span className="authority-also-paths">{paths.map((path) => <Code key={path}>{path}</Code>)}</span>
    {reason && <span className="authority-also-reason">{reason}</span>}
  </>
}
