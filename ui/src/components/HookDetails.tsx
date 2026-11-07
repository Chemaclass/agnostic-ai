import { useState } from 'react'
import type { SectionItem, Value } from '../contract'
import { SourceState, useSource, useSourceShown, WithheldNote } from './SourceContent'

type Level = 'summary' | 'fields' | 'full'

type Field = { key: string; value: string }

/** Zoom on a hook row: the summary, its key fields from the overview, or every field the file sets, read from the running server. */
export function HookDetails({ item }: { item: SectionItem }) {
  const [level, setLevel] = useState<Level>('summary')
  const full = useSourceShown(item)
  const loaded = useSource(item, level === 'full')
  const fields = (item.fields ?? []).map(fieldOf)
  if (!fields.length && !full) return null
  // A personal file can override a few fields of a shared hook; the server reads that file alone, while Key fields are merged.
  const fullLabel = item.layer === 'project-user' || item.layer === 'global-local' ? 'Fields this file sets' : 'Full file'
  const levels: [Level, string][] = [['summary', 'Summary'], ['fields', 'Key fields'], ...(full ? [['full', fullLabel] as [Level, string]] : [])]
  return (
    <div className="hook-details">
      <div className="segmented hook-zoom" role="group" aria-label={`Details of ${item.label}`}>
        {levels.map(([id, label]) => (
          <button key={id} type="button" aria-pressed={level === id} onClick={() => setLevel(id)}>{label}</button>
        ))}
      </div>
      {level === 'fields' && (fields.length ? <FieldList fields={fields} label={`Key fields of ${item.label}`} /> : <p className="muted">The hook sets none of its key fields.</p>)}
      {level === 'full' && (
        <SourceState loaded={loaded}>
          {(view) => view.withheld
            ? <WithheldNote view={view} />
            : view.frontmatter.length
              ? <FieldList fields={view.frontmatter} label={`${fullLabel} of ${item.label}`} />
              : <p className="muted">The file does not read as YAML. Open it to see it.</p>}
        </SourceState>
      )}
    </div>
  )
}

function fieldOf(v: Value): Field {
  return { key: v.key, value: v.type === 'list' ? (v.list ?? []).join(', ') : v.value ?? '' }
}

function FieldList({ fields, label }: { fields: Field[]; label: string }) {
  return (
    <dl className="values hook-fields" aria-label={label}>
      {fields.map((f) => (
        <div key={f.key} className="value-row">
          <dt><code>{f.key}</code></dt>
          <dd><code className="hook-field-value">{f.value}</code></dd>
        </div>
      ))}
    </dl>
  )
}
