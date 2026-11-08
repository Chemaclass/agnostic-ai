import { useId, useRef } from 'react'
import type { Section } from '../contract'
import { AppLink } from './app-context'
import { kindEntryId } from '../model/identity'
import { entryTab, sectionTabs } from '../model/section-tabs'
import { ToolAvatar } from './ui'
import { supportSummary } from '../model/verdict'

/** The facts line's Tool support link: it opens the support table in a dialog. */
export function SupportLink({ section, toolLabel }: { section: Section; toolLabel: (target: string) => string }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const titleId = useId()
  // A press that starts inside the panel, such as a text selection, never closes it.
  const pressedBackdrop = useRef(false)
  if (!section.support.length) return null
  const tabs = sectionTabs(section)
  const close = () => dialog.current?.close()
  return (
    <>
      <button type="button" className="link-button page-facts-support" aria-haspopup="dialog" onClick={() => dialog.current?.showModal()}>Tool support</button>
      <dialog ref={dialog} className="support-dialog" aria-labelledby={titleId} onPointerDown={(event) => { pressedBackdrop.current = event.target === event.currentTarget }} onClick={(event) => { if (pressedBackdrop.current && event.target === event.currentTarget) close() }}>
      <div className="support-dialog-body">
      <div className="support-dialog-head">
        <div>
          <h2 id={titleId}>Coding tool support</h2>
          <p className="support-line">{supportSummary(section.support)}</p>
        </div>
        <button type="button" className="button button-quiet" onClick={close}>Close</button>
      </div>
      <div className="table-wrap">
        <table className="table support-table" role="table" aria-labelledby={titleId}>
          <thead role="rowgroup">
            <tr role="row">
              <th id="support-kind" scope="col" role="columnheader">Type</th>
              <th id="support-supported" scope="col" role="columnheader">Supported on</th>
              <th id="support-unsupported" scope="col" role="columnheader">Not supported on</th>
            </tr>
          </thead>
          <tbody role="rowgroup">
            {section.support.map((k) => (
              <tr key={k.kind} role="row">
                <th id={`${section.id}-${k.kind}-support`} scope="row" role="rowheader">{section.kinds.some((kind) => kind.kind === k.kind) ? <AppLink to={{ tab: entryTab(section, tabs, kindEntryId(k.kind)), entry: kindEntryId(k.kind) }} onNavigate={close}>{k.label}</AppLink> : k.label}</th>
                <td role="cell" headers={`${section.id}-${k.kind}-support support-supported`}>
                  <span className="support-status-label" aria-hidden="true">Supported on</span>
                  {k.supported.length ? (
                    <span className="tool-list">
                      {k.supported.map((t) => (
                        <span key={t} className="tool-tag"><ToolAvatar target={t} label={toolLabel(t)} /><span className="tool-tag-name" aria-hidden="true">{toolLabel(t)}</span></span>
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
                        <span key={t} className="tool-tag"><ToolAvatar target={t} label={toolLabel(t)} /><span className="tool-tag-name" aria-hidden="true">{toolLabel(t)}</span></span>
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
      </div>
      </dialog>
    </>
  )
}
