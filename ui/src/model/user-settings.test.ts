import { describe, expect, it } from 'vitest'
import type { SettingsView } from '../contract'
import { approvalsStatus, canChange, loosens, modeConfirm, syncConfirm, syncFailure, syncResult } from './user-settings'

const doc = (patch: Partial<SettingsView['approvals']> = {}, rest: Partial<SettingsView> = {}): SettingsView => ({
  schemaVersion: 'settings/v1',
  configRoot: '/Users/you/.agnostic-ai',
  approvals: { mode: 'ask', modeFile: '', companion: false, overrides: { tools: 0, projects: 0, alwaysAsk: 0 }, shadowed: [], tools: [], ...patch },
  mailbox: { approveAgentSends: true },
  sync: { needed: false, reasons: [] },
  canLoosen: false,
  ...rest,
})

const name = (tool: string) => ({ claude: 'Claude Code', codex: 'Codex' })[tool] ?? tool

describe('user settings', () => {
  it('knows which changes loosen approvals', () => {
    expect(loosens('approvals.mode', 'yolo')).toBe(true)
    expect(loosens('approvals.mode', 'ask')).toBe(false)
    expect(loosens('approvals.companion', true)).toBe(true)
    expect(loosens('approvals.companion', false)).toBe(false)
    expect(loosens('mailbox.approveAgentSends', false)).toBe(true)
    expect(loosens('mailbox.approveAgentSends', true)).toBe(false)
  })

  it('locks only the loosening changes on a tab without the key', () => {
    expect(canChange(doc(), 'approvals.mode', 'ask')).toBe(true)
    expect(canChange(doc(), 'approvals.mode', 'yolo')).toBe(false)
    expect(canChange(doc({}, { canLoosen: true }), 'approvals.mode', 'yolo')).toBe(true)
    expect(canChange(doc(), 'mailbox.approveAgentSends', true)).toBe(true)
  })

  it('asks before review and yolo, not before ask', () => {
    expect(modeConfirm('ask')).toBeUndefined()
    expect(modeConfirm('review')?.go).toBe('Turn on')
    expect(modeConfirm('yolo')?.body).toMatch(/one at a time/)
  })

  it('says why the mode is not in effect, what has its own mode, and what a local block hides', () => {
    const reason = 'the hook is not installed; add approvals to builtins in the home config and run sync --scope user'
    const lines = approvalsStatus(doc({ mode: 'yolo', overrides: { tools: 2, projects: 1, alwaysAsk: 1 }, shadowed: ['approvals.tools'] }, { sync: { needed: true, reasons: [`claude: ${reason}`, `codex: ${reason}`] } }), name)
    expect(lines).toEqual([
      'Not in effect yet: the hook is not installed for Claude Code and Codex.',
      '2 tools and 1 folder have their own mode. Edit them in the home config.',
      '1 extra pattern always asks.',
      'Your local home config hides approvals.tools from the shared one.',
    ])
    expect(approvalsStatus(doc(), name)).toEqual([])
  })

  it('lists every file and note before a sync and says what it wrote after', () => {
    const question = syncConfirm({ id: 'x', files: [{ path: '/h/.codex/hooks.json', change: 'create' }], commands: ['hook staged: curl x | sh'], notes: ['a note'] })
    expect(question).toMatchObject({ files: ['create /h/.codex/hooks.json'], commands: ['hook staged: curl x | sh'], notes: ['a note'], go: 'Apply' })
    expect(question.body).toMatch(/all pending changes of your user configuration/)
    expect(syncConfirm({ id: 'x', files: [], commands: [], notes: [] }).go).toBeUndefined()
    expect(syncResult({ written: ['a', 'b'], notes: ['Trust it.'], settings: doc() })).toBe('Applied: 2 files written. Trust it.')
    expect(syncFailure('conflict.', 'agnostic-ai sync --scope user')).toBe('conflict. Run agnostic-ai sync --scope user in a terminal.')
  })
})
