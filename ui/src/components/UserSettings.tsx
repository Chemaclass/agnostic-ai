import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react'
import type { CompanionAppSettings, SettingKey, SettingsSyncPlan, SettingsView } from '../contract'
import { applySync, planSync, readSettings, saveSetting, SettingsError, unlockProblem } from '../settings-api'
import { APPROVALS_SETUP_HINT, approvalsStatus, canChange, COMPANION_APPROVALS_HINT, COMPANION_APPROVALS_LABEL, companionApprovalsNeedSetup, COMPANION_SECTIONS, COMPANION_SWITCHES, companionMenuItem, companionShortcutHelp, LOCKED_HINT, MODE_CHOICES, modeConfirm, syncConfirm, syncFailure, syncResult } from '../model/user-settings'
import { useApp } from './app-context'
import { ConfigSwitch } from './ConfigSwitch'
import { useServedConfig } from './Shell'
import { SectionHeading } from './SectionHeading'
import { APPLE_PLATFORM, Toggle, toolName } from './ui'

/** How often an open tab reads the settings again, so a change made in the companion shows. */
const READ_EVERY_MS = 2000

interface Question {
  title: string
  body: string
  files?: string[]
  commands?: string[]
  notes?: string[]
  go?: string
  run?: () => void | Promise<void>
}

/** User-level settings, with loosening changes available only to an unlocked tab. */
export function UserSettings() {
  const app = useApp()
  if (app.source.kind !== 'api') return null
  return app.overview?.global === true ? <UserSettingsPage /> : <ProjectPointer />
}

/** A project view names where the user settings are, with the switch to that view. */
function ProjectPointer() {
  const { served } = useServedConfig(useApp().source.kind, 0)
  return (
    <section className="block user-settings" aria-labelledby="settings-user">
      <SectionHeading id="settings-user" focusable>Approvals and companion</SectionHeading>
      <p className="muted">User settings are in the user-level view.</p>
      {served && !served.userLevel && <ConfigSwitch served={served} />}
    </section>
  )
}

function UserSettingsPage() {
  const id = useId()
  const [doc, setDoc] = useState<SettingsView | undefined>()
  const [unread, setUnread] = useState<string | undefined>()
  const [busy, setBusy] = useState(false)
  const [said, setSaid] = useState<string | undefined>()
  const [question, setQuestion] = useState<Question | undefined>()
  const tag = useRef<string | undefined>(undefined)
  // A read that started before a save or a newer read finished is dropped.
  const taken = useRef(0)

  const take = useCallback((next: SettingsView, nextTag?: string) => {
    taken.current += 1
    tag.current = nextTag
    setDoc(next)
    setUnread(undefined)
  }, [])

  const read = useCallback(async () => {
    const started = taken.current
    try {
      const answer = await readSettings(tag.current)
      if (answer && started === taken.current) take(answer.settings, answer.tag)
    } catch (err) {
      if (started !== taken.current || (err as Error).name === 'AbortError') return
      tag.current = undefined
      setUnread(err instanceof Error ? err.message : String(err))
    }
  }, [take])

  useEffect(() => {
    void read()
    let timer: number | undefined
    const start = () => {
      window.clearInterval(timer)
      timer = document.visibilityState === 'visible' ? window.setInterval(() => void read(), READ_EVERY_MS) : undefined
    }
    const visibility = () => {
      start()
      if (document.visibilityState === 'visible') void read()
    }
    const focus = () => void read()
    start()
    document.addEventListener('visibilitychange', visibility)
    window.addEventListener('focus', focus)
    return () => {
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', visibility)
      window.removeEventListener('focus', focus)
    }
  }, [read])

  const save = async (key: SettingKey, value: string | boolean, expected: string | boolean | undefined) => {
    if (busy) return
    setBusy(true)
    setSaid(undefined)
    try {
      take((await saveSetting(key, value, expected)).settings)
    } catch (err) {
      const failure = err instanceof SettingsError ? err : new SettingsError(String(err))
      setSaid(failure.message)
      if (failure.settings) take(failure.settings)
    } finally {
      setBusy(false)
    }
  }

  const chooseMode = (mode: string) => {
    if (!doc || mode === doc.approvals.mode || busy) return
    const current = doc.approvals.mode
    const confirm = modeConfirm(mode)
    if (!confirm) {
      void save('approvals.mode', mode, current)
      return
    }
    setQuestion({ ...confirm, run: () => save('approvals.mode', mode, current) })
  }

  const run = async (plan: SettingsSyncPlan) => {
    setBusy(true)
    setSaid('Applying to your tools…')
    try {
      const reply = await applySync(plan.id)
      take(reply.settings)
      setSaid(syncResult(reply))
    } catch (err) {
      const failure = err instanceof SettingsError ? err : new SettingsError(String(err))
      if (failure.plan) {
        const moved = failure.plan
        setSaid(failure.message)
        setQuestion({ ...syncConfirm(moved), run: () => run(moved) })
      } else {
        setSaid(syncFailure(failure.message, failure.command))
      }
    } finally {
      setBusy(false)
    }
  }

  const sync = async () => {
    if (busy) return
    setBusy(true)
    setSaid('Checking which settings files will change…')
    try {
      const plan = await planSync()
      setSaid(undefined)
      setQuestion({ ...syncConfirm(plan), run: () => run(plan) })
    } catch (err) {
      const failure = err instanceof SettingsError ? err : new SettingsError(String(err))
      setSaid(syncFailure(failure.message, failure.command))
    } finally {
      setBusy(false)
    }
  }

  if (!doc) {
    return (
      <section className="block user-settings" aria-labelledby="settings-user">
        <SectionHeading id="settings-user" focusable>Approvals and companion</SectionHeading>
        <p className="muted" role="status">{unread ? `The settings could not be read: ${unread}` : 'Reading the settings…'}</p>
      </section>
    )
  }

  const locked = !doc.canLoosen
  const status = approvalsStatus(doc, toolName)
  const companionOn = doc.approvals.companion
  const setupRequired = companionApprovalsNeedSetup(doc)
  const content: Record<string, ReactNode> = {
    appearance: doc.companionApp && <CompanionAppearance app={doc.companionApp} canLoosen={doc.canLoosen} busy={busy} asking={question !== undefined} save={save} />,
    permissions: <>
      <fieldset data-companion-item="approvalsMode" className="user-settings-group" disabled={busy || question !== undefined}>
        <legend>{companionMenuItem('approvalsMode').label}</legend>
        {MODE_CHOICES.map((choice) => {
          const chosen = doc.approvals.mode === choice.value
          const locked = !chosen && !canChange(doc, 'approvals.mode', choice.value)
          return (
            <label key={choice.value} className={`user-settings-choice${locked ? ' is-locked' : ''}`}>
              <input type="radio" name={`${id}-mode`} value={choice.value} checked={chosen} disabled={locked} onChange={() => chooseMode(choice.value)} />
              <span>
                <span className="user-settings-choice-label">{choice.label}</span>
                <span className="muted">{choice.hint}</span>
              </span>
            </label>
          )
        })}
      </fieldset>

      <div className="user-settings-row" data-companion-item="approvalsCompanion">
        <div className="user-settings-row-label">
          <span>{COMPANION_APPROVALS_LABEL}</span>
          {setupRequired && <strong id={`${id}-companion-setup`}>Setup required</strong>}
          <span className="muted" id={`${id}-companion-hint`}>{COMPANION_APPROVALS_HINT}</span>
        </div>
        <Toggle on={companionOn} label={COMPANION_APPROVALS_LABEL} describedBy={setupRequired ? `${id}-companion-setup ${id}-companion-hint` : `${id}-companion-hint`} pending={busy} disabled={question !== undefined || !canChange(doc, 'approvals.companion', !companionOn)} onToggle={() => void save('approvals.companion', !companionOn, companionOn)} />
      </div>

      {status.length > 0 && (
        <div className="user-settings-status">
          {status.map((line) => <p key={line}>{line}</p>)}
        </div>
      )}
      {doc.sync.needed && !question && (doc.canLoosen
        ? <div className="user-settings-row-label">
            <p className="muted">{APPROVALS_SETUP_HINT}</p>
            <button type="button" className="button button-primary" disabled={busy} onClick={() => void sync()}>Finish setup…</button>
          </div>
        : <details>
            <summary>Finish setup in a terminal</summary>
            <p className="muted">Run <code>agnostic-ai sync --scope user</code>.</p>
          </details>)}

      {question && (
        <div className="user-settings-confirm" role="group" aria-labelledby={`${id}-question`}>
          <p id={`${id}-question`} className="user-settings-confirm-title">{question.title}</p>
          <p>{question.body}</p>
          {question.files && question.files.length > 0 && <ul className="user-settings-files">{question.files.map((file) => <li key={file}><code>{file}</code></li>)}</ul>}
          {question.commands && question.commands.length > 0 && (
            <>
              <p>Your tools will then run these commands:</p>
              <ul className="user-settings-files">{question.commands.map((command) => <li key={command}><code>{command}</code></li>)}</ul>
            </>
          )}
          {question.notes?.map((note) => <p key={note} className="muted">{note}</p>)}
          <div className="user-settings-actions">
            <button type="button" className="button button-quiet" autoFocus onClick={() => setQuestion(undefined)}>Cancel</button>
            {question.go && <button type="button" className="button button-primary" onClick={() => { const go = question.run; setQuestion(undefined); void go?.() }}>{question.go}</button>}
          </div>
        </div>
      )}
      <p className="muted" role="status">{said ?? ''}</p>

      {doc.approvals.tools.length > 0 && (
        <div className="table-wrap">
          <table className="table" aria-labelledby={`${id}-tools`}>
            <caption id={`${id}-tools`} className="sr-only">Approvals per tool</caption>
            <thead>
              <tr>
                <th scope="col">Tool</th>
                <th scope="col">Asks for</th>
                <th scope="col">Does</th>
                <th scope="col">Why</th>
              </tr>
            </thead>
            <tbody>
              {doc.approvals.tools.map((tool) => (
                <tr key={tool.tool}>
                  <td>{toolName(tool.tool)}</td>
                  <td>{tool.mode}</td>
                  <td>{tool.effective}</td>
                  <td>{tool.reason ?? tool.notes?.join(' ') ?? ''}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {doc.mailbox && (
          <div className="user-settings-row" data-companion-item="approveAgentSends">
            <span>{companionMenuItem('approveAgentSends').label}</span>
            <Toggle on={doc.mailbox.approveAgentSends} label={companionMenuItem('approveAgentSends').label} pending={busy} disabled={question !== undefined || !canChange(doc, 'mailbox.approveAgentSends', !doc.mailbox.approveAgentSends)} onToggle={() => void save('mailbox.approveAgentSends', !doc.mailbox!.approveAgentSends, doc.mailbox!.approveAgentSends)} />
          </div>
      )}
    </>,
    shortcuts: <CompanionShortcuts app={doc.companionApp} />,
    app: doc.companionApp && <CompanionApp app={doc.companionApp} />,
  }
  return (
    <section className="block user-settings" aria-labelledby="settings-user">
      <SectionHeading id="settings-user" focusable>Approvals and companion</SectionHeading>
      <p className="muted">Saved in your home config, the same settings the companion's menu shows. A change in either place shows in the other within seconds.</p>
      <p className="muted">Global bindings and {companionMenuItem('startAtLogin').label} show while the companion runs. Change them in its menu.</p>
      {unread && <p className="edit-error" role="alert">The settings could not be read again: {unread}</p>}
      {locked && <p className="user-settings-locked">{unlockProblem() ?? LOCKED_HINT} Changes that make approvals stricter work here.</p>}
      {COMPANION_SECTIONS.map((section) => {
        const body = content[section.id]
        return body && <section className="user-settings-section" key={section.id} aria-labelledby={`${id}-${section.id}`} data-companion-section={section.id}>
          <h3 id={`${id}-${section.id}`} className="user-settings-subheading">{section.label}</h3>
          {body}
        </section>
      })}
    </section>
  )
}

function CompanionAppearance({ app, canLoosen, busy, asking, save }: {
  app: CompanionAppSettings
  canLoosen: boolean
  busy: boolean
  asking: boolean
  save: (key: SettingKey, value: boolean, expected: boolean) => Promise<void>
}) {
  return (
    <>
      {app.problem && <p className="edit-error">The companion settings do not load: {app.problem}</p>}
      {COMPANION_SWITCHES.map(({ key, label }) => (
        <div key={key} className="user-settings-row" data-companion-item={key}>
          <span>{label}</span>
          <Toggle on={app[key]} label={label} pending={busy} disabled={asking || app.problem !== undefined || !canChange({ canLoosen }, `companion.${key}`, !app[key])} onToggle={() => void save(`companion.${key}`, !app[key], app[key])} />
        </div>
      ))}
    </>
  )
}

function CompanionShortcuts({ app }: { app?: CompanionAppSettings }) {
  const live = app?.running ? app.live : undefined
  return <>
    {live && <dl className="user-settings-live">
      <div data-companion-item="shortcut">
        <dt>{companionMenuItem('shortcut').label}</dt>
        <dd><kbd>{live.shortcut}</kbd>{live.shortcutProblem && <span className="muted"> {live.shortcutProblem}</span>}</dd>
      </div>
      <div data-companion-item="sendShortcut">
        <dt>{companionMenuItem('sendShortcut').label}</dt>
        <dd><kbd>{live.sendShortcut}</kbd>{live.sendShortcutProblem && <span className="muted"> {live.sendShortcutProblem}</span>}</dd>
      </div>
    </dl>}
    <details className="user-settings-shortcuts" data-companion-item="keyboardShortcuts">
      <summary>{companionMenuItem('keyboardShortcuts').label}</summary>
      <p className="muted">These keys work in the companion panel.</p>
      <dl className="user-settings-shortcut-help">
        {companionShortcutHelp(APPLE_PLATFORM).map((row) => <div key={row.id}>
          <dt>{row.label}<span>{row.keys.map((key) => <kbd key={key}>{key}</kbd>)}</span></dt>
          <dd>{row.description}</dd>
        </div>)}
      </dl>
    </details>
  </>
}

function CompanionApp({ app }: { app: CompanionAppSettings }) {
  const live = app.running ? app.live : undefined
  return <dl className="user-settings-live">
    {live && <div data-companion-item="startAtLogin">
      <dt>{companionMenuItem('startAtLogin').label}</dt>
      <dd>{live.startAtLoginProblem ?? (live.startAtLogin ? 'On' : 'Off')}</dd>
    </div>}
    {typeof app.checkForUpdates === 'boolean' && <div data-companion-item="checkForUpdates">
      <dt>{companionMenuItem('checkForUpdates').label}</dt>
      <dd>{app.problem !== undefined ? 'Unavailable' : app.checkForUpdates ? 'On' : 'Off'}</dd>
    </div>}
  </dl>
}
