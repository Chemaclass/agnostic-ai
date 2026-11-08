import { useState } from 'react'
import type { SectionItem, Value } from '../contract'
import { launcherCall } from '../model/hook-command'
import { SourceState, useSource, useSourceShown, WithheldNote } from './SourceContent'
import { CopyButton } from './ui'

type Field = { key: string; value: string }

// The stage already says when a hook runs.
const SHOWN_ELSEWHERE = new Set(['command', 'on', 'event'])
// The server's stand-in for a value it hides because it looks like a credential.
const REDACTED = '<redacted>'

/** A hook's command, its other key fields, its name, and, from the running server, every field its file sets. */
export function HookCommand({ item }: { item: SectionItem }) {
  const [full, setFull] = useState(false)
  const fileShown = useSourceShown(item)
  const loaded = useSource(item, full)
  const fields = (item.fields ?? []).map(fieldOf)
  const command = fields.find((field) => field.key === 'command')?.value
  const others = fields.filter((field) => !SHOWN_ELSEWHERE.has(field.key))
  // A personal file can override a few fields of a shared hook; the server reads that file alone, while key fields are merged.
  const fullLabel = item.layer === 'project-user' || item.layer === 'global-local' ? 'Fields this file sets' : 'Full file'
  return (
    <details className="hook-command">
      <summary>{command ? 'Command' : 'Details'}</summary>
      <div className="hook-command-body">
        {command && <CommandValue value={command} />}
        {others.length > 0 && <FieldList fields={others} label={`Key fields of ${item.label}`} />}
        <div className="hook-command-foot">
          <code className="muted hook-name">{item.name}</code>
          {command && command !== REDACTED && <CopyButton text={command} label="Copy command" ariaLabel={`Copy command of ${item.label}`} />}
          {fileShown && <button type="button" className="button button-quiet" aria-pressed={full} onClick={() => setFull(!full)}>{fullLabel}</button>}
        </div>
        {full && (
          <SourceState loaded={loaded}>
            {(view) => view.withheld
              ? <WithheldNote view={view} />
              : view.frontmatter.length
                ? <FieldList fields={view.frontmatter} label={`${fullLabel} of ${item.label}`} />
                : <p className="muted">The file does not read as YAML. Open it to see it.</p>}
          </SourceState>
        )}
      </div>
    </details>
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

/** A launcher shows the agnostic-ai call it makes; the whole script stays one click away. */
function CommandValue({ value }: { value: string }) {
  const call = launcherCall(value)
  return (
    <div className="hook-call">
      <code className="hook-field-value">{call ?? value}</code>
      {call && (
        <details className="hook-launcher">
          <summary>Full command</summary>
          <p className="muted">It finds the project root, uses the project's pinned agnostic-ai when there is one, does nothing when agnostic-ai is not installed, and stops with an error when that version is too old.</p>
          <code className="hook-field-value">{value}</code>
        </details>
      )}
    </div>
  )
}
