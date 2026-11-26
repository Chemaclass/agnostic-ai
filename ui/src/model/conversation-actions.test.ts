import { describe, expect, it } from 'vitest'
import type { Conversation } from '../contract'
import { canComplete, canRead, canRename, conversationMenuOf, deleteConfirmOf, deletedOf, deleteReason, freedOf, isArchived, renameOf, unchangedForMark } from './conversation-actions'

const row = (over: Partial<Conversation> = {}): Conversation => ({ tool: 'claude', app: 'cli', id: 'internal', title: 'Chat', state: 'active', updatedAt: '2026-11-07T12:00:00Z', open: { kind: 'terminal', label: 'Open' }, ...over })
const conflict = { id: 'c', worktree: '/repo', path: 'app.ts', requester: { tool: 'claude' as const, id: 'internal' } }
const menu = (over: Partial<Conversation> = {}, options: { localResume?: string; server?: boolean } = {}) => conversationMenuOf(row(over), { server: true, ...options })

describe('conversation menu', () => {
  it('offers the Companion items for each state, in menu order', () => {
    expect((['blocked', 'unread', 'working', 'active', 'completed', 'dormant'] as const).map((state) => menu({ state, ...(state === 'completed' ? { completedAt: 'now' } : {}) }))).toEqual([
      ['read', 'rename', 'complete', 'copy-session-id'],
      ['read', 'rename', 'complete', 'copy-session-id'],
      ['rename', 'copy-session-id'],
      ['rename', 'complete', 'copy-session-id'],
      ['rename', 'reopen', 'hide', 'copy-session-id', 'delete'],
      ['rename', 'hide', 'copy-session-id', 'delete'],
    ])
  })
  it('keeps Hide but drops Delete on a live archived row', () => {
    expect(menu({ state: 'completed', completedAt: 'now', live: true })).toEqual(['rename', 'reopen', 'hide', 'copy-session-id'])
    expect(menu({ state: 'dormant', deleteUnsupported: 'Stored in the cloud.' })).toContain('delete')
  })
  it('gives an archived row no read state', () => {
    expect(menu({ state: 'dormant', unread: true })).not.toContain('read')
    expect(menu({ state: 'active', unread: true })).toContain('read')
  })
  it('offers no Rename on Claude chat and Cowork rows or Codex cloud rows', () => {
    for (const over of [{ app: 'chat' as const }, { app: 'cowork' as const }, { tool: 'codex' as const, app: 'cloud' as const }]) expect(menu(over)).not.toContain('rename')
    expect(menu({ app: 'cloud' })).toContain('rename')
  })
  it('offers Open on claude.ai on a shared row and a local resume command only when the row has one', () => {
    expect(menu({ cloud: { id: 'cse_1', link: 'https://claude.ai/code/cse_1' } }, { localResume: 'claude --resume internal' })).toEqual(['rename', 'complete', 'web', 'copy-session-id', 'copy-resume'])
    expect(menu({}, {})).not.toContain('copy-resume')
  })
  it('offers Copy resume command on a Claude cloud row with cloudResumeCommand only', () => {
    expect(menu({ app: 'cloud', cloudResumeCommand: 'claude --teleport cse_1' })).toContain('copy-resume')
    expect(menu({ app: 'cloud', cloudResumeCommand: ' ' })).not.toContain('copy-resume')
    expect(menu({ tool: 'codex', app: 'cloud', cloudResumeCommand: 'codex resume' })).not.toContain('copy-resume')
  })
  it('offers Stop shell only with a stuck shell and a running server', () => {
    const stuckShells = [{ pid: 37, procStart: 'start', command: 'sleep 90', processCount: 1, since: '2026-11-07T12:00:00Z' }]
    expect(menu({ state: 'working', stuckShells })).toEqual(['rename', 'copy-session-id', 'stop-shell'])
    expect(menu({ state: 'working', stuckShells }, { server: false })).toEqual(['rename', 'copy-session-id'])
  })
  it('keeps a row with a path conflict in Needs you, with Mark complete and no archive actions', () => {
    const items = menu({ state: 'dormant', claimConflicts: [conflict] })
    expect(items).toEqual(['rename', 'complete', 'copy-session-id'])
    expect(menu({ state: 'dormant', completedAt: 'now', claimConflicts: [conflict] })).toEqual(['rename', 'reopen', 'copy-session-id'])
  })
})

describe('conversation actions', () => {
  it('keeps read, completion and archived eligibility distinct for all states', () => {
    expect((['blocked', 'unread', 'active', 'working', 'completed', 'dormant'] as const).map((state) => [canRead(row({ state })), canComplete(row({ state })), isArchived(row({ state }))])).toEqual([[true, true, false], [true, true, false], [false, true, false], [false, false, false], [false, false, true], [false, false, true]])
    expect(canRead(row({ unread: true }))).toBe(true)
    expect(canRead(row({ state: 'dormant', unread: true }))).toBe(false)
    expect(canComplete(row({ completedAt: 'now' }))).toBe(false)
    expect(canComplete(row({ state: undefined, live: true, activity: 'working' }))).toBe(true)
  })
  it('uses the displayed Needs you group for archived rows with file conflicts', () => {
    const conflicted = row({ state: 'dormant', claimConflicts: [conflict] })
    expect(isArchived(conflicted)).toBe(false)
    expect(canComplete(conflicted)).toBe(true)
    expect(canRead(conflicted)).toBe(false)
    expect(deleteReason(conflicted)).toBeTruthy()
    expect(canComplete({ ...conflicted, state: 'working' })).toBe(false)
    expect(canRead({ ...conflicted, unread: true })).toBe(true)
  })
  it('matches Companion rename exclusions', () => {
    expect(canRename(row({ app: 'chat' }))).toBe(false)
    expect(canRename(row({ app: 'cowork' }))).toBe(false)
    expect(canRename(row({ tool: 'codex', app: 'cloud' }))).toBe(false)
    expect(canRename(row({ app: 'cloud' }))).toBe(true)
  })
  it('decides what Enter does with a typed name', () => {
    expect(renameOf('  Parser   release ', 'Parser')).toEqual({ title: 'Parser release' })
    expect(renameOf(' Parser ', 'Parser')).toEqual({ keep: true })
    expect(renameOf('   ', 'Parser')).toEqual({ empty: true })
  })
  it('refuses unarchived, live and server-unsupported deletes', () => {
    expect(deleteReason(row())).toBeTruthy()
    expect(deleteReason(row({ state: 'completed', live: true }))).toBeTruthy()
    expect(deleteReason(row({ state: 'dormant', deleteUnsupported: 'Stored in the cloud.' }))).toBe('Stored in the cloud.')
    expect(deleteReason(row({ state: 'completed' }))).toBeUndefined()
  })
  it('words the delete confirmation and its result', () => {
    expect(deleteConfirmOf(row({ title: 'Parser' })).title).toBe('Delete "Parser" permanently?')
    expect(deleteConfirmOf(row({ tool: 'codex' })).body).toBe('This removes the conversation from this computer. It cannot be undone.')
    expect([undefined, 0, 512, 12_345_678].map(freedOf)).toEqual(['', '', 'Freed 512 bytes', 'Freed 12 MB'])
    expect(deletedOf({ deleted: true })).toBe('Deleted.')
    expect(deletedOf({ deleted: true, freedBytes: 4200, note: 'A backup stayed.' })).toBe('Deleted. Freed 4.2 KB. A backup stayed.')
  })
  it('skips changed rows in a group action, including renewed unread marks and completion changes', () => {
    const shown = row({ state: 'unread' })
    expect(unchangedForMark(shown, { ...shown }, 'seen')).toBe(true)
    for (const changed of [undefined, row({ state: 'working' }), { ...shown, updatedAt: 'later' }, { ...shown, seenAt: 'later' }, { ...shown, completedAt: 'later' }, { ...shown, live: true }, { ...shown, waiting: { for: 'permission' as const } }, { ...shown, claimConflicts: [conflict] }]) expect(unchangedForMark(shown, changed, 'seen')).toBe(false)
    expect(unchangedForMark(row(), row(), 'completed')).toBe(true)
  })
  it('treats missing-state rows by liveness', () => {
    expect(isArchived(row({ state: undefined, live: false }))).toBe(true)
    expect(canRead(row({ state: undefined, live: false, unread: true }))).toBe(false)
    expect(isArchived(row({ state: undefined, live: true }))).toBe(false)
    expect(canRead(row({ state: undefined, live: true, unread: true }))).toBe(true)
  })
})
