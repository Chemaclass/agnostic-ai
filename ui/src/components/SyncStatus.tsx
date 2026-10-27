// Whether each configured tool's generated files match the source, with
// an explicit refresh. It reads the overview the page already loaded.

import type { ReactNode } from 'react'
import type { OverviewView, SyncState } from '../contract'
import { overviewFindings } from '../model/recheck'
import { syncSummary } from '../model/sync-status'
import { plural } from '../model/format'
import type { Remote } from './Shell'
import { Badge, Code, ToolAvatar, type Tone } from './ui'

const STATES: Record<SyncState, { label: string; tone: Tone }> = {
  'in-sync': { label: 'Matches source', tone: 'permitted' },
  drifted: { label: 'Needs sync', tone: 'approval' },
  'not-checked': { label: 'Not checked', tone: 'neutral' },
}

function checkedAt(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  return date.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'medium' })
}

export function SyncStatus({ overview, snapshot, saved, onRefresh }: { overview: Remote<OverviewView>; snapshot: boolean; saved: boolean; onRefresh: () => void }) {
  const data = overview.loaded?.data
  if (!data) return null
  const refreshing = overview.status === 'loading'
  const failed = overview.error !== undefined
  const summary = syncSummary(data.targets)
  const at = data.snapshot.generatedAt
  const time = at ? <time dateTime={at}>{checkedAt(at)}</time> : undefined
  const checking = refreshing ? <> Checking again…</> : undefined
  let line: ReactNode
  const recheckNote = snapshot && overviewFindings(data).length > 0 ? ' Rechecking a finding needs a running agnostic-ai serve.' : ''
  if (snapshot) line = <>{time ? <>Snapshot from {time}.</> : 'Snapshot time unavailable.'} Not a live check of your files.{recheckNote}{failed && !refreshing && <> The latest reload failed.</>}{checking}</>
  else if (failed) line = <>{time ? <>Last successful check {time}.</> : 'Check time unavailable.'}{refreshing ? checking : <> The latest refresh failed, so these results may be out of date.</>}</>
  else if (refreshing) line = <>Checking again…</>
  else line = time ? <>Checked {time}.</> : 'Check time unavailable.'
  return (
    <section className="sync-status" aria-labelledby="sync-status-title" data-tone={failed || !time ? 'unknown' : summary.tone} aria-busy={refreshing || undefined}>
      <div className="sync-status-row">
        <div className="sync-status-text">
          <h2 id="sync-status-title" className="sync-status-title">{snapshot ? 'Generated files at snapshot time' : 'Generated files'}</h2>
          {saved && !snapshot && <p className="sync-status-saved">Saved to the source configuration. Generated files change only when you run sync.</p>}
          <p className="sync-status-headline">{summary.headline}{summary.detail && <> {summary.detail}</>}</p>
          <p className="muted sync-status-time" role="status">{line}</p>
        </div>
        <button type="button" className="button" disabled={refreshing} onClick={onRefresh}>
          {snapshot ? 'Reload snapshot' : refreshing ? 'Refreshing…' : 'Refresh'}
        </button>
      </div>
      {data.targets.length > 0 && (
        <details className="sync-status-tools">
          <summary>{plural(data.targets.length, 'configured tool')}</summary>
          <ul>
            {data.targets.map((target) => {
              const state = STATES[target.sync.state]
              return (
                <li key={target.target} className="sync-status-tool">
                  <span className="sync-status-tool-name"><span aria-hidden="true"><ToolAvatar target={target.target} label={target.label} /></span>{target.label || target.target}</span>
                  <Badge tone={state.tone}>{state.label}</Badge>
                  {target.sync.files.length > 0 && (
                    <details>
                      <summary>{plural(target.sync.files.length, 'file')}</summary>
                      <ul className="sync-status-files">
                        {target.sync.files.map((file) => <li key={file.path}><Code>{file.path}</Code> <span className="muted">{file.state}</span></li>)}
                      </ul>
                    </details>
                  )}
                </li>
              )
            })}
          </ul>
          <p className="muted">Matches source means the generated files on disk equal what sync would write. It does not show what a running coding tool has loaded.</p>
        </details>
      )}
    </section>
  )
}
