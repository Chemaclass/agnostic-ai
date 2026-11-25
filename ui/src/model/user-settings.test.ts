import { describe, expect, it } from 'vitest'
import menu from '../../../companion/frontend/menu.json'
import type { SettingsView } from '../contract'
import { approvalsStatus, canChange, COMPANION_SECTIONS, COMPANION_SWITCHES, companionMenuItem, companionShortcutHelp, companionApprovalsNeedSetup, loosens, modeConfirm, syncConfirm, syncFailure, syncResult } from './user-settings'

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
    expect(loosens('companion.smallCompanion', false)).toBe(false)
    expect(loosens('companion.checkForUpdates', false)).toBe(true)
    expect(loosens('companion.checkForUpdates', true)).toBe(false)
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
      'Claude Code and Codex need setup.',
      '2 tools and 1 folder have their own mode. Edit them in the home config.',
      '1 extra pattern always asks.',
      'Your local home config hides approvals.tools from the shared one.',
    ])
    expect(approvalsStatus(doc(), name)).toEqual([])
  })

  it('keeps companion setup pending while a tool needs trust or settings changes', () => {
    const saved = doc({ companion: true, companionSetup: { claude: 'unavailable', codex: 'trust' } })
    expect(companionApprovalsNeedSetup(saved)).toBe(true)
    expect(approvalsStatus(saved, name)).toEqual([
      'Check permissions in Claude Code’s settings before answering from the companion.',
      'Open /hooks in Codex and trust the permission entry.',
    ])
    expect(companionApprovalsNeedSetup(doc({ companion: true }))).toBe(false)
  })

  it('requires a configured tool for companion permission requests', () => {
    const saved = doc({ companion: true, companionUnavailable: true })
    expect(companionApprovalsNeedSetup(saved)).toBe(true)
    expect(approvalsStatus(saved, name)).toEqual([
      'Add Claude Code or Codex to your configured tools to answer requests from the companion.',
    ])
  })

  it('lists every file and note before a sync and says what it wrote after', () => {
    const question = syncConfirm({ id: 'x', files: [{ path: '/h/.codex/hooks.json', change: 'create' }], commands: ['hook staged: curl x | sh'], notes: ['a note'] })
    expect(question).toMatchObject({ files: ['create /h/.codex/hooks.json'], commands: ['hook staged: curl x | sh'], notes: ['a note'], go: 'Apply' })
    expect(question.body).toMatch(/all pending changes to your tool settings/)
    expect(syncConfirm({ id: 'x', files: [], commands: [], notes: [] }).go).toBeUndefined()
    expect(syncResult({ written: ['a', 'b'], notes: ['Trust it.'], settings: doc() })).toBe('Applied: 2 files written. Trust it.')
    expect(syncFailure('conflict.', 'agnostic-ai sync --scope user')).toBe('conflict. Run agnostic-ai sync --scope user in a terminal.')
  })
})


describe('shared companion menu', () => {
  it('takes section order and setting labels from the native menu', () => {
    expect(COMPANION_SECTIONS).toEqual(menu.sections)
    expect(COMPANION_SWITCHES).toEqual(menu.items.filter((item) => item.section === 'appearance').map((item) => ({ key: item.id, label: item.label })))
    for (const item of menu.items) expect(companionMenuItem(item.id)).toEqual(item)
  })

  it('uses every fixed help row and expands the platform paste chord', () => {
    for (const isMac of [true, false]) {
      const expected = menu.shortcutHelp.map((row) => ({ ...row, keys: row.id === 'paste' ? (isMac ? ['⌘', '⇧', 'V'] : ['Ctrl', 'Shift', 'V']) : row.keys }))
      expect(companionShortcutHelp(isMac)).toEqual(expected)
    }
  })
})
