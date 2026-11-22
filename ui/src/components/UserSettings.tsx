import { useCallback, useEffect, useId, useRef, useState } from 'react'
import type { SettingKey, SettingsSyncPlan, SettingsView } from '../contract'
import { applySync, planSync, readSettings, saveSetting, SettingsError } from '../settings-api'
import { approvalsStatus, canChange, LOCKED_HINT, MODE_CHOICES, modeConfirm, syncConfirm, syncFailure, syncResult } from '../model/user-settings'
import { useApp } from './app-context'
import { ConfigSwitch } from './ConfigSwitch'
import { useServedConfig } from './Shell'
import { SectionHeading } from './SectionHeading'
import { Toggle, toolName } from './ui'

/** How often an open tab reads the settings again, so a change made in the companion shows. */
const READ_EVERY_MS = 2000

interface Question {
  title: string
  body: string
  files?: string[]
  notes?: string[]
  go?: string
  run?: () => void | Promise<void>
}

/**
 * The approvals and mailbox settings of the user-level configuration,
 * the same ones the companion's menu shows. A tab that view did not open
 * through its unlock file can tighten them but not loosen them.
 */
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
    setSaid('Checking what a sync would change…')
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
  return (
    <section className="block user-settings" aria-labelledby="settings-user">
      <SectionHeading id="settings-user" focusable>Approvals and companion</SectionHeading>
      <p className="muted">Saved in your home config, the same settings the companion's menu shows. A change in either place shows in the other within seconds.</p>
      {unread && <p className="edit-error" role="alert">The settings could not be read again: {unread}</p>}
      {locked && <p className="user-settings-locked">{LOCKED_HINT} Changes that make approvals stricter work here.</p>}

      <fieldset className="user-settings-group" disabled={busy || question !== undefined}>
        <legend>When a tool asks permission</legend>
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

      <div className="user-settings-row">
        <span>Approve and deny from the companion list</span>
        <Toggle on={companionOn} label="Approve and deny from the companion list" pending={busy} disabled={question !== undefined || !canChange(doc, 'approvals.companion', !companionOn)} onToggle={() => void save('approvals.companion', !companionOn, companionOn)} />
      </div>

      {status.length > 0 && (
        <div className="user-settings-status">
          {status.map((line) => <p key={line}>{line}</p>)}
        </div>
      )}
      {doc.sync.needed && !question && (doc.canLoosen
        ? <button type="button" className="button button-primary" disabled={busy} onClick={() => void sync()}>Apply to my tools</button>
        : <p className="muted">Run <code>agnostic-ai sync --scope user</code> in a terminal to put it in effect.</p>)}

      {question && (
        <div className="user-settings-confirm" role="group" aria-labelledby={`${id}-question`}>
          <p id={`${id}-question`} className="user-settings-confirm-title">{question.title}</p>
          <p>{question.body}</p>
          {question.files && question.files.length > 0 && <ul className="user-settings-files">{question.files.map((file) => <li key={file}><code>{file}</code></li>)}</ul>}
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
        <>
          <h3 className="user-settings-subheading">Mailbox</h3>
          <div className="user-settings-row">
            <span>Ask me before an agent sends a message</span>
            <Toggle on={doc.mailbox.approveAgentSends} label="Ask me before an agent sends a message" pending={busy} disabled={question !== undefined || !canChange(doc, 'mailbox.approveAgentSends', !doc.mailbox.approveAgentSends)} onToggle={() => void save('mailbox.approveAgentSends', !doc.mailbox!.approveAgentSends, doc.mailbox!.approveAgentSends)} />
          </div>
        </>
      )}
    </section>
  )
}
