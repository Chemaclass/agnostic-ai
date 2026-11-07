import { useState, useSyncExternalStore } from 'react'
import type { Section } from '../contract'
import { AppLink } from './app-context'
import { DocLink } from './SectionHeading'
import { kindEntryId } from '../model/identity'
import { entryTab, sectionTabs } from '../model/section-tabs'
import { ToolAvatar } from './ui'

const narrowScreen = '(max-width: 760px)'

function supportOpenKey(section: string) {
  return `agnostic-ai.support-open:${section}`
}

function readSupportChoice(section: string): boolean | undefined {
  try {
    const value = window.localStorage.getItem(supportOpenKey(section))
    return value === null ? undefined : value === '1'
  } catch {
    return undefined
  }
}

function useNarrowScreen() {
  return useSyncExternalStore((change) => {
    const query = window.matchMedia?.(narrowScreen)
    query?.addEventListener('change', change)
    return () => query?.removeEventListener('change', change)
  }, () => window.matchMedia?.(narrowScreen).matches ?? false)
}

export function SupportTable({ section, toolLabel }: { section: Section; toolLabel: (target: string) => string }) {
  const [choice, setChoice] = useState(() => readSupportChoice(section.id))
  // On a phone the stacked table would push the tabs below the fold.
  const narrow = useNarrowScreen()
  const open = choice ?? !narrow
  if (!section.support.length) return null
  const tabs = sectionTabs(section)
  const toggle = (next: boolean) => {
    if (next === open) return
    setChoice(next)
    try {
      window.localStorage.setItem(supportOpenKey(section.id), next ? '1' : '0')
    } catch { /* The choice lasts until this page reloads. */ }
  }
  return (
    <details className="block support-block" open={open} onToggle={(event) => toggle(event.currentTarget.open)}>
      <summary className="support-summary"><h2 id="support-title">Coding tool support</h2></summary>
      <DocLink topic="support" />
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
                <th id={`${section.id}-${k.kind}-support`} scope="row" role="rowheader">{section.kinds.some((kind) => kind.kind === k.kind) ? <AppLink to={{ tab: entryTab(section, tabs, kindEntryId(k.kind)), entry: kindEntryId(k.kind) }}>{k.label}</AppLink> : k.label}</th>
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
    </details>
  )
}
