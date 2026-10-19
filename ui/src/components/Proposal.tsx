// The proposal panel. A draft is "Proposed · client preview, not
// validated" until the server validates it, then "Validated, not
// applied". Applying is not supported: the panel shows the exact file
// diff to copy instead.

import { useId, useState } from 'react'
import type { AuthorityView, Change, FileDiff, ImpactResult } from '../contract'
import type { ImpactState } from '../App'
import { directionDisplay } from '../model/effects'
import { diffLines, unifiedDiff } from '../model/diff'
import { describeChange, sameChange, type ProposalPreview } from '../model/proposal'
import { labelForId } from '../model/domains'
import { ErrorPanel } from './Banner'
import { Badge, Code, CopyButton, DeclaredTransition, EffectTransition } from './ui'

export function ProposalPanel({
  view,
  changes,
  preview,
  impact,
  validation,
  onChange,
  onValidate,
  onDiscard,
}: {
  view: AuthorityView
  changes: Change[]
  preview: ProposalPreview | undefined
  impact: ImpactState
  validation: { available: boolean; note?: string }
  onChange: (changes: Change[]) => void
  onValidate: () => void
  onDiscard: () => void
}) {
  const formId = useId()
  const [op, setOp] = useState<Change['op']>('add')
  const [list, setList] = useState<Change['list']>('ask')
  const [rule, setRule] = useState('')
  const apply = view.supportedActions.find((a) => /apply/i.test(a.id))
  const result = impact.status === 'done' ? impact.result?.data : undefined
  const validated = result?.valid === true
  const invalid = result !== undefined && !result.valid
  const hypothetical = view.harness.installed.state !== 'in-sync'
  const hypotheticalFor = (operationId: string) => {
    for (const r of view.resources) {
      const found = r.operations.find((o) => o.id === operationId)
      if (found) return found.hypothetical
    }
    return hypothetical
  }

  return (
    <section className="panel proposal" aria-labelledby="proposal-title" id="proposal">
      <div className="proposal-head">
        <h2 id="proposal-title" tabIndex={-1}>
          Proposal
        </h2>
        {validated ? (
          <Badge tone="info">Validated, not applied</Badge>
        ) : invalid ? (
          <Badge tone="blocked">Not valid</Badge>
        ) : (
          <Badge tone="approval">Proposed</Badge>
        )}
      </div>
      <p className="muted">
        For {view.agent.label} on {view.harness.label}. Nothing is written: validation computes the impact on a copy of the specs.
      </p>

      {changes.length ? (
        <ol className="change-list" aria-label="Proposed changes">
          {changes.map((c, i) => (
            <li key={`${c.op}:${c.list}:${c.rule}:${i}`}>
              <span>
                {c.op === 'add' ? 'Add' : 'Remove'} <Code>{c.rule}</Code> {c.op === 'add' ? 'to' : 'from'} the {c.list} list
              </span>
              <button type="button" className="link-button" onClick={() => onChange(changes.filter((_, j) => j !== i))}>
                Remove<span className="sr-only"> the change: {describeChange(c)}</span>
              </button>
            </li>
          ))}
        </ol>
      ) : (
        <p>Add a change below, or choose Revoke, Require approval, or Block on an operation.</p>
      )}

      <form
        className="rule-form"
        aria-label="Add a change"
        onSubmit={(event) => {
          event.preventDefault()
          const text = rule.trim()
          if (!text) return
          const change: Change = { op, list, rule: text }
          if (!changes.some((c) => sameChange(c, change))) onChange([...changes, change])
          setRule('')
        }}
      >
        <label className="field">
          <span className="field-label">Change</span>
          <select value={op} onChange={(e) => setOp(e.target.value as Change['op'])}>
            <option value="add">Add</option>
            <option value="remove">Remove</option>
          </select>
        </label>
        <label className="field">
          <span className="field-label">List</span>
          <select value={list} onChange={(e) => setList(e.target.value as Change['list'])}>
            <option value="allow">allow</option>
            <option value="ask">ask</option>
            <option value="deny">deny</option>
          </select>
        </label>
        <label className="field field-grow">
          <span className="field-label">Rule</span>
          <input id={`${formId}-rule`} value={rule} onChange={(e) => setRule(e.target.value)} placeholder="mcp:railway/deploy" spellCheck={false} autoComplete="off" />
        </label>
        <button type="submit" className="button" disabled={!rule.trim()}>
          Add change
        </button>
      </form>

      {preview && changes.length > 0 && !validated && (
        <section className="preview" aria-labelledby={`${formId}-preview`}>
          <h3 id={`${formId}-preview`}>
            <Badge tone="approval">Proposed · client preview, not validated</Badge>
          </h3>
          {preview.changes.length ? (
            <div className="table-wrap">
              <table className="table change-table">
                <thead>
                  <tr>
                    <th scope="col">Operation</th>
                    <th scope="col">Declared</th>
                    <th scope="col">Effective on {view.harness.label}</th>
                    <th scope="col">Direction</th>
                  </tr>
                </thead>
                <tbody>
                  {preview.changes.map((c) => (
                    <tr key={c.operationId}>
                      <th scope="row">
                        {c.operationLabel}
                        <span className="cell-note">{labelForId(view, c.resourceId) ?? c.resourceLabel}</span>
                      </th>
                      <td>
                        <DeclaredTransition before={c.declaredBefore} after={c.declaredAfter} />
                      </td>
                      <td>
                        <EffectTransition before={c.effectiveBefore} after={c.effectiveAfter} hypothetical={c.hypothetical} />
                      </td>
                      <td>
                        <DirectionCell direction={c.direction} reason={c.reason} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <p className="muted">The client sees no change for {view.agent.label} in this view; the server may still find one.</p>
          )}
          {preview.unmatched.length > 0 && (
            <p className="cell-note">
              Matches no operation of {view.agent.label} here:{' '}
              {preview.unmatched.map((c, i) => (
                <span key={i}>
                  {i > 0 && ', '}
                  <Code>{c.rule}</Code>
                </span>
              ))}
              .
            </p>
          )}
          <p className="cell-note">Only {view.agent.label}'s operations are previewed. The server checks every subject.</p>
        </section>
      )}

      <div className="proposal-actions">
        <button type="button" className="button button-primary" onClick={onValidate} disabled={!validation.available || changes.length === 0 || impact.status === 'validating'}>
          {impact.status === 'validating' ? 'Validating…' : 'Validate on server'}
        </button>
        {!validation.available && <span className="cell-note">Validation {validation.note}.</span>}
        <button type="button" className="button" disabled aria-describedby={`${formId}-apply`}>
          Apply
        </button>
        <span className="cell-note" id={`${formId}-apply`}>
          {apply?.description ?? 'Applying a proposal is not supported here.'}
        </span>
        <button type="button" className="button button-quiet" onClick={onDiscard}>
          Discard
        </button>
      </div>

      <div className="impact" aria-live="polite">
        {impact.status === 'validating' && <p className="muted">Validating on the server…</p>}
        {impact.status === 'error' && impact.error && <ErrorPanel error={impact.error} />}
        {result && <ImpactResultView view={view} result={result} hypotheticalFor={hypotheticalFor} />}
      </div>
    </section>
  )
}

function DirectionCell({ direction, reason }: { direction: string; reason?: string }) {
  const d = directionDisplay(direction)
  return (
    <span className={`direction direction-${direction}`}>
      <Badge tone={d.tone}>{d.text}</Badge>
      {direction === 'indeterminate' && reason && <span className="cell-note">{reason}</span>}
    </span>
  )
}

function ImpactResultView({ view, result, hypotheticalFor }: { view: AuthorityView; result: ImpactResult; hypotheticalFor: (operationId: string) => boolean }) {
  const subjectLabel = (id: string) => view.agents.find((a) => a.id === id)?.label ?? id
  if (!result.valid) {
    return (
      <div className="impact-result impact-invalid">
        <h3>Not valid</h3>
        {result.problems.length ? (
          <ul className="plain-list">
            {result.problems.map((p) => (
              <li key={p}>{p}</li>
            ))}
          </ul>
        ) : (
          <p>The server refused the proposal without naming a problem.</p>
        )}
      </div>
    )
  }
  const s = result.summary
  return (
    <div className="impact-result">
      <h3>
        <Badge tone="info">Validated, not applied</Badge>
      </h3>
      <p>
        {s.changed === 0
          ? 'No operation changes for any subject.'
          : `${s.changed} operation${s.changed === 1 ? '' : 's'} change: ${s.narrowed} narrowed, ${s.widened} widened, ${s.moved} moved, ${s.indeterminate ?? 0} indeterminate.`}
      </p>
      {result.affectedSubjects.length > 0 && (
        <p>
          Affected subjects:{' '}
          {result.affectedSubjects.map((id, i) => (
            <span key={id}>
              {i > 0 && ', '}
              <strong>{subjectLabel(id)}</strong>
            </span>
          ))}
        </p>
      )}
      {result.changes.length > 0 && (
        <div className="table-wrap">
          <table className="table change-table">
            <thead>
              <tr>
                <th scope="col">Subject</th>
                <th scope="col">Operation</th>
                <th scope="col">Declared</th>
                <th scope="col">Effective on {view.harness.label}</th>
                <th scope="col">Direction</th>
              </tr>
            </thead>
            <tbody>
              {result.changes.map((c) => (
                <tr key={`${c.subjectId}|${c.operationId}`}>
                  <td>{subjectLabel(c.subjectId)}</td>
                  <th scope="row">
                    {c.operationLabel}
                    <span className="cell-note">{labelForId(view, c.resourceId) ?? c.resourceLabel}</span>
                  </th>
                  <td>
                    <DeclaredTransition before={c.declaredBefore} after={c.declaredAfter} />
                  </td>
                  <td>
                    <EffectTransition before={c.effectiveBefore} after={c.effectiveAfter} hypothetical={hypotheticalFor(c.operationId)} />
                  </td>
                  <td>
                    <DirectionCell direction={c.direction} reason={c.reason} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {result.diff.length > 0 && (
        <div className="diffs">
          <h4>Copy this change into the spec</h4>
          {result.diff.map((d) => (
            <DiffView key={d.path} diff={d} />
          ))}
        </div>
      )}
      <p className="cell-note">
        {view.harness.installed.state !== 'in-sync'
          ? `${view.harness.label} has no generated files in sync yet, so a copied change takes effect only after agnostic-ai sync.`
          : 'After copying the change, run agnostic-ai sync to write it for each coding tool.'}
      </p>
    </div>
  )
}

export function DiffView({ diff }: { diff: FileDiff }) {
  const lines = diffLines(diff.before, diff.after)
  const patch = unifiedDiff(diff.path, diff.before, diff.after)
  return (
    <figure className="diff">
      <figcaption className="diff-head">
        <Code>{diff.path}</Code>
        <span className="diff-actions">
          <CopyButton text={patch} label="Copy diff" copiedLabel="Diff copied" />
          <CopyButton text={diff.after} label="Copy new file" copiedLabel="File copied" />
        </span>
      </figcaption>
      <pre className="diff-body" tabIndex={0} aria-label={`Changes to ${diff.path}`}>
        {lines.map((line, i) => (
          <span key={i} className={`diff-line diff-${line.kind}`}>
            <span className="diff-sign" aria-hidden="true">
              {line.kind === 'add' ? '+' : line.kind === 'del' ? '−' : ' '}
            </span>
            <span className="sr-only">{line.kind === 'add' ? 'Added: ' : line.kind === 'del' ? 'Removed: ' : ''}</span>
            {line.text || ' '}
            {'\n'}
          </span>
        ))}
      </pre>
    </figure>
  )
}
